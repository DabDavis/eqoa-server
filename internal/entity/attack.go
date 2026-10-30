package entity

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// attack.go — a player's auto-attack, run on the server the way CLIENT and
// Sandstorm run it.
//
// CLIENT's attack button is VIClient::ToggleAutoAttack (0x00BD59E8 [V]): the
// press sends 0x000F (u32 target) — once, and only when its own swing is due
// — and sets m_autoAttack, which the 0x40 carries in byte 36; the release
// (MaybeStopAutoAttack [V]) clears it unless the auto-attack option makes the
// button a toggle. CLIENT never sends another message per swing: in reach,
// AutoAttackTick (0x00BC76A0 [V]) only plays the swing's own effect locally;
// out of reach it sends a bare 0x00D8 and tries again in 2 s. The hits are
// the server's to deal.
//
// What Sandstorm does with that, measured live on 2026-10-16 [C]
// (claude-notes/go-client-egress-pass-20261016.md, "Live"):
//   - the first hit comes about one swing delay after the 0x000F (7 s for a
//     6 s swing), as bare 0x00DB damage — no 0x00B1 for the player's own
//     swing (the NPC's swings get one each);
//   - a TAP — byte 36 back at 0 well before that first swing — starts no
//     fight;
//   - a HELD button does, and once the NPC fights back the fight goes on
//     after the release (two more hits after it in the live run, and the
//     same between a PS2's taps in pcaps/ps2_main_capture.pcap).
//
// So the rule here: a 0x000F on an attackable NPC engages it, its first
// round counted from the press and begun if the button is still down a
// moment later (firstRound); a round begins only with byte 36 up and the
// player in reach (CLIENT's TargetInRange), and waits for both, the fight
// kept. A change of target (0x0010), the NPC's death or the player's, or the
// NPC gone from view ends it.

// Swing kinds, CLIENT's GetSwing (0x00CBDEF8 [A]); VISwing::Effect
// (0x00CBE0A8 [A]) names a spell effect for each and VISwing::Delay
// (0x00CBE190 [A]) the time to the next swing. Ported from the Go client's
// internal/game/combat.go, which a MIPS interpreter over CLIENT's own code
// checked on 27,695 inputs.
//
//	kind  effect   delay   kind  effect   delay
//	0     Attack1  3000    6     Attack3  4000
//	1     Attack1  3500    7     Attack3  5000
//	2     Attack3  4000    8     Range1   3500
//	3     Attack3  5000    9     Range2   5000
//	4     Attack2  4000    10    Range3   5000
//	5     Attack4  6000    11    Range4   5000
//	12    none     3000
const swingNone = 12

// GetSwing is CLIENT's GetSwing: the swing a class makes at a level with
// what is worn in equip slots 14 (primary), 15 (secondary), 16 (two hands),
// 18 (ranged), 17 (shield), 19 and 20 (held).
func GetSwing(cls, level int, worn [21]bool) int {
	b14, b15, b16, b17, b18, b19, b20 := worn[14], worn[15], worn[16], worn[17], worn[18], worn[19], worn[20]
	if b16 {
		if b14 || b15 || b18 {
			return 0
		}
	} else if b18 && (b14 || b15) {
		return 0
	}
	low := func() int {
		switch {
		case b16:
			return 1
		case b19:
			return swingNone
		}
		return 0
	}
	ranged := func() int {
		switch {
		case level < 20:
			return 8
		case level < 35:
			return 9
		}
		return 11
	}
	mid := func() int {
		switch {
		case b17:
			return 0
		case !b20:
			return 4
		}
		return 0
	}
	high := func() int {
		switch {
		case b19:
			return swingNone
		case b17:
			return 2
		case !b20:
			return 5
		}
		return 2
	}
	switch cls {
	case 7, 8, 9, 10, 11, 12, 13, 14: // DRD, SHA, CL, MAG, NEC, ENC, WIZ, ALC
		if b18 {
			return 8
		}
		return low()
	case 2, 3: // PAL, SK
		switch {
		case b18:
			return ranged()
		case level < 20:
			return low()
		case b16 && level < 35:
			return 3
		case b16:
			return 7
		case b19:
			return swingNone
		}
		return 2
	case 0, 1, 4: // WAR, RAN, MNK
		switch {
		case b18:
			return ranged()
		case level < 20:
			return low()
		case level < 35:
			if b16 {
				return 3
			}
			if b19 {
				return swingNone
			}
			return mid()
		case b16:
			return 7
		}
		return high()
	case 5, 6: // BRD, RGE
		switch {
		case b18:
			return ranged()
		case level < 20:
			return low()
		case b16:
			return 1
		case level < 35:
			if b19 {
				return swingNone
			}
			return mid()
		}
		return high()
	}
	return 0
}

// SwingDelay is VISwing::Delay.
func SwingDelay(kind int) time.Duration {
	ms := 3000
	switch kind {
	case 1, 8:
		ms = 3500
	case 2, 4, 6:
		ms = 4000
	case 3, 7, 9, 10, 11:
		ms = 5000
	case 5:
		ms = 6000
	}
	return time.Duration(ms) * time.Millisecond
}

// SwingEffectName is VISwing::Effect's table: the name it hashes.
func SwingEffectName(kind int) string {
	switch kind {
	case 0, 1:
		return "Attack1"
	case 4:
		return "Attack2"
	case 2, 3, 6, 7:
		return "Attack3"
	case 5:
		return "Attack4"
	case 8:
		return "Range1"
	case 9:
		return "Range2"
	case 10:
		return "Range3"
	case 11:
		return "Range4"
	}
	return ""
}

// HashResourceID is VIHashResourceID: h = h*131 + (signed) byte.
func HashResourceID(name string) uint32 {
	var h uint32
	for i := 0; i < len(name); i++ {
		h = h*131 + uint32(int32(int8(name[i])))
	}
	return h
}

// meleeReach is TargetInRange's melee distance (and a bare hand's), before
// the target's own allowance.
const meleeReach = 4.5

// Swing is what one engagement swings with: the kind GetSwing picked, its
// delay, and how far it reaches.
type Swing struct {
	Kind  int
	Delay time.Duration
	Reach float32 // before the target's allowance (npcReachExtra)
	Never bool    // the swinging item's attack type is never in reach
	// Potion is the item id of a potion worn as the thrown weapon, 0 for
	// none, and PotionLeft how many of it are on the stack (potion.go)
	Potion, PotionLeft int
}

// SwingFor is the swing CLIENT's AutoAttackTick would make with this gear:
// worn[e] for equip slots 14..20, and the first swinging item (slots 14,
// 16, 18 in inventory order) — its attack type and range — when there is one.
// TargetInRange (0x00BDC4B8 [V]): no item reaches 4.5 (bare hands); attack
// types 1-6 reach 4.5, 7, 9 and 10 the item's range; any other type never.
func SwingFor(cls, level int, worn [21]bool, hasItem bool, attackType, itemRange int) Swing {
	kind := GetSwing(cls, level, worn)
	s := Swing{Kind: kind, Delay: SwingDelay(kind), Reach: meleeReach}
	if !hasItem {
		return s
	}
	switch attackType {
	case 1, 2, 3, 4, 5, 6:
	case 7, 9, 10:
		s.Reach = float32(itemRange)
	default:
		s.Never = true
	}
	return s
}

// dist3 is the distance between two places.
func dist3(ax, ay, az, bx, by, bz float32) float32 {
	dx, dy, dz := ax-bx, ay-by, az-bz
	return float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
}

// inReach is TargetInRange's distance test: the squared 3D distance within
// the reach plus the target's allowance.
func inReach(p *Player, npc *NPC) bool {
	if p.Swing.Never {
		return false
	}
	r := p.Swing.Reach + npcReachExtra(npc)
	dx, dy, dz := p.X-npc.X, p.Y-npc.Y, p.Z-npc.Z
	return dx*dx+dy*dy+dz*dz <= r*r
}

// EngageAttack is the server's half of a 0x000F: the player engages the NPC
// it names and its first swing comes one delay from now. A 0x000F on the
// target already engaged keeps the swing that is due (CLIENT sends one per
// swing of its own). It reports whether a fight is on.
func (w *World) EngageAttack(sess *session.Session, objectID uint32, swing Swing, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return false
	}
	ent, found := w.findEntityByObjectIDLocked(sess, objectID)
	if !found {
		return false
	}
	if w.duelEngageLocked(p, ent, objectID, swing, now) { // a duel's opponent or its pet (duel.go)
		return true
	}
	npc, ok := ent.(*NPC)
	if !ok || npc.Dead || npc.NPCType&0x80 != 0 || npc.Owner != 0 {
		// CLIENT's ToggleAutoAttack refuses a not-attackable creature
		// (+0x25A & 0x80) itself; TargetInRange refuses a player. A pet is
		// on the players' side (pet.go): its byte 142 keeps CLIENT from
		// swinging at it, though the 0x000F still comes.
		return false
	}
	p.Swing = swing
	if p.AttackTarget == npc.ServerID && p.AttackObjectID == objectID {
		return true
	}
	p.AttackTarget = npc.ServerID
	p.AttackObjectID = objectID
	p.RoundStart, p.RoundHits = time.Time{}, 0
	// the first round is the press's: its hits on the press's clock, its
	// button read a moment later (firstRound)
	p.NextSwing = now
	p.ButtonReadAt = now.Add(firstRound)
	p.roundWaits = false
	w.breakInvisLocked(p, "an attack") // invis.go
	log.Printf("Combat: %s engages %s (swing %d, %v, reach %.1f)", p.Name, npc.Name, swing.Kind, swing.Delay, swing.Reach)
	return true
}

// StopAttackOnRetarget ends the player's engagement when a 0x0010 names any
// other target: CLIENT's every target change (SetTarget, SelectTarget,
// CycleTarget) turns auto-attack off.
func (w *World) StopAttackOnRetarget(sess *session.Session, objectID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && p.AttackTarget != 0 && p.AttackObjectID != objectID {
		p.disengage()
	}
}

func (p *Player) disengage() {
	p.AttackTarget = 0
	p.AttackObjectID = 0
	p.NextSwing = time.Time{}
	p.ButtonReadAt = time.Time{}
	p.roundWaits = false
	p.RoundStart, p.RoundHits = time.Time{}, 0
}

// Attacking reports whether the player's auto-attack is on now: a fight
// with the attack button down, or a round in hand. A fight kept while the
// button is up (attackSystem waits for it) is not: what ends on an attack
// (a meal, a potion's effect) and an aggressive pet go by this.
func (p *Player) Attacking() bool {
	if p.AttackTarget == 0 {
		return false
	}
	pressed := p.RoundStart.IsZero() && !p.roundWaits // a 0x000F's first round, not read yet
	inHand := !p.RoundStart.IsZero() && p.RoundHits < len(swingHits(p.Swing.Kind))
	return p.AutoAttack || pressed || inHand
}

// firstRound is how long after its 0x000F the first round's button is
// read. The round itself is the press's: its hits on the press's clock, the
// first 0.5 s after it (Attack1's), as the client's own swing, which plays
// at the press (AutoAttackTick). Sandstorm's first hit came 0.56 to 0.76 s
// after the 0x000F in 17 of 20 fights, round trip included, and a tap, the
// button up again 0.2 s later, started none: a round read 0.25 s in and
// counted from the press is both (F3 20261026-015610: counted from the read,
// as until then, every hit landed 0.25 s after the client's swing struck).
const firstRound = 250 * time.Millisecond

// attackSystem runs every engaged player's melee in rounds, as Sandstorm's
// (rounds.go): a round every swing delay, its hits at the swing's times
// (swingHits), each in reach a hit (playerHitLocked) and out of it lost. A
// round begins only with the attack button held (0x40 byte 36) and the
// target in reach, and waits for both with the fight kept; the round in hand
// lands whole, the button up or not, as Sandstorm's did (a bard's hits went
// on 1.15 s past the release, and no further, the training dummy still
// hitting him), and a press between the client's swings, which sends no
// 0x000F, takes the fight up again. The player's own swing is played by its
// client: no 0x00B1. Every tick; must be called with the lock held.
func (w *World) attackSystem(now time.Time) {
	for _, p := range w.Players {
		if p.AttackTarget == 0 {
			continue
		}
		if w.duelRoundLocked(p, now) { // a duel's melee (duel.go)
			continue
		}
		npc := w.NPCs[p.AttackTarget]
		if npc == nil || npc.Dead || npc.Owner != 0 || p.IsDead() {
			// gone, dead, or someone's pet now (a charm: charm.go)
			p.disengage()
			continue
		}
		if !p.RoundStart.IsZero() && w.playerRoundHitsLocked(p, npc, now) {
			continue // the fight ended
		}
		if now.Before(p.NextSwing) || now.Before(p.ButtonReadAt) {
			continue
		}
		if !p.AutoAttack || !inReach(p, npc) {
			// The round waits, and begins with the button down again (a tap
			// that never held, a release, a toggled button) and in reach.
			// The fight is kept: CLIENT sends no new 0x000F for a press that
			// comes between its swings, so a fight let go on a release never
			// came back while the client swung on (F3 20261026-020605: one
			// hit, then only the swings).
			p.roundWaits = true
			continue
		}
		// at its due time, not the tick's: a steady cadence; one that waited
		// begins with this tick
		start := p.NextSwing
		if p.roundWaits {
			start, p.roundWaits = now, false
		}
		p.RoundStart, p.RoundHits = start, 0
		p.NextSwing = p.RoundStart.Add(p.Swing.Delay)
		w.breakInvisLocked(p, "a swing") // invis.go
		w.playerRoundHitsLocked(p, npc, now)
	}
}

// playerRoundHitsLocked lands the hits of the player's round that are due.
// It reports whether the fight ended (a kill, or the NPC out of view).
func (w *World) playerRoundHitsLocked(p *Player, npc *NPC, now time.Time) bool {
	hits := swingHits(p.Swing.Kind)
	for p.RoundHits < len(hits) && !now.Before(p.RoundStart.Add(hits[p.RoundHits])) {
		p.RoundHits++
		if !inReach(p, npc) {
			continue // swung at a target that stepped away
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess == nil {
			p.disengage()
			return true
		}
		objectID := w.objectIDForEntityLocked(sess, npc.ServerID)
		if objectID == 0 {
			p.disengage() // gone from the player's view
			return true
		}
		if w.playerHitLocked(sess, p, npc, objectID) {
			p.disengage()
			return true
		}
	}
	return false
}

// playerHitLocked is one landed player swing: Sandstorm's roll on the worn
// weapon, STR, DEX and the target (offence.go), the damage figure to the
// attacker (0x00DB, attacker → NPC, negative), the NPC's threat (so it
// fights back), and on a kill the XP and the faction hits (faction.go). It
// reports a kill.
func (w *World) playerHitLocked(sess *session.Session, attacker *Player, npc *NPC, objectID uint32) bool {
	if w.apartLocked(attacker, npc) {
		w.tellApartLocked(sess, attacker, npc, time.Now()) // water.go
		return false
	}
	if attacker.Swing.Potion != 0 {
		return w.thrownPotionHitLocked(sess, attacker, npc, objectID) // potion.go
	}
	damage := w.dealtLocked(npc, attacker, playerMeleeHit(attacker, npc, meleeIntn)) // balance.go
	if npc.Dummy != NotDummy {
		// told, taken off nothing; the dummy fights back (dummy.go)
		now := time.Now()
		attacker.LastCombatTime = now
		w.sendDamageIndication(sess, sess.PlayerObjectID(), -damage, objectID)
		w.dummyEngagedLocked(npc, attacker, now)
		w.playerProcsLocked(sess, attacker, npc, now)
		return false
	}
	npc.HP -= damage
	if npc.HP < 0 {
		npc.HP = 0
	}
	killed := npc.HP <= 0
	attacker.LastCombatTime = time.Now()

	w.sendDamageIndication(sess, sess.PlayerObjectID(), -damage, objectID)
	log.Printf("Combat: %s hit %s for %d damage (HP: %d/%d) %.2f off, reach %.2f", attacker.Name, npc.Name, damage, npc.HP, npc.MaxHP,
		dist3(attacker.X, attacker.Y, attacker.Z, npc.X, npc.Y, npc.Z), attacker.Swing.Reach+npcReachExtra(npc))

	w.updateAggro(npc, attacker.ServerID, damage)
	w.rootBreakLocked(npc, attacker.Level) // root.go
	if killed {
		w.killNPC(npc)
		w.awardKillXPLocked(sess, attacker, npc) // killexp.go
		w.factionKillLocked(sess, attacker, npc)
		return true
	}
	// a proc's damage after the hit (proc.go)
	return w.playerProcsLocked(sess, attacker, npc, attacker.LastCombatTime)
}

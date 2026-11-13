package entity

import (
	"fmt"
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
// moment later (firstRound); a round begins only with byte 36 up, and waits
// for it, the fight kept. A change of target (0x0010), the NPC's death or
// the player's, or the NPC gone from view ends it.
//
// The rounds keep the client's own swing timer (m_p071E8), the beat its
// swings play to, which the server is told of twice: a 0x000F comes only
// with a press whose timer has run out, and starts the timer again from the
// press (pressed); a 0x00D8 is a due swing at a target out of the client's
// reach, no swing's effect played and the timer 2 s on (SwingOutOfReach). Between the
// two the timer goes a swing's delay a swing. The server's own reach test
// decides only whether each hit lands, never when a round begins: begun
// again from the tick its view of the reach came back, the rounds kept a
// beat of their own while the client's swings kept theirs, and the damage
// came anywhere in the client's swing for the rest of the fight (the user,
// 2026-10-30: "the animation and damage don't sync well for auto attacks
// especially on eqoaclassic", whose creatures chase and step about; a press
// a little out of reach, the client swinging 2 s later, was enough).
//
// A press with the timer still running sends nothing: ToggleAutoAttack only
// sets m_autoAttack, and AutoAttackTick swings at the target when the timer
// runs out, on and on, with no 0x000F at all. With the fight already on that
// is the round waiting for the button; with none on, the next target after a
// kill or a change of target within a swing's delay, it was a client swinging
// for nothing until its player let the button go long enough for the timer to
// run out (petition 1112: "switching targets or un con'ing target results in
// combat breaking. you have to go into menus unequip weapon and reequip it.
// then damage resumes"; a level 60 ranger's timer is 6 s, and the menus are
// what let it run out). So the button up (0x40 byte 36, which CLIENT clears on
// every change of target and sets only with a press) on the target it has named
// (0x0010) is a press too, at the timer's end: pressUnsentLocked.

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
	// Held is the name of the held item the hands hold when they hold no
	// weapon (equip slot 19 or 20 worn, none of 14, 16 or 18; Never is set
	// with it): such hands never swing, and a press is refused with
	// heldNoFightWords. Petition 1435 (Afk, a wizard holding Swampstench,
	// punched for 1), the GM Jibrish's rule: hitting with a held item is
	// "supposed to be no". CLIENT's own AutoAttackTick swings bare hands so.
	Held string
}

// heldNoFightWords is a press refused for hands holding a held item and no
// weapon (Swing.Held), said at most once every heldNoFightGap.
const (
	heldNoFightWords = "You can't fight while holding %s."
	heldNoFightGap   = 2 * time.Second
)

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
// it names, its first round the press's. CLIENT sends one only with its
// swing timer run out (ToggleAutoAttack: m_p071E8 <= now), and starts the
// timer from the press, so a 0x000F on the target already engaged starts
// the rounds from it too. It reports whether a fight is on.
func (w *World) EngageAttack(sess *session.Session, objectID uint32, swing Swing, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return false
	}
	return w.engageLocked(sess, p, objectID, swing, now)
}

// engageLocked is a press on the object named, a 0x000F's or one that sent
// none (pressUnsentLocked): the player engages it if it may be fought.
func (w *World) engageLocked(sess *session.Session, p *Player, objectID uint32, swing Swing, now time.Time) bool {
	ent, found := w.findEntityByObjectIDLocked(sess, objectID)
	if !found {
		return false
	}
	if swing.Held != "" {
		w.heldRefusalLocked(sess, p, swing.Held, now)
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
	if line := challengeRefusal(npc, p.ServerID); line != "" {
		w.tellChallengeLocked(sess, p, line, now) // another's duel (metalmoogie.go)
		return false
	}
	p.Swing, p.swingNew = swing, false
	if p.AttackTarget == npc.ServerID && p.AttackObjectID == objectID {
		p.pressed(now) // a press with the client's timer run out: its timer from the press
		return true
	}
	p.AttackTarget = npc.ServerID
	p.AttackObjectID = objectID
	p.pressed(now)
	w.breakInvisLocked(p, "an attack")     // invis.go
	w.breakBoardWolfLocked(p, "an attack") // boardwolf.go
	log.Printf("Combat: %s engages %s (swing %d, %v, reach %.1f)", p.Name, npc.Name, swing.Kind, swing.Delay, swing.Reach)
	return true
}

// StopAttackOnRetarget ends the player's engagement when a 0x0010 names any
// other target: CLIENT's every target change (SetTarget, SelectTarget,
// CycleTarget) turns auto-attack off, which its next 0x40 says and the button's
// copy here says now, so a press cannot be read off the old target's button
// (pressUnsentLocked).
func (w *World) StopAttackOnRetarget(sess *session.Session, objectID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if p.TargetObjectID != objectID {
		p.AutoAttack = false
	}
	if p.AttackTarget != 0 && p.AttackObjectID != objectID {
		p.disengage()
	}
}

// unsentPressWait is how long the button may be up, its timer run out as the
// server follows it, before the press is taken as one that sent no 0x000F: a
// press with the timer run out sends its 0x000F ahead of the 0x40 that shows the
// button, and only a timer the server lost sight of (a restart's) leaves none to
// come.
const unsentPressWait = 500 * time.Millisecond

// pressUnsentLocked is a press CLIENT sent no 0x000F for (the file's doc): with
// no fight on and the button up on the target the client named, the player
// engages it when the client's own swing comes, its round on that swing's beat,
// with the hands' swing (HandsChanged). The button let go by then is a tap and
// starts nothing, as AutoAttackTick's own test has it. It reports whether a
// fight began. Must be called with the lock held.
func (w *World) pressUnsentLocked(p *Player, now time.Time) bool {
	if !p.AutoAttack || p.TargetObjectID == 0 || p.TargetObjectID == noTargetObject || p.Swing.Delay <= 0 || p.IsDead() {
		return false
	}
	due := p.swingTimer
	if now.Before(due) || (!p.buttonUpAt.Before(due) && now.Sub(p.buttonUpAt) < unsentPressWait) {
		return false // the client has not swung yet, or its 0x000F is on the way
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil || !w.engageLocked(sess, p, p.TargetObjectID, p.Swing, now) {
		return false
	}
	start := now
	if now.Sub(due) <= buttonLate {
		start = due // the client's swing's beat, the tick a little after it
	}
	p.NextSwing, p.swingTimer, p.ButtonReadAt = start, start, start
	log.Printf("Combat: %s's press sent no 0x000F: its rounds from its swing timer", p.Name)
	return true
}

// noTargetObject is the object id a 0x0010 names no target by.
const noTargetObject uint32 = 0xFFFFFFFF

// pressed is a 0x000F: CLIENT's swing timer from the press (ToggleAutoAttack:
// m_p071E8 = now), the first round the press's, its hits on the press's
// clock and its button read a moment later (firstRound); a round in hand is
// the timer's last swing's and goes.
func (p *Player) pressed(now time.Time) {
	start := now
	if early := p.swingTimer.Add(-pressSlack); now.Before(early) {
		// a press before the client's timer ran out, which CLIENT never
		// sends (ToggleAutoAttack): its round waits for the timer, so a
		// press cannot swing sooner than the weapon does (until 2026-11-06
		// only the 0x000F's rate limit held that, at 2.5 s whatever the
		// swing's delay: handlers/security.go)
		start = early
	}
	p.RoundStart, p.RoundHits = time.Time{}, 0
	p.NextSwing = start
	p.swingTimer = start
	p.ButtonReadAt = now.Add(firstRound)
	p.roundWaits = false
}

// pressSlack is how much sooner than the server's copy of the client's swing
// timer a press may start a round: the server begins a round when the press's
// button is read (firstRound) and a 0x40 a slot old, so its copy runs a little
// behind the client's own.
const pressSlack = 500 * time.Millisecond

// outOfReachRetry is how long CLIENT's AutoAttackTick waits after a due
// swing at a target out of its reach before it looks again (its timer's
// 2000).
const outOfReachRetry = 2 * time.Second

// buttonLate is how long after a round was due the button may come down
// with the round still on the client's beat: a press just before the
// client's swing, which sends no 0x000F (its timer runs), reaches the
// server in a 0x40 that left at its next slot (101.7 ms) and may come a
// tick or two after the round was due. Later than that, the press came
// after the client's timer ran out, and its 0x000F starts the rounds again
// (pressed); until it comes the round begins with the button.
const buttonLate = 300 * time.Millisecond

// SwingOutOfReach is a 0x00D8: CLIENT's AutoAttackTick found its due swing's
// target out of its reach (TargetInRange), played no swing's effect and put its
// timer 2 s on. It still plays the swing's animation on its own sprite (SetAction,
// but for a ranged weapon or a swing with no effect: viclient.cpp), so the player
// sees itself swing every 2 s while no one else is told of it (no round, no
// 0x00B1): a swing only the swinger sees is one out of its reach. A round begun
// for that swing, none of its hits come yet, is called off, and the next round is
// the client's next look, 2 s on (and a 0x00D8 that comes as the round is due puts
// it there before it begins).
func (w *World) SwingOutOfReach(sess *session.Session, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if p.AttackTarget == 0 { // the timer goes on with no fight, a press's that sent nothing (pressUnsentLocked)
		p.swingTimer = now.Add(outOfReachRetry)
		return
	}
	if !p.RoundStart.IsZero() && p.RoundHits == 0 {
		p.RoundStart = time.Time{}
	}
	p.NextSwing = now.Add(outOfReachRetry)
	p.swingTimer = p.NextSwing
	p.roundWaits = false
}

// roundStartLocked is when the player's next round begins, if it begins at
// this tick: at its due time (the client's beat) with the button down,
// waiting for the button with the fight kept (a tap that never held, a
// release, a toggled button: CLIENT sends no new 0x000F for a press between
// its swings, F3 20261026-020605), then on the beat when the button came
// within buttonLate of it, else from now. Not by the reach: see the file's
// doc.
func (p *Player) roundStart(now time.Time) (time.Time, bool) {
	if now.Before(p.NextSwing) || now.Before(p.ButtonReadAt) {
		return time.Time{}, false
	}
	if !p.AutoAttack {
		p.roundWaits = true
		return time.Time{}, false
	}
	start := p.NextSwing
	if p.roundWaits && now.Sub(start) > buttonLate {
		start = now
	}
	p.roundWaits = false
	return start, true
}

func (p *Player) disengage() {
	p.newRound() // the hands it holds now, for a press that sends no 0x000F
	p.AttackTarget = 0
	p.AttackObjectID = 0
	p.NextSwing = time.Time{}
	p.ButtonReadAt = time.Time{}
	p.roundWaits = false
	p.RoundStart, p.RoundHits = time.Time{}, 0
	p.swingNew = false
}

// HandsChanged is a change of the player's hands (an item worn or taken
// off, a weapon set, the switch for an art: handlers'), with the swing CLIENT
// would make with what it holds now. A fight in progress swings with it
// from its next round, as the client's does: AutoAttackTick (0x00BC76A0)
// picks its swing from the hands as they are at every due swing and puts its
// timer on by that swing's delay, so the round in hand lands as it was
// swung and the next comes on the client's beat with the new hands. Until
// 2026-11-01 the swing was the engagement's for the whole fight (the next
// 0x000F's: one comes only with the attack button pressed again), so a
// player who took up a bow fought on with its blades' reach and rounds. Out
// of a fight it is what a press that sends no 0x000F swings with
// (pressUnsentLocked); the next 0x000F brings the hands' own.
func (w *World) HandsChanged(sess *session.Session, swing Swing) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if p.AttackTarget == 0 {
		p.Swing, p.swingNew = swing, false
		return
	}
	if swing == p.Swing { // the swing it makes now (armour changed, or the hands back): nothing waits
		p.swingNew = false
		return
	}
	p.swingNext, p.swingNew = swing, true
	log.Printf("Combat: %s's hands changed: the next round swings %d (%v, reach %.1f)", p.Name, swing.Kind, swing.Delay, swing.Reach)
}

// newRound takes up the swing the hands changed to (HandsChanged), at a
// round's start.
func (p *Player) newRound() {
	if p.swingNew {
		p.Swing, p.swingNew = p.swingNext, false
	}
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

// broadcastPlayerSwingLocked shows everyone who sees the player, but the player,
// its round's swing: 0x00B1 with the swing's effect at its target, as a creature's
// (rounds.go). The swinger's own client plays its own (AutoAttackTick) and is sent
// none, as Sandstorm's was; nothing told anyone else, so to them a player in a fight
// stood still while its target's hit points fell (the GM Jibrish, watching two players
// fight: "im not seeing their auto attack animations fire off reliably"). Must be
// called with the lock held.
func (w *World) broadcastPlayerSwingLocked(p *Player, target EntityID) {
	name := SwingEffectName(p.Swing.Kind)
	if name == "" || w.SessionManager == nil {
		return
	}
	fx, ms := HashResourceID(name), uint32(p.Swing.Delay/time.Millisecond)
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() || w.sessionPlayerLocked(s) == p {
			return true
		}
		c := w.objectIDSeenByLocked(s, p.ServerID)
		if c == 0 {
			return true
		}
		t := w.objectIDSeenByLocked(s, target)
		if t == 0 {
			t = c
		}
		w.sendCastingCombat(s, c, t, ms, fx)
		return true
	})
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
// (rounds.go): a round every swing delay on the client's beat (the file's
// doc), its hits at the swing's times (swingHits), each in reach a hit
// (playerHitLocked) and out of it lost. A round begins only with the attack
// button held (0x40 byte 36), and waits for it with the fight kept
// (roundStart); the round in hand lands whole, the button up or not, as
// Sandstorm's did (a bard's hits went on 1.15 s past the release, and no
// further, the training dummy still hitting him). The player's own swing is
// played by its client: no 0x00B1 to it, one to those who see it
// (broadcastPlayerSwingLocked). Every tick; must be called with the lock
// held.
func (w *World) attackSystem(now time.Time) {
	for _, p := range w.Players {
		if p.AttackTarget == 0 && !w.pressUnsentLocked(p, now) {
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
		start, ok := p.roundStart(now)
		if !ok {
			continue
		}
		p.newRound() // the hands it holds now (HandsChanged)
		p.RoundStart, p.RoundHits = start, 0
		p.NextSwing = p.RoundStart.Add(p.Swing.Delay)
		p.swingTimer = p.NextSwing
		w.broadcastPlayerSwingLocked(p, npc.ServerID)
		w.playerRoundHitsLocked(p, npc, now)
	}
}

// playerRoundHitsLocked lands the hits of the player's round that are due.
// It reports whether the fight ended (a kill, or the NPC out of view).
func (w *World) playerRoundHitsLocked(p *Player, npc *NPC, now time.Time) bool {
	hits := swingHits(p.Swing.Kind)
	for p.RoundHits < len(hits) && !now.Before(p.RoundStart.Add(hits[p.RoundHits])) {
		p.RoundHits++
		if p.RoundHits == 1 {
			// the client swung: a round a 0x00D8 calls off ends nothing
			w.breakInvisLocked(p, "a swing")     // invis.go
			w.breakBoardWolfLocked(p, "a swing") // boardwolf.go
			w.ownHitLocked(p, "a swing")         // workbook.go: a run speed
		}
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

// A bow's shot (the user, 2026-11-06: "at level 35 bow damage should double,
// because 2 arrows are shot for one hit", then "tmac should hit 2000 on level
// 50 training dummy with bow"): every ranged swing's roll (CLIENT's Range1-4,
// GetSwing's kinds 8-11) is rangedShare times the ranged roll on the bow
// (offence.go's playerRangedRoll: DEX sets its top, WIS its bottom, the beta
// stat sheet's, 2026-11-11), and Range4, the swing from level 35, looses two
// arrows for its one hit, each its own roll. rangedShare is fitted to the
// user's figure on Tmac as the Heirloom snapshot of 2026-11-11 has him (a level
// 60 elf ranger, DEX 453 (his masteries raise its cap), WIS 260, his level 60
// heirloom bow's DMG 759): an arrow rolls 503-523 on a level 50 training dummy,
// 513 on average, so 1.95 makes his shot 2,000 on average (1,962-2,040). Until 2026-11-11 a shot rolled
// the melee roll (STR and DEX) times 2.11, fitted on a ranger of STR 227 and
// DEX 366 with a bow of 754 (474 an arrow); on Tmac's own stats that shot was
// 1,920. Ours: no record of the live game's bow damage exists.
const (
	swingRange1, swingRange4 = 8, 11
	rangedShare              = 1.95
)

// playerShotHit is one landed player swing's damage before the balance: a
// melee roll (playerMeleeHit), or a bow's shot of one arrow, or two at Range4,
// each rangedShare times a ranged roll (playerRangedHit).
func playerShotHit(p *Player, npc *NPC, intn func(int) int) int {
	kind := p.Swing.Kind
	if kind < swingRange1 || kind > swingRange4 || p.Swing.Potion != 0 {
		return playerMeleeHit(p, npc, intn)
	}
	arrows := 1
	if kind == swingRange4 {
		arrows = 2
	}
	hit := 0
	for range arrows {
		hit += int(math.Round(float64(playerRangedHit(p, npc, intn)) * rangedShare))
	}
	return hit
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
	if npc.duelHeld() {
		return false // a Metal Moogie's countdown: nothing lands (metalmoogie.go, petition 1623)
	}
	if w.shotUnseenLocked(sess, attacker, npc) {
		return true // a shot or a throw at what its shooter cannot see: the fight let go (sight.go)
	}
	if attacker.Swing.Potion != 0 {
		return w.thrownPotionHitLocked(sess, attacker, npc, objectID) // potion.go
	}
	if !w.shotPowerLocked(sess, attacker) {
		return true // a bow's shot without the power: the fight let go (rangedpower.go)
	}
	damage := w.meleeDealtLocked(npc, attacker, playerShotHit(attacker, npc, meleeIntn), meleeIntn) // balance.go
	if npc.Dummy != NotDummy {
		// told, taken off nothing; the dummy fights back (dummy.go)
		now := time.Now()
		attacker.LastCombatTime = now
		w.sendDamageIndication(sess, sess.PlayerObjectID(), -damage, objectID, numberMelee)
		w.markHealPlayerLocked(sess, attacker, npc, now) // markheal.go
		w.dummyEngagedLocked(npc, attacker.ServerID, damage, now)
		w.playerProcsLocked(sess, attacker, npc, now)
		w.markHitLocked(sess, attacker, npc, now) // markhit.go: told, as a dummy's hits are
		return false
	}
	damage = w.npcTakesLocked(npc, damage, time.Now())              // nothing while held at 1 (deathhold.go)
	w.tallyDamageLocked(npc, attacker, min(damage, max(npc.HP, 0))) // killcredit.go
	npc.HP -= damage
	if npc.HP < 0 {
		npc.HP = 0
	}
	killed := npc.HP <= 0
	attacker.LastCombatTime = time.Now()

	w.sendDamageIndication(sess, sess.PlayerObjectID(), -damage, objectID, numberMelee)
	w.markHealPlayerLocked(sess, attacker, npc, attacker.LastCombatTime) // markheal.go: a mark on it heals the hitter
	log.Printf("Combat: %s hit %s for %d damage (HP: %d/%d) %.2f off, reach %.2f", attacker.Name, npc.Name, damage, npc.HP, npc.MaxHP,
		dist3(attacker.X, attacker.Y, attacker.Z, npc.X, npc.Y, npc.Z), attacker.Swing.Reach+npcReachExtra(npc))

	w.updateAggro(npc, attacker.ServerID, damage)
	w.rootBreakLocked(npc, attacker.Level) // root.go
	if killed {
		w.killNPC(npc)
		cs, cp := w.killCreditLocked(npc, sess, attacker) // killcredit.go: the most damage's group
		w.awardKillXPLocked(cs, cp, npc)                  // killexp.go
		w.factionKillLocked(cs, cp, npc)
		return true
	}
	w.hitLandedLocked(npc, damage, sess, attacker.ServerID) // its heal at every hit that lands, its ward's answer (hitheal.go, ward.go)
	// a proc's damage after the hit (proc.go), then the creature's damage
	// shield's answer, which may end the fight the other way (creatureshield.go)
	if w.playerProcsLocked(sess, attacker, npc, attacker.LastCombatTime) {
		return true
	}
	// a damage mark on it answers the hit (markhit.go)
	if w.markHitLocked(sess, attacker, npc, attacker.LastCombatTime) {
		return true
	}
	return w.creatureShieldLocked(sess, attacker, npc, attacker.LastCombatTime)
}

// heldRefusalLocked tells a player pressing attack with a held item and no
// weapon in its hands that it cannot fight so (Swing.Held), at most once every
// heldNoFightGap.
func (w *World) heldRefusalLocked(sess *session.Session, p *Player, item string, now time.Time) {
	if !p.heldToldAt.IsZero() && now.Sub(p.heldToldAt) < heldNoFightGap {
		return
	}
	p.heldToldAt = now
	w.sendColoredLocked(sess, fmt.Sprintf(heldNoFightWords, item), castRefusalRGB)
	log.Printf("Combat: %s presses attack holding %q and no weapon: refused", p.Name, item)
}

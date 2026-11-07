package entity

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// pvp.go — the PvP flag, a server's own switch (the user, 2026-11-07: "for heirloom server
// lets add a /pvp command to allow players to be attackable. players types /pvp for 1min
// cant take off flag until no pvp attacks for 1min. players with the flag on and be attacked
// by other players and attack other players. for now in any area.", then "for pvp on players
// add an inidicator next to their name like the red face icon").
//
// world.pvp turns it on (SetPvP); off, /pvp says so and no flag can go up, so nothing below
// ever runs. /pvp (or /t pvp, as a PS2 keeps an unknown /command to itself: raidflag.go) puts
// the flag up, and takes it down once PvPHoldTime has passed since it went up and since the
// player's last PvP attack, given or taken. It ends at a logout (it is in no row) and comes
// through a takeover (the same Player) and a keep restart (keep.go), its time with it.
//
// Two flagged players in the same world and not in one group are foes (pvpFoe), everywhere,
// towns too. Foes fight by a duel's own code: a PvP fight is a Duel in no table (pvpFightLocked),
// made for the moment, so its melee, spells, procs, shields and pets are duelRoundLocked,
// duelLandLocked, duelprocs.go and duelPetRoundLocked, at PvPDamageShare. It differs from a duel in
// that it needs no asking, nobody forfeits, and a hit the victim cannot take is a death
// (pvpDeathLocked, the one place that says what a PvP death costs). It is nothing to a creature:
// no threat, no faction, no guard's notice.
//
// What a client sees, the same on a PS2 and on vanilla, with no client change:
//
//   - The face in the target window: 0x0010's disposition 0 (DrawTarget's icon 115, the red face
//     a hostile creature gets) for a flagged player, whoever looks; a player's blue otherwise
//     (PlayerFace). CLIENT reads it only at 0x0010, and Msg0010 asks only that its token match
//     (m_p2BB68, which nothing sets but to 0), so a flag's change sends it to everyone targeting
//     that player (TargetersOf; handlers/pvp.go) with no re-target.
//   - The name over the head: channel byte 157 (VICreatureEntity m_p299, bit 0: ApplyLook's colour
//     2, NameColors[2]) green, from SerializePlayerUpdate, for every viewer. A foe stays green
//     because of the next line.
//   - A foe can be swung at: TargetInRange refuses a player whose target's m_p256 (channel byte 142)
//     is 2 or 3, so a foe's channel says pvpFoeByte to that viewer alone (pvpViewLocked). The duels'
//     0 would do for the swing, but ApplyLook draws anything under 2 in the NPCs' gold. To CLIENT
//     (decomp, all of m_p256's readers) and vanilla (play.rs, the same tests) 4 is 0 but for the name:
//     selectable by byte 26 (VIEntity::IsSelectable), Tab finds it, the swing and the 0x000F go, □
//     talks (0x0004, nothing to say) where a player's info window would open, and RequestTrade and
//     InviteEntity refuse it (word 1015), as a duel opponent's are. A foe can still be invited from
//     the Who list (InviteToGroup, by its record), or by /raid.
//
// Numbers and choices, one place each: PvPHoldTime, PvPDamageShare, pvpDeathDebt, pvpFoeByte,
// pvpAreaAtTarget, pvpSayEvery, pvpPetAnswers, pvpPetYields.

const (
	// PvPHoldTime is how long the flag stays on after it went on and after the player's last PvP
	// attack, given or taken (the user's "1min").
	PvPHoldTime = time.Minute
	// PvPDamageShare is what a PvP hit lands at: a duel's (the coordinator's default, 2026-11-07).
	PvPDamageShare = DuelDamageShare
	// pvpDeathDebt is the experience debt of a death at a flagged player's hand: none.
	pvpDeathDebt = 0
	// pvpFoeByte is channel byte 142 for a foe (the file's doc).
	pvpFoeByte byte = 4
	// pvpSayEvery is how often a press on a player not to be fought says why: the press repeats
	// every tick while the button is held (pressUnsentLocked).
	pvpSayEvery = 5 * time.Second
	// pvpPetAnswers is how long after a foe's hit a defensive or aggressive pet still goes for it.
	pvpPetAnswers = 10 * time.Second
	// pvpPetYields is how long a pet left at a hit point by a foe's damage shield leaves foes alone
	// (a duel's yields for the duel; nothing kills a pet in either).
	pvpPetYields = 15 * time.Second
)

// pvpAreaAtTarget says whether an area spell may be aimed at a foe: false, every area spell
// spares players (the coordinator's default: "so an AoE can't drag bystanders in"), refused at
// a foe as it is at any player; true, its target's share lands on the foe and its splash
// spares players still (areaStartLocked).
var pvpAreaAtTarget = false

// The lines. A PS2 draws them from its own characters, and they wrap.
const (
	pvpOffServer = "PvP is not available on this server."
	pvpLineOn    = "You are now flagged for PvP. Other flagged players can attack you, and you them, anywhere."
	pvpLineOnHow = "/pvp (on a PS2, /t pvp) takes it off, once a minute has passed since it went on and since your last attack, given or taken."
	pvpLineOff   = "You are no longer flagged for PvP."
	pvpLineStays = "Your PvP flag stays on for %d more %s."
	pvpLineDead  = "You cannot do that now."
	pvpLineStill = "Your PvP flag is still on."
	pvpLineArea  = "Area spells do not reach players."
	pvpLineNot   = "You are not flagged for PvP, so you cannot attack %s. /pvp (on a PS2, /t pvp) flags you."
)

// pvpFlag is a player's flag; its zero value is off.
type pvpFlag struct {
	on       bool
	hold     time.Time // it cannot come off before this
	said     time.Time // the last press refused with a line (pvpSayEvery)
	struckBy EntityID  // the last foe that hit it, for its pet's stance
	struckAt time.Time
}

// SetPvP is world.pvp: whether players may flag themselves at all.
func (w *World) SetPvP(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pvpAllowed = on
}

// PvPFlagged reports the player flagged.
func (p *Player) PvPFlagged() bool { return p.pvp.on }

// pvpFoe reports q a player p may fight now: both flagged, alive, in one world and in no group
// together. Nothing else; in any area.
func (p *Player) pvpFoe(q *Player) bool {
	if p == nil || q == nil || p == q || !p.pvp.on || !q.pvp.on || p.World != q.World {
		return false
	}
	if p.IsDead() || q.IsDead() || p.respawnPending() || q.respawnPending() {
		return false
	}
	return p.GroupID == 0 || p.GroupID != q.GroupID
}

// PvPCommand is /pvp from sess's player: the flag on, or off once its time allows, the player
// told either way. It returns the player when the flag changed, for the handlers to tell those
// targeting it (TargetersOf).
func (w *World) PvPCommand(sess *session.Session, now time.Time) *Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	if !w.pvpAllowed {
		w.sendChatToSession(sess, pvpOffServer)
		return nil
	}
	if p.IsDead() || p.respawnPending() {
		w.sendChatToSession(sess, pvpLineDead)
		return nil
	}
	if !p.pvp.on {
		p.pvp = pvpFlag{on: true, hold: now.Add(PvPHoldTime)}
		log.Printf("PvP: %s puts up the flag", p.Name)
		w.sendChatToSession(sess, pvpLineOn)
		w.sendChatToSession(sess, pvpLineOnHow)
		return p
	}
	if left := p.pvp.hold.Sub(now); left > 0 {
		n, word := int(left.Seconds()+0.999), "seconds"
		if n == 1 {
			word = "second"
		}
		w.sendChatToSession(sess, fmt.Sprintf(pvpLineStays, n, word))
		return nil
	}
	w.pvpOffLocked(p)
	w.sendChatToSession(sess, pvpLineOff)
	return p
}

// pvpOffLocked takes p's flag down: what foes put on it goes with it, as a duel's spells go at its
// end, and its melee on a player is let go next tick (duelRoundLocked).
func (w *World) pvpOffLocked(p *Player) {
	p.pvp = pvpFlag{}
	log.Printf("PvP: %s takes down the flag", p.Name)
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.duelFoe != 0 {
			if d := w.duels.of[p.ServerID]; d == nil || d.other(p.ServerID) != e.duelFoe {
				w.removeEffectLocked(p, e.SpellID)
			}
		}
	}
}

// pvpLeavingLocked is p leaving the world: what it put on foes goes (a caster gone ticks nothing:
// duelTickLocked), as what a duelist put on its opponent does.
func (w *World) pvpLeavingLocked(p *Player) {
	if !p.pvp.on {
		return
	}
	for _, q := range w.Players {
		if q != p && q.pvp.on {
			w.clearDuelEffectsLocked(q, p.ServerID)
		}
	}
}

// pvpFightLocked is the fight between two foes, nil when they are not: a Duel in no table, made
// for the moment (the file's doc). Must be called with the lock held.
func (w *World) pvpFightLocked(a, b *Player) *Duel {
	if !a.pvpFoe(b) {
		return nil
	}
	return &Duel{A: a.ServerID, B: b.ServerID, who: [2]*Player{a, b}, Phase: duelOn, World: a.World, pvp: true,
		pets: make(map[EntityID]bool), petOut: make(map[EntityID]bool), petTarget: make(map[EntityID]EntityID),
		struck: make(map[EntityID]bool), petStruck: make(map[EntityID]bool)}
}

// fightBetweenLocked is the fight p's hit on t belongs to: their duel, else their PvP fight, else nil.
func (w *World) fightBetweenLocked(p, t *Player) *Duel {
	if d := w.duelOnLocked(p.ServerID); d != nil && d.other(p.ServerID) == t.ServerID {
		return d
	}
	return w.pvpFightLocked(p, t)
}

// pvpAttackedLocked is an attack of attacker's landing on t (a hit from it or its pet, a harmful
// spell, a damage tick): both flags held from now, and t's pet told who.
func pvpAttackedLocked(attacker, t *Player, now time.Time) {
	attacker.pvp.hold = now.Add(PvPHoldTime)
	t.pvp.hold = now.Add(PvPHoldTime)
	t.pvp.struckBy, t.pvp.struckAt = attacker.ServerID, now
}

// pvpEngageLocked is EngageAttack's PvP half: a 0x000F at a foe engages it as a creature is
// engaged, the attacker's flag held from the press. It reports whether it took the press; a press
// on a player who is no foe is told why (at most every pvpSayEvery).
func (w *World) pvpEngageLocked(p *Player, ent Entity, objectID uint32, swing Swing, now time.Time) bool {
	t, ok := ent.(*Player)
	if !ok || !w.pvpAllowed || t == p {
		return false
	}
	if !p.pvpFoe(t) {
		if why := pvpRefusal(p, t); why != "" && now.Sub(p.pvp.said) >= pvpSayEvery {
			p.pvp.said = now
			if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
				w.sendColoredLocked(sess, why, castRefusalRGB)
			}
		}
		return false
	}
	p.pvp.hold = now.Add(PvPHoldTime)
	p.Swing, p.swingNew = swing, false
	if p.AttackTarget == t.ServerID && p.AttackObjectID == objectID {
		p.pressed(now)
		return true
	}
	p.AttackTarget, p.AttackObjectID = t.ServerID, objectID
	p.pressed(now)
	w.breakInvisLocked(p, "an attack")
	w.breakBoardWolfLocked(p, "an attack")
	log.Printf("PvP: %s engages %s (swing %d, %v, reach %.1f)", p.Name, t.Name, swing.Kind, swing.Delay, swing.Reach)
	return true
}

// pvpRefusal is why p may not attack t, for the line; "" when it may or when nothing need be said
// (the dead).
func pvpRefusal(p, t *Player) string {
	switch {
	case p.IsDead() || t.IsDead():
		return ""
	case !p.pvp.on:
		return fmt.Sprintf(pvpLineNot, t.Name)
	case !t.pvp.on:
		return fmt.Sprintf("%s is not flagged for PvP.", t.Name)
	case p.GroupID != 0 && p.GroupID == t.GroupID:
		return fmt.Sprintf("%s is in your group.", t.Name)
	}
	return ""
}

// pvpSpellTargetLocked is castTargetsLocked's PvP half: a harmful spell at a foe is cast at it; on a
// server with the flag, a harmful spell at another player who is no foe is refused saying why. ok false
// when target is none of its business.
func (w *World) pvpSpellTargetLocked(p *Player, spell *CastSpell, target Entity) (ids []EntityID, refusal string, ok bool) {
	d := &spell.Def
	t, isPlayer := target.(*Player)
	if !w.pvpAllowed || !d.Harmful || !isPlayer || t == p {
		return nil, "", false
	}
	if !p.pvpFoe(t) {
		if why := pvpRefusal(p, t); why != "" {
			return nil, why, true
		}
		return nil, "", false
	}
	switch {
	case d.Charms():
		return nil, castFriendly, true
	case d.undeadOnly():
		return nil, castNotUndead, true
	case d.TargetKind == "animal":
		return nil, castNotAnimal, true
	case d.TargetKind == "summoned":
		return nil, castNotSummoned, true
	case spell.Area.Centre != AreaNone && !pvpAreaAtTarget:
		return nil, pvpLineArea, true
	}
	if outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
		return nil, refuseOutOfRange(p, spell, t.Name, t.X, t.Y, t.Z, spell.Range), true
	}
	spell.duel = true
	return []EntityID{t.ServerID}, "", true
}

// pvpCastBegunLocked is a cast begun: one aimed at a foe holds its caster's flag from now.
func (w *World) pvpCastBegunLocked(p *Player, spell *CastSpell, targets []EntityID, now time.Time) {
	if !spell.duel || !p.pvp.on {
		return
	}
	for _, id := range targets {
		if p.pvpFoe(w.Players[id]) {
			p.pvp.hold = now.Add(PvPHoldTime)
		}
	}
}

// pvpDeathLocked is t's death at killer's hand, and everything a PvP death costs, here alone: it
// dies as of anything and comes back at its bind after the usual wait (respawn.go) with no
// experience debt (pvpDeathDebt), no item or tunar lost, no corpse (a player has none) and no
// experience for the killer. Must be called with the lock held.
func (w *World) pvpDeathLocked(killer, t *Player, now time.Time) {
	sess := w.findSessionByEntityIDLocked(t.ServerID)
	w.playerDownLocked(sess, t, now, pvpDeathDebt)
	if killer.AttackTarget == t.ServerID {
		killer.disengage()
	}
	log.Printf("PvP: %s kills %s", killer.Name, t.Name)
	if sess != nil {
		w.sendColoredLocked(sess, fmt.Sprintf("%s has slain you.", killer.Name), duelOrange)
	}
	w.duelSayLocked(killer, fmt.Sprintf("You have slain %s.", t.Name), duelOrange)
}

// pvpViewLocked is a flagged player's channel as sess sees it: to a foe its byte 142 is pvpFoeByte
// (the file's doc). Must be called with the lock held.
func (w *World) pvpViewLocked(sess *session.Session, p *Player, buf []byte) {
	if !p.pvp.on || len(buf) <= 142 {
		return
	}
	if viewer := w.sessionPlayerLocked(sess); p.pvpFoe(viewer) {
		buf[142] = pvpFoeByte
	}
}

// PlayerFace is the face a target window shows for player t, whoever looks: the red face flagged, a
// player's blue otherwise.
func (w *World) PlayerFace(t *Player) byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if t.pvp.on {
		return FaceHostile
	}
	return FaceAlly
}

// Targeter is a session whose client has a player as its target.
type Targeter struct {
	Sess     *session.Session
	Viewer   *Player
	ObjectID uint32 // the id it knows the player by
	Token    uint32 // its last 0x0010's token
}

// TargetersOf is everyone whose client has t targeted, t itself among them. A target's face is read
// only from 0x0010, so these are who a flag's change must tell.
func (w *World) TargetersOf(t *Player) []Targeter {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Targeter
	if w.SessionManager == nil {
		return nil
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		v := w.sessionPlayerLocked(s)
		if v == nil || v.TargetObjectID == 0 || v.TargetObjectID == noTargetObject {
			return true
		}
		if id := w.objectIDSeenByLocked(s, t.ServerID); id != 0 && id == v.TargetObjectID {
			out = append(out, Targeter{Sess: s, Viewer: v, ObjectID: id, Token: v.targetToken})
		}
		return true
	})
	return out
}

// SetTargetToken keeps the token of a player's last 0x0010.
func (w *World) SetTargetToken(id EntityID, token uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.Players[id]; p != nil {
		p.targetToken = token
	}
}

// pvpPetLocked is petSystem's PvP half: a flagged owner's pet goes for a foe told to (0x0041 8:
// pvpPetCommandLocked) or by its stance as in a duel: aggressive what its owner swings at or what
// struck its owner, defensive what struck its owner; every hit is its owner's PvP hit. It is never
// hit in return but by a damage shield: to a foe a pet stays a pet, as to anyone (byte 142 3). It
// reports whether the pet fought this tick (its other fights and its following wait).
func (w *World) pvpPetLocked(ps *petState, dt float32, now time.Time) bool {
	owner, pet := ps.owner, ps.npc
	if !owner.pvp.on || pet.Charm != nil || !pet.petFights() {
		ps.pvpTarget = 0
		return false
	}
	t := w.Players[ps.pvpTarget]
	if ps.pvpTarget != 0 && !owner.pvpFoe(t) {
		ps.pvpTarget, t = 0, nil
		pet.endRound()
		pet.stopMoving()
	}
	if ps.pvpTarget == 0 {
		if now.Before(ps.heldUntil) {
			return false
		}
		if ps.stance == petAggressive && owner.Attacking() {
			if q := w.Players[owner.AttackTarget]; owner.pvpFoe(q) {
				t = q
			}
		}
		if t == nil && (ps.stance == petAggressive || ps.stance == petDefensive) && now.Sub(owner.pvp.struckAt) < pvpPetAnswers {
			if q := w.Players[owner.pvp.struckBy]; owner.pvpFoe(q) {
				t = q
			}
		}
		if t == nil {
			return false
		}
		ps.pvpTarget = t.ServerID
		log.Printf("PvP: %s's pet %q goes for %s", owner.Name, pet.Name, t.Name)
	}
	if !w.duelPetRoundLocked(ps, w.pvpFightLocked(owner, t), t.ServerID, dt, now) {
		ps.pvpTarget = 0 // past its leash: it follows its owner
		return false
	}
	return true
}

// pvpPetCommandLocked is PetCommand's attack (0x0041 8) at a foe of the owner's: the pet goes for
// it. It reports whether the target was one.
func (w *World) pvpPetCommandLocked(ps *petState, target Entity) bool {
	t, ok := target.(*Player)
	if !ok || !ps.owner.pvpFoe(t) || ps.npc.Charm != nil || !ps.npc.petFights() {
		return false
	}
	ps.pvpTarget, ps.target = t.ServerID, 0
	ps.heldUntil = time.Time{}
	return true
}

// pvpMeleeRoll is a player's one roll on a foe: as a duel's, the creature line on the foe's level.
func pvpMeleeRoll(p, t *Player) int {
	return playerMeleeHit(p, duelStandIn(t.Level), rand.Intn)
}

package entity

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Training dummies as Sandstorm's, measured 2026-10-21 with eqoa-rs's
// `eqoa harvest` attack step on the Freeport "Training Dummy Lv50" and
// Muniel's Tea Garden's ladder of Lv20-60
// (claude-notes/training-dummies-20261021.md):
//
//   - A dummy never dies. What it is hit for, by swing or spell, is told
//     (0x00DB) and taken off nothing; its health byte never moved.
//   - It fights the player whose hit landed first, for as long as that
//     player stays within its reach, attacking it or not (a dummy went on
//     with its rounds for 80 s while its player fought the dummies beside
//     it), and nobody else.
//   - It fights in rounds as every NPC does (rounds.go): the Lv20-30 swing
//     Attack2 every 4 s and hit at 0.5 and 1.5 s; from Lv35, Attack4 every
//     6 s with hits at 0.5, 1, 1.7 and 2.4 s.
//   - A dummy's hit is its level's roll on the player's AC, AGI and
//     archetype (defence.go, dummyRolls): the floor on a well-armoured tank
//     (two tanks of 1763 and 1430 AC took the Lv45's 24 every time), up to
//     its top on little armour (an enchanter of 167 AC took 24-124). A
//     Training Dummy's every hit is followed at once by a heal of 300
//     (0x00DB +300, the dummy the healer), so the player never dies of it;
//     a Regular Dummy heals nothing.
//
// It gives no experience and never takes a target's aggro (CurrentEnemy
// stays 0), so the player's swings end with the attack button, as against
// any creature that is not fighting back.

// DummyKind says whether an NPC is a training dummy, and which.
type DummyKind uint8

const (
	NotDummy      DummyKind = iota
	TrainingDummy           // "Training Dummy LvN": a heal after each hit
	RegularDummy            // "Regular Dummy LvN": a creature's hits, no heal
	SparringDummy           // "Sparring Dummy": its level and element the player's pick (sparring.go)
)

// DummyKindOf is the kind the NPC's name gives it.
func DummyKindOf(name string) DummyKind {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "training dummy"):
		return TrainingDummy
	case strings.HasPrefix(n, "regular dummy"):
		return RegularDummy
	case strings.HasPrefix(n, "sparring dummy"):
		return SparringDummy
	}
	return NotDummy
}

// dummyHeal is what a Training Dummy gives back after each hit.
const dummyHeal = 300

// dummyReach is how near the player must stay for the dummy to go on
// fighting it. A dummy went on at 7.1 and let go of a player 12.8 away.
const dummyReach = 10

// dummyEngagedLocked is a player's hit landing on a dummy: the dummy takes
// that player on if it is fighting no one. The first round starts at once,
// as Sandstorm's did within a tick of the first hit.
func (w *World) dummyEngagedLocked(npc *NPC, p *Player, now time.Time) {
	if npc.DummyTarget != 0 {
		return
	}
	npc.DummyTarget = p.ServerID
	npc.endRound()
	log.Printf("Dummy: %s fights %s", npc.Name, p.Name)
}

// dummyFightLocked runs a fighting dummy's round (aiSystem's pass, every
// tick): its fight ends when its player is gone, dead or out of its reach.
// Must be called with the lock held.
func (w *World) dummyFightLocked(npc *NPC, now time.Time) {
	p := w.Players[npc.DummyTarget]
	var sess *session.Session
	if p != nil {
		sess = w.findSessionByEntityIDLocked(p.ServerID)
	}
	if p == nil || sess == nil || p.IsDead() || npc.Dead || !dummyNear(npc, p) {
		npc.endDummyFight()
		return
	}
	start := npc.RoundStart
	w.npcRoundLocked(npc, p, now, func() bool { return w.dummyHitLocked(sess, npc, p, now) })
	if npc.Dummy == SparringDummy && npc.DummyTarget == p.ServerID {
		w.sparRoundLocked(sess, npc, p, start, now)
	}
}

// dummyHitLocked is one of a dummy's hits on the player, and a Training
// Dummy's heal after it. It reports the player's death.
func (w *World) dummyHitLocked(sess *session.Session, npc *NPC, p *Player, now time.Time) bool {
	damage := npcMeleeRoll(npc).hitOn(p, rand.Intn)
	damage = w.playerInvulnerableLocked(p, npc, damage, now) // invulnerable: nothing taken (spiritform.go)
	damage = w.playerWardLocked(p, npc, damage, now)         // the player's ward, a sparring dummy's melee too: the hit as taken (ward.go)
	if npc.Dummy == SparringDummy {
		if w.sparHitLocked(sess, npc, p, damage, "", now) {
			return true
		}
		w.playerShieldsLocked(sess, p, npc, now) // the player's shields: told, taken off nothing (dshield.go)
		return false
	}
	p.HP = max(p.HP-damage, 0)
	p.LastCombatTime = now
	npcObjectID := w.objectIDForEntityLocked(sess, npc.ServerID)
	w.sendDamageIndication(sess, npcObjectID, -damage, sess.PlayerObjectID())
	if p.HP <= 0 {
		npc.endDummyFight()
		w.playerDiedLocked(sess, p, now)
		return true
	}
	w.playerShieldsLocked(sess, p, npc, now) // the player's shields: told, taken off nothing (dshield.go)
	if npc.Dummy == TrainingDummy {
		p.HP = min(p.HP+dummyHeal, p.MaxHPNow())
		w.sendDamageIndication(sess, npcObjectID, dummyHeal, sess.PlayerObjectID())
	}
	return false
}

// dummyNear is whether the player is within the dummy's reach.
func dummyNear(npc *NPC, p *Player) bool {
	dx, dz := npc.X-p.X, npc.Z-p.Z
	return p.World == npc.World && dx*dx+dz*dz <= dummyReach*dummyReach
}

func (n *NPC) endDummyFight() {
	n.DummyTarget = 0
	n.endRound()
}

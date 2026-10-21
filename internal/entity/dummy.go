package entity

import (
	"log"
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
//   - It fights in rounds: a swing at the player (0x00B1, the round's length
//     as the cast time and VISwing::Effect's Attack2 or Attack4), then its
//     hits at fixed times into the round.
//   - A Training Dummy's hit is a fixed amount by its level, each followed at
//     once by a heal of 300 (0x00DB +300, the dummy the healer), so the
//     player never dies of it. A Regular Dummy hits as a creature does and
//     heals nothing.
//
// It gives no experience and never takes a target's aggro (CurrentEnemy
// stays 0), so the player's swings end with the attack button, as against
// any creature that is not fighting back.

// DummyKind says whether an NPC is a training dummy, and which.
type DummyKind uint8

const (
	NotDummy      DummyKind = iota
	TrainingDummy           // "Training Dummy LvN": fixed hits, a heal after each
	RegularDummy            // "Regular Dummy LvN": a creature's hits, no heal
)

// DummyKindOf is the kind the NPC's name gives it.
func DummyKindOf(name string) DummyKind {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "training dummy"):
		return TrainingDummy
	case strings.HasPrefix(n, "regular dummy"):
		return RegularDummy
	}
	return NotDummy
}

// dummyHeal is what a Training Dummy gives back after each hit.
const dummyHeal = 300

// dummyReach is how near the player must stay for the dummy to go on
// fighting it. A dummy went on at 7.1 and let go of a player 12.8 away.
const dummyReach = 10

// dummyRound is the shape of a dummy's round at its level: its length, the
// swing it plays and the times of its hits. Sandstorm's: the Lv20, Lv25
// and Lv30 dummies swing Attack2 every 4 s and hit at 0.5 and 1.5 s; from
// Lv35 they swing Attack4 every 6 s and hit at 0.5, 1, 1.7 and 2.4 s.
func dummyRound(level int) (length time.Duration, swing string, hits []time.Duration) {
	if level < dummyLongRoundLevel {
		return 4 * time.Second, "Attack2", []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond}
	}
	return 6 * time.Second, "Attack4", []time.Duration{500 * time.Millisecond, 1000 * time.Millisecond, 1700 * time.Millisecond, 2400 * time.Millisecond}
}

// dummyLongRoundLevel is the first level whose rounds are 6 s.
const dummyLongRoundLevel = 35

// trainingDummyHit is a Training Dummy's hit by its level, as Sandstorm's
// were on every player measured (a level 50 and a level 51 of 1763 and 1430
// AC took the same). Between the levels measured it is interpolated.
var trainingDummyHits = [][2]int{{20, 8}, {25, 10}, {30, 12}, {35, 14}, {45, 24}, {50, 21}, {55, 30}}

func trainingDummyHit(level int) int {
	t := trainingDummyHits
	if level <= t[0][0] {
		return t[0][1]
	}
	for i := 1; i < len(t); i++ {
		if level <= t[i][0] {
			a, b := t[i-1], t[i]
			return a[1] + (b[1]-a[1])*(level-a[0])/(b[0]-a[0])
		}
	}
	return t[len(t)-1][1]
}

// dummyEngagedLocked is a player's hit landing on a dummy: the dummy takes
// that player on if it is fighting no one. The first round starts at once,
// as Sandstorm's did within a tick of the first hit.
func (w *World) dummyEngagedLocked(npc *NPC, p *Player, now time.Time) {
	if npc.DummyTarget != 0 {
		return
	}
	npc.DummyTarget = p.ServerID
	npc.DummyRound = time.Time{}
	npc.DummyHits = 0
	log.Printf("Dummy: %s fights %s", npc.Name, p.Name)
}

// dummySystem runs every fighting dummy's rounds. Every tick; must be
// called with the lock held.
func (w *World) dummySystem(now time.Time) {
	for _, npc := range w.NPCs {
		if npc.Dummy == NotDummy || npc.DummyTarget == 0 {
			continue
		}
		p := w.Players[npc.DummyTarget]
		var sess *session.Session
		if p != nil {
			sess = w.findSessionByEntityIDLocked(p.ServerID)
		}
		if p == nil || sess == nil || p.IsDead() || npc.Dead || !dummyNear(npc, p) {
			npc.endDummyFight()
			continue
		}
		length, swing, hits := dummyRound(npc.Level)
		if npc.DummyRound.IsZero() || !now.Before(npc.DummyRound.Add(length)) {
			npc.DummyRound = now
			npc.DummyHits = 0
			w.sendCastingCombat(sess, w.objectIDForEntityLocked(sess, npc.ServerID), sess.PlayerObjectID(),
				uint32(length/time.Millisecond), HashResourceID(swing))
		}
		for npc.DummyHits < len(hits) && !now.Before(npc.DummyRound.Add(hits[npc.DummyHits])) {
			npc.DummyHits++
			if w.dummyHitLocked(sess, npc, p, now) {
				break
			}
		}
	}
}

// dummyHitLocked is one of a dummy's hits on the player, and a Training
// Dummy's heal after it. It reports the player's death.
func (w *World) dummyHitLocked(sess *session.Session, npc *NPC, p *Player, now time.Time) bool {
	var damage int
	if npc.Dummy == TrainingDummy {
		damage = trainingDummyHit(npc.Level)
	} else {
		damage = CalculateMeleeDamage(npc.OffMod, RollWeaponDamage(npc.WeaponDamage), npc.Level, p.Level, p.DefMod)
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
	n.DummyRound = time.Time{}
	n.DummyHits = 0
}

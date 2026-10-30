package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A sparring dummy (F3 20261025-174503, the user's ask after jib's shout: "a
// training dummy that you can set its level, and it just quads you and nukes
// you for an element you can pick"). Talked to, it asks a level (SparLevels)
// and an element (SparElements); struck, it fights the player as the other
// dummies do (dummy.go), at that level:
//
//   - every round is Attack4, four hits a creature of its level rolls on
//     the player's defence (defence.go's creature line);
//   - every sparNukeEvery-th round, after the four hits, it strikes with its
//     element (sparStrike): its level's damage, the player's resist to that
//     element taking its share off (resist.go), said in the chat with the share;
//   - it never kills: a hit that would leave the player at nothing leaves
//     one hit point and ends the fight.
//
// The strike's numbers are this server's own, chosen, not measured: no
// recording of an NPC's spell on a player exists here.

// SparLevels are the levels a sparring dummy is offered at.
var SparLevels = [...]int{10, 20, 30, 40, 50, 60}

// SparElements are the elements it strikes with, each the player's resist
// to it.
var SparElements = [...]struct {
	Name   string
	Resist Stat
}{
	{"Fire", StatFR},
	{"Cold", StatCR},
	{"Lightning", StatLR},
	{"Arcane", StatAR},
	{"Poison", StatPR},
	{"Disease", StatDR},
}

const (
	// sparSwing is Attack4 (SwingEffectName's 5): four hits every 6 s.
	sparSwing = 5
	// sparNukeEvery is how many rounds go to a strike: every second one.
	sparNukeEvery = 2
	// sparStrikeAt is when in its round the strike comes: after the four
	// hits (0.5 to 2.4 s), before the next round (6 s).
	sparStrikeAt = 3 * time.Second
	// sparStrikePerLevel is a strike's damage a level, before its spread
	// (±20%) and the resist's share: a level 60's 600, a level 10's 100.
	sparStrikePerLevel = 10
	// sparFX is the strike's visual: the damage spells' stand-in (Fiery
	// Burst's, the one Sandstorm showed us; cast.go's fxDamage).
	sparFX = fxDamage
)

// sparStrikeDamage is a strike of a dummy of level, spread by roll (0 to 1),
// less the share its target's resist took (resist.go's RollResist).
func sparStrikeDamage(level int, share, roll float64) int {
	base := float64(level*sparStrikePerLevel) * (0.8 + 0.4*roll)
	return max(int(math.Round(base*(1-share))), 1)
}

// SetSparring sets a sparring dummy's level and element (indexes of
// SparLevels and SparElements) for player by: refused while it spars with
// someone else (busy names them); its fight with by goes on at the new
// level and element.
func (w *World) SetSparring(npcID, by EntityID, level, element int) (name, busy string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[npcID]
	if npc == nil || npc.Dummy != SparringDummy || level < 0 || level >= len(SparLevels) || element < 0 || element >= len(SparElements) {
		return "", "", false
	}
	if t := npc.DummyTarget; t != 0 && t != by {
		if p := w.Players[t]; p != nil {
			return npc.Name, p.Name, false
		}
	}
	npc.Level = SparLevels[level]
	npc.SparElement = element
	npc.SparRounds = 0
	npc.SparStruck = time.Time{}
	npc.HP, npc.MaxHP = NPCMaxHP(npc.Level), NPCMaxHP(npc.Level)
	log.Printf("Sparring: %s set to level %d, %s", npc.Name, npc.Level, SparElements[element].Name)
	return npc.Name, "", true
}

// sparRoundLocked follows a sparring dummy's round (dummyFightLocked, after
// its hits): a round begun since start is counted, and on every
// sparNukeEvery-th its strike comes sparStrikeAt into it, once. Must be
// called with the lock held.
func (w *World) sparRoundLocked(sess *session.Session, npc *NPC, p *Player, start, now time.Time) {
	if npc.RoundStart.IsZero() {
		return
	}
	if !npc.RoundStart.Equal(start) {
		npc.SparRounds++
	}
	if npc.SparRounds%sparNukeEvery != 0 || npc.SparStruck.Equal(npc.RoundStart) || now.Before(npc.RoundStart.Add(sparStrikeAt)) {
		return
	}
	npc.SparStruck = npc.RoundStart
	el := SparElements[npc.SparElement]
	share, how := RollResist(p.StatNow(el.Resist))
	damage := sparStrikeDamage(npc.Level, share, rand.Float64())
	damage = w.playerInvulnerableLocked(p, npc, damage, now) // a creature's spell: nothing taken of it (spiritform.go)
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, sparFX)
	line := fmt.Sprintf("%s strikes you with %s for %d (%s).", npc.Name, el.Name, damage, resistWords(share, how))
	w.sparHitLocked(sess, npc, p, damage, line, now)
}

// sparHitLocked lands a sparring dummy's hit or strike: its 0x00DB in full,
// the hit points it takes but the last one, which ends the fight instead
// (said). line, when not empty, is said first. It reports the fight's end.
// A hit comes already cut by the player's ward (dummyHitLocked); a strike,
// a spell, is not cut (ward.go). Both come to nothing on an invulnerable
// player (spiritform.go).
func (w *World) sparHitLocked(sess *session.Session, npc *NPC, p *Player, damage int, line string, now time.Time) bool {
	p.LastCombatTime = now
	npcObjectID := w.objectIDForEntityLocked(sess, npc.ServerID)
	if line != "" {
		w.sendChatToSession(sess, line)
	}
	w.sendDamageIndication(sess, npcObjectID, -damage, sess.PlayerObjectID())
	if p.HP-damage >= 1 {
		p.HP -= damage
		return false
	}
	p.HP = 1
	npc.endDummyFight()
	w.sendChatToSession(sess, npc.Name+": That will do. You are not fit to go on.")
	log.Printf("Sparring: %s stopped short of killing %s", npc.Name, p.Name)
	return true
}

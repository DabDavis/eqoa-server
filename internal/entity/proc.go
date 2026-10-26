package entity

import (
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// proc.go — a weapon proc: an effect that, while it lasts, gives its
// bearer's melee hits a chance to do more (F3 20261025-165245, jib: the
// enchanter's line from Chaotic Flux "should add a buff that when meleeing
// has a chance to proc damage. Proc rate is determined by a users
// intelligence. Damage has no mod, but is AR resisted. This was castable on
// all players and pets.").
//
// The lines say a hit and a duration ("Target, Decrease Hitpoints by 64,
// 5min"); only the ability's description says it is a proc, and ParseSpell
// reads it so (spelltext.go): an effect on a friend, a player or a pet,
// which carries the damage (ActiveEffect.Proc) on the effects channel for
// its duration. Each of its bearer's melee hits that leaves the creature
// standing may then proc: the damage, with no mod, to the creature, unless
// its arcane resist takes it all; told as a spell's hit is (0x00DB from the
// bearer, or a pet's owner), its hate the bearer's, its kill the bearer's
// (a pet's its owner's) as a melee kill is.
//
// Not measured (Sandstorm's recordings have no proc in them), so chosen:
//   - the chance a hit procs: 3%, and one more for each 20 INT the bearer
//     has, at most 25% (INT 100: 8%; 300: 18%); a pet, which has no INT, as
//     three a level;
//   - a creature's arcane resist: 5% against a bearer of its own level, 3
//     more for each level it is above the bearer, 3 fewer for each below,
//     from none to 75%; all or nothing.

const (
	procBasePct  = 3
	procIntPer   = 20
	procMostPct  = 25
	resistAtPar  = 5
	resistPerLvl = 3
	resistMost   = 75
	petIntPerLvl = 3
)

// procIntn is the procs' dice (a test's own).
var procIntn = rand.Intn

// procChancePct is the chance, in percent, that a hit procs for a bearer of
// that INT.
func procChancePct(intel int) int {
	return min(procBasePct+max(intel, 0)/procIntPer, procMostPct)
}

// procResistPct is the chance, in percent, that a creature of targetLevel
// resists the proc of a bearer of bearerLevel (all of it).
func procResistPct(bearerLevel, targetLevel int) int {
	return min(max(resistAtPar+resistPerLvl*(targetLevel-bearerLevel), 0), resistMost)
}

// procRolls is the damage of each proc a hit makes among the effects, of a
// bearer of that INT and level on a creature of targetLevel: one roll a
// proc effect (each its own chance), a resisted one left out.
//
// resistOff is what arcane resist debuffs on the creature take off its
// chance to resist, in points (enchanter.go).
func procRolls(effects []*ActiveEffect, intel, bearerLevel, targetLevel, resistOff int, now time.Time, intn func(int) int) (hits []int, resisted int) {
	for _, ae := range effects {
		if ae.Proc <= 0 || !now.Before(ae.Until) {
			continue
		}
		if intn(100) >= procChancePct(intel) {
			continue
		}
		if intn(100) < procResistPct(bearerLevel, targetLevel)-resistOff {
			resisted++
			continue
		}
		n := ae.Proc
		if ae.ProcMax > ae.Proc {
			n += intn(ae.ProcMax - ae.Proc + 1)
		}
		hits = append(hits, n)
	}
	return hits, resisted
}

// playerProcsLocked runs a player's procs after one of its melee hits on a
// creature still standing. It reports a kill.
func (w *World) playerProcsLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	hits, resisted := procRolls(p.Effects, p.StatNow(StatInt), p.Level, npc.Level, npcResistOffPct(npc, now), now, procIntn)
	if resisted > 0 {
		log.Printf("Proc: %s's proc resisted by %s", p.Name, npc.Name)
	}
	for _, n := range hits {
		log.Printf("Proc: %s's proc hits %s for %d", p.Name, npc.Name, n)
		if w.spellHitLocked(p, sess, npc, n) {
			return true
		}
	}
	return false
}

// petProcsLocked runs a pet's procs after one of its melee hits on a
// creature still standing: its owner's, as the pet's hits are. It reports a
// kill.
func (w *World) petProcsLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	hits, resisted := procRolls(pet.Effects, petIntPerLvl*pet.Level, pet.Level, t.Level, npcResistOffPct(t, now), now, procIntn)
	if resisted > 0 {
		log.Printf("Proc: the pet %q's proc resisted by %s", pet.Name, t.Name)
	}
	for _, n := range hits {
		log.Printf("Proc: the pet %q's proc hits %s for %d", pet.Name, t.Name, n)
		if w.petDamageLocked(ps, t, n, now) {
			return true
		}
	}
	return false
}

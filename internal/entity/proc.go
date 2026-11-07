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
// bearer of those stats and that level on a creature of targetLevel: one
// roll a proc effect (each its own chance, by the bearer's INT), a resisted
// one left out; a proc scaled by its bearer's stat adds its share of it, and
// a capped one does no more than its cap.
//
// resistOff is what arcane resist debuffs on the creature take off its
// chance to resist, in points (enchanter.go); undead says whether it is
// undead (asked only of a proc for the undead alone, which nothing else sets
// off: Smite Undead's, the workbook's "Enemy - Undead").
func procRolls(effects []*ActiveEffect, stat func(Stat) int, bearerLevel, targetLevel, resistOff int, undead func() bool,
	now time.Time, intn func(int) int) (hits []int, resisted int) {
	rolls, resisted := procRollsOf(effects, stat, bearerLevel, targetLevel, resistOff, undead, now, intn)
	for _, r := range rolls {
		hits = append(hits, r.n)
	}
	return hits, resisted
}

// procRoll is one proc a hit made: its damage and the effect that made it, whose own heal
// goes with it (procTapLocked).
type procRoll struct {
	n  int
	ae *ActiveEffect
}

// procRollsOf is procRolls with each proc's effect.
func procRollsOf(effects []*ActiveEffect, stat func(Stat) int, bearerLevel, targetLevel, resistOff int, undead func() bool,
	now time.Time, intn func(int) int) (hits []procRoll, resisted int) {
	intel := stat(StatInt)
	for _, ae := range effects {
		if ae.Proc <= 0 || !now.Before(ae.Until) || (ae.ProcUndead && !undead()) {
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
		if ae.ProcScale != StatNone && ae.ProcScalePct != 0 {
			n += stat(ae.ProcScale) * ae.ProcScalePct / 100
		}
		hits = append(hits, procRoll{n: capAmount(n, ae.ProcCap), ae: ae})
	}
	return hits, resisted
}

// procTapLocked heals the player by the heal of the effect whose proc just hit: a life steal is one
// proc, its damage and its heal together (Veil of Death, Shroud of Death and their + forms give both as
// offensive procs). Until 2026-11-07 they rolled apart, so a proc's damage and its heal came on different
// hits, and at full health the heal showed nothing: petition 1156, "does not proc".
func (w *World) procTapLocked(p *Player, sess *session.Session, ae *ActiveEffect, npc *NPC) {
	r := ae.Rules
	if r.ProcHeal <= 0 || r.ProcHealDef || (r.ProcHealUndead && (npc == nil || !w.undeadLocked(npc))) {
		return
	}
	n := r.ProcHeal
	if r.ProcHealScale != StatNone && r.ProcHealScalePct != 0 {
		n += p.StatNow(r.ProcHealScale) * r.ProcHealScalePct / 100
	}
	log.Printf("Proc: %s's %q heals it for %d", p.Name, ae.Name, n)
	w.healLocked(p, sess, p, n)
}

// playerProcsLocked runs a player's procs after one of its melee hits on a
// creature still standing. It reports a kill.
func (w *World) playerProcsLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	undead := func() bool { return w.undeadLocked(npc) }
	hits, resisted := procRollsOf(p.Effects, p.StatNow, p.Level, npc.Level, npcResistOffPct(npc, now), undead, now, procIntn)
	if resisted > 0 {
		log.Printf("Proc: %s's proc resisted by %s", p.Name, npc.Name)
	}
	for _, h := range hits {
		log.Printf("Proc: %s's %q hits %s for %d", p.Name, h.ae.Name, npc.Name, h.n)
		// seen as a damage shield's answer is (petition 1156: "does not proc",
		// when all it showed was one more figure among the swings')
		w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
		killed := w.spellHitLocked(p, sess, npc, h.n)
		w.procTapLocked(p, sess, h.ae, npc)
		if killed {
			return true
		}
	}
	w.procHealsLocked(p, npc, false, now)        // its offensive proc heals (workbook.go)
	w.procSpellsLocked(p, sess, npc, false, now) // and spell procs (workbookmore.go)
	// and the weapon's own (weaponproc.go)
	if !npc.Dead && w.weaponProcLocked(sess, p, npc, now) {
		return true
	}
	return npc.Dead
}

// petProcsLocked runs a pet's procs after one of its melee hits on a
// creature still standing: its owner's, as the pet's hits are. It reports a
// kill.
func (w *World) petProcsLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	// a pet has no stats of its own: each is its INT's stand-in, 3 a level
	petStat := func(Stat) int { return petIntPerLvl * pet.Level }
	undead := func() bool { return w.undeadLocked(t) }
	hits, resisted := procRolls(pet.Effects, petStat, pet.Level, t.Level, npcResistOffPct(t, now), undead, now, procIntn)
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

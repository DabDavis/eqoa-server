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
// The chance a hit procs is the beta's stat sheet's (betaformula.webp, adopted
// by the owner 2026-11-11: "Intelligence factors in to calculating how likely
// you are to process 'Proc' a spell when you have a buff on you that grants
// you a chance to proc. These spells have a base chance of 25% plus 1% for
// every 10 points of INT"): 25%, one more for each whole 10 INT the bearer
// has, at most 100% (the sheet names no cap: INT 100 35%, 300 55%, 750 the
// whole); a pet, which has no INT, as three a level (a level 60 pet's 180:
// 43%). Until then ours, unmeasured: 3% and one more each 20 INT, at most 25%,
// which a weapon's proc keeps, by DEX (weaponproc.go, petition 1360). A proc
// that states its own chance keeps it. The rest not measured (Sandstorm's
// recordings have no proc in them), so chosen:
//   - a creature's resist (a proc's arcane, and every spell's, workbook.go):
//     5% against a bearer of its own level, 1 more for each of the first 10
//     levels it is above the bearer and 5 more for each level past them, 1
//     fewer for each level below, from none to 75% (the user, 2026-11-07:
//     "shouldnt be resist much unless huge level difference"; until then 3 a
//     level either way). What a resist leaves is the hit at its least.

const (
	procBasePct  = 25  // the beta sheet's INT rule (above)
	procIntPer   = 10  // a point of chance each
	procMostPct  = 100 // ours: the sheet names no cap
	resistAtPar  = 5
	resistPerLvl = 1  // each of the first resistNear levels above
	resistNear   = 10 // the levels a resist grows slowly over
	resistFarLvl = 5  // each level past resistNear
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
	d := targetLevel - bearerLevel
	pct := resistAtPar + resistPerLvl*d
	if d > resistNear {
		pct = resistAtPar + resistPerLvl*resistNear + resistFarLvl*(d-resistNear)
	}
	return min(max(pct, 0), resistMost)
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
		chance := ae.Rules.ProcChance // its own (a form's: weaponproc.go), else by INT
		if chance <= 0 {
			chance = procChancePct(intel)
		}
		if intn(100) >= chance {
			continue
		}
		if intn(100) < procResistPct(bearerLevel, targetLevel)-resistOff {
			// resisted, it does the least it can (workbook.go's leastHarm): its range's
			// low end, no bearer's stat
			resisted++
			hits = append(hits, procRoll{n: capAmount(ae.Proc, ae.ProcCap), ae: ae})
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

// procsOfRound holds a hit's procs to what is left of the most their effects
// may do in the bearer's round (EffectRules.ProcRound: "up to 800 per round",
// weaponproc.go): each is cut to what is left, and one with nothing left does
// not happen. A round is RoundStart's (rounds.go), whatever its number of hits.
func (p *Player) procsOfRound(hits []procRoll) []procRoll {
	out := hits[:0]
	for _, h := range hits {
		if most := h.ae.Rules.ProcRound; most > 0 {
			if !p.procRound.Equal(p.RoundStart) {
				p.procRound, p.procRoundDone = p.RoundStart, 0
			}
			h.n = min(h.n, most-p.procRoundDone)
			if h.n <= 0 {
				continue
			}
			p.procRoundDone += h.n
		}
		out = append(out, h)
	}
	return out
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
	w.procHealLocked(p, sess, p, n)
}

// procHealLocked is a proc's heal of t (a life steal's, a heal proc's, a weapon's, a
// spell proc's share for its bearer, a duel's): healLocked's, and when t is at its most
// already the figure alone, green, so a proc's heal is always seen (the user, 2026-11-08:
// "the heal proc should show green numbers for player": a healer kept Larrybird full, and
// his Shroud of Death's heals showed nothing). A spell's heal on a full target still shows
// none (healLocked). Must be called with the lock held.
func (w *World) procHealLocked(caster *Player, casterSess *session.Session, t *Player, n int) {
	if n > 0 && !t.IsDead() && t.HP >= t.MaxHPNow() {
		w.tellAmountLocked(caster, casterSess, t.ServerID, n, numberProc)
		return
	}
	w.healLocked(caster, casterSess, t, n, numberProc)
}

// procHitFX is the look of ae's proc on what it hits: its own (EffectRules.ProcFX, the effects
// JSON's proc_fx: the owner, 2026-11-11, "0x830d8a97 is the dictid for LoP proc"), else a
// damage shield's answer's (fxShieldHit).
func procHitFX(ae *ActiveEffect) uint32 {
	if ae != nil && ae.Rules.ProcFX != 0 {
		return ae.Rules.ProcFX
	}
	return fxShieldHit
}

// playerProcsLocked runs a player's procs after one of its melee hits on a
// creature still standing. It reports a kill.
func (w *World) playerProcsLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	undead := func() bool { return w.undeadLocked(npc) }
	hits, resisted := procRollsOf(p.Effects, p.StatNow, p.Level, npc.Level, npcResistOffPct(npc, now), undead, now, procIntn)
	if resisted > 0 {
		log.Printf("Proc: %s's proc resisted by %s", p.Name, npc.Name)
	}
	for _, h := range p.procsOfRound(hits) {
		n := -creatureResists(h.ae.Rules.Resist, npc, -h.n, now)
		log.Printf("Proc: %s's %q hits %s for %d", p.Name, h.ae.Name, npc.Name, n)
		// seen as a damage shield's answer is (petition 1156: "does not proc",
		// when all it showed was one more figure among the swings')
		w.broadcastCastLocked(p, npc.ServerID, 0, procHitFX(h.ae))
		killed := w.spellHitLocked(p, sess, npc, n, numberProc)
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
	hits, resisted := procRollsOf(pet.Effects, petStat, pet.Level, t.Level, npcResistOffPct(t, now), undead, now, procIntn)
	if resisted > 0 {
		log.Printf("Proc: the pet %q's proc resisted by %s", pet.Name, t.Name)
	}
	for _, h := range hits {
		log.Printf("Proc: the pet %q's %q hits %s for %d", pet.Name, h.ae.Name, t.Name, h.n)
		if fx := h.ae.Rules.ProcFX; fx != 0 {
			// its own look at what it hits (Largesse of Plague's); a proc of none shows its
			// figure alone, as before
			w.broadcastNPCSwingLocked(pet, t.ServerID, 0, fx)
		}
		if w.petDamageLocked(ps, t, h.n, now, numberProc) {
			return true
		}
	}
	return false
}

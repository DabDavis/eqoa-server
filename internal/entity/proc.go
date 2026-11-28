package entity

import (
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// proc.go — a weapon proc: an effect that, while it lasts, gives its
// bearer's melee hits a chance to do more (F3 20261025-165245, a GM: the
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
// that states its own chance keeps it. The rest not measured (the reference server's
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

// procRoll is one proc a hit made: its damage and the effect that made it, with the line's own
// resist, look and percent more on a debuffed creature; tap says its effect's heal goes with
// it (procTapLocked: the first proc line's alone, so a life steal heals once a proc).
type procRoll struct {
	n           int
	ae          *ActiveEffect
	resist      Stat
	fx          uint32
	debuffedPct int
	tap         bool
}

// MoreProc is a proc buff's second or later offensive proc (ActiveEffect.MoreProcs): its own
// damage and range, its own scale on its bearer's stat, cap, resist, look and chance, each
// rolled and resisted apart from the first's (petition 5645, Garrak: Shroud of Death's two
// procs; until 2026-11-26 the second's damage and scale overwrote the first's at the cast,
// the resist staying the first's, so a hit made one proc of the two mixed). Kept over a
// logout (buffsave.go).
type MoreProc struct {
	Proc        int    `json:",omitempty"`
	ProcMax     int    `json:",omitempty"`
	Scale       Stat   `json:",omitempty"`
	ScalePct    int    `json:",omitempty"`
	Cap         int    `json:",omitempty"`
	Undead      bool   `json:",omitempty"`
	Resist      Stat   `json:",omitempty"`
	FX          uint32 `json:",omitempty"`
	DebuffedPct int    `json:",omitempty"`
	// Chance is its own chance a hit in percent, 0 its buff's (procRollsOf)
	Chance int `json:",omitempty"`
}

// moreProcOf is a proc buff's later proc line e as it lands from caster, as lastingParts reads
// the first: a bearer's scale kept for each proc, a caster's added now, capped; its resist its
// own, else spellResist (the spell's).
func moreProcOf(e SpellEffect, caster *Player, spellResist Stat) MoreProc {
	m := MoreProc{Cap: e.Cap, Undead: e.Undead, Resist: e.Resist, FX: e.ProcFX, DebuffedPct: e.DebuffedPct, Chance: e.Chance}
	if m.Resist == StatNone {
		m.Resist = spellResist
	}
	if e.ScaleBearer {
		m.Proc, m.ProcMax = e.Amount, max(e.AmountMax, e.Amount)
		m.Scale, m.ScalePct = e.Scale, e.ScalePct
		return m
	}
	bonus := procBonus(e, caster)
	m.Proc, m.ProcMax = e.Amount+bonus, max(e.AmountMax, e.Amount)+bonus
	if e.Cap > 0 {
		m.Proc, m.ProcMax = min(m.Proc, e.Cap), min(m.ProcMax, e.Cap)
	}
	return m
}

// procLines is ae's offensive procs, each one rolled apart: the first is its own fields (Proc,
// its Rules' resist, look and percent more, as every proc buff's was), then MoreProcs.
func (ae *ActiveEffect) procLines() []MoreProc {
	if ae.Proc <= 0 && len(ae.MoreProcs) == 0 {
		return nil
	}
	lines := make([]MoreProc, 0, 1+len(ae.MoreProcs))
	if ae.Proc > 0 {
		lines = append(lines, MoreProc{Proc: ae.Proc, ProcMax: ae.ProcMax, Scale: ae.ProcScale, ScalePct: ae.ProcScalePct,
			Cap: ae.ProcCap, Undead: ae.ProcUndead, Resist: ae.Rules.Resist, FX: ae.Rules.ProcFX, DebuffedPct: ae.Rules.ProcDebuffedPct})
	}
	return append(lines, ae.MoreProcs...)
}

// procRollsOf is procRolls with each proc's effect.
func procRollsOf(effects []*ActiveEffect, stat func(Stat) int, bearerLevel, targetLevel, resistOff int, undead func() bool,
	now time.Time, intn func(int) int) (hits []procRoll, resisted int) {
	intel := stat(StatInt)
	for _, ae := range effects {
		if !now.Before(ae.Until) {
			continue
		}
		for i, l := range ae.procLines() {
			if l.Proc <= 0 || (l.Undead && !undead()) {
				continue
			}
			chance := ae.Rules.ProcChance // its own (a form's: weaponproc.go), else by INT
			if ae.Rules.ProcChanceFrom > 0 {
				chance = dexChancePct(ae.Rules.ProcChanceFrom, ae.Rules.ProcChanceMost, stat(StatDex)) // by DEX (weaponproc.go)
			}
			if l.Chance > 0 {
				chance = l.Chance // a later line's own
			}
			if chance <= 0 {
				chance = procChancePct(intel)
			}
			if intn(100) >= chance {
				continue
			}
			roll := procRoll{ae: ae, resist: l.Resist, fx: l.FX, debuffedPct: l.DebuffedPct, tap: i == 0 && ae.Proc > 0}
			if intn(100) < procResistPct(bearerLevel, targetLevel)-resistOff {
				// resisted, it does the least it can (workbook.go's leastHarm): its range's
				// low end, no bearer's stat
				resisted++
				roll.n = capAmount(l.Proc, l.Cap)
				hits = append(hits, roll)
				continue
			}
			n := l.Proc
			if l.ProcMax > l.Proc {
				n += intn(l.ProcMax - l.Proc + 1)
			}
			if l.Scale != StatNone && l.ScalePct != 0 {
				n += stat(l.Scale) * l.ScalePct / 100
			}
			roll.n = capAmount(n, l.Cap)
			hits = append(hits, roll)
		}
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
//
// more is the percent more it heals (procDebuffedMore: its hit's own), 0 for none.
func (w *World) procTapLocked(p *Player, sess *session.Session, ae *ActiveEffect, npc *NPC, more int) {
	r := ae.Rules
	if r.ProcHeal <= 0 || r.ProcHealDef || (r.ProcHealUndead && (npc == nil || !w.undeadLocked(npc))) {
		return
	}
	n := r.ProcHeal
	if r.ProcHealScale != StatNone && r.ProcHealScalePct != 0 {
		n += p.StatNow(r.ProcHealScale) * r.ProcHealScalePct / 100
	}
	if more > 0 {
		n = n * (100 + more) / 100
	}
	w.procHealGroupLocked(p, sess, ae, n) // workbook.go: its group too, where it reaches them
}

// procDebuffedMore is the percent more ae's proc does, its hit and its heal, at a hit on
// npc: its ProcDebuffedPct when npc carries a lasting harmful spell of the bearer's (bearer:
// its id), a debuff, a snare or a damage over time it put there; else none. Petition 1805
// (a player): the Undead Servant's lifetap, "a 80 lifetap proc that after the mob is
// debuffed, will often proc 160 instead".
func procDebuffedMore(ae *ActiveEffect, npc *NPC, bearer EntityID, now time.Time) int {
	if ae == nil {
		return 0
	}
	return procDebuffedMorePct(ae.Rules.ProcDebuffedPct, npc, bearer, now)
}

// procDebuffedMorePct is procDebuffedMore of a proc line's own percent (procRoll.debuffedPct).
func procDebuffedMorePct(pct int, npc *NPC, bearer EntityID, now time.Time) int {
	if pct <= 0 || npc == nil {
		return 0
	}
	for _, e := range npc.Effects {
		if e.Caster == bearer && e.Rules.Harm && now.Before(e.Until) {
			return pct
		}
	}
	return 0
}

// procHealLocked is a proc's heal of t (a life steal's, a heal proc's, a weapon's, a
// spell proc's share for its bearer, a duel's): healLocked's, and when t is at its most
// already the figure alone, green, so a proc's heal is always seen (the user, 2026-11-08:
// "the heal proc should show green numbers for player": a healer kept a player full, and
// his Shroud of Death's heals showed nothing). A spell's heal on a full target still shows
// none (healLocked). Its figure is told as a heal's (numberHeal, green on Default), not a
// proc's: petition 3022 (a player colouring "Your Procs" saw Crusader's Aura's heal in the
// proc colour; the owner, 2026-11-25: "you can draw proc heals in green"). Must be called
// with the lock held.
func (w *World) procHealLocked(caster *Player, casterSess *session.Session, t *Player, n int) {
	if n > 0 {
		n += heroicHealBonus(caster)
	}
	if n > 0 && !t.IsDead() && t.HP >= t.MaxHPNow() {
		w.dpsWorthLocked(0, func() { w.tellAmountLocked(caster, casterSess, t.ServerID, n, numberHeal) }) // the meter's: nothing restored (dpsheal.go)
		return
	}
	w.healLocked(caster, casterSess, t, n, numberHeal)
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

// rollHitFX is the look of one proc roll on what it hits: its line's own (MoreProc.FX, the
// first line's its effect's), else the stand-in (procHitFX).
func rollHitFX(h procRoll) uint32 {
	if h.fx != 0 {
		return h.fx
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
		more := procDebuffedMorePct(h.debuffedPct, npc, p.ServerID, now)
		n := -creatureResists(h.resist, npc, -h.n*(100+more)/100, now) // the line's own resist (petition 5645)
		log.Printf("Proc: %s's %q hits %s for %d", p.Name, h.ae.Name, npc.Name, n)
		// seen as a damage shield's answer is (petition 1156: "does not proc",
		// when all it showed was one more figure among the swings')
		w.broadcastCastLocked(p, npc.ServerID, 0, rollHitFX(h))
		killed := w.spellHitLocked(p, sess, npc, n, numberProc)
		if h.tap {
			w.procTapLocked(p, sess, h.ae, npc, procDebuffedMore(h.ae, npc, p.ServerID, now))
		}
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
	return npc.Dead || w.heroicAttackProcsLocked(sess, p, npc, now)
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
		if fx := h.fx; fx != 0 {
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

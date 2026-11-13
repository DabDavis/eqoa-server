package entity

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/database"
)

// duelpet.go — a duelist's pet fights a duel as it fights a creature (2026-11-13, Ruggid: "lol pet
// didnt swing?" in a duel with Hoofbite, and the owner: "the pet is not procing or using abilities
// also"). Its melee landed (duelPetRoundLocked), told to its owner as the owner's own figures, but
// that was all it did: its template's abilities (the Swarming Death's damage over time, its life
// tap), its template's melee proc (Swarming Sting) and the procs of the effects on it (its owner's
// buffs: Soul Mastery +, Mastery of Plague), which against a creature are most of its damage, never
// ran in a duel. Now, as against a creature:
//
//   - at each turn in reach its template's abilities cast at its foe (nukes, area nukes at the foe
//     alone, life taps, damages over time), on their own clocks, in the open (0x00B1 with the
//     spell's cast time and visual), landing after their cast time;
//   - after each of its melee hits that lands, its template's proc and its effects' procs;
//
// every figure duel damage (duelHurtLocked: at DuelDamageShare, or a PvP fight's share, and never
// past the loser's hit point), from the pet (the target sees the pet's figure). A damage over time
// on the opponent is the duel's (duelFoe: off at its end), each tick halved; on the opponent's pet
// its first tick alone lands, as nothing lasts on a duel's pet (duelLandPetLocked). Its melee is
// its template's (melee_pct, a recorded kind's scale) as against a creature. Every line is in the
// log as "Duel N: X's pet Y ...". Must be called with the lock held.

// duelMarkOfLocked is the duel's mark at target: its opponent, or the opponent's pet.
func (w *World) duelMarkOfLocked(target EntityID) (duelMark, bool) {
	if t := w.Players[target]; t != nil {
		return duelMark{t: t}, true
	}
	if pet := w.NPCs[target]; pet != nil {
		return duelMark{pet: pet}, true
	}
	return duelMark{}, false
}

// duelPetHurtLocked lands n of the pet's duel damage (before the halving) on the mark: told to
// its owner as its pet's (kind), seen by the target as the pet's. It reports the duel over, or
// the mark's pet out of it.
func (w *World) duelPetHurtLocked(d *Duel, ps *petState, m duelMark, n int, now time.Time, kind numberKind) bool {
	ps.npc.LastCombatTime = now
	if m.t != nil {
		return w.duelHurtLocked(d, ps.owner, ps.npc.ServerID, m.t, n, now, kind.byPet())
	}
	w.duelHurtPetLocked(d, ps.owner, m.pet, n, now, kind.byPet())
	return d.petOut[m.pet.Owner]
}

// duelPetProcsLocked runs the pet's procs after one of its melee hits landed on the mark: its
// template's (petTemplateProcLocked's dice and figure) and its effects' (petProcsLocked's: a
// pet's stats its INT's stand-in). It reports the duel over, or the mark's pet out of it.
func (w *World) duelPetProcsLocked(d *Duel, ps *petState, m duelMark, now time.Time) bool {
	pet := ps.npc
	if ps.tmpl != nil {
		tm := ps.tmpl.tmpl
		if tm.ProcChance > 0 && tm.ProcDamage > 0 && templateIntn(100) < tm.ProcChance {
			w.broadcastNPCSwingLocked(pet, m.id(), 0, fxShieldHit)
			log.Printf("%s: %s's pet %q's %s hits %s for %d", d.tag(), ps.owner.Name, pet.Name, procName(tm), m.name(), tm.ProcDamage)
			if w.duelPetHurtLocked(d, ps, m, tm.ProcDamage, now, numberProc) {
				return true
			}
		}
	}
	petStat := func(Stat) int { return petIntPerLvl * pet.Level }
	undead := func() bool { return m.pet != nil && w.undeadLocked(m.pet) }
	hits, resisted := procRollsOf(pet.Effects, petStat, pet.Level, m.level(), 0, undead, now, procIntn)
	if resisted > 0 {
		log.Printf("%s: %s's pet %q's proc resisted by %s", d.tag(), ps.owner.Name, pet.Name, m.name())
	}
	for _, h := range hits {
		log.Printf("%s: %s's pet %q's %q hits %s for %d", d.tag(), ps.owner.Name, pet.Name, h.ae.Name, m.name(), h.n)
		if fx := h.ae.Rules.ProcFX; fx != 0 {
			w.broadcastNPCSwingLocked(pet, m.id(), 0, fx)
		}
		if w.duelPetHurtLocked(d, ps, m, h.n, now, numberProc) {
			return true
		}
	}
	return false
}

// duelPetKind is whether a template ability is one the pet casts at its duel foe: its harm (a
// root, a debuff, a taunt, a strip are a creature fight's).
func duelPetKind(kind string) bool {
	switch kind {
	case database.AbilityNuke, database.AbilityAoE, database.AbilityLifetap, database.AbilityDoT:
		return true
	}
	return false
}

// duelPetTurnLocked is the pet's template's turn in a duel, at each of its turns in reach: its
// heals and buffs as while it follows (petTemplateTurnLocked), then the first of its harmful
// abilities due at the mark (on the clocks a creature fight keeps: its first into the fight, then
// its every). It reports whether it is casting, its melee waiting.
func (w *World) duelPetTurnLocked(ps *petState, m duelMark, now time.Time) bool {
	abilities := ps.petAbilities()
	if len(abilities) == 0 {
		return false
	}
	if w.petTemplateTurnLocked(ps, nil, now) {
		return true
	}
	pet := ps.npc
	f := ps.petAbilityClocks(m.id(), now)
	if now.Before(f.busy) {
		return true
	}
	for i := range abilities {
		a := &abilities[i]
		if !duelPetKind(a.Kind) || a.Chain || now.Before(f.next[i]) || pet.Level < a.From {
			continue
		}
		spell := a.spellFor(petSpellClass(a, pet), pet.Level)
		if spell == nil || spell.Def.undeadOnly() && (m.pet == nil || !w.undeadLocked(m.pet)) {
			continue
		}
		if a.Kind == database.AbilityDoT && w.duelPetDoTUpLocked(ps, a, spell, m, now) {
			continue // kept up: again as the one on the foe is about to run out
		}
		f.next[i] = now.Add(a.every)
		if c := a.ChanceOr(); c < 100 && templateIntn(100) >= c {
			continue
		}
		cost := petCastCost(pet, a, spell)
		if cost > pet.Power {
			continue
		}
		ct := a.castTime(spell)
		w.petCasts = append(w.petCasts, &petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: spell, target: m.id(),
			lands: now.Add(ct + castLand), duel: true})
		pet.Power -= cost
		f.busy = now.Add(ct + castBusy)
		w.broadcastNPCSwingLocked(pet, m.id(), uint32(ct/time.Millisecond), spell.visual())
		log.Printf("Duel: %s's pet %q begins %q (spell %d) on %s: %v cast", ps.owner.Name, pet.Name, spell.Name, spell.SpellID, m.name(), ct)
		return now.Before(f.busy)
	}
	return false
}

// duelPetDoTUpLocked is petDoTUpLocked at a duel's mark: the pet's damage over time on its way
// there, or on the opponent from this pet with more than the cast's time and its landing to run.
func (w *World) duelPetDoTUpLocked(ps *petState, a *templateAbility, s *CastSpell, m duelMark, now time.Time) bool {
	for _, c := range w.petCasts {
		if c.ps == ps && c.target == m.id() && c.spell.SpellID == s.SpellID {
			return true
		}
	}
	if m.t == nil {
		return false
	}
	lead := a.castTime(s) + castLand
	for _, e := range m.t.Effects {
		if e.SpellID == s.SpellID && e.pet == ps.npc.ServerID && e.Until.Sub(now) > lead {
			return true
		}
	}
	return false
}

// duelPetLandLocked lands a pet's cast at a duel's mark (petCastSystemLocked), while the fight
// with the mark goes on: a hit's figure (petLandLocked's: its sheet's, else its template's, at
// the ability's percent) as duel damage, a tap's drained share of what landed back on the pet; a
// damage over time on the opponent as the duel's.
func (w *World) duelPetLandLocked(c *petCast, now time.Time) {
	ps, pet, a := c.ps, c.ps.npc, c.a
	owner := ps.owner
	m, ok := w.duelMarkOfLocked(c.target)
	if !ok || owner.IsDead() {
		return
	}
	var d *Duel
	if m.t != nil {
		d = w.fightBetweenLocked(owner, m.t) // their duel, or a PvP fight (pvp.go)
		if m.t.IsDead() {
			d = nil
		}
	} else if dd := w.duelOnLocked(owner.ServerID); dd != nil && m.pet.Owner == dd.other(owner.ServerID) && !dd.petOut[m.pet.Owner] {
		d = dd
	}
	if d == nil {
		log.Printf("Duel: %s's pet %q's %q on %s lands on nothing: no fight between them", owner.Name, pet.Name, c.spell.Name, m.name())
		return
	}
	switch a.Kind {
	case database.AbilityNuke, database.AbilityAoE, database.AbilityLifetap:
		hit, drained := templateFigure(c.spell.Def.Effects, a.DamageOr())
		if pet.sheet != nil {
			hit, drained = sheetFigure(pet, c.spell.Def.Effects, a.DamageOr(), false)
		}
		if hit <= 0 {
			return
		}
		log.Printf("%s: %s's pet %q lands %q on %s for %d before the halving", d.tag(), owner.Name, pet.Name, c.spell.Name, m.name(), hit)
		w.duelPetHurtLocked(d, ps, m, hit, now, numberSpell)
		if drained > 0 && pet.HP < pet.MaxHP && !pet.Dead {
			n := min(int(math.Round(float64(d.hit(hit))*float64(drained)/float64(hit))), pet.MaxHP-pet.HP)
			if n > 0 {
				pet.HP += n
				w.tellPetHealLocked(ps, n)
			}
		}
	case database.AbilityDoT:
		tick := petTickFigure(pet, c.spell.Def.Effects, a.DamageOr())
		if tick <= 0 {
			return
		}
		if m.t == nil {
			// nothing lasts on a duel's pet: its first tick alone
			log.Printf("%s: %s's pet %q's %q ticks on %s for %d before the halving", d.tag(), owner.Name, pet.Name, c.spell.Name, m.name(), tick)
			w.duelPetHurtLocked(d, ps, m, tick, now, numberDoT)
			return
		}
		dur := c.spell.Def.Duration
		if dur <= 0 {
			dur = tickDuration
		}
		pet.LastCombatTime = now
		log.Printf("%s: %s's pet %q puts %q on %s: %d a tick (before the halving) for %v", d.tag(), owner.Name, pet.Name, c.spell.Name, m.name(), tick, dur)
		w.addEffectLocked(m.t, &ActiveEffect{SpellID: c.spell.SpellID, Name: c.spell.Name, Icon: c.spell.Icon, Caster: owner.ServerID,
			Until: now.Add(dur), NextTick: now, HPTick: -tick, pet: pet.ServerID, duelFoe: owner.ServerID,
			Rules: EffectRules{Harm: true}}, now)
	}
}

package entity

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/database"
)

// pettemplate.go — a summoned pet's own class (the user, 2026-10-30: "necro
// pets are like SKs that quad, or 2H if the player bought the CMs. Magician
// pets are more varied. the water pet is a single hitting healer cleric that
// cast a offensive spell. earth is a tank that quads at higher levels and
// taunts and buffs ac/agi. fire pet is a melee that cast a fire offensive
// spell, also buffs str/dex, the highest dps for melee, the air pet is a
// wizard that casts several offensive nukes, it also single hits melee, can
// compete with fire pet on damage, just not as tanky. enchanter pets are all
// the same, a single hitting, a damaging taunt, and several self heals").
//
// A pet fights by the class template its summoning spell's pettemplate row
// names (database/pettemplates.go), picked when it is summoned:
//
//   - its class is the template's: its swing (CLIENT's GetSwing of its
//     level), unless the row names a swing class (a necromancer's pet, a
//     shadow knight, swings a warrior's round) or a fixed swing and round
//     (the Death Sacrament pets, Sandstorm's recorded rounds; its hits then
//     scaled to keep the damage a second its class's round gave it);
//   - hp_pct of its hit points (after the balance's pet hit points), and
//     taken_pct of a creature's melee hit on it lands; its own melee hits
//     are melee_pct of their roll;
//   - its abilities, cast from where it stands in reach of what it fights
//     (heals and buffs also while it follows), one at a time, its melee
//     waiting out a cast's time: a harmful spell of a class's (nuke,
//     lifetap) on what it fights, through the balance as its hits are; a
//     heal on its owner (below its share of hit points) or on itself (a
//     spell's figure below its share, or a share of its own most: of those
//     due, the biggest it is missing, never an overheal, each on its own
//     clock); a buff on itself when it lacks it (the user, 2026-10-30: "the
//     earth pet ... is casting the ac buff on the player instead of itself,
//     if fire does the same should also be on itself"), or on its owner when
//     its row's target says so and the owner lacks it (the spell's line: one
//     of a higher rank on keeps it off), its AC and AGI then counting
//     against a creature's hits on it and its STR and DEX in its own
//     (pet.go); a taunt on what it fights, the top of its hate and a hit (a
//     melee hit of its own, or the row's figure by level, landing as it is).
//
// A template's area spells and special are an NPC's alone. A pet with no
// row (a shaman's, a wizard's, the Grand Animation, a charmed creature)
// fights as pet.go always had it.

// petTemplate is a pettemplate row as the world summons by it: its template.
type petTemplate struct {
	database.PetTemplate
	tmpl *ClassTemplate
}

// petTemplateForLocked is the row a summoning spell's pet fights by: its
// name's (its weapon version's own, else its name's base row), with a
// template the world has; nil for none. Must be called with the lock held.
func (w *World) petTemplateForLocked(spell *CastSpell) *petTemplate {
	if len(w.templates.pets) == 0 {
		return nil
	}
	k := database.PetTemplateKey{Spell: database.TemplateKey(spell.Name), Weapon: w.templates.weapon[spell.SpellID]}
	pt := w.templates.pets[k]
	if pt == nil && k.Weapon {
		pt = w.templates.pets[database.PetTemplateKey{Spell: k.Spell}]
	}
	if pt == nil || pt.tmpl == nil {
		return nil
	}
	return pt
}

// petWorn is a pet's worn slots as its swing takes them (npcSwingKind's).
func petWorn(pet *NPC) [21]bool {
	var worn [21]bool
	worn[14] = pet.PrimaryWeapon != 0
	worn[17] = pet.Shield != 0
	return worn
}

// applyPetTemplateLocked makes a new pet its template's: its class, its swing
// and round, its hit points. It gives back the scale of its melee hits: a
// fixed swing keeps the damage a second its class's round gave it.
func (w *World) applyPetTemplateLocked(pet *NPC, pt *petTemplate) float64 {
	t := pt.tmpl
	if t.Class >= 0 {
		pet.Class = t.Class
	}
	pet.SwingFixed, pet.SwingKind, pet.SwingRound = false, 0, 0
	if pt.SwingClass >= 0 {
		pet.SwingFixed, pet.SwingKind = true, GetSwing(pt.SwingClass, pet.Level, petWorn(pet))
	}
	scale := 1.0
	if pt.Swing >= 0 {
		was := petHitRate(pet)
		pet.SwingFixed, pet.SwingKind = true, pt.Swing
		if pt.RoundMS > 0 {
			pet.SwingRound = time.Duration(pt.RoundMS) * time.Millisecond
		}
		scale = was / petHitRate(pet)
	} else if pt.RoundMS > 0 {
		pet.SwingFixed, pet.SwingKind = true, npcSwingKind(pet)
		pet.SwingRound = time.Duration(pt.RoundMS) * time.Millisecond
	}
	if t.HPPct != 100 {
		pet.MaxHP = max(1, int(math.Round(float64(pet.MaxHP)*float64(t.HPPct)/100)))
		pet.HP = pet.MaxHP
	}
	return scale
}

// meleeOut is one of the pet's melee hits as its template makes it.
func (ps *petState) meleeOut(n int) int {
	if ps.tmpl == nil || ps.tmpl.tmpl.MeleePct == 100 || n <= 0 {
		return n
	}
	return max(1, int(math.Round(float64(n)*float64(ps.tmpl.tmpl.MeleePct)/100)))
}

// petTakenLocked is a creature's melee hit on a pet as its template takes
// it (taken_pct), at least 1.
func (w *World) petTakenLocked(pet *NPC, n int) int {
	ps := w.pets[pet.Owner]
	if ps == nil || ps.npc != pet || ps.tmpl == nil || ps.tmpl.tmpl.TakenPct == 100 || n <= 0 {
		return n
	}
	return max(1, int(math.Round(float64(n)*float64(ps.tmpl.tmpl.TakenPct)/100)))
}

// petAbilities is the pet's template's abilities, none without one.
func (ps *petState) petAbilities() []templateAbility {
	if ps.tmpl == nil {
		return nil
	}
	return ps.tmpl.tmpl.abilities
}

// petSpellClass is the class whose spells the pet's ability casts: its own
// spells_of, else the template's class (the pet's).
func petSpellClass(a *templateAbility, pet *NPC) int {
	if a.SpellsOf != nil {
		return *a.SpellsOf
	}
	return pet.Class
}

// petAbilityClocks starts the pet's ability clocks once (its heals and buffs
// keep theirs from fight to fight), and those of what it casts at a foe
// again for a new one (its first into that fight).
func (ps *petState) petAbilityClocks(foe EntityID, now time.Time) *templateFight {
	abilities := ps.petAbilities()
	f := ps.npc.fight()
	if !f.on {
		f.on = true
		for i := range abilities {
			f.next[i] = now.Add(abilities[i].first)
		}
	}
	if foe != 0 && foe != ps.foe {
		ps.foe = foe
		for i := range abilities {
			if petAtFoe(abilities[i].Kind) {
				f.next[i] = now.Add(abilities[i].first)
			}
		}
	}
	return f
}

// petAtFoe is whether a kind is cast at what the pet fights.
func petAtFoe(kind string) bool {
	switch kind {
	case database.AbilityNuke, database.AbilityLifetap, database.AbilityTaunt:
		return true
	}
	return false
}

// petTemplateTurnLocked is a templated pet's turn: in its fight with t (nil
// while it follows its owner: its heals and buffs alone), the biggest of its
// share heals that it is missing, else the first of its abilities that is
// due and wanted. It reports whether it is casting, its melee waiting. Must
// be called with the lock held.
func (w *World) petTemplateTurnLocked(ps *petState, t *NPC, now time.Time) bool {
	abilities := ps.petAbilities()
	if len(abilities) == 0 {
		return false
	}
	var foe EntityID
	if t != nil {
		foe = t.ServerID
	}
	f := ps.petAbilityClocks(foe, now)
	if now.Before(f.busy) {
		return true
	}
	if i := ps.petSharePickLocked(f, now); i >= 0 {
		f.next[i] = now.Add(abilities[i].every)
		w.petShareHealLocked(ps, &abilities[i], now)
	}
	if f.chain > 0 {
		i := f.chain - 1
		f.chain = 0
		if i < len(abilities) && w.petCastLocked(ps, i, t, now) {
			return now.Before(f.busy)
		}
	}
	for i := range abilities {
		a := &abilities[i]
		if a.Chain || now.Before(f.next[i]) || ps.npc.Level < a.From || petShareHeal(a) {
			continue
		}
		if petAtFoe(a.Kind) && t == nil || a.Kind == database.AbilityAoE {
			continue
		}
		if !w.petWantsLocked(ps, a, now) {
			continue // not wanted yet: it waits, due
		}
		f.next[i] = now.Add(a.every)
		if c := a.ChanceOr(); c < 100 && templateIntn(100) >= c {
			continue
		}
		if w.petCastLocked(ps, i, t, now) && now.Before(f.busy) {
			return true
		}
	}
	return false
}

// petShareHeal is whether an ability is a heal of the pet's own share of
// hit points on itself (the animations').
func petShareHeal(a *templateAbility) bool {
	return a.Kind == database.AbilityHeal && a.HealPct > 0 && a.TargetOr() == database.AbilityOnSelf
}

// petShareAmount is a share heal's hit points on a maximum: at least 1.
func petShareAmount(maxHP, pct int) int {
	return max(1, int(math.Round(float64(maxHP)*float64(pct)/100)))
}

// petSharePickLocked is the share heal the pet casts now, its index: of
// those due and from its level, the biggest whose hit points it is missing
// (no overheal: the user, "it would always use the biggest available, IF
// below the hp amount required for it to not overheal"); -1 for none.
func (ps *petState) petSharePickLocked(f *templateFight, now time.Time) int {
	pet := ps.npc
	missing := pet.MaxHP - pet.HP
	best, most := -1, 0
	for i, a := range ps.petAbilities() {
		if !petShareHeal(&a) || a.Chain || now.Before(f.next[i]) || pet.Level < a.From {
			continue
		}
		if n := petShareAmount(pet.MaxHP, a.HealPct); n <= missing && n > most {
			best, most = i, n
		}
	}
	return best
}

// petShareHealLocked lands a share heal on the pet at once: its share of
// its most hit points, its visual from it at itself (cast 0: its pinned
// spell's, else a heal's), the owner told over the pet as a tap's heal is.
func (w *World) petShareHealLocked(ps *petState, a *templateAbility, now time.Time) {
	pet := ps.npc
	n := min(petShareAmount(pet.MaxHP, a.HealPct), pet.MaxHP-pet.HP)
	if n <= 0 {
		return
	}
	pet.HP += n
	fx := fxHeal
	if a.pinned != nil {
		fx = a.pinned.visual()
	}
	w.broadcastNPCSwingLocked(pet, pet.ServerID, 0, fx)
	w.tellPetHealLocked(ps, n)
	log.Printf("Pet: %s's %q heals itself %d%% for %d (HP %d/%d)", ps.owner.Name, pet.Name, a.HealPct, n, pet.HP, pet.MaxHP)
}

// tellPetHealLocked tells the owner of a heal on its pet: a 0x00DB from the
// owner over the pet, as a tap's heal is told (pettap.go).
func (w *World) tellPetHealLocked(ps *petState, n int) {
	if sess := w.findSessionByEntityIDLocked(ps.owner.ServerID); sess != nil {
		if id := w.objectIDForEntityLocked(sess, ps.npc.ServerID); id != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), n, id)
		}
	}
}

// petWantsLocked is whether a due ability is wanted now: a heal on its
// owner or a spell's on itself below its share, a buff its target (itself,
// or its owner) lacks; anything at a foe, yes.
func (w *World) petWantsLocked(ps *petState, a *templateAbility, now time.Time) bool {
	switch a.Kind {
	case database.AbilityHeal:
		if a.TargetOr() == database.AbilityOnOwner {
			o := ps.owner
			top := o.MaxHPNow()
			return top > 0 && o.HP*100 < top*a.BelowOr()
		}
		return ps.npc.HP*100 < ps.npc.MaxHP*a.BelowOr()
	case database.AbilityBuff:
		s := a.spellFor(petSpellClass(a, ps.npc), ps.npc.Level)
		if s == nil {
			return false
		}
		if a.TargetOr() == database.AbilityOnOwner {
			return !buffOn(ps.owner.Effects, s)
		}
		return !buffOn(ps.npc.Effects, s)
	}
	return true
}

// buffOn is whether effects hold the buff's effect, or one of its line of a
// higher rank (which would keep it off: buffline.go, putOnPetLocked).
func buffOn(effects []*ActiveEffect, s *CastSpell) bool {
	line := BuffLine(s)
	for _, e := range effects {
		if e.SpellID == s.SpellID || line != "" && e.Line == line && e.Rank >= s.Level {
			return true
		}
	}
	return false
}

// petCast is a templated pet's cast on its way to landing (petCastSystem).
type petCast struct {
	ps     *petState
	tmpl   *petTemplate // the row it was cast by: another since drops it
	a      *templateAbility
	spell  *CastSpell
	target EntityID // the creature hit, or the one healed or buffed
	lands  time.Time
}

// petCastLocked begins the pet's ability i (at t, a foe, when it is cast at
// one): a taunt lands at once; a spell's 0x00B1 goes to everyone who sees
// the pet with its cast time and visual, and it lands castLand after its
// cast time. It reports whether it was cast: not when it has no spell at the
// pet's level, nor at a foe gone. Must be called with the lock held.
func (w *World) petCastLocked(ps *petState, i int, t *NPC, now time.Time) bool {
	abilities := ps.petAbilities()
	a := &abilities[i]
	pet := ps.npc
	if a.Kind == database.AbilityTaunt {
		if t == nil || t.Dead {
			return false
		}
		w.petTemplateTauntLocked(ps, a, t, now)
		return true
	}
	spell := a.spellFor(petSpellClass(a, pet), pet.Level)
	if spell == nil {
		return false
	}
	c := &petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: spell, lands: now.Add(spell.CastTime + castLand)}
	switch {
	case petAtFoe(a.Kind):
		if t == nil || t.Dead {
			return false
		}
		c.target = t.ServerID
	case a.TargetOr() == database.AbilityOnOwner:
		c.target = ps.owner.ServerID
	default:
		c.target = pet.ServerID
	}
	w.petCasts = append(w.petCasts, c)
	f := pet.fight()
	f.busy = now.Add(spell.CastTime + castBusy)
	if j := i + 1; j < len(abilities) && abilities[j].Chain {
		f.chain = j + 1
	}
	w.broadcastNPCSwingLocked(pet, c.target, uint32(spell.CastTime/time.Millisecond), spell.visual())
	log.Printf("Pet: %s's %q (%s) begins %q (spell %d) on %s: %v cast", ps.owner.Name, pet.Name, ps.tmpl.Template, spell.Name,
		spell.SpellID, w.entityNameLocked(c.target), spell.CastTime)
	return true
}

// petCastSystemLocked lands every templated pet's cast whose time has come,
// by a pet still its owner's, alive, by the row it was cast by. Every tick
// (petSystem); must be called with the lock held.
func (w *World) petCastSystemLocked(now time.Time) {
	if len(w.petCasts) == 0 {
		return
	}
	var due []*petCast
	keep := w.petCasts[:0]
	for _, c := range w.petCasts {
		if now.Before(c.lands) {
			keep = append(keep, c)
		} else {
			due = append(due, c)
		}
	}
	for i := len(keep); i < len(w.petCasts); i++ {
		w.petCasts[i] = nil
	}
	w.petCasts = keep
	for _, c := range due {
		ps := c.ps
		if ps.npc.Dead || w.pets[ps.owner.ServerID] != ps || ps.tmpl != c.tmpl {
			continue
		}
		w.petLandLocked(c, now)
	}
}

// petLandLocked lands a pet's cast: a harmful spell's figure (its hits
// rolled, no stat: a pet has none, at the ability's percent) on the
// creature as one of the pet's hits (petDamageLocked), a tap's drained
// share back on the pet; a heal's figure on its owner or itself; a buff on
// itself (petBuffItselfLocked), or its lasting effect on its owner, as the
// owner's own cast on itself.
func (w *World) petLandLocked(c *petCast, now time.Time) {
	ps, pet, a := c.ps, c.ps.npc, c.a
	switch a.Kind {
	case database.AbilityNuke, database.AbilityLifetap:
		t := w.NPCs[c.target]
		if t == nil || t.Dead || t.World != pet.World {
			return
		}
		hit, drained := templateFigure(c.spell.Def.Effects, a.DamageOr())
		if hit <= 0 {
			return
		}
		pet.LastCombatTime = now
		log.Printf("Pet: %s's %q lands %q on %s for %d before the balance", ps.owner.Name, pet.Name, c.spell.Name, t.Name, hit)
		w.petDamageLocked(ps, t, hit, now)
		if drained > 0 && pet.HP < pet.MaxHP {
			n := min(drained, pet.MaxHP-pet.HP)
			pet.HP += n
			w.tellPetHealLocked(ps, n)
		}
	case database.AbilityHeal:
		n := petHealFigure(c.spell, a)
		if c.target == ps.owner.ServerID {
			o := ps.owner
			if a.HealPct > 0 {
				n = petShareAmount(o.MaxHPNow(), a.HealPct)
			}
			if n <= 0 || o.IsDead() || o.HP >= o.MaxHPNow() {
				return
			}
			n = min(n, o.MaxHPNow()-o.HP)
			o.HP += n
			o.StatsDirty = true
			if sess := w.findSessionByEntityIDLocked(o.ServerID); sess != nil {
				if id := w.objectIDForEntityLocked(sess, pet.ServerID); id != 0 {
					w.sendDamageIndication(sess, id, n, sess.PlayerObjectID())
				}
			}
			log.Printf("Pet: %s's %q heals its owner with %q for %d (HP %d/%d)", o.Name, pet.Name, c.spell.Name, n, o.HP, o.MaxHPNow())
			return
		}
		if a.HealPct > 0 {
			n = petShareAmount(pet.MaxHP, a.HealPct)
		}
		if n = min(n, pet.MaxHP-pet.HP); n > 0 {
			pet.HP += n
			w.tellPetHealLocked(ps, n)
			log.Printf("Pet: %s's %q heals itself with %q for %d (HP %d/%d)", ps.owner.Name, pet.Name, c.spell.Name, n, pet.HP, pet.MaxHP)
		}
	case database.AbilityBuff:
		if c.target == pet.ServerID {
			w.petBuffItselfLocked(ps, c.spell, now)
			return
		}
		o := ps.owner
		if o.IsDead() || buffOn(o.Effects, c.spell) {
			return
		}
		cs := &castState{spell: *c.spell, caster: o.ServerID, targets: []EntityID{o.ServerID}}
		w.landOnPlayerLocked(cs, o, w.findSessionByEntityIDLocked(o.ServerID), o, now)
		log.Printf("Pet: %s's %q buffs its owner with %q", o.Name, pet.Name, c.spell.Name)
	}
}

// petBuffItselfLocked lands a pet's buff on itself: what it heals now, and
// what lasts kept on it as the best of its line (putOnPetLocked: not under
// a higher rank of it), its figures rolled as its owner's cast's (a pet has
// no stat to scale them).
func (w *World) petBuffItselfLocked(ps *petState, s *CastSpell, now time.Time) {
	pet, o := ps.npc, ps.owner
	cs := &castState{spell: *s, caster: o.ServerID, targets: []EntityID{pet.ServerID}}
	ae := lasting(cs, o, now)
	if ae == nil || buffOn(pet.Effects, s) {
		return
	}
	for _, e := range s.Def.Effects {
		if e.Kind == EffectHP && !e.Self {
			if n := spellAmount(e, o); n > 0 && pet.HP < pet.MaxHP {
				n = min(n, pet.MaxHP-pet.HP)
				pet.HP += n
				w.tellPetHealLocked(ps, n)
			}
		}
	}
	putOnPetLocked(pet, s, ae)
	log.Printf("Pet: %s's %q buffs itself with %q (%d effects on it)", o.Name, pet.Name, s.Name, len(pet.Effects))
}

// petHealFigure is a heal spell's figure from a pet at the ability's
// percent: each heal rolled in its range, no stat.
func petHealFigure(s *CastSpell, a *templateAbility) int {
	n := 0
	for _, e := range s.Def.Effects {
		if e.Kind == EffectHP && e.Amount > 0 && !e.Self && e.Drain == DrainNone {
			n += int(math.Round(float64(capAmount(rollAmount(e), e.Cap)) * float64(a.DamageOr()) / 100))
		}
	}
	return n
}

// petTauntFigure is a taunt's damage from its hits points at the pet's
// level: a roll between the line's least and most there, at least 1; ok
// false with no points.
func petTauntFigure(hits [][3]float64, level int) (n int, ok bool) {
	if len(hits) == 0 {
		return 0, false
	}
	least, most := hits[0][1], hits[0][2]
	switch x := float64(level); {
	case x <= hits[0][0]:
	case x >= hits[len(hits)-1][0]:
		least, most = hits[len(hits)-1][1], hits[len(hits)-1][2]
	default:
		for i := 1; i < len(hits); i++ {
			if x <= hits[i][0] {
				a, b := hits[i-1], hits[i]
				f := (x - a[0]) / (b[0] - a[0])
				least, most = a[1]+(b[1]-a[1])*f, a[2]+(b[2]-a[2])*f
				break
			}
		}
	}
	lo, hi := int(math.Round(least)), int(math.Round(most))
	if hi > lo {
		lo += templateIntn(hi - lo + 1)
	}
	return max(lo, 1), true
}

// petTemplateTauntLocked is a templated pet's taunt on t: the top of its
// hate (pet.go petTauntLocked's), and a hit: the row's figure by level,
// landing as it is, else one of its melee hits at the ability's percent,
// through the balance as its hits are.
func (w *World) petTemplateTauntLocked(ps *petState, a *templateAbility, t *NPC, now time.Time) bool {
	pet := ps.npc
	w.petTopHateLocked(pet, t)
	pet.LastCombatTime = now
	if n, ok := petTauntFigure(a.Hits, pet.Level); ok {
		log.Printf("Pet: %s's %q taunts %s for %d", ps.owner.Name, pet.Name, t.Name, n)
		return w.petLandHitLocked(ps, t, n, now, false)
	}
	n := float64(petMeleeHit(pet, t, meleeIntn))
	if ps.hitScale > 0 {
		n *= ps.hitScale
	}
	m := ps.meleeOut(int(math.Round(n * float64(a.DamageOr()) / 100)))
	log.Printf("Pet: %s's %q taunts %s (a melee hit of %d)", ps.owner.Name, pet.Name, t.Name, m)
	return w.petDamageLocked(ps, t, m, now)
}

// petTopHateLocked puts the pet at the top of t's hate, and makes it what t
// fights (a training dummy's is its own: dummy.go).
func (w *World) petTopHateLocked(pet, t *NPC) {
	if t.Dummy != NotDummy {
		return
	}
	var top int64
	for id, v := range t.Threats {
		if id != pet.ServerID && v > top {
			top = v
		}
	}
	if t.Threats == nil {
		t.Threats = make(map[EntityID]int64)
	}
	if t.Threats[pet.ServerID] <= top {
		t.Threats[pet.ServerID] = top + 1
	}
	t.CurrentEnemy = pet.ServerID
}

// PetTemplateSummary is the pets' templates for the server's log.
func (w *World) PetTemplateSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	by := map[string]int{}
	for _, pt := range w.templates.pets {
		if pt.tmpl != nil {
			by[pt.tmpl.Name]++
		}
	}
	return fmt.Sprintf("%d summonings fight by a pet template (%d templates), %d weapon versions", len(w.templates.pets), len(by),
		len(w.templates.weapon))
}

// petOnly is whether an ability is a pet's alone: a taunt, a buff, a heal
// on its owner or of its own share (an NPC of the template casts none).
func petOnly(a *templateAbility) bool {
	switch {
	case a.Kind == database.AbilityTaunt, a.Kind == database.AbilityBuff:
		return true
	case a.Kind == database.AbilityHeal && (a.HealPct > 0 || a.TargetOr() == database.AbilityOnOwner):
		return true
	}
	return false
}

// abilityElement is the resist an ability's element names (StatNone: any).
func abilityElement(e string) Stat {
	switch e {
	case "fire":
		return StatFR
	case "cold":
		return StatCR
	case "lightning":
		return StatLR
	case "poison":
		return StatPR
	case "disease":
		return StatDR
	case "arcane":
		return StatAR
	}
	return StatNone
}

// abilityStat is the stat an ability's buff raises (StatNone: any).
func abilityStat(st string) Stat {
	switch st {
	case "ac":
		return StatAC
	case "str":
		return StatStr
	case "sta":
		return StatSta
	case "agi":
		return StatAgi
	case "dex":
		return StatDex
	case "wis":
		return StatWis
	case "int":
		return StatInt
	case "cha":
		return StatCha
	case "hp":
		return StatMaxHP
	case "power":
		return StatMaxPower
	}
	return StatNone
}

// spellRaises is whether a spell raises the stat on its target.
func spellRaises(s *CastSpell, st Stat) bool {
	for _, e := range s.Def.Effects {
		if e.Kind == EffectStat && e.Stat == st && e.Amount > 0 && !e.Self {
			return true
		}
	}
	return false
}

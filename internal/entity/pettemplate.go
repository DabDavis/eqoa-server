package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
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
// An ability of the kind "aoe" (an area spell of the class's) is cast at what
// the pet fights and lands on its area (below); a template's timed special
// (special_every and its kin) is a pet's crit as it is a creature's
// (petTemplateSpecialLocked; special_chance, the kick by chance, an NPC's
// alone). A "dot" is a damage over time on what it fights, kept up, and a
// template's melee proc (proc_chance) is the pet's own melee hits' too, as
// it is a creature's (the GM Jibrish's petition 1388, the Swarming Deaths':
// petDoTLocked, petTemplateProcLocked); an ability's cast may be its own
// (cast: the half second a record's whole seconds cannot hold). A charmed
// creature fights by its npccharm row's template
// (charm.go), its class and swing its own, and its casts cost it their
// spells' power (petCastCost) unless the ability is free. A pet with no row
// (a shaman's, a wizard's, the Grand Animation, a charmed creature without
// one) fights as pet.go always had it.

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
	if pet.isMerc() && balanced(pet) && n > 0 {
		// a player's share of a creature's melee of its class and level
		// (npcHitPlayerLocked's balanceclass and balanceclasslevel)
		b := pet.merc.body
		return max(1, pct(n, w.balance.MeleeTakenPctAt(b.Class, b.Level)))
	}
	ps := w.petStateOfNPCLocked(pet)
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
	case database.AbilityNuke, database.AbilityAoE, database.AbilityLifetap, database.AbilityTaunt, database.AbilityRoot,
		database.AbilityDoT:
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
		if a.Chain || now.Before(f.next[i]) || ps.npc.Level < a.From || petShareHeal(a) || a.Kind == database.AbilityDebuff ||
			a.Kind == database.AbilityStrip {
			continue // a debuff and a strip are a creature's, at a player (a strip at a pet too)
		}
		if petAtFoe(a.Kind) && t == nil {
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

// tellPetHealLocked tells the owner of a heal the pet made on itself: a 0x00DB
// from the owner over the pet, as a tap's heal is told (pettap.go), its pet's
// heal (numbertags.go).
func (w *World) tellPetHealLocked(ps *petState, n int) {
	if sess := w.findSessionByEntityIDLocked(ps.owner.ServerID); sess != nil {
		if id := w.objectIDForEntityLocked(sess, ps.npc.ServerID); id != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), n, id, numberHeal.byPet())
		}
	}
}

// petWantsLocked is whether a due ability is wanted now: a heal on its
// owner or a spell's on itself below its share, a buff its target (itself,
// or its owner) lacks; anything at a foe, yes, but a spell for undead alone
// at a foe that is not undead (it waits, due, for one that is).
func (w *World) petWantsLocked(ps *petState, a *templateAbility, now time.Time) bool {
	if petAtFoe(a.Kind) && !w.petUndeadMayLocked(a.spellFor(petSpellClass(a, ps.npc), ps.npc.Level), w.NPCs[ps.target]) {
		return false
	}
	switch a.Kind {
	case database.AbilityHeal:
		owner := func() bool {
			o := ps.owner
			top := o.MaxHPNow()
			return top > 0 && !o.IsDead() && o.HP*100 < top*a.BelowOr()
		}
		switch a.TargetOr() {
		case database.AbilityOnOwner:
			return owner()
		case database.AbilityOnEither:
			return owner() || ps.npc.HP*100 < ps.npc.MaxHP*a.BelowOr()
		case database.AbilityOnGroup:
			return w.petGroupHurtLocked(ps, a) // anyone of its side in reach (petgroupheal.go)
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
	case database.AbilityRoot:
		t := w.NPCs[ps.target] // not again on what its root still holds
		return t != nil && !t.rooted(now)
	case database.AbilityDoT:
		// kept up: again once the one on its target is about to run out
		t := w.NPCs[ps.target]
		s := a.spellFor(petSpellClass(a, ps.npc), ps.npc.Level)
		return t != nil && s != nil && !w.petDoTUpLocked(ps, a, s, t, now)
	}
	return true
}

// petUndeadMayLocked is whether a pet's spell may go at t: any spell at
// anything, but a spell for undead alone (a "Target undead" line, undead.go) at
// an undead creature alone (its npcundead row, else its name: undeadLocked), as
// a player's is refused at the living (the magician Waldorf's petition 1395:
// the Ghilan Noble's "undead nuke against undead creatures"); so it is neither
// cast at nor lands on the living, and an area of it spares them. No spell or
// no target is the caller's to say. Must be called with the lock held.
func (w *World) petUndeadMayLocked(s *CastSpell, t *NPC) bool {
	return s == nil || t == nil || !s.Def.undeadOnly() || w.undeadLocked(t)
}

// buffOn is whether effects hold the buff's effect, or one of its line of a
// higher rank (which would keep it off: buffline.go, putOnPetLocked).
func buffOn(effects []*ActiveEffect, s *CastSpell) bool {
	line := BuffLine(s)
	for _, e := range effects {
		if e.SpellID == s.SpellID || sameLine(e.Line, line) && e.Rank >= lineRank(s) {
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
	cost := petCastCost(pet, a, spell)
	if cost > pet.Power {
		return false // a charmed creature out of power: it fights with its melee (charm.go)
	}
	ct := a.castTime(spell)
	c := &petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: spell, lands: now.Add(ct + castLand)}
	switch {
	case petAtFoe(a.Kind):
		if t == nil || t.Dead || !w.petUndeadMayLocked(spell, t) {
			return false // a spell for undead alone not cast at the living (a chained one too)
		}
		c.target = t.ServerID
	case a.TargetOr() == database.AbilityOnOwner:
		c.target = ps.owner.ServerID
	case a.TargetOr() == database.AbilityOnEither:
		// its owner or itself, the lower share of its hit points
		c.target = pet.ServerID
		if o := ps.owner; !o.IsDead() && o.MaxHPNow() > 0 && pet.MaxHP > 0 &&
			float64(o.HP)/float64(o.MaxHPNow()) < float64(pet.HP)/float64(pet.MaxHP) {
			c.target = o.ServerID
		}
	default:
		c.target = pet.ServerID
	}
	w.petCasts = append(w.petCasts, c)
	pet.Power -= cost
	f := pet.fight()
	f.busy = now.Add(ct + castBusy)
	if j := i + 1; j < len(abilities) && abilities[j].Chain {
		f.chain = j + 1
	}
	w.broadcastNPCSwingLocked(pet, c.target, uint32(ct/time.Millisecond), spell.visual())
	log.Printf("Pet: %s's %q (%s) begins %q (spell %d) on %s: %v cast, %d power (%d left)", ps.owner.Name, pet.Name, ps.tmpl.Template,
		spell.Name, spell.SpellID, w.entityNameLocked(c.target), ct, cost, pet.Power)
	return true
}

// petCastCost is the power a pet's cast of the spell costs it: a charmed
// creature's, its spell's power (charm.go: the wiki's tree "not casting as
// much as usual ... give it some power"), none for an ability marked free; a
// summoned pet's casts cost nothing.
func petCastCost(pet *NPC, a *templateAbility, spell *CastSpell) int {
	if pet.Charm == nil || a.Free || spell.PowerCost <= 0 {
		return 0
	}
	return spell.PowerCost
}

// petTemplateSpecialLocked is a templated pet's special in its fight with t,
// at each of its turns in reach (petFightLocked): a charm template's crit (the
// wiki's rhinos' "3k+ critical dmg attack", the griffons' "700+ crit"), on its
// template's clock of time at its foe as a creature's timed special is
// (templateSpecialLocked: special_every, its first special_first into the
// fight, each special_jitter either way, from special_from of its own level),
// its level times special_damage as one of its hits (the balance's pet damage
// and con), cast 0 with its visual. It reports a kill. Must be called with
// the lock held.
func (w *World) petTemplateSpecialLocked(ps *petState, t *NPC, now time.Time) bool {
	if ps.tmpl == nil {
		return false
	}
	tm, pet := ps.tmpl.tmpl, ps.npc
	if tm.SpecialDamage <= 0 || tm.SpecialEvery <= 0 || pet.Level < tm.SpecialFrom {
		return false
	}
	f := pet.fight()
	switch gap := now.Sub(f.timedSeen); {
	case f.timedSeen.IsZero():
		f.timedLeft = templateJitter(tm.SpecialFirst, tm.SpecialJitter)
	case gap > 0 && gap <= templateContactGap:
		f.timedLeft -= gap // at its foe since its last turn
	}
	f.timedSeen = now
	if f.timedLeft > 0 || now.Before(f.busy) {
		return false
	}
	f.timedLeft = templateJitter(tm.SpecialEvery, tm.SpecialJitter)
	n := max(int(math.Round(float64(pet.Level)*tm.SpecialDamage)), 1)
	fx := tm.SpecialFX
	if fx == 0 {
		fx = templateSpecialFX
	}
	w.broadcastNPCSwingLocked(pet, t.ServerID, 0, fx)
	pet.LastCombatTime = now
	log.Printf("Pet: %s's %q (%s) lands its %s on %s for %d before the balance", ps.owner.Name, pet.Name, tm.Name, specialName(tm), t.Name, n)
	// a charmed creature's lands as a creature's does, fixed (classtemplate.go)
	return w.petLandHitLocked(ps, t, n, now, pet.Charm == nil, numberSpell)
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
		if ps.npc.Dead || !w.petLiveLocked(ps) || ps.tmpl != c.tmpl {
			continue
		}
		w.petLandLocked(c, now)
	}
}

// A pet's area spell lands on its area too (the user, 2026-11-01: "fix them
// all", of the workbook's Air pets' area nukes, which their import cast on
// the target alone): a nuke whose spell is round its target (the workbook's
// target-based area: Air AoE, Lightning Maelstrom's kind) hits the target with
// its whole figure (the spell's hp and hp2, its target a second time: the
// workbook's legend) and every creature the pet may fight within the area's
// radius of it, nearest first and as many as the area's cap, with the area's
// own figure (hp2); a root (AbilityRoot) holds its target and the same others.
// A caster's area (a "PB AE": the charm templates' area spells, 2026-11-02) is
// round the pet itself, its target taking the spell's whole figure as well.

// petAreaFoesLocked is the creatures round t a pet's area spell lands on
// besides t: those the pet may fight, not hidden, within the area's radius of
// t that it reaches (sight.go), nearest first, the area's cap with t counted.
// Must be called with the lock held.
func (w *World) petAreaFoesLocked(pet, t *NPC, area SpellArea, now time.Time) []*NPC {
	if area.Centre == AreaNone || w.SpatialIndex == nil {
		return nil
	}
	r := area.radiusFor(true)
	cx, cy, cz := t.X, t.Y, t.Z
	onCaster := area.Centre == AreaOnCaster || area.Centre == AreaCone
	if onCaster {
		cx, cy, cz = pet.X, pet.Y, pet.Z // a caster's area: round the pet (a charmed crawler's); a cone's apex
	}
	cone := area.coneOf(pet.X, pet.Y, pet.Z, pet.Facing, true, t.X, t.Z) // area.go: in front of the pet, at t
	place := areaPlace{world: pet.World, caster: [3]float32{pet.X, pet.Y, pet.Z}, centre: [3]float32{cx, cy, cz}, onOther: !onCaster}
	type near struct {
		npc *NPC
		d   float32
	}
	var out []near
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, queryRadius(cone, r)) {
		o := w.NPCs[id]
		if o == nil || o == t || !petMayFight(pet, o) || o.hiddenAt(now) || w.metalSparesLocked(o, pet.Owner) {
			continue
		}
		if in, d := inArea(cone, cx, cy, cz, r, o.X, o.Y, o.Z, npcReachExtra(o)); in && w.reachesLocked(place, o.X, o.Y, o.Z) {
			out = append(out, near{o, d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].d < out[j].d })
	if most := area.most() - 1; len(out) > most {
		out = out[:max(most, 0)]
	}
	foes := make([]*NPC, len(out))
	for i, n := range out {
		foes[i] = n.npc
	}
	return foes
}

// petSplashLocked lands a pet's area nuke's figure on the others of its area
// (petAreaFoesLocked), each as one of the pet's hits; a spell for undead alone
// on its undead alone.
func (w *World) petSplashLocked(ps *petState, t *NPC, c *petCast, a *templateAbility, now time.Time) {
	area := c.spell.Area
	if area.Centre == AreaNone {
		return
	}
	effects := area.Effects
	if effects == nil {
		effects = c.spell.Def.Effects
	}
	pet := ps.npc
	for _, o := range w.petAreaFoesLocked(pet, t, area, now) {
		if !w.petUndeadMayLocked(c.spell, o) {
			continue
		}
		n, _ := templateFigure(effects, a.DamageOr())
		if pet.sheet != nil {
			n, _ = sheetFigure(pet, effects, a.DamageOr(), false)
		}
		if n <= 0 {
			continue
		}
		log.Printf("Pet: %s's %q splashes %q on %s for %d before the balance", ps.owner.Name, pet.Name, c.spell.Name, o.Name, n)
		w.petDamageLocked(ps, o, -creatureResists(c.spell.resist, o, -n, now), now, numberSpell) // an area's: a spell's
	}
}

// petRootLocked lands a pet's root (the workbook's Djinni Vizier's Air Root)
// on its target and, for an area spell, the others round it: each held for
// the spell's time unless it resists, as a player's root on it is resisted by
// the con (balancecon's root_resist_pct at the pet's balance level, the
// arcane resist debuffs on it taken off: root.go), and freed as any root is
// by a hit (rootBreakLocked: the pet's own melee too).
func (w *World) petRootLocked(ps *petState, t *NPC, spell *CastSpell, now time.Time) {
	pet, d := ps.npc, &spell.Def
	if d.Duration <= 0 {
		return
	}
	level := w.balanceLevelOf(pet)
	for _, o := range append([]*NPC{t}, w.petAreaFoesLocked(pet, t, spell.Area, now)...) {
		if balanced(o) {
			chance := w.balance.Con(o.Level-level).RootResistPct - float64(npcResistOffPct(o, now))
			if chance > 0 && float64(rootIntn(10000)) < chance*100 {
				log.Printf("Pet: %s resists %s's %q's %q (%.0f%%)", o.Name, ps.owner.Name, pet.Name, spell.Name, chance)
				continue
			}
		}
		keep := o.Effects[:0]
		for _, e := range o.Effects {
			if e.SpellID != spell.SpellID {
				keep = append(keep, e)
			}
		}
		o.Effects = append(keep, &ActiveEffect{SpellID: spell.SpellID, Name: spell.Name, Icon: spell.Icon,
			Caster: ps.owner.ServerID, Until: now.Add(d.Duration), Root: true, Line: BuffLine(spell), Rank: lineRank(spell)})
		if w.affected == nil {
			w.affected = make(map[EntityID]*NPC)
		}
		w.affected[o.ServerID] = o
		o.stopMoving()
		log.Printf("Pet: %s's %q roots %s with %q for %v", ps.owner.Name, pet.Name, o.Name, spell.Name, d.Duration)
	}
}

// petDrainStatsLocked lands what a pet's drain takes of its target's stats
// (the workbook's Undead Bane, its necromancer's pets from 30: STR and AC
// taken and given to the pet for a minute; the user, 2026-11-01:
// "necromancers mummy and higher pets have a drain"): each drained stat
// lowered on the creature as a player's debuff is (an effect of the spell's,
// which counts in its fights: pet.go's creatureMods) and raised on the pet
// for the spell's time (an effect of its own, which counts in the pet's:
// effectMods), a share of the pet's own stat added as the workbook's scale
// gives it (petStat). Never resisted, as a pet's nuke is not.
func (w *World) petDrainStatsLocked(ps *petState, t *NPC, spell *CastSpell, now time.Time) {
	pet, d := ps.npc, &spell.Def
	if d.Duration <= 0 {
		return
	}
	var loss, gain StatMods
	for _, e := range d.Effects {
		if e.Kind != EffectStat || e.Amount >= 0 || e.Drain == DrainNone || e.Self {
			continue
		}
		n := -e.Amount
		if e.Scale != StatNone && e.ScalePct != 0 {
			n += pet.petStat(e.Scale) * e.ScalePct / 100
		}
		loss.add(e.Stat, -n)
		gain.add(e.Stat, n)
	}
	if loss == (StatMods{}) {
		return
	}
	until := now.Add(d.Duration)
	// on the creature, as a player's harm lands (cast.go's landOnNPCLocked)
	cut := t.Effects[:0]
	for _, e := range t.Effects {
		if e.SpellID != spell.SpellID {
			cut = append(cut, e)
		}
	}
	t.Effects = append(cut, &ActiveEffect{SpellID: spell.SpellID, Name: spell.Name, Icon: spell.Icon, Caster: ps.owner.ServerID,
		Until: until, Mods: loss, Line: BuffLine(spell), Rank: lineRank(spell)})
	t.syncEffectMaxHP()
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[t.ServerID] = t
	// on the pet, its own share: its earlier one taken off
	own := -spell.SpellID
	keep := pet.Effects[:0]
	for _, e := range pet.Effects {
		if e.SpellID != own {
			keep = append(keep, e)
		}
	}
	pet.Effects = append(keep, &ActiveEffect{SpellID: own, Name: spell.Name, Icon: spell.Icon, Caster: ps.owner.ServerID,
		Until: until, Mods: gain})
	pet.syncEffectMaxHP()
	log.Printf("Pet: %s's %q drains %s with %q: %+v", ps.owner.Name, pet.Name, t.Name, spell.Name, gain)
}

// A pet's damage over time (AbilityDoT; the GM Jibrish's petition 1388, of the
// Swarming Deaths: "a 300 a tick dot (.5 second cast) it keeps up that lasts for
// 24 seconds") is its spell's damage tick on what it fights for the spell's
// time, ticking at its landing and every effectTick after it (a 24 s spell four
// times), each tick one of the pet's hits. The pet casts it again once the one
// on its target has no more than the cast's time and its landing left, so the
// new one lands as the old one ends: never two of its owner's on one creature
// (the old one is replaced), never down while the pet fights it. Another
// owner's pet keeps its own on the same creature.

// petDoTUpLocked is whether the pet's damage over time s is up on t: on its way
// there, or on it from this pet with more than the cast's time and its landing
// to run (petWantsLocked). Must be called with the lock held.
func (w *World) petDoTUpLocked(ps *petState, a *templateAbility, s *CastSpell, t *NPC, now time.Time) bool {
	for _, c := range w.petCasts {
		if c.ps == ps && c.target == t.ServerID && c.spell.SpellID == s.SpellID {
			return true
		}
	}
	lead := a.castTime(s) + castLand
	for _, e := range t.Effects {
		if e.SpellID == s.SpellID && e.pet == ps.npc.ServerID && e.Until.Sub(now) > lead {
			return true
		}
	}
	return false
}

// petDoTLocked lands a pet's damage over time on t: its tick (petTickFigure) on
// it for the spell's time, its owner's pets' earlier one of the spell taken off,
// and its first tick at once, as a player's on a creature ticks
// (landOnNPCLocked). Must be called with the lock held.
func (w *World) petDoTLocked(ps *petState, t *NPC, spell *CastSpell, a *templateAbility, now time.Time) {
	pet, d := ps.npc, &spell.Def
	tick := petTickFigure(pet, d.Effects, a.DamageOr())
	if tick <= 0 {
		return
	}
	dur := d.Duration
	if dur <= 0 {
		dur = tickDuration // a tick's line with no time (cast.go)
	}
	owner := ps.owner.ServerID
	keep := t.Effects[:0]
	for _, e := range t.Effects {
		if e.pet == 0 || e.SpellID != spell.SpellID || e.Caster != owner {
			keep = append(keep, e)
		}
	}
	for i := len(keep); i < len(t.Effects); i++ {
		t.Effects[i] = nil
	}
	resist := spell.resist
	if spell.unresistable {
		resist = StatNone
	}
	ae := &ActiveEffect{SpellID: spell.SpellID, Name: spell.Name, Icon: spell.Icon, Caster: owner, Until: now.Add(dur),
		NextTick: now, HPTick: -tick, pet: pet.ServerID, Rules: EffectRules{Resist: resist, Harm: true}}
	t.Effects = append(keep, ae)
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[t.ServerID] = t
	log.Printf("Pet: %s's %q puts %q on %s: %d a tick for %v", ps.owner.Name, pet.Name, spell.Name, t.Name, tick, dur)
	w.tickNPCEffectLocked(t, ae, now)
}

// petTickFigure is a pet's damage over time's tick: each damage tick of its
// effects rolled, a workbook pet's stat share added as its spells' figures are
// (sheetFigure), within its cap, at the ability's percent.
func petTickFigure(pet *NPC, effects []SpellEffect, pct int) int {
	n := 0
	for _, e := range effects {
		if e.Kind != EffectHPTick || e.Amount >= 0 || e.Self {
			continue
		}
		v := -rollAmount(e)
		if pet.sheet != nil && e.Scale != StatNone && e.ScalePct != 0 {
			v += int(math.Round(float64(pet.petStat(e.Scale)) * float64(e.ScalePct) / 100))
		}
		if e.Cap > 0 {
			v = min(v, e.Cap)
		}
		n += int(math.Round(float64(v) * float64(pct) / 100))
	}
	return n
}

// petTickLocked is a tick of a pet's damage over time on the creature
// (tickNPCEffectLocked): one of the pet's hits of the tick, less the share the
// creature's template resists of its element, as the pet's spells land
// (petLandLocked), through the balance (petDamageLocked). A tick whose pet is
// gone (dead, dismissed, its owner's pet another since) does nothing: what is
// left of it runs out. Must be called with the lock held.
func (w *World) petTickLocked(npc *NPC, ae *ActiveEffect, now time.Time) {
	ae.NextTick = ae.NextTick.Add(effectTick)
	ps := w.pets[ae.Caster]
	if ps == nil || ps.npc.ServerID != ae.pet {
		ps = w.petStateOfNPCLocked(w.NPCs[ae.pet]) // a merc's pet's (mercpet.go)
	}
	if ps == nil || ps.npc.ServerID != ae.pet || !w.petLiveLocked(ps) || ps.npc.Dead || npc.Dead || ae.HPTick >= 0 {
		return
	}
	n := -creatureResists(ae.Rules.Resist, npc, ae.HPTick, now)
	ae.ticking = true // its own tick is no contact for itself (contactNPCLocked)
	defer func() { ae.ticking = false }()
	ps.npc.LastCombatTime = now
	log.Printf("Pet: %s's %q's %q ticks on %s for %d before the balance", ps.owner.Name, ps.npc.Name, ae.Name, npc.Name, n)
	w.petDamageLocked(ps, npc, n, now, numberDoT)
}

// petTemplateProcLocked rolls the pet's template's melee proc after one of its
// melee hits landed on t, still standing (petition 1388, Jibrish: the Swarming
// Deaths' "melee proc for 250 damage, with a 25% proc rate, base"): proc_chance
// percent of its hits that land also deal proc_damage, neither scaled by
// anything of the pet's, less the share the creature's template resists of
// proc_resist's element (creatureResists, as its spells), one of its hits
// through the balance (petDamageLocked), seen as a player's proc is
// (fxShieldHit). It reports a kill. A template's dice are templateIntn.
func (w *World) petTemplateProcLocked(ps *petState, t *NPC, now time.Time) bool {
	if ps.tmpl == nil || t.Dead {
		return false
	}
	tm := ps.tmpl.tmpl
	if tm.ProcChance <= 0 || tm.ProcDamage <= 0 || templateIntn(100) >= tm.ProcChance {
		return false
	}
	n := -creatureResists(shieldResistStat(tm.ProcResist), t, -tm.ProcDamage, now)
	w.broadcastNPCSwingLocked(ps.npc, t.ServerID, 0, fxShieldHit)
	log.Printf("Pet: %s's %q's %s hits %s for %d before the balance", ps.owner.Name, ps.npc.Name, procName(tm), t.Name, n)
	return w.petDamageLocked(ps, t, n, now, numberProc)
}

// petLandLocked lands a pet's cast: a harmful spell's figure (its hits
// rolled, no stat: a pet has none, at the ability's percent) on the
// creature as one of the pet's hits (petDamageLocked), a tap's drained
// share back on the pet; a heal's figure on its owner or itself; a buff on
// itself (petBuffItselfLocked), or its lasting effect on its owner, as the
// owner's own cast on itself.
func (w *World) petLandLocked(c *petCast, now time.Time) {
	ps, pet, a := c.ps, c.ps.npc, c.a
	if petAtFoe(a.Kind) && !w.petUndeadMayLocked(c.spell, w.NPCs[c.target]) {
		return // for undead alone, and its target no longer is (a GM's tag in its cast)
	}
	switch a.Kind {
	case database.AbilityNuke, database.AbilityAoE, database.AbilityLifetap:
		t := w.NPCs[c.target]
		if t == nil || t.Dead || t.World != pet.World {
			return
		}
		hit, drained := templateFigure(c.spell.Def.Effects, a.DamageOr())
		if pet.sheet != nil {
			hit, drained = sheetFigure(pet, c.spell.Def.Effects, a.DamageOr(), false) // its stats scale it (petstats.go)
		}
		if a.Kind == database.AbilityLifetap {
			w.petDrainStatsLocked(ps, t, c.spell, now) // a drain of stats: Undead Bane's
		}
		if hit <= 0 {
			return
		}
		pet.LastCombatTime = now
		log.Printf("Pet: %s's %q lands %q on %s for %d before the balance", ps.owner.Name, pet.Name, c.spell.Name, t.Name, hit)
		taken := -creatureResists(c.spell.resist, t, -hit, now)
		if drained > 0 {
			// a tap heals the pet by what it took (a player's tap's rule, lifetap.go):
			// its drained share of the hit after the creature's resist and the
			// balance, whole though the hit kills (petition 1388: until then the
			// figure before either)
			drained = int(math.Round(float64(w.petDealtLocked(pet, t, taken)) * float64(drained) / float64(hit)))
		}
		w.petSplashLocked(ps, t, c, a, now) // an area spell's others (before the target's hit, which may kill it)
		w.petDamageLocked(ps, t, taken, now, numberSpell)
		if drained > 0 && pet.HP < pet.MaxHP {
			n := min(drained, pet.MaxHP-pet.HP)
			pet.HP += n
			w.tellPetHealLocked(ps, n)
		}
	case database.AbilityDoT:
		t := w.NPCs[c.target]
		if t == nil || t.Dead || t.World != pet.World {
			return
		}
		pet.LastCombatTime = now
		w.petDoTLocked(ps, t, c.spell, a, now)
	case database.AbilityRoot:
		t := w.NPCs[c.target]
		if t == nil || t.Dead || t.World != pet.World {
			return
		}
		pet.LastCombatTime = now
		w.petRootLocked(ps, t, c.spell, now)
	case database.AbilityHeal:
		n := petHealFigure(c.spell, a)
		if pet.sheet != nil {
			n, _ = sheetFigure(pet, c.spell.Def.Effects, a.DamageOr(), true) // its stats scale it (petstats.go)
		}
		if a.TargetOr() == database.AbilityOnGroup {
			w.petGroupHealLocked(ps, c.spell, n) // everyone of its side in reach (petgroupheal.go)
			return
		}
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
					w.sendDamageIndication(sess, id, n, sess.PlayerObjectID(), numberHeal.byPet()) // its pet's heal on it (numbertags.go)
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
		cs.spell.figures = figuresPet // what it heals now and at its ticks the pet's (numbertags.go)
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
// level: a roll between the line's least and most there, at least 1 (none
// where the most is 0: a taunt of hate alone); ok
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
	if hi <= 0 {
		return 0, true // a taunt of hate alone (a workbook pet's "Modify Hate")
	}
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
	w.showPetTauntLocked(pet, t.ServerID) // pet.go
	pet.LastCombatTime = now
	if n, ok := petTauntFigure(a.Hits, pet.Level); ok {
		if n == 0 {
			log.Printf("Pet: %s's %q taunts %s", ps.owner.Name, pet.Name, t.Name)
			return false
		}
		log.Printf("Pet: %s's %q taunts %s for %d", ps.owner.Name, pet.Name, t.Name, n)
		return w.petLandHitLocked(ps, t, n, now, false, numberSpell)
	}
	n := float64(petMeleeHit(pet, t, meleeIntn))
	if ps.hitScale > 0 {
		n *= ps.hitScale
	}
	m := ps.meleeOut(int(math.Round(n * float64(a.DamageOr()) / 100)))
	log.Printf("Pet: %s's %q taunts %s (a melee hit of %d)", ps.owner.Name, pet.Name, t.Name, m)
	return w.petDamageLocked(ps, t, m, now, numberSpell) // a taunt's: an ability's
}

// petTopHateLocked puts the pet at the top of t's hate, and makes it what t
// fights (a training dummy's hate is its own: dummy.go).
func (w *World) petTopHateLocked(pet, t *NPC) {
	if t.Dummy != NotDummy {
		w.dummyTopHateLocked(t, pet.ServerID, time.Now()) // a dummy's hate is its own (dummy.go, petition 1484)
		return
	}
	if t.challenge != nil { // a Metal Moogie keeps to his challenger (metalmoogie.go)
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
	t.CurrentEnemy = w.tauntedLocked(t, pet.ServerID) // a preferred foe of its stays its enemy (prefer.go)
	w.wakeNPCLocked(t)                                // npcsets.go
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

// petOnly is whether an ability is a pet's alone: a taunt, a buff, a root, a heal
// on its owner (an NPC of the template casts none). A heal of its own share is a
// creature's too since petition 1394 (abilitytimes.go: Mr. Moogie's Lay Hands),
// cast as its other abilities are; a pet's comes by size (petSharePickLocked); and
// a damage over time since 2026-11-10 (creaturedot.go: the owner's Innoruuk).
func petOnly(a *templateAbility) bool {
	switch {
	case a.Kind == database.AbilityTaunt, a.Kind == database.AbilityBuff, a.Kind == database.AbilityRoot:
		return true
	case a.Kind == database.AbilityHeal && (a.TargetOr() == database.AbilityOnOwner || a.TargetOr() == database.AbilityOnGroup):
		return true // a creature's heal of its share is its own now (abilitytimes.go: petition 1394)
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

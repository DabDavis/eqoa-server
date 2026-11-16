package entity

import (
	"log"
	"time"
)

// mercselfpotion.go — an alchemist merc's Master Class mixtures (petition
// 2216, Felthious: "alc merc missing master class abilitys"). The alchemist's
// three Master Class abilities that are not stats are mixtures: a spell that
// summons a potion, which a merc never casts (mercSkips: a summoning of
// items) and whose potion until then none of its turns used, so a Transmuter
// or a Metabolist merc held its Master Class's stats and nothing else:
//
//   - the Transmuter's Creative Transmutation (Transformative Pill: Self,
//     +1250 HP and maximum, +150 STR/STA/AGI/DEX, 30 min, an ogre's look)
//     and Transmutative Mixture (Transmutive Potion: thrown, a debuff of
//     every stat and 300 AC, 24 s);
//   - the Metabolist's Miraculous Mixture (Miraculous Draught: Self, 650 HP
//     a tick, 24 s).
//
// Each is now used as its path's own potions are (mercthrow.go,
// mercbrew.go): conjured as its book's summoning conjures it, never used up,
// whatever its ten slots hold (a mixture is never listed in its window, as a
// refinement is not: petition 2195), not if its hirer turned the mixture off.
// Only its Master Class's own (a spell its ranks add, MercKit.MasterRanks):
// classspells gives an alchemist all three mixtures by level 60 (their source
// is plain eqoa-data, not "master <name>"), so every alchemist's book holds
// them, and one with another Master Class or none uses none of them; nor does
// any alchemist take its book's other self potions this way (its tonics and
// vaccine, each line's ranks besides: as before).
//
//   - A buff potion (a self line that lasts and gives stats, hit points or
//     their most, not a power draught) it takes whenever it is not on it, in
//     a fight or out of one, its look left as it was hired (the pill's
//     illusion is not put on a merc: mercSkips' rule for its spells).
//   - A heal potion (a self line of health over time alone) it drinks below
//     mercHealDraughtBelow of its most health, when none is on it.
//   - A debuff potion (thrown, harmful, a lasting cut of its target's stats
//     and nothing a merc throws for: mercThrowPickLocked) it throws first at
//     a creature already fighting its team that has none of it on it, so it
//     pulls nothing; its other throws follow while it lasts.
//
// A potion of these kinds in its pack or its owner's stock is left to its
// owner: its throws and draughts as before.

// mercHealDraughtBelow: an alchemist merc drinks its heal potion below this
// share of its health (ours).
const mercHealDraughtBelow = 60

// mercSelfPotionAgain: the least time between two of the same self potion,
// so one its effects refuse (a stronger line on it) is not taken each turn
// (ours).
const mercSelfPotionAgain = 30 * time.Second

// mercSelfPotion is a self potion of its book the merc takes: its use and
// whether it is a heal (else a buff).
type mercSelfPotion struct {
	pe   PotionEffect
	heal bool
}

// mercLooksOff is the potion as it lands on a merc: its illusion (a body, a
// size, a look) left out, as a merc's spells' are.
func mercLooksOff(pe PotionEffect) PotionEffect {
	pe.Def.Form, pe.Def.FormKind, pe.Def.FormModel, pe.Def.FormSize = "", FormNone, 0, 0
	var keep []SpellEffect
	for _, e := range pe.Def.Effects {
		switch e.Kind {
		case EffectIllusion, EffectEndIllusion, EffectSize, EffectDisguise:
			continue
		}
		keep = append(keep, e)
	}
	pe.Def.Effects = keep
	return pe
}

// mercSelfPotionOf is the potion as a merc's self potion (the file's doc),
// false for none: a thrown one, a harmful one, a power draught, or a line it
// has no use for.
func (ms *mercState) mercSelfPotionOf(pe *PotionEffect) (mercSelfPotion, bool) {
	if pe == nil || pe.Thrown() || pe.Def.Harmful {
		return mercSelfPotion{}, false
	}
	if _, _, draught := ms.draughtOf(pe); draught {
		return mercSelfPotion{}, false // a power draught: mercDrinkTurnLocked's
	}
	look := mercLooksOff(*pe)
	cs := look.castSpell()
	if mercUseOf(&cs) != useBuff {
		return mercSelfPotion{}, false
	}
	hot, other := false, false
	for _, e := range look.Def.Effects {
		switch {
		case e.Kind == EffectHPTick && e.Amount > 0:
			hot = true
		case e.Kind == EffectHP && e.Amount > 0:
		default:
			other = true
		}
	}
	return mercSelfPotion{pe: look, heal: hot && !other}, true
}

// mercDebuffPotion is whether the potion is a thrown debuff (the file's doc):
// harmful, thrown, its line a lasting cut of stats and no hit or harm over
// time (those are its throws').
func mercDebuffPotion(pe *PotionEffect) bool {
	if pe == nil || !pe.Thrown() || !pe.Def.Harmful || !pe.Def.lasts() {
		return false
	}
	cut := false
	for _, e := range pe.Def.Effects {
		switch {
		case e.Self:
		case e.Kind == EffectHP && e.Amount < 0, e.Kind == EffectHPTick && e.Amount < 0:
			return false
		case e.Kind == EffectStat && e.Amount < 0:
			cut = true
		}
	}
	return cut
}

// mercMasterPotionsLocked is the potions its Master Class's mixtures make
// (spells of its book its ranks add), each once, a mixture its hirer turned
// off left out. Must be called with the lock held.
func (w *World) mercMasterPotionsLocked(ms *mercState) []*PotionEffect {
	return w.mercRankPotionsLocked(ms, ms.kit.MasterRanks)
}

// mercRankPotionsLocked is mercMasterPotionsLocked for the ranks given. Must
// be called with the lock held.
func (w *World) mercRankPotionsLocked(ms *mercState, ranks []*CMRank) []*PotionEffect {
	master := map[int]bool{}
	for _, r := range ranks {
		for _, a := range r.Adds {
			if a.SpellID > 0 {
				master[a.SpellID] = true
			}
		}
	}
	if len(master) == 0 {
		return nil
	}
	byName := map[string]*PotionEffect{}
	for _, pe := range w.potions.byPattern {
		byName[lowerTrim(pe.ItemName)] = pe
	}
	seen := map[int]bool{}
	var out []*PotionEffect
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if !master[s.SpellID] || containsInt(ms.kit.SpellsOff, s.SpellID) || s.Def.Summons() {
			continue
		}
		for _, it := range append(append([]SpellItem(nil), s.Def.Items...), unreadSummons(&s.Def)...) {
			if pe := byName[lowerTrim(it.Name)]; pe != nil && !seen[pe.PatternID] {
				seen[pe.PatternID] = true
				out = append(out, pe)
			}
		}
	}
	return out
}

// mercSelfPotionsLocked is the merc's Master Class's self potions (the file's doc).
// Must be called with the lock held.
func (w *World) mercSelfPotionsLocked(ms *mercState) []mercSelfPotion {
	if !ms.mercThrowsPotions() {
		return nil
	}
	var out []mercSelfPotion
	for _, pe := range w.mercMasterPotionsLocked(ms) {
		if sp, ok := ms.mercSelfPotionOf(pe); ok {
			out = append(out, sp)
		}
	}
	return out
}

// mercBookDebuffsLocked is the merc's Master Class's thrown debuffs (the file's
// doc). Must be called with the lock held.
func (w *World) mercBookDebuffsLocked(ms *mercState) []mercThrowable {
	if !ms.mercThrowsPotions() {
		return nil
	}
	var out []mercThrowable
	for _, pe := range w.mercMasterPotionsLocked(ms) {
		if mercDebuffPotion(pe) {
			out = append(out, mercThrowable{pe: *pe, debuff: true})
		}
	}
	return out
}

// npcHasEffect is whether an effect of the id is on the NPC.
func npcHasEffect(n *NPC, id int) bool {
	for _, e := range n.Effects {
		if e.SpellID == id {
			return true
		}
	}
	return false
}

// mercSelfPotionTurnLocked takes the merc's buff potion that is not on it, or
// drinks its heal potion when it is hurt (the file's doc); it reports one
// taken. Must be called with the lock held.
func (w *World) mercSelfPotionTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if m.Dead {
		return false
	}
	for _, sp := range w.mercSelfPotionsLocked(ms) {
		id := PotionEffectID(sp.pe.PatternID)
		if until, ok := ms.ready[id]; npcHasEffect(m, id) || ok && now.Before(until) {
			continue // on it, or taken a moment ago (a stronger line on it refused it)
		}
		if sp.heal && (m.MaxHP <= 0 || m.HP*100 >= m.MaxHP*mercHealDraughtBelow) {
			continue
		}
		spell := sp.pe.castSpell()
		cs := &castState{spell: spell, caster: m.ServerID, targets: []EntityID{m.ServerID}}
		if ms.ready == nil {
			ms.ready = map[int]time.Time{}
		}
		ms.ready[id] = now.Add(mercSelfPotionAgain)
		w.landOnPetLocked(cs, ms.body, nil, m, now)
		syncMercBody(ms)
		w.broadcastNPCCastLocked(m, m.ServerID, 0, fxConsumed)
		log.Printf("Merc: %s's %q takes %q (%d/%d HP)", ms.owner.Name, m.Name, sp.pe.ItemName, m.HP, m.MaxHP)
		return true
	}
	return false
}

// MercPotionUsesFor is the potions (pattern ids) of a merc of the kit's
// Master Class it takes as buffs, drinks as heals and throws as debuffs (the file's doc),
// its body made from the kit: the handlers' tests'.
func (w *World) MercPotionUsesFor(kit MercKit) (buffs, heals, debuffs []int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
	for _, sp := range w.mercSelfPotionsLocked(ms) {
		if sp.heal {
			heals = append(heals, sp.pe.PatternID)
		} else {
			buffs = append(buffs, sp.pe.PatternID)
		}
	}
	for _, c := range w.mercBookDebuffsLocked(ms) {
		debuffs = append(debuffs, c.pe.PatternID)
	}
	return buffs, heals, debuffs
}

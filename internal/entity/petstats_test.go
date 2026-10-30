package entity

import (
	"fmt"
	"math"
	"testing"
)

// A pet's stats from its effects count in its fights (the user, 2026-10-30:
// the earth pet's AC buff on itself): a creature's hit on it loses the
// creature's AC slope for each point of AC its effects add and a tank's AGI
// slope for each of AGI, as a player's (defence.go); its own hit gains a
// tank's STR share at the top and the floor, and its DEX raises the bottom by
// petDexShare of the roll's width. The control: with no effects both rolls
// are the formulas they were.
func TestAPetsOwnStatsCountInItsFights(t *testing.T) {
	pet := &NPC{ServerID: 1, Name: "Earth Elemental", Level: 34, Owner: 9}
	foe := &NPC{ServerID: 2, Name: "Desert Crab", Level: 50}
	r := npcMeleeRoll(foe)
	bare := npcHitOnPet(foe, pet, diceHigh)
	if want := max(int(math.Round(r.Top-r.ACSlope*340-agiSlope[archTank]*34)), r.Floor, 1); bare != want {
		t.Fatalf("a creature's top on a bare pet %d, want %d (the formula as it was)", bare, want)
	}
	pet.Effects = []*ActiveEffect{{SpellID: 700267, Mods: StatMods{AC: 310}}, {SpellID: 700061, Mods: StatMods{Agi: 10}}}
	buffed := npcHitOnPet(foe, pet, diceHigh)
	if want := max(int(math.Round(r.Top-r.ACSlope*650-agiSlope[archTank]*44)), r.Floor, 1); buffed != want || buffed >= bare {
		t.Errorf("a creature's top on the buffed pet %d, want %d, under the bare pet's %d", buffed, want, bare)
	}
	if low := npcHitOnPet(foe, pet, diceLow); low != max(buffed-r.Width, r.Floor, 1) {
		t.Errorf("its bottom %d, want the top less the width, %d", low, max(buffed-r.Width, r.Floor, 1))
	}

	// its own hits, on a target of no defence (a level 20's), the bottom over
	// the floor
	fire := &NPC{ServerID: 3, Name: "Fire Elemental", Level: 44, Owner: 9}
	target := &NPC{ServerID: 4, Name: "Desert Crab", Level: 20}
	cr := creatureRoll(fire.Level)
	top0 := int(math.Round(cr.Top - targetDefence(target)))
	lowBare, highBare := petMeleeHit(fire, target, diceLow), petMeleeHit(fire, target, diceHigh)
	if lowBare != max(top0-cr.Width, cr.Floor, 1) || highBare != max(top0, cr.Floor, 1) || lowBare <= cr.Floor {
		t.Fatalf("a bare pet's roll %d..%d, want %d..%d over its floor %d (the formula as it was)", lowBare, highBare,
			top0-cr.Width, top0, cr.Floor)
	}
	fire.Effects = []*ActiveEffect{{SpellID: 700124, Mods: StatMods{Str: 25, Sta: 25}}, {SpellID: 700141, Mods: StatMods{Dex: 25, Agi: 25}}}
	low, high := petMeleeHit(fire, target, diceLow), petMeleeHit(fire, target, diceHigh)
	wantHigh := int(math.Round(cr.Top - targetDefence(target) + offHighSTR*25))
	wantLow := top0 - cr.Width + int(math.Round(petDexShare*25*float64(cr.Width)))
	if high != wantHigh || low != wantLow || high <= highBare || low <= lowBare {
		t.Errorf("the buffed pet's roll %d..%d, want %d..%d (STR the top, DEX the bottom), over the bare %d..%d", low, high,
			wantLow, wantHigh, lowBare, highBare)
	}
	if math.Abs(petDexShare-0.001555) > 0.000005 {
		t.Errorf("petDexShare %.6f, want a tank's 0.001555 (offence.go's lines)", petDexShare)
	}
	// a creature's own effects (a player's harm on it) do not move its roll
	target.Effects = []*ActiveEffect{{SpellID: 1, Mods: StatMods{AC: -500}}}
	if got := petMeleeHit(fire, target, diceHigh); got != high {
		t.Errorf("the pet's top on a creature with -500 AC on it %d, want %d: a creature fights by its level", got, high)
	}
}

// Only the best of a buff line counts on a pet, as on a player (buffline.go),
// now that its stats count: a higher rank takes a lower one off, a lower one
// does not take under a higher (which is given back), a recast replaces
// itself, and another line stays.
func TestOnlyTheBestOfALineCountsOnAPet(t *testing.T) {
	spell := func(id, level int, name, line string) *CastSpell {
		return &CastSpell{SpellID: id, Level: level, Name: name, Scope: 1, Def: ParseSpellText(line)}
	}
	wood := spell(342, 1, "Woodskin", "Target, Increase AC by 12, 30min")
	steel := spell(700267, 24, "Steelskin", "Target, Increase AC by 310, 30min")
	cat := spell(700061, 8, "Feet Like Cat", "Target, Increase AGI by 10, 30min")
	effect := func(s *CastSpell) *ActiveEffect {
		ae := &ActiveEffect{SpellID: s.SpellID, Name: s.Name}
		for _, e := range s.Def.Effects {
			if e.Kind == EffectStat {
				ae.Mods.add(e.Stat, e.Amount)
			}
		}
		return ae
	}
	pet := &NPC{Name: "Earth Elemental", Owner: 9}
	put := func(s *CastSpell) *ActiveEffect { return putOnPetLocked(pet, s, effect(s)) }
	on := func() string {
		m := pet.effectMods()
		return fmt.Sprintf("%d effects, AC %+d, AGI %+d", len(pet.Effects), m.AC, m.Agi)
	}
	put(wood)
	put(cat)
	if over := put(steel); over != nil || on() != "2 effects, AC +310, AGI +10" {
		t.Fatalf("Steelskin over Woodskin: refused by %v, %s; want it in Woodskin's place: 2 effects, AC +310, AGI +10", over, on())
	}
	if over := put(wood); over == nil || over.SpellID != steel.SpellID || on() != "2 effects, AC +310, AGI +10" {
		t.Errorf("Woodskin under Steelskin: refused by %v, %s; want refused by Steelskin, nothing changed", over, on())
	}
	if over := put(steel); over != nil || on() != "2 effects, AC +310, AGI +10" {
		t.Errorf("Steelskin again: refused by %v, %s; want it in its own place", over, on())
	}
}

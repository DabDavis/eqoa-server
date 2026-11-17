package entity

import (
	"testing"
)

// sacrifice is the Doomseeker's Sacrifice (spellpattern 901849) as the
// server reads it: 20000 hit points to a friend, 20000 off its caster.
func sacrifice() CastSpell {
	return wbSpell(901849, 60, 1, "Sacrifice", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"stack":0,"level":60,
		"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"0974","type":"Dmg/Heal","kind":"hp","amount":20000},
		{"id":"0975","type":"Dmg/Heal","kind":"hp","amount":-20000,"scope":"self","target_type":"Self"}]}`)
}

// A Doomseeker merc (Heirloom petition 2277) never casts its Sacrifice on
// itself, however low it is; it casts it on a mate merc low. The control:
// the same heal with no price on its caster is cast on itself.
func TestMercDoomseekerSacrificesForAnother(t *testing.T) {
	s := sacrifice()
	if mercUseOf(&s) != useHeal || !selfPriced(&s) {
		t.Fatalf("Sacrifice reads as use %v, priced %v", mercUseOf(&s), selfPriced(&s))
	}
	plain := wbSpell(901850, 60, 1, "Plain Heal", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"level":60,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":20000}]}`)
	for _, c := range []struct {
		name      string
		spell     CastSpell
		selfHeals bool
	}{{"Sacrifice", s, false}, {"the control, a plain heal", plain, true}} {
		t.Run(c.name, func(t *testing.T) {
			r := newMercRig(t)
			k := r.kit(0, 4, 0, c.spell)
			k.Master = "Doomseeker"
			k2 := k
			k2.Name = "Ormon"
			a := r.hireSlot(1, k)
			b := r.hireSlot(2, k2)
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			// a alone hurt: the worst off of its team is itself
			a.npc.HP = a.npc.MaxHP / 5
			if got := r.w.mercHealTurnLocked(a, r.at(0)); got != c.selfHeals {
				t.Fatalf("hurt alone, a cast begun %v, want %v", got, c.selfHeals)
			}
			if c.selfHeals {
				return
			}
			// its mate lower still: Sacrifice at it
			b.npc.HP = b.npc.MaxHP / 10
			if !r.w.mercHealTurnLocked(a, r.at(0)) {
				t.Fatal("no Sacrifice for its mate")
			}
			if _, cast := a.ready[s.SpellID]; !cast {
				t.Fatal("what it cast was not Sacrifice")
			}
		})
	}
}

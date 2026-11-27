package entity

import "testing"

// harvest is the wizard's Harvest as the workbook holds its line: power now on
// the caster alone (scope self).
func harvest() CastSpell {
	return wbSpell(700098, 12, 0, "Harvest", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":12,
		"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"power","amount":60,"scale":"int","scale_pct":15}]}`)
}

// Heirloom petition 3431 ("WIZ mercenaries are missing the self power
// replenish lines? Harvest, etc."): a wizard merc's Harvest is its own power
// (useSelfPower), in its book; under mercSelfPowerBelowPct of its power it
// casts it at itself, and at full power it does not (the control).
func TestMercCastsHarvestWhenLow(t *testing.T) {
	h := harvest()
	if u := mercUseOf(&h); u != useSelfPower {
		t.Fatalf("Harvest reads as use %v, want its own power", u)
	}
	r := newMercRig(t)
	ms := r.hire(r.kit(13, 0, 0, h), MercAssist) // a wizard
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	inBook := false
	for _, sp := range ms.mercBook() {
		inBook = inBook || sp.s.SpellID == h.SpellID && sp.use == useSelfPower
	}
	if !inBook {
		t.Fatal("Harvest is not in the wizard merc's book as its own power")
	}
	m := ms.npc
	if m.MaxPower <= h.PowerCost*4 {
		t.Fatalf("the rig's merc power %d", m.MaxPower)
	}
	// the control: full power, nothing cast
	m.Power = m.MaxPower
	if r.w.mercSelfPowerTurnLocked(ms, r.at(0)) {
		t.Fatal("a merc at full power cast Harvest")
	}
	// low: Harvest at itself
	m.Power = m.MaxPower * (mercSelfPowerBelowPct - 10) / 100
	if !r.w.mercSelfPowerTurnLocked(ms, r.at(0)) {
		t.Fatal("a merc low on power did not cast Harvest")
	}
	c := r.w.mercCasts[len(r.w.mercCasts)-1]
	if c.spell.SpellID != h.SpellID || c.target != m.ServerID || c.group {
		t.Errorf("cast %q at %d (group %v), want Harvest at itself %d", c.spell.Name, c.target, c.group, m.ServerID)
	}
}

package entity

import "testing"

// Heirloom petition 2698 (2026-11-22): the shadow knight's cleave line, an
// area strike once Soul Cleave is bought (EQOA-Data's SK Exemplar guide: "Life
// tap applies only to the primary target; AoE portion deals damage only"), is
// a workbook target_aoe: the target takes the drain and both hits, the others
// round it the second hit (hp2) alone.
func TestACleaveAreaSharesItsSecondHitAlone(t *testing.T) {
	const deathcleave = `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"1HB,1HS,1HP, 2HB,2HS,2HP","sheet_type":"Drain","effects":[{"id":"07A2","type":"Drain","kind":"drain","amount":-256,"scale":"int","scale_pct":30,"resist":"ar"},{"id":"07A3","type":"Dmg/Heal","kind":"hp","amount":-770,"scale":"str","scale_pct":40,"resist":"blunt"},{"id":"07A4","type":"Dmg/Heal","kind":"hp2","amount":-128,"resist":"blunt"}],"radius":10}`
	d := SpellDefFrom("Deathcleave", "", "", deathcleave)
	a := ReadSpellArea("Deathcleave", "", d)
	if a.Centre != AreaOnTarget || a.Radius != 10 {
		t.Fatalf("area %+v, want round its target, radius 10", a)
	}
	if len(a.Effects) != 1 || a.Effects[0].Kind != EffectHP || a.Effects[0].Amount != -128 || a.Effects[0].Drain != 0 {
		t.Errorf("the others take %+v, want the second hit alone (-128, no drain)", a.Effects)
	}
	drains := 0
	for _, e := range d.Effects {
		if e.Drain != 0 {
			drains++
		}
	}
	if drains != 1 {
		t.Errorf("the target's effects %+v: want its drain among them", d.Effects)
	}

	// the control: as the record was (scope target), no area at all
	d = SpellDefFrom("Deathcleave", "", "", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"effects":[{"id":"07A3","type":"Dmg/Heal","kind":"hp","amount":-770},{"id":"07A4","type":"Dmg/Heal","kind":"hp2","amount":-128}]}`)
	if a := ReadSpellArea("Deathcleave", "", d); a.Centre != AreaNone {
		t.Errorf("scope target: area %+v, want none", a)
	}
}

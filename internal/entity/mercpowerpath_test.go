package entity

import "testing"

// Heirloom petition 2302 (Gonzo, 2026-11-17: "I picked power path and they
// have no power path spells"): the necromancer's Power path, a foe's power
// drained to the caster's group, is a merc's hit at its foe as the Blood
// path's hit points are, and worth its figure; a power drain to the caster
// alone is still none of a merc's.
func TestMercCastsThePowerPath(t *testing.T) {
	squall := wbSpell(700341, 30, 1, "Power Squall", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"type":"Drain Group","kind":"drain_group","stat":"power","amount":-450}]}`)
	blood := wbSpell(700323, 30, 1, "Blood Squall", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"type":"Drain Group","kind":"drain_group","stat":"hp","amount":-450}]}`)
	tap := wbSpell(9701, 30, 1, "Test Mana Tap", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Drain","effects":[{"type":"Drain","kind":"drain","stat":"power","amount":-450}]}`)
	if u := mercUseOf(&squall); u != useNuke {
		t.Errorf("Power Squall reads as use %v, want a hit", u)
	}
	if u := mercUseOf(&blood); u != useNuke {
		t.Errorf("Blood Squall reads as use %v, want a hit", u)
	}
	if u := mercUseOf(&tap); u != useNone {
		t.Errorf("a power tap to its caster reads as use %v, want none", u)
	}
	b := &Player{Level: 60}
	if p, h := mercWorth(&squall, useNuke, b), mercWorth(&blood, useNuke, b); p != h || p <= 0 {
		t.Errorf("Power Squall worth %v, Blood Squall %v: want the same, above 0", p, h)
	}
}

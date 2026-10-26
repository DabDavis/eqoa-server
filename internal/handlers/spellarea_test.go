package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A cast's spell is read with its area (entity/area.go), and a GM's spellarea
// row goes over it; with no row it is as its line reads.
func TestACastsAreaAndItsRow(t *testing.T) {
	e := &database.CharSpellEntry{Pattern: database.SpellPatternRecord{SpellID: 700145, SpellName: "Fire Storm", Scope: 1,
		CastTime: 2, Recast: 3, Power: 64}}
	spell := castSpell(e, "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36", 0)
	if spell.Area.Centre != entity.AreaOnTarget || len(spell.Area.Effects) != 1 || spell.Area.Effects[0].Amount != -36 {
		t.Fatalf("Fire Storm's area: %+v", spell.Area)
	}
	if got := withAreaRow(spell, database.SpellAreaRow{}, false); got.Area.Centre != entity.AreaOnTarget || got.Area.Radius != 0 {
		t.Errorf("no row: %+v", got.Area)
	}
	got := withAreaRow(spell, database.SpellAreaRow{Centre: entity.AreaRuleCaster, Radius: 12, MaxTargets: 3}, true)
	if got.Area.Centre != entity.AreaOnCaster || got.Area.Radius != 12 || got.Area.Max != 3 || len(got.Area.Effects) != 1 {
		t.Errorf("a row round its caster, 12, 3: %+v", got.Area)
	}
	if got := withAreaRow(spell, database.SpellAreaRow{Centre: entity.AreaRuleNone}, true); got.Area.Centre != entity.AreaNone {
		t.Errorf("a row saying not an area spell: %+v", got.Area)
	}
	bolt := castSpell(&database.CharSpellEntry{Pattern: database.SpellPatternRecord{SpellID: 650, SpellName: "Static Bolt", Scope: 1}},
		"Target, Decrease Hitpoints by 70", 0)
	if bolt.Area.Centre != entity.AreaNone {
		t.Errorf("a bolt read as an area: %+v", bolt.Area)
	}
	if got := withAreaRow(bolt, database.SpellAreaRow{Centre: entity.AreaRuleTarget}, true); got.Area.Centre != entity.AreaOnTarget || got.Area.Effects != nil {
		t.Errorf("a row making a bolt an area: %+v", got.Area)
	}
}

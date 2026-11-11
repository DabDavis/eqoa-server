package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A GM's spellarea row (entity/area.go) is written through the row editor,
// audited as any row is, and read at the next cast; the table is the catalog's
// "Spell areas", its centre an enumeration.
func TestSpellAreaRowsAreAGMs(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	code, out := c.op("insert_row", map[string]any{"table": "spellarea",
		"values": map[string]any{"spellid": 701065, "centre": 2, "radius": 8, "maxtargets": 4, "note": "round its caster"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "every cast") {
		t.Errorf("live: %v", out["live"])
	}
	row, ok, err := database.NewSpellbookStore(f.db).SpellArea(701065)
	if !ok || err != nil || row != (database.SpellAreaRow{Centre: 2, Radius: 8, MaxTargets: 4}) {
		t.Errorf("the cast's read: %+v %v %v", row, ok, err)
	}
	var audited int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'spellarea'`).Scan(&audited); err != nil || audited != 1 {
		t.Errorf("audited %d times (%v)", audited, err)
	}
	if n, ok := catalog["spellarea"]; !ok || n.Label != "Spell areas" || n.Cols["centre"].Enum != "areacentre" || len(enums()["areacentre"]) != 5 ||
		n.Cols["cone"].Label == "" {
		t.Errorf("the catalog's spellarea: %+v", n)
	}

	// a cone (Cleave's), its half-angle in column cone
	code, out = c.op("insert_row", map[string]any{"table": "spellarea",
		"values": map[string]any{"spellid": 700534, "centre": 3, "radius": 6, "maxtargets": 3, "cone": 60, "note": "Cleave"}})
	if code != 200 {
		t.Fatalf("insert the cone: %d %v", code, out)
	}
	if row, ok, err := database.NewSpellbookStore(f.db).SpellArea(700534); !ok || err != nil ||
		row != (database.SpellAreaRow{Centre: 3, Radius: 6, MaxTargets: 3, Cone: 60}) {
		t.Errorf("the cone's read: %+v %v %v", row, ok, err)
	}
}

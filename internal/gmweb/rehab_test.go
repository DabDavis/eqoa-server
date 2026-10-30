package gmweb

import (
	"strings"
	"testing"
)

// A character's resets in the panel (charresets): a GM gives Hero a free
// path respec back as a row, audited, which the revert takes away again; a
// kind the game does not count, a negative count and a row of no character
// are refused. Deleting the character takes its counts with it. The
// control: before the change Hero has no row.
func TestCharacterResetRowsAreAuditedAndRevertible(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	count := func() int {
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM charresets WHERE serverid = 7`).Scan(&n)
		return n
	}
	if count() != 0 {
		t.Fatal("the control: Hero has a row before any change")
	}
	code, out := c.op("describe_table", map[string]any{"table": "charresets"})
	if code != 200 || out["label"] != "Character resets" || out["group"] != "Players" ||
		!strings.Contains(out["help"].(string), "rehabilitator") {
		t.Fatalf("describe_table charresets: %d %v", code, out)
	}

	code, out = c.op("insert_row", map[string]any{"table": "charresets", "values": map[string]any{"serverid": 7, "kind": "path", "bonus": 1}})
	if code != 200 {
		t.Fatalf("give a respec: %d %v", code, out)
	}
	audit, _ := out["audit"].(float64)
	var bonus int
	f.db.QueryRow(`SELECT bonus FROM charresets WHERE serverid = 7 AND kind = 'path'`).Scan(&bonus)
	if count() != 1 || audit == 0 || bonus != 1 {
		t.Fatalf("after: %d rows, audit %v, bonus %d; want one, audited, 1", count(), audit, bonus)
	}
	for _, bad := range []map[string]any{
		{"serverid": 7, "kind": "masteries"},
		{"serverid": 7, "kind": "Path"},
		{"serverid": 7, "kind": "training", "used": -1},
		{"serverid": 7, "kind": "training", "bonus": 1.5},
		{"kind": "mastery"},
	} {
		if code, out := c.op("insert_row", map[string]any{"table": "charresets", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if count() != 1 {
		t.Fatalf("after the refusals: %d rows", count())
	}
	if code, out = c.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if count() != 0 {
		t.Errorf("the change reverted, %d rows left", count())
	}

	c.op("insert_row", map[string]any{"table": "charresets", "values": map[string]any{"serverid": 7, "kind": "mastery", "used": 1}})
	code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}, "dry_run": true})
	if also, _ := out["alsoDeleted"].(map[string]any); code != 200 || also["charresets"] != float64(1) {
		t.Fatalf("deleting Hero would take %v (%d), want its counts too", out["alsoDeleted"], code)
	}
	if code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}}); code != 200 {
		t.Fatalf("delete Hero: %d %v", code, out)
	}
	if count() != 0 {
		t.Errorf("Hero deleted, %d of its counts left", count())
	}
}

// The rehabilitators' prices (rehabsettings) come with every default, a GM
// changes one, audited and revertible; a setting the game does not read, one
// spelled otherwise and a value no purse can pay are refused.
func TestRehabSettingsAreAGMs(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	value := func(name string) int {
		var v int
		f.db.QueryRow(`SELECT value FROM rehabsettings WHERE setting = ?`, name).Scan(&v)
		return v
	}
	if value("price_rose") != 10_000_000 || value("price_mastery") != 1_000_000 || value("price_both") != 2_000_000 ||
		value("free_path") != 2 || value("free_mastery") != 1 || value("free_training") != 1 || value("price_training") != 1_000_000 {
		t.Fatal("the defaults are not the user's")
	}
	code, out := c.op("describe_table", map[string]any{"table": "rehabsettings"})
	if code != 200 || out["group"] != "World" || !strings.Contains(out["help"].(string), "Rose of Rehabilitation") {
		t.Fatalf("describe_table rehabsettings: %d %v", code, out)
	}
	code, out = c.op("update_row", map[string]any{"table": "rehabsettings", "key": map[string]any{"setting": "price_mastery"},
		"values": map[string]any{"value": 750000}})
	if code != 200 || value("price_mastery") != 750_000 {
		t.Fatalf("the change: %d %v, now %d", code, out, value("price_mastery"))
	}
	audit, _ := out["audit"].(float64)
	for _, bad := range []map[string]any{
		{"setting": "price_masteries", "value": 5},
		{"setting": "Price_Rose", "value": 5},
		{"setting": "price_sword", "value": 5},
	} {
		if code, out := c.op("insert_row", map[string]any{"table": "rehabsettings", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	for _, v := range []any{-1, 3_000_000_000, 1.25} {
		if code, out := c.op("update_row", map[string]any{"table": "rehabsettings", "key": map[string]any{"setting": "price_rose"},
			"values": map[string]any{"value": v}}); code != 400 {
			t.Errorf("value %v: %d %v, want refused", v, code, out)
		}
	}
	if value("price_rose") != 10_000_000 {
		t.Fatalf("refused, yet %d", value("price_rose"))
	}
	if code, out = c.op("revert_change", map[string]any{"id": audit}); code != 200 || value("price_mastery") != 1_000_000 {
		t.Errorf("revert: %d %v, now %d", code, out, value("price_mastery"))
	}
}

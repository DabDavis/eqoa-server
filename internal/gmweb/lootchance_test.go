package gmweb

import "testing"

// npcloot.chance takes a fraction in the panel (work board 1310 item 2, 1599;
// the owner of the spectre robe: "should be a 0.1 % drop"): an update of 0.1
// is kept as 0.1, read back so, and the camp loot layer shows it; a whole
// chance is as before; a word is refused.
func TestTheLootChanceTakesAFraction(t *testing.T) {
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('spectre', 'rare spectre robe', 3, 1, 'robes5117')`)
	c := f.browser()
	c.login("gm")
	key := map[string]any{"npc_name": "spectre", "tablename": "rare spectre robe"}
	code, out := c.op("update_row", map[string]any{"table": "npcloot", "key": key, "values": map[string]any{"chance": 0.1}})
	if code != 200 {
		t.Fatalf("0.1: %d %v", code, out)
	}
	var got float64
	var typ string
	if err := f.db.QueryRow(`SELECT chance, typeof(chance) FROM npcloot WHERE npc_name = 'spectre'`).Scan(&got, &typ); err != nil {
		t.Fatal(err)
	}
	if got != 0.1 || typ != "real" {
		t.Fatalf("stored %v (%s), want 0.1 real", got, typ)
	}
	code, out = c.op("get_row", map[string]any{"table": "npcloot", "key": key})
	if code != 200 {
		t.Fatalf("get_row: %d %v", code, out)
	}
	if row, _ := out["row"].(map[string]any); row == nil || row["chance"] != 0.1 {
		t.Fatalf("get_row shows %v, want chance 0.1", out)
	}
	if code, out := c.op("update_row", map[string]any{"table": "npcloot", "key": key, "values": map[string]any{"chance": 5}}); code != 200 {
		t.Fatalf("5: %d %v", code, out)
	}
	if code, out := c.op("update_row", map[string]any{"table": "npcloot", "key": key, "values": map[string]any{"chance": "often"}}); code != 400 {
		t.Fatalf("a word: %d %v, want refused", code, out)
	}
}

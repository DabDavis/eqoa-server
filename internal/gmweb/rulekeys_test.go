package gmweb

import "testing"

// A GM's npcclass and lootcreature rows are stored by their key (board 743):
// "Acolyte Satyr" written is "acolyte satyr" kept, on an insert and on an
// update of the name; a "#<row>" key stays as it is. The control: a table not
// keyed so (npctint) keeps the name as typed.
func TestRuleRowsAreStoredByTheirKey(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	ins := func(table string, values map[string]any) {
		t.Helper()
		if code, out := c.op("insert_row", map[string]any{"table": table, "values": values}); code != 200 {
			t.Fatalf("insert %s: %d %v", table, code, out)
		}
	}
	ins("npcclass", map[string]any{"name": "Acolyte Satyr", "class": 3, "source": "gm"})
	ins("lootcreature", map[string]any{"npc_name": "Acolyte Satyr ", "region": "odus", "profile": "", "source": "gm"})
	ins("lootcreature", map[string]any{"npc_name": "#118877", "region": "odus", "profile": "heroic trash", "source": "gm"})
	ins("npctint", map[string]any{"name": "Yhafiran", "color": 0x558C50FF, "note": "the control"})
	has := func(q string, args ...any) bool {
		var n int
		f.db.QueryRow(q, args...).Scan(&n)
		return n == 1
	}
	if !has(`SELECT count(*) FROM npcclass WHERE name = 'acolyte satyr' AND class = 3`) {
		t.Error("npcclass: not stored as acolyte satyr")
	}
	if !has(`SELECT count(*) FROM lootcreature WHERE npc_name = 'acolyte satyr'`) || !has(`SELECT count(*) FROM lootcreature WHERE npc_name = '#118877'`) {
		t.Error("lootcreature: not stored by its keys")
	}
	if !has(`SELECT count(*) FROM npctint WHERE name = 'Yhafiran'`) {
		t.Error("the control: npctint's name changed")
	}
	if code, out := c.op("update_row", map[string]any{"table": "npcclass", "key": map[string]any{"name": "acolyte satyr"},
		"values": map[string]any{"name": "Acolyte Faun"}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if !has(`SELECT count(*) FROM npcclass WHERE name = 'acolyte faun'`) {
		t.Error("npcclass's update: not stored as acolyte faun")
	}
}

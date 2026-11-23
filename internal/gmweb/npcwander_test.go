package gmweb

import (
	"strings"
	"testing"
)

// A spawn's own wander (npcwander) is a GM's table: a GM writes it, the world
// takes it at once (the next leg each wanderer chooses), each change is
// audited and reverts, and a radius no wander can be is refused with the world
// keeping what it had. The control: before the insert the spawn has no row.
func TestNPCWanderEditsAreLiveAuditedAndChecked(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if _, ok := f.world.RowWanderOf(502); ok {
		t.Fatal("the control: a row before the insert")
	}
	code, out := c.op("insert_row", map[string]any{"table": "npcwander",
		"values": map[string]any{"npcid": 502, "radius": 120, "note": "spread the camp"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "next leg") {
		t.Errorf("live: %v", out["live"])
	}
	if r, ok := f.world.RowWanderOf(502); !ok || r != 120 {
		t.Fatalf("after the insert: %v %v, want 120", r, ok)
	}

	for _, bad := range []any{2500, "far"} {
		code, out := c.op("update_row", map[string]any{"table": "npcwander", "key": map[string]any{"npcid": 502},
			"values": map[string]any{"radius": bad}})
		if code != 400 {
			t.Errorf("radius %v: %d %v, want refused", bad, code, out)
		}
	}
	if code, out := c.op("insert_row", map[string]any{"table": "npcwander",
		"values": map[string]any{"npcid": 0, "radius": 50}}); code != 400 {
		t.Errorf("npcid 0: %d %v, want refused", code, out)
	}
	if r, _ := f.world.RowWanderOf(502); r != 120 {
		t.Errorf("after the refusals: %v, want 120", r)
	}

	// stands: below 0 is read as -1
	code, out = c.op("update_row", map[string]any{"table": "npcwander", "key": map[string]any{"npcid": 502},
		"values": map[string]any{"radius": -5}})
	if code != 200 {
		t.Fatalf("radius -5: %d %v", code, out)
	}
	if r, _ := f.world.RowWanderOf(502); r != -1 {
		t.Errorf("radius -5 read as %v, want -1 (it stands)", r)
	}

	// audited by the GM, through the page; the update reverts
	var id int64
	var who, via string
	f.db.QueryRow(`SELECT id, username, via FROM gm_audit WHERE tbl = 'npcwander' AND action = 'update' ORDER BY id DESC LIMIT 1`).
		Scan(&id, &who, &via)
	if id == 0 || who != "gm" || via != "web" {
		t.Fatalf("the update's audit line: %d %q %q", id, who, via)
	}
	if code, out := c.op("revert_change", map[string]any{"id": id}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if r, _ := f.world.RowWanderOf(502); r != 120 {
		t.Errorf("reverted, the world has %v, want 120", r)
	}

	// deleted: back to its name's and its kind's
	if code, out := c.op("delete_row", map[string]any{"table": "npcwander", "key": map[string]any{"npcid": 502}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if _, ok := f.world.RowWanderOf(502); ok {
		t.Error("after the delete the world still has the row")
	}

	// the catalog: a GM's table, with its help
	code, out = c.op("describe_table", map[string]any{"table": "npcwander"})
	if code != 200 {
		t.Fatalf("describe: %d %v", code, out)
	}
	if out["write"] != "gm" || !strings.Contains(out["help"].(string), "own wander") {
		t.Errorf("npcwander's catalog: write %v, help %v", out["write"], out["help"])
	}
}

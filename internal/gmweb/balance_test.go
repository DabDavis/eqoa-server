package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A GM's balance rows are the world's at once: an NPC's hit points become its
// level's under the new row (keeping their share), the con row the next
// hit's, and a row out of bounds is refused.
func TestBalanceEditsAreLive(t *testing.T) {
	f := newFixture(t)
	f.exec(`DELETE FROM balancelevel`) // the seed rows out: the formula's hit points, then one row
	f.exec(`DELETE FROM balancecon`)
	c := f.browser()
	c.login("gm")
	st, ok := f.world.NPCStateOf(500)
	if !ok {
		t.Fatal("the fixture's NPC 500 is not in the world")
	}
	base := entity.NPCMaxHP(st.Level)

	code, out := c.op("insert_row", map[string]any{"table": "balancelevel",
		"values": map[string]any{"level": 1, "hp_pct": 300, "damage_pct": 150}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.MaxHP != 3*base || st.HP != 3*base {
		t.Errorf("after a 300%% row: %d/%d HP, want %d", st.HP, st.MaxHP, 3*base)
	}
	if !strings.Contains(out["live"].(string), "hit points") {
		t.Errorf("live: %v", out["live"])
	}

	code, out = c.op("update_row", map[string]any{"table": "balancelevel",
		"key": map[string]any{"level": 1}, "values": map[string]any{"hp_pct": 0}})
	if code == 200 {
		t.Errorf("a 0%% row was taken: %v", out)
	}
	code, out = c.op("delete_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 1}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.MaxHP != base {
		t.Errorf("after the row went: %d HP, want the formula's %d", st.MaxHP, base)
	}
	code, out = c.op("insert_row", map[string]any{"table": "balancecon",
		"values": map[string]any{"diff": 3, "damage_pct": 160, "taken_pct": 70}})
	if code != 200 {
		t.Fatalf("insert con: %d %v", code, out)
	}
	if !strings.Contains(out["live"].(string), "next hit") {
		t.Errorf("live: %v", out["live"])
	}
	code, out = c.op("insert_row", map[string]any{"table": "balancecon",
		"values": map[string]any{"diff": 40, "damage_pct": 160, "taken_pct": 70}})
	if code == 200 {
		t.Errorf("a diff of 40 was taken: %v", out)
	}
}

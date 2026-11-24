package gmweb

import (
	"fmt"
	"strings"
	"testing"
)

// post_work and work_board carry next, waiting_on, basis and basis_ref.
func TestTheWorkBoardCarriesTheResumeFields(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("post_work", map[string]any{"title": "Qeynos guards", "status": "in_progress",
		"next": "rename npcs 121689", "waiting_on": "jibrish", "basis": "era", "basis_ref": "Prima p.212"})
	if code != 200 {
		t.Fatalf("post: %d %v", code, out)
	}
	e := workEntryOf(t, out)
	if e["next"] != "rename npcs 121689" || e["waiting_on"] != "jibrish" || e["basis"] != "era" || e["basis_ref"] != "Prima p.212" {
		t.Fatalf("the entry: %v", e)
	}
	if code, out := gm.op("post_work", map[string]any{"id": e["id"], "basis": "vibes"}); code != 400 {
		t.Fatalf("a bad basis: %d %v", code, out)
	}
	code, board := gm.op("work_board", map[string]any{"basis": "era"})
	if code != 200 || board["matching"] != float64(1) {
		t.Fatalf("work_board basis era: %d %v", code, board)
	}
	got := board["entries"].([]any)[0].(map[string]any)
	for _, k := range []string{"next", "waiting_on", "basis", "basis_ref", "owner", "status", "version"} {
		if _, ok := got[k]; !ok {
			t.Errorf("work_board's entry has no %s: %v", k, got)
		}
	}
	// the table editor keeps the rules too
	if code, out := gm.op("update_row", map[string]any{"table": "workboard", "key": map[string]any{"id": e["id"]},
		"values": map[string]any{"basis": "vibes"}}); code == 200 {
		t.Fatalf("the editor took a bad basis: %v", out)
	}
}

// A row edit's reason, basis and basis_ref are its audit line's words; a bad
// basis is refused before anything is written.
func TestARowEditCarriesItsBasis(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501},
		"values": map[string]any{"npc_level": 21}, "reason": "the camp's level", "basis": "shared", "basis_ref": "petition 5590"}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	n, list := auditTotal(t, c, map[string]any{"search": "[basis:shared"})
	if n != 1 || list[0]["detail"] != "[basis:shared petition 5590] the camp's level" {
		t.Fatalf("the line: %v %v", n, list)
	}
	// the reference's petition is the line's petition, as any petition its words name
	if n, _ := auditTotal(t, c, map[string]any{"petition": 5590}); n != 1 {
		t.Fatalf("by petition: %v", n)
	}
	if code, out := c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501},
		"values": map[string]any{"npc_level": 22}, "basis": "vibes"}); code != 400 {
		t.Fatalf("a bad basis: %d %v", code, out)
	}
	if code, out := c.op("update_rows", map[string]any{"table": "npcs", "filters": []any{map[string]any{"column": "id", "op": "=", "value": 501}},
		"values": map[string]any{"npc_level": 23}, "basis": "owner", "basis_ref": "owner 2026-11-24"}); code != 200 {
		t.Fatalf("update_rows: %d %v", code, out)
	}
	if n, list := auditTotal(t, c, map[string]any{"search": "[basis:owner owner 2026-11-24]"}); n != 1 ||
		!strings.HasPrefix(fmt.Sprint(list[0]["detail"]), "[basis:owner owner 2026-11-24]") {
		t.Fatalf("update_rows' line: %v %v", n, list)
	}
	_, row := c.op("get_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501}})
	if r := row["row"].(map[string]any); r["npc_level"] != float64(23) {
		t.Fatalf("the row: %v", r)
	}
}

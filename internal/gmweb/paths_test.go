package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// The path editor's operations (ops_paths.go): a path set, changed and taken
// away through the audited row changes, each applied to the running world at
// once and each revertible; and what they refuse.

func pathAudit(t *testing.T, out map[string]any) float64 {
	t.Helper()
	ch, _ := out["change"].(map[string]any)
	id, _ := ch["audit"].(float64)
	if id == 0 {
		t.Fatalf("no audit entry in %v", out)
	}
	return id
}

func TestAPathIsSetChangedClearedAndRevertedLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	sq := []map[string]any{{"x": 19990, "z": 11990}, {"x": 20010, "z": 11990, "pause": 3}, {"x": 20010, "z": 12010}}

	// set: inserted, audited, walked
	code, out := c.op("set_npc_path", map[string]any{"id": 502, "points": sq, "mode": "back and forth", "pause": 2})
	if code != 200 {
		t.Fatalf("set_npc_path: %d %v", code, out)
	}
	setAudit := pathAudit(t, out)
	if live := out["live"].(string); !strings.Contains(live, "walks its path now: 3 point(s), back and forth") {
		t.Errorf("live %q", live)
	}
	p, ok := f.world.NPCPathOf(502)
	if !ok || p.Mode != entity.PathPingPong || len(p.Points) != 3 || p.Points[1].Pause <= 0 || p.Points[0].Pause >= 0 {
		t.Fatalf("the world's path: %+v %v", p, ok)
	}
	// no ground in the test: the heights are the spawn's
	if p.Points[0].Y != 40 || p.Points[2].Y != 40 {
		t.Errorf("heights %v, %v: want the spawn's 40", p.Points[0].Y, p.Points[2].Y)
	}

	// read back
	code, out = c.op("npc_path", map[string]any{"id": 502})
	path, _ := out["path"].(map[string]any)
	if code != 200 || path == nil || path["mode"] != "pingpong" || len(path["points"].([]any)) != 3 || out["live"] == nil {
		t.Fatalf("npc_path: %d %v", code, out)
	}
	code, out = c.op("npc_paths", map[string]any{"world": 0})
	if code != 200 || len(out["paths"].([]any)) != 1 {
		t.Fatalf("npc_paths world 0: %d %v", code, out)
	}
	if _, out = c.op("npc_paths", map[string]any{"world": 2}); len(out["paths"].([]any)) != 0 {
		t.Errorf("npc_paths world 2: %v", out)
	}

	// change: an update, audited
	code, out = c.op("set_npc_path", map[string]any{"id": 502, "points": sq, "mode": "once", "speed": "run"})
	if code != 200 || out["change"].(map[string]any)["action"] != "update" {
		t.Fatalf("change: %d %v", code, out)
	}
	changeAudit := pathAudit(t, out)
	if p, _ := f.world.NPCPathOf(502); p.Mode != entity.PathOnce || !p.Run {
		t.Fatalf("after the change %+v", p)
	}
	// revert the change: back and forth again, live
	if code, out = c.op("revert_change", map[string]any{"id": changeAudit}); code != 200 {
		t.Fatalf("revert the change: %d %v", code, out)
	}
	if p, _ := f.world.NPCPathOf(502); p.Mode != entity.PathPingPong || p.Run {
		t.Fatalf("reverted, the world has %+v", p)
	}

	// clear: deleted, audited, the NPC sent home
	code, out = c.op("clear_npc_path", map[string]any{"id": 502})
	if code != 200 {
		t.Fatalf("clear: %d %v", code, out)
	}
	clearAudit := pathAudit(t, out)
	if _, ok := f.world.NPCPathOf(502); ok {
		t.Fatal("cleared, the world still has the path")
	}
	if code, _ = c.op("clear_npc_path", map[string]any{"id": 502}); code != 404 {
		t.Errorf("clearing a path twice: %d, want 404", code)
	}
	// revert the clear: the path is back in the row and the world
	if code, out = c.op("revert_change", map[string]any{"id": clearAudit}); code != 200 {
		t.Fatalf("revert the clear: %d %v", code, out)
	}
	if p, ok := f.world.NPCPathOf(502); !ok || len(p.Points) != 3 {
		t.Fatalf("revert of the clear: %+v %v", p, ok)
	}
	// the row is as the first insert left it again, so that insert's revert
	// takes the path away
	if code, out = c.op("revert_change", map[string]any{"id": setAudit}); code != 200 {
		t.Fatalf("revert the insert: %d %v", code, out)
	}
	if _, ok := f.world.NPCPathOf(502); ok {
		t.Error("the insert reverted, the world still has the path")
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'npcpaths' AND username = 'gm'`).Scan(&n)
	if n != 6 { // insert, update, its revert, delete, its revert, the insert's revert
		t.Errorf("%d audit entries for npcpaths, want 6", n)
	}
}

func TestPathOpsRefuseWhatCannotBeWalked(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	pt := []map[string]any{{"x": 1, "z": 2}}
	for _, tc := range []struct {
		args map[string]any
		code int
	}{
		{map[string]any{"id": 502, "points": pt, "mode": "zigzag"}, 400},
		{map[string]any{"id": 502, "points": pt, "speed": "fly"}, 400},
		{map[string]any{"id": 502, "points": []map[string]any{}}, 400},
		{map[string]any{"id": 502, "points": []map[string]any{{"x": 1}}}, 400},
		{map[string]any{"id": 502, "points": pt, "pause": 601}, 400},
		{map[string]any{"id": 9999, "points": pt}, 404},
	} {
		if code, out := c.op("set_npc_path", tc.args); code != tc.code {
			t.Errorf("%v: %d %v, want %d", tc.args, code, out, tc.code)
		}
	}
	// the generic editor is held to the same: no bad path through insert_row
	if code, out := c.op("insert_row", map[string]any{"table": "npcpaths", "values": map[string]any{"npcid": 502, "points": "not json"}}); code != 400 {
		t.Errorf("insert_row with bad points: %d %v", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "npcpaths", "values": map[string]any{"npcid": 502, "points": `[{"x":1,"y":2,"z":3}]`, "mode": "zigzag"}}); code != 400 {
		t.Errorf("insert_row with a bad mode: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM npcpaths`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d paths written by refused calls", n)
	}
	// but a good one is taken there too, and walked
	if code, out := c.op("insert_row", map[string]any{"table": "npcpaths", "values": map[string]any{"npcid": 502, "points": `[{"x":1,"y":2,"z":3}]`}}); code != 200 {
		t.Fatalf("insert_row with a good path: %d %v", code, out)
	}
	if _, ok := f.world.NPCPathOf(502); !ok {
		t.Error("a path inserted through the generic editor is not walked")
	}
	// and an update that would break it is refused
	if code, out := c.op("update_row", map[string]any{"table": "npcpaths", "key": map[string]any{"npcid": 502}, "values": map[string]any{"points": "[]"}}); code != 400 {
		t.Errorf("update_row to an empty path: %d %v", code, out)
	}
}

func TestAReadOnlyCredentialCannotSetAPath(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "reader", "scope": "read", "days": 1})
	if code != 200 {
		t.Fatalf("token: %d %v", code, tok)
	}
	r := f.bearer(tok["token"].(string))
	if code, out := r.op("npc_paths", nil); code != 200 {
		t.Errorf("a read token listing paths: %d %v", code, out)
	}
	if code, _ := r.op("set_npc_path", map[string]any{"id": 502, "points": []map[string]any{{"x": 1, "z": 2}}}); code != 403 {
		t.Errorf("a read token set a path: %d", code)
	}
	if _, ok := f.world.NPCPathOf(502); ok {
		t.Error("the refused path reached the world")
	}
}

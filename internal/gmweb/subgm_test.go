package gmweb

import (
	"strings"
	"testing"
)

// A sub GM's panel (subgm.go; the owner, 2026-11-20: "give sub gms the gm panel
// map also to move mobs"): a tester whose row says subgm signs in as
// "tester+subgm", moves a creature's spawn (its place alone) and keeps the
// tester's tools; anything else of an NPC, an NPC row added or taken away, and
// the GM's map tools (teleport, a path) are refused. A plain tester is the
// control: it moves nothing.
func TestASubGMMovesCreaturesOnTheMap(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newTester()
	plain := f.routesOf("tess")
	if code, text, _ := atOp(plain.page, "update_row", `{"table":"npcs","key":{"id":502},"values":{"x_coord":20100}}`); code != 403 {
		t.Fatalf("a plain tester moved a creature: %d %s", code, text)
	}

	mustExec(t, f.db, `UPDATE gm_testers SET subgm = 1 WHERE accountid = 5`)
	r := f.routesOf("tess")
	code, _, out := atOp(r.page, "server_status", `{}`)
	you, _ := out["you"].(map[string]any)
	if code != 200 || you["role"] != "tester+subgm" {
		t.Fatalf("the sub GM's status: %d %v", code, out)
	}

	for route, c := range map[string]*client{"page": r.page, "openapi": r.token} {
		x := 20100
		if route == "openapi" {
			x = 20200
		}
		raw := `{"table":"npcs","key":{"id":502},"values":{"x_coord":` + itoa(x) + `,"z_coord":12050,"world":0}}`
		if code, text, _ := atOp(c, "update_row", raw); code != 200 {
			t.Fatalf("%s: the sub GM's move: %d %s", route, code, text)
		}
		var got float64
		f.db.QueryRow(`SELECT x_coord FROM npcs WHERE id = 502`).Scan(&got)
		if got != float64(x) {
			t.Fatalf("%s: the row's x %v, want %d", route, got, x)
		}
	}
	var user, tbl string
	f.db.QueryRow(`SELECT username, tbl FROM gm_audit ORDER BY id DESC LIMIT 1`).Scan(&user, &tbl)
	if user != "tess" || tbl != "npcs" {
		t.Errorf("the move's audit line: %q on %q", user, tbl)
	}
	for _, read := range []string{"map_markers", "map_layers"} {
		if code, text, _ := atOp(r.page, read, `{"world":0}`); code == 403 {
			t.Errorf("the map's %s refused a sub GM: %s", read, text)
		}
	}
	// still a tester
	if code, text, _ := atOp(r.page, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":6}}`); code != 200 {
		t.Errorf("the sub GM's item change: %d %s", code, text)
	}

	before := f.digest()
	for name, raw := range map[string]string{
		"update_row":         `{"table":"npcs","key":{"id":502},"values":{"npc_name":"a king"}}`,
		"insert_row":         `{"table":"npcs","values":{"npc_name":"new","world":0,"x_coord":1,"y_coord":1,"z_coord":1}}`,
		"delete_row":         `{"table":"npcs","key":{"id":502}}`,
		"clone_row":          `{"table":"npcs","key":{"id":502}}`,
		"update_rows":        `{"table":"npcs","search":"skeleton","values":{"x_coord":1}}`,
		"teleport_character": `{"character":"Hero","x":1,"y":1,"z":1,"world":0}`,
		"set_npc_path":       `{"id":502,"points":[]}`,
		"spawn_npc":          `{"name":"a decaying skeleton"}`,
		"npc_action":         `{"id":502,"action":"kill"}`,
		"sql_query":          `{"sql":"SELECT 1"}`,
	} {
		code, text, _ := atOp(r.page, name, raw)
		if code != 403 {
			t.Errorf("the sub GM's %s: %d %.200s", name, code, text)
		} else if !strings.Contains(text, "not a sub GM's") && !strings.Contains(text, "is not a tester's") {
			t.Errorf("the sub GM's %s refused in other words: %.200s", name, text)
		}
	}
	if f.digest() != before {
		t.Error("a refused call changed the database")
	}
}

func itoa(n int) string {
	var b []byte
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

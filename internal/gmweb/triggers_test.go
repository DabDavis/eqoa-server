package gmweb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// triggerEngine starts the trigger engine over the fixture's database and
// world.
func (f *fixture) triggerEngine() *handlers.TriggerEngine {
	f.t.Helper()
	e := handlers.NewTriggerEngine(f.db, f.world, f.s.d.Sessions, database.NewInventoryStore(f.db))
	e.Start()
	f.t.Cleanup(e.Stop)
	return e
}

// A trigger saved on the page: one row (logged), its steps given as a list
// kept as their JSON, its problems named (a spawn of an NPC the world lacks,
// an item not on this server, a broadcast with no words), listed with them;
// a scope there is not is refused and nothing written; a change reverted from
// the log.
func TestATriggerIsSavedAndListed(t *testing.T) {
	f := newFixture(t)
	f.triggerEngine()
	c := f.browser()
	c.login("gm")
	code, out := c.op("trigger_save", map[string]any{"trigger": map[string]any{
		"slot": 2, "name": "Skeletons", "enabled": 1, "scope": "near",
		"steps": []any{
			map[string]any{"do": "spawn", "npc": "a decaying skeleton", "count": 2, "hostile": true},
			map[string]any{"do": "spawn", "npc": "Nobody Here"},
			map[string]any{"do": "give", "items": "11, 999"},
			map[string]any{"do": "broadcast"},
		},
	}})
	if code != 200 {
		t.Fatalf("trigger_save: %d %v", code, out)
	}
	ps := problemsOf(out)
	for _, want := range []string{`step 2: no NPC named "Nobody Here"`, "step 3: no item 999", "step 4: broadcast says nothing"} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	if strings.Contains(ps, "step 1") || strings.Contains(ps, "item 11") {
		t.Errorf("the control: a step written right has a problem:\n%s", ps)
	}
	var steps string
	f.db.QueryRow(`SELECT steps FROM gmtriggers WHERE slot = 2`).Scan(&steps)
	if !strings.HasPrefix(steps, `[{"do":"spawn","npc":"a decaying skeleton","count":2,"hostile":true}`) {
		t.Errorf("steps kept as %q", steps)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'gmtriggers' AND action = 'insert'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d insert lines in the log", n)
	}
	code, out = c.op("trigger_list", nil)
	list, _ := out["triggers"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("trigger_list: %d %v", code, out)
	}
	row := list[0].(map[string]any)
	if fmt.Sprint(row["problems"]) != "3" || row["menu"] != true || fmt.Sprint(row["steps"]) != "4" {
		t.Errorf("listed as %v", row)
	}
	if code, out := c.op("trigger_save", map[string]any{"trigger": map[string]any{"slot": 2, "scope": "everyone"}}); code != 400 {
		t.Errorf("a scope there is not: %d %v", code, out)
	}
	if code, out := c.op("trigger_save", map[string]any{"trigger": map[string]any{"slot": 0, "name": "x"}}); code != 400 {
		t.Errorf("slot 0: %d %v", code, out)
	}
	if code, out := c.op("trigger_save", map[string]any{"trigger": map[string]any{"slot": 2, "name": "Bones"}}); code != 200 {
		t.Fatalf("a rename: %d %v", code, out)
	}
	var name string
	f.db.QueryRow(`SELECT name FROM gmtriggers WHERE slot = 2`).Scan(&name)
	if name != "Bones" {
		t.Fatalf("renamed to %q", name)
	}
	var auditID int64
	f.db.QueryRow(`SELECT id FROM gm_audit WHERE tbl = 'gmtriggers' AND action = 'update' ORDER BY id DESC LIMIT 1`).Scan(&auditID)
	if code, out := c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT name FROM gmtriggers WHERE slot = 2`).Scan(&name)
	if name != "Skeletons" {
		t.Errorf("reverted, the name is %q", name)
	}
	if names := handlers.Triggers().List(); len(names) != 1 || names[0].Name != "Skeletons" {
		t.Errorf("the engine read %+v", names)
	}
}

// Fired from the page: a trigger with a place of its own runs with no
// character in the world (its spawns stand there) and is logged; one at the
// GM's place is refused, as is an empty slot; trigger_runs shows the run and
// trigger_stop ends it, its spawns taken away.
func TestATriggerIsFiredAndStoppedFromThePanel(t *testing.T) {
	f := newFixture(t)
	f.triggerEngine()
	c := f.browser()
	c.login("gm")
	for _, trig := range []map[string]any{
		{"slot": 3, "name": "Gate raid", "scope": "all", "world": 0, "x": 25000, "y": 50, "z": 15000,
			"steps": `[{"do":"spawn","npc":"a decaying skeleton","count":2,"minutes":5},{"do":"wait","seconds":600},{"do":"despawn"}]`},
		{"slot": 4, "name": "Here", "scope": "near", "steps": `[{"do":"broadcast","text":"hi"}]`},
	} {
		if code, out := c.op("trigger_save", map[string]any{"trigger": trig}); code != 200 {
			t.Fatalf("trigger_save %v: %d %v", trig["slot"], code, out)
		}
	}
	code, out := c.op("trigger_fire", map[string]any{"slot": 3})
	if code != 200 || !strings.Contains(fmt.Sprint(out["lines"]), "spawned 2 a decaying skeleton within 5 of (25000, 15000)") {
		t.Fatalf("trigger_fire 3: %d %v", code, out)
	}
	if got := f.world.TemporaryNPCs(); len(got) != 2 {
		t.Fatalf("%d spawned", len(got))
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'gmtriggers' AND detail LIKE 'fired trigger 3:%'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d firing lines in the log", n)
	}
	if code, out := c.op("trigger_fire", map[string]any{"slot": 4}); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "where the GM stands") {
		t.Errorf("at the GM's place with no character: %d %v", code, out)
	}
	if code, out := c.op("trigger_fire", map[string]any{"slot": 9}); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "none set") {
		t.Errorf("an empty slot: %d %v", code, out)
	}
	code, out = c.op("trigger_runs", nil)
	runs, _ := out["runs"].([]any)
	if code != 200 || len(runs) != 1 || fmt.Sprint(runs[0].(map[string]any)["spawns"]) != "2" {
		t.Fatalf("trigger_runs: %d %v", code, out)
	}
	code, out = c.op("trigger_stop", map[string]any{"slot": 3})
	if code != 200 || fmt.Sprint(out["runs"]) != "1" || fmt.Sprint(out["spawns"]) != "2" {
		t.Fatalf("trigger_stop: %d %v", code, out)
	}
	if got := f.world.TemporaryNPCs(); len(got) != 0 {
		t.Errorf("after the stop: %d spawns", len(got))
	}
}

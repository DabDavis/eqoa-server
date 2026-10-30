package gmweb

import (
	"fmt"
	"strings"
	"testing"
)

// A kill or loot step's other creatures (its also, table queststeptargets)
// on the Quests page: saved with the step as row changes (each logged), read
// back by quest_get in the order given, a name no creature has named among
// the problems (the control: the names that spawn here are not), changed by a
// save (the one dropped deleted, the one kept left), reverted from the log,
// the step's own deleted with the step, a talk step's said to count for
// nothing, a draft lacking one of them in the quest list, and all of them
// gone with the quest.
func TestAStepsOtherCreaturesAreSaved(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	f.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (503, 'Magpie ', 0, 20010, 40, 12010, 0, 1893243078, 1.0, 2, 0, '0'),
		       (504, 'crow', 0, 20020, 40, 12020, 0, 1893243078, 1.0, 2, 0, '0')`)
	c := f.browser()
	c.login("gm")
	steps := []map[string]any{
		{"kind": "loot", "target": "a decaying skeleton", "also": []any{"magpie", " Crow", "MAGPIE", "a missing bird"},
			"items": "10", "count": 1, "chance": 0.5, "journal": "Take the sword from the skeletons, magpies and crows."},
		{"kind": "talk", "target": "Guard Dahvel", "also": []any{"Merchant Shadehand"}, "journal": "Return to the guard."},
	}
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "Birds", "enabled": 0, "giver": "Guard Dahvel"}, "steps": steps})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	quest, _ := out["quest"].(map[string]any)
	id, _ := asFloat(quest["questid"])
	ps := problemsOf(out)
	for _, want := range []string{`step 1: no creature named "a missing bird" spawns here`,
		"step 2 is a talk step: its other creatures (Merchant Shadehand) count for nothing"} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	if strings.Contains(ps, `"magpie"`) || strings.Contains(ps, `"Crow"`) || strings.Contains(ps, "none of its creatures") {
		t.Errorf("the control: a name that spawns here is a problem:\n%s", ps)
	}
	also := func(step int) string {
		var names []string
		rs, err := f.db.Query(`SELECT target FROM queststeptargets WHERE questid = ? AND step = ? ORDER BY rowid`, int64(id), step)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		for rs.Next() {
			var n string
			rs.Scan(&n)
			names = append(names, n)
		}
		return strings.Join(names, ", ")
	}
	if got := also(1); got != "magpie, Crow, a missing bird" {
		t.Errorf("step 1's rows %q, want each name once, trimmed, in order", got)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'queststeptargets' AND action = 'insert'`).Scan(&n)
	if n != 4 {
		t.Errorf("%d audit lines for the rows, want 4", n)
	}
	code, out = c.op("quest_get", map[string]any{"quest": id})
	if code != 200 {
		t.Fatalf("quest_get: %d %v", code, out)
	}
	got, _ := out["steps"].([]any)
	if len(got) != 2 || fmt.Sprint(got[0].(map[string]any)["also"]) != "[magpie Crow a missing bird]" {
		t.Errorf("quest_get's steps %v", got)
	}

	// the quest list: a draft that lacks a creature says so
	code, out = c.op("quest_list", map[string]any{"search": "birds"})
	if code != 200 || !strings.Contains(fmt.Sprint(out["quests"]), `no creature "a missing bird" (step 1)`) {
		t.Errorf("quest_list: %d %v", code, out)
	}

	// saved again: the missing bird dropped, crow kept as it is, a new one added
	steps[0]["also"] = []any{"crow", "magpie", "Skeleton Lord"}
	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "Birds"}, "steps": steps})
	if code != 200 {
		t.Fatalf("quest_save again: %d %v", code, out)
	}
	if got := also(1); got != "magpie, Crow, Skeleton Lord" {
		t.Errorf("step 1's rows after %q", got)
	}
	var auditID int64
	f.db.QueryRow(`SELECT id FROM gm_audit WHERE tbl = 'queststeptargets' AND action = 'delete' ORDER BY id DESC LIMIT 1`).Scan(&auditID)
	if code, out := c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if got := also(1); !strings.Contains(got, "a missing bird") {
		t.Errorf("reverted, step 1's rows are %q", got)
	}

	// a step taken away takes its rows; the quest deleted takes the rest
	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "Birds"}, "steps": steps[:1]})
	if code != 200 {
		t.Fatalf("quest_save shorter: %d %v", code, out)
	}
	if got := also(2); got != "" {
		t.Errorf("step 2 deleted, its rows are %q", got)
	}
	if code, out := c.op("quest_delete", map[string]any{"quest": id}); code != 200 {
		t.Fatalf("quest_delete: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM queststeptargets WHERE questid = ?`, int64(id)).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows left after the quest was deleted", n)
	}
}

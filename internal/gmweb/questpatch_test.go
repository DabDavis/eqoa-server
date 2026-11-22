package gmweb

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/handlers"
)

// Board entry 1250: quest_definition_patch, a reviewed quest content patch as one guarded
// change (ops_questpatch.go, handlers/questdefgate.go). The quests and numbers are the tests'.

// wholeRow is a row as the panel reads it, every column.
func (f *fixture) wholeRow(table string, key map[string]any) map[string]any {
	f.t.Helper()
	t, err := f.s.table(table)
	if err != nil {
		f.t.Fatal(err)
	}
	r, err := getRow(f.db, t, key)
	if err != nil {
		f.t.Fatal(err)
	}
	delete(r, "__key")
	return r
}

// patchFixture is quest 40, two steps, the second worth the experience the patch moves to
// the first; the manifest moves it and renames the quest.
func patchFixture(t *testing.T) (*fixture, *handlers.QuestEngine, []map[string]any) {
	f := newFixture(t)
	q := f.questEngine()
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (40, 'The Old Name', 1, 'Guard Dahvel')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, xp) VALUES (40, 1, 'talk', 'Guard Dahvel', 'Go.', 0)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, xp) VALUES (40, 2, 'talk', 'Guard Dahvel', 'Back.', 263426)`)
	if err := q.Reload(); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, r := range []struct {
		table string
		key   map[string]any
		col   string
		to    any
	}{
		{"queststeps", map[string]any{"questid": 40, "step": 1}, "xp", 263426},
		{"queststeps", map[string]any{"questid": 40, "step": 2}, "xp", 0},
		{"quests", map[string]any{"questid": 40}, "name", "The New Name"},
	} {
		before := f.wholeRow(r.table, r.key)
		after := map[string]any{}
		for k, v := range before {
			after[k] = v
		}
		after[r.col] = r.to
		rows = append(rows, map[string]any{"table": r.table, "key": r.key, "before": before, "after": after})
	}
	return f, q, rows
}

func (f *fixture) xpOf(step int) int {
	var xp int
	f.db.QueryRow(`SELECT xp FROM queststeps WHERE questid = 40 AND step = ?`, step).Scan(&xp)
	return xp
}

func (f *fixture) patchAudits() int {
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE detail LIKE 'quest definition patch:%'`).Scan(&n)
	return n
}

// The success: every row written and audited, the quests published once with the new name.
func TestQuestPatchAppliesOneGuardedChange(t *testing.T) {
	f, q, rows := patchFixture(t)
	c := f.browser()
	c.login("boss")
	gen := q.PublishGen()
	code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "board entry 1250 test"})
	if code != 200 {
		t.Fatalf("quest_definition_patch: %d %v", code, out)
	}
	if f.xpOf(1) != 263426 || f.xpOf(2) != 0 {
		t.Fatalf("xp now %d / %d", f.xpOf(1), f.xpOf(2))
	}
	if n := f.patchAudits(); n != 3 {
		t.Fatalf("%d audit lines, want 3", n)
	}
	if q.PublishGen() != gen+1 {
		t.Fatalf("published %d times, want once", q.PublishGen()-gen)
	}
	if name := q.InGame()[40]; name != "The New Name" {
		t.Fatalf("the game reads %q", name)
	}
	// a GM below admin may not
	g := f.browser()
	g.login("gm")
	if code, _ := g.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "x"}); code != 403 {
		t.Fatalf("a GM's patch: %d, want 403", code)
	}
}

// Drift, someone with the quest (in the database, finished; or in memory only), a dry run:
// nothing changes, nothing is audited, nothing published.
func TestQuestPatchRefusesAndChangesNothing(t *testing.T) {
	f, q, rows := patchFixture(t)
	c := f.browser()
	c.login("boss")
	gen := q.PublishGen()
	check := func(what string) {
		t.Helper()
		if f.xpOf(1) != 0 || f.xpOf(2) != 263426 || f.patchAudits() != 0 || q.PublishGen() != gen {
			t.Fatalf("%s changed something: xp %d/%d, %d audits, %d publications", what, f.xpOf(1), f.xpOf(2),
				f.patchAudits(), q.PublishGen()-gen)
		}
	}
	// drift: the second step moved since the manifest was read
	f.exec(`UPDATE queststeps SET journal = 'Back now.' WHERE questid = 40 AND step = 2`)
	if code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "moved") {
		t.Fatalf("drift: %d %v", code, out)
	}
	check("drift")
	f.exec(`UPDATE queststeps SET journal = 'Back.' WHERE questid = 40 AND step = 2`)
	// a finished row in the database
	f.exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES (7, 40, 0, 1)`)
	if code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "character row") {
		t.Fatalf("a finished row: %d %v", code, out)
	}
	check("a finished row")
	// in memory only: the character's row read, then gone from the database
	if _, err := q.SetCharacterQuest(7, 40, handlers.QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.exec(`DELETE FROM charquests WHERE questid = 40`)
	gen = q.PublishGen()
	if code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "character 7 quest 40") {
		t.Fatalf("in memory: %d %v", code, out)
	}
	check("a row in memory")
	if _, err := q.SetCharacterQuest(7, 40, handlers.QuestReset, 0); err != nil {
		t.Fatal(err)
	}
	f.exec(`DELETE FROM charquests WHERE questid = 40`)
	// the control of all three: a dry run passes and writes nothing; the patch then applies
	if code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d", "dry_run": true}); code != 200 ||
		out["dry_run"] != true {
		t.Fatalf("dry run: %d %v", code, out)
	}
	check("a dry run")
}

// A failure at any write or the commit leaves the rows, the audit and the game's quests as
// they were, and lets the quests go: the same patch then applies.
func TestQuestPatchFailureLeavesEverything(t *testing.T) {
	for at := 0; at <= 3; at++ {
		f, q, rows := patchFixture(t)
		c := f.browser()
		c.login("boss")
		gen := q.PublishGen()
		k := at
		questPatchFault = func(i int) error {
			if i == k {
				return errors.New("the disk is full")
			}
			return nil
		}
		code, _ := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d"})
		questPatchFault = nil
		if code == 200 || f.xpOf(1) != 0 || f.xpOf(2) != 263426 || f.patchAudits() != 0 || q.PublishGen() != gen ||
			q.InGame()[40] != "The Old Name" {
			t.Fatalf("fault at %d: code %d, xp %d/%d, %d audits, %d publications, name %q", at, code, f.xpOf(1), f.xpOf(2),
				f.patchAudits(), q.PublishGen()-gen, q.InGame()[40])
		}
		if code, out := c.op("quest_definition_patch", map[string]any{"rows": rows, "detail": "d"}); code != 200 {
			t.Fatalf("fault at %d, then none: %d %v", at, code, out)
		}
	}
}

// The manifest's shape: whole rows, an unchanged key, a table of its own, no row twice.
func TestQuestPatchManifestShape(t *testing.T) {
	f, _, rows := patchFixture(t)
	c := f.browser()
	c.login("boss")
	part := map[string]any{"table": "queststeps", "key": map[string]any{"questid": 40, "step": 1},
		"before": map[string]any{"questid": 40, "step": 1, "xp": 0}, "after": map[string]any{"questid": 40, "step": 1, "xp": 5}}
	for name, bad := range map[string][]any{
		"a part row":    {part},
		"a row twice":   {rows[0], rows[0]},
		"another table": {map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}, "before": map[string]any{}, "after": map[string]any{}}},
	} {
		if code, _ := c.op("quest_definition_patch", map[string]any{"rows": bad, "detail": "d"}); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if code, _ := c.op("quest_definition_patch", map[string]any{"rows": rows}); code != 400 {
		t.Error("taken without detail")
	}
}

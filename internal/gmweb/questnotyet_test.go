package gmweb

import (
	"strings"
	"testing"
)

// questnotyet (a quest giver's "not yet", handlers/quests_notyet.go) is a
// GM's table, in the catalog: a GM's line is the game's at once (the edit
// clears the 30 s read the engine keeps), the change is audited, and its
// revert takes the line out again. The control: a line written past the
// panel is not read until that read is old.
func TestQuestNotYetEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	has := func(line string) bool {
		t.Helper()
		lines, err := f.db.QuestNotYet()
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			if l.Line == line {
				return true
			}
		}
		return false
	}
	if !has("You too puny. Come back when you big and strong.") {
		t.Fatal("the table as made has no Oggok line")
	}
	mustExec(t, f.db, `INSERT INTO questnotyet (city, line) VALUES ('Oggok', 'Written past the panel.')`)
	if has("Written past the panel.") {
		t.Fatal("the control: a line written past the panel was read at once")
	}

	const line = "You tiny. Go eat rocks. Come back big."
	code, out := c.op("insert_row", map[string]any{"table": "questnotyet", "values": map[string]any{"city": "Oggok", "line": line}})
	if code != 200 {
		t.Fatalf("a GM's insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") {
		t.Errorf("live: %v", out["live"])
	}
	if !has(line) || !has("Written past the panel.") {
		t.Error("after the GM's insert the table is not read again")
	}
	audit, _ := out["audit"].(float64)
	if audit == 0 {
		t.Fatalf("no audit entry in %v", out)
	}
	if code, out = c.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if has(line) {
		t.Error("after the revert the GM's line is still read")
	}
}

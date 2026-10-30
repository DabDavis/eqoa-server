package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A GM's signtext row is what the sign says at once (database.SignTexts read
// again): an insert, an edit, the news row, a delete; a row the server
// cannot use (news on a sign, a sign's words on the news row, a kind it does
// not know, words past a few boxes, no sign named) is refused and changes
// nothing. The control: a row written behind the panel's back is not seen
// until the read runs out (30 s), which a GM's edit does not wait for.
func TestSignTextEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	defer database.InvalidateSignText()
	database.InvalidateSignText()
	words := func() map[int]database.SignText {
		t.Helper()
		rows, err := f.s.d.DB.SignTexts()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if len(words()) != 0 {
		t.Fatal("a new database has sign words")
	}
	mustExec(t, f.s.d.DB, `INSERT INTO signtext (npcid, kind, text) VALUES (499, 'gm', 'behind the panel')`)
	if _, seen := words()[499]; seen {
		t.Fatal("the control: a row written behind the panel is seen before the read runs out")
	}

	code, out := c.op("insert_row", map[string]any{"table": "signtext",
		"values": map[string]any{"npcid": 500, "kind": "road", "text": "North: Highpass Hold\nSouth: Freeport", "source": "gm"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") {
		t.Errorf("live: %v", out["live"])
	}
	rows := words()
	if rows[500].Text != "North: Highpass Hold\nSouth: Freeport" || rows[499].Text != "behind the panel" {
		t.Fatalf("after the insert: %v", rows)
	}
	if code, out := c.op("update_row", map[string]any{"table": "signtext", "key": map[string]any{"npcid": 500},
		"values": map[string]any{"text": "East: Freeport"}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if got := words()[500].Text; got != "East: Freeport" {
		t.Errorf("after the update: %q", got)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "signtext",
		"values": map[string]any{"npcid": 0, "kind": "news", "text": "The bank opens at dawn."}}); code != 200 {
		t.Fatalf("the news row: %d %v", code, out)
	}
	if got := words()[database.SignNewsID].Text; got != "The bank opens at dawn." {
		t.Errorf("the news: %q", got)
	}
	for _, bad := range []struct {
		row  map[string]any
		says string
	}{
		{map[string]any{"npcid": 501, "kind": "news", "text": "news on a sign"}, "news row"},
		{map[string]any{"npcid": 501, "kind": "shop", "text": "a kind the server does not know"}, "kind is one of"},
		{map[string]any{"npcid": 501, "kind": "gm", "text": strings.Repeat("x", database.SignTextMax+1)}, "at most"},
		{map[string]any{"kind": "gm", "text": "no sign named"}, "npcs row"},
	} {
		code, out := c.op("insert_row", map[string]any{"table": "signtext", "values": bad.row})
		if msg, _ := out["error"].(string); code != 400 || !strings.Contains(msg, bad.says) {
			t.Errorf("insert %v: %d %v, want refused saying %q", bad.row, code, out, bad.says)
		}
	}
	if code, out := c.op("update_row", map[string]any{"table": "signtext", "key": map[string]any{"npcid": 0},
		"values": map[string]any{"kind": "road"}}); code != 400 {
		t.Errorf("the news row made a road sign: %d %v, want refused", code, out)
	}
	if _, there := words()[501]; there {
		t.Error("a refused row was written")
	}
	if code, out := c.op("delete_row", map[string]any{"table": "signtext", "key": map[string]any{"npcid": 500}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if _, there := words()[500]; there {
		t.Error("after the delete the sign still has its words")
	}
}

package database

import (
	"strings"
	"testing"
)

// npctalk is read by the name's key and node, a row of choices that do not read left out, and read again
// once invalidated (the control: a row written after a read is not seen until then).
func TestNPCTalkReadAndInvalidated(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/talk.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npctalk (npc, node, says, choices) VALUES
		('alanaxia', 1, 'First line.' || char(10) || 'The question?', '[{"text":"Go on.","next":2}]'),
		('alanaxia', 2, 'The end.', '[]'),
		('broken', 1, 'x', 'not json')`); err != nil {
		t.Fatal(err)
	}
	InvalidateNPCTalk()
	trees, err := db.NPCTalk()
	if err != nil {
		t.Fatal(err)
	}
	n1 := trees["alanaxia"][1]
	if len(n1.Says) != 2 || n1.Says[1] != "The question?" || len(n1.Choices) != 1 || n1.Choices[0].Next != 2 {
		t.Errorf("node 1: %+v", n1)
	}
	if len(trees["alanaxia"][2].Choices) != 0 || trees["broken"] != nil {
		t.Errorf("node 2 or the broken row: %+v", trees)
	}
	db.Exec(`INSERT INTO npctalk (npc, node, says) VALUES ('alanaxia', 3, 'Later.')`)
	if trees, _ = db.NPCTalk(); len(trees["alanaxia"]) != 2 {
		t.Errorf("the control: a row written after the read was seen at once")
	}
	InvalidateNPCTalk()
	if trees, _ = db.NPCTalk(); len(trees["alanaxia"]) != 3 {
		t.Errorf("after InvalidateNPCTalk: %d nodes", len(trees["alanaxia"]))
	}
}

func TestTalkNodeProblems(t *testing.T) {
	if p := TalkNodeProblems("Hello.\nWell?", `[{"text":"Yes.","next":2},{"text":"No.","next":0}]`); len(p) != 0 {
		t.Errorf("a good node: %v", p)
	}
	for _, c := range []struct{ says, choices, want string }{
		{"x", "nope", "JSON"},
		{"x", `[{"text":"","next":1}]`, "no text"},
		{"x", `[{"text":"` + strings.Repeat("a", 64) + `","next":1}]`, "characters"},
		{"x", `[{"text":"a","next":-1}]`, "below 0"},
		{"x", `[{"text":"a"},{"text":"b"},{"text":"c"},{"text":"d"},{"text":"e"},{"text":"f"}]`, "at most"},
		{"a || b", "[]", "|"},
		{"x", `[{"text":"a","quest":-1}]`, "quest is below 0"},
		{"x", `[{"text":"a","quest":5,"lacking":"a|b"}]`, "|"},
		{strings.Repeat("q", 128), `[{"text":"a","next":0}]`, "question"},
	} {
		if p := strings.Join(TalkNodeProblems(c.says, c.choices), "; "); !strings.Contains(p, c.want) {
			t.Errorf("%q %q: %q, want %q", c.says, c.choices, p, c.want)
		}
	}
}

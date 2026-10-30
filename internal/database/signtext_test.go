package database

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A row is held to what the server reads: the news row and only it is npcid
// 0 of kind news, a sign is a positive npcid of a kind the server knows, and
// its words fit a few boxes.
func TestSignTextProblem(t *testing.T) {
	for _, c := range []struct {
		id         int
		kind, text string
		ok         bool
	}{
		{702, SignKindRoad, "North: Highpass Hold\nSouth: Freeport", true},
		{462, SignKindBoard, "Welcome to Freeport.", true},
		{SignNewsID, SignKindNews, "", true},
		{SignNewsID, SignKindNews, "The bank opens at dawn.", true},
		{703, SignKindGM, strings.Repeat("x", SignTextMax), true},
		{703, SignKindGM, strings.Repeat("x", SignTextMax+1), false},
		{SignNewsID, SignKindRoad, "East: Freeport", false},
		{702, SignKindNews, "news on a sign", false},
		{-1, SignKindGM, "a negative row", false},
		{702, "shop", "a kind the server does not know", false},
	} {
		if p := SignTextProblem(c.id, c.kind, c.text); (p == "") != c.ok {
			t.Errorf("npcid %d kind %q (%d characters): problem %q, want ok %v", c.id, c.kind, len(c.text), p, c.ok)
		}
	}
}

// signtext is read at most every signTextFor, and a GM's edit at once: a row
// written after a read is not seen until InvalidateSignText (the control),
// then is.
func TestSignTextsAreReadAgainWhenInvalidated(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/signs.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer InvalidateSignText()
	InvalidateSignText()
	if rows, err := db.SignTexts(); err != nil || len(rows) != 0 {
		t.Fatalf("a new database: %v %v", rows, err)
	}
	if _, err := db.Exec(`INSERT INTO signtext (npcid, kind, text) VALUES (702, 'road', 'East: Freeport')`); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.SignTexts(); len(rows) != 0 {
		t.Errorf("the control: a row written after a read is seen before the cache runs out: %v", rows)
	}
	InvalidateSignText()
	rows, err := db.SignTexts()
	if err != nil || rows[702].Text != "East: Freeport" || rows[702].Kind != SignKindRoad || rows[702].Source != "gm" {
		t.Errorf("after InvalidateSignText: %v %v", rows, err)
	}
}

// tools/seed_signs.py makes the table when the server has not, so its schema
// must be the server's.
func TestSeedSignsKeepsTheSchema(t *testing.T) {
	src, err := os.ReadFile("../../tools/seed_signs.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)\nSCHEMA = """(.*?)"""`).FindSubmatch(src)
	if m == nil {
		t.Fatal("tools/seed_signs.py has no SCHEMA")
	}
	norm := func(s string) string {
		return strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(s), ";")), " ")
	}
	if got, want := norm(string(m[1])), norm(signTextSchema); got != want {
		t.Errorf("the tool's schema:\n%s\nthe server's:\n%s", got, want)
	}
}

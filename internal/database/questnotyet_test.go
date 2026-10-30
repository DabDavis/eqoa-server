package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// A new database's questnotyet holds the seed lines (source seed), the same
// read back; a database that has the table keeps what is in it, rows a GM
// took out included; its names are none a snapshot refuses.
func TestQuestNotYetIsMadeWithItsLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notyet.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM questnotyet WHERE source = 'seed'`).Scan(&n); err != nil || n != len(questNotYetSeed) {
		t.Fatalf("a new database's seed lines: %d (%v), want %d", n, err, len(questNotYetSeed))
	}
	InvalidateQuestNotYet()
	lines, err := db.QuestNotYet()
	if err != nil || len(lines) != len(questNotYetSeed) || lines[0].City != questNotYetSeed[0].City || lines[0].Line != questNotYetSeed[0].Line {
		t.Fatalf("read back: %d lines (%v), the first %+v", len(lines), err, lines[0])
	}
	if _, err := db.Exec(`DELETE FROM questnotyet`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT count(*) FROM questnotyet`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the lines a GM took out came back: %d (%v)", n, err)
	}
	for _, name := range []string{"questnotyet", "id", "city", "line", "source"} {
		if SecretLikeName(name) {
			t.Errorf("%q is named like a secret: every snapshot would be refused", name)
		}
	}
}

// Every starting city of the quest files (tools/quests/data, a 1-20 guide's
// key) and the were quests have lines of their own, and there is a line of
// no city; each line fits one dialogue box after the longest giver's name,
// says to come back, and names no sex: town dialog's one pool of lines is
// heard from every speaker, and these are as well.
func TestQuestNotYetSeedLines(t *testing.T) {
	cities := map[string]bool{QuestCityKey("Weres"): true}
	files, err := filepath.Glob("../../tools/quests/data/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("the quest files: %v (%d)", err, len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Quests []struct{ Key string } `json:"quests"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, q := range doc.Quests {
			if parts := strings.Split(q.Key, "/"); len(parts) >= 3 && parts[0] == "1-20" {
				cities[QuestCityKey(parts[1])] = true
			}
		}
	}
	if len(cities) < 14 {
		t.Fatalf("only %d cities in the quest files: %v", len(cities), cities)
	}
	have := map[string]int{}
	for _, s := range questNotYetSeed {
		have[QuestCityKey(s.City)]++
	}
	for c := range cities {
		if have[c] == 0 {
			t.Errorf("no line for %q", c)
		}
	}
	if have[""] == 0 {
		t.Error("no line of no city")
	}
	// the longest giver's name with its ": " (Circle of the Spider Queen)
	const room = 217 - len("Circle of the Spider Queen: ")
	gendered := regexp.MustCompile(`(?i)\b(lad|lass|lassie|sir|madam|ma'am|milady|milord|my lord|my lady|lord|lady|brother|sister|son|daughter|boy|girl|man|woman|mister|mistress|miss|he|she|his|her|him|husband|wife)\b`)
	if !gendered.MatchString("Come back when you are stronger, lad.") || gendered.MatchString("Come back when ye are stronger, friend.") {
		t.Fatal("the sex check's control")
	}
	for _, s := range questNotYetSeed {
		switch {
		case strings.TrimSpace(s.Line) == "":
			t.Errorf("%s: an empty line", s.City)
		case utf8.RuneCountInString(s.Line) > room:
			t.Errorf("%s: %d characters, more than one box holds after a giver's name: %q", s.City, utf8.RuneCountInString(s.Line), s.Line)
		case !strings.Contains(strings.ToLower(s.Line), "come back") && !strings.Contains(strings.ToLower(s.Line), "return"):
			t.Errorf("%s: says nothing of coming back: %q", s.City, s.Line)
		case gendered.MatchString(s.Line):
			t.Errorf("%s: names a sex (%q): %q", s.City, gendered.FindString(s.Line), s.Line)
		}
	}
}

// A city is one city however the game's data spells it: the quest files'
// slugs, the factions' towns and the coach map's.
func TestQuestCityKey(t *testing.T) {
	for _, same := range [][]string{
		{"Klick'Anon", "Klick`Anon", "Klik'Anon", "klick-anon"},
		{"Neriak", "neriak-nektulos", "Nektulos"},
		{"Fayspire", "Fayspires", "fayspire"},
		{"Surefall Glade", "surefall-glade"},
		{"Highpass", "Highpass Hold"},
		{"Hazinak", "Hazinak (Evil)", "Hazinak (Good)"},
		{"Muniel's Tea Garden", "Muniels Tea Garden"},
		{"Weres", "weres"},
	} {
		for _, s := range same[1:] {
			if QuestCityKey(s) != QuestCityKey(same[0]) {
				t.Errorf("%q is %q, %q is %q", s, QuestCityKey(s), same[0], QuestCityKey(same[0]))
			}
		}
	}
	if QuestCityKey("Freeport") == QuestCityKey("Qeynos") || QuestCityKey("") != "" || QuestCityKey("Grobb") == "" {
		t.Error("different cities, or none")
	}
}

package gmweb

import (
	"strings"
	"testing"
)

// A quest offered at a place (handlers/questplaceoffer.go; Heirloom petitions
// 3322/3325, the owner 2026-11-27) saved through the panel: its place columns
// written, read back by quest_get, and no "no giver" problem though it has
// none, for its place offers it. Its place at 0, 0 is a problem. The control:
// the same quest with no place and no giver is "no one offers it".
func TestAQuestOfferedAtAPlaceIsSaved(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	c := f.browser()
	c.login("gm")
	steps := []map[string]any{{"kind": "kill", "target": "a decaying skeleton", "count": 1, "journal": "Kill a skeleton."}}
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "Envaric Ascension", "enabled": 1, "giver": "", "accept": "I will.",
			"offerworld": 1, "offerx": 5408.1, "offery": 44.5, "offerz": 9159.4, "offerradius": 6},
		"steps": steps,
	})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	ps := problemsOf(out)
	if strings.Contains(ps, "no giver") {
		t.Errorf("a quest offered at a place called giverless:\n%s", ps)
	}
	quest, _ := out["quest"].(map[string]any)
	id, _ := asFloat(quest["questid"])
	var w int
	var x, r float64
	f.db.QueryRow(`SELECT offerworld, offerx, offerradius FROM quests WHERE questid = ?`, int64(id)).Scan(&w, &x, &r)
	if w != 1 || x < 5408 || x > 5408.2 || r != 6 {
		t.Errorf("written: world %d x %v radius %v", w, x, r)
	}

	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "Envaric Ascension",
		"offerx": 0, "offerz": 0}, "steps": steps})
	if code != 200 || !strings.Contains(problemsOf(out), "its place is 0, 0") {
		t.Errorf("a place at 0, 0 is no problem: %d\n%s", code, problemsOf(out))
	}
	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "Envaric Ascension",
		"offerradius": 0}, "steps": steps})
	if code != 200 || !strings.Contains(problemsOf(out), "it has no giver: no one offers it") {
		t.Errorf("the control: no place, no giver is no problem: %d\n%s", code, problemsOf(out))
	}
}

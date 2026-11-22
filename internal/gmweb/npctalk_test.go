package gmweb

import "testing"

// Work board 883: an npctalk row is keyed by the name's key and a node of 1 or more, its answers JSON a
// client can draw; checkRow refuses anything else. The table is a storyteller's whole, as npcdialog is.
func TestCheckRowHoldsNPCTalk(t *testing.T) {
	good := []map[string]any{
		{"npc": "alanaxia", "node": 1, "says": "Line.\nQuestion?", "choices": `[{"text":"Tell me.","next":2}]`},
		{"npc": "alanaxia", "node": 5.0, "says": "The end.", "choices": "[]"},
		{"says": "an edit of its words alone"},
	}
	for _, r := range good {
		if err := checkRow("npctalk", r); err != nil {
			t.Errorf("%v refused: %v", r, err)
		}
	}
	bad := []map[string]any{
		{"npc": "Alanaxia", "node": 1, "says": "x"},
		{"npc": "alanaxia", "node": 0, "says": "x"},
		{"npc": "alanaxia", "node": 1, "says": "x", "choices": "not json"},
		{"npc": "alanaxia", "node": 1, "says": "x", "choices": `[{"text":"a|b","next":0}]`},
	}
	for _, r := range bad {
		if err := checkRow("npctalk", r); err == nil {
			t.Errorf("%v allowed", r)
		}
	}
	if catalog["npctalk"].Label != "NPC talk" {
		t.Errorf("no NPC talk page: %+v", catalog["npctalk"])
	}
	if !storytellerWrites("npctalk") || !isStorytellerTable("npctalk") {
		t.Error("npctalk is not a storyteller's")
	}
	if err := storytellerRow("npctalk", "insert", map[string]any{"npc": "alanaxia", "says": "a || b"}); err == nil {
		t.Error("a storyteller wrote a | into a talk node")
	}
	if err := storytellerRow("npctalk", "insert", map[string]any{"npc": "alanaxia", "node": 1, "says": "Hello."}); err != nil {
		t.Errorf("a storyteller's node refused: %v", err)
	}
}

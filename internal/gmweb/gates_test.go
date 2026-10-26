package gmweb

import (
	"strings"
	"testing"
)

// A GM's spellgates row is the game's at once (entity/gates.go): inserted,
// the spell sends there; moved, it sends to the new place; a row the game
// cannot use (a kind of none of the three, a world past the client's table,
// no spell) is refused and the world keeps what it had. The control: before
// the insert the spell is no gate.
func TestSpellGateEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if _, ok := f.world.SpellGate("Nerius Gate"); ok {
		t.Fatal("the control: Nerius Gate a gate before the insert")
	}
	code, out := c.op("insert_row", map[string]any{"table": "spellgates",
		"values": map[string]any{"spell": "nerius gate", "kind": "self", "place": "Kara Village", "world": 0,
			"x": 21000, "y": 61, "z": 7667}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if g, ok := f.world.SpellGate("Nerius Gate"); !ok || g.X != 21000 || g.Place != "Kara Village" {
		t.Fatalf("after the insert: %+v %v", g, ok)
	}
	if !strings.Contains(out["live"].(string), "at once") {
		t.Errorf("live: %v", out["live"])
	}
	code, out = c.op("update_row", map[string]any{"table": "spellgates", "key": map[string]any{"spell": "nerius gate"},
		"values": map[string]any{"x": 21100, "z": 7600}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if g, _ := f.world.SpellGate("Nerius Gate"); g.X != 21100 || g.Z != 7600 {
		t.Errorf("after the move: %+v", g)
	}
	for _, bad := range []map[string]any{
		{"kind": "portal"},
		{"world": 30},
		{"radius": -1},
	} {
		code, out := c.op("update_row", map[string]any{"table": "spellgates", "key": map[string]any{"spell": "nerius gate"}, "values": bad})
		if code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	if code, out := c.op("insert_row", map[string]any{"table": "spellgates", "values": map[string]any{"kind": "self"}}); code != 400 {
		t.Errorf("a row with no spell: %d %v, want refused", code, out)
	}
	if g, _ := f.world.SpellGate("Nerius Gate"); g.Kind != "self" || g.World != 0 || g.X != 21100 {
		t.Errorf("after the refusals: %+v, want the moved row", g)
	}
}

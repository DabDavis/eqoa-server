package gmweb

import (
	"strings"
	"testing"
)

// A GM's teleport orb is the game's at once (entity/orbs.go): inserted, the
// world has it; a row the game cannot use (a destination inside the orb
// itself, a world past the client's table, a radius below 0 or past the most)
// is refused and the world keeps what it had. The control: before the insert
// the world has no orb. The places are made up.
func TestOrbEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if got := f.world.OrbsSummary(); !strings.Contains(got, "none") {
		t.Fatalf("the control: before the insert, %q", got)
	}
	code, out := c.op("insert_row", map[string]any{"table": "orbs",
		"values": map[string]any{"name": "a made orb", "world": 0, "x": 5000, "y": 50, "z": 5000,
			"to_world": 0, "to_x": 5300, "to_y": 50, "to_z": 5296.5, "to_facing": 3.14}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if got := f.world.OrbsSummary(); !strings.Contains(got, "1 in table orbs (world 0: 1)") {
		t.Fatalf("after the insert: %q", got)
	}
	if !strings.Contains(out["live"].(string), "at once") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{
		{"to_x": 5000.5, "to_y": 50, "to_z": 5000}, // inside itself
		{"world": 30},
		{"to_world": -1},
		{"radius": -1},
		{"radius": 51},
	} {
		code, out := c.op("update_row", map[string]any{"table": "orbs", "key": map[string]any{"id": 1}, "values": bad})
		if code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	// the control: a good move is taken
	code, out = c.op("update_row", map[string]any{"table": "orbs", "key": map[string]any{"id": 1},
		"values": map[string]any{"to_x": 5310, "radius": 2}})
	if code != 200 {
		t.Fatalf("a good update: %d %v", code, out)
	}
	if got := f.world.OrbsSummary(); !strings.Contains(got, "1 in table orbs (world 0: 1)") {
		t.Errorf("after the refusals and the move: %q", got)
	}
}

package gmweb

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A GM's npcmove speed is the world's at once (petition 5189: gharials
// walked a man's 5.0); one out of bounds (below 0.5, past a chase's 9.9, not
// a number) is refused and the world keeps what it had. The control: before
// the insert the name walks at the server's 5.0.
func TestNPCMoveSpeedEditsAreLiveAndChecked(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if s := f.world.WalkSpeedOf("Gharial"); s != entity.NPCWalkSpeed {
		t.Fatalf("the control: %v before the insert", s)
	}
	code, out := c.op("insert_row", map[string]any{"table": "npcmove",
		"values": map[string]any{"name": "gharial", "speed": 3}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if s := f.world.WalkSpeedOf("Gharial"); s != 3 {
		t.Fatalf("after the insert: %v, want 3", s)
	}
	for _, bad := range []any{0.2, -1, 12, "fast"} {
		code, out := c.op("update_row", map[string]any{"table": "npcmove", "key": map[string]any{"name": "gharial"},
			"values": map[string]any{"speed": bad}})
		if code != 400 {
			t.Errorf("speed %v: %d %v, want refused", bad, code, out)
		}
	}
	if s := f.world.WalkSpeedOf("Gharial"); s != 3 {
		t.Errorf("after the refusals: %v, want 3", s)
	}
	code, out = c.op("update_row", map[string]any{"table": "npcmove", "key": map[string]any{"name": "gharial"},
		"values": map[string]any{"speed": 0}})
	if code != 200 {
		t.Fatalf("speed 0: %d %v", code, out)
	}
	if s := f.world.WalkSpeedOf("Gharial"); s != entity.NPCWalkSpeed {
		t.Errorf("speed 0 read as %v, want the server's %v", s, entity.NPCWalkSpeed)
	}
}

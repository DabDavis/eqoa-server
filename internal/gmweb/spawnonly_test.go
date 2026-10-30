package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A GM's npcspawnonly row is the world's at once (entity/spawnonly.go): the
// NPC of the name, in any case, stands out of the world, and is back in it
// when the row goes; a row naming no one is refused. The control: the guard
// beside it is in the world throughout.
func TestSpawnOnlyEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	held := func(id entity.EntityID) bool {
		st, ok := f.world.NPCStateOf(id)
		return ok && st.SpawnOnly
	}

	code, out := c.op("insert_row", map[string]any{"table": "npcspawnonly",
		"values": map[string]any{"name": "A Decaying Skeleton", "note": "the test's"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if !held(502) || held(500) {
		t.Errorf("after the insert: the skeleton out of the world %v, the guard %v", held(502), held(500))
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "1 out of the world") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{{"name": "  "}, {"note": "no name"}} {
		if code, out := c.op("insert_row", map[string]any{"table": "npcspawnonly", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}

	code, out = c.op("delete_row", map[string]any{"table": "npcspawnonly", "key": map[string]any{"name": "A Decaying Skeleton"}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if held(502) {
		t.Errorf("after the delete the skeleton is still out of the world")
	}
}

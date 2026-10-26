package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A GM's npcsocial row is the next hit's at once: inserted, edited and
// deleted, the world's rules follow (entity/socialaggro.go).
func TestSocialEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	code, out := c.op("insert_row", map[string]any{"table": "npcsocial",
		"values": map[string]any{"npc_name": "Guard Dahvel", "social": 1, "faction": "The Freeport Militia"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if r, ok := f.world.SocialRuleFor("Guard Dahvel"); !ok || r != (entity.SocialRule{Social: true, Faction: "The Freeport Militia"}) {
		t.Errorf("the world's rule after the insert: %+v %v", r, ok)
	}
	if !strings.Contains(out["live"].(string), "next hit") {
		t.Errorf("live: %v", out["live"])
	}

	code, out = c.op("update_row", map[string]any{"table": "npcsocial",
		"key": map[string]any{"npc_name": "Guard Dahvel"}, "values": map[string]any{"radius": 45, "social": 0}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if r, _ := f.world.SocialRuleFor("Guard Dahvel"); r.Social || r.Radius != 45 {
		t.Errorf("the world's rule after the update: %+v", r)
	}

	code, out = c.op("delete_row", map[string]any{"table": "npcsocial", "key": map[string]any{"npc_name": "Guard Dahvel"}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if _, ok := f.world.SocialRuleFor("Guard Dahvel"); ok {
		t.Errorf("the rule is still in the world after the delete")
	}
}

package gmweb

import (
	"strings"
	"testing"
)

// A GM's cmcatalog row is the game's at once: inserted, the world offers it;
// disabled, no longer. The control: before the insert the world has no such
// rank.
func TestCMCatalogEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if f.world.CMRankByID(4242) != nil {
		t.Fatal("the control: rank 4242 before the insert")
	}
	code, out := c.op("insert_row", map[string]any{"table": "cmcatalog",
		"values": map[string]any{"id": 4242, "name": "Steady Hands 1", "mastery": "Steady Hands", "cost": 12,
			"level": 20, "effects": "DEX:3,DEXMax:10"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	r := f.world.CMRankByID(4242)
	if r == nil || r.Cost != 12 || r.Effects[3] != 3 || r.Effects[10] != 10 || !r.Enabled || r.Races != 2047 {
		t.Fatalf("the world's rank after the insert: %+v", r)
	}
	if !strings.Contains(out["live"].(string), "at once") {
		t.Errorf("live: %v", out["live"])
	}
	code, out = c.op("update_row", map[string]any{"table": "cmcatalog", "key": map[string]any{"id": 4242},
		"values": map[string]any{"enabled": 0}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if r := f.world.CMRankByID(4242); r == nil || r.Enabled {
		t.Errorf("after disabling: %+v", r)
	}
}

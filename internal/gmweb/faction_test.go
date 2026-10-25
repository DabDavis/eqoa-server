package gmweb

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A GM's faction hit is the next kill's at once; a character's standing
// edited while it is in the world is applied where it stands (so its logout
// does not save the old one over it), a deleted row the starting standing
// again.
func TestFactionEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	code, out := c.op("insert_row", map[string]any{"table": "factionhits",
		"values": map[string]any{"npc_name": "Guard Dahvel", "faction": "Citizens of Freeport", "delta": -25}})
	if code != 200 {
		t.Fatalf("insert hit: %d %v", code, out)
	}
	if got := f.world.FactionHitsFor("Guard Dahvel"); len(got) != 1 || got[0] != (entity.FactionHit{Faction: "Citizens of Freeport", Delta: -25}) {
		t.Errorf("the world's hits after the insert: %v", got)
	}
	if !strings.Contains(out["live"].(string), "next kill") {
		t.Errorf("live: %v", out["live"])
	}

	// Hero (character 7) in the world
	sess := f.world.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 5007}, 7, 7, 7, 1, true)
	p := f.world.CreatePlayer(sess, &database.CharacterRecord{ServerID: 7, CharName: "Hero", X: 25300, Y: 54, Z: 15700})
	code, out = c.op("insert_row", map[string]any{"table": "charfactions",
		"values": map[string]any{"serverid": 7, "faction": "Citizens of Freeport", "standing": -500}})
	if code != 200 {
		t.Fatalf("insert standing: %d %v", code, out)
	}
	if v, ok := f.world.SavedFactions(p.ServerID); !ok || v["Citizens of Freeport"] != -500 {
		t.Errorf("Hero's standings after the insert: %v", v)
	}
	if !strings.Contains(out["live"].(string), "Hero stands") {
		t.Errorf("live: %v", out["live"])
	}
	code, out = c.op("delete_row", map[string]any{"table": "charfactions",
		"key": map[string]any{"serverid": 7, "faction": "Citizens of Freeport"}})
	if code != 200 {
		t.Fatalf("delete standing: %d %v", code, out)
	}
	if v, _ := f.world.SavedFactions(p.ServerID); len(v) != 0 {
		t.Errorf("Hero's standings after the delete: %v", v)
	}

	// a starting standing edited reaches every window at once
	code, out = c.op("update_row", map[string]any{"table": "factions",
		"key": map[string]any{"name": "Citizens of Freeport"}, "values": map[string]any{"standing": -800}})
	if code != 200 {
		t.Fatalf("update faction: %d %v", code, out)
	}
	for _, s := range f.world.PlayerFactions(sess) {
		if s.Name == "Citizens of Freeport" && s.Standing != -800 {
			t.Errorf("the window says %d", s.Standing)
		}
	}
}

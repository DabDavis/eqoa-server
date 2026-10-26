package gmweb

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The World map's layers (ops_maplayers.go): the XP camps where their anchors
// stand, a camp's loot tables by its creatures, and the coach stops with the
// towns their coach menu offers and the live game's; another world's coachman
// is not on this world's map, and a camp nobody named is not found.
func TestTheMapLayersAreCampsAndCoaches(t *testing.T) {
	f := newFixture(t)
	// dfc's anchor where camps.go writes it (21080.5, 19078.3), two coachmen
	// of Tunaria and the Rathe's
	mustExec(t, f.db, `INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (600, 'Deathfist champion', 0, 21080.5, 54.1, 19078.3, 0, 1893243078, 1.0, 36, 0, '0'),
		       (601, 'Coachman Ronks', 0, 25274, 54.1, 15719, 0, 1893243078, 1.0, 20, 128, '0'),
		       (602, 'Coachman Billfer', 0, 4813, 54, 17144, 0, 1893243078, 1.0, 20, 128, '0'),
		       (603, 'Coachman Dilga', 1, 8962, 90, 7214, 0, 1893243078, 1.0, 20, 128, '0')`)
	mustExec(t, f.db, `INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES
		('plains', 10, 1, 30, 40, 'camp-seed'), ('plains', 11, 1, 50, 60, 'camp-seed')`)
	mustExec(t, f.db, `INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('deathfist champion', 'plains', 25, 1, 'camp-seed')`)
	f.world.LoadNPCs(database.NewNPCStore(f.db))
	c := f.browser()
	c.login("gm")

	code, out := c.op("map_layers", map[string]any{"world": 0})
	if code != 200 {
		t.Fatalf("map_layers: %d %v", code, out)
	}
	if out["you"] != nil {
		t.Errorf("you %v: the GM has no character in the game", out["you"])
	}
	var dfc map[string]any
	for _, v := range out["camps"].([]any) {
		if m := v.(map[string]any); m["key"] == "dfc" {
			dfc = m
		}
		if m := v.(map[string]any); m["world"] != float64(0) {
			t.Errorf("world 0's camps hold %v of world %v", m["key"], m["world"])
		}
	}
	if dfc == nil || dfc["fallback"] != false || dfc["x"] != 21080.5 || dfc["note"] == "" || dfc["guide"] != "29-34" {
		t.Fatalf("dfc %v, want at its anchor with EQOA-Data's note", dfc)
	}
	coaches := out["coaches"].([]any)
	if len(coaches) != 2 {
		t.Fatalf("%d coaches in Tunaria, want Ronks and Billfer (not the Rathe's Dilga): %v", len(coaches), coaches)
	}
	ronks := coaches[0].(map[string]any)
	if ronks["town"] != "Freeport" || ronks["id"] != float64(601) || ronks["x"] != 25274.0 {
		t.Fatalf("first coach %v, want Freeport's Coachman Ronks at his spawn", ronks)
	}
	if routes := ronks["routes"].([]any); len(routes) != 1 || routes[0] != "Qeynos" {
		t.Errorf("Ronks's menu %v, want Qeynos", routes)
	}
	if live := ronks["live"].([]any); !slices.Contains(live, any("Muniel's Tea Garden")) {
		t.Errorf("Freeport's live routes %v", live)
	}

	// a camp's loot, by an alias: the champion's table at the camp's middle level
	code, out = c.op("map_camp", map[string]any{"camp": "deathfist"})
	if code != 200 || out["key"] != "dfc" || out["level"] != float64(36) {
		t.Fatalf("map_camp: %d %v", code, out)
	}
	loot := out["loot"].([]any)
	if len(loot) != 1 {
		t.Fatalf("loot %v, want the plains table", loot)
	}
	pl := loot[0].(map[string]any)
	if pl["table"] != "plains" || pl["chance"] != float64(25) || pl["count"] != float64(1) || pl["items"].([]any)[0] != "Rusty Sword" {
		t.Errorf("the plains table %v, want 25%%, the one item of level 36: the Rusty Sword", pl)
	}
	if cr := pl["creatures"].([]any); len(cr) != 1 || cr[0] != "Deathfist champion" {
		t.Errorf("bound to %v, want the champion", cr)
	}
	// the controls
	if code, _ := c.op("map_camp", map[string]any{"camp": "freeport"}); code != 404 {
		t.Errorf("map_camp freeport: %d, want 404 (a place, not a camp)", code)
	}
	if code, out = c.op("map_layers", map[string]any{"world": 1}); code != 200 || len(out["coaches"].([]any)) != 1 {
		t.Errorf("the Rathe's coaches %v, want Dilga alone", out["coaches"])
	}
}

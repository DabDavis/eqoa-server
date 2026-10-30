package gmweb

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The World map's layers (ops_maplayers.go): the XP camps where their anchors
// stand, a camp's loot tables by its creatures, the coach stops with the
// towns their coach menu may offer (those of the world its coach runs to) and
// the live game's, and the towns to name; another world's coachman or town is
// not on this world's map, and a camp nobody named is not found.
func TestTheMapLayersAreCampsAndCoaches(t *testing.T) {
	f := newFixture(t)
	// dfc's anchor where camps.go writes it (21080.5, 19078.3), three
	// coachmen of Tunaria and the Rathe's
	mustExec(t, f.db, `INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (600, 'Deathfist champion', 0, 21080.5, 54.1, 19078.3, 0, 1893243078, 1.0, 36, 0, '0'),
		       (601, 'Coachman Ronks', 0, 25274, 54.1, 15719, 0, 1893243078, 1.0, 20, 128, '0'),
		       (602, 'Coachman Billfer', 0, 4813, 54, 17144, 0, 1893243078, 1.0, 20, 128, '0'),
		       (603, 'Coachman Dilga', 1, 8962, 90, 7214, 0, 1893243078, 1.0, 20, 128, '0'),
		       (604, 'Coachman Quicksteed', 0, 16781, 188, 15356, 0, 1893243078, 1.0, 20, 128, '0')`)
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
	if len(coaches) != 3 {
		t.Fatalf("%d coaches in Tunaria, want Ronks, Quicksteed and Billfer (not the Rathe's Dilga): %v", len(coaches), coaches)
	}
	ronks := coaches[0].(map[string]any)
	if ronks["town"] != "Freeport" || ronks["id"] != float64(601) || ronks["x"] != 25274.0 {
		t.Fatalf("first coach %v, want Freeport's Coachman Ronks at his spawn", ronks)
	}
	if routes := ronks["routes"].([]any); len(routes) != 1 || routes[0] != "Highpass Hold" || ronks["routed"] != true {
		t.Errorf("Ronks's menu %v (routed %v), want Highpass Hold: Freeport's coach never ran to Qeynos", routes, ronks["routed"])
	}
	if billfer := coaches[2].(map[string]any); billfer["town"] != "Qeynos" || len(billfer["routes"].([]any)) != 0 {
		t.Errorf("third coach %v, want Qeynos with no stop here its coach runs to", billfer)
	}
	if live := ronks["live"].([]any); !slices.Contains(live, any("Muniel's Tea Garden")) {
		t.Errorf("Freeport's live routes %v", live)
	}
	// the towns: Tunaria's coach towns where and as coachtowns.go has them,
	// none of Odus's
	towns := map[string]map[string]any{}
	for _, v := range out["towns"].([]any) {
		m := v.(map[string]any)
		towns[m["name"].(string)] = m
	}
	if fp := towns["Freeport"]; fp == nil || fp["x"] != 25270.0 || fp["z"] != 15710.0 || towns["Hazinak (Good)"] == nil {
		t.Errorf("Tunaria's towns %v, want Freeport at (25270, 15710) and Hazinak (Good)", towns)
	}
	if towns["Arcadin"] != nil {
		t.Errorf("Odus's Arcadin is on Tunaria's map: %v", towns["Arcadin"])
	}
	code, out = c.op("map_layers", map[string]any{"world": 2})
	towns = map[string]map[string]any{}
	for _, v := range out["towns"].([]any) {
		m := v.(map[string]any)
		towns[m["name"].(string)] = m
	}
	if a := towns["Arcadin"]; code != 200 || a == nil || a["x"] != 8813.0 || a["world"] != 2.0 || towns["Freeport"] != nil {
		t.Errorf("Odus's towns %v, want Arcadin at (8813, 4338) by its map and no Freeport", towns)
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
	if code, out = c.op("map_layers", map[string]any{"world": 1}); code != 200 || len(out["coaches"].([]any)) != 1 ||
		out["coaches"].([]any)[0].(map[string]any)["routed"] != false {
		t.Errorf("the Rathe's coaches %v, want Dilga alone, off the network", out["coaches"])
	}
	if len(out["towns"].([]any)) != 0 {
		t.Errorf("the Rathe's towns %v, want none (EQOA-Data's map has none there)", out["towns"])
	}
}

// The map's quest givers (map_quests): every NPC that gives a quest, matched
// as the engine matches a giver (trimmed, any case), at its spawn, gold ("in
// the game") when any of its quests is in the game, else a draft's; with its
// quests and where their steps send players. A giver of another world is not
// on this one's map, and one no NPC of any world is, is missing.
func TestTheMapsQuestGivers(t *testing.T) {
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (603, 'Coachman Dilga', 1, 8962, 90, 7214, 0, 1893243078, 1.0, 20, 128, '0')`)
	f.world.LoadNPCs(database.NewNPCStore(f.db))
	mustExec(t, f.db, `INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, requires, source, notes) VALUES
		(1, 'Bread', 1, ' guard dahvel ', 1, 10, '0', '', 'gm', ''),
		(2, 'Skeletons', 0, 'Guard Dahvel', 3, 60, '0', '', 'gm', ''),
		(3, 'Wares', 0, 'Merchant Shadehand', 5, 60, '0,9', '', 'gm', ''),
		(4, 'Lost', 0, 'Maryan', 5, 60, '0', '', 'gm', ''),
		(5, 'The Rathe', 1, 'Coachman Dilga', 20, 60, '', '', 'gm', '')`)
	mustExec(t, f.db, `INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES
		(1, 1, 'talk', 'Merchant Shadehand', 1, 'Talk.'), (1, 2, 'talk', 'Guard Dahvel', 1, 'Back.'),
		(2, 1, 'kill', 'a decaying skeleton', 2, 'Kill two.'), (3, 1, 'talk', 'Merchant Shadehand', 1, 'Talk.'),
		(4, 1, 'talk', 'Maryan', 1, 'Talk.'), (5, 1, 'talk', 'Coachman Dilga', 1, 'Talk.')`)
	c := f.browser()
	c.login("gm")
	code, out := c.op("map_quests", map[string]any{"world": 0})
	if code != 200 {
		t.Fatalf("map_quests: %d %v", code, out)
	}
	byName := map[string]map[string]any{}
	for _, v := range out["givers"].([]any) {
		g := v.(map[string]any)
		byName[g["name"].(string)] = g
	}
	if len(byName) != 2 {
		t.Fatalf("givers %v, want Guard Dahvel and Merchant Shadehand (not the Rathe's Dilga)", byName)
	}
	dahvel := byName["Guard Dahvel"]
	if dahvel["status"] != "in the game" || dahvel["x"] != 25000.0 || len(dahvel["quests"].([]any)) != 2 {
		t.Fatalf("Guard Dahvel %v, want his two quests, in the game (one is), at his spawn", dahvel)
	}
	if tg := dahvel["targets"].([]any); len(tg) != 1 || tg[0] != "Merchant Shadehand" {
		t.Errorf("Dahvel's quests send players to %v, want Merchant Shadehand (not himself)", tg)
	}
	if q := dahvel["quests"].([]any)[0].(map[string]any); q["name"] != "Bread" || q["status"] != "in the game" {
		t.Errorf("his first quest %v, want Bread, in the game", q)
	}
	if shade := byName["Merchant Shadehand"]; shade["status"] != "draft" {
		t.Errorf("Shadehand %v, want a draft's (its one quest is not in the game)", shade)
	}
	if m := out["missing"].([]any); len(m) != 1 || m[0] != "Maryan" {
		t.Errorf("missing %v, want Maryan", m)
	}
	if _, out = c.op("map_quests", map[string]any{"world": 1}); len(out["givers"].([]any)) != 1 {
		t.Errorf("the Rathe's givers %v, want Dilga alone", out["givers"])
	}
}

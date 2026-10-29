package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A GM's npcfaction and factionstart rows (entity/faction.go) are written
// through the row editor, audited as any row is, and reach the running world
// at once: the NPC is of the faction, and a new character of the race starts
// with the standing; the catalog names the tables and their "any" columns.
func TestFactionTablesAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	skeleton := f.world.NPCs[entity.EntityID(502)]
	if skeleton == nil {
		t.Fatal("no skeleton in the world")
	}
	code, out := c.op("insert_row", map[string]any{"table": "npcfaction",
		"values": map[string]any{"name": "A Decaying Skeleton", "world": 0, "x": 20000, "z": 12000, "radius": 30,
			"faction": "Undead of Paineel", "note": "the test's"}})
	if code != 200 {
		t.Fatalf("npcfaction insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "1 NPCs are of 1 factions") {
		t.Errorf("npcfaction live: %v", out["live"])
	}
	if got := f.world.NPCFaction(skeleton); got != "Undead of Paineel" {
		t.Errorf("the skeleton is of %q", got)
	}

	// a new human of the east starting KOS to it: the skeleton beside its
	// start attacks it on sight
	code, out = c.op("insert_row", map[string]any{"table": "factionstart",
		"values": map[string]any{"race": 0, "class": -1, "humtype": 1, "faction": "Undead of Paineel", "standing": -2000}})
	if code != 200 {
		t.Fatalf("factionstart insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "faction window") {
		t.Errorf("factionstart live: %v", out["live"])
	}
	spot := database.StartSpot{Race: 0, Class: 0, HumType: 1, World: 0, X: 20010, Y: 40, Z: 12000}
	if th := f.world.StartThreats([]database.StartSpot{spot}); len(th) != 1 || th[0].Faction != "Undead of Paineel" || th[0].Standing != -2000 {
		t.Errorf("a new eastern human by the skeleton: %+v", th)
	}
	// a dark elf starts with the faction's own (-500 in the capture): left alone
	spot.Race, spot.HumType = 2, 0
	if th := f.world.StartThreats([]database.StartSpot{spot}); len(th) != 0 {
		t.Errorf("a new dark elf by the skeleton: %+v", th)
	}

	for _, tbl := range []string{"npcfaction", "factionstart"} {
		var audited int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = ?`, tbl).Scan(&audited); err != nil || audited != 1 {
			t.Errorf("%s audited %d times (%v)", tbl, audited, err)
		}
	}
	if n := catalog["factionstart"]; n.Label != "Faction starts" || n.Cols["race"].Enum != "raceany" ||
		n.Cols["class"].Enum != "classany" || n.Cols["humtype"].Enum != "humtype" || n.Cols["faction"].Ref != "factions" {
		t.Errorf("the catalog's factionstart: %+v", n)
	}
	if n := catalog["npcfaction"]; n.Label != "NPC factions" || n.Cols["world"].Enum != "worldany" {
		t.Errorf("the catalog's npcfaction: %+v", n)
	}
	e := enums()
	if e["raceany"][0].Value != -1 || len(e["raceany"]) != 12 || len(e["classany"]) != 16 || len(e["humtype"]) != 4 ||
		e["worldany"][0].Value != -1 || e["worldany"][1].Label != e["world"][0].Label {
		t.Errorf("the enumerations: raceany %d, classany %d, humtype %d, worldany %v", len(e["raceany"]), len(e["classany"]),
			len(e["humtype"]), e["worldany"][:2])
	}
}

package gmweb

import (
	"strings"
	"testing"
)

// The tradeskills' tables in the panel (live_craft.go): a recipe the game
// could not use is refused, a good one saved and read by the game at once; a
// character's skill outside the scale is refused; both are in the catalog.
func TestCraftRecipesInThePanel(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	good := map[string]any{"id": 77, "skill": 1, "comp": 0, "level": 1, "rarity": 0, "name": "Test Plate",
		"product": 10, "ingredients": `[{"item": 11, "count": 2, "keep": false}]`, "difficulty": 10, "xp": 40}
	for name, bad := range map[string]map[string]any{
		"bad JSON":            {"ingredients": `[{"item": 11`},
		"a component past 10": {"comp": 11},
		"level 0":             {"level": 0},
		"no ingredients":      {"ingredients": `[]`},
		"a yield of 0":        {"yield": 0},
	} {
		row := map[string]any{}
		for k, v := range good {
			row[k] = v
		}
		for k, v := range bad {
			row[k] = v
		}
		if code, out := c.op("insert_row", map[string]any{"table": "craftrecipes", "values": row}); code != 400 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	code, out := c.op("insert_row", map[string]any{"table": "craftrecipes", "values": good})
	if code != 200 {
		t.Fatalf("a good recipe: %d %v", code, out)
	}
	recipes, err := f.db.CraftRecipes()
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := recipes.Recipe(77); !ok || r.Yield != 1 || !r.Discover || r.Ingredients[0].Count != 2 {
		t.Fatalf("the game's recipe 77 after the insert: %+v (%v)", r, ok)
	}
	// an edit is the game's at once (the cache dropped)
	if code, out := c.op("update_row", map[string]any{"table": "craftrecipes", "key": map[string]any{"id": 77},
		"values": map[string]any{"xp": 55}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	recipes, _ = f.db.CraftRecipes()
	if r, _ := recipes.Recipe(77); r.XP != 55 {
		t.Fatalf("after the edit, xp %d", r.XP)
	}

	if code, out := c.op("insert_row", map[string]any{"table": "charcraftskills",
		"values": map[string]any{"charid": 7, "skill": 1, "level": 11, "rank": 0, "xp": 0}}); code != 400 {
		t.Errorf("a level 11 skill: %d %v", code, out)
	}
	code, out = c.op("insert_row", map[string]any{"table": "charcraftskills",
		"values": map[string]any{"charid": 7, "skill": 1, "level": 4, "rank": 2, "xp": 0}})
	if code != 200 || !strings.Contains(out["live"].(string), "next login") {
		t.Fatalf("a player's skill: %d %v", code, out)
	}
	for _, name := range []string{"craftrecipes", "charcraftskills", "charrecipes"} {
		if n, ok := catalog[name]; !ok || n.Label == "" {
			t.Errorf("%s is not in the catalog", name)
		}
	}
	if catalog["charrecipes"].Cols["charid"].Ref != "characters" || catalog["craftrecipes"].Cols["skill"].Enum != "craftskill" {
		t.Error("the catalog's columns")
	}
}

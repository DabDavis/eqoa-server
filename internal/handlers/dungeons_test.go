package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/worlds"
)

// TestReadDungeonsRecipe reads the repository's recipe's dungeons block (tools/worlds/dungeons.json) and refuses a
// bad one.
func TestReadDungeonsRecipe(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tools", "worlds", "dungeons.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r worlds.Recipe
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.World != DefaultDungeonsWorld {
		t.Fatalf("the recipe's slot %d, the default %d", r.World, DefaultDungeonsWorld)
	}
	defs, err := ReadDungeons(worlds.Built{Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]entity.DungeonDef{}
	for _, d := range defs {
		keys[d.Key] = d
	}
	for _, k := range []string{"guk", "blackburrow", "runnyeye"} {
		d, ok := keys[k]
		if !ok || len(d.Boxes) == 0 || d.Entrance.X == 0 {
			t.Fatalf("%s: %+v", k, d)
		}
	}
	// Guk's entrance faces north (toward smaller z: its gate), a facing of pi
	if f := keys["guk"].Entrance.Facing; f < 3.1 || f > 3.2 {
		t.Fatalf("Guk's entrance faces %.2f", f)
	}
	for _, bad := range []string{`[{"key":"a","name":"A","boxes":[]}]`, `[{"key":"a","name":"A","boxes":[[0,0,1,1]]}]`,
		`[{"key":"a","name":"A","boxes":[[0,0,1,1]],"entrance":{"x":5,"z":5}},{"key":"A","name":"B","boxes":[[0,0,1,1]],"entrance":{"x":5,"z":5}}]`} {
		if _, err := ReadDungeons(worlds.Built{Recipe: worlds.Recipe{World: 14, Dungeons: json.RawMessage(bad)}}); err == nil {
			t.Fatalf("read %s", bad)
		}
	}
}

// TestDungeonCreatureRollsItsRowsLoot: a creature of the dungeons' world rolls its row's tables ("#n") and every
// creature's, never its namesakes' in the open world.
func TestDungeonCreatureRollsItsRowsLoot(t *testing.T) {
	gw := entity.NewWorld()
	gw.SetDungeons(true, 14, nil, entity.DungeonSettings{})
	t.Cleanup(func() { gw.SetDungeons(false, 14, nil, entity.DungeonSettings{}) })
	tmpl := &entity.NPC{ServerID: 777001, Name: "gnoll scout", World: 14}
	copyOf := &entity.NPC{ServerID: 5, Name: "gnoll scout", World: 14, Temporary: true, CopyOf: 777001}
	out := &entity.NPC{ServerID: 6, Name: "gnoll scout", World: 0}
	for _, c := range []struct {
		n    *entity.NPC
		want []string
	}{{tmpl, []string{"#777001", "*"}}, {copyOf, []string{"#777001", "*"}}, {out, []string{"gnoll scout", "*"}}} {
		if got := lootNames(c.n); len(got) != len(c.want) || got[0] != c.want[0] || got[1] != c.want[1] {
			t.Fatalf("%+v rolls %v, want %v", c.n, got, c.want)
		}
	}
}

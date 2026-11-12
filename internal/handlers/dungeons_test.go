package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
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

// TestDungeonLoginAtTheEntrance: a login saved inside its party's open instance resumes at the dungeon's entrance,
// never where it logged out (petition 1808: a player stuck under a rock at Blackburrow's old landing would have been
// put back under it), and in the instance.
func TestDungeonLoginAtTheEntrance(t *testing.T) {
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	r.anna.SetServerWorlds([]uint8{14})
	dungeonCommand(r.gw, r.anna, "enter bb")
	r.travel()
	if r.pa.World != 14 || r.pa.InstanceKey() == 0 {
		t.Fatalf("Anna not in the dungeon: world %d instance %d", r.pa.World, r.pa.InstanceKey())
	}
	held := func(w uint8) bool { return w == 14 }
	rec := &database.CharacterRecord{ServerID: r.pa.CharServerID, CharName: "Anna", World: 14, X: 1050, Y: 57, Z: 960}
	loginSpawn(r.gw, rec, held, nil)
	if rec.World != 14 || rec.X != 1000 || rec.Y != 50 || rec.Z != 1150 {
		t.Fatalf("a login inside the instance: world %d (%.1f, %.1f, %.1f), want its entrance", rec.World, rec.X, rec.Y, rec.Z)
	}
}

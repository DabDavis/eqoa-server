package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The season step (gmtriggers.go's seasonLocked; the owner, 2026-11-16: "lets
// make a christmas season trigger"): fired, the Christmas season turns on, its
// tree comes into the world and its lights are laid, the text said; fired again
// it turns off, the tree leaves and the offtext is said; the season is kept in
// gmseasons. on and off turn it so whatever it was, and a turn to what it is
// says nothing.
func TestTriggerSeasonTurnsTheTreeAndItsLights(t *testing.T) {
	w := newTrigWorld(t)
	for _, q := range []string{
		`INSERT INTO npcseason (name, season) VALUES ('Christmas Tree', 'Christmas')`,
		`INSERT INTO lighttemplates (name, colors) VALUES ('Christmas lights', 'FF2020,20D040')`,
		`INSERT INTO lightstrings (template, npc, shape, params, season) VALUES ('Christmas lights', 'Christmas Tree', 'spiral', '{"r0":3,"y0":1,"r1":0.5,"y1":9,"turns":3}', 'Christmas')`,
	} {
		if _, err := w.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w.gw.NPCs[701] = &entity.NPC{ServerID: 701, Name: "Christmas Tree", HP: 100, MaxHP: 100, Level: 1, NPCType: 0x80, X: 100, Y: 50, Z: 100}
	if _, _, err := w.gw.LoadSpawnOnly(w.db); err != nil {
		t.Fatal(err)
	}
	tree := w.gw.NPCs[701]
	if !tree.SpawnOnly {
		t.Fatalf("the season never turned on: the tree stands in the world")
	}
	w.set(20, "Christmas Season", "all", 0, `[{"do":"season","season":"Christmas","text":"Merry Christmas, %p!","offtext":"It is over."}]`)
	w.clear()
	lines, err := w.fire(20)
	if err != nil {
		t.Fatalf("%v %q", err, lines)
	}
	on, _ := w.db.SeasonActive("christmas")
	if !on || tree.SpawnOnly {
		t.Fatalf("fired: season on %v, tree held %v; want on, in the world (%q)", on, tree.SpawnOnly, lines)
	}
	if got := yellow(t, w.ps); len(got) != 1 || got[0] != "Merry Christmas, Player!" {
		t.Errorf("fired: the player heard %q", got)
	}
	if strs, bulbs, _, _, _ := w.gw.LoadDecorLights(w.db); strs != 1 || bulbs < 10 {
		t.Errorf("fired: %d strings, %d bulbs lit", strs, bulbs)
	}
	w.clear()
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	on, _ = w.db.SeasonActive("christmas")
	if on || !tree.SpawnOnly {
		t.Fatalf("fired again: season on %v, tree held %v; want off, out of the world", on, tree.SpawnOnly)
	}
	if got := yellow(t, w.ps); len(got) != 1 || got[0] != "It is over." {
		t.Errorf("fired again: the player heard %q", got)
	}
	// off when off: nothing said, still off
	w.set(21, "Christmas Off", "all", 0, `[{"do":"season","season":"Christmas","turn":"off","offtext":"It is over."}]`)
	w.clear()
	if _, err := w.fire(21); err != nil {
		t.Fatal(err)
	}
	if got := yellow(t, w.ps); len(got) != 0 {
		t.Errorf("turned off when off: the player heard %q", got)
	}
	if on, _ := w.db.SeasonActive("christmas"); on {
		t.Errorf("turned off: on")
	}
}

func TestSeasonStepProblems(t *testing.T) {
	if p := database.StepProblems([]database.TriggerStep{{Do: "season", Season: "Christmas"}, {Do: "season", Season: "x", Turn: "on"}}); len(p) != 0 {
		t.Errorf("good steps: %v", p)
	}
	if p := database.StepProblems([]database.TriggerStep{{Do: "season"}, {Do: "season", Season: "x", Turn: "sideways"}}); len(p) != 2 {
		t.Errorf("bad steps: %v", p)
	}
}

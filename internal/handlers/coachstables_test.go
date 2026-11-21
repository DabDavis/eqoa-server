package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// Blackvein Deep's coachman (world 10, in the cave under the orcs' home) signs
// Blackvein Deep's ledger and runs to Moradhim alone; Moradhim's coachman runs
// to Blackvein Deep for a client that holds world 10, and never offers it to
// one that does not (a PS2), signed or not, nor to its GM.
func TestBlackveinDeepsCoachRunsToMoradhimForAClientThatHoldsIt(t *testing.T) {
	tw, _, book := coachWorld(t)
	entity.SetComposedWorld(10, true)
	b := worlds.Built{Recipe: worlds.Recipe{World: 10, Name: "the Homelands",
		Zones: []worlds.RecipeZone{{Name: "Thedruk", Race: 21}}},
		Parts: []worlds.BuiltZone{{Box: [6]float32{18000, -200, 8000, 20000, 300, 10000}}}}
	SetServerWorlds([]worlds.Built{b})
	t.Cleanup(func() { SetServerWorlds(nil); entity.SetComposedWorld(10, false) })
	for _, n := range []*entity.NPC{
		{ServerID: 9101, Name: "Coachman Deep", World: 10, SpawnX: 18760, SpawnY: -59, SpawnZ: 9150},
		{ServerID: 9102, Name: "Coachman Dursh", World: 10, SpawnX: 19804, SpawnY: 58, SpawnZ: 8850},
		{ServerID: 975, Name: "Coachman Doba", SpawnX: 15352, SpawnY: 54, SpawnZ: 9267},
	} {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		tw.w.NPCs[n.ServerID] = n
	}
	tw.s.SetServerWorlds([]uint8{10})

	deep := tw.at(2, 9101)
	tw.talk(deep)
	if !book[44]["Blackvein Deep"] || len(book[44]) != 1 {
		t.Fatalf("after the talk the book holds %v, want Blackvein Deep", book[44])
	}
	want := "Coachman Deep: My horses run to Moradhim, but you are in none of their stables' ledgers. Without that record, I cannot offer you passage."
	if got := dialogueBoxes(tw.s); !slices.Equal(got, []string{want}) {
		t.Errorf("his answer %q, want %q", got, want)
	}
	tw.s.Queue().GetPending()

	book[44]["Moradhim"], book[44]["Freeport"], book[44]["Thedruk"] = true, true, true
	tw.talk(deep)
	if got := offered(tw.p); !slices.Equal(got, []string{"Moradhim"}) {
		t.Fatalf("Blackvein Deep's coach offered %q, want Moradhim alone", got)
	}
	tw.pick(deep, 0)
	if x, z, world, moved := teleported(tw.s); !moved || !nearCoach(x, z, 15352, 9267) || world != 0 {
		t.Errorf("the ride to Moradhim went to (%.1f, %.1f) world %d (%v), want Doba's spot in Tunaria", x, z, world, moved)
	}
	tw.s.Queue().GetPending()

	doba := tw.at(3, 975)
	tw.talk(doba)
	if got := offered(tw.p); !slices.Contains(got, "Blackvein Deep") {
		t.Fatalf("Moradhim's coach offered %q, without Blackvein Deep", got)
	}
	tw.s.Queue().GetPending()
	// the orcs' camp's coachman is Thedruk's stable, and his coach does not run to the deep
	dursh := tw.at(4, 9102)
	book[44]["Blackvein Deep"] = true
	tw.talk(dursh)
	if got := offered(tw.p); slices.Contains(got, "Blackvein Deep") || slices.Contains(got, "Moradhim") {
		t.Errorf("Thedruk's coach offered %q", got)
	}
	tw.s.Queue().GetPending()

	// a client that does not hold world 10 (a PS2), and a GM's: Moradhim's coach never offers the deep
	tw.s.SetServerWorlds(nil)
	for _, gm := range []bool{false, true} {
		tw.h.isGM = func(*session.Session) bool { return gm }
		doba = tw.at(3, 975)
		tw.talk(doba)
		if got := offered(tw.p); slices.Contains(got, "Blackvein Deep") {
			t.Errorf("a client that does not hold world 10 (GM %v) was offered %q", gm, got)
		}
		tw.s.Queue().GetPending()
	}
}

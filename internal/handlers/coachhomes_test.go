package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// homesBuilt is a server's world 10 as built with three races' homes (and a
// zone that is no one's): Aviak Village, West Plateau, and Gerntar Mines,
// whose name a coach town of the game bears.
func homesBuilt() worlds.Built {
	zone := func(name string, race int, x, z float32) (worlds.RecipeZone, worlds.BuiltZone) {
		return worlds.RecipeZone{Name: name, Race: race}, worlds.BuiltZone{Box: [6]float32{x, 0, z, x + 2000, 300, z + 2000}}
	}
	b := worlds.Built{Recipe: worlds.Recipe{World: 10, Name: "the Homelands"}}
	for _, z := range []struct {
		name string
		race int
		x, z float32
	}{{"Aviak Village", 14, 8000, 20000}, {"West Plateau", 11, 6000, 2000}, {"Gerntar Mines", 23, 12000, 26000}, {"No One's Vale", 0, 2000, 2000}} {
		rz, bz := zone(z.name, z.race, z.x, z.z)
		b.Zones, b.Parts = append(b.Zones, rz), append(b.Parts, bz)
	}
	return b
}

// homesCoachWorld is coachWorld with the Homelands: the homes' coachmen (and
// one standing in the zone that is no race's), the session's client holding
// world 10.
func homesCoachWorld(t *testing.T) (tw *talkWorld, ronks uint32, book ledgerBook) {
	tw, ronks, book = coachWorld(t)
	entity.SetComposedWorld(10, true)
	SetServerWorlds([]worlds.Built{homesBuilt()})
	t.Cleanup(func() { SetServerWorlds(nil); entity.SetComposedWorld(10, false) })
	for _, n := range []*entity.NPC{
		{ServerID: 9001, Name: "Coachman Harrek", World: 10, SpawnX: 9211.7, SpawnY: 84.1, SpawnZ: 21547.1}, // Aviak Village
		{ServerID: 9002, Name: "Coachman Grumm", World: 10, SpawnX: 6947, SpawnY: 57.7, SpawnZ: 3300},       // West Plateau
		{ServerID: 9003, Name: "Coachman Brakk", World: 10, SpawnX: 13000, SpawnY: 60, SpawnZ: 27000},       // Gerntar Mines
		{ServerID: 9004, Name: "Coachman Lost", World: 10, SpawnX: 3000, SpawnY: 60, SpawnZ: 3000},          // no race's zone
	} {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		tw.w.NPCs[n.ServerID] = n
	}
	tw.s.SetServerWorlds([]uint8{10})
	return tw, ronks, book
}

// at stands the player by the NPC of that id, in its world, and puts it on
// the session's slot.
func (tw *talkWorld) at(slot int, id entity.EntityID) uint32 {
	n := tw.w.NPCs[id]
	tw.p.World = n.World
	tw.w.UpdatePosition(tw.p.ServerID, n.SpawnX, n.SpawnY, n.SpawnZ+2, 0)
	return tw.add(slot, n)
}

const homesMenuGerntar = "Gerntar Mines (the Homelands)"

// A home's coachman is a stable as any: the talk signs his home's ledger, said
// once; with no ledger of a town his coach runs to he says so, the homes
// together by their world's name, and offers only to sign every ledger. With
// Freeport's and another home's signed he offers those, and the rides land on
// the coachmen's own spots, Freeport's in Tunaria. No town but Freeport is
// his coach's: Highpass Hold, signed, is not offered.
func TestAHomesCoachRunsToFreeportAndTheOtherHomes(t *testing.T) {
	tw, _, book := homesCoachWorld(t)
	harrek := tw.at(2, 9001)
	tw.talk(harrek)
	if !book[44]["Aviak Village"] || len(book[44]) != 1 {
		t.Fatalf("after the talk the book holds %v, want Aviak Village", book[44])
	}
	if got := said(tw); !slices.Contains(got, "Coachman Harrek: "+ledgerSigned) {
		t.Errorf("Harrek said %q in chat, want his ledger signed", got)
	}
	want := "Coachman Harrek: My horses run to Freeport and the Homelands, but you are in none of their stables' ledgers. " +
		"Without that record, I cannot offer you passage."
	if got := dialogueBoxes(tw.s); !slices.Equal(got, []string{want}) {
		t.Errorf("Harrek's answer %q, want %q", got, want)
	}
	if len(offered(tw.p)) != 0 || !offersEvery(tw.p) {
		t.Errorf("with no ledger the menu holds %v (every ledger: %v), want his offer alone", offered(tw.p), offersEvery(tw.p))
	}
	tw.s.Queue().GetPending()

	book[44]["Freeport"], book[44]["West Plateau"], book[44]["Highpass Hold"] = true, true, true
	tw.talk(harrek)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Freeport", "West Plateau", everyLedgerOption}) {
		t.Fatalf("Harrek offered %q, want Freeport, West Plateau and his offer (Highpass Hold is signed, and no coach of a home's "+
			"runs there; Gerntar Mines' ledger is not signed)", got)
	}
	tw.pick(harrek, 0)
	if x, z, world, moved := teleported(tw.s); !moved || x != 25273.7 || z != 15719.1 || world != 0 {
		t.Errorf("the ride to Freeport went to (%.1f, %.1f) world %d (%v), want Ronks's spot in Tunaria", x, z, world, moved)
	}
	tw.s.Queue().GetPending()

	harrek = tw.at(2, 9001)
	tw.talk(harrek)
	tw.s.Queue().GetPending()
	tw.pick(harrek, 1)
	if x, z, world, moved := teleported(tw.s); !moved || x != 6947 || z != 3300 || world != 10 {
		t.Errorf("the ride to West Plateau went to (%.0f, %.0f) world %d (%v), want its coachman's spot in the Homelands", x, z, world, moved)
	}
}

// Freeport's coach runs to every home besides its own towns, for a traveller
// in their ledgers whose client holds the Homelands: told apart from the
// game's own Gerntar Mines by the world's name. One whose client does not
// hold the world is offered no home, signed or not, and a GM's neither: a
// client sent there would stay where it was. No other town's coach runs to a
// home: Highpass Hold's offers Freeport alone.
func TestFreeportsCoachRunsToTheHomesAClientHolds(t *testing.T) {
	tw, ronks, book := homesCoachWorld(t)
	book[44] = map[string]bool{"Freeport": true, "Highpass Hold": true, "Aviak Village": true, "West Plateau": true,
		homesMenuGerntar: true, "Gerntar Mines": true}
	tw.talk(ronks)
	want := []string{"Aviak Village", homesMenuGerntar, "Highpass Hold", "West Plateau"}
	if got := offered(tw.p); !slices.Equal(got, want) {
		t.Fatalf("Ronks offered %q, want %q (Tunaria's Gerntar Mines is no neighbour of Freeport's)", got, want)
	}
	if got := optionBox(t, tw.s); len(got) != 5 || got[4] != everyLedgerOption {
		t.Errorf("Ronks's page %q, want the four towns and his offer", got)
	}
	tw.pick(ronks, 0)
	if x, z, world, moved := teleported(tw.s); !moved || x != 9211.7 || z != 21547.1 || world != 10 {
		t.Errorf("the ride to Aviak Village went to (%.1f, %.1f) world %d (%v), want Harrek's spot in the Homelands", x, z, world, moved)
	}
	tw.s.Queue().GetPending()

	// a client that does not hold the Homelands: a retail disc's, a page's with no storage (back at Ronks's: the
	// ride took the entities it saw off its channels)
	ronks = tw.at(1, 416)
	tw.s.SetServerWorlds(nil)
	tw.talk(ronks)
	if got := offered(tw.p); !slices.Equal(got, []string{"Highpass Hold"}) {
		t.Errorf("a client that does not hold the Homelands was offered %q, want Highpass Hold alone", got)
	}
	tw.s.Queue().GetPending()
	tw.h.isGM = func(*session.Session) bool { return true }
	tw.talk(ronks)
	for _, town := range offered(tw.p) {
		if entity.IsCoachHome(town) {
			t.Errorf("a GM whose client does not hold the Homelands was offered %q", town)
		}
	}
	tw.s.Queue().GetPending()
	// a July S2's table has every slot and nothing in the server's own: no home either
	tw.h.isGM = func(*session.Session) bool { return false }
	tw.s.SetDiscVersion(protocol.GameVersionSandstormS2)
	tw.talk(ronks)
	if got := offered(tw.p); !slices.Equal(got, []string{"Highpass Hold"}) {
		t.Errorf("a July S2 client that built nothing was offered %q, want Highpass Hold alone", got)
	}
	tw.s.Queue().GetPending()
	tw.s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
	tw.s.SetServerWorlds([]uint8{10})

	// the GM whose client holds it: every coach town of Tunaria, and the homes
	tw.h.isGM = func(*session.Session) bool { return true }
	tw.talk(ronks)
	got := offered(tw.p)
	for _, home := range []string{"Aviak Village", homesMenuGerntar, "West Plateau"} {
		if !slices.Contains(got, home) {
			t.Errorf("the GM was offered %q, without %s", got, home)
		}
	}
	tw.s.Queue().GetPending()
	tw.h.isGM = func(*session.Session) bool { return false }

	quick := tw.add(3, tw.w.NPCs[415])
	tw.w.UpdatePosition(tw.p.ServerID, 16781, 188, 15353, 0)
	tw.talk(quick)
	if got := offered(tw.p); !slices.Equal(got, []string{"Freeport"}) {
		t.Errorf("Highpass Hold's coach offered %q, want Freeport alone: no town but Freeport runs to a home", got)
	}
}

// The offer to sign every ledger at once takes the homes' in: Yes signs
// them with the towns', and the menu then lists them. A coachman standing in a
// zone of the server's world that is no race's home runs no coach, signs
// nothing and says nothing.
func TestEveryLedgerTakesTheHomesIn(t *testing.T) {
	tw, ronks, book := homesCoachWorld(t)
	tw.talk(ronks)
	tw.s.Queue().GetPending()
	if !offersEvery(tw.p) {
		t.Fatal("Ronks made no offer to sign every ledger")
	}
	tw.pick(ronks, 0)
	id := boxID(questMsgs(tw.s))
	tw.pick(id, 0)
	for _, town := range []string{"Aviak Village", "West Plateau", homesMenuGerntar, "Gerntar Mines", "Halas"} {
		if !book[44][town] {
			t.Errorf("every ledger signed, and %s's is not: %v", town, len(book[44]))
		}
	}
	if len(book[44]) != len(entity.CoachTownNames()) {
		t.Errorf("%d ledgers signed of %d coach towns", len(book[44]), len(entity.CoachTownNames()))
	}
	if got := offered(tw.p); !slices.Contains(got, "Aviak Village") || !slices.Contains(got, "West Plateau") {
		t.Errorf("his menu again offers %q, want the homes among them", got)
	}
	if offersEvery(tw.p) {
		t.Error("every ledger signed, and he offers to sign them again")
	}
	tw.s.Queue().GetPending()

	lost := tw.at(4, 9004)
	before := len(book[44])
	tw.p.PendingCoachDests = nil
	tw.talk(lost)
	if got := opcodes(tw.s); len(got) != 0 || len(tw.p.PendingCoachDests) != 0 || len(book[44]) != before {
		t.Errorf("a coachman of no home answered %v (menu %v, %d ledgers, were %d)", got, offered(tw.p), len(book[44]), before)
	}
}

// The homes are read from the server's worlds as built: a zone a recipe gives
// a race, by its name and its ground; a world with no built zones (a test's)
// has none, and taking the worlds away takes the stables.
func TestTheHomesAreTheServersWorldsZones(t *testing.T) {
	SetServerWorlds([]worlds.Built{homesBuilt(), {Recipe: worlds.Recipe{World: 11, Name: "the Canyon of Banners",
		Zones: []worlds.RecipeZone{{Name: "Box Canyons"}}}}})
	t.Cleanup(func() { SetServerWorlds(nil) })
	want := []string{"Aviak Village", homesMenuGerntar, "West Plateau"}
	if got := entity.CoachHomeNames(); !slices.Equal(got, want) {
		t.Errorf("the homes' stables %q, want %q", got, want)
	}
	SetServerWorlds(nil)
	if got := entity.CoachHomeNames(); len(got) != 0 {
		t.Errorf("with no server worlds the homes' stables are %q", got)
	}
}

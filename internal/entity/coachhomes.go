package entity

import (
	"log"
	"sort"
	"sync/atomic"
)

// coachhomes.go — the stables of the races' homes (the owner, 2026-11-10:
// "the coach will connect to freeport and the other homeland areas").
//
// A race's home is a zone of a world the server made of the disc's own zones
// (the Homelands: handlers/serverworlds.go, tools/worlds/homelands.json), and
// each has a coachman (tools/seed_race_homes.py places him; a new character's
// second quest has it sign his ledger, as every city's does). His stable is a
// stable like any other, on a network of its own beside the live game's
// (coachnetwork.go):
//
//   - his town is his home: the zone he stands in, by the name its recipe
//     gives it ("Aviak Village"), read from the built recipe the server
//     loaded (SetCoachHomes), never from a table here. Where a coach town of
//     the game bears the name already (the minotaurs' Gerntar Mines is a
//     zone of Tunaria with a stable of its own), the world's name is added
//     ("Gerntar Mines (the Homelands)"): a ledger and a menu line name one
//     stable;
//   - a home's coach runs to Freeport (CoachHomeHub) and to every other
//     home, and Freeport's runs to every home besides the towns it ran to. No
//     other town gains a route;
//   - the stables' rules are the game's (handlers/coachledger.go): talking to
//     a home's coachman signs its ledger, a ride is sold only to a town whose
//     ledger the traveller has signed, and the offer to sign every ledger at
//     once takes the homes' in. A home is offered only to a client that holds
//     its world (the handler's holdsWorld), and a rider lands on the
//     coachman's own spot, as on every coach.
//
// The gatekeepers of the homes stay off every network (Composed, world.go):
// a home's flight to its race's city is its quests' reward, one way.

// CoachHomeHub is the town of the game's the homes' coaches run to, and whose
// coach runs to them.
const CoachHomeHub = "Freeport"

// CoachHome is a race's home as the server's world has it: the zone a
// stable's coachman stands in.
type CoachHome struct {
	World     uint8
	WorldName string // the world's name, as its recipe has it ("the Homelands")
	Zone      string // the zone's name, as its recipe has it
	// the zone's ground, x and z
	MinX, MinZ, MaxX, MaxZ float32
}

// coachHome is a home with its stable's town (its zone's name, made its own).
type coachHome struct {
	CoachHome
	town string
}

type coachHomeSet struct {
	homes  []coachHome
	byTown map[string]*coachHome
}

var coachHomes atomic.Pointer[coachHomeSet]

// SetCoachHomes makes homes the races' homes that have a stable (the server's
// own worlds' zones that are a race's home: handlers.SetServerWorlds); none
// takes them all away. A home whose name a coach town of the game or an
// earlier home bears is named with its world's too; one still not its own is
// left out, and said.
func SetCoachHomes(homes []CoachHome) {
	set := &coachHomeSet{byTown: map[string]*coachHome{}}
	game := map[string]bool{}
	for _, t := range gameCoachTowns() {
		game[t] = true
	}
	taken := map[string]bool{}
	for _, h := range homes {
		if h.Zone == "" {
			continue
		}
		town := h.Zone
		if game[town] || taken[town] {
			town = h.Zone + " (" + h.WorldName + ")"
		}
		if game[town] || taken[town] || h.WorldName == "" && town != h.Zone {
			log.Printf("Coach: the home %q of world %d has no name of its own among the stables: no coach runs to it", h.Zone, h.World)
			continue
		}
		taken[town] = true
		set.homes = append(set.homes, coachHome{CoachHome: h, town: town})
	}
	for i := range set.homes {
		set.byTown[set.homes[i].town] = &set.homes[i]
	}
	coachHomes.Store(set)
}

func loadCoachHomes() *coachHomeSet {
	if s := coachHomes.Load(); s != nil {
		return s
	}
	return &coachHomeSet{}
}

// coachHomeAt is the stable's town of the home (x, z) of world lies in.
func coachHomeAt(world uint8, x, z float32) (string, bool) {
	for _, h := range loadCoachHomes().homes {
		if h.World == world && x >= h.MinX && x < h.MaxX && z >= h.MinZ && z < h.MaxZ {
			return h.town, true
		}
	}
	return "", false
}

// IsCoachHome reports a stable's town that is a race's home.
func IsCoachHome(town string) bool {
	return loadCoachHomes().byTown[town] != nil
}

// CoachHomeWorld is the name of the world a home's stable stands in ("the
// Homelands"): what a coachman calls the homes together.
func CoachHomeWorld(town string) (string, bool) {
	if h := loadCoachHomes().byTown[town]; h != nil {
		return h.WorldName, true
	}
	return "", false
}

// CoachHomeNames are the homes' stables' towns, sorted.
func CoachHomeNames() []string {
	set := loadCoachHomes()
	out := make([]string, 0, len(set.homes))
	for _, h := range set.homes {
		out = append(out, h.town)
	}
	sort.Strings(out)
	return out
}

// coachHomesRun is whether a coach runs between a and b by the homes'
// network: home to home, and each home to and from the hub.
func coachHomesRun(a, b string) bool {
	if a == b {
		return false
	}
	ha, hb := IsCoachHome(a), IsCoachHome(b)
	return ha && hb || ha && b == CoachHomeHub || a == CoachHomeHub && hb
}

// gameCoachTowns are the coach towns of the game's own: the coachmen's
// (coachLocationNames, coachTowns) and the live network's.
func gameCoachTowns() []string {
	seen := map[string]bool{}
	for _, t := range coachLocationNames {
		seen[t] = true
	}
	for _, t := range coachTowns {
		seen[t.name] = true
	}
	for t := range coachNetwork {
		seen[t] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

package entity

// coachtrips.go: the coaches as ways for the quests' marks (the owner,
// 2026-11-25, Heirloom petition 3014: "the mini map and quest finder should
// direct you to the coachman"). handlers/questcoachway.go makes each stop of
// a coachman's menu a trip of the way a step's mark is found by.

// CoachTrip is a coachman's stable as a way: where he stands and the stops
// his menu lists (handlers' coachDests, before a player's ledgers and the
// worlds its client holds are asked).
type CoachTrip struct {
	From NPCSpot
	// Dests are his stops: on a network, the towns his coach runs to
	// (CoachMenu); else the other coachmen of his world
	Dests []CoachDest
	// Ledger is a network's stable: a stop is sold only to a traveller who
	// has signed its ledger (the stop's Name)
	Ledger bool
}

// CoachTrips is every coachman's trips: one pass over the NPCs under the
// lock, his stops made of the others' (coachDestLocked), as his menu makes
// them (GetCoachDestinations, GetCoachDestinationsEveryWorld, CoachMenu). A
// coachman of a server's world that names no town runs none.
func (w *World) CoachTrips() []CoachTrip {
	type man struct {
		spot  NPCSpot
		name  string
		town  string
		named bool
		stop  CoachDest
	}
	var men []man
	w.mu.RLock()
	for _, n := range w.NPCs {
		if !IsCoachman(n.Name) || n.Temporary || n.Owner != 0 {
			continue
		}
		town, named := coachLabel(n)
		men = append(men, man{
			spot: NPCSpot{ID: n.ServerID, World: n.World, X: n.SpawnX, Z: n.SpawnZ, Name: n.Name},
			name: n.Name, town: town, named: named, stop: w.coachDestLocked(n),
		})
	}
	w.mu.RUnlock()
	var out []CoachTrip
	for i, m := range men {
		if Composed(m.spot.World) && !m.named {
			continue
		}
		var every, same []CoachDest
		for j, o := range men {
			if j == i || o.name == m.name {
				continue
			}
			every = append(every, o.stop)
			if o.spot.World == m.spot.World {
				same = append(same, o.stop)
			}
		}
		sortDests(every)
		sortDests(same)
		runs, routed := CoachMenu(m.town, m.named, every)
		t := CoachTrip{From: m.spot, Dests: same, Ledger: routed}
		if routed {
			t.Dests = runs
		}
		if len(t.Dests) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// CoachHomeAt is the town of the race's home (x, z) of world lies in, as its
// stable names it (coachhomes.go); false outside every home.
func CoachHomeAt(world uint8, x, z float32) (string, bool) {
	return coachHomeAt(world, x, z)
}

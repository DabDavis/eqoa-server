package entity

import "sort"

// The coach network and the stables' ledgers (the user, 2026-10-30: "our
// coach ledger lets any player coach anywhere, lets make it unlocked by
// talking to the coachman and also only coach to connecting"). In the live
// game a coach ran only between neighbouring stables, and a stable sold a
// ride only to a town whose ledger the traveller had signed, as Sandstorm's
// Coachman Ronks tells a new character (questrec16): "if you find yourself in
// a nearby city, you can visit the stable there and pay for a ride to
// Freeport. Without that record, they cannot offer you passage." Every
// class's level 2 quest sends a new character to sign its city's ledger
// (EQOA-Data's guides: "sign the ledger", "he signs you in").
//
// The network is liveCoachRoutes (EQOA-Data's coach map) read both ways: a
// coach runs between two towns when either lists the other, as Ronks's
// nearby cities sell a ride to Freeport, whose coach runs to them. EQOA-Data
// gives Neriak, Bobble-by-Water and Hazinak (Evil) no list of their own
// (their coaches are only other towns' destinations there) and lists two
// routes one way only (Fayspire to Rivervale, Oggok to Dark Solace). The
// ledgers are the game's to keep (handlers/coachledger.go, table
// charcoaches).

// coachNetwork is liveCoachRoutes both ways: each town, the towns its coach
// runs to.
var coachNetwork = func() map[string]map[string]bool {
	net := map[string]map[string]bool{}
	join := func(a, b string) {
		if net[a] == nil {
			net[a] = map[string]bool{}
		}
		net[a][b] = true
	}
	for town, dests := range liveCoachRoutes {
		for _, d := range dests {
			if d == town {
				continue
			}
			join(town, d)
			join(d, town)
		}
	}
	return net
}()

// CoachRunsTo is whether a coach runs between towns a and b: the live
// game's ran either way between them.
func CoachRunsTo(a, b string) bool {
	return coachNetwork[a][b]
}

// OnCoachNetwork is whether the town has a stable on the live game's coach
// network. A coachman of a town that has one runs only to its neighbours
// there; any other offers every coach town of his world, as all did before.
func OnCoachNetwork(town string) bool {
	return len(coachNetwork[town]) > 0
}

// CoachTown is the town a coachman serves, as the coach menus list it, and
// whether it is a town (coachLabel: else it is his own name).
func CoachTown(npc *NPC) (string, bool) {
	return coachLabel(npc)
}

// CoachMenu is what a coachman of town (named: a town's, CoachTown) may
// offer of the other coachmen's stops, all (GetCoachDestinations): on the
// network, the ones his coach runs to, routed true; else all of them.
func CoachMenu(town string, named bool, all []CoachDest) (runs []CoachDest, routed bool) {
	if !named || !OnCoachNetwork(town) {
		return all, false
	}
	for _, d := range all {
		if CoachRunsTo(town, d.Name) {
			runs = append(runs, d)
		}
	}
	return runs, true
}

// CoachTownNames are every coach town's name, sorted: the coachmen's towns
// (coachLocationNames, coachTowns) and the network's. A ledger names one.
func CoachTownNames() []string {
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

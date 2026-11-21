package entity

import "sort"

// coachstables.go — stables of a server's world that are no race's home: a
// place inside a zone of the world with a stable and a coach of its own, to
// the towns it names (the owner, 2026-11-21: "make the coach go to moradhim
// for the sk/nec dwarf homeland").
//
// Blackvein Deep, the evil dwarves' start (tools/evil_dwarf_start_20261121.py),
// is the cave under Thedruk in world 10, whose zone is the orcs' home: a
// coachman standing in it would be the orcs' stable (coachHomeAt, by zone).
// One standing in the cave's tunnels is Blackvein Deep's instead, whatever he
// is called, and his coach runs to Moradhim, and Moradhim's to him; to no other
// town, and no home's coach or Freeport's runs to him. A stable of a world the
// server has not made is not there at all (Composed): no town of that name, no
// route, no ledger. As every stable of a server's world, it is offered only to
// a client that holds the world (handlers.holdsWorld): a PS2 never sees it in
// Moradhim's menu.

// serverStable is a stable in a box of a server's world (x and z), and the
// towns its coach runs to, both ways.
type serverStable struct {
	world                  uint8
	town                   string
	minX, minZ, maxX, maxZ float32
	runs                   []string
}

// serverStables are the server's worlds' stables that are no home's. Blackvein
// Deep's box holds the cave's tunnels (18500-19080, 8890-9310) and stops short
// of the orcs' camp (its coachman at 19804, 8850; its rats from 19436).
var serverStables = []serverStable{
	{world: 10, town: "Blackvein Deep", minX: 18450, minZ: 8850, maxX: 19150, maxZ: 9350, runs: []string{"Moradhim"}},
}

// serverStableAt is the stable's town whose box (x, z) of world lies in, where
// the server made that world.
func serverStableAt(world uint8, x, z float32) (string, bool) {
	for _, s := range serverStables {
		if s.world == world && Composed(world) && x >= s.minX && x < s.maxX && z >= s.minZ && z < s.maxZ {
			return s.town, true
		}
	}
	return "", false
}

// isServerStable reports a town that is a stable of a world the server made.
func isServerStable(town string) bool {
	for _, s := range serverStables {
		if s.town == town && Composed(s.world) {
			return true
		}
	}
	return false
}

// serverStablesRun is whether a coach runs between a and b by a server's
// world's stable: the stable to a town it names, and back.
func serverStablesRun(a, b string) bool {
	for _, s := range serverStables {
		if !Composed(s.world) {
			continue
		}
		for _, r := range s.runs {
			if a == s.town && b == r || b == s.town && a == r {
				return true
			}
		}
	}
	return false
}

// serverStableNames are the towns of the stables of the worlds the server
// made, sorted: each a ledger (CoachTownNames).
func serverStableNames() []string {
	var out []string
	for _, s := range serverStables {
		if Composed(s.world) {
			out = append(out, s.town)
		}
	}
	sort.Strings(out)
	return out
}

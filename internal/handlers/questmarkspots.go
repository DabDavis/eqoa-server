package handlers

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Where a step's mark goes (the owner, 2026-11-14: "also lets update quest
// helper to be more accurate"; questmarks.go sends the marks, questheadmarks.go
// the "?" over a head). For a player in world at (x, z):
//
//   - a talk, show or give step: its NPC (a pinned spawn alone), the copy a
//     step's spawn event made for this character where it stands now, else
//     the nearest of the name's spawns that stand in the world, else (none
//     standing) the originals a step's spawns come again by (spawnonly.go);
//   - a kill or loot step: the character's own spawned copies of its
//     creatures (the step's Target and Also) where they stand, else their
//     nearest CAMPS, never one stray: the spawns of every name the step counts
//     within huntCampLink of one another are a camp, and a camp of fewer than
//     huntCampFew (or the world's biggest, if smaller) counts only where the
//     world has no bigger one. A mark is at a camp's spawn nearest its middle
//     (a real creature's place). None in the player's world: the biggest
//     elsewhere. A pinned spawn is marked besides;
//   - a reach step: its place.
//
// The marks are the nearest maxMarkSpots, the player's world first. And where
// none is in the player's world, a "way" mark goes at the nearest NPC whose
// trip (waysender.go's wayRoutes: a waysender, a dockmaster, a gate, a portal)
// leads toward the step's world (the fewest trips, the nearest first), so the
// map of the world the player stands in says where to go.

// huntCampLink is how near one another a creature's spawns are one camp.
const huntCampLink = 120

// huntCampFew is the size of camp a lone spawn or a pair is passed over for.
const huntCampFew = 3

// huntPlaceReach is how near a hunt step's own place (queststeps x, z) a camp
// must stand to be marked for it.
const huntPlaceReach = 1500

// huntCamp is a camp of a name's spawns: the spawn nearest its middle, and how
// many it has.
type huntCamp struct {
	at entity.NPCSpot
	n  int
}

// campsOf is a name's spawns as camps (spawn-only originals with the rest),
// biggest first then by the spawn's id.
func campsOf(spots []entity.NPCSpot) []huntCamp {
	n := len(spots)
	if n == 0 {
		return nil
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	type cell struct {
		w    uint8
		x, z int
	}
	grid := map[cell][]int{}
	cellOf := func(s entity.NPCSpot) cell {
		return cell{s.World, int(math.Floor(float64(s.X) / huntCampLink)), int(math.Floor(float64(s.Z) / huntCampLink))}
	}
	for i, s := range spots {
		c := cellOf(s)
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				for _, j := range grid[cell{c.w, c.x + dx, c.z + dz}] {
					o := spots[j]
					if math.Hypot(float64(o.X-s.X), float64(o.Z-s.Z)) <= huntCampLink {
						parent[root(i)] = root(j)
					}
				}
			}
		}
		grid[c] = append(grid[c], i)
	}
	members := map[int][]int{}
	for i := range spots {
		members[root(i)] = append(members[root(i)], i)
	}
	out := make([]huntCamp, 0, len(members))
	for _, m := range members {
		var cx, cz float64
		for _, i := range m {
			cx += float64(spots[i].X)
			cz += float64(spots[i].Z)
		}
		cx /= float64(len(m))
		cz /= float64(len(m))
		best := m[0]
		bd := math.MaxFloat64
		for _, i := range m {
			d := math.Hypot(float64(spots[i].X)-cx, float64(spots[i].Z)-cz)
			if d < bd || d == bd && spots[i].ID < spots[best].ID {
				best, bd = i, d
			}
		}
		out = append(out, huntCamp{at: spots[best], n: len(m)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].at.ID < out[j].at.ID
	})
	return out
}

// spotDist is a spot's distance from (x, z) in world, -1 in another.
func spotDist(s entity.NPCSpot, world uint8, x, z float32) float32 {
	if s.World != world {
		return -1
	}
	return float32(math.Hypot(float64(s.X-x), float64(s.Z-z)))
}

// nearestFirst orders spots the player's world first, nearest first (another
// world's in the order given).
func nearestFirst(spots []entity.NPCSpot, world uint8, x, z float32) {
	sort.SliceStable(spots, func(i, j int) bool {
		di, dj := spotDist(spots[i], world, x, z), spotDist(spots[j], world, x, z)
		if (di < 0) != (dj < 0) {
			return di >= 0
		}
		return di < dj
	})
}

// campsLocked is the camps (campsOf) of the spawns of names together, made
// once a read of the spots. Must be called with q.mu held.
func (q *QuestEngine) campsLocked(spots map[string][]entity.NPCSpot, names []string) []huntCamp {
	keys := make([]string, 0, len(names))
	for _, n := range names {
		keys = append(keys, foldName(n))
	}
	sort.Strings(keys)
	k := strings.Join(keys, "|")
	if q.camps == nil {
		q.camps = map[string][]huntCamp{}
	}
	if c, ok := q.camps[k]; ok {
		return c
	}
	var all []entity.NPCSpot
	seen := map[string]bool{}
	for _, n := range keys {
		if !seen[n] {
			seen[n] = true
			all = append(all, spots[n]...)
		}
	}
	c := campsOf(all)
	q.camps[k] = c
	return c
}

// copiesLocked is where the character's own spawned copies (a step's spawn
// event's) of the names stand now. Must be called with q.mu held.
func (q *QuestEngine) copiesLocked(serverID uint32, names []string) []entity.NPCSpot {
	if q.gw == nil || len(q.spawns[serverID]) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[foldName(n)] = true
	}
	var out []entity.NPCSpot
	for _, s := range q.spawns[serverID] {
		if !want[foldName(s.npc)] {
			continue
		}
		if w, x, _, z, ok := q.gw.NPCSpot(s.id); ok {
			out = append(out, entity.NPCSpot{ID: s.id, World: w, X: x, Z: z, Name: s.npc})
		}
	}
	return out
}

// stepSpotsLocked is where a step's marks go for a player (character
// serverID, 0 for none) in world at (x, z): its kind of mark and the spots,
// at most maxMarkSpots, the player's world first, nearest first. Must be
// called with q.mu held.
func (q *QuestEngine) stepSpotsLocked(spots map[string][]entity.NPCSpot, st *database.QuestStep, serverID uint32, world uint8, x, z float32) (byte, []entity.NPCSpot) {
	var out []entity.NPCSpot
	kind := markStep
	switch {
	case database.AtNPC(st.Kind):
		out = q.copiesLocked(serverID, []string{st.Target})
		if len(out) == 0 {
			var standing, orig []entity.NPCSpot
			for _, s := range spots[foldName(st.Target)] {
				if st.NPCID != 0 && int(s.ID) != st.NPCID {
					continue
				}
				if s.SpawnOnly {
					orig = append(orig, s)
				} else {
					standing = append(standing, s)
				}
			}
			out = standing
			if len(out) == 0 {
				out = orig
			}
		}
	case database.Hunts(st.Kind):
		kind = markHunt
		names := st.Targets()
		out = q.copiesLocked(serverID, names)
		if len(out) > 0 {
			break
		}
		if st.NPCID != 0 {
			// a pinned spawn: that one alone, the others counted anywhere
			for _, s := range spots[foldName(st.Target)] {
				if int(s.ID) == st.NPCID {
					out = append(out, s)
				}
			}
			names = names[1:]
		}
		// the step's creatures of every name it counts are one kind: their
		// camps, the nearest that count in the player's world (at least
		// huntCampFew, or its biggest where none is that big)
		camps := q.campsLocked(spots, names)
		if len(camps) == 0 {
			break
		}
		// a step with a place of its own ("the giant widows of central
		// Collonridge Cemetery": queststeps x, z, world): the camps of any
		// size within huntPlaceReach of it, before the camps elsewhere
		// (petition 5099/5104, Lostinpot: the mark went to Sslathis's six
		// widows, 20,000 off, for the cemetery's scattered ones)
		if st.X != 0 || st.Z != 0 {
			var at []entity.NPCSpot
			for _, c := range camps {
				if int(c.at.World) == st.World && math.Hypot(float64(c.at.X-st.X), float64(c.at.Z-st.Z)) <= huntPlaceReach {
					at = append(at, c.at)
				}
			}
			if len(at) > 0 {
				out = append(out, at...)
				break
			}
		}
		big := 0
		for _, c := range camps {
			if c.at.World == world && c.n > big {
				big = c.n
			}
		}
		var near []entity.NPCSpot
		for _, c := range camps {
			if c.at.World == world && c.n >= min(huntCampFew, big) {
				near = append(near, c.at)
			}
		}
		if len(near) == 0 {
			near = []entity.NPCSpot{camps[0].at} // another world's biggest
		}
		out = append(out, near...)
	case st.Kind == database.StepReach && (st.X != 0 || st.Z != 0) && st.World >= 0 && st.World < 256:
		return markStep, []entity.NPCSpot{{World: uint8(st.World), X: st.X, Z: st.Z, Name: st.Journal}}
	default:
		return markStep, nil
	}
	nearestFirst(out, world, x, z)
	// one mark a place
	var kept []entity.NPCSpot
	for _, s := range out {
		dup := false
		for _, k := range kept {
			if k.World == s.World && math.Hypot(float64(k.X-s.X), float64(k.Z-s.Z)) < 1 {
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, s)
		}
	}
	if len(kept) > maxMarkSpots {
		kept = kept[:maxMarkSpots]
	}
	return kind, kept
}

// wayHop is one trip of a travel NPC: where the NPC stands, the world it
// lands in and the place's name.
type wayHop struct {
	from entity.NPCSpot
	to   uint8
	name string
	// questOnly is a trip only a quest opens (the Plane of Sky's): a way
	// only where no open one leads
	questOnly bool
}

// wayHopsLocked is every trip of wayRoutes whose NPC stands in the world,
// read with the spots (a vampire's coffin left out: not a way for anyone).
// Must be called with q.mu held.
func (q *QuestEngine) wayHopsLocked(spots map[string][]entity.NPCSpot) []wayHop {
	if q.hops != nil {
		return q.hops
	}
	q.hops = []wayHop{}
	names := make([]string, 0, len(wayRoutes))
	for n := range wayRoutes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, r := range wayRoutes[n] {
			if r.Vampire {
				continue
			}
			to := -1
			if r.At != nil {
				to = int(r.At.World)
			} else if a := spots[foldName(r.Arrive)]; len(a) > 0 {
				to = int(a[0].World)
			}
			if to < 0 {
				continue
			}
			for _, s := range spots[foldName(n)] {
				if !s.SpawnOnly && int(s.World) != to {
					q.hops = append(q.hops, wayHop{from: s, to: uint8(to), name: r.To, questOnly: r.QuestOnly})
				}
			}
		}
	}
	return q.hops
}

// wayToLocked is the travel NPC nearest (x, z) in world whose trip starts the
// shortest way (the fewest trips) to world to, and the place it goes, by the
// trips open to anyone, else those a quest opens too; false where no trips
// lead there. Must be called with q.mu held.
func (q *QuestEngine) wayToLocked(spots map[string][]entity.NPCSpot, world, to uint8, x, z float32) (wayHop, bool) {
	var open []wayHop
	for _, h := range q.wayHopsLocked(spots) {
		if !h.questOnly {
			open = append(open, h)
		}
	}
	if h, ok := wayBy(open, world, to, x, z); ok {
		return h, true
	}
	return wayBy(q.wayHopsLocked(spots), world, to, x, z)
}

// wayBy is wayToLocked over the trips hops.
func wayBy(hops []wayHop, world, to uint8, x, z float32) (wayHop, bool) {
	// the trips each world is from the target: a breadth-first walk back
	far := map[uint8]int{to: 0}
	for changed := true; changed; {
		changed = false
		for _, h := range hops {
			if d, ok := far[h.to]; ok {
				if e, seen := far[h.from.World]; !seen || d+1 < e {
					far[h.from.World] = d + 1
					changed = true
				}
			}
		}
	}
	if _, ok := far[world]; !ok || world == to {
		return wayHop{}, false
	}
	var best wayHop
	found := false
	for _, h := range hops {
		if h.from.World != world {
			continue
		}
		d, ok := far[h.to]
		if !ok || d+1 != far[world] {
			continue
		}
		if !found || spotDist(h.from, world, x, z) < spotDist(best.from, world, x, z) {
			best, found = h, true
		}
	}
	return best, found
}

// wayLine is a way mark's detail: the trip and where it leads.
func wayLine(h wayHop, step string) string {
	s := fmt.Sprintf("Travel with %s to %s", strings.TrimSpace(h.from.Name), h.name)
	if step = strings.TrimSpace(step); step != "" {
		s += ", for: " + step
	}
	return s
}

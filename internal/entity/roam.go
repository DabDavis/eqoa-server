package entity

import (
	"math"
	"math/rand"
	"time"
)

// Creatures wander round their spawn (F3 20261025-044257: "Wildlife does not
// roam around") as Sandstorm's do (the user, 2026-10-31, choosing
// "Sandstorm's everywhere" for the range once the census of Rathe and Odus
// had measured it): within the roam radius (world.roam_radius, 70; it was 225
// from F3 20261029-024920, jib's "by triple", 75 from F3 20261025-164404, 15
// before), in legs of Sandstorm's lengths (roamLegs) in any direction from
// where it stands, inside the radius, walked at Sandstorm's walking pace
// (npcmove.go), each followed by Sandstorm's rest of 4.1 to 4.6 s, so a
// creature is on its feet about half the time, as Sandstorm's are.
//
// The census (cmd/sandroute's flights, tools/seed_sandstorm_census.py; the
// harvester tracked every creature in sight every half second, its velocity
// too) saw 82% of Odus's and 86% of Rathe's creatures watched 20 s or more
// walk, at 4.92 a second; Sandstorm sends a walker's place only at its stops
// and leg starts, so a leg is the distance between the rests either side of
// it: 11,646 of them, median 19, 90% under 40, none past 73; its 12,120
// rests 4.09 to 4.64 s (5% to 95%), no short pause among them; and over five
// minutes a creature's places kept within 47 of their middle nine times in
// ten, 66 ninety-nine times in a hundred. (It was legs of 10 to 75, rests of
// 2 to 12 s or, three times in five, a pause of half a second to two: on its
// feet 70% of the time, which jib asked for.) A leg never ends in a town nor
// passes through one (towns.go: "they should generally not path near or
// into cities"), unless the creature lives there. Sandstorm's town guards,
// sentries and townsfolk stand.
//
// What wanders (movement.go's movementLocked): wildlife (wildlife.go: an
// animal by its name's head noun, not undead), and any creature table
// npcmove names a roamer (roam 1: the census's kinds Sandstorm showed
// walking, whatever they are, tools/seed_sandstorm_census.py --move), on a body of its own (a model, and not
// one of the players' races, charactermodel: those are the guards, the
// merchants and the townsfolk, which a roamer's row moves too), that can be
// fought (no 0x80 in npc_type: an NPC a player talks to stays put; a path
// walks it), not a training dummy. Anything else stands on its spawn until
// pulled and goes back there after (F3 20261026-042712, jib: "Not all mobs
// are wild life. Fire Giants are roaming currently when they should not be
// unless otherwise specified / designated as a roamer with a path"); until
// then every creature that was not on a player's body wandered. It stands
// while it fights, while a spell roots it, dead, and when no player is near
// (roamActiveRange): only those near someone move, so the tick is as cheap
// with 20,000 NPCs as with 200. A snare halves its pace.
//
// It walks on the ground (groundnpc.go), and only where it can stand, in or
// out of the water as it keeps (movement.go's legs, water.go); table
// npcmove stands some on their spawns, and one standing off the ground (a
// keep's floor, a bridge) does not wander.
//
// A creature of the land on the ground wanders between spots of its own
// (the user, 2026-11-04: "do the leg caching too", after the tick's profile:
// with players about, most of the wander's time was checking new legs,
// movement.go's legCheckLocked, and most of that the walls and CLIENT's
// set-down at every unit; claude-notes/server-tick-profile-20261103.md). It
// keeps up to wanderSpotsMax: its spawn, then the end of each leg it walks
// until it has them all, every leg until then picked as before. From then on
// a leg's length is drawn as ever (roamLeg) and it walks to the spot whose
// distance from where it stands is nearest that length, in whatever
// direction the spot lies (none nearer than roamMinStep nor farther than its
// radius), or to one it walked to from here before when that is nearly as
// near (spotKnownSlack: a creature keeps to the ways it knows). The way from
// one spot to another is checked whole the first time and remembered,
// walkable or not: a way it walked before needs only its floor read again
// (movement.go's legFloorsLocked), and one that failed is not tried again.
// The radius, the town check (made at every leg), the rests and the walk are
// as before; what changes is that its legs end on the same few places, as a
// creature keeps to the same few places of its ground. Measured over eight
// creatures' three hours each on open ground (TestAWandererWalksBetweenItsSpots
// holds one to it): legs of median 19.7, 90% under 40.2 (Sandstorm's 19 and
// 40.4; drawn anywhere, as every leg was before, they came out 16.9 and
// 36.1, a leg ending past the radius drawn again), on its feet half the
// time; a leg checked whole 56% of the time in its first ten minutes (its
// spots being made), 20% in the next ten, 8% to forty minutes, 5% to an
// hour, 1-2% after. One standing at none of its spots (a fight's end left it
// elsewhere) walks back to one, that way checked whole; one whose spots will
// not do takes a leg as before. Its spots are made again when what they were
// found for changes: its spawn, world, radius, lift over the terrain or what
// it keeps to, or the world's ground or water.

// DefaultRoamRadius is how far from its spawn a creature wanders: Sandstorm's
// (the file's comment), a little past the 66 its creatures kept to.
const DefaultRoamRadius float32 = 70

// The rest between walks (Sandstorm's, the file's comment), and the first
// rest of one that comes into a player's range (so a crowd does not set off
// at once).
const (
	roamRestMin   = 4100 * time.Millisecond
	roamRestMax   = 4600 * time.Millisecond
	roamFirstRest = 10 * time.Second
)

// roamLegs is Sandstorm's leg length at every 5% of the census's legs (the
// file's comment), 0% to 100%: a leg's length is drawn from it (roamLeg).
var roamLegs = [...]float32{0.5, 4.1, 6.1, 7.9, 9.7, 11.2, 12.7, 14.4, 16.0, 17.6, 19.2,
	20.8, 22.7, 24.7, 27.1, 29.6, 32.3, 35.9, 40.4, 47.5, 72.6}

// roamLeg is the leg length at u (0 to 1) of Sandstorm's (roamLegs, straight
// between its points).
func roamLeg(u float64) float32 {
	n := len(roamLegs) - 1
	f := float32(min(max(u, 0), 1) * float64(n))
	i := min(int(f), n-1)
	return roamLegs[i] + (roamLegs[i+1]-roamLegs[i])*(f-float32(i))
}

// A far roamer's legs (the owner, 2026-11-12: "skeletal giant and others are
// not roaming correctly, they only move a small distance"): a creature given a
// radius past the server's (npcmove's radius, npcwander's: the kinds that roam
// a canyon or a zone, not a camp's ground) walks legs as much longer as its
// radius is wider, up to roamLegScaleMax times Sandstorm's, so it crosses its
// ground rather than shuffling round its spawn: its spots (the file's comment)
// are the ends of those legs, as spread as they are. Sandstorm's lengths were
// measured on creatures keeping within 66 of their middle; at the server's
// radius and under it a leg is as ever.
const roamLegScaleMax float32 = 4

// roamLegScaleLocked is how much longer than Sandstorm's a leg is drawn for a
// creature wandering within r (the comment above): 1 up to the server's
// radius. Must be called with the lock held.
func (w *World) roamLegScaleLocked(r float32) float32 {
	base := w.roamRadius
	if base <= 0 {
		base = DefaultRoamRadius
	}
	if r <= base {
		return 1
	}
	return min(r/base, roamLegScaleMax)
}

// roamActiveRange is how near a player a creature must be to wander: the
// farthest any client can be shown it (handlers/entityslots.go's 1000).
const roamActiveRange float32 = 1000

// roamRefreshTicks is how often the creatures near players are found again.
const roamRefreshTicks = 10

// roamMinStep is the shortest walk a creature takes.
const roamMinStep float32 = 2

// wanderSpotsMax is how many spots a creature wanders between (the file's
// comment); spotFloorSame how near the floor it stands on must be a spot's
// for it to stand at that spot (a floor above or below it, a cave's under a
// hill, is another place). A spot whose way from here it walked before is
// taken over the spot nearest the drawn length while its length is off the
// draw by no more than the nearest's and max(spotSlackMin, spotKnownSlack ×
// the draw) more: a creature keeps to the ways it knows.
const (
	wanderSpotsMax         = 16
	spotFloorSame  float32 = 0.5
	spotKnownSlack float32 = 0.2
	spotSlackMin   float32 = 2
)

// wanderSpots is a creature's spots (the file's comment): spot i at (x[i],
// z[i]) on the floor y[i], spot 0 its spawn; bit j of tried[i] the way from
// spot i to spot j checked whole, and of good[i] found walkable (a uint32's
// bits: no more than 32 spots, which the line after the type holds it to).
type wanderSpots struct {
	key         wanderKey
	n           int
	x, y, z     [wanderSpotsMax]float32
	tried, good [wanderSpotsMax]uint32
}

var _ [32 - wanderSpotsMax]struct{} // more spots than a uint32 has bits does not build

// wanderKey is what a creature's spots were found for: the world's ground
// and water (World.groundGen), and the creature's world, spawn, radius, lift
// over the terrain and what it keeps to.
type wanderKey struct {
	gen            uint32
	world          uint8
	spawnX, spawnZ float32
	radius, lift   float32
	keeps          string
}

// add makes (x, z), on the floor y, a spot: its index, -1 when it has them
// all.
func (s *wanderSpots) add(x, y, z float32) int {
	if s.n == wanderSpotsMax {
		return -1
	}
	i := s.n
	s.x[i], s.y[i], s.z[i] = x, y, z
	s.n++
	return i
}

// at is the spot at (x, z) on the floor y, -1 for none. A leg's end puts a
// creature there exactly (stepNPCLocked).
func (s *wanderSpots) at(x, y, z float32) int {
	for i := 0; i < s.n; i++ {
		if s.x[i] == x && s.z[i] == z && s.y[i]-y < spotFloorSame && y-s.y[i] < spotFloorSame {
			return i
		}
	}
	return -1
}

// learn keeps what the way from spot i to spot j was found; nothing for a
// place that is no spot (-1).
func (s *wanderSpots) learn(i, j int, walkable bool) {
	if i < 0 || j < 0 {
		return
	}
	s.tried[i] |= 1 << j
	if walkable {
		s.good[i] |= 1 << j
	} else {
		s.good[i] &^= 1 << j
	}
}

// roamState is a creature's wander: walking to (toX, toZ), or resting until
// restUntil, and when it last moved.
type roamState struct {
	walking    bool
	toX, toZ   float32
	restUntil  time.Time
	last       time.Time
	nearPlayer bool

	// The leg it walks to (toX, toZ), as its check found it (movement.go's
	// legCheckLocked): where it began and how long it is, and the floor
	// every legSample along it, heights[0] the start's. legUsable when a step
	// may take its height from them (a creature of the land on the ground:
	// legY); else every step asks the ground, as before.
	legX, legZ, legLen float32
	heights            []float32
	legUsable          bool
}

// legOffLeg is how far off its leg a creature may be and still take its
// height from it (legY): a step stays on the line, so only something else's
// move of it (a fight, a GM) is more.
const legOffLeg float32 = 0.25

// legY is the floor under npc on the leg it walks, straight between the
// heights its check found (legSample apart, the walk's own floor at those
// points; a step's own ask of the ground is the same function from a height
// at most legSample before): false for no leg, one that cannot say, a
// swimmer, or a place off the leg (the step asks the ground then).
func (r *roamState) legY(npc *NPC) (float32, bool) {
	if r == nil || !r.legUsable || r.legLen <= 0 || npc.swimming || len(r.heights) < 2 {
		return 0, false
	}
	ux, uz := (r.toX-r.legX)/r.legLen, (r.toZ-r.legZ)/r.legLen
	rx, rz := npc.X-r.legX, npc.Z-r.legZ
	along, off := rx*ux+rz*uz, rx*uz-rz*ux
	if off > legOffLeg || off < -legOffLeg || along < -legOffLeg || along > r.legLen+legOffLeg {
		return 0, false
	}
	n := len(r.heights) - 1
	f := min(max(along/r.legLen, 0), 1) * float32(n)
	i := min(int(f), n-1)
	return r.heights[i] + (r.heights[i+1]-r.heights[i])*(f-float32(i)), true
}

// SetRoamRadius sets how far from its spawn a creature wanders (0 for no
// wandering), on every NPC already in the world too.
func (w *World) SetRoamRadius(r float32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r < 0 {
		r = 0
	}
	w.roamRadius = r
	for _, npc := range w.NPCs {
		npc.RoamRadius = r
	}
}

// SetPlayerModels names the players' race models (charactermodel), whose
// NPCs are townsfolk and do not wander.
func (w *World) SetPlayerModels(models map[uint32]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.playerModels = models
	w.towns = nil // its townsfolk changed (towns.go)
}

// wanders reports an NPC of the kind that wanders (the file's doc), alive
// and free to; never a pet, which follows its owner (pet.go), nor one that
// walks a GM's path (path.go). Must be called with the lock held.
func (w *World) wanders(npc *NPC) bool {
	ok, _, _ := w.movementLocked(npc) // movement.go
	return ok
}

// roamSystem walks the creatures near a player (the file's doc). Every
// tick; must be called with the lock held.
func (w *World) roamSystem(now time.Time) {
	if w.TickCount%roamRefreshTicks == 0 || w.roamers == nil {
		w.findRoamersLocked(now)
	}
	for _, npc := range w.roamers {
		w.roamStepLocked(npc, now)
		w.companionsFollowLocked(npc, now) // its own pets beside it (companion.go)
	}
}

// findRoamersLocked finds the wandering kind within roamActiveRange of a
// player in its world; one that left every player's range stops where it
// is. Must be called with the lock held.
func (w *World) findRoamersLocked(now time.Time) {
	w.linkCompanionsLocked() // the creatures' own pets, who walk with them and not of themselves (companion.go)
	found := make(map[EntityID]bool)
	var next []*NPC
	for _, p := range w.Players {
		for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, roamActiveRange) {
			npc, ok := w.NPCs[id]
			if !ok || found[id] || !w.npcSharesLocked(npc, p) || !w.wanders(npc) {
				continue
			}
			found[id] = true
			next = append(next, npc)
		}
	}
	for _, npc := range w.roamers {
		if !found[npc.ServerID] {
			npc.stopMoving()
			npc.roam = roamState{}
		}
	}
	for _, npc := range next {
		if !npc.roam.nearPlayer {
			npc.roam = roamState{nearPlayer: true, restUntil: now.Add(w.roamDuration(0, roamFirstRest)), last: now}
		}
	}
	if next == nil {
		next = []*NPC{}
	}
	w.roamers = next
}

// roamStepLocked is one tick of a creature's wander. Must be called with the
// lock held.
func (w *World) roamStepLocked(npc *NPC, now time.Time) {
	if w.NPCs[npc.ServerID] != npc {
		return // taken out of the world since the creatures were found
	}
	dt := float32(now.Sub(npc.roam.last).Seconds())
	npc.roam.last = now
	if dt > 0.5 {
		dt = 0.5 // a stalled tick does not jump it
	}
	if npc.CurrentEnemy != 0 || npc.chase.returning {
		// a fight, or the way home after one: its movement theirs once the
		// chase has it (chase.go), standing until then (a pet's fight is
		// not chased), and a rest before it wanders on
		if !npc.chase.on && !npc.chase.returning && (npc.roam.walking || npc.moving()) {
			npc.stopMoving()
		}
		npc.roam.walking = false
		if npc.roam.restUntil.Before(now) {
			npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
		}
		return
	}
	if !w.wanders(npc) || npc.DummyTarget != 0 || npc.rooted(now) {
		// a root, a dummy's fight or a death: standing, and a rest before it goes on
		if npc.roam.walking || npc.moving() {
			npc.stopMoving()
			npc.roam.walking = false
		}
		if npc.roam.restUntil.Before(now) {
			npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
		}
		return
	}
	if !npc.roam.walking {
		if now.Before(npc.roam.restUntil) {
			return
		}
		npc.roam.toX, npc.roam.toZ = w.roamPointLocked(npc)
		npc.roam.walking = true
	}
	speed := NPCWalkSpeed
	speed *= npc.snareFactor(now) // a snare's slow (workbook.go)
	if w.stepNPCOnLegLocked(npc, speed, dt) {
		npc.roam.walking = false
		npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
	}
}

// roamPointLocked is where a creature walks next: a leg of Sandstorm's
// lengths (roamLeg; at least roamMinStep, at most the radius) in any
// direction from where it stands, ending within its radius of its spawn,
// neither ending in a town nor passing through one unless its spawn is in
// that town's ground (towns.go), nor in or through a camp's but its own
// (campspawn.go), and standable all along (legOKLocked), to
// one of its spots once it has them all (the file's comment); where it
// stands when no such leg is found (it rests and tries again).
func (w *World) roamPointLocked(npc *NPC) (x, z float32) {
	npc.roam.legUsable = false
	r := w.roamRadiusLocked(npc)
	if r < roamMinStep {
		return npc.SpawnX, npc.SpawnZ
	}
	townsfolk := w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ)
	spots, from := w.spotsLocked(npc, r)
	if spots != nil && spots.n == wanderSpotsMax {
		if x, z, ok := w.spotLegLocked(npc, spots, from, r, townsfolk); ok {
			return x, z
		}
	}
	for i := 0; i < 16; i++ {
		d := min(max(roamLeg(w.roamRand().Float64())*w.roamLegScaleLocked(r), roamMinStep), r)
		a := w.roamRand().Float64() * 2 * math.Pi
		x = npc.X + d*float32(math.Sin(a))
		z = npc.Z + d*float32(math.Cos(a))
		if distXZ(x, z, npc.SpawnX, npc.SpawnZ) > r {
			continue
		}
		if !townsfolk && w.legInTownLocked(npc.World, npc.X, npc.Z, x, z) {
			continue
		}
		if w.legInCampLocked(npc, x, z) {
			continue // a camp's ground (campspawn.go)
		}
		heights := npc.roam.heights[:0]
		if !w.legCheckLocked(npc, x, z, &heights) {
			npc.roam.heights = heights[:0] // its room kept for the next try
			continue                       // somewhere it cannot stand, or into or out of the water (movement.go)
		}
		w.keepLegLocked(npc, x, z, heights)
		if spots != nil && len(heights) > 0 {
			// its end a spot while it has room for one, and the way there walkable
			spots.learn(from, spots.add(x, heights[len(heights)-1], z), true)
		}
		return x, z
	}
	return npc.X, npc.Z
}

// spotLegLocked is a leg to one of npc's spots (the file's comment), from
// the spot it stands at (from; -1 for none): up to 16 tries, each a length
// drawn as ever and the spot whose distance from here is nearest it (or one
// whose way from here it walked before, nearly as near: spotKnownSlack),
// none tried already, none nearer than roamMinStep or farther than its
// radius r, none whose way from here failed before; false when none will
// do. Must be called with the lock held.
func (w *World) spotLegLocked(npc *NPC, s *wanderSpots, from int, r float32, townsfolk bool) (x, z float32, ok bool) {
	var skip uint32
	if from >= 0 {
		skip = s.tried[from] &^ s.good[from] // its way there failed
	}
	for i := 0; i < 16; i++ {
		d := min(max(roamLeg(w.roamRand().Float64())*w.roamLegScaleLocked(r), roamMinStep), r)
		j, gap := -1, float32(0)
		jk, gapk := -1, float32(0)
		for k := 0; k < s.n; k++ {
			if skip&(1<<k) != 0 {
				continue
			}
			dk := distXZ(npc.X, npc.Z, s.x[k], s.z[k])
			if dk < roamMinStep || dk > r {
				skip |= 1 << k // where it stands, or as near; or past a leg's longest
				continue
			}
			g := float32(math.Abs(float64(dk - d)))
			if j < 0 || g < gap {
				j, gap = k, g
			}
			if from >= 0 && s.good[from]&(1<<k) != 0 && (jk < 0 || g < gapk) {
				jk, gapk = k, g
			}
		}
		if jk >= 0 && gapk <= gap+max(spotSlackMin, spotKnownSlack*d) {
			j = jk // a way it knows, as near the length as will do
		}
		if j < 0 {
			return 0, 0, false
		}
		skip |= 1 << j
		x, z = s.x[j], s.z[j]
		if !townsfolk && w.legInTownLocked(npc.World, npc.X, npc.Z, x, z) {
			continue // the towns are found again as their people change
		}
		if w.legInCampLocked(npc, x, z) {
			continue // a camp's ground, which a GM's /camp makes (campspawn.go)
		}
		heights := npc.roam.heights[:0]
		walkable := from >= 0 && s.good[from]&(1<<j) != 0 && w.legFloorsLocked(npc, x, z, &heights)
		if !walkable {
			heights = heights[:0]
			walkable = w.legCheckLocked(npc, x, z, &heights)
			s.learn(from, j, walkable)
		}
		if !walkable {
			npc.roam.heights = heights[:0]
			continue
		}
		w.keepLegLocked(npc, x, z, heights)
		return x, z, true
	}
	return 0, 0, false
}

// keepLegLocked keeps the leg npc walks to (x, z) for the walk, its floor
// read off heights at each step (legY). Must be called with the lock held.
func (w *World) keepLegLocked(npc *NPC, x, z float32, heights []float32) {
	npc.roam.heights = heights
	npc.roam.legX, npc.roam.legZ, npc.roam.legLen = npc.X, npc.Z, distXZ(npc.X, npc.Z, x, z)
	npc.roam.legUsable = len(heights) >= 2 && w.spawnOnGroundLocked(npc) &&
		(w.env == nil || w.keepsToLocked(npc) == keepsToLand)
}

// spotsLocked is npc's spots (the file's comment), made again when what they
// were found for changed, and the one it stands at (-1 for none); nil for a
// creature that keeps none: with no ground known, off the ground, or not of
// the land alone. Must be called with the lock held.
func (w *World) spotsLocked(npc *NPC, r float32) (*wanderSpots, int) {
	if w.ground == nil || !w.spawnOnGroundLocked(npc) {
		return nil, -1
	}
	keeps := keepsToLand
	if w.env != nil {
		if keeps = w.keepsToLocked(npc); keeps != keepsToLand {
			return nil, -1
		}
	}
	key := wanderKey{w.groundGen, npc.World, npc.SpawnX, npc.SpawnZ, r, npc.spawnLift, keeps}
	s := npc.spots
	if s == nil || s.key != key {
		s = &wanderSpots{key: key}
		if f, ok := w.ground.Floor(npc.World, npc.SpawnX, npc.SpawnY, npc.SpawnZ); ok {
			s.add(npc.SpawnX, f, npc.SpawnZ)
		}
		npc.spots = s
	}
	here, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z)
	if !ok {
		return s, -1
	}
	return s, s.at(npc.X, here, npc.Z)
}

// roamDuration is a time evenly between lo and hi.
func (w *World) roamDuration(lo, hi time.Duration) time.Duration {
	return lo + time.Duration(w.roamRand().Int63n(int64(hi-lo)+1))
}

// roamRand is the wander's own random source (tests seed it).
func (w *World) roamRand() *rand.Rand {
	if w.roamRnd == nil {
		w.roamRnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return w.roamRnd
}

// rooted reports a spell holding it where it stands.
func (n *NPC) rooted(now time.Time) bool {
	for _, e := range n.Effects {
		if e.Root && now.Before(e.Until) {
			return true
		}
	}
	return false
}

// snared reports a spell slowing it.
func (n *NPC) snared(now time.Time) bool {
	for _, e := range n.Effects {
		if e.Snare && now.Before(e.Until) {
			return true
		}
	}
	return false
}

// RoamRadius is how far from its spawn a creature wanders.
func (w *World) RoamRadius() float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.roamRadius
}

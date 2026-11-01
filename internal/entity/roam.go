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

// roamActiveRange is how near a player a creature must be to wander: the
// farthest any client can be shown it (handlers/entityslots.go's 1000).
const roamActiveRange float32 = 1000

// roamRefreshTicks is how often the creatures near players are found again.
const roamRefreshTicks = 10

// roamMinStep is the shortest walk a creature takes.
const roamMinStep float32 = 2

// roamState is a creature's wander: walking to (toX, toZ), or resting until
// restUntil, and when it last moved.
type roamState struct {
	walking    bool
	toX, toZ   float32
	restUntil  time.Time
	last       time.Time
	nearPlayer bool
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
	}
}

// findRoamersLocked finds the wandering kind within roamActiveRange of a
// player in its world; one that left every player's range stops where it
// is. Must be called with the lock held.
func (w *World) findRoamersLocked(now time.Time) {
	found := make(map[EntityID]bool)
	var next []*NPC
	for _, p := range w.Players {
		for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, roamActiveRange) {
			npc, ok := w.NPCs[id]
			if !ok || found[id] || npc.World != p.World || !w.wanders(npc) {
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
	if w.stepNPCToward(npc, npc.roam.toX, npc.roam.toZ, speed, dt) {
		npc.roam.walking = false
		npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
	}
}

// roamPointLocked is where a creature walks next: a leg of Sandstorm's
// lengths (roamLeg; at least roamMinStep, at most the radius) in any
// direction from where it stands, ending within its radius of its spawn,
// neither ending in a town nor passing through one unless its spawn is in
// that town's ground (towns.go), and standable all along (legOKLocked); where
// it stands when no such leg is found (it rests and tries again).
func (w *World) roamPointLocked(npc *NPC) (x, z float32) {
	r := w.roamRadiusLocked(npc)
	if r < roamMinStep {
		return npc.SpawnX, npc.SpawnZ
	}
	townsfolk := w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ)
	for i := 0; i < 16; i++ {
		d := min(max(roamLeg(w.roamRand().Float64()), roamMinStep), r)
		a := w.roamRand().Float64() * 2 * math.Pi
		x = npc.X + d*float32(math.Sin(a))
		z = npc.Z + d*float32(math.Cos(a))
		if distXZ(x, z, npc.SpawnX, npc.SpawnZ) > r {
			continue
		}
		if !townsfolk && w.legInTownLocked(npc.World, npc.X, npc.Z, x, z) {
			continue
		}
		if !w.legOKLocked(npc, x, z) {
			continue // somewhere it cannot stand, or into or out of the water (movement.go)
		}
		return x, z
	}
	return npc.X, npc.Z
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

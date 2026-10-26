package entity

import (
	"math"
	"math/rand"
	"time"
)

// Creatures wander round their spawn (F3 20261025-044257: "Wildlife does not
// roam around"), within the roam radius (world.roam_radius, 75 since F3
// 20261025-164404: "Wild life roaming needs increased distance and
// randomization. At least 5x the distance"; it was 15): legs of 5 to 35 in
// any direction from where it stands, inside the radius, walked at
// Sandstorm's walking pace (npcmove.go), each followed by a rest of 3 to 25 s,
// or, two times in five, a pause of half a second to two and another leg, so
// a creature meanders about its ground rather than crossing it straight. A
// leg never ends in a town nor passes through one (towns.go: "they should
// generally not path near or into cities"), unless the creature lives there.
// Sandstorm's own wander between rests, walking at 5 units a second
// (pcaps/ps2_sandstorm_1min10secs.pcap); its town guards stand.
//
// What wanders (movement.go's movementLocked): wildlife (wildlife.go: an
// animal by its name's head noun, not undead), and any creature table
// npcmove names a roamer (roam 1), on a body of its own (a model, and not
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

// DefaultRoamRadius is how far from its spawn a creature wanders.
const DefaultRoamRadius float32 = 75

// The rest between walks, the pause before a leg walked straight on (the
// chance of one), and the first rest of one that comes into a player's
// range (so a crowd does not set off at once).
const (
	roamRestMin   = 3 * time.Second
	roamRestMax   = 25 * time.Second
	roamPauseMin  = 500 * time.Millisecond
	roamPauseMax  = 2 * time.Second
	roamOnChance  = 0.4
	roamFirstRest = 10 * time.Second
)

// A leg's length.
const (
	roamLegMin float32 = 5
	roamLegMax float32 = 35
)

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
	if npc.snared(now) {
		speed /= 2
	}
	if w.stepNPCToward(npc, npc.roam.toX, npc.roam.toZ, speed, dt) {
		npc.roam.walking = false
		if w.roamRand().Float64() < roamOnChance {
			npc.roam.restUntil = now.Add(w.roamDuration(roamPauseMin, roamPauseMax))
		} else {
			npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
		}
	}
}

// roamPointLocked is where a creature walks next: a leg of roamLegMin to
// roamLegMax (at most the radius) in any direction from where it stands,
// ending within its radius of its spawn, neither ending in a town nor
// passing through one unless its spawn is in that town's ground (towns.go),
// and standable all along (legOKLocked); where it stands when no such leg is
// found (it rests and tries again).
func (w *World) roamPointLocked(npc *NPC) (x, z float32) {
	r := w.roamRadiusLocked(npc)
	lo, hi := min(roamLegMin, r), min(roamLegMax, r)
	if hi < roamMinStep {
		return npc.SpawnX, npc.SpawnZ
	}
	townsfolk := w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ)
	for i := 0; i < 16; i++ {
		d := lo + (hi-lo)*float32(w.roamRand().Float64())
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

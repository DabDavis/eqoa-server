package entity

import (
	"math"
	"math/rand"
	"time"
)

// Creatures wander round their spawn (F3 20261025-044257: "Wildlife does not
// roam around"): a point within the roam radius (world.roam_radius), walked
// to at Sandstorm's walking pace (npcmove.go), a rest of 5 to 20 s there,
// then another. Sandstorm's own wander that way between rests, walking at
// 5 units a second (pcaps/ps2_sandstorm_1min10secs.pcap); its town guards
// stand.
//
// What wanders: a creature, an NPC with a body of its own (a model, and
// not one of the players' races, charactermodel: those are the guards, the
// merchants and the townsfolk) that can be fought (no 0x80 in npc_type: an
// NPC a player talks to stays put), not a training dummy. It stands while
// it fights, while a spell roots it, dead, and when no player is near
// (roamActiveRange): only those near someone move, so the tick is as cheap
// with 20,000 NPCs as with 200. A snare halves its pace.
//
// The server has no ground: a creature keeps its spawn's height, and the
// client drops it onto the ground within 1.5 above or below that
// (ExtrapolateGround), which a small radius keeps it within on all but
// steep ground.

// DefaultRoamRadius is how far from its spawn a creature wanders.
const DefaultRoamRadius float32 = 15

// The rest between walks, and the first rest of one that comes into a
// player's range (so a crowd does not set off at once).
const (
	roamRestMin   = 5 * time.Second
	roamRestMax   = 20 * time.Second
	roamFirstRest = 10 * time.Second
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
}

// wanders reports an NPC of the kind that wanders (the file's doc), alive
// and free to; never a pet, which follows its owner (pet.go).
func (w *World) wanders(npc *NPC) bool {
	if npc.Dead || npc.RoamRadius <= 0 || npc.Dummy != NotDummy || npc.Owner != 0 {
		return false
	}
	if npc.NPCType&0x80 != 0 || npc.ModelID == 0 || w.playerModels[uint32(npc.ModelID)] {
		return false
	}
	return true
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
	if !w.wanders(npc) || npc.CurrentEnemy != 0 || npc.DummyTarget != 0 || npc.rooted(now) {
		// a fight, a root or a death: standing, and a rest before it goes on
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
		npc.roam.restUntil = now.Add(w.roamDuration(roamRestMin, roamRestMax))
	}
}

// roamPointLocked is where a creature walks next: a point evenly within its
// radius of its spawn, at least roamMinStep from where it stands (the spawn
// itself when the radius leaves no such point).
func (w *World) roamPointLocked(npc *NPC) (x, z float32) {
	r := float64(npc.RoamRadius)
	for i := 0; i < 8; i++ {
		d := r * math.Sqrt(w.roamRand().Float64())
		a := w.roamRand().Float64() * 2 * math.Pi
		x = npc.SpawnX + float32(d*math.Sin(a))
		z = npc.SpawnZ + float32(d*math.Cos(a))
		if dx, dz := x-npc.X, z-npc.Z; dx*dx+dz*dz >= roamMinStep*roamMinStep {
			return x, z
		}
	}
	return npc.SpawnX, npc.SpawnZ
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

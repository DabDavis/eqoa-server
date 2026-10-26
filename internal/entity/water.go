package entity

import (
	"time"

	"github.com/eqoa/server/internal/session"
)

// Where the water is, and what keeps to it (F3 20261026-020316, jib:
// "Wildlife that is not aquatic should avoid naturally roaming into the
// water, unless a player has agro or something and pulls it in. Caiman,
// crocodiles, crabs can roam both ... You should not be able to nuke
// monsters in the water when on land, or from the water when they are on
// land. Same biome for damage.").
//
// The water is the game's own: each world's BSP down to a zone leaf and its
// flags (internal/ground's env.go, baked off the operator's disc beside the
// ground files), the leaves CLIENT's walk swims in. A body is in the water
// when the point half a unit above its feet is (bodyAbove: a player wading
// at the edge is on land, one in to the knees is not).
//
// What a creature keeps to (table npcmove's water, movement.go): the land,
// the water, or both; with no word for it, where it spawned: in the water a
// creature of the water, else of the land. Wandering, a creature of the land
// never steps into the water and one of the water never out of it (roam.go's
// legs); a fight takes a creature where its enemy is, but one of the water
// stops at the water's edge. And no damage passes between the water and the
// land: a harmful spell between one in the water and one out of it is
// refused with the line below, and a melee or thrown hit across it is lost,
// both ways (the player's said once in a while). The words are ours: no
// recording of the live game's exists, and CLIENT keeps no line of sight.
//
// Without the files (no <world>.env) the server knows no water: nothing of
// this applies.

// Env is where the water is: ground.Env, read from the bake's <world>.env.
type Env interface {
	Flags(world uint8, x, y, z float32) (uint32, bool)
}

// envWater is a zone leaf's water flag (GetEnvFlags; ground.EnvWater).
const envWater uint32 = 1

// bodyAbove is how far above its feet a body is tested for the water.
const bodyAbove float32 = 0.5

// The water words of npcmove: what a creature keeps to.
const (
	keepsBySpawn = ""
	keepsToLand  = "land"
	keepsToBoth  = "both"
	keepsToWater = "water"
)

// The refusals of a harmful spell across the water's edge.
const (
	castTargetInWater  = "Your target is in the water."
	castTargetOutWater = "Your target is out of the water."
)

// apartTold is how often a player's melee lost across the water's edge is
// said.
const apartTold = 5 * time.Second

// SetEnv gives the world its water (and lava, slime and mud): ground.Env.
func (w *World) SetEnv(e Env) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.env = e
}

// waterAtLocked reports a point in the water; false where the server knows
// none. Must be called with the lock held.
func (w *World) waterAtLocked(world uint8, x, y, z float32) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(world, x, y, z)
	return ok && f&envWater != 0
}

// npcInWaterLocked and playerInWaterLocked report a body in the water.
func (w *World) npcInWaterLocked(npc *NPC) bool {
	return w.waterAtLocked(npc.World, npc.X, npc.Y+bodyAbove, npc.Z)
}

func (w *World) playerInWaterLocked(p *Player) bool {
	return w.waterAtLocked(p.World, p.X, p.Y+bodyAbove, p.Z)
}

// spawnInWaterLocked reports a creature that spawned in the water. Must be
// called with the lock held.
func (w *World) spawnInWaterLocked(npc *NPC) bool {
	return w.waterAtLocked(npc.World, npc.SpawnX, npc.SpawnY+bodyAbove, npc.SpawnZ)
}

// keepsToLocked is what a creature keeps to: npcmove's word, or by its
// spawn. Must be called with the lock held.
func (w *World) keepsToLocked(npc *NPC) string {
	if r, ok := w.moveRuleLocked(npc); ok && r.Water != keepsBySpawn {
		return r.Water
	}
	if w.spawnInWaterLocked(npc) {
		return keepsToWater
	}
	return keepsToLand
}

// staysInWaterLocked reports a creature of the water that is in it now: a
// fight does not take it out (one somehow out of it goes where it must).
// Must be called with the lock held.
func (w *World) staysInWaterLocked(npc *NPC) bool {
	return w.env != nil && w.keepsToLocked(npc) == keepsToWater && w.npcInWaterLocked(npc)
}

// waterAheadLocked reports the water at (x, z) where a creature would stand
// there: on the floor for one on the ground, else at its height. Must be
// called with the lock held.
func (w *World) waterAheadLocked(npc *NPC, x, z float32) bool {
	y := npc.Y
	if w.spawnOnGroundLocked(npc) {
		if f, ok := w.ground.Floor(npc.World, x, y, z); ok {
			y = f
		}
	}
	return w.waterAtLocked(npc.World, x, y+bodyAbove, z)
}

// apartLocked reports a player and a creature on either side of the
// water's edge. Must be called with the lock held.
func (w *World) apartLocked(p *Player, npc *NPC) bool {
	if w.env == nil {
		return false
	}
	return w.playerInWaterLocked(p) != w.npcInWaterLocked(npc)
}

// waterRefusalLocked is a harmful cast's refusal when its target is on the
// other side of the water's edge from its caster, "" otherwise. Must be
// called with the lock held.
func (w *World) waterRefusalLocked(caster *Player, targets []EntityID) string {
	if w.env == nil || len(targets) == 0 {
		return ""
	}
	in := w.playerInWaterLocked(caster)
	var theirs bool
	if npc := w.NPCs[targets[0]]; npc != nil {
		theirs = w.npcInWaterLocked(npc)
	} else if p := w.Players[targets[0]]; p != nil && p != caster {
		theirs = w.playerInWaterLocked(p)
	} else {
		return ""
	}
	switch {
	case in == theirs:
		return ""
	case theirs:
		return castTargetInWater
	default:
		return castTargetOutWater
	}
}

// tellApartLocked says a player's melee or throw at a creature across the
// water's edge was lost, once in apartTold. Must be called with the lock
// held.
func (w *World) tellApartLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) {
	if w.apartToldAt == nil {
		w.apartToldAt = make(map[EntityID]time.Time)
	}
	if now.Sub(w.apartToldAt[p.ServerID]) < apartTold {
		return
	}
	w.apartToldAt[p.ServerID] = now
	line := castTargetOutWater
	if w.npcInWaterLocked(npc) {
		line = castTargetInWater
	}
	w.sendColoredLocked(sess, line, castRefusalRGB)
}

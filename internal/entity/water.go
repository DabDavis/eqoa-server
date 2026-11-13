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
	w.groundGen++
}

// waterAtLocked reports a point in the water, or in slime, which bodies swim
// in as they do in water (the Plane of Disease's lake is all slime: its
// piranhas, keeping to the water, stood still on its spawn as "off the
// ground", 2026-11-13, Skywalker; fishing already took both:
// fishing.go); false where the server knows none. Must be called with the
// lock held.
func (w *World) waterAtLocked(world uint8, x, y, z float32) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(world, x, y, z)
	return ok && f&(envWater|envSlime) != 0
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

// Swimming, for the creatures of the land and the pets (the user,
// 2026-11-01: "also pets should swim if in water, also npcs, not just
// players"): one whose place is in water deeper than it can wade swims at
// the surface as a player does, its feet swimDepth under it, and CLIENT
// plays the liquid's animations for it (byte 58: LocoSwim moving, LocoTread
// still), where it walked the water's bed under the swimmers it followed or
// chased. CLIENT's walk takes a body as at the surface while its middle (1.2
// over its feet, m_parm38) is in the liquid and its top (1.8, m_parm30) is
// not; swimDepth is between them (ours: no recording of a swimming creature
// exists). A creature of the water (one that spawned in it, or npcmove's
// word) keeps its own depth and its animations, as does one of both (a
// crocodile on the bed); a pet summoned on land swims after its owner, one
// summoned in the water keeps the water's rule (pet.go). CLIENT's own
// ExtrapolateGround leaves a creature where it is sent when no floor lies
// within 1.5 under it, so it shows one floating at the surface.
const (
	swimDepth float32 = 1.5
	// surfaceSearch is how far over a body the surface is looked for
	surfaceSearch float32 = 400
)

// waterSurfaceLocked is the top of the water over a point in it: the first
// height above it the zone leaves say is out of it, to a twentieth; false
// for a point out of the water (or no water known). hint is a surface found
// near it before (a swimmer's last), tried first: two lookups where it
// holds; else the search widens (1, 2, 4 ... over the point) and halves, a
// score of lookups in the deepest sea. Must be called with the lock held.
func (w *World) waterSurfaceLocked(world uint8, x, y, z, hint float32) (float32, bool) {
	if !w.waterAtLocked(world, x, y, z) {
		return 0, false
	}
	if hint > y && w.waterAtLocked(world, x, hint-0.025, z) && !w.waterAtLocked(world, x, hint+0.025, z) {
		return hint, true
	}
	lo, hi := y, y
	for step := float32(1); ; step *= 2 {
		hi = y + step
		if step > surfaceSearch {
			return 0, false
		}
		if !w.waterAtLocked(world, x, hi, z) {
			break
		}
		lo = hi
	}
	for hi-lo > 0.05 {
		mid := (lo + hi) / 2
		if w.waterAtLocked(world, x, mid, z) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo, true
}

// swimLocked puts a creature of the land or a pet just set on its floor at
// the water's surface when the water there is deeper than swimDepth, and
// says whether it swims; anything else is left as it is, out of the water.
// Must be called with the lock held.
func (w *World) swimLocked(npc *NPC) bool {
	npc.swimming = false
	if w.env == nil || npc.Dead {
		return false
	}
	if npc.Owner == 0 && w.keepsToLocked(npc) != keepsToLand {
		return false
	}
	top, ok := w.waterSurfaceLocked(npc.World, npc.X, npc.Y+bodyAbove, npc.Z, npc.surface)
	if !ok || top-npc.Y <= swimDepth {
		return false // out of the water, or wading
	}
	npc.Y = top - swimDepth
	npc.VelY = 0
	npc.swimming, npc.surface = true, top
	return true
}

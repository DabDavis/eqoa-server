package entity

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Where the water is, and what keeps to it (F3 20261026-020316, a GM:
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
// ground", 2026-11-13, a player; fishing already took both:
// fishing.go); false where the server knows none. Must be called with the
// lock held.
func (w *World) waterAtLocked(world uint8, x, y, z float32) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(world, x, y, z)
	return ok && f&(envWater|envSlime) != 0
}

// envMud is a zone leaf's mud flag (ground.EnvMud).
const envMud uint32 = 8

// swimmableAtLocked is waterAtLocked with mud too: CLIENT's walker swims in water, lava, slime and mud
// (walkFlagsSubmerged), so its player swims across Geomancer's Citadel's mud pits, and a pet following it does
// too (Heirloom petition 2510, 2026-11-20: "pets don't register the mud pits as water so they walk along the
// bottom or on top when your toon is swimming"). A pet's (swimLocked) and the water's edge's
// (edgeInLocked, petition 2512): mud is no water to the rest of the server (a creature's keeping to water).
func (w *World) swimmableAtLocked(world uint8, x, y, z float32) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(world, x, y, z)
	return ok && f&(envWater|envSlime|envMud) != 0
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

// The slime's surface (the owner, 2026-11-14, the Plane of Disease: "the Fish
// seem to get out of the water very easy here like the environment
// boundaries didn't work very good"). The Plane's slime is a floor: its
// pools' surface is the terrain, flat at -49.94 (the river's at -34.06;
// beds slope on below the pools' top in places, to -51.6), and the zone leaf flagged slime over it reaches 14.7 to 25 units higher
// (to -35.2 over the eastern pools), over the banks round it as well: of the
// leaf's columns on world 5, 84,000 are the pools' flat surface and some
// 40,000 the banks under the leaf's top. A creature of the water kept to
// the leaf walked up the banks onto the shore, and one placed in it at a
// swimmer's height (a GM's own, CLIENT floating his body at the leaf's
// top) hung in the air over the slime: 75 of them taken out by hand,
// 2026-11-13. So in the slime (no water there) a creature of the water
// keeps to its pool: no more than slimeLevel over the pool's flat top
// (slimeSurfaceLocked; the banks rise off it), and one placed over the
// surface is set on it
// (groundnpc.go's spawnOnGroundLocked). The water is as it was: its
// creatures swim at their depths.
const slimeLevel float32 = 0.5

// slimeSurfaces turns the slime's surface rule on; a test's control turns it
// off (the leaf alone, as before).
var slimeSurfaces = true

// slimeOnlyLocked reports a point in the slime and not in the water. Must
// be called with the lock held.
func (w *World) slimeOnlyLocked(world uint8, x, y, z float32) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(world, x, y, z)
	return ok && f&envSlime != 0 && f&envWater == 0
}

// slimeSurfaceLocked is the slime's surface by a floor at y in it: its pool's
// flat top, the floor flat round a point in the slime (the Plane of
// Disease's pools at -49.94, their beds sloping on below it in places to
// -51.6; its river at -34.06; the banks rising off them are not flat),
// within slimeLook: the highest such no higher than y over it (one spawned
// up a bank keeps to the pool below it, and comes down to it), else the
// lowest (one spawned on a bed below its pool's top); y where none is. Must
// be called with the lock held and a ground.
func (w *World) slimeSurfaceLocked(world uint8, x, y, z float32) float32 {
	flat := func(px, pz, f float32) bool {
		for _, d := range [4][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			g, ok := w.floorLocked(world, px+d[0], f+1, pz+d[1])
			if !ok || absf(g-f) > 0.02 {
				return false
			}
		}
		return w.slimeOnlyLocked(world, px, f+bodyAbove, pz)
	}
	under, over := float32(0), float32(0)
	foundUnder, foundOver := false, false
	for r := float32(0); r <= slimeLook; r += slimeStep {
		n := 16
		if r == 0 {
			n = 1
		}
		for a := 0; a < n; a++ {
			s, c := math.Sincos(float64(a) * math.Pi / 8)
			px, pz := x+r*float32(c), z+r*float32(s)
			f, ok := w.floorLocked(world, px, y+slimeAbove, pz)
			if !ok || !flat(px, pz, f) {
				continue
			}
			if f <= y+slimeLevel {
				if !foundUnder || f > under {
					under, foundUnder = f, true
				}
			} else if !foundOver || f < over {
				over, foundOver = f, true
			}
		}
	}
	switch {
	case foundUnder:
		return under
	case foundOver:
		return over
	}
	return y
}

// slimeLook is how far round a spawn in the slime its surface is looked for
// (the Plane of Disease's banks are 16 to 40 across), slimeStep the rings'
// spacing, slimeAbove how far over the spawn's floor a floor is looked for
// (its pool's top over a bed below it).
const (
	slimeLook  float32 = 48
	slimeStep  float32 = 3
	slimeAbove float32 = 4
)

// slimeSettlesLocked reports a creature of the water whose spawn is over
// the floor at y, in the slime there and at the floor: set on the slime's
// surface (spawnOnGroundLocked). Must be called with the lock held.
func (w *World) slimeSettlesLocked(npc *NPC, y float32) bool {
	return slimeSurfaces && w.slimeOnlyLocked(npc.World, npc.SpawnX, npc.SpawnY+bodyAbove, npc.SpawnZ) &&
		w.slimeOnlyLocked(npc.World, npc.SpawnX, y+bodyAbove, npc.SpawnZ) &&
		w.keepsToLocked(npc) == keepsToWater
}

// keepsInLocked reports (x, z) at height y (its floor, for a creature on
// the ground) as where a creature of the water may be: in the water; or in
// the slime no more than slimeLevel over its pool's flat top
// (slimeSurfaceLocked). Must be called with
// the lock held.
//
// Never a point in the ground: a sea's water leaf runs on under its beach, so a point at a swimmer's height under
// the sand is in the water by the env alone, and a swimmer let in there was lifted out onto the sand
// (groundnpc.go's liftOutLocked), stranded (Live petition 5582, 2026-11-24: Croc Island's mako sharks by Blackwater
// "on land").
func (w *World) keepsInLocked(npc *NPC, x, y, z float32) bool {
	if !w.waterAtLocked(npc.World, x, y+bodyAbove, z) || w.inGroundLocked(npc.World, x, y, z) {
		return false
	}
	if !slimeSurfaces || !w.slimeOnlyLocked(npc.World, x, y+bodyAbove, z) || !w.spawnOnGroundLocked(npc) {
		return true
	}
	return y <= npc.spawnFloor+slimeLevel // its pool's top, or its bed below it
}

// swimmerAshoreLocked reports a creature of the water that swims off the ground (spawned in it, over the bed) and
// stands out of its water: no wander leg takes it back in (a leg's every point in the water, at its height), so it
// stood there until it died. Croc Island's mako sharks by Blackwater were lifted onto the sand before keepsInLocked
// knew the ground (Live petition 5582), and a restart keeps a creature where it stood (keepnpc.go). At its next leg
// it is set back on its spawn instead (backInTheWaterLocked), as its death and return would: a walk home slopes
// from the sand down to its swimming height, and that line runs into the ground wherever the shore falls steeper,
// where its steps are refused and it would stand on the sand again. Must be called with the lock held.
func (w *World) swimmerAshoreLocked(npc *NPC) bool {
	return w.env != nil && w.ground != nil && !w.spawnOnGroundLocked(npc) && w.keepsToLocked(npc) == keepsToWater &&
		w.spawnInWaterLocked(npc) && !w.npcInWaterLocked(npc)
}

// backInTheWaterLocked sets a swimmer ashore back on its spawn, standing (swimmerAshoreLocked). Must be called
// with the lock held.
func (w *World) backInTheWaterLocked(npc *NPC) {
	oldX, oldZ := npc.X, npc.Z
	npc.stopMoving()
	npc.X, npc.Y, npc.Z, npc.Facing = npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing
	npc.roam.walking = false
	w.swimLocked(npc)
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
	log.Printf("Water: %s (id %d) was out of its water at (%.1f, %.1f): back on its spawn", npc.Name, npc.ServerID, oldX, oldZ)
}

// waterAheadLocked reports the water at (x, z) where a creature would stand
// there (on the floor for one on the ground, else at its height) as one of
// the water keeps to it (keepsInLocked); one off its slime's surface (up a
// bank, as it could go before) may come down to it through the slime. Must
// be called with the lock held.
func (w *World) waterAheadLocked(npc *NPC, x, z float32) bool {
	y := npc.Y
	if w.spawnOnGroundLocked(npc) {
		if f, ok := w.ground.Floor(npc.World, x, y, z); ok {
			y = f
		}
	}
	if w.keepsInLocked(npc, x, y, z) {
		return true
	}
	return y <= npc.Y && !w.keepsInLocked(npc, npc.X, npc.Y, npc.Z) && w.waterAtLocked(npc.World, x, y+bodyAbove, z) &&
		!w.inGroundLocked(npc.World, x, y, z)
}

// bodyWet is how far above its feet a body is tested for water at its feet:
// a body wading (water there, not at bodyAbove) is on both sides of the edge.
const bodyWet float32 = 0.05

// wetAtLocked reports water (or slime) at a body's feet or over them. Must be
// called with the lock held.
func (w *World) wetAtLocked(world uint8, x, y, z float32) bool {
	return w.waterAtLocked(world, x, y+bodyWet, z) || w.waterAtLocked(world, x, y+bodyAbove, z)
}

// apartBodies is whether two bodies are on either side of the water's edge:
// one in the water (at bodyAbove) and the other dry (no water at its feet).
// A body wading in shallow water is apart from neither (Heirloom petition
// 2145, a player, 2026-11-15, Guk: "ill be fighting a mob and will get the error
// 'your target is in the water' even if im in the water or the water is very
// low").
func apartBodies(inA, wetA, inB, wetB bool) bool {
	return inA && !wetB || inB && !wetA
}

// apartLocked reports a player and a creature on either side of the
// water's edge. Must be called with the lock held.
func (w *World) apartLocked(p *Player, npc *NPC) bool {
	if w.env == nil {
		return false
	}
	return apartBodies(w.edgeInLocked(p.World, p.X, p.Y, p.Z), w.edgeWetLocked(p.World, p.X, p.Y, p.Z),
		w.edgeInLocked(npc.World, npc.X, npc.Y, npc.Z), w.edgeWetLocked(npc.World, npc.X, npc.Y, npc.Z))
}

// edgeInLocked and edgeWetLocked are the water's edge's in-the-water and wet-feet tests: water, slime and mud
// (swimmableAtLocked), as CLIENT's walker swims in all three. A body swimming in Geomancer's Citadel's mud is on
// the far side of the edge from one on its bank (Heirloom petition 2512, 2026-11-20: "the mud also isn't
// registered by your character when attacking mobs in it or while you are swimming in it. Doesn't block you
// from casting a creature on land while swimming"). Must be called with the lock held.
func (w *World) edgeInLocked(world uint8, x, y, z float32) bool {
	return w.swimmableAtLocked(world, x, y+bodyAbove, z)
}

func (w *World) edgeWetLocked(world uint8, x, y, z float32) bool {
	return w.swimmableAtLocked(world, x, y+bodyWet, z) || w.swimmableAtLocked(world, x, y+bodyAbove, z)
}

// waterRefusalLocked is a harmful cast's refusal when its target is on the
// other side of the water's edge from its caster, "" otherwise. Must be
// called with the lock held.
func (w *World) waterRefusalLocked(caster *Player, targets []EntityID) string {
	if w.env == nil || len(targets) == 0 {
		return ""
	}
	in := w.edgeInLocked(caster.World, caster.X, caster.Y, caster.Z)
	wet := w.edgeWetLocked(caster.World, caster.X, caster.Y, caster.Z)
	var theirs, theirsWet bool
	if npc := w.NPCs[targets[0]]; npc != nil {
		theirs, theirsWet = w.edgeInLocked(npc.World, npc.X, npc.Y, npc.Z), w.edgeWetLocked(npc.World, npc.X, npc.Y, npc.Z)
	} else if p := w.Players[targets[0]]; p != nil && p != caster {
		theirs, theirsWet = w.edgeInLocked(p.World, p.X, p.Y, p.Z), w.edgeWetLocked(p.World, p.X, p.Y, p.Z)
	} else {
		return ""
	}
	switch {
	case !apartBodies(in, wet, theirs, theirsWet):
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
	return w.liquidSurfaceLocked(w.waterAtLocked, world, x, y, z, hint)
}

// liquidSurfaceLocked is waterSurfaceLocked for the liquid in says is there.
func (w *World) liquidSurfaceLocked(in func(world uint8, x, y, z float32) bool, world uint8, x, y, z, hint float32) (float32, bool) {
	if !in(world, x, y, z) {
		return 0, false
	}
	if hint > y && in(world, x, hint-0.025, z) && !in(world, x, hint+0.025, z) {
		return hint, true
	}
	lo, hi := y, y
	for step := float32(1); ; step *= 2 {
		hi = y + step
		if step > surfaceSearch {
			return 0, false
		}
		if !in(world, x, hi, z) {
			break
		}
		lo = hi
	}
	for hi-lo > 0.05 {
		mid := (lo + hi) / 2
		if in(world, x, mid, z) {
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
	// a creature of the water keeps its own depth; one of the land and one
	// that goes both ways (caimans, crocodiles) swims at the surface where
	// the water is deep (petition 5446: "young caimen run along the bottom of
	// water when chasing you")
	if npc.Owner == 0 && w.keepsToLocked(npc) == keepsToWater {
		return false
	}
	in := w.waterAtLocked
	if npc.Owner != 0 {
		in = w.swimmableAtLocked // a pet swims where its owner can: mud too
	}
	top, ok := w.liquidSurfaceLocked(in, npc.World, npc.X, npc.Y+bodyAbove, npc.Z, npc.surface)
	if !ok || top-npc.Y <= swimDepth {
		return false // out of the water, or wading
	}
	npc.Y = top - swimDepth
	npc.VelY = 0
	npc.swimming, npc.surface = true, top
	return true
}

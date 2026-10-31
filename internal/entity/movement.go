package entity

import (
	"fmt"

	"github.com/eqoa/server/internal/database"
)

// How the creatures of a name move: table npcmove (database/npcmove.go; the
// GM panel's "Creature movement", applied at once). Whether a creature
// wanders is its row's roam: 1 it wanders (a designated roamer, wildlife or
// not: Klik'Anon's pirate patrols), 0 it stands on its spawn until a fight
// takes it (F3 20261026-021730, jib: "hexbones in zentaurs keep do NOT roam,
// they are static spawn until agrod"), -1 (the default) or no row as its
// kind: wildlife wanders (wildlife.go) and anything else stands (F3
// 20261026-042712: "Not all mobs are wild life. Fire Giants are roaming
// currently when they should not be unless otherwise specified / designated
// as a roamer with a path"). A path (path.go) walks any NPC. radius is its
// own wander radius; water what it keeps to (water.go).
//
// Wherever it wanders, a creature must be able to stand all along the leg
// (legOKLocked): one on the ground finds a floor at every two units and no
// drop past legDrop between two of them (a cliff it would fall off), one
// keeping to the land meets no water, one keeping to the water leaves none
// and passes under no rising sea floor. And one that stands off the ground
// (on what the ground leaves out: a keep's floor, a bridge, a dock; or with
// no floor under it) does not wander at all, whatever its row says, but for
// one swimming where it spawned: walked at its spawn's height it would float
// wherever the floor beneath it fell away (F3 20261026-021425).
//
// Nor does a leg pass through a wall (F3 pass 24, the user: "they are
// walking outside the walls of the tavern basement, also city cat does the
// same and walks through walls"): from each of those points to the next, at
// a creature's knee above the floor (legKnee, and as far again as it spawned
// above the terrain: a house's floor over the street), the way meets no wall
// of the terrain and nothing placed on it (the ground's walls files: Neriak's
// tavern is a house placed in the city's cavern, its rats spawned on its
// floor over the cavern's, and they walked the cavern's floor out through
// its walls). A wall lower than the knee (a kerb, a step, a threshold) is
// stepped over, as a table or a fence higher than it is not.
//
// And every point of a leg is one where CLIENT sets the creature down on its
// own floor (clearLocked; the user, 2026-10-31: "the tavern rats (and
// probably any mob) though they are confined by the walls can still somewhat
// walk up them"). Its ExtrapolateGround (viaiextrapolate.cpp) sweeps a ball
// 0.1 round down from 1.5 above the height the server sends and stands the
// creature where it first touches: on a bench or a stair tread it was walked
// under or over, up a wall it came within 0.1 of. Over two hours of the
// Double Daggers' cellar rats' wander, 1.3% of their places were drawn so
// (vanilla's own collision, bit-exact to the console): on the benches 0.47
// over the floor, up the walls and the stairs to 0.72. So nothing placed
// may stand between the creature's floor and the ball's start at any point
// of a leg (each unit, and half a unit past its end, where the client's
// carrying it on until the stop comes may take it), and no placed wall
// within clearReach of it at its feet.

// legSample is the spacing of the points a wander leg is tried at, and
// legDrop how far the floor may fall between two of them (51 degrees);
// legKnee how far above the floor the way between two is tested for walls.
const (
	legSample float32 = 2
	legDrop   float32 = 2.5
	legKnee   float32 = 1
)

// CLIENT's ground probe and the room a creature is kept clear in
// (clearLocked): the ball's start over the height sent (1.5, and the ball's
// 0.1); how far over its own floor something counts as standing above it;
// how far round it no wall may be, and how high over its floor that is
// looked at; and how far past a leg's end the client may carry it.
const (
	clearProbe     float32 = 1.6
	clearAbove     float32 = 0.15
	clearReach     float32 = 0.4
	clearFeet      float32 = 0.3
	clearOvershoot float32 = 0.5
)

// clearDirs are the directions a creature's room is looked at in.
var clearDirs = [8][2]float32{{1, 0}, {0.7071, 0.7071}, {0, 1}, {-0.7071, 0.7071}, {-1, 0}, {-0.7071, -0.7071}, {0, -1}, {0.7071, -0.7071}}

// clearLocked is whether CLIENT sets a creature sent at height y over (x, z)
// down on its own floor, floor (the package's doc): nothing placed under
// its ground probe's start above that floor, and no placed wall within
// clearReach at its feet. Must be called with the lock held.
func clearLocked(p Placed, world uint8, x, y, z, floor float32) bool {
	if p.Placed(world, x, y+clearProbe, z, x, floor+clearAbove, z) {
		return false // a bench, a stair, a ledge: it would be set on it
	}
	feet := floor + clearFeet
	for _, d := range clearDirs {
		if p.Placed(world, x, feet, z, x+d[0]*clearReach, feet, z+d[1]*clearReach) {
			return false // a wall it would be set up
		}
	}
	return true
}

// SetMoveRules sets table npcmove's rules, keyed as database.NPCClassKey
// keys a name.
func (w *World) SetMoveRules(rules map[string]database.NPCMoveRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.moveRules = rules
}

// LoadMoveRules reads table npcmove into the world: at the start and after a
// GM's edit. Call it without the database held. It reports how many names it
// read.
func (w *World) LoadMoveRules(db *database.SQLiteDB) (int, error) {
	rules, err := db.NPCMoveRules()
	if err != nil {
		return 0, fmt.Errorf("npcmove: %w", err)
	}
	w.SetMoveRules(rules)
	return len(rules), nil
}

// MoveSummary says how creatures move, for the log at the start.
func (w *World) MoveSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	stand, roam, both, water := 0, 0, 0, 0
	for _, r := range w.moveRules {
		switch {
		case r.ByKind:
		case r.Roam:
			roam++
		default:
			stand++
		}
		switch r.Water {
		case keepsToBoth:
			both++
		case keepsToWater:
			water++
		}
	}
	where := "no water known (no <world>.env)"
	if w.env != nil {
		where = "the water known"
	}
	return fmt.Sprintf("movement: %d names in npcmove (%d stand, %d wander whatever their kind, the rest as their kind: wildlife wanders; "+
		"%d go in and out of the water, %d keep to it), %s", len(w.moveRules), stand, roam, both, water, where)
}

// moveRuleLocked is a creature's row of npcmove, false with none. Must be
// called with the lock held.
func (w *World) moveRuleLocked(npc *NPC) (database.NPCMoveRule, bool) {
	r, ok := w.moveRules[database.NPCClassKey(npc.Name)]
	return r, ok
}

// moveWhy is why an NPC wanders or stands (movementLocked), said in a GM's
// words by MoveWhyText.
type moveWhy uint8

const (
	whyDead moveWhy = iota
	whyPet
	whyDummy
	whyPath
	whyNoBody
	whyTalkedTo
	whyNoRadius
	whyRowStands
	whyRowWanders
	whyTownsfolk
	whyWildlife
	whyUndead
	whyNotWildlife
	whyOffGround
	whySpawnOnly
)

// MoveWhyText says why, in a GM's words (the panel's NPC page); word is the
// name's animal noun for whyWildlife and whyUndead.
func MoveWhyText(why moveWhy, word string) string {
	switch why {
	case whyDead:
		return "dead"
	case whyPet:
		return "a pet: it follows its owner"
	case whyDummy:
		return "a training dummy: it stands"
	case whyPath:
		return "it walks its path"
	case whyNoBody:
		return "no body: it stands"
	case whyTalkedTo:
		return "an NPC players talk to (the NPC bit): it stands; a path walks it"
	case whyNoRadius:
		return "nothing wanders on this server (world.roam_radius 0)"
	case whyRowStands:
		return "it stands on its spawn until pulled: Creature movement says so"
	case whyRowWanders:
		return "it wanders: Creature movement says so"
	case whyTownsfolk:
		return "townsfolk (a player race's body): it stands"
	case whyWildlife:
		return fmt.Sprintf("it wanders: wildlife (its name's %q)", word)
	case whyUndead:
		return "undead: it stands on its spawn until pulled"
	case whyNotWildlife:
		return "not wildlife: it stands on its spawn until pulled (Creature movement's Wanders 1 makes it wander)"
	case whyOffGround:
		return "it stands off the ground (a keep's floor, a bridge, a dock): it does not wander"
	case whySpawnOnly:
		return "spawned only (NPC spawn only): it stands on its spawn out of every view, there for the quests and triggers that copy it"
	}
	return ""
}

// movementLocked is whether an NPC wanders round its spawn, and why (the
// file's comment and roam.go's); word is the name's animal noun, if any. Must
// be called with the lock held (a read lock will do).
func (w *World) movementLocked(npc *NPC) (wanders bool, why moveWhy, word string) {
	switch {
	case npc.SpawnOnly:
		return false, whySpawnOnly, "" // spawnonly.go
	case npc.Dead:
		return false, whyDead, ""
	case npc.Owner != 0:
		return false, whyPet, ""
	case npc.Dummy != NotDummy:
		return false, whyDummy, ""
	case w.pathedLocked(npc.ServerID):
		return false, whyPath, ""
	case npc.ModelID == 0:
		return false, whyNoBody, ""
	case npc.NPCType&0x80 != 0:
		return false, whyTalkedTo, ""
	case npc.RoamRadius <= 0:
		return false, whyNoRadius, ""
	}
	if r, ok := w.moveRuleLocked(npc); ok && !r.ByKind {
		if !r.Roam {
			return false, whyRowStands, ""
		}
		why = whyRowWanders
	} else if w.playerModels[uint32(npc.ModelID)] {
		return false, whyTownsfolk, ""
	} else {
		var wild bool
		word, wild = w.wildlifeLocked(npc)
		switch {
		case wild:
			why = whyWildlife
		case word != "":
			return false, whyUndead, word
		default:
			return false, whyNotWildlife, ""
		}
	}
	if w.ground != nil && !w.spawnOnGroundLocked(npc) && !w.spawnInWaterLocked(npc) {
		return false, whyOffGround, word
	}
	return true, why, word
}

// roamRadiusLocked is how far from its spawn a creature wanders: its row's
// radius, else its own. Must be called with the lock held.
func (w *World) roamRadiusLocked(npc *NPC) float32 {
	if r, ok := w.moveRuleLocked(npc); ok && r.Radius > 0 {
		return r.Radius
	}
	return npc.RoamRadius
}

// legOKLocked reports a wander leg from where a creature stands to (x, z)
// that it can walk (the file's comment). Must be called with the lock held.
func (w *World) legOKLocked(npc *NPC, x, z float32) bool {
	d := distXZ(npc.X, npc.Z, x, z)
	n := int(d/legSample) + 1
	onGround := w.spawnOnGroundLocked(npc)
	keeps := keepsToBoth
	if w.env != nil {
		keeps = w.keepsToLocked(npc)
	}
	y := npc.Y
	walls, _ := w.ground.(Walls)
	placed, _ := w.ground.(Placed)
	if !onGround {
		placed = nil // set down where it was sent: nothing of the ground's to keep it off
	}
	lift := max(npc.spawnLift, 0) // its own floor over the terrain (a house's over the street)
	knee, px, py, pz := legKnee, npc.X, y, npc.Z
	if onGround {
		knee += npc.spawnLift
		if f, ok := w.ground.Floor(npc.World, npc.X, y, npc.Z); ok {
			py = f
		}
	}
	for i := 1; i <= n; i++ {
		t := float32(i) / float32(n)
		sx, sz := npc.X+(x-npc.X)*t, npc.Z+(z-npc.Z)*t
		if onGround {
			f, ok := w.ground.Floor(npc.World, sx, y, sz)
			if !ok || f < y-legDrop {
				return false // no floor, or a fall
			}
			y = f
		}
		if walls != nil && walls.Wall(npc.World, px, py+knee, pz, sx, y+knee, sz) {
			return false // a wall in the way
		}
		if placed != nil {
			// set down on its own floor here and halfway from the last point
			mx, my, mz := (px+sx)/2, (py+y)/2, (pz+sz)/2
			if !clearLocked(placed, npc.World, mx, my, mz, my+lift) || !clearLocked(placed, npc.World, sx, y, sz, y+lift) {
				return false
			}
			if i == n && d > 0 {
				ox, oz := sx+(x-npc.X)/d*clearOvershoot, sz+(z-npc.Z)/d*clearOvershoot
				if !clearLocked(placed, npc.World, ox, y, oz, y+lift) {
					return false // where the client may carry it on to before it stops
				}
			}
		}
		px, py, pz = sx, y, sz
		switch keeps {
		case keepsToLand:
			if w.waterAtLocked(npc.World, sx, y+bodyAbove, sz) {
				return false
			}
		case keepsToWater:
			if !w.waterAtLocked(npc.World, sx, y+bodyAbove, sz) {
				return false
			}
			if !onGround && w.ground != nil {
				if f, ok := w.ground.Floor(npc.World, sx, y, sz); ok && f > y {
					return false // the sea floor rises above it
				}
			}
		}
	}
	return true
}

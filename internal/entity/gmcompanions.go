package entity

// gmcompanions.go — a GM's copy of a creature brings its companions (the owner,
// 2026-11-18: a /place of Tunare, row 190868 a copy of 162665, stood without
// Wrath and Grace of the Mother, the spawn-only originals beside the original in
// Freeport).
//
// Who a creature's companions are: what companion.go reads off the spawns (a
// pet's name naming its owner, a pet's kind beside its caster), and the GM
// triggers' resummons (SetTriggerCompanions): trigger 141's "Tunare calls Wrath
// of the Mother back to her side" makes a creature called Wrath of the Mother
// the pet of the creature called Tunare its name rule would give it (within
// companionNamedReach), as a "Goran's pet" is Goran's. Nothing else ties Wrath
// and Grace to Tunare: no name rule, no npcsocial row, and all three originals
// are spawn-only (companionFighter leaves them out). So an original's companions
// (companionsOfOriginalLocked) are read as linkCompanionsLocked would read them
// with the spawn-only originals taking part: the rows within companionNamedReach
// of its spawn whose best owner it is.
//
// A copy brings copies of them, each standing off it as its original stands off
// the original, turned with it (companionSpots: the copy's facing less the
// original's), on the ground's floor there when one lies near:
//
//   - #spawn and the panel's spawn_npc (SpawnGMCopy): the copies are GMSpawn,
//     each pet's gmCompanionOf the owner's copy, paired so and no other way
//     (linkCompanionsLocked). They fight as one, walk with it, die with it
//     (companion.go), go with it at #despawn, and a keep restart carries the
//     pair (keepgmspawn.go).
//   - /place and /clone (handlers/gmnpcrow.go): each pet a new row too, a copy
//     of its original's row placed at its spot (CompanionSpots), standing as the
//     owner's does when its name is spawn-only (npcstands); the rows pair by the
//     rules above, at every start, as any row's.
//
// A trigger's or a quest's spawn brings nothing: a trigger spawns and resummons
// its pets itself (trigger 141 spawns all three).

import (
	"math"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// SetTriggerCompanions takes the GM triggers' resummons as pairs (the file's
// doc): pets maps each resummoned NPC's name to the name of the spawn that calls
// it back ("Wrath of the Mother": "Tunare"). The pairs are read again at once.
func (w *World) SetTriggerCompanions(pets map[string]string) {
	m := make(map[string]string, len(pets))
	for pet, by := range pets {
		if k := database.NPCClassKey(pet); k != "" && strings.TrimSpace(by) != "" && !strings.EqualFold(strings.TrimSpace(pet), strings.TrimSpace(by)) {
			m[k] = strings.TrimSpace(by)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.triggerPets = m
	w.companionOf = nil // read again (linkCompanionsLocked)
}

// companionOriginal is whether n takes part in an original's pairs
// (companionsOfOriginalLocked): companionFighter's, a spawn-only original too, of
// a row (no copy of any kind).
func companionOriginal(n *NPC) bool {
	return n != nil && !n.Temporary && n.Owner == 0 && n.Charm == nil && n.NPCType&0x80 == 0 && n.Dummy == NotDummy
}

// companionsOfOriginalLocked is t's companions as its copies bring them (the
// file's doc): the rows in its world within companionNamedReach of its spawn
// whose best owner, spawn-only originals counted, is t. Nil for none, or for t
// no original. Must be called with the lock held.
func (w *World) companionsOfOriginalLocked(t *NPC) []*NPC {
	if !companionOriginal(t) {
		return nil
	}
	if pet, _, _ := w.companionPetRuleLocked(t.Name); pet {
		return nil // a pet has none of its own
	}
	var near, pets []*NPC
	for _, n := range w.NPCs {
		if n == t || !companionOriginal(n) || n.World != t.World || n.Instance != t.Instance ||
			distXZ(n.SpawnX, n.SpawnZ, t.SpawnX, t.SpawnZ) > 2*companionNamedReach {
			continue
		}
		if pet, _, _ := w.companionPetRuleLocked(n.Name); pet {
			if distXZ(n.SpawnX, n.SpawnZ, t.SpawnX, t.SpawnZ) <= companionNamedReach {
				pets = append(pets, n)
			}
			continue
		}
		near = append(near, n)
	}
	var out []*NPC
	for _, pet := range pets {
		_, named, ownerOnly := w.companionPetRuleLocked(pet.Name)
		mine, ok := companionScore(pet, t, named, ownerOnly)
		if !ok {
			continue
		}
		theirs := false // a better owner of it than t
		for _, o := range near {
			if s, ok := companionScore(pet, o, named, ownerOnly); ok && (s < mine || s == mine && o.ServerID < t.ServerID) {
				theirs = true
				break
			}
		}
		if !theirs {
			out = append(out, pet)
		}
	}
	// in id order, so the copies' order (and ids) do not change from one call to the next
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return out
}

// CompanionSpot is where a copy's companion stands (the file's doc): Row its
// original, Name its name, its place and facing (radians).
type CompanionSpot struct {
	Row             EntityID
	Name            string
	SpawnOnly       bool // its original's name is spawn-only (npcspawnonly): a row of it needs npcstands
	X, Y, Z, Facing float32
}

// companionSpotsLocked is where t's companions stand by a copy of t at (x, y, z)
// facing facing in world (the file's doc). Must be called with the lock held.
func (w *World) companionSpotsLocked(t *NPC, x, y, z, facing float32, world uint8) []CompanionSpot {
	pets := w.companionsOfOriginalLocked(t)
	if len(pets) == 0 {
		return nil
	}
	turn := float64(facing - t.SpawnFacing)
	sin, cos := float32(math.Sin(turn)), float32(math.Cos(turn))
	out := make([]CompanionSpot, 0, len(pets))
	for _, p := range pets {
		ox, oy, oz := p.SpawnX-t.SpawnX, p.SpawnY-t.SpawnY, p.SpawnZ-t.SpawnZ
		px, pz := x+ox*cos+oz*sin, z+oz*cos-ox*sin
		py := y + oy
		if w.ground != nil {
			if fy, ok := w.ground.Floor(world, px, py+3, pz); ok && fy-py <= triggerFloorReach && py-fy <= triggerFloorReach {
				py = fy
			}
		}
		f := float32(math.Mod(float64(p.SpawnFacing)+turn, 2*math.Pi))
		if f < 0 {
			f += 2 * math.Pi
		}
		out = append(out, CompanionSpot{Row: p.ServerID, Name: p.Name, SpawnOnly: p.SpawnOnly, X: px, Y: py, Z: pz, Facing: f})
	}
	return out
}

// CompanionSpots is where the companions of the NPC row (an original) stand by a
// copy of it at (x, y, z) facing facing (radians) in world, for /place and /clone
// to make rows of (the file's doc); nil for none.
func (w *World) CompanionSpots(row EntityID, x, y, z, facing float32, world uint8) []CompanionSpot {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.NPCs[row]
	if t == nil || t.Temporary {
		return nil
	}
	return w.companionSpotsLocked(t, x, y, z, facing, world)
}

// SpawnGMCopy is a GM's spawn (#spawn, the panel's spawn_npc): SpawnNPCCopy's
// copy, GMSpawn, with copies of its original's companions beside it, paired with
// it (the file's doc). It returns the copy's id, the name as the original spells
// it, and its companions' copies' ids.
func (w *World) SpawnGMCopy(name string, x, y, z, facing float32, world uint8) (EntityID, string, []EntityID, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.npcByNameForLocked(name, world)
	if t == nil {
		return 0, "", nil, false
	}
	spots := w.companionSpotsLocked(t, x, y, z, facing, world)
	c := w.spawnCopyLocked(t, x, y, z, facing, world)
	c.GMSpawn = true
	var pets []EntityID
	for _, s := range spots {
		o := w.NPCs[s.Row]
		if o == nil {
			continue
		}
		p := w.spawnCopyLocked(o, s.X, s.Y, s.Z, s.Facing, world)
		p.GMSpawn, p.gmCompanionOf = true, c.ServerID
		pets = append(pets, p.ServerID)
	}
	if len(pets) > 0 {
		w.companionOf = nil // the pairs read again (linkCompanionsLocked)
	}
	return c.ServerID, t.Name, pets, true
}

// gmCompanionsLocked is the living GM's spawns paired with owner (its companions'
// copies). Must be called with the lock held.
func (w *World) gmCompanionsLocked(owner *NPC) []*NPC {
	if owner == nil || !owner.GMSpawn {
		return nil
	}
	var out []*NPC
	for _, n := range w.NPCs {
		if n.GMSpawn && n.gmCompanionOf == owner.ServerID {
			out = append(out, n)
		}
	}
	return out
}

// CompanionMaster is the creature the NPC id fights beside as its pet (the pairs,
// companion.go), false for none.
func (w *World) CompanionMaster(id EntityID) (EntityID, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil {
		return 0, false
	}
	if m := w.companionMasterLocked(npc); m != nil {
		return m.ServerID, true
	}
	return 0, false
}

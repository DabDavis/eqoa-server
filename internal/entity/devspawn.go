package entity

import (
	"sort"
	"strings"
	"time"
)

// What the GM commands need of the world (handlers/devcmds.go): the NPCs'
// names to search, a copy of one placed somewhere, and the travel NPCs'
// places by the names a player knows them by.

// allocFreeIDLocked returns an entity id that no player or NPC holds.
// allocID alone counts up from 1000, and NPC ids are their database rows
// (360..3255 today), so the counter walks straight through them. Must be
// called with the lock held.
func (w *World) allocFreeIDLocked() EntityID {
	for {
		id := w.allocID()
		if _, npc := w.NPCs[id]; npc {
			continue
		}
		if _, p := w.Players[id]; p {
			continue
		}
		return id
	}
}

// NPCNames is every loaded NPC's name, each once, sorted without case.
func (w *World) NPCNames() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	seen := make(map[string]bool, len(w.NPCs))
	var names []string
	for _, npc := range w.NPCs {
		if npc.Temporary || seen[npc.Name] {
			continue
		}
		seen[npc.Name] = true
		names = append(names, npc.Name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := strings.ToLower(names[i]), strings.ToLower(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names
}

// NPCByName is the NPC of that name (without case) that a GM command means:
// a live one before a dead one, and of those the lowest id, so the answer
// does not change from one call to the next (map order is random).
func (w *World) NPCByName(name string) (x, y, z, facing float32, world uint8, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if npc := w.npcByNameLocked(name); npc != nil {
		return npc.X, npc.Y, npc.Z, npc.Facing, npc.World, true
	}
	return 0, 0, 0, 0, 0, false
}

func (w *World) npcByNameLocked(name string) *NPC {
	var best *NPC
	for _, npc := range w.NPCs {
		if npc.Temporary || !strings.EqualFold(npc.Name, name) {
			continue
		}
		switch {
		case best == nil,
			best.Dead && !npc.Dead,
			best.Dead == npc.Dead && npc.ServerID < best.ServerID:
			best = npc
		}
	}
	return best
}

// SpawnNPCCopy places a copy of the NPC named name (as NPCByName picks it)
// at (x, y, z) in world, facing facing: its model, size, gear, level and
// faction, full health, no fight. The copy is Temporary: no database row,
// gone after its corpse time once killed. It returns the copy's id and the
// name as the template spells it.
func (w *World) SpawnNPCCopy(name string, x, y, z, facing float32, world uint8) (EntityID, string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.npcByNameLocked(name)
	if t == nil {
		return 0, "", false
	}
	c := *t
	c.ServerID = w.allocFreeIDLocked()
	c.X, c.Y, c.Z, c.Facing = x, y, z, facing
	c.SpawnX, c.SpawnY, c.SpawnZ, c.SpawnFacing = x, y, z, facing
	c.World = world
	c.HP = c.MaxHP
	c.Dead = false
	c.DeathTime = time.Time{}
	c.Threats = nil
	c.CurrentEnemy = 0
	c.LastCombat = time.Time{}
	c.LastCombatTime = time.Time{}
	c.Temporary = true
	w.NPCs[c.ServerID] = &c
	w.SpatialIndex.Insert(c.ServerID, x, z)
	return c.ServerID, t.Name, true
}

// DespawnNPC takes a GM's spawned NPC out of the world at once. An NPC with a
// database row is refused: it would only come back at the next start.
func (w *World) DespawnNPC(id EntityID) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, ok := w.NPCs[id]
	if !ok || !npc.Temporary {
		return "", false
	}
	w.removeNPCLocked(npc)
	return npc.Name, true
}

func (w *World) removeNPCLocked(npc *NPC) {
	w.SpatialIndex.Remove(npc.ServerID, npc.X, npc.Z)
	delete(w.NPCs, npc.ServerID)
}

// Place is a travel NPC's spot under the name players know it by: a
// coachman's town ("Neriak") or a harbormaster's or gatekeeper's route.
type Place struct {
	Name            string
	NPC             string
	World           uint8
	X, Y, Z, Facing float32
}

// Places lists the coachmen's and the cross-world travel NPCs' spots by
// their labels (coachLocationNames, warpLocationNames), sorted by name. A
// label with no NPC loaded is not a place.
func (w *World) Places() []Place {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Place
	for _, npc := range w.NPCs {
		label, ok := coachLocationNames[npc.Name]
		if !ok {
			label, ok = warpLocationNames[npc.Name]
		}
		if !ok || npc.Temporary {
			continue
		}
		out = append(out, Place{Name: label, NPC: npc.Name, World: npc.World,
			X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Facing: npc.SpawnFacing})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].NPC < out[j].NPC
	})
	return out
}

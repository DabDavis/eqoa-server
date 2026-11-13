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
	if id, ok := ParseRowKey(name); ok {
		// "#n": that npcs row, whatever its name (npcrow.go)
		if npc := w.NPCs[id]; npc != nil && !npc.Temporary {
			return npc
		}
		return nil
	}
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

// npcByNameForLocked is the original a copy of the NPC named name put in world
// is of: one standing in that world before one of another, then
// npcByNameLocked's order (a live one, the lowest id); never a dungeon's
// template (dungeons.go, world 14's held-out rows: their level 60-65 and their
// "dungeon trash/named/boss" template by row) for a copy outside the
// dungeons' world while any other has the name. Until 2026-11-13 the copy was
// of npcByNameLocked's pick whatever its world: a quest's spawn of
// Necromancer Warpheus, Ancient Death, Skahyir or Lady Vox's Voxite mage, whose
// open world original was dead, was of the dungeon's copy, a level 65 boss at
// five times the hit points, in the middle of a level 40 quest (petition 1886's
// walk of the stat quests: Sage's Wit's third cases). "#n" is that row, as
// npcByNameLocked's. Must be called with the lock held.
func (w *World) npcByNameForLocked(name string, world uint8) *NPC {
	if _, ok := ParseRowKey(name); ok {
		return w.npcByNameLocked(name)
	}
	rank := func(npc *NPC) int {
		r := 0
		if w.dungeonTemplateLocked(npc) && world != npc.World {
			r += 4 // a dungeon's template for a copy outside its world: the last
		}
		if npc.World != world {
			r += 2
		}
		if npc.Dead {
			r++
		}
		return r
	}
	var best *NPC
	bestRank := 0
	for _, npc := range w.NPCs {
		if npc.Temporary || !strings.EqualFold(npc.Name, name) {
			continue
		}
		r := rank(npc)
		if best == nil || r < bestRank || r == bestRank && npc.ServerID < best.ServerID {
			best, bestRank = npc, r
		}
	}
	return best
}

// SpawnNPCCopy places a copy of the NPC named name (as npcByNameForLocked picks it)
// at (x, y, z) in world, facing facing: its model, size, gear, level and
// faction, full health, no fight, in view (an original that stands out of
// the world, spawnonly.go, is copied as any other: the copy is in it). The
// copy is Temporary: no database row, gone after its corpse time once
// killed. It returns the copy's id and the name as the template spells it.
func (w *World) SpawnNPCCopy(name string, x, y, z, facing float32, world uint8) (EntityID, string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.npcByNameForLocked(name, world)
	if t == nil {
		return 0, "", false
	}
	c := w.spawnCopyLocked(t, x, y, z, facing, world)
	return c.ServerID, t.Name, true
}

// spawnCopyLocked is SpawnNPCCopy's copy of t, put in the world. Must be called
// with the lock held.
func (w *World) spawnCopyLocked(t *NPC, x, y, z, facing float32, world uint8) *NPC {
	c := w.spawnCopyAsLocked(t, x, y, z, facing, world, 0)
	// a copy of a row that walks a path walks it too, from its nearest leg (the
	// Trial of Contention's Watchman, spawned by Kithgrin's talk on the row's
	// round of the halls: npcrow.go); dropNPCLocked takes it with the copy
	if p := w.paths.byNPC[t.ServerID]; p != nil && p.World == world && !t.Temporary {
		w.setPathLocked(c.ServerID, p)
	}
	return c
}

// spawnCopyAsLocked is spawnCopyLocked's copy under the id given (0: a free
// one; keepcopy.go's corpses ask for their own), walking no path. Must be
// called with the lock held.
func (w *World) spawnCopyAsLocked(t *NPC, x, y, z, facing float32, world uint8, id EntityID) *NPC {
	c := *t
	c.CopyOf = t.row() // a copy of a copy is of the same row (npcrow.go)
	if id == 0 {
		id = w.allocFreeIDLocked()
	}
	c.ServerID = id
	c.X, c.Y, c.Z, c.Facing = x, y, z, facing
	c.SpawnX, c.SpawnY, c.SpawnZ, c.SpawnFacing = x, y, z, facing
	c.World = world
	c.HP = c.MaxHP
	c.Dead = false
	c.DeathTime = time.Time{}
	c.CorpseGone = time.Time{}
	c.corpseHeld = time.Time{}
	c.stopMoving()
	c.roam = roamState{}
	c.spots = nil // its own, round its own spawn
	c.Threats = nil
	c.CurrentEnemy = 0
	c.castsAt = 0 // a trigger's sic sets its own (gmtriggers.go)
	c.LastCombat = time.Time{}
	c.LastCombatTime = time.Time{}
	// none of the original's spells: the copy shared their list, a body a
	// thrown potion put on the original among them (illusion.go), and kept
	// them past their time (the effect system ends only those of a creature
	// a spell landed on: cast.go's affected)
	c.Effects = nil
	c.Temporary = true
	c.SpawnOnly, c.HiddenUntil = false, time.Time{} // in every view near it
	c.endDummyFight()
	c.tfight = nil    // its template's fight is its own (classtemplate.go)
	c.challenge = nil // a duel's is its own (metalmoogie.go)
	c.path = pathState{}
	w.putNPCLocked(&c)
	w.SpatialIndex.Insert(c.ServerID, x, z)
	return &c
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
	w.dropNPCLocked(npc)
}

// NPCsNamed is the spawn spot of every NPC (not a GM's temporary copy) whose
// name match takes, as Places' Place with the NPC's name for both names,
// sorted by name and then id: what a place named by its people resolves over
// (handlers' namesakes: "guk" is where the "froglok guk ..." stand).
func (w *World) NPCsNamed(match func(name string) bool) []Place {
	w.mu.RLock()
	defer w.mu.RUnlock()
	type spot struct {
		Place
		id EntityID
	}
	var spots []spot
	for _, npc := range w.NPCs {
		if npc.Temporary || !match(npc.Name) {
			continue
		}
		spots = append(spots, spot{Place{Name: npc.Name, NPC: npc.Name, World: npc.World,
			X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Facing: npc.SpawnFacing}, npc.ServerID})
	}
	sort.Slice(spots, func(i, j int) bool {
		if spots[i].Name != spots[j].Name {
			return spots[i].Name < spots[j].Name
		}
		return spots[i].id < spots[j].id
	})
	out := make([]Place, len(spots))
	for i, s := range spots {
		out[i] = s.Place
	}
	return out
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
// their labels (coachLabel, warpLocationNames), sorted by name, of the
// worlds loadable takes (nil: all of them). A label with no NPC loaded is not
// a place.
func (w *World) Places(loadable func(world uint8) bool) []Place {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Place
	for _, npc := range w.NPCs {
		label, ok := warpLocationNames[npc.Name]
		if IsCoachman(npc.Name) {
			label, ok = coachLabel(npc)
		}
		if !ok || npc.Temporary {
			continue
		}
		if loadable != nil && !loadable(npc.World) {
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

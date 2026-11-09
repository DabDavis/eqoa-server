package entity

import "sort"

// What the Teleporter (handlers/teleporter.go) reads of the world to say where
// it can send a player: the spawns of the NPCs that make its raids and camps,
// with their levels, and the GMs' camps as they stand.

// NPCSpawn is an NPC's spawn spot and level.
type NPCSpawn struct {
	Name    string
	World   uint8
	X, Y, Z float32
	Level   int
}

// NPCSpawnsNamed is the spawn of every NPC (not a GM's temporary copy) whose
// name match takes, kept out of the world (spawnonly.go) or dead or not, in no
// order: one pass over the NPCs for every name the caller looks for.
func (w *World) NPCSpawnsNamed(match func(name string) bool) []NPCSpawn {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []NPCSpawn
	for _, npc := range w.NPCs {
		if npc.Temporary || !match(npc.Name) {
			continue
		}
		out = append(out, NPCSpawn{Name: npc.Name, World: npc.World, X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Level: npc.Level})
	}
	return out
}

// GMCampSpot is a GM's camp (campspawn.go: a /camp's static spawns) as it
// stands: its world, the middle of its spawns (y their mean), the kinds it
// pops by name in the order its spawns list them, and their levels (the rows
// they are made from).
type GMCampSpot struct {
	Camp               int64
	World              uint8
	X, Y, Z            float32
	Kinds              []string
	MinLevel, MaxLevel int
}

// GMCampSpots are the GMs' camps the world holds (LoadCampSpawns), by camp
// and world, in the camps' order.
func (w *World) GMCampSpots() []GMCampSpot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ids := make([]EntityID, 0, len(w.camps.spawns))
	for id := range w.camps.spawns {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	type key struct {
		camp  int64
		world uint8
	}
	byCamp := map[key]*GMCampSpot{}
	n := map[key]int{}
	var order []key
	for _, id := range ids {
		cs := w.camps.spawns[id]
		own := w.camps.recs[int(id)]
		if own == nil {
			continue
		}
		k := key{cs.camp, uint8(own.rec.World)}
		s := byCamp[k]
		if s == nil {
			s = &GMCampSpot{Camp: cs.camp, World: k.world}
			byCamp[k] = s
			order = append(order, k)
		}
		s.X += own.rec.XCoord
		s.Y += own.rec.YCoord
		s.Z += own.rec.ZCoord
		n[k]++
		for _, kid := range cs.kinds {
			r := w.camps.recs[kid]
			if r == nil {
				continue
			}
			seen := false
			for _, name := range s.Kinds {
				seen = seen || sameNPCName(name, r.rec.NPCName)
			}
			if !seen {
				s.Kinds = append(s.Kinds, r.rec.NPCName)
			}
			if lv := r.rec.NPCLevel; lv > 0 {
				if s.MinLevel == 0 || lv < s.MinLevel {
					s.MinLevel = lv
				}
				s.MaxLevel = max(s.MaxLevel, lv)
			}
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].camp != order[j].camp {
			return order[i].camp < order[j].camp
		}
		return order[i].world < order[j].world
	})
	out := make([]GMCampSpot, 0, len(order))
	for _, k := range order {
		s := byCamp[k]
		f := float32(n[k])
		s.X, s.Y, s.Z = s.X/f, s.Y/f, s.Z/f
		out = append(out, *s)
	}
	return out
}

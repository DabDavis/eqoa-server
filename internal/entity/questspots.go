package entity

import (
	"sort"
	"strings"
)

// NPCSpot is where an NPC of a name stands: its entity, world and spawn, for
// the quest marks a client draws on its map (handlers/questmarks.go).
type NPCSpot struct {
	ID    EntityID
	World uint8
	X, Z  float32
	Name  string
}

// NPCSpotsNamed is the spawns of the NPCs of each name in names (trimmed and
// lowered, as the quests match a name), by that name: the World's own NPCs,
// not a GM's copies or pets, each name's in entity order.
func (w *World) NPCSpotsNamed(names map[string]bool) map[string][]NPCSpot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := map[string][]NPCSpot{}
	for _, n := range w.NPCs {
		if n.Temporary || n.Owner != 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(n.Name))
		if !names[k] {
			continue
		}
		out[k] = append(out[k], NPCSpot{ID: n.ServerID, World: n.World, X: n.SpawnX, Z: n.SpawnZ, Name: n.Name})
	}
	for k := range out {
		s := out[k]
		sort.Slice(s, func(i, j int) bool { return s[i].ID < s[j].ID })
	}
	return out
}

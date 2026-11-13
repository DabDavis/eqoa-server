package entity

import (
	"fmt"
	"math"
	"sort"

	"github.com/eqoa/server/internal/database"
)

// Walls a quest lets a character through, and what a character holds that a
// creature's aggro looks at (the Cazic-Thule trials, 2026-11-11; work board
// 430's code ask 5, 656's ask 1).
//
// A wall is an NPC (an invisible blocker's body: CHAR.ESF 0x6F412342, a box
// CLIENT stops a walk at, drawn as nothing: claude-notes/invisible-blockers-
// 20261029.md) of table questwalls, by its npcs row: in the view, so solid, to
// every character but those past a step of its quest (database.QuestWall: the
// quest finished, or under way at a step after Step), to whom it is in no
// view at all, so CLIENT makes no actor of it and the walk goes through. "Each
// completed trial lets the questor pass a new invisible wall" (the t196
// walkthrough); "logging off and on to run through walls can be done also":
// so it was the client's collision, as here. A wall near a player is in its
// view before any other entity (visibilitySystem's pins), so a crowd round it
// never pushes it out of the slots.
//
// The quest engine (handlers) holds the characters' quests and items: it
// tells the World each second which walls each character in the world passes
// and which of the watched items (npcaggro's holding) it holds
// (SetQuestState), since the World's lock may not wait on the engine's.

// questWallPin is how near a player a wall must stand to be pinned into its
// view before nearer entities (visibilitySystem).
const questWallPin float32 = 120

// questWallState is the World's walls and what each character passes and holds.
type questWallState struct {
	walls  map[EntityID]database.QuestWall
	passes map[uint32]map[EntityID]bool // by character id
	holds  map[uint32]map[int]bool      // by character id: the watched items it holds
}

// SetQuestWalls sets table questwalls' rows (at the start and after a GM's edit).
func (w *World) SetQuestWalls(rows []database.QuestWall) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.qwalls.walls = make(map[EntityID]database.QuestWall, len(rows))
	for _, r := range rows {
		w.qwalls.walls[EntityID(r.NPCID)] = r
	}
}

// LoadQuestWalls reads table questwalls into the world; it reports how many.
func (w *World) LoadQuestWalls(db *database.SQLiteDB) (int, error) {
	rows, err := db.QuestWalls()
	if err != nil {
		return 0, fmt.Errorf("questwalls: %w", err)
	}
	w.SetQuestWalls(rows)
	return len(rows), nil
}

// QuestWalls is every wall, by npcs row.
func (w *World) QuestWalls() []database.QuestWall {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]database.QuestWall, 0, len(w.qwalls.walls))
	for _, q := range w.qwalls.walls {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NPCID < out[j].NPCID })
	return out
}

// WatchedItems is every item a creature's aggro looks for (npcaggro's
// holding) and every item a wall opens for (questwalls' item).
func (w *World) WatchedItems() []int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	seen := map[int]bool{}
	var out []int
	for _, r := range w.aggro.rules {
		if r.Holding > 0 && !seen[r.Holding] {
			seen[r.Holding] = true
			out = append(out, r.Holding)
		}
	}
	for _, q := range w.qwalls.walls {
		if q.Item > 0 && !seen[q.Item] {
			seen[q.Item] = true
			out = append(out, q.Item)
		}
	}
	sort.Ints(out)
	return out
}

// SetQuestState sets the walls a character passes and the watched items it
// holds (nil none), as the quest engine reads them; a character gone from the
// world is forgotten with ForgetQuestState.
func (w *World) SetQuestState(charID uint32, passes map[EntityID]bool, holds map[int]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.qwalls.passes == nil {
		w.qwalls.passes = map[uint32]map[EntityID]bool{}
		w.qwalls.holds = map[uint32]map[int]bool{}
	}
	w.qwalls.passes[charID] = passes
	w.qwalls.holds[charID] = holds
}

// ForgetQuestState drops what SetQuestState kept of a character.
func (w *World) ForgetQuestState(charID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.qwalls.passes, charID)
	delete(w.qwalls.holds, charID)
}

// isQuestWallLocked is whether the NPC is a wall (a copy of none).
func (w *World) isQuestWallLocked(npc *NPC) bool {
	if npc.Temporary || len(w.qwalls.walls) == 0 {
		return false
	}
	_, ok := w.qwalls.walls[npc.ServerID]
	return ok
}

// wallPassedLocked is whether p walks through the wall npc: out of its view.
// The quest engine says who passes by a quest or an item held
// (SetQuestState); a wall of an item lets a GM through, and anyone standing on
// its far side (its inner boxes), so the way back is always open.
func (w *World) wallPassedLocked(npc *NPC, p *Player) bool {
	if !w.isQuestWallLocked(npc) {
		return false
	}
	if w.qwalls.passes[p.CharServerID][npc.ServerID] {
		return true
	}
	wl := w.qwalls.walls[npc.ServerID]
	return wl.Item > 0 && (p.GM || insideWall(wl, npc, p))
}

// insideWall is whether p stands on the wall's far side: in one of its inner
// boxes, in its world.
func insideWall(wl database.QuestWall, npc *NPC, p *Player) bool {
	if p.World != npc.World {
		return false
	}
	for _, b := range wl.Inner {
		if b.Holds(p.X, p.Y, p.Z) {
			return true
		}
	}
	return false
}

// WallStop is a wall of an item near a player that stops it: the wall's npcs
// row, how far off its middle the player stands, and the wall's line.
type WallStop struct {
	NPCID int
	Dist  float32
	Text  string
}

// ItemWallsStopping are the walls of an item within reach of p that it does
// not pass (wallPassedLocked): the quest engine tells it a wall's line once as
// it comes up against it (handlers/questwalls.go).
func (w *World) ItemWallsStopping(p *Player, reach float32) []WallStop {
	if p == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []WallStop
	for id, wl := range w.qwalls.walls {
		if wl.Item <= 0 {
			continue
		}
		n := w.NPCs[id]
		if n == nil || n.World != p.World {
			continue
		}
		dx, dz := n.X-p.X, n.Z-p.Z
		d := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		if d > reach || w.wallPassedLocked(n, p) {
			continue
		}
		out = append(out, WallStop{NPCID: int(id), Dist: d, Text: wl.Text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NPCID < out[j].NPCID })
	return out
}

// holdsLocked is whether p holds the item (one watched: SetQuestState).
func (w *World) holdsLocked(p *Player, pattern int) bool {
	return w.qwalls.holds[p.CharServerID][pattern]
}

// questHoldReach is how near a creature whose aggro looks for an item a player
// must be for the engine to look at what it holds (NearHoldingRule).
const questHoldReach float32 = 150

// NearHoldingRule is whether p stands within questHoldReach of a creature
// whose aggro looks for an item (npcaggro's holding) or of a wall an item
// opens (questwalls' item).
func (w *World) NearHoldingRule(p *Player) bool {
	if p == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for id, wl := range w.qwalls.walls {
		if wl.Item <= 0 {
			continue
		}
		if n := w.NPCs[id]; n != nil && n.World == p.World {
			dx, dz := n.X-p.X, n.Z-p.Z
			if dx*dx+dz*dz <= questHoldReach*questHoldReach {
				return true
			}
		}
	}
	if w.SpatialIndex == nil {
		return false
	}
	for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, questHoldReach) {
		n := w.NPCs[id]
		if n == nil || n.World != p.World || n.Dead {
			continue
		}
		if r, ok := w.aggro.ruleFor(n.Name); ok && r.Holding > 0 {
			return true
		}
	}
	return false
}

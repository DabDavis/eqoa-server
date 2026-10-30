package entity

import (
	"fmt"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The NPCs that stand in no one's world (table npcspawnonly, by name): the
// original a quest step's spawn event copies (questhooks.go's SpawnForQuest),
// and a GM trigger's or a GM's #spawn, and nothing else. The user, playing a
// level 10 quest at Jarek N`Toxis's hut, 2026-10-30: "his creation is already
// spawned waiting, but should spawn after I complete the steps to bring him the
// crab and fungus cockroach parts. he then makes his creation and thats when
// you fight it". A spawn copies a LIVE NPC of its name (devspawn.go's
// npcByNameLocked), so tools/seed_quest_npcs.py stood one in the world for each
// creature a quest spawns, where FrontiersForge saw it or the guide put it, and
// there it waited for anyone to kill, beside the copy the quest made.
//
// Such an NPC is loaded as any other and kept out of the world by the test that
// keeps out one the GM panel is remaking (hiddenAt; HiddenUntil, gmlive.go): it
// is in no view (visibility.go), so no client can target, fight, talk to or
// cast at it; it attacks no one on sight (aggro.go), comes to no one's help
// (socialaggro.go), is in no area spell or thrown potion's splash (area.go,
// potion.go), and never wanders (movement.go). It is a field of its own, not a
// far HiddenUntil: the panel's edit of its row sets that hold afresh (UpsertNPC,
// ReloadNPCs) and the NPC would be back in view 1.2 s later. A copy
// (SpawnNPCCopy, and a pet made from its body, pet.go) is neither.
//
// Where it stands still counts: the quest marks point there (questspots.go),
// and a player on a kill or loot step who lost the step's spawn of it (its
// copy's ten minutes up, its corpse gone unlooted, a logout) gets the step's
// spawns again on coming near it (handlers/questevents.go).

// hiddenAt reports an NPC kept out of every view at now: spawn-only, or the
// panel's refresh hold not over.
func (n *NPC) hiddenAt(now time.Time) bool {
	return n.SpawnOnly || now.Before(n.HiddenUntil)
}

// spawnOnlyLocked is whether the NPCs of that name stand out of the world.
// Must be called with the lock held.
func (w *World) spawnOnlyLocked(name string) bool {
	return w.spawnOnly[database.NPCClassKey(name)]
}

// SetSpawnOnly sets table npcspawnonly's names (keyed as database.NPCClassKey
// keys a name) and applies them to every NPC a row made: one newly spawn-only
// is put back on its spawn, out of any fight, and leaves every view at the next
// visibility round; one no longer is comes back into them. A GM's copy and a
// pet are never spawn-only, and a charmed NPC is left as it is until it is read
// again. It reports how many NPCs are spawn-only now, and how many changed.
func (w *World) SetSpawnOnly(names map[string]bool) (held, changed int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.spawnOnly = names
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Owner != 0 {
			continue
		}
		want := names[database.NPCClassKey(npc.Name)]
		if want {
			held++
		}
		if npc.SpawnOnly == want {
			continue
		}
		npc.SpawnOnly = want
		changed++
		if want && !npc.Dead {
			// as RespawnNPCNow (gmlive.go): no fight goes on out of sight
			w.respawnNPCLocked(npc)
			npc.Effects = nil
			npc.endDummyFight()
			delete(w.affected, npc.ServerID)
		}
	}
	return held, changed
}

// LoadSpawnOnly reads table npcspawnonly into the world: at the start and after
// a GM's edit. Call it without the database held. It reports SetSpawnOnly's
// counts.
func (w *World) LoadSpawnOnly(db *database.SQLiteDB) (held, changed int, err error) {
	names, err := db.NPCSpawnOnly()
	if err != nil {
		return 0, 0, fmt.Errorf("npcspawnonly: %w", err)
	}
	held, changed = w.SetSpawnOnly(names)
	return held, changed, nil
}

// SpawnOnlySummary says what stands out of the world, for the log at the start.
func (w *World) SpawnOnlySummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	n := 0
	for _, npc := range w.NPCs {
		if npc.SpawnOnly {
			n++
		}
	}
	return fmt.Sprintf("%d NPCs of %d names stand out of the world, there only for the spawns that copy them (npcspawnonly)",
		n, len(w.spawnOnly))
}

// SpawnOnlyNear reports a spawn-only NPC of one of names (trimmed, any case)
// within r of (x, z) in world: where a quest step's lost spawn comes again
// (handlers/questevents.go).
func (w *World) SpawnOnlyNear(world uint8, x, z, r float32, names []string) bool {
	if len(names) == 0 || w.SpatialIndex == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, id := range w.SpatialIndex.QueryRadius(x, z, r) {
		npc := w.NPCs[id]
		if npc == nil || !npc.SpawnOnly || npc.World != world {
			continue
		}
		for _, n := range names {
			if strings.EqualFold(strings.TrimSpace(npc.Name), strings.TrimSpace(n)) {
				return true
			}
		}
	}
	return false
}

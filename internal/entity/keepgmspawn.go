package entity

// keepgmspawn.go — a GM's living spawns through a restart that keeps the sessions
// (the owner, 2026-11-18: "if a gm spawns a mob the rolling restart shouldnt
// remove that spawn"). A GM's spawn (#spawn, the panel's spawn_npc: SpawnGMCopy,
// GMSpawn) is a copy no row makes again, which keepnpc.go passes over, so until
// this a keep restart took every one away, a raid boss a GM had put up for a
// fight among them.
//
// Now each living one is written (KeepGMSpawns) as keeptrigger.go writes a
// trigger's spawn: the row it is a copy of, its name, level, hit points' maximum,
// flags and size as the copy had them, the GM's spawn whose companion it is
// (gmcompanions.go), and its state as keepnpc.go carries a row's creature
// (keptStateLocked: its place, hit points, way home from a fight, path,
// cooldowns). The new process makes each again from its row (RestoreGMSpawns),
// on its own spawn (where the GM put it), under its old id when free, and gives it
// what was kept as restoreNPCLocked gives a row's creature: alive where it stood
// at its hit points; a fight is not carried (threats, the round), so one fighting
// runs back to where that fight began at its hit points. A companion is paired
// with its owner's copy again. Its clients' slots go on describing it
// (keptEvent: keep.go's RestoreView). A GM's spawn has no time of its own to go:
// it stays until killed or #despawn, as before the restart.
//
// A dead one is not written here: its corpse, if it lies, is keepcopy.go's, as
// every copy's (a companion laid down with its owner leaves a corpse of nothing,
// carried by nothing). Nor one charmed by a player (it is the player's pet while
// the charm holds, and a charmed copy has no row to come back to: gone, as
// before), nor one in an instance.
//
// A trigger's spawn is its run's (keeptrigger.go) and a quest's its quest's
// (keepcopy.go's doc): neither is GMSpawn, and neither is carried here.

import (
	"log"
	"strings"
	"time"
)

// KeptGMSpawn is a GM's living spawn as a restart carries it.
type KeptGMSpawn struct {
	KeptNPC
	Row     uint32  `json:"row"` // the row it is a copy of (CopyOf)
	Level   int     `json:"level"`
	FullHP  int     `json:"full_hp"` // its hit points' maximum
	NPCType int     `json:"npc_type"`
	Size    float32 `json:"size,omitempty"`
	// ScaleBase is its body's own size under a GM's /size (gmscale.go), 0 none
	ScaleBase float32 `json:"scale_base,omitempty"`
	Event     bool    `json:"event,omitempty"`  // EventAggro
	NoLoot    bool    `json:"noloot,omitempty"` // a corpse of nothing
	// CompanionOf is the GM's spawn (its old id) whose companion it is, 0 none
	CompanionOf uint32 `json:"companion_of,omitempty"`
}

// keepsGMSpawn reports a GM's spawn a restart carries alive (the file's doc).
func keepsGMSpawn(c *NPC) bool {
	return c.GMSpawn && c.Temporary && c.CopyOf != 0 && !c.Dead && c.Owner == 0 && c.Charm == nil && c.Instance == 0
}

// KeepGMSpawns is every GM's living spawn as a restart carries it (the file's
// doc), in no order. The listener is paused.
func (w *World) KeepGMSpawns(now time.Time) []KeptGMSpawn {
	w.mu.Lock() // not RLock: as KeepNPCs
	defer w.mu.Unlock()
	var out []KeptGMSpawn
	for _, c := range w.NPCs {
		if !keepsGMSpawn(c) {
			continue
		}
		kn, ok := w.keptStateLocked(c, now, true)
		if !ok {
			continue
		}
		out = append(out, KeptGMSpawn{KeptNPC: kn, Row: uint32(c.CopyOf), Level: c.Level, FullHP: c.MaxHP, NPCType: c.NPCType,
			Size: c.Size, ScaleBase: c.gmScaleBase, Event: c.EventAggro, NoLoot: c.NoLoot, CompanionOf: uint32(c.gmCompanionOf)})
	}
	return out
}

// RestoreGMSpawns makes the kept spawns again (the file's doc), once the rows are
// loaded, the creatures restored and the copies' corpses made, before anything
// else takes an id (a pet, a merc). It returns each made one's old id's new id;
// one whose row is gone (or another creature's now) is not made, and counted
// lost.
func (w *World) RestoreGMSpawns(kept []KeptGMSpawn) (ids map[EntityID]EntityID, lost int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids = make(map[EntityID]EntityID, len(kept))
	made := make([]*NPC, len(kept))
	var nr NPCsRestored
	for i := range kept {
		k := kept[i]
		t := w.NPCs[EntityID(k.Row)]
		if t == nil || t.Temporary || !strings.EqualFold(t.Name, k.Name) {
			log.Printf("Keep: GM's spawn %q (copy %d of row %d) not made again: its row is gone or another's", k.Name, k.ID, k.Row)
			lost++
			continue
		}
		want := EntityID(k.ID)
		if _, taken := w.NPCs[want]; taken {
			want = 0
		} else if _, taken := w.Players[want]; taken {
			want = 0
		}
		// as SpawnGMCopy made it, on its own spawn (and its row's path) ...
		c := w.spawnCopyAsLocked(t, k.Spawn[0], k.Spawn[1], k.Spawn[2], k.Spawn[3], k.World, want)
		if p := w.paths.byNPC[t.ServerID]; p != nil && p.World == k.World {
			w.setPathLocked(c.ServerID, p)
		}
		c.Name, c.NPCType, c.EventAggro, c.NoLoot = k.Name, k.NPCType, k.Event, k.NoLoot
		if k.Level > 0 {
			c.Level = k.Level
		}
		if k.FullHP > 0 {
			c.MaxHP = k.FullHP
		}
		if k.Size > 0 {
			c.Size = k.Size
		}
		c.gmScaleBase = k.ScaleBase
		c.HP = c.MaxHP
		c.GMSpawn, c.keptEvent = true, true
		// ... then as it was (keepnpc.go)
		w.restoreNPCLocked(c, &k.KeptNPC, true, &nr)
		ids[EntityID(k.ID)] = c.ServerID
		made[i] = c
	}
	// the pairs again, by the new ids
	paired := false
	for i, c := range made {
		if c == nil || kept[i].CompanionOf == 0 {
			continue
		}
		if o, ok := ids[EntityID(kept[i].CompanionOf)]; ok {
			c.gmCompanionOf = o
			paired = true
		}
	}
	if paired {
		w.companionOf = nil // read again (linkCompanionsLocked)
	}
	return ids, lost
}

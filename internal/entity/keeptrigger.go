package entity

// keeptrigger.go — a GM trigger's living spawns through a restart that keeps the
// sessions (the owner, 2026-11-13, after a player lost two Rallos Zek fights to
// server updates: a scripted encounter survives a keep restart as a dungeon's
// instance does, keepdungeon.go). The run itself (its step, lock, clock, waits)
// is the handlers' (handlers/gmtriggerkeep.go); this is its creatures.
//
// A trigger's spawn is a copy (Temporary, CopyOf: SpawnEventNPC) no row makes
// again, which keepnpc.go passes over, so until this a keep restart left the
// run's lock and clock in nothing (the run was not carried either) and the boss
// gone. Now each living spawn a run names is written (KeepEventNPCs): the row it
// is a copy of, its name, level, hit points' maximum and flags as the copy had
// them (a hostile one's cleared NPC bit and EventAggro; an attackable one's bit),
// and its state as keepnpc.go carries a row's creature (keptStateLocked: its
// place, hit points, way home from a fight, path, cooldowns), with the NPC a sic
// set it on (its enemy when that is an NPC, and whether it fights it with its
// class template's casts: SetNPCOn).
//
// The new process makes each again from its row (RestoreEventNPCs), on its own
// spawn (where the step placed it), under its old id when free, then gives it
// what was kept as restoreNPCLocked gives a row's creature: alive where it
// stood at its hit points (the same share of a maximum a data step changed). One
// that was set on an NPC still standing is set on it again (a threat, as the sic
// left), not run home; a fight with players is not carried (threats on a
// player, the round), so one fighting players runs back to where that fight
// began at its hit points, and a hostile one attacks on sight again. Its client
// slots keep describing it (keptEvent: keep.go's RestoreView).
//
// A dead spawn is not written here: its corpse, if it lies, is keepcopy.go's
// (and the run finds it by the id RestoreCopies returns).

import (
	"log"
	"strings"
	"time"
)

// KeptEventNPC is a trigger's living spawn as a restart carries it.
type KeptEventNPC struct {
	KeptNPC
	Row     uint32 `json:"row"` // the row it is a copy of (CopyOf)
	Level   int    `json:"level"`
	FullHP  int    `json:"full_hp"` // its hit points' maximum
	NPCType int    `json:"npc_type"`
	Event   bool   `json:"event,omitempty"` // EventAggro: attacks any player on sight
	// Enemy is the NPC a sic set it on (0 none), Casts whether it fights it by
	// its class template's casts too
	Enemy uint32 `json:"enemy,omitempty"`
	Casts bool   `json:"casts,omitempty"`
}

// KeepEventNPCs is the living copies among ids (a trigger's runs' spawns) as a
// restart carries them (the file's doc). The listener is paused.
func (w *World) KeepEventNPCs(ids []EntityID, now time.Time) []KeptEventNPC {
	w.mu.Lock() // not RLock: as KeepNPCs
	defer w.mu.Unlock()
	var out []KeptEventNPC
	seen := map[EntityID]bool{}
	for _, id := range ids {
		c := w.NPCs[id]
		if c == nil || seen[id] || !c.Temporary || c.CopyOf == 0 || c.Dead || c.Owner != 0 || c.Charm != nil || c.Instance != 0 {
			continue
		}
		seen[id] = true
		kn, ok := w.keptStateLocked(c, now, true)
		if !ok {
			continue
		}
		k := KeptEventNPC{KeptNPC: kn, Row: uint32(c.CopyOf), Level: c.Level, FullHP: c.MaxHP, NPCType: c.NPCType, Event: c.EventAggro}
		if e := w.NPCs[c.CurrentEnemy]; c.CurrentEnemy != 0 && e != nil && !e.Dead {
			k.Enemy = uint32(c.CurrentEnemy)
			k.Casts = c.castsAt == c.CurrentEnemy
		}
		out = append(out, k)
	}
	return out
}

// RestoreEventNPCs makes the kept spawns again (the file's doc), once the rows
// are loaded and the creatures and copies' corpses restored, before the sessions
// are put back. It returns each made one's old id's new id; one whose row is gone
// (or another creature's now) is not made, and counted lost.
func (w *World) RestoreEventNPCs(kept []KeptEventNPC) (ids map[EntityID]EntityID, lost int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids = make(map[EntityID]EntityID, len(kept))
	var nr NPCsRestored
	for i := range kept {
		k := kept[i]
		t := w.NPCs[EntityID(k.Row)]
		if t == nil || t.Temporary || !strings.EqualFold(t.Name, k.Name) {
			log.Printf("Keep: trigger spawn %q (copy %d of row %d) not made again: its row is gone or another's", k.Name, k.ID, k.Row)
			lost++
			continue
		}
		want := EntityID(k.ID)
		if _, taken := w.NPCs[want]; taken {
			want = 0
		} else if _, taken := w.Players[want]; taken {
			want = 0
		}
		// as SpawnEventNPC made it, on its own spawn (and its row's path) ...
		c := w.spawnCopyAsLocked(t, k.Spawn[0], k.Spawn[1], k.Spawn[2], k.Spawn[3], k.World, want)
		if p := w.paths.byNPC[t.ServerID]; p != nil && p.World == k.World {
			w.setPathLocked(c.ServerID, p)
		}
		c.Name, c.NPCType, c.EventAggro = k.Name, k.NPCType, k.Event
		if k.Level > 0 {
			c.Level = k.Level
		}
		if k.FullHP > 0 {
			c.MaxHP = k.FullHP
		}
		c.HP = c.MaxHP
		c.keptEvent = true
		// ... then as it was (keepnpc.go); one set on an NPC still standing is set on it again, not sent home
		enemy := w.NPCs[EntityID(k.Enemy)]
		if k.Enemy == 0 || enemy == nil || enemy.Dead {
			enemy = nil
		} else {
			k.Home = nil
		}
		w.restoreNPCLocked(c, &k.KeptNPC, true, &nr)
		if enemy != nil {
			w.addThreatLocked(c, enemy.ServerID, 1)
			c.CurrentEnemy = enemy.ServerID
			if k.Casts {
				c.castsAt = enemy.ServerID
			}
		}
		ids[EntityID(k.ID)] = c.ServerID
	}
	return ids, lost
}

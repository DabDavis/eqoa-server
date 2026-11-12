package entity

// keepcopy.go — the corpses of the copies through a restart that keeps the
// sessions (the owner, 2026-11-12: "the rolling reset is deleting loot on mobs").
// keepnpc.go carries the creatures of rows; a copy (Temporary, CopyOf: a quest
// step's spawn, a GM trigger's, a GM's /spawn, a creature's summoned add) has no
// row, so the new process had nothing to make it from and its corpse was simply
// gone, with what was on it: Graal the Leviathan, a quest step's spawn, dies at
// 08:07 and lies to be looted; a restart in that minute took the corpse, its
// items and the coins no one had taken yet.
//
// Now a copy's corpse that lies is written down (KeptCopy: its row, where it
// lies, the death's and the corpse's times, its level, hit points and flags as the
// copy had them) and made again in the new process from its row as the copy was
// (spawnCopyLocked), dead, under its own id when that is free (it always is: a
// row's id is its row's, and nothing else is made before), else a new one
// (RestoreCopies returns the old ids' new ones for the corpses' contents,
// handlers/keep.go). Its corpse then decays and goes as a copy's does
// (corpseSystem). What it holds (rolled or not, its kill group's right to it, a
// raid's hold, a loot step's drops) is the handlers' and goes by the same id.
//
// Not carried: a LIVING copy. Each belongs to something a restart does not carry
// (a quest's spawn list, which makes a kill or loot step's lost spawn again when
// its player comes near: handlers/questevents.go's respawnLost; a GM trigger,
// which a restart stops; a creature's fight, whose adds go with it), so carried it
// would stand in the world for good, which nothing takes away. Nor a corpse of a
// copy in an instance (dungeons close at a restart, by design), of a siege's, of a
// Metal Moogie's challenge (its tribute is its duel's) or of a copy of no row.

import (
	"log"
	"strings"
	"time"
)

// KeptCopy is a copy's corpse as a restart carries it.
type KeptCopy struct {
	ID    uint32     `json:"id"`
	Row   uint32     `json:"row"` // the row it is a copy of (CopyOf)
	Name  string     `json:"name"`
	World uint8      `json:"world,omitempty"`
	Place [4]float32 `json:"place"` // x, y, z, facing
	Level int        `json:"level"`
	MaxHP int        `json:"max_hp"`
	// NPCType is its NPC flags (a hostile spawn's 0x80 cleared), Quest and Event
	// whether it was a quest's or a trigger's
	NPCType int  `json:"npc_type"`
	Quest   bool `json:"quest,omitempty"`
	Event   bool `json:"event,omitempty"`
	// Died and Gone are its death's and its corpse's going's times (zero while
	// it lies), CorpseHeld a raid's hold on it (HoldCorpse); Held a GM's hold
	// and GuardsKill whether the kill is the town guards'
	Died       time.Time `json:"died"`
	Gone       time.Time `json:"gone,omitzero"`
	CorpseHeld time.Time `json:"corpse_held,omitzero"`
	Held       bool      `json:"held,omitempty"`
	GuardsKill bool      `json:"guards_kill,omitempty"`
}

// keepsCopyCorpse reports a copy whose corpse a restart carries (the file's
// doc) at now.
func (w *World) keepsCopyCorpse(npc *NPC, now time.Time) bool {
	return npc.Temporary && npc.CopyOf != 0 && npc.Instance == 0 && npc.Owner == 0 && npc.Charm == nil &&
		npc.GenieOf == 0 && npc.PortalOf == 0 && npc.Dummy == NotDummy && npc.siege == nil && npc.challenge == nil &&
		npc.corpseLies(now, w.corpseDecay)
}

// KeepCopies is every copy's corpse that lies (the file's doc), in no order.
// The listener is paused.
func (w *World) KeepCopies() []KeptCopy {
	w.mu.RLock()
	defer w.mu.RUnlock()
	now := time.Now()
	var out []KeptCopy
	for _, npc := range w.NPCs {
		if !w.keepsCopyCorpse(npc, now) {
			continue
		}
		k := KeptCopy{ID: uint32(npc.ServerID), Row: uint32(npc.CopyOf), Name: npc.Name, World: npc.World,
			Place: [4]float32{npc.X, npc.Y, npc.Z, npc.Facing}, Level: npc.Level, MaxHP: npc.MaxHP,
			NPCType: npc.NPCType, Quest: npc.QuestSpawn, Event: npc.EventAggro,
			Died: npc.DeathTime, Gone: npc.CorpseGone, CorpseHeld: npc.corpseHeld, Held: npc.GMHeld, GuardsKill: npc.guardHit}
		if (&KeptNPC{Spawn: k.Place}).finite() {
			out = append(out, k)
		}
	}
	return out
}

// RestoreCopies makes the kept copies' corpses again (the file's doc), once
// the rows are loaded and before anything else is made (a pet, a merc): each
// under its old id when free. It returns each made one's old id's new id, and
// how many of the kept could not be made (its row gone, or another creature's
// now).
func (w *World) RestoreCopies(kept []KeptCopy) (ids map[EntityID]EntityID, lost int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids = make(map[EntityID]EntityID, len(kept))
	for _, k := range kept {
		t := w.NPCs[EntityID(k.Row)]
		if t == nil || t.Temporary || !strings.EqualFold(t.Name, k.Name) {
			log.Printf("Keep: the corpse of %q (copy %d of row %d) not made again: its row is gone or another's", k.Name, k.ID, k.Row)
			lost++
			continue
		}
		want := EntityID(k.ID)
		if _, taken := w.NPCs[want]; taken {
			want = 0
		} else if _, taken := w.Players[want]; taken {
			want = 0
		}
		c := w.spawnCopyAsLocked(t, k.Place[0], k.Place[1], k.Place[2], k.Place[3], k.World, want)
		c.Name, c.Level, c.NPCType = k.Name, k.Level, k.NPCType
		c.QuestSpawn, c.EventAggro = k.Quest, k.Event
		if k.MaxHP > 0 {
			c.MaxHP = k.MaxHP
		}
		c.HP, c.Dead = 0, true
		c.DeathTime, c.CorpseGone, c.corpseHeld = k.Died, k.Gone, k.CorpseHeld
		c.GMHeld, c.guardHit = k.Held, k.GuardsKill
		w.wakeNPCLocked(c) // the corpses' set, for its decay and its going
		ids[EntityID(k.ID)] = c.ServerID
	}
	return ids, lost
}

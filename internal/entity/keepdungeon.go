package entity

// keepdungeon.go — the dungeons' instances through a restart that keeps the
// sessions (the owner, 2026-11-13: "on server restart the instanced raids are
// booting players, lets keep them in the instance and progress"; dungeons.go).
//
// Until this an instance lived in memory alone: a keep restart lost every one,
// its creatures (Temporary copies, which keepnpc.go and keepcopy.go pass over)
// with it, and the players carried through came back standing in the dungeons'
// world in no instance, so dungeonsSystem sent them out after its grace.
//
// Now each open instance is written down (KeepDungeons): its id, its dungeon by
// KEY (a recipe reordered still maps), its owner, the owner's group, its times,
// how many creatures it opened with, the characters standing in it, and each of
// its creatures still in the world: the row it is a copy of and its state as
// keepnpc.go carries a row's creature (its place, hit points, way home, path,
// cooldowns), or, dead, its corpse that still lies (its death's and its going's
// times, a raid's hold). A creature killed whose corpse is gone is not written:
// that is the instance's progress ("xx of yy killed" counts it still, from the
// number it opened with). Also kept: the next instance's id and where each
// character in the dungeons came from (its way out).
//
// The new process makes each instance again under its id (RestoreDungeons),
// before the sessions are put back: each creature a copy of its row as the
// instance's opening made it (spawnCopyLocked, Instance), under its old id when
// free, then given what was kept as keepnpc.go gives a row's creature
// (restoreNPCLocked): alive where it stood at its hit points, or dead, its corpse
// lying, whose contents, kill group's right and raid's hold handlers/keep.go puts
// back by the ids returned. A character carried through standing in an instance
// is put back in it where it stood (resumeIn: CreatePlayer's
// dungeonInstanceOnLoadLocked, and handlers' dungeonLogin, which leaves it in
// place rather than at the entrance); the groups come back under their ids after
// the players (udp/keep.go), so an instance's group is its group again, and once
// they have (DungeonsResumed) every player in the dungeons is given the grace a
// move gets.
//
// Not carried, as for a row's creature: a fight (threats, the target, a
// template fight's clocks: one in a fight runs back to where it began), the
// spells on a creature. A dungeon gone from the recipe, or the dungeons off,
// leaves its instance gone; a row gone leaves its creature gone. A file without
// the section (a binary's before this) leaves every instance closed, as before.

import (
	"log"
	"sort"
	"strings"
	"time"
)

// KeptDungeons is the dungeons' instances as a restart carries them.
type KeptDungeons struct {
	Next  uint32            `json:"next"`
	Insts []KeptDungeonInst `json:"insts,omitempty"`
	Stays []KeptDungeonStay `json:"stays,omitempty"`
}

// KeptDungeonInst is one open instance.
type KeptDungeonInst struct {
	ID        uint32    `json:"id"`
	Key       string    `json:"key"` // its dungeon's (DungeonDef.Key)
	Owner     uint32    `json:"owner"`
	OwnerName string    `json:"owner_name"`
	Group     uint32    `json:"group,omitempty"`
	Opened    time.Time `json:"opened"`
	LastIn    time.Time `json:"last_in"`
	Creatures int       `json:"creatures"` // how many it opened with
	// NPCs are its creatures still in the world, alive or a corpse that lies
	NPCs []KeptInstNPC `json:"npcs,omitempty"`
	// In are the characters standing in it
	In []uint32 `json:"in,omitempty"`
}

// KeptInstNPC is an instance's creature: its state as keepnpc.go carries a
// row's, and the row it is a copy of.
type KeptInstNPC struct {
	KeptNPC
	Row uint32 `json:"row"`
}

// KeptDungeonStay is where a character in the dungeons came from.
type KeptDungeonStay struct {
	Char   uint32   `json:"char"`
	From   GateSpot `json:"from"`
	Facing float32  `json:"facing,omitempty"`
}

// DungeonsRestored says what RestoreDungeons did.
type DungeonsRestored struct {
	Insts, InstsLost int // instances made again; not (their dungeon gone, the dungeons off, the id taken)
	Alive, Dead      int // creatures made again, alive and as corpses
	NPCsLost         int // creatures not made again (their row gone, or another's)
	Players          int // characters to be put back in one
}

// KeepDungeons is the dungeons' open instances at now (the file's doc); nil
// with none and no one's way out. The listener is paused.
func (w *World) KeepDungeons(now time.Time) *KeptDungeons {
	w.mu.Lock() // not RLock: as KeepNPCs
	defer w.mu.Unlock()
	if !w.dungeons.on || len(w.dungeons.insts) == 0 && len(w.dungeons.stays) == 0 {
		return nil
	}
	k := &KeptDungeons{Next: w.dungeons.next}
	ids := make([]uint32, 0, len(w.dungeons.insts))
	for id := range w.dungeons.insts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		in := w.dungeons.insts[id]
		w.dungeonFollowOwnerLocked(in)
		ki := KeptDungeonInst{ID: in.id, Key: w.dungeons.defs[in.def].Key, Owner: in.owner, OwnerName: in.ownerName,
			Group: in.group, Opened: in.opened, LastIn: in.lastIn, Creatures: in.creatures}
		for _, cid := range in.npcs {
			c := w.NPCs[cid]
			if c == nil || !c.Temporary || c.Instance != in.id || c.CopyOf == 0 {
				continue
			}
			if c.Dead && !c.corpseLies(now, w.corpseDecay) {
				continue // killed, its corpse gone: the instance's progress
			}
			kn, ok := w.keptStateLocked(c, now, true)
			if !ok {
				continue
			}
			ki.NPCs = append(ki.NPCs, KeptInstNPC{KeptNPC: kn, Row: uint32(c.CopyOf)})
		}
		for _, p := range w.Players {
			if w.inDungeonsLocked(p) && p.Instance == in.id {
				ki.In = append(ki.In, p.CharServerID)
			}
		}
		sort.Slice(ki.In, func(i, j int) bool { return ki.In[i] < ki.In[j] })
		k.Insts = append(k.Insts, ki)
	}
	for c, s := range w.dungeons.stays {
		k.Stays = append(k.Stays, KeptDungeonStay{Char: c, From: s.From, Facing: s.FromFacing})
	}
	sort.Slice(k.Stays, func(i, j int) bool { return k.Stays[i].Char < k.Stays[j].Char })
	return k
}

// RestoreDungeons makes the kept instances again (the file's doc), once the rows
// and the dungeons are loaded, the creatures' and the copies' corpses restored,
// and before the sessions are put back. It returns each creature's old id's new
// id (for the corpses' contents: handlers.RestoreCorpsesAs).
func (w *World) RestoreDungeons(k *KeptDungeons) (ids map[EntityID]EntityID, r DungeonsRestored) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids = map[EntityID]EntityID{}
	if k == nil {
		return ids, r
	}
	if !w.dungeons.on {
		r.InstsLost = len(k.Insts)
		return ids, r
	}
	w.dungeons.next = max(w.dungeons.next, k.Next)
	for _, s := range k.Stays {
		w.dungeons.stays[s.Char] = dungeonStay{From: s.From, FromFacing: s.Facing}
	}
	for i := range k.Insts {
		ki := &k.Insts[i]
		def := -1
		for j, d := range w.dungeons.defs {
			if strings.EqualFold(d.Key, ki.Key) {
				def = j
			}
		}
		if _, taken := w.dungeons.insts[ki.ID]; def < 0 || taken || ki.ID == 0 {
			log.Printf("Dungeons: instance %d of %q (%s's) not made again: its dungeon is gone, or its id taken", ki.ID, ki.Key, ki.OwnerName)
			r.InstsLost++
			continue
		}
		in := &dungeonInst{id: ki.ID, def: def, owner: ki.Owner, ownerName: ki.OwnerName, group: ki.Group,
			opened: ki.Opened, lastIn: ki.LastIn, creatures: ki.Creatures}
		var nr NPCsRestored
		for j := range ki.NPCs {
			kn := &ki.NPCs[j]
			t := w.NPCs[EntityID(kn.Row)]
			if t == nil || !w.dungeonTemplateLocked(t) {
				r.NPCsLost++
				continue
			}
			onRow := kn.World == t.World && kn.Spawn == [4]float32{t.SpawnX, t.SpawnY, t.SpawnZ, t.SpawnFacing}
			if !onRow && !strings.EqualFold(kn.Name, t.Name) {
				r.NPCsLost++ // another creature on the row now
				continue
			}
			want := EntityID(kn.ID)
			if _, taken := w.NPCs[want]; taken {
				want = 0
			} else if _, taken := w.Players[want]; taken {
				want = 0
			}
			// as dungeonOpenLocked makes it, on its row's spawn and path ...
			c := w.spawnCopyAsLocked(t, t.SpawnX, t.SpawnY, t.SpawnZ, t.SpawnFacing, t.World, want)
			if p := w.paths.byNPC[t.ServerID]; p != nil && p.World == t.World {
				w.setPathLocked(c.ServerID, p)
			}
			c.Instance = in.id
			c.RespawnTime = t.RespawnTime
			// ... then as it was (keepnpc.go)
			w.restoreNPCLocked(c, &kn.KeptNPC, onRow, &nr)
			in.npcs = append(in.npcs, c.ServerID)
			ids[EntityID(kn.ID)] = c.ServerID
			if c.Dead {
				r.Dead++
			} else {
				r.Alive++
			}
		}
		if in.creatures < len(in.npcs) {
			in.creatures = len(in.npcs)
		}
		w.dungeons.insts[in.id] = in
		w.dungeons.next = max(w.dungeons.next, in.id)
		for _, c := range ki.In {
			if w.dungeons.resumeIn == nil {
				w.dungeons.resumeIn = map[uint32]uint32{}
			}
			w.dungeons.resumeIn[c] = in.id
			r.Players++
		}
		r.Insts++
		log.Printf("Dungeons: instance %d of %s (%s's, group %d) made again: %d of %d creatures alive, %d corpses lying, %d inside",
			in.id, w.dungeons.defs[def].Name, in.ownerName, in.group, w.dungeonAliveLocked(in), in.creatures,
			len(in.npcs)-w.dungeonAliveLocked(in), len(ki.In))
	}
	w.companionOf = nil // shamans and their pets pair within each again (companion.go)
	return ids, r
}

// DungeonResumeIn is the instance a character carried through a restart stood
// in, while the sessions are being put back (false: none, or it is gone).
func (w *World) DungeonResumeIn(char uint32, world int) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.dungeons.on || world != int(w.dungeons.world) {
		return 0, false
	}
	id, ok := w.dungeons.resumeIn[char]
	if !ok || w.dungeons.insts[id] == nil {
		return 0, false
	}
	return id, true
}

// DungeonsResumed ends a restart's putting back (once the sessions and the
// groups are): every player standing in the dungeons is left alone dungeonsGrace
// from now, as one just moved, and an instance with someone in it was last stood
// in now. It reports how many players stand in an instance.
func (w *World) DungeonsResumed(now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dungeons.resumeIn = nil
	if !w.dungeons.on {
		return 0
	}
	n := 0
	for _, p := range w.Players {
		if !w.inDungeonsLocked(p) {
			continue
		}
		w.dungeons.movedAt[p.ServerID] = now
		delete(w.dungeons.badAt, p.ServerID)
		if in := w.dungeons.insts[p.Instance]; in != nil {
			in.lastIn = now
			n++
		}
	}
	return n
}

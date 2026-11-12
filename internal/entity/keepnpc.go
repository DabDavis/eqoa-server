package entity

// keepnpc.go — the creatures and NPCs a restart of the server process carries
// as they were (the user, 2026-11-07: "when we restart lets leave mobs and npcs
// where they were"; claude-notes/session-keep-restart-20261106.md; keep.go has
// the players' part). The new process loads every NPC from its row, on its
// spawn at full health, so until this every restart that kept the players sent
// every creature home whole: a raid boss killed a minute ago stood there again,
// one at 40% was at 100%, and a client's creatures slid back to their spawns.
//
// What is written (KeepNPCs) is only what differs from what the row makes: an
// NPC of a row that has left its spawn or its spawn's facing, is short of its
// hit points, is dead, was fighting or going home after a fight, has taken up its
// path, is walking back to its spawn, or has an ability's cooldown running
// (abilitytimes.go). What comes back (RestoreNPCs, once
// everything the rows, the balance and the templates give is loaded, before a
// datagram is read or a tick runs):
//
//   - its place and facing, standing. A wanderer rests and takes its next leg
//     from there (findRoamersLocked's first rest; roamPointLocked draws from
//     where it stands). A path walker takes its path up at the leg nearest it,
//     as after a fight (pathState.broken), which leaves one at the end of a path
//     walked once where it is (a fresh pathState would walk it back to its first
//     point). Neither a leg's nor a pause's state is carried: both start over
//     from where it stands.
//   - its hit points. A maximum a data step changed (a level, the balance) keeps
//     the same share. Its regeneration counts its calm ticks anew.
//   - its death: the times of the death and of its corpse's going, as they are,
//     so corpseSystem decays the corpse and brings it back when each comes due,
//     one whose time came during the restart at its first tick. A corpse opened
//     has its contents carried by handlers/keep.go; one not opened rolls them
//     when it first is. A GM's hold (GMHeld) and the guards' claim on the kill
//     (guardHit) go with it.
//   - a fight's end: the threats and the round are gone, as they always were, and
//     one that was fighting or on its way home runs back to where the fight began
//     (Home; chase.go's goHomeLocked), its hit points as they were.
//   - its template abilities' cooldowns, each to its time (abilitytimes.go: Mr.
//     Moogie's Lay Hands is once an hour, a restart or no), dead or alive.
//   - one walking back to its spawn with its path taken away goes on doing so,
//     and so does one with nothing to bring it back: left away from its spawn by
//     a charm's end, say, standing or past its wander's reach of it (ToSpawn).
//
// What does not come back: the spells on it (their casters' ids are gone), its
// enemy and threats, a template fight's clocks, a pickpocket's mark.
//
// A client sees each where the file has it from the first update on: its slots go on
// describing the same creatures under the same ids (keep.go's RestoreView), and every
// client is told the creatures stand before the world freezes (StandNPCs).
//
// Not written at all, and so as their rows make them, as every restart had them:
//
//   - a charmed creature (Owner, Charm), dead or alive: its row makes it, whole,
//     on its spawn, in its own world and with its own flags, and a living one
//     whose master comes back is charmed to it again where it stood, with its
//     share of hit points and its charm's time left (keeppet.go, petition 1363:
//     left free on its spawn it attacked its master on sight). A released one
//     is an ordinary creature.
//   - a training dummy, which never dies and fights no one once its player is
//     gone; a spawn-only original, which never leaves its spawn.
//   - what no row made (Temporary): a player's pet (made again by its spell
//     beside its master who comes back: keeppet.go), a GM's /spawn, a quest's or
//     a trigger's copy, a genie, a portal. Nothing makes the rest again: they
//     are gone, but for a copy's corpse that lies, which keepcopy.go carries
//     (2026-11-12) with what is on it.
//
// A data step in the swap's hold (tools/swap_keep.sh's before.sh) runs after the
// file is written, so each entry says what its row was, and the row as it is now
// wins (RestoreNPCs): a row gone leaves the entry unused, a spawn point or world
// changed puts the creature on the new one with nothing of its way about, its hit
// points and any death kept (a boss's spawn fixed right after it died does not raise
// it: its corpse lies on the new spot), and an id with both a new spawn and another
// name is another creature, made by its row alone. A new row is not in the file. Its
// name or level changed on the same spawn is the same creature, the same share of
// its hit points: a typo's fix does not raise a raid boss.
//
// Only a restart that keeps the sessions writes this. A cold start (the swap
// when empty, a crash, the GM panel's restart, #servdown) loads the rows alone,
// as does the first restart out of a binary from before this: its file has no
// such section (udp.Snapshot.NPCs).

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// KeptNPC is an NPC of a row that differs from what its row makes of it, as a
// restart carries it.
type KeptNPC struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
	// World and Spawn (x, y, z, facing) are what its row said at the writing, to
	// tell a row a data step changed.
	World uint8      `json:"world,omitempty"`
	Spawn [4]float32 `json:"spawn"`
	// Place is where it stands (x, y, z, facing), nil on its spawn.
	Place *[4]float32 `json:"place,omitempty"`
	// HP and MaxHP are its hit points when it lives and is short of them.
	HP    int `json:"hp,omitempty"`
	MaxHP int `json:"max_hp,omitempty"`
	// Dead: its death's time and when its corpse went (zero while it lies), a
	// GM's hold on it and whether the kill is the guards'.
	Dead       bool      `json:"dead,omitempty"`
	Died       time.Time `json:"died,omitzero"`
	Gone       time.Time `json:"gone,omitzero"`
	Held       bool      `json:"held,omitempty"`
	GuardsKill bool      `json:"guards_kill,omitempty"`
	// CorpseHeld is how long a raid's loot keeps its corpse lying (corpse.go's
	// HoldCorpse), zero none.
	CorpseHeld time.Time `json:"corpse_held,omitzero"`
	// Home is where the fight it was in, or is on its way back from, began.
	Home *[3]float32 `json:"home,omitempty"`
	// PathDir is the way it walked its path (+1 on, -1 back), 0 for no path taken
	// up; ToSpawn that it walks, or should walk, back to its spawn.
	PathDir int8 `json:"path_dir,omitempty"`
	ToSpawn bool `json:"to_spawn,omitempty"`
	// Surface is the water's top over one swimming there (water.go), 0 for none:
	// finding it again would read the world's files during the restart.
	Surface float32 `json:"surface,omitempty"`
	// Cooldowns are its template abilities' cooldowns still running, by ability
	// (abilitytimes.go: an hour is an hour, petition 1394), alive or dead.
	Cooldowns map[string]time.Time `json:"cooldowns,omitempty"`
}

// StandNPCs stops every creature where it stands, its channel on each client's
// slots saying so, for a restart about to freeze the world. A client carries an
// NPC on along the velocity it last heard until it hears otherwise (CLIENT's
// ExtrapolateMove), whatever else has gone quiet: measured on a headless vanilla
// through a 4 s hold, a walker on a real world's ground was drawn 20 units past where
// it stood and then dragged back when the channels came again. Told first, it stands
// through the freeze where the file has it. A step moves it again if the restart
// fails. The listener is paused; it reports how many stood.
func (w *World) StandNPCs() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, npc := range w.NPCs {
		if npc.moving() {
			npc.stopMoving()
			n++
		}
	}
	w.entityChannelSyncSystem()
	return n
}

// keepsNoNPC reports an NPC a restart does not carry (the file's doc).
func keepsNoNPC(npc *NPC) bool {
	return npc.Temporary || npc.Owner != 0 || npc.Charm != nil || npc.Dummy != NotDummy || npc.SpawnOnly
}

// KeepNPCs is every NPC of a row that differs from what its row makes of it (the
// file's doc), in no order. The listener is paused.
func (w *World) KeepNPCs() []KeptNPC {
	w.mu.Lock() // not RLock: wanders may note what it finds on the NPC
	defer w.mu.Unlock()
	var out []KeptNPC
	now := time.Now()
	for _, npc := range w.NPCs {
		if k, ok := w.keptNPCLocked(npc, now); ok {
			out = append(out, k)
		}
	}
	return out
}

// keptNPCLocked is npc as a restart carries it at now, false when it carries
// nothing of it. Must be called with the lock held.
func (w *World) keptNPCLocked(npc *NPC, now time.Time) (KeptNPC, bool) {
	if keepsNoNPC(npc) {
		return KeptNPC{}, false
	}
	k := KeptNPC{ID: uint32(npc.ServerID), Name: npc.Name, World: npc.World,
		Spawn:     [4]float32{npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing},
		Cooldowns: npc.pendingCooldowns(now)}
	away := npc.X != npc.SpawnX || npc.Y != npc.SpawnY || npc.Z != npc.SpawnZ || npc.Facing != npc.SpawnFacing
	if away {
		k.Place = &[4]float32{npc.X, npc.Y, npc.Z, npc.Facing}
	}
	if npc.Dead {
		if npc.DeathTime.IsZero() {
			return KeptNPC{}, false // no death corpseSystem could bring it back from
		}
		k.Dead, k.Died, k.Gone = true, npc.DeathTime, npc.CorpseGone
		k.Held, k.GuardsKill, k.CorpseHeld = npc.GMHeld, npc.guardHit, npc.corpseHeld
	} else {
		if npc.HP < npc.MaxHP {
			k.HP, k.MaxHP = npc.HP, npc.MaxHP
		}
		if npc.chase.on || npc.chase.returning {
			k.Home = &[3]float32{npc.chase.fromX, npc.chase.fromY, npc.chase.fromZ}
		}
		if npc.swimming {
			k.Surface = npc.surface
		}
		if w.paths.byNPC[npc.ServerID] != nil && npc.path.started {
			k.PathDir = 1
			if npc.path.dir < 0 {
				k.PathDir = -1
			}
		}
		k.ToSpawn = w.paths.home[npc.ServerID] || away && k.Home == nil && w.strandedLocked(npc)
	}
	// a camp's spawn standing as another of its kinds than its row's: its name
	// says which (campspawn.go)
	kind := w.campKindChangedLocked(npc)
	if !away && !k.Dead && k.MaxHP == 0 && k.Home == nil && k.PathDir == 0 && !k.ToSpawn && !kind && len(k.Cooldowns) == 0 {
		return KeptNPC{}, false
	}
	if !k.finite() {
		// a float JSON cannot hold would fail the whole file, and with it the
		// restart, for one creature: this one goes back to its row
		log.Printf("Keep: '%s' (id %d) not kept: its place is no number: %v at %v", npc.Name, npc.ServerID, k.Spawn, k.Place)
		return KeptNPC{}, false
	}
	return k, true
}

// strandedLocked reports a living NPC away from its spawn that nothing would walk
// back: not on a path, and either of the kind that stands or past its own wander's
// reach of its spawn, where no leg ends (roamPointLocked). A charm's end leaves one
// so, up to a zone from home: only its death or, until this, a restart brought it
// back. Must be called with the lock held.
func (w *World) strandedLocked(npc *NPC) bool {
	if w.pathedLocked(npc.ServerID) {
		return false
	}
	d := distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ)
	if d <= 0.5 {
		return false
	}
	return !w.wanders(npc) || d > w.roamRadiusLocked(npc)
}

// finite reports that every float of k is one JSON can hold.
func (k *KeptNPC) finite() bool {
	all := append(k.Spawn[:len(k.Spawn):len(k.Spawn)], k.Surface)
	if k.Place != nil {
		all = append(all, k.Place[:]...)
	}
	if k.Home != nil {
		all = append(all, k.Home[:]...)
	}
	for _, v := range all {
		if v-v != 0 { // NaN and the infinities
			return false
		}
	}
	return true
}

// NPCsRestored says what RestoreNPCs did with a file's NPCs.
type NPCsRestored struct {
	Back     int // given what was kept of them, of which:
	Dead     int // dead, their times running on
	Hurt     int // short of their hit points
	Placed   int // standing where they stood
	Fight    int // running back to where a fight began
	Path     int // taking their paths up again where they stand
	ToSpawn  int // walking back to their spawns
	RowMoved int // on the new place of a row a data step moved
	Kind     int // camps' spawns made again as the kinds they stood as (campspawn.go)
	Cooldown int // with an ability's cooldown running on (abilitytimes.go)

	RowGone int // not used: the row is gone
	NewNPC  int // not used: another creature on the id (a new spawn and another name)
	NotKept int // not used: the row is now of a kind not carried (keepsNoNPC)
}

func (r NPCsRestored) String() string {
	return fmt.Sprintf("%d back (%d dead, %d hurt, %d where they stood, %d going back to where a fight began, %d taking up their paths, %d walking to their spawns, %d on a row's new place, %d camp spawns as the kinds they stood as, %d with a cooldown running); not used: %d rows gone, %d other creatures on the id, %d of a kind not kept",
		r.Back, r.Dead, r.Hurt, r.Placed, r.Fight, r.Path, r.ToSpawn, r.RowMoved, r.Kind, r.Cooldown, r.RowGone, r.NewNPC, r.NotKept)
}

// RestoreNPCs gives the NPCs a restart's file names what was kept of them (the
// file's doc). Everything the rows make must be loaded (the balance, the
// templates, the paths, who stands out of the world), and no tick running. It
// reads no ground: a restart's hold is the players'.
func (w *World) RestoreNPCs(kept []KeptNPC) NPCsRestored {
	w.mu.Lock()
	defer w.mu.Unlock()
	var r NPCsRestored
	for i := range kept {
		k := &kept[i]
		npc := w.NPCs[EntityID(k.ID)]
		if npc == nil {
			r.RowGone++
			continue
		}
		if keepsNoNPC(npc) {
			r.NotKept++
			continue
		}
		onRow := k.World == npc.World && k.Spawn == [4]float32{npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing}
		if !onRow && !strings.EqualFold(k.Name, npc.Name) {
			r.NewNPC++
			continue
		}
		if w.campKeptKindLocked(npc, k.Name) {
			r.Kind++ // a camp's spawn, as the kind it stood as (campspawn.go)
		}
		w.restoreNPCLocked(npc, k, onRow, &r)
	}
	return r
}

// restoreNPCLocked gives npc, as its row just made it, what was kept: its place
// and way about only when its row still says what it said (onRow). Must be called
// with the lock held.
func (w *World) restoreNPCLocked(npc *NPC, k *KeptNPC, onRow bool, r *NPCsRestored) {
	r.Back++
	if len(k.Cooldowns) > 0 {
		npc.abilityReady = make(map[string]time.Time, len(k.Cooldowns)) // its cooldowns run on (abilitytimes.go)
		for key, until := range k.Cooldowns {
			npc.abilityReady[key] = until
		}
		r.Cooldown++
	}
	switch {
	case !onRow:
		r.RowMoved++
	case k.Place != nil:
		oldX, oldZ := npc.X, npc.Z
		npc.X, npc.Y, npc.Z, npc.Facing = k.Place[0], k.Place[1], k.Place[2], k.Place[3]
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		r.Placed++
	}
	if k.Dead {
		npc.HP, npc.Dead = 0, true
		npc.DeathTime, npc.CorpseGone = k.Died, k.Gone
		npc.GMHeld, npc.guardHit, npc.corpseHeld = k.Held, k.GuardsKill, k.CorpseHeld
		r.Dead++
		w.wakeNPCLocked(npc) // the corpses' set, for its decay and its return
		return
	}
	if k.MaxHP > 0 {
		npc.HP = keptHP(k, npc.MaxHP)
		r.Hurt++
	}
	w.wakeNPCLocked(npc) // its regeneration
	if !onRow {
		return // on its new spawn, nothing to go back to
	}
	if k.Surface != 0 {
		npc.swimming, npc.surface = true, k.Surface
		npc.Loco = LocoTread
	}
	if k.Home != nil {
		npc.chase = chaseState{on: true, fromX: k.Home[0], fromY: k.Home[1], fromZ: k.Home[2]}
		w.goHomeLocked(npc)
		if npc.chase.returning {
			r.Fight++
		}
	}
	switch {
	case w.paths.byNPC[npc.ServerID] != nil:
		if k.PathDir != 0 {
			npc.path = pathState{started: true, broken: true, dir: int(k.PathDir)}
			r.Path++
		}
	case k.PathDir != 0 || k.ToSpawn:
		// its path is gone with a data step, or it was on its way already
		if w.walkToSpawnLocked(npc) {
			r.ToSpawn++
		}
	}
}

// keptHP is the hit points of one kept at k's share of a maximum, at maxHP now:
// the same number under the same maximum, at least 1 (it lives), at most maxHP.
func keptHP(k *KeptNPC, maxHP int) int {
	hp := k.HP
	if k.MaxHP != maxHP {
		hp = int(int64(k.HP) * int64(maxHP) / int64(k.MaxHP))
	}
	return max(1, min(hp, maxHP))
}

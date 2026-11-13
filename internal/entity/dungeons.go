package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/session"
)

// dungeons.go — instanced raid dungeons (the owner, 2026-11-12: "lets add a dungeon world. instanced versions of
// popular dungeons like Guk, Blackburrow, Runnyeye. they have the same mobs, but will be tuned for level 60 raids and
// raid gear").
//
// The dungeons are one world of the server's own (handlers/serverworlds.go; tools/worlds/dungeons.json, slot 14: "the
// Dungeons"): the disc's own zones of each dungeon, where they stand on Tunaria, built once. The world is instanced
// (homes.go's engine: Player.Instance, NPC.Instance, SamePlace, npcSharesLocked): each raid (or group, or a player
// alone) that goes into a dungeon gets a copy of it of its own, an INSTANCE, with its own creatures, corpses and loot.
//
// The world's own npcs rows are the dungeons' TEMPLATES: every row of the dungeons' world (but the Dungeon Warden's
// twin, the way out, which stands in every instance) stands out of the world (spawnonly.go's hold: no view, no aggro,
// no wander), and an instance opened for a dungeon is a Temporary copy of each template whose spawn lies in the
// dungeon's boxes (the recipe's "dungeons" block), standing in that instance alone. A copy is of its row (CopyOf), so
// the row's npctemplate ("#n" keys; tools/dungeons_20261112.py binds each to a raid template), its loot tables (npcloot
// "#n", handlers/loot.go) and its level are its. A copy killed is gone once its corpse is (corpse.go): an instance's
// creatures never come back; a fresh instance is the way to fight them again.
//
// Who opens: the leader of a group or raid (its first member), or a player alone. An instance belongs to its opener
// (owner) and the opener's group: its members go in from the Dungeon Warden (or /dungeon) into theirs. Who may stand
// in one: its owner and the members of the owner's group of the moment (the instance follows its owner into a group
// formed later, or another); anyone else is sent out within a few seconds (dungeonsSystem), as is a player standing in
// the world with no instance (a GM's move, or a cold start: the instances live in memory). An instance no one has
// stood in for CloseAfter closes: its creatures go. Where a player came from is kept (in memory) for its way out; else
// it goes in front of the Dungeon Warden outside.
//
// A restart that keeps the sessions keeps the instances too (keepdungeon.go; the owner, 2026-11-13: "on server
// restart the instanced raids are booting players, lets keep them in the instance and progress"): each under its id,
// its creatures alive where they stood at their hit points, its corpses lying with what they hold, its killed ones
// still killed, the players in it back in it where they stood, and the ways out.

// DungeonWardenName is the NPC that opens the dungeons (tools/dungeons_20261112.py stands one beside Dabrish and its
// twin, the way out, at each dungeon's entrance).
const DungeonWardenName = "Dungeon Warden"

// DungeonRaidBoardName is Freeport's papered board against the well that offers the same dungeons (the owner,
// 2026-11-12: "for the instanced raids add them to a bulletin board similar to PvP Events, against the well";
// tools/raidboard_20261112.py made the Freeport warden that board; the entrance twins stay wardens).
const DungeonRaidBoardName = "Instanced Raids"

// IsDungeonWarden is whether an NPC of the name opens the dungeons: a Dungeon Warden or the Instanced Raids board.
func IsDungeonWarden(name string) bool {
	n := strings.TrimSpace(name)
	return strings.EqualFold(n, DungeonWardenName) || strings.EqualFold(n, DungeonRaidBoardName)
}

// IsDungeonRaidBoard is whether an NPC of the name is the Instanced Raids board.
func IsDungeonRaidBoard(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), DungeonRaidBoardName)
}

// dungeonsCheckEvery is how often the dungeons' world is looked over.
const dungeonsCheckEvery = time.Second

// dungeonsGrace is how long a player who may not stand in an instance (its group left) is let stay before it is
// sent out, and how long a player the server just moved is left alone.
const dungeonsGrace = 5 * time.Second

// DefaultDungeonCloseAfter is how long an instance no one stands in stays open.
const DefaultDungeonCloseAfter = 30 * time.Minute

// DefaultDungeonMaxInstances is how many instances may be open at once.
const DefaultDungeonMaxInstances = 30

// dungeonWorlds marks the dungeons' world (its rows are templates: spawnonly.go, handlers/loot.go).
var dungeonWorlds [256]atomic.Bool

// IsDungeonWorld reports whether world is the dungeons' world.
func IsDungeonWorld(world uint8) bool { return dungeonWorlds[world].Load() }

// DungeonBox is a box of the world's ground (x0 <= x <= x1, z0 <= z <= z1).
type DungeonBox struct{ X0, Z0, X1, Z1 float32 }

func (b DungeonBox) has(x, z float32) bool { return x >= b.X0 && x <= b.X1 && z >= b.Z0 && z <= b.Z1 }

// DungeonDef is one dungeon of the world: its key ("guk"), its name, the boxes its creatures stand in, and where a
// player sent in lands (facing the dungeon).
type DungeonDef struct {
	Key      string
	Name     string
	Boxes    []DungeonBox
	Entrance HomeSpot
	// Cap is how many players its instance holds, the party and those added to it
	// (instancemembers.go; the recipe's "cap", 0: DungeonSettings.Cap); a raid's
	// instance holds DungeonSettings.RaidCap
	Cap int
}

func (d *DungeonDef) has(x, z float32) bool {
	for _, b := range d.Boxes {
		if b.has(x, z) {
			return true
		}
	}
	return false
}

// DungeonSettings are world.dungeons' numbers.
type DungeonSettings struct {
	CloseAfter   time.Duration
	MaxInstances int
	// Cap is a dungeon's instance's players with no cap of its own (0:
	// DefaultDungeonCap), RaidCap a raid's instance's (0: DefaultDungeonRaidCap):
	// instancemembers.go
	Cap, RaidCap int
}

// dungeonInst is one open instance.
type dungeonInst struct {
	id        uint32
	def       int
	owner     uint32 // the opener's character
	ownerName string
	group     uint32 // the owner's group when last seen (0: alone)
	opened    time.Time
	lastIn    time.Time // when a player last stood in it
	npcs      []EntityID
	creatures int
	// invited are the characters added to it beyond its party (their names), removed
	// those taken out of it and when (instancemembers.go)
	invited map[uint32]string
	removed map[uint32]time.Time
}

// dungeonStay is where a character in the dungeons came from.
type dungeonStay struct {
	From       GateSpot
	FromFacing float32
}

type dungeonsState struct {
	on      bool
	world   uint8
	defs    []DungeonDef
	set     DungeonSettings
	insts   map[uint32]*dungeonInst
	next    uint32
	stays   map[uint32]dungeonStay
	movedAt map[EntityID]time.Time
	badAt   map[EntityID]time.Time // when a player was first seen where it may not stand
	checkAt time.Time
	// resumeIn is, while a restart that keeps the sessions puts them back, the instance each character stood in
	// (keepdungeon.go); empty otherwise
	resumeIn map[uint32]uint32
}

// SetDungeons turns the dungeons on (or off) in world with its dungeons. Their rows become templates (out of the
// world) at once.
func (w *World) SetDungeons(on bool, world uint8, defs []DungeonDef, set DungeonSettings) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dungeons.on && w.dungeons.world != world {
		dungeonWorlds[w.dungeons.world].Store(false)
		SetInstancedWorld(w.dungeons.world, false)
	}
	if set.CloseAfter <= 0 {
		set.CloseAfter = DefaultDungeonCloseAfter
	}
	if set.MaxInstances <= 0 {
		set.MaxInstances = DefaultDungeonMaxInstances
	}
	if set.Cap <= 0 {
		set.Cap = DefaultDungeonCap
	}
	if set.RaidCap <= 0 {
		set.RaidCap = DefaultDungeonRaidCap
	}
	w.dungeons = dungeonsState{on: on, world: world, defs: defs, set: set, insts: map[uint32]*dungeonInst{},
		stays: map[uint32]dungeonStay{}, movedAt: map[EntityID]time.Time{}, badAt: map[EntityID]time.Time{}}
	dungeonWorlds[world].Store(on)
	SetInstancedWorld(world, on)
	held := 0
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Owner != 0 {
			continue
		}
		want := w.npcSpawnOnlyLocked(npc)
		if want && w.dungeonTemplateLocked(npc) {
			held++
		}
		if npc.SpawnOnly == want {
			continue
		}
		npc.SpawnOnly = want
		if want && !npc.Dead {
			w.respawnNPCLocked(npc)
			npc.Effects = nil
			npc.endDummyFight()
			delete(w.affected, npc.ServerID)
		}
	}
	if on {
		for i := range defs {
			n := 0
			for _, npc := range w.NPCs {
				if w.dungeonTemplateLocked(npc) && defs[i].has(npc.SpawnX, npc.SpawnZ) {
					n++
				}
			}
			log.Printf("Dungeons: %s (%s): %d creatures, the entrance at (%.1f, %.1f, %.1f)", defs[i].Name, defs[i].Key, n,
				defs[i].Entrance.X, defs[i].Entrance.Y, defs[i].Entrance.Z)
		}
		log.Printf("Dungeons: world %d, %d dungeons, %d templates held out of the world; an instance closes %v after its last player",
			world, len(defs), held, set.CloseAfter)
	}
}

// DungeonsOn reports whether the dungeons are on, and their world.
func (w *World) DungeonsOn() (bool, uint8) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.dungeons.on, w.dungeons.world
}

// DungeonDefs is the dungeons (a copy).
func (w *World) DungeonDefs() []DungeonDef {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]DungeonDef(nil), w.dungeons.defs...)
}

// dungeonTemplateLocked is whether an NPC is a dungeon's template: a row of the dungeons' world (not a copy, a pet,
// nor the warden's twin). Must be called with the lock held.
func (w *World) dungeonTemplateLocked(npc *NPC) bool {
	return npc != nil && w.dungeons.on && npc.World == w.dungeons.world && !npc.Temporary && npc.Owner == 0 &&
		!IsDungeonWarden(npc.Name)
}

// inDungeonsLocked is whether p stands in the dungeons' world.
func (w *World) inDungeonsLocked(p *Player) bool {
	return w.dungeons.on && p != nil && p.World == w.dungeons.world
}

// dungeonAllowsLocked is whether p may stand in the instance. Must be called with the lock held.
func (w *World) dungeonAllowsLocked(in *dungeonInst, p *Player) bool {
	if in == nil || p == nil {
		return false
	}
	w.dungeonFollowOwnerLocked(in)
	if _, out := in.removed[p.CharServerID]; out {
		return false // taken out of it (instancemembers.go)
	}
	_, added := in.invited[p.CharServerID]
	return p.CharServerID == in.owner || (in.group != 0 && p.GroupID == in.group) || added
}

// dungeonFollowOwnerLocked moves an instance to its owner's group of the moment, while the owner is online.
func (w *World) dungeonFollowOwnerLocked(in *dungeonInst) {
	if o := w.playerByCharLocked(in.owner); o != nil && o.GroupID != 0 && o.GroupID != in.group {
		in.group = o.GroupID
	}
}

// dungeonInstForLocked is the open instance of dungeon def p may stand in, nil for none.
func (w *World) dungeonInstForLocked(def int, p *Player) *dungeonInst {
	var best *dungeonInst
	for _, in := range w.dungeons.insts {
		if in.def == def && w.dungeonAllowsLocked(in, p) && (best == nil || in.id > best.id) {
			best = in
		}
	}
	return best
}

// dungeonLeaderLocked is whether p may open an instance: alone, or its group's leader.
func (w *World) dungeonLeaderLocked(p *Player) bool {
	if p.GroupID == 0 {
		return true
	}
	g := w.groups[p.GroupID]
	return g == nil || len(g.Members) == 0 || g.Members[0] == p.ServerID
}

// DungeonChoice is a dungeon as the warden offers it to a player: open for it (an instance of its party's), and
// whether the player may open one.
type DungeonChoice struct {
	Key, Name string
	Open      bool
	MayOpen   bool
	Owner     string
	Alive     int // the creatures still standing in its instance
}

// DungeonChoices is each dungeon as the session's player finds it.
func (w *World) DungeonChoices(sess *session.Session) []DungeonChoice {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.dungeons.on {
		return nil
	}
	lead := w.dungeonLeaderLocked(p)
	out := make([]DungeonChoice, 0, len(w.dungeons.defs))
	for i, d := range w.dungeons.defs {
		c := DungeonChoice{Key: d.Key, Name: d.Name, MayOpen: lead}
		if in := w.dungeonInstForLocked(i, p); in != nil {
			c.Open, c.Owner, c.Alive = true, in.ownerName, w.dungeonAliveLocked(in)
		}
		out = append(out, c)
	}
	return out
}

// dungeonAliveLocked counts an instance's creatures still alive, those a player can kill alone
// (dungeonKillable).
func (w *World) dungeonAliveLocked(in *dungeonInst) int {
	n := 0
	for _, id := range in.npcs {
		if c := w.NPCs[id]; c != nil && c.Instance == in.id && !c.Dead && dungeonKillable(c) {
			n++
		}
	}
	return n
}

// dungeonKillable is whether an instance's creature counts in its "%d of %d creatures killed" (/dungeon, the
// opening line, the warden's choices): one a player can kill. Not one with the NPC bit (0x80: no one may attack
// it, the attack refuses it), a training dummy's or a spawn-only original's (Waldorf's ticket 2010, 2026-11-13,
// "I've killed 306 of 307. Is the last npc the teleport? If so that shouldn't be counted").
func dungeonKillable(c *NPC) bool {
	return c != nil && c.NPCType&0x80 == 0 && c.Dummy == NotDummy && !c.SpawnOnly
}

// dungeonKillableCountLocked is how many of an instance's creatures a player can kill (dungeonKillable), alive
// or dead.
func (w *World) dungeonKillableCountLocked(in *dungeonInst) int {
	n := 0
	for _, id := range in.npcs {
		if c := w.NPCs[id]; c != nil && c.Instance == in.id && dungeonKillable(c) {
			n++
		}
	}
	return n
}

// dungeonColour is the dungeons' lines' colour.
var dungeonColour = [3]byte{230, 190, 120}

// The dungeons' lines (ours).
const (
	dungeonLineOpened  = "%s opens an instance of %s for their party: %d creatures await."
	dungeonLineEnter   = "You enter %s (%s's instance)."
	dungeonLineNoOpen  = "No instance of %s is open for your party. Your group's leader must open one at a Dungeon Warden."
	dungeonLineFull    = "Every instance of the dungeons is taken; try again a little later."
	dungeonLineOut     = "You are not in %s's party now, and you step out of their instance."
	dungeonLineLost    = "That instance of the dungeons is closed, and you step out."
	dungeonLineLeft    = "You leave the dungeon."
	dungeonLineReset   = "Your instance of %s is closed; the next entry opens a new one."
	dungeonLineResetNo = "Only the one who opened the instance may close it, and only with no one inside."
)

// DungeonEnter sends the session's player into its party's instance of the dungeon key, opening one when it may
// (alone or its group's leader) and none is open. It reports why not (false).
func (w *World) DungeonEnter(sess *session.Session, key string, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	switch {
	case !w.dungeons.on:
		return "The dungeons are closed on this server.", false
	case p == nil || p.IsDead() || p.respawnPending():
		return "You cannot go there now.", false
	}
	def := -1
	for i, d := range w.dungeons.defs {
		if strings.EqualFold(d.Key, key) || strings.EqualFold(d.Name, key) {
			def = i
		}
	}
	if def < 0 {
		return fmt.Sprintf("There is no dungeon named %s.", key), false
	}
	d := &w.dungeons.defs[def]
	in := w.dungeonInstForLocked(def, p)
	if in == nil {
		if !w.dungeonLeaderLocked(p) {
			return fmt.Sprintf(dungeonLineNoOpen, d.Name), false
		}
		if len(w.dungeons.insts) >= w.dungeons.set.MaxInstances {
			return dungeonLineFull, false
		}
		in = w.dungeonOpenLocked(def, p, now)
	}
	if !w.inDungeonsLocked(p) {
		w.dungeons.stays[p.CharServerID] = dungeonStay{From: GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z,
			Place: "where you were"}, FromFacing: p.Facing}
	}
	p.Instance = in.id
	in.lastIn = now
	w.dungeons.movedAt[p.ServerID] = now
	delete(w.dungeons.badAt, p.ServerID)
	e := d.Entrance
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: GateSpot{World: w.dungeons.world, X: e.X, Y: e.Y,
		Z: e.Z, Place: d.Name}, Spell: DungeonWardenName, Facing: e.Facing, HasFacing: true})
	w.sendColoredLocked(sess, fmt.Sprintf(dungeonLineEnter, d.Name, in.ownerName), dungeonColour)
	log.Printf("Dungeons: %s enters %s (instance %d, %s's), %d creatures alive", p.Name, d.Name, in.id, in.ownerName,
		w.dungeonAliveLocked(in))
	return "", true
}

// dungeonOpenLocked opens an instance of dungeon def for p: a copy of each of its templates, in it alone.
func (w *World) dungeonOpenLocked(def int, p *Player, now time.Time) *dungeonInst {
	w.dungeons.next++
	in := &dungeonInst{id: w.dungeons.next, def: def, owner: p.CharServerID, ownerName: p.Name, group: p.GroupID,
		opened: now, lastIn: now}
	d := &w.dungeons.defs[def]
	var ts []*NPC
	for _, npc := range w.NPCs {
		if w.dungeonTemplateLocked(npc) && d.has(npc.SpawnX, npc.SpawnZ) {
			ts = append(ts, npc)
		}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].ServerID < ts[j].ServerID })
	for _, t := range ts {
		c := w.spawnCopyLocked(t, t.SpawnX, t.SpawnY, t.SpawnZ, t.SpawnFacing, t.World)
		c.Instance = in.id
		c.RespawnTime = t.RespawnTime
		in.npcs = append(in.npcs, c.ServerID)
	}
	in.creatures = w.dungeonKillableCountLocked(in) // none a player cannot kill (dungeonKillable)
	w.companionOf = nil                             // its shamans and their pets pair within it (companion.go), read again
	w.dungeons.insts[in.id] = in
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendColoredLocked(sess, fmt.Sprintf(dungeonLineOpened, p.Name, d.Name, in.creatures), dungeonColour)
	}
	log.Printf("Dungeons: %s opens instance %d of %s (group %d): %d creatures", p.Name, in.id, d.Name, in.group, in.creatures)
	return in
}

// dungeonCloseLocked closes an instance: its creatures go.
func (w *World) dungeonCloseLocked(in *dungeonInst, why string) {
	gone := 0
	for _, id := range in.npcs {
		if c := w.NPCs[id]; c != nil && c.Temporary && c.Instance == in.id {
			w.removeNPCLocked(c)
			gone++
		}
	}
	delete(w.dungeons.insts, in.id)
	w.companionOf = nil
	log.Printf("Dungeons: instance %d of %s (%s's) closed (%s): %d creatures gone", in.id,
		w.dungeons.defs[in.def].Name, in.ownerName, why, gone)
}

// DungeonReset closes the session's player's instance of the dungeon key, when it opened it and no one stands in
// it. It reports a line for the player.
func (w *World) DungeonReset(sess *session.Session, key string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.dungeons.on {
		return ""
	}
	for i, d := range w.dungeons.defs {
		if !strings.EqualFold(d.Key, key) && !strings.EqualFold(d.Name, key) {
			continue
		}
		in := w.dungeonInstForLocked(i, p)
		if in == nil {
			return fmt.Sprintf(dungeonLineNoOpen, d.Name)
		}
		if in.owner != p.CharServerID || w.dungeonPeopleLocked(in) > 0 {
			return dungeonLineResetNo
		}
		w.dungeonCloseLocked(in, "reset by its owner")
		return fmt.Sprintf(dungeonLineReset, d.Name)
	}
	return fmt.Sprintf("There is no dungeon named %s.", key)
}

// dungeonPeopleLocked counts the players standing in an instance.
func (w *World) dungeonPeopleLocked(in *dungeonInst) int {
	n := 0
	for _, p := range w.Players {
		if w.inDungeonsLocked(p) && p.Instance == in.id {
			n++
		}
	}
	return n
}

// DungeonLeave sends the session's player out of the dungeons, to where it came from.
func (w *World) DungeonLeave(sess *session.Session) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.inDungeonsLocked(p) {
		return "You are not in a dungeon.", false
	}
	if !w.dungeonSendOutLocked(p, dungeonLineLeft) {
		return "You cannot leave now.", false
	}
	return "", true
}

// dungeonSendOutLocked sends p out of the dungeons: where it came from, else in front of a Dungeon Warden outside.
func (w *World) dungeonSendOutLocked(p *Player, line string) bool {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil {
		return false
	}
	to, facing, ok := w.dungeonWayOutLocked(p.CharServerID)
	if !ok {
		log.Printf("Dungeons: %s cannot be sent out: no place to go (no Dungeon Warden outside)", p.Name)
		return false
	}
	w.dungeons.movedAt[p.ServerID] = time.Now()
	delete(w.dungeons.badAt, p.ServerID)
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: "the way out of the dungeon",
		Facing: facing, HasFacing: true})
	if line != "" {
		w.sendColoredLocked(sess, line, dungeonColour)
	}
	log.Printf("Dungeons: %s goes out to %s (%.1f, %.1f, %.1f) world=%d", p.Name, to.Place, to.X, to.Y, to.Z, to.World)
	return true
}

// dungeonWayOutLocked is where a character leaving the dungeons goes.
func (w *World) dungeonWayOutLocked(char uint32) (GateSpot, float32, bool) {
	if s, ok := w.dungeons.stays[char]; ok && (s.From.X != 0 || s.From.Z != 0) && s.From.World != w.dungeons.world {
		return s.From, s.FromFacing, true
	}
	return w.dungeonWardenSpotLocked()
}

// dungeonWardenSpotLocked is the spot in front of a Dungeon Warden outside the dungeons, facing away from it.
func (w *World) dungeonWardenSpotLocked() (GateSpot, float32, bool) {
	var best *NPC
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Dead || !IsDungeonWarden(npc.Name) || npc.World == w.dungeons.world {
			continue
		}
		if best == nil || npc.ServerID < best.ServerID {
			best = npc
		}
	}
	if best == nil {
		return GateSpot{}, 0, false
	}
	f := float64(best.Facing)
	return GateSpot{World: best.World, Y: best.Y, Place: "in front of the " + strings.TrimSpace(best.Name),
		X: best.X + homesOut*float32(math.Sin(f)), Z: best.Z + homesOut*float32(math.Cos(f))}, best.Facing, true
}

// DungeonLogin decides a login of a character saved in the dungeons' world: it resumes in its party's open instance of the dungeon it stood in, else it goes out (out: where).
// dungeon false: not saved there.
func (w *World) DungeonLogin(char uint32, world int, x, z float32) (dungeon, resume bool, inst uint32, out GateSpot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.dungeons.on || world != int(w.dungeons.world) {
		return false, false, 0, GateSpot{}
	}
	var groupID uint32 // its group, kept over its logout (a player's entity id is its character's)
	for _, g := range w.groups {
		for _, id := range g.Members {
			if uint32(id) == char {
				groupID = g.ID
			}
		}
	}
	out, _, _ = w.dungeonWayOutLocked(char)
	if in := w.dungeonLoginInstLocked(char, groupID, x, z); in != nil {
		return true, true, in.id, out
	}
	return true, false, 0, out
}

// DungeonEntrance is where a player sent into the open instance inst lands (its dungeon's entrance); false when no
// such instance is open.
func (w *World) DungeonEntrance(inst uint32) (HomeSpot, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	in := w.dungeons.insts[inst]
	if in == nil {
		return HomeSpot{}, false
	}
	return w.dungeons.defs[in.def].Entrance, true
}

// dungeonLoginInstLocked is the open instance a character (of group groupID) logging in at (x, z) stands in.
func (w *World) dungeonLoginInstLocked(char, groupID uint32, x, z float32) *dungeonInst {
	var best *dungeonInst
	for _, in := range w.dungeons.insts {
		w.dungeonFollowOwnerLocked(in)
		if !w.dungeons.defs[in.def].has(x, z) && !w.dungeonNearEntranceLocked(in.def, x, z) {
			continue
		}
		_, added := in.invited[char]
		if _, out := in.removed[char]; out {
			continue // taken out of it (instancemembers.go)
		}
		if (in.owner == char || (in.group != 0 && in.group == groupID) || added) && (best == nil || in.id > best.id) {
			best = in
		}
	}
	return best
}

// dungeonNearEntranceLocked is whether (x, z) is within 300 of the dungeon's entrance.
func (w *World) dungeonNearEntranceLocked(def int, x, z float32) bool {
	e := w.dungeons.defs[def].Entrance
	return math.Hypot(float64(x-e.X), float64(z-e.Z)) <= 300
}

// dungeonInstanceOnLoadLocked is the instance a player made standing in the dungeons' world is in (0: none, sent
// out by dungeonsSystem).
func (w *World) dungeonInstanceOnLoadLocked(p *Player) uint32 {
	if !w.inDungeonsLocked(p) {
		return 0
	}
	if id, ok := w.dungeons.resumeIn[p.CharServerID]; ok && w.dungeons.insts[id] != nil {
		// carried through a restart standing in it: back in it where it stood (keepdungeon.go), left alone a while
		w.dungeons.movedAt[p.ServerID] = time.Now()
		return id
	}
	if in := w.dungeonLoginInstLocked(p.CharServerID, p.GroupID, p.X, p.Z); in != nil {
		return in.id
	}
	return 0
}

// dungeonsSystem looks over the dungeons' world once a second: a player in an instance it may not stand in (or in
// none, or a closed one) goes out after a grace; an instance no one stood in for CloseAfter closes. Must be called
// with the lock held.
func (w *World) dungeonsSystem(now time.Time) {
	if !w.dungeons.on || now.Before(w.dungeons.checkAt) {
		return
	}
	w.dungeons.checkAt = now.Add(dungeonsCheckEvery)
	for id, at := range w.dungeons.movedAt {
		if now.Sub(at) > time.Minute {
			delete(w.dungeons.movedAt, id)
		}
	}
	for id := range w.dungeons.badAt {
		if p := w.Players[id]; p == nil || !w.inDungeonsLocked(p) {
			delete(w.dungeons.badAt, id)
		}
	}
	for _, p := range w.Players {
		if !w.inDungeonsLocked(p) || p.IsDead() || p.respawnPending() {
			continue
		}
		if now.Sub(w.dungeons.movedAt[p.ServerID]) < dungeonsGrace {
			continue
		}
		in := w.dungeons.insts[p.Instance]
		if in != nil && w.dungeonAllowsLocked(in, p) {
			in.lastIn = now
			delete(w.dungeons.badAt, p.ServerID)
			continue
		}
		if in != nil && w.dungeonRemovedTurnLocked(in, p, now) {
			continue // its warning's minute, then Freeport (instancemembers.go)
		}
		if p.GM && in != nil {
			in.lastIn = now
			continue // a GM visits any instance it was put in
		}
		first, seen := w.dungeons.badAt[p.ServerID]
		if !seen {
			w.dungeons.badAt[p.ServerID] = now
			continue
		}
		if now.Sub(first) < dungeonsGrace {
			continue
		}
		line := dungeonLineLost
		if in != nil {
			line = fmt.Sprintf(dungeonLineOut, in.ownerName)
		}
		w.dungeonSendOutLocked(p, line)
	}
	for _, in := range w.dungeons.insts {
		if now.Sub(in.lastIn) >= w.dungeons.set.CloseAfter && w.dungeonPeopleLocked(in) == 0 {
			w.dungeonCloseLocked(in, fmt.Sprintf("no one in it for %v", w.dungeons.set.CloseAfter))
		}
	}
}

// DungeonStatus is a line for /dungeon: the open instances of the session's player's party.
func (w *World) DungeonStatus(sess *session.Session) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.dungeons.on {
		return ""
	}
	var parts []string
	for i, d := range w.dungeons.defs {
		if in := w.dungeonInstForLocked(i, p); in != nil {
			// "xx of yy killed" (petition 1841, Waldorf: "a tracker to see how many of the total we killed")
			parts = append(parts, fmt.Sprintf("%s (%s's): %d of %d creatures killed, %d inside", d.Name, in.ownerName,
				in.creatures-w.dungeonAliveLocked(in), in.creatures, w.dungeonPeopleLocked(in)))
		}
	}
	log.Printf("Dungeons: %s asks its party's dungeons: %s", p.Name, strings.Join(parts, "; "))
	if len(parts) == 0 {
		return "Your party has no dungeon open. A group's leader (or a player alone) opens one at a Dungeon Warden."
	}
	return "Your party's dungeons: " + strings.Join(parts, "; ") + "."
}

// DungeonInstanceInfo is (for tests and the log) the instance id an entity stands in and whether that instance is
// open.
func (w *World) DungeonInstanceInfo(id EntityID) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	inst := w.instanceOfLocked(id)
	_, open := w.dungeons.insts[inst]
	return inst, open
}

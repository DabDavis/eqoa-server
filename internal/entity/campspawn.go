package entity

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A GM's camps (the GMs' /camp, handlers/gmcamps.go; tables gmcamps and
// campspawns, database/gmcamps.go; petition 1293: "this rock had 4 spawn
// points that could pop child of xrag or pariah of xrag. They were static
// positions, as this was an Xp camp. Roamers also did not roam into this
// spot"). A camp's spawn is a row of npcs of its own, which is its spot and
// the kind it was made as, and a row of campspawns naming the kinds it pops:
//
//   - it stands on its spawn whatever its kind (movementLocked's whyCampSpawn,
//     ahead of npcmove's rule by name: a camp's wolf stands where the hills'
//     wolves wander) until a fight takes it, and goes home after as a creature
//     that stands does, turning to its row's facing (chase.go's atHomeLocked).
//     A GM's path walks it all the same: the path is looked at first;
//   - each time it comes back from a death (respawnNPCLocked) it pops one of
//     its kinds at random, made from that kind's row as a row makes an NPC
//     (NPCFromRecord, then the world's settings: its balance, its class
//     template, its respawn time by its own row) on its own row's spot. The
//     kind it was made as comes from its own row, another from the row of
//     that name nearest it, not a camp's (/camp copied the same); the rows
//     are read with the camps (LoadCampSpawns: at the start, after a /camp
//     or its undo, at a GM's edit of campspawns in the panel) and kept up
//     with a GM's edit of them (UpsertNPC, ReloadNPCs) and of their names'
//     classes (SetNPCClass). A new kind stays out of every view for
//     NPCRefreshHold, so every client makes it again as itself;
//   - a restart that keeps the players keeps its kind (keepnpc.go: a kept
//     camp spawn's name is the kind it stood as);
//   - no wander leg ends within campKeepOut of one of its camp's spawns, nor
//     comes that near from outside, but for a creature that spawned that near
//     (legInCampLocked, roam.go): it lives there.

// campKeepOut is how near a camp's spawns no wander leg comes: a creature's
// aggro reach (DefaultAggroRadius, 21) and more, so a roamer passing by walks
// into none of the camp's fights.
const campKeepOut float32 = 30

// campRec is a row a camp's spawns are made from: a spawn's own (its spot,
// and the kind it was made as) or another kind's, and its name's class.
type campRec struct {
	rec   database.NPCRecord
	class int
}

// campSpawn is a camp's spawn: its camp and the rows its kinds are made from
// (campTable.recs' keys; its own among them for the kind it was made as).
type campSpawn struct {
	camp  int64
	kinds []int
}

// campArea is one camp's spawns in a world, for the keep-out: their middle,
// the farthest of them from it, and each.
type campArea struct {
	camp   int64
	cx, cz float32
	r      float32
	pts    [][2]float32
}

// campTable is the camps' spawns by their npcs rows, the rows their kinds
// are made from, and each world's camps' grounds.
type campTable struct {
	spawns map[EntityID]*campSpawn
	recs   map[int]*campRec
	areas  map[uint8][]campArea
	rnd    *rand.Rand // the kinds' roll (tests seed it)
}

// campRand is the kinds' own random source.
func (w *World) campRand() *rand.Rand {
	if w.camps.rnd == nil {
		w.camps.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return w.camps.rnd
}

// campSpawnLocked reports one of a camp's spawns. Must be called with the
// lock held.
func (w *World) campSpawnLocked(id EntityID) bool {
	return w.camps.spawns[id] != nil
}

// campWant is a kind a spawn needs a row for: its name's key and where the
// spawn stands, and the row found.
type campWant struct {
	key   string
	world uint8
	x, z  float32
	id    EntityID
	dist  float32
	same  bool
}

// nearestRowsLocked finds each want's row: the NPC of its name (by
// database.NPCClassKey) whose spawn is nearest it, one in its world before
// any other, the lowest id between two as near; never a GM's spawn or an id
// of skip. Must be called with the lock held.
func (w *World) nearestRowsLocked(wants []*campWant, skip func(EntityID) bool) {
	byKey := map[string][]*campWant{}
	for _, wt := range wants {
		byKey[wt.key] = append(byKey[wt.key], wt)
	}
	keys := map[string]string{}
	for id, npc := range w.NPCs {
		if npc.Temporary || skip(id) {
			continue
		}
		k, ok := keys[npc.Name]
		if !ok {
			k = database.NPCClassKey(npc.Name)
			keys[npc.Name] = k
		}
		for _, wt := range byKey[k] {
			same := npc.World == wt.world
			d := distXZ(npc.SpawnX, npc.SpawnZ, wt.x, wt.z)
			if wt.id == 0 || same && !wt.same || same == wt.same && (d < wt.dist || d == wt.dist && id < wt.id) {
				wt.id, wt.dist, wt.same = id, d, same
			}
		}
	}
}

// CampTemplate is the row a camp's spawn of name copies (/camp): the NPC of
// that name whose spawn is nearest (x, z), one in world before any other,
// none a GM's spawn or a camp's; failing those, a camp's spawn made as that
// name, its own row. False when no row is of the name.
func (w *World) CampTemplate(name string, world uint8, x, z float32) (EntityID, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	wt := &campWant{key: database.NPCClassKey(name), world: world, x: x, z: z}
	w.nearestRowsLocked([]*campWant{wt}, w.campSpawnLocked)
	if wt.id != 0 {
		return wt.id, true
	}
	for id := range w.camps.spawns {
		c := w.camps.recs[int(id)]
		if c == nil || database.NPCClassKey(c.rec.NPCName) != wt.key {
			continue
		}
		same := uint8(c.rec.World) == world
		d := distXZ(c.rec.XCoord, c.rec.ZCoord, x, z)
		if wt.id == 0 || same && !wt.same || same == wt.same && (d < wt.dist || d == wt.dist && id < wt.id) {
			wt.id, wt.dist, wt.same = id, d, same
		}
	}
	return wt.id, wt.id != 0
}

// CampKindRefused is why no camp may pop the NPCs of name, "" when one may:
// one that stands out of the world (spawnonly.go) would stand out of it on
// the camp's spot too, and a training dummy never dies to pop another.
func (w *World) CampKindRefused(name string) string {
	if DummyKindOf(name) != NotDummy {
		return "is a training dummy, which never dies to pop another"
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.spawnOnlyLocked(name) {
		return "stands out of the world (NPC spawn only), as a camp's spawn of it would"
	}
	return ""
}

// LoadCampSpawns reads the camps (table campspawns, each spawn's own row and
// the rows its other kinds are made from) into the world: at the start and
// after a change to them. Call it without the database held, the NPCs loaded.
// It reports how many spawns it read; a spawn whose row is gone, or a kind
// with no row of its name, is logged and left out.
func (w *World) LoadCampSpawns(db *database.SQLiteDB) (int, error) {
	rows, err := db.CampSpawns()
	if err != nil {
		return 0, fmt.Errorf("campspawns: %w", err)
	}
	store := database.NewNPCStore(db)
	classes, err := store.NPCClasses()
	if err != nil {
		return 0, fmt.Errorf("npcclass: %w", err)
	}
	own := make(map[int]*database.NPCRecord, len(rows))
	for _, r := range rows {
		rec, err := store.GetNPC(r.NPCID)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("Camps: spawn %d of camp %d: its NPC row is gone (left out)", r.NPCID, r.Camp)
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("camp spawn %d: %w", r.NPCID, err)
		}
		own[r.NPCID] = rec
	}
	// the rows the other kinds are made from: the nearest of each name, not a camp's
	type need struct {
		spawn int
		want  *campWant
	}
	var needs []need
	var wants []*campWant
	for _, r := range rows {
		o := own[r.NPCID]
		if o == nil {
			continue
		}
		mine := database.NPCClassKey(o.NPCName)
		for _, name := range r.Kinds {
			if k := database.NPCClassKey(name); k != mine {
				wt := &campWant{key: k, world: uint8(o.World), x: o.XCoord, z: o.ZCoord}
				needs = append(needs, need{r.NPCID, wt})
				wants = append(wants, wt)
			}
		}
	}
	w.mu.RLock()
	w.nearestRowsLocked(wants, func(id EntityID) bool { _, camp := own[int(id)]; return camp })
	w.mu.RUnlock()
	type spawnKind struct {
		spawn int
		key   string
	}
	kindRow := map[spawnKind]int{} // the row a spawn's kind is made from
	others := map[int]*database.NPCRecord{}
	for _, n := range needs {
		id := int(n.want.id)
		if id == 0 {
			// no row of the name but a camp's spawn made as it: its own row
			for oid, o := range own {
				if database.NPCClassKey(o.NPCName) == n.want.key && (id == 0 || oid < id) {
					id = oid
				}
			}
		}
		if id == 0 {
			log.Printf("Camps: spawn %d: no NPC row is called %q: that kind is left out", n.spawn, n.want.key)
			continue
		}
		kindRow[spawnKind{n.spawn, n.want.key}] = id
		if own[id] == nil && others[id] == nil {
			rec, err := store.GetNPC(id)
			if err != nil {
				return 0, fmt.Errorf("NPC row %d: %w", id, err)
			}
			others[id] = rec
		}
	}

	t := campTable{spawns: map[EntityID]*campSpawn{}, recs: map[int]*campRec{}, areas: map[uint8][]campArea{}}
	rec := func(id int) {
		if t.recs[id] != nil {
			return
		}
		r := own[id]
		if r == nil {
			r = others[id]
		}
		t.recs[id] = &campRec{rec: *r, class: classes[database.NPCClassKey(r.NPCName)]}
	}
	byCamp := map[[2]int64][][2]float32{} // (world, camp) -> its spawns
	for _, r := range rows {
		o := own[r.NPCID]
		if o == nil {
			continue
		}
		rec(r.NPCID)
		cs := &campSpawn{camp: r.Camp}
		mine := database.NPCClassKey(o.NPCName)
		seen := map[string]bool{}
		for _, name := range r.Kinds {
			k := database.NPCClassKey(name)
			if seen[k] {
				continue
			}
			seen[k] = true
			id := r.NPCID
			if k != mine {
				if id = kindRow[spawnKind{r.NPCID, k}]; id == 0 {
					continue
				}
				rec(id)
			}
			cs.kinds = append(cs.kinds, id)
		}
		t.spawns[EntityID(r.NPCID)] = cs
		key := [2]int64{int64(o.World), r.Camp}
		byCamp[key] = append(byCamp[key], [2]float32{o.XCoord, o.ZCoord})
	}
	for key, pts := range byCamp {
		a := campArea{camp: key[1], pts: pts}
		for _, p := range pts {
			a.cx += p[0] / float32(len(pts))
			a.cz += p[1] / float32(len(pts))
		}
		for _, p := range pts {
			a.r = max(a.r, distXZ(a.cx, a.cz, p[0], p[1]))
		}
		t.areas[uint8(key[0])] = append(t.areas[uint8(key[0])], a)
	}
	w.mu.Lock()
	t.rnd = w.camps.rnd
	w.camps = t
	w.mu.Unlock()
	return len(t.spawns), nil
}

// CampSummary says how many camp spawns stand in how many camps, for the log.
func (w *World) CampSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	camps := map[int64]bool{}
	for _, cs := range w.camps.spawns {
		camps[cs.camp] = true
	}
	return fmt.Sprintf("%d camp spawns (campspawns) in %d camps stand, pop their kinds and keep the wander out", len(w.camps.spawns), len(camps))
}

// campRekindLocked is a camp's spawn coming back from a death: one of its
// kinds at random, made in its place (the file's doc). Must be called with
// the lock held.
func (w *World) campRekindLocked(npc *NPC, now time.Time) {
	cs := w.camps.spawns[npc.ServerID]
	if cs == nil || len(cs.kinds) == 0 {
		return
	}
	spot := w.camps.recs[int(npc.ServerID)]
	kind := w.camps.recs[cs.kinds[w.campRand().Intn(len(cs.kinds))]]
	if spot == nil || kind == nil || database.NPCClassKey(kind.rec.NPCName) == database.NPCClassKey(npc.Name) {
		return // the kind it is
	}
	log.Printf("Camps: spawn %d of camp %d comes back as %q (it was %q)", npc.ServerID, cs.camp, kind.rec.NPCName, npc.Name)
	w.makeKindLocked(npc, spot.rec, kind, now.Add(NPCRefreshHold))
}

// makeKindLocked makes npc, a camp's spawn, the kind of row kind on its own
// row's spot, in place: its pointer kept, as the panel's remake keeps one
// (UpsertNPC), out of every view until hold at least. Must be called with
// the lock held.
func (w *World) makeKindLocked(npc *NPC, spot database.NPCRecord, kind *campRec, hold time.Time) {
	rec := kind.rec
	rec.ID = spot.ID
	rec.World, rec.XCoord, rec.YCoord, rec.ZCoord, rec.Facing = spot.World, spot.XCoord, spot.YCoord, spot.ZCoord, spot.Facing
	fresh := NPCFromRecord(&rec, kind.class, w.rangedModels)
	w.applyNPCSettingsLocked(fresh)
	fresh.HiddenUntil = npc.HiddenUntil
	if hold.After(fresh.HiddenUntil) {
		fresh.HiddenUntil = hold
	}
	*npc = *fresh
	w.npcChangedLocked(npc) // npcsets.go
}

// campKindChangedLocked reports a camp's spawn standing as another kind than
// its own row's (keepnpc.go carries it). Must be called with the lock held.
func (w *World) campKindChangedLocked(npc *NPC) bool {
	if w.camps.spawns[npc.ServerID] == nil {
		return false
	}
	own := w.camps.recs[int(npc.ServerID)]
	return own != nil && database.NPCClassKey(own.rec.NPCName) != database.NPCClassKey(npc.Name)
}

// campKeptKindLocked makes npc, a camp's spawn just made from its row, the
// kind of its that a restart's file says it stood as (name); false when it is
// that kind already, or none of its kinds is called so. Must be called with
// the lock held.
func (w *World) campKeptKindLocked(npc *NPC, name string) bool {
	cs := w.camps.spawns[npc.ServerID]
	key := database.NPCClassKey(name)
	if cs == nil || key == database.NPCClassKey(npc.Name) {
		return false
	}
	spot := w.camps.recs[int(npc.ServerID)]
	if spot == nil {
		return false
	}
	for _, id := range cs.kinds {
		if kind := w.camps.recs[id]; kind != nil && database.NPCClassKey(kind.rec.NPCName) == key {
			w.makeKindLocked(npc, spot.rec, kind, time.Time{})
			return true
		}
	}
	return false
}

// campRecordLocked keeps the camps' copy of a row a GM changed (UpsertNPC,
// ReloadNPCs) as the row is now: a spawn's own row whatever changed (its spot
// follows it), another kind's while it is still of that name. Must be called
// with the lock held.
func (w *World) campRecordLocked(rec *database.NPCRecord, class int) {
	c := w.camps.recs[rec.ID]
	if c == nil {
		return
	}
	if w.camps.spawns[EntityID(rec.ID)] != nil || database.NPCClassKey(c.rec.NPCName) == database.NPCClassKey(rec.NPCName) {
		c.rec, c.class = *rec, class
	}
}

// campClassLocked gives the camps' rows of a name's key their class anew
// (SetNPCClass). Must be called with the lock held.
func (w *World) campClassLocked(key string, class int) {
	for _, c := range w.camps.recs {
		if database.NPCClassKey(c.rec.NPCName) == key {
			c.class = class
		}
	}
}

// near reports (x, z) within campKeepOut of one of the area's spawns.
func (a *campArea) near(x, z float32) bool {
	if distXZ(x, z, a.cx, a.cz) > a.r+campKeepOut {
		return false
	}
	for _, p := range a.pts {
		if distXZ(x, z, p[0], p[1]) <= campKeepOut {
			return true
		}
	}
	return false
}

// legInCampLocked reports a wander leg from where npc stands to (x, z) that
// ends within campKeepOut of a camp's spawn, or comes that near one from
// outside the camp's ground, unless npc spawned that near the camp (it lives
// there); one standing in a camp's ground may walk out of it (the file's
// doc). Must be called with the lock held.
func (w *World) legInCampLocked(npc *NPC, x, z float32) bool {
	areas := w.camps.areas[npc.World]
	for i := range areas {
		a := &areas[i]
		if segmentDist(npc.X, npc.Z, x, z, a.cx, a.cz) > a.r+campKeepOut || a.near(npc.SpawnX, npc.SpawnZ) {
			continue // far from it, or it lives there
		}
		if a.near(x, z) {
			return true // it would end in it
		}
		if a.near(npc.X, npc.Z) {
			continue // it walks out
		}
		for _, p := range a.pts {
			if segmentDist(npc.X, npc.Z, x, z, p[0], p[1]) <= campKeepOut {
				return true // through it
			}
		}
	}
	return false
}

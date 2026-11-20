package entity

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/esf"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// Where a box's creatures spawn, judged by the server's own ground (the owner,
// 2026-11-20: "runnyeye has messed up mob placements like fire pits etc"). For
// every npcs row whose spawn lies in a box of EQOA_SPAWNCHECK it reports the
// floor the server sets it on (Stand) and how far its row's height is off it,
// what CLIENT would show it in (something placed: Embedded, the movement
// check's ball; a floor over it within a body; the ground over it with no floor
// under), a placed fire it stands in (the disc's fires: a group of a collision
// sprite and a particle sprite, or a lone particle sprite, read off the world
// file EQOA_SPAWNCHECK_ESF's zones EQOA_SPAWNCHECK_ZONES), how many of 16 legs
// of 4 the wander's own check (legOKLocked) lets it walk, a hazard or a
// no-roam area it is in; and for each bad one the nearest place that is none of
// those, by the same rules. Then it wanders the box's creatures for half an
// hour with a player at each box's middle and reports any that walked into a
// fire or something placed.
//
//	EQOA_REAL_DB=<copy of a database> EQOA_GROUND_DIR=<baked ground> \
//	EQOA_SPAWNCHECK='0:15900,11800,18100,14100;14:15900,11800,18100,14100' \
//	EQOA_SPAWNCHECK_ESF=TUNARIA.ESF EQOA_SPAWNCHECK_ZONES=67,54 EQOA_SPAWNCHECK_MODELS=worlds/models.txt \
//	EQOA_SPAWNCHECK_OUT=report.tsv go test ./internal/entity/ -run TestRealSpawnPlaces -v
//
// A world's fires are the one file's: world 14's Runnyeye is Tunaria's zones 54
// and 67 where they stand (tools/worlds/dungeons.json). Skipped without them.
func TestRealSpawnPlaces(t *testing.T) {
	spec := os.Getenv("EQOA_SPAWNCHECK")
	if os.Getenv("EQOA_REAL_DB") == "" || os.Getenv("EQOA_GROUND_DIR") == "" || spec == "" {
		t.Skip("EQOA_REAL_DB, EQOA_GROUND_DIR and EQOA_SPAWNCHECK name a database copy, a ground and the boxes to check")
	}
	boxes, err := parseSpawnBoxes(spec)
	if err != nil {
		t.Fatal(err)
	}
	var fires []placedFire
	if p := os.Getenv("EQOA_SPAWNCHECK_ESF"); p != "" {
		if fires, err = readPlacedFires(p, os.Getenv("EQOA_SPAWNCHECK_ZONES")); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d placed fires", len(fires))
	if p := os.Getenv("EQOA_SPAWNCHECK_MODELS"); p != "" {
		m, err := LoadModelRadii(p) // <worlds>/models.txt: the bodies' edges, for a leg's knee
		if err != nil {
			t.Fatal(err)
		}
		SetModelRadii(m)
		defer SetModelRadii(nil)
	}
	g, err := ground.Open(os.Getenv("EQOA_GROUND_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w, db := spawnCheckWorld(t)
	w.SetGround(g)
	if _, err := w.LoadHazards(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadNoRoam(db); err != nil {
		t.Fatal(err)
	}
	sc := &spawnChecker{w: w, g: g, fires: fires}

	out := io.Writer(io.Discard)
	if p := os.Getenv("EQOA_SPAWNCHECK_OUT"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		out = f
	}
	fmt.Fprintln(out, "world\tid\tname\tx\ty\tz\twanders\tfloor\tplaced\tdy\tterrain\tembedded\tfloorover\tinground\tfire\tlegs\thazard\tnoroam\tbad\tto_x\tto_y\tto_z\tto_d\tto_legs")

	w.mu.Lock()
	var ids []EntityID
	for id, npc := range w.NPCs {
		if inSpawnBoxes(boxes, npc.World, npc.SpawnX, npc.SpawnZ) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	bad := 0
	for _, id := range ids {
		npc := w.NPCs[id]
		r := sc.judgeLocked(npc)
		line := fmt.Sprintf("%d\t%d\t%s\t%.2f\t%.2f\t%.2f\t%v\t%.2f\t%v\t%.2f\t%s\t%v\t%v\t%v\t%s\t%d\t%.0f\t%s\t%v\t%s",
			npc.World, npc.ServerID, npc.Name, npc.SpawnX, npc.SpawnY, npc.SpawnZ, r.wanders, r.floor, r.placed, r.dy,
			r.terrain, r.embedded, r.floorOver, r.inGround, r.fire, r.legs, r.reach, r.hazard, r.noRoam, strings.Join(r.bad, ","))
		if len(r.bad) > 0 {
			bad++
			if x, y, z, d, legs, ok := sc.freeSpotLocked(npc, r); ok {
				line += fmt.Sprintf("\t%.2f\t%.2f\t%.2f\t%.2f\t%d", x, y, z, d, legs)
			} else {
				line += "\t\t\t\tnone\t"
			}
		} else {
			line += "\t\t\t\t\t"
		}
		fmt.Fprintln(out, line)
	}
	w.mu.Unlock()
	t.Logf("%d spawns in the boxes, %d bad", len(ids), bad)

	if os.Getenv("EQOA_SPAWNCHECK_WANDER") == "" {
		return
	}
	// the wander: a player at each box's middle, the world-14 templates let
	// out as their instances' copies are (same rows, same spawns)
	w.mu.Lock()
	// a watcher in every 100-unit square a spawn is in, so every one wakes
	type sq struct {
		world  uint8
		cx, cz int
	}
	seen := map[sq]bool{}
	for _, id := range ids {
		npc := w.NPCs[id]
		k := sq{npc.World, int(npc.SpawnX / 100), int(npc.SpawnZ / 100)}
		if seen[k] {
			continue
		}
		seen[k] = true
		p := &Player{ServerID: EntityID(990000 + len(seen)), Name: "Watcher", HP: 100, MaxHP: 100,
			X: (float32(k.cx) + 0.5) * 100, Y: npc.SpawnY + 500, Z: (float32(k.cz) + 0.5) * 100, World: k.world}
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	}
	for _, id := range ids {
		w.NPCs[id].SpawnOnly = false
	}
	w.roamRnd = rand.New(rand.NewSource(1))
	w.mu.Unlock()
	now := time.Unix(1_800_000_000, 0)
	into := map[EntityID]string{}
	moved := map[EntityID]bool{}
	ticks, _ := strconv.Atoi(os.Getenv("EQOA_SPAWNCHECK_WANDER"))
	for tick := 0; tick < ticks; tick++ {
		w.mu.Lock()
		w.TickCount++
		w.roamSystem(now)
		if tick%5 == 0 {
			for _, id := range ids {
				npc := w.NPCs[id]
				if npc.X == npc.SpawnX && npc.Z == npc.SpawnZ {
					continue
				}
				moved[id] = true
				if _, done := into[id]; done {
					continue
				}
				if f := sc.fireAt(npc.World, npc.X, npc.Y, npc.Z, spawnFireMargin); f != "" {
					into[id] = fmt.Sprintf("fire %s at (%.1f, %.1f, %.1f) tick %d, spawn (%.1f, %.1f, %.1f)", f, npc.X, npc.Y, npc.Z, tick, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
				} else if g.Embedded(npc.World, npc.X, npc.Y, npc.Z, mcMiddle, mcBall) {
					into[id] = fmt.Sprintf("in something placed at (%.1f, %.1f, %.1f) tick %d, spawn (%.1f, %.1f, %.1f)", npc.X, npc.Y, npc.Z, tick, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
				}
			}
		}
		w.mu.Unlock()
		now = now.Add(100 * time.Millisecond)
	}
	t.Logf("wander: %d of the box's creatures moved in %d ticks; %d walked into a fire or something placed", len(moved), ticks, len(into))
	for id, s := range into {
		fmt.Fprintf(out, "#wander\t%d\t%d\t%s\t%s\n", w.NPCs[id].World, id, w.NPCs[id].Name, s)
	}
}

type spawnBox struct {
	world          uint8
	x0, z0, x1, z1 float32
}

func parseSpawnBoxes(s string) ([]spawnBox, error) {
	var out []spawnBox
	for _, part := range strings.Split(s, ";") {
		wv, rest, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("box %q: want world:x0,z0,x1,z1", part)
		}
		wn, err := strconv.Atoi(wv)
		if err != nil {
			return nil, err
		}
		var v [4]float32
		f := strings.Split(rest, ",")
		if len(f) != 4 {
			return nil, fmt.Errorf("box %q: want four numbers", part)
		}
		for i := range f {
			x, err := strconv.ParseFloat(f[i], 32)
			if err != nil {
				return nil, err
			}
			v[i] = float32(x)
		}
		out = append(out, spawnBox{uint8(wn), v[0], v[1], v[2], v[3]})
	}
	return out, nil
}

func inSpawnBoxes(bs []spawnBox, world uint8, x, z float32) bool {
	for _, b := range bs {
		if b.world == world && x >= b.x0 && x <= b.x1 && z >= b.z0 && z <= b.z1 {
			return true
		}
	}
	return false
}

// spawnCheckWorld is realWorld with its database kept open (the hazards' and
// the no-roam areas' tables), on a copy in the test's own folder.
func spawnCheckWorld(t *testing.T) (*World, *database.SQLiteDB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eqoa.db")
	in, err := os.Open(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	outf, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(outf, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	outf.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewNPCStore(db)
	w := NewWorld()
	if models, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(models)
	}
	w.LoadNPCs(store)
	return w, db
}

// placedFire is a fire the disc places: a group of a collision sprite and a
// particle sprite (a fire pit, a brazier, a candle, a torch), r its collision
// sprite's half width times its scale (1 for a lone particle sprite).
type placedFire struct {
	x, y, z, r float32
	sprite     uint32
}

// readPlacedFires reads the fires of a world file's zones (indexes, comma
// separated; every zone with none).
func readPlacedFires(path, zones string) ([]placedFire, error) {
	want := map[int]bool{}
	for _, s := range strings.Split(zones, ",") {
		if s = strings.TrimSpace(s); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
			want[n] = true
		}
	}
	f, err := esf.Open(path)
	if err != nil {
		return nil, err
	}
	root, err := f.Root()
	if err != nil {
		return nil, err
	}
	var out []placedFire
	for zn, zi := range root.Child(esf.TypeWorld).ChildrenOfType(esf.TypeZone) {
		if len(want) > 0 && !want[zn] {
			continue
		}
		obj, err := f.GetObject(zi)
		if err != nil {
			continue
		}
		acts, err := obj.(*esf.Zone).GetZoneActors(f)
		if err != nil || acts == nil {
			continue
		}
		rae, err := acts.GetActorsWithRoomIDs(f)
		if err != nil {
			continue
		}
		for _, a := range rae {
			pl := a.Actor.Placement
			o, err := f.FindObject(pl.SpriteID)
			if err != nil || o == nil {
				continue
			}
			fire, r := false, float32(0)
			switch s := o.(type) {
			case *esf.ParticleSprite:
				fire, r = true, 1
			case *esf.GroupSprite:
				for _, mp := range s.Placements {
					mo, err := f.FindObject(mp.SpriteID)
					if err != nil || mo == nil {
						continue
					}
					switch m := mo.(type) {
					case *esf.ParticleSprite:
						fire = true
					case *esf.SimpleSprite:
						b := m.BBox
						r = max(r, -b.MinX, b.MaxX, -b.MinZ, b.MaxZ) * mp.Scale
					}
				}
				if r == 0 {
					r = 1
				}
			}
			if fire {
				sc := pl.Scale
				if sc == 0 {
					sc = 1
				}
				out = append(out, placedFire{pl.Pos.X, pl.Pos.Y, pl.Pos.Z, r * sc, uint32(pl.SpriteID)})
			}
		}
	}
	return out, nil
}

type spawnChecker struct {
	w     *World
	g     *ground.Ground
	fires []placedFire
}

// fireAt names a fire a body with its feet at (x, y, z) stands in: within the
// fire's half width and margin of its middle, its base between 2.5 under the
// feet and 2 over them.
func (sc *spawnChecker) fireAt(world uint8, x, y, z, margin float32) string {
	for _, f := range sc.fires {
		if d := distXZ(x, z, f.x, f.z); d < f.r+margin && f.y > y-2.5 && f.y < y+2 {
			return fmt.Sprintf("%08x(%.1f,%.1f,%.1f r%.1f d%.1f)", f.sprite, f.x, f.y, f.z, f.r, d)
		}
	}
	return ""
}

type spawnJudgement struct {
	wanders                       bool
	floor, dy                     float32
	placed, floorOk               bool
	terrain                       string
	embedded, floorOver, inGround bool
	fire                          string
	legs                          int
	reach                         float32 // how far out a walk on the floors from it gets, at most spawnReach
	hazard                        string
	noRoam                        bool
	bad                           []string
	standY                        float32 // where the server sets it: its floor if on the ground, else its row's height
}

const (
	spawnCheckLegs   = 16
	spawnCheckLeg    = 4
	spawnFireMargin  = 0.6 // a body's half width
	spawnFireClear   = 2.0 // a new spot this far off a fire's edge
	spawnFloorHead   = 2.5 // a floor this far over the feet: CLIENT shows the body in it
	spawnFloorMargin = 0.15
	spawnReach       = 30 // the walk from a spawn looked at this far out
	spawnReachOK     = 20 // a spawn whose walk gets no further out than this is shut in
	spawnReachStep   = 1
	spawnGoodLegs    = 6 // a new spot has this many of the legs out at least
)

// reach is how far from (x, y, z) a walk over the floors gets, at most
// spawnReach: steps of spawnReachStep in eight ways, each onto the floor there
// (Stand) no more than the ground's step up nor 2 down (a way back), nothing
// placed in the body, no wall of the terrain's or anything placed at the knee
// or the middle. A spawn on a pillar, in a closed room or a pocket of rock gets
// nowhere; one on a ledge it can only fall from, not climb back to, as far as
// its ledge.
func (sc *spawnChecker) reach(wn uint8, x, y, z float32) float32 {
	g := sc.g
	type cell struct {
		ix, iz int
		iy     int32
	}
	type node struct {
		x, y, z float32
	}
	n := int(spawnReach / spawnReachStep)
	seen := map[cell]bool{}
	q := []node{{x, y, z}}
	seen[cell{0, 0, int32(math.Floor(float64(y)))}] = true
	best := float32(0)
	for len(q) > 0 && len(seen) < 20000 {
		c := q[0]
		q = q[1:]
		for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
			nx, nz := c.x+float32(d[0])*spawnReachStep, c.z+float32(d[1])*spawnReachStep
			ix, iz := int(math.Round(float64((nx-x)/spawnReachStep))), int(math.Round(float64((nz-z)/spawnReachStep)))
			if ix < -n || ix > n || iz < -n || iz > n {
				continue
			}
			h, _, roofed, ok := g.Stand(wn, nx, c.y, nz)
			if !ok || roofed || h < c.y-2 {
				continue
			}
			k := cell{ix, iz, int32(math.Floor(float64(h)))}
			if seen[k] {
				continue
			}
			if g.Wall(wn, c.x, c.y+0.5, c.z, nx, h+0.5, nz) || g.Wall(wn, c.x, c.y+mcMiddle, c.z, nx, h+mcMiddle, nz) ||
				g.Embedded(wn, nx, h, nz, mcMiddle, mcBall) {
				continue
			}
			seen[k] = true
			if dd := distXZ(x, z, nx, nz); dd > best {
				best = dd
			}
			q = append(q, node{nx, h, nz})
		}
	}
	return best
}

// judgeLocked judges a creature's spawn. Must be called with the lock held.
func (sc *spawnChecker) judgeLocked(npc *NPC) spawnJudgement {
	w, g := sc.w, sc.g
	var r spawnJudgement
	r.wanders, _, _ = w.movementLocked(npc)
	wn, x, y, z := npc.World, npc.SpawnX, npc.SpawnY, npc.SpawnZ
	h, placed, roofed, ok := g.Stand(wn, x, y, z)
	r.floor, r.placed, r.floorOk = h, placed, ok
	r.standY = y
	if ok {
		r.dy = y - h
		if r.dy >= -spawnedBelow && r.dy <= spawnedAbove {
			r.standY = h
		}
	}
	if t, tok := g.Floor(wn, x, y, z); tok {
		r.terrain = fmt.Sprintf("%.2f", t)
	} else {
		r.terrain = "-"
	}
	sy := r.standY
	r.embedded = g.Embedded(wn, x, sy, z, mcMiddle, mcBall)
	for _, s := range g.Surfaces(wn, x, z, sy+spawnFloorMargin, sy+spawnFloorHead) {
		if s.Up {
			r.floorOver = true
		}
	}
	r.floorOver = r.floorOver || (ok && roofed && r.standY == h)
	r.inGround = w.inGroundLocked(wn, x, y, z)
	r.fire = sc.fireAt(wn, x, sy, z, spawnFireMargin)
	r.legs = sc.legsLocked(npc, x, sy, z)
	r.reach = sc.reach(wn, x, sy, z)
	if hz := w.hazardAtLocked(wn, x, sy, z); hz != nil {
		r.hazard = hz.Name
	}
	for _, a := range w.noRoam.byWorld[wn] {
		if distXZ(x, z, a.X, a.Z) <= a.Radius {
			r.noRoam = true
		}
	}
	switch {
	case !ok && r.inGround:
		r.bad = append(r.bad, "in the ground")
	case !ok:
		r.bad = append(r.bad, "no floor")
	case r.dy > spawnedAbove:
		r.bad = append(r.bad, fmt.Sprintf("%.1f over its floor", r.dy))
	case r.dy < -spawnedBelow:
		r.bad = append(r.bad, fmt.Sprintf("%.1f under its floor", -r.dy))
	}
	if r.embedded {
		r.bad = append(r.bad, "in something placed")
	}
	if r.floorOver {
		r.bad = append(r.bad, "a floor over it")
	}
	if r.fire != "" {
		r.bad = append(r.bad, "in a fire")
	}
	if r.legs == 0 {
		r.bad = append(r.bad, "no leg out")
	}
	if r.reach < spawnReachOK {
		r.bad = append(r.bad, fmt.Sprintf("shut in (a walk gets %.0f out)", r.reach))
	}
	if r.hazard != "" {
		r.bad = append(r.bad, "in hazard "+r.hazard)
	}
	return r
}

// legsLocked is how many of spawnCheckLegs legs of spawnCheckLeg the wander's
// own check lets a creature standing at (x, y, z) walk: a copy of it moved
// there, its spawn too. Must be called with the lock held.
func (sc *spawnChecker) legsLocked(npc *NPC, x, y, z float32) int {
	c := *npc
	c.X, c.Y, c.Z, c.SpawnX, c.SpawnY, c.SpawnZ = x, y, z, x, y, z
	c.groundKnown = false
	n := 0
	for i := 0; i < spawnCheckLegs; i++ {
		a := 2 * math.Pi * float64(i) / spawnCheckLegs
		if sc.w.legOKLocked(&c, x+spawnCheckLeg*float32(math.Sin(a)), z+spawnCheckLeg*float32(math.Cos(a))) {
			n++
		}
	}
	return n
}

// freeSpotLocked is the nearest place to a bad spawn that is good: its floor
// within 3 of the floor (or height) it had, on the ground (within the spawn
// rule's heights: the place is its floor), nothing placed in the body, no floor
// over it, clear of every fire by spawnFireClear past its edge, no hazard, at
// least spawnGoodLegs of the legs out (a creature talked to, which never
// fights, none), a walk from it getting spawnReachOK out, and no wall of the
// terrain's or anything placed between the body's middle at the old place and
// the new. The same (x, z) set
// on its floor first, then rings half a unit apart out to 15, 32 a ring.
// Must be called with the lock held.
func (sc *spawnChecker) freeSpotLocked(npc *NPC, r spawnJudgement) (fx, fy, fz, d float32, legs int, ok bool) {
	w, g := sc.w, sc.g
	wn, x0, z0 := npc.World, npc.SpawnX, npc.SpawnZ
	y0 := npc.SpawnY
	if r.floorOk && r.dy >= -spawnFloorHead && r.dy <= 400 {
		y0 = r.floor
	}
	good := func(x, z, rise float32) (float32, int, bool) {
		h, _, roofed, ok := g.Stand(wn, x, y0+rise, z)
		if !ok || roofed || h < y0-rise || h > y0+rise {
			return 0, 0, false
		}
		if g.Embedded(wn, x, h, z, mcMiddle, mcClear) {
			return 0, 0, false
		}
		for _, s := range g.Surfaces(wn, x, z, h+spawnFloorMargin, h+spawnFloorHead) {
			if s.Up {
				return 0, 0, false
			}
		}
		if sc.fireAt(wn, x, h, z, spawnFireClear) != "" {
			return 0, 0, false
		}
		if w.hazardAtLocked(wn, x, h, z) != nil {
			return 0, 0, false
		}
		if (x != x0 || z != z0) && g.Wall(wn, x0, y0+mcMiddle, z0, x, h+mcMiddle, z) {
			return 0, 0, false
		}
		n := sc.legsLocked(npc, x, h, z)
		if (n < spawnGoodLegs && npc.NPCType&0x80 == 0) || sc.reach(wn, x, h, z) < spawnReachOK {
			return 0, 0, false
		}
		return h, n, true
	}
	// on its own floor's level first (within half a unit of it), then a unit, then 3
	for _, rise := range []float32{0.5, 1, 3} {
		if h, n, ok := good(x0, z0, rise); ok {
			return x0, h, z0, 0, n, true
		}
		for ring := float32(0.5); ring <= 15; ring += 0.5 {
			for i := 0; i < 32; i++ {
				a := 2 * math.Pi * float64(i) / 32
				x, z := x0+ring*float32(math.Sin(a)), z0+ring*float32(math.Cos(a))
				if h, n, ok := good(x, z, rise); ok {
					return x, h, z, ring, n, true
				}
			}
		}
	}
	return 0, 0, 0, 0, 0, false
}

package entity

import (
	"bytes"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// The creatures of the land and the water on the real ground (EQOA_GROUND_DIR)
// under the real creatures (EQOA_REAL_DB), as cmd/gameserver loads them (paths,
// npcmove, npcwander, camps, undead, spawn-only): which keep to what, how many
// of the land's wander into the water, how many come home through it when a
// way round on the land was there, and how many GM paths cross it. Skipped
// without both.
func landWaterWorld(t *testing.T) (*World, *database.SQLiteDB, func()) {
	t.Helper()
	dir, src := os.Getenv("EQOA_GROUND_DIR"), os.Getenv("EQOA_REAL_DB")
	if dir == "" || src == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	path := filepath.Join(t.TempDir(), "eqoa.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.SetOutput(io.Discard)
	store := database.NewNPCStore(db)
	w := NewWorld()
	models, _ := store.PlayerModels()
	w.SetPlayerModels(models)
	w.LoadNPCs(store)
	w.SetGround(g)
	w.SetEnv(env)
	if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
		w.LoadNPCPaths(recs)
	}
	w.LoadCampSpawns(db)
	w.LoadUndeadTags(db)
	if _, err := w.LoadMoveRules(db); err != nil {
		t.Fatal(err)
	}
	w.LoadRowWander(db)
	w.LoadSpawnOnly(db)
	return w, db, func() { log.SetOutput(os.Stderr); g.Close(); db.Close() }
}

// TestRealLandAndWater counts, on the real data: the creatures by what they
// keep to; the land's wanderers' ticks in the water over a simulated stretch;
// the land's ways home across the water (a chase to the far side of the water
// and the way back) and whether a way on the land was taken; and the GM paths
// of land creatures that cross the water.
func TestRealLandAndWater(t *testing.T) {
	w, _, done := landWaterWorld(t)
	defer done()
	w.mu.Lock()
	defer w.mu.Unlock()
	envWorld := func(world uint8) bool {
		_, ok := w.env.Flags(world, 0, 0, 0)
		e, _ := w.env.(*ground.Env)
		return ok || (e != nil && e.Knows(world))
	}
	// what each keeps to, and why
	type tally struct{ land, water, both, byRuleLand, byRuleWater, byRuleBoth, bySpawnWater int }
	var all, wand tally
	var landWanderers []*NPC
	ids := make([]EntityID, 0, len(w.NPCs))
	for id := range w.NPCs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		npc := w.NPCs[id]
		if !envWorld(npc.World) || npc.Owner != 0 {
			continue
		}
		k := w.keepsToLocked(npc)
		r, rule := w.moveRuleLocked(npc)
		rule = rule && r.Water != keepsBySpawn
		count := func(c *tally) {
			switch k {
			case keepsToLand:
				c.land++
				if rule {
					c.byRuleLand++
				}
			case keepsToWater:
				c.water++
				if rule {
					c.byRuleWater++
				} else {
					c.bySpawnWater++
				}
			case keepsToBoth:
				c.both++
				if rule {
					c.byRuleBoth++
				}
			}
		}
		count(&all)
		if w.wanders(npc) {
			count(&wand)
			if k == keepsToLand && w.spawnOnGroundLocked(npc) {
				landWanderers = append(landWanderers, npc)
			}
		}
	}
	t.Logf("all creatures in worlds with water known: %+v", all)
	t.Logf("wanderers: %+v", wand)

	// those of the land's wanderers with water within their radius
	var shore []*NPC
	for _, npc := range landWanderers {
		r := w.roamRadiusLocked(npc)
		wet := false
		for a := 0; a < 16 && !wet; a++ {
			for d := float32(4); d <= r && !wet; d += 4 {
				x := npc.SpawnX + d*float32(math.Sin(float64(a)*math.Pi/8))
				z := npc.SpawnZ + d*float32(math.Cos(float64(a)*math.Pi/8))
				if f, ok := w.ground.Floor(npc.World, x, npc.SpawnY+20, z); ok && w.waterAtLocked(npc.World, x, f+bodyAbove, z) {
					wet = true
				}
			}
		}
		if wet {
			shore = append(shore, npc)
		}
	}
	t.Logf("land wanderers: %d, %d with water within their radius", len(landWanderers), len(shore))

	// the wander: every shore wanderer (up to 400) for 15 simulated minutes
	if len(shore) > 400 {
		step := len(shore) / 400
		var some []*NPC
		for i := 0; i < len(shore); i += step {
			some = append(some, shore[i])
		}
		shore = some
	}
	w.roamRnd = rand.New(rand.NewSource(11))
	now := time.Unix(1_800_000_000, 0)
	var wetTicks, ticks, legsN, wetCreatures, feetTicks, feetCreatures int
	var pick time.Duration
	for _, npc := range shore {
		npc.roam = roamState{nearPlayer: true, last: now}
		t0 := now
		wasWet, feetWet := false, false
		for i := 0; i < 9000; i++ {
			t0 = t0.Add(100 * time.Millisecond)
			walking := npc.roam.walking
			b := time.Now()
			w.roamStepLocked(npc, t0)
			if !walking && npc.roam.walking {
				legsN++
				pick += time.Since(b)
			}
			ticks++
			if w.npcInWaterLocked(npc) {
				wetTicks++
				wasWet = true
			}
			if w.waterAtLocked(npc.World, npc.X, npc.Y+0.05, npc.Z) {
				feetTicks++
				if !feetWet {
					feetWet = true
					feetCreatures++
				}
			}
		}
		if wasWet {
			wetCreatures++
		}
		npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
		npc.roam = roamState{}
		npc.stopMoving()
	}
	if wetTicks != 0 {
		t.Errorf("a creature of the land wandered into the water")
	}
	t.Logf("wander: %d shore creatures x 15 min: %d of %d ticks in the water, %d creatures ever in it (wading, its feet wet: %d ticks, %d creatures); %d legs, %v a leg picked",
		len(shore), wetTicks, ticks, wetCreatures, feetTicks, feetCreatures, legsN, pick/time.Duration(max(legsN, 1)))

	// the way home: a chase across the water to the far shore (or into its
	// middle), given up there; with the dry way home and without (the control)
	type trip struct {
		npc        *NPC
		tx, ty, tz float32 // the far shore
		mx, my, mz float32 // the water's middle on the way
	}
	var trips []trip
	for _, npc := range landWanderers {
		if len(trips) >= 300 {
			break
		}
		for a := 0; a < 16; a++ {
			sx, sz := float32(math.Sin(float64(a)*math.Pi/8)), float32(math.Cos(float64(a)*math.Pi/8))
			wetAt, dryAt := float32(-1), float32(-1)
			var ty float32
			for d := float32(2); d <= 80; d += 1 {
				x, z := npc.SpawnX+d*sx, npc.SpawnZ+d*sz
				f, ok := w.ground.Floor(npc.World, x, npc.SpawnY+20, z)
				if !ok {
					break
				}
				wet := w.waterAtLocked(npc.World, x, f+bodyAbove, z)
				if wetAt < 0 && wet {
					wetAt = d
				} else if wetAt >= 0 && !wet {
					dryAt, ty = d+4, f
					break
				}
			}
			if wetAt > 0 && dryAt > 0 && dryAt-wetAt <= 50 {
				m := (wetAt + dryAt - 4) / 2
				mx, mz := npc.SpawnX+m*sx, npc.SpawnZ+m*sz
				my, _ := w.ground.Floor(npc.World, mx, npc.SpawnY+20, mz)
				trips = append(trips, trip{npc, npc.SpawnX + dryAt*sx, ty, npc.SpawnZ + dryAt*sz, mx, my, mz})
				break
			}
		}
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	deep := os.Getenv("EQOA_DEEP") != ""
	var deepNone, deepWet, deepDry int
	var deepTried []int
	var deepTime time.Duration
	home := func(dry, middle bool) int {
		was := homesDry
		homesDry = dry
		defer func() { homesDry = was }()
		buf.Reset()
		var reached, startWet, wetTrips, wetFromDry, wetTicks, ticks, gotHome, jumps int
		var spent time.Duration
		for _, tr := range trips {
			npc := tr.npc
			npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
			npc.chase, npc.roam, npc.swimming = chaseState{}, roamState{}, false
			t0 := now
			ok := false
			for i := 0; i < 600; i++ {
				t0 = t0.Add(100 * time.Millisecond)
				w.TickCount++
				var in, giveUp bool
				if middle {
					in, giveUp = w.chaseLocked(npc, tr.mx, tr.my, tr.mz, 0, 0, t0)
				} else {
					in, giveUp = w.chaseLocked(npc, tr.tx, tr.ty, tr.tz, 0, 0, t0)
				}
				if giveUp {
					break
				}
				if wet := w.npcInWaterLocked(npc); (middle && wet && distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) > 3) || (!middle && in && !wet) {
					ok = true
					break
				}
			}
			if !ok {
				npc.chase = chaseState{}
				npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
				continue
			}
			reached++
			if deep && dry && !middle {
				w.routeDry = true
				fx, fy, fz := npc.chase.fromX, npc.chase.fromY, npc.chase.fromZ
				b := time.Now()
				pts, tried, found := w.planRouteLocked(npc, fx, fy, fz, 40000)
				took := time.Since(b)
				var wetLen float32
				px, py, pz := npc.X, npc.Y, npc.Z
				if f, ok := w.ground.Floor(npc.World, px, py, pz); ok {
					py = f
				}
				for _, p := range pts {
					w.routeLegLocked(npc, px, py, pz, p.x, p.z)
					wetLen += w.routeWet
					px, py, pz = p.x, p.y, p.z
				}
				w.routeDry = false
				switch {
				case !found:
					deepNone++
				case wetLen > 0:
					deepWet++
				default:
					deepDry++
					deepTried = append(deepTried, tried)
				}
				deepTime += took
			}
			dryStart := !w.npcInWaterLocked(npc)
			if !dryStart {
				startWet++
			}
			w.giveUpLocked(npc)
			wet := 0
			b := time.Now()
			for i := 0; i < 1200 && npc.chase.returning; i++ {
				t0 = t0.Add(100 * time.Millisecond)
				w.TickCount++
				px, pz := npc.X, npc.Z
				w.homeSystem(t0)
				if distXZ(px, pz, npc.X, npc.Z) > 5 {
					jumps++
				}
				ticks++
				if w.npcInWaterLocked(npc) {
					wet++
				}
			}
			spent += time.Since(b)
			if !npc.chase.returning {
				gotHome++
			}
			wetTicks += wet
			if wet > 0 {
				wetTrips++
				if dryStart {
					wetFromDry++
				}
			}
			w.returning = nil
			npc.chase = chaseState{}
			npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
			npc.stopMoving()
			npc.swimming = false
		}
		where := "the far shore"
		if middle {
			where = "the water's middle"
		}
		t.Logf("home (dry way %v) from %s: %d of %d chases got there (%d in the water); %d trips home in the water (%d from a dry start), %d ticks of %d in it; %d home, %d set home (held), %d searches round (%d found nothing); %v the trips (%v a trip)",
			dry, where, reached, len(trips), startWet, wetTrips, wetFromDry, wetTicks, ticks, gotHome, jumps,
			strings.Count(buf.String(), "goes round")+strings.Count(buf.String(), "finds no way"), strings.Count(buf.String(), "finds no way"),
			spent.Round(time.Millisecond), (spent / time.Duration(max(reached, 1))).Round(time.Microsecond))
		return wetFromDry
	}
	for _, middle := range []bool{false, true} {
		before, after := home(false, middle), home(true, middle)
		if !middle && after*2 > before {
			t.Errorf("from the far shore %d ways home from the land went into the water, %d without the dry way: want under half", after, before)
		}
		if deep && !middle {
			sort.Ints(deepTried)
			t.Logf("a search of 40000 points from the far shore: %d found no way, %d only through the water, %d a dry way (points tried %v), %v", deepNone, deepWet, deepDry, deepTried, deepTime)
		}
		if os.Getenv("EQOA_TRACE") != "" {
			for _, l := range strings.Split(buf.String(), "\n") {
				if strings.Contains(l, "finds no way") || strings.Contains(l, "goes round") {
					t.Log(l)
				}
			}
		}
	}

	// the cost of a look at the water against one at the floor
	{
		var pts [][3]float32
		for _, npc := range shore {
			pts = append(pts, [3]float32{npc.SpawnX, npc.SpawnY, npc.SpawnZ})
		}
		const reps = 200
		b := time.Now()
		hits := 0
		for r := 0; r < reps; r++ {
			for i, p := range pts {
				if w.waterAtLocked(shore[i].World, p[0]+float32(r)*0.1, p[1], p[2]) {
					hits++
				}
			}
		}
		water := time.Since(b) / time.Duration(reps*len(pts))
		b = time.Now()
		for r := 0; r < reps; r++ {
			for i, p := range pts {
				if _, ok := w.ground.Floor(shore[i].World, p[0]+float32(r)*0.1, p[1]+1, p[2]); ok {
					hits++
				}
			}
		}
		floor := time.Since(b) / time.Duration(reps*len(pts))
		t.Logf("a look at the water %v, at the floor %v (%d)", water, floor, hits)
	}

	// the GM paths of the land's creatures across the water
	pathsN, wetPaths, wetLegs := 0, 0, 0
	for id, p := range w.paths.byNPC {
		npc := w.NPCs[id]
		if npc == nil || !envWorld(p.World) || w.keepsToLocked(npc) != keepsToLand {
			continue
		}
		pathsN++
		n := len(p.Points)
		legs := n - 1
		if p.Mode == PathLoop {
			legs = n
		}
		any := false
		for i := 0; i < legs; i++ {
			a, b := p.Points[i], p.Points[(i+1)%n]
			d := distXZ(a.X, a.Z, b.X, b.Z)
			steps := int(d/2) + 1
			for s := 0; s <= steps; s++ {
				f := float32(s) / float32(steps)
				x, y, z := a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f, a.Z+(b.Z-a.Z)*f
				if fl, ok := w.ground.Floor(p.World, x, y+2, z); ok && npc.onGround {
					y = fl
				}
				if w.waterAtLocked(p.World, x, y+bodyAbove, z) {
					wetLegs++
					any = true
					break
				}
			}
		}
		if any {
			wetPaths++
		}
	}
	t.Logf("paths: %d of land creatures, %d cross the water (%d legs)", pathsN, wetPaths, wetLegs)
}

package entity

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// Pets in Permafrost on the real ground (petition 1496, the magician Waldorf's:
// "pets in general don't respect walls/barriers/floors/ceilings"): a pet 10 to
// 70 off its owner across the ground, each standing where one of the caves'
// creatures spawns (so both on a floor), is told to follow, pair after pair. A
// step "goes through" when the way from where the pet stood to where it stands
// crosses a wall at its knee (the ground's own Wall: the terrain's steep faces,
// what is placed) or takes it past a ledge's drop or the routes' climb at
// once. With the routes, a pet that found a way never goes through anything
// and nearly every one comes to its owner on its owner's floor; the control,
// the same pairs with the routes off, goes through walls and floors. Skipped
// without EQOA_GROUND_DIR and EQOA_REAL_DB.
func TestRealPetsKeepToPermafrostsCaves(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	db := simRealDB(t)
	src := NewWorld()
	src.LoadNPCs(database.NewNPCStore(db))
	type spot struct{ x, y, z float32 }
	var spots []spot
	// EQOA_PET_BOX "x0,x1,z0,z1" walks another place of world 0 instead, for
	// looking at by hand (its figures logged, nothing held)
	box, elsewhere := [4]float32{4700, 5100, 4200, 4600}, false
	if v := os.Getenv("EQOA_PET_BOX"); v != "" {
		if n, _ := fmt.Sscanf(v, "%f,%f,%f,%f", &box[0], &box[1], &box[2], &box[3]); n != 4 {
			t.Fatalf("EQOA_PET_BOX %q: x0,x1,z0,z1", v)
		}
		elsewhere = true
	}
	for _, n := range src.NPCs {
		if n.World != 0 || n.SpawnX < box[0] || n.SpawnX > box[1] || n.SpawnZ < box[2] || n.SpawnZ > box[3] {
			continue
		}
		if f, ok := g.Floor(0, n.SpawnX, n.SpawnY, n.SpawnZ); ok && n.SpawnY-f <= petOnTerrain {
			spots = append(spots, spot{n.SpawnX, f, n.SpawnZ})
		}
	}
	sort.Slice(spots, func(i, j int) bool {
		if spots[i].x != spots[j].x {
			return spots[i].x < spots[j].x
		}
		return spots[i].z < spots[j].z
	})
	if len(spots) < 60 && !elsewhere {
		t.Fatalf("only %d of Permafrost's spawns stand on the ground's floor", len(spots))
	}
	type pair struct{ pet, owner spot }
	var pairs []pair
	for i, a := range spots {
		for j, b := range spots {
			if d := distXZ(a.x, a.z, b.x, b.z); i != j && d >= 8 && d <= 45 && (i*31+j*17)%11 == 0 {
				pairs = append(pairs, pair{a, b})
			}
		}
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	type result struct {
		through, ticks int
		arrived        bool
		found, noWay   int
		took           time.Duration
	}
	run := func(routes bool, p pair) (res result) {
		was := chaseRoutes
		chaseRoutes = routes
		defer func() { chaseRoutes = was }()
		r := newPetRig(t)
		r.w.SetGround(g)
		r.w.SpatialIndex.Update(r.p.ServerID, r.p.X, r.p.Z, p.owner.x, p.owner.z)
		r.p.X, r.p.Y, r.p.Z, r.p.World = p.owner.x, p.owner.y, p.owner.z, 0
		pet, _ := r.summon(t, fireElemental(), 0)
		r.w.SpatialIndex.Update(pet.ServerID, pet.X, pet.Z, p.pet.x, p.pet.z)
		pet.X, pet.Y, pet.Z = p.pet.x, p.pet.y, p.pet.z
		r.w.pets[r.p.ServerID].last = r.at(10 * time.Second) // a tick's step from the first
		buf.Reset()
		start := time.Now()
		px, py, pz := pet.X, pet.Y, pet.Z
		at := r.walk(pet, 400, func() bool {
			res.ticks++
			if dy := pet.Y - py; dy < -legDrop-0.5 || dy > routeClimb+0.5 ||
				(g.Wall(0, px, py+legKnee, pz, pet.X, pet.Y+legKnee, pet.Z) && g.Wall(0, px, py+3, pz, pet.X, pet.Y+3, pet.Z)) {
				res.through++
				log.Printf("THROUGH (%.1f, %.1f, %.1f) -> (%.1f, %.1f, %.1f) route %d", px, py, pz, pet.X, pet.Y, pet.Z, len(pet.way.route.pts))
			}
			px, py, pz = pet.X, pet.Y, pet.Z
			return distXZ(pet.X, pet.Z, r.p.X, r.p.Z) <= petFollowStart && pet.Loco == LocoStand
		})
		res.took = time.Since(start)
		last := at[len(at)-1]
		res.arrived = distXZ(last[0], last[2], r.p.X, r.p.Z) <= petFollowStart && absf(last[1]-r.p.Y) <= routeLevel
		res.found = strings.Count(buf.String(), "goes round")
		res.noWay = strings.Count(buf.String(), "finds no way")
		return res
	}
	var routed, clean, arrived, straight, straightThrough, noWay, noWayThrough, controlThrough, controlArrived, searches int
	var sameFloor, sameNoWay, sameControl int // owner and pet on one floor (within routeLevel): how many, with no way found, the control through
	var took, worst time.Duration
	for _, p := range pairs {
		res := run(true, p)
		same := absf(p.pet.y-p.owner.y) <= routeLevel
		if same {
			sameFloor++
			if res.noWay > 0 {
				sameNoWay++
			}
		}
		took += res.took
		worst = max(worst, res.took/time.Duration(max(res.ticks, 1)))
		searches += res.found + res.noWay
		switch {
		case res.noWay > 0:
			noWay++
			if res.through > 0 {
				noWayThrough++
			}
			if os.Getenv("EQOA_TRACE") != "" {
				for _, l := range strings.Split(buf.String(), "\n") {
					if strings.Contains(l, "finds no way") {
						t.Logf("no way: %s", l)
						break
					}
				}
			}
		case res.found == 0:
			straight++
			if res.through > 0 {
				straightThrough++
				t.Logf("straight, through %d: pet (%.0f, %.0f, %.0f) to its owner (%.0f, %.0f, %.0f)", res.through, p.pet.x, p.pet.y, p.pet.z,
					p.owner.x, p.owner.y, p.owner.z)
			}
		default:
			routed++
			if res.through == 0 {
				clean++
			} else {
				t.Logf("routed, through %d: pet (%.0f, %.0f, %.0f) to its owner (%.0f, %.0f, %.0f)\n%s", res.through, p.pet.x, p.pet.y, p.pet.z,
					p.owner.x, p.owner.y, p.owner.z, lastLines(buf.String(), 8))
			}
			if res.arrived {
				arrived++
			}
		}
		c := run(false, p)
		if c.through > 0 {
			controlThrough++
			if same {
				sameControl++
			}
		}
		if c.arrived {
			controlArrived++
		}
	}
	t.Logf("%d pets following in x %.0f-%.0f, z %.0f-%.0f (Permafrost's unless EQOA_PET_BOX): %d straight all the way (%d through something), %d with a way round (%d never through "+
		"anything, %d came to their owner), %d with a search that found none (%d of them through something, as before); the control "+
		"(no routes): %d went through a wall or a floor, %d came; %d searches, %v a walk on average, %v a tick at most", len(pairs),
		box[0], box[1], box[2], box[3], straight, straightThrough, routed, clean, arrived, noWay, noWayThrough, controlThrough, controlArrived, searches,
		took/time.Duration(max(len(pairs), 1)), worst)
	t.Logf("of them %d with pet and owner on one floor: %d found no way, the control through something in %d", sameFloor, sameNoWay, sameControl)
	if elsewhere {
		return
	}
	if routed == 0 || controlThrough == 0 {
		t.Fatalf("%d pairs: %d routed, the control through something in %d: the pairs test nothing", len(pairs), routed, controlThrough)
	}
	if clean < routed*9/10 || arrived < routed*9/10 {
		t.Errorf("of %d pets with a way round, %d never went through anything and %d came to their owner", routed, clean, arrived)
	}
	if straightThrough > straight/20 {
		t.Errorf("%d of %d pets on a straight way went through something", straightThrough, straight)
	}
}

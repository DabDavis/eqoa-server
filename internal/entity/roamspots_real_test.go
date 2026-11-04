package entity

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/eqoa/server/internal/ground"
)

// The spots on a real world's ground (EQOA_GROUND_DIR, cmd/groundbake's)
// under its real creatures (EQOA_REAL_DB): 600 wandering creatures of the
// land on the ground, each walking 80 legs as its walk walks them, and every
// leg it is given checked whole again from where it stood: a leg its spots
// give it must be one the whole check walks, on the same floor, so a way
// remembered walkable never walks a creature where the check would not
// have. Skipped without both: neither is in this repository.
func TestRealSpotLegsAreWalkable(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := realWorld(t)
	w.SetGround(g)
	if env, err := ground.OpenEnv(dir); err == nil {
		w.SetEnv(env)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var ids []EntityID
	for id, npc := range w.NPCs {
		if w.wanders(npc) && w.spawnOnGroundLocked(npc) && (w.env == nil || w.keepsToLocked(npc) == keepsToLand) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if step := len(ids) / 600; step > 1 {
		var some []EntityID
		for i := 0; i < len(ids); i += step {
			some = append(some, ids[i])
		}
		ids = some
	}
	legs, toSpots, fromSpot, refused, offFloor, standing := 0, 0, 0, 0, 0, 0
	var again []float32
	var spent [2]time.Duration // picking legs before its spots were all made, and after
	var picked [2]int
	for _, id := range ids {
		npc := w.NPCs[id]
		for i := 0; i < 80; i++ {
			sx, sy, sz := npc.X, npc.Y, npc.Z
			_, from := w.spotsLocked(npc, w.roamRadiusLocked(npc))
			full := npc.spots != nil && npc.spots.n == wanderSpotsMax
			t0 := time.Now()
			x, z := w.roamPointLocked(npc)
			took := time.Since(t0)
			if full {
				spent[1] += took
				picked[1]++
			} else {
				spent[0] += took
				picked[0]++
			}
			npc.roam.toX, npc.roam.toZ = x, z // as roamStepLocked sets them
			if x == sx && z == sz {
				standing++
				continue
			}
			legs++
			if full && npc.spots.at(x, npc.roam.heights[len(npc.roam.heights)-1], z) >= 0 {
				toSpots++
				if from >= 0 {
					fromSpot++ // from a spot too: its way remembered, or learned now
				}
			}
			got := append([]float32(nil), npc.roam.heights...)
			again = again[:0]
			if !w.legCheckLocked(npc, x, z, &again) {
				refused++
				t.Errorf("creature %d (%s) at (%.2f, %.2f, %.2f): a leg to (%.2f, %.2f) the whole check refuses", id, npc.Name, sx, sy, sz, x, z)
				if refused > 10 {
					t.FailNow()
				}
			} else if len(again) != len(got) {
				offFloor++
			} else {
				for k := range got {
					if got[k] != again[k] {
						offFloor++
						break
					}
				}
			}
			for k := 0; !w.stepNPCOnLegLocked(npc, NPCWalkSpeed, 0.1); k++ { // its walk to the leg's end
				if k > 1000 {
					t.Fatalf("creature %d: a leg of %.1f not walked in 1000 steps", id, distXZ(sx, sz, x, z))
				}
			}
		}
	}
	t.Logf("%d creatures, %d legs (%d stood): %d to their spots, %d of them from a spot; refused by the whole check %d, on another floor %d",
		len(ids), legs, standing, toSpots, fromSpot, refused, offFloor)
	t.Logf("a pick took %.1f µs while its spots were being made (every leg checked whole), %.1f µs after",
		float64(spent[0].Microseconds())/float64(max(picked[0], 1)), float64(spent[1].Microseconds())/float64(max(picked[1], 1)))
	if legs < 10000 {
		t.Errorf("only %d legs walked", legs)
	}
	if offFloor != 0 {
		t.Errorf("%d legs read another floor than the whole check's", offFloor)
	}
}

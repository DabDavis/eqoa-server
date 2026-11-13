package entity

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/ground"
)

// terrainOnly is a real ground as the creatures' moves had it before it knew
// placed floors: its Floor, Wall and Placed, not its Stand (the control).
type terrainOnly struct{ g *ground.Ground }

func (f terrainOnly) Floor(w uint8, x, y, z float32) (float32, bool) { return f.g.Floor(w, x, y, z) }
func (f terrainOnly) Wall(w uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	return f.g.Wall(w, x0, y0, z0, x1, y1, z1)
}
func (f terrainOnly) Placed(w uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	return f.g.Placed(w, x0, y0, z0, x1, y1, z1)
}

// world14Ground opens EQOA_GROUND_DIR's world 14 (the dungeons), skipping
// without it.
func world14Ground(t *testing.T) *ground.Ground {
	t.Helper()
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground with world 14 (the dungeons)")
	}
	if _, err := os.Stat(filepath.Join(dir, "14.ground")); err != nil {
		t.Skip("no 14.ground in EQOA_GROUND_DIR")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g
}

// surfacer is a ground's every surface over a place between two heights
// (internal/ground's Surfaces).
type surfacer interface {
	Surfaces(world uint8, x, z, lo, hi float32) []ground.Surface
}

// throughAFloor is the first tick a creature stood where CLIENT would show it
// in or under a floor, or went through one between two ticks: a floor (a
// surface facing up) over its feet within its body (floorHead), or a surface
// between its height at one tick and the next at the new place, a floor it went
// down through, or a ceiling it went up through further than a step (a step
// up onto a slab passes its underside, as CLIENT's ground probe does).
func throughAFloor(g surfacer, world uint8, at [][3]float32) (int, string) {
	const floorHead, margin = 2.5, 0.15
	for i, p := range at {
		for _, s := range g.Surfaces(world, p[0], p[2], p[1]+margin, p[1]+floorHead) {
			if s.Up {
				return i, fmt.Sprintf("a floor at %.2f over its feet at %.2f", s.Y, p[1])
			}
		}
		if i == 0 {
			continue
		}
		y0, y1 := at[i-1][1], p[1]
		lo, hi := min(y0, y1)+margin, max(y0, y1)-margin
		if hi <= lo || (y1 > y0 && y1-y0 <= ground.StepUp) {
			continue
		}
		for _, s := range g.Surfaces(world, p[0], p[2], lo, hi) {
			if s.Up == (y1 < y0) {
				return i, fmt.Sprintf("went from %.2f to %.2f through a surface at %.2f", y0, y1, s.Y)
			}
		}
	}
	return -1, ""
}

// Collonridge Cemetery's floors over floors (the owner, 2026-11-13: "mobs are
// going through the floor in the collinridge cemetary instance that zerkziee is
// in"): its mausoleum, a placed slab at 56.4 over the terrain's 54.1 with a step
// round it; its crypt, terrain at 42.1 under the terrain's 54.1; its tower, placed
// floors at 56.4, 62.7, 68.7 and 74.8. Each chase is a creature at a spawn of the
// instance's after an enemy on another floor (or on the slab). With placed floors
// known (Stand) no chase stands in a floor or goes through one, and those with a
// way reach their enemy; the control (the terrain's floors alone, as before) put
// the mausoleum's chasers under its floor, as the live log had them (06:55:59 to
// 06:56:39 UTC, a lesser ghoul and a Wilting Flesh at 54.1 under a warrior on
// the floor at 56.4). Skipped without a world 14 ground (EQOA_GROUND_DIR).
func TestCollonridgeChasesKeepToTheirFloors(t *testing.T) {
	g := world14Ground(t)
	type chase struct {
		what        string
		from, enemy [3]float32
		reach       bool // a way the creature finds to it (the crypt's stairs are past a search's reach from over it: it holds, and gives up)
	}
	chases := []chase{
		{"onto the mausoleum's floor", [3]float32{22612, 54.1, 8790}, [3]float32{22590, 56.4, 8806}, true},
		{"off the mausoleum's floor", [3]float32{22598.0, 56.4, 8813.9}, [3]float32{22560, 54.1, 8835}, true},
		{"crypt to the ground over it", [3]float32{22702.5, 42.1, 8740.6}, [3]float32{22705, 54.1, 8745}, true},
		{"ground to the crypt under it", [3]float32{22702.7, 54.1, 8745.7}, [3]float32{22700, 42.1, 8740}, false},
		{"tower's 62.7 to the ground", [3]float32{22873.0, 62.7, 8767.7}, [3]float32{22890, 54.1, 8740}, true},
		{"tower's 74.8 to its 56.4", [3]float32{22861.2, 74.8, 8768.5}, [3]float32{22850, 56.4, 8768}, true},
		{"ground to the tower's 62.7", [3]float32{22856.7, 54.1, 8741.3}, [3]float32{22873.0, 62.7, 8767.7}, true},
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	run := func(gr Ground, c chase) (bool, [][3]float32) {
		w := roamWorld(t)
		w.SetGround(gr)
		npc := addNPC(w, 100, "lesser ghoul", c.from[0], c.from[2], creatureModel, 40)
		npc.World, npc.Y, npc.SpawnY = 14, c.from[1], c.from[1]
		return chaseOver(w, npc, c.enemy[0], c.enemy[1], c.enemy[2], 600)
	}
	controlBad := 0
	for _, c := range chases {
		buf.Reset()
		ok, at := run(g, c)
		last := at[len(at)-1]
		if i, why := throughAFloor(g, 14, at); i >= 0 {
			t.Errorf("%s: tick %d at (%.1f, %.1f, %.1f): %s\n%s", c.what, i, at[i][0], at[i][1], at[i][2], why, lastLines(buf.String(), 4))
		}
		if c.reach && !ok {
			t.Errorf("%s: never reached its enemy: at (%.1f, %.1f, %.1f) after %d ticks\n%s", c.what, last[0], last[1], last[2], len(at)-1, lastLines(buf.String(), 4))
		}
		if !c.reach && absf(last[1]-c.from[1]) > 0.5 {
			t.Errorf("%s: no way to its enemy, but it left its floor: at (%.1f, %.1f, %.1f)", c.what, last[0], last[1], last[2])
		}
		t.Logf("%s: reached %v, at (%.1f, %.1f, %.1f) after %d ticks", c.what, ok, last[0], last[1], last[2], len(at)-1)
		_, cat := run(terrainOnly{g}, c)
		if i, why := throughAFloor(g, 14, cat); i >= 0 {
			controlBad++
			t.Logf("the control, %s: tick %d: %s", c.what, i, why)
		}
	}
	if controlBad == 0 {
		t.Error("the control (the terrain's floors alone) never stood in a floor")
	}
}

// The tower's servant on its floor at 74.8, its fight taken down the stairs to
// the floor at 56.4, goes home up the stairs: never up through a floor, home at
// the height it left. Before, its way home asked the floor from the height it
// left (74.8), and it stood on that floor at its first step, up through the two
// between. Skipped without a world 14 ground (EQOA_GROUND_DIR).
func TestCollonridgeTowerCreatureGoesHomeByItsStairs(t *testing.T) {
	g := world14Ground(t)
	w := roamWorld(t)
	w.SetGround(g)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	npc := addNPC(w, 100, "skeletal servant", 22861.2, 8768.5, creatureModel, 40)
	npc.World, npc.Y, npc.SpawnY = 14, 74.8, 74.8
	ok, at := chaseOver(w, npc, 22850, 56.4, 8768, 600)
	if !ok {
		last := at[len(at)-1]
		t.Fatalf("never came down to its enemy: at (%.1f, %.1f, %.1f)\n%s", last[0], last[1], last[2], lastLines(buf.String(), 4))
	}
	home, hat := homeOver(w, npc, 600)
	if i, why := throughAFloor(g, 14, hat); i >= 0 {
		t.Errorf("tick %d of its way home at (%.1f, %.1f, %.1f): %s", i, hat[i][0], hat[i][1], hat[i][2], why)
	}
	last := hat[len(hat)-1]
	if !home || absf(last[1]-74.8) > 0.1 {
		t.Errorf("home %v at (%.1f, %.1f, %.1f), want its place at 74.8", home, last[0], last[1], last[2])
	}
}

// homeOver runs npc's way home after its fight a tick at a time (a tenth of a
// second) until it is home or ticks pass, and reports the heights it stood at.
func homeOver(w *World, npc *NPC, ticks int) (bool, [][3]float32) {
	now := time.Unix(1_800_100_000, 0)
	w.mu.Lock()
	w.giveUpLocked(npc)
	w.mu.Unlock()
	at := [][3]float32{{npc.X, npc.Y, npc.Z}}
	for i := 0; i < ticks && len(w.returning) > 0; i++ {
		now = now.Add(100 * time.Millisecond)
		w.TickCount++
		w.mu.Lock()
		w.homeSystem(now)
		w.mu.Unlock()
		at = append(at, [3]float32{npc.X, npc.Y, npc.Z})
	}
	return len(w.returning) == 0, at
}

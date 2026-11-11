package entity

import (
	"os"
	"testing"
	"time"

	"github.com/eqoa/server/internal/ground"
)

// shoreCase is a creature swimming in deep water at (wx, wz) and its enemy on
// the land at (lx, ly, lz), past the shore.
type shoreCase struct {
	wx, wz     float32
	lx, ly, lz float32
}

// realShores finds up to most places in world 0 within span of (cx, cz) where
// deep water (a creature of the land swims there) meets land within 24 along
// one of the four ways, the land's floor out of the water.
func realShores(w *World, cx, cz, span float32, most int) []shoreCase {
	var out []shoreCase
	dirs := [4][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for x := cx - span; x <= cx+span && len(out) < most; x += 16 {
		for z := cz - span; z <= cz+span && len(out) < most; z += 16 {
			bed, ok := w.ground.Floor(0, x, 2000, z)
			if !ok {
				continue
			}
			top, ok := w.waterSurfaceLocked(0, x, bed+bodyAbove, z, 0)
			if !ok || top-bed < 4 {
				continue
			}
			for _, d := range dirs {
				for s := float32(2); s <= 24; s += 2 {
					lx, lz := x+d[0]*s, z+d[1]*s
					ly, ok := w.ground.Floor(0, lx, 2000, lz)
					if !ok || ly < top+0.5 || ly > top+40 || w.waterAtLocked(0, lx, ly+bodyAbove, lz) {
						continue
					}
					// the enemy a little past the shore, on that floor
					ex, ez := lx+d[0]*6, lz+d[1]*6
					ey, ok := w.ground.Floor(0, ex, ly+3, ez)
					if !ok || w.waterAtLocked(0, ex, ey+bodyAbove, ez) {
						break
					}
					out = append(out, shoreCase{x, z, ex, ey, ez})
					break
				}
			}
		}
	}
	return out
}

// underGround reports a creature standing under the terrain: no floor at or
// under it (within the ground's step) but one over it within 60.
func underGround(w *World, npc *NPC) bool {
	if _, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok {
		return false
	}
	_, over := w.ground.Floor(npc.World, npc.X, npc.Y+60, npc.Z)
	return over
}

// Petition 1500 (Jibrish): "Players are able to run monsters into the water,
// swim down, then run quickly on land and the mobs can end up underneath the
// world floor, unable to hit the player." On the real ground and water of
// Tunaria about Freeport's harbour: a creature of the land swimming in deep
// water chases its enemy up onto the shore; it never ends a tick under the
// terrain. Skipped without EQOA_GROUND_DIR.
func TestSwimmerChasesOntoTheShoreNotUnderIt(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Skip("no water in the bake: ", err)
	}
	w := roamWorld(t)
	w.SetGround(g)
	w.SetEnv(env)
	w.mu.Lock()
	cases := realShores(w, 25316, 15750, 1600, 400)
	w.mu.Unlock()
	if len(cases) < 20 {
		t.Fatalf("only %d shores found", len(cases))
	}
	under, reached := 0, 0
	for i, c := range cases {
		npc := addNPC(w, EntityID(500000+i), "a sand giant", c.lx, c.lz, creatureModel, 40)
		npc.Y, npc.SpawnY = c.ly, c.ly // spawned on the land, a creature of it
		npc.X, npc.Z = c.wx, c.wz
		w.mu.Lock()
		bed, _ := w.ground.Floor(0, c.wx, 2000, c.wz)
		npc.Y = bed
		w.groundNPCLocked(npc) // at the surface, swimming
		swam := npc.swimming
		w.mu.Unlock()
		if !swam {
			delete(w.NPCs, npc.ServerID)
			continue
		}
		ok, at := chaseOver(w, npc, c.lx, c.ly, c.lz, 300)
		if ok {
			reached++
		}
		w.mu.Lock()
		for j, p := range at {
			probe := &NPC{World: 0, X: p[0], Y: p[1], Z: p[2]}
			if underGround(w, probe) {
				under++
				if under <= 10 {
					t.Logf("case %d: under the ground at tick %d (%.1f, %.1f, %.1f), from the water at (%.0f, %.0f) to (%.1f, %.1f, %.1f)",
						i, j, p[0], p[1], p[2], c.wx, c.wz, c.lx, c.ly, c.lz)
				}
				break
			}
		}
		w.mu.Unlock()
		delete(w.NPCs, npc.ServerID)
	}
	t.Logf("%d shores: %d reached their enemy, %d went under the ground", len(cases), reached, under)
	if under > 0 {
		t.Errorf("%d of %d chases out of the water went under the ground", under, len(cases))
	}
}

// shorePath is a player's way in petition 1500: from the land at the case's
// enemy place into the water, down to its bed, held there, back out the way
// it came and on inland, a place every tenth of a second.
func shorePath(w *World, c shoreCase) [][3]float32 {
	bed, _ := w.ground.Floor(0, c.wx, 2000, c.wz)
	var path [][3]float32
	line := func(x0, y0, z0, x1, y1, z1 float32, onLand bool) {
		d := distXZ(x0, z0, x1, z1)
		n := int(d/0.883) + 1
		for i := 1; i <= n; i++ {
			f := float32(i) / float32(n)
			x, z := x0+(x1-x0)*f, z0+(z1-z0)*f
			y := y0 + (y1-y0)*f
			if fy, ok := w.ground.Floor(0, x, max(y, y0, y1)+3, z); ok && (onLand || fy > y) {
				y = fy
			}
			path = append(path, [3]float32{x, y, z})
		}
	}
	dx, dz := c.lx-c.wx, c.lz-c.wz
	d := distXZ(c.wx, c.wz, c.lx, c.lz)
	ux, uz := dx/d, dz/d
	line(c.lx, c.ly, c.lz, c.wx, bed+0.3, c.wz, false)
	for i := 0; i < 30; i++ {
		path = append(path, path[len(path)-1])
	}
	line(c.wx, bed+0.3, c.wz, c.lx, c.ly, c.lz, false)
	line(c.lx, c.ly, c.lz, c.lx+ux*40, c.ly, c.lz+uz*40, true)
	for i := 0; i < 60; i++ {
		path = append(path, path[len(path)-1])
	}
	return path
}

// The shores of all of Tunaria with a player who pulls the creature into the
// water, dives to the bed, and runs back out onto the land and on (petition
// 1500's way), the ticks a tenth of a second apart and, as on a loaded server,
// three tenths: the creature never ends a tick under the terrain. Before the
// fix 41 of 4,001 such chases went under it at a tenth, 693 at three tenths;
// the enemy's way here is rough (on the land it keeps to its first floor, and
// can pass under a hill), which is what a chase must survive. Skipped without
// EQOA_GROUND_DIR.
func TestDiveAndRunOutKeepsTheChaserAboveGround(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Skip("no water in the bake: ", err)
	}
	w := roamWorld(t)
	w.SetGround(g)
	w.SetEnv(env)
	w.mu.Lock()
	cases := realShores(w, 15000, 15000, 15000, 1500)
	w.mu.Unlock()
	if len(cases) < 1000 {
		t.Fatalf("only %d shores found", len(cases))
	}
	for _, tick := range []time.Duration{100 * time.Millisecond, 300 * time.Millisecond} {
		under, ended, gaveUp := 0, 0, 0
		for i, c := range cases {
			w.mu.Lock()
			path := shorePath(w, c)
			npc := addNPC(w, EntityID(600000+i), "a sand giant", c.lx+1, c.lz, creatureModel, 40)
			npc.Y, _ = w.ground.Floor(0, npc.X, c.ly+3, npc.Z)
			npc.SpawnY = npc.Y
			w.mu.Unlock()
			now := time.Unix(1_800_000_000, 0)
			for j, p := range path {
				now = now.Add(tick)
				w.TickCount++
				var vx, vz float32
				if j > 0 {
					vx, vz = (p[0]-path[j-1][0])*10, (p[2]-path[j-1][2])*10
				}
				w.mu.Lock()
				_, giveUp := w.chaseLocked(npc, p[0], p[1], p[2], vx, vz, now)
				bad := underGround(w, npc)
				w.mu.Unlock()
				if bad {
					under++
					if under <= 10 {
						t.Logf("%v: case %d under the ground at tick %d of %d (%.1f, %.1f, %.1f), its enemy at (%.1f, %.1f, %.1f)",
							tick, i, j, len(path), npc.X, npc.Y, npc.Z, p[0], p[1], p[2])
					}
					break
				}
				if giveUp {
					gaveUp++
					break
				}
			}
			last := path[len(path)-1]
			if distXZ(npc.X, npc.Z, last[0], last[2]) <= npcReach(npc) && absf(npc.Y-last[1]) <= routeLevel {
				ended++
			}
			delete(w.NPCs, npc.ServerID)
		}
		t.Logf("ticks %v, %d dives: %d chasers reached the player on the land after, %d gave up, %d went under the ground",
			tick, len(cases), ended, gaveUp, under)
		if under > 0 {
			t.Errorf("ticks %v: %d of %d chasers went under the ground", tick, under, len(cases))
		}
	}
}

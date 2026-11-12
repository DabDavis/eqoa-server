package entity

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// A magician with its fire elemental, a merc and Healbot on the real ground
// (followslot.go, mercidle.go): in Permafrost's caves (or EQOA_PET_BOX's
// place of world 0) the owner walks from one creature's spawn to another 15
// to 40 off along a way a pet could walk straight, then stands 40 s. Measured
// every tick: a follower's step through a wall (at its knee and its head, as
// petwalk_real_test.go counts one) or past a fall or a climb; at the stop
// (4 s on) two followers under 1.5 apart or one under 1.5 off its owner; at
// ease, the merc's strolls and any of them out of its owner's sight. The
// control: the same walks with the slots off (the old follow), all three
// stacked. EQOA_FOLLOW_CSV writes the first walk's places, tick by tick.
// Skipped without EQOA_GROUND_DIR and EQOA_REAL_DB.
func TestRealFollowersSpreadAndStroll(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	src := NewWorld()
	src.LoadNPCs(database.NewNPCStore(simRealDB(t)))
	box := [4]float32{4700, 5100, 4200, 4600}
	if v := os.Getenv("EQOA_PET_BOX"); v != "" {
		if n, _ := fmt.Sscanf(v, "%f,%f,%f,%f", &box[0], &box[1], &box[2], &box[3]); n != 4 {
			t.Fatalf("EQOA_PET_BOX %q: x0,x1,z0,z1", v)
		}
	}
	type spot struct{ x, y, z float32 }
	var spots []spot
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
	// walks a pet could take straight (the leg a pet's way checks)
	probe := NewWorld()
	probe.SetGround(g)
	walker := &NPC{Owner: 1, World: 0}
	type walk struct{ a, b spot }
	var walks []walk
	for i, a := range spots {
		for j, b := range spots {
			d := distXZ(a.x, a.z, b.x, b.z)
			if i == j || d < 15 || d > 40 || (i*31+j*17)%7 != 0 {
				continue
			}
			walker.X, walker.Y, walker.Z = a.x, a.y, a.z
			if probe.legFromLocked(walker, a.x, a.y, a.z, b.x, b.z, nil, legRoute) {
				walks = append(walks, walk{a, b})
			}
		}
	}
	if len(walks) > 60 {
		walks = walks[:60]
	}
	if len(walks) < 10 {
		t.Fatalf("only %d walks in the box", len(walks))
	}
	type result struct {
		through, overlaps, slotless, strolls, unseen, far int
	}
	run := func(slots bool, wk walk, csv *strings.Builder) (res result) {
		was := followSlots
		followSlots = slots
		defer func() { followSlots = was }()
		r, fs := spreadRig(t)
		r.w.SetGround(g)
		r.w.roamRnd = nil
		r.w.roamRand().Seed(int64(wk.a.x*7 + wk.b.z))
		r.p.World = 0
		h := headingTo(wk.b.x-wk.a.x, wk.b.z-wk.a.z)
		r.w.UpdatePosition(r.p.ServerID, wk.a.x, wk.a.y, wk.a.z, h)
		at := 14 * time.Second
		last := make([][3]float32, len(fs))
		tick := func(phase string) {
			at += 100 * time.Millisecond
			followTick(r.w, r.t0, at)
			for i, n := range fs {
				p := last[i]
				if p != ([3]float32{}) && n.World == 0 && distXZ(p[0], p[2], n.X, n.Z) < 20 {
					if dy := n.Y - p[1]; dy < -legDrop-0.5 || dy > routeClimb+0.5 ||
						(g.Wall(0, p[0], p[1]+legKnee, p[2], n.X, n.Y+legKnee, n.Z) && g.Wall(0, p[0], p[1]+3, p[2], n.X, n.Y+3, n.Z)) {
						res.through++
						if os.Getenv("EQOA_TRACE") != "" {
							t.Logf("THROUGH %s %s (%.1f,%.1f,%.1f)->(%.1f,%.1f,%.1f) owner (%.1f,%.1f,%.1f) route %d idle %+v", phase, n.Name,
								p[0], p[1], p[2], n.X, n.Y, n.Z, r.p.X, r.p.Y, r.p.Z, len(n.way.route.pts), r.w.merc1(r.p.ServerID).idle)
						}
					}
				}
				last[i] = [3]float32{n.X, n.Y, n.Z}
			}
			if csv != nil {
				fmt.Fprintf(csv, "%.1f,%s,%.2f,%.2f,%.2f", (at - 14*time.Second).Seconds(), phase, r.p.X, r.p.Y, r.p.Z)
				for _, n := range fs {
					fmt.Fprintf(csv, ",%.2f,%.2f,%.2f,%d", n.X, n.Y, n.Z, n.Loco)
				}
				csv.WriteString("\n")
			}
		}
		for range 30 { // called beside it there
			tick("arrive")
		}
		d := distXZ(wk.a.x, wk.a.z, wk.b.x, wk.b.z)
		steps := int(d / 0.88)
		for s := 1; s <= steps; s++ {
			f := float32(s) / float32(steps)
			x, z := wk.a.x+(wk.b.x-wk.a.x)*f, wk.a.z+(wk.b.z-wk.a.z)*f
			y := wk.a.y
			if fl, ok := g.Floor(0, x, r.p.Y, z); ok {
				y = fl
			}
			r.w.UpdatePosition(r.p.ServerID, x, y, z, h)
			tick("walk")
		}
		for range 40 {
			tick("stop")
		}
		for i, a := range fs {
			if c := &a.fslot; slots && !c.ok {
				res.slotless++
			}
			if distXZ(a.X, a.Z, r.p.X, r.p.Z) < 1.5 {
				res.overlaps++
			}
			for _, b := range fs[i+1:] {
				if distXZ(a.X, a.Z, b.X, b.Z) < 1.5 {
					res.overlaps++
					if slots && os.Getenv("EQOA_TRACE") != "" {
						t.Logf("OVERLAP %s (%.1f,%.1f,%.1f) slot %+v moving %v / %s (%.1f,%.1f,%.1f) slot %+v moving %v; owner (%.1f,%.1f,%.1f)",
							a.Name, a.X, a.Y, a.Z, a.fslot, a.moving(), b.Name, b.X, b.Y, b.Z, b.fslot, b.moving(), r.p.X, r.p.Y, r.p.Z)
					}
				}
			}
		}
		ms := r.w.merc1(r.p.ServerID)
		for range 400 {
			was := ms.idle.strolls
			mx, my, mz := ms.npc.X, ms.npc.Y, ms.npc.Z
			tick("ease")
			m := ms.npc
			if ms.idle.strolls != was && os.Getenv("EQOA_TRACE") != "" {
				t.Logf("STROLL from (%.1f,%.1f,%.1f) now (%.1f,%.1f) to (%.1f,%.1f) leg %v", mx, my, mz, m.X, m.Z, ms.idle.x, ms.idle.z,
					r.w.legFromLocked(followWalker(m), mx, my, mz, ms.idle.x, ms.idle.z, nil, legRoute))
			}
			if ms.idle.off && !m.moving() && !r.w.inSightLocked(0, r.p.X, r.p.Y, r.p.Z, m.X, m.Y, m.Z) {
				res.unseen++
			}
			if distXZ(m.X, m.Z, r.p.X, r.p.Z) > mercStrollFar+1 {
				res.far++
			}
		}
		res.strolls = ms.idle.strolls
		return res
	}
	var tot, ctl result
	var csv strings.Builder
	csv.WriteString("t,phase,ox,oy,oz,petx,pety,petz,petloco,mercx,mercy,mercz,mercloco,famx,famy,famz,famloco\n")
	for i, wk := range walks {
		var c *strings.Builder
		if i == 0 {
			c = &csv
		}
		res := run(true, wk, c)
		tot.through += res.through
		tot.overlaps += res.overlaps
		tot.slotless += res.slotless
		tot.strolls += res.strolls
		tot.unseen += res.unseen
		tot.far += res.far
		cr := run(false, wk, nil)
		ctl.overlaps += cr.overlaps
		ctl.through += cr.through
	}
	if p := os.Getenv("EQOA_FOLLOW_CSV"); p != "" {
		if err := os.WriteFile(p, []byte(csv.String()), 0o644); err != nil {
			t.Error(err)
		}
	}
	t.Logf("%d walks in x %.0f-%.0f, z %.0f-%.0f: through %d steps, %d overlaps at the stop (%d followers with no slot), "+
		"%d strolls in %d s at ease, %d ticks a strolled merc stood out of its owner's sight, %d ticks past %v off; "+
		"the control (no slots): %d overlaps, through %d", len(walks), box[0], box[1], box[2], box[3], tot.through, tot.overlaps,
		tot.slotless, tot.strolls, len(walks)*40, tot.unseen, tot.far, mercStrollFar+1, ctl.overlaps, ctl.through)
	if tot.overlaps > len(walks)/10 || tot.unseen > 0 || tot.far > 0 || tot.strolls < len(walks) {
		t.Errorf("overlaps %d, unseen %d, far %d, strolls %d", tot.overlaps, tot.unseen, tot.far, tot.strolls)
	}
	// a follower with no clear way to its slot nor to its owner follows on
	// the line as before (followTrailLocked: a familiar, never owned, steps
	// straight), so a few steps through stay, as the control's do
	if tot.through > ctl.through+len(walks)/20 {
		t.Errorf("the slots went through %d, the old follow %d", tot.through, ctl.through)
	}
}

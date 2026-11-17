package entity

import (
	"math"
	"os"
	"testing"

	"github.com/eqoa/server/internal/ground"
)

// Santa's tour over Freeport's real collision (EQOA_GROUND_DIR, cmd/groundbake's):
// every member of the party and every place on the deck, a tenth of a second
// at a time, clear of everything under and over it once off the sand (a body
// within 1.5 of a floor is set down on it by every client: ExtrapolateGround),
// and no step of the sleigh or of a horse through anything placed or the
// terrain (the owner's "through no walls"). Each view a PS2 rider is set down
// at stands on a floor. Skipped without a baked ground: none is in this
// repository.
func TestTheTourFliesClearOfFreeport(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	r := newTourRig(t)
	d := r.w.tours.defs["santa's sleigh tour"]
	veh := r.npc("Santa's Sleigh")
	members := []tourMember{{vehicle: true, home: [4]float32{veh.SpawnX, veh.SpawnY, veh.SpawnZ, veh.SpawnFacing}}}
	names := []string{"Santa's Sleigh"}
	for _, n := range []string{"Santa Claus", "Rudolph", "Dasher", "Dancer", "Prancer", "Vixen", "Comet", "Cupid", "Donner", "Blitzen"} {
		m := r.npc(n)
		members = append(members, tourMember{rel: [3]float32{m.SpawnX - veh.SpawnX, m.SpawnY - veh.SpawnY, m.SpawnZ - veh.SpawnZ},
			home: [4]float32{m.SpawnX, m.SpawnY, m.SpawnZ, m.SpawnFacing}})
		names = append(names, n)
	}
	type at struct{ x, y, z float32 }
	last := make([]at, len(members))
	least, leastAt := float32(1e9), ""
	for k := 0; float64(k)*0.1 <= d.total; k++ {
		tt := float64(k) * 0.1
		// off the sand: past the first stretch and before the last
		aloft := d.sAt(tt) > float64(d.knotS[2]) && d.sAt(tt) < float64(d.knotS[len(d.knotS)-3])
		for i, m := range members {
			x, y, z, _ := d.memberAt(tt, m)
			if aloft {
				for _, s := range g.Surfaces(d.World, x, z, y-2, y+4) {
					t.Errorf("at %.1f s %s at (%.1f, %.1f, %.1f) meets a surface at %.1f", tt, names[i], x, y, z, s.Y)
				}
				var top float32 = -1e9
				for _, s := range g.Surfaces(d.World, x, z, -500, y) {
					top = max(top, s.Y)
				}
				if c := y - top; c < least {
					least, leastAt = c, names[i]
				}
			}
			if k > 0 {
				p := last[i]
				if c := g.Crosses(d.World, p.x, p.y+1, p.z, x, y+1, z); c != ground.CrossNone {
					t.Errorf("at %.1f s %s goes through %s from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f)", tt, names[i], c, p.x, p.y, p.z, x, y, z)
				}
			}
			last[i] = at{x, y, z}
		}
		for slot := range d.Deck {
			x, y, z, _ := d.riderAt(tt, veh.SpawnFacing, d.Deck[slot])
			if aloft {
				for _, s := range g.Surfaces(d.World, x, z, y-2, y+4) {
					t.Errorf("at %.1f s deck place %d at (%.1f, %.1f, %.1f) meets a surface at %.1f", tt, slot, x, y, z, s.Y)
				}
			}
		}
	}
	t.Logf("the flight: %.0f s; its closest pass over anything, %.1f (%s)", d.total, least, leastAt)
	for i, p := range d.Route {
		if p.View == nil {
			continue
		}
		v := *p.View
		f, ok := g.Support(d.World, v[0], v[1], v[2], 0.5, 2)
		if !ok || math.Abs(float64(f-v[1])) > 0.3 {
			t.Errorf("point %d's view (%.1f, %.1f, %.1f) is not on a floor (%.2f %v)", i, v[0], v[1], v[2], f, ok)
		}
	}
}

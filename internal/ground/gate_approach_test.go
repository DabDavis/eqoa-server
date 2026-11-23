package ground

import (
	"math"
	"os"
	"testing"
)

// Walk a player's approach in short segments across both faces of the gate collision.
func TestThanksgivingWestGateWalk(t *testing.T) {
	dir := os.Getenv("EQOA_EVENT_GROUND")
	if dir == "" {
		t.Skip("requires the disc collision bake")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	walk := func(x1, z1, x2, z2 float32) bool {
		y := float32(54.1)
		x, z := x1, z1
		steps := int(math.Ceil(math.Hypot(float64(x2-x1), float64(z2-z1)) / .75))
		for i := 1; i <= steps; i++ {
			t := float32(i) / float32(steps)
			nx, nz := x1+(x2-x1)*t, z1+(z2-z1)*t
			ny, ok := g.Floor(0, nx, y+1, nz)
			if !ok || math.Abs(float64(ny-y)) > 1.3 || g.Wall(0, x, y+1, z, nx, ny+1, nz) || g.Wall(0, nx, ny+1, nz, x, y+1, z) {
				return false
			}
			x, y, z = nx, ny, nz
		}
		return true
	}
	if !walk(25258, 15760, 25201, 15760) {
		t.Fatal("blocked approach inside the west gate")
	}
	if !walk(25201, 15760, 25163, 15759) {
		t.Fatal("blocked approach outside the gate to the King")
	}
	if walk(25258, 15790, 25201, 15790) {
		t.Fatal("gate wall control unexpectedly walkable")
	}
}

package entity

import (
	"testing"
	"time"
)

// basementWallGround is a basement floor at 0 with solid ground east of x 101: there the only floor is a hill's surface
// 76 up, none near the basement's height.
type basementWallGround struct{}

func (basementWallGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	if x > 101 {
		return 76, y >= 74
	}
	return 0, y >= -2
}

// A creature against a wall summons its pet where a floor near its height is, never inside the wall over the hill's
// surface (Heirloom petition 3450: High Priest Vorin's Servant, set 1.5 into the basement wall, rose onto the hill
// at its first chase step). The control: with the floor all round, the pet stands at the usual place, beside it.
func TestACreaturePetStandsOnTheOwnersFloorByAWall(t *testing.T) {
	for _, c := range []struct {
		name   string
		facing float32
		walled bool
	}{
		{"its left in the wall", 3 * 3.14159265 / 2, true},
		{"the control: no wall", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, owner, _, _ := creaturePetsWorld(t)
			w.mu.Lock()
			owner.Y, owner.SpawnY, owner.Facing = 0, 0, c.facing
			w.mu.Unlock()
			if c.walled {
				w.SetGround(basementWallGround{})
			} else {
				w.SetGround(flatGround{})
			}
			w.sweepCreaturePets(time.Now())
			pets := w.creaturePetsOf(owner)
			if len(pets) != 1 {
				t.Fatalf("pets: %d", len(pets))
			}
			p := pets[0]
			if c.walled && (p.X > 101 || p.SpawnX > 101) {
				t.Errorf("the pet stands at x %.2f (spawn %.2f), inside the wall past 101", p.X, p.SpawnX)
			}
			if p.Y != 0 {
				t.Errorf("the pet stands at height %.2f, want the basement's 0", p.Y)
			}
			if d := distXZ(p.X, p.Z, owner.X, owner.Z); d < 0.5 || d > creaturePetBeside+0.01 {
				t.Errorf("the pet stands %.2f off its owner, want beside it", d)
			}
		})
	}
}

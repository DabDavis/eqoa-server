package entity

import "testing"

// pierEnv is the sea north of z 1000 (in front of a dockmaster at (1000, 50,
// 1000) facing north, h 0: +z), at any height; land elsewhere.
type pierEnv struct{}

func (pierEnv) Flags(world uint8, x, y, z float32) (uint32, bool) {
	if z > 1001 {
		return envWater, true
	}
	return 0, true
}

// Petition 1349: a trip arriving at a dockmaster who faces the sea lands
// behind him on the pier, facing him, not in the water in front. The
// controls: facing the land the trip lands in front as ever, and a world
// with no ground answers nothing (the old landing stands).
func TestATripLandsOnThePierNotInTheSea(t *testing.T) {
	w := NewWorld()
	w.ground = flatGround{y: 50}
	w.env = pierEnv{}

	lx, ly, lz, face, ok := w.ArrivalLanding(0, 1000, 50, 1000, 0, 3)
	if !ok || ly != 50 || lz > 1000 || absf(lx-1000) > 0.01 || absf(lz-997) > 0.01 {
		t.Fatalf("facing the sea: landed (%.2f, %.2f, %.2f) ok %v, want behind him at (1000, 50, 997)", lx, ly, lz, ok)
	}
	if absf(face-0) > 0.01 && absf(face-2*3.14159265) > 0.01 {
		t.Errorf("behind him the traveller faces him (north, 0): faces %.3f", face)
	}

	// facing the land (south, pi): in front, as ever
	lx, _, lz, _, ok = w.ArrivalLanding(0, 1000, 50, 1000, 3.14159265, 3)
	if !ok || absf(lz-997) > 0.01 || absf(lx-1000) > 0.01 {
		t.Errorf("facing the land: landed (%.2f, %.2f) ok %v, want in front at (1000, 997)", lx, lz, ok)
	}

	// a floor too far below (the sea bed under a pier end) is no landing
	w.ground = flatGround{y: 40}
	if _, _, _, _, ok := w.ArrivalLanding(0, 1000, 50, 1000, 0, 3); ok {
		t.Errorf("with every floor 10 below the dockmaster, a landing was found")
	}

	// no ground: nothing (the trip lands in front as it always did)
	if _, _, _, _, ok := NewWorld().ArrivalLanding(0, 1000, 50, 1000, 0, 3); ok {
		t.Errorf("a world with no ground found a landing")
	}
}

// pierGround is the sea bed at 22 under the water everywhere, and a pier's
// deck at 54 placed on it from the shore up to z 1001 (x 995..1005): what
// the terrain alone (Floor) does not know.
type pierGround struct{}

func (pierGround) Floor(world uint8, x, y, z float32) (float32, bool) { return 22, true }

func (pierGround) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	if x >= 995 && x <= 1005 && z <= 1001 && 54 >= y-down && 54 <= y+up {
		return 54, true
	}
	if 22 >= y-down && 22 <= y+up {
		return 22, true
	}
	return 0, false
}

// seaEnv is water up to 52 everywhere.
type seaEnv struct{}

func (seaEnv) Flags(world uint8, x, y, z float32) (uint32, bool) {
	if y < 52 {
		return envWater, true
	}
	return 0, true
}

// Petition 1770 (Hoofbite): "when taking dockmaster from Highbourne to
// Freeport, you load into the water and not onto the dock". Freeport's Endol
// stands on his pier's end over the sea bed (the terrain 32 under him), the
// pier placed on the terrain: the trip lands behind him on its deck, not in
// front of him in the harbour. The control: a ground of the terrain alone
// finds no landing (the old fall back, in front).
func TestATripLandsOnAPlacedPier(t *testing.T) {
	w := NewWorld()
	w.ground = pierGround{}
	w.env = seaEnv{}
	lx, ly, lz, _, ok := w.ArrivalLanding(0, 1000, 54, 1000, 0, 3)
	if !ok || ly != 54 || absf(lx-1000) > 0.01 || absf(lz-997) > 0.01 {
		t.Fatalf("landed (%.2f, %.2f, %.2f) ok %v, want on the deck behind him at (1000, 54, 997)", lx, ly, lz, ok)
	}
	w.ground = flatGround{y: 22}
	if _, _, _, _, ok := w.ArrivalLanding(0, 1000, 54, 1000, 0, 3); ok {
		t.Error("the terrain alone (the sea bed) gave a landing")
	}
}

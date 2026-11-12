package entity

import "math"

// ArrivalLanding is where a traveller lands by the NPC its trip arrives at
// (handlers/waysender.go: a boat's dockmaster, a waysender, a coachman), the
// NPC standing at (x, y, z) facing h: reach in front of it, where the trips
// have always landed, when that is dry ground near the NPC's own height; else
// behind it, else to its left, else to its right. Each faces the NPC. The
// dockmasters stand at the ends of their piers facing the sea, so in front of
// Freeport's Dockmaster Endol was the harbour (petition 1349, Changeface:
// "Highbourne dockmaster drops players into water in Freeport"). A floor is
// the terrain's or anything's placed on it (a pier's deck).
//
// ly is the floor there. ok is false where the world has no ground (a test
// world, a server with no ground files: the trip lands in front as it always
// did) or none of the four spots is dry ground.
func (w *World) ArrivalLanding(world uint8, x, y, z, h, reach float32) (lx, ly, lz, face float32, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.ground == nil {
		return 0, 0, 0, 0, false
	}
	for _, turn := range [...]float64{0, math.Pi, math.Pi / 2, -math.Pi / 2} {
		a := float64(h) + turn
		px, pz := x+reach*float32(math.Sin(a)), z+reach*float32(math.Cos(a))
		fy, found := w.ground.Floor(world, px, y+arrivalReach, pz)
		// a pier is placed on the terrain, not of it (petition 1770,
		// Hoofbite: "when taking dockmaster from Highbourne to Freeport, you
		// load into the water and not onto the dock"): Freeport's Endol and
		// Bogman Village's Geben stand on their piers' ends over the sea
		// bed, so the terrain alone found no dry spot by them, and the trip
		// landed in front of them, in the harbour
		if s, ok := w.ground.(supportGround); ok {
			if d, ok := s.Support(world, px, y, pz, arrivalReach, arrivalReach); ok && (!found || d > fy) {
				fy, found = d, true
			}
		}
		if !found || fy < y-arrivalReach || w.waterAtLocked(world, px, fy+bodyAbove, pz) {
			continue
		}
		return px, fy, pz, float32(math.Mod(a+math.Pi, 2*math.Pi)), true
	}
	return 0, 0, 0, 0, false
}

// arrivalReach is how far above or below the NPC's own height a landing's
// floor may be: a step up onto the pier, not the sea bed nor a roof.
const arrivalReach float32 = 3

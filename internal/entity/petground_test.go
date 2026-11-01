package entity

import (
	"math"
	"testing"
	"time"
)

// hillGround is the pet rig's ground as a hill: its top at 76.5 to x 8634,
// falling 10 over 20 units to 66.5 from x 8654 on; with cave, a cave's
// floor at 60 under its slope. Floor as internal/ground's: the highest layer
// at x no more than 2 above the height asked, else that height and false.
type hillGround struct{ cave bool }

func hillTop(x float32) float32 {
	switch {
	case x <= 8634:
		return 76.5
	case x >= 8654:
		return 66.5
	}
	return 76.5 - (x-8634)/2
}

func (g hillGround) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	layers := []float32{hillTop(x)}
	if g.cave && x > 8636 && x < 8652 {
		layers = append(layers, 60)
	}
	best, found := float32(0), false
	for _, h := range layers {
		if h <= y+2 && (!found || h > best) {
			best, found = h, true
		}
	}
	if !found {
		return y, false
	}
	return best, true
}

// A pet sent from a hill at a creature below runs down the hill on it (the
// user, 2026-10-31: "when i was sending jedds pet skeleton to attack mobs,
// it would glitch under the hill i was on then run at the mob"): every step
// stands on the hill's top. The control: a creature in a cave under the
// hill's slope is still followed down to the cave's floor, as before (its
// height looked for first).
func TestAPetRunsDownTheHillItIsSentFrom(t *testing.T) {
	run := func(t *testing.T, g hillGround, crabX, crabY float32) (worst float32, at float32, pet *NPC) {
		r := newPetRig(t)
		r.w.SetGround(g)
		r.p.X, r.p.Y, r.p.Z = 8628, 76.5, 5421.5
		r.enemy.X, r.enemy.Y, r.enemy.Z = crabX, crabY, 5421.5
		pet, _ = r.summon(t, fireElemental(), 0)
		r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(10*time.Second))
		for ms := 10_100; ms <= 16_000; ms += 100 {
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
			if g.cave && pet.X > 8636 && pet.X < 8652 {
				continue // over the cave: its floor or the hill's are both ground
			}
			if d := hillTop(pet.X) - pet.Y; d > worst {
				worst, at = d, pet.X
			}
		}
		return worst, at, pet
	}
	if worst, at, pet := run(t, hillGround{}, 8670, 66.5); worst > 0.01 || pet.X < 8655 {
		t.Errorf("down the hill: %.2f under its top at x %.1f (the pet ended at x %.1f y %.1f)", worst, at, pet.X, pet.Y)
	}
	_, _, pet := run(t, hillGround{cave: true}, 8646, 60)
	if pet.Y != 60 {
		t.Errorf("the control: the pet at a creature in the cave stands at %.1f, want the cave's floor, 60", pet.Y)
	}
}

// bridgeGround is a river crossed by a bridge: the banks at 60, the river's
// bed at 50 under the bridge, whose deck at 60 spans x 8630 to 8690 between
// them. The deck is placed on the ground (a walls file's), so Floor (the
// terrain's alone) never sees it and Support (every surface) does, as
// internal/ground's.
type bridgeGround struct{}

func onDeck(x float32) bool { return x >= 8630 && x <= 8690 }

func terrainAt(x float32) float32 {
	if onDeck(x) {
		return 50
	}
	return 60
}

func (bridgeGround) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	if h := terrainAt(x); h <= y+2 {
		return h, true
	}
	return y, false
}

func (bridgeGround) Support(_ uint8, x, y, _, up, down float32) (float32, bool) {
	best, found := float32(0), false
	for _, h := range []float32{terrainAt(x), 60} {
		if h == 60 && !onDeck(x) && terrainAt(x) != 60 {
			continue
		}
		if h >= y-down && h <= y+up && (!found || h > best) {
			best, found = h, true
		}
	}
	return best, found
}

// A pet follows its owner over a bridge on the bridge (the user, 2026-11-01:
// "Jedds pet skeleton collision made it stand under the bridge"), summoned
// on the bank or on the bridge itself; the control: its owner walking the
// river's bed under the bridge, the pet walks the bed too.
func TestAPetFollowsItsOwnerOverABridge(t *testing.T) {
	walk := func(t *testing.T, fromX, y float32) (pet *NPC, worst float32) {
		r := newPetRig(t)
		r.w.SetGround(bridgeGround{})
		r.p.X, r.p.Y, r.p.Z = fromX, y, 5421.5
		pet, _ = r.summon(t, fireElemental(), 0)
		for ms := 100; ms <= 12_000; ms += 100 {
			r.p.X = min(fromX+float32(ms)/1000*8.8, 8688) // the owner runs along the deck (or the bed)
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
			if onDeck(pet.X) && pet.Loco != LocoStand {
				if d := float32(math.Abs(float64(pet.Y - y))); d > worst {
					worst = d
				}
			}
		}
		return pet, worst
	}
	for _, c := range []struct {
		name string
		x, y float32
	}{
		{"summoned on the bank", 8610, 60},
		{"summoned on the bridge", 8640, 60},
		{"the control: the owner on the bed under it", 8634, 50},
	} {
		pet, worst := walk(t, c.x, c.y)
		if worst > 0.01 || pet.X < 8660 || pet.Y != c.y {
			t.Errorf("%s: the pet %.1f off its owner's floor %.0f at worst (it ended at x %.1f y %.1f)", c.name, worst, c.y, pet.X, pet.Y)
		}
	}
}

// riverWater is bridgeGround's river: deep water under the bridge to 57,
// three under its deck.
type riverWater struct{}

func (riverWater) Flags(_ uint8, x, y, _ float32) (uint32, bool) {
	if onDeck(x) && y < 57 {
		return envWater, true
	}
	return 0, true
}

// A pet that follows its owner over a bridge across deep water walks the
// deck, out of the water and with the walk's animations (the user,
// 2026-11-01: "the hulking bones pet is on the bridge ,but swimming in mid
// air. seems to trigger at where the bridge meets the land"): its step was
// grounded on the river's bed first, made a swimmer there, and kept the
// swim's animation once set on the deck. The control: its owner swimming
// the river under the bridge, the pet swims after it.
func TestAPetOnABridgeOverWaterWalks(t *testing.T) {
	walk := func(t *testing.T, y float32) (swam, walked int, pet *NPC) {
		r := newPetRig(t)
		r.w.SetGround(bridgeGround{})
		r.w.SetEnv(riverWater{})
		r.p.X, r.p.Y, r.p.Z = 8610, 60, 5421.5
		pet, _ = r.summon(t, fireElemental(), 0)
		for ms := 100; ms <= 12_000; ms += 100 {
			r.p.X = min(8610+float32(ms)/1000*8.8, 8688)
			if onDeck(r.p.X) {
				r.p.Y = y
			}
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
			if !onDeck(pet.X) || pet.X < 8634 {
				continue
			}
			switch {
			case pet.Loco == LocoSwim || pet.Loco == LocoTread || pet.swimming:
				swam++
				if y == 60 {
					t.Fatalf("on the deck at x %.1f y %.2f: animation %d, swimming %v; want the walk's, out of the water", pet.X, pet.Y, pet.Loco, pet.swimming)
				}
			default:
				walked++
			}
		}
		return swam, walked, pet
	}
	if swam, walked, pet := walk(t, 60); swam != 0 || walked == 0 || pet.Y != 60 {
		t.Errorf("over the deck: %d steps swimming, %d walking, ended at y %.2f", swam, walked, pet.Y)
	}
	if swam, walked, pet := walk(t, 57-swimDepth); swam == 0 || walked != 0 || math.Abs(float64(pet.Y-(57-swimDepth))) > 0.06 || pet.Loco != LocoTread {
		t.Errorf("the control, its owner swimming under the bridge: %d steps swimming, %d walking, ended at y %.2f animation %d", swam, walked, pet.Y, pet.Loco)
	}
}

// archGround is bridgeGround's river under an arched bridge: its deck rises
// from the banks' 60 to 63 ten units in, so its crest at a bridge's end
// stands over the land beside it (Neriak's by half a unit and more).
type archGround struct{}

func archDeck(x float32) float32 { return 60 + 0.3*min(x-8630, 8690-x, 10) }

func (archGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	return bridgeGround{}.Floor(world, x, y, z)
}

func (archGround) Support(_ uint8, x, y, _, up, down float32) (float32, bool) {
	best, found := float32(0), false
	hs := []float32{terrainAt(x)}
	if onDeck(x) {
		hs = append(hs, archDeck(x))
	}
	for _, h := range hs {
		if h >= y-down && h <= y+up && (!found || h > best) {
			best, found = h, true
		}
	}
	return best, found
}

func (archGround) PlacedSupport(_ uint8, x, y, _, up, down float32) (float32, bool) {
	if h := archDeck(x); onDeck(x) && h >= y-down && h <= y+up {
		return h, true
	}
	return 0, false
}

// A pet keeps to an arched bridge's deck over deep water while its owner
// steps off it onto lower land at either end (the same report): looked for
// from its owner's feet alone, the deck's crest a few units behind was out
// of reach, and the pet fell to the river's bed under it and swam. The
// control: the owner on a ground that cannot tell the deck from the terrain
// (no PlacedSupport) leaves the pet as before, in the river, swimming.
func TestAPetKeepsToAnArchedDeck(t *testing.T) {
	cross := func(t *testing.T, g Ground, from, to float32) (off int, worst float32) {
		r := newPetRig(t)
		r.w.SetGround(g)
		r.w.SetEnv(riverWater{})
		r.p.X, r.p.Y, r.p.Z = from, 60, 5421.5
		pet, _ := r.summon(t, fireElemental(), 0)
		dir := float32(1)
		if to < from {
			dir = -1
		}
		for ms := 100; ms <= 14_000; ms += 100 {
			x := from + dir*float32(ms)/1000*8.8
			if dir*(x-to) > 0 {
				x = to
			}
			r.p.X, r.p.Y = x, 60
			if onDeck(x) {
				r.p.Y = archDeck(x)
			}
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
			if !onDeck(pet.X) {
				continue
			}
			if d := float32(math.Abs(float64(pet.Y - archDeck(pet.X)))); d > 0.01 || pet.swimming || pet.Loco == LocoSwim || pet.Loco == LocoTread {
				off++
				worst = max(worst, d)
			}
		}
		return off, worst
	}
	for _, c := range []struct {
		name     string
		from, to float32
	}{{"west to east", 8610, 8700}, {"east to west", 8710, 8620}} {
		if off, worst := cross(t, archGround{}, c.from, c.to); off != 0 {
			t.Errorf("%s: %d steps off the deck or swimming (%.1f under it at worst)", c.name, off, worst)
		}
	}
	if off, _ := cross(t, archSupportOnly{}, 8610, 8700); off == 0 {
		t.Errorf("the control: a ground that cannot tell the deck kept the pet on it")
	}
}

// archSupportOnly is archGround without PlacedSupport.
type archSupportOnly struct{}

func (archSupportOnly) Floor(world uint8, x, y, z float32) (float32, bool) {
	return archGround{}.Floor(world, x, y, z)
}

func (archSupportOnly) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	return archGround{}.Support(world, x, y, z, up, down)
}

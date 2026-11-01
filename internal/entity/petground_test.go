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

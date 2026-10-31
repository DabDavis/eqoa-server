package entity

import (
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

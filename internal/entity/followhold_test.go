package entity

import (
	"math"
	"testing"
	"time"
)

// A follower standing holds its place while its owner is near it
// (followslot.go's followHolds): the owner, 2026-11-17, "drizzt cant pet his
// own cat guenhwyvar if he gets close it movese".

// walkTo moves r's player at the run (0.88 a 100 ms tick) toward (x, z) until
// stop off it, ticking every follower's system, and returns the time reached.
func walkTo(r *petRig, at time.Duration, x, z, stop float32) time.Duration {
	for i := 0; i < 400; i++ {
		d := distXZ(r.p.X, r.p.Z, x, z)
		if d <= stop+0.001 {
			break
		}
		step := min(float32(0.88), d-stop)
		dx, dz := (x-r.p.X)/d, (z-r.p.Z)/d
		at += 100 * time.Millisecond
		r.w.UpdatePosition(r.p.ServerID, r.p.X+dx*step, r.p.Y, r.p.Z+dz*step, headingTo(dx, dz))
		followTick(r.w, r.t0, at)
	}
	return at
}

// stand ticks every follower's system for d with its owner standing.
func stand(r *petRig, at, d time.Duration) time.Duration {
	for end := at + d; at < end; {
		at += 100 * time.Millisecond
		followTick(r.w, r.t0, at)
	}
	return at
}

// The owner walks up to his pet, and to his familiar, from the far side of
// it, and stands a unit off it: neither stirs; before, each walked off to keep
// its slot at its owner's right as its owner came on (and within followCrowd
// went on to its slot however still he stood).
func TestOwnerWalksUpToHisFollower(t *testing.T) {
	for _, which := range []string{"pet", "familiar"} {
		t.Run(which, func(t *testing.T) {
			r, fs := spreadRig(t)
			n := fs[0]
			if which == "familiar" {
				n = fs[2]
			}
			at := stand(r, 14*time.Second, time.Second)
			// first away (the long way round) so he comes to it from afar
			at = walkTo(r, at, r.p.X-(n.X-r.p.X)*3, r.p.Z-(n.Z-r.p.Z)*3, 0)
			at = stand(r, at, 4*time.Second)
			if n.moving() {
				t.Fatalf("%s still moves after its owner stopped", n.Name)
			}
			x, z := n.X, n.Z
			at = walkTo(r, at, x, z, 1)
			at = stand(r, at, 3*time.Second)
			if n.X != x || n.Z != z {
				t.Errorf("%s went from (%.2f, %.2f) to (%.2f, %.2f) as its owner walked up to it, %.2f off it now",
					n.Name, x, z, n.X, n.Z, distXZ(n.X, n.Z, r.p.X, r.p.Z))
			}
			if d := distXZ(n.X, n.Z, r.p.X, r.p.Z); d > 1.01 {
				t.Errorf("its owner stands %.2f off %s, not beside it", d, n.Name)
			}
			// round it, a unit off, and it stays
			for _, a := range []float32{1.5, 3, 4.5} {
				at = walkTo(r, at, x+float32(math.Sin(float64(a))), z+float32(math.Cos(float64(a))), 0)
			}
			at = stand(r, at, time.Second)
			if n.X != x || n.Z != z {
				t.Errorf("%s moved as its owner went round it", n.Name)
			}
			// away past the follow distance: it follows, and stands by him
			// once he stops
			hx, hz := r.p.X+(r.p.X-x)*20, r.p.Z+(r.p.Z-z)*20
			moved := false
			for i := 0; i < 400 && distXZ(r.p.X, r.p.Z, hx, hz) > 0.01; i++ {
				at = walkTo(r, at, hx, hz, max(distXZ(r.p.X, r.p.Z, hx, hz)-0.88, 0))
				d := distXZ(x, z, r.p.X, r.p.Z)
				if n.X != x || n.Z != z {
					moved = true
					if d < followSlotRadius(n, slotPet)+followSlack-0.9 {
						t.Errorf("%s left its place with its owner %.2f off it", n.Name, d)
					}
				}
			}
			if !moved {
				t.Fatalf("%s did not follow its owner away", n.Name)
			}
			stand(r, at, 5*time.Second)
			if d := distXZ(n.X, n.Z, r.p.X, r.p.Z); d < 2.5 || d > 6 {
				t.Errorf("%s stands %.2f off its owner after following him", n.Name, d)
			}
			if n.moving() {
				t.Errorf("%s still moves 5 s after its owner stopped", n.Name)
			}
		})
	}
}

// A pet's stances and commands work as before with its owner beside it: told
// to attack it goes (held is no leash), passive it comes back to its owner and
// stands; and he walks up to it again.
func TestHeldPetTakesCommands(t *testing.T) {
	r, fs := spreadRig(t)
	pet := fs[0]
	at := stand(r, 14*time.Second, time.Second)
	at = walkTo(r, at, pet.X, pet.Z, 1)
	at = stand(r, at, time.Second)
	x, z := pet.X, pet.Z
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(at))
	ps := r.w.pets[r.p.ServerID]
	if ps == nil || ps.target != r.enemy.ServerID {
		t.Fatal("the attack command was not taken")
	}
	at = stand(r, at, 3*time.Second)
	if distXZ(pet.X, pet.Z, x, z) < 1 || distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z) > petReach+1 {
		t.Fatalf("told to attack, the pet went from (%.1f, %.1f) to (%.1f, %.1f), %.1f off its target",
			x, z, pet.X, pet.Z, distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z))
	}
	r.w.PetCommand(r.sess, byte(petPassive), 0, r.at(at))
	at = stand(r, at, 6*time.Second)
	if ps.target != 0 {
		t.Fatal("passive, it still fights")
	}
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d > followSlotRadius(pet, slotPet)+followSlack || pet.moving() {
		t.Fatalf("passive, the pet stands %.2f off its owner (moving %v)", d, pet.moving())
	}
	x, z = pet.X, pet.Z
	at = walkTo(r, at, x, z, 1)
	stand(r, at, 2*time.Second)
	if pet.X != x || pet.Z != z {
		t.Errorf("passive, the pet moved as its owner walked up to it")
	}
}

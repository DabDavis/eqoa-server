package entity

import (
	"testing"
	"time"
)

// A pet stuck on its way to its owner is put beside it (followstuck.go;
// petition 2592: a merc's charmed centipede "is getting stuck on rock" on a
// bridge in Lavastorm).

// holdTicks ticks the rig's pets from a to b every 100 ms, putting n back
// where it stood after each (a rock it cannot get round), and reports the
// time it was first seen moved off that place by the server's own put (more
// than 2 from where it was held), 0 for never.
func holdTicks(r *petRig, n *NPC, a, b time.Duration) time.Duration {
	x, z := n.X, n.Z
	for at := a; at <= b; at += 100 * time.Millisecond {
		r.tick(at)
		if distXZ(n.X, n.Z, x, z) > 2 {
			return at
		}
		n.X, n.Z = x, z
		n.stopMoving()
	}
	return 0
}

// A summoned pet that cannot move while its owner stands 30 off is put
// beside its owner once followStuckWindow has gone by, not before.
func TestAStuckPetIsPutBesideItsOwner(t *testing.T) {
	for _, charmed := range []bool{false, true} {
		name := "summoned"
		if charmed {
			name = "charmed"
		}
		t.Run(name, func(t *testing.T) { stuckPetCase(t, charmed) })
	}
}

func stuckPetCase(t *testing.T, charmed bool) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	if charmed {
		pet.Charm = &charmHold{world: pet.World} // as a charm's: not carried by the lost rule, it walks below petLost
	}
	r.tick(9500 * time.Millisecond)
	r.w.UpdatePosition(r.p.ServerID, r.p.X+30, r.p.Y, r.p.Z, r.p.Facing)
	start := 9600 * time.Millisecond
	at := holdTicks(r, pet, start, start+10*time.Second)
	if at == 0 {
		t.Fatal("the stuck pet was never put beside its owner")
	}
	if at-start < followStuckWindow {
		t.Errorf("put beside its owner %v after it stuck, before the window of %v", at-start, followStuckWindow)
	}
	if at-start > followStuckWindow+time.Second {
		t.Errorf("put beside its owner only %v after it stuck", at-start)
	}
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d > followRadius+0.01 || pet.World != r.p.World {
		t.Errorf("the pet ends %.2f off its owner, world %d", d, pet.World)
	}
}

// A pet following an owner who runs on and on (the gap open the whole way
// while it catches up) is never put anywhere: it walks every step.
func TestAPetMakingProgressIsLeftToWalk(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.tick(9500 * time.Millisecond)
	px, pz := pet.X, pet.Z
	jumped := 0
	x0 := r.p.X
	// at the owner's run, 0.88 a 100 ms tick, 80 east, then back west
	at := 9600 * time.Millisecond
	for i := 0; i < 200; i++ {
		step := float32(0.88)
		if i >= 100 {
			step = -0.88
		}
		r.w.UpdatePosition(r.p.ServerID, r.p.X+step, r.p.Y, r.p.Z, r.p.Facing)
		r.tick(at)
		at += 100 * time.Millisecond
		if distXZ(pet.X, pet.Z, px, pz) > 2 {
			jumped++
		}
		px, pz = pet.X, pet.Z
	}
	if jumped != 0 {
		t.Errorf("the pet was put somewhere %d times while it followed", jumped)
	}
	if r.p.X-x0 > 1 || distXZ(pet.X, pet.Z, r.p.X, r.p.Z) > 20 {
		t.Errorf("the owner ends %.1f from his start, the pet %.1f off him", r.p.X-x0, distXZ(pet.X, pet.Z, r.p.X, r.p.Z))
	}
}

// An owner faster than his pet (the gap legitimately growing for seconds
// on end) leaves it to run after him: its own advance is progress.
func TestAPetFallingBehindAFastOwnerIsLeftToRun(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.tick(9500 * time.Millisecond)
	px, pz := pet.X, pet.Z
	var most float32
	at := 9600 * time.Millisecond
	for i := 0; i < 80; i++ { // 1.6 a tick, past the pet's catch-up pace
		r.w.UpdatePosition(r.p.ServerID, r.p.X+1.6, r.p.Y, r.p.Z, r.p.Facing)
		r.tick(at)
		at += 100 * time.Millisecond
		if distXZ(pet.X, pet.Z, px, pz) > 2 {
			t.Fatalf("the pet was put somewhere at %v, %.1f off its owner", at, distXZ(pet.X, pet.Z, r.p.X, r.p.Z))
		}
		px, pz = pet.X, pet.Z
		most = max(most, distXZ(pet.X, pet.Z, r.p.X, r.p.Z))
	}
	if most <= followStuckFar+followRadius {
		t.Errorf("the gap grew only to %.1f: the case is not the one meant", most)
	}
}

// A pet fighting stays at its fight, however far from its owner it is held.
func TestAFightingPetIsNotPulledToItsOwner(t *testing.T) {
	r := newPetRig(t)
	r.enemy.HP, r.enemy.MaxHP = 10_000_000, 10_000_000
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.HP, pet.MaxHP = 10_000_000, 10_000_000
	r.tick(9500 * time.Millisecond)
	// the crab 30 off its owner, the pet beside it
	r.w.mu.Lock()
	r.enemy.X = r.p.X + 30
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	r.w.mu.Unlock()
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(9600*time.Millisecond))
	if at := holdTicks(r, pet, 9700*time.Millisecond, 20*time.Second); at != 0 {
		t.Errorf("the fighting pet was put beside its owner at %v", at)
	}
	if r.w.pets[r.p.ServerID].target != r.enemy.ServerID {
		t.Errorf("the pet's target: %d", r.w.pets[r.p.ServerID].target)
	}
}

// A merc's pet stuck far from its merc is put beside its merc (the
// petition's own case); a merc told to stay holds its place, its pet
// beside it, however far its hirer goes within petLost.
func TestAMercsStuckPetAndAMercOnStay(t *testing.T) {
	t.Run("stuck", func(t *testing.T) {
		r := newMercRig(t)
		ms := mercWithPet(t, r, MercAssist)
		pet, m := ms.pet.npc, ms.npc
		// A merc's random idle stroll can close the gap to the held pet,
		// which correctly restarts the stuck timer. Keep its target still.
		if r.w.SetMercStay(r.sess, 0, true) == 0 {
			t.Fatal("no merc told to stay")
		}
		r.w.mu.Lock()
		pet.X, pet.Z = m.X+30, m.Z
		r.w.mu.Unlock()
		x, z := pet.X, pet.Z
		var put time.Duration
		for at := 10100 * time.Millisecond; at <= 20*time.Second; at += 100 * time.Millisecond {
			r.tick(at)
			if distXZ(pet.X, pet.Z, x, z) > 2 {
				put = at - 10100*time.Millisecond
				break
			}
			pet.X, pet.Z = x, z
			pet.stopMoving()
		}
		if put < followStuckWindow || put > followStuckWindow+time.Second {
			t.Fatalf("the merc's stuck pet put beside it after %v (0: never)", put)
		}
		if d := distXZ(pet.X, pet.Z, m.X, m.Z); d > followRadiusOf(pet)+0.01 {
			t.Errorf("the merc's pet ends %.2f off its merc", d)
		}
	})
	t.Run("stay", func(t *testing.T) {
		r := newMercRig(t)
		ms := mercWithPet(t, r, MercAssist)
		pet, m := ms.pet.npc, ms.npc
		if r.w.SetMercStay(r.sess, 0, true) == 0 {
			t.Fatal("no merc told to stay")
		}
		sx, sz := m.X, m.Z
		at := 10100 * time.Millisecond
		for i := 0; i < 50; i++ { // 44 east at the run
			r.w.UpdatePosition(r.p.ServerID, r.p.X+0.88, r.p.Y, r.p.Z, r.p.Facing)
			r.tick(at)
			at += 100 * time.Millisecond
		}
		r.run(at, at+10*time.Second)
		if distXZ(m.X, m.Z, sx, sz) > 1 {
			t.Errorf("the merc on stay moved %.1f off its place", distXZ(m.X, m.Z, sx, sz))
		}
		if d := distXZ(pet.X, pet.Z, m.X, m.Z); d > 8 {
			t.Errorf("the merc's pet is %.1f off its merc on stay (%.1f off the hirer)", d, distXZ(pet.X, pet.Z, r.p.X, r.p.Z))
		}
	})
}

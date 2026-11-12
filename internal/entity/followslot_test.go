package entity

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// Followers keep a slot each round their owner (followslot.go) and a merc
// takes its ease round a standing owner (mercidle.go): the owner, 2026-11-12,
// "the mercenary and healbot are following jib perfectly stacked on top of
// each other, lets have them spread out. also let merc after idle for 5 secs
// walk around the player ever so often".

// followTick runs every system a follower lives in at d.
func followTick(w *World, t0 time.Time, d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TickCount++
	now := t0.Add(d)
	w.aiSystem(now)
	w.petSystem(now)
	w.mercSystem(now)
	w.familiarSystem(now)
	w.castSystem(now)
	w.effectSystem(now)
}

// spreadRig is a magician with its fire elemental, a hired merc in its group
// and Healbot out.
func spreadRig(t *testing.T) (*petRig, []*NPC) {
	t.Helper()
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	kit := MercKit{Class: 9, Race: 0, Level: r.p.Level, Name: "Ralthas", Base: SimBaseStats(9, 0, r.p.Level), Look: MercLook{Robe: -1}}
	if _, why := r.w.HireMerc(r.sess, kit, MercAssist, r.at(10*time.Second)); why != "" {
		t.Fatalf("the hire was refused: %s", why)
	}
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	d := healbot()
	if !r.w.StartCast(r.sess, FamiliarCast(d), 0xFFFFFFFF, r.at(10*time.Second)) {
		t.Fatal("the familiar's cast did not begin")
	}
	for at := 10 * time.Second; at <= 14*time.Second; at += 100 * time.Millisecond {
		followTick(r.w, r.t0, at)
	}
	ms, fs := r.w.mercs[r.p.ServerID], r.w.familiars[r.p.ServerID]
	if ms == nil || fs == nil || !r.w.mercInOwnGroupLocked(ms) {
		t.Fatalf("merc %v (grouped %v), familiar %v", ms != nil, ms != nil && r.w.mercInOwnGroupLocked(ms), fs != nil)
	}
	return r, []*NPC{pet, ms.npc, fs.npc}
}

// apart fails unless every two followers stand 2 apart and each is 2.5 to 6
// off its owner.
func apart(t *testing.T, when string, p *Player, fs []*NPC) {
	t.Helper()
	for i, a := range fs {
		if d := distXZ(a.X, a.Z, p.X, p.Z); d < 2.5 || d > 6 {
			t.Errorf("%s: %s %.2f off its owner", when, a.Name, d)
		}
		for _, b := range fs[i+1:] {
			if d := distXZ(a.X, a.Z, b.X, b.Z); d < 2 {
				t.Errorf("%s: %s and %s %.2f apart (%.1f,%.1f) (%.1f,%.1f)", when, a.Name, b.Name, d, a.X, a.Z, b.X, b.Z)
			}
		}
	}
}

// A pet, a merc and a familiar following one owner end apart, each in its
// own slot, wherever it runs and stops: before, all three came up the line
// behind it to petFollow off and stood in one place.
func TestFollowersSpreadOut(t *testing.T) {
	r, fs := spreadRig(t)
	apart(t, "called", r.p, fs)
	at := 14 * time.Second
	for _, leg := range []struct{ dx, dz float32 }{{1, 0}, {0, 1}, {-1, 0}, {0.6, -0.8}} {
		h := headingTo(leg.dx, leg.dz)
		for i := 0; i < 40; i++ { // 4 s at the run
			at += 100 * time.Millisecond
			r.w.UpdatePosition(r.p.ServerID, r.p.X+leg.dx*0.88, r.p.Y, r.p.Z+leg.dz*0.88, h)
			followTick(r.w, r.t0, at)
		}
		for i := 0; i < 40; i++ { // stands 4 s (no ease yet)
			at += 100 * time.Millisecond
			followTick(r.w, r.t0, at)
		}
		apart(t, fmt.Sprintf("after the leg (%v, %v)", leg.dx, leg.dz), r.p, fs)
		for _, n := range fs {
			if n.moving() || n.Loco != LocoStand {
				t.Errorf("%s still moves 4 s after its owner stopped", n.Name)
			}
			if d := math.Abs(float64(n.Facing - h)); d > 0.05 && math.Abs(d-2*math.Pi) > 0.05 {
				t.Errorf("%s faces %.2f, its owner's way %.2f", n.Name, n.Facing, h)
			}
		}
	}
	// turning on the spot stirs no one
	before := [3][2]float32{}
	for i, n := range fs {
		before[i] = [2]float32{n.X, n.Z}
	}
	r.w.UpdatePosition(r.p.ServerID, r.p.X, r.p.Y, r.p.Z, r.p.Facing+math.Pi)
	for i := 0; i < 20; i++ {
		at += 100 * time.Millisecond
		followTick(r.w, r.t0, at)
	}
	for i, n := range fs {
		if n.X != before[i][0] || n.Z != before[i][1] {
			t.Errorf("%s moved as its owner turned on the spot", n.Name)
		}
	}
}

// While its owner runs a follower keeps up without stopping: from the run's
// second second to its end each moves every tick.
func TestFollowersKeepUpSmoothly(t *testing.T) {
	r, fs := spreadRig(t)
	at := 14 * time.Second
	stops := map[string]int{}
	for i := 0; i < 80; i++ {
		at += 100 * time.Millisecond
		r.w.UpdatePosition(r.p.ServerID, r.p.X+0.88, r.p.Y, r.p.Z, headingTo(1, 0))
		followTick(r.w, r.t0, at)
		if i >= 20 {
			for _, n := range fs {
				if !n.moving() {
					stops[n.Name]++
				}
				if d := distXZ(n.X, n.Z, r.p.X, r.p.Z); d > 8 {
					t.Errorf("tick %d: %s %.1f behind", i, n.Name, d)
				}
			}
		}
	}
	for name, n := range stops {
		if n > 2 {
			t.Errorf("%s stopped %d times in 6 s of its owner's run", name, n)
		}
	}
}

// The merc at ease: nothing before its owner has stood mercIdleAfter; then a
// stroll at a walk to places mercStrollNear to mercStrollFar off its owner,
// every mercIdleEvery; its owner's move brings it back to its slot. The pet
// and the familiar never stroll.
func TestMercStrollsRoundItsStandingOwner(t *testing.T) {
	r, fs := spreadRig(t)
	pet, merc, fam := fs[0], fs[1], fs[2]
	ms := r.w.mercs[r.p.ServerID]
	r.w.roamRnd = nil
	r.w.roamRand().Seed(7)
	at := 14 * time.Second
	r.w.UpdatePosition(r.p.ServerID, r.p.X+2, r.p.Y, r.p.Z, headingTo(1, 0)) // a step: the stand counts from here
	followTick(r.w, r.t0, at)
	stood := at
	petAt, famAt := [2]float32{}, [2]float32{}
	var firstStroll time.Duration
	places := 0
	lastX, lastZ := merc.X, merc.Z
	for i := 0; i < 900; i++ { // 90 s
		at += 100 * time.Millisecond
		followTick(r.w, r.t0, at)
		if at-stood == 2*time.Second {
			petAt, famAt = [2]float32{pet.X, pet.Z}, [2]float32{fam.X, fam.Z}
		}
		if merc.X != lastX || merc.Z != lastZ {
			if at-stood > 3*time.Second && firstStroll == 0 {
				firstStroll = at - stood
			}
			if sp := distXZ(merc.X, merc.Z, lastX, lastZ) / 0.1; at-stood > 3*time.Second && (sp > NPCWalkSpeed+0.01 || merc.Loco == LocoRun) {
				t.Fatalf("at ease it moved at %.2f a second (loco %d)", sp, merc.Loco)
			}
			lastX, lastZ = merc.X, merc.Z
		}
		if d := distXZ(merc.X, merc.Z, r.p.X, r.p.Z); d > mercStrollFar+0.5 {
			t.Fatalf("it wandered %.1f off its owner", d)
		}
		if !merc.moving() && ms.idle.off {
			if d := distXZ(merc.X, merc.Z, r.p.X, r.p.Z); d < mercStrollNear-followArrive {
				t.Errorf("it stood %.2f off its owner", d)
			}
			for _, o := range []*NPC{pet, fam} {
				if d := distXZ(merc.X, merc.Z, o.X, o.Z); d < 1.5 {
					t.Errorf("it stood %.2f off %s", d, o.Name)
				}
			}
		}
		if ms.idle.off && !merc.moving() && (i%10 == 0) {
			places++
		}
	}
	if firstStroll < mercIdleAfter || firstStroll > mercIdleAfter+mercIdleFirstHi+time.Second {
		t.Errorf("its first stroll %v after its owner stopped, want %v to %v", firstStroll, mercIdleAfter, mercIdleAfter+mercIdleFirstHi)
	}
	if ms.idle.strolls < 3 || ms.idle.strolls > 12 {
		t.Errorf("%d strolls in 90 s", ms.idle.strolls)
	}
	if pet.X != petAt[0] || pet.Z != petAt[1] || fam.X != famAt[0] || fam.Z != famAt[1] {
		t.Error("the pet or the familiar strolled")
	}
	// its owner moves on: it comes back to its slot, and stands there
	for i := 0; i < 20; i++ {
		at += 100 * time.Millisecond
		r.w.UpdatePosition(r.p.ServerID, r.p.X, r.p.Y, r.p.Z+0.88, 0)
		followTick(r.w, r.t0, at)
	}
	for i := 0; i < 30; i++ {
		at += 100 * time.Millisecond
		followTick(r.w, r.t0, at)
	}
	sx, sz, ok := r.w.followSlotLocked(merc, r.p, slotMerc)
	if !ok || distXZ(merc.X, merc.Z, sx, sz) > followSlack || ms.idle.off {
		t.Errorf("after its owner moved it stands %.2f off its slot (off %v)", distXZ(merc.X, merc.Z, sx, sz), ms.idle.off)
	}
	apart(t, "back", r.p, fs)
}

// No ease in a fight: a merc with a target never strolls, nor in the
// mercIdleAfter after it.
func TestMercTakesNoEaseInAFight(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercPassive)
	crab := r.creature("Desert Crab", 28, 2)
	crab.HP, crab.MaxHP = 9000000, 9000000
	ms.ordered = crab.ServerID
	r.run(0, 30*time.Second)
	if ms.idle.strolls != 0 || ms.idle.off {
		t.Errorf("it strolled %d times fighting", ms.idle.strolls)
	}
	r.w.mu.Lock()
	r.w.removeNPCLocked(crab)
	r.w.mu.Unlock()
	ms.ordered, ms.target = 0, 0
	r.run(30100*time.Millisecond, 34*time.Second)
	if ms.idle.strolls != 0 {
		t.Error("it strolled within mercIdleAfter of its fight")
	}
}

// slotWallGround is flat ground with a wall along x = wall, seen through nowhere.
type slotWallGround struct{ wall float32 }

func (g slotWallGround) Floor(_ uint8, _, _, _ float32) (float32, bool) { return 0, true }

func (g slotWallGround) Wall(_ uint8, x0, _, _, x1, _, _ float32) bool {
	return (x0 < g.wall) != (x1 < g.wall)
}

func (g slotWallGround) Sees(_ uint8, x0, _, _, x1, _, _ float32) bool {
	return (x0 < g.wall) == (x1 < g.wall)
}

// A slot behind a wall is not taken: the merc's (behind its owner's left)
// with a wall a unit to its owner's left is found turned round its owner,
// on its side of the wall, and the merc goes there; it never strolls past
// the wall.
func TestFollowSlotRoundAWall(t *testing.T) {
	r := newMercRig(t)
	r.p.Y = 0
	wall := r.p.X - 1
	r.w.SetGround(slotWallGround{wall: wall})
	r.p.Facing = 0 // facing +z: its left is -x
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	n := ms.npc
	n.X = r.p.X + 1
	r.run(0, 2*time.Second)
	r.w.mu.Lock()
	sx, sz, ok := r.w.followSlotLocked(n, r.p, slotMerc)
	r.w.mu.Unlock()
	if !ok || sx < wall {
		t.Fatalf("its slot (%.2f, %.2f) ok %v, past the wall at x %.2f", sx, sz, ok, wall)
	}
	r.run(2100*time.Millisecond, 60*time.Second)
	if n.X < wall {
		t.Errorf("it went through the wall: x %.2f", n.X)
	}
	if ms.idle.strolls == 0 {
		t.Error("no stroll in a minute")
	}
}

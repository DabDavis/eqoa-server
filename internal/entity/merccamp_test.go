package entity

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// moveOwner moves the rig's owner by (dx, dz), the spatial index with it.
func (r *mercRig) moveOwner(dx, dz float32) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ox, oz := r.p.X, r.p.Z
	r.p.X, r.p.Z = ox+dx, oz+dz
	r.w.SpatialIndex.Update(r.p.ServerID, ox, oz, r.p.X, r.p.Z)
}

// placeNPC puts n at (x, z), the spatial index with it.
func (r *mercRig) placeNPC(n *NPC, x, z float32) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ox, oz := n.X, n.Z
	n.X, n.Z = x, z
	r.w.SpatialIndex.Update(n.ServerID, ox, oz, x, z)
}

// The camp holds while its owner is within 125 of it: the merc stays at its
// place with the owner 110 off (the control: without a camp, /merc stay at
// the same place, it is put beside its owner past petLost). Past 125 the camp
// breaks, the owner is told, and the merc runs back (not put beside), taking
// no ease until it is with its owner again.
func TestMercCampHoldsUntilItsOwnerGoes125(t *testing.T) {
	// the control: a merc holding where it stands is put beside its owner 110 off
	c := newMercRig(t)
	cms := c.hire(c.kit(0, 0, 0), MercAssist)
	c.w.SetMercStay(c.sess, 0, true)
	c.moveOwner(110, 0)
	c.run(0, time.Second)
	if d := distXZ(cms.npc.X, cms.npc.Z, c.p.X, c.p.Z); d > 10 {
		t.Fatalf("the control: a staying merc %.1f from its owner 110 off, want put beside", d)
	}

	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	spotX, spotZ := r.p.X, r.p.Z
	if n, camped := r.w.ToggleMercCamp(r.sess, 0); n != 1 || !camped {
		t.Fatalf("the camp: %d, %v", n, camped)
	}
	if ms.camp == nil || ms.camp.X != spotX || ms.camp.Z != spotZ {
		t.Fatalf("the camp is not the owner's spot: %+v", ms.camp)
	}
	r.run(0, 3*time.Second)
	if d := distXZ(ms.npc.X, ms.npc.Z, ms.camp.At[0], ms.camp.At[2]); d > 1 {
		t.Fatalf("the merc %.1f from its camp place", d)
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, spotX, spotZ); d > followRadius+0.5 {
		t.Fatalf("its camp place %.1f from the spot", d)
	}
	r.moveOwner(110, 0)
	r.run(3*time.Second, 8*time.Second)
	if ms.camp == nil || distXZ(ms.npc.X, ms.npc.Z, ms.camp.At[0], ms.camp.At[2]) > 1 {
		t.Fatalf("the owner 110 off: camp %+v, the merc at (%.1f, %.1f)", ms.camp, ms.npc.X, ms.npc.Z)
	}
	r.sent()
	r.moveOwner(20, 0) // 130 off
	r.tick(8*time.Second + 100*time.Millisecond)
	if ms.camp != nil || !ms.campBack {
		t.Fatalf("past 125: camp %+v, back %v", ms.camp, ms.campBack)
	}
	if !r.told(fmt.Sprintf(mercCampLeftLine, ms.npc.Name)) {
		t.Error("the owner was not told the camp broke")
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); d < 100 {
		t.Fatalf("the merc put beside its owner (%.1f), want running back", d)
	}
	atX := ms.npc.X
	r.run(8*time.Second+200*time.Millisecond, 9*time.Second)
	if ms.npc.X == atX {
		t.Fatal("the merc has not moved off its camp")
	}
	r.run(9*time.Second, 40*time.Second)
	if d := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); d > mercStrollFar+2 || ms.campBack {
		t.Fatalf("at 40 s the merc is %.1f from its owner, back %v", d, ms.campBack)
	}
}

// /merc camp is a toggle: a slot's alone, then (any holding one) a break, then
// every merc's; /merc follow ends it running back, /merc stay holding.
func TestMercCampToggles(t *testing.T) {
	r := newMercRig(t)
	one := r.hire(r.kit(0, 0, 0), MercAssist)
	k := r.kit(0, 0, 0)
	k.Name = "Kel"
	if _, why, _ := r.w.HireMercAt(r.sess, 2, k, MercAssist, r.at(0)); why != "" {
		t.Fatal(why)
	}
	two := r.w.mercAtLocked(r.p, 2)
	if two == nil {
		t.Fatal("no second merc")
	}
	if n, camped := r.w.ToggleMercCamp(r.sess, 2); n != 1 || !camped || two.camp == nil || one.camp != nil {
		t.Fatalf("/merc 2 camp: %d %v, camps %v %v", n, camped, one.camp, two.camp)
	}
	if n, camped := r.w.ToggleMercCamp(r.sess, 0); n != 1 || camped || two.camp != nil || !two.campBack {
		t.Fatalf("/merc camp (one camped): %d %v", n, camped)
	}
	if n, camped := r.w.ToggleMercCamp(r.sess, 0); n != 2 || !camped || one.camp == nil || two.camp == nil {
		t.Fatalf("/merc camp again: %d %v", n, camped)
	}
	if one.camp.At == two.camp.At {
		t.Error("both mercs hold one place")
	}
	for _, info := range r.w.MercInfosOf(r.sess) {
		if !info.Camp {
			t.Errorf("merc %d's info says no camp", info.Slot)
		}
	}
	r.w.SetMercStay(r.sess, 1, false)
	if one.camp != nil || !one.campBack {
		t.Errorf("/merc 1 follow: camp %v back %v", one.camp, one.campBack)
	}
	r.w.SetMercStay(r.sess, 2, true)
	if two.camp != nil || two.campBack || !two.stay {
		t.Errorf("/merc 2 stay: camp %v back %v stay %v", two.camp, two.campBack, two.stay)
	}
	if n, _ := r.w.ToggleMercCamp(r.sess, 3); n != 0 {
		t.Errorf("/merc 3 camp took %d", n)
	}
}

// Camped, the merc runs to what its owner attacks and engages, then walks
// back to its camp place.
func TestMercCampFightsWhatItsOwnerAttacks(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	r.w.ToggleMercCamp(r.sess, 0)
	r.run(0, 3*time.Second)
	r.moveOwner(40, 0)
	crab := r.creature("crab", 20, 2)
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.run(3*time.Second, 12*time.Second)
	if ms.target != crab.ServerID || ms.camp == nil {
		t.Fatalf("the merc's target %d (crab %d), camp %v", ms.target, crab.ServerID, ms.camp)
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, crab.X, crab.Z); d > 8 {
		t.Fatalf("the merc %.1f from the crab", d)
	}
	r.w.mu.Lock()
	crab.Dead, crab.HP = true, 0
	r.w.wakeNPCLocked(crab)
	r.w.mu.Unlock()
	r.p.AttackTarget, r.p.AutoAttack = 0, false
	r.run(12*time.Second, 40*time.Second)
	if d := distXZ(ms.npc.X, ms.npc.Z, ms.camp.At[0], ms.camp.At[2]); d > 1 {
		t.Fatalf("after the fight the merc is %.1f from its camp place", d)
	}
}

// With its owner 90 off pulling, a crab falling on the camped merc is fought
// back: it looks round its camp too. The control: a merc holding by /merc
// stay at the same place does not see it (round its owner alone).
func TestMercCampFightsBackWithItsOwnerAway(t *testing.T) {
	for _, camp := range []bool{true, false} {
		r := newMercRig(t)
		ms := r.hire(r.kit(0, 0, 0), MercAssist)
		if camp {
			r.w.ToggleMercCamp(r.sess, 0)
		} else {
			r.w.SetMercStay(r.sess, 0, true)
		}
		r.run(0, 2*time.Second)
		r.moveOwner(90, 0)
		r.run(2*time.Second, 3*time.Second)
		crab := r.creature("crab", 20, 0)
		r.placeNPC(crab, ms.npc.X+1, ms.npc.Z)
		crab.CurrentEnemy = ms.npc.ServerID
		r.w.mu.Lock()
		r.w.wakeNPCLocked(crab)
		r.w.mercChooseTargetLocked(ms)
		r.w.mu.Unlock()
		if got := ms.target == crab.ServerID; got != camp {
			t.Errorf("camped %v: fights the crab %v", camp, got)
		}
	}
}

// A keep restart carries a camp its owner is still at: the same camp, the
// merc holding its place.
func TestMercCampKeepRestart(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(0, 0, 0)
	ms := r.hire(k, MercAssist)
	r.w.ToggleMercCamp(r.sess, 0)
	r.run(0, 3*time.Second)
	r.moveOwner(110, 0)
	camp := *ms.camp
	kp, ok := r.w.KeepPlayer(r.sess, r.at(4*time.Second))
	if !ok || len(kp.Mercs) != 1 || kp.Mercs[0].Camp == nil {
		t.Fatalf("kept: %+v", kp.Mercs)
	}
	b, err := json.Marshal(kp.Mercs[0])
	if err != nil {
		t.Fatal(err)
	}
	var km KeptMerc
	if err := json.Unmarshal(b, &km); err != nil {
		t.Fatal(err)
	}
	r2 := newMercRig(t)
	r2.moveOwner(r.p.X-r2.p.X, r.p.Z-r2.p.Z)
	if !r2.w.RestoreMerc(r2.sess, k, km, r2.at(0)) {
		t.Fatal("the merc did not come back")
	}
	m := r2.w.merc1(r2.p.ServerID)
	if m.camp == nil || *m.camp != camp {
		t.Fatalf("the camp back: %+v, want %+v", m.camp, camp)
	}
	r2.run(0, 5*time.Second)
	if m.camp == nil || distXZ(m.npc.X, m.npc.Z, camp.At[0], camp.At[2]) > 1 {
		t.Fatalf("after the restart: camp %v, the merc %.1f from its place", m.camp, distXZ(m.npc.X, m.npc.Z, camp.At[0], camp.At[2]))
	}
}

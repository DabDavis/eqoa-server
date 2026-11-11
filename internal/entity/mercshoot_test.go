package entity

import (
	"testing"
	"time"
)

// A tank's or a melee fighter's bow (mercshoot.go), held to the owner's ask
// of 2026-11-11: "have a bow also and shoot when not within range so they can
// pull mobs". /merc attack on a creature 40 off: the bow in hand, a shot that
// lands and angers it at the merc, the merc holding while it comes, its melee
// hands back in reach and swinging. The controls: a creature past the bow's
// reach is walked to, not shot; a caster carries no bow and never shoots.

const testBowModel = 0x1234ABCD

func (r *mercRig) bowKit(class int) MercKit {
	k := r.kit(class, 0, 0)
	k.Bow = MercBow{Model: testBowModel, Type: 7, Damage: 400, Range: 55, Name: "Heirloom Longbow"}
	return k
}

func TestMercPullsWithItsBow(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.bowKit(0), MercAssist)
	m := ms.npc
	far := r.creature("Desert Crab", 28, 40)
	far.HP, far.MaxHP = 900000, 900000
	r.run(0, time.Second)
	if ms.bowOut || m.PrimaryWeapon == 0 {
		t.Fatalf("no target: its bow out %v, its main hand %#x", ms.bowOut, m.PrimaryWeapon)
	}
	ms.ordered = far.ServerID // /merc attack
	r.run(1100*time.Millisecond, 1800*time.Millisecond)
	if !ms.bowOut || m.SecondaryWeapon != testBowModel || m.PrimaryWeapon != 0 || m.SecondaryType != 7 {
		t.Fatalf("at 40 off: bow out %v, hands %#x/%#x type %d", ms.bowOut, m.PrimaryWeapon, m.SecondaryWeapon, m.SecondaryType)
	}
	if far.HP >= far.MaxHP {
		t.Fatal("the shot never landed")
	}
	if far.CurrentEnemy != m.ServerID {
		t.Fatalf("the crab fights %d after the shot, not the merc %d", far.CurrentEnemy, m.ServerID)
	}
	if ms.shotReady.IsZero() {
		t.Error("no shot loosed")
	}
	// it comes; the merc holds where it stood
	at := m.X
	r.run(1900*time.Millisecond, 3*time.Second)
	if far.Loco != LocoStand && distXZ(m.X, m.Z, at, m.Z) > 0.5 {
		t.Errorf("the merc moved %.1f while its pull came at it", distXZ(m.X, m.Z, at, m.Z))
	}
	r.run(3100*time.Millisecond, 15*time.Second)
	if d := distXZ(m.X, m.Z, far.X, far.Z); d > mercReach(far)+mercBowTo {
		t.Fatalf("after 15 s the crab is %.1f off", d)
	}
	if ms.bowOut || m.PrimaryWeapon == 0 || m.SecondaryWeapon == testBowModel {
		t.Errorf("in melee reach: bow out %v, hands %#x/%#x", ms.bowOut, m.PrimaryWeapon, m.SecondaryWeapon)
	}
	if m.RoundStart.IsZero() {
		t.Error("in melee reach the merc never swung")
	}
}

// A creature past the bow's reach is closed on, not shot; a caster never
// takes up a bow.
func TestMercBowReachAndCasters(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.bowKit(0), MercPassive)
	far := r.creature("Far Crab", 28, 59) // in /merc attack's leash, past the bow's 55 and its allowance
	r.run(0, time.Second)
	ms.ordered = far.ServerID
	x := ms.npc.X
	r.run(1100*time.Millisecond, 1800*time.Millisecond)
	if far.HP < far.MaxHP || ms.shot != nil {
		t.Fatal("shot past its bow's reach")
	}
	if ms.npc.X == x {
		t.Error("it did not close on a creature past its bow's reach")
	}

	r2 := newMercRig(t)
	k := r2.kit(13, 0, 0)
	k.Bow = MercBow{Model: testBowModel, Type: 7, Damage: 400, Range: 55}
	cs := r2.hire(k, MercPassive)
	crab := r2.creature("Crab", 28, 30)
	r2.run(0, time.Second)
	cs.ordered = crab.ServerID
	r2.run(1100*time.Millisecond, 3*time.Second)
	if cs.bowOut || cs.shot != nil || cs.npc.SecondaryWeapon == testBowModel {
		t.Error("a caster took up a bow")
	}
}

// A tank holding its target taunts no more (the owner, 2026-11-11: "lets make
// melee/tanks use their damage abilities also"): its strike instead; the
// control, a target on its owner is taunted (TestMercTankTauntsWhatItsOwnerFights).
func TestMercTankHoldingItsTargetStrikes(t *testing.T) {
	r := newMercRig(t)
	taunt := bookSpell(2295, "Taunt", 1, 0, 5*time.Second, 19, "Target, Increase Hate by 100 ; Increase 10% based on STR")
	kick := bookSpell(2296, "Kick", 1, 0, 6*time.Second, 10, "Target, Decrease Hitpoints by 50")
	kick.Range, taunt.Range = 4.5, 15
	ms := r.hire(r.kit(0, 0, 0, taunt, kick), MercAssist)
	crab := r.creature("Desert Crab", 28, 2)
	crab.HP, crab.MaxHP = 900000, 900000
	r.run(0, time.Second)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, ms.npc.ServerID, 400)
	r.w.mu.Unlock()
	ms.ordered = crab.ServerID
	r.run(1100*time.Millisecond, 4*time.Second)
	if crab.CurrentEnemy != ms.npc.ServerID {
		t.Fatalf("the crab fights %d, not the tank", crab.CurrentEnemy)
	}
	if _, cast := ms.ready[taunt.SpellID]; cast {
		t.Error("the tank taunted a crab already on it")
	}
	if _, cast := ms.ready[kick.SpellID]; !cast {
		t.Error("the tank never kicked")
	}
}

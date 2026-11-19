package entity

import (
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Petition 2390 (Mightyeagle): a bow's shot spends an arrow. With two arrows
// held, two shots land and two are taken through the hook; the third finds
// none and is not made, nothing taken (CLIENT's HasAmmoFor says so itself).
// A swing that needs no ammunition (a melee weapon, Ammo 0) takes none.
func TestARangedShotSpendsItsAmmunition(t *testing.T) {
	var mu sync.Mutex
	spent := map[int]int{}
	done := make(chan struct{}, 8)
	shoot := func(c *caster, kind, ammo, left int, rounds []time.Duration) {
		c.w.SetRangedPowerPct(0) // the power's own test is rangedpower_test.go
		c.w.OnAmmoSpent(func(_ *session.Session, _ uint32, k int) {
			mu.Lock()
			spent[k]++
			mu.Unlock()
			done <- struct{}{}
		})
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		swing := Swing{Kind: kind, Delay: 3500 * time.Millisecond, Reach: 20, Ammo: ammo, AmmoLeft: left}
		if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
			t.Fatal("no fight")
		}
		c.p.AutoAttack = true
		for _, d := range rounds {
			c.w.mu.Lock()
			c.w.attackSystem(c.at(d * time.Millisecond))
			c.w.mu.Unlock()
		}
	}
	wait := func(n int) {
		for i := 0; i < n; i++ {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("the hook ran %d times, want %d", i, n)
			}
		}
	}

	c := newCaster(t)
	shoot(c, swingRange1, AmmoArrows, 2, []time.Duration{250, 800, 3750, 4300, 7250, 7800})
	wait(2)
	if spent[AmmoArrows] != 2 || c.p.Swing.AmmoLeft != 0 {
		t.Errorf("two arrows: %d spent, %d left on the swing; want 2, 0", spent[AmmoArrows], c.p.Swing.AmmoLeft)
	}
	if c.npc.HP == 100000 {
		t.Error("the shots did not land")
	}
	if c.p.AttackTarget != 0 {
		t.Errorf("out of arrows and still fighting %d", c.p.AttackTarget)
	}

	spent = map[int]int{}
	c = newCaster(t)
	shoot(c, swingRange1, AmmoBolts, 0, []time.Duration{250, 800})
	if c.npc.HP != 100000 || spent[AmmoBolts] != 0 {
		t.Errorf("no bolts: the creature at %d, %d spent; want untouched, 0", c.npc.HP, spent[AmmoBolts])
	}

	c = newCaster(t)
	shoot(c, 5, 0, 0, []time.Duration{250, 800, 3750, 4300}) // a melee swing
	time.Sleep(50 * time.Millisecond)
	if len(spent) != 0 || c.npc.HP == 100000 {
		t.Errorf("melee: %v spent, the creature at %d; want none, hurt", spent, c.npc.HP)
	}
}

// HasAmmoFor's rules: a bow wants kind 1, a crossbow kind 2, nothing else
// any; ammunition is its kind and never worn (a bow carrying 1 is worn).
func TestAmmoKinds(t *testing.T) {
	for at, want := range map[int]int{7: AmmoArrows, 9: AmmoBolts, 10: 0, 1: 0, 0: 0} {
		if got := AmmoKind(at); got != want {
			t.Errorf("AmmoKind(%d) = %d, want %d", at, got, want)
		}
	}
	cases := []struct {
		unk3, slot, kind int
		want             bool
	}{
		{1, -1, AmmoArrows, true},  // arrow
		{2, -1, AmmoBolts, true},   // bolt
		{1, 16, AmmoArrows, false}, // a bow with 1 in the field
		{2, -1, AmmoArrows, false}, // a bolt is no arrow
		{0, -1, 0, false},
	}
	for _, c := range cases {
		if got := IsAmmo(c.unk3, c.slot, c.kind); got != c.want {
			t.Errorf("IsAmmo(%d, %d, %d) = %v, want %v", c.unk3, c.slot, c.kind, got, c.want)
		}
	}
}

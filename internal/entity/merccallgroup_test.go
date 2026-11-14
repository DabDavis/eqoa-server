package entity

import (
	"testing"
	"time"
)

// GM ticket 2025 (merccall.go): an aggressive merc answers its group mates'
// calls as its owner's, and its heals reach them; passive and assist keep the
// owner's alone; a player out of the group, one in a duel, and in a raid
// another group number's (mercscope_test.go's TestMercLeavesTheRaidsFightsAlone)
// are not answered.

// A group mate's auto-attack, and a mate's hit with no swing, call an
// aggressive merc and its pet; a crab that aggroed on the mate with no act of
// hers does not; a player outside the group calls nothing.
func TestAnAggressiveMercAssistsItsGroup(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAggressive)
	_, b := r.player("Bea", 0, 0)
	r.group(b)
	_, c := r.player("Cal", 0, 0) // no group of theirs

	// a crab on Bea, uncalled
	x := bigCrab(r, 6)
	r.w.mu.Lock()
	r.w.addThreatLocked(x, b.ServerID, 10)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Fatalf("a crab aggroed on Bea, uncalled: merc %d, pet %d", m, p)
	}
	// Cal's auto-attack and hit, out of the group
	y := bigCrab(r, -6)
	r.w.mu.Lock()
	c.AttackTarget, c.AutoAttack = y.ServerID, true
	r.w.updateAggro(y, c.ServerID, 10)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Fatalf("Cal (no group mate) called: merc %d, pet %d", m, p)
	}
	// Bea's auto-attack on the crab on her
	r.w.mu.Lock()
	b.AttackTarget, b.AutoAttack = x.ServerID, true
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != x.ServerID || p != x.ServerID {
		t.Fatalf("Bea's auto-attack: merc %d, pet %d, want %d", m, p, x.ServerID)
	}
	// Bea's hit alone (a spell, a shot): her call holds once she stops swinging
	r.w.mu.Lock()
	b.AttackTarget, b.AutoAttack = 0, false
	r.w.killNPC(x)
	r.w.mu.Unlock()
	z := bigCrab(r, 8)
	r.w.mu.Lock()
	r.w.updateAggro(z, b.ServerID, 5)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != z.ServerID || p != z.ServerID {
		t.Fatalf("Bea's hit: merc %d, pet %d, want %d", m, p, z.ServerID)
	}
	// the same merc set to assist leaves Bea's crab alone
	ms.stance = MercAssist
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Errorf("an assist merc took on Bea's crab: merc %d, pet %d", m, p)
	}
	// aggressive again, but Bea in a duel: hers are not answered
	ms.stance = MercAggressive
	r.w.mu.Lock()
	if r.w.duels.of == nil {
		r.w.duels.of = map[EntityID]*Duel{}
	}
	r.w.duels.of[b.ServerID] = &Duel{Phase: duelOn}
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Errorf("Bea in a duel: merc %d, pet %d", m, p)
	}
}

// An aggressive healer merc heals a group mate a crab fights once she has hit
// it (her call); an assist one does not, nor an aggressive one while she
// duels.
func TestAnAggressiveMercHealsItsGroup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stance MercStance
		duel   bool
		heals  bool
	}{
		{"aggressive", MercAggressive, false, true},
		{"assist", MercAssist, false, false},
		{"passive", MercPassive, false, false},
		{"aggressive, she duels", MercAggressive, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newMercRig(t)
			ms := r.hire(r.kit(9, 0, 0, mercHealSpell()), tc.stance)
			_, b := r.player("Bea", 0, 0)
			r.group(b)
			if !groupHas(r.w.groups[r.p.GroupID], ms.npc.ServerID) {
				t.Fatal("the merc is not in its owner's group")
			}
			r.run(0, time.Second)
			crab := bigCrab(r, 3)
			r.w.mu.Lock()
			r.w.addThreatLocked(crab, b.ServerID, 10)
			r.w.updateAggro(crab, b.ServerID, 5) // she hits back
			if tc.duel {
				if r.w.duels.of == nil {
					r.w.duels.of = map[EntityID]*Duel{}
				}
				r.w.duels.of[b.ServerID] = &Duel{Phase: duelOn}
			}
			top := hurtTo(b, 50)
			r.w.mu.Unlock()
			c := r.firstMercCast(time.Second, 4*time.Second)
			got := c != nil && c.target == b.ServerID
			if got != tc.heals {
				t.Fatalf("the merc's cast %+v on Bea: want a heal on her %v", c, tc.heals)
			}
			if !tc.heals {
				return
			}
			r.run(4*time.Second, 8*time.Second)
			if b.HP <= top/2 {
				t.Errorf("the heal did not land on Bea: %d/%d", b.HP, top)
			}
		})
	}
}

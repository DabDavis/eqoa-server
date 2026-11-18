package entity

import (
	"testing"
	"time"
)

// taunthold.go's rules, each beside its control. The figures are Heirloom's Nagafen fight's of
// 2026-11-17 (taunthold.go's doc): a level 60 warrior's Goad every 6 s and his melee at 500 a
// second, a damage dealer's spells at 1,100 a second, a cleric's heals of 1,500 on the tank every
// 3 s; the creature has 40k of the damage dealer's hate on it when the tank taunts.

type holdRig struct {
	*areaRig
	clock       time.Time
	tank        *Player
	dps, healer *Player
	npc         *NPC
}

func newHoldRig(t *testing.T) *holdRig {
	t.Helper()
	r := newAreaRig(t)
	h := &holdRig{areaRig: r, clock: r.t0, tank: r.p}
	old := holdNow
	holdNow = func() time.Time { return h.clock }
	t.Cleanup(func() { holdNow = old })
	noResist(t)
	r.p.Class, r.p.Level, r.p.Str = 0, 60, 300 // a warrior
	h.dps = r.player(1002, "Potsgoboom", 1003, 1003)
	h.healer = r.player(1003, "Tims", 1004, 996)
	h.npc = r.creature(2001, "Lord Nagafen", 1003, 1000)
	h.npc.HP, h.npc.MaxHP, h.npc.Level = 700_000, 700_000, 65
	r.w.mu.Lock()
	r.w.updateAggro(h.npc, h.dps.ServerID, 40_000)
	r.w.updateAggro(h.npc, h.tank.ServerID, 20_000)
	r.w.mu.Unlock()
	return h
}

// tick is one second of the fight: the damage dealer's hate dps, the tank's melee 500, and every
// third second a heal of 1,500 on the tank (its whole figure's hate: healHateLocked).
func (h *holdRig) tick(dps int) {
	h.clock = h.clock.Add(time.Second)
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	h.w.updateAggro(h.npc, h.dps.ServerID, dps)
	h.w.updateAggro(h.npc, h.tank.ServerID, 500)
	if h.clock.Sub(h.t0)/time.Second%3 == 0 {
		h.w.healHateLocked(h.healer, h.tank, 1500, 1500)
	}
}

func (h *holdRig) taunt(t *testing.T) {
	t.Helper()
	h.t0 = h.clock
	h.cast(t, goad(), h.npc.ServerID)
	if h.npc.CurrentEnemy != h.tank.ServerID {
		t.Fatalf("the Goad did not take the creature: it fights %d", h.npc.CurrentEnemy)
	}
}

// A Goad holds the creature on the tank for the 6 s to his next Goad through a damage dealer's burst
// of 2,000 a second (Potsgoboom's spell hits on Nagafen were 1,238 each) and the heals, and Goads every 6 s hold it the whole fight. The control: with no
// taunt, the damage dealer's hate takes it from the tank as soon as it is the highest, as ever.
func TestATauntHoldsTheCreatureToTheNextTaunt(t *testing.T) {
	h := newHoldRig(t)
	if h.npc.CurrentEnemy != h.dps.ServerID {
		t.Fatalf("before the taunt it fights %d, want the damage dealer", h.npc.CurrentEnemy)
	}
	for round := 0; round < 5; round++ {
		h.taunt(t)
		for s := 1; s <= 6; s++ {
			h.tick(2000)
			if h.npc.CurrentEnemy != h.tank.ServerID {
				t.Fatalf("round %d, %d s after the Goad: it fights %d (threats: tank %d, dps %d, healer %d)", round, s,
					h.npc.CurrentEnemy, h.npc.Threats[h.tank.ServerID], h.npc.Threats[h.dps.ServerID], h.npc.Threats[h.healer.ServerID])
			}
		}
		// the next press comes 6 s on: the hold's last instant has passed
		h.clock = h.clock.Add(time.Millisecond)
	}

	// the control: the same fight with no taunt, the damage dealer on top fights it
	c := newHoldRig(t)
	for s := 1; s <= 6; s++ {
		c.tick(2000)
	}
	if c.npc.CurrentEnemy != c.dps.ServerID {
		t.Errorf("no taunt: it fights %d, want the damage dealer, whose hate is the highest", c.npc.CurrentEnemy)
	}
}

// After the hold, the creature stays on its taunter while the others' hate is within 10% of his,
// and goes to one who passes him by more. The control: before this rule the highest took it at
// once (a creature no one taunted: the damage dealer one point over the tank).
func TestAfterTheHoldATaunterKeepsItWithinATenth(t *testing.T) {
	h := newHoldRig(t)
	h.taunt(t)
	h.clock = h.clock.Add(tauntHoldFor)
	tank := h.npc.Threats[h.tank.ServerID]
	h.w.mu.Lock()
	h.w.updateAggro(h.npc, h.dps.ServerID, int(tank*108/100-h.npc.Threats[h.dps.ServerID])) // 108% of the tank's
	h.w.mu.Unlock()
	if h.npc.CurrentEnemy != h.tank.ServerID {
		t.Errorf("the damage dealer at 108%% of the tank's hate took it after the hold")
	}
	h.w.mu.Lock()
	h.w.updateAggro(h.npc, h.dps.ServerID, int(tank*3/100)) // 111%
	h.w.mu.Unlock()
	if h.npc.CurrentEnemy != h.dps.ServerID {
		t.Errorf("the damage dealer at 111%% of the tank's hate: it fights %d", h.npc.CurrentEnemy)
	}

	c := newHoldRig(t) // no taunt: the dps at 40,000 against the tank's 20,000; the tank one over
	c.w.mu.Lock()
	c.w.updateAggro(c.npc, c.tank.ServerID, 20_001)
	c.w.updateAggro(c.npc, c.dps.ServerID, 2)
	c.w.mu.Unlock()
	if c.npc.CurrentEnemy != c.dps.ServerID {
		t.Errorf("no taunt: the highest by one point does not take it (fights %d)", c.npc.CurrentEnemy)
	}
}

// The hold ends with its taunter: dead, the creature turns to the highest at once, his 10% gone too;
// and another's
// taunt takes the hold over.
func TestATauntHoldEndsWithItsTaunterAndAnothersTaunt(t *testing.T) {
	h := newHoldRig(t)
	h.taunt(t)
	h.tank.HP = 0
	h.w.mu.Lock()
	h.w.updateAggro(h.npc, h.dps.ServerID, int(h.npc.Threats[h.tank.ServerID]-h.npc.Threats[h.dps.ServerID])+1) // one over him
	h.w.mu.Unlock()
	if h.npc.CurrentEnemy != h.dps.ServerID {
		t.Errorf("the taunter is dead (in his hold, the damage dealer one over him): it fights %d", h.npc.CurrentEnemy)
	}

	h = newHoldRig(t)
	h.taunt(t)
	h.w.mu.Lock()
	h.w.tauntHateLocked(h.npc, h.healer.ServerID, 1000, 105, 60) // a paladin's taunt a second later
	h.w.mu.Unlock()
	h.tick(1100)
	if h.npc.CurrentEnemy != h.healer.ServerID {
		t.Errorf("a second tank's taunt: it fights %d, want the second tank", h.npc.CurrentEnemy)
	}
}

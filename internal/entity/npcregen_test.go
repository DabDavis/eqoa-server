package entity

import (
	"testing"
	"time"
)

// A creature out of a fight (npcregen.go): nothing for two of the regen's
// ticks, then 15% of its maximum a tick until whole; any threat starts the
// count again.

// regenTick is one of the regen's six-second ticks.
func (f *aggroFix) regenTick() {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.w.TickCount = 60 * (f.w.TickCount/60 + 1)
	f.now = f.now.Add(RegenInterval)
	f.w.npcRegenSystem(f.now)
}

func TestACreatureOutOfAFightHealsFromItsThirdTick(t *testing.T) {
	f := newAggroFix(t, 10)
	n := f.npc(100, "Black Bear", 20, bearBody, 1100, 1000)
	n.HP, n.MaxHP = 100, 1000
	want := []int{100, 100, 250, 400, 550, 700, 850, 1000, 1000}
	for i, w := range want {
		f.regenTick()
		if n.HP != w {
			t.Fatalf("tick %d: HP %d, want %d (all %v)", i+1, n.HP, w, want)
		}
	}
}

// Fought again between ticks, it waits its two ticks anew; in a fight it
// heals nothing.
func TestAFightStartsTheCountAgain(t *testing.T) {
	f := newAggroFix(t, 10)
	n := f.npc(100, "Black Bear", 20, bearBody, 1100, 1000)
	n.HP, n.MaxHP = 100, 1000
	f.regenTick()
	f.regenTick()
	f.hit(n, 0) // a hit (or a spell, a notice) between ticks
	n.Threats, n.CurrentEnemy = nil, 0
	f.regenTick()
	f.regenTick()
	if n.HP != 100 {
		t.Fatalf("healed %d within two ticks of a fight", n.HP-100)
	}
	f.regenTick()
	if n.HP != 250 {
		t.Fatalf("third tick after the fight: HP %d, want 250", n.HP)
	}

	f.hit(n, 1) // fighting now
	for i := 0; i < 4; i++ {
		f.regenTick()
	}
	if n.HP != 250 {
		t.Errorf("healed in a fight: HP %d, want 250", n.HP)
	}
}

// Past its leash it gives up and heals on its way home.
func TestALeashedCreatureHealsOnItsWayHome(t *testing.T) {
	f := newAggroFix(t, 10)
	n := f.npc(100, "Black Bear", 20, bearBody, 1100, 1000)
	n.HP, n.MaxHP = 300, 1000
	f.hit(n, 5)
	f.w.mu.Lock()
	n.chase = chaseState{on: true, fromX: 1100, fromZ: 1000}
	n.X = 1250 // 150 off where the fight began: past its leash
	f.w.giveUpLocked(n)
	f.w.mu.Unlock()
	if n.CurrentEnemy != 0 || !n.chase.returning {
		t.Fatalf("gave up: enemy %d, returning %v", n.CurrentEnemy, n.chase.returning)
	}
	f.regenTick()
	f.regenTick()
	f.regenTick()
	if n.HP != 450 {
		t.Errorf("third tick after its leash: HP %d, want 450", n.HP)
	}
}

// A dummy and a corpse heal nothing here (a pet heals as a player,
// TestAPetHealsAndGainsPowerAsAPlayerDoes); a creature of 1 hit point a tick
// heals 1.
func TestWhatDoesNotHealHere(t *testing.T) {
	f := newAggroFix(t, 10)
	pet := f.npc(100, "Iron Golem", 20, bearBody, 1100, 1000)
	pet.HP, pet.MaxHP, pet.Owner = 10, 1000, f.p.ServerID
	dummy := f.npc(101, "Training Dummy Lv20", 20, bearBody, 1110, 1000)
	dummy.HP, dummy.MaxHP, dummy.Dummy = 10, 1000, TrainingDummy
	corpse := f.npc(102, "Black Bear", 20, bearBody, 1120, 1000)
	corpse.HP, corpse.MaxHP, corpse.Dead = 0, 1000, true
	tiny := f.npc(103, "Rat", 1, bearBody, 1130, 1000)
	tiny.HP, tiny.MaxHP = 1, 5
	for i := 0; i < 3; i++ {
		f.regenTick()
	}
	if pet.HP != 70 || dummy.HP != 10 || corpse.HP != 0 {
		t.Errorf("healed here: pet %d (want 70: 20 a tick), dummy %d, corpse %d", pet.HP, dummy.HP, corpse.HP)
	}
	if tiny.HP != 2 {
		t.Errorf("a 5-HP rat healed to %d at its third tick, want 2 (at least 1)", tiny.HP)
	}
}

// Only the regen's ticks count: the world's other ticks do nothing.
func TestOnlyTheRegensTicksCount(t *testing.T) {
	f := newAggroFix(t, 10)
	n := f.npc(100, "Black Bear", 20, bearBody, 1100, 1000)
	n.HP, n.MaxHP = 100, 1000
	f.w.mu.Lock()
	for tick := uint64(1); tick < 60*3; tick++ {
		if tick%60 == 0 {
			continue
		}
		f.w.TickCount = tick
		f.w.npcRegenSystem(f.now.Add(time.Duration(tick) * 100 * time.Millisecond))
	}
	f.w.mu.Unlock()
	if n.HP != 100 || n.calmTicks != 0 {
		t.Errorf("between the regen's ticks: HP %d, calm %d", n.HP, n.calmTicks)
	}
}

// Under the seed rows a creature heals its level's regen_pct, 5% a tick, from
// its third tick (the user: "Mobs regen slower"); the control is
// TestACreatureOutOfAFightHealsFromItsThirdTick: with no rows, jib's 15%.
func TestACreatureHealsAtItsLevelsRate(t *testing.T) {
	f := newAggroFix(t, 10)
	f.w.balance = SeedBalance()
	n := f.npc(100, "Black Bear", 20, bearBody, 1100, 1000)
	n.HP, n.MaxHP = 100, 1000
	want := []int{100, 100, 150, 200, 250}
	for i, w := range want {
		f.regenTick()
		if n.HP != w {
			t.Fatalf("tick %d: HP %d, want %d (all %v)", i+1, n.HP, w, want)
		}
	}
	// whole from nothing: two calm ticks and twenty of 5%, 2 min 12 s at most
	n.HP = 1
	n.calmTicks, n.healing = 0, false
	ticks := 0
	for n.HP < n.MaxHP && ticks < 100 {
		f.regenTick()
		ticks++
	}
	if ticks != 22 {
		t.Errorf("whole from 1 HP in %d ticks, want 22 (%v)", ticks, time.Duration(ticks)*RegenInterval)
	}
}

// A pet, summoned or charmed, heals and gains power every tick as a player
// does (a 50 of each maximum), in a fight too, from the first tick; a
// creature that is nobody's pet heals nothing in its fight (the control).
func TestAPetHealsAndGainsPowerAsAPlayerDoes(t *testing.T) {
	f := newAggroFix(t, 10)
	f.w.balance = SeedBalance()
	pet := f.npc(100, "Iron Golem", 50, bearBody, 1100, 1000)
	pet.HP, pet.MaxHP, pet.Owner = 10, 1000, f.p.ServerID
	petPowerLocked(pet)
	if pet.MaxPower != 1400 || pet.Power != 1400 {
		t.Fatalf("a level 50 pet's power: %d/%d, want 1400", pet.Power, pet.MaxPower)
	}
	pet.Power = 0
	charmed := f.npc(101, "Black Bear", 20, bearBody, 1110, 1000)
	charmed.HP, charmed.MaxHP, charmed.Owner, charmed.Charm = 500, 2000, f.p.ServerID, &charmHold{}
	foe := f.npc(102, "Black Bear", 20, bearBody, 1120, 1000)
	foe.HP, foe.MaxHP = 300, 1000
	pet.CurrentEnemy, charmed.CurrentEnemy = foe.ServerID, foe.ServerID // fighting
	f.hit(foe, 10)
	f.regenTick()
	if pet.HP != 30 || pet.Power != 28 || charmed.HP != 540 {
		t.Errorf("one tick: pet %d HP %d power, charmed %d HP; want 30, 28, 540", pet.HP, pet.Power, charmed.HP)
	}
	if foe.HP != 300 {
		t.Errorf("a creature in its fight healed: %d", foe.HP)
	}
	f.w.mu.Lock()
	f.w.unCharmLocked(charmed)
	f.w.mu.Unlock()
	if charmed.MaxPower != 0 || charmed.Power != 0 {
		t.Errorf("let go, a creature keeps power %d/%d", charmed.Power, charmed.MaxPower)
	}
}

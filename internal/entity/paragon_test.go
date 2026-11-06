package entity

import (
	"testing"
	"time"
)

// The Hammer of the Paragon's proc (petitions 1047, 1048): 50 hit points and
// 50 power back to its wielder and its group near, never resisted; the
// control, a member too far off, gets nothing.
func TestTheHammerOfTheParagonRestoresItsGroup(t *testing.T) {
	wp, ok := WeaponProcOf(5468, "hammer of the paragon", "A masterwork hammer. It holds a power enchantment.")
	if !ok || wp.Kind != ProcRestore || wp.Damage != 50 || wp.Power != 50 || wp.FX != fxBuff {
		t.Fatalf("the proc: %+v %v", wp, ok)
	}
	k := newKillWorld(t, 60, 60, 60)
	for _, p := range k.players {
		p.HP, p.MaxHP, p.Power, p.MaxPower = 100, 1000, 100, 1000
	}
	me, near, far := k.players[0], k.players[1], k.players[2]
	near.X += 20
	far.X += 200
	k.w.mu.Lock()
	k.w.restoreProcLocked(k.sess[0], me, &wp, 50)
	k.w.mu.Unlock()
	for _, c := range []struct {
		p      *Player
		hp, pw int
	}{{me, 150, 150}, {near, 150, 150}, {far, 100, 100}} {
		if c.p.HP != c.hp || c.p.Power != c.pw {
			t.Errorf("player %d: HP %d power %d, want %d and %d", c.p.ServerID, c.p.HP, c.p.Power, c.hp, c.pw)
		}
	}
}

// A training dummy's hit sets off the player's defensive proc heals (petition
// 1048: Spirit Shield healed nothing against a dummy, whose hits ran the
// shields alone), told before the dummy's own heal.
func TestADummysHitSetsOffSpiritShield(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv50", 50)
	c.p.HP, c.p.MaxHP = 2000, 9000
	shield := &ActiveEffect{SpellID: 701228, Name: "Spirit Shield", Until: c.at(time.Hour)}
	shield.Rules.ProcHeal, shield.Rules.ProcHealDef, shield.Rules.ProcHealChance = 52, true, 100
	c.p.Effects = append(c.p.Effects, shield)
	c.w.mu.Lock()
	c.w.dummyHitLocked(c.sess, c.npc, c.p, c.at(0))
	c.w.mu.Unlock()
	var amounts []int64
	for _, d := range damageIndications(c.sent()) {
		amounts = append(amounts, d[1])
	}
	if len(amounts) < 3 || amounts[0] >= 0 || amounts[1] != 52 || amounts[2] != dummyHeal {
		t.Fatalf("the hit's 0x00DB: %v, want the hit, +52, +%d", amounts, dummyHeal)
	}
}

// A description's proc of piercing damage is resisted by pierce (petition
// 1053: Davoh's Dirk), and Toxxulia's Fang's disease damage over time is
// read as one (1050). The control: a figure with no element is arcane's.
func TestProcTextsOfTheRoguesBlades(t *testing.T) {
	dirk, ok := WeaponProcFromText("A masterwork dirk. Once the property of Davoh. It holds a greater strength enchantment. Procs 100 piercing damage.")
	if !ok || dirk.Kind != ProcHit || dirk.Damage != 100 || dirk.Resist != StatPierce {
		t.Errorf("Davoh's Dirk: %+v %v", dirk, ok)
	}
	fang, ok := WeaponProcFromText("A flawless blade. It comes from the Toxxulia Forest. A health enchantment has been laid upon it. Procs a 100 point Disease DoT.")
	if !ok || fang.Kind != ProcDoT || fang.Damage != 100 || fang.Resist != StatDR {
		t.Errorf("Toxxulia's Fang: %+v %v", fang, ok)
	}
	if wp, ok := WeaponProcOf(1, "a blade", "Procs a 100 point DD Spell."); !ok || wp.Resist != StatAR {
		t.Errorf("the control: %+v %v", wp, ok)
	}
}

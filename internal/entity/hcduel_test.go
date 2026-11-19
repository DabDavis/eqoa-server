package entity

import "testing"

// A Hardcore duelist beaten to DuelLoserHP is healed to HardcoreDuelHP at the
// duel's end, and a snowball takes nothing off a Hardcore character
// (petition 2401); an ordinary loser stays at DuelLoserHP (TestDuelHurt).
func TestHardcoreDuelistHealedAndSnowballHarmless(t *testing.T) {
	was := HardcoreOn()
	SetHardcoreOn(true)
	t.Cleanup(func() { SetHardcoreOn(was) })

	d := newDuelists(t)
	d.b.Hardcore = true
	duel := d.start(t)
	d.w.mu.Lock()
	over := d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 5000, d.at(3e9), numberMelee)
	d.w.mu.Unlock()
	if !over || d.b.HP != HardcoreDuelHP || d.b.IsDead() {
		t.Fatalf("the Hardcore loser: over %v, HP %d, dead %v; want %d", over, d.b.HP, d.b.IsDead(), HardcoreDuelHP)
	}

	// a snowball at the Hardcore Beta: no HP taken; at Alpha: one
	d.w.mu.Lock()
	d.w.snowballHitLocked(d.sa, d.a, d.b, 0)
	hb := d.b.HP
	ha := d.a.HP
	d.w.snowballHitLocked(d.sb, d.b, d.a, 0)
	d.w.mu.Unlock()
	if hb != HardcoreDuelHP {
		t.Errorf("a snowball took Hardcore Beta to %d", hb)
	}
	if d.a.HP != ha-1 {
		t.Errorf("the control: Alpha %d after a snowball, want %d", d.a.HP, ha-1)
	}
}

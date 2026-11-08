package entity

import "testing"

// A GM's #testmode (the GM Jibrish's petition 1272): a creature hitting it to
// death leaves it at 1 HP every time, no death, no debt, the creature still on
// it; the control: with it off the same blows kill.
func TestAGMInTestModeTakesTheHitsAndLives(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP = f.p.MaxHP
	if on, ok := f.w.ToggleGMTestMode(f.p.ServerID); !on || !ok {
		t.Fatalf("test mode on: %v %v", on, ok)
	}
	f.w.mu.Lock()
	for i := 0; i < 20; i++ { // twenty blows, each a death without it
		f.w.npcAttackPlayer(f.killer, f.p.ServerID, f.t0)
	}
	f.w.mu.Unlock()
	if f.p.HP != 1 || f.p.respawnPending() || f.p.Debt != 0 || f.died() != 0 {
		t.Fatalf("in test mode: HP %d, dead %v, debt %d, 0x005E %d", f.p.HP, f.p.respawnPending(), f.p.Debt, f.died())
	}
	if f.killer.CurrentEnemy != f.p.ServerID {
		t.Error("the creature let go of a GM in test mode")
	}
	if on, _ := f.w.ToggleGMTestMode(f.p.ServerID); on {
		t.Fatal("test mode did not toggle off")
	}
	f.kill(t)
	if !f.p.IsDead() || !f.p.respawnPending() {
		t.Fatalf("test mode off: HP %d, dead %v", f.p.HP, f.p.respawnPending())
	}
}

// #tankprofile1-3 set AGI 400 and AC 1000 x the profile whatever is worn or
// on, as defence and the stat channel read them (StatNow); another profile
// replaces it, the same one again (or 0) turns it off and the player's own
// come back.
func TestATankProfileSetsAGIAndAC(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 7, Name: "Gm", Level: 60, Agi: 120, AC: 300}
	p.Mods.AC = 50
	w.Players[p.ServerID] = p
	agi, ac := p.StatNow(StatAgi), p.StatNow(StatAC)
	for _, c := range []struct{ n, want int }{{1, 1}, {2, 2}, {3, 3}, {3, 0}, {1, 1}, {0, 0}} {
		now, ok := w.SetTankProfile(p.ServerID, c.n)
		if !ok || now != c.want {
			t.Fatalf("#tankprofile%d: profile %d, want %d", c.n, now, c.want)
		}
		wantAgi, wantAC := agi, ac
		if now > 0 {
			wantAgi, wantAC = GMTankAgi, GMTankAC*now
		}
		if p.StatNow(StatAgi) != wantAgi || p.StatNow(StatAC) != wantAC || !p.StatsDirty {
			t.Errorf("profile %d: AGI %d AC %d, want %d and %d", now, p.StatNow(StatAgi), p.StatNow(StatAC), wantAgi, wantAC)
		}
	}
}

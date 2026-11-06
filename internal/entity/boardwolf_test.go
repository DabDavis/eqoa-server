package entity

import (
	"testing"
	"time"
)

// The boards' Spirit of the Wolf (petition 1042): an hour of the movement
// buff's run speed, one at a time, a talk again renewing it; ended by the
// player's own hostile act (an attack engaged here) and by nothing friendly.
// The control: with none on, a hostile act ends nothing.
func TestTheBoardsWolfRunsUntilAHostileAct(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	p.Speed = RunSpeed
	base := p.SpeedNow()
	t0 := time.Now()
	if given, renewed := k.w.GiveBoardWolf(k.sess[0], t0); !given || renewed {
		t.Fatalf("given %v renewed %v", given, renewed)
	}
	if got := p.SpeedNow(); got <= base*1.29 || got >= base*1.31 {
		t.Fatalf("run speed %v, want %v x1.3", got, base)
	}
	if given, renewed := k.w.GiveBoardWolf(k.sess[0], t0.Add(30*time.Minute)); !given || !renewed {
		t.Fatalf("again: given %v renewed %v", given, renewed)
	}
	n := 0
	for _, e := range p.Effects {
		if e.SpellID == BoardWolfEffectID {
			n++
			if !e.Until.Equal(t0.Add(30*time.Minute+BoardWolfLength)) || e.Name != BoardWolfName || e.Icon != BoardWolfIcon {
				t.Errorf("renewed: %+v", e)
			}
		}
	}
	if n != 1 || len(p.Buffs) != 1 {
		t.Fatalf("%d on, %d slots", n, len(p.Buffs))
	}
	k.w.mu.Lock()
	k.w.breakBoardWolfLocked(p, "an attack")
	k.w.mu.Unlock()
	if p.hasEffect(BoardWolfEffectID) || len(p.Buffs) != 0 || p.SpeedNow() != base {
		t.Errorf("after a hostile act: on %v, slots %v, speed %v", p.hasEffect(BoardWolfEffectID), p.Buffs, p.SpeedNow())
	}
	k.w.mu.Lock()
	k.w.breakBoardWolfLocked(p, "an attack") // none on: nothing
	k.w.mu.Unlock()
}

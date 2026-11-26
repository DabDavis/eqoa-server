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

// Heirloom petition 3253: the Fang's wolf ended at a swing at the air of a
// distant foe. A press and a swing out of reach end nothing; a swing that
// reaches its target is the hostile act that ends it.
func TestTheBoardsWolfSurvivesASwingOutOfReach(t *testing.T) {
	f := newFight(t)
	f.p.Speed = RunSpeed
	f.npc.X = 40 // far beyond the reach of 4.5
	f.w.mu.Lock()
	f.w.addEffectLocked(f.p, &ActiveEffect{SpellID: BoardWolfEffectID, Name: BoardWolfName, Icon: BoardWolfIcon,
		Caster: f.p.ServerID, Until: f.t0.Add(BoardWolfLength), Speed: true}, f.t0)
	f.w.mu.Unlock()
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0) {
		t.Fatal("not engaged")
	}
	f.p.AutoAttack = true
	for _, at := range []time.Duration{100 * time.Millisecond, 700 * time.Millisecond, 7 * time.Second, 8 * time.Second} {
		f.tick(at)
	}
	if !f.p.hasEffect(BoardWolfEffectID) {
		t.Fatal("a swing at a foe out of reach ended the wolf")
	}
	f.npc.X = 102 // the foe comes into reach: the next round's swing ends it
	for _, at := range []time.Duration{13 * time.Second, 13*time.Second + 700*time.Millisecond, 14 * time.Second} {
		f.tick(at)
	}
	if f.p.hasEffect(BoardWolfEffectID) {
		t.Error("a swing that reached its foe left the wolf on")
	}
}

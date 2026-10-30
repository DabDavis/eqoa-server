package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// rootSpell is the druid's Root (cast 3 s, 36 s), and a damaging root.
func rootSpell() CastSpell {
	return CastSpell{SpellID: 460, DisplayIndex: 5, Level: 4, Scope: 1, Name: "Root", CastTime: 3 * time.Second,
		Recast: 6 * time.Second, PowerCost: 20, Range: 30, Def: ParseSpellText("Target, root, 36sec")}
}

func thornyRoot() CastSpell {
	return CastSpell{SpellID: 700847, DisplayIndex: 6, Level: 7, Scope: 1, Name: "Thorny Weeds", CastTime: 2 * time.Second,
		Recast: 3 * time.Second, PowerCost: 63, Range: 30, Def: ParseSpellText("Target, Decrease Hitpoints by 22, root for 30 seconds")}
}

// rootBalance is a con table of one row: every con the same resist and break.
func rootBalance(resist, brk float64) *Balance {
	return NewBalance(nil, []database.BalanceCon{{Diff: 0, DamagePct: 100, TakenPct: 100, RootResistPct: resist, RootBreakPct: brk}})
}

// rootRig is a druid of 23 and a creature of 28 standing by, the balance b.
func rootRig(t *testing.T, b *Balance) *caster {
	c := newCaster(t)
	c.w.balance = b
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 28, 5000, 5000
	return c
}

// A hit may free a rooted creature: with a break of 100% the first hit
// after the root ends it and its caster is told; the control, a break of
// 0: ten hits later it is still held.
func TestARootBreaksOnAHit(t *testing.T) {
	for _, tc := range []struct {
		brk  float64
		held bool
	}{{100, false}, {0, true}} {
		c := rootRig(t, rootBalance(0, tc.brk))
		if !c.w.StartCast(c.sess, rootSpell(), c.npcO, c.at(0)) {
			t.Fatal("not begun")
		}
		c.tick(3*time.Second + castLand)
		if !c.npc.rooted(c.at(5 * time.Second)) {
			t.Fatalf("break %g: the root did not take", tc.brk)
		}
		c.w.mu.Lock()
		for range 10 {
			c.w.spellHitLocked(c.p, c.sess, c.npc, 10)
		}
		c.w.mu.Unlock()
		if got := c.npc.rooted(c.at(6 * time.Second)); got != tc.held {
			t.Errorf("break %g%%: rooted %v after ten hits, want %v", tc.brk, got, tc.held)
		}
		told := false
		for _, l := range lines(c.sess) {
			told = told || strings.Contains(l, "breaks free of your Root")
		}
		if told == tc.held {
			t.Errorf("break %g%%: told %v", tc.brk, told)
		}
	}
}

// A higher con may resist a root: with a resist of 100% it does not take
// (the caster told, the creature angered), and a damaging root's hit still
// lands; the control, a resist of 0: it takes.
func TestAHigherConResistsARoot(t *testing.T) {
	c := rootRig(t, rootBalance(100, 0))
	c.w.StartCast(c.sess, thornyRoot(), c.npcO, c.at(0))
	c.tick(2*time.Second + castLand)
	if c.npc.rooted(c.at(4 * time.Second)) {
		t.Error("a resist of 100% let the root take")
	}
	if c.npc.HP != 5000-22 {
		t.Errorf("the root's hit: %d HP, want %d", c.npc.HP, 5000-22)
	}
	if red := redLines(c.sess); len(red) == 0 || red[len(red)-1] != rootResisted {
		t.Errorf("told %q", red)
	}
	if c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("a resisted root left the creature calm (enemy %d)", c.npc.CurrentEnemy)
	}

	c = rootRig(t, rootBalance(100, 0))
	c.w.StartCast(c.sess, rootSpell(), c.npcO, c.at(0))
	c.tick(3*time.Second + castLand)
	if len(c.npc.Effects) != 0 || c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("a pure root resisted: effects %+v, enemy %d (want none, the caster)", c.npc.Effects, c.npc.CurrentEnemy)
	}

	c = rootRig(t, rootBalance(0, 0))
	c.w.StartCast(c.sess, rootSpell(), c.npcO, c.at(0))
	c.tick(3*time.Second + castLand)
	if !c.npc.rooted(c.at(5 * time.Second)) {
		t.Error("a resist of 0 did not let the root take")
	}
}

// The seed's chances climb with the con: none resisted and 5% broken at
// even, 65% and 35% five up and past it.
func TestTheSeedsRootChances(t *testing.T) {
	b := SeedBalance()
	for _, tc := range []struct {
		diff        int
		resist, brk float64
	}{{-3, 0, 5}, {0, 0, 5}, {1, 5, 10}, {2, 15, 15}, {3, 35, 25}, {5, 65, 35}, {9, 65, 35}} {
		if r := b.Con(tc.diff); r.RootResistPct != tc.resist || r.RootBreakPct != tc.brk {
			t.Errorf("con %+d: resist %g break %g, want %g %g", tc.diff, r.RootResistPct, r.RootBreakPct, tc.resist, tc.brk)
		}
	}
}

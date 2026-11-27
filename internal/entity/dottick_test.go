package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Heirloom petition 3353 (Arryl, a level 60 enchanter): "Strangling Hold is a
// DoT and many times when I cast it it hits 3 times immediately then ticks a
// DoT." The server's log had two hits in the second of every landing, then one
// every 6 s: the workbook's first tick twice (a HoT's and a PoT's) was given to
// a damage over time too.

// strangling is the record's effects (spell 700734): 24 s, -177 a tick.
func strangling() CastSpell {
	return wbSpell(700734, 5, 1, "Strangling Hold", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,
		"effects":[{"kind":"hp_tick","amount":-177,"resist":"ar"}]}`)
}

// damageFigures counts the 0x00DB damage figures queued for the caster since
// the last call.
func (c *caster) damageFigures() int {
	n := 0
	for _, m := range c.sent() {
		if len(m) >= 2 && binary.LittleEndian.Uint16(m) == uint16(protocol.OpcodeDamageIndication) {
			n++
		}
	}
	return n
}

// dotRig casts the DoT on a creature that will not die of it and returns
// the landing's offset.
func dotRig(t *testing.T) (*caster, CastSpell, time.Duration) {
	t.Helper()
	noResist(t)
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 100000, 100000, 23
	dot := strangling()
	if !c.w.StartCast(c.sess, dot, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	c.sent()
	return c, dot, dot.CastTime + castLand
}

// One hit at the landing, one every 6 s after, four in its 24 s, each the
// same; one figure to the caster per hit.
func TestDoTTicksOnceAtItsLanding(t *testing.T) {
	c, _, land := dotRig(t)
	c.tick(land)
	first := 100000 - c.npc.HP
	if first <= 0 {
		t.Fatalf("no tick at the landing")
	}
	if got := c.damageFigures(); got != 1 {
		t.Errorf("%d damage figures at the landing, want 1", got)
	}
	hits, hp := 1, c.npc.HP
	for d := land + time.Second; d <= land+40*time.Second; d += time.Second {
		c.tick(d)
		if c.npc.HP != hp {
			if took := hp - c.npc.HP; took != first {
				t.Errorf("at %v a tick of %d, the landing's %d", d-land, took, first)
			}
			if (d-land)%effectTick != 0 {
				t.Errorf("a tick at %v after the landing, not on a 6 s step", d-land)
			}
			hits++
			hp = c.npc.HP
		}
	}
	if hits != 4 {
		t.Errorf("%d ticks over its 24 s, want 4 (one at the landing, then every 6 s)", hits)
	}
	if total := 100000 - c.npc.HP; total != 4*first {
		t.Errorf("%d in all, want %d", total, 4*first)
	}
}

// A recast while it ticks replaces it: no tick at the recast's landing, the
// next on the old one's step, never two tickers, and its time renewed.
func TestDoTRecastKeepsItsTick(t *testing.T) {
	c, dot, land := dotRig(t)
	c.tick(land)
	per := 100000 - c.npc.HP
	c.tick(land + 2*time.Second)
	recast := land + 3*time.Second // its next tick due at land+6
	if !c.w.StartCast(c.sess, dot, c.npcO, c.at(recast)) {
		t.Fatal("recast not begun")
	}
	again := recast + dot.CastTime + castLand
	hp := c.npc.HP
	c.tick(again)
	if again < land+effectTick && c.npc.HP != hp {
		t.Errorf("the recast's landing hit for %d with the old tick still to come", hp-c.npc.HP)
	}
	n := 0
	for _, e := range c.npc.Effects {
		if e.SpellID == dot.SpellID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d of it on the creature after the recast, want 1", n)
	}
	var at []time.Duration
	hp = c.npc.HP
	for d := again + 500*time.Millisecond; d <= again+40*time.Second; d += 500 * time.Millisecond {
		c.tick(d)
		if c.npc.HP != hp {
			if took := hp - c.npc.HP; took != per {
				t.Errorf("at %v a hit of %d, want one tick's %d", d-land, took, per)
			}
			at = append(at, d-land)
			hp = c.npc.HP
		}
	}
	// the old step (land+6, +12, ...) within the recast's 24 s
	want := []time.Duration{6 * time.Second, 12 * time.Second, 18 * time.Second, 24 * time.Second}
	if len(at) != len(want) {
		t.Fatalf("ticks after the recast at %v, want %v", at, want)
	}
	for i := range want {
		if at[i] != want[i] {
			t.Errorf("ticks after the recast at %v, want %v", at, want)
			break
		}
	}
}

// The controls: a workbook heal over time and power over time still land
// their first tick twice (the workbook's legend).
func TestHoTAndPoTStillTickTwiceAtTheLanding(t *testing.T) {
	noResist(t)
	hot := wbSpell(921, 5, 1, "Test Renew", `{"v":1,"scope":"target","target_type":"Friendly","duration_s":24,
		"effects":[{"kind":"hp_tick","amount":20}]}`)
	c := newCaster(t)
	c.p.HP = 300
	c.castOn(t, hot, 0)
	if c.p.HP != 340 {
		t.Errorf("a HoT's landing: %d HP, want 340 (two ticks of 20)", c.p.HP)
	}
	pot := wbSpell(923, 5, 1, "Test Clarity", `{"v":1,"scope":"target","target_type":"Friendly","duration_s":24,
		"effects":[{"kind":"power_tick","amount":15}]}`)
	c = newCaster(t)
	c.castOn(t, pot, 0)
	c.p.Power = 100
	c.w.mu.Lock()
	for _, e := range c.p.Effects {
		if e.SpellID == pot.SpellID {
			e.NextTick, e.Rules.Twice = c.at(10*time.Second), true
		}
	}
	c.w.effectSystem(c.at(10 * time.Second))
	c.w.mu.Unlock()
	if c.p.Power != 130 {
		t.Errorf("a PoT's first tick: %d power, want 130 (two of 15)", c.p.Power)
	}
	if sh := strangling(); rulesOf(&sh.Def).Twice {
		t.Error("a damage over time's rules land its first tick twice")
	}
	if r := rulesOf(&pot.Def); !r.Twice {
		t.Error("a power over time's rules lost the workbook's first tick twice")
	}
	if r := rulesOf(&hot.Def); !r.Twice {
		t.Error("a heal over time's rules lost the workbook's first tick twice")
	}
}

// A damage over time on a player (its own spell's price here, as a duel's or a
// creature's goes through the same tick) kept from before the
// fix with Twice set still hits once a tick.
func TestDoTOnAPlayerNeverTicksTwice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 5000, 5000
	c.w.mu.Lock()
	c.w.addEffectLocked(c.p, &ActiveEffect{SpellID: 777, Name: "Old Rot", Caster: c.p.ServerID, Until: c.at(time.Minute),
		NextTick: c.at(0), HPTick: -50, Rules: EffectRules{Twice: true}}, c.at(0))
	c.w.mu.Unlock()
	c.tick(0)
	if c.p.HP != 4950 {
		t.Errorf("a damage tick with Twice set: %d HP, want 4950 (one tick of 50)", c.p.HP)
	}
}

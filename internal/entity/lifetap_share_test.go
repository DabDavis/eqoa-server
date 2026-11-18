package entity

import "testing"

// A drain whose workbook row gives only a share of its take ("give_pct"):
// Vampiric Strike heals its caster half the damage it deals (the owner,
// 2026-11-18, petition 2377); the same drain without the share gives all of
// it (the control), and 100 or more is all of it too.
func TestADrainGivesItsShareOfTheTake(t *testing.T) {
	noResist(t)
	for _, c := range []struct {
		give string
		gain int
	}{{`,"give_pct":50`, 200}, {``, 400}, {`,"give_pct":100`, 400}} {
		strike := wbSpell(9703, 40, 1, "Test Strike", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":40,
			"effects":[{"id":"0957","type":"Drain","kind":"drain","amount":-400`+c.give+`}]}`)
		if len(strike.Def.Unsupported) != 0 {
			t.Fatalf("read with %v left over", strike.Def.Unsupported)
		}
		k := newCaster(t)
		k.p.Level, k.p.HP, k.p.MaxHP = 40, 1000, 3000
		k.npc.HP, k.npc.MaxHP, k.npc.Level = 20000, 20000, 40
		k.landSeen(t, strike, k.npcO)
		if took, gained := 20000-k.npc.HP, k.p.HP-1000; took != 400 || gained != c.gain {
			t.Errorf("give %q: the creature lost %d, the caster gained %d; want 400 and %d", c.give, took, gained, c.gain)
		}
	}
}

func TestDrainShare(t *testing.T) {
	for _, c := range []struct{ pct, n, want int }{{0, 1089, 1089}, {50, 1089, 545}, {50, 1, 1}, {100, 7, 7}, {25, 400, 100}} {
		if got := drainShare(SpellEffect{DrainPct: c.pct}, c.n); got != c.want {
			t.Errorf("drainShare(%d%%, %d) = %d, want %d", c.pct, c.n, got, c.want)
		}
	}
}

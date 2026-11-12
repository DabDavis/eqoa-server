package entity

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Wisp's kit as Zauz designed it (tools/wisp_kit_20261112.py, the owner
// 2026-11-12), its records as the tool writes them, read by the engine as it
// stands: Ponder's PoT, 80% of the caster's level and 3% of its INT; and
// Metaphysical Wound's FR, CR and LR -125 each and a tenth of the caster's
// INT more, on a creature for a minute; and the ranks' effects summed as
// refreshCMLocked sums them: a Wisp holding the whole kit has max HP -875,
// max power +1750 and so on.
func TestWispKitAbilities(t *testing.T) {
	const (
		ponder = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":760173,"level":6,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F079","type":"Statistic Change","kind":"stat","stat":"pot","amount":0,"level_pct":80,"scale":"int","scale_pct":3}]}`
		wound  = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F080","type":"Statistic Change","kind":"stat","stat":"fr","amount":-125,"scale":"int","scale_pct":10},{"id":"F081","type":"Statistic Change","kind":"stat","stat":"cr","amount":-125,"scale":"int","scale_pct":10},{"id":"F082","type":"Statistic Change","kind":"stat","stat":"lr","amount":-125,"scale":"int","scale_pct":10}]}`
	)
	spell := func(id, level, rng, cast, power, scope, recast int, name, js string) CastSpell {
		sp := SpellOfRow(&database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: rng, CastTime: cast, Power: power,
			Scope: scope, Recast: recast, SpellName: name}, id%40, "", js, 0)
		if len(sp.Def.Unsupported) != 0 {
			t.Fatalf("%s: unread parts %v", name, sp.Def.Unsupported)
		}
		return sp
	}

	for _, c := range []struct{ level, int_ int }{{6, 100}, {30, 222}, {45, 350}, {60, 400}} {
		g := newCaster(t)
		g.p.Level, g.p.Int = c.level, c.int_
		g.castOn(t, spell(760173, 6, 16528, 3, 10, 0, 10, "Ponder", ponder), 0)
		if want := c.level*80/100 + c.int_*3/100; g.p.Mods.PoT != want {
			t.Errorf("Ponder at level %d, INT %d: PoT %d; want %d", c.level, c.int_, g.p.Mods.PoT, want)
		}
		if len(g.p.Effects) != 1 || g.p.Effects[0].Until.Sub(g.at(0)) < 30*time.Minute {
			t.Errorf("Ponder: %d effects on the caster; want one for 30 minutes", len(g.p.Effects))
		}
	}
	// a drink beside it: both count (its line is its own)
	g := newCaster(t)
	g.p.Level, g.p.Int = 60, 400
	g.castOn(t, spell(760173, 6, 16528, 3, 10, 0, 10, "Ponder", ponder), 0)
	if err := g.w.Consume(g.sess, Consumable{Drink: true, Name: "gnomish spirits", Amount: 54}, g.at(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if want := 48 + 12 + 54; g.p.Mods.PoT != want {
		t.Errorf("Ponder and gnomish spirits: PoT %d; want %d", g.p.Mods.PoT, want)
	}

	c := newCaster(t)
	c.p.Level, c.p.Int, c.p.MaxPower = 60, 400, 100000
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 50, 5000, 5000
	c.land(t, spell(760174, 60, 16544, 0, 250, 1, 600, "Metaphysical Wound", wound), c.npcO, time.Second)
	want := -125 - 400*10/100
	if m := c.npc.creatureMods(); m.FR != want || m.CR != want || m.LR != want {
		t.Errorf("Metaphysical Wound (INT 400): creature mods %+v; want FR, CR, LR %d", m, want)
	}
	if len(c.npc.Effects) != 1 || c.npc.Effects[0].Until != c.at(time.Second+castLand+time.Minute) {
		t.Errorf("Metaphysical Wound: %d effects on the creature; want one, a minute from its landing", len(c.npc.Effects))
	}
}

// wispRanks are the kit's ranks' effects as the tool writes them.
var wispRanks = []struct {
	id      int
	effects string
}{
	{2141, "PoT:10"},
	{2142, ""},
	{2143, "DEXMax:50,INTMax:50,WISMax:25,Pow:500,PoT:20,FR:20,CR:20,LR:20"},
	{2144, "DEXMax:50,INTMax:50,DEX:25,INT:25,Pow:500,PoT:25,STA:-50,AGI:-50,STAMax:-25,AGIMax:-25,HP:-500,HoT:-25,AC:-250"},
	{2145, ""},
	{2146, "PoT:5"},
	{2147, "HP:-25,Pow:50"},
	{2148, "HP:-50,Pow:100"},
	{2149, "HP:-75,Pow:150"},
	{2150, "HP:-100,Pow:200"},
	{2151, "HP:-125,Pow:250"},
}

func TestWispKitRanksSum(t *testing.T) {
	var rows []database.CMRankRow
	p := &Player{Race: 31, Class: 13, Level: 60, CMs: map[int]int{}}
	for _, r := range wispRanks {
		rows = append(rows, database.CMRankRow{ID: r.id, Category: 1, Classes: 32767, Races: 1 << 31, Effects: r.effects, Enabled: true})
		p.CMs[r.id] = 1
	}
	w := NewWorld()
	w.SetCMCatalog(rows)
	p.refreshCMLocked(w.cms)
	s := p.CMEffects
	got := map[string]int{"HP": s[cmSlotHP], "Pow": s[cmSlotPower], "PoT": s[cmSlotPoT], "HoT": s[cmSlotHoT], "AC": s[cmSlotAC],
		"STA": s[1], "AGI": s[2], "DEX": s[3], "INT": s[5], "STAMax": s[8], "AGIMax": s[9], "DEXMax": s[10], "WISMax": s[11],
		"INTMax": s[12], "FR": s[cmSlotFR], "CR": s[cmSlotCR], "LR": s[cmSlotLR]}
	want := map[string]int{"HP": -875, "Pow": 1750, "PoT": 60, "HoT": -25, "AC": -250, "STA": -50, "AGI": -50, "DEX": 25,
		"INT": 25, "STAMax": -25, "AGIMax": -25, "DEXMax": 100, "WISMax": 25, "INTMax": 100, "FR": 20, "CR": 20, "LR": 20}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("the whole kit's %s: %d; want %d", k, got[k], v)
		}
	}
}

// TestRealWispFullSpec prints a level 60 wizard's maxima as the server's kit
// gives them (SimKitFromDB, the middle tier), a human with no racial or
// Master Class beside a Wisp with Zauz's whole kit, for the owner
// (EQOA_REAL_DB: a copy with tools/wisp_kit_20261112.py applied;
// EQOA_WISP_PROBE=1).
func TestRealWispFullSpec(t *testing.T) {
	if os.Getenv("EQOA_WISP_PROBE") == "" {
		t.Skip("EQOA_WISP_PROBE=1 prints the full spec")
	}
	db := simRealDB(t)
	t.Cleanup(func() { SetRaceDefs(nil) })
	if err := LoadRaceDefs(db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.CMCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ranks := map[int]*CMRank{}
	for _, r := range rows {
		ranks[r.ID] = ParseCMRank(r)
	}
	sum := func(ids ...int) (s [CMSlots]int) {
		for _, id := range ids {
			for i, v := range ranks[id].Effects {
				s[i] += v
			}
		}
		return s
	}
	type spec struct {
		name  string
		class int
		race  int
		cms   []int
		tier  float64
	}
	kit := []int{2141, 2142, 2143, 2144, 2145, 2146, 2147, 2148, 2149, 2150, 2151}
	var specs []spec
	for _, class := range []int{13, 10, 12, 9} {
		for _, tier := range []float64{0.5, 1} {
			specs = append(specs, spec{"human", class, 0, nil, tier}, spec{"wisp, no kit", class, 31, nil, tier},
				spec{"wisp, whole kit", class, 31, kit, tier})
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%-6s %-4s %-16s %6s %6s %5s %5s %5s %5s %5s %5s %5s\n", "class", "tier", "who", "maxHP", "power", "AC", "HoT", "PoT", "STA", "INT", "FR", "LR")
	for _, s := range specs {
		// the kit a human's (a Wisp wears what a human does: its wire race),
		// the base stats its race's
		pl, err := SimKitFromDB(db, s.class, 0, 60, s.tier)
		if err != nil {
			t.Fatal(err)
		}
		pl.Race, pl.Base = s.race, SimBaseStats(s.class, s.race, 60)
		pl.CMs = sum(s.cms...)
		f := newSimFight(pl, SimNPC{Level: 60}, SeedBalance())
		p := f.p
		fmt.Fprintf(&out, "%-6d %-4.1f %-16s %6d %6d %5d %5d %5d %5d %5d %5d %5d\n", s.class, s.tier, s.name, p.MaxHPNow(), p.MaxPowerNow(),
			p.StatNow(StatAC), p.HoTNow(), p.PoTNow(), p.StatNow(StatSta), p.StatNow(StatInt), p.StatNow(StatFR), p.StatNow(StatLR))
	}
	t.Log("\n" + out.String())
}

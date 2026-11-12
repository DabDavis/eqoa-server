package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Mermaid's kit (tools/mermaid_kit_20261112.py, the owner's design
// 2026-11-12), its records as the tool writes them, read by the engine as it
// stands: Hydrate's HoT and PoT, four fifths of the caster's level; Splash,
// the gnome's Cantrip (5 and 300% of the level) with 5% of CHA; Siren's
// Call's 750 power drained and given to the caster's group in range (a
// creature has none to lose; a duel's opponent loses what it had, halved as a
// duel's hits are, and the caster gains it); Quintashi's Kiss's 1000 and 20%
// of CHA, and WIS and INT -25 on its target for a minute.
func TestMermaidKitAbilities(t *testing.T) {
	const (
		hydrate = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":760175,"level":6,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F083","type":"Statistic Change","kind":"stat","stat":"hot","amount":0,"level_pct":80},{"id":"F084","type":"Statistic Change","kind":"stat","stat":"pot","amount":0,"level_pct":80}]}`
		splash  = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":6,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"F085","type":"Dmg/Heal","kind":"hp","amount":-5,"scale":"cha","scale_pct":5,"level_pct":300,"resist":"cr"}]}`
		call    = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"id":"F086","type":"Drain Group","kind":"drain_group","stat":"power","amount":-750}]}`
		kiss    = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"F087","type":"Dmg/Heal","kind":"hp","amount":-1000,"scale":"cha","scale_pct":20,"resist":"cr"},{"id":"F088","type":"Statistic Change","kind":"stat","stat":"wis","amount":-25,"resist":"cr","duration_s":60},{"id":"F089","type":"Statistic Change","kind":"stat","stat":"int","amount":-25,"resist":"cr","duration_s":60}]}`
	)
	old := procIntn
	procIntn = func(int) int { return 99 } // no resist roll takes
	t.Cleanup(func() { procIntn = old })
	spell := func(id, level, rng, cast, power, scope, recast int, name, js string) CastSpell {
		sp := SpellOfRow(&database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: rng, CastTime: cast, Power: power,
			Scope: scope, Recast: recast, SpellName: name}, id%40, "", js, 0)
		if len(sp.Def.Unsupported) != 0 {
			t.Fatalf("%s: unread parts %v", name, sp.Def.Unsupported)
		}
		return sp
	}
	hydrateSp := spell(760175, 6, 16528, 3, 10, 0, 10, "Hydrate", hydrate)
	splashSp := spell(760176, 6, 16880, 0, 30, 1, 10, "Splash", splash)
	callSp := spell(760177, 60, 16880, 0, 500, 1, 120, "Siren's Call", call)
	kissSp := spell(760178, 60, 16880, 0, 250, 1, 60, "Quintashi's Kiss", kiss)
	if callSp.Range != 30 || splashSp.Range != 30 || callSp.Recast != 2*time.Minute || kissSp.Recast != time.Minute {
		t.Errorf("ranges %v, %v, recasts %v, %v; want 30, 30, 2m, 1m", splashSp.Range, callSp.Range, callSp.Recast, kissSp.Recast)
	}

	// Hydrate: 80% of the level, HoT and PoT both, for 30 minutes
	for _, lv := range []int{6, 30, 54, 60} {
		g := newCaster(t)
		g.p.Level = lv
		g.castOn(t, hydrateSp, 0)
		if want := lv * 80 / 100; g.p.Mods.PoT != want || g.p.Mods.HoT != want {
			t.Errorf("Hydrate at level %d: PoT %d, HoT %d; want %d each", lv, g.p.Mods.PoT, g.p.Mods.HoT, want)
		}
		if len(g.p.Effects) != 1 || g.p.Effects[0].Until.Sub(g.at(0)) < 30*time.Minute {
			t.Errorf("Hydrate: %d effects on the caster; want one for 30 minutes", len(g.p.Effects))
		}
	}
	// a drink beside it: both count (its line is its own), and the drink of
	// the caster's level gives more than Hydrate does
	g := newCaster(t)
	g.p.Level = 54
	g.castOn(t, hydrateSp, 0)
	if err := g.w.Consume(g.sess, Consumable{Drink: true, Name: "gnomish spirits", Amount: 54}, g.at(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if want := 43 + 54; g.p.Mods.PoT != want {
		t.Errorf("Hydrate and gnomish spirits: PoT %d; want %d", g.p.Mods.PoT, want)
	}

	// Splash: 5 + 300% of 60 + 5% of CHA 200 = 195
	s := newCaster(t)
	s.p.Level, s.p.Cha, s.p.MaxPower = 60, 200, 100000
	s.npc.Level, s.npc.HP, s.npc.MaxHP = 60, 5000, 5000
	s.land(t, splashSp, s.npcO, time.Second)
	if took := 5000 - s.npc.HP; took != 195 {
		t.Errorf("Splash at 60, CHA 200: took %d; want 195", took)
	}

	// Siren's Call on a creature: the caster and its group within 30 gain
	// 750, one out of range nothing
	c := newCaster(t)
	c.p.Level, c.p.Power, c.p.MaxPower = 60, 2000, 5000
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 60, 5000, 5000
	near := &Player{ServerID: 1002, Name: "Near", HP: 50, MaxHP: 500, Power: 100, MaxPower: 3000, X: 110, Y: 10, Z: 100}
	far := &Player{ServerID: 1003, Name: "Far", HP: 50, MaxHP: 500, Power: 100, MaxPower: 3000, X: 900, Y: 10, Z: 100}
	c.w.Players[near.ServerID], c.w.Players[far.ServerID] = near, far
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, near.ServerID, far.ServerID}}}
	c.p.GroupID, near.GroupID, far.GroupID = 7, 7, 7
	c.landSeen(t, callSp, c.npcO)
	if c.p.Power != 2000-500+750 || near.Power != 850 || far.Power != 100 {
		t.Errorf("Siren's Call: caster %d, near %d, far %d power; want 2250, 850, 100", c.p.Power, near.Power, far.Power)
	}

	// Quintashi's Kiss: 1000 + 20% of CHA 200 = 1040, WIS and INT -25 for a minute
	k := newCaster(t)
	k.p.Level, k.p.Cha, k.p.MaxPower = 60, 200, 100000
	k.npc.Level, k.npc.HP, k.npc.MaxHP = 60, 5000, 5000
	k.land(t, kissSp, k.npcO, time.Second)
	if took := 5000 - k.npc.HP; took != 1040 {
		t.Errorf("Quintashi's Kiss at CHA 200: took %d; want 1040", took)
	}
	if m := k.npc.creatureMods(); m.Wis != -25 || m.Int != -25 || m.CR != 0 {
		t.Errorf("Quintashi's Kiss: creature mods %+v; want WIS and INT -25", m)
	}
	if len(k.npc.Effects) != 1 || k.npc.Effects[0].Until != k.at(time.Second+castLand+time.Minute) {
		t.Errorf("Quintashi's Kiss: %d effects on the creature; want one, a minute from its landing", len(k.npc.Effects))
	}
}

// Siren's Call in a duel: the opponent loses the power it has (the 750
// halved, as a duel's hits are), and the caster gains what was taken.
func TestMermaidSirensCallInADuel(t *testing.T) {
	const call = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"id":"F086","type":"Drain Group","kind":"drain_group","stat":"power","amount":-750}]}`
	sp := SpellOfRow(&database.SpellPatternRecord{SpellID: 760177, AbilityLvl: 60, Range: 16880, Power: 500, Scope: 1,
		Recast: 120, SpellName: "Siren's Call"}, 760177%40, "", call, 0)
	d := newDuelists(t)
	d.start(t)
	d.b.Power, d.b.MaxPower = 1200, 5000 // B casts (500), A has 300
	if !d.w.StartCast(d.sb, sp, d.bSeesA, d.at(2*time.Second)) {
		t.Fatalf("Siren's Call at the opponent was refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(d.at(2*time.Second + sp.CastTime + castLand))
	d.w.mu.Unlock()
	if d.a.Power != 0 || d.b.Power != 1200-500+300 {
		t.Errorf("Siren's Call in a duel: opponent %d, caster %d power; want 0 and 1000", d.a.Power, d.b.Power)
	}
}

// mermaidRanks are the kit's ranks' effects as the tool writes them.
var mermaidRanks = []struct {
	id      int
	effects string
}{
	{2171, "PoT:5,HoT:5"},
	{2172, "PoT:5"},
	{2173, "CR:50,LR:-50"},
	{2174, "WISMax:50,CHAMax:50,STAMax:25,Def Mod:2,CR:50,LR:-50,Pow:500,PoT:10,HoT:10"},
	{2175, ""},
	{2176, ""},
}

// The whole kit summed as refreshCMLocked sums it, offered to a Mermaid of
// any class and to no other race.
func TestMermaidKitRanksSum(t *testing.T) {
	var rows []database.CMRankRow
	p := &Player{Race: 32, Class: 9, Level: 60, CMs: map[int]int{}}
	for _, r := range mermaidRanks {
		cat := 1
		if r.id >= 2174 {
			cat = 4
		}
		rows = append(rows, database.CMRankRow{ID: r.id, Category: cat, Classes: 32767, Races: 1 << 32, Effects: r.effects, Enabled: true})
		p.CMs[r.id] = 1
	}
	w := NewWorld()
	w.SetCMCatalog(rows)
	for _, r := range rows {
		rk := ParseCMRank(r)
		if !rk.OfferedTo(0, 32) || !rk.OfferedTo(14, 32) || rk.OfferedTo(0, 31) || rk.OfferedTo(0, 8) || rk.OfferedTo(0, 0) {
			t.Errorf("rank %d: offered to the Mermaid alone? %v %v %v %v %v", r.ID, rk.OfferedTo(0, 32), rk.OfferedTo(14, 32),
				rk.OfferedTo(0, 31), rk.OfferedTo(0, 8), rk.OfferedTo(0, 0))
		}
	}
	p.refreshCMLocked(w.cms)
	s := p.CMEffects
	got := map[string]int{"Pow": s[cmSlotPower], "PoT": s[cmSlotPoT], "HoT": s[cmSlotHoT], "WISMax": s[11], "CHAMax": s[13],
		"STAMax": s[8], "Def": s[cmSlotDef], "CR": s[cmSlotCR], "LR": s[cmSlotLR]}
	want := map[string]int{"Pow": 500, "PoT": 20, "HoT": 15, "WISMax": 50, "CHAMax": 50, "STAMax": 25, "Def": 2, "CR": 100, "LR": -100}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("the whole kit's %s: %d; want %d", k, got[k], v)
		}
	}
}

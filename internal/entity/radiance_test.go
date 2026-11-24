package entity

import (
	"testing"
	"time"
)

// radianceJSON is Radiance's (900491) workbook row as Heirloom petition 2886 left it: the target's 730 + 25% CHA heal
// over time beside the group's 350 tick and 100 PoT, the healer's own 150 PoT (the forum's Zaoran Plateau chart, f52
// t308: "At 600 cha the target tick is 880 and there is a 350 stacking group tick (target totals 1230), plus adds the
// 100 PoT buff from vibrance to group but healer gets 150").
const radianceJSON = `{"v":1,"scope":"target_aoe","target_type":"Friendly","harmful":false,"duration_s":24,"stack":3,"level":60,"min_level":36,"radius":20,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"ZP Heals Combined","effects":[` +
	`{"id":"0247","type":"Statistic Change","kind":"hp_tick","amount":730,"scale":"cha","scale_pct":25,"scope":"target"},` +
	`{"id":"0248","type":"Statistic Change","kind":"hp_tick","amount":350},` +
	`{"id":"0248","type":"Statistic Change","kind":"stat","stat":"pot","amount":100},` +
	`{"id":"0248","type":"Statistic Change","kind":"stat","stat":"pot","amount":150,"scope":"self","target_type":"Self"}]}`

// Heirloom petition 2886 (Radiance): the target ticks its own heal and the group's together, a group member in the
// area the group's alone (until then the area took the target's part too), all with the 100 PoT, the healer 150.
func TestRadianceTargetAndGroupTicks(t *testing.T) {
	sum := func(p *Player) (hp, pot int) {
		for _, e := range p.Effects {
			hp += e.HPTick
		}
		return hp, p.Mods.PoT // the regen's PoT, as summed (buffline.go)
	}
	r := newAreaRig(t)
	r.p.Class = 9
	tank := r.player(1002, "Tank", 1010, 1000)
	near := r.player(1003, "Near", 1020, 1000)
	tank.MaxHP, near.MaxHP = 5000, 5000
	r.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{r.p.ServerID, tank.ServerID, near.ServerID}}}
	r.p.GroupID, tank.GroupID, near.GroupID = 7, 7, 7
	sp := wbSpell(900491, 60, 1, "Radiance", radianceJSON)
	if a := sp.Def.Area; a == nil || len(a.Effects) != 2 {
		t.Fatalf("the area's effects: %+v", sp.Def.Area)
	}
	r.cast(t, sp, tank.ServerID)
	// the first tick lands twice (the workbook's, workbook.go), then three more
	target := 730 + 25 + 350 // 25% of CHA 100
	if hp, pot := sum(tank); hp != target || pot != 100 || tank.HP != 500+2*target {
		t.Errorf("the target: %d a tick (HP %d), PoT %d; want %d, %d, 100", hp, tank.HP, pot, target, 500+2*target)
	}
	if hp, pot := sum(near); hp != 350 || pot != 100 || near.HP != 500+2*350 {
		t.Errorf("a member in the area: %d a tick (HP %d), PoT %d; want 350, 1200, 100", hp, near.HP, pot)
	}
	for _, q := range []*Player{tank, near} {
		if len(q.Effects) != 1 || q.Effects[0].Ticks != 3 {
			t.Errorf("%s's ticks: %+v", q.Name, q.Effects)
		}
	}
	if _, pot := sum(r.p); pot != 150 {
		t.Errorf("the healer: PoT %d, want 150 (%+v)", pot, r.p.Effects) // its own 150 over the area's 100 (one line: counted once)
	}

	// a healer outside its target's area: its own 150 all the same
	r.p.X = 980
	r.w.SpatialIndex.Insert(r.p.ServerID, r.p.X, r.p.Z)
	r.p.Effects, r.p.Buffs, r.p.Mods = nil, nil, StatMods{}
	r.t0 = r.t0.Add(time.Minute)
	r.cast(t, sp, tank.ServerID)
	if hp, pot := sum(r.p); hp != 0 || pot != 150 {
		t.Errorf("a healer 30 off its target: %d a tick, PoT %d; want 0, 150", hp, pot)
	}
}

package entity

import (
	"testing"
	"time"
)

// Petition 1738, Hoofbite: Rakish Assault's 100 off pierce on a creature
// doubles a rogue's piercing swings, and now its pierce abilities as well:
// Assassinate and the backstabs (resisted by pierce), the Rakish strikes; a
// slashing or crushing art by a slash or blunt debuff; a backstab of no
// resist (Rush, Mug) by the weapon it swings. Agony keeps its own
// debuff_scale (0.45 a point), and a fire nuke none of it.
func TestRakishRaisesAbilityHits(t *testing.T) {
	r, _, _ := charmRig(t)
	noResist(t)
	crab := r.enemy
	crab.HP, crab.MaxHP = 50000000, 50000000
	r.p.Name, r.p.Class, r.p.Level, r.p.Dex, r.p.Str = "Hoofbite", 6, 60, 400, 300
	r.p.PrimaryType = 5 // a dagger
	r.p.Gear.WeaponDamage, r.p.Gear.WeaponCond, r.p.Gear.WeaponSlot, r.p.Gear.WeaponType = 300, 1, 14, 5
	spell := func(id int, name, js string) CastSpell { return wbSpell(id, 60, 1, name, js) }
	assassinate := spell(901315, "Assassinate", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"067B","type":"Backstab","kind":"backstab","amount":-3400,"scale":"dex","scale_pct":200,"resist":"pierce"}]}`)
	backstab := spell(901302, "Murderous Backstab +", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"1HP","sheet_type":"Backstab","effects":[{"id":"0665","type":"Backstab","kind":"backstab","amount":-1086,"scale":"dex","scale_pct":58,"resist":"pierce"}]}`)
	assault := spell(701209, "Rakish Assault", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"1HP","sheet_type":"Dmg/Heal","effects":[{"id":"0669","type":"Dmg/Heal","kind":"hp","amount":-428,"scale":"str","scale_pct":40,"resist":"pierce"}]}`)
	mug := spell(701011, "Mug", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":44,"min_level":1,"req_equipment":"Any","sheet_type":"Backstab","effects":[{"id":"0636","type":"Backstab","kind":"backstab","amount":-138}]}`)
	agony := spell(901288, "Agony", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":51,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"064F","type":"Dmg/Heal","kind":"hp","amount":-1,"scale":"dex","scale_pct":100,"resist":"pr","debuff_scale":{"stat":"pierce","pct":0.45}}]}`)
	slashArt := spell(9401, "Test Slash", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-1000,"resist":"slash"}]}`)
	fire := spell(9402, "Test Fire", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-1000,"resist":"fr"}]}`)
	took := func(s CastSpell) int {
		before := crab.HP
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
		return before - crab.HP
	}
	fixed := func(int) int { return 0 }
	r.p.RoundHits = 1
	type row struct {
		name string
		s    CastSpell
		pct  int // percent more under Rakish
	}
	rows := []row{{"Assassinate", assassinate, 100}, {"Murderous Backstab +", backstab, 100}, {"Rakish Assault's hit", assault, 100},
		{"Mug (a dagger)", mug, 100}, {"Agony", agony, 45}, {"a slashing art", slashArt, 0}, {"a fire nuke", fire, 0}}
	base := map[string]int{}
	for _, c := range rows {
		base[c.name] = took(c.s)
	}
	swing := playerMeleeHit(r.p, crab, fixed)
	crab.Effects = append(crab.Effects, &ActiveEffect{SpellID: 701209, Until: r.at(time.Hour), Mods: StatMods{Pierce: -100}})
	if got := playerMeleeHit(r.p, crab, fixed); got != swing*2 {
		t.Errorf("a dagger swing: %d under Rakish, %d without; want double", got, swing)
	}
	t.Logf("a dagger swing: %d -> %d", swing, playerMeleeHit(r.p, crab, fixed))
	near := func(got, want int) bool { return got >= want-2 && got <= want+2 }
	for _, c := range rows {
		got, want := took(c.s), base[c.name]*(100+c.pct)/100
		t.Logf("%s: %d -> %d", c.name, base[c.name], got)
		if base[c.name] <= 0 || !near(got, want) {
			t.Errorf("%s: %d under Rakish, %d without; want %d", c.name, got, base[c.name], want)
		}
	}
	// a slash debuff raises a slashing art the same; Mug with a sword in hand
	// goes by slash, not by Rakish's pierce
	crab.Effects = append(crab.Effects, &ActiveEffect{SpellID: 2, Until: r.at(time.Hour), Mods: StatMods{Slash: -50}})
	if got, want := took(slashArt), base["a slashing art"]*150/100; !near(got, want) {
		t.Errorf("a slashing art under 50 off slash: %d, want %d", got, want)
	}
	r.p.PrimaryType, r.p.Gear.WeaponType = 1, 1
	if got, want := took(mug), base["Mug (a dagger)"]*150/100; !near(got, want) {
		t.Errorf("Mug with a sword: %d, want %d (the slash debuff's 50%%)", got, want)
	}
}

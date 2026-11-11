package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Jal'Raeth's three abilities (tools/jalraeth_masteries_20261111.py, the
// owner's design 2026-11-11), their spelleffect records as the tool writes
// them, read by the engine as it stands: Path of the Devout's six parts on
// its caster; Touch of the Devoted's -125 STA, AGI and AC each, 30% of the
// caster's WIS more, on a creature; Guided Meditation's PoT and HoT, 5 and a
// fifth of the caster's level more.
func TestJalRaethAbilities(t *testing.T) {
	const (
		path  = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":60,"stack":760170,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F068","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":500},{"id":"F069","type":"Statistic Change","kind":"stat","stat":"ac","amount":250},{"id":"F070","type":"Statistic Change","kind":"off_mod","amount":2},{"id":"F071","type":"Statistic Change","kind":"def_mod","amount":2},{"id":"F072","type":"Statistic Change","kind":"stat","stat":"sta","amount":25},{"id":"F073","type":"Statistic Change","kind":"stat","stat":"wis","amount":25}]}`
		touch = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F074","type":"Statistic Change","kind":"stat","stat":"sta","amount":-125,"scale":"wis","scale_pct":30},{"id":"F075","type":"Statistic Change","kind":"stat","stat":"agi","amount":-125,"scale":"wis","scale_pct":30},{"id":"F076","type":"Statistic Change","kind":"stat","stat":"ac","amount":-125,"scale":"wis","scale_pct":30}]}`
		med   = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":760172,"level":6,"min_level":1,"radius":30,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F077","type":"Statistic Change","kind":"stat","stat":"pot","amount":5,"level_pct":20},{"id":"F078","type":"Statistic Change","kind":"stat","stat":"hot","amount":5,"level_pct":20}]}`
	)
	// as handlers/cast.go makes one, from the spellpattern row the tool writes
	spell := func(id, level, rng, cast, power, scope, recast int, name, js string) CastSpell {
		sp := SpellOfRow(&database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: rng, CastTime: cast, Power: power,
			Scope: scope, Recast: recast, SpellName: name}, id%40, "", js, 0)
		if len(sp.Def.Unsupported) != 0 {
			t.Fatalf("%s: unread parts %v", name, sp.Def.Unsupported)
		}
		return sp
	}

	c := newCaster(t)
	c.p.Level, c.p.Wis, c.p.MaxPower = 60, 200, 100000
	c.castOn(t, spell(760170, 60, 16528, 0, 500, 0, 600, "Path of the Devout", path), 0)
	if m := c.p.Mods; m.MaxHP != 500 || m.AC != 250 || m.OffMod != 2 || m.DefMod != 2 || m.Sta != 25 || m.Wis != 25 {
		t.Errorf("Path of the Devout: mods %+v; want MaxHP 500, AC 250, Off/Def Mod 2, STA 25, WIS 25", m)
	}

	wis := c.p.StatNow(StatWis) // 200 + Path's 25
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 50, 5000, 5000
	c.land(t, spell(760171, 60, 16544, 0, 250, 1, 30, "Touch of the Devoted", touch), c.npcO, time.Second)
	want := -125 - wis*30/100
	if m := c.npc.creatureMods(); m.Sta != want || m.Agi != want || m.AC != want {
		t.Errorf("Touch of the Devoted (WIS %d): creature mods %+v; want STA, AGI, AC %d", wis, m, want)
	}
	if len(c.npc.Effects) != 1 || c.npc.Effects[0].Until != c.at(time.Second+castLand+time.Minute) {
		t.Errorf("Touch of the Devoted: %d effects on the creature; want one, a minute from its landing", len(c.npc.Effects))
	}

	for _, lv := range []int{6, 30, 60} {
		g := newCaster(t)
		g.p.Level = lv
		g.castOn(t, spell(760172, 6, 16880, 2, 30, 2, 10, "Guided Meditation", med), 0)
		if want := 5 + lv*20/100; g.p.Mods.PoT != want || g.p.Mods.HoT != want {
			t.Errorf("Guided Meditation at level %d: PoT %d, HoT %d; want %d each", lv, g.p.Mods.PoT, g.p.Mods.HoT, want)
		}
	}
}

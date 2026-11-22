package entity

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The players' Ability Stacking Guide's eight exceptions (the owner, 2026-11-22: "check all 8 and fix them"), each
// as the guide states it, on the spells' effects as the shards hold them after tools/stacking_t277_20261122.py (the
// stack type the one figure it changes); each control the stack type before, which stacked otherwise.

// t277Stat is a workbook stat buff of stack type stack: stats "ac:650,maxhp:650".
func t277Stat(id, level, scope int, name, jscope string, stack int, stats string) CastSpell {
	var effs []string
	for _, f := range strings.Split(stats, ",") {
		st, amt, _ := strings.Cut(f, ":")
		effs = append(effs, fmt.Sprintf(`{"type":"Statistic Change","kind":"stat","stat":%q,"amount":%s}`, st, amt))
	}
	js := fmt.Sprintf(`{"v":1,"scope":%q,"target_type":"Friendly","harmful":false,"duration_s":1800,"stack":%d,"level":%d,"sheet_type":"Statistic Change","effects":[%s]}`,
		jscope, stack, level, strings.Join(effs, ","))
	return wbSpell(id, level, scope, name, js)
}

// t277Both casts a then b (10 s apart) on a fresh caster and reports the effects on it.
func t277Both(t *testing.T, a, b CastSpell) (*caster, []string) {
	t.Helper()
	c := newCaster(t)
	c.castOn(t, a, 0)
	c.castOn(t, b, 10*time.Second)
	return c, c.effectNames()
}

// 1. Cleric and druid AC/HP lines: a line's single and group forms do not stack (the cleric's all "group", the
// druid's all "target"), and the cleric's stack with the druid's. Control: the stack types before (237 for Courage to
// Order, 700834 for the druid's group forms) let both forms of a line stack.
func TestT277ClericAndDruidLines(t *testing.T) {
	for _, before := range []bool{false, true} {
		single, druidGroup := 700189, 1
		if before {
			single, druidGroup = 237, 700834
		}
		order := t277Stat(700805, 60, 1, "Order", "target", single, "ac:650,maxhp:650")
		groupOrder := t277Stat(701154, 60, 2, "Group Order", "group", 700189, "ac:650,maxhp:650")
		united := t277Stat(701249, 60, 0, "United Faith", "player_aoe", 700189, "ac:650,maxhp:650")
		natureskin := t277Stat(700624, 49, 1, "Natureskin", "target", 1, "ac:625,maxhp:125,maxpower:250")
		groupNature := t277Stat(701027, 49, 2, "Group Natureskin", "group", druidGroup, "ac:625,maxhp:125,maxpower:250")
		vitaes := t277Stat(900447, 60, 0, "Vitae's Blessing", "player_aoe", druidGroup, "ac:750,maxhp:150,maxpower:300")
		for _, pair := range [][2]CastSpell{{order, groupOrder}, {groupOrder, order}, {order, united}, {natureskin, groupNature}, {groupNature, natureskin}, {natureskin, vitaes}} {
			_, got := t277Both(t, pair[0], pair[1])
			if want := 1; before {
				if len(got) != 2 {
					t.Errorf("control (before): %s then %s: %v, want both (the forms stacked)", pair[0].Name, pair[1].Name, got)
				}
			} else if len(got) != want {
				t.Errorf("%s then %s: %v, want one (a line's forms do not stack)", pair[0].Name, pair[1].Name, got)
			}
		}
		if before {
			continue
		}
		// the cleric's and the druid's stack, every form
		c := newCaster(t)
		c.castOn(t, groupOrder, 0)
		c.castOn(t, natureskin, 10*time.Second)
		c.castOn(t, order, 20*time.Second) // takes Group Order off: one line
		c.castOn(t, groupNature, 30*time.Second)
		if got := c.effectNames(); len(got) != 2 || !slices.Contains(got, "Order") || !slices.Contains(got, "Group Natureskin") {
			t.Errorf("cleric beside druid: %v, want Order and Group Natureskin", got)
		}
		if c.p.Mods.AC != 650+625 {
			t.Errorf("cleric beside druid: AC %d, want %d", c.p.Mods.AC, 650+625)
		}
		// the druid's plain Vitae Blessing (a single target's line: its scope's stack type 1) is one line with them
		vitae := buffOf(701252, 60, 1, "Vitae Blessing", "Target, Increase AC by 750, MaxPower by 300, MaxHP by 150, HP by 125, 30min")
		_, got := t277Both(t, vitae, groupNature)
		if len(got) != 1 {
			t.Errorf("Vitae Blessing then Group Natureskin: %v, want one", got)
		}
	}
}

// 2. Alchemist and shaman stat lines: the same rule. Each class's single and group forms of a line are one line; its
// two lines (no stat shared) stack; the alchemist's stack with the shaman's. Control: the stack types before.
func TestT277AlchemistAndShamanLines(t *testing.T) {
	for _, before := range []bool{false, true} {
		thought, intent, finesse, tribal := 700833, 700833, 700149, 700149
		if before {
			thought, intent, finesse, tribal = 700002, 700065, 700062, 1
		}
		prodigious := t277Stat(700730, 55, 1, "Prodigious Thought", "target", thought, "agi:50,wis:50")
		groupProd := t277Stat(701081, 55, 2, "Group Prodigiousness", "group", 700833, "agi:50,wis:50")
		cunning := t277Stat(700773, 60, 1, "Cunning Intent", "target", intent, "dex:55,int:55")
		groupCunning := t277Stat(701153, 60, 2, "Group Cunning", "group", 700833, "dex:55,int:55")
		acumen := t277Stat(700702, 55, 1, "Acumen", "target", finesse, "agi:60,dex:60")
		groupAcumen := t277Stat(700713, 55, 2, "Group Acumen", "group", 700149, "agi:60,dex:60")
		thew := t277Stat(700686, 51, 1, "Tribal Thew", "target", tribal, "str:65,sta:65")
		groupThew := t277Stat(700680, 51, 2, "Group Tribal Thew", "group", 700149, "str:65,sta:65")
		for _, pair := range [][2]CastSpell{{prodigious, groupProd}, {cunning, groupCunning}, {acumen, groupAcumen}, {thew, groupThew}} {
			_, got := t277Both(t, pair[0], pair[1])
			if before && len(got) != 2 {
				t.Errorf("control (before): %s then %s: %v, want both", pair[0].Name, pair[1].Name, got)
			} else if !before && len(got) != 1 {
				t.Errorf("%s then %s: %v, want one (a line's forms do not stack)", pair[0].Name, pair[1].Name, got)
			}
		}
		// lines of no shared stat, and the two classes', stack (before as after)
		for _, pair := range [][2]CastSpell{{prodigious, groupCunning}, {acumen, groupThew}, {prodigious, acumen}, {groupProd, groupAcumen}} {
			if _, got := t277Both(t, pair[0], pair[1]); len(got) != 2 {
				t.Errorf("before %v: %s then %s: %v, want both", before, pair[0].Name, pair[1].Name, got)
			}
		}
	}
	// Tribal Spirit (the shaman's AoE CM, all four stats) takes both its lines' forms off, being all they gave
	spirit := t277Stat(701245, 60, 0, "Tribal Spirit", "player_aoe", 700149, "str:65,sta:65,agi:65,dex:65")
	c := newCaster(t)
	c.castOn(t, t277Stat(700702, 55, 1, "Acumen", "target", 700149, "agi:60,dex:60"), 0)
	c.castOn(t, t277Stat(700686, 51, 1, "Tribal Thew", "target", 700149, "str:65,sta:65"), 10*time.Second)
	c.castOn(t, spirit, 20*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != "Tribal Spirit" {
		t.Errorf("Tribal Spirit over Acumen and Tribal Thew: %v, want Tribal Spirit alone", got)
	}
}

// 3. Enchanter PoT: "they can use that buff in their group ability slot and still use the single target ability
// that was upgraded": Tranquility (single, stack 1) and Empathic Tranquility or Empathy (the Empathy CM's, stack 3)
// both count; the controls: two of one slot do not (Clarity under Tranquility, Empathy and Empathic Tranquility).
func TestT277EnchanterPoTSlots(t *testing.T) {
	tranq := t277Stat(700826, 60, 1, "Tranquility", "target", 1, "pot:23")
	empathic := t277Stat(701134, 60, 2, "Empathic Tranquility", "group", 3, "pot:23")
	empathy := t277Stat(700779, 60, 1, "Empathy", "target", 3, "pot:23")
	clarity := t277Stat(561, 34, 1, "Clarity", "target", 1, "pot:10")
	for _, pair := range [][2]CastSpell{{tranq, empathic}, {empathic, tranq}, {tranq, empathy}} {
		c, got := t277Both(t, pair[0], pair[1])
		if len(got) != 2 || c.p.Mods.PoT != 46 {
			t.Errorf("%s then %s: %v, PoT %d; want both, 46", pair[0].Name, pair[1].Name, got, c.p.Mods.PoT)
		}
	}
	for _, pair := range [][2]CastSpell{{tranq, clarity}, {empathy, empathic}} {
		if _, got := t277Both(t, pair[0], pair[1]); len(got) != 1 {
			t.Errorf("control: %s then %s: %v, want one", pair[0].Name, pair[1].Name, got)
		}
	}
}

// 4. Form of the Master and Wolf's Instinct "stack in the form category": with self, target and group buffs of their
// stats. Form of the Master (the sheet's Alter Appearance, no illusion) has no line; Wolf's Instinct and Hunter's
// Instinct (its no-form upgrade) are their recast group's line, one with each other alone (the control).
func TestT277FormOfTheMasterAndWolfsInstinct(t *testing.T) {
	fotm := wbSpell(701060, 51, 0, "Form of the Master", `{"v":1,"scope":"self","harmful":false,"duration_s":1800,"stack":0,"level":51,"sheet_type":"Alter Appearance","effects":[{"kind":"stat","stat":"str","amount":80},{"kind":"stat","stat":"sta","amount":80},{"kind":"stat","stat":"agi","amount":80},{"kind":"stat","stat":"dex","amount":80},{"kind":"stat","stat":"ac","amount":250},{"kind":"stat","stat":"maxhp","amount":500}]}`)
	hunter := wbSpell(760110, 51, 0, "Hunter's Instinct", `{"v":1,"scope":"self","harmful":false,"duration_s":1800,"stack":9,"level":51,"sheet_type":"Statistic Change","effects":[{"kind":"stat","stat":"str","amount":175},{"kind":"stat","stat":"agi","amount":175},{"kind":"stat","stat":"dex","amount":175},{"kind":"stat","stat":"maxhp","amount":1500}]}`)
	hunter.RecastGroup = "ranger instinct"
	wolf := wbSpell(701068, 51, 0, "Wolf's Instinct", `{"v":1,"scope":"self","harmful":false,"duration_s":1800,"stack":2,"level":51,"sheet_type":"Alter Appearance","effects":[{"kind":"illusion"},{"kind":"stat","stat":"str","amount":175}]}`)
	wolf.RecastGroup = "ranger instinct"
	self := t277Stat(9301, 40, 0, "Test Self STR", "self", 2, "str:50,agi:50")
	target := t277Stat(9302, 40, 1, "Test Target STR", "target", 1, "str:50,agi:50")
	group := t277Stat(9303, 40, 2, "Test Group STR", "group", 3, "str:50,agi:50")
	for _, form := range []CastSpell{fotm, hunter} {
		c := newCaster(t)
		c.castOn(t, self, 0)
		c.castOn(t, target, 10*time.Second)
		c.castOn(t, group, 20*time.Second)
		c.castOn(t, form, 30*time.Second)
		if got := c.effectNames(); len(got) != 4 {
			t.Errorf("%s beside self, target and group STR buffs: %v, want all four", form.Name, got)
		}
	}
	if BuffLine(&fotm) != "" {
		t.Errorf("Form of the Master's line %q, want none", BuffLine(&fotm))
	}
	if !sameLine(EffectLine(&wolf), EffectLine(&hunter)) {
		t.Errorf("control: Wolf's Instinct %q and Hunter's Instinct %q not one line", EffectLine(&wolf), EffectLine(&hunter))
	}
	if sameLine(EffectLine(&wolf), EffectLine(&self)) || nestedLines(EffectLine(&wolf), EffectLine(&self)) {
		t.Errorf("Wolf's Instinct %q one with a self buff %q", EffectLine(&wolf), EffectLine(&self))
	}
}

// 5. Werehunter buffs: "For tanks and melees, they are category 4 (form). For casters and priests, they're category 2
// (self)." Blessing of Valentina (tanks') and Troka's Blessing (melees') stack with a self buff of their stats;
// Blessing of Luludja (priests') does not (its self category). Control: Valentina's stack type before (2).
func TestT277WerehunterBlessings(t *testing.T) {
	controlled := t277Stat(700903, 40, 0, "Controlled Force", "self", 2, "str:40")
	selfInt := t277Stat(9304, 40, 0, "Test Self INT", "self", 2, "int:30")
	valentina := t277Stat(1614, 60, 0, "Blessing of Valentina", "self", 4, "str:80,int:40")
	troka := t277Stat(901211, 60, 0, "Troka's Blessing", "self", 4, "dex:40,int:80")
	luludja := t277Stat(900483, 60, 0, "Blessing of Luludja", "self", 2, "str:80,wis:40")
	for _, pair := range [][2]CastSpell{{controlled, valentina}, {valentina, controlled}, {selfInt, troka}} {
		if _, got := t277Both(t, pair[0], pair[1]); len(got) != 2 {
			t.Errorf("%s then %s: %v, want both (form beside self)", pair[0].Name, pair[1].Name, got)
		}
	}
	old := t277Stat(1614, 60, 0, "Blessing of Valentina", "self", 2, "str:80,int:40")
	for _, pair := range [][2]CastSpell{{controlled, luludja}, {controlled, old}} {
		if _, got := t277Both(t, pair[0], pair[1]); len(got) != 1 {
			t.Errorf("control: %s then %s: %v, want one (both self)", pair[0].Name, pair[1].Name, got)
		}
	}
}

// t277Taunt is a taunt whose AC lasts on its caster for dur s (the taunt paths' Pillars, Towers, Bastions).
func t277Taunt(id, level int, name string, stack, ac, dur int) CastSpell {
	return wbSpell(id, level, 1, name, fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":%d,"level":%d,"min_level":1,"req_equipment":"Any","sheet_type":"Modify Hate","effects":[{"type":"Statistic Change","kind":"stat","stat":"ac","amount":%d,"duration_s":%d,"scope":"self","target_type":"Self"},{"type":"Modify Hate","kind":"hate","amount":795}]}`,
		stack, level, ac, dur))
}

// 6. Short tank AC buffs, "the 12-24 second AC buffs included in the various taunt paths ... effective in the first
// or target category to provide maximum stacking with self and group buffs": two taunts' AC on their caster count the
// stronger once (one line), beside a self and a group AC buff. Control: no stack type (before), the two added up.
func TestT277TauntACShares(t *testing.T) {
	noResist(t)
	for _, before := range []bool{false, true} {
		stack := 1
		if before {
			stack = 0
		}
		heroics := t277Taunt(700627, 49, "Pillar of Heroics", stack, 1000, 24)
		deeds := t277Taunt(700626, 49, "Pillar of Deeds", stack, 800, 18)
		for _, order := range [][2]CastSpell{{heroics, deeds}, {deeds, heroics}} {
			c := newCaster(t)
			c.npc.HP, c.npc.MaxHP, c.npc.Level = 50_000, 50_000, 30
			c.castOn(t, t277Stat(9305, 25, 0, "Test Self AC", "self", 2, "ac:102"), 0)
			c.castOn(t, t277Stat(9306, 20, 2, "Test Group AC", "group", 3, "ac:100"), 10*time.Second)
			base := c.p.Mods.AC
			if base < 200 {
				t.Fatalf("self and group AC: %d", base)
			}
			c.land(t, order[0], c.npcO, 20*time.Second)
			c.land(t, order[1], c.npcO, 26*time.Second)
			want := 1000
			if before {
				want = 1800
			}
			if got := c.p.Mods.AC - base; got != want {
				t.Errorf("before %v: %s then %s: taunts' AC %d, want %d (effects %v)", before, order[0].Name, order[1].Name, got, want, c.effectNames())
			}
			if got := c.p.Mods.AC; got < want+200 {
				t.Errorf("before %v: the self and group AC lost under the taunts: AC %d", before, got)
			}
		}
	}
}

// 7. Necromancer DoTs: "Some higher level necromancer DoT abilities stack with each other" (none named). Ours lets
// every DoT on a creature tick: two of a necromancer's both stay on it (the half the guide states that ours can).
func TestT277NecromancerDoTsBothOn(t *testing.T) {
	noResist(t)
	dot := func(id int, name string, n int) CastSpell {
		return wbSpell(id, 60, 1, name, fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,"stack":0,"level":60,"sheet_type":"Statistic Change","effects":[{"type":"Dmg/Heal","kind":"hp_tick","amount":%d,"resist":"dr"}]}`, -n))
	}
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 50_000, 50_000, 30
	c.land(t, dot(700825, "Toxxulian Blight", 193), c.npcO, 0)
	c.land(t, dot(701108, "Blight of Bertoxxulous", 257), c.npcO, 5*time.Second)
	var on []string
	for _, e := range c.npc.Effects {
		on = append(on, e.Name)
	}
	if len(on) != 2 {
		t.Errorf("two necromancer DoTs on a creature: %v, want both", on)
	}
}

// 8. Lycan buffs, racial CMs and MCs: "yet to be altered", they stack with everything (as Pure Heart). A were's
// ability and a racial master class's buff, each its own stack type, stack with a self or group buff of their
// stats; control: their stack types before (2, 3) did not.
func TestT277LycanAndRacialBuffs(t *testing.T) {
	controlled := t277Stat(700903, 40, 0, "Controlled Force", "self", 2, "str:40")
	groupAC := t277Stat(700831, 55, 2, "Test Group AC", "group", 3, "ac:250")
	for _, before := range []bool{false, true} {
		bear, marr := 901199, 760121
		if before {
			bear, marr = 2, 3
		}
		caveBear := t277Stat(901199, 40, 0, "Cave Bear Strength", "self", bear, "str:50")
		marrs := t277Stat(760121, 60, 2, "Marr's Blessing", "group", marr, "ac:200")
		for _, pair := range [][2]CastSpell{{controlled, caveBear}, {caveBear, controlled}, {groupAC, marrs}} {
			_, got := t277Both(t, pair[0], pair[1])
			if want := 2; before {
				want = 1
				if len(got) != want {
					t.Errorf("control (before): %s then %s: %v, want one", pair[0].Name, pair[1].Name, got)
				}
			} else if len(got) != want {
				t.Errorf("%s then %s: %v, want both", pair[0].Name, pair[1].Name, got)
			}
		}
	}
}

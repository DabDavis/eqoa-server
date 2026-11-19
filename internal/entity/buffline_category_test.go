package entity

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// The spells' effects as the production server's spelleffect rows hold them (2026-11-17, both shards).
const (
	needlesJSON = `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":1,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Damage Shield","effects":[{"id":"0204","type":"Statistic Change","kind":"stat","stat":"ar","amount":50},{"id":"0205","type":"Defensive Proc","kind":"def_proc","procs":[{"id":"0D19","type":"Dmg/Heal","kind":"hp","amount":-120,"resist":"pierce","on":"defense","scope":"target","target_type":"Enemy"},{"id":"0D1A","type":"Statistic Change","kind":"stat","stat":"ar","amount":-50,"on":"defense","scope":"target","target_type":"Enemy","duration_s":60}]}]}`
	megaWardJSON = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":1,"level":60,"min_level":36,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Buff All Resists","effects":[{"id":"0161","type":"Statistic Change","kind":"stat","stat":"fr","amount":85,"scale":"wis","scale_pct":10},{"id":"0161","type":"Statistic Change","kind":"stat","stat":"cr","amount":85,"scale":"wis","scale_pct":10},{"id":"0161","type":"Statistic Change","kind":"stat","stat":"lr","amount":85,"scale":"wis","scale_pct":10},{"id":"0161","type":"Statistic Change","kind":"stat","stat":"pr","amount":85,"scale":"wis","scale_pct":10},{"id":"0161","type":"Statistic Change","kind":"stat","stat":"dr","amount":85,"scale":"wis","scale_pct":10},{"id":"0161","type":"Statistic Change","kind":"stat","stat":"ar","amount":85,"scale":"wis","scale_pct":10}],"radius":30}`
	elemWardJSON = `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":600,"stack":1,"level":20,"min_level":12,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Buff All Resists","effects":[{"id":"011E","type":"Statistic Change","kind":"stat","stat":"fr","amount":25},{"id":"011E","type":"Statistic Change","kind":"stat","stat":"cr","amount":25},{"id":"011E","type":"Statistic Change","kind":"stat","stat":"lr","amount":25},{"id":"011E","type":"Statistic Change","kind":"stat","stat":"pr","amount":25},{"id":"011E","type":"Statistic Change","kind":"stat","stat":"dr","amount":25},{"id":"011E","type":"Statistic Change","kind":"stat","stat":"ar","amount":25}]}`
	zealousJSON = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":600,"stack":3,"level":55,"min_level":33,"radius":30,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Buff AC,PR,DR","effects":[{"id":"0521","type":"Statistic Change","kind":"stat","stat":"ac","amount":250,"level_pct":100},{"id":"0521","type":"Statistic Change","kind":"stat","stat":"pr","amount":125,"level_pct":100},{"id":"0521","type":"Statistic Change","kind":"stat","stat":"dr","amount":125,"level_pct":100}]}`
	carapaceJSON = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":3,"level":60,"min_level":36,"radius":10,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Buff All Resists","effects":[{"id":"0162","type":"Statistic Change","kind":"stat","stat":"fr","amount":75},{"id":"0162","type":"Statistic Change","kind":"stat","stat":"cr","amount":75},{"id":"0162","type":"Statistic Change","kind":"stat","stat":"lr","amount":75},{"id":"0162","type":"Statistic Change","kind":"stat","stat":"pr","amount":75},{"id":"0162","type":"Statistic Change","kind":"stat","stat":"dr","amount":75},{"id":"0162","type":"Statistic Change","kind":"stat","stat":"ar","amount":75}]}`
	groupPRDRJSON = `{"v":1,"scope":"group","stack":3,"level":35,"duration_s":1800,"effects":[{"kind":"stat","stat":"pr","amount":45},{"kind":"stat","stat":"dr","amount":45}]}`
	thornsJSON = `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":40,"arch_type":"Damage Shield","effects":[{"kind":"stat","stat":"ar","amount":20},{"kind":"def_proc","procs":[{"kind":"hp","amount":-40,"on":"defense","scope":"target","target_type":"Enemy"}]}]}`
)

func hasShieldOn(c *caster, id int) bool {
	e := c.effectOf(id)
	return e != nil && e.Shield != nil
}

// A damage shield and a resist buff are not one line (the owner, 2026-11-17: "druid's damage shields like 1000
// needles isnt stacking with bruce buffers mega ward. mega ward is resist buff, 1000 needles is damage shield not
// same category"): Shield of 1000 Needles (+50 AR beside its shield) and Mega's Clutch Ward (+85 to all six
// resists) both stay on, in either order, each AR counting; a damage shield still replaces a damage shield and a
// ward a ward.
func TestDamageShieldStacksWithResistWard(t *testing.T) {
	noResist(t)
	needles := wbSpell(701223, 60, 1, "Shield of 1000 Needles", needlesJSON)
	ward := wbSpell(701184, 60, 2, "Mega's Clutch Ward", megaWardJSON)
	if sameLine(EffectLine(&needles), EffectLine(&ward)) || nestedLines(EffectLine(&needles), EffectLine(&ward)) {
		t.Fatalf("lines %q and %q are one", EffectLine(&needles), EffectLine(&ward))
	}
	for _, order := range [][2]CastSpell{{needles, ward}, {ward, needles}} {
		c := newCaster(t)
		c.castOn(t, order[0], 0)
		c.castOn(t, order[1], 10*time.Second)
		got := c.effectNames()
		if len(got) != 2 || !hasShieldOn(c, needles.SpellID) || c.effectOf(ward.SpellID) == nil {
			t.Fatalf("%s then %s: effects %v (want both, the shield on)", order[0].Name, order[1].Name, got)
		}
		if ar := c.p.Mods.AR; ar < 50+85 || c.p.Mods.FR < 85 {
			t.Errorf("%s then %s: AR %d FR %d (want 50 + the ward's)", order[0].Name, order[1].Name, ar, c.p.Mods.FR)
		}
		if red := redLines(c.sess); len(red) != 0 {
			t.Errorf("%s then %s: told %q", order[0].Name, order[1].Name, red)
		}
	}

	// a damage shield replaces a damage shield: a weaker one is refused under Needles, Needles takes it off
	thorns := wbSpell(9401, 40, 1, "Test Thorns", thornsJSON)
	c := newCaster(t)
	c.castOn(t, thorns, 0)
	c.castOn(t, ward, 10*time.Second)
	c.castOn(t, needles, 20*time.Second)
	if got := c.effectNames(); len(got) != 2 || c.effectOf(thorns.SpellID) != nil || !hasShieldOn(c, needles.SpellID) {
		t.Fatalf("Needles over Test Thorns beside the ward: %v", got)
	}
	c.castOn(t, thorns, 30*time.Second)
	if c.effectOf(thorns.SpellID) != nil {
		t.Errorf("Test Thorns took under Needles: %v", c.effectNames())
	}

	// a resist ward replaces a resist ward, the damage shield left on
	elem := wbSpell(900258, 20, 1, "Elemental Ward", elemWardJSON)
	c = newCaster(t)
	c.castOn(t, elem, 0)
	c.castOn(t, needles, 10*time.Second)
	c.castOn(t, ward, 20*time.Second)
	if got := c.effectNames(); len(got) != 2 || c.effectOf(elem.SpellID) != nil || !hasShieldOn(c, needles.SpellID) {
		t.Fatalf("Mega's Clutch Ward over Elemental Ward beside Needles: %v", got)
	}
	c.castOn(t, elem, 30*time.Second)
	if c.effectOf(elem.SpellID) != nil {
		t.Errorf("Elemental Ward took under Mega's Clutch Ward: %v", c.effectNames())
	}
}

// Bruce Buffer's Mega's Clutch Ward lands beside a druid's Shield of 1000 Needles, and Needles lands beside his ward.
func TestBufferWardBesideNeedles(t *testing.T) {
	noResist(t)
	needles := wbSpell(701223, 60, 1, "Shield of 1000 Needles", needlesJSON)
	ward := bufferSpell(701184, 60, 2, "Mega's Clutch Ward", megaWardJSON, 0)
	for _, needlesFirst := range []bool{true, false} {
		c := newBufferRig(t)
		at := time.Duration(0)
		if needlesFirst {
			c.castOn(t, needles, 0)
			at = 10 * time.Second
		}
		if _, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, []CastSpell{ward}, c.at(at)); !ok {
			t.Fatal("Buffer's cast not begun")
		}
		c.bufferTick(at + BufferCastTime)
		if !needlesFirst {
			c.castOn(t, needles, at+10*time.Second)
		}
		if got := c.effectNames(); !slices.Contains(got, ward.Name) || !hasShieldOn(c, needles.SpellID) {
			t.Fatalf("needles first %v: effects %v (want the ward and the shield)", needlesFirst, got)
		}
	}
}

// The paladin's group Defenders (+AC +PR +DR) and the cleric's group resist buffs are of two kinds too: Zealous
// Defender no longer takes Mega's Mighty Carapace off (its FR, CR, LR and AR with it) nor refuses the cleric's PR/DR
// buff; a resist line within a resist line keeps the nested rule (buffline_kinds_test.go).
func TestDefenderStacksWithResistBuffs(t *testing.T) {
	noResist(t)
	zealous := wbSpell(700831, 55, 2, "Zealous Defender", zealousJSON)
	carapace := wbSpell(701185, 60, 2, "Mega's Mighty Carapace", carapaceJSON)
	prdr := wbSpell(700396, 35, 2, "Group Resist Affliction", groupPRDRJSON)
	for _, order := range [][2]CastSpell{{zealous, carapace}, {carapace, zealous}, {zealous, prdr}, {prdr, zealous}} {
		c := newCaster(t)
		c.castOn(t, order[0], 0)
		c.castOn(t, order[1], 10*time.Second)
		if got := c.effectNames(); len(got) != 2 {
			t.Errorf("%s then %s: effects %v (want both)", order[0].Name, order[1].Name, got)
		}
	}
	// two resist lines: the cleric's PR/DR within its all-resist line, as before (nested, the stronger PR once)
	if !nestedLines(EffectLine(&prdr), EffectLine(&carapace)) {
		t.Errorf("%q within %q: not nested", EffectLine(&prdr), EffectLine(&carapace))
	}
	// a line of another kind with a resist still goes by its other stats: Steel Stance under Diamond Stance
	steel := wbSpell(700650, 49, 0, "Steel Stance", `{"v":1,"scope":"self","stack":2,"level":49,"duration_s":1800,"effects":[{"kind":"stat","stat":"ac","amount":100},{"kind":"stat","stat":"agi","amount":20}]}`)
	diamond := wbSpell(700586, 49, 0, "Diamond Stance", `{"v":1,"scope":"self","stack":2,"level":49,"duration_s":1800,"effects":[{"kind":"stat","stat":"ac","amount":120},{"kind":"stat","stat":"agi","amount":25},{"kind":"def_proc","procs":[{"kind":"hp","amount":-30,"on":"defense","scope":"target","target_type":"Enemy"}]}]}`)
	if !sameLine(EffectLine(&steel), EffectLine(&diamond)) {
		t.Errorf("%q and %q: not one line", EffectLine(&steel), EffectLine(&diamond))
	}
	if !strings.Contains(EffectLine(&diamond), "shield") {
		t.Errorf("Diamond Stance's line %q has no shield", EffectLine(&diamond))
	}
}

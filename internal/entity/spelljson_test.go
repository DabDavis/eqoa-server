package entity

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The spells here are made up (the workbook's own rows are the operator's,
// never this repository's); only the shape of spelleffect.effects is the
// import's.

// wbSpell is a spell whose spelleffect row holds effects js (and the line
// "Target, Decrease Hitpoints by 1", which they must win over), cast 1 s, at
// a range of 30.
func wbSpell(id, level, scope int, name, js string) CastSpell {
	sp := &database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: 0x41F0, CastTime: 1, Power: 10, Scope: scope,
		SpellName: name, Icon: int64(id)}
	return SpellOfRow(sp, id%40, "Target, Decrease Hitpoints by 1", js, 0)
}

// noResist makes every workbook resist roll fail (and every proc chance
// land) until the test ends.
func noResist(t *testing.T) {
	old := procIntn
	procIntn = func(int) int { return 99 }
	t.Cleanup(func() { procIntn = old })
}

// A spell's effects read as the line reader's kinds and fields, and its
// line is read only without them: empty, or not JSON of version 1 (the
// control).
func TestWorkbookEffectsAreWhatASpellDoes(t *testing.T) {
	heal := `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":24,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp_tick","amount":20,"scale":"cha","scale_pct":25}]}`
	d := SpellDefFrom("Test Mend", "Target, Decrease Hitpoints by 9", "", heal)
	if !d.Workbook || d.Harmful || d.Target != TargetSingle || d.Duration != 24*time.Second || len(d.Effects) != 1 {
		t.Fatalf("the heal: %+v", d)
	}
	if e := d.Effects[0]; e.Kind != EffectHPTick || e.Amount != 20 || e.AmountMax != 20 || e.Scale != StatCha || e.ScalePct != 25 || e.ID != "0001" {
		t.Errorf("its tick: %+v", e)
	}
	storm := `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"radius":25,
		"effects":[{"id":"0002","kind":"hp","amount":-40,"resist":"fr"},{"id":"0002","kind":"hp2","amount":-15,"resist":"fr"}]}`
	d = SpellDefFrom("Test Storm", "", "", storm)
	if !d.Harmful || len(d.Effects) != 2 || d.Effects[0].Amount != -40 || d.Effects[1].Amount != -15 || d.Effects[0].Resist != StatFR {
		t.Fatalf("the storm: %+v", d)
	}
	a := ReadSpellArea("Test Storm", "", d)
	if a.Centre != AreaOnTarget || a.Radius != 25 || len(a.Effects) != 1 || a.Effects[0].Amount != -15 {
		t.Errorf("the storm's area: %+v", a)
	}
	for _, bad := range []string{"", "  ", `{"v":2,"effects":[{"kind":"hp","amount":-5}]}`, `{"v":1,"effects":[]}`, `not json`} {
		d := SpellDefFrom("Test Bolt", "Target, Decrease Hitpoints by 9", "", bad)
		if d.Workbook || len(d.Effects) != 1 || d.Effects[0].Amount != -9 {
			t.Errorf("effects %q: not the line's %+v", bad, d)
		}
	}
	// an effect for its caster in a spell for a target: a price, a tap's
	// share (lifetap.go); a speed with no figure, the movement buff
	d = SpellDefFrom("Test Pact", "", "", `{"v":1,"scope":"target","target_type":"Friendly","effects":[
		{"kind":"hp","amount":100},{"kind":"hp","amount":-30,"scope":"self"},{"kind":"speed","duration_s":60}]}`)
	if len(d.Effects) != 3 || d.Effects[0].Self || !d.Effects[1].Self || d.Effects[2].Kind != EffectSpeed || d.Harmful {
		t.Errorf("the pact: %+v", d)
	}
	// what the engine has no rule for is kept, never acted on
	d = SpellDefFrom("Test Odd", "", "", `{"v":1,"scope":"target","harmful":true,"effects":[{"kind":"no_such"},{"kind":"stat_set","stat":"pierce","amount":3}]}`)
	if len(d.Unsupported) != 2 || len(d.Effects) != 0 {
		t.Errorf("the unsupported: %+v", d)
	}
}

// The kinds that name a thing: a pet by name with its workbook id, an item
// made, an illusion's body by its sprite, the invisible body as
// invisibility, a resurrection's share, a mitigation, a cure's words, a
// teleport read as one (gates.go), a stat set, a slow.
func TestWorkbookKindsMapOntoTheEngines(t *testing.T) {
	d := SpellDefFrom("Summon Test Golem", "", "", `{"v":1,"scope":"self","effects":[{"kind":"summon","pet":{"sheet_pet_id":"00AB","name":"Test Golem"}}]}`)
	if !d.Summons() || d.Pet != "test golem" || d.SheetPetID != "00AB" || d.Harmful || d.Target != TargetSelf {
		t.Errorf("the summoning: %+v", d)
	}
	d = SpellDefFrom("Test Bread", "", "", `{"v":1,"scope":"self","effects":[{"kind":"create_item","item":{"name":"Test Loaf","count":3}}]}`)
	if len(d.Items) != 1 || d.Items[0] != (SpellItem{Name: "Test Loaf", Count: 3}) {
		t.Errorf("the item: %+v", d.Items)
	}
	// the sprite as the workbook writes it, byte-reversed: the body is the
	// CSprite the other way round (spriteBody)
	d = SpellDefFrom("Test Shape", "", "", `{"v":1,"scope":"self","duration_s":60,"effects":[{"kind":"illusion","sprite":"0x12345678"}]}`)
	if d.FormKind != FormSelf || d.FormModel != 0x78563412 || d.Duration != time.Minute {
		t.Errorf("the illusion: %+v", d)
	}
	d = SpellDefFrom("True Form", "", "", `{"v":1,"scope":"self","effects":[{"kind":"illusion","sprite":"0x12345678"}]}`)
	if d.FormKind != FormEnd || len(d.Effects) != 1 || d.Effects[0].Kind != EffectEndIllusion {
		t.Errorf("an illusion's end: %+v", d)
	}
	d = SpellDefFrom("Test Veil", "", "", `{"v":1,"scope":"self","duration_s":60,"effects":[{"kind":"illusion","sprite":"0x435EF663"}]}`)
	if d.FormKind != FormNone || len(d.Effects) != 1 || d.Effects[0].Kind != EffectInvis {
		t.Errorf("the invisible body: %+v", d)
	}
	d = SpellDefFrom("Test Rez", "", "", `{"v":1,"scope":"target","effects":[{"kind":"resurrect","rez_pct":-35}]}`)
	if d.Resurrects() != 35 {
		t.Errorf("the rez: %d", d.Resurrects())
	}
	d = SpellDefFrom("Test Wall", "", "", `{"v":1,"scope":"self","duration_s":30,"effects":[{"kind":"invulnerable","mitigation":{"pct":40,"max":-2000}},
		{"kind":"mitigation","mitigation":{"pct":5,"max":100000}}]}`)
	if len(d.Effects) != 2 || d.Effects[0].Kind != EffectMitigate || d.Effects[0].Amount != 40 || d.Effects[0].AmountMax != 2000 ||
		d.Effects[1].Amount != 5 || d.Effects[1].AmountMax != 0 {
		t.Errorf("the mitigations: %+v", d.Effects)
	}
	d = SpellDefFrom("Test Cure", "", "", `{"v":1,"scope":"target","effects":[{"kind":"cure","removes":"poison"}]}`)
	if len(d.Removes) != 1 || d.Removes[0] != "poison" || d.Effects[0].Kind != EffectCure {
		t.Errorf("the cure: %+v", d)
	}
	d = SpellDefFrom("Test Gate", "", "", `{"v":1,"scope":"self","effects":[{"kind":"teleport","location":"Tunaria,1,2,3"}]}`)
	if tp, _ := gateLine(&d); !tp || d.Location != "Tunaria,1,2,3" {
		t.Errorf("the teleport: %+v", d)
	}
	d = SpellDefFrom("Test Hold", "", "", `{"v":1,"scope":"target","harmful":true,"duration_s":30,
		"effects":[{"kind":"speed","amount":-30,"resist":"ar"},{"kind":"stat_set","stat":"maxpower","amount":0},{"kind":"stat_set","stat":"str","amount":5}]}`)
	if len(d.Effects) != 3 || d.Effects[0].Kind != EffectSnare || d.Effects[0].Amount != -30 || d.Effects[1].Kind != EffectPowerCap ||
		d.Effects[2].Kind != EffectStat || !d.Effects[2].Set || d.Effects[2].Stat != StatStr {
		t.Errorf("the slow and the sets: %+v", d.Effects)
	}
	// a "Statistic Absolute" speed below 0 holds in place; its power is set
	d = SpellDefFrom("Test Bind", "", "", `{"v":1,"scope":"target","harmful":true,"duration_s":30,
		"effects":[{"kind":"speed","amount":-50,"absolute":true},{"kind":"power","amount":240,"absolute":true}]}`)
	if len(d.Effects) != 2 || d.Effects[0].Kind != EffectRoot || d.Effects[1].Kind != EffectPower || !d.Effects[1].Set ||
		d.Effects[1].Amount != 240 {
		t.Errorf("the absolutes: %+v", d.Effects)
	}
	d = SpellDefFrom("Test Thorns", "", "", `{"v":1,"scope":"self","duration_s":300,"effects":[{"kind":"def_proc","procs":[
		{"kind":"hp","amount":-8,"on":"defense","resist":"pierce"},{"kind":"hp","amount":12,"on":"defense"}]},
		{"kind":"proc","procs":[{"kind":"hp","amount":-20,"on":"offense","scale":"int","scale_pct":10}]}]}`)
	if len(d.Effects) != 3 || d.Effects[0].Kind != EffectShield || d.Effects[0].Amount != 8 ||
		d.Effects[1].Kind != EffectProcHeal || !d.Effects[1].Defense || d.Effects[1].Amount != 12 ||
		d.Effects[2].Kind != EffectProc || d.Effects[2].Amount != 20 || !d.Effects[2].ScaleBearer || d.Harmful {
		t.Errorf("the procs: %+v", d.Effects)
	}
}

// Level scaling: the caster's level times its share more, signed as the
// effect is; without it (the control) the figure alone.
func TestWorkbookLevelScaling(t *testing.T) {
	c := newCaster(t) // a druid of 23
	for _, tc := range []struct {
		e    SpellEffect
		want int
	}{
		{SpellEffect{Kind: EffectHate, Amount: 20, AmountMax: 20, LevelPct: 100}, 43},
		{SpellEffect{Kind: EffectStat, Stat: StatAC, Amount: 50, AmountMax: 50, LevelPct: 200}, 96},
		{SpellEffect{Kind: EffectHP, Amount: -10, AmountMax: -10, LevelPct: 50}, -21},
		{SpellEffect{Kind: EffectHP, Amount: -10, AmountMax: -10}, -10},
	} {
		if got := spellAmount(tc.e, c.p); got != tc.want {
			t.Errorf("%+v: %d, want %d", tc.e, got, tc.want)
		}
	}
}

// A creature resists a harmful workbook effect by its resist basis, wholly,
// at the proc's chance (15% for one ten levels above the caster): resisted,
// the caster is told and the creature is angered; the control, the roll
// against it, lands. A dummy never resists, nor does a line's spell.
func TestWorkbookResists(t *testing.T) {
	bolt := wbSpell(901, 5, 1, "Test Bolt", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,
		"effects":[{"id":"0003","kind":"hp","amount":-12,"resist":"fr"}]}`)
	for _, tc := range []struct {
		roll    int
		landed  bool
		dummy   bool
		message bool
	}{{0, true, false, true}, {14, true, false, true}, {15, true, false, false}, {0, true, true, false}} {
		c := newCaster(t)
		c.npc.Level, c.npc.HP, c.npc.MaxHP = 33, 1000, 1000
		if tc.dummy {
			c.npc.Dummy = TrainingDummy
		}
		roll := tc.roll
		old := procIntn
		procIntn = func(int) int { return roll }
		if !c.w.StartCast(c.sess, bolt, c.npcO, c.at(0)) {
			t.Fatal("not begun")
		}
		c.sent()
		c.tick(bolt.CastTime + castLand)
		procIntn = old
		landed := c.npc.HP < 1000
		var told bool
		for _, m := range c.sent() {
			told = told || strings.Contains(string(m), "r\x00e\x00s\x00i\x00s\x00t\x00e\x00d")
		}
		if tc.dummy {
			landed = told == false
		}
		if landed != tc.landed || told != tc.message {
			t.Errorf("roll %d (dummy %v): landed %v, told %v; want %v, %v", tc.roll, tc.dummy, landed, told, tc.landed, tc.message)
		}
		// resisted, it lands at its least: the bolt's 12, never nothing (the user, 2026-11-07)
		if tc.message && !tc.dummy && 1000-c.npc.HP != 12 {
			t.Errorf("roll %d: a resisted bolt took %d, want its least, 12", tc.roll, 1000-c.npc.HP)
		}
		if !tc.landed && c.npc.CurrentEnemy != c.p.ServerID {
			t.Errorf("roll %d: resisted, the creature is not angered (%d)", tc.roll, c.npc.CurrentEnemy)
		}
	}
	if got := npcSpellResistPct(23, &NPC{Level: 33}, StatFR, time.Now()); got != 15 {
		t.Errorf("ten above: %d%%, want 15", got)
	}
	// a fire resist debuff of the cap's on it takes 75 points off
	debuffed := &NPC{Level: 60, Effects: []*ActiveEffect{{Until: time.Now().Add(time.Minute), Mods: StatMods{FR: -ResistCap}}}}
	if got := npcSpellResistPct(23, debuffed, StatFR, time.Now()); got != 0 {
		t.Errorf("under the debuff: %d%%, want 0", got)
	}
	if got := npcSpellResistPct(23, debuffed, StatCR, time.Now()); got != 75 {
		t.Errorf("another kind under it: %d%%, want 75", got)
	}
}

// Stack types: two buffs of one stack type and the same stats are one line,
// the higher level's taking the lower's place and a lower refused under it;
// another stack type, or no stat shared, stacks (the control). (Nested stats,
// the one's within the other's: TestNestedResistLines.)
func TestWorkbookStackTypes(t *testing.T) {
	noResist(t)
	buff := func(id, level, stack int, stats string) CastSpell {
		var parts []string
		for _, s := range strings.Split(stats, ",") {
			parts = append(parts, `{"kind":"stat","stat":"`+s+`","amount":10}`)
		}
		js := `{"v":1,"scope":"self","stack":` + itoa(stack) + `,"level":` + itoa(level) + `,"duration_s":600,"effects":[` + strings.Join(parts, ",") + `]}`
		return wbSpell(id, level, 0, "Test Buff "+itoa(id), js)
	}
	low, high := buff(911, 10, 2, "ac,str"), buff(912, 20, 2, "ac,str")
	other, apart := buff(913, 5, 3, "ac"), buff(914, 5, 2, "dex")
	none := buff(915, 1, 0, "ac")
	c := newCaster(t)
	c.castOn(t, low, 0)
	c.castOn(t, high, 10*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != high.Name {
		t.Fatalf("the higher over the lower: %v", got)
	}
	c.castOn(t, low, 20*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != high.Name {
		t.Errorf("the lower under the higher: %v", got)
	}
	c.castOn(t, other, 30*time.Second)
	c.castOn(t, apart, 40*time.Second)
	c.castOn(t, none, 50*time.Second)
	if got := c.effectNames(); len(got) != 4 {
		t.Errorf("another stack type, a stat not shared and no stack type stack: %v", got)
	}
	if BuffLine(&none) != "" {
		t.Errorf("no stack type has a line: %q", BuffLine(&none))
	}
	if !sameLine(BuffLine(&low), BuffLine(&high)) || sameLine(BuffLine(&high), BuffLine(&other)) || sameLine(BuffLine(&high), BuffLine(&apart)) {
		t.Errorf("the lines: %q %q %q %q", BuffLine(&low), BuffLine(&high), BuffLine(&other), BuffLine(&apart))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// Ticks: a workbook heal over time ticks twice at its landing, then once a
// tick; a damage tick on a creature the same; a line's (the control) once.
func TestWorkbookTicksTwiceAtTheLanding(t *testing.T) {
	noResist(t)
	hot := wbSpell(921, 5, 1, "Test Renew", `{"v":1,"scope":"target","target_type":"Friendly","duration_s":24,
		"effects":[{"kind":"hp_tick","amount":20}]}`)
	c := newCaster(t)
	c.p.HP = 300
	c.castOn(t, hot, 0)
	if c.p.HP != 340 {
		t.Errorf("after the landing: %d HP, want 340 (two ticks of 20)", c.p.HP)
	}
	dot := wbSpell(922, 5, 1, "Test Rot", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,
		"effects":[{"kind":"hp_tick","amount":-10,"resist":"dr"}]}`)
	c = newCaster(t)
	c.npc.HP, c.npc.MaxHP = 1000, 1000
	if !c.w.StartCast(c.sess, dot, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	land := dot.CastTime + castLand
	c.tick(land)
	first := 1000 - c.npc.HP
	c.tick(land + effectTick)
	second := 1000 - c.npc.HP - first
	if first != 2*second || second <= 0 {
		t.Errorf("the damage ticks: %d at the landing, %d at the next; want twice the next", first, second)
	}
	c = newCaster(t)
	c.p.HP = 300
	line := minorHeal()
	line.Def = ParseSpellText("Target, Increase HP per tic by 20, 24sec")
	c.castOn(t, line, 0)
	if c.p.HP != 320 {
		t.Errorf("the control, a line's heal: %d HP, want 320", c.p.HP)
	}
}

// What ends on a cast ends when its bearer begins one, but a run speed, which
// only a harmful cast ends (petition 1067); what ends on contact ends at a
// creature's hit; the rest stays (the control). (A hide is invisibility,
// which any cast ends besides: invis.go.) On the code before 1067 the skin's
// cast ended the haste too.
func TestWorkbookDispels(t *testing.T) {
	noResist(t)
	quick := wbSpell(931, 5, 0, "Test Haste", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"speed","amount":130,"dispel_on_cast":true}]}`)
	focus := wbSpell(934, 5, 0, "Test Focus", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"stat","stat":"str","amount":5,"dispel_on_cast":true}]}`)
	shade := wbSpell(932, 5, 0, "Test Shade", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"hide","dispel_on_contact":true}]}`)
	skin := wbSpell(933, 5, 0, "Test Skin", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"stat","stat":"ac","amount":10}]}`)
	c := newCaster(t)
	c.castOn(t, quick, 0)
	c.castOn(t, focus, 5*time.Second)
	// a cast ends the focus; the haste, a run speed, ends only at a harmful
	// cast (petition 1067: TestABeneficialCastKeepsARunSpeed)
	c.castOn(t, skin, 10*time.Second)
	c.castOn(t, shade, 20*time.Second)
	if got := c.effectNames(); len(got) != 3 || got[0] != quick.Name || got[1] != skin.Name || got[2] != shade.Name {
		t.Fatalf("after a cast: %v, want the haste, the skin and the shade", got)
	}
	if !c.p.Invisible() {
		t.Error("hidden, the player is not unseen")
	}
	c.w.mu.Lock()
	c.w.npcAttackPlayer(c.npc, c.p.ServerID, c.at(30*time.Second))
	c.w.mu.Unlock()
	if got := c.effectNames(); len(got) != 2 || got[0] != quick.Name || got[1] != skin.Name {
		t.Errorf("after a hit: %v, want the haste and the skin (the shade ends at a hit, the haste has no such end)", got)
	}
}

// A mitigation takes its share off each melee hit until its most is spent;
// one with no most never runs out.
func TestWorkbookMitigation(t *testing.T) {
	wall := wbSpell(941, 5, 0, "Test Wall", `{"v":1,"scope":"self","duration_s":30,"effects":[{"kind":"invulnerable","mitigation":{"pct":40,"max":-100}}]}`)
	c := newCaster(t)
	c.castOn(t, wall, 0)
	now := c.at(5 * time.Second)
	for i, want := range []int{60, 60, 80, 100} {
		if got := c.w.mitigateLocked(c.p, 100, now); got != want {
			t.Errorf("hit %d: %d taken, want %d", i+1, got, want)
		}
	}
	// its most spent, it breaks (the owner, 2026-11-13: Stoneskin "breaks after 7500 stopped")
	if got := c.effectNames(); len(got) != 0 {
		t.Errorf("a spent mitigation is still on: %v", got)
	}
	cloak := wbSpell(942, 5, 0, "Test Cloak", `{"v":1,"scope":"self","duration_s":30,"effects":[{"kind":"mitigation","mitigation":{"pct":10,"max":100000}}]}`)
	c = newCaster(t)
	c.castOn(t, cloak, 0)
	for i := 0; i < 50; i++ {
		if got := c.w.mitigateLocked(c.p, 1000, c.at(5*time.Second)); got != 900 {
			t.Fatalf("hit %d: %d taken, want 900", i+1, got)
		}
	}
	if got := c.w.mitigateLocked(c.p, 1000, c.at(time.Minute)); got != 1000 {
		t.Errorf("over: %d taken, want 1000", got)
	}
}

// A lull angers nothing as it lands, and the creature notices no one while
// it lasts; a hit ends it and the fight begins.
func TestWorkbookLull(t *testing.T) {
	noResist(t)
	lull := wbSpell(951, 5, 1, "Test Hush", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":12,
		"effects":[{"kind":"lull","resist":"ar","dispel_on_cast":true,"dispel_on_contact":true}]}`)
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 1000, 1000
	if !c.w.StartCast(c.sess, lull, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	at := lull.CastTime + castLand
	c.tick(at)
	if !c.npc.lulledAt(c.at(at)) || c.npc.CurrentEnemy != 0 || len(c.npc.Threats) != 0 {
		t.Fatalf("lulled: %v, enemy %d, threats %v", c.npc.lulledAt(c.at(at)), c.npc.CurrentEnemy, c.npc.Threats)
	}
	b := strike()
	b.SpellID = 952
	if !c.w.StartCast(c.sess, b, c.npcO, c.at(5*time.Second)) {
		t.Fatal("the strike not begun")
	}
	c.tick(5*time.Second + b.CastTime + castLand)
	if c.npc.lulledAt(c.at(9*time.Second)) || c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("after a hit: lulled %v, enemy %d", c.npc.lulledAt(c.at(9*time.Second)), c.npc.CurrentEnemy)
	}
}

// The beta sheet's CHA rule (the owner, 2026-11-11: "Spells that modify an NPCs
// behavior (Charm, Lull, Etc) will have a CHA mod on them that increases their
// duration"): a lull lasts its time and its CHA share of its caster's CHA in
// seconds, its effect's own (Soothe's 10%) else charm's 25% (Soothing Verse's
// DEX 15% is not CHA). Landed by a caster of CHA 100: a 12 s lull of no share
// 37 s, of 10% CHA 22 s, of 15% DEX 37 s; the control, CHA 0: 12 s.
func TestALullLastsLongerByCHA(t *testing.T) {
	for i, c := range []struct {
		scale string
		cha   int
		want  time.Duration
	}{{``, 100, 37 * time.Second}, {`,"scale":"cha","scale_pct":10`, 100, 22 * time.Second},
		{`,"scale":"dex","scale_pct":15`, 100, 37 * time.Second}, {``, 0, 12 * time.Second}} {
		noResist(t)
		lull := wbSpell(951+i, 5, 1, "Test Hush", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":12,
			"effects":[{"kind":"lull","resist":"ar"`+c.scale+`,"dispel_on_cast":true,"dispel_on_contact":true}]}`)
		cs := newCaster(t)
		cs.p.Cha = c.cha
		if !cs.w.StartCast(cs.sess, lull, cs.npcO, cs.at(0)) {
			t.Fatal("not begun")
		}
		at := lull.CastTime + castLand
		cs.tick(at)
		if len(cs.npc.Effects) != 1 {
			t.Fatalf("%q: effects %+v", c.scale, cs.npc.Effects)
		}
		if got := cs.npc.Effects[0].Until.Sub(cs.at(at)); got != c.want {
			t.Errorf("%q, CHA %d: the lull lasts %v, want %v", c.scale, c.cha, got, c.want)
		}
		if !cs.npc.lulledAt(cs.at(at+c.want-time.Second)) || cs.npc.lulledAt(cs.at(at+c.want)) {
			t.Errorf("%q, CHA %d: not lulled to its end alone", c.scale, c.cha)
		}
	}
	// a spell that does not lull gets nothing
	if d := ParseSpellText("Target, Root, 36sec"); lullBonus(&d, 300) != 0 {
		t.Errorf("a root's CHA share: %v", lullBonus(&d, 300))
	}
}

// A root that ends at contact (the workbook's Root, Arcane Bindings,
// Caltrops) holds after its landing and ends at the next hit; until
// 2026-11-01 its own landing's anger was that contact, and it never held.
func TestARootThatEndsAtContactHoldsUntilAHit(t *testing.T) {
	noResist(t)
	root := wbSpell(9811, 8, 1, "Test Root", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":36,
		"effects":[{"kind":"speed","amount":-100,"absolute":true,"resist":"ar","dispel_on_contact":true}]}`)
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 5000, 5000, 5
	held := func() bool {
		for _, e := range c.npc.Effects {
			if e.SpellID == root.SpellID && e.Root {
				return true
			}
		}
		return false
	}
	if !c.w.StartCast(c.sess, root, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	c.tick(root.CastTime + castLand)
	if !held() {
		t.Fatal("the root ended at its own landing")
	}
	b := strike()
	b.SpellID = 9812
	if !c.w.StartCast(c.sess, b, c.npcO, c.at(5*time.Second)) {
		t.Fatal("the strike not begun")
	}
	c.tick(5*time.Second + b.CastTime + castLand)
	if held() {
		t.Error("the root held through a hit")
	}
}

// A spell whose snare ends at contact and whose damage ticks (Caltrops):
// its own tick is no contact for itself, so it holds through its ticks; a
// hit from elsewhere ends the snare alone, the part the workbook marks (the
// user, 2026-11-01: "yes only end the marked part"), and its tick goes on to
// its 18 s. Until then the hit ended its ticks with it.
func TestASpellsOwnTickDoesNotEndItsContactSnare(t *testing.T) {
	noResist(t)
	caltrops := wbSpell(9831, 4, 1, "Test Caltrops", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":18,
		"effects":[{"kind":"hp_tick","amount":-5,"resist":"pierce"},{"kind":"speed","amount":-50,"resist":"dr","duration_s":8,"dispel_on_contact":true}]}`)
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 5000, 5000, 5
	on := func() *ActiveEffect {
		for _, e := range c.npc.Effects {
			if e.SpellID == caltrops.SpellID {
				return e
			}
		}
		return nil
	}
	if !c.w.StartCast(c.sess, caltrops, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	at := caltrops.CastTime + castLand
	c.tick(at)
	c.tick(at + 7*time.Second)
	if e := on(); e == nil || c.npc.HP >= 5000 || c.npc.snareFactor(c.at(at+7*time.Second)) != 0.5 {
		t.Fatalf("after its ticks: on %v, HP %d, pace %v", e != nil, c.npc.HP, c.npc.snareFactor(c.at(at+7*time.Second)))
	}
	b := strike()
	b.SpellID = 9832
	if !c.w.StartCast(c.sess, b, c.npcO, c.at(8*time.Second)) {
		t.Fatal("the strike not begun")
	}
	c.tick(8*time.Second + b.CastTime + castLand)
	e := on()
	if e == nil {
		t.Fatal("the hit ended its ticks with its snare")
	}
	if e.Snare || e.Rules.SpeedPct != 0 || c.npc.snareFactor(c.at(11*time.Second)) != 1 || e.HPTick >= 0 {
		t.Errorf("after a hit: snare %v, slow %d%%, pace %v, tick %d; want the snare gone, the tick on",
			e.Snare, e.Rules.SpeedPct, c.npc.snareFactor(c.at(11*time.Second)), e.HPTick)
	}
	hp := c.npc.HP
	c.tick(at + 13*time.Second)
	if c.npc.HP >= hp {
		t.Errorf("its tick stopped after the hit: HP %d, was %d", c.npc.HP, hp)
	}
	c.tick(at + 19*time.Second)
	if on() != nil {
		t.Error("still on past its 18 s")
	}
}

// A harmful spell the workbook gives no time whose effect ends at contact
// lasts until contact (Enshrouding darkness, the shadow knight's snare: the
// user's "until hit"): still on ten minutes later, gone at a hit. The
// control: a debuff of no time that does not end at contact lasts none.
func TestASnareOfNoTimeThatEndsAtContactLastsUntilHit(t *testing.T) {
	noResist(t)
	snare := wbSpell(9821, 60, 1, "Test Darkness", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,
		"effects":[{"id":"07A7","type":"Statistic Change","kind":"speed","amount":-50,"resist":"ar","dispel_on_contact":true}]}`)
	if snare.Def.Duration != untilContact {
		t.Fatalf("the snare's time %v, want %v", snare.Def.Duration, untilContact)
	}
	stench := wbSpell(9822, 6, 1, "Test Stench", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,
		"effects":[{"kind":"stat","stat":"agi","amount":-5}]}`)
	if stench.Def.Duration != 0 {
		t.Fatalf("the control's time %v, want none", stench.Def.Duration)
	}
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 5000, 5000, 55
	on := func() bool {
		for _, e := range c.npc.Effects {
			if e.SpellID == snare.SpellID {
				return true
			}
		}
		return false
	}
	if !c.w.StartCast(c.sess, snare, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	c.tick(snare.CastTime + castLand)
	c.tick(10 * time.Minute)
	if !on() {
		t.Fatal("the snare is gone before any hit")
	}
	b := strike()
	b.SpellID = 9823
	if !c.w.StartCast(c.sess, b, c.npcO, c.at(10*time.Minute)) {
		t.Fatal("the strike not begun")
	}
	c.tick(10*time.Minute + b.CastTime + castLand)
	if on() {
		t.Error("the snare stayed through a hit")
	}
}

// A cure takes off what its words remove and leaves the rest: poison goes,
// a disease stays (the control); no words take every harmful effect.
func TestWorkbookCure(t *testing.T) {
	c := newCaster(t)
	until := c.at(time.Hour)
	put := func(id int, name string, resist Stat) {
		c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: id, Name: name, Until: until, Rules: EffectRules{Harm: true, Resist: resist}})
	}
	put(1, "Test Venom", StatPR)
	put(2, "Test Fever", StatDR)
	c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: 3, Name: "Test Skin", Until: until, Mods: StatMods{AC: 10}})
	cure := wbSpell(961, 5, 0, "Test Antidote", `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"poison"}]}`)
	c.castOn(t, cure, 0)
	if got := c.effectNames(); len(got) != 2 || got[0] != "Test Fever" || got[1] != "Test Skin" {
		t.Errorf("after the poison's cure: %v", got)
	}
	all := wbSpell(962, 5, 0, "Test Purge", `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"ID?"}]}`)
	c.castOn(t, all, 10*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != "Test Skin" {
		t.Errorf("after a cure of anything harmful: %v", got)
	}
}

// A cure the workbook leaves at "ID?" goes by its spell's name (the user:
// "by name"): an antidote takes the poison and leaves the disease, a
// curative the reverse, a mask's removal is a mask's; one whose name says
// nothing still takes every harmful effect (the control).
func TestAnIDCureGoesByItsName(t *testing.T) {
	put := func(c *caster) {
		until := c.at(time.Hour)
		c.p.Effects = append(c.p.Effects,
			&ActiveEffect{SpellID: 1, Name: "Test Venom", Until: until, Rules: EffectRules{Harm: true, Resist: StatPR}},
			&ActiveEffect{SpellID: 2, Name: "Test Fever", Until: until, Rules: EffectRules{Harm: true, Resist: StatDR}},
			&ActiveEffect{SpellID: 3, Name: "Test Hex", Until: until, Rules: EffectRules{Harm: true, Resist: StatAR}},
			&ActiveEffect{SpellID: 4, Name: "Test Skin", Until: until, Mods: StatMods{AC: 10}})
	}
	for _, k := range []struct {
		name string
		left string
	}{
		{"Minor Antidote", "Test Fever,Test Hex,Test Skin"},
		{"Vital Curative", "Test Venom,Test Hex,Test Skin"},
		{"Test Purity", "Test Skin"},
	} {
		c := newCaster(t)
		put(c)
		cure := wbSpell(971, 5, 0, k.name, `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"ID?"}]}`)
		c.castOn(t, cure, 0)
		if got := strings.Join(c.effectNames(), ","); got != k.left {
			t.Errorf("after %s: %s, want %s", k.name, got, k.left)
		}
	}
	if mask := wbSpell(972, 5, 0, "Remove Mask", `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"ID?"}]}`); len(mask.Def.Removes) != 1 || mask.Def.Removes[0] != "mask" {
		t.Errorf("Remove Mask removes %q", mask.Def.Removes)
	}
}

// A cure of damage over time (Envaric Salve's "PoE DoT?": the user, "damage
// over time") takes a harmful effect whose tick hurts and leaves a poison and
// a curse that do not tick; an antidote still takes the poison alone (the
// control: "antidote" holds the letters of "dot").
func TestADoTCureTakesWhatTicks(t *testing.T) {
	for _, k := range []struct {
		name, removes, left string
	}{
		{"Envaric Salve", "PoE DoT?", "Test Sting,Test Hex,Test Skin"},
		{"Minor Antidote", "ID?", "Test Rot,Test Hex,Test Skin"},
	} {
		c := newCaster(t)
		until := c.at(time.Hour)
		c.p.Effects = append(c.p.Effects,
			&ActiveEffect{SpellID: 1, Name: "Test Rot", Until: until, HPTick: -50, NextTick: until, Rules: EffectRules{Harm: true, Resist: StatDR}},
			&ActiveEffect{SpellID: 2, Name: "Test Sting", Until: until, Rules: EffectRules{Harm: true, Resist: StatPR}},
			&ActiveEffect{SpellID: 3, Name: "Test Hex", Until: until, Rules: EffectRules{Harm: true, Resist: StatAR}},
			&ActiveEffect{SpellID: 4, Name: "Test Skin", Until: until, Mods: StatMods{AC: 10}})
		cure := wbSpell(973, 55, 0, k.name, `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"`+k.removes+`"}]}`)
		c.castOn(t, cure, 0)
		if got := strings.Join(c.effectNames(), ","); got != k.left {
			t.Errorf("after %s: %s, want %s", k.name, got, k.left)
		}
	}
}

// A spell for a target whose every effect is for the area round it
// (Lightning Maelstrom's record) is that area: its target takes both
// figures, a creature 8 off the area's, one 40 off nothing; a target spell
// with an effect of its own scope stays a target's (the control).
func TestATargetSpellOfAreaEffectsIsAnArea(t *testing.T) {
	noResist(t)
	storm := wbSpell(9841, 60, 1, "Test Maelstrom", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"radius":25,
		"effects":[{"id":"08ED","type":"Dmg/Heal","kind":"hp","amount":-846,"resist":"lr","scope":"target_aoe"},
		{"id":"08ED","type":"Dmg/Heal","kind":"hp2","amount":-360,"resist":"lr","scope":"target_aoe"}]}`)
	plain := wbSpell(9842, 60, 1, "Test Bolt", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"radius":25,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-846,"resist":"lr"},{"id":"0001","type":"Dmg/Heal","kind":"hp2","amount":-360,"resist":"lr"}]}`)
	for _, c := range []struct {
		spell      CastSpell
		near, area bool
	}{{storm, true, true}, {plain, false, false}} {
		k := newCaster(t)
		k.p.Level = 60
		k.npc.HP, k.npc.MaxHP, k.npc.Level = 50000, 50000, 55
		k.w.SpatialIndex.Insert(k.npc.ServerID, k.npc.X, k.npc.Z)
		mk := func(id EntityID, dx float32) *NPC {
			n := &NPC{ServerID: id, Name: "Test Crab", Level: 55, HP: 50000, MaxHP: 50000, X: k.npc.X + dx, Y: k.npc.Y, Z: k.npc.Z}
			k.w.NPCs[id] = n
			k.w.SpatialIndex.Insert(id, n.X, n.Z)
			return n
		}
		near, far := mk(7201, 8), mk(7202, 40)
		if got := c.spell.Area.Centre != AreaNone; got != c.area {
			t.Fatalf("%s: an area %v, want %v", c.spell.Name, got, c.area)
		}
		k.landSeen(t, c.spell, k.npcO)
		tookT, tookN, tookF := 50000-k.npc.HP, 50000-near.HP, 50000-far.HP
		if tookT < 1206 || (tookN > 0) != c.near || tookF != 0 || c.near && tookN >= tookT {
			t.Errorf("%s: the target took %d, 8 off %d, 40 off %d", c.spell.Name, tookT, tookN, tookF)
		}
	}
}

// A cure of up to three (Restorative: the user, "make it cure up to 3
// effects") takes three of four harmful effects, in the order they came, and
// leaves the newest; one of no count takes all four (the control).
func TestACureOfUpToThree(t *testing.T) {
	for _, k := range []struct {
		js   string
		left string
	}{
		{`{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"","cure_max":3}]}`, "Test D"},
		{`{"v":1,"scope":"self","effects":[{"kind":"cure","removes":""}]}`, ""},
	} {
		c := newCaster(t)
		until := c.at(time.Hour)
		for i, n := range []string{"Test A", "Test B", "Test C", "Test D"} {
			c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: i + 1, Name: n, Until: until, Rules: EffectRules{Harm: true, Resist: StatAR}})
		}
		cure := wbSpell(974, 16, 0, "Test Restorative", k.js)
		c.castOn(t, cure, 0)
		if got := strings.Join(c.effectNames(), ","); got != k.left {
			t.Errorf("%s: left %q, want %q", k.js, got, k.left)
		}
	}
}

// A spell for a target with a part for the area round its caster (the
// warrior's Taunting Shout: 100 hate on its target, 20 on every creature
// within 10 of the warrior): the target takes both, a creature 6 from the
// warrior the area's part, one 30 off nothing.
func TestAShoutsAreaPartIsRoundTheWarrior(t *testing.T) {
	noResist(t)
	shout := wbSpell(9851, 5, 1, "Test Shout", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":5,"radius":10,
		"effects":[{"id":"07BB","type":"Modify Hate","kind":"hate","amount":20,"scope":"player_aoe"},
		{"id":"07BC","type":"Modify Hate","kind":"hate","amount":100}]}`)
	if len(shout.Def.Unsupported) != 0 || shout.Area.Centre != AreaOnCaster {
		t.Fatalf("read with %v left over, an area of %v", shout.Def.Unsupported, shout.Area.Centre)
	}
	k := newCaster(t)
	k.npc.HP, k.npc.MaxHP, k.npc.Level = 5000, 5000, 5
	k.w.SpatialIndex.Insert(k.npc.ServerID, k.npc.X, k.npc.Z)
	mk := func(id EntityID, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: "Test Crab", Level: 5, HP: 5000, MaxHP: 5000, X: k.p.X + dx, Y: k.p.Y, Z: k.p.Z}
		k.w.NPCs[id] = n
		k.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	near, far := mk(7301, -6), mk(7302, -30)
	k.landSeen(t, shout, k.npcO)
	tT, tN, tF := k.npc.Threats[k.p.ServerID], near.Threats[k.p.ServerID], far.Threats[k.p.ServerID]
	if tT < 120 || tN < 20 || tN >= tT || tF != 0 {
		t.Errorf("hate: the target %d, 6 from the warrior %d, 30 off %d", tT, tN, tF)
	}
}

// A warrior's stance (a self spell raising stats with the workbook's cure of
// "ID?") ends the warrior's other stances, and no longer takes a harmful
// effect off; a cure of "ID?" with no stats (the monk's Purity) still takes
// every harmful one (the control).
func TestAStanceEndsTheOtherStances(t *testing.T) {
	stance := func(id int, name, stat string) CastSpell {
		sp := wbSpell(id, 20, 0, name, `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":600,"stack":2,"level":20,
			"effects":[{"kind":"cure","removes":"ID?"},{"kind":"stat","stat":"`+stat+`","amount":50}]}`)
		sp.Recast = 0
		return sp
	}
	defense, force := stance(9861, "Test Defense", "ac"), stance(9862, "Test Force", "str")
	if !defense.Def.Stance || defense.Def.Removes[0] != "stance" {
		t.Fatalf("read as a stance %v, its cure %q", defense.Def.Stance, defense.Def.Removes)
	}
	c := newCaster(t)
	c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: 1, Name: "Test Venom", Until: c.at(time.Hour), Rules: EffectRules{Harm: true, Resist: StatPR}})
	c.castOn(t, defense, 0)
	c.castOn(t, force, 10*time.Second)
	if got := strings.Join(c.effectNames(), ","); got != "Test Venom,Test Force" {
		t.Errorf("after two stances: %s, want the venom kept and the second stance alone", got)
	}
	purity := wbSpell(9863, 20, 0, "Test Purity", `{"v":1,"scope":"self","effects":[{"kind":"cure","removes":"ID?"}]}`)
	if purity.Def.Stance {
		t.Fatal("a cure with no stats read as a stance")
	}
	c.castOn(t, purity, 20*time.Second)
	if got := strings.Join(c.effectNames(), ","); got != "Test Force" {
		t.Errorf("after the control's cure: %s, want the stance alone", got)
	}
}

// A run speed buff whose cure is the workbook's "ID?" (Spirit of the Wolf)
// ends its bearer's other speed buffs and leaves its harm (the user: "yes
// make them end other speed buffs").
func TestASpeedBuffsIDCureEndsOtherSpeedBuffs(t *testing.T) {
	wolf := wbSpell(9871, 8, 0, "Test Wolf Spirit", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":2,"level":8,
		"effects":[{"kind":"cure","removes":"ID?"},{"kind":"speed","amount":30}]}`)
	if len(wolf.Def.Removes) != 1 || wolf.Def.Removes[0] != "speed" || wolf.Def.Stance {
		t.Fatalf("its cure %q, a stance %v", wolf.Def.Removes, wolf.Def.Stance)
	}
	c := newCaster(t)
	until := c.at(time.Hour)
	c.p.Effects = append(c.p.Effects,
		&ActiveEffect{SpellID: 1, Name: "Test Venom", Until: until, Rules: EffectRules{Harm: true, Resist: StatPR}},
		&ActiveEffect{SpellID: 2, Name: "Test Haste", Until: until, Speed: true})
	c.castOn(t, wolf, 0)
	if got := strings.Join(c.effectNames(), ","); got != "Test Venom,Test Wolf Spirit" {
		t.Errorf("after the wolf: %s, want the venom kept, the haste ended", got)
	}
}

// The paladin's Take a Shield Break (a cure of "ID?" by its name) ends the
// paladin's barriers, an effect that mitigates melee, and leaves a harmful
// effect and a resist buff of a barrier's name (the controls).
func TestAShieldBreakEndsTheBarriers(t *testing.T) {
	brk := wbSpell(9881, 1, 0, "Take a Shield Break", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":1,
		"effects":[{"kind":"cure","removes":"ID?","scale":"str","scale_pct":40}]}`)
	if len(brk.Def.Removes) != 1 || brk.Def.Removes[0] != "mitigation" {
		t.Fatalf("its cure %q", brk.Def.Removes)
	}
	c := newCaster(t)
	until := c.at(time.Hour)
	c.p.Effects = append(c.p.Effects,
		&ActiveEffect{SpellID: 1, Name: "Test Venom", Until: until, Rules: EffectRules{Harm: true, Resist: StatPR}},
		&ActiveEffect{SpellID: 2, Name: "Test Granite Barrier", Until: until, Root: true, Rules: EffectRules{MitigatePct: 50, MitigateLeft: 6000, MitigateCapped: true}},
		&ActiveEffect{SpellID: 3, Name: "Test Elemental Barrier", Until: until, Mods: StatMods{FR: 45}})
	c.castOn(t, brk, 0)
	if got := strings.Join(c.effectNames(), ","); got != "Test Venom,Test Elemental Barrier" {
		t.Errorf("after the break: %s, want the barrier ended alone", got)
	}
}

// A backstab (the workbook's backstab effect, or a line's "via backstab")
// needs its caster behind its target: refused in front of a creature that
// faces it, landing from behind; a plain strike from in front lands (the
// control). A creature swinging at someone turns to face it.
func TestABackstabNeedsItsCasterBehind(t *testing.T) {
	stab := wbSpell(9901, 12, 1, "Test Backstab", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":12,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"backstab","amount":-100}]}`)
	line := ParseSpellText("Target, Decrease HP via Backstab by 500 ; Increase 200% based on DEX")
	if !stab.Def.Backstab || !line.Backstab {
		t.Fatalf("read as backstabs: workbook %v, line %v", stab.Def.Backstab, line.Backstab)
	}
	plain := wbSpell(9902, 12, 1, "Test Strike", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":12,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-100}]}`)
	for _, c := range []struct {
		spell  CastSpell
		facing float32 // the creature's, as seen against the caster to its -X
		begins bool
	}{
		{stab, float32(3 * math.Pi / 2), false}, // facing the caster
		{stab, float32(math.Pi / 2), true},      // facing away
		{plain, float32(3 * math.Pi / 2), true},
	} {
		k := newCaster(t)
		k.p.Class, k.p.Level = 6, 30
		k.npc.HP, k.npc.MaxHP, k.npc.Level = 5000, 5000, 10
		k.npc.X, k.npc.Z = k.p.X+2, k.p.Z
		k.npc.Facing = c.facing
		if got := k.w.StartCast(k.sess, c.spell, k.npcO, k.at(0)); got != c.begins {
			t.Errorf("%s with the creature facing %.2f: began %v, want %v", c.spell.Name, c.facing, got, c.begins)
		}
	}
	k := newCaster(t)
	k.npc.X, k.npc.Z, k.npc.Facing = k.p.X+2, k.p.Z, 0
	k.w.mu.Lock()
	k.w.broadcastNPCSwingLocked(k.npc, k.p.ServerID, 0, 0)
	k.w.mu.Unlock()
	if behind(k.npc, k.p.X, k.p.Z) {
		t.Errorf("after swinging at the caster the creature faces %.2f: the caster still behind it", k.npc.Facing)
	}
}

// Above level 50 a player below a spell's lowest level is not affected, and one at
// it is (the control). Up to level 50 there is no lowest level (petition 1130, the GM
// Jibrish: "All level 50 or below buffs should work on all players below level 50,
// including level 1").
func TestWorkbookLowestLevel(t *testing.T) {
	c := newCaster(t)
	for _, tc := range []struct {
		spell, level int
		takes        bool
	}{{51, 19, false}, {51, 20, true}, {50, 19, true}, {50, 1, true}} {
		buff := wbSpell(971, tc.spell, 1, "Test Ward", `{"v":1,"scope":"target","target_type":"Friendly","min_level":20,"duration_s":600,
		"effects":[{"kind":"stat","stat":"ac","amount":10}]}`)
		other := &Player{ServerID: 1002, Name: "Other", Level: tc.level, HP: 100, MaxHP: 100}
		c.w.Players[other.ServerID] = other
		c.w.mu.Lock()
		c.w.landOnPlayerLocked(&castState{spell: buff, caster: c.p.ServerID}, c.p, c.sess, other, c.at(0))
		c.w.mu.Unlock()
		if got := len(other.Effects) == 1; got != tc.takes {
			t.Errorf("a level %d spell on level %d: takes %v, want %v", tc.spell, tc.level, got, tc.takes)
		}
	}
}

// A workbook slow slows by its percent, a line's snare by half (the
// control); a buff's own slow on a player slows its run.
func TestWorkbookSlows(t *testing.T) {
	now := time.Now()
	npc := &NPC{Effects: []*ActiveEffect{{Until: now.Add(time.Minute), Snare: true, Rules: EffectRules{SpeedPct: -30}}}}
	if f := npc.snareFactor(now); f < 0.69 || f > 0.71 {
		t.Errorf("a workbook slow: %v, want 0.7", f)
	}
	npc.Effects[0].Rules.SpeedPct = 0
	if f := npc.snareFactor(now); f != 0.5 {
		t.Errorf("a line's snare: %v, want 0.5", f)
	}
	// the control: a line's slow by a figure still halves the pace
	c := newCaster(t)
	lineSlow := strike()
	lineSlow.SpellID, lineSlow.Def = 982, ParseSpellText("Target, Decrease Movement Speed by 30, 30sec")
	if !c.w.StartCast(c.sess, lineSlow, c.npcO, c.at(0)) {
		t.Fatal("the line's slow not begun")
	}
	c.tick(lineSlow.CastTime + castLand)
	if f := c.npc.snareFactor(c.at(lineSlow.CastTime + castLand)); f != 0.5 {
		t.Errorf("a line's slow: %v, want 0.5", f)
	}
	c = newCaster(t)
	wall := wbSpell(981, 5, 0, "Test Bulwark", `{"v":1,"scope":"self","duration_s":45,"effects":[{"kind":"stat","stat":"ac","amount":50},
		{"kind":"speed","amount":-30}]}`)
	before := c.p.SpeedNow()
	c.castOn(t, wall, 0)
	if got := c.p.SpeedNow(); got < before*0.69 || got > before*0.71 || c.p.Mods.AC != 50 {
		t.Errorf("under its own slow: %v of %v, AC %d", got, before, c.p.Mods.AC)
	}
}

// A workbook effect's rules are kept over a logout.
func TestWorkbookRulesKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.p.Effects = []*ActiveEffect{{SpellID: 7, Name: "Test Shade", Until: c.at(time.Minute), Invis: true,
		Rules: EffectRules{Hide: true, DispelOnContact: true, MitigatePct: 20, MitigateLeft: 50, MitigateCapped: true}}}
	rows := savedEffects(c.p, c.at(0))
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(0)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if r := c.p.Effects[0].Rules; !r.Hide || !r.DispelOnContact || r.MitigatePct != 20 || r.MitigateLeft != 50 || !r.MitigateCapped {
		t.Errorf("its rules after the login: %+v", r)
	}
}

// A defensive proc heal heals its bearer at a hit on it; an offensive one
// not then (the control).
func TestWorkbookProcHeals(t *testing.T) {
	noResist(t)
	c := newCaster(t)
	c.p.HP = 300
	c.p.Effects = []*ActiveEffect{
		{SpellID: 8, Name: "Test Bloom", Until: c.at(time.Minute), Rules: EffectRules{ProcHeal: 25, ProcHealDef: true, ProcHealChance: 100}},
		{SpellID: 9, Name: "Test Vigor", Until: c.at(time.Minute), Rules: EffectRules{ProcHeal: 40}},
	}
	procIntn = func(int) int { return 0 }
	c.w.mu.Lock()
	c.w.procHealsLocked(c.p, nil, true, c.at(0))
	c.w.mu.Unlock()
	if c.p.HP != 325 {
		t.Errorf("after a hit on it: %d HP, want 325", c.p.HP)
	}
}

// The undead check on the workbook's spells (2026-11-01): "Enemy - Undead"
// (the control), a harmful spell named for the undead that the workbook calls
// "Enemy" (a paladin's Disrupt Undead), and one whose line says "Target
// undead" (Punish Death) are for undead alone; a friendly spell its name and
// its line give the undead (a paladin's Smite Undead, a proc on a friend) and
// a harmful spell with neither are for anyone.
func TestWorkbookUndeadCheck(t *testing.T) {
	nuke := func(tt string) string {
		return `{"v":1,"scope":"target","target_type":"` + tt + `","harmful":true,
			"effects":[{"id":"0003","kind":"hp","amount":-140,"resist":"ar"}]}`
	}
	friend := `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":300,
		"effects":[{"id":"0004","kind":"stat","stat":"str","amount":10}]}`
	for _, c := range []struct {
		name, line, js string
		undead         bool
	}{
		{"Test Death", "", nuke("Enemy - Undead"), true},
		{"Test Disrupt Undead", "", nuke("Enemy"), true},
		{"Test Punishment", "Target undead, Decrease Hitpoints by 118", nuke("Enemy"), true},
		{"Test Bolt", "Target, Decrease Hitpoints by 118", nuke("Enemy"), false},
		{"Test Smite Undead", "Target undead, Decrease HP by 32, 5min", friend, false},
	} {
		d := SpellDefFrom(c.name, c.line, "", c.js)
		if !d.Workbook || d.undeadOnly() != c.undead || d.UndeadOnly != c.undead {
			t.Errorf("%s: undead only %v (field %v), want %v", c.name, d.undeadOnly(), d.UndeadOnly, c.undead)
		}
	}
}

// Avatar's shape as the follow-up writes it (the user, 2026-11-01: "a one
// time max hp buff and massive stat buff lasts until next avatar recast"):
// a heal of the workbook's HP1 once, at the landing, after the maximum it
// raises (so a target at full health gains it all), the stats and the
// maximum for the duration, and no tick; a recast takes the first one's
// place. Made-up figures.
func TestAOneTimeHealWithAMaxHPBuff(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 536, 536
	sp := wbSpell(9100, 20, 1, "Test Colossus", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":120,"stack":1,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"str","amount":40},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300}]}`)
	sp.Recast = 0
	c.castOn(t, sp, 0)
	if c.p.MaxHPNow() != 836 || c.p.HP != 836 || c.p.Mods.Str != 40 {
		t.Fatalf("at its landing: %d/%d HP, STR +%d; want 836/836, +40", c.p.HP, c.p.MaxHPNow(), c.p.Mods.Str)
	}
	if len(c.p.Effects) != 1 || c.p.Effects[0].HPTick != 0 {
		t.Fatalf("what lasts: %+v", c.p.Effects)
	}
	c.p.HP = 500
	c.tick(30 * time.Second)
	if c.p.HP != 500 {
		t.Errorf("30 s on, %d HP: it healed again", c.p.HP)
	}
	c.castOn(t, sp, 60*time.Second) // recast: its place taken, healed once more
	if len(c.p.Effects) != 1 || c.p.MaxHPNow() != 836 || c.p.HP != 800 {
		t.Errorf("recast: %d effects, %d/%d HP; want 1, 800/836", len(c.p.Effects), c.p.HP, c.p.MaxHPNow())
	}
}

// Nested lines (the user, 2026-11-08: "the stronger pr/dr over rides the weaker
// resist buff, but keeps the rest of the resist buffs from the resist buff"): a
// cleric's all-resist buff (+55 to all six) and its PR/DR buff (+65), one stack
// type, are both on in either order, PR and DR at the stronger 65 once (not 120)
// and the other four at 55; the PR/DR buff ending leaves the resist buff's 55
// to all. A PR/DR buff under a resist buff stronger in both (+85) is refused,
// its caster told; a resist buff stronger in both over a PR/DR buff takes it
// off. The control: two buffs sharing a stat with neither within the other
// (+STR +DEX, +STR +AGI) are one line, the higher rank taking the lower's place.
func TestNestedResistLines(t *testing.T) {
	noResist(t)
	buff := func(id, level int, amount int, stats string) CastSpell {
		var parts []string
		for _, s := range strings.Split(stats, ",") {
			parts = append(parts, `{"kind":"stat","stat":"`+s+`","amount":`+itoa(amount)+`}`)
		}
		js := `{"v":1,"scope":"self","stack":1,"level":` + itoa(level) + `,"duration_s":600,"effects":[` + strings.Join(parts, ",") + `]}`
		return wbSpell(id, level, 0, "Test Ward "+itoa(id), js)
	}
	resist := buff(921, 49, 55, "fr,cr,lr,pr,dr,ar")
	prdr := buff(922, 54, 65, "pr,dr")
	bigResist := buff(923, 60, 85, "fr,cr,lr,pr,dr,ar")
	want := func(c *caster, label string, fr, pr int, names ...string) {
		t.Helper()
		got := c.effectNames()
		if strings.Join(got, ",") != strings.Join(names, ",") {
			t.Errorf("%s: effects %v, want %v", label, got, names)
		}
		m := c.p.Mods
		if m.FR != fr || m.CR != fr || m.LR != fr || m.AR != fr || m.PR != pr || m.DR != pr {
			t.Errorf("%s: FR %d CR %d LR %d AR %d PR %d DR %d, want %d and %d", label, m.FR, m.CR, m.LR, m.AR, m.PR, m.DR, fr, pr)
		}
	}
	c := newCaster(t)
	c.castOn(t, resist, 0)
	c.castOn(t, prdr, 10*time.Second)
	want(c, "the resist buff, then the PR/DR buff", 55, 65, resist.Name, prdr.Name)
	c.w.mu.Lock()
	c.w.removeEffectLocked(c.p, prdr.SpellID)
	c.w.mu.Unlock()
	want(c, "the PR/DR buff ended", 55, 55, resist.Name)

	c = newCaster(t)
	c.castOn(t, prdr, 0)
	c.castOn(t, resist, 10*time.Second)
	want(c, "the PR/DR buff, then the resist buff", 55, 65, prdr.Name, resist.Name)

	c = newCaster(t)
	c.castOn(t, bigResist, 0)
	c.castOn(t, prdr, 10*time.Second)
	want(c, "a PR/DR buff under a stronger resist buff", 85, 85, bigResist.Name)
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], bigResist.Name+", which is stronger") {
		t.Errorf("the caster was not told: %q", red)
	}

	c = newCaster(t)
	c.castOn(t, prdr, 0)
	c.castOn(t, bigResist, 10*time.Second)
	want(c, "a stronger resist buff over a PR/DR buff", 85, 85, bigResist.Name)

	// the control: sharing STR, neither within the other
	strDex, strAgi := buff(924, 20, 30, "str,dex"), buff(925, 30, 30, "str,agi")
	c = newCaster(t)
	c.castOn(t, strDex, 0)
	c.castOn(t, strAgi, 10*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != strAgi.Name || c.p.Mods.Dex != 0 || c.p.Mods.Str != 30 {
		t.Errorf("the control: effects %v, STR %d DEX %d; want the higher alone", got, c.p.Mods.Str, c.p.Mods.Dex)
	}
}

// Petition 1359: Graal's Final Strike ("stack2 player proc", level 60, a 500
// proc) and Stormblade ("stack2 player +INT proc", level 49, a 300 proc) are
// nested lines that both carry a proc: the higher rank wins in either order.
// Stormblade over Graal's is refused, its caster told; Graal's over
// Stormblade takes it off. Of one rank, the bigger proc wins (Graal's Final
// Strike over Graal's Venomus Strike, both 60).
func TestNestedProcLinesGoByRank(t *testing.T) {
	noResist(t)
	graal := wbSpell(931, 60, 0, "Graal's Final Strike", `{"v":1,"scope":"self","stack":2,"level":60,"duration_s":300,`+
		`"effects":[{"kind":"proc","procs":[{"kind":"hp","amount":-500,"scale":"str","scale_pct":10,"on":"offense","scope":"target"}]}]}`)
	storm := wbSpell(932, 49, 0, "Stormblade", `{"v":1,"scope":"self","stack":2,"level":49,"duration_s":300,`+
		`"effects":[{"kind":"stat","stat":"int","amount":100},`+
		`{"kind":"proc","procs":[{"kind":"hp","amount":-300,"scale":"str","scale_pct":10,"on":"offense","scope":"target"}]}]}`)
	venom := wbSpell(933, 60, 0, "Graal's Venomus Strike", `{"v":1,"scope":"self","stack":2,"level":60,"duration_s":300,`+
		`"effects":[{"kind":"stat","stat":"int","amount":175},`+
		`{"kind":"proc","procs":[{"kind":"hp","amount":-300,"scale":"str","scale_pct":10,"on":"offense","scope":"target"}]}]}`)
	if g, s := BuffLine(&graal), BuffLine(&storm); !nestedLines(g, s) {
		t.Fatalf("lines %q and %q are not nested: the test reads nothing", g, s)
	}
	want := func(c *caster, label string, names ...string) {
		t.Helper()
		if got := c.effectNames(); strings.Join(got, ",") != strings.Join(names, ",") {
			t.Errorf("%s: effects %v, want %v", label, got, names)
		}
	}

	c := newCaster(t)
	c.castOn(t, graal, 0)
	c.castOn(t, storm, 10*time.Second)
	want(c, "Stormblade over Graal's", graal.Name)
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], graal.Name+", which is stronger") {
		t.Errorf("the caster was not told: %q", red)
	}
	if c.p.Mods.Int != 0 {
		t.Errorf("INT %d with Stormblade refused, want 0", c.p.Mods.Int)
	}

	c = newCaster(t)
	c.castOn(t, storm, 0)
	c.castOn(t, graal, 10*time.Second)
	want(c, "Graal's over Stormblade", graal.Name)
	if c.p.Mods.Int != 0 {
		t.Errorf("INT %d after Stormblade was taken off, want 0", c.p.Mods.Int)
	}

	// one rank: the bigger proc, either order
	c = newCaster(t)
	c.castOn(t, venom, 0)
	c.castOn(t, graal, 10*time.Second)
	want(c, "Final Strike over Venomus Strike", graal.Name)
	c = newCaster(t)
	c.castOn(t, graal, 0)
	c.castOn(t, venom, 10*time.Second)
	want(c, "Venomus Strike over Final Strike", graal.Name)
}

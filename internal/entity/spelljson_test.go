package entity

import (
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
	d = SpellDefFrom("Test Shape", "", "", `{"v":1,"scope":"self","duration_s":60,"effects":[{"kind":"illusion","sprite":"0x12345678"}]}`)
	if d.FormKind != FormSelf || d.FormModel != 0x12345678 || d.Duration != time.Minute {
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
	if len(d.Effects) != 3 || d.Effects[0].Kind != EffectDefProc || d.Effects[0].Amount != 8 || d.Effects[0].Chance != 100 ||
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
// at the proc's chance (35% for one ten levels above the caster): resisted,
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
	}{{0, false, false, true}, {34, false, false, true}, {35, true, false, false}, {0, true, true, false}} {
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
		if !tc.landed && c.npc.CurrentEnemy != c.p.ServerID {
			t.Errorf("roll %d: resisted, the creature is not angered (%d)", tc.roll, c.npc.CurrentEnemy)
		}
	}
	if got := npcSpellResistPct(23, &NPC{Level: 33}, StatFR, time.Now()); got != 35 {
		t.Errorf("ten above: %d%%, want 35", got)
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

// Stack types: two buffs of one stack type sharing a stat are one line, the
// higher level's taking the lower's place and a lower refused under it;
// another stack type, or no stat shared, stacks (the control).
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
	low, high := buff(911, 10, 2, "ac,str"), buff(912, 20, 2, "ac")
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

// What ends on a cast ends when its bearer begins one; what ends on contact
// ends at a creature's hit; the rest stays (the control). (A hide is
// invisibility, which any cast ends besides: invis.go.)
func TestWorkbookDispels(t *testing.T) {
	noResist(t)
	quick := wbSpell(931, 5, 0, "Test Haste", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"speed","amount":130,"dispel_on_cast":true}]}`)
	shade := wbSpell(932, 5, 0, "Test Shade", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"hide","dispel_on_contact":true}]}`)
	skin := wbSpell(933, 5, 0, "Test Skin", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"stat","stat":"ac","amount":10}]}`)
	c := newCaster(t)
	c.castOn(t, quick, 0)
	c.castOn(t, skin, 10*time.Second) // a cast ends the haste
	c.castOn(t, shade, 20*time.Second)
	if got := c.effectNames(); len(got) != 2 || got[0] != skin.Name || got[1] != shade.Name {
		t.Fatalf("after a cast: %v, want the skin and the shade", got)
	}
	if !c.p.Invisible() {
		t.Error("hidden, the player is not unseen")
	}
	c.w.mu.Lock()
	c.w.npcAttackPlayer(c.npc, c.p.ServerID, c.at(30*time.Second))
	c.w.mu.Unlock()
	if got := c.effectNames(); len(got) != 1 || got[0] != skin.Name {
		t.Errorf("after a hit: %v, want the skin alone", got)
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
		if got := mitigateLocked(c.p, 100, now); got != want {
			t.Errorf("hit %d: %d taken, want %d", i+1, got, want)
		}
	}
	cloak := wbSpell(942, 5, 0, "Test Cloak", `{"v":1,"scope":"self","duration_s":30,"effects":[{"kind":"mitigation","mitigation":{"pct":10,"max":100000}}]}`)
	c = newCaster(t)
	c.castOn(t, cloak, 0)
	for i := 0; i < 50; i++ {
		if got := mitigateLocked(c.p, 1000, c.at(5*time.Second)); got != 900 {
			t.Fatalf("hit %d: %d taken, want 900", i+1, got)
		}
	}
	if got := mitigateLocked(c.p, 1000, c.at(time.Minute)); got != 1000 {
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

// A player below a spell's lowest level is not affected; one at it is (the
// control).
func TestWorkbookLowestLevel(t *testing.T) {
	c := newCaster(t)
	buff := wbSpell(971, 30, 1, "Test Ward", `{"v":1,"scope":"target","target_type":"Friendly","min_level":20,"duration_s":600,
		"effects":[{"kind":"stat","stat":"ac","amount":10}]}`)
	for _, tc := range []struct {
		level int
		takes bool
	}{{19, false}, {20, true}} {
		other := &Player{ServerID: 1002, Name: "Other", Level: tc.level, HP: 100, MaxHP: 100}
		c.w.Players[other.ServerID] = other
		c.w.mu.Lock()
		c.w.landOnPlayerLocked(&castState{spell: buff, caster: c.p.ServerID}, c.p, c.sess, other, c.at(0))
		c.w.mu.Unlock()
		if got := len(other.Effects) == 1; got != tc.takes {
			t.Errorf("level %d: takes %v, want %v", tc.level, got, tc.takes)
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
	c.w.procHealsLocked(c.p, true, c.at(0))
	c.w.mu.Unlock()
	if c.p.HP != 325 {
		t.Errorf("after a hit on it: %d HP, want 325", c.p.HP)
	}
}

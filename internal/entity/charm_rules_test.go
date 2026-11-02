package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The live game's charm rules as eqoa.fandom.com has them (charm.go's doc;
// the user, 2026-11-02, picked each).

// The caps: SOE's notes for Allure and Entrancing Allure (53, over Entrancing
// Allure's words' 54), a charm's words (Binding Grasp's 58, the bard's 54),
// read on the workbook's path too (until 2026-11-02 Binding Grasp took up to
// 60), else two under its level, three for a druid's animal charm.
func TestTheCharmCaps(t *testing.T) {
	grasp := `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":18000,"level":60,` +
		`"sheet_type":"Charm","effects":[{"id":"02EB","type":"Charm","kind":"charm","scale":"cha","scale_pct":100}]}`
	animal := `{"v":1,"scope":"target","target_type":"Enemy - Animal","harmful":true,"duration_s":120,"level":30,` +
		`"sheet_type":"Charm","effects":[{"id":"01A8","type":"Charm","kind":"charm","scale":"cha","scale_pct":25,"resist":"ar"}]}`
	allure := `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":360,"level":55,` +
		`"sheet_type":"Charm","effects":[{"id":"02DF","type":"Charm","kind":"charm","scale":"cha","scale_pct":25,"resist":"ar"}]}`
	for _, c := range []struct {
		name          string
		level         int
		line, desc, j string
		want          int
	}{
		{"Binding Grasp", 60, "Target, Charm ; Increase 100% based on CHA, 300min",
			"Target, Charm ; Increase 25% based on CHA, 3600sec, Charms up to level 58 mobs", grasp, 58},
		{"Entrancing Song", 51, "Target, Charm ; Increase 25% based on CHA, 30min", "Charm up to level 54. Duration: 45 min", "", 54},
		{"Entrancing Allure", 55, "Target, Charm ; Increase 25% based on CHA, 6min",
			"Target, Charm ; Increase 25% based on CHA, 360sec, Charms up to level 54 mobs", allure, 53},
		{"Allure", 55, "Target, Charm ; Increase 25% based on CHA, 3min", "Target, Charm ; Increase 25% based on CHA, 3min", "", 53},
		{"Entrancing Captivation", 39, "Target, Charm ; Increase 25% based on CHA, 6min", "Charms an enemy to aid you.", "", 37},
		{"Entice", 4, "Target, Charm ; Increase 25% based on CHA, 2min", "", "", 2},
		{"Charm Animal", 30, "Target, Charm ; Increase 25% based on CHA, 2min", "Charm ; Increase 25% based on CHA, 2min", animal, 27},
	} {
		s := CastSpell{Name: c.name, Level: c.level, Def: SpellDefFrom(c.name, c.line, c.desc, c.j)}
		if !s.Def.Charms() {
			t.Fatalf("%s: not read as a charm", c.name)
		}
		if got := charmMaxLevel(&s); got != c.want {
			t.Errorf("%s (level %d): cap %d, want %d", c.name, c.level, got, c.want)
		}
	}
}

// The CHA share of the time: "25% of your CHA is added to the duration of
// the spell, meaning that 600 CHA would add an extra 150 seconds".
func TestACharmsTimeTakesItsCastersCHA(t *testing.T) {
	s := entrancing("Captivate", 39, "Target, Charm ; Increase 25% based on CHA, 3min")
	for _, c := range []struct {
		cha  int
		want time.Duration
	}{{0, 3 * time.Minute}, {600, 3*time.Minute + 150*time.Second}, {150, 3*time.Minute + 37500*time.Millisecond}} {
		if got := charmDuration(&s, c.cha); got != c.want {
			t.Errorf("CHA %d: %v, want %v", c.cha, got, c.want)
		}
	}
	if got := charmDuration(&CastSpell{Def: ParseSpell("Target, Charm ; Increase 25% based on CHA", "")}, 600); got != 0 {
		t.Errorf("a charm of no time held %v, want 0 (until it is let go)", got)
	}
}

// npccharm's rows: an uncharmable name refused at the start in red with
// nothing taken (the control: its level alone lets it); a resist of 100 never
// charmed, the creature at its caster; a resist of 50 by the dice.
func TestTheCharmRules(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)

	r, _, _ := charmRig(t)
	r.w.SetCharmRules(map[string]database.CharmRule{"desert crab": {Charmable: false}})
	power := r.p.Power
	began, msgs := castCharm(t, r, captivation(), crabID, 0)
	if began || len(msgs) != 1 || !bytes.Equal(msgs[0], redLine(charmCannot)) || r.p.Power != power || len(r.p.Lockouts) != 0 {
		t.Errorf("an uncharmable crab: began %v, sent %x, power %d (was %d)", began, msgs, r.p.Power, power)
	}
	r.w.SetCharmRules(nil)
	if began, _ := castCharm(t, r, captivation(), crabID, time.Minute); !began || r.enemy.Owner != r.p.ServerID {
		t.Errorf("the control, no rule: began %v, owner %d", began, r.enemy.Owner)
	}

	r2, _, _ := charmRig(t)
	r2.w.SetCharmRules(map[string]database.CharmRule{"desert crab": {Charmable: true, ResistPct: 100}})
	if began, _ := castCharm(t, r2, captivation(), crabID, 0); !began || r2.enemy.Owner != 0 || r2.enemy.CurrentEnemy != r2.p.ServerID {
		t.Errorf("resist 100: began %v, owner %d, enemy %d", began, r2.enemy.Owner, r2.enemy.CurrentEnemy)
	}

	for _, c := range []struct {
		roll int
		took bool
	}{{49, false}, {50, true}} {
		charmIntn = dice(c.roll)
		r3, _, _ := charmRig(t)
		r3.w.SetCharmRules(map[string]database.CharmRule{"desert crab": {Charmable: true, ResistPct: 50}})
		castCharm(t, r3, captivation(), crabID, 0)
		if took := r3.enemy.Owner == r3.p.ServerID; took != c.took {
			t.Errorf("resist 50, a roll of %d: charmed %v, want %v", c.roll, took, c.took)
		}
	}
}

// A charmed creature fights by its npccharm row's template: its nuke cast at
// what it fights (a class's spell of its level, as a summoned pet's), its
// spell's power spent from its own pool; with too little left it melees;
// power its master gives it (an enchanter's Power Boon) lets it cast again;
// an ability marked free costs nothing.
func TestACharmedCreatureFightsByItsTemplate(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	nuke := &CastSpell{SpellID: 777, Level: 30, Scope: 1, Name: "Burst of Flame", CastTime: time.Second, PowerCost: 300,
		Def: ParseSpellText("Target, Decrease Hitpoints by 200")}
	tmpl := &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "charm test", Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100}}
	a := templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityNuke, Every: 6}, every: 6 * time.Second}
	a.byClass[r.enemy.Class] = []*CastSpell{nuke}
	tmpl.abilities = []templateAbility{a}
	r.w.templates.byName = map[string]*ClassTemplate{"charm test": tmpl}
	r.w.SetCharmRules(map[string]database.CharmRule{"desert crab": {Charmable: true, Template: "charm test"}})
	if began, _ := castCharm(t, r, captivation(), crabID, 0); !began || r.enemy.Owner != r.p.ServerID {
		t.Fatalf("not charmed")
	}
	crab := r.enemy
	ps := r.w.pets[r.p.ServerID]
	if ps.tmpl == nil || ps.tmpl.tmpl != tmpl || crab.Level != 34 || crab.MaxHP != 2040 {
		t.Fatalf("its template %v; level %d, max HP %d (its own kept)", ps.tmpl, crab.Level, crab.MaxHP)
	}
	foe := &NPC{ServerID: 3400, Name: "sand giant", Level: 33, HP: 5000, MaxHP: 5000, ModelID: 0x55432169, Size: 1,
		X: crab.X + 2, Y: crab.Y, Z: crab.Z, World: 2}
	r.w.NPCs[foe.ServerID] = foe
	r.w.SpatialIndex.Insert(foe.ServerID, foe.X, foe.Z)

	cast := func(at time.Duration) bool {
		t.Helper()
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		ps.target = foe.ServerID
		n := len(r.w.petCasts)
		r.w.petTemplateTurnLocked(ps, foe, r.at(at))
		return len(r.w.petCasts) > n
	}
	full := crab.Power
	if full != CalculateMaxPower(34, classWarrior, 0, 0, 0, 0, 0, 0, 0) || !cast(10*time.Second) || crab.Power != full-300 {
		t.Fatalf("its first nuke: power %d of %d", crab.Power, full)
	}
	crab.Power = 299
	if cast(20 * time.Second) {
		t.Errorf("cast with 299 power of the 300 it costs")
	}
	boon := CastSpell{SpellID: 900, DisplayIndex: 12, Level: 20, Scope: 1, Name: "Power Boon", CastTime: time.Second, Recast: time.Second,
		PowerCost: 100, Range: 30, Def: ParseSpellText("Target, Increase Power by 100")}
	petID := uint32(crabID)
	if !r.w.StartCast(r.sess, boon, petID, r.at(30*time.Second)) {
		t.Fatal("Power Boon at the pet did not begin")
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(30*time.Second + boon.CastTime + castLand))
	r.w.mu.Unlock()
	if crab.Power != 399 {
		t.Fatalf("Power Boon on the pet: power %d, want 399", crab.Power)
	}
	if !cast(40*time.Second) || crab.Power != 99 {
		t.Errorf("after its master's power: cast with power left %d", crab.Power)
	}
	tmpl.abilities[0].Free = true
	if !cast(50*time.Second) || crab.Power != 99 {
		t.Errorf("a free ability: power %d, want 99 kept", crab.Power)
	}
}

// The charm templates' new pet abilities: a timed special (a rhino's crit)
// on its template's clock as one of its hits, from special_from; and an area
// spell round its target landing on every creature there it may fight.
func TestACharmTemplatesSpecialAndArea(t *testing.T) {
	defer func(f, g func(int) int) { charmIntn, templateIntn = f, g }(charmIntn, templateIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	tmpl := &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "charm rhino", Class: -1, HPPct: 100, TakenPct: 100,
		MeleePct: 100, Special: "Critical Hit", SpecialDamage: 10, SpecialEvery: 30, SpecialFirst: 5}}
	r.w.templates.byName = map[string]*ClassTemplate{"charm rhino": tmpl}
	r.w.SetCharmRules(map[string]database.CharmRule{"desert crab": {Charmable: true, Template: "charm rhino"}})
	castCharm(t, r, captivation(), crabID, 0)
	ps := r.w.pets[r.p.ServerID]
	if ps == nil || ps.tmpl == nil {
		t.Fatal("not charmed by its template")
	}
	foe := &NPC{ServerID: 3400, Name: "sand giant", Level: 33, HP: 5000, MaxHP: 5000, ModelID: 0x55432169, Size: 1,
		X: r.enemy.X + 2, Y: r.enemy.Y, Z: r.enemy.Z, World: 2}
	r.w.NPCs[foe.ServerID] = foe
	special := func(at time.Duration) bool {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		hp := foe.HP
		r.w.petTemplateSpecialLocked(ps, foe, r.at(at))
		return foe.HP < hp
	}
	landed := 0
	for d := 10 * time.Second; d <= 10*time.Second+66*time.Second; d += 100 * time.Millisecond {
		if special(d) {
			landed++
		}
	}
	// at 5 s, 35 s and 65 s of its time at its foe
	if landed != 3 {
		t.Errorf("its crit landed %d times in 66 s at its foe, want 3", landed)
	}
	tmpl.SpecialFrom = 35 // above the crab's 34
	ps.npc.endTemplateFight()
	for d := 2 * time.Minute; d <= 3*time.Minute; d += 100 * time.Millisecond {
		if special(d) {
			t.Fatalf("a crit below its template's special_from")
		}
	}

	// an area spell round its target: the foe and a second creature beside it
	other := &NPC{ServerID: 3401, Name: "sand giant", Level: 33, HP: 5000, MaxHP: 5000, ModelID: 0x55432169, Size: 1,
		X: foe.X + 3, Y: foe.Y, Z: foe.Z, World: 2}
	r.w.NPCs[other.ServerID] = other
	r.w.SpatialIndex.Insert(foe.ServerID, foe.X, foe.Z)
	r.w.SpatialIndex.Insert(other.ServerID, other.X, other.Z)
	quake := &CastSpell{SpellID: 778, Level: 30, Scope: 1, Name: "Tremor", CastTime: time.Second, PowerCost: 100,
		Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	quake.Area = SpellArea{Centre: AreaOnTarget, Radius: 10}
	a := templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityAoE, Every: 15}, every: 15 * time.Second}
	a.byClass[ps.npc.Class] = []*CastSpell{quake}
	tmpl.abilities = []templateAbility{a}
	tmpl.SpecialDamage = 0
	ps.npc.endTemplateFight()
	fh, oh := foe.HP, other.HP
	r.w.mu.Lock()
	ps.target = foe.ServerID
	if !r.w.petCastLocked(ps, 0, foe, r.at(5*time.Minute)) {
		r.w.mu.Unlock()
		t.Fatal("the area spell was not cast")
	}
	r.w.petCastSystemLocked(r.at(5*time.Minute + quake.CastTime + castLand))
	r.w.mu.Unlock()
	if foe.HP >= fh || other.HP >= oh {
		t.Errorf("the area spell: foe %d -> %d, the one beside it %d -> %d", fh, foe.HP, oh, other.HP)
	}
}

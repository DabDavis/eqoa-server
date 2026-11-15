package entity

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/formula"
)

// gameDice makes the game's own draws (the melee rolls' meleeIntn and
// formulaRand, math/rand's global source in the server) one seeded source.
func gameDice(t *testing.T, seed int64) {
	t.Helper()
	oldIntn, oldRand := meleeIntn, formulaRand
	t.Cleanup(func() { meleeIntn, formulaRand = oldIntn, oldRand })
	g := rand.New(rand.NewSource(seed))
	meleeIntn = g.Intn
	formulaRand = formula.RandSource{Float64: g.Float64, NormFloat64: g.NormFloat64}
}

// A made-up formula that draws every kind of draw.
const drawingFormulas = `formulas:
  npc_melee: 'base * (0.5 + uniform()) + normal(0, 1) * 0 + beta(2, 3) * 0'
  player_melee: 'base + pick_index(1, 1, 1)'
`

// With a fixed seed of the game's, the game's draws come out the same with
// shadow on as off: a shadowed formula draws from its own source.
func TestShadowLeavesTheGameRollsAlone(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	writeFormulas(t, r.w, drawingFormulas)
	seq := func() []int {
		gameDice(t, 42)
		var out []int
		for range 25 {
			p.HP, p.MaxHP = 100_000, 100_000
			guard.HP, guard.MaxHP, guard.Dead = 1_000_000, 1_000_000, false
			r.w.mu.Lock()
			r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
			r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
			r.w.mu.Unlock()
			out = append(out, 100_000-p.HP, 1_000_000-guard.HP, meleeIntn(1_000_000))
		}
		return out
	}
	r.w.SetDamageFormulaMode(FormulaOff)
	off := seq()
	path := shadowOn(t, r.w, 0)
	SeedShadowRand(7)
	on := seq()
	for i := range off {
		if off[i] != on[i] {
			t.Fatalf("roll %d: %d with shadow off, %d on", i, off[i], on[i])
		}
	}
	recs := readShadow(t, r.w, path)
	if len(recs) != 50 {
		t.Fatalf("%d records, want 50", len(recs))
	}
	if recs[0].Draws != 3 {
		t.Errorf("the swing's formula made %d draws, want 3", recs[0].Draws)
	}

	// the same shadow seed gives the same shadow figures again
	again := func() []int {
		SeedShadowRand(7)
		gameDice(t, 1)
		var out []int
		for range 5 {
			out = append(out, r.w.formulaDamage(100, dmgIn{src: srcNPC, kind: dkMelee, an: guard, dp: p}))
		}
		r.w.FlushDamageFormulaShadow()
		return out
	}
	again()
	n := len(readShadow(t, r.w, path))
	again()
	all := readShadow(t, r.w, path)
	for i := range 5 {
		if a, b := all[n-5+i].Value, all[n+i].Value; a != b {
			t.Errorf("seed 7, run %d: %d then %d", i, a, b)
		}
	}
}

// A creature's melee on a player behind a capped mitigation: the record
// holds the figure before the mitigation, the one that landed, and the
// formula's through the same mitigation as it stood before the hit, and the
// preview spends nothing of it.
func TestShadowStageMitigation(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, _ := r.player("Hero", guard.X+1, guard.Z)
	writeFormulas(t, r.w, "formulas:\n  npc_melee: 'base_pre * 2'\n")
	path := shadowOn(t, r.w, 0)
	fixDice(t)
	ward := &ActiveEffect{SpellID: 9901, Name: "Toy Ward", Until: time.Now().Add(time.Hour),
		Rules: EffectRules{MitigatePct: 50, MitigateLeft: 1_000_000, MitigateCapped: true}}
	p.Effects = []*ActiveEffect{ward}
	p.HP, p.MaxHP = 100_000, 100_000
	r.w.mu.Lock()
	r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
	r.w.mu.Unlock()
	landed := 100_000 - p.HP
	spent := 1_000_000 - ward.Rules.MitigateLeft
	rec := readShadow(t, r.w, path)[0]
	pre := landed + spent
	if pre <= 1 || rec.Builtin != landed || rec.BuiltinPre != pre || rec.Mitigated != spent || rec.Stage != "mitigation" {
		t.Fatalf("the record: %+v (landed %d, spent %d)", rec, landed, spent)
	}
	if rec.Value != 2*pre || rec.Preview != 2*pre-2*pre*50/100 || rec.In["base_pre"] != float64(pre) {
		t.Errorf("the formula's figure %d, its preview %d: %+v", rec.Value, rec.Preview, rec)
	}

	// a capped ward with little left: the preview holds to what it had before the hit
	ward.Rules.MitigateLeft = 3
	p.HP = 100_000
	r.w.mu.Lock()
	r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
	r.w.mu.Unlock()
	recs := readShadow(t, r.w, path)
	rec = recs[len(recs)-1]
	if rec.Mitigated != 3 || rec.Preview != rec.Value-3 || rec.Builtin != rec.BuiltinPre-3 {
		t.Errorf("a ward with 3 left: %+v", rec)
	}
	if p.hasEffect(9901) {
		t.Error("the spent ward stayed on")
	}
}

// dmgStage's preview: for a formula before the stage, a mitigation then a
// carrier's wounds; after it, the formula's figure as it is. Nothing changes
// on the stage.
func TestDmgStagePreview(t *testing.T) {
	s := dmgStage{on: true, formulaBefore: true, pre: 100, mit: mitSnap{on: true, pct: 40, capped: true, left: 25}, ctfPct: 10}
	if got := s.preview(200); got != ctfWounded(200-25, 10) {
		t.Errorf("preview %d", got)
	}
	if got := s.preview(200); got != 193 || s.mit.left != 25 {
		t.Errorf("a second preview %d, left %d", got, s.mit.left)
	}
	after := s
	after.formulaBefore = false
	if got := after.preview(200); got != 200 {
		t.Errorf("a formula after the stage: %d", got)
	}
	if s.steps() != "mitigation+ctf" || (&dmgStage{}).steps() != "" || (&dmgStage{on: true}).steps() != "none" {
		t.Errorf("steps %q", s.steps())
	}
	if got := (&dmgStage{}).preview(55); got != 55 {
		t.Errorf("no stage: %d", got)
	}
	// a path with no stage: builtin_pre is builtin, the preview the formula's figure
	w := NewWorld()
	writeFormulas(t, w, "formulas:\n  player_spell: 'base_pre + 3'\n")
	path := shadowOn(t, w, 0)
	w.formulaDamage(10, dmgIn{src: srcPlayer, kind: dkSpell, ap: &Player{Name: "Hero"}, dn: &NPC{Name: "Toy Rat"}})
	if rec := readShadow(t, w, path)[0]; rec.BuiltinPre != 10 || rec.Preview != 13 || rec.Value != 13 || rec.Stage != "" {
		t.Errorf("no stage: %+v", rec)
	}
}

// The newer inputs: the weapon's type, the ranged slot's weapon and the
// shot's arrows, the masteries' and the effects' OFF and DEF, the creature's
// row, the world and instance; d_dummy in every record.
func TestDamageFormulaNewerInputs(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	p := &Player{Name: "Hero", Level: 30, World: 4, Instance: 9}
	p.Gear.WeaponType, p.Gear.RangedDamage, p.Gear.RangedType = 3, 40, 7
	p.PrimaryType = 3 // the main hand's (handType)
	p.Swing.Kind = swingRange4
	p.CMEffects[cmSlotOff], p.CMEffects[cmSlotDef] = 5, 6
	p.Mods.OffMod, p.Mods.DefMod = 1, 2
	n := &NPC{ServerID: 77, Name: "Toy Golem", Level: 33, OffMod: 24, DefMod: 13, World: 4, Instance: 9,
		Effects: []*ActiveEffect{{Mods: StatMods{OffMod: 3, DefMod: -2}}}}
	copyOf := &NPC{ServerID: 9000, Temporary: true, CopyOf: 55, Name: "Toy Golem"}
	cases := []struct {
		expr string
		in   dmgIn
		want int
	}{
		{"weapon_type * 100 + ranged_weapon_dmg + ranged_weapon_type * 1000", dmgIn{ap: p, dn: n}, 7340},
		{"projectiles", dmgIn{ap: p, dn: n, ranged: true}, 2},
		{"projectiles", dmgIn{ap: p, dn: n}, 0},
		{"a_cm_offmod * 10 + a_cm_defmod", dmgIn{ap: p, dn: n}, 56},
		{"a_effect_offmod * 10 + a_effect_defmod", dmgIn{ap: p, dn: n}, 12},
		{"d_effect_offmod * 10 + d_effect_defmod + 100", dmgIn{ap: p, dn: n}, 128},
		{"d_offmod * 100 + d_defmod", dmgIn{ap: p, dn: n}, 2413},
		{"d_cm_offmod + a_npc_row + d_npc_row", dmgIn{ap: p, dn: n}, 77},
		{"d_npc_row", dmgIn{ap: p, dn: copyOf}, 55},
		{"world * 100 + instance", dmgIn{ap: p, dn: n}, 409},
		{"a_npc_row + d_cm_offmod * 10 + d_cm_defmod + world", dmgIn{src: srcNPC, an: n, dp: p}, 77 + 56 + 4},
		{"base_pre", dmgIn{ap: p, dn: n}, 1},
	}
	for _, c := range cases {
		writeFormulas(t, w, "formulas:\n  player_melee: '"+c.expr+"'\n  npc_melee: '"+c.expr+"'\n")
		in := c.in
		in.kind = dkMelee
		if got := w.formulaDamage(1, in); got != c.want {
			t.Errorf("%s = %d, want %d", c.expr, got, c.want)
		}
	}

	writeFormulas(t, w, "formulas:\n  player_melee: base\n")
	path := shadowOn(t, w, 0)
	w.formulaDamage(5, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: &NPC{ServerID: 12, Name: "Toy Dummy", Dummy: NotDummy + 1}})
	w.formulaDamage(5, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: n})
	recs := readShadow(t, w, path)
	if len(recs) != 2 || recs[0].In["d_dummy"] != float64(NotDummy+1) || recs[1].In["d_dummy"] != 0.0 {
		t.Fatalf("d_dummy: %+v", recs)
	}
	for _, k := range []string{"weapon_type", "d_npc_row", "world", "instance", "a_cm_offmod", "d_effect_defmod", "projectiles", "ranged_weapon_dmg", "base_pre"} {
		if _, ok := recs[1].In[k]; !ok {
			t.Errorf("the record lacks %s", k)
		}
	}
	if recs[1].In["d_npc_row"] != 77.0 || recs[1].WeapType != 3 || recs[1].In["weapon_type"] != 3.0 {
		t.Errorf("the record: %+v", recs[1])
	}
}

// An older file whose step is named as a newer input still loads and gives
// what it did; a step named as an older input is still refused.
func TestDamageFormulaOlderStepNames(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, `formulas:
  player_melee:
    steps:
      - world: 'base * 2'
      - projectiles: 'world + 1'
    result: projectiles + weapon_dmg
`)
	if _, names := w.DamageFormulaStatus(); len(names) != 1 {
		t.Fatal("the older file did not load")
	}
	p := &Player{Name: "Hero", World: 4}
	if got := w.formulaDamage(10, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: &NPC{Name: "Toy Rat"}}); got != 21 {
		t.Errorf("the older file gave %d, want 21", got)
	}
	_, _, errs := formula.ParseFileYield([]byte("formulas:\n  player_melee:\n    steps:\n      - base: 1\n    result: base\n"),
		knownDamageFormula, dmgVarIndex, dmgVarsYield)
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "an input's") {
		t.Errorf("a step named base: %v", errs)
	}
	_, _, errs = formula.ParseFileYield([]byte("formulas:\n  player_melee:\n    steps:\n      - world: 1\n      - world: 2\n    result: world\n"),
		knownDamageFormula, dmgVarIndex, dmgVarsYield)
	if len(errs) == 0 {
		t.Error("two steps of one name loaded")
	}
}

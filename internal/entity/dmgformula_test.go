package entity

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/formula"
)

// writeFormulas writes a formula file in a test's own directory and names it
// the world's.
func writeFormulas(t *testing.T, w *World, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "damage.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	w.SetDamageFormulaFile(path)
	return path
}

func toyFormulas(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/toy-damage.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fixDice makes the melee rolls and the formulas' draws a test's.
func fixDice(t *testing.T, draws ...float64) *formula.Forced {
	t.Helper()
	oldIntn, oldRand := meleeIntn, formulaRand
	t.Cleanup(func() { meleeIntn, formulaRand = oldIntn, oldRand })
	meleeIntn = func(n int) int { return n / 2 }
	f := &formula.Forced{Draws: draws}
	formulaRand = f
	return f
}

// With no formula file the built-in figures stand: a player's melee hit lands
// what it did, a formula of base alone changes nothing, the toy model's
// figure lands while it is loaded, and the built-in is back once the file is
// unset.
func TestDamageFormulaReplacesPlayerMelee(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	p.Class = 0 // the toy's "Warrior" profile
	hit := func() int {
		guard.HP, guard.MaxHP, guard.Dead = 1_000_000, 1_000_000, false
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
		return 1_000_000 - guard.HP
	}
	fixDice(t)
	builtIn := hit()
	if builtIn <= 0 || hit() != builtIn {
		t.Fatalf("the built-in hit is not fixed: %d", builtIn)
	}

	writeFormulas(t, r.w, "formulas:\n  player_melee: base\n")
	if got := hit(); got != builtIn {
		t.Errorf("player_melee: base landed %d, the built-in %d", got, builtIn)
	}

	writeFormulas(t, r.w, toyFormulas(t))
	f := fixDice(t, 0.9) // pick's draw: the second of two equal slots
	if got, want := hit(), builtIn*3+100+50; got != want {
		t.Errorf("the toy model landed %d, want %d", got, want)
	}
	if f.Next != 1 {
		t.Errorf("the hit made %d draws, want 1", f.Next)
	}
	p.Class = 13 // no profile of its own: the default's
	fixDice(t, 0.1)
	if got, want := hit(), builtIn+0+50; got != want {
		t.Errorf("the default profile landed %d, want %d", got, want)
	}

	r.w.SetDamageFormulaFile("")
	fixDice(t)
	if got := hit(); got != builtIn {
		t.Errorf("with the file unset %d landed, the built-in %d", got, builtIn)
	}
}

// A creature's melee takes the source's formula (npc), and its damage shield
// its own (npc_shield) over the source's.
func TestDamageFormulaCreaturePaths(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, hp_pct, shield_damage) VALUES ('thorns', 0, 100, 500)`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll guard", "thorns")
	guard := r.npc("Gnoll Guard")
	guard.HP, guard.MaxHP = 1_000_000, 1_000_000
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	swing := func() int {
		p.HP, p.MaxHP = 100_000, 100_000
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
		return 100_000 - p.HP
	}
	shield := func() int {
		p.HP, p.MaxHP = 100_000, 100_000
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.creatureShieldLocked(sess, p, guard, time.Now())
		return 100_000 - p.HP
	}
	fixDice(t)
	builtIn, builtShield := swing(), shield()
	if builtShield != 500 {
		t.Fatalf("the built-in shield: %d", builtShield)
	}
	writeFormulas(t, r.w, toyFormulas(t))
	if got := swing(); got != builtIn+7 {
		t.Errorf("npc's formula on a swing: %d, want %d", got, builtIn+7)
	}
	if got := shield(); got != 3 {
		t.Errorf("npc_shield: %d, want 3", got)
	}
}

// A formula's unusable result falls back to the built-in figure for that hit,
// logged once in a while, naming the formula and never its text; the other
// paths are untouched.
func TestDamageFormulaFallsBack(t *testing.T) {
	w := NewWorld()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	writeFormulas(t, w, "formulas:\n  player_melee: base - 31337\n  player_spell: base / 0\n  player_dot: missing_step_zz\n")
	if !strings.Contains(buf.String(), "mistake") {
		t.Fatalf("an unknown name was not refused: %q", buf.String())
	}
	if _, names := w.DamageFormulaStatus(); len(names) != 0 {
		t.Fatalf("a refused file loaded %v", names)
	}
	writeFormulas(t, w, "formulas:\n  player_melee: base - 31337\n  player_spell: base / 0\n  player_proc: 'lookup(\"t\", 1).zz'\ntables:\n  t: {a: {b: 1}}\n")
	in := func(k dmgKind) *dmgIn {
		return &dmgIn{src: srcPlayer, kind: k, ap: &Player{Level: 10}, dn: &NPC{Level: 10}}
	}
	for range 5 {
		if got := w.formulaDamage(100, *in(dkMelee)); got != 100 {
			t.Errorf("a negative result landed %d", got)
		}
	}
	if got := w.formulaDamage(100, *in(dkSpell)); got != 100 {
		t.Errorf("an infinite result landed %d", got)
	}
	if got := w.formulaDamage(100, *in(dkProc)); got != 100 {
		t.Errorf("a missing field landed %d", got)
	}
	if got := w.formulaDamage(100, *in(dkDoT)); got != 100 {
		t.Errorf("a path with no formula landed %d", got)
	}
	out := buf.String()
	if n := strings.Count(out, "Damage formula player_melee:"); n != 1 {
		t.Errorf("player_melee's failure logged %d times in a minute, want 1:\n%s", n, out)
	}
	for _, secret := range []string{"31337", "base -", "/ 0", "zz", "missing_step"} {
		if strings.Contains(out, secret) {
			t.Errorf("the log quotes a formula (%q):\n%s", secret, out)
		}
	}
}

// A reload refuses a broken file whole and keeps what was loaded; a file
// that is gone leaves the built-in figures; the watcher's look sees a change.
func TestDamageFormulaReload(t *testing.T) {
	w := NewWorld()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	path := writeFormulas(t, w, "formulas:\n  player: base * 2\n")
	in := &dmgIn{src: srcPlayer, kind: dkSpell, ap: &Player{}, dn: &NPC{}}
	if got := w.formulaDamage(10, *in); got != 20 {
		t.Fatalf("player covers player_spell: %d", got)
	}
	if w.damageFormulaChanged() {
		t.Error("an unchanged file seen as changed")
	}
	if err := os.WriteFile(path, []byte("formulas:\n  player: base * (2 +\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.damageFormulaChanged() {
		t.Error("a rewritten file not seen")
	}
	names, err := w.ReloadDamageFormulas()
	if err == nil || names != nil || strings.Contains(err.Error(), "base") {
		t.Errorf("a broken reload: %v %v", names, err)
	}
	if got := w.formulaDamage(10, *in); got != 20 {
		t.Errorf("after a broken reload the old formula should stand: %d", got)
	}
	if err := os.WriteFile(path, []byte("formulas:\n  player_spell: base * 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if names, err := w.ReloadDamageFormulas(); err != nil || len(names) != 1 || names[0] != "player_spell" {
		t.Errorf("a good reload: %v %v", names, err)
	}
	if got := w.formulaDamage(10, *in); got != 50 {
		t.Errorf("the new formula: %d", got)
	}
	os.Remove(path)
	if !w.damageFormulaChanged() {
		t.Error("a removed file not seen")
	}
	if _, err := w.ReloadDamageFormulas(); err == nil {
		t.Error("a missing file reloaded")
	}
	if got := w.formulaDamage(10, *in); got != 10 {
		t.Errorf("with the file gone: %d, want the built-in", got)
	}
	if strings.Contains(buf.String(), "base *") {
		t.Errorf("the log quotes a formula:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "Damage formulas loaded: 1 (player_spell)") {
		t.Errorf("the load's line:\n%s", buf.String())
	}
}

// The inputs: what a formula sees of each side.
func TestDamageFormulaInputs(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	p := &Player{Name: "Hero", Level: 30, Class: 9, Str: 100, Sta: 1, Agi: 1, Dex: 1, Wis: 1, Int: 1, Cha: 1, HP: 5}
	p.Mods.OffMod = 2
	n := &NPC{Name: "Toy Golem", Level: 33, Class: 1, HP: 7, MaxHP: 9, DefMod: -1}
	cases := map[string]float64{
		`a_level + a_eff_level / 100`: 30 + 32.0/100,
		`d_level + d_eff_level / 100`: 33 + 32.0/100,
		`con`:                         3,
		`a_str`:                       100,
		`d_hp + d_maxhp * 10`:         97,
		`(a_class_name == "Cleric") + (a_kind == "player") * 2 + (d_kind == "creature") * 4 + (d_name == "Toy Golem") * 8`: 15,
		`raw + kind * 1000 + (mode == "spell") * 10000`:                                                                    4 + 2000 + 10000,
		`pvp + duel + by_pet + ranged + spell_id`:                                                                          0,
	}
	for expr, want := range cases {
		writeFormulas(t, w, "formulas:\n  player_spell: '"+expr+"'\n")
		if _, names := w.DamageFormulaStatus(); len(names) != 1 {
			t.Fatalf("%s did not load", expr)
		}
		got := w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkSpell, ap: p, dn: n, raw: 4})
		if float64(got) != float64(int(want+0.5)) {
			t.Errorf("%s = %d, want %v", expr, got, want)
		}
	}
}

// The doc names every formula and every input.
func TestDamageFormulaDocCoversEveryName(t *testing.T) {
	b, err := os.ReadFile("../../docs/damage-formulas.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, n := range append(DamageFormulaNames(), DamageFormulaVars()...) {
		if !strings.Contains(doc, "`"+n+"`") {
			t.Errorf("docs/damage-formulas.md does not name `%s`", n)
		}
	}
}

// One hit's cost with a formula loaded (the toy: two lookups with fallback,
// a weighted pick, arithmetic), and with a beta draw added; and with none.
func BenchmarkDamageFormula(b *testing.B) {
	p := &Player{Name: "Hero", Level: 30, Class: 0}
	n := &NPC{Name: "Toy Golem", Level: 33}
	run := func(b *testing.B, text string) {
		w := NewWorld()
		if text != "" {
			path := filepath.Join(b.TempDir(), "d.yaml")
			os.WriteFile(path, []byte(text), 0o600)
			log.SetOutput(new(bytes.Buffer))
			w.SetDamageFormulaFile(path)
			log.SetOutput(os.Stderr)
			if _, names := w.DamageFormulaStatus(); len(names) == 0 {
				b.Fatal("did not load")
			}
		}
		b.ReportAllocs()
		for b.Loop() {
			w.formulaDamage(100, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: n, raw: 90})
		}
	}
	toy, _ := os.ReadFile("testdata/toy-damage.yaml")
	b.Run("none", func(b *testing.B) { run(b, "") })
	b.Run("toy", func(b *testing.B) { run(b, string(toy)) })
	b.Run("toy+beta", func(b *testing.B) {
		run(b, strings.Replace(string(toy), `result: base * prof.scale`, `result: base * (0.5 + beta(2, 5)) * prof.scale`, 1))
	})
}

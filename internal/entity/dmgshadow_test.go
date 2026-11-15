package entity

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shadowOn turns shadow mode on with a log in a test's own directory, and
// returns the log's path.
func shadowOn(t *testing.T, w *World, maxBytes int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", "shadow.jsonl")
	if err := w.SetDamageFormulaShadow(ShadowLogConfig{Path: path, MaxBytes: maxBytes}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.SetDamageFormulaShadow(ShadowLogConfig{}) })
	w.SetDamageFormulaMode(FormulaShadow)
	return path
}

func readShadow(t *testing.T, w *World, path string) []ShadowRecord {
	t.Helper()
	if err := w.FlushDamageFormulaShadow(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []ShadowRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r ShadowRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("a line that is no record: %q", sc.Text())
		}
		out = append(out, r)
	}
	return out
}

// Shadow mode never changes the damage that lands: a creature's swing on a
// player and a player's on a creature land the built-in figure with a made-up
// formula loaded, the log holding each pair with its inputs; off lands the
// built-in too and records nothing; live lands the formula's.
func TestShadowModeNeverChangesAppliedDamage(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	p.Class = 0
	swing := func() int {
		p.HP, p.MaxHP = 100_000, 100_000
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
		return 100_000 - p.HP
	}
	hit := func() int {
		guard.HP, guard.MaxHP, guard.Dead = 1_000_000, 1_000_000, false
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
		return 1_000_000 - guard.HP
	}
	fixDice(t)
	builtSwing, builtHit := swing(), hit()

	writeFormulas(t, r.w, toyFormulas(t)) // npc: base + 7; player_melee: the toy's profile
	r.w.SetDamageFormulaMode(FormulaOff)
	fixDice(t)
	if got := swing(); got != builtSwing {
		t.Errorf("off: a swing landed %d, the built-in %d", got, builtSwing)
	}

	path := shadowOn(t, r.w, 0)
	f := fixDice(t, 0.9, 0.9)
	if got := swing(); got != builtSwing {
		t.Errorf("shadow: a swing landed %d, the built-in %d", got, builtSwing)
	}
	if got := hit(); got != builtHit {
		t.Errorf("shadow: a hit landed %d, the built-in %d", got, builtHit)
	}
	if f.Next != 1 {
		t.Errorf("the formulas made %d draws, want 1 (the toy's pick)", f.Next)
	}
	recs := readShadow(t, r.w, path)
	if len(recs) != 2 {
		t.Fatalf("%d records, want 2: %+v", len(recs), recs)
	}
	sw, ht := recs[0], recs[1]
	if sw.Path != "npc_melee" || sw.Formula != "npc" || sw.Builtin != builtSwing || sw.Value != builtSwing+7 || sw.Draws != 0 {
		t.Errorf("the swing's record: %+v", sw)
	}
	if sw.In["a_name"] != "Toy Golem" || sw.In["d_name"] != "Hero" || sw.In["a_kind"] != "creature" || sw.In["d_kind"] != "player" {
		t.Errorf("the swing's inputs: %v", sw.In)
	}
	for _, k := range []string{"a_level", "d_level", "d_ac", "d_agi", "a_class_name", "d_class_name", "weapon_dmg", "a_family", "base"} {
		if _, ok := sw.In[k]; !ok {
			t.Errorf("the swing's record lacks %s: %v", k, sw.In)
		}
	}
	if ht.Path != "player_melee" || ht.Builtin != builtHit || ht.Value != builtHit*3+100+50 || ht.Draws != 1 {
		t.Errorf("the hit's record: %+v (built-in %d)", ht, builtHit)
	}

	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the log's mode: %v %v", fi.Mode(), err)
	}
	if di, err := os.Stat(filepath.Dir(path)); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("its directory's mode: %v %v", di.Mode(), err)
	}
	b, _ := os.ReadFile(path)
	for _, secret := range []string{"base + 7", "lookup", "prof.scale"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the log quotes a formula (%q)", secret)
		}
	}

	r.w.SetDamageFormulaMode(FormulaLive)
	fixDice(t)
	if got := swing(); got != builtSwing+7 {
		t.Errorf("live: a swing landed %d, want the formula's %d", got, builtSwing+7)
	}
	if n := len(readShadow(t, r.w, path)); n != 2 {
		t.Errorf("live recorded %d records, want none more", n)
	}
}

// A formula's shadow_ steps are recorded as its tags; a failed run is
// recorded with its part and base kept; a mitigation's share is recorded.
func TestShadowTagsAndFailures(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, `formulas:
  npc_melee:
    steps:
      - shadow_key: 'a_name == "Toy Golem" ? "golem" : ""'
      - shadow_scale: 'd_level / 10'
      - plain: 3
    result: base * 2
  npc_spell: base - 999999
`)
	path := shadowOn(t, w, 0)
	in := dmgIn{src: srcNPC, kind: dkMelee, an: &NPC{Name: "Toy Golem", Level: 20}, dp: &Player{Name: "Hero", Level: 30}, raw: 40, mitigated: 5}
	if got := w.formulaDamage(50, in); got != 50 {
		t.Errorf("shadow landed %d", got)
	}
	in.kind = dkSpell
	if got := w.formulaDamage(50, in); got != 50 {
		t.Errorf("shadow landed %d on a failed run", got)
	}
	recs := readShadow(t, w, path)
	if len(recs) != 2 {
		t.Fatalf("%d records", len(recs))
	}
	if r := recs[0]; r.Value != 100 || r.Tags["key"] != "golem" || r.Tags["scale"] != 3.0 || r.Tags["plain"] != nil || r.Mitigated != 5 || r.Raw != 40 {
		t.Errorf("the record: %+v", r)
	}
	if r := recs[1]; r.Fail == "" || r.Value != 50 || r.Path != "npc_spell" || strings.Contains(r.Fail, "999999") {
		t.Errorf("a failed run's record: %+v", r)
	}
}

// The summary: counts, the figures by path and group, the ratio, the largest
// differences, the rotated file read too; and nothing of a formula in it.
func TestShadowSummary(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, "formulas:\n  npc_melee: 'a_level > 20 ? base * 2 : base'\n  player_melee: base + 1\n")
	path := shadowOn(t, w, 2000) // small: it rotates
	for i := range 40 {
		lvl := 10
		if i%2 == 1 {
			lvl = 30
		}
		w.formulaDamage(100, dmgIn{src: srcNPC, kind: dkMelee, an: &NPC{Name: "Toy Golem", Level: lvl}, dp: &Player{Name: "Hero", Level: 30}})
	}
	w.formulaDamage(10, dmgIn{src: srcPlayer, kind: dkMelee, ap: &Player{Name: "Hero", Level: 5}, dn: &NPC{Name: "Toy Rat", Level: 4}})
	if err := w.FlushDamageFormulaShadow(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("no rotation at 2000 bytes: %v", err)
	}
	s, err := SummarizeShadowLog(path, ShadowSummaryOptions{Top: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 2 || s.Lines == 0 || s.Bad != 0 {
		t.Fatalf("files %v, lines %d, bad %d", s.Files, s.Lines, s.Bad)
	}
	byKey := map[string]*ShadowStats{}
	for _, g := range append(s.Groups, s.ByPath...) {
		byKey[g.Key] = g
	}
	hi := byKey["npc_melee / creature / L21-30"]
	lo := byKey["npc_melee / creature / L1-10"]
	if hi == nil || lo == nil {
		t.Fatalf("groups: %v", s.Groups)
	}
	// a hit the formula returns base for and draws nothing on is not covered
	if hi.Covered != hi.Hits || hi.Ratio != 2 || hi.FormMed != 200 || hi.BuiltP90 != 100 || lo.Covered != 0 {
		t.Errorf("the groups: hi %+v lo %+v", hi, lo)
	}
	if pm := byKey["player_melee"]; pm == nil || pm.Hits != 1 || pm.Ratio != 1.1 {
		t.Errorf("player_melee: %+v", pm)
	}
	if len(s.Largest) != 3 || s.Largest[0].Diff != 100 || !strings.Contains(s.Largest[0].Attacker, "Toy Golem") {
		t.Errorf("the largest: %+v", s.Largest)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "base * 2") || strings.Contains(string(b), "a_level >") {
		t.Errorf("the summary quotes a formula: %s", b)
	}
	only, err := SummarizeShadowLog(path, ShadowSummaryOptions{Path: "player_melee"})
	if err != nil || only.Lines != 1 {
		t.Errorf("one path: %+v %v", only, err)
	}
}

// The mode's words.
func TestParseFormulaMode(t *testing.T) {
	for in, want := range map[string]FormulaMode{"": FormulaOff, "off": FormulaOff, "Live": FormulaLive, "shadow": FormulaShadow} {
		if got, err := ParseFormulaMode(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseFormulaMode("jibs-ish"); err == nil {
		t.Error("an unknown mode was taken")
	}
}

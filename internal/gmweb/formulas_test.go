package gmweb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/formulamode"
)

// reload_formulas is an admin's: it reads the damage formula file again and
// answers with the names in use, refuses a broken file and keeps what was in
// use, and never shows or audits a formula's text.
func TestReloadFormulas(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "damage.formula")
	if err := os.WriteFile(path, []byte("formulas:\n  player_melee: base * 31337\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.world.SetDamageFormulaFile(path)

	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("reload_formulas", map[string]any{}); code != 403 {
		t.Errorf("a GM reloaded the formulas: %d", code)
	}

	admin := f.browser()
	admin.login("boss")
	if err := os.WriteFile(path, []byte("formulas:\n  npc_melee: base * 31337\n  pet: base + 31337\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := admin.op("reload_formulas", map[string]any{})
	if code != 200 || fmt.Sprint(out["formulas"]) != "[npc_melee pet]" {
		t.Fatalf("reload: %d %v", code, out)
	}
	if err := os.WriteFile(path, []byte("formulas:\n  npc_melee: base * (31337 +\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out = admin.op("reload_formulas", map[string]any{})
	if code != 400 || !strings.Contains(fmt.Sprint(out), "line 2") {
		t.Errorf("a broken file: %d %v, want refused with its line", code, out)
	}
	if _, names := f.world.DamageFormulaStatus(); fmt.Sprint(names) != "[npc_melee pet]" {
		t.Errorf("after a refused reload: %v, want the formulas in use kept", names)
	}

	var audit strings.Builder
	rows, err := f.db.Query(`SELECT detail FROM gm_audit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		rows.Scan(&d)
		audit.WriteString(d + "\n")
	}
	if !strings.Contains(audit.String(), "damage formulas reloaded: 2 in use (npc_melee, pet)") {
		t.Errorf("the audit: %q", audit.String())
	}
	if strings.Contains(audit.String()+fmt.Sprint(out), "31337") {
		t.Errorf("a formula's text was shown or audited: %q %v", audit.String(), out)
	}
}

// The formula mode on the panel: any GM reads each shard's mode, an admin
// alone switches it (refused with no formula loaded), and the shadow
// summary is an admin's, figures alone.
func TestFormulaModesAndShadowSummary(t *testing.T) {
	f := newFixture(t)
	ctl, err := formulamode.New(f.db, f.world, "heirloom", nil, entity.FormulaOff)
	if err != nil {
		t.Fatal(err)
	}
	f.s.d.Formula = ctl

	gm := f.browser()
	gm.login("gm")
	if code, out := gm.op("formula_modes", map[string]any{}); code != 200 || !strings.Contains(fmt.Sprint(out), "heirloom") {
		t.Fatalf("a GM's view: %d %v", code, out)
	}
	if code, _ := gm.op("set_formula_mode", map[string]any{"mode": "jibs"}); code != 403 {
		t.Errorf("a GM switched the mode: %d", code)
	}
	if code, _ := gm.op("formula_shadow_summary", map[string]any{}); code != 403 {
		t.Errorf("a GM read the shadow summary: %d", code)
	}

	admin := f.browser()
	admin.login("boss")
	if code, out := admin.op("set_formula_mode", map[string]any{"mode": "jibs"}); code != 400 || !strings.Contains(fmt.Sprint(out), "no formula loaded") {
		t.Errorf("jibs with no formula: %d %v", code, out)
	}
	path := filepath.Join(t.TempDir(), "damage.formula")
	os.WriteFile(path, []byte("formulas:\n  npc_melee: base * 3\n"), 0o600) // made up
	f.world.SetDamageFormulaFile(path)
	if code, out := admin.op("formula_shadow_summary", map[string]any{}); code != 400 || !strings.Contains(fmt.Sprint(out), "no shadow log") {
		t.Errorf("a summary with no log: %d %v", code, out)
	}
	logPath := filepath.Join(t.TempDir(), "private", "shadow.jsonl")
	if err := f.world.SetDamageFormulaShadow(entity.ShadowLogConfig{Path: logPath}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.world.SetDamageFormulaShadow(entity.ShadowLogConfig{}) })
	code, out := admin.op("set_formula_mode", map[string]any{"shard": "heirloom", "mode": "shadow"})
	if code != 200 || f.world.DamageFormulaMode() != entity.FormulaShadow {
		t.Fatalf("shadow: %d %v (mode %v)", code, out, f.world.DamageFormulaMode())
	}
	// a creature's hit as the shadow log records it (entity's own tests make them from hits)
	line, _ := json.Marshal(entity.ShadowRecord{At: time.Now().UTC().Format(time.RFC3339Nano), Path: "npc_melee", Formula: "npc_melee",
		Builtin: 40, Value: 120, In: map[string]any{"a_kind": "creature", "a_name": "Toy Golem", "a_level": 12.0, "d_kind": "player", "d_level": 12.0}})
	fh, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fh.Write(append(line, '\n'))
	fh.Close()
	code, out = admin.op("formula_shadow_summary", map[string]any{})
	if code != 200 || !strings.Contains(fmt.Sprint(out), "npc_melee") || !strings.Contains(fmt.Sprint(out), "ratio:3") {
		t.Errorf("the summary: %d %v", code, out)
	}
	if strings.Contains(fmt.Sprint(out), "base * 3") {
		t.Errorf("the summary quotes the formula: %v", out)
	}
}

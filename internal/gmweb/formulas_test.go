package gmweb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

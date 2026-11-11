package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Mr. Moogie's template on a real database after tools/moogie_laststand_20261111.py --apply: his nine
// abilities read with no problem, the last stand among them; at 2% with a mercenary-like NPC (an NPC alive on his
// hate list) he stands, healed whole, his melee doubled; with players alone (the control) he does not. Skipped
// without EQOA_REAL_DB, or with a database whose Mr. Moogie has no last stand (the tool not applied).
func TestRealMoogieStandsAgainstANonPlayer(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "moogie.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, `"mr moogie"`) {
			t.Errorf("a problem of his template: %s", p)
		}
	}
	var moogie, other *NPC
	for _, n := range w.NPCs {
		switch {
		case n.Name == "Mr. Moogie" && n.tmpl != nil:
			moogie = n
		case other == nil && n.Name != "Mr. Moogie" && n.World == 0 && !n.Dead:
			other = n
		}
	}
	if moogie == nil {
		t.Skip("no Mr. Moogie with a template here")
	}
	if moogie.tmpl.lastStand == nil {
		t.Skip("his template has no last stand: tools/moogie_laststand_20261111.py not applied")
	}
	if n := len(moogie.tmpl.Abilities); n != 9 {
		t.Errorf("%d abilities, want 9", n)
	}
	other.World, other.Dead = moogie.World, false
	now := time.Unix(1_800_000_000, 0)
	w.mu.Lock()
	defer w.mu.Unlock()
	before := moogie.meleeOut(1000)
	// players alone (an id no NPC has)
	moogie.Threats = map[EntityID]int64{EntityID(1): 1000}
	moogie.HP = moogie.MaxHP / 100
	w.lastStandPhaseLocked(moogie, moogie.fight(), now)
	if moogie.HP != moogie.MaxHP/100 {
		t.Fatalf("players alone at 1%%: HP %d, want no last stand", moogie.HP)
	}
	moogie.Threats[other.ServerID] = 500
	w.lastStandPhaseLocked(moogie, moogie.fight(), now)
	if moogie.HP != moogie.MaxHP {
		t.Fatalf("with %s on his list at 1%%: HP %d of %d, want whole", other.Name, moogie.HP, moogie.MaxHP)
	}
	if after := moogie.meleeOut(1000); after != 2*before {
		t.Errorf("his melee of 1000: %d after, %d before; want doubled", after, before)
	}
	t.Logf("Mr. Moogie (HP %d): his melee of 1000 is %d, %d after his last stand; %s", moogie.MaxHP, before, moogie.meleeOut(1000),
		lastStandWords(moogie))
}

package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A real server's database with tools/mermaid_pets_20261113.py applied
// (EQOA_REAL_DB, copied; skipped when its spell 760179 is not there): each of
// the Mermaid's six summonings makes its pet for a level 60 Mermaid of every
// class among the server's own NPCs, of its row's level, its abilities' spells
// all found; the Ascension's pet fights by the magician's water pet template.
// The hit points each class's pet gets (the balance's pet share of its class
// and level, balanceclasslevel) are logged.
func TestRealMermaidPets(t *testing.T) {
	db := simRealDB(t)
	if _, ok, err := db.TemplateSpellByID(760179); err != nil || !ok {
		t.Skipf("spell 760179: there %v, %v: tools/mermaid_pets_20261113.py not applied to this database", ok, err)
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "petstats 7601") {
			t.Errorf("problem: %s", p)
		}
	}
	want := []struct {
		id, level int
		pet       string
	}{{760180, 4, "Water Elementaling"}, {760181, 19, "Water Elementalkin"}, {760182, 26, "Water Elementalkin"},
		{760183, 31, "Water Elemental"}, {760184, 43, "Ghilan"}, {760179, 51, "Ghilan"}}
	now := time.Unix(1_800_000_000, 0)
	for class := 0; class <= 14; class++ {
		p := &Player{ServerID: EntityID(9000 + class), Name: "Merrow", Race: 32, Class: class, Level: 60, HP: 5000, MaxHP: 5000,
			World: 0, X: 5000, Y: 50, Z: 5000}
		w.Players[p.ServerID] = p
		var hps []int
		for _, c := range want {
			ts, ok, err := db.TemplateSpellByID(c.id)
			if err != nil || !ok {
				t.Fatalf("spell %d: %v %v", c.id, ok, err)
			}
			sp := SpellOfRow(&ts.Pattern, 0, ts.Line, ts.Effects, ts.FX)
			w.mu.Lock()
			ps := w.makePetLocked(&sp, p, p, nil, now)
			w.mu.Unlock()
			if ps == nil {
				t.Errorf("class %d: %q (%d) made no pet", class, sp.Name, c.id)
				continue
			}
			if ps.npc.Level != c.level || ps.npc.Name != c.pet || ps.npc.sheet == nil || len(ps.npc.sheet.abilities) == 0 {
				t.Errorf("class %d: %q made %q level %d; want %q level %d, its workbook row", class, sp.Name, ps.npc.Name,
					ps.npc.Level, c.pet, c.level)
			}
			for i, a := range ps.npc.sheet.abilities {
				if a.pinned == nil {
					t.Errorf("%q's ability %d pins no spell found", sp.Name, i+1)
				}
			}
			if c.id == 760179 && (ps.tmpl == nil || !strings.HasPrefix(ps.tmpl.tmpl.Name, "pet water")) {
				t.Errorf("class %d: Aquatic Ascension's pet fights by %v, want pet water", class, ps.tmpl)
			}
			hps = append(hps, ps.npc.MaxHP)
		}
		t.Logf("class %2d at 60: the pets' HP %v", class, hps)
	}
}

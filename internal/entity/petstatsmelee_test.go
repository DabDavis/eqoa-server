package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A workbook pet's melee share is its petstats row's melee_pct (Live petition 5555: the enchanter's animations at
// 200), none of its template's: a row of 200 hits twice as hard, a row of 0 (a table made before the column) or 100
// as its numbers make it, and its template's melee_pct counts for nothing.
func TestAWorkbookPetsMeleeIsItsRows(t *testing.T) {
	base := &petTemplate{tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "pet animation", Class: 12,
		HPPct: 100, TakenPct: 100, MeleePct: 300}}}
	spell := &CastSpell{Name: "Steel Tempest"}
	for _, c := range []struct{ row, want int }{{200, 200}, {100, 100}, {0, 100}} {
		s := &petSheet{PetStats: database.PetStats{SheetPetID: "x", MeleePct: c.row}}
		pt := petSheetTemplate(s, spell, base)
		if pt.tmpl.MeleePct != c.want {
			t.Errorf("row %d: melee_pct %d, want %d", c.row, pt.tmpl.MeleePct, c.want)
		}
		ps := &petState{tmpl: pt}
		if got := ps.meleeOut(100); got != c.want {
			t.Errorf("row %d: a hit of 100 lands as %d", c.row, got)
		}
	}
}

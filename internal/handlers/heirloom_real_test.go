package handlers

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// On a real database with tools/seed_heirlooms.py applied (EQOA_REAL_DB, a
// copy): every class's heirlooms at every level are the best of their role
// the class may wear there, so a piece never gets worse as its wearer
// levels (by #template's measure, a weapon's damage first), no figure is a
// broken record's, and a level 60's chest piece and main weapon carry
// figures. The control is the level 1 piece: as good as the level 60 one only
// where nothing better exists. An epic heirloom (tools/heirloom_epics.py) is
// held to that below its epic's level and to its epic's own figures and
// proc from it (the view's own path); what it gives up at that level to be
// the epic (a level 49 epic against the best of 48) is logged, not failed:
// the user's choice is the epic itself.
func TestRealHeirloomsGrowWithTheirWearer(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defs, err := db.Heirlooms()
	if err != nil || len(defs) == 0 {
		t.Skipf("no heirlooms in %s (%v): apply tools/seed_heirlooms.py to a copy", path, err)
	}
	h := NewHeirlooms(db)
	items := database.NewItemPatternStore(db)
	ids := make([]int, 0, len(defs))
	for id := range defs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var lines []string
	for _, id := range ids {
		d := defs[id]
		base, err := items.GetItemPattern(id)
		if err != nil {
			t.Fatal(err)
		}
		measure := func(p *database.ItemPatternRecord) [2]int {
			if heirloomRoles[d.Role].weapon {
				return [2]int{p.WeaponDamage, templateScore(p)}
			}
			return [2]int{templateScore(p), 0}
		}
		var epic *database.ItemPatternRecord
		if d.Epic != 0 {
			if epic, err = items.GetItemPattern(d.Epic); err != nil || epic == nil {
				t.Fatalf("%s (%d): its epic %d: %v", base.ItemName, id, d.Epic, err)
			}
		}
		var prev [2]int
		var row []string
		for level := 1; level <= 60; level++ {
			race, _ := firstRace(db, d.Class)
			p := *base
			isEpic := epic != nil && level >= max(epic.LevelReq, 1)
			if isEpic {
				heirloomEpicFigures(&p, epic)
			} else if best, ok := h.bestOf(d.Class, race, level, d.Role); ok {
				heirloomFigures(&p, &best)
			}
			m := measure(&p)
			switch {
			case isEpic && level == max(epic.LevelReq, 1):
				if m[0] < prev[0] {
					lines = append(lines, fmt.Sprintf("  %s at %d is its epic %s: %v, the best of %d was %v", base.ItemName, level,
						epic.ItemName, m, level-1, prev))
				}
				if p.WeaponDamage != epic.WeaponDamage || p.ProcName != epic.ItemName {
					t.Errorf("%s at %d: damage %d, proc %q; want its epic's %d, %q", base.ItemName, level, p.WeaponDamage,
						p.ProcName, epic.WeaponDamage, epic.ItemName)
				}
			case m[0] < prev[0] || (m[0] == prev[0] && m[1] < prev[1]):
				t.Errorf("%s (%d, class %d %s) got worse at %d: %v after %v", base.ItemName, id, d.Class, d.Role, level, m, prev)
			}
			prev = m
			if heirloomUnfit(&p) && p.Rent != 0 {
				t.Errorf("%s at %d carries a broken record's figures", base.ItemName, level)
			}
			if level == 1 || level == 10 || level == 30 || level == 60 {
				row = append(row, fmt.Sprintf("%d:%v", level, m[0]))
			}
			if level == 60 && (d.Role == "chest" || d.Role == "main" || d.Role == "twohand") && m[0] == 0 {
				t.Errorf("%s (class %d) has nothing at 60", base.ItemName, d.Class)
			}
		}
		lines = append(lines, fmt.Sprintf("class %2d %-8s %-22s %s", d.Class, d.Role, base.ItemName, strings.Join(row, " ")))
	}
	t.Logf("each heirloom's measure (AC-weighted score, a weapon's damage) at levels 1, 10, 30, 60:\n%s", strings.Join(lines, "\n"))
}

// firstRace is a race the class's start spots hold (a class any race may be
// is race 0).
func firstRace(db *database.SQLiteDB, class int) (int, error) {
	var race int
	err := db.QueryRow(`SELECT race FROM defaultcharacter WHERE tclass = ? ORDER BY race LIMIT 1`, class).Scan(&race)
	return race, err
}

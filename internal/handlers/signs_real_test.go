package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The signs of a real server (EQOA_REAL_DB, a database tools/seed_signs.py
// has seeded; a copy is opened): every NPC on SignBody has words, every row
// is one the panel would take and names an NPC that is there (or is the news
// row), a road sign points two ways at least, a board is a board. Skipped
// without a database, or with signtext empty.
func TestRealSigns(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "signs.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer database.InvalidateSignText()
	database.InvalidateSignText()
	rows, err := db.SignTexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Skip("signtext is empty: tools/seed_signs.py --apply first")
	}
	npcs := map[int]*entity.NPC{}
	q, err := db.Query(`SELECT id, COALESCE(npc_name, ''), COALESCE(modelid, 0) FROM npcs`)
	if err != nil {
		t.Fatal(err)
	}
	for q.Next() {
		var id int
		var name string
		var model int64
		if err := q.Scan(&id, &name, &model); err != nil {
			t.Fatal(err)
		}
		npcs[id] = &entity.NPC{ServerID: entity.EntityID(id), Name: name, ModelID: model}
	}
	q.Close()

	way := regexp.MustCompile(`^(North|East|South|West): \S`)
	kinds := map[string]int{}
	for id, npc := range npcs {
		if uint32(npc.ModelID) != SignBody {
			continue
		}
		row, ok := rows[id]
		if !ok || strings.TrimSpace(SignWords(npc, rows)) == "" {
			t.Errorf("the sign %d %q says nothing", id, npc.Name)
			continue
		}
		kinds[row.Kind]++
		if boardName.MatchString(npc.Name) != (row.Kind == database.SignKindBoard) {
			t.Errorf("%d %q is a row of kind %s", id, npc.Name, row.Kind)
		}
		if row.Kind == database.SignKindRoad || row.Kind == database.SignKindCity {
			lines := strings.Split(row.Text, "\n")
			if len(lines) < 2 {
				t.Errorf("the %s sign %d points one way: %q", row.Kind, id, row.Text)
			}
			for _, l := range lines {
				if !way.MatchString(l) {
					t.Errorf("the %s sign %d: a line %q", row.Kind, id, l)
				}
			}
		}
	}
	for id, row := range rows {
		if p := database.SignTextProblem(id, row.Kind, row.Text); p != "" {
			t.Errorf("row %d: %s", id, p)
		}
		if _, there := npcs[id]; !there && id != database.SignNewsID {
			t.Errorf("row %d names no NPC", id)
		}
	}
	t.Logf("%d rows; the signs by kind: %v", len(rows), kinds)
}

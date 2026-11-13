package handlers

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A real server's quests (EQOA_REAL_DB, a copy read): every quest whose kill waits on a trigger's spawn, as
// questencounter.go links them, logged; The Mark of Rallos (101441) among them, its step 1 firing "Rallos Zek" and its
// step 3 waiting on Rallos Zek Unbound. Skipped without a database.
func TestRealEncounterLinks(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "encounters.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gw := entity.NewWorld()
	qe, err := NewQuestEngine(db, gw, session.NewManager(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := NewTriggerEngine(db, gw, session.NewManager(), nil)
	rallos := false
	n := 0
	for _, l := range qe.cat.encounters {
		if !e.spawnsAny(l.trigger, l.targets) {
			continue
		}
		n++
		t.Logf("quest %d %q: step %d fires %q, step %d kills %v", l.quest, qe.cat.byID[l.quest].Name, l.from, l.trigger, l.kill, l.targets)
		if l.quest == 101441 && l.from == 1 && l.kill == 3 && l.trigger == "Rallos Zek" {
			rallos = true
		}
	}
	t.Logf("%d links of %d candidates", n, len(qe.cat.encounters))
	if !rallos {
		t.Fatal("The Mark of Rallos is not linked to its encounter")
	}
}

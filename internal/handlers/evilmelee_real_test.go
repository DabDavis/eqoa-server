package handlers

import (
	"io"
	"log"
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The Evil Melee epic-sode 3 walked (the owner, 2026-11-18, the forum's t155;
// tools/evil_melee_20261118.py applied to the database EQOA_REAL_DB names, a
// copy read): a rogue walks Rashiroz the Rogue's chain from the Recruiter
// through Boldunder (the three reach steps by the depression, his soul) and
// Ahriaana (Magi'Kot's gems and port, Magi'Kot on the main isle, her page, the
// hand-in to Rashiroz), holding the twenty summoning gems after it, and then
// Ediopolus's reward; a good caster walks Magi'Kot's twin of it to the ghost.
// Skipped where the tool is not applied.
func TestRealEvilMeleeEpisode3Walk(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var n int
	w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'evil_melee_1118_log'`).Scan(&n)
	if n == 0 {
		t.Skip("tools/evil_melee_20261118.py is not applied to this database")
	}
	id := func(name, giver string) int {
		var q int
		if err := w.db.QueryRow(`SELECT questid FROM quests WHERE name = ? AND giver = ?`, name, giver).Scan(&q); err != nil {
			t.Fatalf("%q by %q: %v", name, giver, err)
		}
		return q
	}
	rogueBold, rogueAhri := id("Boldunder", "Rashiroz the Rogue"), id("Ahriaana the Ultimate", "Rashiroz the Rogue")
	casterAhri := id("Ahriaana the Ultimate", "Magi'Kot")
	var reward int
	if err := w.db.QueryRow(`SELECT questid FROM quests WHERE giver = 'Ediopolus' AND requires = ?`, rogueAhri).Scan(&reward); err != nil {
		t.Fatalf("the rogue's reward after %d: %v", rogueAhri, err)
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	for _, q := range []int{rogueBold, rogueAhri, id("Boldunder", "Magi'Kot"), casterAhri} {
		rows, err := w.db.Query(`SELECT step, events FROM queststeps WHERE questid = ?`, q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var st int
			var ev string
			rows.Scan(&st, &ev)
			if p := database.EventProblems(ev, true); len(p) != 0 {
				t.Errorf("quest %d step %d's events: %q", q, st, p)
			}
		}
		rows.Close()
	}
	if qu := w.quest(rogueBold); len(qu.Steps) != 5 || qu.Steps[2].Kind != database.StepReach || qu.Steps[3].Kind != database.StepLoot {
		t.Fatalf("Boldunder's steps: %+v", qu.Steps)
	}
	if why := w.walkTo(rogueAhri); why != "" {
		t.Fatalf("the rogue's chain: %s", why)
	}
	if w.p.Class != 6 {
		t.Fatalf("walked by class %d, want a rogue", w.p.Class)
	}
	gems := 0
	for _, r := range w.qe.heldItems(w.p) {
		if r.Pattern.PatternID == 716990 {
			gems += r.StackLeft
		}
	}
	if gems != 20 {
		t.Fatalf("the rogue holds %d summoning gems after the summoning, want 20", gems)
	}
	if why := w.walkOne(w.quest(reward)); why != "" {
		t.Fatalf("Ediopolus's reward: %s", why)
	}
	if why := w.walkTo(casterAhri); why != "" {
		t.Fatalf("the good caster's chain: %s", why)
	}
	if w.p.Class == 6 {
		t.Fatal("the good caster's chain walked by the rogue")
	}
}

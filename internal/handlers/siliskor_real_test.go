package handlers

import (
	"io"
	"log"
	"os"
	"testing"
)

// The Siliskor quest repeated (the owner, 2026-11-18: "siliskor quest should be
// repeatable"), on a real server's database with tools/siliskor_quest_20261118.py
// applied (EQOA_REAL_DB names it; a copy is read): a character walks it to its
// end, is offered it again by Zaryn Boome and walks it to its end a second
// time; nothing the first run left (the scepter is taken back at step 7) stands
// in the way. Skipped where the tool is not applied.
func TestRealSiliskorQuestRepeats(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var n, qid int
	w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'siliskor_1118_log'`).Scan(&n)
	if n == 0 {
		t.Skip("tools/siliskor_quest_20261118.py is not applied to this database")
	}
	w.db.QueryRow(`SELECT questid FROM quests WHERE name = 'Siliskor' AND giver = 'Zaryn Boome'`).Scan(&qid)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	if why := w.walkTo(qid); why != "" {
		t.Fatalf("the first run: %s", why)
	}
	if _, done := w.progress(qid); done != 1 {
		t.Fatalf("after the first run, finished %d times", done)
	}
	if why := w.walkOne(w.quest(qid)); why != "" {
		t.Fatalf("the second run: %s", why)
	}
	if _, done := w.progress(qid); done != 2 {
		t.Fatalf("after the second run, finished %d times, want 2", done)
	}
}

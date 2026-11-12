package handlers

import (
	"io"
	"log"
	"os"
	"testing"
)

// A real server's Pluxklez (EQOA_REAL_DB names its database, a copy of which
// is walked; skipped before tools/pluxklez_quest_20261112.py, petition 1651):
// his bulging toad eyes are offered at any standing with The Pusling (the
// owner over the petition's 150), the start's -400 too; each turn-in, again
// and again, gives one scuzz toad stew and 10 with The Pusling and no
// experience; the eye trades.
func TestRealPluxklezEyesForPusling(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var id, eye, stew, trade int
	if err := w.db.QueryRow(`SELECT questid FROM quests WHERE giver = 'Pluxklez' AND name = 'Scuzz Toad Eyes'`).Scan(&id); err != nil {
		t.Skip("no Pluxklez quest here")
	}
	qu := w.quest(id)
	if err := w.db.QueryRow(`SELECT patternid, trade FROM itempattern WHERE itemname = 'bulging toad eye'`).Scan(&eye, &trade); err != nil {
		t.Skip("tools/pluxklez_quest_20261112.py is not applied here")
	}
	if len(qu.FactionReqs) != 0 {
		t.Errorf("the quest asks %v: anyone takes it", qu.FactionReqs)
	}
	if trade != 0 {
		t.Errorf("the bulging toad eye's trade is %d: NO TRADE", trade)
	}
	if err := w.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = 'scuzz toad stew'`).Scan(&stew); err != nil {
		t.Fatal(err)
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	w.become(w.combo[0][0], w.combo[0][1])
	w.p.Level = 55
	held := func(pid int) int {
		var n int
		w.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, w.p.CharServerID, pid).Scan(&n)
		return n
	}
	xp := w.p.TotalXP
	for turn := 1; turn <= 2; turn++ {
		// walkOne tidies the pack first (as here, so the stews before are
		// counted after it)
		w.tidy()
		before := held(stew)
		if why := w.walkOne(qu); why != "" {
			t.Fatalf("turn-in %d: %s", turn, why)
		}
		if got, want := w.p.Factions["The Pusling"], -400+10*turn; got != want {
			t.Errorf("after turn-in %d: The Pusling %d, want %d (the start's -400 and 10 a turn-in)", turn, got, want)
		}
		if n := held(stew); n != before+1 {
			t.Errorf("after turn-in %d: %d stews held, want %d", turn, n, before+1)
		}
		if n := held(eye); n != 0 {
			t.Errorf("after turn-in %d: %d eyes still held", turn, n)
		}
	}
	if w.p.TotalXP != xp {
		t.Errorf("experience %d -> %d: the turn-in gives none", xp, w.p.TotalXP)
	}
}

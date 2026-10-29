package handlers

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The stat-boost ranks of a real server (EQOA_REAL_DB names its database, a
// copy of which is read), after tools/gate_stat_masteries.py: each rank the
// tool logged needs a quest in the game, and a level 60 with the points and
// the rank before it is refused it with that quest's name until the quest is
// finished, then buys it, line by line through every rank. Skipped without a
// database, and on one the tool has not gated: none is in this repository.
func TestRealStatBoostRanksNeedTheirQuests(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "statgate.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'stat_mastery_gate_log'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/gate_stat_masteries.py has not gated this database")
	}
	type gated struct{ id, quest int }
	var ranks []gated
	rows, err := db.Query(`SELECT id, after FROM stat_mastery_gate_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var g gated
		if err := rows.Scan(&g.id, &g.quest); err != nil {
			t.Fatal(err)
		}
		ranks = append(ranks, g)
	}
	rows.Close()

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	if _, err := w.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	qe, err := NewQuestEngine(db, w, mgr, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}
	inGame := qe.InGame()
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7009}, 9, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1009, Name: "Gatetest", Level: 60}
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = 200, 200, 200, 200, 200, 200, 200
	p.Mastery.Points = 1500
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)

	done := map[int]bool{}
	for _, g := range ranks {
		r := w.CMRankByID(g.id)
		name, ok := inGame[g.quest]
		if r == nil || !ok {
			t.Errorf("rank %d (%v) needs quest %d, which is not in the game", g.id, r != nil, g.quest)
			continue
		}
		k := entity.CMKnows{Quests: done, QuestNames: inGame}
		want := fmt.Sprintf("You must finish %s to buy %s.", name, r.Name)
		if res := w.BuyCM(s, g.id, k); res.Refusal != want {
			t.Errorf("%s before its quest: %q, want %q", r.Name, res.Refusal, want)
		}
		done[g.quest] = true
		if res := w.BuyCM(s, g.id, k); res.Refusal != "" {
			t.Errorf("%s after its quest: %q", r.Name, res.Refusal)
		}
	}
	t.Logf("%d ranks gated, each refused before its quest and bought after", len(ranks))
}

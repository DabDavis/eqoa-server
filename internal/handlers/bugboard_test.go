package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The board ranks as the contributors page does: the most reports first, a tie by the name in any case (the page's
// Aardvark 9+1 before Lilypad 10, acorn before Maple1989 before stoat), the other shard's line apart and
// marked; the excluded accounts, in any case, are never listed; the top N alone.
func TestBugBoardRanksAsThePage(t *testing.T) {
	rows := []bugRow{
		{Name: "Lilypad", Bugs: 10}, {Name: "gmone", Bugs: 267}, {Name: "fernwood", Bugs: 196},
		{Name: "Aardvark", Bugs: 9, Crashes: 1}, {Name: "stoat", Bugs: 5}, {Name: "acorn", Bugs: 5},
		{Name: "Maple1989", Bugs: 5}, {Name: "Gmtwo", Bugs: 40}, {Name: "OWNER", Crashes: 2},
		{Name: "Brick17", Bugs: 3, Crashes: 1}, {Name: "Brick17", Mark: "Live", Crashes: 1}, {Name: "cow", Bugs: 1},
		{Name: "nobody"},
	}
	ex := map[string]bool{"owner": true, "gmtwo": true, "gmone": true}
	got := bugBoardRank(rows, ex)
	var names []string
	for _, r := range got {
		names = append(names, r.Name+"/"+r.Mark)
	}
	want := "fernwood/ Aardvark/ Lilypad/ acorn/ Maple1989/ stoat/ Brick17/ Brick17/Live cow/"
	if strings.Join(names, " ") != want {
		t.Errorf("ranked %q, want %q", strings.Join(names, " "), want)
	}
	lines := bugBoardLines(got, 4, true, true)
	wantLines := []string{bugBoardHeadBoth, "1. fernwood: 196 bugs", "2. Aardvark: 10 (9 bugs, 1 crash)",
		"3. Lilypad: 10 bugs", "4. acorn: 5 bugs", bugBoardFootF3}
	if strings.Join(lines, "|") != strings.Join(wantLines, "|") {
		t.Errorf("lines\n%q\nwant\n%q", lines, wantLines)
	}
	if l := bugBoardLine(8, got[7]); l != "8. Brick17 (Live): 1 crash" {
		t.Errorf("a Live line: %q", l)
	}
	if l := bugBoardLine(3, bugRow{Name: "x", Bugs: 1, Crashes: 3}); l != "3. x: 4 (1 bug, 3 crashes)" {
		t.Errorf("plurals: %q", l)
	}
	if l := bugBoardLines(nil, 15, false, false); strings.Join(l, "|") != bugBoardHeadOne+"|"+bugBoardEmpty+"|"+bugBoardFootText {
		t.Errorf("no reports: %q", l)
	}
}

// bugDB is a database with these accounts (id: name) and petitions (account, category, subcategory, status).
func bugDB(t *testing.T, path string, accounts map[int]string, petitions [][3]int) *database.SQLiteDB {
	t.Helper()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for id, name := range accounts {
		if _, err := db.Exec(`INSERT INTO accountinfo (accountid, username) VALUES (?, ?)`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	addPetitions(t, db, petitions)
	return db
}

func addPetitions(t *testing.T, db *database.SQLiteDB, petitions [][3]int) {
	t.Helper()
	for i, p := range petitions {
		status := "open"
		if i%3 == 1 {
			status = "deleted" // a deleted report counts, as on the page
		}
		if _, err := db.Exec(`INSERT INTO petitions (serverid, charname, accountid, category, subcategory, filed, status)
			VALUES (?, ?, ?, ?, ?, '2026-11-14T00:00:00Z', ?)`, 1000+p[0], bugPlayer(p[0]), p[0], p[1], p[2], status); err != nil {
			t.Fatal(err)
		}
	}
}

// repeat is n petitions of account under (category, subcategory).
func repeat(n, account, cat, sub int) [][3]int {
	out := make([][3]int, n)
	for i := range out {
		out[i] = [3]int{account, cat, sub}
	}
	return out
}

// A talk to the board lists both shards' reports, counted live: a bug any Bug Report topic but Crash (a crash) and
// Feature Request (not counted), any status; another category not counted; the excluded accounts left off; a
// player on both shards two lines, the other's marked; in boxes of at most 217 characters, the heading first and
// the way to report last. A recount comes a minute on (a report filed meanwhile shows then, not before); the other
// shard unreadable leaves this shard's alone, said as one shard's. The controls: an NPC of another name on the same
// body is not the board; with no database the board says it cannot be read.
func TestBugBoardTalk(t *testing.T) {
	dir := t.TempDir()
	accounts := map[int]string{1: "owner", 8: "gmtwo", 10: "gmone", 20: "Zephyr", 21: "fernwood", 22: "Brick17", 23: "quiet"}
	var here [][3]int
	here = append(here, repeat(6, 21, 1, 13)...)  // fernwood: 6 bugs
	here = append(here, repeat(2, 21, 1, 100)...) // and 2 feature requests, no bugs
	here = append(here, repeat(4, 20, 1, 15)...)  // Zephyr: 4 bugs
	here = append(here, repeat(1, 20, 1, 10)...)  // and a crash
	here = append(here, repeat(9, 10, 1, 13)...)  // gmone: excluded
	here = append(here, repeat(3, 8, 1, 17)...)   // gmtwo: excluded
	here = append(here, repeat(2, 1, 1, 10)...)   // owner: excluded
	here = append(here, repeat(1, 22, 1, 19)...)  // Brick17: a chat report
	here = append(here, repeat(5, 23, 3, 30)...)  // quiet: a Player Issue, no bug
	db := bugDB(t, dir+"/heirloom.db", accounts, here)
	_ = bugDB(t, dir+"/live.db", accounts, append(repeat(2, 22, 1, 10), repeat(7, 10, 1, 12)...))

	SetBugBoard(config.BugBoardConfig{OtherDB: dir + "/live.db", OtherMark: "Live", Exclude: []string{"owner", "gmtwo", "gmone"}})
	t.Cleanup(func() { SetBugBoard(config.BugBoardConfig{}) })
	tw := newTalkWorld(t, 25301, 15754)
	b := NewBugBoard(db)
	now := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	tw.h.bugBoard = b
	board := tw.add(1, &entity.NPC{ServerID: 900, Name: BugBoardName, ModelID: int64(entity.PaperedBoardBody), SpawnX: 25301.08, SpawnY: 54.13, SpawnZ: 15754.48})
	plain := tw.add(2, &entity.NPC{ServerID: 901, Name: "Some Notice", ModelID: int64(entity.PaperedBoardBody), SpawnX: 25301.5, SpawnY: 54.13, SpawnZ: 15754})

	// boxes is what a talk to id said, its boxes one text (each checked within a box's room)
	boxes := func(id uint32) []string {
		tw.talk(id)
		m := questMsgs(tw.s)
		for _, x := range m {
			if n := len([]rune(strings.TrimPrefix(x, "box:"))); n > questBoxChars {
				t.Errorf("a box of %d characters: %q", n, x)
			}
		}
		if len(m) < 2 {
			return m
		}
		for i := range m {
			m[i] = strings.TrimPrefix(m[i], "box:")
		}
		return []string{"box:" + strings.Join(m, "\n")}
	}
	want := "box:" + bugBoardHeadBoth + "\n1. Fernmeadow: 6 bugs\n2. Zaeirechar: 5 (4 bugs, 1 crash)\n3. Blorck (Live): 2 crashes\n4. Blorck: 1 bug\n" + bugBoardFootText
	m := boxes(board)
	if strings.Join(m, "|") != want {
		t.Errorf("the board said\n%q\nwant\n%q", m, want)
	}

	// a report filed now shows a minute on, not before
	addPetitions(t, db, repeat(3, 22, 1, 14))
	if m := boxes(board); strings.Join(m, "|") != want {
		t.Errorf("recounted within the minute: %q", m)
	}
	now = now.Add(bugBoardFresh)
	if m := boxes(board); len(m) == 0 || !strings.Contains(m[0], "\n1. Fernmeadow: 6 bugs\n2. Zaeirechar: 5 (4 bugs, 1 crash)\n3. Blorck: 4 bugs\n4. Blorck (Live): 2 crashes\n") {
		t.Errorf("after the minute: %q", m)
	}

	// the other shard unreadable: this shard's alone, told as one shard's
	b.open = func(string) (*sql.DB, error) { return nil, errors.New("gone") }
	now = now.Add(bugBoardFresh)
	if m := boxes(board); len(m) == 0 || !strings.HasPrefix(m[0], "box:"+bugBoardHeadOne+"\n1. Fernmeadow") || strings.Contains(m[0], "Live") {
		t.Errorf("with the other shard gone: %q", m)
	}

	// many players: every box within the room, every one of the top listed, no more
	var many [][3]int
	for i := 0; i < 30; i++ {
		accounts := 100 + i
		if _, err := db.Exec(`INSERT INTO accountinfo (accountid, username) VALUES (?, ?)`, accounts, fmt.Sprintf("Longplayername%02d", i)); err != nil {
			t.Fatal(err)
		}
		many = append(many, repeat(1+i%5, accounts, 1, 13)...)
	}
	addPetitions(t, db, many)
	SetBugBoard(config.BugBoardConfig{Top: 20})
	now = now.Add(bugBoardFresh)
	tw.talk(board)
	m = questMsgs(tw.s)
	all := strings.ReplaceAll(strings.Join(m, "\n"), "box:", "")
	if len(m) < 2 || !strings.Contains(all, "\n20. ") || strings.Contains(all, "\n21. ") || !strings.HasSuffix(all, bugBoardFootText) {
		t.Errorf("twenty listed in boxes: %q", m)
	}
	for _, x := range m {
		if n := len([]rune(strings.TrimPrefix(x, "box:"))); n > questBoxChars {
			t.Errorf("a box of %d characters: %q", n, x)
		}
	}

	// the controls
	if m := boxes(plain); strings.Contains(strings.Join(m, ""), "Leaderboard") {
		t.Errorf("another board read the leaderboard: %q", m)
	}
	tw.h.bugBoard = nil
	if m := boxes(board); strings.Join(m, "|") != "box:"+bugBoardClosed {
		t.Errorf("with no database: %q", m)
	}
}

// world.bug_board's exclusions: unset, none; set, its own, in any case; Top clamped.
func TestBugBoardSettings(t *testing.T) {
	t.Cleanup(func() { SetBugBoard(config.BugBoardConfig{}) })
	SetBugBoard(config.BugBoardConfig{})
	s := bugBoardCfg.Load()
	if len(s.exclude) != 0 || s.top != bugBoardTop {
		t.Errorf("the defaults: %+v", s)
	}
	SetBugBoard(config.BugBoardConfig{Exclude: []string{" Somebody "}, Top: 500})
	s = bugBoardCfg.Load()
	if !s.exclude["somebody"] || s.exclude["gmone"] || s.top != bugBoardTopMax {
		t.Errorf("set: %+v", s)
	}
	if !IsBugBoard(" bug finder leaderboard ") || IsBugBoard("Bug Finder") {
		t.Error("IsBugBoard")
	}
}

// bugPlayer is the character an account's test reports are filed under: the board shows it, not the account.
func bugPlayer(account int) string {
	return map[int]string{1: "Dust", 8: "Funyunchar", 10: "Jinkles", 20: "Zaeirechar", 21: "Fernmeadow", 22: "Blorck", 23: "Quietone"}[account]
}

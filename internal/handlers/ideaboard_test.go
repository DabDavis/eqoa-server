package handlers

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The board ranks as the Bug Finder Leaderboard does: the most adopted features first, a tie by the name in any case
// (acorn before Maple before stoat, each a rank of its own), the other shard's line apart and marked; the excluded
// accounts, in any case, never listed; an account with none left off; the top N alone; each line's features on the
// line under it.
func TestIdeaBoardRanks(t *testing.T) {
	f := func(n ...string) []string { return n }
	rows := []ideaRow{
		{Name: "stoat", Features: f("Auto Loot")}, {Name: "Maple", Features: f("Quest Tracking")},
		{Name: "acorn", Features: f("Auto Loot")}, {Name: "Cucumber", Features: f("Auto Loot", "Quest Tracking", "Stat Window")},
		{Name: "Gmone", Account: "GMONE", Features: f("Everything", "Else", "Too", "Many")},
		{Name: "Waldorf", Features: f("Stacks of 999", "Aura Try-On")}, {Name: "Waldorf", Mark: "Live", Features: f("Klik'Anon Board")},
		{Name: "nobody"},
	}
	got := ideaBoardRank(rows, map[string]bool{"gmone": true})
	var names []string
	for _, r := range got {
		names = append(names, r.Name+"/"+r.Mark)
	}
	if want := "Cucumber/ Waldorf/ acorn/ Maple/ stoat/ Waldorf/Live"; strings.Join(names, " ") != want {
		t.Errorf("ranked %q, want %q", strings.Join(names, " "), want)
	}
	lines := ideaBoardLines(got, 4, true, true)
	want := []string{ideaBoardHeadBoth,
		"1. Cucumber: 3 ideas adopted", "Auto Loot, Quest Tracking, Stat Window",
		"2. Waldorf: 2 ideas adopted", "Stacks of 999, Aura Try-On",
		"3. acorn: 1 idea adopted", "Auto Loot",
		"4. Maple: 1 idea adopted", "Quest Tracking",
		ideaBoardFootF3}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("lines\n%q\nwant\n%q", lines, want)
	}
	if l := ideaBoardEntry(6, got[5]); strings.Join(l, "|") != "6. Waldorf (Live): 1 idea adopted|Klik'Anon Board" {
		t.Errorf("a Live line: %q", l)
	}
	if l := ideaBoardLines(nil, 15, false, false); strings.Join(l, "|") != ideaBoardHeadOne+"|"+ideaBoardEmpty+"|"+ideaBoardFootText {
		t.Errorf("none adopted: %q", l)
	}
}

// adopt marks account's n-th petition (by id, from 0) on db as asking for feature.
func adopt(t *testing.T, db *database.SQLiteDB, account, n int, feature string) {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM petitions WHERE accountid = ? ORDER BY id LIMIT 1 OFFSET ?`, account, n).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AdoptFeature(id, feature, "dab", time.Date(2026, 11, 25, 0, 0, n, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

// A talk to the board lists both shards' adopted features, counted live: two players who asked for the same feature
// each have it; one account's two petitions for one feature (in another case) count it once; a feature request
// never adopted counts nothing, nor does a bug report; the excluded accounts are left off; a player on both shards
// two lines, the other's marked; named by the character that filed most of its reports, as the bug board names it;
// in boxes within the room, the heading first and the way to ask last. A mark set now shows a minute on; the other
// shard with no table yet (before its deploy) counts as none; the other shard unreadable leaves this shard's alone.
// The controls: the bug board on the same body still says its own list; with no database the board says it cannot
// be read.
func TestIdeaBoardTalk(t *testing.T) {
	dir := t.TempDir()
	accounts := map[int]string{1: "owner", 10: "gmone", 20: "Zephyr", 21: "fernwood", 22: "Brick17", 23: "quiet"}
	var here [][3]int
	here = append(here, repeat(3, 21, 1, 100)...) // fernwood: 3 feature requests
	here = append(here, repeat(2, 20, 1, 100)...) // Zephyr: 2
	here = append(here, repeat(1, 20, 1, 13)...)  // and a bug
	here = append(here, repeat(2, 10, 1, 100)...) // gmone: excluded
	here = append(here, repeat(1, 22, 1, 100)...) // Brick17
	here = append(here, repeat(2, 23, 1, 100)...) // quiet: none adopted
	db := bugDB(t, dir+"/heirloom.db", accounts, here)
	live := bugDB(t, dir+"/live.db", accounts, append(repeat(1, 22, 1, 100), repeat(1, 1, 1, 100)...))

	adopt(t, db, 21, 0, "Auto Loot")
	adopt(t, db, 21, 1, "Quest Tracking")
	adopt(t, db, 21, 2, "auto loot ")  // the same feature again, another case: once
	adopt(t, db, 20, 0, "Auto Loot")   // Zephyr asked for it too: both have it
	adopt(t, db, 20, 2, "Stat Window") // a bug report adopted as a feature: our call, counted
	adopt(t, db, 10, 0, "Debug Mode")  // excluded
	adopt(t, db, 22, 0, "Buff Timers")
	adopt(t, live, 22, 0, "Klik'Anon Board")
	adopt(t, live, 1, 0, "Everything") // the owner: excluded

	SetBugBoard(config.BugBoardConfig{OtherDB: dir + "/live.db", OtherMark: "Live", Exclude: []string{"OWNER", "gmone"}})
	t.Cleanup(func() { SetBugBoard(config.BugBoardConfig{}) })
	tw := newTalkWorld(t, 25309, 15718)
	b := NewIdeaBoard(db)
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	tw.h.ideaBoard = b
	tw.h.bugBoard = NewBugBoard(db)
	board := tw.add(1, &entity.NPC{ServerID: 900, Name: IdeaBoardName, ModelID: int64(entity.PaperedBoardBody), SpawnX: 25309.62, SpawnY: 54.13, SpawnZ: 15718.33})
	bugs := tw.add(2, &entity.NPC{ServerID: 901, Name: BugBoardName, ModelID: int64(entity.PaperedBoardBody), SpawnX: 25309.59, SpawnY: 54.13, SpawnZ: 15721.95})

	boxes := func(id uint32) string {
		tw.talk(id)
		m := questMsgs(tw.s)
		for _, x := range m {
			if n := len([]rune(strings.TrimPrefix(x, "box:"))); n > questBoxChars {
				t.Errorf("a box of %d characters: %q", n, x)
			}
		}
		return strings.ReplaceAll(strings.Join(m, "\n"), "box:", "")
	}
	want := ideaBoardHeadBoth + "\n1. Fernmeadow: 2 ideas adopted\nAuto Loot, Quest Tracking\n2. Zaeirechar: 2 ideas adopted\n" +
		"Auto Loot, Stat Window\n3. Blorck: 1 idea adopted\nBuff Timers\n4. Blorck (Live): 1 idea adopted\nKlik'Anon Board\n" + ideaBoardFootText
	if got := boxes(board); got != want {
		t.Errorf("the board said\n%q\nwant\n%q", got, want)
	}

	// a mark set now shows a minute on, not before; three tied at 2 then go by name, each a rank of its own
	adopt(t, db, 22, 0, "Saved HUD Layouts")
	if got := boxes(board); got != want {
		t.Errorf("recounted within the minute: %q", got)
	}
	now = now.Add(bugBoardFresh)
	if got := boxes(board); !strings.Contains(got, "\n1. Blorck: 2 ideas adopted\nBuff Timers, Saved HUD Layouts\n2. Fernmeadow: 2 ideas adopted\n") {
		t.Errorf("after the minute: %q", got)
	}

	// the other shard before its deploy, no table: none of its own, both shards still
	if _, err := live.Exec(`DROP TABLE petitionadopted`); err != nil {
		t.Fatal(err)
	}
	now = now.Add(bugBoardFresh)
	if got := boxes(board); !strings.HasPrefix(got, ideaBoardHeadBoth) || strings.Contains(got, "Live") || !strings.Contains(got, "1. Blorck: 2") {
		t.Errorf("the other shard with no table: %q", got)
	}

	// the other shard unreadable: this shard's alone, told as one shard's
	b.open = func(string) (*sql.DB, error) { return nil, errors.New("gone") }
	now = now.Add(bugBoardFresh)
	if got := boxes(board); !strings.HasPrefix(got, ideaBoardHeadOne+"\n1. Blorck: 2") {
		t.Errorf("with the other shard gone: %q", got)
	}

	// the controls
	if got := boxes(bugs); !strings.HasPrefix(got, bugBoardHeadBoth) || strings.Contains(got, "adopted") {
		t.Errorf("the bug board beside it: %q", got)
	}
	tw.h.ideaBoard = nil
	if got := boxes(board); got != ideaBoardClosed {
		t.Errorf("with no database: %q", got)
	}
	if !IsIdeaBoard(" idea contributors ") || IsIdeaBoard("Idea") || IsIdeaBoard(BugBoardName) {
		t.Error("IsIdeaBoard")
	}
}

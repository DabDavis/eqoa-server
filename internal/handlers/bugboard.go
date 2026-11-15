package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// bugboard.go — the Bug Finder Leaderboard (the owner, 2026-11-15: "lets add a bulletin board in freeport by the
// well like the others with Bug Finder Leaderboard. the same list from the website with out jib and I.").
//
// Any NPC named BugBoardName is the board (tools/bugboard_20261115.py stands one, the papered bulletin board with
// its name plate, against Freeport's well on each shard). A talk to it is the list in dialogue boxes (0x0046, as
// many lines a box as hold them: sayPacked), which every client reads: a heading, the top players ranked, and how to
// report a bug. The counts are the website's contributors page's (eqoaclassic.com/contributors.html, made by hand
// on 2026-11-13 from both shards' databases): every report of Bug Report (petitions category 1) filed in the game,
// a crash its hidden Crash topic (crashSubcategory), any other topic a bug, open, answered, closed or deleted alike,
// counted by account and shard (one account on both shards is two lines, the other shard's marked, as the page's
// "Live"); a Feature Request (featureSubcategory, which came after the page) is no bug and not counted. Ranked by
// the total, then by the account's name in any case, each a rank of its own (the page's order). They are counted
// live, from this shard's database and, with world.bug_board.other_db, the other's opened read-only, at most once
// a bugBoardFresh. The accounts in world.bug_board.exclude (unset: BugBoardExcludeDefault) are never listed: the
// GM Jib's and the owner's. The words are ours.

// BugBoardName is the board's name: an NPC of it is the board.
const BugBoardName = "Bug Finder Leaderboard"

// BugBoardExcludeDefault are the accounts left off the board unless world.bug_board.exclude names others: the
// owner's (dab, and funyun, the GM panel's account 8) and the GM Jib's (jibrish), as the owner asked ("with out
// jib and I").
var BugBoardExcludeDefault = []string{"dab", "funyun", "jibrish"}

// bugBoardTop is how many are listed unless world.bug_board.top says; bugBoardTopMax the most it may say.
const (
	bugBoardTop    = 15
	bugBoardTopMax = 50
)

// bugBoardFresh is how long a count is shown before it is counted again.
const bugBoardFresh = time.Minute

// The board's words.
const (
	bugBoardHeadBoth = "Bug Finder Leaderboard: the players who reported the most bugs and crashes from inside the game, on both servers. Thank you!"
	bugBoardHeadOne  = "Bug Finder Leaderboard: the players who reported the most bugs and crashes from inside the game. Thank you!"
	bugBoardEmpty    = "No bug has been reported yet. Be the first!"
	bugBoardClosed   = "The leaderboard cannot be read just now. Try again in a while."
	bugBoardFootF3   = "Found a bug? Press F3, or type /bug and what went wrong, right where it happened."
	bugBoardFootText = "Found a bug? Type #bug and what went wrong, right where it happened."
)

// bugBoardSettings is world.bug_board as the board reads it.
type bugBoardSettings struct {
	mark, otherDB, otherMark string
	exclude                  map[string]bool
	top                      int
}

var bugBoardCfg atomic.Pointer[bugBoardSettings]

func init() { SetBugBoard(config.BugBoardConfig{}) }

// SetBugBoard sets world.bug_board for the board.
func SetBugBoard(c config.BugBoardConfig) {
	s := &bugBoardSettings{mark: strings.TrimSpace(c.Mark), otherDB: strings.TrimSpace(c.OtherDB),
		otherMark: strings.TrimSpace(c.OtherMark), exclude: map[string]bool{}, top: c.Top}
	ex := c.Exclude
	if ex == nil {
		ex = BugBoardExcludeDefault
	}
	for _, a := range ex {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			s.exclude[a] = true
		}
	}
	if s.top <= 0 {
		s.top = bugBoardTop
	}
	if s.top > bugBoardTopMax {
		s.top = bugBoardTopMax
	}
	bugBoardCfg.Store(s)
}

// IsBugBoard is whether an NPC of the name is the board.
func IsBugBoard(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), BugBoardName)
}

// bugRow is one account's line: on one shard, marked with the shard's mark.
type bugRow struct {
	Name, Mark    string
	Bugs, Crashes int
}

func (r bugRow) total() int { return r.Bugs + r.Crashes }

// BugBoard is the board's talk.
type BugBoard struct {
	db   database.Queryer
	open func(path string) (*sql.DB, error) // the other shard's database (database.OpenReadOnly)
	now  func() time.Time

	mu    sync.Mutex
	at    time.Time
	key   bugBoardSettings // the settings the count was made under
	rows  []bugRow
	other bool // the other shard's were counted
}

// NewBugBoard is the board on db (nil: nil, an NPC of its name then says nothing of its own).
func NewBugBoard(db *database.SQLiteDB) *BugBoard {
	if db == nil {
		return nil
	}
	return &BugBoard{db: db, open: database.OpenReadOnly, now: time.Now}
}

// bugTallies is q's tallies as rows marked mark.
func bugTallies(q database.Queryer, mark string) ([]bugRow, error) {
	ts, err := database.BugReportTallies(q, 1, crashSubcategory, featureSubcategory)
	if err != nil {
		return nil, err
	}
	out := make([]bugRow, 0, len(ts))
	for _, t := range ts {
		name := t.Username
		if name == "" {
			name = fmt.Sprintf("account %d", t.AccountID)
		}
		out = append(out, bugRow{Name: name, Mark: mark, Bugs: t.Bugs, Crashes: t.Crashes})
	}
	return out, nil
}

// count is the rows of both shards, counted again when the last count is bugBoardFresh old or was made under
// other settings; other reports whether the other shard's are in them. A failed read of the other shard leaves
// its rows out (logged) and is tried again at the next talk.
func (b *BugBoard) count(s bugBoardSettings) (rows []bugRow, other bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	same := b.key.mark == s.mark && b.key.otherDB == s.otherDB && b.key.otherMark == s.otherMark
	if !b.at.IsZero() && same && now.Sub(b.at) < bugBoardFresh {
		return b.rows, b.other, nil
	}
	rows, err = bugTallies(b.db, s.mark)
	if err != nil {
		return nil, false, err
	}
	complete := true
	if s.otherDB != "" {
		db, err := b.open(s.otherDB)
		if err == nil {
			var more []bugRow
			more, err = bugTallies(db, s.otherMark)
			db.Close()
			if err == nil {
				rows, other = append(rows, more...), true
			}
		}
		if err != nil {
			log.Printf("BugBoard: the other shard's reports (%s): %v; this shard's alone", s.otherDB, err)
			complete = false
		}
	}
	if complete {
		b.at, b.key, b.rows, b.other = now, bugBoardSettings{mark: s.mark, otherDB: s.otherDB, otherMark: s.otherMark}, rows, other
	}
	return rows, other, nil
}

// bugBoardRank is rows without the excluded accounts, ranked: the most reports first, a tie by the account's name
// in any case, then its mark.
func bugBoardRank(rows []bugRow, exclude map[string]bool) []bugRow {
	out := make([]bugRow, 0, len(rows))
	for _, r := range rows {
		if r.total() > 0 && !exclude[strings.ToLower(strings.TrimSpace(r.Name))] {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.total() != b.total() {
			return a.total() > b.total()
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Mark < b.Mark
	})
	return out
}

// bugBoardLine is rank's line for r: "3. Zaeire: 50 bugs", "14. lonewulf0910: 14 (11 bugs, 3 crashes)",
// "40. Blorck17 (Live): 1 crash".
func bugBoardLine(rank int, r bugRow) string {
	name := r.Name
	if r.Mark != "" {
		name += " (" + r.Mark + ")"
	}
	bugs, crashes := countOfThing(r.Bugs, "bug", "bugs"), countOfThing(r.Crashes, "crash", "crashes")
	var what string
	switch {
	case r.Crashes == 0:
		what = bugs
	case r.Bugs == 0:
		what = crashes
	default:
		what = fmt.Sprintf("%d (%s, %s)", r.total(), bugs, crashes)
	}
	return fmt.Sprintf("%d. %s: %s", rank, name, what)
}

// bugBoardLines is what the board says: its heading, the top ranked, the way to report.
func bugBoardLines(ranked []bugRow, top int, both, f3 bool) []string {
	head := bugBoardHeadOne
	if both {
		head = bugBoardHeadBoth
	}
	lines := []string{head}
	if len(ranked) == 0 {
		lines = append(lines, bugBoardEmpty)
	}
	for i, r := range ranked {
		if i >= top {
			break
		}
		lines = append(lines, bugBoardLine(i+1, r))
	}
	if f3 {
		return append(lines, bugBoardFootF3)
	}
	return append(lines, bugBoardFootText)
}

// Talk is a talk to npc: the board's list when npc is the board, else false.
func (b *BugBoard) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsBugBoard(npc.Name) || p == nil {
		return false
	}
	forgetDialogs(p)
	if b == nil {
		sayPacked(sess, "", []string{bugBoardClosed})
		return true
	}
	s := *bugBoardCfg.Load()
	rows, other, err := b.count(s)
	if err != nil {
		log.Printf("BugBoard: the reports: %v", err)
		sayPacked(sess, "", []string{bugBoardClosed})
		return true
	}
	ranked := bugBoardRank(rows, s.exclude)
	sayPacked(sess, "", bugBoardLines(ranked, s.top, other, sess.BugReports()))
	log.Printf("BugBoard: %s read the board: %d players ranked", p.Name, len(ranked))
	return true
}

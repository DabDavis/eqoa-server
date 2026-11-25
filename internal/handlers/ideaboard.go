package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// ideaboard.go — the Idea Contributors board (petition 3016, Cucumber, approved by the owner 2026-11-25: "For the
// bulletins in FP that show bug report top scores and totals, create another for Feature Request top contributors
// and which features they suggested that ultimately were adopted. If 2 people suggested the same feature, each gets
// it listed under them.").
//
// Any NPC named IdeaBoardName is the board (tools/ideaboard_20261125.py stands one, the papered bulletin board with
// its name plate, beside the Bug Finder Leaderboard at Freeport's well on each shard). A talk to it is the list in
// dialogue boxes (sayPacked), which every client reads: a heading, the top players ranked by the features they asked
// for that the game took up, each with those features' names on the line under it, and how to ask for one.
//
// What counts is table petitionadopted (database/ideaboard.go): a mark a feature an adopted petition asked for, set
// by us (tools/adopt_feature.py, or the GM panel's Adopted features). A player's are the features its account's
// adopted petitions named, each once in any case, whatever became of the petitions since; two players who asked for
// the same feature each have it. Everything else is the Bug Finder Leaderboard's (bugboard.go), so the two boards
// agree: world.bug_board's settings (this shard's mark, the other shard's database read-only and its mark, the
// excluded accounts, how many are listed), a line an account and shard, named by the same character, ranked by the
// count then the name in any case, each a rank of its own, counted again at most once a bugBoardFresh.

// IdeaBoardName is the board's name: an NPC of it is the board.
const IdeaBoardName = "Idea Contributors"

// The board's words.
const (
	ideaBoardHeadBoth = "Idea Contributors: the players whose feature requests became part of the game, on both servers. Thank you!"
	ideaBoardHeadOne  = "Idea Contributors: the players whose feature requests became part of the game. Thank you!"
	ideaBoardEmpty    = "No idea has been adopted yet. Yours could be the first!"
	ideaBoardClosed   = "The board cannot be read just now. Try again in a while."
	ideaBoardFootF3   = "Have an idea? Type /feature and what you would like added or changed."
	ideaBoardFootText = "Have an idea? Type #feature and what you would like added or changed."
)

// IsIdeaBoard is whether an NPC of the name is the board.
func IsIdeaBoard(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), IdeaBoardName)
}

// ideaRow is one account's line: on one shard, marked with the shard's mark.
type ideaRow struct {
	Name, Mark string
	Account    string // the account's name, which world.bug_board.exclude names (Name is its player's)
	Features   []string
}

// IdeaBoard is the board's talk.
type IdeaBoard struct {
	db   database.Queryer
	open func(path string) (*sql.DB, error) // the other shard's database (database.OpenReadOnly)
	now  func() time.Time

	mu    sync.Mutex
	at    time.Time
	key   bugBoardSettings
	rows  []ideaRow
	other bool
}

// NewIdeaBoard is the board on db (nil: nil, the board then says it cannot be read).
func NewIdeaBoard(db *database.SQLiteDB) *IdeaBoard {
	if db == nil {
		return nil
	}
	return &IdeaBoard{db: db, open: database.OpenReadOnly, now: time.Now}
}

// ideaTallies is q's tallies as rows marked mark.
func ideaTallies(q database.Queryer, mark string) ([]ideaRow, error) {
	ts, err := database.IdeaTallies(q, database.FeatureRequestCategory)
	if err != nil {
		return nil, err
	}
	out := make([]ideaRow, 0, len(ts))
	for _, t := range ts {
		name := t.Player
		if name == "" {
			name = t.Username
		}
		if name == "" {
			name = fmt.Sprintf("account %d", t.AccountID)
		}
		out = append(out, ideaRow{Name: name, Account: t.Username, Mark: mark, Features: t.Features})
	}
	return out, nil
}

// count is the rows of both shards, as BugBoard.count counts them.
func (b *IdeaBoard) count(s bugBoardSettings) (rows []ideaRow, other bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	same := b.key.mark == s.mark && b.key.otherDB == s.otherDB && b.key.otherMark == s.otherMark
	if !b.at.IsZero() && same && now.Sub(b.at) < bugBoardFresh {
		return b.rows, b.other, nil
	}
	rows, err = ideaTallies(b.db, s.mark)
	if err != nil {
		return nil, false, err
	}
	complete := true
	if s.otherDB != "" {
		db, err := b.open(s.otherDB)
		if err == nil {
			var more []ideaRow
			more, err = ideaTallies(db, s.otherMark)
			db.Close()
			if err == nil {
				rows, other = append(rows, more...), true
			}
		}
		if err != nil {
			log.Printf("IdeaBoard: the other shard's adopted features (%s): %v; this shard's alone", s.otherDB, err)
			complete = false
		}
	}
	if complete {
		b.at, b.key, b.rows, b.other = now, bugBoardSettings{mark: s.mark, otherDB: s.otherDB, otherMark: s.otherMark}, rows, other
	}
	return rows, other, nil
}

// ideaBoardRank is rows without the excluded accounts, ranked: the most features first, a tie by the name in any
// case, then its mark.
func ideaBoardRank(rows []ideaRow, exclude map[string]bool) []ideaRow {
	out := make([]ideaRow, 0, len(rows))
	for _, r := range rows {
		acct := r.Account
		if acct == "" {
			acct = r.Name
		}
		if len(r.Features) > 0 && !exclude[strings.ToLower(strings.TrimSpace(acct))] {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if len(a.Features) != len(b.Features) {
			return len(a.Features) > len(b.Features)
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Mark < b.Mark
	})
	return out
}

// ideaBoardEntry is rank's lines for r: "2. Cucumber (Live): 3 ideas adopted" and under it its features,
// "Auto Loot, Quest Tracking, Attack With Abilities".
func ideaBoardEntry(rank int, r ideaRow) []string {
	name := r.Name
	if r.Mark != "" {
		name += " (" + r.Mark + ")"
	}
	return []string{fmt.Sprintf("%d. %s: %s adopted", rank, name, countOfThing(len(r.Features), "idea", "ideas")),
		strings.Join(r.Features, ", ")}
}

// ideaBoardLines is what the board says: its heading, the top ranked with their features, the way to ask.
func ideaBoardLines(ranked []ideaRow, top int, both, f3 bool) []string {
	head := ideaBoardHeadOne
	if both {
		head = ideaBoardHeadBoth
	}
	lines := []string{head}
	if len(ranked) == 0 {
		lines = append(lines, ideaBoardEmpty)
	}
	for i, r := range ranked {
		if i >= top {
			break
		}
		lines = append(lines, ideaBoardEntry(i+1, r)...)
	}
	if f3 {
		return append(lines, ideaBoardFootF3)
	}
	return append(lines, ideaBoardFootText)
}

// Talk is a talk to npc: the board's list when npc is the board, else false.
func (b *IdeaBoard) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsIdeaBoard(npc.Name) || p == nil {
		return false
	}
	forgetDialogs(p)
	if b == nil {
		sayPacked(sess, "", []string{ideaBoardClosed})
		return true
	}
	s := *bugBoardCfg.Load()
	rows, other, err := b.count(s)
	if err != nil {
		log.Printf("IdeaBoard: the adopted features: %v", err)
		sayPacked(sess, "", []string{ideaBoardClosed})
		return true
	}
	ranked := ideaBoardRank(rows, s.exclude)
	sayPacked(sess, "", ideaBoardLines(ranked, s.top, other, sess.BugReports()))
	log.Printf("IdeaBoard: %s read the board: %d players ranked", p.Name, len(ranked))
	return true
}

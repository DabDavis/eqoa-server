package database

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Personal boards: a private thread between one GM (with that GM's AI agents)
// and dab's sessions (Claude Code on dab's own machine, reaching the box by
// ssh), so the owner can say "post to jib's personal board" and the GM can
// tell their agent to check it (the GM Jib's ask, 2026-11-15). The work board
// (workboard.go) is everyone's; a personal board is read by its members
// alone: the panel accounts config.yaml's gm.personal_boards names for it,
// the panel's admins, and cmd/workboard on the box. Which board is whose is
// config, never this table: a message here is only text on a board's name.
//
// The GM panel keeps the tables out of its table pages and generic row
// operations altogether (gmweb's catalog marks them hidden), so a GM who is
// not a member cannot read a board by list_rows or search; the board's own
// operations (gmweb/ops_boards.go) check membership first. A database
// snapshot clears both tables (snapshot.go), as it clears the work board.
//
// Each post is written in one transaction with a gm_audit line naming the
// board and the message's id, never its text (the audit log is every GM's
// to read). A message is not a row change the panel can revert: a board is a
// conversation, and a word said is answered, not unsaid.
//
// gmboard_read is how far each reader has read a board: the id of the newest
// message they were shown. A reader is a panel account's name in lower case,
// or the name cmd/workboard posts as ("dab's sessions"); a message's author
// key is the same, so a reader's own posts never count as unread to them.
const gmBoardSchema = `
	CREATE TABLE IF NOT EXISTS gmboard (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		board      TEXT NOT NULL,
		at         TEXT NOT NULL,
		author     TEXT NOT NULL,
		author_key TEXT NOT NULL,
		via        TEXT NOT NULL,
		text       TEXT NOT NULL,
		reply_to   INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_gmboard_board ON gmboard(board, id);
	CREATE TABLE IF NOT EXISTS gmboard_read (
		board   TEXT NOT NULL,
		reader  TEXT NOT NULL,
		last_id INTEGER NOT NULL DEFAULT 0,
		at      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (board, reader)
	);`

// Where a message was posted from.
const (
	BoardViaPanel = "panel" // the GM panel: the page, a token or an OAuth agent
	BoardViaCLI   = "cli"   // cmd/workboard on the box: dab's sessions
)

// The bounds of a message, in characters, and of a read.
const (
	BoardMaxText      = 20000
	BoardMaxName      = 32
	BoardDefaultLimit = 50
	BoardMaxLimit     = 200
)

var boardNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// NormalizeBoardName is a board's name as it is kept: trimmed, in lower case,
// letters, digits, - and _ (jib, jib-agents); false for a name that is not.
func NormalizeBoardName(s string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(s))
	if k == "" || len(k) > BoardMaxName || !boardNameRE.MatchString(k) {
		return "", false
	}
	return k, true
}

// BoardReaderKey is a reader (or an author) as the read marks name them: the
// account's name in lower case, trimmed.
func BoardReaderKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func (db *SQLiteDB) ensureGMBoard() error {
	if _, err := db.Exec(gmBoardSchema); err != nil {
		return fmt.Errorf("gmboard: %w", err)
	}
	return nil
}

// BoardMessage is one message on a board.
type BoardMessage struct {
	ID      int64  `json:"id"`
	Board   string `json:"board"`
	At      string `json:"at"`
	Author  string `json:"author"` // as workBy names them: "jibrish (oauth:Claude)", "dab's sessions"
	Via     string `json:"via"`    // panel or cli
	Text    string `json:"text"`   // markdown
	ReplyTo int64  `json:"reply_to,omitempty"`
	// Unread is whether the reader had not been shown it before this read
	// (and did not write it).
	Unread bool `json:"unread,omitempty"`

	authorKey string
}

// BoardPost is a message to post.
type BoardPost struct {
	Board   string
	Text    string
	ReplyTo int64
	Via     string // BoardViaPanel or BoardViaCLI
}

// PostBoardMessage writes a message to a board, with its gm_audit line (who,
// the board and the message's id; not the text), in one transaction. The
// board's name must already be one its caller may post to: this checks only
// the message. A message is read by its own author already.
func (db *SQLiteDB) PostBoardMessage(p BoardPost, who GMEdit) (BoardMessage, int64, error) {
	board, ok := NormalizeBoardName(p.Board)
	if !ok {
		return BoardMessage{}, 0, fmt.Errorf("%q is not a board's name", p.Board)
	}
	text := workText(p.Text)
	if strings.TrimSpace(text) == "" {
		return BoardMessage{}, 0, errors.New("a message needs its text")
	}
	if n := workRunes(text); n > BoardMaxText {
		return BoardMessage{}, 0, fmt.Errorf("the message is %d characters: %d at most", n, BoardMaxText)
	}
	via := p.Via
	if via != BoardViaCLI {
		via = BoardViaPanel
	}
	key := BoardReaderKey(who.Username)
	if key == "" {
		return BoardMessage{}, 0, errors.New("a message needs its author's name")
	}
	tx, err := db.Begin()
	if err != nil {
		return BoardMessage{}, 0, err
	}
	defer tx.Rollback()
	if p.ReplyTo != 0 {
		var on string
		err := tx.QueryRow(`SELECT board FROM gmboard WHERE id = ?`, p.ReplyTo).Scan(&on)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && on != board) {
			return BoardMessage{}, 0, fmt.Errorf("reply_to %d is not a message on board %s", p.ReplyTo, board)
		}
		if err != nil {
			return BoardMessage{}, 0, err
		}
	}
	m := BoardMessage{Board: board, At: workStamp(time.Now()), Author: workBy(who), Via: via, Text: text,
		ReplyTo: p.ReplyTo, authorKey: key}
	res, err := tx.Exec(`INSERT INTO gmboard (board, at, author, author_key, via, text, reply_to) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.Board, m.At, m.Author, key, m.Via, m.Text, m.ReplyTo)
	if err != nil {
		return BoardMessage{}, 0, err
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return BoardMessage{}, 0, err
	}
	detail := fmt.Sprintf("message %d on personal board %s", m.ID, board)
	if m.ReplyTo != 0 {
		detail += fmt.Sprintf(", a reply to %d", m.ReplyTo)
	}
	// not a row change (no key, no rows): the audit log lists it and nothing
	// reverts it, and the text stays on the board alone
	audit, err := tx.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, ?, ?, ?, ?, 'board_post', 'gmboard', '', NULL, NULL, ?)`,
		time.Now().UTC().Format(time.RFC3339), who.AccountID, who.Username, who.Via, who.IP, detail)
	if err != nil {
		return BoardMessage{}, 0, fmt.Errorf("gm_audit: %w", err)
	}
	auditID, err := audit.LastInsertId()
	if err != nil {
		return BoardMessage{}, 0, err
	}
	return m, auditID, tx.Commit()
}

// BoardQuery is a read of a board.
type BoardQuery struct {
	Board string
	// Reader is who reads (BoardReaderKey's form is made here): the messages
	// they had not been shown are marked Unread, and their read mark moves to
	// the newest message returned unless Peek.
	Reader string
	// SinceID: only messages after it, the oldest first, Limit of them.
	// 0: the newest Limit messages (still the oldest of them first).
	SinceID int64
	Limit   int
	Peek    bool
}

// BoardThread is a read of a board.
type BoardThread struct {
	Board    string         `json:"board"`
	Messages []BoardMessage `json:"messages"`
	Total    int            `json:"total"` // messages on the board
	// More: messages after the last returned (a since_id read cut at the limit).
	More int `json:"more"`
	// Unread is what is still unread to the reader after this read: by
	// others, newer than the read mark.
	Unread int   `json:"unread"`
	ReadTo int64 `json:"read_to"` // the reader's mark after this read
}

// BoardMessages reads a board (see BoardQuery), and moves the reader's mark.
func (db *SQLiteDB) BoardMessages(q BoardQuery) (BoardThread, error) {
	board, ok := NormalizeBoardName(q.Board)
	if !ok {
		return BoardThread{}, fmt.Errorf("%q is not a board's name", q.Board)
	}
	reader := BoardReaderKey(q.Reader)
	limit := q.Limit
	if limit <= 0 {
		limit = BoardDefaultLimit
	}
	if limit > BoardMaxLimit {
		limit = BoardMaxLimit
	}
	if q.SinceID < 0 {
		return BoardThread{}, errors.New("since_id is a message's id, 0 or more")
	}
	mark, err := db.boardMark(board, reader)
	if err != nil {
		return BoardThread{}, err
	}
	const cols = `id, board, at, author, author_key, via, text, reply_to`
	var rs *sql.Rows
	if q.SinceID > 0 {
		rs, err = db.Query(`SELECT `+cols+` FROM gmboard WHERE board = ? AND id > ? ORDER BY id LIMIT ?`, board, q.SinceID, limit)
	} else {
		rs, err = db.Query(`SELECT * FROM (SELECT `+cols+` FROM gmboard WHERE board = ? ORDER BY id DESC LIMIT ?) ORDER BY id`, board, limit)
	}
	if err != nil {
		return BoardThread{}, err
	}
	th := BoardThread{Board: board, Messages: []BoardMessage{}}
	for rs.Next() {
		var m BoardMessage
		if err := rs.Scan(&m.ID, &m.Board, &m.At, &m.Author, &m.authorKey, &m.Via, &m.Text, &m.ReplyTo); err != nil {
			rs.Close()
			return BoardThread{}, err
		}
		m.Unread = m.ID > mark && m.authorKey != reader
		th.Messages = append(th.Messages, m)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return BoardThread{}, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gmboard WHERE board = ?`, board).Scan(&th.Total); err != nil {
		return BoardThread{}, err
	}
	last := q.SinceID
	if n := len(th.Messages); n > 0 {
		last = th.Messages[n-1].ID
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gmboard WHERE board = ? AND id > ?`, board, last).Scan(&th.More); err != nil {
		return BoardThread{}, err
	}
	th.ReadTo = mark
	if !q.Peek && reader != "" && last > mark {
		if _, err := db.Exec(`INSERT INTO gmboard_read (board, reader, last_id, at) VALUES (?, ?, ?, ?)
			ON CONFLICT (board, reader) DO UPDATE SET last_id = MAX(last_id, excluded.last_id), at = excluded.at`,
			board, reader, last, workStamp(time.Now())); err != nil {
			return BoardThread{}, err
		}
		th.ReadTo = last
	}
	if th.Unread, err = db.boardUnread(board, reader, th.ReadTo); err != nil {
		return BoardThread{}, err
	}
	return th, nil
}

func (db *SQLiteDB) boardMark(board, reader string) (int64, error) {
	var mark int64
	err := db.QueryRow(`SELECT last_id FROM gmboard_read WHERE board = ? AND reader = ?`, board, reader).Scan(&mark)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return mark, err
}

func (db *SQLiteDB) boardUnread(board, reader string, mark int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM gmboard WHERE board = ? AND id > ? AND author_key != ?`, board, mark, reader).Scan(&n)
	return n, err
}

// BoardSummary is a board as a list of boards shows it.
type BoardSummary struct {
	Board    string   `json:"board"`
	Members  []string `json:"members,omitempty"`
	Messages int      `json:"messages"`
	Unread   int      `json:"unread"`
	LastID   int64    `json:"last_id,omitempty"`
	LastAt   string   `json:"last_at,omitempty"`
	LastBy   string   `json:"last_by,omitempty"`
}

// BoardSummaryFor is a board's counts for a reader: its messages, those
// unread to the reader, and the newest's id, time and author.
func (db *SQLiteDB) BoardSummaryFor(board, reader string) (BoardSummary, error) {
	b, ok := NormalizeBoardName(board)
	if !ok {
		return BoardSummary{}, fmt.Errorf("%q is not a board's name", board)
	}
	reader = BoardReaderKey(reader)
	s := BoardSummary{Board: b}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gmboard WHERE board = ?`, b).Scan(&s.Messages); err != nil {
		return s, err
	}
	err := db.QueryRow(`SELECT id, at, author FROM gmboard WHERE board = ? ORDER BY id DESC LIMIT 1`, b).Scan(&s.LastID, &s.LastAt, &s.LastBy)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	mark, err := db.boardMark(b, reader)
	if err != nil {
		return s, err
	}
	s.Unread, err = db.boardUnread(b, reader, mark)
	return s, err
}

// BoardNamesWithMessages is every board that has a message, for a list that
// should also show a board config no longer names.
func (db *SQLiteDB) BoardNamesWithMessages() ([]string, error) {
	rs, err := db.Query(`SELECT DISTINCT board FROM gmboard ORDER BY board`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []string
	for rs.Next() {
		var b string
		if err := rs.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rs.Err()
}

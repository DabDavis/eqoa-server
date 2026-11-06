package database

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Character transfers (the user, 2026-11-05: "players from sandstorm want to
// keep their characters", then "build the form"): a player's request to bring
// a character over from another server, filed on the transfer page
// (internal/transfers) for a character the player has made here (its race,
// sex and look are theirs), decided by a GM on the panel's Character transfers
// page (gmweb/ops_transfers.go), and applied by internal/charmove at that
// character's next login (handlers/transfers.go), before the login reads it:
// a character in the world would write its level back at its logout.
//
// Nothing another server sends is signed, so a transfer is the player's word
// until a GM decides: the GM's plan (level, mastery points, the class's spells,
// the items matched to this server's records or made for what it lacks) is what
// applies, not the player's text.
const transfersSchema = `
	CREATE TABLE IF NOT EXISTS chartransfers (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid      INTEGER NOT NULL,
		serverid       INTEGER NOT NULL,
		charname       TEXT NOT NULL DEFAULT '',
		from_server    TEXT NOT NULL DEFAULT '',
		from_name      TEXT NOT NULL DEFAULT '',
		from_class     TEXT NOT NULL DEFAULT '',
		from_level     INTEGER NOT NULL DEFAULT 0,
		from_masteries INTEGER NOT NULL DEFAULT 0,
		items          TEXT NOT NULL DEFAULT '',
		notes          TEXT NOT NULL DEFAULT '',
		status         TEXT NOT NULL DEFAULT 'waiting',
		filed          TEXT NOT NULL,
		updated        TEXT NOT NULL DEFAULT '',
		plan           TEXT NOT NULL DEFAULT '',
		answer         TEXT NOT NULL DEFAULT '',
		decided        TEXT NOT NULL DEFAULT '',
		decided_by     TEXT NOT NULL DEFAULT '',
		applied        TEXT NOT NULL DEFAULT '',
		run            TEXT NOT NULL DEFAULT '',
		told           INTEGER NOT NULL DEFAULT 0,
		claims         TEXT NOT NULL DEFAULT '',
		told_ready     INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_chartransfers_char ON chartransfers(serverid, status);
	CREATE INDEX IF NOT EXISTS idx_chartransfers_status ON chartransfers(status);`

// The statuses of a transfer.
const (
	TransferWaiting   = "waiting"   // filed: a GM's to decide
	TransferApproved  = "approved"  // a GM's plan set: applies at the character's next login
	TransferApplied   = "applied"   // done (run names it in charmigrate_log)
	TransferRejected  = "rejected"  // a GM said no (answer says why)
	TransferCancelled = "cancelled" // the player took it back before a GM decided
	TransferUndone    = "undone"    // a GM took an applied one back
)

// TransferStatuses is every status.
var TransferStatuses = []string{TransferWaiting, TransferApproved, TransferApplied, TransferRejected, TransferCancelled, TransferUndone}

// The limits of what a player files.
const (
	TransferMaxName   = 32
	TransferMaxItems  = 4000 // the gear list, as typed
	TransferMaxNotes  = 2000
	TransferMaxAnswer = 2000
)

// Transfer is one row of chartransfers.
type Transfer struct {
	ID            int64  `json:"id"`
	AccountID     int    `json:"accountid"`
	ServerID      uint32 `json:"serverid"`
	CharName      string `json:"charname"`
	FromServer    string `json:"from_server"`
	FromName      string `json:"from_name"`
	FromClass     string `json:"from_class"`
	FromLevel     int    `json:"from_level"`
	FromMasteries int    `json:"from_masteries"`
	Items         string `json:"items"`
	Notes         string `json:"notes"`
	Status        string `json:"status"`
	Filed         string `json:"filed"`
	Updated       string `json:"updated"`
	Plan          string `json:"plan"`
	Answer        string `json:"answer"`
	Decided       string `json:"decided"`
	DecidedBy     string `json:"decided_by"`
	Applied       string `json:"applied"`
	Run           string `json:"run"`
	Told          bool   `json:"told"`
	// Claims is what the player says the character had done besides its gear
	// (TransferClaims as JSON, "" none).
	Claims string `json:"claims"`
	// ToldReady is whether the player was told it was approved (in the game,
	// on any character of the account).
	ToldReady bool `json:"told_ready"`
}

// Open is whether a GM or the next login still has something to do with it.
func (t Transfer) Open() bool { return t.Status == TransferWaiting || t.Status == TransferApproved }

const transferCols = `id, accountid, serverid, charname, from_server, from_name, from_class, from_level, from_masteries,
	items, notes, status, filed, updated, plan, answer, decided, decided_by, applied, run, told, claims, told_ready`

func scanTransfer(sc interface{ Scan(...any) error }) (Transfer, error) {
	var t Transfer
	var told, toldReady int
	err := sc.Scan(&t.ID, &t.AccountID, &t.ServerID, &t.CharName, &t.FromServer, &t.FromName, &t.FromClass, &t.FromLevel,
		&t.FromMasteries, &t.Items, &t.Notes, &t.Status, &t.Filed, &t.Updated, &t.Plan, &t.Answer, &t.Decided, &t.DecidedBy,
		&t.Applied, &t.Run, &told, &t.Claims, &toldReady)
	t.Told, t.ToldReady = told != 0, toldReady != 0
	return t, err
}

func (db *SQLiteDB) ensureTransfers() error {
	if _, err := db.Exec(transfersSchema); err != nil {
		return fmt.Errorf("chartransfers: %w", err)
	}
	// a table made before the claims (2026-11-05 05:57 to the next start)
	for _, c := range []struct{ col, decl string }{
		{"claims", `TEXT NOT NULL DEFAULT ''`}, {"told_ready", `INTEGER NOT NULL DEFAULT 0`},
	} {
		has, err := hasColumn(db.DB, "chartransfers", c.col)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE chartransfers ADD COLUMN ` + c.col + ` ` + c.decl); err != nil {
				return fmt.Errorf("chartransfers.%s: %w", c.col, err)
			}
		}
	}
	return nil
}

func transferStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// transferText is a player's text as kept: valid UTF-8, line ends "\n", no
// other control character but a tab, trimmed.
func transferText(s string) string {
	s = strings.ToValidUTF8(strings.ReplaceAll(s, "\r\n", "\n"), "?")
	var b strings.Builder
	for _, r := range s {
		if r == '\r' {
			r = '\n'
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// The refusals a player is told.
var (
	ErrTransferNotFound = errors.New("no such transfer")
	ErrTransferOpen     = errors.New("this character has a transfer waiting already")
	ErrTransferDecided  = errors.New("a GM has decided this transfer already")
)

// FileTransfer files a player's request for one of the account's own
// characters, waiting on a GM: refused while that character has one open
// (waiting or approved). An account may have any number open, one a
// character (the user, 2026-11-06: "it is capped at 3, lets unlimit it": a
// player bringing several characters over hit three, an approved one counting
// until its character's next login). The character is checked to be the
// account's.
func (db *SQLiteDB) FileTransfer(t Transfer, now time.Time) (Transfer, error) {
	t.FromServer = strings.TrimSpace(transferText(t.FromServer))
	t.FromName = strings.TrimSpace(transferText(t.FromName))
	t.FromClass = strings.TrimSpace(transferText(t.FromClass))
	t.Items, t.Notes = transferText(t.Items), transferText(t.Notes)
	switch {
	case t.FromName == "":
		return Transfer{}, errors.New("name the character on the other server")
	case utf8.RuneCountInString(t.FromName) > TransferMaxName || utf8.RuneCountInString(t.FromClass) > TransferMaxName ||
		utf8.RuneCountInString(t.FromServer) > TransferMaxName:
		return Transfer{}, fmt.Errorf("a name or a class is %d characters at most", TransferMaxName)
	case t.FromLevel < 1 || t.FromLevel > 60:
		return Transfer{}, errors.New("the level is 1 to 60")
	case t.FromMasteries < 0 || t.FromMasteries > 1500:
		return Transfer{}, errors.New("mastery points are 0 to 1500")
	case utf8.RuneCountInString(t.Items) > TransferMaxItems:
		return Transfer{}, fmt.Errorf("the gear list is %d characters at most", TransferMaxItems)
	case utf8.RuneCountInString(t.Notes) > TransferMaxNotes:
		return Transfer{}, fmt.Errorf("the notes are %d characters at most", TransferMaxNotes)
	}
	if t.FromServer == "" {
		t.FromServer = "Sandstorm"
	}
	claims, err := ParseTransferClaims(t.Claims)
	if err != nil {
		return Transfer{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback()
	var owner int
	var name string
	err = tx.QueryRow(`SELECT accountid, charName FROM characters WHERE serverid = ?`, t.ServerID).Scan(&owner, &name)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != t.AccountID) {
		return Transfer{}, errors.New("that is not one of your characters")
	}
	if err != nil {
		return Transfer{}, err
	}
	t.CharName = name
	var forChar int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM chartransfers WHERE serverid = ? AND accountid = ? AND status IN (?, ?)`,
		t.ServerID, t.AccountID, TransferWaiting, TransferApproved).Scan(&forChar); err != nil {
		return Transfer{}, err
	}
	if forChar > 0 {
		return Transfer{}, ErrTransferOpen
	}
	if claims, err = knownClaims(tx, claims); err != nil {
		return Transfer{}, err
	}
	t.Claims = FormatTransferClaims(claims)
	stamp := transferStamp(now)
	res, err := tx.Exec(`INSERT INTO chartransfers (accountid, serverid, charname, from_server, from_name, from_class, from_level,
		from_masteries, items, notes, status, filed, updated, claims) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.AccountID, t.ServerID, t.CharName, t.FromServer, t.FromName, t.FromClass, t.FromLevel, t.FromMasteries, t.Items, t.Notes,
		TransferWaiting, stamp, stamp, t.Claims)
	if err != nil {
		return Transfer{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Transfer{}, err
	}
	out, err := scanTransfer(tx.QueryRow(`SELECT `+transferCols+` FROM chartransfers WHERE id = ?`, id))
	if err != nil {
		return Transfer{}, err
	}
	return out, tx.Commit()
}

// AccountTransfers is an account's transfers, newest first, at most 50.
func (db *SQLiteDB) AccountTransfers(accountID int) ([]Transfer, error) {
	return db.queryTransfers(`SELECT `+transferCols+` FROM chartransfers WHERE accountid = ? ORDER BY id DESC LIMIT 50`, accountID)
}

func (db *SQLiteDB) queryTransfers(q string, args ...any) ([]Transfer, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CancelTransfer takes back the account's own transfer while it waits on a GM.
func (db *SQLiteDB) CancelTransfer(accountID int, id int64, now time.Time) error {
	res, err := db.Exec(`UPDATE chartransfers SET status = ?, updated = ? WHERE id = ? AND accountid = ? AND status = ?`,
		TransferCancelled, transferStamp(now), id, accountID, TransferWaiting)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var status string
		if err := db.QueryRow(`SELECT status FROM chartransfers WHERE id = ? AND accountid = ?`, id, accountID).Scan(&status); err != nil {
			return ErrTransferNotFound
		}
		return ErrTransferDecided
	}
	return nil
}

// TransferQuery is what the GMs' list shows.
type TransferQuery struct {
	Status string // a status, "open" (waiting or approved), or "" / "all"
	Limit  int
	Offset int
}

// TransferList is a page of transfers, how many match, and how many each
// status holds.
type TransferList struct {
	Transfers []Transfer     `json:"transfers"`
	Matching  int            `json:"matching"`
	Counts    map[string]int `json:"counts"`
}

// Transfers is the GMs' list: the open ones (waiting first, then approved,
// oldest first) by default, else by status, newest first.
func (db *SQLiteDB) Transfers(q TransferQuery) (TransferList, error) {
	var out TransferList
	out.Counts = map[string]int{}
	rows, err := db.Query(`SELECT status, COUNT(*) FROM chartransfers GROUP BY status`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			rows.Close()
			return out, err
		}
		out.Counts[s] = n
	}
	rows.Close()
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	where, order, args := "", "ORDER BY id DESC", []any{}
	switch q.Status {
	case "", "open":
		where, order = "WHERE status IN (?, ?)", "ORDER BY status = 'approved', id"
		args = append(args, TransferWaiting, TransferApproved)
	case "all":
	default:
		if !inFold(TransferStatuses, q.Status) {
			return out, fmt.Errorf("status %q: one of %s, open or all", q.Status, strings.Join(TransferStatuses, ", "))
		}
		where = "WHERE status = ?"
		args = append(args, q.Status)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM chartransfers `+where, args...).Scan(&out.Matching); err != nil {
		return out, err
	}
	out.Transfers, err = db.queryTransfers(`SELECT `+transferCols+` FROM chartransfers `+where+` `+order+` LIMIT ? OFFSET ?`,
		append(args, limit, max(q.Offset, 0))...)
	return out, err
}

// TransferByID is one transfer.
func (db *SQLiteDB) TransferByID(id int64) (Transfer, bool, error) {
	t, err := scanTransfer(db.QueryRow(`SELECT `+transferCols+` FROM chartransfers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Transfer{}, false, nil
	}
	return t, err == nil, err
}

// decideTransfer moves a transfer from one of the statuses in from to status,
// with set's columns, in one transaction with its gm_audit line as who's row
// change (revertible).
func (db *SQLiteDB) decideTransfer(id int64, from []string, status string, set map[string]any, who GMEdit, summary string, now time.Time) (Transfer, int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return Transfer{}, 0, err
	}
	defer tx.Rollback()
	before, err := fullRow(tx, `SELECT * FROM chartransfers WHERE id = ?`, id)
	if err != nil {
		return Transfer{}, 0, err
	}
	if before == nil {
		return Transfer{}, 0, fmt.Errorf("no transfer %d (transfer_list lists them)", id)
	}
	t, err := scanTransfer(tx.QueryRow(`SELECT `+transferCols+` FROM chartransfers WHERE id = ?`, id))
	if err != nil {
		return Transfer{}, 0, err
	}
	if !inFold(from, t.Status) {
		return Transfer{}, 0, fmt.Errorf("transfer %d is %s: only one %s may be", id, t.Status, strings.Join(from, " or "))
	}
	cols := []string{"status = ?", "updated = ?"}
	args := []any{status, transferStamp(now)}
	names := make([]string, 0, len(set))
	for c := range set {
		names = append(names, c)
	}
	sort.Strings(names)
	for _, c := range names {
		cols = append(cols, c+" = ?")
		args = append(args, set[c])
	}
	if _, err := tx.Exec(`UPDATE chartransfers SET `+strings.Join(cols, ", ")+` WHERE id = ?`, append(args, id)...); err != nil {
		return Transfer{}, 0, err
	}
	after, err := fullRow(tx, `SELECT * FROM chartransfers WHERE id = ?`, id)
	if err != nil {
		return Transfer{}, 0, err
	}
	audit, err := auditRowChange(tx, who, "update", "chartransfers", map[string]any{"id": id}, before, after, summary)
	if err != nil {
		return Transfer{}, 0, err
	}
	out, err := scanTransfer(tx.QueryRow(`SELECT `+transferCols+` FROM chartransfers WHERE id = ?`, id))
	if err != nil {
		return Transfer{}, 0, err
	}
	return out, audit, tx.Commit()
}

// ApproveTransfer sets a waiting transfer's plan (charmove's JSON) and the
// GM's word to the player: it applies at the character's next login.
func (db *SQLiteDB) ApproveTransfer(id int64, plan, answer string, who GMEdit, now time.Time) (Transfer, int64, error) {
	answer = transferText(answer)
	if utf8.RuneCountInString(answer) > TransferMaxAnswer {
		return Transfer{}, 0, fmt.Errorf("the answer is %d characters at most", TransferMaxAnswer)
	}
	if strings.TrimSpace(plan) == "" {
		return Transfer{}, 0, errors.New("an approval needs a plan")
	}
	return db.decideTransfer(id, []string{TransferWaiting, TransferApproved}, TransferApproved,
		map[string]any{"plan": plan, "answer": answer, "decided": transferStamp(now), "decided_by": workBy(who)}, who,
		fmt.Sprintf("approved transfer %d", id), now)
}

// RejectTransfer says no to a waiting or approved transfer, with the GM's word
// to the player.
func (db *SQLiteDB) RejectTransfer(id int64, answer string, who GMEdit, now time.Time) (Transfer, int64, error) {
	answer = transferText(answer)
	if answer == "" {
		return Transfer{}, 0, errors.New("say why: the player reads it")
	}
	if utf8.RuneCountInString(answer) > TransferMaxAnswer {
		return Transfer{}, 0, fmt.Errorf("the answer is %d characters at most", TransferMaxAnswer)
	}
	return db.decideTransfer(id, []string{TransferWaiting, TransferApproved}, TransferRejected,
		map[string]any{"answer": answer, "decided": transferStamp(now), "decided_by": workBy(who)}, who,
		fmt.Sprintf("rejected transfer %d", id), now)
}

// MarkTransferUndone records that a GM took an applied transfer back.
func (db *SQLiteDB) MarkTransferUndone(id int64, answer string, who GMEdit, now time.Time) (Transfer, int64, error) {
	set := map[string]any{"decided": transferStamp(now), "decided_by": workBy(who)}
	if a := transferText(answer); a != "" {
		set["answer"] = a
	}
	return db.decideTransfer(id, []string{TransferApplied}, TransferUndone, set, who, fmt.Sprintf("undid transfer %d", id), now)
}

// ApprovedTransfer is the oldest approved transfer of a character, if any.
func (db *SQLiteDB) ApprovedTransfer(serverID uint32) (Transfer, bool, error) {
	t, err := scanTransfer(db.QueryRow(`SELECT `+transferCols+` FROM chartransfers WHERE serverid = ? AND status = ? ORDER BY id LIMIT 1`,
		serverID, TransferApproved))
	if errors.Is(err, sql.ErrNoRows) {
		return Transfer{}, false, nil
	}
	return t, err == nil, err
}

// MarkTransferApplied records the run an approved transfer applied as.
func (db *SQLiteDB) MarkTransferApplied(id int64, run string, now time.Time) error {
	stamp := transferStamp(now)
	res, err := db.Exec(`UPDATE chartransfers SET status = ?, run = ?, applied = ?, updated = ? WHERE id = ? AND status = ?`,
		TransferApplied, run, stamp, stamp, id, TransferApproved)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("transfer %d was not approved", id)
	}
	return nil
}

// TransfersToTell is a character's applied transfers its player has not been
// told of in the game; MarkTransferTold marks one told.
func (db *SQLiteDB) TransfersToTell(serverID uint32) ([]Transfer, error) {
	return db.queryTransfers(`SELECT `+transferCols+` FROM chartransfers WHERE serverid = ? AND status = ? AND told = 0`,
		serverID, TransferApplied)
}

func (db *SQLiteDB) MarkTransferTold(id int64) error {
	_, err := db.Exec(`UPDATE chartransfers SET told = 1, told_ready = 1 WHERE id = ?`, id)
	return err
}

// TransfersReadyToTell is an account's approved transfers it has not been
// told of (once, on any of its characters in the world); MarkTransferToldReady
// marks one told.
func (db *SQLiteDB) TransfersReadyToTell(accountID int) ([]Transfer, error) {
	return db.queryTransfers(`SELECT `+transferCols+` FROM chartransfers WHERE accountid = ? AND status = ? AND told_ready = 0 ORDER BY id`,
		accountID, TransferApproved)
}

// MarkTransferToldReady marks a transfer's approval told.
func (db *SQLiteDB) MarkTransferToldReady(id int64) error {
	_, err := db.Exec(`UPDATE chartransfers SET told_ready = 1 WHERE id = ?`, id)
	return err
}

// knownClaims is claims without the quests this server has no record of (a
// page older than the quests, a hand-made request).
func knownClaims(tx *sql.Tx, c TransferClaims) (TransferClaims, error) {
	if len(c.Quests) == 0 && len(c.Picks) == 0 {
		return c, nil
	}
	rows, err := tx.Query(`SELECT questid FROM quests`)
	if err != nil {
		return c, err
	}
	known := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return c, err
		}
		known[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return c, err
	}
	var qs []int
	for _, q := range c.Quests {
		if known[q] {
			qs = append(qs, q)
		}
	}
	var ps []ClaimPick
	for _, p := range c.Picks {
		if known[p.Quest] {
			ps = append(ps, p)
		}
	}
	c.Quests, c.Picks = qs, ps
	return c, nil
}

// --- the gear list, matched to this server's records

// ItemLine is one line of a player's gear list: the item as written, how many,
// and this server's records it may be, best first.
type ItemLine struct {
	Text       string          `json:"text"`
	Name       string          `json:"name"`
	Count      int             `json:"count"`
	Candidates []ItemCandidate `json:"candidates"`
	// Exact is true when the first candidate's name is the line's (any case,
	// apostrophes, hyphens and a final s aside, or our records' 32-character cut)
	Exact bool `json:"exact"`
}

// ItemCandidate is a record a line may be.
type ItemCandidate struct {
	PatternID int     `json:"patternid"`
	Name      string  `json:"name"`
	Slot      int     `json:"slot"`
	Level     int     `json:"level"`
	ClassUse  int     `json:"classuse"`
	Score     float64 `json:"score"`
}

var (
	reCountAfter  = regexp.MustCompile(`(?i)\s*(?:x\s*(\d{1,2})|\((\d{1,2})\)|\*\s*(\d{1,2}))\s*$`)
	reCountBefore = regexp.MustCompile(`(?i)^\s*(\d{1,2})\s*(?:x\b|\*)\s*`)
	reBullet      = regexp.MustCompile(`^\s*(?:[-*•·]+|\d{1,2}[.)])\s+`)
	reNoteAfter   = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
)

// itemWords is a name's words as matched: lower case, apostrophes gone,
// hyphens and other marks as spaces, a word's final s, ed or d off.
func itemWords(s string) []string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("'", "", "’", "", "`", "").Replace(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	for i, w := range words {
		for _, suf := range []string{"ed", "s", "d"} {
			if len(w) > 3 && strings.HasSuffix(w, suf) {
				w = strings.TrimSuffix(w, suf)
				break
			}
		}
		words[i] = w
	}
	return words
}

// ParseItemLine reads a gear list's line: its name and how many ("x2", "(2)",
// "2x" before it; a note in brackets at its end and a bullet dropped). ok is
// false for a line with no name.
func ParseItemLine(line string) (name string, count int, ok bool) {
	s := strings.TrimSpace(reBullet.ReplaceAllString(line, ""))
	count = 1
	if m := reCountBefore.FindStringSubmatch(s); m != nil {
		count, _ = strconv.Atoi(m[1])
		s = s[len(m[0]):]
	}
	for i := 0; i < 3; i++ { // "name x2 (a note)", "name (a note) x2", "name (2)"
		if m := reCountAfter.FindStringSubmatch(s); m != nil {
			for _, g := range m[1:] {
				if g != "" {
					count, _ = strconv.Atoi(g)
				}
			}
			s = strings.TrimSpace(s[:len(s)-len(m[0])])
			continue
		}
		if loc := reNoteAfter.FindStringIndex(s); loc != nil {
			s = strings.TrimSpace(s[:loc[0]])
			continue
		}
		break
	}
	s = strings.Trim(s, " ,;.")
	if count < 1 {
		count = 1
	}
	return s, min(count, 20), s != ""
}

// MatchItemLines matches each line of a gear list to this server's records:
// the records whose words are the line's (exact), then the 32-character cut of
// a longer name, then the most words in common (at most five, half the words
// at least).
func (db *SQLiteDB) MatchItemLines(text string) ([]ItemLine, error) {
	type rec struct {
		ItemCandidate
		words []string
		key   string
	}
	rows, err := db.Query(`SELECT patternid, itemname, equipslot, levelreq, classuse FROM itempattern`)
	if err != nil {
		return nil, err
	}
	var recs []rec
	byKey := map[string][]int{}
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.PatternID, &r.Name, &r.Slot, &r.Level, &r.ClassUse); err != nil {
			rows.Close()
			return nil, err
		}
		r.words = itemWords(r.Name)
		r.key = strings.Join(r.words, " ")
		byKey[r.key] = append(byKey[r.key], len(recs))
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ItemLine
	for _, raw := range strings.Split(text, "\n") {
		name, count, ok := ParseItemLine(raw)
		if !ok {
			continue
		}
		words := itemWords(name)
		key := strings.Join(words, " ")
		l := ItemLine{Text: strings.TrimSpace(raw), Name: name, Count: count, Candidates: []ItemCandidate{}}
		seen := map[int]bool{}
		add := func(i int, score float64) {
			if seen[recs[i].PatternID] {
				return
			}
			seen[recs[i].PatternID] = true
			c := recs[i].ItemCandidate
			c.Score = score
			l.Candidates = append(l.Candidates, c)
		}
		for _, i := range byKey[key] {
			add(i, 1)
		}
		l.Exact = len(l.Candidates) > 0
		if !l.Exact {
			lower := strings.ToLower(name)
			for i, r := range recs {
				// a name our records cut at 32 characters
				if utf8.RuneCountInString(r.Name) == 32 && strings.HasPrefix(lower, strings.ToLower(r.Name)) {
					add(i, 0.99)
				}
			}
			l.Exact = len(l.Candidates) > 0
		}
		var fuzzy []struct {
			i     int
			score float64
		}
		if len(words) > 0 {
			want := map[string]bool{}
			for _, w := range words {
				want[w] = true
			}
			for i, r := range recs {
				if seen[r.PatternID] || len(r.words) == 0 {
					continue
				}
				common := 0
				for _, w := range r.words {
					if want[w] {
						common++
					}
				}
				if common == 0 {
					continue
				}
				score := 2 * float64(common) / float64(len(words)+len(r.words))
				if score >= 0.5 {
					fuzzy = append(fuzzy, struct {
						i     int
						score float64
					}{i, score})
				}
			}
		}
		sort.SliceStable(fuzzy, func(a, b int) bool {
			if fuzzy[a].score != fuzzy[b].score {
				return fuzzy[a].score > fuzzy[b].score
			}
			return recs[fuzzy[a].i].PatternID < recs[fuzzy[b].i].PatternID
		})
		for _, f := range fuzzy {
			if len(l.Candidates) >= 6 {
				break
			}
			add(f.i, f.score)
		}
		out = append(out, l)
	}
	return out, nil
}

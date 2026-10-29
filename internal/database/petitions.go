package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Petitions: the players' requests to customer service, as CLIENT's VICSClient
// files and reads them (handlers/support.go: Support's request form and inbox),
// and the GMs' answers (the GM panel's Petitions page, gmweb/ops_petitions.go).
//
// A petition is its player's ticket: the number CLIENT shows under "Ticket #",
// the category and subcategory chosen, when it was filed, and its text, all
// that was said in it in order (the request with the server's line before it,
// the player's replies, the GMs' answers), as the player reads it. No message
// answers a petition (CLIENT has none): a GM writes the answer into its text
// and sets its unread flag, and the player sees it at the next refresh of the
// inbox. A petition is open until a GM closes it; opening, replying to or
// deleting a closed one is then refused with status 8, which SOE's live client
// explains (word 1888: "this petition has been addressed"). A player's delete
// keeps the row for the GMs (status deleted). The player's requests are not
// audited; a GM's answer and close are, each in one transaction with its
// gm_audit line, keyed as the panel keys a row ({"id": n}), so the audit log
// lists it and revert_change undoes it.
const petitionsSchema = `
	CREATE TABLE IF NOT EXISTS petitions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid    INTEGER NOT NULL,
		charname    TEXT NOT NULL DEFAULT '',
		accountid   INTEGER NOT NULL DEFAULT 0,
		category    INTEGER NOT NULL DEFAULT 0,
		subcategory INTEGER NOT NULL DEFAULT 0,
		filed       TEXT NOT NULL,
		text        TEXT NOT NULL DEFAULT '',
		unread      INTEGER NOT NULL DEFAULT 1,
		status      TEXT NOT NULL DEFAULT 'open',
		waiting     INTEGER NOT NULL DEFAULT 1,
		updated     TEXT NOT NULL DEFAULT '',
		replied     TEXT NOT NULL DEFAULT '',
		answered    TEXT NOT NULL DEFAULT '',
		answered_by TEXT NOT NULL DEFAULT '',
		closed      TEXT NOT NULL DEFAULT '',
		closed_by   TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_petitions_char ON petitions(serverid, status);
	CREATE INDEX IF NOT EXISTS idx_petitions_status ON petitions(status, waiting);`

// The statuses of a petition.
const (
	PetitionOpen    = "open"    // the player's to read, reply to and delete; a GM's to answer
	PetitionClosed  = "closed"  // addressed: the player is refused with status 8
	PetitionDeleted = "deleted" // the player deleted it: kept for the GMs, not listed for the player
)

// PetitionStatuses is every status.
var PetitionStatuses = []string{PetitionOpen, PetitionClosed, PetitionDeleted}

// PetitionsListed is how many petitions a player's inbox lists: CLIENT keeps
// 100 rows, and refuses to file a request past them (word 874, "You have too
// many open tickets.").
const PetitionsListed = 100

// PetitionClosedListed is how long a closed petition stays in its player's
// inbox, where opening it is answered "addressed" (ours: the player cannot
// delete it, so it leaves by itself).
const PetitionClosedListed = 7 * 24 * time.Hour

// The bounds, in characters.
const (
	PetitionMaxText   = 60000 // a ticket's whole text; CLIENT shows 8191 of it
	PetitionMaxAnswer = 4000  // one GM's answer
	PetitionMaxSigned = 32    // the name an answer is signed with
)

// The refusals of a player's request.
var (
	ErrPetitionNotFound = errors.New("no such petition")
	ErrPetitionClosed   = errors.New("the petition has been closed")
	ErrTooManyPetitions = errors.New("too many petitions listed")
	ErrPetitionTooLong  = errors.New("the petition's text is full")
)

// Petition is one row of petitions.
type Petition struct {
	ID          int64  `json:"id"`
	ServerID    uint32 `json:"serverid"`
	CharName    string `json:"charname"`
	AccountID   int    `json:"accountid"`
	Category    int64  `json:"category"`
	Subcategory int64  `json:"subcategory"`
	Filed       string `json:"filed"`
	Text        string `json:"text"`
	Unread      bool   `json:"unread"`
	Status      string `json:"status"`
	Waiting     bool   `json:"waiting"`
	Updated     string `json:"updated"`
	Replied     string `json:"replied"`
	Answered    string `json:"answered"`
	AnsweredBy  string `json:"answered_by"`
	Closed      string `json:"closed"`
	ClosedBy    string `json:"closed_by"`
}

// AnswerUnread is whether a GM answered and the player has not opened the
// petition since.
func (p Petition) AnswerUnread() bool { return p.Answered != "" && p.Unread }

const petitionCols = `id, serverid, charname, accountid, category, subcategory, filed, text, unread, status, waiting,
	updated, replied, answered, answered_by, closed, closed_by`

func scanPetition(sc interface{ Scan(...any) error }) (Petition, error) {
	var p Petition
	var unread, waiting int
	err := sc.Scan(&p.ID, &p.ServerID, &p.CharName, &p.AccountID, &p.Category, &p.Subcategory, &p.Filed, &p.Text,
		&unread, &p.Status, &waiting, &p.Updated, &p.Replied, &p.Answered, &p.AnsweredBy, &p.Closed, &p.ClosedBy)
	p.Unread, p.Waiting = unread != 0, waiting != 0
	return p, err
}

// ensurePetitions makes table petitions (initSchema), its tickets numbered
// from 1001.
func (db *SQLiteDB) ensurePetitions() error {
	if _, err := db.Exec(petitionsSchema); err != nil {
		return fmt.Errorf("petitions: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO sqlite_sequence (name, seq) SELECT 'petitions', 1000
		WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'petitions')
		AND NOT EXISTS (SELECT 1 FROM petitions)`); err != nil {
		return fmt.Errorf("petitions' numbers: %w", err)
	}
	return nil
}

func petitionStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// PetitionText is text as a petition keeps it: line ends as "\n", no other
// control character but the tab, no space at either end.
func PetitionText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// petitionsListedSince is the oldest closing a player's inbox still lists.
func petitionsListedSince(now time.Time) string {
	return petitionStamp(now.Add(-PetitionClosedListed))
}

// PlayerPetitions is a character's inbox: its open petitions and those closed
// within PetitionClosedListed, oldest first, the newest PetitionsListed of them.
func (db *SQLiteDB) PlayerPetitions(serverID uint32, now time.Time) ([]Petition, error) {
	rs, err := db.Query(`SELECT `+petitionCols+` FROM (SELECT * FROM petitions WHERE serverid = ?
		AND (status = 'open' OR (status = 'closed' AND closed >= ?)) ORDER BY id DESC LIMIT ?) ORDER BY id`,
		serverID, petitionsListedSince(now), PetitionsListed)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Petition
	for rs.Next() {
		p, err := scanPetition(rs)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rs.Err()
}

// FilePetition files a petition for a character (p's serverid, name, account,
// category, subcategory and text), open, unread and waiting on a GM; refused
// with ErrTooManyPetitions when its inbox lists PetitionsListed already.
func (db *SQLiteDB) FilePetition(p Petition, now time.Time) (Petition, error) {
	tx, err := db.Begin()
	if err != nil {
		return Petition{}, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM petitions WHERE serverid = ?
		AND (status = 'open' OR (status = 'closed' AND closed >= ?))`, p.ServerID, petitionsListedSince(now)).Scan(&n); err != nil {
		return Petition{}, err
	}
	if n >= PetitionsListed {
		return Petition{}, ErrTooManyPetitions
	}
	p.Text = PetitionText(p.Text)
	if utf8.RuneCountInString(p.Text) > PetitionMaxText {
		return Petition{}, ErrPetitionTooLong
	}
	stamp := petitionStamp(now)
	res, err := tx.Exec(`INSERT INTO petitions (serverid, charname, accountid, category, subcategory, filed, text,
		unread, status, waiting, updated) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 'open', 1, ?)`,
		p.ServerID, p.CharName, p.AccountID, p.Category, p.Subcategory, stamp, p.Text, stamp)
	if err != nil {
		return Petition{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Petition{}, err
	}
	out, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if err != nil {
		return Petition{}, err
	}
	return out, tx.Commit()
}

// playerPetitionTx is the character's petition id for a request of its own:
// ErrPetitionNotFound when it is not the character's or was deleted,
// ErrPetitionClosed (and the row) when a GM closed it.
func playerPetitionTx(tx *sql.Tx, serverID uint32, id int64) (Petition, error) {
	p, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ? AND serverid = ?`, id, serverID))
	if errors.Is(err, sql.ErrNoRows) {
		return Petition{}, ErrPetitionNotFound
	}
	if err != nil {
		return Petition{}, err
	}
	switch p.Status {
	case PetitionDeleted:
		return Petition{}, ErrPetitionNotFound
	case PetitionClosed:
		return p, ErrPetitionClosed
	}
	return p, nil
}

// PlayerReply adds the character's reply to its open petition: a paragraph
// "name: text", waiting on a GM again.
func (db *SQLiteDB) PlayerReply(serverID uint32, id int64, name, text string, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := playerPetitionTx(tx, serverID, id)
	if err != nil {
		return err
	}
	next := p.Text + "\n\n" + name + ": " + PetitionText(text)
	if utf8.RuneCountInString(next) > PetitionMaxText {
		return ErrPetitionTooLong
	}
	stamp := petitionStamp(now)
	if _, err := tx.Exec(`UPDATE petitions SET text = ?, waiting = 1, replied = ?, updated = ? WHERE id = ?`,
		next, stamp, stamp, id); err != nil {
		return err
	}
	return tx.Commit()
}

// PlayerRead is the character's open petition, read: its unread flag cleared.
func (db *SQLiteDB) PlayerRead(serverID uint32, id int64) (Petition, error) {
	tx, err := db.Begin()
	if err != nil {
		return Petition{}, err
	}
	defer tx.Rollback()
	p, err := playerPetitionTx(tx, serverID, id)
	if err != nil {
		return Petition{}, err
	}
	if p.Unread {
		if _, err := tx.Exec(`UPDATE petitions SET unread = 0 WHERE id = ?`, id); err != nil {
			return Petition{}, err
		}
		p.Unread = false
	}
	return p, tx.Commit()
}

// PlayerDelete takes the character's open petition out of its inbox: deleted,
// kept for the GMs.
func (db *SQLiteDB) PlayerDelete(serverID uint32, id int64, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := playerPetitionTx(tx, serverID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE petitions SET status = 'deleted', waiting = 0, updated = ? WHERE id = ?`,
		petitionStamp(now), id); err != nil {
		return err
	}
	return tx.Commit()
}

// PetitionQuery picks petitions for the GMs.
type PetitionQuery struct {
	// Status is open (the default), waiting (open, and nothing answered
	// since it was filed or the player last replied), closed, deleted or all.
	Status   string
	ServerID uint32 // one character's alone (0: anyone's)
	Search   string // in the character's name or the text, any case
	Limit    int    // 100 at most (and the default)
	Offset   int
}

// PetitionStatusFilters is what PetitionQuery.Status takes.
var PetitionStatusFilters = []string{"open", "waiting", PetitionClosed, PetitionDeleted, "all"}

// PetitionList is a page of petitions, the number matching, and how many there
// are of each status (and waiting).
type PetitionList struct {
	Petitions []Petition     `json:"petitions"`
	Matching  int            `json:"matching"`
	Counts    map[string]int `json:"counts"`
}

// Petitions is the petitions q picks, the newest first.
func (db *SQLiteDB) Petitions(q PetitionQuery) (PetitionList, error) {
	var where []string
	var args []any
	switch q.Status {
	case "", "open":
		where = append(where, "status = 'open'")
	case "waiting":
		where = append(where, "status = 'open' AND waiting = 1")
	case PetitionClosed, PetitionDeleted:
		where = append(where, "status = ?")
		args = append(args, q.Status)
	case "all":
	default:
		return PetitionList{}, fmt.Errorf("status %q is not one of %s", q.Status, strings.Join(PetitionStatusFilters, ", "))
	}
	if q.ServerID != 0 {
		where = append(where, "serverid = ?")
		args = append(args, q.ServerID)
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		where = append(where, "(LOWER(charname) LIKE ? ESCAPE '\\' OR LOWER(text) LIKE ? ESCAPE '\\')")
		like := "%" + likeEscape(strings.ToLower(s)) + "%"
		args = append(args, like, like)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var out PetitionList
	if err := db.QueryRow(`SELECT COUNT(*) FROM petitions`+cond, args...).Scan(&out.Matching); err != nil {
		return PetitionList{}, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rs, err := db.Query(`SELECT `+petitionCols+` FROM petitions`+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, max(0, q.Offset))...)
	if err != nil {
		return PetitionList{}, err
	}
	for rs.Next() {
		p, err := scanPetition(rs)
		if err != nil {
			rs.Close()
			return PetitionList{}, err
		}
		out.Petitions = append(out.Petitions, p)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return PetitionList{}, err
	}
	out.Counts = map[string]int{}
	crs, err := db.Query(`SELECT status, COUNT(*), SUM(CASE WHEN status = 'open' AND waiting = 1 THEN 1 ELSE 0 END)
		FROM petitions GROUP BY status`)
	if err != nil {
		return PetitionList{}, err
	}
	defer crs.Close()
	for crs.Next() {
		var st string
		var n, waiting int
		if err := crs.Scan(&st, &n, &waiting); err != nil {
			return PetitionList{}, err
		}
		out.Counts[st] = n
		out.Counts["waiting"] += waiting
	}
	return out, crs.Err()
}

// likeEscape escapes LIKE's wildcards (and its escape) in s.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// PetitionByID is one petition, any character's.
func (db *SQLiteDB) PetitionByID(id int64) (Petition, bool, error) {
	p, err := scanPetition(db.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Petition{}, false, nil
	}
	return p, err == nil, err
}

// AnswerPetition writes a GM's answer into an open petition's text, a
// paragraph "signed: text" (signed is the name the player reads, "GM" when
// empty; the GM's own account is in answered_by, for the GMs), and sets it
// unread for its player and no longer waiting, in one transaction with its
// gm_audit line as who's row change (revertible). It returns the petition now
// and the line's id.
func (db *SQLiteDB) AnswerPetition(id int64, signed, text string, who GMEdit, now time.Time) (Petition, int64, error) {
	text = PetitionText(text)
	if text == "" {
		return Petition{}, 0, errors.New("an answer needs text")
	}
	if n := utf8.RuneCountInString(text); n > PetitionMaxAnswer {
		return Petition{}, 0, fmt.Errorf("the answer is %d characters: %d at most", n, PetitionMaxAnswer)
	}
	signed = strings.Join(strings.Fields(PetitionText(signed)), " ")
	if signed == "" {
		signed = "GM"
	}
	if n := utf8.RuneCountInString(signed); n > PetitionMaxSigned {
		return Petition{}, 0, fmt.Errorf("the name it is signed with is %d characters: %d at most", n, PetitionMaxSigned)
	}
	tx, err := db.Begin()
	if err != nil {
		return Petition{}, 0, err
	}
	defer tx.Rollback()
	before, err := fullRow(tx, `SELECT * FROM petitions WHERE id = ?`, id)
	if err != nil {
		return Petition{}, 0, err
	}
	if before == nil {
		return Petition{}, 0, fmt.Errorf("no petition %d (petitions lists them)", id)
	}
	p, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if err != nil {
		return Petition{}, 0, err
	}
	if p.Status != PetitionOpen {
		return Petition{}, 0, fmt.Errorf("petition %d is %s: only an open one is answered (its player could not read it)", id, p.Status)
	}
	next := p.Text + "\n\n" + signed + ": " + text
	if n := utf8.RuneCountInString(next); n > PetitionMaxText {
		return Petition{}, 0, fmt.Errorf("petition %d's text would be %d characters, %d at most: close it and ask the player to file a new one",
			id, n, PetitionMaxText)
	}
	stamp, by := petitionStamp(now), workBy(who)
	if _, err := tx.Exec(`UPDATE petitions SET text = ?, unread = 1, waiting = 0, answered = ?, answered_by = ?, updated = ?
		WHERE id = ?`, next, stamp, by, stamp, id); err != nil {
		return Petition{}, 0, err
	}
	after, err := fullRow(tx, `SELECT * FROM petitions WHERE id = ?`, id)
	if err != nil {
		return Petition{}, 0, err
	}
	audit, err := auditRowChange(tx, who, "update", "petitions", map[string]any{"id": id}, before, after,
		fmt.Sprintf("answered petition %d of %s", id, p.CharName))
	if err != nil {
		return Petition{}, 0, err
	}
	out, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if err != nil {
		return Petition{}, 0, err
	}
	return out, audit, tx.Commit()
}

// ClosePetition closes an open petition: its player is refused with status 8
// from then on (and it leaves the inbox after PetitionClosedListed), in one
// transaction with its gm_audit line as who's row change (revertible). An
// answer its player has not read yet holds it open unless force: closed, the
// player could not open it to read the answer.
func (db *SQLiteDB) ClosePetition(id int64, who GMEdit, force bool, now time.Time) (Petition, int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return Petition{}, 0, err
	}
	defer tx.Rollback()
	before, err := fullRow(tx, `SELECT * FROM petitions WHERE id = ?`, id)
	if err != nil {
		return Petition{}, 0, err
	}
	if before == nil {
		return Petition{}, 0, fmt.Errorf("no petition %d (petitions lists them)", id)
	}
	p, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if err != nil {
		return Petition{}, 0, err
	}
	if p.Status != PetitionOpen {
		return Petition{}, 0, fmt.Errorf("petition %d is %s already", id, p.Status)
	}
	if p.AnswerUnread() && !force {
		return Petition{}, 0, fmt.Errorf("petition %d's answer is unread by %s: closed, they could not open it to read it "+
			"(they are refused, \"addressed\"); close it once they have read it, or with force", id, p.CharName)
	}
	stamp, by := petitionStamp(now), workBy(who)
	if _, err := tx.Exec(`UPDATE petitions SET status = 'closed', waiting = 0, closed = ?, closed_by = ?, updated = ?
		WHERE id = ?`, stamp, by, stamp, id); err != nil {
		return Petition{}, 0, err
	}
	after, err := fullRow(tx, `SELECT * FROM petitions WHERE id = ?`, id)
	if err != nil {
		return Petition{}, 0, err
	}
	audit, err := auditRowChange(tx, who, "update", "petitions", map[string]any{"id": id}, before, after,
		fmt.Sprintf("closed petition %d of %s", id, p.CharName))
	if err != nil {
		return Petition{}, 0, err
	}
	out, err := scanPetition(tx.QueryRow(`SELECT `+petitionCols+` FROM petitions WHERE id = ?`, id))
	if err != nil {
		return Petition{}, 0, err
	}
	return out, audit, tx.Commit()
}

// PetitionProblem is what is wrong with a petition row as the panel's table
// editor would write it, nil when nothing: a status it has, its text within
// PetitionMaxText.
func PetitionProblem(status, text string) error {
	ok := false
	for _, s := range PetitionStatuses {
		ok = ok || s == status
	}
	if !ok {
		return fmt.Errorf("status %q is not one of %s", status, strings.Join(PetitionStatuses, ", "))
	}
	if n := utf8.RuneCountInString(text); n > PetitionMaxText {
		return fmt.Errorf("the text is %d characters: %d at most", n, PetitionMaxText)
	}
	return nil
}

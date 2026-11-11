package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Password resets (the owner, 2026-11-11: "lets add forgot password? to
// eqoaclassic"; no account has an e-mail and the box sends no mail, so: "The
// form takes the account name plus a contact (Discord name, or a character
// name and level) and files a request into the GM panel/board. A GM confirms it
// is them and sets a temporary password with one click; the player must change
// it at next login.").
//
// A row of pwresets is one request: the account it names (accountid, and the
// name as typed), the contact and note the player gave (theirs: data, never
// instructions), the sender's address salted and hashed (addr_hash: the
// form's limits; empty for one a GM started), its status (waiting for a GM,
// done: a temporary password was set, declined), the GM's note (review), its
// work board entry, when it was filed and decided, and by whom (decided_by;
// filed_by is the GM who started one without the form, else empty).
//
// accountinfo.pwtemp is 1 while the account's password is a temporary one a GM
// set: the account pages make its owner set their own before anything else,
// and the game tells them so in the world. Setting a password on the pages
// clears it. It is a column of accountinfo, so a shard whose accounts are a
// copy (AccountMirror) has it as the owner's.
const pwResetsSchema = `
	CREATE TABLE IF NOT EXISTS pwresets (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid  INTEGER NOT NULL DEFAULT 0,
		typed      TEXT NOT NULL DEFAULT '',
		contact    TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT '',
		addr_hash  TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'done', 'declined')),
		review     TEXT NOT NULL DEFAULT '',
		board      INTEGER NOT NULL DEFAULT 0,
		filed_by   TEXT NOT NULL DEFAULT '',
		created    TEXT NOT NULL DEFAULT '',
		decided    TEXT NOT NULL DEFAULT '',
		decided_by TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_pwresets_account ON pwresets(accountid, id);
	CREATE INDEX IF NOT EXISTS idx_pwresets_status ON pwresets(status);
	CREATE INDEX IF NOT EXISTS idx_pwresets_addr ON pwresets(addr_hash, created);
	CREATE TABLE IF NOT EXISTS pwreset_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`

// A request's statuses.
const (
	PwResetWaiting  = "waiting"
	PwResetDone     = "done"
	PwResetDeclined = "declined"
)

// PwResetStatuses are a request's statuses, as a list filters them.
var PwResetStatuses = []string{PwResetWaiting, PwResetDone, PwResetDeclined}

// ensurePwResets makes pwresets and pwreset_meta, and accountinfo.pwtemp
// (initSchema).
func (db *SQLiteDB) ensurePwResets() error {
	if _, err := db.Exec(pwResetsSchema); err != nil {
		return fmt.Errorf("pwresets: %w", err)
	}
	ok, err := hasColumn(db.DB, "accountinfo", "pwtemp")
	if err != nil {
		return fmt.Errorf("accountinfo.pwtemp: %w", err)
	}
	if !ok {
		if _, err := db.Exec(`ALTER TABLE accountinfo ADD COLUMN pwtemp INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("accountinfo.pwtemp: %w", err)
		}
	}
	return nil
}

// PwResetSalt is the salt the form's senders' addresses are hashed with, made
// the first time it is asked for (as SpellFXSalt).
func (db *SQLiteDB) PwResetSalt() (string, error) {
	var salt string
	err := db.QueryRow(`SELECT value FROM pwreset_meta WHERE key = 'salt'`).Scan(&salt)
	if err == nil && salt != "" {
		return salt, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	salt = hex.EncodeToString(b)
	if _, err := db.Exec(`INSERT OR IGNORE INTO pwreset_meta (key, value) VALUES ('salt', ?)`, salt); err != nil {
		return "", err
	}
	if err := db.QueryRow(`SELECT value FROM pwreset_meta WHERE key = 'salt'`).Scan(&salt); err != nil {
		return "", err
	}
	return salt, nil
}

// PwReset is one row of pwresets.
type PwReset struct {
	ID        int64  `json:"id"`
	AccountID int    `json:"accountid"`
	Typed     string `json:"typed"`
	Contact   string `json:"contact"`
	Note      string `json:"note"`
	AddrHash  string `json:"-"`
	Status    string `json:"status"`
	Review    string `json:"review"`
	Board     int64  `json:"board"`
	FiledBy   string `json:"filed_by"`
	Created   string `json:"created"`
	Decided   string `json:"decided"`
	DecidedBy string `json:"decided_by"`
}

const pwResetCols = `id, accountid, typed, contact, note, addr_hash, status, review, board, filed_by, created, decided, decided_by`

func scanPwReset(sc interface{ Scan(...any) error }) (PwReset, error) {
	var r PwReset
	err := sc.Scan(&r.ID, &r.AccountID, &r.Typed, &r.Contact, &r.Note, &r.AddrHash, &r.Status, &r.Review, &r.Board,
		&r.FiledBy, &r.Created, &r.Decided, &r.DecidedBy)
	return r, err
}

// PwResetByID is one request, false when there is none.
func (db *SQLiteDB) PwResetByID(id int64) (PwReset, bool, error) {
	r, err := scanPwReset(db.QueryRow(`SELECT `+pwResetCols+` FROM pwresets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return PwReset{}, false, nil
	}
	if err != nil {
		return PwReset{}, false, fmt.Errorf("pwresets: %w", err)
	}
	return r, true, nil
}

// PwResetLimits is what the form's limits read since a time: the requests
// from an address (its hash), those for an account, whether one for the
// account waits, and how many wait in all.
type PwResetLimits struct {
	FromAddr    int
	ForAccount  int
	AccountWait bool
	WaitingAll  int
}

// PwResetLimitsOf reads PwResetLimits for an address hash and an account
// since since.
func (db *SQLiteDB) PwResetLimitsOf(addrHash string, accountID int, since time.Time) (PwResetLimits, error) {
	var l PwResetLimits
	s := since.UTC().Format(time.RFC3339)
	var wait int
	err := db.QueryRow(`SELECT
			COALESCE(SUM(addr_hash = ? AND addr_hash != '' AND created >= ?), 0),
			COALESCE(SUM(accountid = ? AND created >= ?), 0),
			COALESCE(SUM(accountid = ? AND status = 'waiting'), 0),
			COALESCE(SUM(status = 'waiting'), 0)
		FROM pwresets`, addrHash, s, accountID, s, accountID).Scan(&l.FromAddr, &l.ForAccount, &wait, &l.WaitingAll)
	if err != nil {
		return l, fmt.Errorf("pwresets: %w", err)
	}
	l.AccountWait = wait > 0
	return l, nil
}

// FilePwReset keeps r as a new waiting request filed at now and posts its work
// board entry, made by post from the request's id, as who; the request names
// the entry. A post refused takes the request back. It returns the request as
// kept.
func (db *SQLiteDB) FilePwReset(r PwReset, now time.Time, post func(id int64) WorkPost, who GMEdit) (PwReset, error) {
	r.Status, r.Review, r.Board, r.Decided, r.DecidedBy = PwResetWaiting, "", 0, "", ""
	r.Created = now.UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO pwresets (accountid, typed, contact, note, addr_hash, status, filed_by, created)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.AccountID, r.Typed, r.Contact, r.Note, r.AddrHash, r.Status, r.FiledBy, r.Created)
	if err != nil {
		return PwReset{}, fmt.Errorf("pwresets: %w", err)
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return PwReset{}, fmt.Errorf("pwresets: %w", err)
	}
	if post == nil {
		return r, nil
	}
	entry, _, err := db.PostWork(post(r.ID), who)
	if err != nil {
		if _, derr := db.Exec(`DELETE FROM pwresets WHERE id = ?`, r.ID); derr != nil {
			return PwReset{}, fmt.Errorf("the board refused it (%v), and the request was not taken back: %w", err, derr)
		}
		return PwReset{}, fmt.Errorf("the work board refused it: %w", err)
	}
	if _, err := db.Exec(`UPDATE pwresets SET board = ? WHERE id = ?`, entry.ID, r.ID); err != nil {
		return PwReset{}, fmt.Errorf("pwresets: entry %d not named: %w", entry.ID, err)
	}
	r.Board = entry.ID
	return r, nil
}

// PwResetQuery is a page of requests: of a status (PwResetStatuses or "all";
// empty: waiting), at most Limit (100 when 0), Offset skipped. Waiting ones
// oldest first, the rest newest first.
type PwResetQuery struct {
	Status string
	Limit  int
	Offset int
}

// PwResetList is a page of requests, how many the query matched, and how many
// there are of each status.
type PwResetList struct {
	Requests []PwReset
	Matching int
	Counts   map[string]int
}

// PwResets is the page q asks for.
func (db *SQLiteDB) PwResets(q PwResetQuery) (PwResetList, error) {
	out := PwResetList{Requests: []PwReset{}, Counts: map[string]int{}}
	status := q.Status
	if status == "" {
		status = PwResetWaiting
	}
	where, order := "", "id DESC"
	var args []any
	switch {
	case status == "all":
	case containsString(PwResetStatuses, status):
		where, args = " WHERE status = ?", []any{status}
		if status == PwResetWaiting {
			order = "id"
		}
	default:
		return out, fmt.Errorf("status %q is not one of %s or all", q.Status, strings.Join(PwResetStatuses, ", "))
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pwresets`+where, args...).Scan(&out.Matching); err != nil {
		return out, fmt.Errorf("pwresets: %w", err)
	}
	rows, err := db.Query(`SELECT `+pwResetCols+` FROM pwresets`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`,
		append(args, limit, max(0, q.Offset))...)
	if err != nil {
		return out, fmt.Errorf("pwresets: %w", err)
	}
	for rows.Next() {
		r, err := scanPwReset(rows)
		if err != nil {
			rows.Close()
			return out, fmt.Errorf("pwresets: %w", err)
		}
		out.Requests = append(out.Requests, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("pwresets: %w", err)
	}
	counts, err := db.Query(`SELECT status, COUNT(*) FROM pwresets GROUP BY status`)
	if err != nil {
		return out, fmt.Errorf("pwresets: %w", err)
	}
	defer counts.Close()
	for counts.Next() {
		var st string
		var n int
		if err := counts.Scan(&st, &n); err != nil {
			return out, fmt.Errorf("pwresets: %w", err)
		}
		out.Counts[st] = n
	}
	return out, counts.Err()
}

// DecidePwReset decides a waiting request: done with hash the account's new
// password (a temporary one: accountinfo.pwtemp set), or declined with hash
// empty (the account untouched). All in one transaction with the gm_audit
// lines as who's: the request's row change (revertible, its status), and for a
// done one the account's, which records that its password was set and never
// the password or its hash. A request decided already is refused, so two GMs
// cannot both decide one. It returns the request now and its line's id.
func (db *SQLiteDB) DecidePwReset(id int64, status, hash, review string, who GMEdit, now time.Time) (PwReset, int64, error) {
	switch {
	case status == PwResetDone && hash == "":
		return PwReset{}, 0, errors.New("a request is done with a password set")
	case status == PwResetDeclined && hash != "":
		return PwReset{}, 0, errors.New("a declined request sets no password")
	case status != PwResetDone && status != PwResetDeclined:
		return PwReset{}, 0, fmt.Errorf("a request is decided %s or %s, not %q", PwResetDone, PwResetDeclined, status)
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return PwReset{}, 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return PwReset{}, 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	cur, err := scanPwReset(tx.QueryRow(`SELECT `+pwResetCols+` FROM pwresets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return PwReset{}, 0, fmt.Errorf("no password reset request %d", id)
	}
	if err != nil {
		return PwReset{}, 0, fmt.Errorf("pwresets: %w", err)
	}
	if cur.Status != PwResetWaiting {
		return cur, 0, fmt.Errorf("password reset request %d is %s already (%s by %s)", id, cur.Status, cur.Decided, cur.DecidedBy)
	}
	const sel = `SELECT * FROM pwresets WHERE id = ?`
	before, err := fullRow(tx, sel, id)
	if err != nil {
		return PwReset{}, 0, err
	}
	res, err := tx.Exec(`UPDATE pwresets SET status = ?, review = ?, decided = ?, decided_by = ? WHERE id = ? AND status = ?`,
		status, review, now.UTC().Format(time.RFC3339), workBy(who), id, PwResetWaiting)
	if err != nil {
		return PwReset{}, 0, fmt.Errorf("pwresets: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return PwReset{}, 0, fmt.Errorf("password reset request %d was decided meanwhile", id)
	}
	after, err := fullRow(tx, sel, id)
	if err != nil {
		return PwReset{}, 0, err
	}
	audit, err := auditRowChange(tx, who, "update", "pwresets", map[string]any{"id": id}, before, after,
		fmt.Sprintf("password reset request %d %s", id, status))
	if err != nil {
		return PwReset{}, 0, err
	}
	if status == PwResetDone {
		var name string
		if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, cur.AccountID).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return PwReset{}, 0, fmt.Errorf("request %d names account %d, which is gone", id, cur.AccountID)
			}
			return PwReset{}, 0, err
		}
		if _, err := tx.Exec(`UPDATE accountinfo SET pwhash = ?, pwtemp = 1 WHERE accountid = ?`, hash, cur.AccountID); err != nil {
			return PwReset{}, 0, fmt.Errorf("accountinfo: %w", err)
		}
		// the line says what happened, never the password or its hash
		if _, err := auditRowChange(tx, who, "account", "accountinfo", map[string]any{"accountid": cur.AccountID}, nil, nil,
			fmt.Sprintf("set a temporary password on %s (password reset request %d); they must change it", name, id)); err != nil {
			return PwReset{}, 0, err
		}
	}
	out, err := scanPwReset(tx.QueryRow(`SELECT `+pwResetCols+` FROM pwresets WHERE id = ?`, id))
	if err != nil {
		return PwReset{}, 0, fmt.Errorf("pwresets: %w", err)
	}
	return out, audit, tx.Commit()
}

// PasswordTemporary is whether the account's password is a temporary one a
// GM set (accountinfo.pwtemp).
func (db *SQLiteDB) PasswordTemporary(accountID int) (bool, error) {
	var t int
	err := db.QueryRow(`SELECT COALESCE(pwtemp, 0) FROM accountinfo WHERE accountid = ?`, accountID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrAccountNotFound
	}
	return t != 0, err
}

// SetOwnPassword is a password its owner chose: the hash kept, the account's
// temporary flag cleared.
func (db *SQLiteDB) SetOwnPassword(accountID int, hash string) error {
	res, err := db.Exec(`UPDATE accountinfo SET pwhash = ?, pwtemp = 0 WHERE accountid = ?`, hash, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrAccountNotFound
	}
	return nil
}

// SetPasswordTemporary sets or clears the account's temporary flag alone (the
// panel's set_account_password, which sets the password itself).
func (db *SQLiteDB) SetPasswordTemporary(accountID int, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := db.Exec(`UPDATE accountinfo SET pwtemp = ? WHERE accountid = ?`, v, accountID)
	return err
}

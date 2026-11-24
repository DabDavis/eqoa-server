package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The accounts' e-mail (the owner, 2026-11-15: "lets add email verification
// to accounts, so we can make sure players dont lose their account. and we can
// setup email password reset"; internal/accountmail).
//
// accountinfo.email is the address as kept (mailer.Normalize: trimmed, lower
// case), email_verified_at when it was confirmed (RFC 3339, UTC; ” while it is
// not) and email_verified_by how: "link" (its owner opened the link mailed to
// it) or "GM <name>" (a GM's override, for a player who cannot get the mail;
// the address may then be empty). Changing the address clears both. They are
// columns of accountinfo, so a shard whose accounts are a copy (AccountMirror)
// has them as the owner's.
//
// email_tokens is the links mailed: each a random token of which only the
// SHA-256 is kept, for one account and the address it went to, a kind
// (verify: 24 hours; reset, a new password: 1 hour), when it was made, when it
// ends and when it was used. A token is good once, before it ends, while the
// account's address is still the one it went to. Its rows are also what the
// sends are counted by (the resend limits).
const emailVerifySchema = `
	CREATE TABLE IF NOT EXISTS email_tokens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid  INTEGER NOT NULL,
		kind       TEXT NOT NULL CHECK (kind IN ('verify', 'reset')),
		token_hash TEXT NOT NULL UNIQUE,
		email      TEXT NOT NULL DEFAULT '',
		created    TEXT NOT NULL,
		expires    TEXT NOT NULL,
		used       TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_email_tokens_account ON email_tokens(accountid, kind, created);
	CREATE INDEX IF NOT EXISTS idx_email_tokens_email ON email_tokens(email, kind, created);`

// The kinds of link.
const (
	EmailTokenVerify = "verify"
	EmailTokenReset  = "reset"
)

// ErrEmailToken is a link that is no good: unknown, used, ended, or sent to
// an address the account no longer has.
var ErrEmailToken = errors.New("that link is used, too old or not one of ours: ask for a new one")

// ensureEmailVerify makes email_tokens and accountinfo's two columns (initSchema).
func (db *SQLiteDB) ensureEmailVerify() error {
	if _, err := db.Exec(emailVerifySchema); err != nil {
		return fmt.Errorf("email_tokens: %w", err)
	}
	for _, c := range []string{"email_verified_at", "email_verified_by"} {
		ok, err := hasColumn(db.DB, "accountinfo", c)
		if err != nil {
			return fmt.Errorf("accountinfo.%s: %w", c, err)
		}
		if !ok {
			if _, err := db.Exec(`ALTER TABLE accountinfo ADD COLUMN ` + c + ` TEXT NOT NULL DEFAULT ''`); err != nil &&
				!strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("accountinfo.%s: %w", c, err)
			}
		}
	}
	return nil
}

// EmailState is an account's e-mail as the pages, the game and the panel ask
// about it. Email is the whole address: its owner's alone to see (the panel
// shows mailer.Mask of it).
type EmailState struct {
	AccountID  int
	Username   string
	Level      int
	Banned     bool
	LastLogin  string
	Email      string
	VerifiedAt string
	VerifiedBy string
}

// Verified is whether the account has met the requirement: an address
// confirmed by its link, or a GM's override.
func (e EmailState) Verified() bool { return e.VerifiedAt != "" }

const emailStateCols = `accountid, username, COALESCE(acctlevel, 0), COALESCE(acctstatus, 0) < 0, COALESCE(CAST(lastlogin AS TEXT), ''),
	COALESCE(email, ''), COALESCE(email_verified_at, ''), COALESCE(email_verified_by, '')`

func scanEmailState(sc interface{ Scan(...any) error }) (EmailState, error) {
	var e EmailState
	err := sc.Scan(&e.AccountID, &e.Username, &e.Level, &e.Banned, &e.LastLogin, &e.Email, &e.VerifiedAt, &e.VerifiedBy)
	return e, err
}

// AccountEmail is the account's e-mail state.
func (db *SQLiteDB) AccountEmail(accountID int) (EmailState, error) {
	e, err := scanEmailState(db.QueryRow(`SELECT `+emailStateCols+` FROM accountinfo WHERE accountid = ?`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrAccountNotFound
	}
	return e, err
}

// EmailVerified is whether the account has met the requirement (a failed
// read is an error; no such account is false).
func (db *SQLiteDB) EmailVerified(accountID int) (bool, error) {
	e, err := db.AccountEmail(accountID)
	if errors.Is(err, ErrAccountNotFound) {
		return false, nil
	}
	return e.Verified(), err
}

// SetAccountEmail keeps email (normalised already) as the account's address,
// unconfirmed: the verified time and how are cleared, and every link not yet
// used is void (a reset link to the old address among them). Setting the same
// address again changes nothing and keeps a confirmation.
func (db *SQLiteDB) SetAccountEmail(accountID int, email string, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cur string
	err = tx.QueryRow(`SELECT COALESCE(email, '') FROM accountinfo WHERE accountid = ?`, accountID).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountNotFound
	}
	if err != nil {
		return err
	}
	if cur == email {
		return nil
	}
	if _, err := tx.Exec(`UPDATE accountinfo SET email = ?, email_verified_at = '', email_verified_by = '' WHERE accountid = ?`,
		email, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE email_tokens SET used = ? WHERE accountid = ? AND used = ''`,
		"void "+now.UTC().Format(time.RFC3339), accountID); err != nil {
		return err
	}
	return tx.Commit()
}

// EmailSends is what was mailed lately: links of a kind made since a time for
// the account and for the address, and the newest for the account.
type EmailSends struct {
	ForAccount int
	ToAddress  int
	Last       time.Time
}

// EmailSendsSince counts the links of kind made since since, for accountID
// and to email (empty: not counted).
func (db *SQLiteDB) EmailSendsSince(accountID int, email, kind string, since time.Time) (EmailSends, error) {
	var out EmailSends
	s := since.UTC().Format(time.RFC3339)
	var last sql.NullString
	if err := db.QueryRow(`SELECT COUNT(*), MAX(created) FROM email_tokens WHERE accountid = ? AND kind = ? AND created >= ?`,
		accountID, kind, s).Scan(&out.ForAccount, &last); err != nil {
		return out, err
	}
	if last.Valid {
		out.Last, _ = time.Parse(time.RFC3339, last.String)
	}
	if email != "" {
		if err := db.QueryRow(`SELECT COUNT(*) FROM email_tokens WHERE email = ? AND kind = ? AND created >= ?`,
			email, kind, s).Scan(&out.ToAddress); err != nil {
			return out, err
		}
	}
	return out, nil
}

// emailTokenHash is a token as it is kept.
func emailTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewEmailToken makes a link's token of kind for the account and the address
// it goes to, good for ttl from now, and answers it: it is kept only as its
// hash, so this is the one time it is known. Links ended a week ago go.
func (db *SQLiteDB) NewEmailToken(accountID int, kind, email string, ttl time.Duration, now time.Time) (string, error) {
	if kind != EmailTokenVerify && kind != EmailTokenReset {
		return "", fmt.Errorf("no link of kind %q", kind)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	now = now.UTC()
	if _, err := db.Exec(`DELETE FROM email_tokens WHERE expires < ?`, now.Add(-7*24*time.Hour).Format(time.RFC3339)); err != nil {
		return "", err
	}
	if _, err := db.Exec(`INSERT INTO email_tokens (accountid, kind, token_hash, email, created, expires) VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, kind, emailTokenHash(raw), email, now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339)); err != nil {
		return "", err
	}
	return raw, nil
}

// useEmailToken spends a good token of kind inside tx: its account's state
// (as it stood before), or ErrEmailToken.
func useEmailToken(tx *sql.Tx, raw, kind string, now time.Time) (EmailState, error) {
	var id int64
	var accountID int
	var email, expires string
	err := tx.QueryRow(`SELECT id, accountid, email, expires FROM email_tokens WHERE token_hash = ? AND kind = ? AND used = ''`,
		emailTokenHash(strings.TrimSpace(raw)), kind).Scan(&id, &accountID, &email, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailState{}, ErrEmailToken
	}
	if err != nil {
		return EmailState{}, err
	}
	end, err := time.Parse(time.RFC3339, expires)
	if err != nil || !now.Before(end) {
		return EmailState{}, ErrEmailToken
	}
	e, err := scanEmailState(tx.QueryRow(`SELECT `+emailStateCols+` FROM accountinfo WHERE accountid = ?`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return EmailState{}, ErrEmailToken
	}
	if err != nil {
		return EmailState{}, err
	}
	if e.Email == "" || e.Email != email {
		return EmailState{}, ErrEmailToken
	}
	res, err := tx.Exec(`UPDATE email_tokens SET used = ? WHERE id = ? AND used = ''`, now.UTC().Format(time.RFC3339), id)
	if err != nil {
		return EmailState{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return EmailState{}, ErrEmailToken
	}
	return e, nil
}

// VerifyEmail spends a verify link: the account's address is confirmed now.
// It answers the account's state after.
func (db *SQLiteDB) VerifyEmail(raw string, now time.Time) (EmailState, error) {
	tx, err := db.Begin()
	if err != nil {
		return EmailState{}, err
	}
	defer tx.Rollback()
	e, err := useEmailToken(tx, raw, EmailTokenVerify, now)
	if err != nil {
		return e, err
	}
	e.VerifiedAt, e.VerifiedBy = now.UTC().Format(time.RFC3339), "link"
	if _, err := tx.Exec(`UPDATE accountinfo SET email_verified_at = ?, email_verified_by = ? WHERE accountid = ?`,
		e.VerifiedAt, e.VerifiedBy, e.AccountID); err != nil {
		return e, err
	}
	// the other verify links of the account are spent with it
	if _, err := tx.Exec(`UPDATE email_tokens SET used = ? WHERE accountid = ? AND kind = ? AND used = ''`,
		"void "+e.VerifiedAt, e.AccountID, EmailTokenVerify); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// ResetPasswordByEmail spends a reset link: hash is the account's password
// now, its own (accountinfo.pwtemp cleared), and every other reset link of the
// account is void. A reset link goes only to a verified address, and changing
// the address voids it, so it is the confirmed owner's.
func (db *SQLiteDB) ResetPasswordByEmail(raw, hash string, now time.Time) (EmailState, error) {
	if hash == "" {
		return EmailState{}, errors.New("no password to set")
	}
	tx, err := db.Begin()
	if err != nil {
		return EmailState{}, err
	}
	defer tx.Rollback()
	e, err := useEmailToken(tx, raw, EmailTokenReset, now)
	if err != nil {
		return e, err
	}
	if e.Banned {
		return e, errors.New("this account is suspended")
	}
	if _, err := tx.Exec(`UPDATE accountinfo SET pwhash = ?, pwtemp = 0 WHERE accountid = ?`, hash, e.AccountID); err != nil {
		return e, err
	}
	stamp := now.UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE email_tokens SET used = ? WHERE accountid = ? AND kind = ? AND used = ''`,
		"void "+stamp, e.AccountID, EmailTokenReset); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// MarkEmailVerifiedByGM is a GM's override: the account counts as verified
// now (its address, if any, is not mailed), by who, with an audit line that
// says so (never the address). An account verified already is refused.
func (db *SQLiteDB) MarkEmailVerifiedByGM(accountID int, who GMEdit, note string, now time.Time) (EmailState, int64, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return EmailState{}, 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return EmailState{}, 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	e, err := scanEmailState(tx.QueryRow(`SELECT `+emailStateCols+` FROM accountinfo WHERE accountid = ?`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return e, 0, ErrAccountNotFound
	}
	if err != nil {
		return e, 0, err
	}
	if e.Verified() {
		return e, 0, fmt.Errorf("%s is verified already (%s, by %s)", e.Username, e.VerifiedAt, e.VerifiedBy)
	}
	e.VerifiedAt, e.VerifiedBy = now.UTC().Format(time.RFC3339), "GM "+who.Username
	if _, err := tx.Exec(`UPDATE accountinfo SET email_verified_at = ?, email_verified_by = ? WHERE accountid = ?`,
		e.VerifiedAt, e.VerifiedBy, accountID); err != nil {
		return e, 0, err
	}
	detail := fmt.Sprintf("marked the e-mail of %s verified by hand", e.Username)
	if e.Email == "" {
		detail += " (no address on the account)"
	}
	if note = strings.TrimSpace(note); note != "" {
		detail += ": " + note
	}
	audit, err := auditRowChange(tx, who, "account", "accountinfo", map[string]any{"accountid": accountID}, nil, nil, detail)
	if err != nil {
		return e, 0, err
	}
	return e, audit, tx.Commit()
}

// EmailCounts is how many accounts are in each e-mail state.
type EmailCounts struct {
	Accounts int `json:"accounts"`
	// Verified by a link or a GM; Pending have an address not yet
	// confirmed; NoAddress have none (and no override).
	Verified  int `json:"verified"`
	ByGM      int `json:"verifiedByGM"`
	Pending   int `json:"pending"`
	NoAddress int `json:"noAddress"`
}

// AccountEmailCounts counts the accounts by their e-mail state.
func (db *SQLiteDB) AccountEmailCounts() (EmailCounts, error) {
	var c EmailCounts
	err := db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(COALESCE(email_verified_at, '') != ''), 0),
		COALESCE(SUM(COALESCE(email_verified_by, '') LIKE 'GM %'), 0),
		COALESCE(SUM(COALESCE(email_verified_at, '') = '' AND COALESCE(email, '') != ''), 0),
		COALESCE(SUM(COALESCE(email_verified_at, '') = '' AND COALESCE(email, '') = ''), 0)
		FROM accountinfo`).Scan(&c.Accounts, &c.Verified, &c.ByGM, &c.Pending, &c.NoAddress)
	return c, err
}

// EmailStateFilters are AccountEmails' filters.
var EmailStateFilters = []string{"unverified", "verified", "pending", "none", "all"}

// AccountEmails lists accounts by e-mail state (unverified: pending or none),
// by name, with how many match.
func (db *SQLiteDB) AccountEmails(filter string, limit, offset int) ([]EmailState, int, error) {
	where := ""
	switch filter {
	case "", "unverified":
		where = `COALESCE(email_verified_at, '') = ''`
	case "verified":
		where = `COALESCE(email_verified_at, '') != ''`
	case "pending":
		where = `COALESCE(email_verified_at, '') = '' AND COALESCE(email, '') != ''`
	case "none":
		where = `COALESCE(email_verified_at, '') = '' AND COALESCE(email, '') = ''`
	case "all":
		where = `1`
	default:
		return nil, 0, fmt.Errorf("filter is one of %s", strings.Join(EmailStateFilters, ", "))
	}
	if limit <= 0 {
		limit = 50
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accountinfo WHERE ` + where).Scan(&n); err != nil {
		return nil, 0, err
	}
	rows, err := db.Query(`SELECT `+emailStateCols+` FROM accountinfo WHERE `+where+` ORDER BY LOWER(username) LIMIT ? OFFSET ?`,
		limit, max(offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []EmailState
	for rows.Next() {
		e, err := scanEmailState(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, n, rows.Err()
}

// HasAuthenticator is whether the account has an authenticator on the GM
// panel (gm_totp, which the panel makes; none while it is not there).
func (db *SQLiteDB) HasAuthenticator(accountID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = 'gm_totp')`).Scan(&exists)
	if err != nil || !exists {
		return false, err
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM gm_totp WHERE accountid = ?`, accountID).Scan(&n)
	return n > 0, err
}

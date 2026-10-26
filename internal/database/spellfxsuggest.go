package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// spellfx_suggestions: the community's matches of a spell to the visual it
// should play (internal/spellfx: the public spell-visual page, anyone, no
// sign-in; the user, F3 pass 17: "a form that any player can fill out, then
// submit results go to server for comparison", "GMs accept in the panel").
// fxdict is the suggested visual (a SPELLFX.CSF 0xC200 record's DictID, as
// spelleffect.fxdict holds one); name and note are what the player typed,
// kept as given and never acted on. addr_hash is the sender's address
// salted and hashed (SpellFXAddrHash), never the address: it lets one
// address hold one live suggestion a spell and a GM clear a spammer's rows.
// status: new, accepted (a GM gave the spell that visual) or dismissed.
const spellFXSuggestionsSchema = `
	CREATE TABLE IF NOT EXISTS spellfx_suggestions (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		spellid    INTEGER NOT NULL,
		fxdict     INTEGER NOT NULL,
		name       TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT '',
		addr_hash  TEXT NOT NULL,
		created    TEXT NOT NULL,
		status     TEXT NOT NULL DEFAULT 'new',
		decided_by TEXT NOT NULL DEFAULT '',
		decided_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_spellfx_suggestions_spell ON spellfx_suggestions(spellid, status);
	CREATE INDEX IF NOT EXISTS idx_spellfx_suggestions_addr ON spellfx_suggestions(addr_hash, spellid);
	CREATE TABLE IF NOT EXISTS spellfx_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`

// Suggestion statuses.
const (
	SpellFXNew       = "new"
	SpellFXAccepted  = "accepted"
	SpellFXDismissed = "dismissed"
)

// ensureSpellFXSuggestions makes the community suggestions' tables
// (initSchema).
func (db *SQLiteDB) ensureSpellFXSuggestions() error {
	if _, err := db.Exec(spellFXSuggestionsSchema); err != nil {
		return fmt.Errorf("spellfx_suggestions: %w", err)
	}
	return nil
}

// stampNow is a time as the panel's tables write one (fmt.js's when/ago read
// it as UTC).
func stampNow(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

// SpellFXSalt is the salt the senders' addresses are hashed with: made once,
// at random, and kept in spellfx_meta, so a hash means nothing off this
// database.
func (db *SQLiteDB) SpellFXSalt() (string, error) {
	var salt string
	err := db.QueryRow(`SELECT value FROM spellfx_meta WHERE key = 'salt'`).Scan(&salt)
	if err == nil && salt != "" {
		return salt, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	salt = hex.EncodeToString(b)
	// another start may have made one meanwhile: the first kept wins
	if _, err := db.Exec(`INSERT OR IGNORE INTO spellfx_meta (key, value) VALUES ('salt', ?)`, salt); err != nil {
		return "", err
	}
	if err := db.QueryRow(`SELECT value FROM spellfx_meta WHERE key = 'salt'`).Scan(&salt); err != nil {
		return "", err
	}
	return salt, nil
}

// SpellFXSuggestion is one suggestion as it is kept.
type SpellFXSuggestion struct {
	ID        int64
	SpellID   int
	FX        uint32
	Name      string
	Note      string
	AddrHash  string
	Created   string
	Status    string
	DecidedBy string
	DecidedAt string
}

// AddSpellFXSuggestion keeps a suggestion, in place of the same address's
// live one for the same spell (one live suggestion a spell an address), and
// answers how many the spell has had (every status).
func (db *SQLiteDB) AddSpellFXSuggestion(s SpellFXSuggestion, now time.Time) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM spellfx_suggestions WHERE addr_hash = ? AND spellid = ? AND status = ?`,
		s.AddrHash, s.SpellID, SpellFXNew); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO spellfx_suggestions (spellid, fxdict, name, note, addr_hash, created, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.SpellID, int64(s.FX), s.Name, s.Note, s.AddrHash, stampNow(now), SpellFXNew); err != nil {
		return 0, err
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM spellfx_suggestions WHERE spellid = ?`, s.SpellID).Scan(&n); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// SpellFXSuggestionCount is how many suggestions are waiting (status new):
// the box's size, which the public route caps.
func (db *SQLiteDB) SpellFXSuggestionCount() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM spellfx_suggestions WHERE status = ?`, SpellFXNew).Scan(&n)
	return n, err
}

// SpellFXSuggestions is every suggestion of the given status ("" or "all":
// every one), for spell when it is not 0, newest first.
func (db *SQLiteDB) SpellFXSuggestions(status string, spell int) ([]SpellFXSuggestion, error) {
	q := `SELECT id, spellid, fxdict, name, note, addr_hash, created, status, decided_by, decided_at FROM spellfx_suggestions`
	var where []string
	var args []any
	if status != "" && status != "all" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if spell != 0 {
		where = append(where, "spellid = ?")
		args = append(args, spell)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpellFXSuggestion
	for rows.Next() {
		var s SpellFXSuggestion
		var fx int64
		if err := rows.Scan(&s.ID, &s.SpellID, &fx, &s.Name, &s.Note, &s.AddrHash, &s.Created, &s.Status, &s.DecidedBy, &s.DecidedAt); err != nil {
			return nil, err
		}
		s.FX = uint32(fx)
		out = append(out, s)
	}
	return out, rows.Err()
}

// DecideSpellFXSuggestions sets the status of a spell's live suggestions:
// those for visual fx become status, and when others is not "" every other
// live one of the spell becomes others. It answers how many rows changed.
func (db *SQLiteDB) DecideSpellFXSuggestions(spell int, fx uint32, status, others, by string, now time.Time) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	at := stampNow(now)
	res, err := tx.Exec(`UPDATE spellfx_suggestions SET status = ?, decided_by = ?, decided_at = ?
		WHERE spellid = ? AND fxdict = ? AND status = ?`, status, by, at, spell, int64(fx), SpellFXNew)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if others != "" {
		res, err := tx.Exec(`UPDATE spellfx_suggestions SET status = ?, decided_by = ?, decided_at = ?
			WHERE spellid = ? AND fxdict != ? AND status = ?`, others, by, at, spell, int64(fx), SpellFXNew)
		if err != nil {
			return 0, err
		}
		m, _ := res.RowsAffected()
		n += m
	}
	return int(n), tx.Commit()
}

// DeleteSpellFXSuggestionsFrom deletes every suggestion of the senders whose
// address hash begins with prefix (at least 8 characters: a spammer's rows,
// as the panel shows the hash), and answers how many.
func (db *SQLiteDB) DeleteSpellFXSuggestionsFrom(prefix string) (int, error) {
	if len(prefix) < 8 || strings.ContainsAny(prefix, "%_") {
		return 0, fmt.Errorf("an address hash prefix of at least 8 characters")
	}
	res, err := db.Exec(`DELETE FROM spellfx_suggestions WHERE addr_hash LIKE ?`, prefix+"%")
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// spellFXGen counts InvalidateSpellFX: the public spell list (internal/spellfx)
// is built again before its next answer once it has moved.
var spellFXGen atomic.Int64

// InvalidateSpellFX says the spells' visuals, lines or classes changed (a GM's
// row edit, set_spell_visual, #spellfx): the community page's spell list is
// built again at its next request rather than within the minute.
func InvalidateSpellFX() { spellFXGen.Add(1) }

// SpellFXGeneration is how many times the spells changed since the start.
func SpellFXGeneration() int64 { return spellFXGen.Load() }

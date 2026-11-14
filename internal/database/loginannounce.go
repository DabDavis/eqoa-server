package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// loginannounce: the GMs' announcement at login, one a shard (petition 2120,
// the GM Jibwiz, 2026-11-14: "GM's need to be able to set an announce that
// displays at login for all players. This needs to load as the newest message
// in chat upon login, after all the other tickets closure entries and so on").
// One row or none: id is always 1, text the line every player entering the
// world is sent last (handlers/loginannounce.go), set_by the account that set
// it, changed when (UTC, RFC 3339). No row is no announcement. A GM sets it
// with /announce in the game or set_login_announce in the panel; each change
// is a gm_audit row change, so the panel reverts it. It is read at each
// login, so a change counts at the next one, with no restart.
const loginAnnounceSchema = `
	CREATE TABLE IF NOT EXISTS loginannounce (
		id      INTEGER PRIMARY KEY CHECK (id = 1),
		text    TEXT NOT NULL,
		set_by  TEXT NOT NULL DEFAULT '',
		changed TEXT NOT NULL DEFAULT ''
	);`

// LoginAnnounceMost is the most characters an announcement keeps: a chat
// line of that many letters wraps on a PS2 (consoletext.go's measure of
// VIUIWindow::TextBox: 511 letters survive), with room for its prefix.
const LoginAnnounceMost = 400

// LoginAnnounce is the shard's announcement: Text empty when there is none.
type LoginAnnounce struct {
	Text    string `json:"text"`
	SetBy   string `json:"set_by"`
	Changed string `json:"changed"`
}

// ensureLoginAnnounce makes table loginannounce (initSchema).
func (db *SQLiteDB) ensureLoginAnnounce() error {
	if _, err := db.Exec(loginAnnounceSchema); err != nil {
		return fmt.Errorf("loginannounce: %w", err)
	}
	return nil
}

// LoginAnnounce is the announcement as it is now (Text empty: none).
func (db *SQLiteDB) LoginAnnounce() (LoginAnnounce, error) {
	var a LoginAnnounce
	err := db.QueryRow(`SELECT text, set_by, changed FROM loginannounce WHERE id = 1`).Scan(&a.Text, &a.SetBy, &a.Changed)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginAnnounce{}, nil
	}
	return a, err
}

// SetLoginAnnounceAudited makes text the announcement, text empty taking it
// away, with its gm_audit line in the same transaction (detail what it was
// for). It returns the announcement it replaced. Setting what is there
// already, or clearing none, writes nothing.
func (db *SQLiteDB) SetLoginAnnounceAudited(text string, who GMEdit, detail string) (LoginAnnounce, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return LoginAnnounce{}, err
	}
	if err := db.ensureLoginAnnounce(); err != nil {
		return LoginAnnounce{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return LoginAnnounce{}, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	const sel = `SELECT * FROM loginannounce WHERE id = 1`
	before, err := fullRow(tx, sel)
	if err != nil {
		return LoginAnnounce{}, err
	}
	var old LoginAnnounce
	if before != nil {
		old.Text, _ = before["text"].(string)
		old.SetBy, _ = before["set_by"].(string)
		old.Changed, _ = before["changed"].(string)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	action := "update"
	switch {
	case text == "":
		if before == nil {
			return old, nil
		}
		action = "delete"
		_, err = tx.Exec(`DELETE FROM loginannounce WHERE id = 1`)
	case before == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO loginannounce (id, text, set_by, changed) VALUES (1, ?, ?, ?)`, text, who.Username, now)
	case old.Text == text:
		return old, nil
	default:
		_, err = tx.Exec(`UPDATE loginannounce SET text = ?, set_by = ?, changed = ? WHERE id = 1`, text, who.Username, now)
	}
	if err != nil {
		return LoginAnnounce{}, err
	}
	after, err := fullRow(tx, sel)
	if err != nil {
		return LoginAnnounce{}, err
	}
	if _, err := auditRowChange(tx, who, action, "loginannounce", map[string]any{"id": int64(1)}, before, after, detail); err != nil {
		return LoginAnnounce{}, err
	}
	return old, tx.Commit()
}

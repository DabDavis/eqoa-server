package database

import (
	"fmt"
	"time"
)

// charwelcome: the welcome guide's box, a row a character
// (handlers/welcome.go). shown_day is the UTC calendar day (2006-01-02) its
// daily box last went out; never is 1 once the player chose never to be asked
// again on this character (only /tutorial shows the box then); changed is
// when the row last changed (UTC, RFC 3339). A character with no row has
// never been asked. A player's own choice, nothing shared: a new server's
// copy starts it empty.
const charWelcomeSchema = `
	CREATE TABLE IF NOT EXISTS charwelcome (
		serverid  INTEGER PRIMARY KEY,
		shown_day TEXT NOT NULL DEFAULT '',
		never     INTEGER NOT NULL DEFAULT 0,
		changed   TEXT NOT NULL DEFAULT ''
	);`

// ensureCharWelcome makes table charwelcome (initSchema).
func (db *SQLiteDB) ensureCharWelcome() error {
	if _, err := db.Exec(charWelcomeSchema); err != nil {
		return fmt.Errorf("charwelcome: %w", err)
	}
	return nil
}

// WelcomeDue reports whether the character's daily box is due on day: it has
// not gone out that day and the player has not said never.
func (db *SQLiteDB) WelcomeDue(serverID uint32, day string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charwelcome WHERE serverid = ? AND (never <> 0 OR shown_day = ?)`,
		serverID, day).Scan(&n)
	return n == 0, err
}

// TakeWelcomeDay takes day as the one the character's daily box goes out on,
// when it is due (WelcomeDue), in one statement, so two sessions of one
// character cannot both take it; it reports whether it was due.
func (db *SQLiteDB) TakeWelcomeDay(serverID uint32, day string) (bool, error) {
	res, err := db.Exec(`INSERT INTO charwelcome (serverid, shown_day, changed) VALUES (?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET shown_day = excluded.shown_day, changed = excluded.changed
		WHERE charwelcome.never = 0 AND charwelcome.shown_day <> excluded.shown_day`,
		serverID, day, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// SetWelcomeNever keeps whether the character's daily box is off for good (on:
// never asked again but by /tutorial).
func (db *SQLiteDB) SetWelcomeNever(serverID uint32, never bool) error {
	v := 0
	if never {
		v = 1
	}
	_, err := db.Exec(`INSERT INTO charwelcome (serverid, never, changed) VALUES (?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET never = excluded.never, changed = excluded.changed`,
		serverID, v, time.Now().UTC().Format(time.RFC3339))
	return err
}

// WelcomeNever reports whether the character's daily box is off for good.
func (db *SQLiteDB) WelcomeNever(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charwelcome WHERE serverid = ? AND never <> 0`, serverID).Scan(&n)
	return n > 0, err
}

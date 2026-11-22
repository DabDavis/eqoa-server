package database

import (
	"fmt"
	"log"
	"time"
)

// chatmutes: a GM's chat mutes by the hour, per account (board entry 1656;
// handlers/chatmute.go): a row per account and scope, each with its own end.
// Scope "shout" stops the account's shouts, "all" its every chat line. Each
// shard keeps its own.
const chatMutesSchema = `
	CREATE TABLE IF NOT EXISTS chatmutes (
		accountid INTEGER NOT NULL,
		scope     TEXT NOT NULL,
		until     TEXT NOT NULL,
		gm        TEXT NOT NULL DEFAULT '',
		since     TEXT NOT NULL,
		PRIMARY KEY (accountid, scope)
	)`

// The chat mutes' scopes.
const (
	ChatMuteShout = "shout"
	ChatMuteAll   = "all"
)

// ChatMute is a chatmutes row: the account's mute of the scope, until when,
// given by which GM's character, since when.
type ChatMute struct {
	AccountID int
	Scope     string
	Until     time.Time
	GM        string
	Since     time.Time
}

// ensureChatMutes makes table chatmutes (initSchema).
func (db *SQLiteDB) ensureChatMutes() error {
	if _, err := db.Exec(chatMutesSchema); err != nil {
		return fmt.Errorf("chatmutes: %w", err)
	}
	return nil
}

// ChatMutes is every row, ended or not. A row of another scope, or whose until
// does not parse, is left out and logged.
func (db *SQLiteDB) ChatMutes() ([]ChatMute, error) {
	rows, err := db.Query(`SELECT accountid, scope, until, gm, since FROM chatmutes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMute
	for rows.Next() {
		var m ChatMute
		var until, since string
		if err := rows.Scan(&m.AccountID, &m.Scope, &until, &m.GM, &since); err != nil {
			return nil, err
		}
		if m.Scope != ChatMuteShout && m.Scope != ChatMuteAll {
			log.Printf("ChatMutes: account %d's row of scope %q left out", m.AccountID, m.Scope)
			continue
		}
		if m.Until, err = time.Parse(time.RFC3339, until); err != nil {
			log.Printf("ChatMutes: account %d's %s row left out: until %q", m.AccountID, m.Scope, until)
			continue
		}
		m.Since, _ = time.Parse(time.RFC3339, since)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetChatMute keeps the account's mute of the scope (replacing one of the same
// scope), then deletes every row ended by its Since.
func (db *SQLiteDB) SetChatMute(m ChatMute) error {
	if m.Scope != ChatMuteShout && m.Scope != ChatMuteAll {
		return fmt.Errorf("chatmutes: no scope %q", m.Scope)
	}
	since := m.Since.UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT OR REPLACE INTO chatmutes (accountid, scope, until, gm, since) VALUES (?, ?, ?, ?, ?)`,
		m.AccountID, m.Scope, m.Until.UTC().Format(time.RFC3339), m.GM, since); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM chatmutes WHERE until <= ?`, since)
	return err
}

// ClearChatMutes lifts the account's mute of the scope ("": both).
func (db *SQLiteDB) ClearChatMutes(accountID int, scope string) error {
	if scope == "" {
		_, err := db.Exec(`DELETE FROM chatmutes WHERE accountid = ?`, accountID)
		return err
	}
	_, err := db.Exec(`DELETE FROM chatmutes WHERE accountid = ? AND scope = ?`, accountID, scope)
	return err
}

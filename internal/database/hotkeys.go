package database

import (
	"database/sql"
	"fmt"
	"strconv"
)

// hotkeys: a character's quick-chat sets, the L2 d-pad's menus (CLIENT's
// VITextSet, VIWndMenu4), one row a set: `direction` the set's id as a number
// (its path: the low two bits its depth, a 2-bit child index for each level
// above, so 0 to 255 and the root 0: VITextSet::MakeId), and for each of its four
// choices (N, W, E, S: slots 0, 1, 2, 3) the label its button shows and the
// message it sends ("|<n>" is a command, anything else is said). Until the
// player first changes one a character has no rows, and the server's default sets
// go out (handlers' defaultHotkeys); the first change writes them all and then
// itself, so each change after is one row.
type HotkeyPair struct {
	Text  string // the message: VIRec388 m_p04
	Title string // the label: VIRec388 m_p104
}

// HotkeySet is one set of four choices.
type HotkeySet struct {
	ID    int
	Pairs [4]HotkeyPair
}

// The bounds of a set: its id a byte, and CLIENT's record (VIRec388) keeps 128
// wide characters of a message and 64 of a label, the end among them.
const (
	HotkeySetIDs   = 256
	HotkeyTextMax  = 127
	HotkeyTitleMax = 63
)

// Hotkeys is the character's sets by id, none when it never changed one.
func (db *SQLiteDB) Hotkeys(serverID uint32) ([]HotkeySet, error) {
	rows, err := db.Query(`SELECT direction, COALESCE(Nlabel, ''), COALESCE(Nmessage, ''), COALESCE(Wlabel, ''), COALESCE(Wmessage, ''),
		COALESCE(Elabel, ''), COALESCE(Emessage, ''), COALESCE(Slabel, ''), COALESCE(Smessage, '')
		FROM hotkeys WHERE serverid = ? ORDER BY CAST(direction AS INTEGER)`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HotkeySet
	for rows.Next() {
		var dir string
		var s HotkeySet
		if err := rows.Scan(&dir, &s.Pairs[0].Title, &s.Pairs[0].Text, &s.Pairs[1].Title, &s.Pairs[1].Text,
			&s.Pairs[2].Title, &s.Pairs[2].Text, &s.Pairs[3].Title, &s.Pairs[3].Text); err != nil {
			return nil, err
		}
		id, err := strconv.Atoi(dir)
		if err != nil || id < 0 || id >= HotkeySetIDs {
			continue // a row by hand this server cannot send
		}
		s.ID = id
		out = append(out, s)
	}
	return out, rows.Err()
}

// hotkeyEdit runs fn in a transaction, the character's rows first made from
// defaults when it has none (its first change).
func (db *SQLiteDB) hotkeyEdit(serverID uint32, defaults []HotkeySet, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM hotkeys WHERE serverid = ?`, serverID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		for _, s := range defaults {
			if err := putHotkeySet(tx, serverID, s); err != nil {
				return err
			}
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func putHotkeySet(tx *sql.Tx, serverID uint32, s HotkeySet) error {
	if _, err := tx.Exec(`DELETE FROM hotkeys WHERE serverid = ? AND direction = ?`, serverID, strconv.Itoa(s.ID)); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO hotkeys (serverid, direction, Nlabel, Nmessage, Wlabel, Wmessage, Elabel, Emessage, Slabel, Smessage)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, serverID, strconv.Itoa(s.ID), s.Pairs[0].Title, s.Pairs[0].Text, s.Pairs[1].Title, s.Pairs[1].Text,
		s.Pairs[2].Title, s.Pairs[2].Text, s.Pairs[3].Title, s.Pairs[3].Text)
	return err
}

// SetHotkeyChoice is one choice of a set changed (0x006C), the set made when
// it has none yet.
func (db *SQLiteDB) SetHotkeyChoice(serverID uint32, defaults []HotkeySet, id, slot int, p HotkeyPair) error {
	if id < 0 || id >= HotkeySetIDs || slot < 0 || slot > 3 {
		return fmt.Errorf("hotkey set %d choice %d", id, slot)
	}
	col := "NWES"[slot : slot+1]
	return db.hotkeyEdit(serverID, defaults, func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE hotkeys SET `+col+`label = ?, `+col+`message = ? WHERE serverid = ? AND direction = ?`,
			p.Title, p.Text, serverID, strconv.Itoa(id))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n > 0 {
			return err
		}
		s := HotkeySet{ID: id}
		s.Pairs[slot] = p
		return putHotkeySet(tx, serverID, s)
	})
}

// SetHotkeySet is a whole set made or replaced (0x006A).
func (db *SQLiteDB) SetHotkeySet(serverID uint32, defaults []HotkeySet, s HotkeySet) error {
	if s.ID < 0 || s.ID >= HotkeySetIDs {
		return fmt.Errorf("hotkey set %d", s.ID)
	}
	return db.hotkeyEdit(serverID, defaults, func(tx *sql.Tx) error { return putHotkeySet(tx, serverID, s) })
}

// DropHotkeySet is a set taken away (0x006B).
func (db *SQLiteDB) DropHotkeySet(serverID uint32, defaults []HotkeySet, id int) error {
	if id < 0 || id >= HotkeySetIDs {
		return fmt.Errorf("hotkey set %d", id)
	}
	return db.hotkeyEdit(serverID, defaults, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM hotkeys WHERE serverid = ? AND direction = ?`, serverID, strconv.Itoa(id))
		return err
	})
}

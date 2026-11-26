package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// surname.go — a character's surname (Live petition 5648: "could some how some
// way, add the ability to give yourself a Surname, whether at a certain level
// or via quest"; the owner approved: /surname from level 20, one a character,
// changed with a wait, a GM's clear). Columns of characters, empty and 0 for
// every character before them:
//
//	surname     the surname ("" none), held to the pets' and mercs' name rules
//	            (handlers/surname.go)
//	surname_at  when it was last taken or changed (unix seconds; 0 never, or a
//	            GM's clear): the next change waits from it

// surnameColumns are the columns ensureSurname adds to characters.
var surnameColumns = []struct{ name, def string }{
	{"surname", "TEXT NOT NULL DEFAULT ''"},
	{"surname_at", "INTEGER NOT NULL DEFAULT 0"},
}

// ensureSurname adds the surname's columns to characters (initSchema).
func (db *SQLiteDB) ensureSurname() error {
	for _, c := range surnameColumns {
		ok, err := hasColumn(db.DB, "characters", c.name)
		if err != nil {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
		if ok {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE characters ADD COLUMN ` + c.name + ` ` + c.def); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
	}
	return nil
}

// CharacterSurname is a character's surname ("" none) and when it was last
// taken or changed (zero never); ErrCharacterNotFound for no character.
func (db *SQLiteDB) CharacterSurname(serverID uint32) (string, time.Time, error) {
	var s string
	var at int64
	err := db.QueryRow(`SELECT COALESCE(surname, ''), COALESCE(surname_at, 0) FROM characters WHERE serverid = ?`, serverID).Scan(&s, &at)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, ErrCharacterNotFound
		}
		return "", time.Time{}, err
	}
	if at == 0 {
		return s, time.Time{}, nil
	}
	return s, time.Unix(at, 0), nil
}

// SetSurname writes a character's surname, taken or changed at (a clear by its
// player, "", keeps the wait running from the last change: at is then zero and
// surname_at is left). ErrCharacterNotFound for no character.
func (db *SQLiteDB) SetSurname(serverID uint32, surname string, at time.Time) error {
	var res sql.Result
	var err error
	if at.IsZero() {
		res, err = db.Exec(`UPDATE characters SET surname = ? WHERE serverid = ?`, surname, serverID)
	} else {
		res, err = db.Exec(`UPDATE characters SET surname = ?, surname_at = ? WHERE serverid = ?`, surname, at.Unix(), serverID)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// ClearSurnameNamed is a GM's clear of the surname of the character of the
// name (any case): its surname gone and its wait with it, so a new one may be
// taken at once. It reports the character's id, its name as written and the
// surname it had; ErrCharacterNotFound for no character of the name.
func (db *SQLiteDB) ClearSurnameNamed(name string) (serverID uint32, charName, old string, err error) {
	err = db.QueryRow(`SELECT serverid, charName, COALESCE(surname, '') FROM characters WHERE lower(charName) = lower(?)`,
		strings.TrimSpace(name)).Scan(&serverID, &charName, &old)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", "", ErrCharacterNotFound
		}
		return 0, "", "", err
	}
	if _, err := db.Exec(`UPDATE characters SET surname = '', surname_at = 0 WHERE serverid = ?`, serverID); err != nil {
		return 0, "", "", err
	}
	return serverID, charName, old, nil
}

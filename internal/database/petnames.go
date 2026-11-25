package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// petnames and petnamelog: the names players give their pets and mercs
// (handlers/petrename.go; the owner, 2026-11-25: "let players rename pets and
// mercs but apply the name filter to catch bad actors").
//
// petnames: a character's name for the pet one summoning spell makes:
// serverid, the character; spell, the summoning spell's id (a pet's identity
// as a restart carries it: entity/keeppet.go), so every summoning of that
// spell comes by the name, and another spell's pet by its own; name, as
// chosen. A merc's name is its charmerc row's own (SetMercNameAudited).
//
// petnamelog: every rename asked, accepted or refused: when, the character
// and its account, what (merc or pet), its name before, the name asked, the
// outcome (renamed, refused, filtered: refused by the name filter, the
// words no character's name may carry, or a reserved or a staff name), and
// why. A GM reads the filter's catches there (outcome filtered) and in the
// server's log ("PetRename: FILTERED").
const petNamesSchema = `
	CREATE TABLE IF NOT EXISTS petnames (
		serverid INTEGER NOT NULL,
		spell    INTEGER NOT NULL,
		name     TEXT NOT NULL,
		at       TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serverid, spell)
	);
	CREATE TABLE IF NOT EXISTS petnamelog (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		at        TEXT NOT NULL,
		serverid  INTEGER NOT NULL,
		charname  TEXT NOT NULL DEFAULT '',
		accountid INTEGER NOT NULL DEFAULT 0,
		kind      TEXT NOT NULL,
		oldname   TEXT NOT NULL DEFAULT '',
		asked     TEXT NOT NULL DEFAULT '',
		outcome   TEXT NOT NULL,
		reason    TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_petnamelog_char ON petnamelog(serverid);
	CREATE INDEX IF NOT EXISTS idx_petnamelog_outcome ON petnamelog(outcome);`

// The outcomes petnamelog records.
const (
	PetRenamed  = "renamed"
	PetRefused  = "refused"
	PetFiltered = "filtered"
)

// ensurePetNames makes tables petnames and petnamelog (initSchema).
func (db *SQLiteDB) ensurePetNames() error {
	if _, err := db.Exec(petNamesSchema); err != nil {
		return fmt.Errorf("petnames: %w", err)
	}
	return nil
}

// PetNamesOf is a character's pet names by summoning spell.
func (db *SQLiteDB) PetNamesOf(serverID uint32) (map[int]string, error) {
	if err := db.ensurePetNames(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT spell, name FROM petnames WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var spell int
		var name string
		if err := rows.Scan(&spell, &name); err != nil {
			return nil, err
		}
		if name = strings.TrimSpace(name); name != "" {
			out[spell] = name
		}
	}
	return out, rows.Err()
}

// SetPetNameAudited keeps the character's name for its pet of the summoning
// spell, the change a gm_audit line (as the Pet Groomer's dyes are: the GM
// panel lists and reverts it), who the player.
func (db *SQLiteDB) SetPetNameAudited(serverID uint32, spell int, name string, who GMEdit, detail string) error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	if err := db.ensurePetNames(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	who = auditWho(tx, who)
	const sel = `SELECT * FROM petnames WHERE serverid = ? AND spell = ?`
	prev, err := fullRow(tx, sel, serverID, spell)
	if err != nil {
		return err
	}
	action := "update"
	if prev == nil {
		action = "insert"
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO petnames (serverid, spell, name, at) VALUES (?, ?, ?, ?)`,
		serverID, spell, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	after, err := fullRow(tx, sel, serverID, spell)
	if err != nil {
		return err
	}
	key := map[string]any{"serverid": int64(serverID), "spell": int64(spell)}
	if _, err := auditRowChange(tx, who, action, "petnames", key, prev, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}

// SetMercNameAudited names the character's merc of the slot anew in its
// charmerc row, the change a gm_audit line, who the player. A slot with no
// row is an error.
func (db *SQLiteDB) SetMercNameAudited(serverID uint32, slot int, name string, who GMEdit, detail string) error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	who = auditWho(tx, who)
	const sel = `SELECT * FROM charmerc WHERE serverid = ? AND slot = ?`
	slot = max(slot, 1)
	prev, err := fullRow(tx, sel, serverID, slot)
	if err != nil {
		return err
	}
	if prev == nil {
		return fmt.Errorf("character %d has no mercenary %d", serverID, slot)
	}
	if _, err := tx.Exec(`UPDATE charmerc SET name = ? WHERE serverid = ? AND slot = ?`, name, serverID, slot); err != nil {
		return err
	}
	after, err := fullRow(tx, sel, serverID, slot)
	if err != nil {
		return err
	}
	key := map[string]any{"serverid": int64(serverID), "slot": int64(slot)}
	if _, err := auditRowChange(tx, who, "update", "charmerc", key, prev, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}

// auditWho is who with its account's name filled in where it is missing.
func auditWho(tx *sql.Tx, who GMEdit) GMEdit {
	if who.Username == "" {
		if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username); err != nil {
			who.Username = fmt.Sprintf("account %d", who.AccountID)
		}
	}
	return who
}

// PetRenameEntry is one line of petnamelog.
type PetRenameEntry struct {
	At        time.Time
	ServerID  uint32
	CharName  string
	AccountID int
	Kind      string // "merc" or "pet"
	OldName   string
	Asked     string
	Outcome   string // PetRenamed, PetRefused or PetFiltered
	Reason    string
}

// LogPetRename writes a line of petnamelog.
func (db *SQLiteDB) LogPetRename(e PetRenameEntry) error {
	if err := db.ensurePetNames(); err != nil {
		return err
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	_, err := db.Exec(`INSERT INTO petnamelog (at, serverid, charname, accountid, kind, oldname, asked, outcome, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.At.UTC().Format(time.RFC3339), e.ServerID, e.CharName, e.AccountID, e.Kind,
		e.OldName, e.Asked, e.Outcome, e.Reason)
	return err
}

// PetRenameLog is petnamelog's lines for the character (0: everyone's),
// newest first, at most limit.
func (db *SQLiteDB) PetRenameLog(serverID uint32, limit int) ([]PetRenameEntry, error) {
	if err := db.ensurePetNames(); err != nil {
		return nil, err
	}
	q := `SELECT at, serverid, charname, accountid, kind, oldname, asked, outcome, reason FROM petnamelog`
	var args []any
	if serverID != 0 {
		q += ` WHERE serverid = ?`
		args = append(args, serverID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, max(limit, 1))
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PetRenameEntry
	for rows.Next() {
		var e PetRenameEntry
		var at string
		if err := rows.Scan(&at, &e.ServerID, &e.CharName, &e.AccountID, &e.Kind, &e.OldName, &e.Asked, &e.Outcome, &e.Reason); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// NamedCharacter is whether a character of the name (any case) is kept,
// and whether one of them is a GM's (its account's level 1 or more).
func (db *SQLiteDB) NamedCharacter(name string) (named, gm bool, err error) {
	rows, err := db.Query(`SELECT COALESCE(a.acctlevel, 0) FROM characters c LEFT JOIN accountinfo a ON a.accountid = c.accountid
		WHERE lower(c.charName) = lower(?)`, strings.TrimSpace(name))
	if err != nil {
		return false, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var level int
		if err := rows.Scan(&level); err != nil {
			return false, false, err
		}
		named, gm = true, gm || level >= 1
	}
	return named, gm, rows.Err()
}

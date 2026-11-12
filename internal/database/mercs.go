package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// charmerc: each character's hired mercenaries (handlers/merc.go,
// entity/merc.go), a row a merc, up to MercSlots a character (the owner,
// 2026-11-12: "if a player wants to have 3 mercenaries in their group it is
// fine"), each under its slot (1 to MercSlots: /merc 2, its follow place),
// kept until it is dismissed so the merc comes back at the next login and
// through any restart: its name, class, race and sex, its face and hair as
// the liaison chose them, its stance, and when the rest after its death ends
// (UTC, RFC 3339; ” none). hired is when (UTC, RFC 3339). path is the class
// path its book takes (handlers/mercpath.go: the index of its class's path,
// -1 its class's default; the owner, 2026-11-11: "lets add path choice to each
// merc"). pet is the kind of pet its hirer picked (handlers/mercpetkind.go:
// "air", "bones"; ” its best: petition 1564). A merc on standby (out of a
// full group for a player: entity/mercbench.go) keeps its row as it is.
const charMercSchema = `
	CREATE TABLE IF NOT EXISTS charmerc (
		serverid    INTEGER NOT NULL,
		slot        INTEGER NOT NULL DEFAULT 1,
		name        TEXT NOT NULL,
		class       INTEGER NOT NULL,
		race        INTEGER NOT NULL,
		sex         INTEGER NOT NULL DEFAULT 0,
		face        INTEGER NOT NULL DEFAULT 0,
		hair_color  INTEGER NOT NULL DEFAULT 0,
		hair_length INTEGER NOT NULL DEFAULT 0,
		hair_style  INTEGER NOT NULL DEFAULT 0,
		stance      INTEGER NOT NULL DEFAULT 2,
		dead_until  TEXT NOT NULL DEFAULT '',
		hired       TEXT NOT NULL DEFAULT '',
		path        INTEGER NOT NULL DEFAULT -1,
		pet         TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serverid, slot)
	);`

// MercSlots is how many mercs a character may have hired at once.
const MercSlots = 3

// familiars: what a familiar is and does, by the item whose Use calls it
// (handlers/familiar.go, entity/familiar.go; the GM Jibrish's petition 1445,
// Healbot the first: tools/seed_familiars.py), GM-editable: its name, body
// (CHAR.ESF, as npcs.modelid) and size, its hit points and what it regenerates
// each six-second tick, the item's cast and its visual (0: a buff's stand-in),
// and its heal of its owner: so much (0: none), below heal_below percent of
// its owner's hit points, at most every heal_every_ms, its visual (0: a heal's).
const familiarsSchema = `
	CREATE TABLE IF NOT EXISTS familiars (
		patternid     INTEGER PRIMARY KEY,
		name          TEXT NOT NULL,
		model         INTEGER NOT NULL DEFAULT 0,
		size          REAL NOT NULL DEFAULT 1,
		hp            INTEGER NOT NULL DEFAULT 100000,
		regen         INTEGER NOT NULL DEFAULT 0,
		cast_ms       INTEGER NOT NULL DEFAULT 2000,
		cast_fx       INTEGER NOT NULL DEFAULT 0,
		heal          INTEGER NOT NULL DEFAULT 0,
		heal_below    INTEGER NOT NULL DEFAULT 75,
		heal_every_ms INTEGER NOT NULL DEFAULT 6000,
		heal_fx       INTEGER NOT NULL DEFAULT 0,
		note          TEXT NOT NULL DEFAULT '',
		source        TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureMercs makes tables charmerc and familiars (initSchema).
func (db *SQLiteDB) ensureMercs() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'charmerc'`).Scan(&n); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if n > 0 {
		// path, then pet, to a table made before them
		for _, col := range []struct{ name, def string }{{"path", "INTEGER NOT NULL DEFAULT -1"}, {"pet", "TEXT NOT NULL DEFAULT ''"}} {
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('charmerc') WHERE name = ?`, col.name).Scan(&n); err != nil {
				return fmt.Errorf("charmerc: %w", err)
			}
			if n == 0 {
				if _, err := db.Exec(`ALTER TABLE charmerc ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
					return fmt.Errorf("charmerc: its %s column: %w", col.name, err)
				}
			}
		}
		if err := db.mercSlotsTable(); err != nil {
			return err
		}
	}
	if _, err := db.Exec(charMercSchema); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if _, err := db.Exec(familiarsSchema); err != nil {
		return fmt.Errorf("familiars: %w", err)
	}
	return nil
}

// mercSlotsTable makes a table of one merc a character (its key the
// character's id alone, before 2026-11-12) the table of slots: each row its
// slot 1, in one transaction.
func (db *SQLiteDB) mercSlotsTable() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('charmerc') WHERE name = 'slot'`).Scan(&n); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if n > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	defer tx.Rollback()
	const cols = `serverid, name, class, race, sex, face, hair_color, hair_length, hair_style, stance, dead_until, hired, path, pet`
	for _, q := range []string{
		`ALTER TABLE charmerc RENAME TO charmerc_one`,
		charMercSchema,
		`INSERT INTO charmerc (slot, ` + cols + `) SELECT 1, ` + cols + ` FROM charmerc_one`,
		`DROP TABLE charmerc_one`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("charmerc: its slots: %w", err)
		}
	}
	return tx.Commit()
}

// MercRow is one of a character's hires.
type MercRow struct {
	ServerID                               uint32
	Slot                                   int // 1 to MercSlots (0 writes as 1)
	Name                                   string
	Class, Race, Sex                       int
	Face, HairColor, HairLength, HairStyle int
	Stance                                 int
	Path                                   int       // its class path, -1 its class's default
	Pet                                    string    // the pet kind its hirer picked, "" its best
	DeadUntil                              time.Time // zero: alive
	Hired                                  time.Time
}

const mercCols = `serverid, slot, name, class, race, sex, face, hair_color, hair_length, hair_style, stance, dead_until, hired, path, pet`

// scanMerc reads a row of mercCols.
func scanMerc(sc interface{ Scan(...any) error }) (MercRow, error) {
	var r MercRow
	var dead, hired string
	if err := sc.Scan(&r.ServerID, &r.Slot, &r.Name, &r.Class, &r.Race, &r.Sex, &r.Face, &r.HairColor, &r.HairLength, &r.HairStyle,
		&r.Stance, &dead, &hired, &r.Path, &r.Pet); err != nil {
		return MercRow{}, err
	}
	r.DeadUntil, _ = time.Parse(time.RFC3339, dead)
	r.Hired, _ = time.Parse(time.RFC3339, hired)
	return r, nil
}

// Merc is the character's hire of the slot, false for none.
func (db *SQLiteDB) Merc(serverID uint32, slot int) (MercRow, bool, error) {
	r, err := scanMerc(db.QueryRow(`SELECT `+mercCols+` FROM charmerc WHERE serverid = ? AND slot = ?`, serverID, max(slot, 1)))
	if errors.Is(err, sql.ErrNoRows) {
		return MercRow{}, false, nil
	}
	if err != nil {
		return MercRow{}, false, err
	}
	return r, true, nil
}

// Mercs is the character's hires, by slot.
func (db *SQLiteDB) Mercs(serverID uint32) ([]MercRow, error) {
	rows, err := db.Query(`SELECT `+mercCols+` FROM charmerc WHERE serverid = ? ORDER BY slot`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MercRow
	for rows.Next() {
		r, err := scanMerc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetMerc writes the character's hire of the row's slot in place of any it
// had there.
func (db *SQLiteDB) SetMerc(r MercRow) error {
	dead, hired := "", ""
	if !r.DeadUntil.IsZero() {
		dead = r.DeadUntil.UTC().Format(time.RFC3339)
	}
	if !r.Hired.IsZero() {
		hired = r.Hired.UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO charmerc (`+mercCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ServerID,
		max(r.Slot, 1), r.Name, r.Class, r.Race, r.Sex, r.Face, r.HairColor, r.HairLength, r.HairStyle, r.Stance, dead, hired, r.Path, r.Pet)
	return err
}

// SetMercPath keeps the character's merc's class path (-1 its class's
// default).
func (db *SQLiteDB) SetMercPath(serverID uint32, slot, path int) error {
	_, err := db.Exec(`UPDATE charmerc SET path = ? WHERE serverid = ? AND slot = ?`, path, serverID, max(slot, 1))
	return err
}

// SetMercPet keeps the pet kind the character's merc summons ("" its best).
func (db *SQLiteDB) SetMercPet(serverID uint32, slot int, pet string) error {
	_, err := db.Exec(`UPDATE charmerc SET pet = ? WHERE serverid = ? AND slot = ?`, pet, serverID, max(slot, 1))
	return err
}

// SetMercStance keeps the character's merc's stance.
func (db *SQLiteDB) SetMercStance(serverID uint32, slot, stance int) error {
	_, err := db.Exec(`UPDATE charmerc SET stance = ? WHERE serverid = ? AND slot = ?`, stance, serverID, max(slot, 1))
	return err
}

// SetMercRest keeps when the character's merc's rest after its death ends
// (zero: alive again).
func (db *SQLiteDB) SetMercRest(serverID uint32, slot int, until time.Time) error {
	v := ""
	if !until.IsZero() {
		v = until.UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(`UPDATE charmerc SET dead_until = ? WHERE serverid = ? AND slot = ?`, v, serverID, max(slot, 1))
	return err
}

// DeleteMerc ends the character's hire of the slot.
func (db *SQLiteDB) DeleteMerc(serverID uint32, slot int) error {
	_, err := db.Exec(`DELETE FROM charmerc WHERE serverid = ? AND slot = ?`, serverID, max(slot, 1))
	return err
}

// DeleteMercs ends every hire of the character's (its deletion).
func (db *SQLiteDB) DeleteMercs(serverID uint32) error {
	_, err := db.Exec(`DELETE FROM charmerc WHERE serverid = ?`, serverID)
	return err
}

// FamiliarRow is a familiar as table familiars has it.
type FamiliarRow struct {
	PatternID   int
	Name        string
	Model       uint32
	Size        float64
	HP, Regen   int
	CastMS      int
	CastFX      uint32
	Heal        int
	HealBelow   int
	HealEveryMS int
	HealFX      uint32
}

// Familiar is the familiar the item calls, false for an item that calls none.
func (db *SQLiteDB) Familiar(patternID int) (FamiliarRow, bool, error) {
	var r FamiliarRow
	var model, castFX, healFX int64
	err := db.QueryRow(`SELECT patternid, name, model, size, hp, regen, cast_ms, cast_fx, heal, heal_below, heal_every_ms, heal_fx
		FROM familiars WHERE patternid = ?`, patternID).Scan(&r.PatternID, &r.Name, &model, &r.Size, &r.HP, &r.Regen, &r.CastMS,
		&castFX, &r.Heal, &r.HealBelow, &r.HealEveryMS, &healFX)
	if errors.Is(err, sql.ErrNoRows) {
		return FamiliarRow{}, false, nil
	}
	if err != nil {
		return FamiliarRow{}, false, err
	}
	r.Model, r.CastFX, r.HealFX = uint32(model), uint32(castFX), uint32(healFX)
	return r, true, nil
}

// FamiliarItems is the items that call a familiar, by their record ids.
func (db *SQLiteDB) FamiliarItems() (map[int]bool, error) {
	rows, err := db.Query(`SELECT patternid FROM familiars`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

package database

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cmcatalog: every class mastery a character may buy, a row a rank, and
// charcms: the ranks each character bought (entity/cmbuy.go,
// handlers/classmastery.go). The client's window (decomp
// viwndclassmastery*.cpp) lists them in five categories, CLIENT's own
// words: 0 General, 1 Racial, 2 Archetype, 3 Class, 4 Master Class.
//
//   - id is the skill id the client sends to browse, buy and remove (0x1401
//     sub-ops 5, 2, 3) and the key its window chains ranks by; parent is the
//     previous rank's id (0 for a first rank), which must be owned first.
//   - classes and races are bit sets as itempattern's classuse and raceuse
//     (bit 1<<class, 1<<race; 32767 every class, 2047 every race, the
//     froglok's 1<<10 included): who the rank is listed for. An archetype's
//     rows carry its classes.
//   - cost, level and spent (the points the character must have spent
//     already) are the purchase's; minstats the base stats it needs
//     ("STR:100,STA:150"); requires the abilities it needs ("Name@level;
//     ...", shown) and require_spells the spellpattern ids they were found
//     as, which the spell book must hold (a name not found is shown and not
//     checked); require_ranks other masteries' ranks the character must own
//     first, their ids ("6634"; the rank before it in its own line is
//     parent).
//   - effects is what it gives, by EQOA-Data's stat names ("STR:1,STRMax:15",
//     "HP Factor:1", "Pow:500", "HoT:5", "MovRate:5", resists "FR".."DR");
//     adds and removes the abilities it gives and takes ("Name@level; ...",
//     shown in the window) and add_spells / remove_spells the spellpattern
//     ids they were found as (a name not found grants nothing until a GM
//     sets the id).
//   - enabled 0 hides a row from everyone; source is "gm" or the seeding
//     tool's (tools/seed_masteries.py, whose --undo deletes its rows).
const cmCatalogSchema = `
	CREATE TABLE IF NOT EXISTS cmcatalog (
		id            INTEGER PRIMARY KEY,
		category      INTEGER NOT NULL DEFAULT 0,
		classes       INTEGER NOT NULL DEFAULT 32767,
		races         INTEGER NOT NULL DEFAULT 2047,
		mastery       TEXT NOT NULL DEFAULT '',
		rank          INTEGER NOT NULL DEFAULT 1,
		name          TEXT NOT NULL DEFAULT '',
		parent        INTEGER NOT NULL DEFAULT 0,
		cost          INTEGER NOT NULL DEFAULT 0,
		level         INTEGER NOT NULL DEFAULT 1,
		spent         INTEGER NOT NULL DEFAULT 0,
		minstats      TEXT NOT NULL DEFAULT '',
		requires      TEXT NOT NULL DEFAULT '',
		require_spells TEXT NOT NULL DEFAULT '',
		require_ranks TEXT NOT NULL DEFAULT '',
		effects       TEXT NOT NULL DEFAULT '',
		adds          TEXT NOT NULL DEFAULT '',
		add_spells    TEXT NOT NULL DEFAULT '',
		removes       TEXT NOT NULL DEFAULT '',
		remove_spells TEXT NOT NULL DEFAULT '',
		description   TEXT NOT NULL DEFAULT '',
		enabled       INTEGER NOT NULL DEFAULT 1,
		source        TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS charcms (
		serverid INTEGER NOT NULL,
		cmid     INTEGER NOT NULL,
		cost     INTEGER NOT NULL DEFAULT 0,
		bought   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (serverid, cmid)
	);`

// ensureCMCatalog makes tables cmcatalog and charcms (initSchema), adding
// require_ranks to a table made before it.
func (db *SQLiteDB) ensureCMCatalog() error {
	if _, err := db.Exec(cmCatalogSchema); err != nil {
		return fmt.Errorf("cmcatalog: %w", err)
	}
	has, err := hasColumn(db.DB, "cmcatalog", "require_ranks")
	if err != nil {
		return fmt.Errorf("cmcatalog: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE cmcatalog ADD COLUMN require_ranks TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("cmcatalog: %w", err)
		}
	}
	return nil
}

// CMRankRow is one row of cmcatalog.
type CMRankRow struct {
	ID, Category, Classes, Races int
	Mastery                      string
	Rank                         int
	Name                         string
	Parent, Cost, Level, Spent   int
	MinStats, Effects            string
	Requires, RequireSpells      string
	RequireRanks                 string
	Adds, AddSpells              string
	Removes, RemoveSpells        string
	Description                  string
	Enabled                      bool
	Source                       string
}

// CMCatalog is every row of cmcatalog, by id order.
func (db *SQLiteDB) CMCatalog() ([]CMRankRow, error) {
	rows, err := db.Query(`SELECT id, category, classes, races, mastery, rank, name, parent, cost, level, spent,
		minstats, requires, require_spells, require_ranks, effects, adds, add_spells, removes, remove_spells, description,
		enabled, source FROM cmcatalog ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CMRankRow
	for rows.Next() {
		var r CMRankRow
		var enabled int
		if err := rows.Scan(&r.ID, &r.Category, &r.Classes, &r.Races, &r.Mastery, &r.Rank, &r.Name, &r.Parent,
			&r.Cost, &r.Level, &r.Spent, &r.MinStats, &r.Requires, &r.RequireSpells, &r.RequireRanks, &r.Effects, &r.Adds, &r.AddSpells,
			&r.Removes, &r.RemoveSpells, &r.Description, &enabled, &r.Source); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// OwnedCM is a rank a character bought and what it paid.
type OwnedCM struct {
	ID, Cost int
}

// LoadOwnedCMs is the ranks a character bought.
func (s *SQLiteCharacterStore) LoadOwnedCMs(serverID uint32) ([]OwnedCM, error) {
	rows, err := s.db.Query(`SELECT cmid, cost FROM charcms WHERE serverid = ? ORDER BY cmid`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OwnedCM
	for rows.Next() {
		var o OwnedCM
		if err := rows.Scan(&o.ID, &o.Cost); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// BuyCM keeps a purchase: the rank owned and the points as they are now, in
// one transaction, so a crash between them leaves neither.
func (s *SQLiteCharacterStore) BuyCM(serverID uint32, o OwnedCM, m MasteryRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO charcms (serverid, cmid, cost, bought) VALUES (?, ?, ?, ?)`,
		serverID, o.ID, o.Cost, time.Now().Unix()); err != nil {
		return err
	}
	if err := saveMasteryTx(tx, serverID, m); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveCMs keeps a removal: the ranks no longer owned and the points as
// they are now, in one transaction.
func (s *SQLiteCharacterStore) RemoveCMs(serverID uint32, ids []int, m MasteryRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM charcms WHERE serverid = ? AND cmid = ?`, serverID, id); err != nil {
			return err
		}
	}
	if err := saveMasteryTx(tx, serverID, m); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveSpellFromBook takes a spell out of a character's book (a mastery
// that replaces it); the client's book shows it gone at the next login.
func (s *SQLiteCharacterStore) RemoveSpellFromBook(serverID uint32, spellID int) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM spells WHERE serverid = ? AND spellid = ?`, serverID, spellID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SpellIDs reads a comma list of spellpattern ids ("123,456"); anything that
// is not a number is skipped.
func SpellIDs(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

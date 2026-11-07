package database

import "fmt"

// racedefs: a race this server has and the game does not, one row a race (the
// user, 2026-11-07: "the hill giant race addition is a test of future playable
// races"; entity/race.go reads it, claude-notes/custom-races-20261107.md is how
// to add one). id is the race as characters.race holds it, past CLIENT's ten and
// the froglok; wire_race the race of CLIENT's its characters pass as on the wire;
// the seven bonuses go on 60 and the class's as a race's do (entity.LookupBaseStats);
// body_male and body_female are the CHAR.ESF DictIDs a character of the race
// wears, signed as characters.modelid holds them, and they are NOT in
// charactermodel: an NPC wearing a body in that table is a townsperson to the
// aggro rule (entity.hostileNatureLocked), so every creature of the race's kind
// would go friendly. The retail races' figures stay in their tables in code.
//
// description, class_mask and city are character creation's (entity/creation.go,
// world.open_races): the race page's text; the classes it may be when every class
// is not open to every race (0: its wire race's, as the game's masks have them);
// and for a race passing as a human, the human city it starts in (1 the east's
// Freeport, 2 the west's Qeynos), 0 for every other.
const raceDefsSchema = `
	CREATE TABLE IF NOT EXISTS racedefs (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL,
		short_name  TEXT NOT NULL DEFAULT '',
		wire_race   INTEGER NOT NULL,
		bonus_str   INTEGER NOT NULL DEFAULT 0,
		bonus_sta   INTEGER NOT NULL DEFAULT 0,
		bonus_agi   INTEGER NOT NULL DEFAULT 0,
		bonus_dex   INTEGER NOT NULL DEFAULT 0,
		bonus_wis   INTEGER NOT NULL DEFAULT 0,
		bonus_int   INTEGER NOT NULL DEFAULT 0,
		bonus_cha   INTEGER NOT NULL DEFAULT 0,
		body_male   INTEGER NOT NULL DEFAULT 0,
		body_female INTEGER NOT NULL DEFAULT 0,
		note        TEXT NOT NULL DEFAULT '',
		source      TEXT NOT NULL DEFAULT 'gm',
		description TEXT NOT NULL DEFAULT '',
		class_mask  INTEGER NOT NULL DEFAULT 0,
		city        INTEGER NOT NULL DEFAULT 0
	);`

// raceDefsAdded are the columns the table gained after it was first made, each
// added to a table made without it, as it is declared above.
var raceDefsAdded = []struct{ col, decl string }{
	{"description", "TEXT NOT NULL DEFAULT ''"},
	{"class_mask", "INTEGER NOT NULL DEFAULT 0"},
	{"city", "INTEGER NOT NULL DEFAULT 0"},
}

// raceDefFromCode is the one race that was code before the table: the row a
// database already holding a character of it is given when the table is made, so
// a binary that starts before any data step finds race 11 as it always was. Once,
// as racesize's first row is: a later edit or delete of it stands, and a database
// with no such character (a new server's) makes the table empty.
const raceDefFromCode = `
	INSERT OR IGNORE INTO racedefs (id, name, short_name, wire_race,
		bonus_str, bonus_sta, bonus_agi, bonus_dex, bonus_wis, bonus_int, bonus_cha,
		body_male, body_female, note, source)
	SELECT 11, 'Hill Giant', 'HGT', 9, 25, 20, -5, -5, 0, 0, 0, -5117396, -5117396,
		'made with the table: the race was code until then', 'seed'
	WHERE EXISTS (SELECT 1 FROM characters WHERE race = 11)`

// ensureRaceDefs makes table racedefs (initSchema).
func (db *SQLiteDB) ensureRaceDefs() error {
	var had int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'racedefs'`).Scan(&had); err != nil {
		return fmt.Errorf("racedefs: %w", err)
	}
	if _, err := db.Exec(raceDefsSchema); err != nil {
		return fmt.Errorf("racedefs: %w", err)
	}
	for _, c := range raceDefsAdded {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('racedefs') WHERE name = ?`, c.col).Scan(&n); err != nil {
			return fmt.Errorf("racedefs: %w", err)
		}
		if n == 0 {
			if _, err := db.Exec(`ALTER TABLE racedefs ADD COLUMN ` + c.col + ` ` + c.decl); err != nil {
				return fmt.Errorf("racedefs: column %s: %w", c.col, err)
			}
		}
	}
	if had == 0 {
		if _, err := db.Exec(raceDefFromCode); err != nil {
			return fmt.Errorf("racedefs: the first row: %w", err)
		}
	}
	return nil
}

// RaceDefRow is one row of racedefs.
type RaceDefRow struct {
	ID                   int
	Name, ShortName      string
	WireRace             int
	Bonus                [7]int // STR STA AGI DEX WIS INT CHA
	BodyMale, BodyFemale int64
	Note, Source         string
	// Desc, ClassMask and City are creation's: the race page's text, the
	// classes it may be (0: its wire race's) and a human-passing race's city.
	Desc      string
	ClassMask int
	City      int
}

// RaceDefRows is every row of racedefs, by id.
func (db *SQLiteDB) RaceDefRows() ([]RaceDefRow, error) {
	rows, err := db.Query(`SELECT id, name, short_name, wire_race, bonus_str, bonus_sta, bonus_agi, bonus_dex,
		bonus_wis, bonus_int, bonus_cha, body_male, body_female, note, source, description, class_mask, city
		FROM racedefs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RaceDefRow
	for rows.Next() {
		var r RaceDefRow
		b := &r.Bonus
		if err := rows.Scan(&r.ID, &r.Name, &r.ShortName, &r.WireRace, &b[0], &b[1], &b[2], &b[3], &b[4], &b[5], &b[6],
			&r.BodyMale, &r.BodyFemale, &r.Note, &r.Source, &r.Desc, &r.ClassMask, &r.City); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RaceCharacter is a character of a race racedefs names: what the client's race
// labels and the start's check of bodies read.
type RaceCharacter struct {
	Name    string
	Race    int
	Sex     int
	ModelID int64
	// GM: its account's acctlevel is 1 or more, so its name is drawn with a tag
	// (entity.GMTag)
	GM bool
}

// CharactersOfRaces is every character of those races, offline ones too, by race
// and then name.
func (db *SQLiteDB) CharactersOfRaces(races []int) ([]RaceCharacter, error) {
	if len(races) == 0 {
		return nil, nil
	}
	in := make([]byte, 0, 2*len(races))
	args := make([]any, len(races))
	for i, r := range races {
		if i > 0 {
			in = append(in, ',')
		}
		in = append(in, '?')
		args[i] = r
	}
	rows, err := db.Query(`SELECT c.charName, c.race, c.sex, c.modelid, COALESCE(a.acctlevel, 0) >= 1 FROM characters c
		LEFT JOIN accountinfo a ON a.accountid = c.accountid WHERE c.race IN (`+string(in)+`) ORDER BY c.race, c.charName`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RaceCharacter
	for rows.Next() {
		var c RaceCharacter
		if err := rows.Scan(&c.Name, &c.Race, &c.Sex, &c.ModelID, &c.GM); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

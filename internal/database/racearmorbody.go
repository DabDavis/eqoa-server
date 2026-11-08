package database

import "fmt"

// racearmorbody: the body a race shows for the kind of armour on its chest (entity/armorbody.go),
// for a race whose own body takes no armour (a creature's: the hill giant's has no TSlotList, so no
// client draws armour on it). race is a racedefs id (or a game race), armor one of cloth, leather,
// chain or plate, body a CHAR.ESF CSprite DictID. A kind with no row shows the race's own body.
const raceArmorBodySchema = `
	CREATE TABLE IF NOT EXISTS racearmorbody (
		race   INTEGER NOT NULL,
		armor  TEXT NOT NULL,
		body   INTEGER NOT NULL,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (race, armor)
	);`

// ensureRaceArmorBody makes table racearmorbody (initSchema).
func (db *SQLiteDB) ensureRaceArmorBody() error {
	if _, err := db.Exec(raceArmorBodySchema); err != nil {
		return fmt.Errorf("racearmorbody: %w", err)
	}
	return nil
}

// RaceArmorBody is one row of racearmorbody.
type RaceArmorBody struct {
	Race  int
	Armor string
	Body  uint32
}

// RaceArmorBodies is every row of racearmorbody.
func (db *SQLiteDB) RaceArmorBodies() ([]RaceArmorBody, error) {
	rows, err := db.Query(`SELECT race, armor, body FROM racearmorbody`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RaceArmorBody
	for rows.Next() {
		var r RaceArmorBody
		var body int64
		if err := rows.Scan(&r.Race, &r.Armor, &body); err != nil {
			return nil, err
		}
		r.Body = uint32(body)
		out = append(out, r)
	}
	return out, rows.Err()
}

package database

import (
	"fmt"
	"strings"
)

// gemsetting.go — Hogarth Bartholomew's gem setting (handlers/gemsetter.go): the Gems of
// Khalimet set in the Khal weapons and taken out again. Quest "Gnadriin" ends at him with
// "Khalimet's gems fit the warlords' weapons. For 10,000 tunar I will set one, for 2000 take
// it out" (EQOA-Data's harbingers-of-khalimet p5), and the live game's films show what a set
// gem made: a weapon named by the gem's colour, the plain weapon with the gem's stats ("Green
// Bloodplague", "Yellow Bloodplague", "White Bloodplague": the White one's damage 700 to 900),
// and a gem taken out of one weapon and set in another character's ("i need to get white gem
// off ranger and put on this toon again"). tools/harbingers_20261122.py writes the rows, by
// name on each shard (the shards' ids differ).
//
// A row names an item record (patternid) and what it is to the setter: kind "gem" (a Gem of
// Khalimet, colour its colour) or "weapon" (family the plain weapon's name, lower case;
// colour "" the plain weapon, else the gem set in it). canonical 1 is the record the setter
// hands out for its kind, family and colour; a record of canonical 0 is taken in as that
// one (a transfer's imported copy of a coloured weapon, which nobody else can turn back).
const gemSettingSchema = `
	CREATE TABLE IF NOT EXISTS gemsetting (
		patternid INTEGER PRIMARY KEY,
		kind      TEXT NOT NULL DEFAULT 'weapon',
		family    TEXT NOT NULL DEFAULT '',
		colour    TEXT NOT NULL DEFAULT '',
		canonical INTEGER NOT NULL DEFAULT 1,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// The kinds of gemsetting.kind.
const (
	GemKindGem    = "gem"
	GemKindWeapon = "weapon"
)

// GemSettingRow is a row of gemsetting.
type GemSettingRow struct {
	PatternID int
	Kind      string // GemKindGem, GemKindWeapon
	Family    string // a weapon's plain name, lower case; "" for a gem
	Colour    string // the gem's colour, lower case; "" for a plain weapon
	Canonical bool
}

// ensureGemSetting makes table gemsetting (initSchema).
func (db *SQLiteDB) ensureGemSetting() error {
	if _, err := db.Exec(gemSettingSchema); err != nil {
		return fmt.Errorf("gemsetting: %w", err)
	}
	return nil
}

// GemSettings is every row of gemsetting, by record; none for none. A row of an unknown
// kind, a weapon with no family or a gem with no colour is left out.
func (db *SQLiteDB) GemSettings() ([]GemSettingRow, error) {
	if err := db.ensureGemSetting(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT patternid, kind, family, colour, canonical FROM gemsetting ORDER BY patternid`)
	if err != nil {
		return nil, fmt.Errorf("gemsetting: %w", err)
	}
	defer rows.Close()
	var out []GemSettingRow
	for rows.Next() {
		var r GemSettingRow
		var canon int
		if err := rows.Scan(&r.PatternID, &r.Kind, &r.Family, &r.Colour, &canon); err != nil {
			return nil, fmt.Errorf("gemsetting: %w", err)
		}
		r.Kind = strings.ToLower(strings.TrimSpace(r.Kind))
		r.Family = strings.ToLower(strings.TrimSpace(r.Family))
		r.Colour = strings.ToLower(strings.TrimSpace(r.Colour))
		r.Canonical = canon != 0
		switch {
		case r.Kind == GemKindGem && r.Colour != "":
			r.Family = ""
		case r.Kind == GemKindWeapon && r.Family != "":
		default:
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

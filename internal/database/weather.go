package database

import (
	"fmt"

	"github.com/eqoa/server/internal/weather"
)

// The regional weather's tables (#1549, internal/weather): which climate each
// of the disc's terrain profiles is (weatherprofiles), a GM's word for a
// whole zone above them (weatherzones), and how each climate's weather comes
// (weatherclimates). Rows of source 'seed' are tools/seed_weather.py's. Empty
// weatherprofiles and weatherzones: no automatic weather; a GM's still works.
const weatherSchema = `
	CREATE TABLE IF NOT EXISTS weatherprofiles (
		profile INTEGER PRIMARY KEY,
		climate TEXT NOT NULL DEFAULT 'temperate',
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS weatherzones (
		world   INTEGER NOT NULL,
		col     INTEGER NOT NULL,
		row     INTEGER NOT NULL,
		climate TEXT NOT NULL DEFAULT 'none',
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (world, col, row)
	);
	CREATE TABLE IF NOT EXISTS weatherclimates (
		climate         TEXT PRIMARY KEY,
		kind            TEXT NOT NULL,
		cycle_minutes   INTEGER NOT NULL DEFAULT 40,
		event_chance    REAL NOT NULL DEFAULT 1,
		event_min       INTEGER NOT NULL,
		event_max       INTEGER NOT NULL,
		weight_light    INTEGER NOT NULL DEFAULT 60,
		weight_moderate INTEGER NOT NULL DEFAULT 30,
		weight_heavy    INTEGER NOT NULL DEFAULT 10,
		whiteout_chance REAL NOT NULL DEFAULT 0,
		whiteout_min    INTEGER NOT NULL DEFAULT 30,
		whiteout_max    INTEGER NOT NULL DEFAULT 60,
		fade_seconds    INTEGER NOT NULL DEFAULT 30,
		note            TEXT NOT NULL DEFAULT ''
	);`

// ensureWeather makes the three tables (initSchema).
func (db *SQLiteDB) ensureWeather() error {
	if _, err := db.Exec(weatherSchema); err != nil {
		return fmt.Errorf("weather tables: %w", err)
	}
	return nil
}

// WeatherRules reads the three tables: the rules, and each bad row's
// problem (the row left out).
func (db *SQLiteDB) WeatherRules() (weather.Rules, []error, error) {
	r := weather.Rules{Profiles: map[int]weather.Climate{}, Zones: map[weather.Zone]weather.Climate{}, Climates: map[weather.Climate]weather.Rule{}}
	var bad []error
	rows, err := db.Query(`SELECT profile, climate FROM weatherprofiles`)
	if err != nil {
		return r, nil, err
	}
	for rows.Next() {
		var p int
		var c string
		if err := rows.Scan(&p, &c); err != nil {
			rows.Close()
			return r, nil, err
		}
		if err := weather.CheckProfileRow(p, c); err != nil {
			bad = append(bad, fmt.Errorf("weatherprofiles %d: %w", p, err))
			continue
		}
		cl, _ := weather.ParseClimate(c)
		r.Profiles[p] = cl
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return r, nil, err
	}
	rows, err = db.Query(`SELECT world, col, row, climate FROM weatherzones`)
	if err != nil {
		return r, nil, err
	}
	for rows.Next() {
		var w, c, rw int
		var cl string
		if err := rows.Scan(&w, &c, &rw, &cl); err != nil {
			rows.Close()
			return r, nil, err
		}
		if err := weather.CheckZoneRow(w, c, rw, cl); err != nil {
			bad = append(bad, fmt.Errorf("weatherzones w%d-c%d-r%d: %w", w, c, rw, err))
			continue
		}
		k, _ := weather.ParseClimate(cl)
		r.Zones[weather.Zone{World: w, Col: c, Row: rw}] = k
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return r, nil, err
	}
	rows, err = db.Query(`SELECT climate, kind, cycle_minutes, event_chance, event_min, event_max, weight_light, weight_moderate,
		weight_heavy, whiteout_chance, whiteout_min, whiteout_max, fade_seconds FROM weatherclimates`)
	if err != nil {
		return r, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cl, kind string
		var x weather.Rule
		if err := rows.Scan(&cl, &kind, &x.CycleMinutes, &x.EventChance, &x.EventMin, &x.EventMax, &x.WeightLight, &x.WeightModerate,
			&x.WeightHeavy, &x.WhiteoutChance, &x.WhiteoutMin, &x.WhiteoutMax, &x.FadeSeconds); err != nil {
			return r, nil, err
		}
		x.Climate = weather.Climate(cl)
		k, ok := weather.ParseKind(kind)
		if !ok {
			bad = append(bad, fmt.Errorf("weatherclimates %s: kind %q is rain, snow or sand", cl, kind))
			continue
		}
		x.Kind = k
		if err := x.Check(); err != nil {
			bad = append(bad, fmt.Errorf("weatherclimates %s: %w", cl, err))
			continue
		}
		r.Climates[x.Climate] = x
	}
	return r, bad, rows.Err()
}

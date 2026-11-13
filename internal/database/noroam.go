package database

import (
	"fmt"
	"math"
	"strings"
)

// noroam: ground no wandering creature walks into or through (entity/noroam.go; the GM Jibwiz's
// petitions 1963-1965, 2026-11-13, three kobold camps in Odus: "This kobold camp beneath me needs to
// not have wildlife in this area pathing into it. It can get close, but never through the tented
// camp"). One row a circle: its middle (x, z) in world and its radius across. A creature whose
// spawn is in it is its own (the camp's) and walks it as ever; a fight's chase and the walk home
// from one are not wanders. name is the area as a GM reads it, note where it came from; source gm,
// or a tool's (its --undo deletes those no GM changed).
const noRoamSchema = `
	CREATE TABLE IF NOT EXISTS noroam (
		id     INTEGER PRIMARY KEY,
		name   TEXT NOT NULL DEFAULT '',
		world  INTEGER NOT NULL DEFAULT 0,
		x      REAL NOT NULL DEFAULT 0,
		z      REAL NOT NULL DEFAULT 0,
		radius REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// NoRoam is one row of noroam.
type NoRoam struct {
	ID           int64
	Name         string
	World        uint8
	X, Z, Radius float32
	Note, Source string
}

// NoRoamRadiusMax is the widest a no-roam area may be.
const NoRoamRadiusMax = 1000

// CheckNoRoam is what a row must be for the world to keep wanderers out by it: a world of the
// client's table (0-29), a middle within 200000 of the world's, a radius of 1 to NoRoamRadiusMax.
func CheckNoRoam(a NoRoam, world int) error {
	for _, v := range []float32{a.X, a.Z, a.Radius} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > hazardFar {
			return fmt.Errorf("a number that is no place (%v)", v)
		}
	}
	switch {
	case world < 0 || world > 29:
		return fmt.Errorf("world %d: 0 to 29", world)
	case a.Radius < 1 || a.Radius > NoRoamRadiusMax:
		return fmt.Errorf("radius %.1f: 1 to %d", a.Radius, NoRoamRadiusMax)
	}
	return nil
}

// ensureNoRoam makes table noroam (initSchema).
func (db *SQLiteDB) ensureNoRoam() error {
	if _, err := db.Exec(noRoamSchema); err != nil {
		return fmt.Errorf("noroam: %w", err)
	}
	return nil
}

// NoRoamAreas is every row of noroam the world can use, in id order; a row CheckNoRoam refuses is
// left out and named in bad.
func (db *SQLiteDB) NoRoamAreas() (out []NoRoam, bad []string, err error) {
	rows, err := db.Query(`SELECT id, name, world, x, z, radius, note, source FROM noroam ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a NoRoam
		var world int
		if err := rows.Scan(&a.ID, &a.Name, &world, &a.X, &a.Z, &a.Radius, &a.Note, &a.Source); err != nil {
			return nil, nil, err
		}
		a.Name = strings.TrimSpace(a.Name)
		if err := CheckNoRoam(a, world); err != nil {
			bad = append(bad, fmt.Sprintf("%d %q: %v", a.ID, a.Name, err))
			continue
		}
		a.World = uint8(world)
		out = append(out, a)
	}
	return out, bad, rows.Err()
}

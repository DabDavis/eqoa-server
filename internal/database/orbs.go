package database

import (
	"fmt"
	"math"
	"strings"
)

// orbs: the teleport orbs, places a player steps into to be sent to another
// (entity/orbs.go; the user, 2026-11-09: "the erudite starts that are in towers
// the orb wont teleport them yet to the main highbourne"). One row an orb: its
// place (world, x, y, z: the middle of what a player steps on, y its floor),
// how near it a player's feet must come (radius, across; 0 the server's,
// OrbDefaultRadius), and where it sends (to_world, to_x, to_y, to_z: a spot to
// stand on beside the orb it pairs with, never in it, to_facing the way the
// player then faces, in radians as a 0x40's heading). name is the orb as a GM
// reads it, note where it came from. source: gm, or seed for
// tools/seed_orbs.py's rows (its --undo deletes those no GM changed).
const orbsSchema = `
	CREATE TABLE IF NOT EXISTS orbs (
		id        INTEGER PRIMARY KEY,
		name      TEXT NOT NULL DEFAULT '',
		world     INTEGER NOT NULL DEFAULT 0,
		x         REAL NOT NULL DEFAULT 0,
		y         REAL NOT NULL DEFAULT 0,
		z         REAL NOT NULL DEFAULT 0,
		radius    REAL NOT NULL DEFAULT 0,
		to_world  INTEGER NOT NULL DEFAULT 0,
		to_x      REAL NOT NULL DEFAULT 0,
		to_y      REAL NOT NULL DEFAULT 0,
		to_z      REAL NOT NULL DEFAULT 0,
		to_facing REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// Orb is one row of orbs.
type Orb struct {
	ID       int64
	Name     string
	World    uint8
	X, Y, Z  float32
	Radius   float32 // across, round (X, Z); 0 the server's
	ToWorld  uint8
	ToX      float32
	ToY      float32
	ToZ      float32
	ToFacing float32
}

// How near an orb a player stands to be in it (entity/orbs.go): within its
// radius across (OrbDefaultRadius where the row gives 0), its feet at most
// OrbBelow under the orb's height and OrbAbove over it (on top of a pad, or a
// jump onto one, and not the floor over or under it). orbFar is how far from
// the world's middle a place may be (the panel's bound for a place, as
// spellgates'), OrbRadiusMax the most a radius may be.
const (
	OrbDefaultRadius float32 = 1.5
	OrbBelow         float32 = 1.0
	OrbAbove         float32 = 2.5
	orbFar                   = 200000
	OrbRadiusMax             = 50
)

// CheckOrb is what a row must be for the game to send anyone by it: worlds of
// the client's table (0-29), finite numbers within orbFar of the middle, a
// radius of 0 (the server's) up to OrbRadiusMax, and a destination outside
// its own orb (one that sent a player to where it stands would send it on and
// on).
func CheckOrb(o Orb, world, toWorld int) error {
	for _, v := range []float32{o.X, o.Y, o.Z, o.ToX, o.ToY, o.ToZ, o.ToFacing, o.Radius} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > orbFar {
			return fmt.Errorf("a number that is no place (%v)", v)
		}
	}
	switch {
	case world < 0 || world > 29:
		return fmt.Errorf("world %d: 0 to 29", world)
	case toWorld < 0 || toWorld > 29:
		return fmt.Errorf("to_world %d: 0 to 29", toWorld)
	case o.Radius < 0 || o.Radius > OrbRadiusMax:
		return fmt.Errorf("radius %.1f: 0 (the server's) to %d", o.Radius, OrbRadiusMax)
	}
	if world == toWorld && OrbHolds(o, o.ToX, o.ToY, o.ToZ) {
		return fmt.Errorf("it sends to a spot inside itself (%.1f, %.1f, %.1f)", o.ToX, o.ToY, o.ToZ)
	}
	return nil
}

// OrbHolds reports whether a body whose feet are at (x, y, z), in the orb's
// world, stands in it: within its radius across, its feet from OrbBelow under
// its height to OrbAbove over it.
func OrbHolds(o Orb, x, y, z float32) bool {
	r := o.Radius
	if r <= 0 {
		r = OrbDefaultRadius
	}
	dx, dz := x-o.X, z-o.Z
	return dx*dx+dz*dz <= r*r && y >= o.Y-OrbBelow && y <= o.Y+OrbAbove
}

// ensureOrbs makes table orbs (initSchema).
func (db *SQLiteDB) ensureOrbs() error {
	if _, err := db.Exec(orbsSchema); err != nil {
		return fmt.Errorf("orbs: %w", err)
	}
	return nil
}

// Orbs is every row of orbs the game can send anyone by, in id order; a row
// CheckOrb refuses is left out and named in bad.
func (db *SQLiteDB) Orbs() (orbs []Orb, bad []string, err error) {
	rows, err := db.Query(`SELECT id, name, world, x, y, z, radius, to_world, to_x, to_y, to_z, to_facing FROM orbs ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o Orb
		var world, toWorld int
		if err := rows.Scan(&o.ID, &o.Name, &world, &o.X, &o.Y, &o.Z, &o.Radius, &toWorld, &o.ToX, &o.ToY, &o.ToZ, &o.ToFacing); err != nil {
			return nil, nil, err
		}
		o.Name = strings.TrimSpace(o.Name)
		if err := CheckOrb(o, world, toWorld); err != nil {
			bad = append(bad, fmt.Sprintf("%d %q: %v", o.ID, o.Name, err))
			continue
		}
		o.World, o.ToWorld = uint8(world), uint8(toWorld)
		orbs = append(orbs, o)
	}
	return orbs, bad, rows.Err()
}

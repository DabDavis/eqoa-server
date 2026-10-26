package database

import (
	"fmt"
	"strings"
)

// spellgates: where the spells that teleport send, and whom (entity/gates.go;
// jib's review, F3 pass 19: the wizard's gates, portals and evacuations and
// the druid's circles, 27 spells, took their power and did nothing). One row
// a spell, keyed by its name in lower case (SpellGateKey: every spellpattern
// record of that name). kind is whom it sends: self the caster, group the
// caster and its group within radius of it (0: the server's, 20), evac the
// group within radius to the caster's bind point (its Return Home), the
// row's place unused. place is the place as a player reads it; world, x, y, z
// where it lands, the server finding open, dry ground near it when it has
// the ground files. source: gm, or seed for tools/seed_spell_gates.py's rows
// (its --undo deletes those no GM changed).
const spellGatesSchema = `
	CREATE TABLE IF NOT EXISTS spellgates (
		spell  TEXT PRIMARY KEY,
		kind   TEXT NOT NULL DEFAULT 'self',
		place  TEXT NOT NULL DEFAULT '',
		world  INTEGER NOT NULL DEFAULT 0,
		x      REAL NOT NULL DEFAULT 0,
		y      REAL NOT NULL DEFAULT 0,
		z      REAL NOT NULL DEFAULT 0,
		radius REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// The kinds of spellgates.kind.
const (
	GateSelf  = "self"
	GateGroup = "group"
	GateEvac  = "evac"
)

// SpellGate is one row of spellgates.
type SpellGate struct {
	Spell   string // SpellGateKey of the spell's name
	Kind    string // GateSelf, GateGroup or GateEvac
	Place   string
	World   uint8
	X, Y, Z float32
	Radius  float32 // a group's reach round its caster; 0 the server's
}

// SpellGateKey is a spell's name as spellgates keys it: lower case, trimmed.
func SpellGateKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// CheckSpellGate is what a row must be for the game to send anyone by it: a
// spell, one of the three kinds, a world of the client's table (0-29), and
// no negative radius.
func CheckSpellGate(g SpellGate, world int) error {
	switch {
	case SpellGateKey(g.Spell) == "":
		return fmt.Errorf("no spell")
	case g.Kind != GateSelf && g.Kind != GateGroup && g.Kind != GateEvac:
		return fmt.Errorf("kind %q: self, group or evac", g.Kind)
	case world < 0 || world > 29:
		return fmt.Errorf("world %d: 0 to 29", world)
	case g.Radius < 0:
		return fmt.Errorf("radius %.1f: 0 (the server's) or more", g.Radius)
	}
	return nil
}

// ensureSpellGates makes table spellgates (initSchema).
func (db *SQLiteDB) ensureSpellGates() error {
	if _, err := db.Exec(spellGatesSchema); err != nil {
		return fmt.Errorf("spellgates: %w", err)
	}
	return nil
}

// SpellGates is every row of spellgates a cast can use, keyed by SpellGateKey;
// a row CheckSpellGate refuses is left out and named in bad.
func (db *SQLiteDB) SpellGates() (gates map[string]SpellGate, bad []string, err error) {
	// spell may be NULL (a TEXT key in SQLite may): no spell, no row
	rows, err := db.Query(`SELECT spell, kind, place, world, x, y, z, radius FROM spellgates WHERE spell IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	gates = make(map[string]SpellGate)
	for rows.Next() {
		var g SpellGate
		var world int
		if err := rows.Scan(&g.Spell, &g.Kind, &g.Place, &world, &g.X, &g.Y, &g.Z, &g.Radius); err != nil {
			return nil, nil, err
		}
		g.Spell = SpellGateKey(g.Spell)
		g.Kind = strings.ToLower(strings.TrimSpace(g.Kind))
		if err := CheckSpellGate(g, world); err != nil {
			bad = append(bad, fmt.Sprintf("%q: %v", g.Spell, err))
			continue
		}
		g.World = uint8(world)
		gates[g.Spell] = g
	}
	return gates, bad, rows.Err()
}

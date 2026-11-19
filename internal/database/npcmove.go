package database

import (
	"fmt"
	"math"
)

// npcmove: how the creatures of a name move (F3 20261026-021730, a GM:
// "hexbones in zentaurs keep do NOT roam, they are static spawn until agrod";
// F3 20261026-020316: "Wildlife that is not aquatic should avoid naturally
// roaming into the water ... Caiman, crocodiles, crabs can roam both"; F3
// 20261026-042712: "Not all mobs are wild life. Fire Giants are roaming
// currently when they should not be unless otherwise specified / designated
// as a roamer with a path"), read by entity/movement.go. roam -1, the
// default: as its kind, wildlife wanders and anything else stands
// (entity/wildlife.go); 0: it stands on its spawn until a fight takes it (and
// goes back there after); 1: it wanders, wildlife or not (a designated
// roamer). radius: how far from its spawn it wanders, 0 the server's
// (world.roam_radius). water: 'land' it keeps out of the water as it wanders;
// 'both' it goes in and out of it (crocodiles, caiman, crabs); 'water' it
// keeps to the water, wandering and fighting; empty, by its spawn (water when
// it spawned in the water, else land). speed: its walking pace as it
// wanders (and walks home off a path), units a second, 0 the server's
// (entity.NPCWalkSpeed, 5.0): a body whose walk covers less ground a cycle
// than a man's walks slower (petition 5189, 2026-11-16: "Gharial
// conned runs too fast when roaming"); never its chase. The key is the name
// as npcclass keys it (NPCClassKey). source: gm, or seed for
// tools/seed_npc_move.py's rows (its --undo deletes those).
const npcMoveSchema = `
	CREATE TABLE IF NOT EXISTS npcmove (
		name   TEXT PRIMARY KEY,
		roam   INTEGER NOT NULL DEFAULT -1,
		radius REAL NOT NULL DEFAULT 0,
		water  TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm',
		speed  REAL NOT NULL DEFAULT 0
	);`

// The bounds of npcmove.speed other than 0 (the server's walk): slow enough to
// see it move, and no faster than a chase.
const (
	MinNPCMoveSpeed = 0.5
	MaxNPCMoveSpeed = 9.9
)

// CheckNPCMoveSpeed refuses a speed npcmove cannot hold: 0 (the server's
// walk) or MinNPCMoveSpeed to MaxNPCMoveSpeed units a second.
func CheckNPCMoveSpeed(v float64) error {
	if v == 0 {
		return nil
	}
	if math.IsNaN(v) || v < MinNPCMoveSpeed || v > MaxNPCMoveSpeed {
		return fmt.Errorf("speed is 0 (the server's walk, 5.0) or %g to %g units a second", float64(MinNPCMoveSpeed), float64(MaxNPCMoveSpeed))
	}
	return nil
}

// RoamByKind is npcmove's roam for "as its kind": wildlife wanders, the rest
// stands.
const RoamByKind = -1

// ensureNPCMove makes table npcmove (initSchema), and gives one made before
// roam's "as its kind" (whose roam defaulted to 1, so a row a GM added for its
// water made a creature wander) the default -1: the rows keep their values.
func (db *SQLiteDB) ensureNPCMove() error {
	if _, err := db.Exec(npcMoveSchema); err != nil {
		return fmt.Errorf("npcmove: %w", err)
	}
	dflt, err := columnDefault(db, "npcmove", "roam")
	if err != nil {
		return fmt.Errorf("npcmove: %w", err)
	}
	if dflt == "1" {
		if err := db.npcMoveRoamByKind(); err != nil {
			return err
		}
	}
	// speed (petition 5189): a table made before it gets it, every row 0
	has, err := hasColumn(db.DB, "npcmove", "speed")
	if err != nil {
		return fmt.Errorf("npcmove: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npcmove ADD COLUMN speed REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("npcmove: speed: %w", err)
		}
	}
	return nil
}

// npcMoveRoamByKind remakes a table whose roam defaulted to 1 with the
// default -1 (ensureNPCMove), its rows kept.
func (db *SQLiteDB) npcMoveRoamByKind() error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("npcmove: %w", err)
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE npcmove_kind (
			name   TEXT PRIMARY KEY,
			roam   INTEGER NOT NULL DEFAULT -1,
			radius REAL NOT NULL DEFAULT 0,
			water  TEXT NOT NULL DEFAULT '',
			note   TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'gm'
		)`,
		`INSERT INTO npcmove_kind (name, roam, radius, water, note, source)
			SELECT name, roam, radius, water, note, source FROM npcmove`,
		`DROP TABLE npcmove`,
		`ALTER TABLE npcmove_kind RENAME TO npcmove`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("npcmove: roam's default: %w", err)
		}
	}
	return tx.Commit()
}

// columnDefault is a column's default as the table's schema writes it ("" for
// none).
func columnDefault(db *SQLiteDB, table, col string) (string, error) {
	rs, err := db.Query(`SELECT name, dflt_value FROM pragma_table_info(?)`, table)
	if err != nil {
		return "", err
	}
	defer rs.Close()
	out := ""
	for rs.Next() {
		var name string
		var dflt *string
		if err := rs.Scan(&name, &dflt); err != nil {
			return "", err
		}
		if name == col && dflt != nil {
			out = *dflt
		}
	}
	return out, rs.Err()
}

// NPCMoveRule is a row of npcmove.
type NPCMoveRule struct {
	// ByKind: roam is -1 (or below), "as its kind"; Roam is then not read.
	ByKind bool
	// Roam: 1 wanders, 0 stands (ByKind false).
	Roam   bool
	Radius float32
	Water  string // "" (by its spawn), "land", "both" or "water"
	// Speed is its wandering walk, units a second; 0 the server's
	// (entity.NPCWalkSpeed). One out of CheckNPCMoveSpeed's bounds reads 0.
	Speed float32
}

// NPCMoveRules is every row of npcmove, by NPCClassKey of the name. A water
// word it does not know counts as empty (by its spawn).
func (db *SQLiteDB) NPCMoveRules() (map[string]NPCMoveRule, error) {
	rows, err := db.Query(`SELECT name, roam, radius, water, speed FROM npcmove`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]NPCMoveRule)
	for rows.Next() {
		var name, water string
		var roam int
		var radius, speed float64
		if err := rows.Scan(&name, &roam, &radius, &water, &speed); err != nil {
			return nil, err
		}
		switch water {
		case "land", "both", "water":
		default:
			water = ""
		}
		if CheckNPCMoveSpeed(speed) != nil {
			speed = 0
		}
		out[NPCClassKey(name)] = NPCMoveRule{ByKind: roam < 0, Roam: roam > 0, Radius: float32(max(radius, 0)), Water: water, Speed: float32(speed)}
	}
	return out, rows.Err()
}

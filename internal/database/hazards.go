package database

import (
	"fmt"
	"math"
	"strings"
)

// hazards: places that hurt a player standing in them (entity/hazards.go; the GM Hoofbite's
// petition 1937, 2026-11-13: "spikey vines in toxxulia cave should damage player for 1000 dmg when
// walking through them. player must navigate around vines"). One row an area: a capsule, the
// segment from (x1, y1, z1) to (x2, y2, z2) (the two ends the same for a round area) and radius
// across it; a player's feet within radius of the segment across, and from below under the
// segment's height there to above over it, are in it. damage is what each hit takes (fixed, through
// any armour), every_ms the time from one hit to the next while the player stays in (0: the
// server's, HazardDefaultEvery). words is what the player is told at each hit (%d its damage; empty:
// "<name> hurts you for <n>."). name is the area as a GM reads it, note where it came from. source:
// gm, or a tool's (tools/seed_hazards.py: its --undo deletes those no GM changed). on_move 1 (the GM
// Jibwiz's petition 1960: the thorns, "if they remain in it for 6 seconds THEN MOVE they take 500
// again. However if they stay still it should not continuously damage them") hits a player again
// only once it has moved since its last hit; 0 hits it every every_ms while it stays in, still or
// not. A GM places a round one where it stands with /hazard add (handlers/gmhazard.go).
const hazardsSchema = `
	CREATE TABLE IF NOT EXISTS hazards (
		id       INTEGER PRIMARY KEY,
		name     TEXT NOT NULL DEFAULT '',
		world    INTEGER NOT NULL DEFAULT 0,
		x1       REAL NOT NULL DEFAULT 0,
		y1       REAL NOT NULL DEFAULT 0,
		z1       REAL NOT NULL DEFAULT 0,
		x2       REAL NOT NULL DEFAULT 0,
		y2       REAL NOT NULL DEFAULT 0,
		z2       REAL NOT NULL DEFAULT 0,
		radius   REAL NOT NULL DEFAULT 0,
		below    REAL NOT NULL DEFAULT 0,
		above    REAL NOT NULL DEFAULT 0,
		damage   INTEGER NOT NULL DEFAULT 0,
		every_ms INTEGER NOT NULL DEFAULT 0,
		words    TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm',
		on_move  INTEGER NOT NULL DEFAULT 0
	);`

// Hazard is one row of hazards.
type Hazard struct {
	ID           int64
	Name         string
	World        uint8
	X1, Y1, Z1   float32
	X2, Y2, Z2   float32
	Radius       float32 // across; 0 the server's
	Below, Above float32 // under and over the segment's height; 0 the server's
	Damage       int
	EveryMS      int // 0 the server's
	Words        string
	Note, Source string
	OnMove       bool // hit again only once moved (petition 1960)
}

// The hazards' defaults and bounds: a radius of HazardDefaultRadius, feet from HazardDefaultBelow
// under the segment to HazardDefaultAbove over it, a hit every HazardDefaultEvery ms; hazardFar is
// how far from the world's middle a place may be, HazardRadiusMax the most a radius may be,
// HazardLengthMax the longest segment, HazardDamageMax the most a hit may take.
const (
	HazardDefaultRadius float32 = 1.5
	HazardDefaultBelow  float32 = 2
	HazardDefaultAbove  float32 = 4
	HazardDefaultEvery          = 2000
	hazardFar                   = 200000
	HazardRadiusMax             = 100
	HazardLengthMax             = 500
	HazardDamageMax             = 1000000
)

// RadiusOr, BelowOr, AboveOr and EveryOr are the row's figures, the server's where it gives 0.
func (h Hazard) RadiusOr() float32 { return orF(h.Radius, HazardDefaultRadius) }
func (h Hazard) BelowOr() float32  { return orF(h.Below, HazardDefaultBelow) }
func (h Hazard) AboveOr() float32  { return orF(h.Above, HazardDefaultAbove) }
func (h Hazard) EveryOr() int {
	if h.EveryMS <= 0 {
		return HazardDefaultEvery
	}
	return h.EveryMS
}

func orF(v, d float32) float32 {
	if v <= 0 {
		return d
	}
	return v
}

// CheckHazard is what a row must be for the game to hurt anyone by it: a world of the client's
// table (0-29), finite numbers within hazardFar of the middle, a radius, below and above of 0 (the
// server's) to HazardRadiusMax, a segment no longer than HazardLengthMax, a damage of 1 to
// HazardDamageMax, every_ms 0 (the server's) or 500 to an hour.
func CheckHazard(h Hazard, world int) error {
	for _, v := range []float32{h.X1, h.Y1, h.Z1, h.X2, h.Y2, h.Z2, h.Radius, h.Below, h.Above} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > hazardFar {
			return fmt.Errorf("a number that is no place (%v)", v)
		}
	}
	dx, dy, dz := h.X2-h.X1, h.Y2-h.Y1, h.Z2-h.Z1
	switch {
	case world < 0 || world > 29:
		return fmt.Errorf("world %d: 0 to 29", world)
	case h.Radius < 0 || h.Radius > HazardRadiusMax || h.Below < 0 || h.Below > HazardRadiusMax || h.Above < 0 || h.Above > HazardRadiusMax:
		return fmt.Errorf("radius, below and above: 0 (the server's) to %d", HazardRadiusMax)
	case dx*dx+dy*dy+dz*dz > HazardLengthMax*HazardLengthMax:
		return fmt.Errorf("a segment longer than %d", HazardLengthMax)
	case h.Damage < 1 || h.Damage > HazardDamageMax:
		return fmt.Errorf("damage %d: 1 to %d", h.Damage, HazardDamageMax)
	case h.EveryMS != 0 && (h.EveryMS < 500 || h.EveryMS > 3600000):
		return fmt.Errorf("every_ms %d: 0 (the server's %d) or 500 to 3600000", h.EveryMS, HazardDefaultEvery)
	case strings.Count(h.Words, "%") > strings.Count(h.Words, "%d"):
		return fmt.Errorf("words: no %% but its %%d")
	}
	return nil
}

// HazardHolds reports whether a body whose feet are at (x, y, z), in the area's world, stands in
// it: within its radius across of the nearest point of its segment (across), its feet from below
// under that point's height to above over it.
func HazardHolds(h Hazard, x, y, z float32) bool {
	sx, sz := h.X2-h.X1, h.Z2-h.Z1
	t := float32(0)
	if l := sx*sx + sz*sz; l > 0 {
		t = min(max(((x-h.X1)*sx+(z-h.Z1)*sz)/l, 0), 1)
	}
	px, py, pz := h.X1+t*sx, h.Y1+t*(h.Y2-h.Y1), h.Z1+t*sz
	dx, dz := x-px, z-pz
	r := h.RadiusOr()
	return dx*dx+dz*dz <= r*r && y >= py-h.BelowOr() && y <= py+h.AboveOr()
}

// ensureHazards makes table hazards (initSchema).
func (db *SQLiteDB) ensureHazards() error {
	if _, err := db.Exec(hazardsSchema); err != nil {
		return fmt.Errorf("hazards: %w", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('hazards') WHERE name = 'on_move'`).Scan(&n); err != nil {
		return fmt.Errorf("hazards: %w", err)
	}
	if n == 0 {
		if _, err := db.Exec(`ALTER TABLE hazards ADD COLUMN on_move INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("hazards: %w", err)
		}
	}
	return nil
}

// Hazards is every row of hazards the game can hurt anyone by, in id order; a row CheckHazard
// refuses is left out and named in bad.
func (db *SQLiteDB) Hazards() (out []Hazard, bad []string, err error) {
	rows, err := db.Query(`SELECT id, name, world, x1, y1, z1, x2, y2, z2, radius, below, above, damage, every_ms, words,
		note, source, on_move FROM hazards ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h Hazard
		var world int
		if err := rows.Scan(&h.ID, &h.Name, &world, &h.X1, &h.Y1, &h.Z1, &h.X2, &h.Y2, &h.Z2, &h.Radius, &h.Below, &h.Above,
			&h.Damage, &h.EveryMS, &h.Words, &h.Note, &h.Source, &h.OnMove); err != nil {
			return nil, nil, err
		}
		h.Name = strings.TrimSpace(h.Name)
		if err := CheckHazard(h, world); err != nil {
			bad = append(bad, fmt.Sprintf("%d %q: %v", h.ID, h.Name, err))
			continue
		}
		h.World = uint8(world)
		out = append(out, h)
	}
	return out, bad, rows.Err()
}

// AddHazardAudited keeps h as a new row in world (a GM's /hazard add: source gm), audited; it returns
// its id. A row CheckHazard refuses is not kept.
func (db *SQLiteDB) AddHazardAudited(h Hazard, world int, who GMEdit, detail string) (int64, error) {
	if err := CheckHazard(h, world); err != nil {
		return 0, err
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, err
	}
	if h.Source == "" {
		h.Source = "gm"
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	id, err := auditedInsert(tx, who, "hazards", "id", detail,
		`INSERT INTO hazards (name, world, x1, y1, z1, x2, y2, z2, radius, below, above, damage, every_ms, words, note,
			source, on_move) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.Name, world, h.X1, h.Y1, h.Z1, h.X2, h.Y2, h.Z2, h.Radius, h.Below, h.Above, h.Damage, h.EveryMS, h.Words,
		h.Note, h.Source, h.OnMove)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// DeleteHazardAudited deletes hazard id (a GM's /hazard remove), audited; false when there is none.
func (db *SQLiteDB) DeleteHazardAudited(id int64, who GMEdit, detail string) (bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	ok, err := auditedDelete(tx, who, "hazards", "id", id, detail)
	if err != nil || !ok {
		return false, err
	}
	return true, tx.Commit()
}

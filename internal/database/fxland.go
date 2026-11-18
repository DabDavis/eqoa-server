package database

import (
	"fmt"
	"math"
)

// fxland: when an animated combat art's hit lands, by the visual its cast plays (work board #1643,
// the GM Jibrish's rule of 2026-11-18, from a player's report that the arts hit too early: "for
// abilities with a delay, make it so the next ability is allowed as soon after [the hit]"). A row is a
// visual's DictID (fxdict, as spelleffect.fxdict stores it: compared as uint32) and land_ms, the time
// from the cast time's end to the hit, in place of the half second every other spell lands after
// (entity/cast.go castLand); a player's cast of it keeps every other cast off until it lands, and an
// art's global cooldown runs to that hit (entity/fxland.go). A visual with no row lands as ever.
// source: gm, or seed for the rows the table is made with.
const fxLandSchema = `
	CREATE TABLE IF NOT EXISTS fxland (
		fxdict  INTEGER PRIMARY KEY,
		land_ms REAL NOT NULL,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

// FXLandMaxMS bounds a row's land_ms: 0 to this.
const FXLandMaxMS = 10000

// FXLandSeed is a row the table is made with.
type FXLandSeed struct {
	FXDict uint32
	LandMS float64
	Note   string
}

// FXLandSeeds are the rows the table is made with (work board #1643): each art's graphic's length. The
// Blades' DictID is the entry's decimal, 129220811, which spelleffect stores for all twelve of them: the
// entry's hex for it (07B3C9CB) was a slip for 07B3C0CB.
var FXLandSeeds = []FXLandSeed{
	{0x07B3C0CB, 2000, "ranger Blades line: Dancing, Whirling, Dizzying, Dazzling, Shimmering, Glimmering (+ and Ascended copies)"},
	{0xEA636DCD, 2000, "warrior Critical and Furious Strike/Assault/Barrage/Flurry/Attack"},
	{0x7B14E621, 2500, "monk Brawling Bear"},
	{0x7B14E622, 2500, "monk Lunging Mantis, Swooping Eagle"},
	{0x7B14E623, 2500, "monk Pouncing Tiger, Roaring Dragon"},
}

// ValidFXLandMS reports whether ms is a land_ms the server takes: 0 to FXLandMaxMS.
func ValidFXLandMS(ms float64) bool {
	return !math.IsNaN(ms) && !math.IsInf(ms, 0) && ms >= 0 && ms <= FXLandMaxMS
}

// ensureFXLand makes table fxland (initSchema), with its seed rows when the table is new: a GM's
// later change or delete of one stands.
func (db *SQLiteDB) ensureFXLand() error {
	var had int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'fxland'`).Scan(&had); err != nil {
		return fmt.Errorf("fxland: %w", err)
	}
	if _, err := db.Exec(fxLandSchema); err != nil {
		return fmt.Errorf("fxland: %w", err)
	}
	if had > 0 {
		return nil
	}
	for _, s := range FXLandSeeds {
		if _, err := db.Exec(`INSERT OR IGNORE INTO fxland (fxdict, land_ms, note, source) VALUES (?, ?, ?, 'seed')`,
			int64(s.FXDict), s.LandMS, s.Note); err != nil {
			return fmt.Errorf("fxland: the seed %#08x: %w", s.FXDict, err)
		}
	}
	return nil
}

// FXLand is every row of fxland the server takes, land_ms by visual (a row out of bounds is left out
// and named in bad).
func (db *SQLiteDB) FXLand() (rows map[uint32]float64, bad []string, err error) {
	q, err := db.Query(`SELECT fxdict, land_ms FROM fxland`)
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()
	rows = make(map[uint32]float64)
	for q.Next() {
		var fx int64
		var ms float64
		if err := q.Scan(&fx, &ms); err != nil {
			return nil, nil, err
		}
		if !ValidFXLandMS(ms) {
			bad = append(bad, fmt.Sprintf("%#08x: land_ms %v (0 to %d)", uint32(fx), ms, FXLandMaxMS))
			continue
		}
		rows[uint32(fx)] = ms
	}
	return rows, bad, q.Err()
}

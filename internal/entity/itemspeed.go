package entity

import (
	"fmt"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// A worn item's own run speed (a GM, 2026-11-12, work board 1258: a sash
// "7% faster, beside any other speed"; table itemspeed). Each worn item's pct
// is a percent of the base run (Player.Speed) added to what the movement rate
// makes of it (SpeedNow), outside the rate's cap: beside Spirit of the Wolf
// (1.30 times the run becomes 1.37 with 7) and the movement masteries. A hold
// or a freeze is still 0, and a slow takes its share of the whole. The worn
// items' sum is Gear.SpeedPct (handlers' wornGear), and a change of it is told
// the client (SetGear: 0x00F8).

var itemSpeeds atomic.Pointer[map[int]float64]

// ItemSpeedPct is a record's own run speed, a percent of the base run; 0 none.
func ItemSpeedPct(patternID int) float64 {
	if m := itemSpeeds.Load(); m != nil {
		return (*m)[patternID]
	}
	return 0
}

// SetItemSpeeds puts the records' run speeds in place (a test's, or
// LoadItemSpeeds').
func SetItemSpeeds(m map[int]float64) {
	itemSpeeds.Store(&m)
}

// LoadItemSpeeds reads table itemspeed: at the start and after a GM's edit. A
// wearer's speed follows at its next change of gear. It reports the rows.
func LoadItemSpeeds(db *database.SQLiteDB) (int, error) {
	m, err := db.ItemSpeeds()
	if err != nil {
		return 0, fmt.Errorf("item speeds: %w", err)
	}
	SetItemSpeeds(m)
	return len(m), nil
}

package entity

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
)

// fxland.go — an animated combat art hits when its graphic ends (work board #1643, a GM's
// rule of 2026-11-18, from a player's report that Dazzling Blades' damage showed long before its 2 s
// sword spin ended: "for abilities with a delay, make it so the next ability is allowed as soon after
// [the hit]"). Table fxland gives a visual's DictID (the one a player's cast plays in its 0x00B1,
// Player.castVisual) its hit's time from the cast time's end. A player's cast of a spell whose visual
// has a row (StartCast):
//
//   - lands at its cast time + that hit, in place of castLand;
//   - keeps every other cast off until it lands: its busy time runs to max(castBusy, the hit), a cast
//     sent within it answered as one sent during another cast (castDuring: not at all);
//   - an art's global cooldown runs to the larger of the arts' cooldown (balance.go ArtCooldown) and
//     its cast time + the hit: after a Dazzling Blades press (0 s cast, 2 s hit, a 0.7 s cooldown) any
//     ability may start at 2.0 s.
//
// Its own lockout and 0x00CD stay as every cast's (cast time + recast); a taunt keeps its own rule
// (taunt.go tauntLockout); a visual with no row lands as ever. Creatures' casts (classtemplate.go
// npcCast, creaturedot.go) keep castLand: this is for players. The fight simulator (balancesim.go)
// times its casts by the same rows (SimPlayer.FXLand).

// SetFXLand puts table fxland's rows (land_ms by visual) in place in the world: the next cast uses them.
func (w *World) SetFXLand(rows map[uint32]float64) {
	m := fxLandDurations(rows)
	w.mu.Lock()
	w.fxLand = m
	w.mu.Unlock()
}

// fxLandDurations is fxland's rows as durations, a row out of bounds (database.ValidFXLandMS) left out.
func fxLandDurations(rows map[uint32]float64) map[uint32]time.Duration {
	m := make(map[uint32]time.Duration, len(rows))
	for fx, ms := range rows {
		if !database.ValidFXLandMS(ms) {
			continue
		}
		m[fx] = time.Duration(math.Round(ms * float64(time.Millisecond)))
	}
	return m
}

// LoadFXLand reads table fxland into the world (SetFXLand): at the start and after a GM's edit. It
// returns how many rows it took. Call it without the database held.
func (w *World) LoadFXLand(db *database.SQLiteDB) (int, error) {
	rows, bad, err := db.FXLand()
	if err != nil {
		return 0, fmt.Errorf("fxland: %w", err)
	}
	for _, b := range bad {
		log.Printf("fxland: %s: left out", b)
	}
	w.SetFXLand(rows)
	return len(rows), nil
}

// FXLandHit is the hit time table fxland gives the visual, as the world has it now, and whether it has one.
func (w *World) FXLandHit(fx uint32) (time.Duration, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	d, ok := w.fxLand[fx]
	return d, ok
}

// fxLandFor is the hit time of a player's cast of the spell (from its cast time's end), and whether
// its visual has a row; a taunt has none (its own rule, taunt.go). Must be called with the lock held.
func (w *World) fxLandFor(p *Player, spell *CastSpell) (time.Duration, bool) {
	if len(w.fxLand) == 0 || isTaunt(spell) {
		return 0, false
	}
	d, ok := w.fxLand[p.castVisual(spell)]
	return d, ok
}

// landAfterLocked is how long after its cast time a player's cast of the spell lands: its visual's
// fxland row, else at once for a heal of no cast time (instantHeal), else castLand. Must be called
// with the lock held.
func (w *World) landAfterLocked(p *Player, spell *CastSpell) time.Duration {
	if d, ok := w.fxLandFor(p, spell); ok {
		return d
	}
	if instantHeal(spell) {
		return 0
	}
	return castLand
}

// instantHeal is whether a player's spell is a heal of no cast time, which lands at once rather than
// castLand on (the GM's petition, 2026-11-20: "Lay hands should fire off its heal instantly when
// used"; a monk's Mend the other such ability): not harmful (a taunt's heal of its caster is a taunt's,
// taunt.go), and hit points given now by one of its effects.
func instantHeal(spell *CastSpell) bool {
	if spell.CastTime != 0 || spell.Def.Harmful {
		return false
	}
	for _, e := range spell.Def.Effects {
		if e.Kind == EffectHP && e.Amount > 0 {
			return true
		}
	}
	return false
}

// simFXLandCache is SimFXLandFromDB once a database (SimKitFromDB, SimStartKitFromDB).
var simFXLandCache sync.Map // *database.SQLiteDB -> map[uint32]time.Duration

// simFXLandOf is SimFXLandFromDB, read once a database.
func simFXLandOf(db *database.SQLiteDB) (map[uint32]time.Duration, error) {
	if v, ok := simFXLandCache.Load(db); ok {
		return v.(map[uint32]time.Duration), nil
	}
	m, err := SimFXLandFromDB(db)
	if err != nil {
		return nil, err
	}
	simFXLandCache.Store(db, m)
	return m, nil
}

// SimFXLandFromDB is table fxland as the fight simulator times casts by it (SimPlayer.FXLand).
func SimFXLandFromDB(db *database.SQLiteDB) (map[uint32]time.Duration, error) {
	rows, _, err := db.FXLand()
	if err != nil {
		return nil, fmt.Errorf("fxland: %w", err)
	}
	return fxLandDurations(rows), nil
}

package entity

import (
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// lockoutsave.go — a player's ability lockouts kept over a logout
// (database/lockouts.go, table charlockouts).
//
// A lockout (cast.go's castLockout: the cast time, the recast and 300 ms from
// the cast's start) lived in the World alone: a takeover and a restart that
// keeps the sessions (takeover.go, keep.go) carried it, and a logout ended
// it, so a player could log out and in again to make every recast ready (the
// owner, 2026-11-09: "when eve players logout and login the recast times of
// abilities are being reset, so can exploit"). Now each end of a session
// writes the lockouts still running and the login reads them back. A recast
// runs on the wall clock: the time logged out counts, as an item's cooldown
// does (potion.go), so what is kept is when each ends, and one over by the
// login is gone. A spell is kept by its id: the display index its badge is
// shown on is the book's at the login (a spell the book has no more keeps its
// lockout shown on nothing, as a book change keeps one: bookchange.go).

// SavedLockouts is a player's lockouts still running at now, as they are
// kept: a spell once, with the later end; ok is false when id is no player
// in the world.
func (w *World) SavedLockouts(id EntityID, now time.Time) (rows []database.LockoutRow, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil {
		return nil, false
	}
	return savedLockouts(p, now), true
}

func savedLockouts(p *Player, now time.Time) []database.LockoutRow {
	var rows []database.LockoutRow
	at := map[int]int{}
	for _, l := range p.Lockouts {
		if !now.Before(l.until) {
			continue
		}
		if i, seen := at[l.spellID]; seen {
			if l.until.After(rows[i].Until) {
				rows[i].Until = l.until
			}
			continue
		}
		at[l.spellID] = len(rows)
		rows = append(rows, database.LockoutRow{SpellID: l.spellID, Until: l.until})
	}
	return rows
}

// RestoreLockouts puts a character's kept lockouts back on sess's player as
// it logs in, each still running at now on the ability index names for its
// spell (the display index its book gives it now, the one its MemoryDump
// sends); a spell index lacks keeps its lockout, refusing the spell, shown on
// no ability. A lockout the player has already of the same spell keeps the
// later end. The badges are SendLockouts', after the dump. It returns how
// many it put back.
func (w *World) RestoreLockouts(sess *session.Session, rows []database.LockoutRow, index map[int]int, now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if !now.Before(r.Until) {
			continue
		}
		at, inBook := index[r.SpellID]
		if !inBook {
			at = -1
		}
		n++
		had := false
		for i := range p.Lockouts {
			if l := &p.Lockouts[i]; l.spellID == r.SpellID {
				had = true
				l.displayIndex = at
				if r.Until.After(l.until) {
					l.until = r.Until
				}
			}
		}
		if !had {
			p.Lockouts = append(p.Lockouts, castLockout{spellID: r.SpellID, displayIndex: at, until: r.Until})
		}
	}
	return n
}

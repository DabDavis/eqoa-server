package entity

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// FactionRaise is one standing an item's Use moved (RaiseFactions): From its
// standing before, To after.
type FactionRaise struct {
	Faction  string
	From, To int
}

// RaiseFactions brings the session's player's standing with each faction of
// floors up to its Standing (clamped to the faction range), never down: a
// standing already there stays and is not listed. Kept at the logout as a
// kill's change is; each change is told in Sandstorm's faction line. A faction
// the table does not know is passed by (logged). It reports the changes, and
// false when the session has no player in the world. (handlers/itemfactions.go:
// petition 5089's faction stone.)
func (w *World) RaiseFactions(sess *session.Session, floors []database.ItemFaction) ([]FactionRaise, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil, false
	}
	w.factions.ensure()
	var out []FactionRaise
	for _, r := range floors {
		f := r.Faction
		if _, known := w.factions.byName[f]; !known {
			log.Printf("Factions: %s's item names %q, no faction here: passed by", p.Name, f)
			continue
		}
		old := w.standingLocked(p, f)
		to := clampFaction(r.Standing)
		if old >= to {
			continue
		}
		if p.Factions == nil {
			p.Factions = make(map[string]int)
		}
		p.Factions[f] = to
		out = append(out, FactionRaise{Faction: f, From: old, To: to})
		log.Printf("Factions: %s's item: %s %d -> %d", p.Name, f, old, to)
		w.sendChatToSession(sess, factionLine(f, to-old, to))
	}
	return out, true
}

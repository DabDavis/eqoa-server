package handlers

import (
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questpass.go — one read of a player's pack (and of the Ascended table) for
// a whole marks pass. The map's and the heads' marks ask, for each of the
// ~1,400 quests, whether the player may take it (mayTakeAtLocked), and each ask
// read the player's pack from the database again (planHandInLocked's
// heldItems) and table itemascended: thousands of queries a second with 25
// players, 64% of the server's CPU (a profile, 2026-11-13: the owner's "lets
// look if we can fix tick spikes on the server"; also the cause of a slow keep
// restart whose clients gave up). A pass now reads each once; nothing changes
// between the asks of one pass (q.mu is held throughout), so its answers are
// the same.

// questPass is a pass's reads, for one character.
type questPass struct {
	char uint32
	rows []database.CharInventoryEntry
	read bool
	err  error
	asc  map[int]int
}

// beginPassLocked starts a pass for p; the returned func ends it. Must be
// called with q.mu held.
func (q *QuestEngine) beginPassLocked(p *entity.Player) func() {
	if p == nil {
		return func() {}
	}
	q.pass = &questPass{char: p.CharServerID}
	return func() { q.pass = nil }
}

// packRowsLocked is the player's inventory rows: the pass's when one is open
// for it, else read.
func (q *QuestEngine) packRowsLocked(p *entity.Player) ([]database.CharInventoryEntry, error) {
	if ps := q.pass; ps != nil && ps.char == p.CharServerID {
		if !ps.read {
			ps.rows, ps.err = q.inv.GetCharacterItems(p.CharServerID)
			ps.read = true
		}
		return ps.rows, ps.err
	}
	return q.inv.GetCharacterItems(p.CharServerID)
}

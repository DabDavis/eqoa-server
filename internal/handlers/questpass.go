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
//
// A pass reads less, and once (a profile of the live Heirloom server,
// 2026-11-18, 34 players: the marks still 30% of its CPU, a sync of them
// 0.9 s on the dev box). Nothing in a pass but lackingLocked reads the pack
// (the map's marks and the heads' alike), and a hand-in's count reads only a
// row's record, units, place and slot: the pass reads those of the pack alone
// (InventoryStore.HandInRows, not the bank and every record's fifty columns),
// sorts the carried and the worn rows out once, makes the set of records an
// Ascended record was made of once, and keeps each step's answer: the same
// answers, as nothing they read changes in a pass.
//
// The pack's read and the quests worth asking are kept from one pass to the
// next as well while what they were made of stands (questtakecache.go; the
// owner 2026-11-26: the marks were still 23% of each shard's CPU, pprof).

// questPass is a pass's reads, for one character.
type questPass struct {
	char uint32
	// rows is the pack's rows as HandInRows reads them: a pass never takes
	rows []database.CharInventoryEntry
	read bool
	err  error
	// kept is the character's kept read rows came from (questtakecache.go),
	// nil for a read of this pass alone
	kept *packCache
	asc  map[int]int
	// hasAsc is the records an Ascended record of asc was made of
	hasAsc map[int]bool
	// held is heldItems' rows (carried, then worn), once made
	held   []database.CharInventoryEntry
	heldOK bool
	// lack is lackingLocked's answer, by step
	lack map[*database.QuestStep]string
	// tally is heldTallyLocked's, by whether the step takes its own records
	// alone (ExactBase)
	tally map[bool]*heldTally
}

// heldTally is the units held of each record, and of its Ascended records.
type heldTally struct{ own, standIn map[int]int }

// beginPassLocked starts a pass for p; the returned func ends it. Must be
// called with q.mu held.
func (q *QuestEngine) beginPassLocked(p *entity.Player) func() {
	if p == nil {
		return func() {}
	}
	ps := &questPass{char: p.CharServerID}
	if sa := q.syncAsc; sa != nil {
		ps.asc, ps.hasAsc = sa.asc, sa.has
	}
	q.pass = ps
	return func() {
		if sa := q.syncAsc; sa != nil && ps.asc != nil {
			sa.asc, sa.has = ps.asc, ps.hasAsc
		}
		q.pass = nil
	}
}

// syncAscended is the Ascended records (and the records they were made of)
// read once a sync of the marks, for every character's pass in it: q.mu is
// held throughout, and a hand-in that makes a record holds it too, so they are
// what each pass would have read (a tool's row lands before the sync or after
// it, as it would land between two passes).
type syncAscended struct {
	asc map[int]int
	has map[int]bool
}

// beginSyncLocked starts a sync of passes sharing one read of the Ascended
// records; the returned func ends it. Must be called with q.mu held.
func (q *QuestEngine) beginSyncLocked() func() {
	q.syncAsc = &syncAscended{}
	return func() { q.syncAsc = nil }
}

// packRowsLocked is the player's inventory rows: the pass's when one is open
// for it (its pack's, HandInRows), else read whole.
func (q *QuestEngine) packRowsLocked(p *entity.Player) ([]database.CharInventoryEntry, error) {
	if ps := q.passOf(p); ps != nil {
		if !ps.read {
			ps.rows, ps.kept, ps.err = q.packReadLocked(p) // kept from the pass before while it stands (questtakecache.go)
			ps.read = true
		}
		return ps.rows, ps.err
	}
	return q.inv.GetCharacterItems(p.CharServerID)
}

// passOf is the pass open for p, or nil.
func (q *QuestEngine) passOf(p *entity.Player) *questPass {
	if ps := q.pass; ps != nil && p != nil && ps.char == p.CharServerID {
		return ps
	}
	return nil
}

// hasAscendedLocked is the records an Ascended record of asc was made of:
// made once a pass for the pass's own asc.
func (q *QuestEngine) hasAscendedLocked(asc map[int]int) map[int]bool {
	ps := q.pass
	if ps != nil && ps.hasAsc != nil {
		return ps.hasAsc
	}
	has := make(map[int]bool, len(asc))
	for _, b := range asc {
		has[b] = true
	}
	if ps != nil && ps.asc != nil { // asc is the pass's (ascendedMapLocked)
		ps.hasAsc = has
	}
	return has
}

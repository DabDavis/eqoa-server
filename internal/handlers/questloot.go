package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A loot step's drops (questpreloot.go): since 2026-11-22 each player of the
// kill on the step has a copy of its own (AddQuestDrop, below; the owner,
// 2026-11-22: "Should every grouped member on the step get one per kill?
// yes"), and an item no one of the kill is on the step for lies on the corpse
// ONCE, a shared drop whoever may loot the corpse takes (AddCorpseQuestDrop;
// the owner, 2026-11-21: "players dont need to be on a quest to obtain a quest
// drop"). From 2026-11-21 to 11-22 the shared one was every quest drop.
//
// A character's own drop (AddQuestDrop) lies on THAT character's view of the
// corpse, after the corpse's own items (loot.go), and only that character sees
// and takes it; a keep restart's corpses carry theirs (keep.go), and they are taken as they were. The corpse's window lists the
// two together, keyed by position, so Take and Take all go on as before; a
// corpse goes when its window closes with its own items AND every
// character's drops gone, else when it decays. A drop ahead of its step
// (questpreloot.go) keeps a corpse only a while: aheadKeep from the death, so
// the kill's characters have time to come to it (Live petitions 5337, 5340,
// 5347 and Heirloom 2469, 2026-11-20: one character's window closed empty and
// the corpse went with its group's drops ahead still on it, and a corpse with
// a looter's own drop ahead alone on it rotted the moment the window closed),
// and no longer (Live petition 5118, 2026-11-14: ant soldiers lay on, empty to
// their killer, holding Ant Chitin his other characters could have the quest
// for: "could have a lot of dead wildlife just waiting to despawn holding up
// other people"). Past aheadKeep a corpse with only drops ahead left goes as
// an empty one does: at its window's close, or, closed already, then
// (sweepAheadLocked).
//
// A shared drop (AddSharedDrop: a season's token, seasonrewards.go) is no one
// character's: it lies on every view of the corpse, after the corpse's own
// items and before the viewer's own drops, and whoever may open the corpse
// (lootrights.go: its kill group, then anyone once it unlocks) takes it. The
// tokens of every credited character's roll stack there together (a GM,
// 2026-11-16: "lootable by anyone with kill credit, not a specific player. This
// is crushing multiboxers": each window had to open the corpse for its own).

// sharedDrops is the key of a death's shared drops in questDrops.items: no
// character's serverid is 0.
const sharedDrops uint32 = 0

// questDrops is one death's drops, each character's own, and the shared ones
// under sharedDrops.
type questDrops struct {
	died  time.Time
	items map[uint32][]database.CharInventoryEntry // by characters.serverid
	// ahead is each character's items (pattern ids) dropped ahead of their
	// step: they keep no corpse (emptyLocked)
	ahead map[uint32]map[int]bool
}

// AddQuestDrop puts rec on character serverID's view of the corpse of the
// death died of NPC id (a loot step's drop): a stack of the same item grows.
func (h *NPCWindowHandler) AddQuestDrop(id entity.EntityID, died time.Time, serverID uint32, rec database.CharInventoryEntry) {
	if serverID == sharedDrops {
		return // no character: not everyone's by mistake
	}
	h.addQuestDrop(id, died, serverID, rec, false)
}

// AddSharedDrop puts rec on the corpse of the death died of NPC id for whoever
// may loot it (the file's doc): a stack of the same item grows.
func (h *NPCWindowHandler) AddSharedDrop(id entity.EntityID, died time.Time, rec database.CharInventoryEntry) {
	h.addQuestDrop(id, died, sharedDrops, rec, false)
}

// AddCorpseQuestDrop puts a loot step's item on the corpse of the death died of
// NPC id for whoever may loot it (questpreloot.go): one item, a stack of the same
// item grows. Ahead (no player of the kill on the step) it keeps the corpse only
// aheadKeep, as a drop ahead of its step did.
func (h *NPCWindowHandler) AddCorpseQuestDrop(id entity.EntityID, died time.Time, rec database.CharInventoryEntry, ahead bool) {
	h.addQuestDrop(id, died, sharedDrops, rec, ahead)
}

// AddQuestDropAhead is AddQuestDrop for an item dropped ahead of its step
// (questpreloot.go): it keeps no corpse.
func (h *NPCWindowHandler) AddQuestDropAhead(id entity.EntityID, died time.Time, serverID uint32, rec database.CharInventoryEntry) {
	h.addQuestDrop(id, died, serverID, rec, true)
}

func (h *NPCWindowHandler) addQuestDrop(id entity.EntityID, died time.Time, serverID uint32, rec database.CharInventoryEntry, ahead bool) {
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	d := ls.drops[id]
	if d == nil || !d.died.Equal(died) {
		d = &questDrops{died: died, items: map[uint32][]database.CharInventoryEntry{}}
		ls.drops[id] = d
	}
	rec = catalogRecord(rec)
	if ahead {
		if d.ahead == nil {
			d.ahead = map[uint32]map[int]bool{}
		}
		if d.ahead[serverID] == nil {
			d.ahead[serverID] = map[int]bool{}
		}
		d.ahead[serverID][rec.Pattern.PatternID] = true
	} else if d.ahead != nil {
		delete(d.ahead[serverID], rec.Pattern.PatternID) // one that keeps the corpse joins it: the stack does
	}
	mine := d.items[serverID]
	for i := range mine {
		if mine[i].Pattern.PatternID == rec.Pattern.PatternID {
			mine[i].StackLeft += rec.StackLeft
			return
		}
	}
	d.items[serverID] = append(mine, rec)
}

// dropsLocked is the drops character serverID sees on the corpse of NPC id
// (the death c is the loot of): the shared ones, then its own; or nil. Must be
// called with ls.mu held.
func (ls *lootState) dropsLocked(id entity.EntityID, c *corpse, serverID uint32) []database.CharInventoryEntry {
	d := ls.drops[id]
	if d == nil || c == nil || !d.died.Equal(c.died) {
		return nil
	}
	shared := d.items[sharedDrops]
	if serverID == sharedDrops {
		return shared
	}
	mine := d.items[serverID]
	if len(shared) == 0 {
		return mine
	}
	if len(mine) == 0 {
		return shared
	}
	out := make([]database.CharInventoryEntry, 0, len(shared)+len(mine))
	return append(append(out, shared...), mine...)
}

// viewLocked is the corpse as character serverID's window lists it: its own
// items, then its shares that character may take (lootshare.go), then the
// shared drops, then that character's own drops.
func (ls *lootState) viewLocked(id entity.EntityID, c *corpse, serverID uint32) []database.CharInventoryEntry {
	mine := ls.dropsLocked(id, c, serverID)
	shared := c.sharesViewLocked(serverID)
	if len(mine) == 0 && len(shared) == 0 {
		return c.items
	}
	out := make([]database.CharInventoryEntry, 0, len(c.items)+len(shared)+len(mine))
	return append(append(append(out, c.items...), shared...), mine...)
}

// tookLocked takes n of the record at position i of character serverID's
// view of the corpse (its own items first, then its shares, then the shared
// drops, then the character's own).
func (ls *lootState) tookLocked(id entity.EntityID, c *corpse, serverID uint32, i, n int) {
	if i < len(c.items) {
		c.items[i].StackLeft -= n
		if c.items[i].StackLeft <= 0 {
			c.items = append(c.items[:i], c.items[i+1:]...)
		}
		return
	}
	j := i - len(c.items)
	if c.tookShareLocked(serverID, &j, n) {
		return
	}
	d := ls.drops[id]
	if d == nil || !d.died.Equal(c.died) {
		return
	}
	shared := len(d.items[sharedDrops])
	switch {
	case j < shared:
		d.takeLocked(sharedDrops, j, n)
	case serverID != sharedDrops:
		d.takeLocked(serverID, j-shared, n)
	}
}

// takeLocked takes n of the record at position j of key's drops.
func (d *questDrops) takeLocked(key uint32, j, n int) {
	mine := d.items[key]
	if j < 0 || j >= len(mine) {
		return
	}
	mine[j].StackLeft -= n
	if mine[j].StackLeft <= 0 {
		mine = append(mine[:j], mine[j+1:]...)
	}
	if len(mine) == 0 {
		delete(d.items, key)
	} else {
		d.items[key] = mine
	}
}

// aheadKeep is how long after its death a corpse's drops ahead of their step
// keep it (the file's doc).
const aheadKeep = 2 * time.Minute

// emptyLocked is whether nothing is left on the corpse that keeps it at now: its
// own items and every character's drops, those dropped ahead of their step only
// until aheadKeep after the death.
func (ls *lootState) emptyLocked(id entity.EntityID, c *corpse, now time.Time) bool {
	if len(c.items) > 0 || c.raid != nil && len(c.raid.held) > 0 { // a raid's rolled and won items (raidloot.go)
		return false
	}
	if c.owedLocked() { // a share's copy a character of the kill waits for (lootshare.go)
		return false
	}
	d := ls.drops[id]
	if d == nil || !d.died.Equal(c.died) {
		return true
	}
	keepAhead := now.Before(c.died.Add(aheadKeep))
	for sid, recs := range d.items {
		for _, r := range recs {
			if keepAhead || !d.ahead[sid][r.Pattern.PatternID] {
				return false
			}
		}
	}
	return true
}

// aheadOnlyLocked is whether what keeps the corpse at now is only drops ahead
// of their step (their keep not over): a corpse that goes at its keep's end.
func (ls *lootState) aheadOnlyLocked(id entity.EntityID, c *corpse, now time.Time) bool {
	return !ls.emptyLocked(id, c, now) && ls.emptyLocked(id, c, c.died.Add(aheadKeep))
}

// keepAheadLocked is a corpse whose window closed with only drops ahead left on
// it: it goes at their keep's end unless something takes it first
// (sweepAheadLocked). Must be called with ls.mu held.
func (h *NPCWindowHandler) keepAheadLocked(ls *lootState, id entity.EntityID, c *corpse, now time.Time) {
	if !ls.aheadOnlyLocked(id, c, now) || h.lootNow != nil { // a test sweeps itself
		return
	}
	died := c.died
	time.AfterFunc(died.Add(aheadKeep).Sub(now)+time.Second, func() {
		ls := h.loots()
		ls.mu.Lock()
		defer ls.mu.Unlock()
		h.sweepAheadLocked(ls, id, died, h.lootClock())
	})
}

// sweepAheadLocked takes away the corpse of NPC id's death died when, at now,
// nothing keeps it any more and no window is open on it (one open goes at its
// close). Must be called with ls.mu held.
func (h *NPCWindowHandler) sweepAheadLocked(ls *lootState, id entity.EntityID, died, now time.Time) {
	c := ls.corpses[id]
	if c == nil || !c.died.Equal(died) {
		return
	}
	for _, open := range ls.open {
		if open == id {
			return
		}
	}
	if ls.emptyLocked(id, c, now) {
		log.Printf("Loot: a corpse (entity %d) lay with only drops ahead of their steps on it: gone", id)
		h.emptiedLocked(ls, id, c)
	}
}

// charOf is the character (characters.serverid) a session plays, 0 none.
func (h *NPCWindowHandler) charOf(sess *session.Session) uint32 {
	if h.playerOf == nil {
		return 0
	}
	if p := h.playerOf(sess); p != nil {
		return p.CharServerID
	}
	return 0
}

// lootTake is how many of an item a player took off a corpse at once.
type lootTake struct{ pattern, n int }

// tellQuestsLooted tells the quests each item a player took off a corpse (a
// loot step counts what is taken while it is the step to do). Called with no
// loot lock held.
func tellQuestsLooted(sess *session.Session, p *entity.Player, takes []lootTake) {
	q := Quests()
	if q == nil || p == nil {
		return
	}
	for _, t := range takes {
		q.Looted(sess, p, t.pattern, t.n)
	}
}

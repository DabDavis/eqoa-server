package handlers

import (
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A loot step's drops (quests.go): an item a kill drops for one character on
// a loot step lies on THAT character's view of the corpse, after the corpse's
// own items (loot.go), and only that character sees and takes it, as a quest
// item dropped for one player on the live game. The corpse's window lists the
// two together, keyed by position, so Take and Take all go on as before; a
// corpse goes when its window closes with its own items AND every
// character's drops gone, else when it decays. A drop ahead of its step
// (questpreloot.go) keeps no corpse: it is there to take while the corpse
// lies, and goes with it (Live petition 5118, Waldorf, 2026-11-14: ant
// soldiers by Klick lay on, empty to him, holding Ant Chitin his other
// characters could have the quest for: "could have a lot of dead wildlife
// just waiting to despawn holding up other people").

// questDrops is one death's drops, each character's own.
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
	h.addQuestDrop(id, died, serverID, rec, false)
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

// dropsLocked is character serverID's drops on the corpse of NPC id (the
// death c is the loot of), or nil. Must be called with ls.mu held.
func (ls *lootState) dropsLocked(id entity.EntityID, c *corpse, serverID uint32) []database.CharInventoryEntry {
	d := ls.drops[id]
	if d == nil || c == nil || !d.died.Equal(c.died) {
		return nil
	}
	return d.items[serverID]
}

// viewLocked is the corpse as character serverID's window lists it: its own
// items, then that character's drops.
func (ls *lootState) viewLocked(id entity.EntityID, c *corpse, serverID uint32) []database.CharInventoryEntry {
	mine := ls.dropsLocked(id, c, serverID)
	if len(mine) == 0 {
		return c.items
	}
	out := make([]database.CharInventoryEntry, 0, len(c.items)+len(mine))
	return append(append(out, c.items...), mine...)
}

// tookLocked takes n of the record at position i of character serverID's
// view of the corpse (its own items first, then the drops).
func (ls *lootState) tookLocked(id entity.EntityID, c *corpse, serverID uint32, i, n int) {
	if i < len(c.items) {
		c.items[i].StackLeft -= n
		if c.items[i].StackLeft <= 0 {
			c.items = append(c.items[:i], c.items[i+1:]...)
		}
		return
	}
	d := ls.drops[id]
	if d == nil || !d.died.Equal(c.died) {
		return
	}
	j := i - len(c.items)
	mine := d.items[serverID]
	if j < 0 || j >= len(mine) {
		return
	}
	mine[j].StackLeft -= n
	if mine[j].StackLeft <= 0 {
		mine = append(mine[:j], mine[j+1:]...)
	}
	if len(mine) == 0 {
		delete(d.items, serverID)
	} else {
		d.items[serverID] = mine
	}
}

// emptyLocked is whether nothing is left on the corpse that keeps it: its own
// items and every character's drops, but those dropped ahead of their step.
func (ls *lootState) emptyLocked(id entity.EntityID, c *corpse) bool {
	if len(c.items) > 0 || c.raid != nil && len(c.raid.held) > 0 { // a raid's rolled and won items (raidloot.go)
		return false
	}
	d := ls.drops[id]
	if d == nil || !d.died.Equal(c.died) {
		return true
	}
	for sid, recs := range d.items {
		for _, r := range recs {
			if !d.ahead[sid][r.Pattern.PatternID] {
				return false
			}
		}
	}
	return true
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

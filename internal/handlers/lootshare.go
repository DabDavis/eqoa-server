package handlers

import (
	"sort"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// lootshare.go — a No Trade stack on a corpse is shared, one to a character
// (the owner, 2026-11-17: "the keys dropping from Plane of sky are stacking so only
// 1 player gets them since they are notrade").
//
// The Plane of Sky's isle keys are twenty of a No Trade key on their boss's corpse
// (tools/sky_keys_20261111.py: one npcitem row of stackleft 20), and the first looter
// took all twenty in one take; no one could hand a key on, so the rest of the raid
// had none. A No Trade record a corpse rolls with more than one in its stack (an
// npcitem row's, a loot category's stack, the same item's rows gathered) is a share
// instead: n copies, at most one to a character while the kill's characters wait for
// theirs, and never fewer copies than the kill has characters (the killer and its
// group's or raid's members near it: lootrights.go's kill group, a raid's seats too),
// so every member of a raid gets its own key from one kill.
//
//   - Each character sees one copy (a record of 1) in its view of the corpse, after the
//     corpse's own items and before its quest drops (questloot.go), until it takes it.
//   - A character that holds one already (worn, packed, banked: invList.loreHeld) is
//     not offered a copy while others wait: the next one is theirs.
//   - One outside the kill (a member of the killer's group farther off, anyone once the
//     corpse is anyone's) may take one of the copies beyond those still owed.
//   - When every waiting character has had theirs (taken, held already, or out of the
//     world), what is left is one stack as any other record, anyone's who may open the
//     corpse; the last one owed sees it so already (its own and the spares): a lone
//     killer of a creature with three No Trade pelts takes them all, as before.
//   - A share keeps its corpse only while a character of the kill waits for its copy;
//     the spares left after keep none, so a boss's respawn is not held up by them.
//
// Not a quest item's stack (the owner, 2026-11-21: "no instanced quest items for each
// character, so all 4 players dont need to loot the corpse it slows things down too
// much"): an item a quest's loot, give or show step names (QuestEngine.QuestItem) is one
// stack on the corpse, one looter's, as any other record. A key an event reads in the
// pack (eventKey: the isles' keys above, Tallon's Key) stays a share, a quest's or not:
// the keys are what this was made for. Every other No Trade stack is shared as before:
// the owner spoke of quest items only.
//
// A raid's rules never held these (raidCovers: no stackable), and still do not: a
// master looter's hold and a roll's go on over the corpse's other items as they did.

// lootShare is a No Trade stack shared one to a character.
type lootShare struct {
	rec  database.CharInventoryEntry // one copy (StackLeft 1)
	left int                         // copies left
	took map[uint32]bool             // characters that had theirs
	// waiting are the kill's characters still owed a copy
	waiting map[uint32]bool
}

// sharesItem is whether a corpse's record is shared: a No Trade stack, not a quest
// item's (questItem; nil: none is) unless a key an event reads.
func sharesItem(rec *database.CharInventoryEntry, questItem func(int) bool) bool {
	if rec.Pattern.Trade <= 0 || rec.StackLeft <= 1 {
		return false
	}
	return questItem == nil || !questItem(rec.Pattern.PatternID) || eventKey(rec.Pattern.ItemName)
}

// splitShares takes a corpse's No Trade stacks out of its items as shares, the same
// item's records gathered in one (sharesItem: a quest item's stays a record).
func splitShares(items []database.CharInventoryEntry, questItem func(int) bool) ([]database.CharInventoryEntry, []*lootShare) {
	keep := items[:0:0]
	var shares []*lootShare
	byPattern := map[int]*lootShare{}
	for _, it := range items {
		if !sharesItem(&it, questItem) {
			keep = append(keep, it)
			continue
		}
		if s := byPattern[it.Pattern.PatternID]; s != nil {
			s.left += it.StackLeft
			continue
		}
		s := &lootShare{rec: it, left: it.StackLeft, took: map[uint32]bool{}, waiting: map[uint32]bool{}}
		s.rec.StackLeft = 1
		byPattern[it.Pattern.PatternID] = s
		shares = append(shares, s)
	}
	return keep, shares
}

// owe makes the characters owed a copy of each share: never fewer copies than them.
func owe(shares []*lootShare, chars []uint32) {
	for _, s := range shares {
		for _, c := range chars {
			if c != 0 && !s.took[c] {
				s.waiting[c] = true
			}
		}
		s.left = max(s.left, len(s.waiting))
	}
}

// visibleTo is how many of a share character char's view of the corpse lists, 0 for
// none: one while others wait (its own, or a spare beyond those owed), else the rest
// (to the last one owed too, so a take-all takes no more than the window listed).
func (s *lootShare) visibleTo(char uint32) int {
	switch {
	case s.left <= 0:
		return 0
	case len(s.waiting) == 0, len(s.waiting) == 1 && s.waiting[char]:
		return s.left // anyone's, or the last one owed's: its own and the spares
	case s.took[char]:
		return 0
	case s.waiting[char], s.left > len(s.waiting):
		return 1
	}
	return 0
}

// take is n of a share to char.
func (s *lootShare) take(char uint32, n int) {
	s.left -= n
	s.served(char)
}

// served is char done with its copy: taken, held already, or gone.
func (s *lootShare) served(char uint32) {
	if char != 0 {
		s.took[char] = true
	}
	delete(s.waiting, char)
}

// owedLocked is whether a share still owes a waiting character its copy.
func (c *corpse) owedLocked() bool {
	for _, s := range c.shares {
		if s.left > 0 && len(s.waiting) > 0 {
			return true
		}
	}
	return false
}

// sharesViewLocked is the share records char's view lists, in order.
func (c *corpse) sharesViewLocked(char uint32) []database.CharInventoryEntry {
	var out []database.CharInventoryEntry
	for _, s := range c.shares {
		if n := s.visibleTo(char); n > 0 {
			r := s.rec
			r.StackLeft = n
			out = append(out, r)
		}
	}
	return out
}

// tookShareLocked takes n of the j-th share char's view lists; false when there is no
// j-th (j is then made the position past the shares).
func (c *corpse) tookShareLocked(char uint32, j *int, n int) bool {
	for _, s := range c.shares {
		if s.visibleTo(char) == 0 {
			continue
		}
		if *j == 0 {
			s.take(char, n)
			return true
		}
		*j--
	}
	return false
}

// kill's characters: the kill group the corpse's death was credited to (lootrights.go).
func (h *NPCWindowHandler) killCharsOf(id entity.EntityID, c *corpse) []uint32 {
	v, ok := h.killRights.Load(id)
	if !ok {
		return nil
	}
	r := v.(lootRight)
	if !r.died.Equal(c.died) {
		return nil
	}
	out := make([]uint32, 0, len(r.chars))
	for ch := range r.chars {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// servedHoldersLocked is the shares' waiting characters that are served already: the
// opener (sess, inv) and every other one in the world that holds the item, and those
// out of it. Must be called with ls.mu held.
func (h *NPCWindowHandler) servedHoldersLocked(c *corpse, sess *session.Session, char uint32, inv *invList) {
	if c == nil || len(c.shares) == 0 {
		return
	}
	for _, s := range c.shares {
		if char != 0 && !s.took[char] && inv != nil && inv.loreHeld(s.rec.Pattern.PatternID) && len(s.waiting) > 0 {
			s.served(char)
		}
		for w := range s.waiting {
			if w == char {
				continue
			}
			if h.raid.seat == nil {
				continue // no world to ask (a test's)
			}
			ws, _ := h.raidSeatOf(w)
			if ws == nil {
				s.served(w) // out of the world
				continue
			}
			if wi := sessionInvList(ws); wi != nil && wi.loreHeld(s.rec.Pattern.PatternID) {
				s.served(w)
			}
		}
	}
}

// KeptShare is a share kept over a restart (KeptCorpse.Shares).
type KeptShare struct {
	Item    database.CharInventoryEntry `json:"item"`
	Left    int                         `json:"left"`
	Took    []uint32                    `json:"took,omitempty"`
	Waiting []uint32                    `json:"waiting,omitempty"`
}

func keepShares(shares []*lootShare) []KeptShare {
	var out []KeptShare
	for _, s := range shares {
		k := KeptShare{Item: s.rec, Left: s.left}
		for c := range s.took {
			k.Took = append(k.Took, c)
		}
		for c := range s.waiting {
			k.Waiting = append(k.Waiting, c)
		}
		sort.Slice(k.Took, func(i, j int) bool { return k.Took[i] < k.Took[j] })
		sort.Slice(k.Waiting, func(i, j int) bool { return k.Waiting[i] < k.Waiting[j] })
		out = append(out, k)
	}
	return out
}

func restoreShares(kept []KeptShare) []*lootShare {
	var out []*lootShare
	for _, k := range kept {
		s := &lootShare{rec: k.Item, left: k.Left, took: map[uint32]bool{}, waiting: map[uint32]bool{}}
		for _, c := range k.Took {
			s.took[c] = true
		}
		for _, c := range k.Waiting {
			s.waiting[c] = true
		}
		out = append(out, s)
	}
	return out
}

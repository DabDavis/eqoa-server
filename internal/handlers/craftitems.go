package handlers

import (
	"log"
	"sort"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// How a craft's items move (claude-notes/tradeskill-protocol-20261029.md §8):
// CLIENT's craft replies never touch its real inventory, so every component
// taken, product made and tool worn is also sent as the ordinary message,
// 0x003C, 0x003B or 0x0060, mirrored in the session's invList. 0x003C names
// a LIST INDEX and packs the list, and CLIENT hangs on one that takes more
// than the record holds, so every index is the mirror's at the moment it is
// sent and nothing is taken that the mirror does not hold.

// The tools' wear slots (CLIENT's CanWearIn: 22 + the skill, worn in Held
// alone) and Held's equip slot.
const (
	wearCraftTool0 = 22
	equipHeld      = 19
)

// craftMsg is one message a craft sends after its reply.
type craftMsg struct {
	op      protocol.GameOpcode
	payload []byte
}

// craftBatch is the inventory messages a craft sends after its reply, in
// order.
type craftBatch []craftMsg

func (b *craftBatch) add(op protocol.GameOpcode, payload []byte) {
	*b = append(*b, craftMsg{op, payload})
}

func (b craftBatch) send(sess *session.Session) {
	for _, m := range b {
		queueReliable(sess, m.op, m.payload)
	}
}

// craftHeldRow is an inventory row a craft may use: carried (in the
// inventory and not worn) and in the client's list.
type craftHeldRow struct {
	row database.CharInventoryEntry
	idx int // its list index now
	qty int // what a craft may take of it: the row's stack or the list's, the fewer
}

// craftHeld is the rows a craft may use, in the client's list order (the
// order VIItemList finds an item's records in).
func craftHeld(rows []database.CharInventoryEntry, l *invList) []craftHeldRow {
	if l == nil {
		return nil
	}
	var out []craftHeldRow
	for _, r := range rows {
		if r.Location != 0 || r.EquipLoc >= 0 || r.StackLeft <= 0 {
			continue
		}
		idx := l.indexOfItem(r.ItemID)
		rec, ok := l.at(idx)
		if !ok || rec.patternID != r.Pattern.PatternID || rec.qty <= 0 {
			continue
		}
		out = append(out, craftHeldRow{row: r, idx: idx, qty: min(r.StackLeft, rec.qty)})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].idx < out[b].idx })
	return out
}

// craftHeldCount is how many of an item the rows hold.
func craftHeldCount(held []craftHeldRow, pattern int) int {
	n := 0
	for _, h := range held {
		if h.row.Pattern.PatternID == pattern {
			n += h.qty
		}
	}
	return n
}

// craftNeed is n of an item a craft takes.
type craftNeed struct {
	pattern, n int
}

// craftTake is n of one inventory row a craft takes.
type craftTake struct {
	itemID, pattern, n int
}

// planCraftTakes is what taking the needs takes, each need in its order from
// the rows of its item in list order; false when the rows hold too few.
func planCraftTakes(held []craftHeldRow, needs []craftNeed) ([]craftTake, bool) {
	left := map[int]int{}
	for _, h := range held {
		left[h.row.ItemID] = h.qty
	}
	var out []craftTake
	for _, nd := range needs {
		want := nd.n
		for _, h := range held {
			if want <= 0 {
				break
			}
			if h.row.Pattern.PatternID != nd.pattern {
				continue
			}
			k := min(want, left[h.row.ItemID])
			if k <= 0 {
				continue
			}
			left[h.row.ItemID] -= k
			want -= k
			out = append(out, craftTake{itemID: h.row.ItemID, pattern: nd.pattern, n: k})
		}
		if want > 0 {
			return nil, false
		}
	}
	return out, true
}

// craftNeeds is a recipe's ingredients as needs: all of them, or with
// lostOnly those a failure loses (keep false).
func craftNeeds(r *database.CraftRecipe, lostOnly bool) []craftNeed {
	var out []craftNeed
	for _, in := range r.Ingredients {
		if lostOnly && in.Keep {
			continue
		}
		out = append(out, craftNeed{in.Item, in.Count})
	}
	return out
}

// clone is a copy of the list to try changes on.
func (l *invList) clone() *invList {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &invList{recs: append([]invRec(nil), l.recs...), next: l.next}
}

// craftRoom is whether n of the product fits once the takes are taken, filed
// as the client's AddItem files it: a stackable item onto its first record
// (what does not fit there is dropped, so all n must), anything else a new
// record each, at most 40 records.
func craftRoom(l *invList, takes []craftTake, p *database.ItemPatternRecord, n int) bool {
	if l == nil {
		return false
	}
	sim := l.clone()
	for _, t := range takes {
		if !sim.took(sim.indexOfItem(t.itemID), t.n) {
			return false
		}
	}
	if p.MaxStack >= 2 {
		return sim.wouldAdd(p.PatternID, n, p.MaxStack) == n
	}
	for i := 0; i < n; i++ {
		if _, took := sim.add(0, p.PatternID, 1, p.MaxStack); took != 1 {
			return false
		}
	}
	return true
}

// takeCraft takes what the plan says, in the database, the session's list
// and the client's (0x003C: the record's list index as the takes before it
// left the list, sound 0, the count), and returns the takes made. One the
// list no longer holds is not made: never more than the record holds.
func (h *CraftHandler) takeCraft(charID uint32, l *invList, held []craftHeldRow, takes []craftTake, out *craftBatch) []craftTake {
	stacks := map[int]int{}
	for _, hr := range held {
		stacks[hr.row.ItemID] = hr.row.StackLeft
	}
	var done []craftTake
	for _, t := range takes {
		idx := l.indexOfItem(t.itemID)
		rec, ok := l.at(idx)
		if !ok || rec.qty < t.n {
			log.Printf("Craft: item %d (%d of it to take) is not in the client's list as it was", t.itemID, t.n)
			continue
		}
		left := stacks[t.itemID] - t.n
		var err error
		if left <= 0 {
			err = h.inv.DeleteCharItem(charID, t.itemID)
		} else {
			err = h.inv.SetItemStack(charID, t.itemID, left)
		}
		if err != nil {
			log.Printf("Craft: taking %d of item %d: %v", t.n, t.itemID, err)
			continue
		}
		stacks[t.itemID] = left
		if idx = l.indexOfItem(t.itemID); !l.took(idx, t.n) {
			log.Printf("Craft: item %d left the client's list meanwhile", t.itemID)
			continue
		}
		out.add(protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, t.n))
		done = append(done, t)
	}
	return done
}

// craftTookList is what an Experiment's reply says it took (TookItems,
// ItemResult: each pair Took()s the first record of the item in the client's
// copy of its list): one pair a unit. CLIENT hangs on a Took of more than the
// record holds, and its copy may lag the server's by a message in flight; a
// pair of one never takes more than a record holds, and an Experiment takes
// four units at most.
func craftTookList(taken []craftTake) [][2]uint32 {
	var out [][2]uint32
	for _, t := range taken {
		for i := 0; i < t.n; i++ {
			out = append(out, [2]uint32{uint32(t.pattern), 1})
		}
	}
	return out
}

// craftGiven is a record a craft gave: as 0x003B sent it, and the key the
// client filed it under.
type craftGiven struct {
	rec database.CharInventoryEntry
	key uint32
}

// giveCraft gives n of the product as a sale does (GiveItemNow): a stack as
// one record onto its first record or a new one, anything else a record
// each, at full condition, in the database, the session's list and the
// client's (0x003B). It returns the records sent.
func (h *CraftHandler) giveCraft(charID uint32, l *invList, p database.ItemPatternRecord, n int, out *craftBatch) []craftGiven {
	w := &NPCWindowHandler{inv: h.inv}
	player := &entity.Player{CharServerID: charID}
	qty, records := n, 1
	if p.MaxStack < 2 {
		qty, records = 1, n
	}
	var given []craftGiven
	for i := 0; i < records; i++ {
		took := l.wouldAdd(p.PatternID, qty, p.MaxStack)
		if took == 0 {
			log.Printf("Craft: no room for %q after all", p.ItemName)
			break
		}
		rec := database.CharInventoryEntry{StackLeft: took, RemainHP: p.MaxHP, Pattern: p}
		sent, key, err := w.giveItem(player, l, rec, took, 0)
		if err != nil {
			log.Printf("Craft: giving %q: %v", p.ItemName, err)
			break
		}
		out.add(protocol.OpcodeAddInvItem, addInvItemPayload(sent, key))
		given = append(given, craftGiven{*sent, key})
	}
	return given
}

// craftTool is the tool a craft uses: an item of its skill's tool wear slot
// worn in Held, which the client's list holds, and its key there.
type craftTool struct {
	row database.CharInventoryEntry
	key uint32
}

// craftToolOf is the character's tool for the skill, if it wears one.
func craftToolOf(rows []database.CharInventoryEntry, l *invList, skill int) (craftTool, bool) {
	if l == nil {
		return craftTool{}, false
	}
	for _, r := range rows {
		if r.Location != 0 || r.EquipLoc != equipHeld || r.Pattern.EquipSlot != wearCraftTool0+skill {
			continue
		}
		rec, ok := l.at(l.indexOfItem(r.ItemID))
		if !ok {
			continue
		}
		return craftTool{row: r, key: rec.key}, true
	}
	return craftTool{}, false
}

// broken is a tool worn to nothing (one of no condition never is).
func (t craftTool) broken() bool {
	return t.row.Pattern.MaxHP > 0 && t.row.RemainHP <= 0
}

// wearTool takes an attempt's wear off the tool's condition, kept and sent
// (0x0060, by the record's key), and says whether that broke it. A tool of
// no condition (maxhp 0) never wears.
func (h *CraftHandler) wearTool(charID uint32, t craftTool, out *craftBatch) bool {
	if t.row.Pattern.MaxHP <= 0 {
		return false
	}
	hp := max(0, t.row.RemainHP-craftToolWearHP)
	if err := h.inv.SetItemRemainHP(charID, t.row.ItemID, hp); err != nil {
		log.Printf("Craft: wearing the tool (item %d): %v", t.row.ItemID, err)
		return false
	}
	out.add(protocol.OpcodeAdjustItemHP, adjustItemHPPayload(t.key, hp))
	return hp == 0
}

// craftItemIcon is the icon CLIENT shows an item by (VIClient::ItemIcon,
// 0x00BC9DD0): its second icon (itempattern.unk2) where it has one and the
// disc's expansion flag 2 is set, as on Frontiers, else its icon.
func craftItemIcon(p *database.ItemPatternRecord) uint32 {
	if p.Unk2 != 0 {
		return uint32(p.Unk2)
	}
	return uint32(p.ItemIcon)
}

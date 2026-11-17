package handlers

import (
	"log"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Weapon Swap Back (work board 1581, jibrish 2026-11-16: "I have 2 swords
// equipped / I hit button to switch to bow, I now have bow / I go and hit the
// same button again, and now its those same 2 swords ... Be very careful to not
// accidentally duplicate items."). Vanilla's Extended > Other > Combat >
// "Weapon Swap Back" adds a byte to 0x0022 after the set: bit 0 asks to swap
// back. A PS2 never sends 0x0022; a client without the option sends the set
// alone, and the press is today's.
//
// Each character (by CharServerID, for the server's run: a relog keeps it) has
// the last set taken up that changed the hands, and the hand rows before and
// after it: (charinventory.itemid, slot) for each pack row worn in slots 14-20.
// A press with the bit, of that same set, while the hands hold exactly the
// "after" rows, puts the "before" rows back in their own slots and takes off
// anything else; the memory is spent, so the button toggles. Any other press
// takes the set up as before and is remembered if it changed the hands.
//
// Against duplication: only charinventory.equiploc changes (equipInto's
// planEquipInto + applyPlan, takeOffAllBut's UnequipItem); a row is put back
// only as that exact itemid, in the pack (never the bank, a kept row, or
// another row of the same pattern); one gone since is skipped, and with none
// left nothing changes but a line.

const gearSwapBack = 1 // 0x0022's second byte: swap back

const swapBackGone = "The weapons you swapped from are no longer in your pack."

// handItem is a pack row worn in a hand slot.
type handItem struct{ itemID, slot int }

// gearSwap is a character's last weapon set that changed the hands.
type gearSwap struct {
	mu            sync.Mutex // one press of the character's at a time
	set           int        // -1 none
	before, after []handItem
}

func (m *gearSwap) remember(set int, before, after []handItem) {
	m.set, m.before, m.after = set, before, after
}

func (m *gearSwap) forget() { m.set, m.before, m.after = -1, nil, nil }

// gearSwapOf is the character's memory, made on its first press.
func (h *EquipHandler) gearSwapOf(serverID uint32) *gearSwap {
	h.swapMu.Lock()
	defer h.swapMu.Unlock()
	if h.swaps == nil {
		h.swaps = make(map[uint32]*gearSwap)
	}
	m := h.swaps[serverID]
	if m == nil {
		m = &gearSwap{set: -1}
		h.swaps[serverID] = m
	}
	return m
}

// wornHands is each pack row worn in a hand slot, in handSlots' order.
func wornHands(inv []*database.CharInventoryEntry) []handItem {
	var out []handItem
	for _, s := range handSlots {
		for _, e := range inv {
			if e.EquipLoc == s && e.Location == database.LocPack {
				out = append(out, handItem{e.ItemID, s})
			}
		}
	}
	return out
}

func sameHands(a, b []handItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// packRow is exactly that row, in the pack, or nil.
func packRow(inv []*database.CharInventoryEntry, itemID int) *database.CharInventoryEntry {
	for _, e := range inv {
		if e.ItemID == itemID && e.Location == database.LocPack {
			return e
		}
	}
	return nil
}

// equipOrder is an order in which planEquipInto puts each row in its slot
// without taking off one put in before it, tried on a copy of the rows (a wand
// in 20 takes 18 off, throwing knives in 18 take nothing off); false when there
// is none.
func equipOrder(inv []*database.CharInventoryEntry, want []handItem) ([]handItem, bool) {
	rows := make([]database.CharInventoryEntry, len(inv))
	for i, e := range inv {
		rows[i] = *e
	}
	used := make([]bool, len(want))
	order := make([]handItem, 0, len(want))
	var try func([]database.CharInventoryEntry) bool
	try = func(rows []database.CharInventoryEntry) bool {
		if len(order) == len(want) {
			return true
		}
		for i, w := range want {
			if used[i] {
				continue
			}
			next, ok := triedEquip(rows, w)
			if !ok {
				continue
			}
			kept := true
			for _, p := range order {
				if equipLocOf(next, p.itemID) != p.slot {
					kept = false
					break
				}
			}
			if !kept {
				continue
			}
			used[i], order = true, append(order, w)
			if try(next) {
				return true
			}
			used[i], order = false, order[:len(order)-1]
		}
		return false
	}
	if !try(rows) {
		return nil, false
	}
	return order, true
}

// triedEquip is the rows after planEquipInto puts w in its slot, on a copy.
func triedEquip(rows []database.CharInventoryEntry, w handItem) ([]database.CharInventoryEntry, bool) {
	next := append([]database.CharInventoryEntry(nil), rows...)
	inv := make([]*database.CharInventoryEntry, len(next))
	for i := range next {
		inv[i] = &next[i]
	}
	e := packRow(inv, w.itemID)
	if e == nil {
		return nil, false
	}
	for _, c := range planEquipInto(inv, 0, e, w.slot).changes {
		for i := range next {
			if next[i].ItemID == c.itemID {
				next[i].EquipLoc = c.equipLoc
			}
		}
	}
	return next, equipLocOf(next, w.itemID) == w.slot
}

func equipLocOf(rows []database.CharInventoryEntry, itemID int) int {
	for i := range rows {
		if rows[i].ItemID == itemID {
			return rows[i].EquipLoc
		}
	}
	return -1
}

// restoreHands puts want's rows back in their slots and takes off anything
// else in the hands.
func (h *EquipHandler) restoreHands(sess *session.Session, player *entity.Player, serverID uint32, items []database.CharInventoryEntry, set int, want []handItem) {
	inv := inventoryRows(items)
	var put []handItem
	for _, w := range want {
		e := packRow(inv, w.itemID)
		switch {
		case e == nil:
			log.Printf("UseWeaponSet: serverID=%d set %d again: item %d (slot %d) is no longer in the pack", serverID, set, w.itemID, w.slot)
		case !fitsEquip(e.Pattern.EquipSlot, w.slot):
			log.Printf("UseWeaponSet: serverID=%d set %d again: %q (wear %d) does not go in slot %d", serverID, set, e.Pattern.ItemName, e.Pattern.EquipSlot, w.slot)
		case w.slot == equipSecondary && e.Pattern.EquipSlot == wearEitherHand && offHandWeaponBarred(player.Class):
			SendColoredChat(sess, offHandRefusal, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		default:
			put = append(put, w)
		}
	}
	if len(want) > 0 && len(put) == 0 {
		SendColoredChat(sess, swapBackGone, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		return
	}
	order, ok := equipOrder(inv, put)
	if !ok {
		log.Printf("UseWeaponSet: serverID=%d set %d again: %v cannot be worn together; the hands are left as they are", serverID, set, put)
		return
	}
	for _, w := range order {
		items, err := h.inventoryStore.GetCharacterItems(serverID)
		if err != nil {
			log.Printf("UseWeaponSet: loading items for serverID=%d: %v", serverID, err)
			return
		}
		if e := packRow(inventoryRows(items), w.itemID); e != nil {
			h.equipInto(sess, serverID, items, e, w.slot)
		}
	}
	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("UseWeaponSet: loading items for serverID=%d: %v", serverID, err)
		return
	}
	h.takeOffAllBut(sess, serverID, items, put)
	h.wornChanged(sess, player, serverID) // a fight swings with them from its next round
	log.Printf("UseWeaponSet: serverID=%d set %d again: swapped back to %v", serverID, set, put)
}

// takeOffAllBut takes off every hand row but keep's (UnequipItem each).
func (h *EquipHandler) takeOffAllBut(sess *session.Session, serverID uint32, items []database.CharInventoryEntry, keep []handItem) {
	kept := map[int]bool{}
	for _, k := range keep {
		kept[k.itemID] = true
	}
	inv := inventoryRows(items)
	for _, w := range wornHands(inv) {
		if kept[w.itemID] {
			continue
		}
		plan := equipPlan{name: packRow(inv, w.itemID).Pattern.ItemName, unequip: w.slot, changes: []equipChange{{w.itemID, -1}}}
		if !h.applyPlan(sess, serverID, plan, "UseWeaponSet") {
			return
		}
	}
}

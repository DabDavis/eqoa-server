package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// destroy.go — the item menu's Destroy (the user, 2026-11-05: "players not
// able to delete items").
//
// CLIENT's VIWndItemMenu::Use (0x00C66AD0) asks first for a worn item, one on
// a quick slot or a no-trade one (words 916, 917, 918), the quantity box
// giving how many of a stack; then VIClient::UseItemOn (0x00BCAC90) sends
// 0x003C {u32 the record's key (VIItemList::GetSlotId), u32 how many} and sets
// +0x13B90, which holds back every other Destroy until Msg003C, Msg00D7 or
// Msg00B2 clears it (or the start menu's item list is opened again). It takes
// nothing itself: the server answers 0x003C {u8 list index, u8 sound 0,
// zigzag how many}, and Msg003C takes them (VIItemList::Took) and clears the
// flag. A worn item comes off first, as the item menu's Unequip takes it off
// (0x0040), and the gear is read again after. A refusal is 0x00D7, which
// clears the flag and shows nothing. Until 2026-11-05 nothing answered (16
// requests that day alone, "Unhandled opcode: 0x003C"): the item stayed, and
// the menu sent no other Destroy that session. No record of what a server
// said to a Destroy exists, so nothing is said.

// HandleDestroyItem is 0x003C from the client: how many of the record a key
// names are destroyed.
func (h *EquipHandler) HandleDestroyItem(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 8 {
		log.Printf("DestroyItem: short payload len=%d hex=%x", len(data), data)
		refuse(sess)
		return nil
	}
	r := buffer.NewReader(data)
	key, _ := r.ReadUint32()
	count, _ := r.ReadUint32()
	if !h.destroy(sess, key, int(int32(count))) {
		refuse(sess)
	}
	return nil
}

// destroy takes n of the record key names out of the database and the
// client's list, the item off first when it is worn, and tells the client;
// false (nothing changed) when the list holds no such record or fewer than n.
func (h *EquipHandler) destroy(sess *session.Session, key uint32, n int) bool {
	if h.playerOf == nil || h.inventoryStore == nil {
		return false
	}
	player := h.playerOf(sess)
	l := sessionInvList(sess)
	if player == nil || l == nil {
		return false
	}
	sid := player.CharServerID
	items, err := h.inventoryStore.GetCharacterItems(sid)
	if err != nil {
		log.Printf("DestroyItem: %s: the inventory: %v", player.Name, err)
		return false
	}
	idx, row, err := invItemByKey(items, l, key)
	if err != nil {
		log.Printf("DestroyItem: %s, key %d: %v", player.Name, key, err)
		return false
	}
	// the client's Took asserts the record holds at least n, and the row
	// must hold them too
	rec, _ := l.at(idx)
	held := max(row.StackLeft, 1)
	if n < 1 || n > rec.qty || n > held {
		log.Printf("DestroyItem: %s destroys %d of %q, of which the list holds %d and the inventory %d",
			player.Name, n, row.Pattern.ItemName, rec.qty, held)
		return false
	}
	worn := row.EquipLoc >= 0
	if worn {
		off := equipPlan{name: row.Pattern.ItemName, key: key, changes: []equipChange{{row.ItemID, -1}}, unequip: row.EquipLoc}
		if !h.applyPlan(sess, sid, off, "DestroyItem") {
			return false
		}
	}
	if n == held {
		err = h.inventoryStore.DeleteCharItem(sid, row.ItemID) // its quick slot too
	} else {
		err = h.inventoryStore.SetItemStack(sid, row.ItemID, held-n)
	}
	if err != nil {
		log.Printf("DestroyItem: %s, %q: %v", player.Name, row.Pattern.ItemName, err)
		if worn {
			h.wornChanged(sess, player, sid) // it came off all the same
		}
		return false
	}
	l.took(idx, n)
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, n))
	keepBuyBack(h.inventoryStore, player, row, n, n*merchantWouldPay(row), database.BuyBackDestroyed) // buyback.go
	if worn {
		h.wornChanged(sess, player, sid)
	}
	how := ""
	if worn {
		how = ", taken off first"
	}
	log.Printf("DestroyItem: %s destroyed %d of %q (item %d, list index %d, %d left)%s",
		player.Name, n, row.Pattern.ItemName, row.ItemID, idx, held-n, how)
	return true
}

package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// ammoHeld is how many pieces of ammunition of the kind the pack holds as the
// client's list has it (entity/ammo.go): the rows HasAmmoFor counts, every
// stack of them.
func ammoHeld(l *invList, rows []database.CharInventoryEntry, kind int) int {
	n := 0
	for _, r := range ammoRows(l, rows, kind) {
		n += max(r.StackLeft, 1)
	}
	return n
}

// ammoRows is the pack's ammunition of the kind in the client's list order:
// in the pack (not the bank, not kept beside the list), not worn, a record
// HasAmmoFor counts.
func ammoRows(l *invList, rows []database.CharInventoryEntry, kind int) []database.CharInventoryEntry {
	var out []database.CharInventoryEntry
	for _, r := range inventoryListOrder(l, rows) {
		if r.Location != 0 || r.EquipLoc >= 0 || !entity.IsAmmo(r.Pattern.Unk3, r.Pattern.EquipSlot, kind) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ammoSpent takes one piece of a shot's ammunition off the character's pack
// (entity's AmmoSpentFunc): the first stack of the kind in the client's list
// order, as HasAmmoFor finds one; with the last of it the record goes.
func (h *EquipHandler) ammoSpent(sess *session.Session, charServerID uint32, kind int) {
	if h.inventoryStore == nil || h.gameWorld == nil {
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil || p.CharServerID != charServerID {
		return
	}
	items, err := h.inventoryStore.GetCharacterItems(charServerID)
	if err != nil {
		log.Printf("Ammo: %s's shot: %v", p.Name, err)
		return
	}
	l := sessionInvList(sess)
	ammo := ammoRows(l, items, kind)
	if len(ammo) == 0 {
		log.Printf("Ammo: %s shot and holds no ammunition of kind %d", p.Name, kind)
		return
	}
	row := ammo[0]
	idx := -1
	if l != nil {
		idx = l.indexOfItem(row.ItemID)
	}
	if idx < 0 {
		log.Printf("Ammo: %s's %q is not in the client's list", p.Name, row.Pattern.ItemName)
		return
	}
	if err := h.takeOne(sess, p, idx, &row); err != nil {
		log.Printf("Ammo: %s's %q: %v", p.Name, row.Pattern.ItemName, err)
		return
	}
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
}

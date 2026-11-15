package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// takeoff.go — a worn item taken off by the server (the bank, a trade), and
// a change of the hands put into the game.
//
// Until 2026-11-15 this was handswitch.go, the server's switch of hands for
// an art (the user, 2026-11-01: "fix rangers, they can dual wield and switch
// to a bow"): a cast its required equipment alone would refuse had what it
// named put in the hands from the pack, and an arrow art landed in melee
// reach put the blades back. The owner, 2026-11-15: "abilities are causing
// players to auto switch weapons, but should require players to switch
// manually to use those abilities if they dont have the correct weapon"
// (petition 5161: Cleave took a Prismatic No Dachi off for an Incandescent
// Club from the pack). Such a cast is now refused, its line naming the
// weapons it takes (entity/workbookmore.go equipRefusal), and the hands are
// the player's to change (the item menu, a weapon set). Mercs still change
// their own hands from their own kit (entity/mercshoot.go); the balance's
// simulated fights still switch, as a player who switches by hand would
// (entity/balancesim.go handsFor).

// The wear slots of the hands' items the handlers read (itempattern.equipslot;
// equip.go), and the ranged weapon's equip slot.
const (
	wearTwoHand = 15
	equipRanged = 18
)

// TakeOff takes a worn item off as the item menu's unequip does (the row's
// equip slot written -1, UnequipItem for its slot, the worn gear and look
// again), so the bank or a trade can move it (the user, 2026-11-06: "at the
// moment if you have an item equipped, you cannot bank or trade. lets allow
// equipped to be banked, traded"; until then both refused a worn item, 21
// times that day). A row not worn is left as it is. False when the database
// refused.
func (h *EquipHandler) TakeOff(sess *session.Session, player *entity.Player, row *database.CharInventoryEntry) bool {
	if row == nil || row.EquipLoc < 0 {
		return true
	}
	slot := row.EquipLoc
	if h.inventoryStore == nil {
		return false
	}
	if err := h.inventoryStore.SetEquipLoc(player.CharServerID, row.ItemID, -1); err != nil {
		log.Printf("TakeOff: %s's %q from slot %d: %v", player.Name, row.Pattern.ItemName, slot, err)
		return false
	}
	log.Printf("TakeOff: %s's %q off slot %d", player.Name, row.Pattern.ItemName, slot)
	queueUnequip(sess, slot) // nothing of the cape's slot to a client that does not draw capes (capes.go)
	row.EquipLoc = -1
	h.wornChanged(sess, player, player.CharServerID)
	return true
}

// wornChanged puts a change of the hands into the game: the look and the
// gear (applyEquipmentFromDB) and a fight's swing (handsChanged).
func (h *EquipHandler) wornChanged(sess *session.Session, player *entity.Player, sid uint32) {
	if h.charHandler != nil {
		h.charHandler.applyEquipmentFromDB(player, sid, false)
	}
	handsChanged(h.gameWorld, h.inventoryStore, sess, player)
}

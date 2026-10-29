package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
)

// NoRentAway is how long a character may be out of the world before its No
// Rent items are gone: the owner's ruling of 2026-10-29, "it vanishes if the
// player logs out for more than 5 minutes", of the items a spell summons
// ("Summoned items should all be "No rent""). An item is No Rent by its
// record (itempattern.rent 0, CLIENT's NoRent, as the auction's CanSell
// reads it); the game's own items carry 1.
const NoRentAway = 5 * time.Minute

// takeNoRent takes away a character's No Rent items, worn, in the inventory
// and in the bank, at a login more than NoRentAway after it left the world
// (table charlogout, saved at a logout and a shutdown: udp/norentsave.go),
// before anything of its inventory is read for the world. The logout's row
// is taken with the reading, so a character with none (none since the table
// was made, or a server that stopped without its save) keeps them, and a
// character taken over in the world never left it. Each item taken is
// logged; nothing is said to the player (the item is simply not there).
func (h *CharacterHandler) takeNoRent(record *database.CharacterRecord, now time.Time) {
	if h.store == nil || h.inventoryStore == nil {
		return
	}
	left, ok, err := h.store.TakeLogout(record.ServerID)
	if err != nil {
		log.Printf("SelectedChar: '%s' logout time: %v", record.CharName, err)
		return
	}
	if !ok {
		return
	}
	away := now.Sub(left)
	if away <= NoRentAway {
		return
	}
	items, err := h.inventoryStore.NoRentItems(record.ServerID)
	if err != nil {
		log.Printf("SelectedChar: '%s' No Rent items: %v", record.CharName, err)
		return
	}
	for _, e := range items {
		if err := h.inventoryStore.DeleteCharItem(record.ServerID, e.ItemID); err != nil {
			log.Printf("SelectedChar: '%s' No Rent %q (item %d) not taken: %v", record.CharName, e.Pattern.ItemName, e.ItemID, err)
			continue
		}
		log.Printf("SelectedChar: '%s' away %v: No Rent %d of %q (pattern %d, item %d, location %d, worn %d) gone",
			record.CharName, away.Round(time.Second), e.StackLeft, e.Pattern.ItemName, e.Pattern.PatternID, e.ItemID, e.Location, e.EquipLoc)
	}
}

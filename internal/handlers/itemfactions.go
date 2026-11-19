package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// An item whose Use raises the user's standings (table itemfactions,
// database/itemfactions.go): petition 5089 (Live, a player, 2026-11-15:
// "would it be possible to get some kind of item or something, so I can test
// the other 17s?"), the owner: "give them faction stone that increases
// factions with all lvl 17 q givers". Each row brings one faction up to its
// standing, never down; the item is not spent, so a standing lost since (a
// kill's hit) comes back at the next Use. Each change is told in the faction
// line a kill's is; a Use that changes nothing says so.

// Our words.
const (
	itemFactionsDone  = "The %s glows. Your standing has risen with %d of its factions."
	itemFactionsQuiet = "The %s is quiet: your standing with every one of its factions is already as high as it can raise it."
)

// useItemFactions answers an item's Use when it has itemfactions rows; false
// when it has none (the Use goes on as before).
func (h *EquipHandler) useItemFactions(sess *session.Session, player *entity.Player, pat *database.ItemPatternRecord) bool {
	if h.inventoryStore == nil || h.gameWorld == nil || player == nil {
		return false
	}
	floors, err := h.inventoryStore.ItemFactionsOf(pat.PatternID)
	if err != nil {
		log.Printf("ItemFactions: %q: %v", pat.ItemName, err)
		return false
	}
	if len(floors) == 0 {
		return false
	}
	raised, ok := h.gameWorld.RaiseFactions(sess, floors)
	if !ok {
		return true
	}
	name := strings.TrimSpace(pat.ItemName)
	if len(raised) == 0 {
		SendClientMessage(sess, fmt.Sprintf(itemFactionsQuiet, name))
	} else {
		SendClientMessage(sess, fmt.Sprintf(itemFactionsDone, name, len(raised)))
	}
	log.Printf("ItemFactions: %s used %q: %d of %d factions raised", player.Name, name, len(raised), len(floors))
	return true
}

package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// dungeonreturn.go — the Scroll of Returning's Use (Heirloom petition 3199, Wolfmoon:
// "Heroic Dungeon scrolls: scrolls that return you to town, and back to your exact
// position in the dungeon before consumed on use"). entity/dungeonreturn.go keeps the
// place and decides the way back; here the item is known by its record's name (made
// and stocked by tools/scroll_of_returning_20261126.py: a record cloned from a scroll,
// sold by Freeport's Spell Archivist beside the Heroic Dungeons board) and spent on the
// way back alone: read in a heroic dungeon it takes its reader home and is kept, read
// again within entity.DungeonReturnWindow it takes it back and crumbles.
//
// Refused, nothing done: a Hardcore character (hcrules.go: the server's own
// conveniences carry none), a session the quality-of-life additions are off for
// (world.vanilla_ui, an [OG] character: vanillaui.go), and a use within another
// cast or trip (TravelBusy), answered with nothing as a cast is.

// ScrollOfReturningName is the scroll's record's name.
const ScrollOfReturningName = "Scroll of Returning"

// The scroll's refusals (ours).
const (
	returnHardcoreWords = "The Scroll of Returning will not carry a Hardcore adventurer."
	returnQoLWords      = "The Scroll of Returning has no power on this server."
)

// IsScrollOfReturning is whether an item record is the scroll: worn nowhere and so named.
func IsScrollOfReturning(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), ScrollOfReturningName)
}

// useScrollOfReturning is the scroll's Use: the World's read, its line to the player,
// and the scroll spent when it took the player back.
func (h *EquipHandler) useScrollOfReturning(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry) {
	if h.gameWorld == nil || player == nil {
		return
	}
	if player.HardcoreBound() {
		hcSay(sess, returnHardcoreWords)
		return
	}
	if !QoLOnFor(sess) {
		SendErrorMessage(sess, returnQoLWords)
		return
	}
	now := time.Now()
	if h.gameWorld.TravelBusy(sess, now) {
		return
	}
	res, line := h.gameWorld.DungeonReturnRead(sess, now)
	switch res {
	case entity.ReturnRefused:
		SendErrorMessage(sess, line)
		return
	case entity.ReturnBack:
		if err := h.takeOne(sess, player, idx, row); err != nil {
			log.Printf("Scroll of Returning: %s's scroll not taken: %v", player.Name, err)
		}
	}
	SendColoredChat(sess, line, 230, 190, 120)
}

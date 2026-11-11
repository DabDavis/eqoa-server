package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// An item whose Use near an NPC takes that NPC out of the world for a while
// (table itemhide; the Cazic-Thule trials, 2026-11-11, work board 656's code
// ask 2): "the spawner goes to the main entrance, and uses "Cazic's Scent" to
// remove the Guardian blocking the doorway" (the t196 walkthrough), needed
// "each and every time". The item is not spent. The NPC is out of every view
// for the row's minutes (World.HideNPCs, a trigger's hide), put back on its
// spawn and out of its fight first; a user not yet past the row's quest step
// (as a wall's passer is: questwalls.go) is refused, and so is one with no
// such NPC within the row's radius.

// Our words, when a row has none.
const (
	itemHideDone   = "The %s fades from the doorway."
	itemHideRefuse = "Nothing answers it yet."
	itemHideNoneAt = "Nothing here answers it."
)

// PassedStep is whether a character is past a step of a quest: the quest
// finished, or under way at a later step (questwalls.go's rule).
func (q *QuestEngine) PassedStep(serverID uint32, quest, step int) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	cq := q.charLocked(serverID).quests[quest]
	return cq != nil && (cq.Done > 0 || (cq.Active() && cq.Step > step))
}

// useItemHide answers an item's Use when it has an itemhide row; false when it
// has none (the Use goes on as before).
func (h *EquipHandler) useItemHide(sess *session.Session, player *entity.Player, pat *database.ItemPatternRecord) bool {
	if h.inventoryStore == nil || h.gameWorld == nil || player == nil {
		return false
	}
	row, ok, err := h.inventoryStore.ItemHideOf(pat.PatternID)
	if err != nil {
		log.Printf("ItemHide: %q: %v", pat.ItemName, err)
		return false
	}
	if !ok {
		return false
	}
	if row.QuestID != 0 && !Quests().PassedStep(player.CharServerID, row.QuestID, row.Step) {
		SendClientMessage(sess, orText(row.Refuse, itemHideRefuse))
		return true
	}
	world, x, _, z, alive := h.gameWorld.PlayerPlace(player)
	if !alive {
		return true
	}
	r := float32(row.Radius)
	if r <= 0 {
		r = 20
	}
	n := h.gameWorld.HideNPCs(row.NPC, world, x, z, r, time.Now().Add(time.Duration(row.Minutes*float64(time.Minute))))
	if n == 0 {
		SendClientMessage(sess, itemHideNoneAt)
		return true
	}
	text := row.Text
	if strings.TrimSpace(text) == "" {
		text = strings.Replace(itemHideDone, "%s", row.NPC, 1)
	}
	SendClientMessage(sess, text)
	log.Printf("ItemHide: %s used %q: %d %q out of the world for %.0f min", player.Name, pat.ItemName, n, row.NPC, row.Minutes)
	return true
}

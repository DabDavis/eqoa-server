package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/charmove"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Character transfers at the login (database/transfers.go): a GM's approved
// transfer applies when its character logs in, before the login reads the
// character, so no logout save can write over it; the player is told once in
// the world. A character still in the world (a login that takes it over as it
// stands) is not the time: the live character would be saved over the
// transfer, so it waits for a login that reads it from the database.

// applyTransfer applies serverID's approved transfer, if it has one and is not
// in the world: true when it did (the record is then read again).
func (h *CharacterHandler) applyTransfer(serverID uint32) bool {
	if h.store == nil {
		return false
	}
	db := h.store.DB()
	t, ok, err := db.ApprovedTransfer(serverID)
	if err != nil {
		log.Printf("Transfers: character %d's approved transfer: %v", serverID, err)
		return false
	}
	if !ok {
		return false
	}
	if h.gameWorld != nil {
		if _, live := h.gameWorld.PlayerByCharacter(serverID); live {
			log.Printf("Transfers: transfer %d waits: '%s' is still in the world (taken over as it stands)", t.ID, t.CharName)
			return false
		}
	}
	plan, err := charmove.ParsePlan([]byte(t.Plan))
	if err != nil {
		log.Printf("Transfers: transfer %d's plan: %v", t.ID, err)
		return false
	}
	plan.CharacterID = serverID
	var out strings.Builder
	run, err := charmove.Migrate(db.DB, plan, true, false, &out)
	if err != nil {
		log.Printf("Transfers: transfer %d for '%s' NOT applied: %v", t.ID, t.CharName, err)
		return false
	}
	if err := db.MarkTransferApplied(t.ID, run, time.Now()); err != nil {
		log.Printf("Transfers: transfer %d applied as run %s but not marked: %v", t.ID, run, err)
	}
	log.Printf("Transfers: transfer %d applied at '%s''s login, run %s:\n%s", t.ID, t.CharName, run, strings.TrimSpace(out.String()))
	return true
}

// tellTransfers tells a player entering the world of each transfer applied
// to its character it has not been told of.
func tellTransfers(db *database.SQLiteDB, sess *session.Session, p *entity.Player) {
	if db == nil {
		return
	}
	list, err := db.TransfersToTell(p.CharServerID)
	if err != nil || len(list) == 0 {
		return
	}
	for _, t := range list {
		line := fmt.Sprintf("Your character from %s has arrived (transfer #%d): its gear is in your pack, the bank when the pack was full.", t.FromServer, t.ID)
		if t.Answer != "" {
			line += " A GM says: " + t.Answer
		}
		SendColoredChat(sess, line, 255, 255, 0)
		if err := db.MarkTransferTold(t.ID); err != nil {
			log.Printf("Transfers: transfer %d told but not marked: %v", t.ID, err)
		}
	}
}

// TellTransferApproved tells a character in the world that its transfer was
// approved and applies at its next login; false when it is not in the world.
func TellTransferApproved(gw *entity.World, sm *session.Manager, serverID uint32, id int64) bool {
	sess := SessionOfCharacter(gw, sm, serverID)
	if sess == nil {
		return false
	}
	SendColoredChat(sess, fmt.Sprintf("Your character transfer #%d was approved: log out and back in to receive it.", id), 255, 255, 0)
	return true
}

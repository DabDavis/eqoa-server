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
	// the quest engine keeps a character's quests from their first use for
	// the server's run: read them again, or it offers the quests just done
	if len(plan.Quests) > 0 {
		if q := Quests(); q != nil {
			q.ReloadCharacter(serverID)
		}
	}
	log.Printf("Transfers: transfer %d applied at '%s''s login, run %s:\n%s", t.ID, t.CharName, run, strings.TrimSpace(out.String()))
	return true
}

// transferLineMax is the most a transfer line to the player holds; a longer
// one is cut after a sentence (splitSay).
const transferLineMax = 200

// ApproximatedNotice is what a player is asked of the items a transfer made
// for what this server lacked (the user, 2026-11-05: "some armor, equipment
// pieces aren't added yet so will be approximated. Please submit bug report
// to fix them with the correct stats, icon, model").
func ApproximatedNotice(names []string) string {
	return fmt.Sprintf("Some armor and equipment pieces are not on this server yet, so these were approximated: %s. "+
		"Please submit a bug report (type /bug) with each one's correct stats, icon and model so a GM can fix them.",
		strings.Join(names, ", "))
}

// transferNotice is a transfer's lines to its player: head, the GM's word,
// and the items made for what this server lacked with the ask to report them.
func transferNotice(head string, t database.Transfer) []string {
	lines := []string{head}
	if a := strings.TrimSpace(t.Answer); a != "" {
		for _, l := range sayLines(a) {
			lines = append(lines, splitSay("A GM says: "+l, transferLineMax)...)
		}
	}
	if names := database.ApproximatedItems(t.Plan); len(names) > 0 {
		lines = append(lines, splitSay(ApproximatedNotice(names), transferLineMax)...)
	}
	return lines
}

// readyLine is an approved transfer's first line to a player on the
// character here (serverid; the transfer's own: log out and in).
func readyLine(t database.Transfer, here uint32) string {
	if t.ServerID == here {
		return fmt.Sprintf("Your character transfer #%d is ready: log out and back in to receive it.", t.ID)
	}
	return fmt.Sprintf("Your character transfer #%d is ready: log %s in to receive it.", t.ID, t.CharName)
}

func tellLines(sess *session.Session, lines []string) {
	for _, l := range lines {
		SendColoredChat(sess, l, 255, 255, 0)
	}
}

// tellTransfers tells a player entering the world of each transfer applied
// to its character it has not been told of, then of each of its account's
// transfers approved since it was last in the world.
func tellTransfers(db *database.SQLiteDB, sess *session.Session, p *entity.Player) {
	if db == nil {
		return
	}
	arrived, err := db.TransfersToTell(p.CharServerID)
	if err != nil {
		log.Printf("Transfers: %s's to tell: %v", p.Name, err)
	}
	for _, t := range arrived {
		tellLines(sess, transferNotice(fmt.Sprintf("Your character from %s has arrived (transfer #%d): its gear is in your pack, "+
			"the bank when the pack was full.", t.FromServer, t.ID), t))
		if err := db.MarkTransferTold(t.ID); err != nil {
			log.Printf("Transfers: transfer %d told but not marked: %v", t.ID, err)
		}
	}
	ready, err := db.TransfersReadyToTell(int(sess.AccountID()))
	if err != nil {
		log.Printf("Transfers: %s's account's ready to tell: %v", p.Name, err)
	}
	for _, t := range ready {
		tellLines(sess, transferNotice(readyLine(t, p.CharServerID), t))
		if err := db.MarkTransferToldReady(t.ID); err != nil {
			log.Printf("Transfers: transfer %d told ready but not marked: %v", t.ID, err)
		}
	}
}

// TellTransferApproved tells the player of an approved transfer at once on a
// character of its account in the world (the transfer's own first: log out
// and in), with the GM's word and the items approximated; false when none is
// in the world (it is told at its next entry: tellTransfers).
func TellTransferApproved(db *database.SQLiteDB, gw *entity.World, sm *session.Manager, t database.Transfer) bool {
	sess, here := sessionOfAccount(gw, sm, t.AccountID, t.ServerID)
	if sess == nil {
		return false
	}
	tellLines(sess, transferNotice(readyLine(t, here), t))
	if db != nil {
		if err := db.MarkTransferToldReady(t.ID); err != nil {
			log.Printf("Transfers: transfer %d told ready but not marked: %v", t.ID, err)
		}
	}
	return true
}

// sessionOfAccount is a session of the account with its character in the
// world, the character prefer's when it is one of them, and its character.
func sessionOfAccount(gw *entity.World, sm *session.Manager, accountID int, prefer uint32) (*session.Session, uint32) {
	if gw == nil || sm == nil || accountID <= 0 {
		return nil, 0
	}
	var found *session.Session
	var char uint32
	sm.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() || int(sess.AccountID()) != accountID {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() {
			return true
		}
		p := gw.SessionPlayer(sess)
		if p == nil || !gw.IsLivePlayer(p) {
			return true
		}
		if found == nil || p.CharServerID == prefer {
			found, char = sess, p.CharServerID
		}
		return p.CharServerID != prefer
	})
	return found, char
}

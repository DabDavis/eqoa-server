package handlers

import (
	"log"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// safemode.go — /testinvis, a player's safe mode, everyone's (petition 1424,
// the GM Jibrish; entity/safemode.go has the rules): typed, no creature
// attacks the character on sight until it is typed again. A fight the player
// starts works as any other, and it can die. The mode is kept in table
// charsafemode, so it outlives a logout and every restart; a login in it is
// told so once in the world.
//
// world.testinvis (SetTestInvis: off unless set; the Heirloom Test Server
// sets it) turns the command on. With it off the command says so, and a
// character left in safe mode is taken out of it at its next login.

// Its spellings: "/testinvis" and "#testinvis" (chat.go, before the GMs'
// commands), and a PS2's "/t testinvis" (it keeps an unknown /command to
// itself and cannot type a #).
const testInvisWord = "testinvis"

// The lines it answers with.
const (
	safeOnLine    = "Safe mode on: no creature attacks you on sight. A fight you start goes on as any other, and you can still die. Type /testinvis again to turn it off."
	safeOffLine   = "Safe mode off: creatures attack you on sight again."
	safeLoginLine = "You are in safe mode: no creature attacks you on sight. Type /testinvis to turn it off."
	safeOffServer = "Safe mode (/testinvis) is off on this server."
)

var testInvisOn atomic.Bool

// SetTestInvis sets world.testinvis.
func SetTestInvis(on bool) { testInvisOn.Store(on) }

// TestInvisOn reports world.testinvis.
func TestInvisOn() bool { return testInvisOn.Load() }

// safeModeStore is where the mode is kept (database.SQLiteDB).
type safeModeStore interface {
	SafeMode(serverID uint32) (bool, error)
	SetSafeMode(serverID uint32, on bool) error
}

// SetSafeModeStore sets where /testinvis keeps the mode.
func (h *ChatHandler) SetSafeModeStore(s safeModeStore) { h.safe = s }

// testInvisCommand is /testinvis: the player's safe mode turned the other way.
func (h *ChatHandler) testInvisCommand(sess *session.Session, player *entity.Player) {
	if h.gameWorld == nil || player == nil {
		return
	}
	if !TestInvisOn() {
		SendErrorMessage(sess, safeOffServer)
		return
	}
	on, ok := h.gameWorld.SafeMode(sess)
	if !ok {
		return
	}
	on = !on
	if h.safe != nil {
		if err := h.safe.SetSafeMode(player.CharServerID, on); err != nil {
			log.Printf("Safe mode: '%s' not kept: %v", player.Name, err)
			SendErrorMessage(sess, "Safe mode could not be changed just now: try again.")
			return
		}
	}
	if !h.gameWorld.SetSafeMode(sess, on) {
		return
	}
	if on {
		SendColoredChat(sess, safeOnLine, 255, 255, 0)
	} else {
		SendColoredChat(sess, safeOffLine, 255, 255, 0)
	}
	log.Printf("Safe mode: '%s' turned it %s", player.Name, map[bool]string{true: "on", false: "off"}[on])
}

// restoreSafeMode gives a login just built from its row (sess's player) the
// safe mode it was left in (table charsafemode); with world.testinvis off it
// takes the character out of it instead.
func (h *CharacterHandler) restoreSafeMode(sess *session.Session, record *database.CharacterRecord) {
	if h.store == nil || h.gameWorld == nil {
		return
	}
	db := h.store.DB()
	on, err := db.SafeMode(record.ServerID)
	if err != nil {
		log.Printf("SelectedChar: '%s' safe mode: %v", record.CharName, err)
		return
	}
	if !on {
		return
	}
	if !TestInvisOn() {
		if err := db.SetSafeMode(record.ServerID, false); err != nil {
			log.Printf("SelectedChar: '%s' safe mode not taken off: %v", record.CharName, err)
		}
		return
	}
	if h.gameWorld.SetSafeMode(sess, true) {
		log.Printf("SelectedChar: '%s' is in safe mode (/testinvis)", record.CharName)
	}
}

// tellSafeMode tells a player entering the world in safe mode, once a login,
// that it is (OnPlayerEntered: after the world entry, when a line is read).
func tellSafeMode(w *entity.World) func(sess *session.Session, p *entity.Player) {
	return func(sess *session.Session, p *entity.Player) {
		if !TestInvisOn() {
			return
		}
		if on, ok := w.SafeMode(sess); ok && on {
			SendColoredChat(sess, safeLoginLine, 255, 255, 0)
		}
	}
}

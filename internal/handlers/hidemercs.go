package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// hidemercs.go — /hide mercs and /show mercs, everyone's (petition 1858, the
// GM Jibrish, 2026-11-12: "add the command /hide mercs and /show mercs — this
// hide the character model and nameplate for mercenaries, or restore them";
// entity/hidemercs.go has the rule). Hidden, other players' mercenaries are
// sent to the player's own client no more, so it draws neither their bodies
// nor their name plates; nothing of the game changes, and everyone else sees
// them as before. The player's own mercs stay in view: it commands them by
// the merc menu (□ on its own merc), which a merc off its client could not
// open. The choice is kept in table charhidemercs, so it outlives a logout
// and every restart; a login with them hidden is told so once in the world.
//
// Its spellings: "/hide mercs", "/show mercs", "#hide mercs" and "#show mercs"
// (chat.go, before the GMs' commands), and a PS2's "/t hide mercs" and "/t
// show mercs" (it keeps an unknown /command to itself and cannot type a #).
// No other command is /hide or /show: Hide Look is the item menu's
// (hidelook.go), by its own opcode. "/t hide" with anything but "mercs" is a
// tell.

// The lines it answers with.
const (
	mercsHiddenLine      = "Other players' mercenaries are hidden from your view (/show mercs to see them again). Your own stay in view."
	mercsShownLine       = "Other players' mercenaries are in your view again (/hide mercs to hide them)."
	mercsHiddenLoginLine = "Other players' mercenaries are hidden from your view. Type /show mercs to see them again."
)

// mercsHiddenStore is where /hide mercs is kept (database.SQLiteDB).
type mercsHiddenStore interface {
	MercsHidden(serverID uint32) (bool, error)
	SetMercsHidden(serverID uint32, on bool) error
}

// SetMercsHiddenStore sets where /hide mercs is kept.
func (h *ChatHandler) SetMercsHiddenStore(s mercsHiddenStore) { h.hideMercs = s }

// hideMercsLine reads a typed line as /hide mercs or /show mercs: the command
// word ("/hide", "#show", any case; a PS2's "/t hide mercs" comes as "/hide")
// and its rest. ok false for any other line.
func hideMercsLine(word, rest string) (hide, ok bool) {
	if !strings.EqualFold(strings.TrimSpace(rest), "mercs") {
		return false, false
	}
	switch strings.ToLower(word) {
	case "/hide", "#hide":
		return true, true
	case "/show", "#show":
		return false, true
	}
	return false, false
}

// hideMercsCommand is /hide mercs (hide) and /show mercs: other players'
// mercenaries out of the player's view or back in it.
func (h *ChatHandler) hideMercsCommand(sess *session.Session, player *entity.Player, hide bool) {
	if h.gameWorld == nil || player == nil {
		return
	}
	if h.hideMercs != nil {
		if err := h.hideMercs.SetMercsHidden(player.CharServerID, hide); err != nil {
			log.Printf("Hide mercs: '%s' not kept: %v", player.Name, err)
			SendErrorMessage(sess, "Mercenaries could not be hidden or shown just now: try again.")
			return
		}
	}
	if !h.gameWorld.SetMercsHidden(sess, hide) {
		return
	}
	if hide {
		SendColoredChat(sess, mercsHiddenLine, 255, 255, 0)
	} else {
		SendColoredChat(sess, mercsShownLine, 255, 255, 0)
	}
	log.Printf("Hide mercs: '%s' %s other players' mercenaries", player.Name, map[bool]string{true: "hid", false: "shows"}[hide])
}

// restoreMercsHidden gives a login just built from its row (sess's player)
// the /hide mercs it was left with (table charhidemercs).
func (h *CharacterHandler) restoreMercsHidden(sess *session.Session, record *database.CharacterRecord) {
	if h.store == nil || h.gameWorld == nil {
		return
	}
	on, err := h.store.DB().MercsHidden(record.ServerID)
	if err != nil {
		log.Printf("SelectedChar: '%s' hidden mercs: %v", record.CharName, err)
		return
	}
	if on && h.gameWorld.SetMercsHidden(sess, true) {
		log.Printf("SelectedChar: '%s' hides other players' mercenaries (/hide mercs)", record.CharName)
	}
}

// tellMercsHidden tells a player entering the world with other players'
// mercenaries hidden, once a login, that they are (OnPlayerEntered).
func tellMercsHidden(w *entity.World) func(sess *session.Session, p *entity.Player) {
	return func(sess *session.Session, p *entity.Player) {
		if on, ok := w.MercsHidden(sess); ok && on {
			SendColoredChat(sess, mercsHiddenLoginLine, 255, 255, 0)
		}
	}
}

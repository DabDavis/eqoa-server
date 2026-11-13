package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// hidegmtag.go — /hide gmtag and /show gmtag, a GM's (the owner, 2026-11-13:
// "for gm accounts let them be able to hide GM tag from their chars";
// entity/hidegmtag.go has the rule). Hidden, the GM's name is drawn bare over
// its head and in a target's window, as a player's; it keeps every GM power.
// The choice is the character's, kept in table charhidegmtag, so it outlives a
// logout and every restart.
//
// Its spellings, as /hide mercs's (hidemercs.go): "/hide gmtag", "/show
// gmtag", "#hide gmtag", "#show gmtag" and a PS2's "/t hide gmtag", "/t show
// gmtag"; "gm tag" and "gm" do too. A player who is no GM is told it has none.

// The lines it answers with.
const (
	gmTagHiddenLine = "Your [GM] tag is hidden: your name shows as a player's (/show gmtag to show it again)."
	gmTagShownLine  = "Your [GM] tag shows again over your name (/hide gmtag to hide it)."
	gmTagNoneLine   = "Only a GM has a [GM] tag to hide."
)

// gmTagHiddenStore is where /hide gmtag is kept (database.SQLiteDB).
type gmTagHiddenStore interface {
	GMTagHidden(serverID uint32) (bool, error)
	SetGMTagHidden(serverID uint32, on bool) error
}

// SetGMTagHiddenStore sets where /hide gmtag is kept.
func (h *ChatHandler) SetGMTagHiddenStore(s gmTagHiddenStore) { h.hideGMTag = s }

// hideGMTagLine reads a typed line as /hide gmtag or /show gmtag (hide). ok
// false for any other line.
func hideGMTagLine(word, rest string) (hide, ok bool) {
	switch strings.ToLower(strings.Join(strings.Fields(rest), " ")) {
	case "gmtag", "gm tag", "gm":
	default:
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

// hideGMTagCommand is /hide gmtag (hide) and /show gmtag.
func (h *ChatHandler) hideGMTagCommand(sess *session.Session, player *entity.Player, hide bool) {
	if h.gameWorld == nil || player == nil {
		return
	}
	if !player.GM {
		SendErrorMessage(sess, gmTagNoneLine)
		return
	}
	if h.hideGMTag != nil {
		if err := h.hideGMTag.SetGMTagHidden(player.CharServerID, hide); err != nil {
			log.Printf("Hide GM tag: '%s' not kept: %v", player.Name, err)
			SendErrorMessage(sess, "Your [GM] tag could not be hidden or shown just now: try again.")
			return
		}
	}
	if !h.gameWorld.SetGMTagHidden(sess, hide) {
		return
	}
	if hide {
		SendColoredChat(sess, gmTagHiddenLine, 255, 255, 0)
	} else {
		SendColoredChat(sess, gmTagShownLine, 255, 255, 0)
	}
	log.Printf("Hide GM tag: '%s' %s its [GM] tag", player.Name, map[bool]string{true: "hid", false: "shows"}[hide])
}

// restoreGMTagHidden gives a GM entering the world (SendGMChannel, once its
// player is known a GM's) the /hide gmtag its character was left with (table
// charhidegmtag).
func (h *ChatHandler) restoreGMTagHidden(sess *session.Session) {
	if h.hideGMTag == nil || h.gameWorld == nil {
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return
	}
	on, err := h.hideGMTag.GMTagHidden(p.CharServerID)
	if err != nil {
		log.Printf("GM channel: '%s' hidden GM tag: %v", p.Name, err)
		return
	}
	if on && h.gameWorld.SetGMTagHidden(sess, true) {
		log.Printf("GM channel: '%s' hides its [GM] tag (/hide gmtag)", p.Name)
	}
}

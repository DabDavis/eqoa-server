package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// quickchat.go — /quickchat reset, a player's own quick chat (the L2 d-pad's
// menus) back to the game's defaults, everyone's (work board 1960, petition
// 2624: a player had changed the lines his group and pet commands were on).
// A character's sets are its own hotkeys rows once it changes one (hotkeys.go);
// with none its MemoryDump carries defaultHotkeySets, so the reset deletes its
// rows and the defaults come at its next login. CLIENT reads the sets only from
// the dump (VIRec6A), and a dump where the player stands clears its pet and
// target, so nothing is resent: the player is told to log in again. A change
// made before that writes the defaults and then itself (hotkeyEdit).
//
// Its spellings: "/quickchat reset" and "#quickchat reset" (chat.go, before the
// GMs' commands), and a PS2's "/t quickchat reset". "/quickchat" alone, or with
// any other word, only says how; "/t quickchat" with another body is a tell.
const quickChatWord = "quickchat"

// The lines it answers with.
const (
	quickChatDoneLine = "Your quick chat is back to the game's defaults. Log out and back in (or change characters) to load them."
	quickChatSameLine = "Your quick chat is already the game's defaults. If your menu still shows your own lines, log out and back in."
	quickChatHelpLine = "Type /quickchat reset to put your quick chat (the L2 menus) back to the game's defaults. Your own lines are lost."
	quickChatFailLine = "Your quick chat could not be reset just now: try again."
)

// quickChatStore is where a character's sets are kept (database.SQLiteDB).
type quickChatStore interface {
	ResetHotkeys(serverID uint32) (int, error)
}

// SetQuickChatStore sets where /quickchat reset deletes a character's sets.
func (h *ChatHandler) SetQuickChatStore(s quickChatStore) { h.quickChat = s }

// quickChatCommand is /quickchat with rest, the words after it: "reset"
// deletes the character's sets; anything else answers how.
func (h *ChatHandler) quickChatCommand(sess *session.Session, player *entity.Player, rest string) {
	if player == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(rest), "reset") {
		SendColoredChat(sess, quickChatHelpLine, 255, 255, 0)
		return
	}
	if h.quickChat == nil || player.CharServerID == 0 {
		SendErrorMessage(sess, quickChatFailLine)
		return
	}
	n, err := h.quickChat.ResetHotkeys(player.CharServerID)
	if err != nil {
		log.Printf("Hotkeys: '%s' (character %d) not reset: %v", player.Name, player.CharServerID, err)
		SendErrorMessage(sess, quickChatFailLine)
		return
	}
	if n == 0 {
		SendColoredChat(sess, quickChatSameLine, 255, 255, 0)
		return
	}
	log.Printf("Hotkeys: '%s' (character %d) reset to the defaults (%d sets)", player.Name, player.CharServerID, n)
	SendColoredChat(sess, quickChatDoneLine, 255, 255, 0)
}

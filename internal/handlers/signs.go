package handlers

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Signs: the sign posts and the towns' announcement boards (the user,
// 2026-10-30: "we also need to add signpost dialog, in cities usually it is
// the title of the building, on the roads outside town, they usually give
// directions to nearest towns in 4 directions or so"). Every one of the
// server's 46 stands on one body, SignBody; the Frontiers manual: "Use the R1
// button to target the signpost and push the [square] button and the
// signpost will give you helpful directions".
//
// A sign says its row of signtext (database/signtext.go, tools/seed_signs.py)
// in dialogue boxes, as a greeter says world.npc_text (a sign with no row
// still does, by its name), and a board then the news row's words and gives
// the town's boon (entity/boon.go: the user, 2026-11-05, "players want the
// boon at the Freeport Announcement, and other city announcements"). Nothing
// else answers a sign: never a quest (it gives none, and no step is done at
// it), the town dialog, a shop or a route.

// SignBody is the body every sign post and announcement board stands on
// (CHAR.ESF's 0x6A82352C).
const SignBody = 0x6A82352C

// boardName is an announcement board by its name: "Freeport Announcements",
// and Surefall's "Surefall Anouncements".
var boardName = regexp.MustCompile(`(?i)\bann?ouncements?\s*$`)

// SetSignTexts gives the handler signtext's reader (database.SignTexts).
func (h *InteractHandler) SetSignTexts(f func() (map[int]database.SignText, error)) {
	h.signTexts = f
}

// IsSign reports a sign: an NPC on SignBody, or one signtext gives words to
// (a GM may make any NPC a sign by giving it a row).
func IsSign(npc *entity.NPC, rows map[int]database.SignText) bool {
	if uint32(npc.ModelID) == SignBody {
		return true
	}
	_, ok := signRow(npc, rows)
	return ok
}

// signRow is the NPC's row of signtext: by its npcs row, so never a GM's
// temporary copy's (which has none, its id the world's own).
func signRow(npc *entity.NPC, rows map[int]database.SignText) (database.SignText, bool) {
	if npc.Temporary {
		return database.SignText{}, false
	}
	row, ok := rows[int(npc.ServerID)]
	return row, ok
}

// IsBoard reports a town's announcement board: a sign whose row's kind is
// board, or with no row one named as a board.
func IsBoard(npc *entity.NPC, rows map[int]database.SignText) bool {
	row, has := signRow(npc, rows)
	return row.Kind == database.SignKindBoard || (!has && boardName.MatchString(npc.Name))
}

// BoardTown is the town a board's name gives, its "Announcements" taken off:
// "Freeport Announcements" is Freeport's; a name without the word is its own.
func BoardTown(name string) string {
	return strings.TrimSpace(boardName.ReplaceAllString(name, ""))
}

// SignWords is what a sign says: its row's text, else world.npc_text's for
// its name; a board's (IsBoard) followed by the news row's when there is
// news. Empty: nothing.
func SignWords(npc *entity.NPC, rows map[int]database.SignText) string {
	row, has := signRow(npc, rows)
	text := strings.TrimSpace(row.Text)
	if !has {
		text, _ = npcTextFor(npc.Name)
		text = strings.TrimSpace(text)
	}
	if news := strings.TrimSpace(rows[database.SignNewsID].Text); IsBoard(npc, rows) && news != "" {
		if text == "" {
			return news
		}
		return text + "\n\n" + news
	}
	return text
}

// signSay answers a sign: its words in dialogue boxes, or nothing, and a
// board's boon; it reports whether the NPC was a sign (nothing else then
// answers it).
func (h *InteractHandler) signSay(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	var rows map[int]database.SignText
	if h.signTexts != nil {
		r, err := h.signTexts()
		if err != nil {
			log.Printf("Interact: signtext: %v", err)
		}
		rows = r
	}
	if !IsSign(npc, rows) {
		return false
	}
	forgetDialogs(player)
	if text := SignWords(npc, rows); text == "" {
		log.Printf("Interact: the sign %s (%d) has nothing to say to %s", npc.Name, npc.ServerID, player.Name)
	} else {
		// a box at most CLIENT shows, as a quest NPC's words (tellLocked); a
		// '%' doubled, which CLIENT's text expander (VIClientText::Expand)
		// shows as one
		for _, part := range splitSay(text, questBoxChars) {
			h.sendDialogueBox(sess, strings.ReplaceAll(part, "%", "%%"))
		}
	}
	if IsBoard(npc, rows) && h.gameWorld != nil {
		if g, ok := h.gameWorld.GiveBoon(sess, BoardTown(npc.Name), time.Now()); ok {
			SendClientMessage(sess, g.Line())
		}
		// and the boards' Spirit of the Wolf (entity/boardwolf.go)
		if given, renewed := h.gameWorld.GiveBoardWolf(sess, time.Now()); given {
			SendClientMessage(sess, boardWolfLine(renewed))
		}
	}
	return true
}

// boardWolfLine is the player's line for the boards' Spirit of the Wolf (ours).
func boardWolfLine(renewed bool) string {
	if renewed {
		return "The board renews your Spirit of the Wolf for an hour of play. A hostile act ends it."
	}
	return "The spirit of the wolf quickens your step for an hour of play. A hostile act ends it."
}

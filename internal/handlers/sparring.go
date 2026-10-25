package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A sparring dummy's menu (entity/sparring.go; F3 20261025-174503): talked
// to, it asks the level to spar at, then the element to strike with; the
// player then strikes it to begin, as any dummy.

const (
	sparAskLevel   = "Spar with me at what level?"
	sparAskElement = "And with what shall I strike you?"
)

// askSparLevel opens the dummy's first question.
func (h *InteractHandler) askSparLevel(sess *session.Session, player *entity.Player, npc *entity.NPC) {
	forgetDialogs(player)
	player.PendingSpar = entity.SparPick{NPC: npc.ServerID, Asked: 1}
	options := make([]string, len(entity.SparLevels))
	for i, l := range entity.SparLevels {
		options[i] = fmt.Sprintf("Level %d", l)
	}
	h.sendOptionBox(sess, sparAskLevel, options)
}

// answerSpar takes an answer: a level asks the element, an element sets the
// dummy (refused while it spars with someone else).
func (h *InteractHandler) answerSpar(sess *session.Session, player *entity.Player, option byte) {
	pick := player.PendingSpar
	player.PendingSpar = entity.SparPick{}
	switch pick.Asked {
	case 1:
		if int(option) >= len(entity.SparLevels) {
			return
		}
		player.PendingSpar = entity.SparPick{NPC: pick.NPC, Asked: 2, Level: int(option)}
		options := make([]string, len(entity.SparElements))
		for i, e := range entity.SparElements {
			options[i] = e.Name
		}
		h.sendOptionBox(sess, sparAskElement, options)
	case 2:
		if int(option) >= len(entity.SparElements) {
			return
		}
		name, busy, ok := h.gameWorld.SetSparring(pick.NPC, player.ServerID, pick.Level, int(option))
		switch {
		case ok:
			sess.Queue().AddWithSequence(buildClientMessage(fmt.Sprintf("%s: Level %d, %s. Strike me when you are ready.",
				name, entity.SparLevels[pick.Level], strings.ToLower(entity.SparElements[option].Name))))
		case busy != "":
			sess.Queue().AddWithSequence(buildClientMessage(fmt.Sprintf("%s: I am sparring with %s.", name, busy)))
		default:
			log.Printf("Sparring: %s's pick for NPC %d was refused", player.Name, pick.NPC)
		}
	}
}

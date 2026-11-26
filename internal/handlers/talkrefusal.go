package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The people of a faction listed in factionrefusetalk will not speak to a
// player it holds at or below -350 (entity/talkrefusal.go; petition 3111):
// a talk (HandleInteract, before any of the NPC's business: its quests, talk
// tree, town lines, menus) and a bank's or blacksmith's window are answered
// with one refusal line in a dialogue box: the NPC's own hostile line of
// npcdialog when it has one (ownRefusal, as a coachman's), else its faction's
// line, else entity.DefaultTalkRefusal. A GM is always spoken to. A merchant
// of any faction refuses the red bands already (factionrefusal.go).

// talkRefuses reports whether npc will not speak to the session's player, and
// says the refusal when it will not.
func (h *InteractHandler) talkRefuses(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	if h.gameWorld == nil || (h.isGM != nil && h.isGM(sess)) {
		return false
	}
	faction, line, ok := h.gameWorld.RefusesTalk(sess, npc)
	if !ok {
		return false
	}
	log.Printf("Talk: %q will not speak to %s, at or below %d with %s", npc.Name, player.Name, entity.FactionKOSAt, faction)
	sayTalkRefusal(sess, h.gameWorld, h.dialogLines, player, npc, line)
	return true
}

// windowRefuses is talkRefuses for a window's request (a bank, a blacksmith):
// the caller refuses the request.
func (h *NPCWindowHandler) windowRefuses(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	if h.gameWorld == nil || (h.isGM != nil && h.isGM(sess)) {
		return false
	}
	faction, line, ok := h.gameWorld.RefusesTalk(sess, npc)
	if !ok {
		return false
	}
	log.Printf("Window: %q will not deal with %s, at or below %d with %s", npc.Name, player.Name, entity.FactionKOSAt, faction)
	sayTalkRefusal(sess, h.gameWorld, h.dialogLines, player, npc, line)
	return true
}

// sayTalkRefusal says npc's refusal in a dialogue box: its own hostile line
// (its name before it), else line ("" entity.DefaultTalkRefusal) with its
// words filled in (%n the NPC, %p the player).
func sayTalkRefusal(sess *session.Session, gw *entity.World, lines func() ([]database.NPCDialogLine, error), player *entity.Player, npc *entity.NPC, line string) {
	forgetDialogs(player)
	text := ""
	if speaker, own, ok := ownRefusal(gw, lines, player, npc); ok {
		text = speaker + ": " + own
	} else {
		if line == "" {
			line = entity.DefaultTalkRefusal
		}
		text = entity.DialogSay(line, player, strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(npc.Name), "$")), "")
	}
	for _, part := range splitSay(text, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

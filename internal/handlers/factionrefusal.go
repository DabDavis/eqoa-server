package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Coachmen and merchants refuse a player their faction holds in the red bands
// (board #742, jibrish's decision from live-era film; the owner, 2026-11-21:
// "use jibs -350", in place of the coach gate's -99 of the same day):
// entity.World.Refuses, a standing at or below entity.FactionKOSAt with the
// NPC's npcfaction faction. An NPC of no faction serves anyone.
//
//   - A coachman (coachMenu, before his menu; signLedger signs nothing) says
//     his own hostile line of npcdialog in a dialogue box, his name before it,
//     as townSay says a line: Coachman Lothi's "I will not help you %p. Be
//     glad I do not slay you where you stand for crimes against Neriak!",
//     Coachman Zuggug's "Gah! You be evil! ...". One with no hostile line of
//     his own says coachRefusalLine (Lothi's first sentence, the only generic
//     part on film). A town's hostile lines for its people are not his. A GM
//     is always served.
//   - A merchant (HandleMerchantOpen, after windowNPC) opens no window (no
//     0x00B7; 0x00D7 clears the client's pending request): one with a hostile
//     line of its own name says it in a dialogue box (the Jextalan
//     shopkeepers), any other prints merchantRefusal in chat, red as the
//     server's other refusals. A buy or a sale (a standing that fell while the
//     window was open) is refused with the chat line. A genie's window is
//     never refused (Refuses).
//
// Warp NPCs, waysenders and dockmasters were not filmed refusing: unchanged.

// coachRefusalLine is what a coachman with no hostile line of his own says
// as he refuses (%p the player's name).
const coachRefusalLine = "I will not help you, %p."

// merchantRefusal is a refusing merchant's chat line (the baga shopkeeper, on
// film).
const merchantRefusal = "Your faction with that merchant is too low for trading."

// coachRefuses reports whether the coachman will not serve the session's
// player (entity.World.Refuses); a GM is always served.
func (h *InteractHandler) coachRefuses(sess *session.Session, npc *entity.NPC) bool {
	if h.gameWorld == nil || (h.isGM != nil && h.isGM(sess)) {
		return false
	}
	if !h.gameWorld.Refuses(sess, npc) {
		return false
	}
	log.Printf("Coach: %s refuses a player at or below %d with %s", npc.Name, entity.FactionKOSAt, h.gameWorld.NPCFaction(npc))
	return true
}

// sayCoachRefusal is the coachman's refusal: his own hostile line, else
// coachRefusalLine, in a dialogue box with his name before it.
func (h *InteractHandler) sayCoachRefusal(sess *session.Session, player *entity.Player, npc *entity.NPC) {
	speaker, line, ok := ownRefusal(h.gameWorld, h.dialogLines, player, npc)
	if !ok {
		speaker = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(npc.Name), "$")) // as TownLine shows it
		line = entity.DialogSay(coachRefusalLine, player, speaker, "")
	}
	forgetDialogs(player)
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// ownRefusal is npc's own hostile line to player (entity.World.RefusalLine),
// ok false when it has none or npcdialog cannot be read.
func ownRefusal(gw *entity.World, lines func() ([]database.NPCDialogLine, error), player *entity.Player, npc *entity.NPC) (speaker, line string, ok bool) {
	if gw == nil || lines == nil {
		return "", "", false
	}
	rows, err := lines()
	if err != nil {
		log.Printf("Refusal: npcdialog: %v", err)
		return "", "", false
	}
	return gw.RefusalLine(player, npc, rows)
}

// SetDialogLines gives the windows npcdialog's reader (database.NPCDialog),
// for a refusing merchant's own words.
func (h *NPCWindowHandler) SetDialogLines(f func() ([]database.NPCDialogLine, error)) {
	h.dialogLines = f
}

// merchantRefuses reports whether the merchant will not trade with the
// session's player (entity.World.Refuses), and says so when it will not: at
// the window's opening (talk) its own hostile line when it has one, else, and
// at a buy or a sale, merchantRefusal in red. The caller refuses the request.
func (h *NPCWindowHandler) merchantRefuses(sess *session.Session, player *entity.Player, npc *entity.NPC, talk bool) bool {
	if h.gameWorld == nil || !h.gameWorld.Refuses(sess, npc) {
		return false
	}
	log.Printf("Merchant: %q refuses %s, at or below %d with %s", npc.Name, player.Name, entity.FactionKOSAt, h.gameWorld.NPCFaction(npc))
	if talk {
		if speaker, line, ok := ownRefusal(h.gameWorld, h.dialogLines, player, npc); ok {
			forgetDialogs(player)
			for _, part := range splitSay(speaker+": "+line, questBoxChars) {
				SendDialogueBox(sess, 0, part)
			}
			return true
		}
	}
	SendColoredChat(sess, merchantRefusal, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	return true
}

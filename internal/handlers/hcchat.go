package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hcchat.go — the Hardcore channel (the owner, 2026-11-18: "give hardcore
// players their own channel /hcchat"): a shard-wide channel that only living
// Hardcore characters in the world speak in and hear, "[HC] Name: text" in
// hcChatRGB, through the coloured chat line (0x0A7B) every
// client shows in its chat window. Spellings: "/hcchat text" and "/hc text"
// (vanilla forwards an unknown /command); a PS2 keeps an unknown /command to
// itself, so its player types "#hc text" (or "#hcchat"), which goes out as chat,
// or "/t hc text", a tell to the channel's name. A fallen character never
// enters the world, so never speaks; its account's other Hardcore characters do.
// The text is held to what a PS2 can show (consoleLine) and each hearer's
// profanity filter (lineFor), as a tell's. A fall is told in it too
// (hardcoreFell: the channel's colour and tag to the Hardcore, the shard's line
// to everyone else).

// hcChatMost is the most characters of a line's text (a chat line's 127).
const hcChatMost = 127

// hcChatRGB is the channel's colour: amber.
var hcChatRGB = [3]byte{0xFF, 0xA0, 0x3C}

// The channel's words (ours).
const (
	hcChatOnly  = "Only Hardcore characters speak and hear in the Hardcore channel."
	hcChatUsage = "Hardcore channel: %s and what to say."
)

// hcChatText reads a line meant for the channel: its text and true; "" with
// true for the words alone (the usage).
func hcChatText(line string) (string, bool) {
	line = strings.TrimSpace(line)
	word, rest, _ := strings.Cut(line, " ")
	for _, w := range []string{"/hcchat", "/hc", "#hcchat", "#hc"} {
		if strings.EqualFold(word, w) {
			return strings.TrimSpace(rest), true
		}
	}
	if name, body, ok := parseTell(line); ok && (strings.EqualFold(name, "hc") || strings.EqualFold(name, "hcchat")) {
		return strings.TrimSpace(body), true
	}
	return "", false
}

// hcChatSpelling is the channel's word as the player's client sends it.
func hcChatSpelling(ours bool) string {
	if ours {
		return "/hc"
	}
	return "#hc"
}

// hcChat is a line for the channel from player.
func (h *ChatHandler) hcChat(sess *session.Session, player *entity.Player, text string) {
	if !HardcoreOn() {
		SendErrorMessage(sess, hardcoreOffLine)
		return
	}
	if !player.Hardcore || player.Fallen {
		SendErrorMessage(sess, hcChatOnly)
		return
	}
	text = consoleLine(text, hcChatMost)
	if text == "" {
		SendErrorMessage(sess, fmt.Sprintf(hcChatUsage, hcChatSpelling(sess.AckMasks())))
		return
	}
	line := entity.HCTag + player.Name + ": " + text
	n := hcChatSay(h.gameWorld, line)
	log.Printf("HCChat: %s: %q to %d", player.Name, text, n)
}

// hcChatSay tells every living Hardcore character in the world line (each
// through its own profanity filter); it returns how many heard it.
func hcChatSay(gw *entity.World, line string) int {
	if gw == nil {
		return 0
	}
	sessions := playerSessions(gw)
	n := 0
	gw.ForEachPlayer(func(p *entity.Player) {
		s := sessions[p.ServerID]
		if s == nil || !p.Hardcore || p.Fallen {
			return
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeColoredChat)
		msg.SetData(coloredChatPayload(lineFor(p, line), hcChatRGB[0], hcChatRGB[1], hcChatRGB[2]))
		s.Queue().AddWithSequence(msg)
		n++
	})
	return n
}

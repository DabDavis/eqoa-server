package handlers

import (
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// afk.go — /afk, the away tag's commands (entity/afk.go has the rules): /afk
// and #afk from any client that forwards them, and a PS2's "/t afk" (it keeps
// an unknown /command to itself and sends /t on as typed). "/afk" turns it
// over, "/afk on" and "/afk off" go one way, "/afk <words>" puts it up with an
// away message (or, while away, changes the message). Every other typed line
// ends it (typedLine, shoutLine), and a tell to an away player is answered
// with its message (tell, whisper).

// afkLine reads an /afk line: what it asks and its message, ok false for any
// other line. A PS2's "/t afk <words>" is a tell to a player named Afk when one
// is in the world; "/t afk", "/t afk on" and "/t afk off" are always /afk.
func (h *ChatHandler) afkLine(message string) (want entity.AFKWant, msg string, ok bool) {
	word, rest, _ := strings.Cut(strings.TrimSpace(message), " ")
	if !strings.EqualFold(word, "/afk") && !strings.EqualFold(word, "#afk") {
		name, body, tell := parseTell(message)
		if !tell || !strings.EqualFold(name, "afk") {
			return 0, "", false
		}
		if b := strings.ToLower(strings.TrimSpace(body)); b != "" && b != "on" && b != "off" && findPlayerByName(h.gameWorld, name) != nil {
			return 0, "", false // a tell to a player named Afk
		}
		rest = body
	}
	rest = strings.TrimSpace(rest)
	first, more, _ := strings.Cut(rest, " ")
	switch strings.ToLower(first) {
	case "":
		return entity.AFKToggle, "", true
	case "off":
		if strings.TrimSpace(more) == "" {
			return entity.AFKWantOff, "", true
		}
	case "on":
		return entity.AFKWantOn, afkMessage(more), true
	}
	return entity.AFKToggle, afkMessage(rest), true
}

// afkMessage is an away message as other players are sent it: one line a PS2
// can draw (consoletext.go), at most entity.AFKMessageMost.
func afkMessage(s string) string {
	return strings.TrimSpace(consoleLine(strings.TrimSpace(s), entity.AFKMessageMost))
}

// afkTyped is a typed line's turn at the away tag: /afk handled (true), or any
// other line from an away player a sign of life that ends it (false: the line
// goes on).
func (h *ChatHandler) afkTyped(sess *session.Session, message string) bool {
	if h.gameWorld == nil {
		return false
	}
	if want, msg, ok := h.afkLine(message); ok {
		h.gameWorld.AFKAsk(sess, want, msg)
		return true
	}
	h.gameWorld.AFKActive(sess, "a chat line")
	return false
}

// afkAnswer answers a tell's (or a whisper's) sender from to's away message,
// at most once a minute to one sender (entity.AFKReply).
func (h *ChatHandler) afkAnswer(sess *session.Session, from, to *entity.Player) {
	if line, ok := h.gameWorld.AFKReply(to.ServerID, from.ServerID, time.Now()); ok {
		h.tellLine(sess, from, plainText(line))
	}
}

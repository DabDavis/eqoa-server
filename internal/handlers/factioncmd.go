package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// factionCommand is #faction: `<faction>` lists the GM's own standing with the
// factions whose names hold the words (any case), `<faction> <standing>` sets
// it with the one they name (entity/faction.go), kept at the logout as a
// kill's change is.
func (h *ChatHandler) factionCommand(sess *session.Session, args []string) {
	const usage = "Usage: #faction <faction> [standing]"
	if len(args) == 0 {
		h.gmReply(sess, usage)
		return
	}
	if len(args) >= 2 {
		if v, err := strconv.Atoi(args[len(args)-1]); err == nil {
			name, now, err := h.gameWorld.SetFaction(sess, strings.Join(args[:len(args)-1], " "), v)
			if err != nil {
				h.gmReply(sess, err.Error())
				return
			}
			h.gmReply(sess, fmt.Sprintf("Your standing with %s is now %d", strings.TrimSpace(name), now))
			return
		}
	}
	matches := h.gameWorld.FactionsMatching(sess, strings.Join(args, " "))
	if len(matches) == 0 {
		h.gmReply(sess, fmt.Sprintf("No faction's name holds %q. %s", strings.Join(args, " "), usage))
		return
	}
	const most = 8
	for i, f := range matches {
		if i == most {
			h.gmReply(sess, fmt.Sprintf("  ... and %d more", len(matches)-most))
			break
		}
		h.gmReply(sess, fmt.Sprintf("  %s: %d", strings.TrimSpace(f.Name), f.Standing))
	}
}

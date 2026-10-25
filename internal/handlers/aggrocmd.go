package handlers

import (
	"strings"

	"github.com/eqoa/server/internal/session"
)

// aggroCommand is #aggro (a GM's): with no word, whether creatures attack the
// GM's character on sight (entity/aggro.go); `off` leaves it alone until
// `#aggro on` or a restart, for a GM looking round hostile ground; a fight
// already on goes on.
func (h *ChatHandler) aggroCommand(sess *session.Session, args []string) {
	const usage = "Usage: #aggro [on|off]"
	if len(args) > 0 {
		var ignore bool
		switch strings.ToLower(args[0]) {
		case "off":
			ignore = true
		case "on":
		default:
			h.gmReply(sess, usage)
			return
		}
		if !h.gameWorld.SetAggroIgnored(sess, ignore) {
			return
		}
		if ignore {
			h.gmReply(sess, "Creatures leave you alone until #aggro on (or the server restarts); a fight already on goes on")
		} else {
			h.gmReply(sess, "Creatures attack you on sight again")
		}
		return
	}
	ignored, _, ok := h.gameWorld.AggroIgnored(sess)
	switch {
	case !ok:
	case ignored:
		h.gmReply(sess, "Creatures leave you alone (#aggro on undoes it)")
	default:
		h.gmReply(sess, "Creatures attack you on sight (#aggro off leaves you alone)")
	}
}

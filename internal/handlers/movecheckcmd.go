package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// moveCheckCommand is #movecheck (a GM's): with no word, the movement
// check's mode and who it moved back lately (entity/movecheck.go); `off`,
// `log` or `enforce` sets the mode for everyone, at once, until it is set
// again or the server restarts (world.movecheck is the mode it starts in).
func (h *ChatHandler) moveCheckCommand(sess *session.Session, name string, args []string) {
	if len(args) > 0 {
		m, err := entity.ParseMoveCheckMode(args[0])
		if err != nil || strings.TrimSpace(args[0]) == "" {
			h.gmReply(sess, "Usage: #movecheck [off|log|enforce]")
			return
		}
		was := entity.MoveCheck()
		entity.SetMoveCheck(m)
		log.Printf("GM: %s set the movement check from %s to %s", name, was, m)
		h.gmReply(sess, fmt.Sprintf("Movement check: %s (was %s)", m, was))
		return
	}
	h.gmReply(sess, fmt.Sprintf("Movement check: %s (#movecheck off|log|enforce)", entity.MoveCheck()))
	if h.gameWorld == nil {
		return
	}
	lines := h.gameWorld.MoveCheckSummary(time.Now())
	if len(lines) == 0 {
		h.gmReply(sess, "No one moved back in the last 5 minutes")
	}
	for _, l := range lines {
		h.gmReply(sess, l)
	}
}

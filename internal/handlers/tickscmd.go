package handlers

import "github.com/eqoa/server/internal/session"

// ticksCommand is #ticks (a GM's): the tick timer's last report, a line a
// part (entity/ticktime.go): how long the world's turns took over the last
// minute, the NPCs in each of the NPC sets (entity/npcsets.go), and what each
// system took. The same report is in the log every minute ("Tick: ...").
func (h *ChatHandler) ticksCommand(sess *session.Session) {
	if h.gameWorld == nil {
		return
	}
	lines := h.gameWorld.TickReport()
	if len(lines) == 0 {
		h.gmReply(sess, "No tick report yet: the first comes a minute after the server starts")
		return
	}
	for _, l := range lines {
		h.gmReply(sess, l)
	}
}

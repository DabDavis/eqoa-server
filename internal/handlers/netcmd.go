package handlers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// netcmd.go: #net [name] (a GM's), the reliable stream to the players'
// clients (session/netstats.go): what waits for an acknowledgement and how
// long, what went again in the last minute, the round trip, and the share of
// our last packets the client says it never had.
//
// Alone it lists every session with a character, the worst first (the one
// whose oldest unacknowledged message has waited longest); with a name, that
// player's whole report, and its client is asked for its own counters (0x07E0:
// CLIENT answers 0x07E1, connstats.go, relayed as the TEST menu's Network
// Stats relays them), which say what it threw away whole (a segment it could
// not parse) and how many of our messages came twice.

// netListMax is how many sessions #net lists alone.
const netListMax = 12

func (h *ChatHandler) netCommand(sess *session.Session, args []string) {
	if h.sessionManager == nil {
		h.gmReply(sess, "No sessions here")
		return
	}
	now := time.Now()
	if len(args) > 0 {
		h.netPlayer(sess, strings.Join(args, " "), now)
		return
	}
	type row struct {
		s *session.Session
		r session.NetReport
	}
	seen := map[*session.Session]bool{}
	var rows []row
	h.sessionManager.ForEach(func(s *session.Session) bool {
		if seen[s] || s.CharacterName() == "" {
			return true
		}
		seen[s] = true
		rows = append(rows, row{s, s.Queue().Report(now)})
		return true
	})
	if len(rows) == 0 {
		h.gmReply(sess, "No session has a character")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].r.Oldest != rows[j].r.Oldest {
			return rows[i].r.Oldest > rows[j].r.Oldest
		}
		return rows[i].r.ResentMinute > rows[j].r.ResentMinute
	})
	h.gmReply(sess, fmt.Sprintf("%d sessions with a character, the longest wait first (#net <name> for one):", len(rows)))
	for i, rw := range rows {
		if i == netListMax {
			h.gmReply(sess, fmt.Sprintf("  ... and %d more", len(rows)-netListMax))
			break
		}
		h.gmReply(sess, "  "+netLine(rw.s, rw.r))
	}
}

// netLine is one session in #net's list.
func netLine(s *session.Session, r session.NetReport) string {
	line := fmt.Sprintf("%s: %d unacked", s.CharacterName(), r.Unacked)
	if r.Unacked > 0 {
		line += fmt.Sprintf(" (oldest %.1fs)", r.Oldest.Seconds())
	}
	if r.Pending > 0 {
		line += fmt.Sprintf(", %d queued", r.Pending)
	}
	line += fmt.Sprintf(", resent %d/min", r.ResentMinute)
	if r.SRTT > 0 {
		line += fmt.Sprintf(", rtt %d ms", r.SRTT.Milliseconds())
	}
	if r.Lost > 0 && r.LostOf > 0 {
		line += fmt.Sprintf(", missed %d/%d", r.Lost, r.LostOf)
	}
	return line
}

// netPlayer is #net <name>: the whole report, and the client asked for its
// own counters.
func (h *ChatHandler) netPlayer(sess *session.Session, name string, now time.Time) {
	var found *session.Session
	h.sessionManager.ForEach(func(s *session.Session) bool {
		if strings.EqualFold(s.CharacterName(), name) {
			// the newest login of the name: a lingering session of the
			// same character is listed by #net alone
			if found == nil || s.LastActivity().After(found.LastActivity()) {
				found = s
			}
		}
		return true
	})
	if found == nil {
		h.gmReply(sess, fmt.Sprintf("No session has a character named %q", name))
		return
	}
	r := found.Queue().Report(now)
	h.gmReply(sess, fmt.Sprintf("%s (%s), heard from %.1fs ago:", found.CharacterName(), found.RemoteAddr(), now.Sub(found.LastActivity()).Seconds()))
	h.gmReply(sess, "  "+r.String())
	h.gmReply(sess, fmt.Sprintf("  sent %d packets (%d KB), resent %d messages in all", r.Packets, r.Bytes/1024, r.Resent))
	if gm := h.gameWorld.SessionPlayer(sess); gm != nil && found.CharacterInWorld() {
		h.gm.waitNetStats(found, gm.ServerID)
		SendServerReady(found)
		h.gmReply(sess, "  asked its client for its own counters (they follow when it answers)")
	}
}

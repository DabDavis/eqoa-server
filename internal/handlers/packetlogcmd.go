package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/debuglog"
	"github.com/eqoa/server/internal/session"
)

// packetLogCommand is #packetlog (a GM's): with no word, whether the packet
// path's own debug lines go to the log (internal/debuglog: every packet
// received and sent, its DRDP and state channel work, about 100 lines a
// second a player); `on` or `off` turns them on or off for every session at
// once, until set again or the server restarts (server.packet_log is what
// it starts with). cmd/movereplay reads a log written with them on.
func (h *ChatHandler) packetLogCommand(sess *session.Session, name string, args []string) {
	onOff := func(on bool) string {
		if on {
			return "on"
		}
		return "off"
	}
	if len(args) > 0 {
		var on bool
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "on":
			on = true
		case "off":
		default:
			h.gmReply(sess, "Usage: #packetlog [on|off]")
			return
		}
		was := debuglog.Packets()
		debuglog.SetPackets(on)
		log.Printf("GM: %s turned the packet log %s (was %s)", name, onOff(on), onOff(was))
	}
	h.gmReply(sess, fmt.Sprintf("Packet log: %s (#packetlog on|off: every packet's own lines, about 100 a second a player)", onOff(debuglog.Packets())))
}

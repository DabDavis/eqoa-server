package handlers

import (
	"encoding/binary"
	"log"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// ConnStatsHandler handles 0x07E1, the client's own DRDP connection counters.
//
// The client volunteers this; it is not a request and takes no reply. Handling
// it exists for two reasons:
//
//  1. It is the ONLY view we get of the connection from the PS2's side. The
//     server can see what it sent; only the client can say what it discarded,
//     what arrived corrupt, and what arrived out of sequence. Those counters
//     are exactly the ones that matter when a session misbehaves, and they were
//     being thrown away as "Unhandled opcode".
//  2. Without a handler it logged as unknown on every report, which buries real
//     unknowns in noise.
//
// Confirmed identical in shape on pcaps/ps2_sandstorm.pcap (packet #115,
// 512-byte payload) and on our own server, so this is retail behaviour and not
// something the Sandstorm patch introduced.
type ConnStatsHandler struct {
	// relay hands the counters to the GMs who asked for them (the GM's
	// channel's Network Stats, gmchannel.go)
	relay func(sess *session.Session, text string)
}

// NewConnStatsHandler creates a new connection statistics handler.
func NewConnStatsHandler() *ConnStatsHandler { return &ConnStatsHandler{} }

// RegisterHandlers registers the connection statistics handler.
func (h *ConnStatsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeConnStats, h.HandleConnStats)
}

// HandleConnStats parses and logs 0x07E1.
//
// Wire format, read off the capture:
//
//	int32_LE(charCount) + UTF-16LE text   // charCount is CHARACTERS, not bytes
//
// The text is newline-separated "<count>: <label>" lines, e.g.
//
//	0: discarded (old)
//	36: discarded (corrupt)
//	71: recv
//	27: guaranteed recv
//
// No response is sent — the reliable message is acknowledged by the RDP layer
// like any other, and retail sends nothing back.
func (h *ConnStatsHandler) HandleConnStats(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("ConnStats: short payload (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}

	charCount := int(binary.LittleEndian.Uint32(data[:4]))
	body := data[4:]

	// Trust the declared count only as far as the buffer allows — a truncated
	// or bogus length must not panic the handler.
	avail := len(body) / 2
	if charCount < 0 || charCount > avail {
		charCount = avail
	}

	u16 := make([]uint16, charCount)
	for i := 0; i < charCount; i++ {
		u16[i] = binary.LittleEndian.Uint16(body[i*2 : i*2+2])
	}
	text := string(utf16.Decode(u16))

	// Collapse to one line, and surface the counters that indicate trouble.
	var notable []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		num, label, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(num))
		if err != nil || n == 0 {
			continue // zero counters are the healthy case; don't report them
		}
		label = strings.TrimSpace(label)
		if strings.HasPrefix(label, "discarded") || strings.Contains(label, "out of sequence") {
			notable = append(notable, strconv.Itoa(n)+" "+label)
		}
	}

	if len(notable) > 0 {
		log.Printf("ConnStats from %s: PROBLEMS: %s | full: %s",
			sess.RemoteAddr(), strings.Join(notable, ", "),
			strings.ReplaceAll(strings.TrimSpace(text), "\n", "; "))
	} else {
		log.Printf("ConnStats from %s: %s", sess.RemoteAddr(),
			strings.ReplaceAll(strings.TrimSpace(text), "\n", "; "))
	}
	if h.relay != nil {
		h.relay(sess, text)
	}

	return nil
}

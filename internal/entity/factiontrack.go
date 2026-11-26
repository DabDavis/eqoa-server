package entity

import (
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// factiontrack.go — one faction's standing on a client's HUD (Heirloom
// petition 3216, Cucumber: "track one chosen faction on screen, outside the
// faction window"; the owner approved it 2026-11-26).
//
// The faction window's pages (handlers/faction.go, 0x0AC4 and 0x07E2) come only
// while it is open, so a client that draws a tracker names the faction it
// follows (protocol.OpcodeFactionTrackAnswer, after the offer at world entry)
// and is sent its standing at once and whenever it changes
// (OpcodeFactionTrackData): a kill's hits, a quest's, a spent or refunded
// standing, a GM's set, whatever moved it, as the window would show it (the
// player's own, else its start). The pass looks every factionTrackTicks ticks;
// a PS2 never answers the offer and is sent nothing.

// FactionTrackLayout is OpcodeFactionTrackData's layout, the offer's byte.
const FactionTrackLayout = 1

// factionTrackTicks is how often the pass looks for a changed standing: half a
// second at the 10 Hz tick.
const factionTrackTicks = 5

// factionTrack is what a player's client follows and was last sent.
type factionTrack struct {
	// asked is the answer's words; name the faction they name ("" none: the
	// client is told the name is unknown)
	asked, name string
	// sent is the standing last sent; due, a send at the next pass whatever it
	// is (a new answer, a keep restart's resume)
	sent int
	due  bool
}

// resolveFactionTrackLocked is the faction a client's words name, as the
// window lists them: the one of that name in any case, else the one name that
// holds them, else "" (none, or several). The Testing Heroes' is no one's on a
// server without them (heroes.go). Must be called with the lock held.
func (w *World) resolveFactionTrackLocked(asked string) string {
	if strings.TrimSpace(asked) == "" {
		return ""
	}
	name, _ := w.factionNamedLocked(asked)
	if name == HeroesFaction && !w.heroes.on {
		return ""
	}
	return name
}

// SetFactionTrack makes asked the faction sess's player's client follows, ""
// none. The faction it names (canonical, "" when it names none) and false when
// the session has no player in the world.
func (w *World) SetFactionTrack(sess *session.Session, asked string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", false
	}
	asked = strings.TrimSpace(asked)
	if asked == "" {
		p.factionTrack = nil
		return "", true
	}
	name := w.resolveFactionTrackLocked(asked)
	p.factionTrack = &factionTrack{asked: asked, name: name, due: true}
	return name, true
}

// FactionTrack is the words sess's player's client follows, "" none.
func (w *World) FactionTrack(sess *session.Session) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p := w.sessionPlayerLocked(sess); p != nil && p.factionTrack != nil {
		return p.factionTrack.asked
	}
	return ""
}

// factionTrackBody is OpcodeFactionTrackData's body past the opcode.
func factionTrackBody(b *buffer.Writer, known bool, name string, standing int) {
	b.WriteByte(FactionTrackLayout)
	if known {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
		standing = 0
	}
	writeWide(b, name, 128)
	b.Write7BitEncodedInt64(int64(standing))
}

// factionTrackSystem sends each following client its faction's standing when
// it changed since the last send (or a send is due). Called with the lock held.
func (w *World) factionTrackSystem() {
	if w.TickCount%factionTrackTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || p.factionTrack == nil {
			return true
		}
		t := p.factionTrack
		known := t.name != ""
		v := 0
		if known {
			v = w.ownStandingLocked(p, t.name)
		}
		if !t.due && v == t.sent {
			return true
		}
		t.sent, t.due = v, false
		name := t.name
		if !known {
			name = t.asked
		}
		sendGroupMsg(s, protocol.OpcodeFactionTrackData, func(b *buffer.Writer) { factionTrackBody(b, known, name, v) })
		return true
	})
}

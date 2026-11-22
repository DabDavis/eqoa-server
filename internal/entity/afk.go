package entity

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// afk.go — /afk, an away tag over a player's head (the owner, 2026-11-22, on a
// player's petition: "I'm frequently hailed when afk ... a /afk feature that
// puts (AFK) over a players head": "yes add the /afk tag").
//
// /afk (#afk, a PS2's "/t afk": handlers/afk.go) turns it over; "/afk on" and
// "/afk off" go one way only; "/afk <words>" puts it up with an away message,
// which answers a tell (or a whisper) to the player while it is away, "<Name>
// is away: <words>" ("<Name> is away." with none), at most once a minute to
// each teller (afkReplyEvery).
//
// The tag is AFKTag on the drawn name, the entity channel's 24 bytes at 116,
// which every viewer's client reads afresh: the channel is made anew every tick
// (stateSyncSystem, entityChannelSyncSystem), so the change goes out at once,
// as the [HC]/[OG] tags' do. It goes nearest the name, after every other tag
// ("[GM][OG][HC][AFK] Name"), and is the first tag dropped when the 23 bytes
// will not hold them all (tagDropOrder, hardcore.go): it never takes another
// tag's or the name's room. PlainName and IsNameTag know it, and ShownNames
// carries its forms, so a name typed or sent back with it finds the character.
//
// It ends on its own at the player's next sign of life: a typed line other
// than /afk (handlers/afk.go), an attack press (attack.go, duel.go, pvp.go), a
// cast begun (cast.go), or a 0x40 report more than afkMoveMost from where it
// began (clientupdate.go; a server-side move, a GM's summon, takes the anchor
// with it: afkMovedLocked). It is in no row: a logout ends it; a takeover (the
// same Player) and a keep restart (keep.go, the anchor and message with it)
// keep it.

// AFKTag goes before an away player's name where it is drawn.
const AFKTag = "[AFK] "

const (
	// afkMoveMost is how far a 0x40 report may be from where the player went
	// away before it counts as moving: a walk, not a stance's jitter.
	afkMoveMost = 2.0
	// afkReplyEvery is how often one teller is answered by the away message.
	afkReplyEvery = time.Minute
	// AFKMessageMost is the most bytes of an away message (handlers cut it).
	AFKMessageMost = 80
)

// The lines the player is told.
const (
	afkLineOn     = "You are now away (AFK)."
	afkLineOff    = "You are no longer away."
	afkLineNotOn  = "You are not away."
	afkLineMsg    = "Your away message: "
	afkLineNoMsg  = "Tells to you are answered that you are away."
	afkLineIsOnTo = "You are already away (AFK). /afk off ends it."
)

// afkRGB is the colour of the away lines, the system's yellow.
var afkRGB = [3]byte{0xFF, 0xFF, 0x60}

// afkState is a player's away tag: on, its message, where it began (the
// anchor a move is measured from), and when each teller was last answered.
type afkState struct {
	on      bool
	msg     string
	at      [3]float32
	replied map[EntityID]time.Time
}

// AFK reports whether the player is away.
func (p *Player) AFK() bool { return p.afk.on }

// AFKWant is what /afk asks: toggle, on or off.
type AFKWant int

const (
	AFKToggle AFKWant = iota
	AFKWantOn
	AFKWantOff
)

// AFKAsk is /afk: the tag turned over (AFKToggle), put up (AFKWantOn) or
// taken down (AFKWantOff), the player told. msg (already cleaned and cut by
// the caller) is the away message when it goes up; asked with a message while
// already away, the message is replaced. It reports whether the tag changed.
func (w *World) AFKAsk(sess *session.Session, want AFKWant, msg string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > AFKMessageMost {
		msg = msg[:AFKMessageMost]
	}
	if want == AFKToggle {
		want = AFKWantOn
		if p.afk.on && msg == "" {
			want = AFKWantOff
		}
	}
	if want == AFKWantOff {
		if !p.afk.on {
			w.sendColoredLocked(sess, afkLineNotOn, afkRGB)
			return false
		}
		w.afkEndLocked(p, sess, "/afk off")
		return true
	}
	if p.afk.on {
		if msg == "" {
			w.sendColoredLocked(sess, afkLineIsOnTo, afkRGB)
			return false
		}
		p.afk.msg = msg
		w.sendColoredLocked(sess, afkLineMsg+msg, afkRGB)
		return false
	}
	p.afk = afkState{on: true, msg: msg, at: [3]float32{p.X, p.Y, p.Z}}
	log.Printf("AFK: %s is away (%q)", p.Name, msg)
	w.sendColoredLocked(sess, afkLineOn, afkRGB)
	if msg != "" {
		w.sendColoredLocked(sess, afkLineMsg+msg, afkRGB)
	} else {
		w.sendColoredLocked(sess, afkLineNoMsg, afkRGB)
	}
	return true
}

// AFKActive is a sign of life from sess's player that is not /afk (a typed
// line: handlers/afk.go): an away tag ends. It reports whether one did.
func (w *World) AFKActive(sess *session.Session, why string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !p.afk.on {
		return false
	}
	w.afkEndLocked(p, sess, why)
	return true
}

// afkActiveLocked is a sign of life from p (an attack press, a cast): an away
// tag ends, the player told.
func (w *World) afkActiveLocked(p *Player, why string) {
	if p == nil || !p.afk.on {
		return
	}
	w.afkEndLocked(p, w.sessionOfLocked(p), why)
}

// afkMovedLocked is a 0x40 report taken, before the place it moved p from:
// past afkMoveMost from where the player went away, the tag ends. A place
// already that far before the report was the server's move (a teleport, a GM's
// summon, a respawn: every report's own would have ended it), so the anchor
// goes there first, and only the client's own walk from it counts.
func (w *World) afkMovedLocked(p *Player, before [3]float32) {
	if !p.afk.on {
		return
	}
	if afkDist(before, p.afk.at) > afkMoveMost {
		p.afk.at = before
	}
	if afkDist([3]float32{p.X, p.Y, p.Z}, p.afk.at) > afkMoveMost {
		w.afkActiveLocked(p, "a move")
	}
}

// afkDist is the distance between two places.
func afkDist(a, b [3]float32) float64 {
	dx, dy, dz := float64(a[0]-b[0]), float64(a[1]-b[1]), float64(a[2]-b[2])
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// afkEndLocked takes p's tag down and tells it (sess may be nil: nobody told).
func (w *World) afkEndLocked(p *Player, sess *session.Session, why string) {
	p.afk = afkState{}
	log.Printf("AFK: %s is back (%s)", p.Name, why)
	if sess != nil {
		w.sendColoredLocked(sess, afkLineOff, afkRGB)
	}
}

// AFKReply is the line a tell from teller to the player to answers with while
// it is away ("<Name> is away: <msg>", "<Name> is away."), and true, at most
// once each afkReplyEvery to one teller; "" and false when it is not away or
// that teller had one within the minute. A player telling itself is answered
// nothing.
func (w *World) AFKReply(to, teller EntityID, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[to]
	if p == nil || !p.afk.on || to == teller {
		return "", false
	}
	if last, ok := p.afk.replied[teller]; ok && now.Sub(last) < afkReplyEvery {
		return "", false
	}
	if p.afk.replied == nil {
		p.afk.replied = make(map[EntityID]time.Time)
	}
	p.afk.replied[teller] = now
	if p.afk.msg == "" {
		return p.Name + " is away.", true
	}
	return p.Name + " is away: " + p.afk.msg, true
}

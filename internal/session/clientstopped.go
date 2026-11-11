package session

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// clientstopped.go — a client that stops in the world with no logout (the
// owner, 2026-11-11, after two players' clients stopped acking at once in a
// raid: "log a line when a session times out mid-world with no logout"). A
// client in the world is never quiet long: CLIENT sends its 0x40 ten times a
// second (a page in a hidden tab too) and a keepalive once 30 s pass with
// nothing sent, so a session in the world silent ClientStoppedSilence while
// its world is not loading (StateChannelsHeld: a PS2 reads a world off the
// disc in up to 22 s) has a client that stopped: crashed, hung, or its line
// dropped. NoteClientStopped says so in the log once, naming the character,
// and keeps it for the character's next crash report (handlers/crashreport.go:
// the server's side of the moment beside the client's); a client heard from
// again after it is said to have been hung (NoteClientHeard), not crashed.
// A login that takes the character over from a session silent that long says
// it the same way (entity.TakeOverPlayer).

// ClientStoppedSilence is how long a client in the world may be silent before
// it is said to have stopped: past a PS2's longest world load off the disc,
// short of the session's end (InGameSilentEnd, 60 s), so a player who logs in
// again at once is still seen.
const ClientStoppedSilence = 30 * time.Second

// clientStopsKept is how long a stop is kept for a crash report.
const clientStopsKept = 7 * 24 * time.Hour

// ClientStop is a client said to have stopped: its character, when it was
// last heard from and how the server saw it ("silent", "taken over").
type ClientStop struct {
	Name  string    `json:"name"`
	Heard time.Time `json:"heard"`
	Noted time.Time `json:"noted"`
	How   string    `json:"how"`
}

var clientStops = struct {
	sync.Mutex
	by map[uint32]ClientStop
}{by: map[uint32]ClientStop{}}

// NoteClientStopped says, once until its client is heard from again, that s's
// client stopped in the world at now (how: what the server saw), and keeps
// it by character. False when it was said already.
func NoteClientStopped(s *Session, how string, now time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if s.stoppedNoted {
		s.mu.Unlock()
		return false
	}
	s.stoppedNoted = true
	heard := s.lastActivity
	s.mu.Unlock()
	id, name := s.CharacterServerID(), s.CharacterName()
	log.Printf("Session: %s's client stopped in the world with no logout, last heard %s (%.0fs ago), %s: a crash, a hang or a dropped line",
		s.Label(), heard.UTC().Format("15:04:05"), now.Sub(heard).Seconds(), how)
	if id != 0 {
		clientStops.Lock()
		for k, v := range clientStops.by {
			if now.Sub(v.Noted) > clientStopsKept {
				delete(clientStops.by, k)
			}
		}
		clientStops.by[id] = ClientStop{Name: name, Heard: heard, Noted: now, How: how}
		clientStops.Unlock()
	}
	return true
}

// NoteClientHeard is s's client heard from again: one said to have stopped
// was hung, not gone (the log says so, and its stop is no longer kept).
func NoteClientHeard(s *Session, now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	was := s.stoppedNoted
	s.stoppedNoted = false
	s.mu.Unlock()
	if !was {
		return
	}
	log.Printf("Session: %s's client is heard from again: it was hung, not gone", s.Label())
	clientStops.Lock()
	delete(clientStops.by, s.CharacterServerID())
	clientStops.Unlock()
}

// ClientStopOf is the character's last stop kept, false with none.
func ClientStopOf(charServerID uint32) (ClientStop, bool) {
	clientStops.Lock()
	defer clientStops.Unlock()
	c, ok := clientStops.by[charServerID]
	return c, ok
}

// String is the stop as a GM reads it.
func (c ClientStop) String() string {
	return fmt.Sprintf("%s's client stopped in the world with no logout, last heard %s (%s)", c.Name,
		c.Heard.UTC().Format(time.RFC3339), c.How)
}

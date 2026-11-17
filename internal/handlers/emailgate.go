package handlers

import (
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Every account needs a verified e-mail (the owner, 2026-11-15: "A verified
// email is REQUIRED for every account, old ones included"; mail.require_verified
// and mail.require_after, internal/accountmail's Gate).
//
//   - Before the cutoff: a line in yellow at each world entry and every
//     mail.remind_every (30 minutes) while it plays, naming the account page.
//   - From the cutoff the account is refused at the login, not in the world
//     (the owner: "we are able to update and patch the station.pak to
//     accomodate ps2/pcsx2"): the Station login answers it with
//     tcp.EmailRefusalStatus, whose page the served station.pak writes (the
//     reason and the website), and the UDP handshake refuses it as a bad
//     login (AuthHandler.SetEmailRefusal). So nothing here acts past the
//     cutoff; a session already in the world then (kept over a restart) plays
//     on to its next login.
//
// The gate reads this process's accounts, so a shard (its copy of the owner's,
// database.AccountMirror) holds its players to it the same.

// emailRemindRGB is the reminder's colour (a GM's broadcast's yellow).
var emailRemindRGB = [3]uint8{255, 230, 90}

// sendEmailReminder tells the player twice: the yellow line in the chat, and
// the same words in a popup box (0x0046, ShowDialog; queued behind a box
// already open), so it is not lost in a busy chat (the owner, 2026-11-16: "make
// an ingame popup to encourage to verify email every 15 mins until they do";
// the interval is mail.remind_every).
func sendEmailReminder(sess *session.Session, line string) {
	SendColoredChat(sess, line, emailRemindRGB[0], emailRemindRGB[1], emailRemindRGB[2])
	SendDialogueBox(sess, 0, strings.ReplaceAll(line, "%", "%%"))
}

// EmailGate is the requirement at work in the world.
type EmailGate struct {
	gate *accountmail.Gate
	sm   *session.Manager
	// Now is the clock (tests set it); nil: time.Now.
	Now func() time.Time

	mu   sync.Mutex
	last map[*session.Session]time.Time
}

// NewEmailGate is the requirement over sm's sessions; nil for a nil gate.
func NewEmailGate(g *accountmail.Gate, sm *session.Manager) *EmailGate {
	if g == nil {
		return nil
	}
	return &EmailGate{gate: g, sm: sm, last: map[*session.Session]time.Time{}}
}

func (e *EmailGate) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Entered is the OnPlayerEntered hook.
func (e *EmailGate) Entered(sess *session.Session, _ *entity.Player) {
	if e == nil || sess == nil || sess.AccountID() == 0 {
		return
	}
	now := e.now()
	id := sess.AccountID()
	// once a login (Jibrish, petition 2282, the owner's choice 2026-11-17: "no more
	// than once per login"): a later world entry of the same session says nothing
	e.mu.Lock()
	_, told := e.last[sess]
	e.mu.Unlock()
	if told {
		return
	}
	if line := e.gate.Reminder(id, now); line != "" {
		sendEmailReminder(sess, line)
		e.mu.Lock()
		e.last[sess] = now
		e.mu.Unlock()
	}
}

// Remind no longer repeats the reminder (once a login, petition 5281): it
// keeps the told set to the sessions in the world and returns how many it
// told, now always 0. The cutoff still refuses the login from then.
func (e *EmailGate) Remind() int {
	if e == nil || e.sm == nil {
		return 0
	}
	now := e.now()
	if e.gate.Enforced(now) {
		return 0
	}
	type due struct {
		sess *session.Session
		line string
	}
	var told []due
	seen := map[*session.Session]bool{}
	e.sm.ForEach(func(sess *session.Session) bool {
		if !sess.InGame() || sess.AccountID() == 0 {
			return true
		}
		seen[sess] = true
		e.mu.Lock()
		_, ok := e.last[sess]
		e.mu.Unlock()
		if !ok {
			// in the world before the gate saw it (a restart's kept session):
			// it was told at its own login, so it is only remembered
			e.mu.Lock()
			e.last[sess] = now
			e.mu.Unlock()
		}
		// once a login, never on a timer (Jibrish, petition 5281: "This should
		// ONLY happen on character login, once per login"): a session told at
		// its world entry is not told again
		return true
	})
	e.mu.Lock()
	for s := range e.last {
		if !seen[s] {
			delete(e.last, s)
		}
	}
	for _, d := range told {
		e.last[d.sess] = now
	}
	e.mu.Unlock()
	for _, d := range told {
		sendEmailReminder(d.sess, d.line)
	}
	return len(told)
}

// Run reminds once a minute until the returned function is called.
func (e *EmailGate) Run() (stop func()) {
	if e == nil {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				e.Remind()
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

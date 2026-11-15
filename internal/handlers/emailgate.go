package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Every account needs a verified e-mail (the owner, 2026-11-15: "A verified
// email is REQUIRED for every account, old ones included"; mail.require_verified
// and mail.require_after, internal/accountmail's Gate). The world says it, as it
// says a temporary password (pwtemp.go): no game client shows a reason for a
// refused Station login (STATION answers every one with its own word 1394,
// "Your login failed."), so the account logs in and the world tells it.
//
//   - Before the cutoff: a line in yellow at each world entry and every
//     mail.remind_every (30 minutes) while it plays, naming the account page.
//   - From the cutoff: at world entry the line in red, why and where to go,
//     and EmailGateEject later the session is ended as a GM's eject ends one
//     (0x07A2: "You have been ejected by a server administrator."). The pages
//     stay open to it, and the next login after the link is opened plays.
//
// The gate reads this process's accounts, so a shard (its copy of the owner's,
// database.AccountMirror) holds its players to it the same.

// EmailGateEject is how long a refused account stands in the world with the
// reason before it is ejected: long enough to read it.
var EmailGateEject = 12 * time.Second

// emailRemindRGB is the reminder's colour (a GM's broadcast's yellow).
var emailRemindRGB = [3]uint8{255, 230, 90}

// EmailGate is the requirement at work in the world.
type EmailGate struct {
	gate *accountmail.Gate
	sm   *session.Manager
	// Now is the clock (tests set it); nil: time.Now.
	Now func() time.Time
	// eject ends a refused session (tests set it); nil: Eject.
	eject func(*session.Session)
	// after runs f after d (tests set it); nil: time.AfterFunc.
	after func(d time.Duration, f func())

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
	if why := e.gate.Refusal(id, now); why != "" {
		text := "Your account needs a verified e-mail address to play. Add one at " + e.gate.Page +
			" (the E-mail tab) and open the link we send you, then log in again."
		SendErrorMessage(sess, text)
		SendColoredChat(sess, text, emailRemindRGB[0], emailRemindRGB[1], emailRemindRGB[2])
		log.Printf("Accounts: account %d has no verified e-mail: told, ejected in %v", id, EmailGateEject)
		eject, after := e.eject, e.after
		if eject == nil {
			eject = Eject
		}
		if after == nil {
			after = func(d time.Duration, f func()) { time.AfterFunc(d, f) }
		}
		after(EmailGateEject, func() { eject(sess) })
		return
	}
	if line := e.gate.Reminder(id, now); line != "" {
		SendColoredChat(sess, line, emailRemindRGB[0], emailRemindRGB[1], emailRemindRGB[2])
		e.mu.Lock()
		e.last[sess] = now
		e.mu.Unlock()
	}
}

// Remind tells each player in the world whose account has no verified
// address, once every gate.Every, until the cutoff (from it on the next world
// entry is refused). It returns how many it told.
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
		last, ok := e.last[sess]
		e.mu.Unlock()
		if ok && now.Sub(last) < e.gate.Every {
			return true
		}
		if !ok {
			// in the world before the gate saw it (a restart's kept session):
			// its first line comes a whole interval on
			e.mu.Lock()
			e.last[sess] = now
			e.mu.Unlock()
			return true
		}
		if line := e.gate.Reminder(sess.AccountID(), now); line != "" {
			told = append(told, due{sess, line})
		}
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
		SendColoredChat(d.sess, d.line, emailRemindRGB[0], emailRemindRGB[1], emailRemindRGB[2])
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

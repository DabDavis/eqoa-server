package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// loginannounce.go — the GMs' announcement at login (petition 2120, the GM Jibwiz, 2026-11-14: "GM's need to be
// able to set an announce that displays at login for all players. This needs to load as the newest message in chat
// upon login, after all the other tickets closure entries and so on").
//
// One a shard, in its database (database/loginannounce.go, table loginannounce), so it outlives every restart. A GM
// sets it with #announce <text> (or /announce from a client that forwards a /command), takes it away with #announce
// off (clear, none), and #announce alone says it; the panel's set_login_announce and get_login_announce do the same.
// Each change is a gm_audit row change, which the panel reverts.
//
// Every player entering the world is sent it, read then, as the LAST line of the login. The world entry's lines go
// out in this order (HandleCharacterInWorld, then its OnPlayerEntered hooks): at once a GM's answers to the player's
// petitions (support.go's announceAnswers), a transfer applied, No Rent items gone, the rock owed, kept items, safe
// mode, hidden mercenaries, a temporary password, the mercenaries coming back; at reportNoticeAfter (15 s) the line
// on reporting a bug for a client that never answers the offer (a PS2), or at the client's answer to the offer,
// once its world has loaded, F3's line (bugreport.go); at boonNoticeAfter (20 s) the line on where the boon is
// (boon.go). The announcement goes loginAnnounceAfter (22 s) after the entry, after the boon's; and where the line
// on reporting has not gone out by then (a client that answers the offer late, its world slow to load), it waits
// for it, trying again every loginAnnounceRetry, until loginAnnounceGiveUp after the entry, when it goes anyway.
//
// It goes once a login: HandleCharacterInWorld runs the hooks at a session's first world entry alone, and a keep
// restart's session resumed (session/keep.go) is in the world already and runs none of them, so a resume is told
// nothing again (a login whose 22 s were cut by a keep restart is not told it either: the timer was the old
// process's). A world change, a death and a respawn are no login.

// The announcement's times (ours).
const (
	// loginAnnounceAfter is how long after the world entry it goes out: after the boon's line, the last one timed
	loginAnnounceAfter = boonNoticeAfter + 2*time.Second
	// loginAnnounceRetry is how soon it is tried again while the line on reporting has not gone out
	loginAnnounceRetry = 2 * time.Second
	// loginAnnounceGiveUp is how long after the world entry it stops waiting for that line and goes
	loginAnnounceGiveUp = 2 * time.Minute
)

// loginAnnounceColour is the announcement's colour (ours): a light violet no other line of the login has, read
// easily on the chat's dark ground.
var loginAnnounceColour = [3]byte{200, 160, 255}

// loginAnnouncePrefix begins the line, so it reads as the server's and not a player's.
const loginAnnouncePrefix = "Announcement: "

// LoginAnnounceLine is the announcement text as a player is sent it.
func LoginAnnounceLine(text string) string { return loginAnnouncePrefix + text }

// loginAnnounceStore is where the announcement is kept (database.SQLiteDB).
type loginAnnounceStore interface {
	LoginAnnounce() (database.LoginAnnounce, error)
	SetLoginAnnounceAudited(text string, who database.GMEdit, detail string) (database.LoginAnnounce, error)
}

// LoginAnnounces sends the announcement at each login and keeps #announce's changes.
type LoginAnnounces struct {
	store loginAnnounceStore
	now   func() time.Time
	later func(time.Duration, func()) // time.AfterFunc's; a test's own
}

// NewLoginAnnounces makes the announcement over store (nil: none is ever sent).
func NewLoginAnnounces(store loginAnnounceStore) *LoginAnnounces {
	return &LoginAnnounces{store: store, now: time.Now, later: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
}

// Entered is OnPlayerEntered: the announcement, loginAnnounceAfter on.
func (a *LoginAnnounces) Entered(sess *session.Session, _ *entity.Player) {
	if a == nil || a.store == nil || sess == nil {
		return
	}
	until := a.now().Add(loginAnnounceGiveUp)
	a.later(loginAnnounceAfter, func() { a.tell(sess, until) })
}

// tell sends the announcement to sess, if one is set and the session is still in the world, once the line on
// reporting has gone out (or until has passed).
func (a *LoginAnnounces) tell(sess *session.Session, until time.Time) {
	if !sess.CharacterInWorld() || sess.PendingTermination() {
		return
	}
	if _, replaced := sess.ReplacedFor(); replaced {
		return
	}
	if reportNotice.Load() && !sess.ReportNoticeSent() && a.now().Before(until) {
		a.later(loginAnnounceRetry, func() { a.tell(sess, until) })
		return
	}
	cur, err := a.store.LoginAnnounce()
	if err != nil {
		log.Printf("LoginAnnounce: not read: %v", err)
		return
	}
	if cur.Text == "" {
		return
	}
	SendColoredChat(sess, LoginAnnounceLine(cur.Text), loginAnnounceColour[0], loginAnnounceColour[1], loginAnnounceColour[2])
}

// CleanLoginAnnounce is text as an announcement keeps it: one line of what a PS2 can show (consoletext.go), at most
// database.LoginAnnounceMost characters. It may come back empty.
func CleanLoginAnnounce(text string) string {
	return strings.TrimSpace(consoleLine(strings.TrimSpace(text), database.LoginAnnounceMost))
}

// The lines #announce answers (ours).
const (
	announceNone    = "No login announcement is set. #announce <text> sets one; every player entering the world is sent it last."
	announceIs      = "The login announcement (set by %s, %s):"
	announceSet     = "Login announcement set. Every player entering the world is sent it last, as:"
	announceCut     = "(Cut to what a PS2 shows: letters, digits and its keyboard's marks, %d characters at most.)"
	announceCleared = "Login announcement cleared (it was: %s)."
	announceEmpty   = "#announce: nothing a PS2 can show is left of that text. Use letters, digits and its keyboard's marks."
	announceNoStore = "#announce: this server keeps no announcement."
)

// announceCommand is #announce [<text> | off | clear | none]: a GM's (gmCommands).
func (h *ChatHandler) announceCommand(sess *session.Session, gm *entity.Player, message string) {
	a := h.announces
	if a == nil || a.store == nil {
		h.gmReply(sess, announceNoStore)
		return
	}
	_, rest, _ := strings.Cut(strings.TrimSpace(message), " ")
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(rest) {
	case "":
		cur, err := a.store.LoginAnnounce()
		if err != nil {
			h.gmReply(sess, "#announce: "+err.Error())
			return
		}
		if cur.Text == "" {
			h.gmReply(sess, announceNone)
			return
		}
		h.gmReply(sess, fmt.Sprintf(announceIs, cur.SetBy, cur.Changed))
		SendColoredChat(sess, LoginAnnounceLine(cur.Text), loginAnnounceColour[0], loginAnnounceColour[1], loginAnnounceColour[2])
		return
	case "off", "clear", "none":
		old, err := a.store.SetLoginAnnounceAudited("", h.announceWho(sess), "#announce in the game: cleared")
		if err != nil {
			h.gmReply(sess, "#announce: "+err.Error())
			return
		}
		if old.Text == "" {
			h.gmReply(sess, announceNone)
			return
		}
		log.Printf("GM: account %d (%s): login announcement cleared", sess.AccountID(), gm.Name)
		h.gmReply(sess, fmt.Sprintf(announceCleared, old.Text))
		return
	}
	text := CleanLoginAnnounce(rest)
	if text == "" {
		h.gmReply(sess, announceEmpty)
		return
	}
	if _, err := a.store.SetLoginAnnounceAudited(text, h.announceWho(sess), "#announce in the game: "+text); err != nil {
		h.gmReply(sess, "#announce: "+err.Error())
		return
	}
	log.Printf("GM: account %d (%s): login announcement set: %s", sess.AccountID(), gm.Name, text)
	h.gmReply(sess, announceSet)
	SendColoredChat(sess, LoginAnnounceLine(text), loginAnnounceColour[0], loginAnnounceColour[1], loginAnnounceColour[2])
	if text != rest {
		h.gmReply(sess, fmt.Sprintf(announceCut, database.LoginAnnounceMost))
	}
}

// announceWho is sess's GM as the audit log names one acting in the game.
func (h *ChatHandler) announceWho(sess *session.Session) database.GMEdit {
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if addr := sess.RemoteAddr(); addr != nil {
		who.IP = addr.IP.String()
	}
	return who
}

package handlers

import (
	"net"

	"github.com/eqoa/server/internal/session"
)

// browserlimit.go: the most of an account's characters in the game in a browser at
// once (auth.browser_characters; the user, 2026-11-07: "lets make browser limit up to
// 4 characters per account"). The login refuses a window past it, in words the page
// shows (tcp.AuthServer.SetBrowserLimit); here is the second look, at character select,
// for windows that logged in together. CLIENT has no words for it there, so it hears
// word 676, "That character is in an area of the world that is currently unavailable",
// and can pick again. Taking back a character of its own is no new character.
var browserLimit struct {
	most    int
	relayed func(net.IP) bool
}

// SetBrowserLimit sets the limit and how to tell a relayed source (the server's start).
func SetBrowserLimit(most int, relayed func(net.IP) bool) {
	browserLimit.most, browserLimit.relayed = most, relayed
}

// browserFull reports whether a relayed session's character would be one too many
// for its account.
func browserFull(m *session.Manager, sess *session.Session, charServerID uint32) bool {
	if m == nil || browserLimit.most <= 0 || browserLimit.relayed == nil || sess.RemoteAddr() == nil || !browserLimit.relayed(sess.RemoteAddr().IP) {
		return false
	}
	if other := m.LiveHolder(charServerID, sess); other != nil && other.AccountID() == sess.AccountID() {
		return false
	}
	from := func(a *net.UDPAddr) bool { return browserLimit.relayed(a.IP) }
	return m.LiveFrom(sess.AccountID(), from, sess) >= browserLimit.most
}

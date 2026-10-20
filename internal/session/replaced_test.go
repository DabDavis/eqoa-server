package session

import (
	"net"
	"testing"
	"time"
)

// The same character logged in from a second window (two browser tabs): the
// old session is told, once, and kept to deliver it, instead of being dropped
// silently while its client carried on against a server that had forgotten it.
func TestReplacedSessionIsToldNotDropped(t *testing.T) {
	m := NewManager()
	var told []*Session
	m.SetOnReplaced(func(s *Session) { told = append(told, s) })

	ip := net.IPv4(127, 0, 0, 1)
	old := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40001}, 0x1001, 1, 0x1111, 0xB991, true)
	lobby := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40002}, 0x1002, 2, 0x2222, 0xFFFE, false)
	other := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40003}, 0x1003, 3, 0x3333, 0xB991, true)
	old.SetInGame(true)
	old.SetCharacterServerID(42)
	lobby.SetCharacterServerID(42) // not in game: nothing to tell, dropped as before
	other.SetInGame(true)
	other.SetCharacterServerID(43)

	if n := m.RemoveSessionForCharacter(42, 0x1004); n != 2 {
		t.Errorf("retired %d sessions, want 2", n)
	}
	if len(told) != 1 || told[0] != old {
		t.Fatalf("told %d sessions, want only the in-game one", len(told))
	}
	if _, ok := old.ReplacedFor(); !ok || old.PendingTermination() {
		t.Error("the in-game session should be marked replaced and kept for the grace period")
	}
	if !lobby.PendingTermination() {
		t.Error("a pre-game session holding the character should still be dropped at once")
	}
	if _, ok := other.ReplacedFor(); ok || other.PendingTermination() {
		t.Error("another character's session was touched")
	}

	// A third login of the same character does not tell the old one again.
	m.RemoveSessionForCharacter(42, 0x1005)
	if len(told) != 1 {
		t.Errorf("the replaced session was told %d times, want 1", len(told))
	}
}

// LiveLogin: an in-game session heard from lately holds its account and its
// address; one on its way out, long silent, or still in the lobby does not.
func TestLiveLogin(t *testing.T) {
	m := NewManager()
	home := net.IPv4(203, 0, 113, 7)
	s := m.CreateSession(&net.UDPAddr{IP: home, Port: 40001}, 0x1001, 1, 0x1111, 0xB991, true)
	s.SetAccountID(8)
	s.UpdateActivity()

	if got := m.LiveLogin(8, nil, nil); got != "" {
		t.Errorf("a lobby session counted as in the game: %q", got)
	}
	s.SetInGame(true)
	if got := m.LiveLogin(8, net.IPv4(198, 51, 100, 1), nil); got != "account" {
		t.Errorf("same account: %q, want account", got)
	}
	if got := m.LiveLogin(9, home, nil); got != "address" {
		t.Errorf("same address: %q, want address", got)
	}
	if got := m.LiveLogin(9, nil, nil); got != "" {
		t.Errorf("address not checked (exempt): %q", got)
	}
	// a relayed session: its source is the relay, ipOf names the page
	page := net.IPv4(192, 0, 2, 44)
	viaRelay := func(a *net.UDPAddr) net.IP {
		if a.IP.Equal(home) {
			return page
		}
		return a.IP
	}
	if got := m.LiveLogin(9, page, viaRelay); got != "address" {
		t.Errorf("relayed, same page address: %q, want address", got)
	}
	if got := m.LiveLogin(9, home, viaRelay); got != "" {
		t.Errorf("relayed: the relay's own address matched: %q", got)
	}
	if got := m.LiveLogin(9, page, func(*net.UDPAddr) net.IP { return nil }); got != "" {
		t.Errorf("an unknown session address matched: %q", got)
	}
	if got := m.LiveLogin(9, net.IPv4(198, 51, 100, 1), nil); got != "" {
		t.Errorf("another account from another address: %q", got)
	}

	s.mu.Lock()
	s.lastActivity = time.Now().Add(-LiveWithin - time.Second)
	s.mu.Unlock()
	if got := m.LiveLogin(8, home, nil); got != "" {
		t.Errorf("a session silent past LiveWithin still holds its login: %q", got)
	}
	s.UpdateActivity()
	s.MarkReplaced()
	if got := m.LiveLogin(8, home, nil); got != "" {
		t.Errorf("a replaced session still holds its login: %q", got)
	}
}

// EndAccount (the page's "log out the other window"): the account's in-game
// session is told and kept to hear it, its lobby session dropped, another
// account's left alone; told once.
func TestEndAccount(t *testing.T) {
	m := NewManager()
	var told []*Session
	m.SetOnReplaced(func(s *Session) { told = append(told, s) })
	ip := net.IPv4(127, 0, 0, 1)
	game := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40001}, 0x1001, 1, 0x1111, 0xB991, true)
	lobby := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40002}, 0x1002, 2, 0x2222, 0xFFFE, false)
	other := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40003}, 0x1003, 3, 0x3333, 0xB991, true)
	game.SetAccountID(8)
	game.SetInGame(true)
	lobby.SetAccountID(8)
	other.SetAccountID(9)
	other.SetInGame(true)

	if n := m.EndAccount(8); n != 1 || len(told) != 1 || told[0] != game {
		t.Fatalf("ended %d, told %d", n, len(told))
	}
	if !lobby.PendingTermination() {
		t.Error("the lobby session was kept")
	}
	if _, r := other.ReplacedFor(); r || other.PendingTermination() {
		t.Error("another account's session was touched")
	}
	if m.EndAccount(8); len(told) != 1 {
		t.Error("told twice")
	}
	if m.LiveLogin(8, nil, nil) != "" {
		t.Error("the account still holds its login after it was logged out")
	}
}

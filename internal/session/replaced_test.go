package session

import (
	"net"
	"testing"
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

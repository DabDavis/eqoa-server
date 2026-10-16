package session

import (
	"net"
	"testing"
	"time"
)

// Two players behind one address log in at the same moment: a new session
// must not reap the other's login in progress, only a lobby session an
// earlier attempt abandoned.
func TestStaleLobbyReapSparesALiveLoginFromTheSameIP(t *testing.T) {
	m := NewManager()
	ip := net.IPv4(127, 0, 0, 1)
	live := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40001}, 0x1001, 1, 0x1111, 0xFFFE, false)
	stale := m.CreateSession(&net.UDPAddr{IP: ip, Port: 40002}, 0x1002, 2, 0x2222, 0xFFFE, false)
	other := m.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 40003}, 0x1003, 3, 0x3333, 0xFFFE, false)

	live.UpdateActivity()
	stale.mu.Lock()
	stale.lastActivity = time.Now().Add(-staleLobbyIdle - time.Second)
	stale.mu.Unlock()
	other.mu.Lock()
	other.lastActivity = time.Now().Add(-time.Hour)
	other.mu.Unlock()

	if n := m.RemoveStaleGameSessions("127.0.0.1", 0x1004); n != 1 {
		t.Errorf("flagged %d sessions, want 1 (the silent one)", n)
	}
	if live.PendingTermination() {
		t.Error("a login still exchanging packets was reaped")
	}
	if !stale.PendingTermination() {
		t.Error("a lobby session silent past staleLobbyIdle was kept")
	}
	if other.PendingTermination() {
		t.Error("a session from another address was reaped")
	}
}

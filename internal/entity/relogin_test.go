package entity

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The same character logs in again while its old session is still alive; the
// old session then times out. Its cleanup must not remove the new session's
// player: before the fix it did (both are keyed by ServerID), and every later
// position update for the live session went nowhere.
func TestOldSessionTimeoutKeepsTheNewLogin(t *testing.T) {
	w := NewWorld()
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10070}
	oldSess := session.NewSession(addr, 1, 1, 1, 1, true)
	newSess := session.NewSession(addr, 2, 2, 2, 2, true)
	rec := func() *database.CharacterRecord {
		return &database.CharacterRecord{ServerID: 8, CharName: "Twice", Level: 1, X: 100, Y: 54, Z: 100}
	}
	w.CreatePlayer(oldSess, rec())
	p := w.CreatePlayer(newSess, rec())
	if p == nil {
		t.Fatal("no player for the new login")
	}
	w.RemovePlayerBySession(oldSess)
	if got, ok := w.Players[EntityID(8)]; !ok || got != p {
		t.Fatal("the old session's timeout removed the new login's player")
	}
	w.UpdatePosition(EntityID(8), 200, 54, 300, 0)
	if p.X != 200 || p.Z != 300 {
		t.Errorf("the live player did not move: (%.0f, %.0f)", p.X, p.Z)
	}
	// Its own session ending still removes it.
	w.RemovePlayerBySession(newSess)
	if _, ok := w.Players[EntityID(8)]; ok {
		t.Error("the live session's own logout left its player behind")
	}
}

// A character in a group logs in again while its old session lives: the new
// Player is in the group the old one was (the roster named it all along), so
// the group's members, its chat and its voice find it. The controls: a
// character in no group comes back in none, and one who left first is not
// put back.
func TestALoginAgainKeepsTheGroup(t *testing.T) {
	w := NewWorld()
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10070}
	mk := func(n uint32) *session.Session { return session.NewSession(addr, n, n, uint16(n), uint16(n), true) }
	rec := func(id uint32, name string) *database.CharacterRecord {
		return &database.CharacterRecord{ServerID: id, CharName: name, Level: 20, X: 100, Y: 54, Z: 100}
	}
	a := w.CreatePlayer(mk(1), rec(8, "Voicea"))
	b := w.CreatePlayer(mk(2), rec(9, "Voiceb"))
	loner := w.CreatePlayer(mk(3), rec(10, "Loner"))
	w.groups = map[uint32]*Group{4: {ID: 4, Members: []EntityID{a.ServerID, b.ServerID}, lastRoster: map[EntityID]string{9: "sent"}}}
	a.GroupID, b.GroupID = 4, 4
	b2 := w.CreatePlayer(mk(4), rec(9, "Voiceb"))
	if b2 == b || b2.GroupID != 4 {
		t.Fatalf("the new login's group %d (want 4)", b2.GroupID)
	}
	if got := w.GroupMembers(b2); len(got) != 2 || got[0] != a || got[1] != b2 {
		t.Fatalf("its group's members %v", got)
	}
	if _, kept := w.groups[4].lastRoster[9]; kept {
		t.Fatal("the new session is not sent the roster again")
	}
	if l2 := w.CreatePlayer(mk(5), rec(10, "Loner")); l2 == loner || l2.GroupID != 0 {
		t.Fatalf("the control: a character of no group came back in group %d", l2.GroupID)
	}
	w.mu.Lock()
	w.leaveGroupLocked(a)
	w.mu.Unlock()
	if a2 := w.CreatePlayer(mk(6), rec(8, "Voicea")); a2.GroupID != 0 {
		t.Fatalf("the control: one who left came back in group %d", a2.GroupID)
	}
}

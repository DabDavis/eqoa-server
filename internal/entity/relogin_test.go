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

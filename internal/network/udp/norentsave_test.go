package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The logout save keeps when the character left the world, by its row, for
// its No Rent items (handlers/norent.go); a shutdown's SaveAll keeps it too.
func TestTheLogoutSaveKeepsTheLogoutTime(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/logout.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Leaver", AccountID: 1, Level: 12}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 7000}, 1, 1, 1, 2, true)
	w.CreatePlayer(sess, rec)
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)

	at := time.Unix(1_800_000_000, 0)
	l.saveLogout(sess, at)
	if got, ok, err := store.TakeLogout(rec.ServerID); err != nil || !ok || !got.Equal(at) {
		t.Fatalf("kept %v (%v, %v), want %v", got, ok, err, at)
	}

	sess.SetCharacterInWorld(true)
	before := time.Now().Add(-time.Second)
	l.SaveAll()
	if got, ok, err := store.TakeLogout(rec.ServerID); err != nil || !ok || got.Before(before.Truncate(time.Second)) {
		t.Errorf("the shutdown's save kept %v (%v, %v)", got, ok, err)
	}
}

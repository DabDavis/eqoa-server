package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A session's end and a shutdown's save keep when the character left the world (charlogout, for its No Rent
// items: handlers/norent.go); a session whose character a newer login took keeps nothing, as its other saves.
func TestTheLogoutAndShutdownKeepWhenTheCharacterLeft(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/logout.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Leaver", AccountID: 1, Level: 20}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 7000}, 1, 1, 1, 2, true)
	w.CreatePlayer(sess, rec)
	sess.SetCharacterInWorld(true)
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)

	at := time.Unix(1_800_000_000, 0)
	l.saveLogout(sess, at)
	if got, ok, err := db.TakeLogout(rec.ServerID); err != nil || !ok || !got.Equal(at) {
		t.Fatalf("the logout's save: %v %v (%v)", got, ok, err)
	}

	before := time.Now().Add(-time.Second)
	l.SaveAll()
	if got, ok, _ := db.TakeLogout(rec.ServerID); !ok || got.Before(before.Truncate(time.Second)) {
		t.Errorf("the shutdown's save: %v %v", got, ok)
	}

	// the control: a newer login has the character, the old session saves nothing
	newer := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 10), Port: 7000}, 2, 2, 2, 3, true)
	w.CreatePlayer(newer, rec)
	l.saveLogout(sess, at)
	if _, ok, _ := db.TakeLogout(rec.ServerID); ok {
		t.Error("an old session's player kept a logout though its character is in the world")
	}
}

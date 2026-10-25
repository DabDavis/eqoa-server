package udp

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The logout save keeps the level and the experience into it with the place:
// a character that logged in at level 1 and reached 30, 1,234 into it, comes
// back at 30, 1,234 in. Dead, it keeps its level and the place it last
// logged out alive. Until 2026-10-25 only the place was written, and every
// level gained or set was gone at the next login.
func TestTheLogoutSaveKeepsTheLevel(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/logout.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Leveller", AccountID: 1, Level: 1, X: 25300, Y: 54, Z: 15700}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(rec); err != nil { // Create keeps no place
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 7000}, 1, 1, 1, 2, true)
	loaded, err := store.GetByServerID(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	p := w.CreatePlayer(sess, loaded)
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)

	p.Level, p.XPInLevel = 30, 1234
	p.X, p.Z = 25310, 15710
	l.savePlayerPosition(sess)
	got, err := store.GetByServerID(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Level != 30 || got.TotalXP != entity.TotalXP(30, 1234) || got.X != 25310 || got.Z != 15710 {
		t.Fatalf("saved level %d, %d xp at (%v, %v); want 30, %d at (25310, 15710)",
			got.Level, got.TotalXP, got.X, got.Z, entity.TotalXP(30, 1234))
	}

	// dead: the level kept, the place not
	p.Level, p.XPInLevel, p.HP = 31, 5, 0
	p.X, p.Z = 4768, 4500
	l.savePlayerPosition(sess)
	if got, _ = store.GetByServerID(rec.ServerID); got.Level != 31 || got.TotalXP != entity.TotalXP(31, 5) || got.X != 25310 {
		t.Fatalf("saved dead: level %d, %d xp at x %v; want 31, %d at x 25310", got.Level, got.TotalXP, got.X, entity.TotalXP(31, 5))
	}

	// and the next login is at that level, that far into it
	again := w.CreatePlayer(sess, got)
	if again.Level != 31 || again.XPInLevel != 5 {
		t.Fatalf("logged in again at level %d, %d in", again.Level, again.XPInLevel)
	}
}

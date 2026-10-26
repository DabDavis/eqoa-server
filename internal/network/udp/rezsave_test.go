package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The logout save keeps the chance of a rez (entity/resurrect.go) by the
// character's row, what is left of its time; a chance spent is kept as none.
func TestTheLogoutSaveKeepsTheChanceOfARez(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/rez.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Fallen", AccountID: 1, Level: 49}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 8), Port: 7000}, 1, 1, 1, 2, true)
	w.CreatePlayer(sess, rec)
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)

	w.RestoreRez(sess, database.RezRow{Debt: 6137969, LeftMS: (25 * time.Minute).Milliseconds()}, time.Now())
	l.saveRez(sess)
	kept, err := store.LoadRez(rec.ServerID)
	if err != nil || kept.Debt != 6137969 || kept.LeftMS > (25*time.Minute).Milliseconds() || kept.LeftMS < (25*time.Minute-10*time.Second).Milliseconds() {
		t.Fatalf("kept %+v (%v), want 6137969 with 25 minutes", kept, err)
	}

	w.RestoreRez(sess, database.RezRow{}, time.Now()) // spent
	l.saveRez(sess)
	if kept, err = store.LoadRez(rec.ServerID); err != nil || kept != (database.RezRow{}) {
		t.Errorf("a spent chance kept as %+v (%v)", kept, err)
	}
}

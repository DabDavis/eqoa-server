package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A login that replaces a session still in the world takes that session's
// chance of a rez with its debt (entity/resurrect.go): kept in charrez, then
// put back on the new player, the time left as it was. The control: a
// character with no chance kept logs in with none.
func TestALoginKeepsTheChanceOfARez(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/rez.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Fallen", AccountID: 1, Level: 49, Debt: 6137969}
	plain := &database.CharacterRecord{CharName: "Plain", AccountID: 1, Level: 49}
	for _, r := range []*database.CharacterRecord{rec, plain} {
		if err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	old := sm.CreateSession(testAddr(1), 1, 1, 1, 2, true)
	w.CreatePlayer(old, rec)
	// the old window's character died at 49 eleven minutes ago
	if !w.RestoreRez(old, database.RezRow{Debt: 6137969, LeftMS: (19 * time.Minute).Milliseconds()}, time.Now()) {
		t.Fatal("no chance on the old window's player")
	}

	enter := func(id uint32, n byte) *entity.Player {
		s := sm.CreateSession(testAddr(n), uint32(n), uint32(n), 2, 2, true)
		s.SetAccountID(1)
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
		msg.SetData(u32b(id))
		if err := h.HandleSelectedChar(s, msg); err != nil {
			t.Fatal(err)
		}
		return w.SessionPlayer(s)
	}
	np := enter(rec.ServerID, 2)
	if kept, err := store.LoadRez(rec.ServerID); err != nil || kept.Debt != 6137969 {
		t.Fatalf("charrez after the replacing login: %+v (%v)", kept, err)
	}
	debt, left, ok := w.RezOf(np.ServerID, time.Now())
	if !ok || debt != 6137969 || left > 19*time.Minute || left < 19*time.Minute-10*time.Second || np.Debt != 6137969 {
		t.Fatalf("the new player's chance %d, %v left (%v), debt %d; want 6137969, 19m", debt, left, ok, np.Debt)
	}

	pp := enter(plain.ServerID, 3)
	if debt, _, ok := w.RezOf(pp.ServerID, time.Now()); !ok || debt != 0 {
		t.Errorf("the control: a character with no chance kept has %d", debt)
	}
}

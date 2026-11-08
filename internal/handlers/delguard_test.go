package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A delete (0x002D) never takes a character still in the game (the user,
// 2026-11-08: "add the delete guard for characters in another window"; Nuclear
// was deleted from a second window while the first played it): one another
// window plays, and one whose Player is left in the world with no live
// session (a client that dropped), are refused with word 676's notice and the
// list sent again, the row kept. The controls: each is deleted once it is out
// of the world.
func TestADeleteRefusesACharacterStillInTheWorld(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/del.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	mk := func(name string) *database.CharacterRecord {
		rec := &database.CharacterRecord{CharName: name, AccountID: 1, Level: 1}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	played, left := mk("Played"), mk("Left")
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}

	// window 1 plays Played
	win1 := sm.CreateSession(testAddr(1), 1, 1, 1, 2, true)
	win1.SetAccountID(1)
	pp := w.CreatePlayer(win1, played)
	win1.SetCharacterServerID(played.ServerID)
	win1.SetInGame(true)
	win1.SetCharacterInWorld(true)
	// Left's Player is still in the world, its session out of the game
	gone := sm.CreateSession(testAddr(2), 2, 2, 2, 2, true)
	gone.SetAccountID(1)
	lp := w.CreatePlayer(gone, left)
	// window 2, at character select
	win2 := sm.CreateSession(testAddr(3), 3, 3, 3, 2, true)
	win2.SetAccountID(1)

	del := func(rec *database.CharacterRecord) {
		t.Helper()
		b := make([]byte, 16)
		wr := buffer.NewWriter(b)
		wr.Write7BitEncodedInt64(int64(rec.ServerID))
		win2.Queue().GetPending()
		if err := h.HandleDelCharacter(win2, incoming(protocol.OpcodeDelCharacter, b[:wr.Position()])); err != nil {
			t.Fatal(err)
		}
	}
	there := func(rec *database.CharacterRecord) bool {
		r, err := store.GetByServerID(rec.ServerID)
		return err == nil && r != nil
	}
	for _, rec := range []*database.CharacterRecord{played, left} {
		del(rec)
		if !there(rec) {
			t.Fatalf("%s was deleted while still in the world", rec.CharName)
		}
		if sentOp(win2, protocol.OpcodeCharSelectAreaUnavailable) == nil || sentOp(win2, protocol.OpcodeCharacterSelect) == nil {
			t.Errorf("%s's refusal: no notice or no list", rec.CharName)
		}
	}

	w.RemovePlayer(lp.ServerID)
	del(left)
	if there(left) {
		t.Error("Left, out of the world: not deleted")
	}
	w.RemovePlayer(pp.ServerID)
	win1.SetInGame(false)
	win1.SetCharacterInWorld(false)
	del(played)
	if there(played) {
		t.Error("Played, its window gone: not deleted")
	}
}

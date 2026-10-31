package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// CLIENT's Quit (0x09B0) with a creature on the character leaves the session
// to stay while it is fought (entity/linkdead.go); with none, the session
// ends at the next tick, as before.
func TestAQuitInAFightStays(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/quit.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := entity.NewWorld()
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), nil, nil, database.NewSpellbookStore(db))
	h.SetGameWorld(w)
	quit := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDisconnectClient)

	sess := testSession()
	sess.SetCharacterInWorld(true)
	p := &entity.Player{ServerID: 1001, Name: "Jedd", HP: 40, MaxHP: 300, X: 1000, Z: 1000}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	bear := &entity.NPC{ServerID: 3000, Name: "grizzly bear", HP: 900, MaxHP: 900, ModelID: 0x1234, X: 1003, Z: 1000,
		Threats: map[entity.EntityID]int64{p.ServerID: 10}, CurrentEnemy: p.ServerID}
	w.NPCs[bear.ServerID] = bear
	if err := h.HandleLogout(sess, quit); err != nil {
		t.Fatal(err)
	}
	if _, quitting := sess.QuittingSince(); !quitting || sess.PendingTermination() {
		t.Errorf("a quit in a fight: quitting %v, ending %v; want it to stay", quitting, sess.PendingTermination())
	}

	// the control: nothing on it
	bear.Threats, bear.CurrentEnemy = nil, 0
	calm := testSession()
	calm.SetCharacterInWorld(true)
	q := &entity.Player{ServerID: 1002, Name: "Calm", HP: 300, MaxHP: 300}
	w.Players[q.ServerID] = q
	calm.SetECSEntity(q)
	if err := h.HandleLogout(calm, quit); err != nil {
		t.Fatal(err)
	}
	if _, quitting := calm.QuittingSince(); quitting || !calm.PendingTermination() {
		t.Errorf("a quit out of a fight: quitting %v, ending %v; want it to end", quitting, calm.PendingTermination())
	}
}

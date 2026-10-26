package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// F3 20261026-014527 (jib: "any action other than movement, targeting and
// chat should break invis"): through the Registry, an action's sender loses
// its invisibility before the action is handled; targeting and chat leave it
// (the control).
func TestActionsBreakInvisibility(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	invisible := func() bool {
		for _, e := range f.player.Effects {
			if e.Invis {
				return true
			}
		}
		return false
	}
	hide := func() {
		f.player.Effects = append(f.player.Effects, &entity.ActiveEffect{SpellID: 9000, Name: "Invisibility",
			Invis: true, Until: time.Now().Add(time.Minute)})
	}
	r := NewRegistry()
	r.SetInvisBreaker(w.BreakInvis)
	handled := 0
	noop := func(*session.Session, *protocol.Message) error { handled++; return nil }
	for _, op := range []protocol.GameOpcode{protocol.OpcodeTarget, protocol.OpcodeClientSayChat,
		protocol.OpcodeInteractItem, protocol.OpcodeInteract, protocol.OpcodeEmoteWave} {
		r.Register(op, noop)
	}
	send := func(op protocol.GameOpcode) {
		if err := r.Handle(f.sess, protocol.NewMessage(protocol.MessageTypeReliable, op)); err != nil {
			t.Fatal(err)
		}
	}

	hide()
	send(protocol.OpcodeTarget)
	send(protocol.OpcodeClientSayChat)
	if !invisible() {
		t.Fatal("targeting or chat ended the invisibility")
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeInteractItem, protocol.OpcodeInteract, protocol.OpcodeEmoteWave} {
		if !invisible() {
			hide()
		}
		send(op)
		if invisible() {
			t.Errorf("0x%04X (%s) left the player invisible", uint16(op), invisActions[op])
		}
	}
	if handled != 5 {
		t.Errorf("%d messages handled, want 5: the action is still done", handled)
	}
	// a player who is not invisible is left as he is (nothing else removed)
	f.player.Effects = []*entity.ActiveEffect{{SpellID: 9001, Name: "Courage", Until: time.Now().Add(time.Minute)}}
	send(protocol.OpcodeInteractItem)
	if len(f.player.Effects) != 1 {
		t.Errorf("an action took a buff off: %+v", f.player.Effects)
	}
}

// Every action is a message the server handles (a typo'd opcode would never
// break anything), and none is movement, targeting or chat.
func TestInvisActionsAreHandled(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/actions.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewRegistry()
	RegisterDefaultHandlers(r, database.NewSQLiteCharacterStore(db), database.NewSQLiteAccountStore(db),
		session.NewManager(), "127.0.0.1", entity.NewWorld(), database.NewInventoryStore(db), database.NewSpellbookStore(db))
	for op, why := range invisActions {
		if _, ok := r.Get(op); !ok {
			t.Errorf("0x%04X (%s) has no handler", uint16(op), why)
		}
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeTarget, protocol.OpcodeClassicTargetInfo,
		protocol.OpcodeClientSayChat, protocol.OpcodeClientShout, protocol.OpcodeAssist, protocol.OpcodeAssistTarget} {
		if _, ok := invisActions[op]; ok {
			t.Errorf("0x%04X is targeting or chat, yet ends invisibility", uint16(op))
		}
	}
}

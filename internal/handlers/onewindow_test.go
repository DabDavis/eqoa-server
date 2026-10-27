package handlers

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// One window a character: two windows of one account at one address both
// play, each on its own character. A character already in the game in one
// window is refused to the other with CLIENT's character-select notice
// (0x07F0, word 676), not the login's 0x0729, and that window stays at
// character select and enters as the account's other character; the first
// window keeps playing. The control: once the first window is on its way out
// (replaced), its character is free to a new window again.
func TestOneWindowACharacter(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/windows.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	one := &database.CharacterRecord{CharName: "Boxone", AccountID: 7, Level: 1}
	two := &database.CharacterRecord{CharName: "Boxtwo", AccountID: 7, Level: 1}
	for _, r := range []*database.CharacterRecord{one, two} {
		if err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	window := func(port int, instance uint32) *session.Session {
		s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: port}, instance, instance, uint16(instance), 2, true)
		s.SetAccountID(7)
		s.UpdateActivity()
		return s
	}
	sel := func(sess *session.Session, id uint32) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
		msg.SetData(u32b(id))
		if err := h.HandleSelectedChar(sess, msg); err != nil {
			t.Fatal(err)
		}
	}
	told := func(sess *session.Session) bool {
		return sentOp(sess, protocol.OpcodeCharSelectAreaUnavailable) != nil
	}

	first := window(6201, 21)
	sel(first, one.ServerID)
	if told(first) || w.SessionPlayer(first) == nil {
		t.Fatal("setup: the first window did not enter the world")
	}

	second := window(6202, 22)
	sel(second, one.ServerID)
	if !told(second) || w.SessionPlayer(second) != nil || second.InGame() {
		t.Fatalf("the first window's character entered a second window (told %v, in game %v)", told(second), second.InGame())
	}
	if sentOp(second, protocol.OpcodeBadLoginPassword) != nil {
		t.Error("the refusal sent the window back to its login")
	}
	if _, r := first.ReplacedFor(); r || w.SessionPlayer(first) == nil {
		t.Fatal("the first window lost its character")
	}

	sel(second, two.ServerID)
	if w.SessionPlayer(second) == nil {
		t.Fatal("the account's other character did not enter beside the first")
	}
	if _, r := first.ReplacedFor(); r || w.SessionPlayer(first) == nil {
		t.Fatal("the second character replaced the first window")
	}

	// the control: the first window on its way out frees its character
	first.MarkReplaced()
	third := window(6203, 23)
	sel(third, one.ServerID)
	if told(third) || w.SessionPlayer(third) == nil {
		t.Fatal("the control: a replaced window still held its character")
	}
}

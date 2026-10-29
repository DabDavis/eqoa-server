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
// play, each on its own character. A third window of the account choosing a
// character already in the game TAKES IT OVER (entity/takeover.go): the
// window that had it is told it logged in elsewhere (0x07A4) and is on its
// way out, the character is in one window still, the third's, and the
// account's other window plays on. Until 2026-10-29 the third was refused
// (0x07F0) while the first was heard from. The control: a live window of
// ANOTHER account holding the character (moved between accounts as it
// played) still refuses it with 0x07F0, and that window keeps it.
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
	sm.SetOnReplaced(SendLoggedInElsewhere)
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	window := func(port int, instance uint32, account int) *session.Session {
		s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: port}, instance, instance, uint16(instance), 2, true)
		s.SetAccountID(account)
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
	refused := func(sess *session.Session) bool {
		return sentOp(sess, protocol.OpcodeCharSelectAreaUnavailable) != nil
	}
	replaced := func(sess *session.Session) bool {
		_, r := sess.ReplacedFor()
		return r
	}

	first := window(6201, 21, 7)
	sel(first, one.ServerID)
	p := w.SessionPlayer(first)
	if refused(first) || p == nil {
		t.Fatal("setup: the first window did not enter the world")
	}
	second := window(6202, 22, 7)
	sel(second, two.ServerID)
	if w.SessionPlayer(second) == nil || replaced(first) {
		t.Fatal("the account's other character did not enter beside the first")
	}

	third := window(6203, 23, 7)
	sel(third, one.ServerID)
	if refused(third) || w.SessionPlayer(third) != p || !third.InGame() {
		t.Fatalf("the third window did not take the first's character over (refused %v, its player %p, the live one %p)",
			refused(third), w.SessionPlayer(third), p)
	}
	if !replaced(first) || sentOp(first, protocol.OpcodeLoggedInFromAnotherLocationDC) == nil || w.SessionPlayer(first) != nil {
		t.Fatal("the first window was not told it logged in elsewhere, or still has the character")
	}
	if sentOp(first, protocol.OpcodeBadLoginPassword) != nil || sentOp(third, protocol.OpcodeBadLoginPassword) != nil {
		t.Error("a window was sent back to its login")
	}
	if replaced(second) || w.SessionPlayer(second) == nil {
		t.Fatal("the takeover ended the account's other window")
	}

	// the control: the character moved to account 9 while account 7's
	// window plays it; account 9's window is refused, as every window was
	// before, and account 7's keeps it
	if _, err := db.Exec(`UPDATE characters SET accountid = 9 WHERE serverid = ?`, one.ServerID); err != nil {
		t.Fatal(err)
	}
	fourth := window(6204, 24, 9)
	sel(fourth, one.ServerID)
	if !refused(fourth) || w.SessionPlayer(fourth) != nil || fourth.InGame() {
		t.Fatal("the control: another account's live window lost its character to a new one")
	}
	if replaced(third) || w.SessionPlayer(third) != p {
		t.Fatal("the control: the refused window took the character all the same")
	}
}

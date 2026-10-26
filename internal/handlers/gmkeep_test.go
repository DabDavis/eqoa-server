package handlers

import (
	"bytes"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// gmRows gives the chat world's GM (character 1) and player (2) rows in a
// temporary database, and hands the chat handler its stores.
func gmRows(t *testing.T, c *chatWorld) (*database.SQLiteCharacterStore, *database.InventoryStore) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/gm.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	for _, name := range []string{"Gm", "Player"} {
		if err := store.Create(&database.CharacterRecord{CharName: name, AccountID: 1, Level: 1}); err != nil {
			t.Fatal(err)
		}
	}
	inv := database.NewInventoryStore(db)
	c.h.SetStores(store, inv)
	return store, inv
}

// sentOp is the payload of the last message of op queued for s, opcode first.
func sentOp(s *session.Session, op protocol.GameOpcode) []byte {
	var last []byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && opcode(d) == op {
			last = d
		}
	}
	return last
}

// A GM's #setlevel and #tunar are kept: the row says so at once, so the next
// login has them (they were the live Player's alone: Jibrish 60 in the game
// and 1 in the database, F3 pass 13), and the client is told its level
// (0x0020). The control: without the stores the change is the game's alone,
// as it was.
func TestAGMsLevelAndPurseAreKept(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	store, _ := gmRows(t, c)

	c.say(gs, "#setlevel 37")
	row, err := store.GetByServerID(1)
	if err != nil {
		t.Fatal(err)
	}
	if gp.Level != 37 || row.Level != 37 || row.TotalXP != entity.XPBase(37) {
		t.Fatalf("#setlevel 37: the game %d, the row %d with %d xp", gp.Level, row.Level, row.TotalXP)
	}
	if d := sentOp(gs, protocol.OpcodeGrantXP); len(d) < 3 || d[2] != 37*2 { // zigzag
		t.Fatalf("0x0020 not sent with level 37: %x", d)
	}
	c.say(gs, "#tunar 4321")
	if row, _ = store.GetByServerID(1); gp.Tunar != 4321 || row.Tunar != 4321 {
		t.Fatalf("#tunar 4321: the game %d, the row %d", gp.Tunar, row.Tunar)
	}
	if other, _ := store.GetByServerID(2); other.Level != 1 || other.Tunar != 0 {
		t.Fatalf("the other character's row changed: level %d, tunar %d", other.Level, other.Tunar)
	}

	c.h.SetStores(nil, nil)
	c.say(gs, "#setlevel 12")
	if row, _ = store.GetByServerID(1); gp.Level != 12 || row.Level != 37 {
		t.Fatalf("without stores: the game %d, the row %d", gp.Level, row.Level)
	}
}

// A login that replaces a session still in the world (a closed tab) takes
// that session's level with its place: a character that reached 30 in the
// old window, dead or alive, comes in at 30, and the row says so.
func TestAReplacingLoginKeepsTheLevelReached(t *testing.T) {
	for _, dead := range []bool{false, true} {
		db, err := database.NewSQLite(t.TempDir() + "/relogin.db")
		if err != nil {
			t.Fatal(err)
		}
		store := database.NewSQLiteCharacterStore(db)
		rec := &database.CharacterRecord{CharName: "Window", AccountID: 1, Level: 1}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		w := entity.NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
		old := sm.CreateSession(testAddr(1), 1, 1, 1, 2, true)
		p := w.CreatePlayer(old, rec)
		p.Level, p.XPInLevel, p.X, p.Z = 30, 99, 25310, 15710
		if dead {
			p.HP = 0
		}

		now := sm.CreateSession(testAddr(2), 2, 2, 2, 2, true)
		now.SetAccountID(1) // signed in as the character's account
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
		msg.SetData(u32b(rec.ServerID))
		if err := h.HandleSelectedChar(now, msg); err != nil {
			t.Fatal(err)
		}
		row, _ := store.GetByServerID(rec.ServerID)
		np := w.SessionPlayer(now)
		if row.Level != 30 || row.TotalXP != entity.TotalXP(30, 99) || np == nil || np.Level != 30 || np.XPInLevel != 99 {
			t.Fatalf("dead %v: the row level %d (%d xp), the new player %+v", dead, row.Level, row.TotalXP, np)
		}
		db.Close()
	}
}

func testAddr(n byte) *net.UDPAddr {
	return &net.UDPAddr{IP: net.IPv4(198, 51, 100, n), Port: 6100 + int(n)}
}

// A session enters the world only as a character of its own account, and
// makes and lists only its own (F3 20261026-103415: a session the server had
// dropped, made anew with no account by the client's next packet, was sent
// every character on the server and made one under account 1, and select
// asked no one whose a character was). Each refusal is the login's 0x0729,
// so the client goes back to its login. The control: the owner's session
// enters the world, and its list holds its own character only.
func TestOnlyACharactersOwnAccountMayUseIt(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/owner.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	mine := &database.CharacterRecord{CharName: "Mine", AccountID: 5, Level: 1}
	theirs := &database.CharacterRecord{CharName: "Theirs", AccountID: 6, Level: 1}
	for _, r := range []*database.CharacterRecord{mine, theirs} {
		if err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	sel := func(sess *session.Session, id uint32) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
		msg.SetData(u32b(id))
		if err := h.HandleSelectedChar(sess, msg); err != nil {
			t.Fatal(err)
		}
	}
	refused := func(sess *session.Session) bool { return sentOp(sess, protocol.OpcodeBadLoginPassword) != nil }

	other := sm.CreateSession(testAddr(3), 3, 3, 3, 3, true)
	other.SetAccountID(5)
	sel(other, theirs.ServerID)
	if !refused(other) || w.SessionPlayer(other) != nil {
		t.Fatal("a session entered the world as another account's character")
	}
	none := sm.CreateSession(testAddr(4), 4, 4, 4, 4, true)
	sel(none, mine.ServerID)
	if !refused(none) || w.SessionPlayer(none) != nil {
		t.Fatal("a session with no account entered the world")
	}
	list := sm.CreateSession(testAddr(5), 5, 5, 5, 5, true)
	h.SendCharacterSelectEQOA(list)
	if !refused(list) || sentOp(list, protocol.OpcodeCharacterSelect) != nil {
		t.Fatal("a session with no account was sent a character list")
	}

	own := sm.CreateSession(testAddr(6), 6, 6, 6, 6, true)
	own.SetAccountID(5)
	h.SendCharacterSelectEQOA(own)
	if d := sentOp(own, protocol.OpcodeCharacterSelect); d == nil || !bytes.Contains(d, []byte("Mine")) || bytes.Contains(d, []byte("Theirs")) {
		t.Fatalf("the control: the owner's list %q", d)
	}
	sel(own, mine.ServerID)
	if refused(own) || w.SessionPlayer(own) == nil {
		t.Fatal("the control: the owner's session did not enter the world")
	}
}

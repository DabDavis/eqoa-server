package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// 0x0012 is read by the name page as VIMemStream::ReadString(name, 24): a u32
// length and the bytes (decomp viclient.cpp). It was a 7-bit length.
func TestRandomNameIsAReadString(t *testing.T) {
	c := newChatWorld(t)
	sess, _ := c.join(1, "Alice", 25000, 16000, 0)
	h := &CharacterHandler{}
	for i := 0; i < 20; i++ {
		before := len(sess.Queue().PeekPending())
		if err := h.HandleRandomName(sess, incoming(protocol.OpcodeRandomName, []byte{0, 2})); err != nil {
			t.Fatal(err)
		}
		msgs := sess.Queue().PeekPending()
		if len(msgs) != before+1 {
			t.Fatal("no answer")
		}
		d := msgs[len(msgs)-1].Data()
		op, n := binary.LittleEndian.Uint16(d), binary.LittleEndian.Uint32(d[2:])
		if op != uint16(protocol.OpcodeRandomName) || int(n) != len(d)-6 || n < 3 || n >= 24 {
			t.Fatalf("op %#x length %d of %d bytes: % x", op, n, len(d), d)
		}
		if !h.isValidName(string(d[6:])) {
			t.Errorf("%q is not a name the server would accept", d[6:])
		}
	}
}

func TestCheckNewChar(t *testing.T) {
	ok := []int32{5, 0, 3, 2, 4, 6, 0}
	if why := checkNewChar(0, 9, 1, 7, 3, 3, 3, 1, ok); why != "" {
		t.Fatalf("a form the pages can make refused: %s", why)
	}
	if why := checkNewChar(10, 0, 0, 0, 0, 0, 0, 0, make([]int32, 7)); why != "" {
		t.Fatalf("the froglok refused: %s", why)
	}
	bad := map[string]func() string{
		"race 11":       func() string { return checkNewChar(11, 0, 0, 0, 0, 0, 0, 0, ok) },
		"class 15":      func() string { return checkNewChar(0, 15, 0, 0, 0, 0, 0, 0, ok) },
		"sex 2":         func() string { return checkNewChar(0, 0, 2, 0, 0, 0, 0, 0, ok) },
		"hair colour 8": func() string { return checkNewChar(0, 0, 0, 8, 0, 0, 0, 0, ok) },
		"face 4":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 4, 0, ok) },
		"city 3":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 3, ok) },
		"21 points":     func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{21, 0, 0, 0, 0, 0, 0}) },
		"sum 22":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{10, 10, 2, 0, 0, 0, 0}) },
		"negative":      func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{-5, 10, 10, 0, 0, 0, 0}) },
	}
	for name, f := range bad {
		if f() == "" {
			t.Errorf("%s accepted", name)
		}
	}
}

// The game's creation (0x002B) at the server's rules: a message claiming
// level 9 makes a character of the rules' level 50, with its class's spells.
func TestCreateCharacterAtTheServersLevel(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/create.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (13, 9, 0, 0, 16880, 0, 15, 1, 2, 1, 6, -1, 'Sword Art', ''), (12, 1, 0, 0, 16880, 60, 25, 1, 2, 0, 5, -1, 'Going Home', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (0, 13, 'test'), (0, 12, 'test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	store := database.NewSQLiteCharacterStore(db)
	store.SetNewCharacterRules(database.NewCharacterRules{Level: 50, AllSpells: true})
	h := NewCharacterHandler(store, nil, nil, database.NewSpellbookStore(db))
	sess := testSession()
	sess.SetAccountID(1)

	payload := build(protocol.OpcodeCreateCharacter, 128, func(w *buffer.Writer) {
		w.WriteInt32(5)
		w.WriteBytes([]byte("Tasha"))
		for _, v := range []int64{9, 0, 0, 0, 1, 1, 1, 1, 1} { // level 9, a human warrior, Freeport
			w.Write7BitEncodedInt64(v)
		}
		for i := 0; i < 7; i++ {
			w.WriteInt32(0)
		}
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCreateCharacter)
	msg.SetData(payload)
	if err := h.HandleCreateCharacter(sess, msg); err != nil {
		t.Fatal(err)
	}
	rec, err := store.GetByName("Tasha")
	if err != nil {
		t.Fatalf("not made: %v", err)
	}
	if rec.Level != 50 {
		t.Errorf("level %d, want the rules' 50 (the message said 9)", rec.Level)
	}
	rows, err := database.NewSpellbookStore(db).GetSpellRows(rec.ServerID)
	if err != nil || len(rows) != 2 || rows[0].SpellID != 12 || rows[1].SpellID != 13 {
		t.Errorf("book %+v (%v)", rows, err)
	}
}

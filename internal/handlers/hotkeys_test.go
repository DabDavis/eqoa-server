package handlers

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hotkeyWorld is a character in the world and its handler over a real database,
// and a second character the changes must not reach.
type hotkeyWorld struct {
	h         *CharacterHandler
	db        *database.SQLiteDB
	sess      *session.Session
	me, other uint32
	r         *Registry
}

func newHotkeyWorld(t *testing.T) *hotkeyWorld {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/hotkeys.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	a, b := &database.CharacterRecord{CharName: "Waldorf", AccountID: 1}, &database.CharacterRecord{CharName: "Statler", AccountID: 2}
	for _, r := range []*database.CharacterRecord{a, b} {
		if err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	h := NewCharacterHandler(store, nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 500, CharServerID: a.ServerID, Name: "Waldorf"}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	h.gameWorld = w
	r := NewRegistry()
	h.RegisterHandlers(r)
	return &hotkeyWorld{h: h, db: db, sess: sess, me: a.ServerID, other: b.ServerID, r: r}
}

// putWide is a string as CLIENT's VIMemStream writes one: a u32 of its UTF-16 units
// and them (VIMemStream::WriteString, vanilla's put_wide).
func putWide(w *buffer.Writer, s string) { _ = w.WriteStringUnicode(s) }

// send is the message as Send006A, Send006B or Send006C makes it, to the
// registered handler.
func (f *hotkeyWorld) send(t *testing.T, op protocol.GameOpcode, write func(w *buffer.Writer)) {
	t.Helper()
	w := buffer.NewWriterSize(4096)
	write(w)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(append([]byte(nil), w.Bytes()...))
	fn, ok := f.r.Get(op)
	if !ok {
		t.Fatalf("no handler for 0x%04X", uint16(op))
	}
	if err := fn(f.sess, msg); err != nil {
		t.Fatal(err)
	}
}

func (f *hotkeyWorld) choice(t *testing.T, set, slot int64, text, title string) {
	t.Helper()
	f.send(t, protocol.OpcodeHotkeyChoice, func(w *buffer.Writer) {
		_ = w.Write7BitEncodedInt64(set)
		_ = w.Write7BitEncodedInt64(slot)
		putWide(w, text)
		putWide(w, title)
	})
}

// dumped is the MemoryDump's sets for a character, read back as CLIENT's Msg000D
// reads them (VIRec6A): the writer's real bytes, not the store's rows.
func (f *hotkeyWorld) dumped(t *testing.T, serverID uint32) map[int]database.HotkeySet {
	t.Helper()
	w := buffer.NewWriterSize(1 << 16)
	writeHotkeys(w, f.h.hotkeysOf(serverID))
	return readHotkeys(t, w.Bytes())
}

func readHotkeys(t *testing.T, b []byte) map[int]database.HotkeySet {
	t.Helper()
	r := buffer.NewReader(b)
	n, err := r.Read7BitEncodedInt64()
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]database.HotkeySet{}
	for range n {
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			t.Fatal(err)
		}
		s := database.HotkeySet{ID: int(id)}
		for range 4 {
			slot, err := r.Read7BitEncodedInt64()
			if err != nil || slot < 0 || slot > 3 {
				t.Fatalf("set %d: slot %d, %v", id, slot, err)
			}
			text, ok1 := readWide(r)
			title, ok2 := readWide(r)
			if !ok1 || !ok2 {
				t.Fatalf("set %d slot %d: short", id, slot)
			}
			s.Pairs[slot] = database.HotkeyPair{Text: text, Title: title}
		}
		out[int(id)] = s
	}
	if _, err := r.ReadByte(); err == nil {
		t.Error("bytes left after the sets")
	}
	return out
}

// The defaults go out byte for byte as they did when a slot was written as the
// bytes 0, 2, 4, 6 and the first string was called the label: the wire is not
// changed for a character who never edited a hotkey.
func TestTheDefaultHotkeysGoOutAsBefore(t *testing.T) {
	want := buffer.NewWriterSize(1 << 16)
	_ = want.Write7BitEncodedInt64(int64(len(defaultHotkeys)))
	for _, hk := range defaultHotkeys {
		_ = want.Write7BitEncodedInt64(hk.dirCode)
		for i, p := range [][2]string{{hk.nText, hk.nTitle}, {hk.wText, hk.wTitle}, {hk.eText, hk.eTitle}, {hk.sText, hk.sTitle}} {
			_ = want.WriteByte(byte(i * 2))
			_ = want.WriteStringUnicode(p[0])
			_ = want.WriteStringUnicode(p[1])
		}
	}
	f := newHotkeyWorld(t)
	got := buffer.NewWriterSize(1 << 16)
	writeHotkeys(got, f.h.hotkeysOf(f.me))
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatalf("the defaults' %d bytes differ from the %d they were", len(got.Bytes()), len(want.Bytes()))
	}
	if len(defaultHotkeys) != 25 {
		t.Errorf("%d default sets", len(defaultHotkeys))
	}
}

// Petition 1003: "L2 changes to hotkeys don't save after logging out". A choice
// changed (0x006C) is in the character's next MemoryDump with every other set
// still the default's; a set made (0x006A) and a set taken away (0x006B) too; the
// other character keeps the defaults, with no rows of its own. The control: before
// any change the character has no rows and the defaults go out.
func TestHotkeyChangesAreKept(t *testing.T) {
	f := newHotkeyWorld(t)
	base := f.dumped(t, f.me)
	if rows, err := f.db.Hotkeys(f.me); err != nil || len(rows) != 0 {
		t.Fatalf("before any change: %d rows, %v", len(rows), err)
	}

	f.choice(t, 6, 1, "|16", "Shout!") // set 6's west
	got := f.dumped(t, f.me)
	if len(got) != len(base) {
		t.Fatalf("%d sets after a change, want %d", len(got), len(base))
	}
	if p := got[6].Pairs[1]; p.Text != "|16" || p.Title != "Shout!" {
		t.Errorf("the choice changed: %+v", p)
	}
	if got[6].Pairs[0] != base[6].Pairs[0] || got[6].Pairs[3] != base[6].Pairs[3] {
		t.Errorf("the rest of its set moved: %+v, was %+v", got[6], base[6])
	}
	for id, s := range base {
		if id != 6 && got[id] != s {
			t.Errorf("set %d moved: %+v, was %+v", id, got[id], s)
		}
	}
	if o := f.dumped(t, f.other); o[6] != base[6] {
		t.Errorf("the other character's set 6: %+v", o[6])
	}
	if rows, _ := f.db.Hotkeys(f.other); len(rows) != 0 {
		t.Errorf("the other character has %d rows", len(rows))
	}

	// a choice made a branch: CLIENT's 0x006C, then the new set's 0x006A
	f.send(t, protocol.OpcodeHotkeySet, func(w *buffer.Writer) {
		_ = w.Write7BitEncodedInt64(70)
		for slot, p := range [][2]string{{"Inc %4", "Incoming"}, {"", ""}, {"Oom", "Out of power"}, {"", ""}} {
			_ = w.Write7BitEncodedInt64(int64(slot))
			putWide(w, p[0])
			putWide(w, p[1])
		}
	})
	got = f.dumped(t, f.me)
	if s, ok := got[70]; !ok || s.Pairs[0].Title != "Incoming" || s.Pairs[0].Text != "Inc %4" || s.Pairs[2].Title != "Out of power" || s.Pairs[1].Title != "" {
		t.Errorf("the set made: %+v %v", got[70], ok)
	}
	f.choice(t, 70, 3, "Bye", "Leaving") // a later change of it
	if s := f.dumped(t, f.me)[70]; s.Pairs[3].Title != "Leaving" || s.Pairs[0].Title != "Incoming" {
		t.Errorf("the set made, changed: %+v", s)
	}
	f.send(t, protocol.OpcodeHotkeyDrop, func(w *buffer.Writer) { _ = w.Write7BitEncodedInt64(70) })
	if _, ok := f.dumped(t, f.me)[70]; ok {
		t.Error("the set taken away came back")
	}
	if len(f.dumped(t, f.me)) != len(base) {
		t.Errorf("%d sets after the drop, want %d", len(f.dumped(t, f.me)), len(base))
	}

	// a first change that is a drop: the defaults less that set
	f2 := newHotkeyWorld(t)
	f2.send(t, protocol.OpcodeHotkeyDrop, func(w *buffer.Writer) { _ = w.Write7BitEncodedInt64(6) })
	if g := f2.dumped(t, f2.me); len(g) != len(base)-1 || g[1] != base[1] {
		t.Errorf("a first change that drops: %d sets, set 1 %+v", len(g), g[1])
	}
}

// What CLIENT's table cannot hold is not kept: a set past a byte or below 0, a
// slot past 3, words past its record's (cut); a short message writes nothing; no
// player on the session writes nothing. A text with an apostrophe or a percent goes
// through as typed.
func TestHotkeyBounds(t *testing.T) {
	f := newHotkeyWorld(t)
	f.choice(t, 256, 0, "no", "no")
	f.choice(t, -1, 0, "no", "no")
	f.choice(t, 6, 4, "no", "no")
	if rows, _ := f.db.Hotkeys(f.me); len(rows) != 0 {
		t.Errorf("out of bounds wrote %d rows", len(rows))
	}
	f.send(t, protocol.OpcodeHotkeyChoice, func(w *buffer.Writer) { _ = w.Write7BitEncodedInt64(6) }) // short
	if rows, _ := f.db.Hotkeys(f.me); len(rows) != 0 {
		t.Errorf("a short message wrote %d rows", len(rows))
	}
	f.choice(t, 6, 0, strings.Repeat("x", 500), strings.Repeat("y", 500))
	p := f.dumped(t, f.me)[6].Pairs[0]
	if len(p.Text) != database.HotkeyTextMax || len(p.Title) != database.HotkeyTitleMax {
		t.Errorf("cut to %d and %d", len(p.Text), len(p.Title))
	}
	f.choice(t, 6, 2, "Du`Klot's %4 100%", "It's")
	if p := f.dumped(t, f.me)[6].Pairs[2]; p.Text != "Du`Klot's %4 100%" || p.Title != "It's" {
		t.Errorf("as typed: %+v", p)
	}
	before, _ := f.db.Hotkeys(f.me)
	f.sess.SetECSEntity(nil)
	f.choice(t, 6, 0, "gone", "gone")
	after, _ := f.db.Hotkeys(f.me)
	if len(before) != len(after) || after[0] != before[0] {
		t.Error("a session with no player wrote")
	}
}

// The limiter takes a branch dropped (a burst of 0x006B) and refuses a flood.
func TestHotkeyLimits(t *testing.T) {
	sm := NewSecurityMiddleware()
	for _, op := range []protocol.GameOpcode{protocol.OpcodeHotkeyChoice, protocol.OpcodeHotkeySet, protocol.OpcodeHotkeyDrop} {
		p := sm.policies[op]
		if p == nil || p.MinInterval != 0 || p.MaxPerMin < 256 || !p.RequireWorld {
			t.Errorf("0x%04X's policy: %+v", uint16(op), p)
		}
	}
}

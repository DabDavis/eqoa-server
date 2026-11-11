package handlers

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// listNamesOf reads a 0x002C payload's names (disc's or live records).
func listNamesOf(t *testing.T, d []byte, gear int) []string {
	t.Helper()
	pos := 2
	zz := func() int64 {
		var u uint64
		for shift := 0; ; shift += 7 {
			b := d[pos]
			pos++
			u |= uint64(b&0x7F) << shift
			if b&0x80 == 0 {
				break
			}
		}
		return int64(u>>1) ^ -int64(u&1)
	}
	n := zz()
	var out []string
	for i := int64(0); i < n; i++ {
		l := int(binary.LittleEndian.Uint32(d[pos:]))
		out = append(out, string(d[pos+4:pos+4+l]))
		pos += 4 + l
		for k := 0; k < 9; k++ {
			zz()
		}
		pos += gear
	}
	if pos != len(d) {
		t.Fatalf("list read to %d of %d bytes", pos, len(d))
	}
	return out
}

// letters is n as a name's letters (names take letters alone).
func letters(n int) string {
	return string([]byte{'A' + byte(n/26), 'a' + byte(n%26)})
}

// The most recently played of an account's characters, in the list's order:
// played ones by their time, then never played ones the newest made first.
func TestListedCharsMostRecentlyPlayed(t *testing.T) {
	var recs []*database.CharacterRecord
	for i := 1; i <= 12; i++ {
		recs = append(recs, &database.CharacterRecord{ServerID: uint32(i)})
	}
	played := map[uint32]int64{2: 100, 5: 500, 9: 900, 11: 50}
	got := listedChars(recs, 8, played)
	var ids []uint32
	for _, r := range got {
		ids = append(ids, r.ServerID)
	}
	// 9, 5, 2, 11 played; then 12, 10, 8, 7 never: listed in serverid order
	want := []uint32{2, 5, 7, 8, 9, 10, 11, 12}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("listed %v, want %v", ids, want)
	}
	if got := listedChars(recs[:6], 8, nil); len(got) != 6 {
		t.Fatalf("six fit: %d", len(got))
	}
}

// An account of 12: a PS2's session (no answer) is listed the 8 most recently
// played and the look in force is never sent it; a vanilla answering 3 (live
// look + every character) is told 20 and listed all 12 again at once; an older
// vanilla answering 1 is told 8 and stays at 8.
func TestCharacterListOfTwelve(t *testing.T) {
	entity.SetCharSlots(0)
	db, err := database.NewSQLite(t.TempDir() + "/chars.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	for i := 0; i < 12; i++ {
		if err := store.Create(&database.CharacterRecord{CharName: "Many" + letters(i), AccountID: 5, Level: 1}); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := store.GetByAccountID(5)
	// the first four played, the first most lately; the rest never: the
	// newest four made of them fill the 8
	for i, r := range all[:4] {
		if err := db.StampPlayed(r.ServerID, time.Unix(int64(1000-i), 0)); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	look := NewLiveLookHandler(h)

	ps2 := sm.CreateSession(testAddr(31), 31, 31, 31, 31, true)
	ps2.SetAccountID(5)
	ps2.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(ps2)
	names := listNamesOf(t, sentOp(ps2, protocol.OpcodeCharacterSelect), entity.DiscGearSize)
	want := []string{"ManyAa", "ManyAb", "ManyAc", "ManyAd", "ManyAi", "ManyAj", "ManyAk", "ManyAl"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("a PS2's list %v, want %v", names, want)
	}
	if sentOp(ps2, protocol.OpcodeLiveLookInForce) != nil {
		t.Fatal("a PS2 was told the look in force")
	}

	answer := func(s *session.Session, v uint32) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLiveLook)
		msg.SetData(u32b(v))
		if err := look.HandleLiveLook(s, msg); err != nil {
			t.Fatal(err)
		}
	}
	van := sm.CreateSession(testAddr(32), 32, 32, 32, 32, true)
	van.SetAccountID(5)
	van.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(van)
	if n := len(listNamesOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.DiscGearSize)); n != 8 {
		t.Fatalf("before the answer: %d", n)
	}
	answer(van, 3)
	f := sentOp(van, protocol.OpcodeLiveLookInForce)
	if len(f) != 10 || binary.LittleEndian.Uint32(f[2:]) != 1 || binary.LittleEndian.Uint32(f[6:]) != 20 {
		t.Fatalf("in force: %x, want look 1, 20 characters", f)
	}
	if names := listNamesOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.LiveGearSize); len(names) != 12 || names[0] != "ManyAa" || names[11] != "ManyAl" {
		t.Fatalf("vanilla's list again: %v", names)
	}
	// the same answer again: no list
	before := len(lookOpsQueued(van))
	answer(van, 3)
	if got := lookOpsQueued(van); len(got) != before+1 || got[before] != protocol.OpcodeLiveLookInForce {
		t.Fatalf("the same answer again: %v", got[before:])
	}

	old := sm.CreateSession(testAddr(33), 33, 33, 33, 33, true)
	old.SetAccountID(5)
	old.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(old)
	answer(old, 1)
	f = sentOp(old, protocol.OpcodeLiveLookInForce)
	if binary.LittleEndian.Uint32(f[6:]) != 8 {
		t.Fatalf("an older vanilla told %x", f)
	}
	if n := len(listNamesOf(t, sentOp(old, protocol.OpcodeCharacterSelect), entity.LiveGearSize)); n != 8 {
		t.Fatalf("an older vanilla's list: %d", n)
	}

	// the server's own number, and a vanilla that has said it: told it
	entity.SetCharSlots(10)
	defer entity.SetCharSlots(0)
	v2 := sm.CreateSession(testAddr(34), 34, 34, 34, 34, true)
	v2.SetAccountID(5)
	v2.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(v2)
	answer(v2, 2)
	if f := sentOp(v2, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(f[2:]) != 0 || binary.LittleEndian.Uint32(f[6:]) != 10 {
		t.Fatalf("max 10, the disc's look: %x", f)
	}
	if names := listNamesOf(t, sentOp(v2, protocol.OpcodeCharacterSelect), entity.DiscGearSize); len(names) != 10 {
		t.Fatalf("max 10: %v", names)
	}
}

// Creation's cap: a PS2's session makes no ninth (0x002E, CLIENT's "Character
// create failed"); vanilla's makes up to the server's 20 and no 21st.
func TestCreationCapByClient(t *testing.T) {
	entity.SetCharSlots(0)
	db, err := database.NewSQLite(t.TempDir() + "/create.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	h := NewCharacterHandler(store, nil, nil, database.NewSpellbookStore(db))
	create := func(sess *session.Session, name string) bool {
		payload := build(protocol.OpcodeCreateCharacter, 128, func(w *buffer.Writer) {
			w.WriteInt32(int32(len(name)))
			w.WriteBytes([]byte(name))
			for _, v := range []int64{1, 0, 0, 0, 1, 1, 1, 1, 1} {
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
		_, err := store.GetByName(name)
		return err == nil
	}
	ps2 := session.NewSession(testAddr(41), 41, 41, 0, 0, true)
	ps2.SetAccountID(7)
	for i := 0; i < 8; i++ {
		if !create(ps2, "Ps"+letters(i)) {
			t.Fatalf("a PS2's character %d not made", i+1)
		}
	}
	if create(ps2, "Psninth") {
		t.Fatal("a PS2 made a ninth")
	}
	if d := sentOp(ps2, protocol.OpcodeCreateCharFailed); d == nil {
		t.Fatal("the ninth refused without 0x002E")
	}

	van := session.NewSession(testAddr(42), 42, 42, 0, 0, true)
	van.SetAccountID(7)
	van.SetManyChars(true)
	for i := 8; i < 20; i++ {
		if !create(van, "Va"+letters(i)) {
			t.Fatalf("vanilla's character %d not made", i+1)
		}
	}
	if n, _ := store.CountByAccount(7); n != 20 {
		t.Fatalf("%d made", n)
	}
	if create(van, "Vatwentyone") {
		t.Fatal("vanilla made a 21st")
	}
}

package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// selectInfoFixture is a character list's world: an account with a human (no
// mastery row), a Ratman (900 points earned, 100 of them spent) and a Wisp.
type selectInfoFixture struct {
	h    *CharacterHandler
	sm   *session.Manager
	ids  [3]uint32
	info *SelectInfoHandler
}

func newSelectInfoFixture(t *testing.T) *selectInfoFixture {
	t.Helper()
	entity.SetRaceDefs([]database.RaceDefRow{
		{ID: 24, Name: "Ratman", ShortName: "RAT", WireRace: 4, BodyMale: -1159977527, BodyFemale: -1159977527},
		{ID: 31, Name: "Wisp", ShortName: "WSP", WireRace: 0, BodyMale: 0x70D898C6, BodyFemale: 0x0D537335, City: 2},
	})
	t.Cleanup(func() { entity.SetRaceDefs(nil) })
	db, err := database.NewSQLite(t.TempDir() + "/selectinfo.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	f := &selectInfoFixture{}
	for i, c := range []database.CharacterRecord{
		{CharName: "Plain", AccountID: 5, Level: 1, Race: 0, TClass: 14},
		{CharName: "Squeak", AccountID: 5, Level: 60, Race: 24, TClass: 4},
		{CharName: "Glim", AccountID: 5, Level: 12, Race: 31, TClass: 13},
	} {
		rec := c
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		f.ids[i] = rec.ServerID
	}
	if err := store.SaveMastery(f.ids[1], database.MasteryRow{Points: 800, Spent: 100}); err != nil {
		t.Fatal(err)
	}
	sm := session.NewManager()
	w := entity.NewWorld()
	w.SessionManager = sm
	f.h = &CharacterHandler{store: store, inventoryStore: database.NewInventoryStore(db), sessionManager: sm, gameWorld: w}
	f.sm, f.info = sm, NewSelectInfoHandler(f.h)
	return f
}

func (f *selectInfoFixture) session(n byte) *session.Session {
	s := f.sm.CreateSession(testAddr(n), uint32(n), uint32(n), uint16(n), uint16(n), true)
	s.SetAccountID(5)
	s.SetState(session.StateCharacterSelect)
	return s
}

func (f *selectInfoFixture) answer(t *testing.T, s *session.Session, v uint32) {
	t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectInfo)
	msg.SetData(u32b(v))
	if err := f.info.HandleSelectInfo(s, msg); err != nil {
		t.Fatal(err)
	}
}

// readSelectInfo reads an OpcodeSelectInfoData body: by server id, the race,
// its name, the points and the rows.
type selectEntry struct {
	race   int
	name   string
	points int
	rows   []string
}

func readSelectInfo(t *testing.T, b []byte) map[uint32]selectEntry {
	t.Helper()
	if len(b) < 3 || b[0] != selectInfoVersion {
		t.Fatalf("select info: % x", b)
	}
	n := int(binary.LittleEndian.Uint16(b[1:3]))
	at := 3
	str := func() string {
		l := int(b[at])
		s := string(b[at+1 : at+1+l])
		at += 1 + l
		return s
	}
	out := map[uint32]selectEntry{}
	for range n {
		id := binary.LittleEndian.Uint32(b[at:])
		e := selectEntry{race: int(b[at+4])}
		at += 5
		e.name = str()
		e.points = int(binary.LittleEndian.Uint16(b[at:]))
		at += 2
		k := int(b[at])
		at++
		for range k {
			e.rows = append(e.rows, str())
		}
		out[id] = e
	}
	if at != len(b) {
		t.Fatalf("select info: %d bytes read of %d", at, len(b))
	}
	return out
}

// The select info is offered once, after a session's first list; a client that
// answers 1 is sent each character's own race (the Ratman's and the Wisp's
// racedefs' names, the human's none: the client names a game race), its mastery
// points (spent and unspent) and the rows its Customize re-chooses (the Wisp's
// colour), at once and after every list, and nothing at 0. The list itself is
// the same bytes whoever reads it. The controls: a PS2 that never answers is
// sent none, and an answer never offered is not taken.
func TestSelectInfoOfferedAndAnswered(t *testing.T) {
	f := newSelectInfoFixture(t)

	early := f.session(40)
	f.answer(t, early, 1)
	if early.SelectInfo() || hasOp(early, protocol.OpcodeSelectInfoData) {
		t.Fatal("an answer to an offer never made was taken")
	}

	ps2 := f.session(41)
	f.h.SendCharacterSelectEQOA(ps2)
	if d := sentOp(ps2, protocol.OpcodeSelectInfo); len(d) != 6 || binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	ps2List := sentOp(ps2, protocol.OpcodeCharacterSelect)
	ps2.Queue().GetPending()
	f.h.SendCharacterSelectEQOA(ps2)
	if hasOp(ps2, protocol.OpcodeSelectInfo) || hasOp(ps2, protocol.OpcodeSelectInfoData) {
		t.Fatal("a second offer, or info to a client that never answered")
	}

	van := f.session(42)
	f.h.SendCharacterSelectEQOA(van)
	if !bytes.Equal(sentOp(van, protocol.OpcodeCharacterSelect), ps2List) {
		t.Fatal("the list differs by who reads it")
	}
	f.answer(t, van, 1)
	got := readSelectInfo(t, sentOp(van, protocol.OpcodeSelectInfoData)[2:])
	want := map[uint32]selectEntry{
		f.ids[0]: {race: 0},
		f.ids[1]: {race: 24, name: "Ratman", points: 900},
		f.ids[2]: {race: 31, name: "Wisp", rows: []string{"wispcolor"}},
	}
	for id, w := range want {
		g := got[id]
		if g.race != w.race || g.name != w.name || g.points != w.points || len(g.rows) != len(w.rows) || (len(w.rows) > 0 && g.rows[0] != w.rows[0]) {
			t.Errorf("character %d: %+v, want %+v", id, g, w)
		}
	}
	if len(got) != 3 {
		t.Errorf("%d characters", len(got))
	}
	van.Queue().GetPending()
	f.h.SendCharacterSelectEQOA(van)
	if ops := queuedOps(van); len(ops) < 2 || ops[0] != protocol.OpcodeCharacterSelect || ops[len(ops)-1] != protocol.OpcodeSelectInfoData {
		t.Fatalf("a list after the answer: %v, want the list first and the info last", ops)
	}
	f.answer(t, van, 0)
	van.Queue().GetPending()
	f.h.SendCharacterSelectEQOA(van)
	if hasOp(van, protocol.OpcodeSelectInfoData) || van.SelectInfo() {
		t.Fatal("info after the answer 0")
	}
}

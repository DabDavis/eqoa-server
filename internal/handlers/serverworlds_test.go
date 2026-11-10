package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// homelands is a built world of two zones, as cmd/worldbuild writes it.
func homelands() worlds.Built {
	return worlds.Built{
		Recipe: worlds.Recipe{World: 10, Name: "the Homelands", Zones: []worlds.RecipeZone{
			{Name: "Aviak Village", From: "TUNARIA.ESF", Zone: 115, Race: 14}, {Name: "Kerra Isle", From: "ODUS.ESF", Zone: 19, Race: 12},
		}},
		Size: 5880303, CRC: 0xB6D94C34, Rows: 11, Cols: 5,
		Parts: []worlds.BuiltZone{
			{From: "TUNARIA.ESF", Zone: 115, Size: 5879151, CRC: 0x7257AE33, Col: 4, Row: 10},
			{From: "ODUS.ESF", Zone: 19, Size: 8409752, CRC: 0x91047E6F, Col: 2, Row: 3},
		},
	}
}

// withServerWorlds makes the server's own worlds these for the test.
func withServerWorlds(t *testing.T, built ...worlds.Built) {
	t.Helper()
	SetServerWorlds(built)
	t.Cleanup(func() { SetServerWorlds(nil) })
}

// The worlds' bytes, field by field: version, count, then the world (slot,
// name, size, CRC, zones) and each zone (its file, index, record size and CRC,
// cell, name, the race whose home it is).
func TestServerWorldsFormat(t *testing.T) {
	got := encodeServerWorlds([]worlds.Built{homelands()})
	want := []byte{1, 1, 10, 13}
	want = append(want, "the Homelands"...)
	want = binary.LittleEndian.AppendUint32(want, 5880303)
	want = binary.LittleEndian.AppendUint32(want, 0xB6D94C34)
	want = append(want, 2, 11)
	want = append(want, "TUNARIA.ESF"...)
	want = append(want, 115, 0)
	want = binary.LittleEndian.AppendUint32(want, 5879151)
	want = binary.LittleEndian.AppendUint32(want, 0x7257AE33)
	want = append(want, 4, 10, 13)
	want = append(want, "Aviak Village"...)
	want = append(want, 14)
	want = append(want, 8)
	want = append(want, "ODUS.ESF"...)
	want = append(want, 19, 0)
	want = binary.LittleEndian.AppendUint32(want, 8409752)
	want = binary.LittleEndian.AppendUint32(want, 0x91047E6F)
	want = append(want, 2, 3, 10)
	want = append(want, "Kerra Isle"...)
	want = append(want, 12)
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got %x\nwant %x", got, want)
	}
	if b := encodeServerWorlds(nil); !bytes.Equal(b, []byte{1, 0}) {
		t.Fatalf("no worlds: %x", b)
	}
}

// With extras the list is version 2, each zone followed by its extras: the
// places their art comes from, named once each in first use, and each extra
// as its ZoneActor takes it (the scale 1 and the light 0, 0, 0, 255 when none
// is given). A zone with none says so (0, 0). The control: no world with
// extras is version 1 (above), which a client of version 1 still reads.
func TestServerWorldsFormatWithExtras(t *testing.T) {
	w := homelands()
	w.Zones[1].Extras = []worlds.Extra{
		{Sprite: 0xF0178AAC, From: "odus.esf", Zone: 22, At: [3]float32{1, 2, 3}, HPR: [3]float32{0.5, 0, 0}, Scale: 2},
		{Sprite: 0x194FA7CB, At: [3]float32{4, 5, 6}, Color: &[4]uint8{9, 8, 7, 6}},
		{Sprite: 0x5698614C, From: "ODUS.ESF", Zone: 22, At: [3]float32{7, 8, 9}},
	}
	got := encodeServerWorlds([]worlds.Built{w})
	v1 := encodeServerWorlds([]worlds.Built{homelands()})
	// version 2: the same up to the first zone's race, then its (no) extras
	if got[0] != 2 || !bytes.Equal(got[1:], func() []byte {
		first := bytes.Index(v1, []byte("Aviak Village")) + len("Aviak Village") + 1
		b := append([]byte(nil), v1[1:first]...)
		b = append(b, 0, 0, 0)
		b = append(b, v1[first:]...)
		b = append(b, 3, 0, 1, 8)
		b = append(b, "ODUS.ESF"...)
		b = append(b, 22, 0)
		f := func(v ...float32) {
			for _, x := range v {
				b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
			}
		}
		b = append(b, 0)
		b = binary.LittleEndian.AppendUint32(b, 0xF0178AAC)
		f(1, 2, 3, 0.5, 0, 0, 2)
		b = append(b, 0, 0, 0, 255, 0xFF)
		b = binary.LittleEndian.AppendUint32(b, 0x194FA7CB)
		f(4, 5, 6, 0, 0, 0, 1)
		b = append(b, 9, 8, 7, 6, 0)
		b = binary.LittleEndian.AppendUint32(b, 0x5698614C)
		f(7, 8, 9, 0, 0, 0, 1)
		return append(b, 0, 0, 0, 255)
	}()) {
		t.Fatalf("\n got %x\nwant (v1 with extras) from %x", got, v1)
	}
}

// The offer comes once a session, after the first list (after the live look's
// and creation's), only while the server has worlds of its own; a client that
// answers 1 is sent them, 0 nothing; then what it says it holds is what
// LoadableWorlds takes for it, a slot that is not one of the server's left
// out. The controls: a server with none offers nothing; a session that said
// nothing loads the disc's six alone; a PS2 never answers.
func TestServerWorldsOfferAndWhatAClientHolds(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{})
	off := r.session()
	r.h.SendCharacterSelectEQOA(off)
	// the offers after a list are counted by the worlds' own (the live look's,
	// the race options' and the others' come and go with their own switches)
	offers := func(got []protocol.GameOpcode) int {
		n := 0
		for _, op := range got {
			if op == protocol.OpcodeServerWorlds {
				n++
			}
		}
		return n
	}
	if got := lookOpsQueued(off); offers(got) != 0 || len(got) == 0 || got[0] != protocol.OpcodeCharacterSelect {
		t.Fatalf("no worlds of its own: %v, want the list and no worlds' offer", got)
	}

	withServerWorlds(t, homelands())
	van := r.session()
	r.h.SendCharacterSelectEQOA(van)
	first := lookOpsQueued(van)
	if offers(first) != 1 || first[0] != protocol.OpcodeCharacterSelect {
		t.Fatalf("a first list: %v, want the list and one worlds' offer", first)
	}
	if d := sentOp(van, protocol.OpcodeServerWorlds); len(d) != 6 || binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	r.h.SendCharacterSelectEQOA(van)
	if got := lookOpsQueued(van); offers(got) != 1 || len(got) != len(first)+1 {
		t.Fatalf("a second list: %v, want the list again and no second offer", got)
	}
	h := NewServerWorldsHandler(r.h)
	answer := func(s *session.Session, op protocol.GameOpcode, data []byte) {
		t.Helper()
		m := protocol.NewMessage(protocol.MessageTypeReliable, op)
		m.SetData(data)
		var err error
		if op == protocol.OpcodeServerWorlds {
			err = h.HandleOffer(s, m)
		} else {
			err = h.HandleHeld(s, m)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	before := len(lookOpsQueued(van))
	answer(van, protocol.OpcodeServerWorlds, u32b(0))
	if got := lookOpsQueued(van); len(got) != before {
		t.Fatalf("answered 0: %v, want nothing more than %d", got, before)
	}
	answer(van, protocol.OpcodeServerWorlds, u32b(1))
	// which of its characters need them first (none: account 5 has none), then the worlds
	if got := lookOpsQueued(van); len(got) != before+2 || got[before] != protocol.OpcodeServerWorldsChars || got[before+1] != protocol.OpcodeServerWorldsData {
		t.Fatalf("answered 1: %v, want which characters need them, then the worlds, after %d", got, before)
	}
	if d := sentOp(van, protocol.OpcodeServerWorldsChars); !bytes.Equal(d[2:], []byte{1, 0}) {
		t.Fatalf("an account with no character: %x, want version 1, none", d)
	}
	if body := sentOp(van, protocol.OpcodeServerWorldsData)[2:]; !bytes.Equal(body, encodeServerWorlds([]worlds.Built{homelands()})) {
		t.Fatalf("the worlds sent: %x", body)
	}
	if LoadableWorlds(van)(10) {
		t.Fatal("world 10 loadable before the client said it holds it")
	}
	answer(van, protocol.OpcodeServerWorldsData, []byte{2, 10, 11})
	if got := van.ServerWorlds(); len(got) != 1 || got[0] != 10 {
		t.Fatalf("held: %v, want [10] (11 is no world of the server's)", got)
	}
	load := LoadableWorlds(van)
	if !load(10) || !load(0) || !load(5) || load(11) || load(9) {
		t.Fatalf("loadable 10 %v 0 %v 5 %v 11 %v 9 %v", load(10), load(0), load(5), load(11), load(9))
	}
	answer(van, protocol.OpcodeServerWorldsData, []byte{3, 10})
	if got := van.ServerWorlds(); len(got) != 1 {
		t.Fatalf("a list cut short changed what is held: %v", got)
	}
	answer(van, protocol.OpcodeServerWorldsData, []byte{0})
	if LoadableWorlds(van)(10) {
		t.Fatal("still loadable once the client said it holds none")
	}
	if ps2 := r.session(); LoadableWorlds(ps2)(10) || !LoadableWorlds(ps2)(2) {
		t.Fatal("a client that said nothing: the disc's six alone")
	}
}

// A built world is offered only when its slot is listed (world.server_worlds)
// and its file is there with the size its built recipe says: the slot is named
// for it then. The controls: no recipe, a file of another size, no slot listed,
// a recipe naming another slot.
func TestLoadServerWorlds(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { SetServerWorlds(nil); worldNames[10] = "world10" })
	b := homelands()
	b.Size = 64
	j, _ := json.Marshal(b)
	if got := LoadServerWorlds(dir, []int{10}); len(got) != 0 {
		t.Fatalf("an empty folder: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.ESF"), make([]byte, 63), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadServerWorlds(dir, []int{10}); len(got) != 0 || worldName(10) != "world10" {
		t.Fatalf("a file of another size: %v, %q", got, worldName(10))
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.ESF"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadServerWorlds(dir, nil); len(got) != 0 {
		t.Fatalf("no slot listed: %v", got)
	}
	got := LoadServerWorlds(dir, []int{10})
	if len(got) != 1 || got[0].World != 10 || worldName(10) != "the Homelands" {
		t.Fatalf("loaded %v, named %q", got, worldName(10))
	}
	if _, ok := ServerWorld(10); !ok {
		t.Fatal("not the server's")
	}
	b.World = 11
	j, _ = json.Marshal(b)
	if err := os.WriteFile(filepath.Join(dir, "world10.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadServerWorlds(dir, []int{10}); len(got) != 0 {
		t.Fatalf("a recipe of another slot: %v", got)
	}
}

// A login in a world of the server's own resumes there for a client that holds
// it; for one that does not (a PS2, a page), on its race's spot on the disc's
// worlds (its wire race's row: the giant's ogre one in world 1), the homeland's
// own rows passed over; and a world past the disc's that is not the server's
// resumes for no one. The control: a disc world resumes as ever.
func TestALoginInAServerWorld(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	withServerWorlds(t, homelands())
	if _, err := r.db.Exec(`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		VALUES (0, 11, 0, 1, 0, 0, 0, 0, 9100, 60, 21100, 1.5, 10)`); err != nil {
		t.Fatal(err)
	}
	giant := func() *database.CharacterRecord {
		return &database.CharacterRecord{CharName: "Biggy", Race: 11, TClass: 0, X: 9123, Y: 61, Z: 21140, World: 10}
	}
	holds := func(w uint8) bool { return w == 10 }
	rec := giant()
	loginSpawn(nil, rec, holds, r.h.mainland)
	if rec.World != 10 || rec.X != 9123 {
		t.Fatalf("a client that holds the homeland: %+v", rec)
	}
	rec = giant()
	loginSpawn(nil, rec, nil, r.h.mainland)
	if rec.World != 1 || rec.X != 9377.7 || rec.Z != 7385.8 {
		t.Fatalf("a client that does not: %+v, want the ogre's row in world 1", rec)
	}
	// a new giant starts in the homeland (its own rows) all the same
	made := &database.CharacterRecord{CharName: "Newgiant", AccountID: 5, Race: 11, TClass: 0, ModelID: -5117396}
	if _, err := r.store.CreateNew(made); err != nil {
		t.Fatal(err)
	}
	if made.World != 10 || made.X != 9100 {
		t.Fatalf("a new giant: %+v, want the homeland's row", made)
	}
	rec = giant()
	rec.World = 9
	if why, ok := resumable(rec, func(uint8) bool { return true }); ok || !strings.Contains(why, "not on every disc") {
		t.Fatalf("world 9, no world of the server's: %q %v", why, ok)
	}
	rec = giant()
	rec.World, rec.X, rec.Z = 1, 9000, 7000
	if _, ok := resumable(rec, nil); !ok {
		t.Fatal("the control: a disc world resumes")
	}
}

// OpcodeServerWorldsChars' bytes: version 1, a u8 count, each ServerID a u32,
// little-endian; none is the two bytes alone.
func TestServerWorldsCharsFormat(t *testing.T) {
	if got, want := encodeServerWorldsChars([]uint32{630, 0x01020304}), []byte{1, 2, 0x76, 0x02, 0, 0, 4, 3, 2, 1}; !bytes.Equal(got, want) {
		t.Fatalf("\n got %x\nwant %x", got, want)
	}
	if got := encodeServerWorldsChars(nil); !bytes.Equal(got, []byte{1, 0}) {
		t.Fatalf("none: %x", got)
	}
	if got := encodeServerWorldsChars(make([]uint32, 300)); len(got) != 2+4*255 || got[1] != 255 {
		t.Fatalf("300 characters: %d bytes, count %d", len(got), got[1])
	}
}

// The characters a client is told to hold its pick for are exactly those a
// login places differently by what the client holds: loginServerWorld says
// yes for a record just when loginSpawn with the worlds held and loginSpawn
// without them end in different places. One saved in a server's world at a
// place a login resumes needs them; one in a world of the disc, one never
// saved, one under the world's floor there, one in a slot past the disc's
// that is no world of the server's, and one saved in the arena's world (it
// starts beside the herald either way) do not. The control for the last: with
// no arena the same record needs its world.
func TestWhichLoginsNeedTheServersWorlds(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	arena := homelands()
	arena.World, arena.Name = 11, "the Test Canyon"
	withServerWorlds(t, homelands(), arena)
	pr := newPortalRig(t)
	set := entity.DefaultCTFSettings()
	pr.gw.SetCTF(true, set, entity.CTFLayout{Name: "the Test Canyon"})
	ctfHerald(t, pr)
	if int(set.World) != 11 {
		t.Fatalf("the arena's world is %d: this test's is 11", set.World)
	}
	all := func(uint8) bool { return true }
	cases := []struct {
		what  string
		gw    *entity.World
		rec   database.CharacterRecord
		needs bool
	}{
		{"a giant in its homeland", nil, database.CharacterRecord{CharName: "Biggy", Race: 11, X: 9123, Y: 61, Z: 21140, World: 10}, true},
		{"a human in Freeport", nil, database.CharacterRecord{CharName: "Townie", X: 25300, Y: 55, Z: 15700, World: 0}, false},
		{"an ogre in Rathe", nil, database.CharacterRecord{CharName: "Oggy", Race: 9, X: 9000, Y: 53, Z: 7000, World: 1}, false},
		{"a giant never saved", nil, database.CharacterRecord{CharName: "Newborn", Race: 11, World: 10}, false},
		{"a giant fallen under its homeland", nil, database.CharacterRecord{CharName: "Faller", Race: 11, X: 9123, Y: entity.WorldFloorY - 5, Z: 21140, World: 10}, false},
		{"a giant in a slot that is no world of the server's", nil, database.CharacterRecord{CharName: "Lost", Race: 11, X: 9123, Y: 61, Z: 21140, World: 9}, false},
		{"one saved in the arena, an arena on", pr.gw, database.CharacterRecord{CharName: "Arenite", X: 20500, Y: 6, Z: 21100, World: 11}, false},
		{"one saved in the arena's world, no arena (the control)", nil, database.CharacterRecord{CharName: "Arenite", X: 20500, Y: 6, Z: 21100, World: 11}, true},
		{"a giant in its homeland, an arena on", pr.gw, database.CharacterRecord{CharName: "Biggy", Race: 11, X: 9123, Y: 61, Z: 21140, World: 10}, true},
	}
	for _, c := range cases {
		rec := c.rec
		world, needs := loginServerWorld(c.gw, &rec)
		if needs != c.needs || (needs && int(world) != c.rec.World) {
			t.Errorf("%s: needs %v (world %d), want %v", c.what, needs, world, c.needs)
		}
		with, without := c.rec, c.rec
		loginSpawn(c.gw, &with, all, r.h.mainland)
		loginSpawn(c.gw, &without, nil, r.h.mainland)
		differs := with.World != without.World || with.X != without.X || with.Y != without.Y || with.Z != without.Z
		if differs != needs {
			t.Errorf("%s: needs %v, but a login with the worlds held starts at world %d (%.1f, %.1f, %.1f) and one without at world %d (%.1f, %.1f, %.1f)",
				c.what, needs, with.World, with.X, with.Y, with.Z, without.World, without.X, without.Y, without.Z)
		}
		if needs && with.World != c.rec.World {
			t.Errorf("%s: with its world held it starts in world %d", c.what, with.World)
		}
	}
	if _, needs := loginServerWorld(nil, nil); needs {
		t.Error("no record needs a world")
	}
}

// A client that takes the offer is told at once which of its account's
// characters a login needs the server's worlds for, BEFORE the worlds
// themselves (they are seconds on the way), and again after every later
// character list, as the list then stands: a character made meanwhile in a
// homeland is in it, one that left for the mainland is not. The controls: a
// session that never took the offer (a PS2, an older page without the
// storage) is never sent it, not with its first list and not with a later
// one; a first list says nothing (the offer is not yet answered); a client
// already in the world that asks again (its pick went before the worlds were
// all in) is sent the worlds alone; a client that asks while the worlds it
// was sent still wait in its queue is queued nothing more (one copy at most).
func TestAClientIsToldWhichCharactersNeedTheServersWorlds(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	withServerWorlds(t, homelands())
	if _, err := r.db.Exec(`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		VALUES (0, 11, 0, 1, 0, 0, 0, 0, 9100, 60, 21100, 1.5, 10)`); err != nil {
		t.Fatal(err)
	}
	make := func(name string, race int) *database.CharacterRecord {
		t.Helper()
		c := &database.CharacterRecord{CharName: name, AccountID: 5, Race: race, TClass: 0, ModelID: -5117396}
		if _, err := r.store.CreateNew(c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	townie, giant := make("Townie", 0), make("Biggy", 11)
	if townie.World != 0 || giant.World != 10 {
		t.Fatalf("made in worlds %d and %d, want 0 and 10", townie.World, giant.World)
	}
	h := NewServerWorldsHandler(r.h)
	take := func(s *session.Session) {
		t.Helper()
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerWorlds)
		m.SetData(u32b(1))
		if err := h.HandleOffer(s, m); err != nil {
			t.Fatal(err)
		}
	}
	// the ids each OpcodeServerWorldsChars sent so far names, in order
	told := func(s *session.Session) [][]uint32 {
		t.Helper()
		var out [][]uint32
		for _, m := range s.Queue().PeekPending() {
			d := m.Data()
			if len(d) < 2 || opcode(d) != protocol.OpcodeServerWorldsChars {
				continue
			}
			if len(d) < 4 || d[2] != 1 || len(d) != 4+4*int(d[3]) {
				t.Fatalf("which characters: %x", d)
			}
			ids := []uint32{}
			for at := 4; at < len(d); at += 4 {
				ids = append(ids, binary.LittleEndian.Uint32(d[at:]))
			}
			out = append(out, ids)
		}
		return out
	}
	one := func(got [][]uint32, n int, want ...uint32) bool {
		if len(got) != n {
			return false
		}
		last := got[n-1]
		if len(last) != len(want) {
			return false
		}
		for i := range want {
			if last[i] != want[i] {
				return false
			}
		}
		return true
	}
	ops := func(s *session.Session) []protocol.GameOpcode { return lookOpsQueued(s) }
	indexOf := func(got []protocol.GameOpcode, op protocol.GameOpcode, from int) int {
		for i := from; i < len(got); i++ {
			if got[i] == op {
				return i
			}
		}
		return -1
	}

	van := r.session()
	r.h.SendCharacterSelectEQOA(van)
	if got := told(van); len(got) != 0 {
		t.Fatalf("a first list, the offer not yet answered: told %v", got)
	}
	first := len(ops(van))
	take(van)
	if got := told(van); !one(got, 1, uint32(giant.ServerID)) {
		t.Fatalf("the offer taken: told %v, want the giant (%d) alone", got, giant.ServerID)
	}
	got := ops(van)
	chars, data := indexOf(got, protocol.OpcodeServerWorldsChars, first), indexOf(got, protocol.OpcodeServerWorldsData, first)
	if chars != first || data != first+1 || len(got) != first+2 {
		t.Fatalf("after the answer: %v, want which characters need them and then the worlds", got[first:])
	}

	// a later list: a second giant made, the first gone to the mainland
	second := make("Bigger", 11)
	giant.World, giant.X, giant.Y, giant.Z = 1, 9377.7, 53.2, 7385.8
	if err := r.store.Update(giant); err != nil {
		t.Fatal(err)
	}
	before := len(ops(van))
	r.h.SendCharacterSelectEQOA(van)
	if got := told(van); !one(got, 2, uint32(second.ServerID)) {
		t.Fatalf("a later list: told %v, want the second giant (%d) alone", got, second.ServerID)
	}
	got = ops(van)
	if list, chars := indexOf(got, protocol.OpcodeCharacterSelect, before), indexOf(got, protocol.OpcodeServerWorldsChars, before); list != before || chars < list {
		t.Fatalf("a later list: %v, want the list and then which characters need the worlds", got[before:])
	}

	// asked again while the worlds sent still wait in its queue (none
	// acknowledged): nothing more is queued, however often it asks
	before = len(van.Queue().PeekPending())
	for i := 0; i < 5; i++ {
		take(van)
	}
	if n := len(van.Queue().PeekPending()); n != before {
		t.Fatalf("asked 5 times with the worlds on the way: %d messages more queued, want none", n-before)
	}

	// in the world (the pick's promotion emptied the queue), the worlds asked
	// for again: the worlds alone, and once however often it asks
	van.Queue().Clear()
	van.SetInGame(true)
	for i := 0; i < 3; i++ {
		take(van)
	}
	if got := ops(van); len(got) != 1 || got[0] != protocol.OpcodeServerWorldsData {
		t.Fatalf("asked again in the world, 3 times: %v, want the worlds alone, once", got)
	}

	// the controls: a session that never took the offer
	ps2 := r.session()
	r.h.SendCharacterSelectEQOA(ps2)
	r.h.SendCharacterSelectEQOA(ps2)
	if got := told(ps2); len(got) != 0 || ps2.ServerWorldsTaken() {
		t.Fatalf("a client that never answered the offer: told %v, taken %v", got, ps2.ServerWorldsTaken())
	}
	zero := r.session()
	r.h.SendCharacterSelectEQOA(zero)
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerWorlds)
	m.SetData(u32b(0))
	if err := h.HandleOffer(zero, m); err != nil {
		t.Fatal(err)
	}
	r.h.SendCharacterSelectEQOA(zero)
	if got := told(zero); len(got) != 0 || zero.ServerWorldsTaken() {
		t.Fatalf("a client that answered 0: told %v, taken %v", got, zero.ServerWorldsTaken())
	}
}

// A character still in the world (its client dropped; another window plays
// it) is taken over where it STANDS, so it is by where it stands that its
// pick needs the server's worlds or not, whatever its row says: one standing
// in a homeland with a row in Freeport needs them, one standing in Freeport
// with a row in a homeland does not. The control: neither in the world, each
// goes by its row.
func TestACharacterInTheWorldGoesByWhereItStands(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	withServerWorlds(t, homelands())
	rowHome := &database.CharacterRecord{ServerID: 71, CharName: "Rowhome", AccountID: 5, Race: 11, X: 9123, Y: 61, Z: 21140, World: 10}
	rowTown := &database.CharacterRecord{ServerID: 72, CharName: "Rowtown", AccountID: 5, X: 25300, Y: 55, Z: 15700, World: 0}
	records := []*database.CharacterRecord{rowHome, rowTown}
	if got := r.h.serverWorldChars(records); len(got) != 1 || got[0] != 71 {
		t.Fatalf("by their rows: %v, want [71]", got)
	}
	gw := r.h.gameWorld
	sa, sb := r.session(), r.session()
	pa := gw.CreatePlayer(sa, &database.CharacterRecord{ServerID: 71, CharName: "Rowhome", X: 25300, Y: 55, Z: 15700, World: 0})
	pb := gw.CreatePlayer(sb, &database.CharacterRecord{ServerID: 72, CharName: "Rowtown", X: 9123, Y: 61, Z: 21140, World: 10})
	if pa == nil || pb == nil {
		t.Fatal("no players")
	}
	if got := r.h.serverWorldChars(records); len(got) != 1 || got[0] != 72 {
		t.Fatalf("in the world, by where they stand: %v, want [72]", got)
	}
	if x, _, z, _, world, ok := gw.CharacterPlace(72); !ok || world != 10 || x != 9123 || z != 21140 {
		t.Fatalf("where 72 stands: world %d (%.0f, %.0f) %v", world, x, z, ok)
	}
	if _, _, _, _, _, ok := gw.CharacterPlace(73); ok {
		t.Fatal("a character not in the world stands nowhere")
	}
}

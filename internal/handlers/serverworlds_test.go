package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
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
	if got := lookOpsQueued(off); len(got) != 2 {
		t.Fatalf("no worlds of its own: %v, want the list and the live look's offer alone", got)
	}

	withServerWorlds(t, homelands())
	van := r.session()
	r.h.SendCharacterSelectEQOA(van)
	want := []protocol.GameOpcode{protocol.OpcodeCharacterSelect, protocol.OpcodeLiveLook, protocol.OpcodeServerWorlds}
	if got := lookOpsQueued(van); len(got) != 3 || got[2] != want[2] {
		t.Fatalf("a first list: %v, want %v", got, want)
	}
	if d := sentOp(van, protocol.OpcodeServerWorlds); len(d) != 6 || binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	r.h.SendCharacterSelectEQOA(van)
	if got := lookOpsQueued(van); len(got) != 4 {
		t.Fatalf("a second list: %v, want no second offer", got)
	}
	h := NewServerWorldsHandler()
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
	answer(van, protocol.OpcodeServerWorlds, u32b(0))
	if got := lookOpsQueued(van); len(got) != 4 {
		t.Fatalf("answered 0: %v", got)
	}
	answer(van, protocol.OpcodeServerWorlds, u32b(1))
	if got := lookOpsQueued(van); len(got) != 5 || got[4] != protocol.OpcodeServerWorldsData {
		t.Fatalf("answered 1: %v", got)
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

// A built world is offered only when its file is there with the size its
// built recipe says: the slot is named for it then. The controls: no recipe,
// a file of another size, a recipe naming another slot.
func TestLoadServerWorlds(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { SetServerWorlds(nil); worldNames[10] = "world10" })
	b := homelands()
	b.Size = 64
	j, _ := json.Marshal(b)
	if got := LoadServerWorlds(dir); len(got) != 0 {
		t.Fatalf("an empty folder: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.ESF"), make([]byte, 63), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadServerWorlds(dir); len(got) != 0 || worldName(10) != "world10" {
		t.Fatalf("a file of another size: %v, %q", got, worldName(10))
	}
	if err := os.WriteFile(filepath.Join(dir, "world10.ESF"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadServerWorlds(dir)
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
	if got := LoadServerWorlds(dir); len(got) != 0 {
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

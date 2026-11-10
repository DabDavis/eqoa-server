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
	before := len(lookOpsQueued(van))
	answer(van, protocol.OpcodeServerWorlds, u32b(0))
	if got := lookOpsQueued(van); len(got) != before {
		t.Fatalf("answered 0: %v, want nothing more than %d", got, before)
	}
	answer(van, protocol.OpcodeServerWorlds, u32b(1))
	if got := lookOpsQueued(van); len(got) != before+1 || got[before] != protocol.OpcodeServerWorldsData {
		t.Fatalf("answered 1: %v, want the worlds after %d", got, before)
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

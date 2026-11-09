package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// fungusRow is a fungusman race (28, on its real body) beside the rig's hill
// giant and kerran.
func fungusRow(t *testing.T, r *creationRig) {
	t.Helper()
	if _, err := r.db.Exec(`INSERT INTO racedefs (id, name, short_name, wire_race, body_male, body_female)
		VALUES (28, 'Fungusman', 'FNG', 1, 1246595323, 1246595323)`); err != nil {
		t.Fatal(err)
	}
	if err := entity.LoadRaceDefs(r.db); err != nil {
		t.Fatal(err)
	}
}

// createWith sends 0x002B as createChar does, with tail after its own fields
// (the race options' choices, or anything else), and the record made, if one.
func (r *creationRig) createWith(s *session.Session, name string, race, class, sex int64, tail []byte) *database.CharacterRecord {
	r.t.Helper()
	payload := build(protocol.OpcodeCreateCharacter, 256, func(w *buffer.Writer) {
		w.WriteInt32(int32(len(name)))
		w.WriteBytes([]byte(name))
		for _, v := range []int64{1, race, class, sex, 1, 1, 1, 1, 0} {
			w.Write7BitEncodedInt64(v)
		}
		for i := 0; i < 7; i++ {
			w.WriteInt32(0)
		}
		w.WriteBytes(tail)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCreateCharacter)
	msg.SetData(payload)
	if err := r.h.HandleCreateCharacter(s, msg); err != nil {
		r.t.Fatal(err)
	}
	rec, err := r.store.GetByName(name)
	if err != nil {
		return nil
	}
	return rec
}

// choices is the block a client writes after 0x002B (vanilla's
// net::raceopts::choices_block): 'R', version 1, each key and step.
func choices(c ...entity.OptionChoice) []byte {
	b := []byte{'R', 1, byte(len(c))}
	for _, x := range c {
		b = append(b, byte(len(x.Key)))
		b = append(b, x.Key...)
		b = append(b, byte(x.Step))
	}
	return b
}

// A creation that chose a custom race's options: the body chosen is the
// character's model (the red fungusman's), each colour past the first a charlook
// row, a step left at the first none; the clients that asked are sent the new
// look. A creation with no block (a PS2's, any client's that was offered none)
// is made as before, its own body and no row (the control); one whose block is
// cut short, chooses a row the race has not, a step past its row's, or chooses for
// a race with no options is refused, and nothing is made. Bytes after the fields
// that are no block are none.
func TestCreateWithRaceOptions(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true, AllClasses: true})
	fungusRow(t, r)
	s := r.session()
	watcher := r.session()
	watcher.SetRaceLooks(true)
	red := r.createWith(s, "Redcap", 28, 9, 0, choices(entity.OptionChoice{Key: "body", Step: 1}, entity.OptionChoice{Key: "skin", Step: 0},
		entity.OptionChoice{Key: "cap", Step: 3}, entity.OptionChoice{Key: "growths", Step: 5}, entity.OptionChoice{Key: "bag", Step: 0}))
	if red == nil || uint32(red.ModelID) != 0x5874ACFC {
		t.Fatalf("a red fungusman: %+v", red)
	}
	looks, err := r.db.CharLooks(uint32(red.ServerID))
	if err != nil || len(looks) != 2 || looks[0].Part != "cap" || looks[1].Part != "growths" || looks[0].Kind != database.LookColour {
		t.Fatalf("its looks: %+v %v", looks, err)
	}
	if d := sentOp(watcher, protocol.OpcodeRaceLooks); d == nil || !bytes.Contains(d, []byte("Redcap")) {
		t.Fatalf("the watcher was not told the look: %x", d)
	}

	plain := r.createWith(s, "Bluecap", 28, 9, 0, nil)
	if plain == nil || uint32(plain.ModelID) != 0x4A4D88FB {
		t.Fatalf("no block: %+v", plain)
	}
	if l, _ := r.db.CharLooks(uint32(plain.ServerID)); len(l) != 0 {
		t.Fatalf("no block: looks %+v", l)
	}
	junk := r.createWith(s, "Junkcap", 28, 9, 0, []byte{0, 0, 0, 0})
	if junk == nil || uint32(junk.ModelID) != 0x4A4D88FB {
		t.Fatalf("bytes that are no block: %+v", junk)
	}
	for name, tail := range map[string][]byte{
		"Cutcap":   choices(entity.OptionChoice{Key: "body", Step: 1})[:5],
		"Wingcap":  choices(entity.OptionChoice{Key: "wings", Step: 1}),
		"Farcap":   choices(entity.OptionChoice{Key: "body", Step: 3}),
		"Versicap": append([]byte{'R', 2}, choices(entity.OptionChoice{Key: "body", Step: 1})[2:]...),
	} {
		if rec := r.createWith(s, name, 28, 9, 0, tail); rec != nil {
			t.Errorf("%s: made %+v", name, rec)
		}
	}
	if rec := r.createWith(s, "Gnomey", 3, 13, 0, choices(entity.OptionChoice{Key: "body", Step: 0})); rec != nil {
		t.Errorf("a gnome with a choice: made %+v", rec)
	}
	if d := sentOp(s, protocol.OpcodeCreateCharFailed); d == nil {
		t.Error("no refusal sent")
	}
}

// A PS2's creation of the game's own race is the bytes it always was, made as it
// always was; and the character list a PS2 is sent is the list it always was: a
// character's charlook rows add nothing to its record, only a chosen body is its
// model (the control: the same character with no row, byte for byte).
func TestARaceLookIsNothingAPS2IsSent(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true, AllClasses: true})
	fungusRow(t, r)
	s := r.session()
	if rec := r.createWith(s, "Cappy", 28, 9, 0, nil); rec == nil {
		t.Fatal("no fungusman")
	}
	list := func() []byte {
		ps2 := r.session()
		r.h.SendCharacterSelectEQOA(ps2)
		return sentOp(ps2, protocol.OpcodeCharacterSelect)
	}
	before := list()
	rec, _ := r.store.GetByName("Cappy")
	if err := r.db.SetCharLooks(uint32(rec.ServerID), []database.CharLook{{Part: "cap", Kind: database.LookColour, Value: entity.Dye(0, 50, 100)}}); err != nil {
		t.Fatal(err)
	}
	if after := list(); !bytes.Equal(before, after) {
		t.Fatalf("a look changed a PS2's list:\n%x\n%x", before, after)
	}
}

// The offer: once a session, after the first list, on a server whose races have
// options; a client that answers 1 is sent the options (once a session) and the
// looks, 0 nothing more; a PS2, which never answers, is sent neither, nor any list
// a creation refreshes. The control: a server with no custom race offers nothing.
func TestRaceOptionsOfferAndItsGate(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	fungusRow(t, r)
	h := NewRaceOptionsHandler(r.h.gameWorld, r.db)
	answer := func(s *session.Session, v uint32) {
		t.Helper()
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceOptions)
		m.SetData(u32b(v))
		if err := h.HandleRaceOptions(s, m); err != nil {
			t.Fatal(err)
		}
	}
	ps2, van := r.session(), r.session()
	for _, s := range []*session.Session{ps2, van} {
		r.h.SendCharacterSelectEQOA(s)
		r.h.SendCharacterSelectEQOA(s)
		n := 0
		for _, op := range lookOpsQueued(s) {
			if op == protocol.OpcodeRaceOptions {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%d offers after two lists, want one", n)
		}
	}
	answer(van, 0)
	if sentOp(van, protocol.OpcodeRaceOptionsData) != nil || sentOp(van, protocol.OpcodeRaceLooks) != nil {
		t.Fatal("answered 0: sent the lists")
	}
	before := len(van.Queue().PeekPending())
	answer(van, 1)
	// the options (past a message's span: in fragments, put together here), then the looks
	var whole []byte
	pending := van.Queue().PeekPending()[before:]
	for _, m := range pending[:len(pending)-1] {
		whole = append(whole, m.Data()...)
	}
	if want := append([]byte{0x23, 0x7E}, entity.EncodeRaceOptions()...); !bytes.Equal(whole, want) {
		t.Fatalf("answered 1: the options, %d bytes in %d messages, want %d", len(whole), len(pending)-1, len(want))
	}
	if d := pending[len(pending)-1].Data(); opcode(d) != protocol.OpcodeRaceLooks || binary.LittleEndian.Uint16(d[3:]) != 0 {
		t.Fatalf("answered 1: the looks %x", d)
	}
	before = len(lookOpsQueued(van))
	answer(van, 1)
	if got := lookOpsQueued(van); len(got) != before {
		t.Fatalf("asked again: %v sent again", got[before:])
	}
	if rec := r.createWith(van, "Spore", 28, 9, 0, choices(entity.OptionChoice{Key: "cap", Step: 2})); rec == nil {
		t.Fatal("no fungusman")
	}
	if d := sentOp(van, protocol.OpcodeRaceLooks); !bytes.Contains(d, []byte("Spore")) {
		t.Fatalf("after its creation: %x", d)
	}
	for _, op := range lookOpsQueued(ps2) {
		if op == protocol.OpcodeRaceOptionsData || op == protocol.OpcodeRaceLooks {
			t.Fatalf("a PS2 was sent %v", op)
		}
	}

	none := newCreationRig(t, entity.CreationRules{})
	if _, err := none.db.Exec(`DELETE FROM racedefs`); err != nil {
		t.Fatal(err)
	}
	entity.SetRaceDefs(nil)
	s := none.session()
	none.h.SendCharacterSelectEQOA(s)
	for _, op := range lookOpsQueued(s) {
		if op == protocol.OpcodeRaceOptions {
			t.Fatal("the control: a server with no custom race offered race options")
		}
	}
}

// The choices block as vanilla writes it: read whole; none when the bytes do
// not open with the mark; refused cut anywhere or of another version.
func TestRaceOptionsBlock(t *testing.T) {
	b := choices(entity.OptionChoice{Key: "body", Step: 2}, entity.OptionChoice{Key: "cap", Step: 1})
	c, present, ok := raceOptionsBlock(b)
	if !present || !ok || len(c) != 2 || c[0] != (entity.OptionChoice{Key: "body", Step: 2}) || c[1] != (entity.OptionChoice{Key: "cap", Step: 1}) {
		t.Fatalf("%v %v %v", c, present, ok)
	}
	for n := 1; n < len(b); n++ {
		if _, present, ok := raceOptionsBlock(b[:n]); !present || ok {
			t.Errorf("cut at %d: present %v ok %v", n, present, ok)
		}
	}
	if _, present, ok := raceOptionsBlock(nil); present || !ok {
		t.Error("none")
	}
	if _, present, _ := raceOptionsBlock([]byte{0, 1, 2}); present {
		t.Error("bytes that are no block")
	}
	if _, _, ok := raceOptionsBlock(append(b, 0)); ok {
		t.Error("a byte past the block")
	}
}

// What a client asked for at character select goes again at a login's world
// entry (resendPregameExtras, from SendMemoryDump): creation's races, the races'
// options and the looks, which went on the lobby's connection and were lost with
// it when the client selected before they all came (a Wisp drawn as a plain
// human). A client that answered the race options 0 is sent creation's races
// alone; a PS2, which answered nothing, none of the three. The control: before
// it the looks already sent were never sent again (TakeRaceLooks).
func TestPregameExtrasAgainAtWorldEntry(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Races: true})
	fungusRow(t, r)
	opts := NewRaceOptionsHandler(r.h.gameWorld, r.db)
	cre := NewCreationHandler()
	answer := func(s *session.Session, op protocol.GameOpcode, v uint32) {
		t.Helper()
		m := protocol.NewMessage(protocol.MessageTypeReliable, op)
		m.SetData(u32b(v))
		var err error
		if op == protocol.OpcodeRaceOptions {
			err = opts.HandleRaceOptions(s, m)
		} else {
			err = cre.HandleCreationOffer(s, m)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	since := func(s *session.Session, from int) map[protocol.GameOpcode]int {
		n := map[protocol.GameOpcode]int{}
		for _, op := range lookOpsQueued(s)[from:] {
			n[op]++
		}
		return n
	}
	van, off, ps2 := r.session(), r.session(), r.session()
	answer(van, protocol.OpcodeCreationOffer, 1)
	answer(van, protocol.OpcodeRaceOptions, 1)
	answer(off, protocol.OpcodeCreationOffer, 1)
	answer(off, protocol.OpcodeRaceOptions, 0)
	if !van.CreationAsked() || ps2.CreationAsked() {
		t.Fatalf("asked: vanilla %v, a PS2 %v", van.CreationAsked(), ps2.CreationAsked())
	}
	// the control: the looks already sent do not go again by themselves
	before := len(lookOpsQueued(van))
	if r.h.gameWorld.RefreshRaceLooks(r.db, van) != 0 || len(lookOpsQueued(van)) != before {
		t.Fatal("the control: the same looks sent again")
	}
	for _, c := range []struct {
		name                     string
		s                        *session.Session
		creation, options, looks bool
	}{{"vanilla", van, true, true, true}, {"options off", off, true, false, false}, {"a PS2", ps2, false, false, false}} {
		before := len(lookOpsQueued(c.s))
		r.h.resendPregameExtras(c.s)
		got := since(c.s, before)
		if (got[protocol.OpcodeCreationRaces] == 1) != c.creation || (got[protocol.OpcodeRaceOptionsData] >= 1) != c.options || (got[protocol.OpcodeRaceLooks] == 1) != c.looks {
			t.Errorf("%s at world entry: %v, want creation %v, options %v, looks %v", c.name, got, c.creation, c.options, c.looks)
		}
	}
}

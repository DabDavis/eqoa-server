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

// creationRig is a database with two races of racedefs (a giant passing as an
// ogre, a cat folk as a western human with a description) and a list handler.
type creationRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	store *database.SQLiteCharacterStore
	h     *CharacterHandler
	sm    *session.Manager
	next  byte
}

func newCreationRig(t *testing.T, rules entity.CreationRules) *creationRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/creation.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO racedefs (id, name, short_name, wire_race, bonus_str, bonus_sta, bonus_agi, bonus_dex, body_male, body_female)
		 VALUES (11, 'Hill Giant', 'HGT', 9, 25, 20, -5, -5, -5117396, -5117396)`,
		`INSERT INTO racedefs (id, name, short_name, wire_race, bonus_str, bonus_sta, bonus_agi, bonus_dex, body_male, body_female, description, city)
		 VALUES (12, 'Kerran', 'KER', 0, 5, 5, 15, 10, 572940888, -1219903657, 'Cat folk.', 2)`,
		`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		 VALUES (0, 0, 2, 1, 0, 0, 0, 0, 4243.7, 58.1, 17169.1, 0, 0), (0, 9, 0, 1, 0, 0, 0, 0, 9377.7, 53.2, 7385.8, 0, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := entity.LoadRaceDefs(db); err != nil {
		t.Fatal(err)
	}
	entity.SetCreationRules(rules)
	t.Cleanup(func() { entity.SetRaceDefs(nil); entity.SetCreationRules(entity.CreationRules{}) })
	store := database.NewSQLiteCharacterStore(db)
	sm := session.NewManager()
	w := entity.NewWorld()
	w.SessionManager = sm
	return &creationRig{t: t, db: db, store: store, sm: sm, h: &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}}
}

// session is a client at character select of account 5.
func (r *creationRig) session() *session.Session {
	r.next++
	s := r.sm.CreateSession(testAddr(40+r.next), uint32(40+int(r.next)), 1, 2, 3, true)
	s.SetAccountID(5)
	s.SetState(session.StateCharacterSelect)
	return s
}

// answer is the client's OpcodeCreationOffer.
func (r *creationRig) answer(s *session.Session, v uint32) {
	r.t.Helper()
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCreationOffer)
	m.SetData(u32b(v))
	if err := NewCreationHandler().HandleCreationOffer(s, m); err != nil {
		r.t.Fatal(err)
	}
}

// Creation's offer comes once a session, after the first list and the live
// look's, only while creation takes something past the game's; a client that
// answers 1 is sent the races as they stand, 0 nothing; a PS2, which never
// answers, is sent nothing more. The race options' offer comes last, its races'
// bodies having options (raceoptions_test.go has its own controls). The control:
// a server with neither switch offers no creation, so its lists go as they did.
func TestCreationOfferAfterTheFirstList(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{})
	off := r.session()
	r.h.SendCharacterSelectEQOA(off)
	if got := lookOpsQueued(off); len(got) != 3 || got[1] != protocol.OpcodeLiveLook || got[2] != protocol.OpcodeRaceOptions {
		t.Fatalf("switched off: %v, want the list, the live look's offer and the race options'", got)
	}

	r = newCreationRig(t, entity.CreationRules{Races: true, AllClasses: true})
	ps2 := r.session()
	r.h.SendCharacterSelectEQOA(ps2)
	want := []protocol.GameOpcode{protocol.OpcodeCharacterSelect, protocol.OpcodeLiveLook, protocol.OpcodeCreationOffer, protocol.OpcodeRaceOptions}
	if got := lookOpsQueued(ps2); len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("a first list: %v, want %v", got, want)
	}
	if d := sentOp(ps2, protocol.OpcodeCreationOffer); len(d) != 6 || binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	r.h.SendCharacterSelectEQOA(ps2)
	if got := lookOpsQueued(ps2); len(got) != 5 || got[4] != protocol.OpcodeCharacterSelect {
		t.Fatalf("a second list: %v, want no second offer", got)
	}

	van := r.session()
	r.h.SendCharacterSelectEQOA(van)
	r.answer(van, 0)
	if got := lookOpsQueued(van); len(got) != 4 {
		t.Fatalf("answered 0: %v", got)
	}
	r.answer(van, 1)
	got := lookOpsQueued(van)
	if len(got) != 5 || got[4] != protocol.OpcodeCreationRaces {
		t.Fatalf("answered 1: %v", got)
	}
	body := sentOp(van, protocol.OpcodeCreationRaces)[2:]
	if !bytes.Equal(body, encodeCreationRaces(entity.Creation(), entity.CreationRaces())) {
		t.Fatalf("the races sent: %x", body)
	}
	if body[0] != 1 || body[1] != 1 || body[2] != 3 {
		t.Fatalf("version, flags, count: %v", body[:3])
	}
}

// The races' bytes, field by field: the froglok first, then the rows.
func TestCreationRacesFormat(t *testing.T) {
	races := []entity.CreationRace{
		{ID: 10, Like: 0, City: 1, Name: "Froglok", Short: "FRG", Desc: "Frogs.", Bonus: [7]int{5, 5, 5, 5, 5, 5, 5}, ClassMask: 0x7E69,
			Body: [2]int64{0x141BFE0C, 0x141BFE0C}},
		{ID: 11, Like: 9, Name: "Hill Giant", Short: "HGT", Bonus: [7]int{25, 20, -5, -5}, ClassMask: 0x909, Body: [2]int64{-5117396, -5117396}},
	}
	got := encodeCreationRaces(entity.CreationRules{Races: true}, races)
	want := []byte{1, 0, 2}
	want = append(want, 10, 0, 1, 5, 5, 5, 5, 5, 5, 5, 0x69, 0x7E, 0x0C, 0xFE, 0x1B, 0x14, 0x0C, 0xFE, 0x1B, 0x14)
	want = append(want, 7, 'F', 'r', 'o', 'g', 'l', 'o', 'k', 3, 'F', 'R', 'G', 6, 0, 'F', 'r', 'o', 'g', 's', '.')
	want = append(want, 11, 9, 0, 25, 20, 0xFB, 0xFB, 0, 0, 0, 0x09, 0x09, 0x2C, 0xEA, 0xB1, 0xFF, 0x2C, 0xEA, 0xB1, 0xFF)
	want = append(want, 10, 'H', 'i', 'l', 'l', ' ', 'G', 'i', 'a', 'n', 't', 3, 'H', 'G', 'T', 0, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got %x\nwant %x", got, want)
	}
	// every class for every race: bit 0 of the flags (the control)
	if b := encodeCreationRaces(entity.CreationRules{AllClasses: true}, nil); !bytes.Equal(b, []byte{1, 1, 0}) {
		t.Fatalf("all classes, no races: %x", b)
	}
}

// createChar sends 0x002B for a new character as a client does, and its record.
func (r *creationRig) createChar(s *session.Session, name string, race, class, sex, city int64) *database.CharacterRecord {
	r.t.Helper()
	payload := build(protocol.OpcodeCreateCharacter, 128, func(w *buffer.Writer) {
		w.WriteInt32(int32(len(name)))
		w.WriteBytes([]byte(name))
		for _, v := range []int64{1, race, class, sex, 1, 1, 1, 1, city} {
			w.Write7BitEncodedInt64(v)
		}
		for i := 0; i < 7; i++ {
			w.WriteInt32(0)
		}
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

// A race of racedefs is made through the game's own creation (0x002B) only
// while creation takes it: in its row's body by sex, on its wire race's start
// spot (a human-passing race's city), refused otherwise with the creation
// refusal; the game's races as before.
func TestCreateACharacterOfARaceOfRacedefs(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{})
	s := r.session()
	if rec := r.createChar(s, "Biggy", 11, 0, 0, 0); rec != nil {
		t.Fatalf("switched off, a giant was made: %+v", rec)
	}
	if d := sentOp(s, protocol.OpcodeCreateCharFailed); d == nil {
		t.Fatalf("switched off: no refusal sent: %v", lookOpsQueued(s))
	}
	if rec := r.createChar(s, "Smally", 3, 13, 0, 0); rec == nil {
		t.Fatal("switched off, a gnome wizard was refused")
	}

	r = newCreationRig(t, entity.CreationRules{Races: true})
	s = r.session()
	if rec := r.createChar(s, "Biggywiz", 11, 13, 0, 0); rec != nil {
		t.Fatalf("a giant wizard, the ogre's classes only: made %+v", rec)
	}
	giant := r.createChar(s, "Biggy", 11, 0, 1, 0)
	if giant == nil || giant.Race != 11 || giant.ModelID != -5117396 || giant.World != 1 || giant.X != 9377.7 {
		t.Fatalf("a giant warrior: %+v", giant)
	}
	r = newCreationRig(t, entity.CreationRules{Races: true, AllClasses: true})
	s = r.session()
	kat := r.createChar(s, "Kitty", 12, 13, 1, 0)
	if kat == nil || kat.Race != 12 || kat.TClass != 13 || kat.ModelID != -1219903657 || kat.X != 4243.7 || kat.Z != 17169.1 {
		t.Fatalf("a female cat folk wizard: %+v", kat)
	}
	tom := r.createChar(s, "Tomcat", 12, 0, 0, 0)
	if tom == nil || tom.ModelID != 572940888 {
		t.Fatalf("a male cat folk: %+v", tom)
	}
	frog := r.createChar(s, "Ribbit", 10, 1, 0, 0)
	if frog == nil || frog.Race != 10 || frog.ModelID != entity.FroglokBody {
		t.Fatalf("a froglok ranger: %+v", frog)
	}
	if rec := r.createChar(s, "Nobody", 14, 0, 0, 0); rec != nil {
		t.Fatalf("a race with no row: %+v", rec)
	}
}

// The account page's rule for a race past the game's ten: creation's.
func TestNewCharRace(t *testing.T) {
	newCreationRig(t, entity.CreationRules{Races: true})
	if m, why := NewCharRace(11, 0, 0); why != "" || m != -5117396 {
		t.Errorf("a giant warrior: %#x %q", m, why)
	}
	if _, why := NewCharRace(11, 13, 0); why == "" {
		t.Error("a giant wizard taken with every class closed")
	}
	if m, why := NewCharRace(10, 3, 1); why != "" || m != entity.FroglokBody {
		t.Errorf("a froglok: %#x %q", m, why)
	}
	if _, why := NewCharRace(3, 0, 0); why == "" {
		t.Error("the control: a race of the game's is the page's own")
	}
	newCreationRig(t, entity.CreationRules{})
	if _, why := NewCharRace(11, 0, 0); why == "" {
		t.Error("switched off: a giant taken")
	}
}

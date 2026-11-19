package handlers

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// [OG] characters (database/ogchar.go, the owner's of 2026-11-19: "lets make
// [OG] status a char tag like [HC]"), driven as a server drives them. The
// people are made up.

// The offer carries the shard's choices as bits (1 Hardcore, 2 [OG]); a
// client's answer with the [OG] bit is an [OG] client, an older vanilla's 1 is
// not (its [OG] choice is asked in the game); the list's states carry [OG] (8)
// and an open [OG] choice (16), and a PS2's list carries "[OG] Name".
func TestOGOfferAndList(t *testing.T) {
	hardcoreOnFor(t)
	db, store, ids := hcDB(t)
	if _, err := db.ChooseOG(ids[0], true); err != nil { // Plain: [OG]
		t.Fatal(err)
	}
	sm := session.NewManager()
	h := &CharacterHandler{store: store, inventoryStore: database.NewInventoryStore(store.DB()), sessionManager: sm, gameWorld: entity.NewWorld()}
	n := byte(60)
	mk := func() *session.Session {
		n++
		s := sm.CreateSession(testAddr(n), uint32(n), uint32(n), uint16(n), uint16(n), true)
		s.SetAccountID(5)
		s.SetState(session.StateCharacterSelect)
		return s
	}
	offer := func(s *session.Session) uint32 {
		t.Helper()
		h.SendCharacterSelectEQOA(s)
		d := sentOp(s, protocol.OpcodeHardcore)
		if len(d) != 6 {
			return 0
		}
		return binary.LittleEndian.Uint32(d[2:])
	}
	answer := func(s *session.Session, v uint32) {
		t.Helper()
		if err := NewHardcoreHandler(h).HandleHardcore(s, incoming(protocol.OpcodeHardcore, u32b(v))); err != nil {
			t.Fatal(err)
		}
	}
	// og off: the offer is Hardcore's alone, an answer's [OG] bit not taken
	s := mk()
	if v := offer(s); v != 1 {
		t.Fatalf("Hardcore alone: offer %d", v)
	}
	answer(s, 3)
	if !s.HardcoreClient() || s.OGClient() {
		t.Fatalf("og off: hardcore client %v, og client %v", s.HardcoreClient(), s.OGClient())
	}
	ogSwitch(t)
	s = mk()
	if v := offer(s); v != 3 {
		t.Fatalf("both: offer %d", v)
	}
	ps2Names := sentOp(s, protocol.OpcodeCharacterSelect)
	for _, want := range []string{"[OG] Plain", "[HC] Grim"} {
		enc := append(binary.LittleEndian.AppendUint32(nil, uint32(len(want))), want...)
		if !strings.Contains(string(ps2Names), string(enc)) {
			t.Errorf("a PS2's list lacks %q", want)
		}
	}
	old := mk()
	offer(old)
	answer(old, 1)
	if !old.HardcoreClient() || old.OGClient() {
		t.Fatalf("an older vanilla: hardcore client %v, og client %v", old.HardcoreClient(), old.OGClient())
	}
	van := mk()
	offer(van)
	answer(van, 3)
	if !van.HardcoreClient() || !van.OGClient() {
		t.Fatalf("vanilla: hardcore client %v, og client %v", van.HardcoreClient(), van.OGClient())
	}
	b := sentOp(van, protocol.OpcodeHardcoreChars)[2:]
	if b[0] != 1 || binary.LittleEndian.Uint16(b[1:]) != 3 {
		t.Fatalf("states % x", b)
	}
	// Plain's entry first: id, "Plain", flags
	if id := binary.LittleEndian.Uint32(b[3:]); id != ids[0] || string(b[8:8+5]) != "Plain" || b[13] != hcFlagOG {
		t.Fatalf("Plain's entry: % x", b[3:14])
	}
	// [OG] alone: the offer is 2
	SetHardcore(config.HardcoreConfig{})
	if v := offer(mk()); v != 2 {
		t.Fatalf("[OG] alone: offer %d", v)
	}
	SetOGAccounts(false)
	if v := offer(mk()); v != 0 {
		t.Fatalf("neither: offer %d", v)
	}
}

// A creation from a client that took the [OG] bit keeps its choice (no box at
// its entry); one from a PS2, or an older vanilla (Hardcore only), has its
// [OG] choice open; an 'O' block from a client never offered it is a standard
// character; a cut block is refused. A new character on an account that was
// [OG] is not [OG] unless it chose to be.
func TestOGChosenAtCreation(t *testing.T) {
	hardcoreOnFor(t)
	ogSwitch(t)
	db, err := database.NewSQLite(t.TempDir() + "/create.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	acc, err := database.NewSQLiteAccountStore(db).Create("oldog", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountOG(acc.ID, true); err != nil {
		t.Fatal(err)
	}
	store := database.NewSQLiteCharacterStore(db)
	h := NewCharacterHandler(store, nil, nil, database.NewSpellbookStore(db))
	create := func(s *session.Session, name string, tail []byte) {
		w := buffer.NewWriter(make([]byte, 256))
		w.WriteInt32(int32(len(name)))
		w.WriteBytes([]byte(name))
		for _, v := range []int64{1, 0, 0, 0, 1, 1, 1, 1, 1} {
			w.Write7BitEncodedInt64(v)
		}
		for i := 0; i < 7; i++ {
			w.WriteInt32(0)
		}
		data := append(append([]byte(nil), w.Bytes()[:w.Position()]...), tail...)
		if err := h.HandleCreateCharacter(s, incoming(protocol.OpcodeCreateCharacter, data)); err != nil {
			t.Fatal(err)
		}
	}
	state := func(name string) (database.HardcoreState, bool) {
		rec, err := store.GetByName(name)
		if err != nil {
			return database.HardcoreState{}, false
		}
		st, _ := db.HardcoreOf(uint32(rec.ServerID))
		return st, true
	}
	van := testSession()
	van.SetAccountID(acc.ID)
	van.OfferHardcoreOnce()
	van.SetHardcoreClient(true)
	van.SetOGClient(true)
	create(van, "Oggy", []byte{'H', 1, 0, 'O', 1, 1})
	create(van, "Hardog", []byte{'H', 1, 1, 'O', 1, 1})
	create(van, "Plainy", []byte{'H', 1, 0, 'O', 1, 0})
	oldVan := testSession()
	oldVan.SetAccountID(acc.ID)
	oldVan.OfferHardcoreOnce()
	oldVan.SetHardcoreClient(true)
	create(oldVan, "Oldvan", []byte{'H', 1, 0})
	create(oldVan, "Sneakvan", []byte{'H', 1, 0, 'O', 1, 1})
	ps2 := testSession()
	ps2.SetAccountID(acc.ID)
	create(ps2, "Consoley", nil)
	create(van, "Cutty", []byte{'H', 1, 0, 'O', 1})
	for name, want := range map[string][4]bool{ // og, og chosen, og open, hardcore
		"Oggy": {true, true, false, false}, "Hardog": {true, true, false, true}, "Plainy": {false, true, false, false},
		"Oldvan": {false, false, true, false}, "Sneakvan": {false, false, true, false}, "Consoley": {false, false, true, false},
	} {
		st, ok := state(name)
		if !ok || [4]bool{st.OG, st.OGChosen, st.OGChoiceOpen(), st.Hardcore} != want {
			t.Errorf("%s: made %v, %+v, want %v", name, ok, st, want)
		}
	}
	if _, ok := state("Cutty"); ok {
		t.Error("a cut block made a character")
	}
}

// ogAskRig is hcAskRig's with [OG] on and the asker's later calls kept.
type ogAskRig struct {
	*hcAskRig
	later []func()
}

func newOGAskRig(t *testing.T) *ogAskRig {
	t.Helper()
	r := &ogAskRig{hcAskRig: newWelcomeRigLike(t)}
	ogSwitch(t)
	r.ask.later = func(_ time.Duration, f func()) { r.later = append(r.later, f) }
	return r
}

// run runs the asker's waits so far.
func (r *ogAskRig) run() {
	fs := r.later
	r.later = nil
	for _, f := range fs {
		f()
	}
}

// ogBox is the one [OG] box s was sent.
func (r *ogAskRig) ogBox(s *session.Session, question string, answers ...string) uint32 {
	r.t.Helper()
	id := r.box(s, question, answers...)
	if id&0xFF000000 != ogBoxMark {
		r.t.Fatalf("box %#x is not an [OG] box", id)
	}
	return id
}

// A PS2's new character with both choices open is asked Hardcore first and
// [OG] only after Hardcore's last answer, never both at once; two steps to
// [OG], its session told at once (the game's own UI, the tag); a No for good,
// a Close leaves it open, "/t og" asks again; one played on is not asked; with
// world.og_accounts off nothing is asked.
func TestOGAskedAfterHardcore(t *testing.T) {
	r := newOGAskRig(t)
	// Anna: Hardcore's box first, and nothing of [OG] while it is up
	r.ask.Entered(r.anna, r.pa)
	if len(r.later) != 1 || len(r.msgs(r.anna)) != 0 {
		t.Fatalf("at entry: %d waits", len(r.later))
	}
	r.run()
	box := r.box(r.anna, "", hardcoreYes1, hardcoreNo1)
	if box&0xFF000000 != hardcoreBoxMark {
		t.Fatalf("first box %#x: not Hardcore's", box)
	}
	r.pick(r.anna, box, 1) // a normal character
	if got := r.msgs(r.anna); len(got) != 1 || boxID(got) != 0 {
		t.Fatalf("after Hardcore's No: %q (the [OG] box with it?)", got)
	}
	if len(r.later) != 1 {
		t.Fatalf("the [OG] box not waited for: %d", len(r.later))
	}
	r.run()
	box = r.ogBox(r.anna, "Make Anna an [OG] character? Basic upgrades only: no map, minimap or item links, no mouse or touch. Permanent.", ogYes1, ogNo1)
	r.pick(r.anna, box, 0)
	box = r.ogBox(r.anna, "Are you sure? Anna will be [OG] for good, drawn [OG] Anna: the game as it was, with no way back.", ogYes2, ogNo2)
	r.pick(r.anna, box, 0)
	if st, _ := r.db.HardcoreOf(r.pa.CharServerID); !st.OG || !st.OGChosen || st.Hardcore {
		t.Errorf("Anna: %+v", st)
	}
	if !r.anna.OG() || !r.pa.OG || r.pa.ShownName() != "[OG] Anna" || QoLOnFor(r.anna) {
		t.Errorf("Anna's session %v, player %v, drawn %q", r.anna.OG(), r.pa.OG, r.pa.ShownName())
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Anna is now an [OG] character*") {
		t.Errorf("Anna told %q", got)
	}

	// Cora: Hardcore chosen at creation, so [OG] is asked at entry; Close,
	// then "/t og", then No
	if _, err := r.db.ChooseHardcore(r.pc.CharServerID, true); err != nil {
		t.Fatal(err)
	}
	r.ask.Entered(r.cora, r.pc)
	r.run()
	box = r.ogBox(r.cora, "", ogYes1, ogNo1)
	r.now = r.now.Add(time.Minute)
	r.pick(r.cora, box, 255)
	if got := r.msgs(r.cora); !hasMsg(got, "chat:You can still make Cora [OG] while it is new*") {
		t.Errorf("Cora's Close: %q", got)
	}
	if st, _ := r.db.HardcoreOf(r.pc.CharServerID); !st.OGChoiceOpen() {
		t.Errorf("a Close closed the choice: %+v", st)
	}
	r.say(r.cora, "/t og")
	box = r.ogBox(r.cora, "", ogYes1, ogNo1)
	r.pick(r.cora, box, 1)
	if st, _ := r.db.HardcoreOf(r.pc.CharServerID); st.OG || !st.OGChosen {
		t.Errorf("Cora's No: %+v", st)
	}
	r.msgs(r.cora)
	r.say(r.cora, "/t og")
	if got := r.msgs(r.cora); !hasMsg(got, "chat:Cora is a standard character: the [OG] choice is made when a character is new.") {
		t.Errorf("Cora's /og after: %q", got)
	}
	if r.cora.OG() {
		t.Error("Cora's session is [OG]")
	}

	// Bert has earned experience: never asked either
	if _, err := r.db.Exec(`UPDATE characters SET totalXP = 10 WHERE serverid = ?`, r.pb.CharServerID); err != nil {
		t.Fatal(err)
	}
	r.ask.Entered(r.bert, r.pb)
	if len(r.later) != 0 {
		t.Fatal("Bert asked")
	}
	// the control: world.og_accounts off, nothing of [OG]
	SetOGAccounts(false)
	r.say(r.bert, "/og")
	if got := r.msgs(r.bert); !hasMsg(got, "red:There are no [OG] characters on this server.") {
		t.Errorf("Bert's /og with it off: %q", got)
	}
}

// Every [OG] question fits CLIENT's 127 characters with the longest name, and
// carries what [OG] gives up; the answers fit 63.
func TestOGQuestionsFit(t *testing.T) {
	for _, name := range []string{"", "Bob", "Meshuggahhhhhhh"} {
		for _, second := range []bool{false, true} {
			q, answers := ogQuestion(name, second)
			if len([]rune(q)) > questAskChars || strings.ContainsAny(q, "%|") {
				t.Errorf("%q %v: %d characters: %q", name, second, len([]rune(q)), q)
			}
			if name == "Meshuggahhhhhhh" && !strings.Contains(q, name) {
				t.Errorf("a full name left out: %q", q)
			}
			if !second && !strings.Contains(q, "no map, minimap or item links, no mouse or touch") {
				t.Errorf("the first box does not say what [OG] gives up: %q", q)
			}
			for _, a := range answers {
				if len(a) > 63 {
					t.Errorf("answer %q", a)
				}
			}
		}
	}
}

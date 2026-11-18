package handlers

import (
	"bytes"
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

// [HC] Hardcore (hardcore.go, hardcoreask.go, memorial.go) driven as a server
// drives it. The people are made up.

func hardcoreOnFor(t *testing.T) {
	t.Helper()
	SetHardcore(config.HardcoreConfig{Enabled: true, Shard: "Heirloom"})
	t.Cleanup(func() { SetHardcore(config.HardcoreConfig{}) })
}

// hcDB is a database with an account's three characters: Plain (normal),
// Grim (Hardcore) and Fallow (Hardcore, fallen at 2026-11-18 08:45 UTC).
func hcDB(t *testing.T) (*database.SQLiteDB, *database.SQLiteCharacterStore, [3]uint32) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/hardcore.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	var ids [3]uint32
	for i, name := range []string{"Plain", "Grim", "Fallow"} {
		rec := database.CharacterRecord{CharName: name, AccountID: 5, Level: 20, Race: 0, TClass: 3}
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		ids[i] = rec.ServerID
	}
	for _, id := range ids[1:] {
		if _, err := db.ChooseHardcore(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if fell, err := db.Fall(database.FallRecord{ServerID: ids[2], At: hcFallAt.Unix(), Cause: entity.CauseCreature,
		Killer: "a Zek sentry", Place: "the wilds near Freeport", Level: 52, Class: 3, CMs: 140}); err != nil || !fell {
		t.Fatalf("fall: %v %v", fell, err)
	}
	return db, store, ids
}

var hcFallAt = time.Date(2026, 11, 18, 8, 45, 0, 0, time.UTC)

// A PS2's list carries the states in the names; vanilla, once it took the
// offer, is sent each character's state with its plain name, and its list is
// plain. The offer goes once, after the list.
func TestHardcoreListAndOffer(t *testing.T) {
	hardcoreOnFor(t)
	_, store, ids := hcDB(t)
	sm := session.NewManager()
	h := &CharacterHandler{store: store, inventoryStore: database.NewInventoryStore(store.DB()), sessionManager: sm, gameWorld: entity.NewWorld()}
	mk := func(n byte) *session.Session {
		s := sm.CreateSession(testAddr(n), uint32(n), uint32(n), uint16(n), uint16(n), true)
		s.SetAccountID(5)
		s.SetState(session.StateCharacterSelect)
		return s
	}
	// names is the list's names in order, read by their int32 lengths: each
	// name is found by the length before it
	names := func(list []byte) []string {
		var out []string
		for _, n := range []string{"Plain", "Grim", "Fallow"} {
			for _, cand := range []string{n, "[HC] " + n, n + " RIP 11/18 08:45"} {
				enc := append(binary.LittleEndian.AppendUint32(nil, uint32(len(cand))), cand...)
				if bytes.Contains(list, enc) {
					out = append(out, cand)
					break
				}
			}
		}
		return out
	}
	ps2 := mk(50)
	h.SendCharacterSelectEQOA(ps2)
	list := sentOp(ps2, protocol.OpcodeCharacterSelect)
	if got := strings.Join(names(list), ","); got != "Plain,[HC] Grim,Fallow RIP 11/18 08:45" {
		t.Errorf("a PS2's list: %q", got)
	}
	ps2.Queue().GetPending()
	h.SendCharacterSelectEQOA(ps2)
	if hasOp(ps2, protocol.OpcodeHardcore) || hasOp(ps2, protocol.OpcodeHardcoreChars) {
		t.Error("a second offer, or states to a PS2")
	}

	van := mk(51)
	h.SendCharacterSelectEQOA(van)
	if d := sentOp(van, protocol.OpcodeHardcore); len(d) != 6 || binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	if err := NewHardcoreHandler(h).HandleHardcore(van, incoming(protocol.OpcodeHardcore, u32b(1))); err != nil {
		t.Fatal(err)
	}
	b := sentOp(van, protocol.OpcodeHardcoreChars)[2:]
	if b[0] != 1 || binary.LittleEndian.Uint16(b[1:]) != 3 {
		t.Fatalf("states % x", b)
	}
	at := 3
	str := func() string { l := int(b[at]); s := string(b[at+1 : at+1+l]); at += 1 + l; return s }
	type ent struct {
		id            uint32
		name          string
		flags         byte
		when          uint32
		level         byte
		cms           uint16
		killer, place string
	}
	var got []ent
	for range 3 {
		e := ent{id: binary.LittleEndian.Uint32(b[at:])}
		at += 4
		e.name = str()
		e.flags = b[at]
		at++
		if e.flags&hcFlagFallen != 0 {
			e.when = binary.LittleEndian.Uint32(b[at:])
			e.level = b[at+4]
			e.cms = binary.LittleEndian.Uint16(b[at+5:])
			at += 7
			e.killer, e.place = str(), str()
		}
		got = append(got, e)
	}
	if at != len(b) {
		t.Fatalf("%d of %d bytes", at, len(b))
	}
	want := []ent{{ids[0], "Plain", 0, 0, 0, 0, "", ""}, {ids[1], "Grim", hcFlagHardcore, 0, 0, 0, "", ""},
		{ids[2], "Fallow", hcFlagHardcore | hcFlagFallen, uint32(hcFallAt.Unix()), 52, 140, "a Zek sentry", "the wilds near Freeport"}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	van.Queue().GetPending()
	h.SendCharacterSelectEQOA(van)
	if got := strings.Join(names(sentOp(van, protocol.OpcodeCharacterSelect)), ","); got != "Plain,Grim,Fallow" {
		t.Errorf("vanilla's list after the answer: %q", got)
	}

	SetHardcore(config.HardcoreConfig{})
	off := mk(52)
	h.SendCharacterSelectEQOA(off)
	if got := strings.Join(names(sentOp(off, protocol.OpcodeCharacterSelect)), ","); got != "Plain,Grim,Fallow RIP 11/18 08:45" || hasOp(off, protocol.OpcodeHardcore) {
		t.Errorf("world.hardcore off: %q (a fallen character stays marked)", got)
	}
}

// The fallen mark fits CLIENT's 23 bytes with the name whole.
func TestPS2ListNameFits(t *testing.T) {
	hardcoreOnFor(t)
	fallen := database.HardcoreState{Hardcore: true, Fall: database.FallRecord{At: hcFallAt.Unix()}}
	for name, want := range map[string]string{
		"Bob":             "Bob RIP 11/18/26 08:45",
		"Fallow":          "Fallow RIP 11/18 08:45",
		"Meshuggah":       "Meshuggah RIP 11/18/26",
		"Meshuggahhhh":    "Meshuggahhhh RIP 11/18",
		"Meshuggahhhhhh":  "Meshuggahhhhhh RIP11/18",
		"Meshuggahhhhhhh": "MeshuggahhhhhhhRIP11/18",
	} {
		got := ps2ListName(name, fallen)
		if got != want || len(got) > entity.ShownNameMost {
			t.Errorf("%s: %q (%d bytes), want %q", name, got, len(got), want)
		}
	}
	if got := ps2ListName("Meshuggahhhhhhh", database.HardcoreState{Hardcore: true}); got != "[HC] Meshuggahhhhhhh" {
		t.Errorf("alive: %q", got)
	}
}

// A creation from a client that took the offer keeps its choice (no box at
// its entry); one from a PS2 has its choice open; a block from a client never
// offered is a normal character; a cut block is refused.
func TestHardcoreChosenAtCreation(t *testing.T) {
	hardcoreOnFor(t)
	db, err := database.NewSQLite(t.TempDir() + "/create.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
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
		st, _ := db.HardcoreOf(rec.ServerID)
		return st, true
	}
	van := testSession()
	van.SetAccountID(1)
	van.OfferHardcoreOnce()
	van.SetHardcoreClient(true)
	create(van, "Hardy", []byte{'H', 1, 1})
	create(van, "Softy", []byte{'H', 1, 0})
	ps2 := testSession()
	ps2.SetAccountID(1)
	create(ps2, "Consoley", nil)
	create(ps2, "Sneaky", []byte{'H', 1, 1})
	create(van, "Cutty", []byte{'H', 1})
	for name, want := range map[string][3]bool{ // hardcore, chosen, open
		"Hardy": {true, true, false}, "Softy": {false, true, false}, "Consoley": {false, false, true}, "Sneaky": {false, false, true},
	} {
		st, ok := state(name)
		if !ok || [3]bool{st.Hardcore, st.Chosen, st.ChoiceOpen()} != want || st.Born == 0 {
			t.Errorf("%s: made %v, %+v, want %v", name, ok, st, want)
		}
	}
	if _, ok := state("Cutty"); ok {
		t.Error("a cut block made a character")
	}
}

// hcWorld is two players in the world from hcDB's account: Grim (Hardcore)
// and Plain, each a session; and the character handler over them.
type hcWorld struct {
	db         *database.SQLiteDB
	h          *CharacterHandler
	c          *chatWorld
	sg, sp     *session.Session
	grim, pl   *entity.Player
	ids        [3]uint32
	afterFuncs []func()
}

func newHCWorld(t *testing.T) *hcWorld {
	t.Helper()
	hardcoreOnFor(t)
	db, store, ids := hcDB(t)
	c := newChatWorld(t)
	r := &hcWorld{db: db, c: c, ids: ids, h: &CharacterHandler{store: store, sessionManager: c.sm, gameWorld: c.gw}}
	r.sp, r.pl = c.join(int(ids[0]), "Plain", 25300, 15700, 0)
	r.sg, r.grim = c.join(int(ids[1]), "Grim", 25300, 15710, 0)
	r.pl.CharServerID, r.grim.CharServerID = ids[0], ids[1]
	r.sg.SetCharacterServerID(ids[1])
	r.h.applyHardcore(r.sg, r.grim)
	r.h.applyHardcore(r.sp, r.pl)
	old := hardcoreAfter
	hardcoreAfter = func(_ time.Duration, f func()) { r.afterFuncs = append(r.afterFuncs, f) }
	t.Cleanup(func() { hardcoreAfter = old })
	return r
}

// Grim's fall: written (once), the shard told, Grim told and, after the wait,
// sent to the login (0x0777) and its session let go; vanilla told the fall
// first. A select of the fallen character is refused; Plain's is not.
func TestHardcoreFallWrittenAnnouncedAndSentToLogin(t *testing.T) {
	r := newHCWorld(t)
	if !r.grim.Hardcore || r.pl.Hardcore {
		t.Fatalf("at entry: Grim %v, Plain %v", r.grim.Hardcore, r.pl.Hardcore)
	}
	r.sg.OfferHardcoreOnce()
	r.sg.SetHardcoreClient(true)
	r.sg.Queue().GetPending()
	r.sp.Queue().GetPending()
	r.h.hardcoreFell(r.sg, entity.HardcoreFall{CharServerID: r.ids[1], Name: "Grim", Level: 20, Class: 3, At: hcFallAt,
		Cause: entity.CauseCreature, Killer: "a Zek sentry", World: 0, X: 25300, Z: 15710})
	st, _ := r.db.HardcoreOf(r.ids[1])
	if !st.Fallen() || st.Fall.Killer != "a Zek sentry" || st.Fall.Place != "Freeport" || st.Fall.Level != 20 || st.Fall.At != hcFallAt.Unix() {
		t.Fatalf("the row: %+v", st)
	}
	line := "Grim the level 20 Shadow Knight has fallen in Freeport, slain by a Zek sentry."
	if !strings.Contains(string(hcWide(sentOp(r.sp, protocol.OpcodeColoredChat))), line) {
		t.Errorf("Plain was not told %q", line)
	}
	toGrim := r.sg.Queue().GetPending()
	var ops []protocol.GameOpcode
	for _, m := range toGrim {
		ops = append(ops, protocol.GameOpcode(binary.LittleEndian.Uint16(m.Data())))
	}
	if !containsOp(ops, protocol.OpcodeHardcoreFall) || containsOp(ops, protocol.OpcodeDisconnectNoError) {
		t.Errorf("Grim before the wait: %v", ops)
	}
	if len(r.afterFuncs) != 1 {
		t.Fatalf("%d timers", len(r.afterFuncs))
	}
	r.afterFuncs[0]()
	if !hasOp(r.sg, protocol.OpcodeDisconnectNoError) {
		t.Error("no 0x0777 after the wait")
	}
	if _, replaced := r.sg.ReplacedFor(); !replaced {
		t.Error("the session was not let go")
	}
	// written once: a second fall changes nothing
	if fell, _ := r.db.Fall(database.FallRecord{ServerID: r.ids[1], At: 1}); fell {
		t.Error("a second fall was written")
	}

	sel := testSession()
	sel.SetAccountID(5)
	rec, _ := r.h.store.GetByServerID(r.ids[1])
	if !r.h.hardcoreSelectRefused(sel, rec) || !hasOp(sel, protocol.OpcodeCharSelectAreaUnavailable) {
		t.Error("the fallen character's select was not refused")
	}
	plain, _ := r.h.store.GetByServerID(r.ids[0])
	if r.h.hardcoreSelectRefused(testSession(), plain) {
		t.Error("Plain's select was refused")
	}
}

// A keep restart never puts a fallen character back.
func TestHardcoreFallenNotResumed(t *testing.T) {
	r := newHCWorld(t)
	old := keepChars.Load()
	keepChars.Store(r.h)
	t.Cleanup(func() { keepChars.Store(old) })
	s := testSession()
	s.SetAccountID(5)
	s.SetCharacterServerID(r.ids[2])
	if _, err := ResumeCharacter(s, KeptExtras{}, entity.KeptPlayer{Name: "Fallow"}, time.Now()); err == nil || !strings.Contains(err.Error(), "fallen") {
		t.Errorf("resume: %v", err)
	}
}

// The question box for a character made on a PS2: two steps to Hardcore, a
// No for good, a Close leaves it open; /hardcore asks again; a character played
// on is not asked.
func TestHardcoreAskedAtFirstEntry(t *testing.T) {
	r := newWelcomeRigLike(t)
	ask := r.ask
	// Anna: Yes, then Yes
	ask.Show(r.anna, r.pa)
	box := r.box(r.anna, "Make Anna a Hardcore character? Its first death would be final: no respawn, no resurrection. This can never be undone.", hardcoreYes1, hardcoreNo1)
	r.pick(r.anna, box, 0)
	box = r.box(r.anna, "Are you sure? Anna will be Hardcore for good, drawn [HC]. If Anna dies, Anna stays dead.", hardcoreYes2, hardcoreNo2)
	r.pick(r.anna, box, 0)
	if st, _ := r.db.HardcoreOf(r.pa.CharServerID); !st.Hardcore || !st.Chosen || !r.pa.Hardcore {
		t.Errorf("Anna: %+v, player %v", st, r.pa.Hardcore)
	}
	if got := r.msgs(r.anna); !hasMsg(got, "colour:Anna is now a Hardcore character.*") {
		t.Errorf("Anna told %q", got)
	}
	// Cora: Close, then /hardcore, then No
	ask.Show(r.cora, r.pc)
	box = r.box(r.cora, "", hardcoreYes1, hardcoreNo1)
	r.now = r.now.Add(time.Minute)
	r.pick(r.cora, box, 255)
	r.msgs(r.cora)
	if st, _ := r.db.HardcoreOf(r.pc.CharServerID); !st.ChoiceOpen() {
		t.Errorf("a Close closed the choice: %+v", st)
	}
	r.say(r.cora, "/t hardcore")
	box = r.box(r.cora, "", hardcoreYes1, hardcoreNo1)
	r.pick(r.cora, box, 1)
	if st, _ := r.db.HardcoreOf(r.pc.CharServerID); st.Hardcore || !st.Chosen {
		t.Errorf("Cora's No: %+v", st)
	}
	r.say(r.cora, "/t hardcore")
	if got := r.msgs(r.cora); !hasMsg(got, "chat:Cora is a normal character: the Hardcore choice is made when a character is new.") {
		t.Errorf("Cora's /hardcore after: %q", got)
	}
	// Bert has earned experience: never asked
	if _, err := r.db.Exec(`UPDATE characters SET totalXP = 10 WHERE serverid = ?`, r.pb.CharServerID); err != nil {
		t.Fatal(err)
	}
	ask.Show(r.bert, r.pb)
	if got := r.msgs(r.bert); !hasMsg(got, "chat:Bert is a normal character*") {
		t.Errorf("Bert: %q", got)
	}
}

// hcAskRig is the portal rig's players made characters of a database, their
// choice open, and the asker over them.
type hcAskRig struct {
	*portalRig
	db  *database.SQLiteDB
	ask *HardcoreAsk
}

func newWelcomeRigLike(t *testing.T) *hcAskRig {
	t.Helper()
	hardcoreOnFor(t)
	r := newPortalRig(t)
	db, err := database.NewSQLite(t.TempDir() + "/ask.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	for _, p := range []*entity.Player{r.pa, r.pb, r.pc} {
		rec := database.CharacterRecord{CharName: p.Name, AccountID: 1, Level: 1}
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		if err := db.StampBorn(rec.ServerID, 1, time.Now()); err != nil {
			t.Fatal(err)
		}
		p.CharServerID = rec.ServerID
	}
	a := NewHardcoreAsk(r.gw, db)
	a.now = func() time.Time { return r.now }
	a.later = func(time.Duration, func()) {}
	old := hardcoreAsker.Load()
	hardcoreAsker.Store(a)
	t.Cleanup(func() { hardcoreAsker.Store(old) })
	r.drain()
	return &hcAskRig{portalRig: r, db: db, ask: a}
}

// box is the one Hardcore box s was sent; question "" takes any.
func (r *hcAskRig) box(s *session.Session, question string, answers ...string) uint32 {
	r.t.Helper()
	got := r.msgs(s)
	id := boxID(got)
	if len(got) != 1 || !IsHardcoreBox(id) {
		r.t.Fatalf("sent %q, want a Hardcore box", got)
	}
	if question != "" && !strings.HasSuffix(got[0], ":"+question+"|"+strings.Join(answers, "|")) {
		r.t.Fatalf("sent %q, want %q", got[0], question)
	}
	if n := len([]rune(question)); n > questAskChars {
		r.t.Errorf("question %d characters", n)
	}
	return id
}

// The memorial: the newest first, paged, every line a PS2 can show.
func TestMemorialLines(t *testing.T) {
	var falls []database.FallRecord
	for i := range 8 {
		falls = append(falls, database.FallRecord{Name: "Hero" + string(rune('A'+i)), Level: 50 + i, Class: 3, CMs: 140,
			At: hcFallAt.Add(-time.Duration(i) * time.Hour).Unix(), Cause: entity.CauseCreature, Killer: "a Zek | sentry",
			Place: "Freeport"})
	}
	lines, next := memorialLines(falls, 0, "Heirloom")
	if next != 1 || len(lines) != 1+memorialPerPage+1 {
		t.Fatalf("page 1: %d lines, next %d", len(lines), next)
	}
	if want := "HeroA, level 50 Shadow Knight, 140 CMs, fell 2026-11-18 08:45 UTC in Freeport, slain by a Zek  sentry."; lines[1] != want {
		t.Errorf("line %q, want %q", lines[1], want)
	}
	if lines[len(lines)-1] != "Page 1 of 2. Speak to the memorial again for more." {
		t.Errorf("page line %q", lines[len(lines)-1])
	}
	lines, next = memorialLines(falls, 1, "")
	if next != 0 || len(lines) != 1+2+1 || !strings.HasPrefix(lines[1], "HeroG,") {
		t.Errorf("page 2: %q, next %d", lines, next)
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "|%") || len([]rune(l)) > questBoxChars {
			t.Errorf("a PS2 cannot show %q", l)
		}
	}
	if lines, _ := memorialLines(nil, 0, ""); len(lines) != 2 || lines[1] != memorialEmpty {
		t.Errorf("none: %q", lines)
	}
}

// Tells and invites find a tagged name's character by its plain name.
func TestHardcoreTagsInTells(t *testing.T) {
	for line, want := range map[string]string{"/t [OG][HC] Bob hi": "Bob", "/t [HC] Bob hi": "Bob", "/t [GM][OG][HC]Bob hi": "[GM][OG][HC]Bob"} {
		name, text, ok := parseTell(line)
		if !ok || name != want || text != "hi" || entity.PlainName(name) != "Bob" {
			t.Errorf("%q: %q %q %v", line, name, text, ok)
		}
	}
}

func containsOp(ops []protocol.GameOpcode, op protocol.GameOpcode) bool {
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}

// hcWide is a 0x00xx text message's wide string (u32 count, UTF-16).
func hcWide(d []byte) string {
	if len(d) < 6 {
		return ""
	}
	n := int(binary.LittleEndian.Uint32(d[2:]))
	var b strings.Builder
	for i := 0; i < n && 6+2*i+1 < len(d); i++ {
		b.WriteRune(rune(binary.LittleEndian.Uint16(d[6+2*i:])))
	}
	return b.String()
}

// The Hardcore channel: Grim speaks and hears; Plain neither; a fall is told
// in it to the Hardcore and to the shard to the rest.
func TestHardcoreChannel(t *testing.T) {
	r := newHCWorld(t)
	r.sg.Queue().GetPending()
	r.sp.Queue().GetPending()
	r.c.say(r.sg, "#hc anyone near Freeport?")
	if got := hcWide(sentOp(r.sg, protocol.OpcodeColoredChat)); got != "[HC] Grim: anyone near Freeport?" {
		t.Errorf("Grim heard %q", got)
	}
	if hasOp(r.sp, protocol.OpcodeColoredChat) {
		t.Error("Plain heard the channel")
	}
	r.sg.Queue().GetPending()
	r.c.say(r.sg, "/t hc hello")
	if got := hcWide(sentOp(r.sg, protocol.OpcodeColoredChat)); got != "[HC] Grim: hello" {
		t.Errorf("a PS2's tell to the channel: %q", got)
	}
	r.sp.Queue().GetPending()
	r.c.say(r.sp, "/hc let me in")
	if got := hcWide(sentOp(r.sp, protocol.OpcodeErrorMessage)); got != hcChatOnly {
		t.Errorf("Plain told %q", got)
	}
	r.sg.Queue().GetPending()
	r.sp.Queue().GetPending()
	announceFall(r.c.gw, "Fallow the level 52 Shadow Knight has fallen in Freeport, slain by a Zek sentry.")
	if got := hcWide(sentOp(r.sg, protocol.OpcodeColoredChat)); !strings.HasPrefix(got, "[HC] Fallow the level 52") {
		t.Errorf("Grim told %q", got)
	}
	if got := hcWide(sentOp(r.sp, protocol.OpcodeColoredChat)); !strings.HasPrefix(got, "Fallow the level 52") {
		t.Errorf("Plain told %q", got)
	}
}

// The living ranked: level, then CMs, then experience; the fallen and the
// normal left out; the leaderboard's lines a PS2 can show.
func TestHardcoreLeaderboard(t *testing.T) {
	db, store, ids := hcDB(t)
	mk := func(name string, level, xp, cms int) uint32 {
		rec := database.CharacterRecord{CharName: name, AccountID: 6, Level: level, TClass: 0, TotalXP: xp}
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ChooseHardcore(rec.ServerID, true); err != nil {
			t.Fatal(err)
		}
		if cms > 0 {
			store.SaveMastery(rec.ServerID, database.MasteryRow{Points: cms - 10, Spent: 10})
		}
		db.StampBorn(rec.ServerID, 1, hcFallAt.Add(-50*time.Hour))
		return rec.ServerID
	}
	mk("Low", 10, 5, 0)
	mk("HighFew", 40, 100, 20)
	mk("HighMany", 40, 50, 60)
	mk("HighManyXP", 40, 90, 60)
	got, err := db.LivingHardcore(0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, h := range got {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "HighManyXP,HighMany,HighFew,Grim,Low" {
		t.Errorf("ranked %q", names)
	}
	_ = ids
	lines, _ := leaderboardLines(got, 0, "Live", hcFallAt)
	if lines[1] != "1. HighManyXP, level 40 Warrior, 60 CMs, alive 2d 2h" {
		t.Errorf("line %q", lines[1])
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "|%") || len([]rune(l)) > questBoxChars {
			t.Errorf("a PS2 cannot show %q", l)
		}
	}
}

package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raceRig is a server's handler over a real database holding two giants, a world
// and its sessions.
type raceRig struct {
	t     *testing.T
	w     *entity.World
	mgr   *session.Manager
	db    *database.SQLiteDB
	store *database.SQLiteCharacterStore
	h     *RaceLabelsHandler
	next  int
}

func newRaceRig(t *testing.T) *raceRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/races.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO racedefs (id, name, short_name, wire_race, bonus_str, bonus_sta, bonus_agi, bonus_dex, body_male, body_female)
		VALUES (11, 'Hill Giant', 'HGT', 9, 25, 20, -5, -5, -5117396, -5117396)`); err != nil {
		t.Fatal(err)
	}
	if err := entity.LoadRaceDefs(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { entity.SetRaceDefs(nil) })
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	r := &raceRig{t: t, w: w, mgr: mgr, db: db, store: database.NewSQLiteCharacterStore(db), h: NewRaceLabelsHandler(w, db)}
	r.h.RegisterHandlers(NewRegistry())
	r.character("Lagerbottoms", 11)
	return r
}

// character makes a character's row.
func (r *raceRig) character(name string, race int) *database.CharacterRecord {
	r.t.Helper()
	rec := &database.CharacterRecord{CharName: name, AccountID: 1, Level: 60, Race: race, ModelID: -5117396}
	if err := r.store.Create(rec); err != nil {
		r.t.Fatal(err)
	}
	return rec
}

// client puts a session in the world playing a character of the name and race.
func (r *raceRig) client(name string, race int) (*session.Session, *entity.Player) {
	r.t.Helper()
	r.next++
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(r.next)), Port: 6000 + r.next}, uint32(r.next), uint32(r.next), 1, 2, true)
	p := &entity.Player{ServerID: entity.EntityID(1000 + r.next), Name: name, Race: race}
	r.w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	return sess, p
}

// answer is the client's OpcodeRaceLabels.
func (r *raceRig) answer(sess *session.Session, data ...byte) {
	r.t.Helper()
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceLabels)
	m.SetData(data)
	if err := r.h.HandleRaceLabels(sess, m); err != nil {
		r.t.Fatal(err)
	}
}

// raceLists takes every OpcodeRaceLabelsData queued for sess's client, their
// bodies, leaving nothing behind to count twice; any other non-retail opcode is
// a failure of the caller's to look at.
func raceLists(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeRaceLabelsData {
			out = append(out, d[2:])
		}
	}
	return out
}

// wantList is the list's bytes for one race's names.
func wantList(long, short string, names ...string) []byte {
	b := []byte{1, byte(len(long))}
	b = append(b, long...)
	b = append(b, byte(len(short)))
	b = append(b, short...)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(names)))
	for _, n := range names {
		b = append(b, byte(len(n)))
		b = append(b, n...)
	}
	return b
}

// The first 0x0014 offers the labels, a u32 1 after the opcode, once.
func TestRaceLabelsOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	var offers [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeRaceLabels {
			offers = append(offers, d)
		}
	}
	if want := []byte{0x1C, 0x7E, 1, 0, 0, 0}; len(offers) != 1 || !bytes.Equal(offers[0], want) {
		t.Fatalf("world entry offered %x, want one %x", offers, want)
	}
}

// A client that answers is sent the list at once, by the race's row; a client
// that never answers (a PS2, an older vanilla) is sent nothing, whatever happens;
// asking again after stopping sends it again; a short answer changes nothing.
func TestRaceLabelsGoToAClientThatAsked(t *testing.T) {
	r := newRaceRig(t)
	asks, p := r.client("Asker", 3)
	ps2, _ := r.client("Consoleman", 3)
	r.answer(asks, 1, 0, 0)
	if p.RaceLabels || len(raceLists(asks)) != 0 {
		t.Fatalf("a short answer: flag %v, lists %x", p.RaceLabels, raceLists(asks))
	}
	r.answer(asks, 1, 0, 0, 0)
	if !p.RaceLabels {
		t.Fatal("the answer did not take")
	}
	want := wantList("Hill Giant", "HGT", "Lagerbottoms")
	if got := raceLists(asks); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("sent %x, want one %x", got, want)
	}
	if got := raceLists(ps2); len(got) != 0 {
		t.Fatalf("a client that never asked was sent %x", got)
	}
	// nothing changed: nothing more goes out
	if n := r.w.RefreshRaceLabels(r.db); n != 0 || len(raceLists(asks)) != 1 {
		t.Errorf("an unchanged list: %d sent, %d queued", n, len(raceLists(asks)))
	}
	r.answer(asks, 0, 0, 0, 0)
	if p.RaceLabels {
		t.Fatal("0 did not stop it")
	}
	r.character("Secondgiant", 11)
	if n := r.w.RefreshRaceLabels(r.db); n != 0 {
		t.Errorf("a client that stopped was sent the list: %d", n)
	}
	r.answer(asks, 1, 0, 0, 0)
	if got := raceLists(asks); len(got) != 2 || !bytes.Equal(got[1], wantList("Hill Giant", "HGT", "Lagerbottoms", "Secondgiant")) {
		t.Errorf("asked again: %x", got)
	}
	// a session with no player: nothing, no panic
	gone := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 200), Port: 6200}, 200, 200, 1, 2, true)
	r.answer(gone, 1, 0, 0, 0)
	if got := raceLists(gone); len(got) != 0 {
		t.Errorf("no player: %x", got)
	}
}

// A server with no such character sends an asking client nothing: it holds the
// empty list already.
func TestRaceLabelsNoneToSend(t *testing.T) {
	r := newRaceRig(t)
	if _, err := r.db.Exec(`DELETE FROM characters`); err != nil {
		t.Fatal(err)
	}
	asks, _ := r.client("Asker", 3)
	r.answer(asks, 1, 0, 0, 0)
	if got := raceLists(asks); len(got) != 0 {
		t.Fatalf("sent %x", got)
	}
}

// A login of such a character tells the clients in the world that asked, when
// their list lacks it, and only them. An ordinary character's login asks nothing
// of the database and sends nothing (the control: the same login's hook with a
// list that would change).
func TestALoginOfSuchACharacterReachesClientsInTheWorld(t *testing.T) {
	r := newRaceRig(t)
	asks, _ := r.client("Asker", 3)
	never, _ := r.client("Consoleman", 3)
	r.answer(asks, 1, 0, 0, 0)
	if len(raceLists(asks)) != 1 {
		t.Fatal("the first list did not go")
	}

	// a GM made a new giant since: an ordinary login does not look
	r.character("Newgiant", 11)
	plain, _ := r.client("Plainman", 0)
	r.w.PlayerEntered(plain)
	if got := raceLists(asks); len(got) != 1 {
		t.Fatalf("an ordinary login sent the list: %x", got)
	}

	giant, _ := r.client("Newgiant", 11)
	r.w.PlayerEntered(giant)
	want := wantList("Hill Giant", "HGT", "Lagerbottoms", "Newgiant")
	if got := raceLists(asks); len(got) != 2 || !bytes.Equal(got[1], want) {
		t.Fatalf("after the giant's login: %x, want a second %x", got, want)
	}
	if got := raceLists(never); len(got) != 0 {
		t.Errorf("a client that never asked: %x", got)
	}
	// it logs in again: nothing new, nothing sent
	r.w.PlayerEntered(giant)
	if got := raceLists(asks); len(got) != 2 {
		t.Errorf("an unchanged list went again: %d", len(got))
	}
}

// The list as bytes: a race's words from its row, each race its names, a race with
// no character left out, a second race needing nothing but its row. Past a
// message's own span it goes whole, in a message of its own size.
func TestRaceLabelsFormat(t *testing.T) {
	r := newRaceRig(t)
	if _, err := r.db.Exec(`INSERT INTO racedefs (id, name, short_name, wire_race) VALUES (12, 'Stone Giant', 'SGT', 5), (13, 'Ash Giant', '', 2)`); err != nil {
		t.Fatal(err)
	}
	if err := entity.LoadRaceDefs(r.db); err != nil {
		t.Fatal(err)
	}
	r.character("Boulder", 12)
	asks, _ := r.client("Asker", 3)
	r.answer(asks, 1, 0, 0, 0)
	want := []byte{2}
	want = append(want, wantList("Hill Giant", "HGT", "Lagerbottoms")[1:]...)
	want = append(want, wantList("Stone Giant", "SGT", "Boulder")[1:]...)
	if got := raceLists(asks); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("got %x\nwant %x", got, want)
	}

	long := "Lagerbottoms"
	names := []string{long}
	for i := 0; i < 400; i++ {
		name := "Giant" + string(rune('A'+i%26)) + string(rune('a'+i/26))
		r.character(name, 11)
		names = append(names, name)
	}
	r.w.RefreshRaceLabels(r.db)
	got := raceLists(asks)
	if len(got) != 2 || len(got[1]) < 2046 {
		t.Fatalf("the long list: %d lists, the last %d bytes", len(got), len(got[len(got)-1]))
	}
}

// What a client asked for comes through a restart of the server, and the title it
// is shown with it: the flag is kept, and the first look after sends the list again
// whatever it was, as a data step in the hold may have changed it. A client that
// never asked stays so.
func TestRaceLabelsThroughARestart(t *testing.T) {
	r := newRaceRig(t)
	asks, p := r.client("Asker", 3)
	never, _ := r.client("Consoleman", 3)
	r.answer(asks, 1, 0, 0, 0)
	now := time.Unix(1_800_000_000, 0)
	k, ok := r.w.KeepPlayer(asks, now)
	if !ok || !k.RaceLabels {
		t.Fatalf("kept %+v", k)
	}
	kn, _ := r.w.KeepPlayer(never, now)
	raw, _ := json.Marshal(kn)
	if bytes.Contains(raw, []byte("race_labels")) {
		t.Errorf("a client that never asked keeps %s", raw)
	}
	p.RaceLabels = false
	if !r.w.RestoreKeptPlayer(asks, k, now) || !p.RaceLabels {
		t.Fatalf("restored: %v", p.RaceLabels)
	}
	if n := r.w.RefreshRaceLabels(r.db); n != 1 {
		t.Errorf("the same list after a restart went to %d clients, want the one", n)
	}
}

// The heirlooms' best is of the items the race its character passes as may wear:
// no item's mask has a race of racedefs' bit, so a Hill Giant's took nothing.
func TestAnHeirloomOfACustomRaceIsItsLevelsBest(t *testing.T) {
	f := newHeirloomFixture(t)
	if _, err := f.db.Exec(`INSERT INTO racedefs (id, name, wire_race) VALUES (11, 'Hill Giant', 9)`); err != nil {
		t.Fatal(err)
	}
	if err := entity.LoadRaceDefs(f.db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { entity.SetRaceDefs(nil) })
	// the mask of every race but the wire's: its own bit would not help it, the control
	if _, err := f.db.Exec(`UPDATE itempattern SET raceuse = 1023`); err != nil {
		t.Fatal(err)
	}
	f.h.Note(f.sid, 0, 11, 10)
	if got := val(f.held()[hlHelm].AC); got != 20 {
		t.Errorf("a Hill Giant's level 10 heirloom helm has AC %d, want helm 10's 20", got)
	}
}

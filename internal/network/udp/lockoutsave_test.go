package udp

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lockoutRig is a database with a character and its book, the character
// handler a login goes through (as RegisterDefaultHandlers makes it) and the
// listener whose session ends save the character.
type lockoutRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	store *database.SQLiteCharacterStore
	w     *entity.World
	sm    *session.Manager
	h     *handlers.CharacterHandler
	l     *Listener
	rec   *database.CharacterRecord
	port  int
}

// The book: a ward of a ten-minute recast at display index 3, and a quick
// one at 4.
const (
	longWard  = 901
	quickWard = 902
)

func newLockoutRig(t *testing.T) *lockoutRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/lockouts.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Recaster", AccountID: 5, Level: 50, TClass: 9, Race: 0}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (%d, 50, 0, 0, 0, 0, 0, 0, 0, 0, 600, 0, 'Long Ward', '')`, longWard),
		fmt.Sprintf(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (%d, 50, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 'Quick Ward', '')`, quickWard),
		fmt.Sprintf(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (%d, %d, 3, 1, 0, 0, 0)`,
			rec.ServerID, longWard),
		fmt.Sprintf(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (%d, %d, 4, 1, 1, 0, 0)`,
			rec.ServerID, quickWard),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := handlers.NewCharacterHandler(store, sm, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	h.SetGameWorld(w)
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)
	return &lockoutRig{t: t, db: db, store: store, w: w, sm: sm, h: h, l: l, rec: rec, port: 7100}
}

// login is a new client choosing the character and, its world loaded,
// saying it is in it (0x0014).
func (r *lockoutRig) login() *session.Session {
	r.t.Helper()
	r.port++
	inst := uint32(r.port)
	s := r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 21), Port: r.port}, inst, inst, uint16(inst), 2, true)
	s.SetAccountID(r.rec.AccountID)
	s.UpdateActivity()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
	msg.SetData(binary.LittleEndian.AppendUint32(nil, r.rec.ServerID))
	if err := r.h.HandleSelectedChar(s, msg); err != nil {
		r.t.Fatal(err)
	}
	if err := r.h.HandleCharacterInWorld(s, protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)); err != nil {
		r.t.Fatal(err)
	}
	if !s.CharacterInWorld() || r.w.SessionPlayer(s) == nil {
		r.t.Fatal("the login did not reach the world")
	}
	return s
}

// spell is the book's ability as a cast from the bar makes it.
func (r *lockoutRig) spell(id int) entity.CastSpell {
	r.t.Helper()
	book, err := database.NewSpellbookStore(r.db).GetCharacterSpells(r.rec.ServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	for i := range book {
		if book[i].Pattern.SpellID == id {
			return entity.SpellOf(&book[i].Pattern, book[i].AddedOrder, "", 0)
		}
	}
	r.t.Fatalf("no spell %d in the book", id)
	return entity.CastSpell{}
}

// kept is the character's charlockouts rows, spell to when it ends.
func (r *lockoutRig) kept() map[int]time.Time {
	r.t.Helper()
	rows, err := r.db.Query(`SELECT spellid, until FROM charlockouts WHERE serverid = ?`, r.rec.ServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]time.Time{}
	for rows.Next() {
		var id int
		var until int64
		if err := rows.Scan(&id, &until); err != nil {
			r.t.Fatal(err)
		}
		out[id] = time.UnixMilli(until)
	}
	return out
}

// queued is the opcode and body of each reliable message queued on s, in
// order.
func queued(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); m.Type.IsReliable() && len(d) >= 2 {
			out = append(out, d)
		}
	}
	return out
}

func opOf(d []byte) protocol.GameOpcode { return protocol.GameOpcode(binary.LittleEndian.Uint16(d)) }

// zigzag reads a zigzag varint.
func zigzag(b []byte) int64 {
	var u uint64
	var shift uint
	for _, c := range b {
		u |= uint64(c&0x7F) << shift
		shift += 7
		if c&0x80 == 0 {
			break
		}
	}
	return int64(u>>1) ^ -int64(u&1)
}

// lines are the texts of the coloured lines (0x0A7B) queued on s.
func lines(s *session.Session) []string {
	var out []string
	for _, d := range queued(s) {
		if opOf(d) != protocol.OpcodeColoredChat || len(d) < 6 {
			continue
		}
		n := int(binary.LittleEndian.Uint32(d[2:]))
		if 6+2*n > len(d) {
			continue
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(d[6+2*i:])
		}
		out = append(out, string(utf16.Decode(u)))
	}
	return out
}

// The owner's report (2026-11-09): "when eve players logout and login the
// recast times of abilities are being reset, so can exploit". A ward of a
// ten-minute recast cast, the character logged out (the session's end: a
// logout, a dropped client's timeout and a kick all end one there) and in
// again at once: the ward is refused with the time left of it, and the new
// client is shown its badge after its MemoryDump on the ability the book
// has it at. The control: the quick ward, cast at the same time, still locked
// out when the character logged out and over while it was away, is cast
// again at once: the time logged out counts (were a lockout's time to run
// only in the world, as an effect's does, it would still be locked out). The
// shutdown's save keeps them too.
func TestALogoutAndALoginKeepTheLockouts(t *testing.T) {
	r := newLockoutRig(t)
	first := r.login()
	long, quick := r.spell(longWard), r.spell(quickWard)
	// a lockout of 3 s: longer than the login takes, so one whose time ran
	// only in the world would still be on when the login is done
	quick.Recast = 2700 * time.Millisecond
	t0 := time.Now()
	quickEnd := t0.Add(3 * time.Second)
	if !r.w.StartCast(first, long, first.PlayerObjectID(), t0.Add(-time.Second)) {
		t.Fatal("the long ward was not cast")
	}
	if !r.w.StartCast(first, quick, first.PlayerObjectID(), t0) {
		t.Fatal("the quick ward was not cast")
	}

	// the logout
	p := r.w.SessionPlayer(first)
	r.l.endSession(first)
	saved := time.Now()
	if r.w.IsLivePlayer(p) || r.w.PlayerCount() != 0 {
		t.Fatal("the session's end left the character in the world")
	}
	kept := r.kept()
	if until, ok := kept[longWard]; !ok || until.Sub(t0) < 599*time.Second || until.Sub(t0) > 600*time.Second {
		t.Fatalf("the long ward kept until %v from the cast (%v); want 599.3 s", until.Sub(t0), kept)
	}
	if _, ok := kept[quickWard]; !ok {
		// a save over by its end had it running: it must have kept it (a
		// save slower than that, on a loaded disk, proves less, and says so)
		if saved.Before(quickEnd) {
			t.Fatalf("the quick ward, still locked out at the logout, was not kept: %v", kept)
		}
		t.Logf("the logout's save ended %v after the quick ward's lockout: the control proves less", saved.Sub(quickEnd))
	}
	time.Sleep(time.Until(quickEnd.Add(100 * time.Millisecond))) // logged out past the quick ward's end

	// the login
	second := r.login()
	sent := queued(second)
	dumpAt, badges := -1, map[byte]int64{}
	for i, d := range sent {
		switch opOf(d) {
		case protocol.OpcodeMemoryDump:
			dumpAt = i
		case protocol.OpcodeCooldownTimer:
			if dumpAt < 0 {
				t.Fatalf("a lockout went before the dump: % X", d)
			}
			badges[d[2]] = zigzag(d[7:])
		}
	}
	if dumpAt < 0 {
		t.Fatal("no MemoryDump")
	}
	if ms, ok := badges[3]; !ok || len(badges) != 1 || ms < 580_000 || ms > 596_200 {
		t.Fatalf("the badges told after the dump: %v; want the long ward's on ability 3 with about 596000 ms", badges)
	}

	before := len(lines(second))
	now := time.Now()
	if r.w.StartCast(second, long, second.PlayerObjectID(), now) {
		t.Fatal("THE EXPLOIT: a logout and a login made the ten-minute recast ready")
	}
	said := lines(second)[before:]
	var left float64
	if len(said) != 1 || !strings.HasPrefix(said[0], "Recovery time for that ability has not been met (") {
		t.Fatalf("the refusal: %q", said)
	}
	if _, err := fmt.Sscanf(strings.TrimPrefix(said[0], "Recovery time for that ability has not been met ("), "%f s)", &left); err != nil ||
		left < 580 || left > 596.2 {
		t.Fatalf("the refusal %q (%v): want about 596 s left", said[0], err)
	}
	// the control: the quick ward, over while the character was away
	if !r.w.StartCast(second, quick, second.PlayerObjectID(), now) {
		t.Fatal("the control: a lockout that ran out while logged out still refused its ward")
	}

	// a shutdown's save keeps both, the long ward's end unchanged
	r.l.SaveAll()
	kept = r.kept()
	if !kept[longWard].Equal(t0.Add(-time.Second).Add(600*time.Second + 300*time.Millisecond).Truncate(time.Millisecond)) {
		t.Errorf("the shutdown kept the long ward until %v, want %v", kept[longWard], t0.Add(599300*time.Millisecond))
	}
	if until, ok := kept[quickWard]; !ok || until.Before(now) {
		t.Errorf("the shutdown did not keep the quick ward cast again: %v", kept)
	}
}

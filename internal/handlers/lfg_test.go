package handlers

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The live group search (lfg.go, entity/lfg.go) over the chat routing's small
// world: every request fed as the live client's part sends it (0x0801), every
// answer read as its Recv0840 reads it (spec lfg.md section 2).

type lfgReadRow struct {
	name        string
	level       byte
	class, race int64
	x, y, z     float32
	world       byte
	id          uint32
}

// readLFGResults is the live client's 0x0840 reader (0x012EBF38): sub 0x40,
// zz count, the rows; it must end exactly at the message's end.
func readLFGResults(t *testing.T, d []byte) []lfgReadRow {
	t.Helper()
	if len(d) < 2 || binary.LittleEndian.Uint16(d) != uint16(protocol.OpcodeLFGResults) {
		t.Fatalf("not 0x0840: % X", d)
	}
	r := &auctionWire{b: d[2:]}
	if sub := r.u8(); sub != 0x40 {
		t.Fatalf("0x0840 sub %#x, want 0x40", sub)
	}
	var rows []lfgReadRow
	for n := r.zz(); n > 0 && !r.bad; n-- {
		var row lfgReadRow
		row.name = r.str8()
		row.level = r.u8()
		row.class, row.race = r.zz(), r.zz()
		row.x = math.Float32frombits(r.u32())
		row.y = math.Float32frombits(r.u32())
		row.z = math.Float32frombits(r.u32())
		row.world = r.u8()
		row.id = r.u32()
		rows = append(rows, row)
	}
	if r.bad || r.p != len(d)-2 {
		t.Fatalf("0x0840 read %d of %d bytes (bad %v)", r.p, len(d)-2, r.bad)
	}
	return rows
}

// lfgSend feeds 0x0801 with the sub and its fields.
func lfgSend(t *testing.T, h *LFGHandler, s *session.Session, sub byte, fill func(w *buffer.Writer)) {
	t.Helper()
	payload := build(protocol.OpcodeLFG, 64, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	})[2:]
	if err := h.HandleLFG(s, incoming(protocol.OpcodeLFG, payload)); err != nil {
		t.Fatal(err)
	}
}

// lfgSearch is sub 4 as 0x012EC530 writes it: u8 type, the CLASS mask, the
// race mask, u8 min, u8 max; it returns the one answer it must get.
func lfgSearch(t *testing.T, h *LFGHandler, s *session.Session, typ byte, classes, races int32, lo, hi byte) []lfgReadRow {
	t.Helper()
	s.Queue().GetPending()
	lfgSend(t, h, s, 4, func(w *buffer.Writer) {
		w.WriteByte(typ)
		w.Write7BitEncodedInt64(int64(classes))
		w.Write7BitEncodedInt64(int64(races))
		w.WriteByte(lo)
		w.WriteByte(hi)
	})
	got := queued(s, protocol.OpcodeLFGResults)
	if len(got) != 1 {
		t.Fatalf("a search was answered %d times, want once", len(got))
	}
	return readLFGResults(t, got[0])
}

func rowNames(rows []lfgReadRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.name)
	}
	return out
}

// lfgWorld is a world of players to search: the searcher, a registered
// solo cleric, one out of the range, one not registered, a group of two with
// room (its leader and a member both registered) and a full group of four
// whose leader is registered.
type lfgWorld struct {
	*chatWorld
	h                      *LFGHandler
	alice, bob, cara, dave *session.Session
	pa, pb, pc, pd         *entity.Player
	erin, fay, gus         *session.Session
	pe, pf, pg             *entity.Player
}

func newLFGWorld(t *testing.T) *lfgWorld {
	c := newChatWorld(t)
	w := &lfgWorld{chatWorld: c, h: NewLFGHandler(c.gw)}
	setup := func(id int, name string, level, class, race int) (*session.Session, *entity.Player) {
		s, p := c.join(id, name, float32(1000+10*id), float32(2000+id), 0)
		p.Level, p.Class, p.Race = level, class, race
		return s, p
	}
	w.alice, w.pa = setup(1, "Alice", 20, 0, 0) // warrior human: the searcher
	w.bob, w.pb = setup(2, "Bob", 22, 9, 1)     // cleric elf
	w.cara, w.pc = setup(3, "Cara", 40, 13, 3)  // wizard gnome: out of 15-30
	w.dave, w.pd = setup(4, "Dave", 21, 6, 4)   // rogue dwarf: not registered
	w.erin, w.pe = setup(5, "Erin", 19, 2, 0)   // paladin: a member of Fay's group
	w.fay, w.pf = setup(6, "Fay", 23, 8, 5)     // shaman troll: leads a group of two
	w.gus, w.pg = setup(7, "Gus", 24, 5, 6)     // bard barbarian: leads a full group
	w.pb.World = 2                              // Bob is in another world: found all the same
	c.group(t, w.fay, w.pf, w.erin, w.pe)
	var full []*entity.Player
	for i, n := range []string{"Hal", "Ivy", "Jon"} {
		s, p := setup(8+i, n, 24, 0, 0)
		c.gw.GroupInvite(w.gus, c.gw.IDSeenBy(w.gus, p))
		c.gw.GroupAccept(s, c.gw.IDSeenBy(s, w.pg))
		full = append(full, p)
	}
	if len(c.gw.GroupMembers(w.pg)) != entity.MaxGroupSize {
		t.Fatalf("Gus's group: %d", len(c.gw.GroupMembers(w.pg)))
	}
	for _, s := range []*session.Session{w.bob, w.cara, w.erin, w.fay, w.gus} {
		if s == w.erin {
			continue // a member: registered by hand below, as the client would refuse it
		}
		lfgSend(t, w.h, s, 2, nil)
	}
	w.pe.LFG = true
	c.sm.ForEach(func(s *session.Session) bool {
		s.Queue().GetPending()
		return true
	})
	return w
}

// Register, search both ways, unregister: type 0 finds registered players in
// no group, type 1 the registered leaders of groups with room; the rows read
// back as the live client reads them, the id the one 0x0625 takes; nothing
// unregistered, out of the range, grouped, full or the searcher itself.
func TestLFGRegisterSearchUnregister(t *testing.T) {
	w := newLFGWorld(t)
	if !w.pb.LFG || !w.pf.LFG || !w.pg.LFG || w.pd.LFG {
		t.Fatalf("registered: Bob %v Fay %v Gus %v Dave %v", w.pb.LFG, w.pf.LFG, w.pg.LFG, w.pd.LFG)
	}
	rows := lfgSearch(t, w.h, w.alice, 0, -1, -1, 15, 30)
	if got := rowNames(rows); len(got) != 1 || got[0] != "Bob" {
		t.Fatalf("players for a group: %v, want Bob alone", got)
	}
	b := rows[0]
	if b.level != 22 || b.class != 9 || b.race != 1 || b.world != 2 || b.x != w.pb.X || b.y != w.pb.Y || b.z != w.pb.Z {
		t.Errorf("Bob's row %+v", b)
	}
	if want := w.gw.IDSeenBy(w.alice, w.pb); b.id != want {
		t.Errorf("Bob's id %#x, want %#x (the who list's, which 0x0625 takes)", b.id, want)
	}
	if w.gw.PlayerSeenBy(w.alice, b.id) != w.pb {
		t.Error("the row's id does not name Bob to the searcher")
	}
	// the wider range takes Cara too; the searcher never finds itself
	lfgSend(t, w.h, w.alice, 2, nil)
	if got := rowNames(lfgSearch(t, w.h, w.alice, 0, -1, -1, 1, 60)); strings.Join(got, ",") != "Bob,Cara" {
		t.Errorf("1-60: %v", got)
	}
	// groups for a player: Fay leads two, Gus's four are full
	if got := rowNames(lfgSearch(t, w.h, w.alice, 1, -1, -1, 1, 60)); len(got) != 1 || got[0] != "Fay" {
		t.Errorf("groups for a player: %v, want Fay alone", got)
	}
	// Bob unregisters (a line says so), and the answer is the empty one
	w.bob.Queue().GetPending()
	lfgSend(t, w.h, w.bob, 3, nil)
	if got := heard(t, w.bob); len(got) != 1 || got[0].text != "You are no longer registered as looking for a group." {
		t.Errorf("Bob was told %+v", got)
	}
	w.alice.Queue().GetPending()
	lfgSend(t, w.h, w.alice, 3, nil)
	if got := lfgSearch(t, w.h, w.alice, 0, -1, -1, 15, 30); len(got) != 0 {
		t.Errorf("after Bob unregistered: %v", rowNames(got))
	}
	// the empty answer is exactly u8 0x40, zz 0
	w.alice.Queue().GetPending()
	lfgSend(t, w.h, w.alice, 4, func(b *buffer.Writer) {
		b.WriteByte(0)
		b.Write7BitEncodedInt64(-1)
		b.Write7BitEncodedInt64(-1)
		b.WriteByte(15)
		b.WriteByte(30)
	})
	if got := queued(w.alice, protocol.OpcodeLFGResults); len(got) != 1 || string(got[0]) != "\x40\x08\x40\x00" {
		t.Errorf("the empty answer % X", got)
	}
}

// The class mask comes first on the wire: a search for clerics among elves
// finds Bob, and the same two masks the other way round (rangers among
// ogres) find nobody; CLEAR (0) takes nobody.
func TestLFGMasksClassFirst(t *testing.T) {
	w := newLFGWorld(t)
	if got := rowNames(lfgSearch(t, w.h, w.alice, 0, 1<<9, 1<<1, 1, 60)); len(got) != 1 || got[0] != "Bob" {
		t.Errorf("clerics among elves: %v", got)
	}
	if got := lfgSearch(t, w.h, w.alice, 0, 1<<1, 1<<9, 1, 60); len(got) != 0 {
		t.Errorf("the masks read the other way round found %v", rowNames(got))
	}
	if got := lfgSearch(t, w.h, w.alice, 0, 0, -1, 1, 60); len(got) != 0 {
		t.Errorf("CLEAR found %v", rowNames(got))
	}
	// several bits: clerics and wizards (Cara is 40)
	if got := rowNames(lfgSearch(t, w.h, w.alice, 0, 1<<9|1<<13, -1, 1, 60)); strings.Join(got, ",") != "Bob,Cara" {
		t.Errorf("clerics or wizards: %v", got)
	}
}

// Every search is answered once, a short one and one before a player
// included: the client stays "Searching" until it is.
func TestLFGEverySearchAnswered(t *testing.T) {
	w := newLFGWorld(t)
	w.alice.Queue().GetPending()
	lfgSend(t, w.h, w.alice, 4, func(b *buffer.Writer) { b.WriteByte(0) })
	if got := queued(w.alice, protocol.OpcodeLFGResults); len(got) != 1 || len(readLFGResults(t, got[0])) != 0 {
		t.Errorf("a short search: %d answers", len(got))
	}
	nobody := testSession()
	lfgSend(t, w.h, nobody, 4, func(b *buffer.Writer) {
		b.WriteByte(1)
		b.Write7BitEncodedInt64(-1)
		b.Write7BitEncodedInt64(-1)
		b.WriteByte(1)
		b.WriteByte(60)
	})
	if got := queued(nobody, protocol.OpcodeLFGResults); len(got) != 1 {
		t.Errorf("a search with no player: %d answers", len(got))
	}
	// planted control: the type the window never sends finds nobody, answered
	if got := lfgSearch(t, w.h, w.alice, 7, -1, -1, 1, 60); len(got) != 0 {
		t.Errorf("type 7: %v", rowNames(got))
	}
}

// A member of a group who does not lead it is refused in red (word 1848)
// and nothing changes; its leader registers the group.
func TestLFGMemberRefused(t *testing.T) {
	w := newLFGWorld(t)
	w.pe.LFG = false
	w.erin.Queue().GetPending()
	lfgSend(t, w.h, w.erin, 2, nil)
	if got := heard(t, w.erin); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != lfgNotLeader {
		t.Errorf("the member was told %+v", got)
	}
	if w.pe.LFG {
		t.Error("a member registered")
	}
	w.fay.Queue().GetPending()
	lfgSend(t, w.h, w.fay, 2, nil)
	if got := heard(t, w.fay); len(got) != 1 || got[0].text != "Your group is already registered as looking for more members." {
		t.Errorf("the leader was told %+v", got)
	}
}

// /lfg and #lfg, everyone's: on and off as the live client spells them, a
// status line alone, the live client's "Unrecognized command" for the rest;
// # is not taken for a GM command (no GM here).
func TestLFGChatCommands(t *testing.T) {
	w := newLFGWorld(t)
	w.dave.Queue().GetPending()
	w.say(w.dave, "/lfg on")
	if !w.pd.LFG {
		t.Fatal("/lfg on did not register")
	}
	if got := heard(t, w.dave); len(got) != 1 || got[0].text != "You are registered as looking for a group." {
		t.Errorf("/lfg on: %+v", got)
	}
	w.dave.Queue().GetPending()
	w.say(w.dave, "#lfg off")
	if w.pd.LFG {
		t.Fatal("#lfg off did not unregister")
	}
	if got := heard(t, w.dave); len(got) != 1 || got[0].text != "You are no longer registered as looking for a group." {
		t.Errorf("#lfg off: %+v", got)
	}
	w.dave.Queue().GetPending()
	w.say(w.dave, "#LFG register")
	if !w.pd.LFG {
		t.Error("#LFG register did not register")
	}
	w.dave.Queue().GetPending()
	w.say(w.dave, "/lfg")
	if got := texts(heard(t, w.dave)); len(got) != 2 || got[0] != "You are registered as looking for a group." || !strings.Contains(got[1], "/lfg on") {
		t.Errorf("/lfg alone: %q", got)
	}
	w.dave.Queue().GetPending()
	w.say(w.dave, "/lfg maybe")
	if got := heard(t, w.dave); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "/lfg maybe".` {
		t.Errorf("/lfg maybe: %+v", got)
	}
	// nobody else heard any of it
	if got := heard(t, w.cara); len(got) != 0 {
		t.Errorf("a bystander heard %+v", got)
	}
}

// The flag is the Player's: the logout's removal ends it, and the next login
// (a new Player) starts unregistered.
func TestLFGEndsAtLogout(t *testing.T) {
	w := newLFGWorld(t)
	w.gw.RemovePlayerBySession(w.bob)
	if got := rowNames(lfgSearch(t, w.h, w.alice, 0, -1, -1, 1, 60)); len(got) != 1 || got[0] != "Cara" {
		t.Errorf("after Bob's logout: %v", got)
	}
	again, pb := w.join(2, "Bob", 1020, 2002, 0)
	pb.Level, pb.Class, pb.Race = 22, 9, 1
	if pb.LFG {
		t.Error("the new login came in registered")
	}
	_ = again
	if got := rowNames(lfgSearch(t, w.h, w.alice, 0, -1, -1, 1, 60)); len(got) != 1 || got[0] != "Cara" {
		t.Errorf("after Bob's new login: %v", got)
	}
}

// Hide Chat (the disc's 0x0B01 {1, 0}) keeps the group's chat from a member,
// the sender still hears its own; Show Chat (the live 0x0801 sub 1) brings it
// back. The control: before hiding, the member hears it.
func TestGroupChatShowHide(t *testing.T) {
	w := newLFGWorld(t)
	groupLine := func(from *session.Session) {
		for _, s := range []*session.Session{w.erin, w.fay} {
			s.Queue().GetPending()
		}
		w.mode(from, ChatModeGroup)
		w.say(from, "pull")
	}
	gotGroup := func(s *session.Session) int {
		n := 0
		for _, l := range heard(t, s) {
			if l.op == protocol.OpcodeGroupChat {
				n++
			}
		}
		return n
	}
	groupLine(w.fay)
	if gotGroup(w.erin) != 1 || gotGroup(w.fay) != 1 {
		t.Fatalf("before hiding: Erin %d, Fay %d", gotGroup(w.erin), gotGroup(w.fay))
	}
	if err := w.h.HandleGroupShow(w.erin, incoming(protocol.OpcodeGroupShow, []byte{1, 0})); err != nil {
		t.Fatal(err)
	}
	if !w.pe.GroupChatHidden {
		t.Fatal("0x0B01 {1, 0} did not hide")
	}
	groupLine(w.fay)
	if gotGroup(w.erin) != 0 || gotGroup(w.fay) != 1 {
		t.Errorf("hidden: Erin %d (want 0), Fay %d", gotGroup(w.erin), gotGroup(w.fay))
	}
	groupLine(w.erin) // the one who hid it still hears its own line
	if gotGroup(w.erin) != 1 || gotGroup(w.fay) != 1 {
		t.Errorf("Erin's own line: Erin %d, Fay %d", gotGroup(w.erin), gotGroup(w.fay))
	}
	lfgSend(t, w.h, w.erin, 1, func(b *buffer.Writer) { b.WriteByte(1) })
	groupLine(w.fay)
	if gotGroup(w.erin) != 1 {
		t.Errorf("shown again: Erin %d", gotGroup(w.erin))
	}
}

package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// queued is every message of op queued for the session's client, whole.
func queued(sess *session.Session, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == op {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the live duel window's messages (0x7E09, a u32 1)
// and the live client's feature word with the duel bit among the rest (0x07F5,
// 0x1B: livefeatures.go), once.
func TestDuelOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queued(sess, protocol.OpcodeDuelWindow), []byte{0x09, 0x7E, 1, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the duel offer %x, want one %x", got, want)
	}
	if got, want := queued(sess, protocol.OpcodeLiveFeatures), []byte{0xF5, 0x07, 0x1B, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the feature word %x, want one %x", got, want)
	}
}

// The offer's answer is the player's: 1 the live window, 0 boxes again.
func TestHandleDuelWindow(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alice", 100, 100, 0)
	h := NewDuelHandler(c.gw)
	if err := h.HandleDuelWindow(sa, incoming(protocol.OpcodeDuelWindow, []byte{1, 0, 0, 0})); err != nil || !pa.DuelWindow {
		t.Fatalf("answered 1: window %v, err %v", pa.DuelWindow, err)
	}
	h.HandleDuelWindow(sa, incoming(protocol.OpcodeDuelWindow, []byte{0, 0}))
	if !pa.DuelWindow {
		t.Fatal("a short answer changed it")
	}
	h.HandleDuelWindow(sa, incoming(protocol.OpcodeDuelWindow, []byte{0, 0, 0, 0}))
	if pa.DuelWindow {
		t.Fatal("answered 0: still the live window")
	}
}

// /duel at the target, /duel <name>, /duel accept, /duel cancel's box, and
// Sandstorm's challenge "/tell <name> duel" (any client: a PS2 forwards
// /tell), which is a challenge and not a tell.
func TestDuelChatCommands(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alice", 100, 100, 0)
	sb, pb := c.join(2, "Bob", 104, 100, 0)
	c.send(sa, protocol.OpcodeClientSayChat, wide("/duel", c.gw.IDSeenBy(sa, pb)))
	if id, on := c.gw.DuelOf(pb); id == 0 || on {
		t.Fatalf("/duel at the target: duel %d on %v", id, on)
	}
	if got := queued(sb, protocol.OpcodeQuestPopupMulti); len(got) != 1 {
		t.Fatalf("Bob was sent %d boxes", len(got))
	}
	c.send(sb, protocol.OpcodeClientSayChat, wide("/duel accept", 0xFFFFFFFF))
	if _, on := c.gw.DuelOf(pa); !on {
		t.Fatal("/duel accept did not begin it")
	}
	sa.Queue().GetPending()
	c.send(sa, protocol.OpcodeClientSayChat, wide("/duel cancel", 0xFFFFFFFF))
	box := queued(sa, protocol.OpcodeQuestPopupMulti)
	if want := entity.BuildDuelBox(entity.DuelCancelBox, "Cancel duel?", []string{"Yes", "No"}); len(box) != 1 || !bytes.Equal(box[0], want) {
		t.Fatalf("/duel cancel's box % X", box)
	}
	ih := &InteractHandler{gameWorld: c.gw}
	ans := make([]byte, 5)
	binary.LittleEndian.PutUint32(ans, entity.DuelCancelBox)
	ans[4] = 0
	if err := ih.HandleDialogueOption(sa, incoming(protocol.OpcodeDialogueBoxOption, ans)); err != nil {
		t.Fatal(err)
	}
	if id, _ := c.gw.DuelOf(pa); id != 0 {
		t.Fatal("Yes on Cancel duel? kept the duel")
	}

	// Sandstorm's way, by name, from a PS2 that forwards only /tell
	sa.Queue().GetPending()
	c.send(sb, protocol.OpcodeClientSayChat, wide("/tell Alice duel", 0xFFFFFFFF))
	if id, on := c.gw.DuelOf(pa); id == 0 || on {
		t.Fatalf("/tell Alice duel: duel %d on %v", id, on)
	}
	for _, l := range heard(t, sa) {
		if l.op == protocol.OpcodeColoredChat && l.text == "Bob tells you, 'duel'" {
			t.Fatal("the challenge was delivered as a tell")
		}
	}
	if got := queued(sa, protocol.OpcodeQuestPopupMulti); len(got) != 1 {
		t.Fatalf("Alice was sent %d boxes", len(got))
	}
	// the control: a tell with more words is a tell
	c.send(sa, protocol.OpcodeClientSayChat, wide("/tell Bob duel me later", 0xFFFFFFFF))
	found := false
	for _, l := range heard(t, sb) {
		found = found || (l.op == protocol.OpcodeColoredChat && l.text == "Alice tells you, 'duel me later'")
	}
	if !found {
		t.Fatalf("a plain tell was not delivered: %+v", heard(t, sb))
	}
	for _, c := range []struct {
		text  string
		wager int
		ok    bool
	}{{" Duel ", 0, true}, {"duel 250", 250, true}, {"duels", 0, false}, {"", 0, false}, {"duel me later", 0, false}, {"duel 5 6", 0, false}} {
		if w, ok := duelTell(c.text); ok != c.ok || w != c.wager {
			t.Errorf("duelTell(%q) = %d, %v; want %d, %v", c.text, w, ok, c.wager, c.ok)
		}
	}
	for _, c := range []struct {
		rest, name string
		wager      int
	}{{"", "", 0}, {"Bob", "Bob", 0}, {"100", "", 100}, {"Bob 100", "Bob", 100}, {" Bob  250 ", "Bob", 250}} {
		if name, w := duelWords(c.rest); name != c.name || w != c.wager {
			t.Errorf("duelWords(%q) = %q, %d; want %q, %d", c.rest, name, w, c.name, c.wager)
		}
	}
}

// The live window's 0x1501: sub 1 challenges the target it names, sub 3
// accepts the invitation it names, sub 4 surrenders, sub 2 declines; a
// wager sub-op changes nothing.
func TestHandleDuelRequest(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alice", 100, 100, 0)
	sb, pb := c.join(2, "Bob", 104, 100, 0)
	pa.DuelWindow, pb.DuelWindow = true, true
	h := NewDuelHandler(c.gw)
	up := func(s *session.Session, sub byte, vals ...uint32) {
		t.Helper()
		b := []byte{sub, 0, 0, 0, 0}
		for _, v := range vals {
			b = binary.LittleEndian.AppendUint32(b, v)
		}
		if err := h.HandleDuelRequest(s, incoming(protocol.OpcodeDuelRequest, b)); err != nil {
			t.Fatal(err)
		}
	}
	up(sa, 1, c.gw.IDSeenBy(sa, pb))
	id, on := c.gw.DuelOf(pb)
	if id == 0 || on {
		t.Fatalf("sub 1: duel %d on %v", id, on)
	}
	if got := queued(sb, protocol.OpcodeDuel); len(got) != 1 || got[0][2] != 2 || binary.LittleEndian.Uint32(got[0][3:]) != id {
		t.Fatalf("the invitation % X", got)
	}
	up(sb, 5, 1000) // the wager's tunar before the accept: nothing
	if _, on := c.gw.DuelOf(pb); on {
		t.Fatal("a wager sub-op began it")
	}
	up(sb, 3, id) // accepted: two live windows set their wagers first
	// the window opened (13), then zeroed (12: the live 13 keeps the last
	// wager's tunar and accepts)
	if got := queued(sa, protocol.OpcodeDuel); len(got) < 4 || got[len(got)-2][2] != 13 || got[len(got)-1][2] != 12 {
		t.Fatalf("the wager window did not open: % X", got)
	}
	pa.Tunar, pb.Tunar = 100, 100
	up(sa, 5, 40)
	up(sb, 6, 7, 1) // an item: refused
	up(sa, 8)
	up(sb, 8)
	if _, on := c.gw.DuelOf(pa); !on {
		t.Fatal("both accepted the wager: no duel")
	}
	if pa.Tunar != 60 || pb.Tunar != 100 {
		t.Fatalf("the purses %d and %d, want 60 and 100", pa.Tunar, pb.Tunar)
	}
	up(sa, 4)
	if id, _ := c.gw.DuelOf(pa); id != 0 {
		t.Fatal("sub 4 did not end it")
	}
	pa.DuelWindow, pb.DuelWindow = false, false
	c.gw.DuelChallenge(sb, c.gw.IDSeenBy(sb, pa), "", 0, time.Now())
	id, _ = c.gw.DuelOf(pa)
	up(sa, 2, id)
	if id, _ := c.gw.DuelOf(pa); id != 0 {
		t.Fatal("sub 2 did not decline it")
	}
}

// The duel purse on a database: a named wager is held as the duel begins
// (both purses written, the stakes in duel_stakes, each client told its
// purse) and paid at the end (the winner's purse, the stakes gone); stakes a
// stopped server left held go back at the start.
func TestDuelPurseOnTheDatabase(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/duel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	chars := database.NewSQLiteCharacterStore(db)
	c := newChatWorld(t)
	join := func(id int, name string, x float32, tunar int) (*session.Session, *entity.Player) {
		rec := &database.CharacterRecord{CharName: name, AccountID: id}
		if err := chars.Create(rec); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, tunar, rec.ServerID); err != nil {
			t.Fatal(err)
		}
		s, p := c.join(id, name, x, 100, 0)
		p.CharServerID, p.Tunar = uint32(rec.ServerID), tunar
		return s, p
	}
	sa, pa := join(1, "Alice", 100, 700)
	sb, pb := join(2, "Bob", 104, 300)
	purse := func(p *entity.Player) int {
		var n int
		if err := db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, p.CharServerID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	told := func(s *session.Session) int {
		got := queued(s, protocol.OpcodePlayerTunar)
		if len(got) == 0 {
			return -1
		}
		v, _ := readZigzag(got[len(got)-1][2:])
		return v
	}
	if err := db.DuelHold(999, []database.DuelStake{{ServerID: pa.CharServerID, Tunar: 50, PurseNow: 650}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	StartDuelPurse(c.gw, db)
	if purse(pa) != 700 {
		t.Fatalf("a stopped duel's stake was not given back: purse %d", purse(pa))
	}

	c.gw.DuelChallenge(sa, 0, "Bob", 200, time.Now())
	id, _ := c.gw.DuelOf(pb)
	c.gw.DuelAnswer(sb, id, true, time.Now())
	if purse(pa) != 500 || purse(pb) != 100 || pa.Tunar != 500 || pb.Tunar != 100 {
		t.Fatalf("held: purses %d and %d, server %d and %d", purse(pa), purse(pb), pa.Tunar, pb.Tunar)
	}
	if n, _ := db.DuelHeld(id); n != 400 {
		t.Fatalf("the table holds %d, want 400", n)
	}
	if told(sa) != 500 || told(sb) != 100 {
		t.Fatalf("told %d and %d", told(sa), told(sb))
	}
	c.gw.DuelCancel(sa, time.Now()) // Alice surrenders: Bob takes 400
	if purse(pa) != 500 || purse(pb) != 500 || pb.Tunar != 500 {
		t.Fatalf("paid: purses %d and %d, Bob %d", purse(pa), purse(pb), pb.Tunar)
	}
	if n, _ := db.DuelHeld(id); n != 0 {
		t.Fatalf("still held %d", n)
	}
	if told(sb) != 500 {
		t.Fatalf("Bob was told %d", told(sb))
	}
}

// readZigzag reads a 7-bit zigzag int as Write7BitEncodedInt64 writes it.
func readZigzag(b []byte) (int, int) {
	var u uint64
	var shift uint
	for i, c := range b {
		u |= uint64(c&0x7F) << shift
		shift += 7
		if c&0x80 == 0 {
			return int(int64(u>>1) ^ -int64(u&1)), i + 1
		}
	}
	return 0, len(b)
}

package handlers

import (
	"encoding/binary"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The larger pack and bank (itemlists.go): a PS2, and any client that never
// answers the offer, is never sent more than CLIENT's 40 records in a list;
// a client that holds more is sent all it holds; what a smaller client cannot
// hold is kept for the character, untouched, and comes back at a login with
// room. Every test has its control.

// listsWorld is a character in a World with a session, its rows in a
// temporary database.
type listsWorld struct {
	t      *testing.T
	db     *database.SQLiteDB
	inv    *database.InventoryStore
	h      *CharacterHandler
	w      *entity.World
	mgr    *session.Manager
	sid    uint32
	player *entity.Player
	sword  int // a single one
	gem    int // a stack of 20
	charm  int // a Lore one
	port   int // the sessions made
}

func newListsWorld(t *testing.T) *listsWorld {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/lists.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Packrat", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	f := &listsWorld{t: t, db: db, inv: database.NewInventoryStore(db), sid: rec.ServerID}
	f.h = NewCharacterHandler(store, nil, f.inv, database.NewSpellbookStore(db))
	f.w = entity.NewWorld()
	f.mgr = session.NewManager()
	f.w.SessionManager = f.mgr
	f.h.gameWorld = f.w
	f.player = &entity.Player{ServerID: 700, CharServerID: rec.ServerID, Name: "Packrat"}
	f.w.Players[f.player.ServerID] = f.player
	f.sword = f.pattern("Plain Sword", 1, 12, 0)
	f.gem = f.pattern("Rough Gem", 20, -1, 0)
	f.charm = f.pattern("Unique Charm", 1, -1, 1)
	t.Cleanup(func() { entity.SetItemSlots(0, 0) })
	keptNotesMu.Lock()
	delete(keptNotes, rec.ServerID)
	keptNotesMu.Unlock()
	return f
}

func (f *listsWorld) pattern(name string, maxStack, wear, lore int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 0, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, ?, 100, 0, 32767, 1023, 0, ?, 0, 0, ?, 'An item.', 0, 0)`,
		wear, maxStack, lore, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// own gives the character n rows of a pattern at a location (each 1, hp 77,
// 3 charges), returning their ids.
func (f *listsWorld) own(pattern, n, location int) []int {
	f.t.Helper()
	var ids []int
	for range n {
		id, err := f.inv.AddCharItem(f.sid, pattern, 1, 77, 3, location)
		if err != nil {
			f.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (f *listsWorld) wear(itemID, slot int) {
	f.t.Helper()
	if err := f.inv.SetEquipLoc(f.sid, itemID, slot); err != nil {
		f.t.Fatal(err)
	}
}

// session is a new client's session: answered the offer (pack, bank) or not
// (a PS2: 0, 0), its player the world's.
func (f *listsWorld) session(pack, bank int) *session.Session {
	f.t.Helper()
	f.port++
	sess := f.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000 + f.port}, uint32(f.port), 1, 1, 2, true)
	sess.SetECSEntity(f.player)
	if pack > 0 || bank > 0 {
		if sess.OfferItemListsOnce() != true {
			f.t.Fatal("offered twice")
		}
		answer := make([]byte, 12)
		binary.LittleEndian.PutUint32(answer[0:], 1)
		binary.LittleEndian.PutUint32(answer[4:], uint32(pack))
		binary.LittleEndian.PutUint32(answer[8:], uint32(bank))
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeItemLists)
		msg.SetData(answer)
		if err := NewItemListsHandler().HandleItemLists(sess, msg); err != nil {
			f.t.Fatal(err)
		}
	}
	return sess
}

// rowsAt is the character's rows by location, by id: (stack, hp, charges,
// equiploc) each.
func (f *listsWorld) rowsAt() map[int]map[int][4]int {
	f.t.Helper()
	rows, err := f.db.Query(`SELECT itemid, location, stackleft, remainHP, remaincharge, equiploc FROM charinventory WHERE serverid = ?`, f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]map[int][4]int{}
	for rows.Next() {
		var id, loc, a, b, c, d int
		if err := rows.Scan(&id, &loc, &a, &b, &c, &d); err != nil {
			f.t.Fatal(err)
		}
		if out[loc] == nil {
			out[loc] = map[int][4]int{}
		}
		out[loc][id] = [4]int{a, b, c, d}
	}
	return out
}

// dump is a MemoryDump made for sess (login: a login's), and what was queued
// with it, in order.
func (f *listsWorld) dump(sess *session.Session, login bool) (dump []byte, queued []protocol.GameOpcode) {
	f.t.Helper()
	sess.Queue().GetPending() // what came before
	f.h.queueMemoryDumpAt(sess, f.player, login)
	// a large message goes as fragments (queue.go), each under its opcode:
	// put back together
	for _, m := range sess.Queue().GetPending() {
		if m.Opcode == protocol.OpcodeMemoryDump {
			if dump == nil {
				queued = append(queued, m.Opcode)
			}
			dump = append(dump, m.Data()...)
			continue
		}
		queued = append(queued, protocol.GameOpcode(binary.LittleEndian.Uint16(m.Data())))
	}
	if dump == nil {
		f.t.Fatal("no MemoryDump queued")
	}
	return dump, queued
}

// dumpedItem is a record as CLIENT's ReadNetItem reads it (writeItem's order).
type dumpedItem struct {
	stack, hp, charges, equip, quick int64
	key                              uint32
	pattern                          int64
}

func readDumpedItem(r *buffer.Reader) (dumpedItem, bool) {
	var d dumpedItem
	ok := true
	z := func() int64 {
		v, err := r.Read7BitEncodedInt64()
		ok = ok && err == nil
		return v
	}
	wideStr := func() {
		n, err := r.ReadInt32()
		if err != nil || n < 0 || n > 512 {
			ok = false
			return
		}
		_, err = r.ReadBytes(int(n) * 2)
		ok = ok && err == nil
	}
	d.stack, d.hp, d.charges, d.equip, d.quick = z(), z(), z(), z(), z()
	k, err := r.ReadUint32()
	ok = ok && err == nil
	d.key = k
	d.pattern = z()
	for range 22 { // ItemCost .. Craft
		z()
	}
	wideStr()
	wideStr()
	n := z()
	if n < 0 || n > 40 {
		return d, false
	}
	for range n {
		z()
		z()
	}
	return d, ok
}

// dumpedLists finds the dump's inventory and bank lists (Msg000D's
// m_inventory.Read and m_bank.Read: a zigzag count, a u32, the records) as a
// list of pack records followed at once by one of bank records, and reads
// them whole.
func dumpedLists(t *testing.T, dump []byte, pack, bank int) (inv, bnk []dumpedItem) {
	t.Helper()
	head := func(n int) []byte {
		w := buffer.NewWriterSize(16)
		w.Write7BitEncodedInt64(int64(n))
		w.WriteUint32(uint32(n))
		return w.Bytes()
	}
	read := func(r *buffer.Reader, n int) ([]dumpedItem, bool) {
		var out []dumpedItem
		for range n {
			d, ok := readDumpedItem(r)
			if !ok {
				return nil, false
			}
			out = append(out, d)
		}
		return out, true
	}
	hp, hb := head(pack), head(bank)
	for at := 0; at+len(hp) <= len(dump); at++ {
		if string(dump[at:at+len(hp)]) != string(hp) {
			continue
		}
		r := buffer.NewReader(dump[at+len(hp):])
		inv, ok := read(r, pack)
		if !ok || r.Remaining() < len(hb) {
			continue
		}
		next, _ := r.ReadBytes(len(hb))
		if string(next) != string(hb) {
			continue
		}
		if bnk, ok = read(r, bank); ok {
			return inv, bnk
		}
	}
	t.Fatalf("no inventory list of %d followed by a bank list of %d in the %d-byte dump", pack, bank, len(dump))
	return nil, nil
}

// fill gives the character 60 pack rows, three of them worn (one at the very
// end: the tail a PS2 is not sent), and 50 bank rows.
func (f *listsWorld) fill() (pack, bank []int) {
	pack = f.own(f.sword, 60, 0)
	f.wear(pack[2], 14)
	f.wear(pack[45], 0)
	f.wear(pack[59], 12)
	bank = f.own(f.gem, 50, 1)
	return pack, bank
}

// The hard rule: a session that never answered the offer (a PS2) is sent 40
// of each, every worn item among them, and nothing past it; the rest is kept,
// every row as it was. The control: a client that answered 100 of each is sent
// all 60 and 50, and nothing is kept.
func TestItemListsAPS2IsSentFortyAndTheRestIsKept(t *testing.T) {
	f := newListsWorld(t)
	entity.SetItemSlots(100, 100)
	pack, bank := f.fill()
	before := f.rowsAt()

	ps2 := f.session(0, 0)
	dump, queued := f.dump(ps2, true)
	if slices.Contains(queued, protocol.OpcodeItemListsInForce) {
		t.Errorf("a PS2 was sent 0x7E2A: %v", queued)
	}
	inv, bnk := dumpedLists(t, dump, 40, 40)
	for i, d := range bnk {
		if d.key != uint32(i) || d.pattern != int64(f.gem) {
			t.Errorf("bank record %d: key %d item %d", i, d.key, d.pattern)
		}
	}
	worn := 0
	for i, d := range inv {
		if d.key != uint32(i) {
			t.Errorf("record %d keyed %d", i, d.key)
		}
		if d.equip >= 0 {
			worn++
		}
	}
	if worn != 3 {
		t.Errorf("%d worn records sent, want all 3", worn)
	}
	if l := sessionInvList(ps2); l == nil || len(l.recs) != 40 || l.capacity() != 40 {
		t.Errorf("the PS2's pack list: %+v", l)
	}
	if l := sessionBankList(ps2); l == nil || len(l.recs) != 40 || l.capacity() != 40 {
		t.Errorf("the PS2's bank list: %+v", l)
	}
	after := f.rowsAt()
	if len(after[database.LocPack]) != 40 || len(after[database.LocPackKept]) != 20 ||
		len(after[database.LocBank]) != 40 || len(after[database.LocBankKept]) != 10 {
		t.Fatalf("rows by place after: pack %d kept %d, bank %d kept %d", len(after[0]), len(after[2]), len(after[1]), len(after[3]))
	}
	// nothing lost, nothing made, nothing changed but the place
	for _, ids := range [][]int{pack, bank} {
		for _, id := range ids {
			was, here := before[0][id], false
			if v, ok := before[1][id]; ok {
				was = v
			}
			for loc := range 4 {
				if v, ok := after[loc][id]; ok {
					if here {
						t.Errorf("item %d in two places", id)
					}
					here = true
					if v != was {
						t.Errorf("item %d changed: %v, was %v", id, v, was)
					}
				}
			}
			if !here {
				t.Errorf("item %d lost", id)
			}
		}
	}
	// the worn tail stayed; the unworn tail went
	if _, ok := after[database.LocPack][pack[59]]; !ok {
		t.Error("the worn last record was kept, not sent")
	}
	if _, ok := after[database.LocPackKept][pack[58]]; !ok {
		t.Error("the unworn tail was sent")
	}
	n := keptNotes[f.sid]
	if lines := keptLines(n); len(lines) != 2 || !strings.Contains(lines[0], "holds 40 items here, so 20 more of your items are kept") ||
		!strings.Contains(lines[0], "a game client that holds 100") || !strings.Contains(lines[1], "10 more") {
		t.Errorf("the login's lines: %q", lines)
	}
	// on a server that holds 40 itself, nowhere else to go
	if lines := keptLines(keptNote{pack: 40, bank: 40, mostPack: 40, mostBank: 40, keptPack: 1}); len(lines) != 1 ||
		!strings.Contains(lines[0], "1 more of your items is kept") || strings.Contains(lines[0], "client") {
		t.Errorf("a server of 40's line: %q", lines)
	}

	// the control: a client that holds 100 of each, and a login: all of it
	f2 := newListsWorld(t)
	entity.SetItemSlots(100, 100)
	f2.fill()
	big := f2.session(100, 100)
	dump, queued = f2.dump(big, true)
	at := slices.Index(queued, protocol.OpcodeItemListsInForce)
	if at < 0 || at > slices.Index(queued, protocol.OpcodeMemoryDump) {
		t.Errorf("0x7E2A not before the dump: %v", queued)
	}
	dumpedLists(t, dump, 60, 50)
	if rows := f2.rowsAt(); len(rows[database.LocPackKept])+len(rows[database.LocBankKept]) != 0 {
		t.Errorf("kept for a client that holds them all: %d", len(rows[2])+len(rows[3]))
	}
	if l := sessionInvList(big); l.capacity() != 100 || len(l.recs) != 60 {
		t.Errorf("the large pack list: cap %d, %d records", l.capacity(), len(l.recs))
	}
}

// Kept rows come back at a login with room, the first kept first, at the end
// of their list; a respawn's dump never brings them back (the control), and a
// PS2 that freed room gets that many back at its next login.
func TestItemListsKeptRowsComeBackAtALoginWithRoom(t *testing.T) {
	f := newListsWorld(t)
	entity.SetItemSlots(100, 100)
	pack, _ := f.fill()
	f.dump(f.session(0, 0), true)
	kept := f.rowsAt()[database.LocPackKept]
	if len(kept) != 20 {
		t.Fatalf("%d kept", len(kept))
	}

	// a PS2 sells five: a respawn's dump brings none back
	ps2 := f.session(0, 0)
	f.dump(ps2, true)
	for _, id := range pack[3:8] {
		if err := f.inv.DeleteCharItem(f.sid, id); err != nil {
			t.Fatal(err)
		}
	}
	f.dump(ps2, false)
	if rows := f.rowsAt(); len(rows[database.LocPack]) != 35 || len(rows[database.LocPackKept]) != 20 {
		t.Fatalf("a respawn moved rows: pack %d kept %d", len(rows[0]), len(rows[2]))
	}
	// its next login: five back, the first five kept
	dump, _ := f.dump(f.session(0, 0), true)
	dumpedLists(t, dump, 40, 40)
	rows := f.rowsAt()
	if len(rows[database.LocPack]) != 40 || len(rows[database.LocPackKept]) != 15 {
		t.Fatalf("after a PS2 login with room: pack %d kept %d", len(rows[0]), len(rows[2]))
	}
	if lines := keptLines(keptNotes[f.sid]); len(lines) < 1 || !strings.Contains(lines[0], "5 items kept for you came back to your pack") {
		t.Errorf("lines: %q", lines)
	}

	// a client that holds 100: every kept row back, at the end of its list
	dump, _ = f.dump(f.session(100, 100), true)
	inv, _ := dumpedLists(t, dump, 55, 50)
	rows = f.rowsAt()
	if len(rows[database.LocPackKept])+len(rows[database.LocBankKept]) != 0 {
		t.Fatalf("still kept: %d", len(rows[2])+len(rows[3]))
	}
	items, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	var order []int
	for _, it := range items {
		if it.Location == database.LocPack {
			order = append(order, it.ItemID)
		}
	}
	// the first PS2 dump kept the unworn tail, 38..44 and 46..58 (45 and 59
	// worn); the PS2's login with room brought 38..42 back; these are the rest,
	// in the order they were kept, after every row the pack held
	want := append([]int{pack[43], pack[44]}, pack[46:59]...)
	if len(order) != len(inv) || !slices.Equal(order[len(order)-15:], want) {
		t.Errorf("the pack's last 15: %v, want the kept rows %v", order[len(order)-15:], want)
	}
	for i, d := range inv {
		if d.pattern != int64(f.sword) {
			t.Errorf("record %d is item %d", i, d.pattern)
		}
	}
}

// What is kept is out of the game's reach: no read of a pack or bank finds it
// and no change of one touches it; the controls: the same calls on a row the
// client holds.
func TestItemListsKeptRowsAreUnreachable(t *testing.T) {
	f := newListsWorld(t)
	pack, _ := f.fill()
	f.dump(f.session(0, 0), true)
	keptID, heldID := pack[58], pack[0]
	items, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ItemID == keptID {
			t.Fatal("GetCharacterItems read a kept row")
		}
	}
	if len(items) != 80 {
		t.Errorf("%d rows read, want 80", len(items))
	}
	if _, err := f.inv.GetCharItem(f.sid, keptID); err == nil {
		t.Error("GetCharItem read a kept row")
	}
	if err := f.inv.SetItemStack(f.sid, keptID, 5); err == nil {
		t.Error("a kept row's stack changed")
	}
	if err := f.inv.SetItemCharges(f.sid, keptID, 0); err == nil {
		t.Error("a kept row's charges changed")
	}
	if err := f.inv.DeleteCharItem(f.sid, keptID); err == nil {
		t.Error("a kept row was deleted")
	}
	if err := f.inv.MoveCharItem(f.sid, keptID, database.LocBank); err == nil {
		t.Error("a kept row was moved to the bank")
	}
	if v := f.rowsAt()[database.LocPackKept][keptID]; v != [4]int{1, 77, 3, -1} {
		t.Errorf("the kept row is %v", v)
	}
	// the controls
	if _, err := f.inv.GetCharItem(f.sid, heldID); err != nil {
		t.Errorf("a held row: %v", err)
	}
	if err := f.inv.SetItemCharges(f.sid, heldID, 2); err != nil {
		t.Errorf("a held row's charges: %v", err)
	}
}

// A Lore item kept for the character is one held: no second comes. The
// control: without it kept, one may.
func TestItemListsAKeptLoreItemIsHeld(t *testing.T) {
	f := newListsWorld(t)
	f.own(f.sword, 60, 0)
	f.own(f.charm, 1, 0) // the 61st: kept from a PS2
	sess := f.session(0, 0)
	f.dump(sess, true)
	p, ok, err := f.inv.ItemPatternByID(f.charm)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	l := sessionInvList(sess)
	for range 2 {
		l.took(0, 1) // room in the list
	}
	if got := l.loreRoom(&p, l.wouldAdd(p.PatternID, 1, 1)); got != 0 {
		t.Errorf("a kept Lore item let %d more come", got)
	}
	if craftRoom(l, nil, &p, 1) {
		t.Error("a craft made a second of a kept Lore item")
	}
	other := newInvListCap(nil, 0)
	if got := other.loreRoom(&p, other.wouldAdd(p.PatternID, 1, 1)); got != 1 {
		t.Errorf("the control: %d", got)
	}
}

// A list's capacity is its client's: CLIENT's 40 by default, the answer's
// past it. The control is the same list filled to the other size.
func TestItemListsAListHoldsItsClients(t *testing.T) {
	fill := func(l *invList, n int) int {
		took := 0
		for i := range n {
			if _, k := l.add(1000+i, 5000+i, 1, 1); k == 1 {
				took++
			}
		}
		return took
	}
	if n := fill(newInvListCap(nil, 0), 41); n != 40 {
		t.Errorf("CLIENT's list took %d of 41", n)
	}
	big := newInvListCap(nil, 100)
	if n := fill(big, 101); n != 100 {
		t.Errorf("a list of 100 took %d of 101", n)
	}
	if big.wouldAdd(1, 1, 1) != 0 || len(big.roomFor(nil, []invGive{{patternID: 1, n: 1, maxStack: 1}})) != 1 {
		t.Error("a full list of 100 had room")
	}
	l40 := newInvListCap(nil, 40)
	fill(l40, 30)
	if short := l40.roomFor(nil, []invGive{{patternID: 1, n: 10, maxStack: 1}}); len(short) != 0 {
		t.Errorf("ten into thirty of forty: %v", short)
	}
	if short := l40.roomFor(nil, []invGive{{patternID: 1, n: 11, maxStack: 1}}); len(short) != 1 {
		t.Errorf("eleven into thirty of forty: %v", short)
	}
	if c := (&invList{}).clone(); c.capacity() != 40 {
		t.Errorf("a clone of CLIENT's: %d", c.capacity())
	}
	if c := big.clone(); c.capacity() != 100 {
		t.Errorf("a clone of 100: %d", c.capacity())
	}
}

// The offer goes out once a session, only where the server holds more; an
// answer is taken only where it was offered, between 40 and the server's.
func TestItemListsOfferAndAnswer(t *testing.T) {
	t.Cleanup(func() { entity.SetItemSlots(0, 0) })
	answer := func(sess *session.Session, words ...uint32) {
		b := make([]byte, 4*len(words))
		for i, v := range words {
			binary.LittleEndian.PutUint32(b[4*i:], v)
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeItemLists)
		msg.SetData(b)
		if err := NewItemListsHandler().HandleItemLists(sess, msg); err != nil {
			t.Fatal(err)
		}
	}
	caps := func(sess *session.Session) [2]int {
		a, b := listCaps(sess)
		return [2]int{a, b}
	}

	entity.SetItemSlots(0, 0)
	s := testSession()
	SendItemListsOffer(s)
	if n := len(s.Queue().GetPending()); n != 0 {
		t.Errorf("a server of 40 offered (%d messages)", n)
	}
	answer(s, 1, 100, 100)
	if caps(s) != [2]int{40, 40} {
		t.Errorf("an answer to no offer taken: %v", caps(s))
	}

	entity.SetItemSlots(100, 100)
	s = testSession()
	if caps(s) != [2]int{40, 40} {
		t.Errorf("a client that never answered (a PS2): %v", caps(s))
	}
	sendItemListsInForce(s, 40, 40)
	if n := len(s.Queue().GetPending()); n != 0 {
		t.Errorf("a PS2 told the sizes (%d messages)", n)
	}
	SendItemListsOffer(s)
	SendItemListsOffer(s)
	msgs := s.Queue().GetPending()
	if len(msgs) != 1 {
		t.Fatalf("%d offers", len(msgs))
	}
	if b := msgs[0].Data(); len(b) != 14 || protocol.GameOpcode(binary.LittleEndian.Uint16(b)) != protocol.OpcodeItemLists ||
		binary.LittleEndian.Uint32(b[2:]) != 1 || binary.LittleEndian.Uint32(b[6:]) != 100 || binary.LittleEndian.Uint32(b[10:]) != 100 {
		t.Errorf("the offer: % x", b)
	}
	answer(s, 1)
	if caps(s) != [2]int{40, 40} {
		t.Errorf("an answer with no sizes taken: %v", caps(s))
	}
	answer(s, 1, 100, 100)
	if caps(s) != [2]int{100, 100} {
		t.Errorf("the answer: %v", caps(s))
	}
	answer(s, 1, 5000, 12)
	if caps(s) != [2]int{100, 40} {
		t.Errorf("an answer past the server's and under CLIENT's: %v", caps(s))
	}
	entity.SetItemSlots(60, 100)
	if caps(s) != [2]int{60, 40} {
		t.Errorf("a server of 60: %v", caps(s))
	}
	answer(s, 0)
	if caps(s) != [2]int{40, 40} {
		t.Errorf("a stop: %v", caps(s))
	}
	sendItemListsInForce(s, 40, 40)
	if msgs := s.Queue().GetPending(); len(msgs) != 1 {
		t.Errorf("a client that stopped was not told 40 (%d messages)", len(msgs))
	}
	entity.SetItemSlots(1000, 3)
	if p, b := entity.ItemSlots(); p != entity.MaxItemSlots || b != 40 {
		t.Errorf("config clamped to %d and %d", p, b)
	}
}

// planItemLists alone: the worn stay whatever their place, the kept go after
// those kept before, and a restore takes the first kept first after the last
// held row.
func TestPlanItemLists(t *testing.T) {
	row := func(id, loc, equip, list int) database.ItemListRow {
		return database.ItemListRow{ItemID: id, Location: loc, EquipLoc: equip, ListNum: list}
	}
	var rows []database.ItemListRow
	for i := range 5 {
		equip := -1
		if i == 4 {
			equip = 3
		}
		rows = append(rows, row(10+i, database.LocPack, equip, i))
	}
	rows = append(rows, row(90, database.LocPackKept, -1, 7))
	p := planItemLists(rows, 3, 40, false)
	want := []database.ItemListMove{{ItemID: 12, From: 0, To: 2, ListNum: 8}, {ItemID: 13, From: 0, To: 2, ListNum: 9}}
	if !slices.Equal(p.moves, want) || p.keptPack != 3 {
		t.Errorf("keep: %+v, %d kept", p.moves, p.keptPack)
	}
	if p := planItemLists(rows, 5, 40, true); len(p.moves) != 0 {
		t.Errorf("full at 5: %+v", p.moves)
	}
	p = planItemLists(rows, 7, 40, true)
	if want := []database.ItemListMove{{ItemID: 90, From: 2, To: 0, ListNum: 5}}; !slices.Equal(p.moves, want) || p.keptPack != 0 {
		t.Errorf("restore: %+v, %d kept", p.moves, p.keptPack)
	}
	if p := planItemLists(rows, 7, 40, false); len(p.moves) != 0 || p.keptPack != 1 {
		t.Errorf("no restore outside a login: %+v", p.moves)
	}
	bank := []database.ItemListRow{row(1, database.LocBank, -1, 0), row(2, database.LocBank, -1, 1), row(3, database.LocBankKept, -1, 0), row(4, database.LocBankKept, -1, 1)}
	p = planItemLists(bank, 40, 3, true)
	if want := []database.ItemListMove{{ItemID: 3, From: 3, To: 1, ListNum: 2}}; !slices.Equal(p.moves, want) || p.keptBank != 1 {
		t.Errorf("the bank's restore: %+v, %d kept", p.moves, p.keptBank)
	}
	// more worn than the list holds still sends every worn row (never past
	// CLIENT's 40: there are 23 slots)
	if h := heldOf([]database.ItemListRow{row(1, 0, 1, 0), row(2, 0, 2, 1), row(3, 0, -1, 2)}, 1); !slices.Equal(h, []bool{true, true, false}) {
		t.Errorf("worn first: %v", h)
	}
}

// A list cut at the dump (a row came in since the plan) keeps its worn rows;
// the control: a list within its size is as it was.
func TestCapEntries(t *testing.T) {
	var rows []database.CharInventoryEntry
	for i := range 45 {
		e := database.CharInventoryEntry{ItemID: i, EquipLoc: -1}
		if i == 44 {
			e.EquipLoc = 5
		}
		rows = append(rows, e)
	}
	got := capEntries(rows, 40)
	if len(got) != 40 || got[39].ItemID != 44 || got[38].ItemID != 38 {
		t.Errorf("cut to %d, last %d", len(got), got[len(got)-1].ItemID)
	}
	if got := capEntries(rows[:40], 40); len(got) != 40 || got[39].ItemID != 39 {
		t.Error("a list within its size was cut")
	}
	// a broken table with more worn rows than the list holds: still never past it
	for i := range rows {
		rows[i].EquipLoc = 1
	}
	if got := capEntries(rows, 40); len(got) != 40 {
		t.Errorf("45 worn rows cut to %d", len(got))
	}
}

// A restart carries a list's size and what is kept beside it; a file from
// before both reads as CLIENT's 40 with nothing kept.
func TestKeptListCarriesItsSize(t *testing.T) {
	l := newInvListCap(nil, 100)
	l.add(1, 2, 1, 1)
	l.kept = map[int]bool{77: true}
	back := keptList(l).list()
	if back.capacity() != 100 || !back.keptHas(77) || len(back.recs) != 1 {
		t.Errorf("carried: cap %d, kept %v", back.capacity(), back.kept)
	}
	old := (&KeptList{Recs: []KeptListRec{{Key: 0, Item: 1, Pattern: 2, Qty: 1}}, Next: 1}).list()
	if old.capacity() != 40 || old.keptHas(77) {
		t.Errorf("an old file: cap %d", old.capacity())
	}
}

// A trade's room is the taker's client's, and all its offers together: four
// swords into a pack of 40 holding 39 is refused, nothing moved (each alone
// would have fitted); the control is the same trade to a taker whose client
// holds 100.
func TestItemListsATradeFitsTheTakersClient(t *testing.T) {
	run := func(takerCap int) (giver, taker []database.CharInventoryEntry) {
		db, err := database.NewSQLite(t.TempDir() + "/trade.db")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		inv := database.NewInventoryStore(db)
		sword := 900200
		if err := database.NewItemPatternStore(db).UpsertItemPattern(&database.ItemPatternRecord{PatternID: sword, ItemName: "plain sword",
			EquipSlot: 14, MaxHP: 100, MaxStack: 1, ClassUse: 32767, RaceUse: 32767, Rent: 1}); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if _, err := inv.AddCharItem(21, sword, 1, 100, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
		for range 39 {
			if _, err := inv.AddCharItem(22, sword, 1, 100, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
		w := entity.NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		mk := func(port int, account int) *session.Session {
			s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 22), Port: port}, uint32(port), uint32(port), uint16(port), 2, true)
			s.SetAccountID(account)
			s.SetInGame(true)
			s.SetCharacterInWorld(true)
			s.UpdateActivity()
			return s
		}
		aSess, bSess := mk(7201, 1), mk(7202, 2)
		w.CreatePlayer(aSess, &database.CharacterRecord{ServerID: 21, CharName: "Giver", AccountID: 1, Level: 10, X: 100, Y: 54, Z: 100})
		tp := w.CreatePlayer(bSess, &database.CharacterRecord{ServerID: 22, CharName: "Taker", AccountID: 2, Level: 10, X: 102, Y: 54, Z: 100})
		rows, _ := inv.GetCharacterItems(21)
		aSess.SetInventoryList(newInvList(rows))
		rows, _ = inv.GetCharacterItems(22)
		bSess.SetInventoryList(newInvListCap(rows, takerCap))
		h := NewTradeHandler(sm, &NPCWindowHandler{inv: inv})
		h.SetGameWorld(w)
		ui := func(s *session.Session, sub byte, fill func(w *buffer.Writer)) {
			t.Helper()
			msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeUI)
			b := make([]byte, 64)
			wr := buffer.NewWriter(b)
			wr.WriteByte(sub)
			wr.WriteUint32(0)
			if fill != nil {
				fill(wr)
			}
			msg.SetData(b[:wr.Position()])
			if err := h.HandleTradeUI(s, msg); err != nil {
				t.Fatal(err)
			}
		}
		ask := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeRequest)
		ask.SetData(u32b(0x7F000000 | uint32(tp.ServerID)))
		if err := h.HandleTradeRequest(aSess, ask); err != nil {
			t.Fatal(err)
		}
		ui(bSess, 2, nil)
		for key := range uint32(4) {
			ui(aSess, 3, func(w *buffer.Writer) { w.WriteUint32(key); w.Write7BitEncodedInt64(1) })
		}
		ui(aSess, 6, nil)
		ui(bSess, 6, nil)
		giver, _ = inv.GetCharacterItems(21)
		taker, _ = inv.GetCharacterItems(22)
		if l := sessionInvList(bSess); len(l.recs) != len(taker) {
			t.Errorf("the taker's list holds %d, its rows %d", len(l.recs), len(taker))
		}
		return giver, taker
	}
	if giver, taker := run(0); len(giver) != 4 || len(taker) != 39 {
		t.Errorf("four into one free place of 40: the giver holds %d, the taker %d", len(giver), len(taker))
	}
	if giver, taker := run(100); len(giver) != 0 || len(taker) != 43 {
		t.Errorf("the control, a taker of 100: the giver holds %d, the taker %d", len(giver), len(taker))
	}
}

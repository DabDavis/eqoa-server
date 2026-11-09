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

// The cape slot (capes.go) against the game's own messages: what a client that
// draws capes is sent and may do, and that every other client (a PS2) is sent
// a cape as a plain item and never a word of slot 22. Item names are made up.

// capesOn turns world.capes on for the test.
func capesOn(t *testing.T) {
	t.Helper()
	entity.SetCapes(true)
	t.Cleanup(func() { entity.SetCapes(false) })
}

// capeSession is a session whose client took the cape offer.
func capeSession(s *session.Session) *session.Session {
	s.OfferCapesOnce()
	s.SetCapes(true)
	return s
}

// hasOp reports a message of op among those queued for s.
func hasOp(s *session.Session, op protocol.GameOpcode) bool {
	for _, o := range queuedOps(s) {
		if o == op {
			return true
		}
	}
	return false
}

// slotsNamed is the slot each UnequipItem (0x0040) among msgs names, and each
// EquipItem (0x003F) plus 100, so the two are told apart.
func slotsNamed(msgs [][]byte) []int {
	var out []int
	for _, d := range msgs {
		switch {
		case len(d) >= 3 && opcode(d) == protocol.OpcodeUnequipItem:
			out = append(out, int(d[2]))
		case len(d) >= 4 && opcode(d) == protocol.OpcodeEquipItem:
			out = append(out, 100+int(d[3]))
		}
	}
	return out
}

// Every item record the server writes goes out to a client that does not draw
// capes with a cape's wear slot -1 and a worn cape's equip slot -1, byte for byte
// the record of a plain item never worn, and with 28 and 22 to one that does: the
// MemoryDump's inventory and bank rows, 0x003B, the bank's 0x1251, a merchant's
// list, a corpse's, a catch, a crafted item and an auction lot. The control: any
// other record is the same bytes to both.
func TestCapeRecordsAsEachClientReadsThem(t *testing.T) {
	cape := database.CharInventoryEntry{ItemID: 77, StackLeft: 1, EquipLoc: equipCape, Pattern: database.ItemPatternRecord{
		PatternID: 715001, ItemName: "Azure Knight's Cape", EquipSlot: wearCape, Model: 1, ClassUse: 32767, RaceUse: 32767, LevelReq: 1, MaxStack: 1}}
	plainCape := cape
	plainCape.EquipLoc, plainCape.Pattern.EquipSlot = -1, -1
	helm := database.CharInventoryEntry{ItemID: 78, StackLeft: 1, EquipLoc: 0, Pattern: database.ItemPatternRecord{
		PatternID: 5000, ItemName: "Iron Helm", EquipSlot: 1, Model: 4, ClassUse: 32767, RaceUse: 32767, LevelReq: 1, MaxStack: 1}}
	lot := func(e database.CharInventoryEntry) *database.AuctionLot {
		return &database.AuctionLot{ID: 9, MinBid: 1, Buyout: 2, SellerName: "Seller", Item: e}
	}
	writers := map[string]func(e database.CharInventoryEntry, capes bool) []byte{
		"MemoryDump inventory row": func(e database.CharInventoryEntry, c bool) []byte {
			return build(protocol.OpcodeMemoryDump, 4096, func(w *buffer.Writer) { writeItem(w, &e, 3, -1, c) })
		},
		"MemoryDump bank row": func(e database.CharInventoryEntry, c bool) []byte {
			return build(protocol.OpcodeMemoryDump, 4096, func(w *buffer.Writer) { writeSingleItem(w, &e, 3, c) })
		},
		"0x003B": func(e database.CharInventoryEntry, c bool) []byte { return addInvItemPayload(&e, 3, c) },
		"0x1251": func(e database.CharInventoryEntry, c bool) []byte { return bankAddPayload(&e, 3, c) },
		"0x00B7": func(e database.CharInventoryEntry, c bool) []byte {
			return merchantBoxPayload(42, []database.CharInventoryEntry{e}, "Hail.", c, e)
		},
		"0x0018": func(e database.CharInventoryEntry, c bool) []byte {
			return buildLoot([]database.CharInventoryEntry{e}, c)
		},
		"a catch": func(e database.CharInventoryEntry, c bool) []byte {
			return catchPayload([]database.CharInventoryEntry{e}, c)
		},
		"a crafted item": func(e database.CharInventoryEntry, c bool) []byte {
			return craftExpMadePayload(craftExpMade, craftReport{}, 1, nil, &e, 3, c)
		},
		"an auction lot": func(e database.CharInventoryEntry, c bool) []byte {
			return build(protocol.OpcodeAuctionStuff, 4096, func(w *buffer.Writer) { writeLot(w, lot(e), false, lotView{capes: c}) })
		},
	}
	for name, write := range writers {
		if got, want := write(cape, false), write(plainCape, true); !bytes.Equal(got, want) {
			t.Errorf("%s: a cape to a client that does not draw capes is not a plain item never worn:\n%x\n%x", name, got, want)
		}
		if bytes.Equal(write(cape, true), write(cape, false)) {
			t.Errorf("%s: a client that draws capes is sent the cape as a plain item", name)
		}
		if !bytes.Equal(write(helm, true), write(helm, false)) {
			t.Errorf("%s: the control: a helm's record differs by the client's capes", name)
		}
	}
	// what the dump's row carries, read as CLIENT reads it: the equip slot is
	// the fourth zigzag, the wear slot the sixth of the record's after the key
	row := func(c bool) (equip, wear int64) {
		buf := build(protocol.OpcodeMemoryDump, 4096, func(w *buffer.Writer) { writeItem(w, &cape, 3, -1, c) })
		r := buffer.NewReader(buf[2:])
		var z [5]int64
		for i := range z {
			z[i], _ = r.Read7BitEncodedInt64()
		}
		r.ReadUint32()
		var p [6]int64
		for i := range p {
			p[i], _ = r.Read7BitEncodedInt64()
		}
		return z[3], p[5]
	}
	if e, w := row(true); e != equipCape || w != wearCape {
		t.Errorf("to a client that draws capes: equip %d wear %d, want 22 and 28", e, w)
	}
	if e, w := row(false); e != -1 || w != -1 {
		t.Errorf("to a PS2: equip %d wear %d, want -1 and -1", e, w)
	}
}

// capeListFixture is a character list's world: an account whose one character
// wears a cape of style 3 in red.
type capeListFixture struct {
	h    *CharacterHandler
	sm   *session.Manager
	sid  uint32
	caps *CapesHandler
}

func newCapeListFixture(t *testing.T) *capeListFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/capes.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Cloaked", AccountID: 5, Level: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1394465037, 0, 28, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Violet Mantle', '', 3, ?)`,
		int64(0xC01010FF))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	inv := database.NewInventoryStore(db)
	id, err := inv.AddCharItem(rec.ServerID, int(pid), 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.SetEquipLoc(rec.ServerID, id, equipCape); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, inventoryStore: inv, sessionManager: sm, gameWorld: w}
	return &capeListFixture{h: h, sm: sm, sid: rec.ServerID, caps: NewCapesHandler(h)}
}

func (f *capeListFixture) session(t *testing.T, n byte) *session.Session {
	s := f.sm.CreateSession(testAddr(n), uint32(n), uint32(n), uint16(n), uint16(n), true)
	s.SetAccountID(5)
	s.SetState(session.StateCharacterSelect)
	return s
}

func (f *capeListFixture) answer(t *testing.T, s *session.Session, v uint32) {
	t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCapes)
	msg.SetData(u32b(v))
	if err := f.caps.HandleCapes(s, msg); err != nil {
		t.Fatal(err)
	}
}

// The cape offer comes once, after a session's first character list and the
// live look's offer; a client that answers 1 is sent its characters' capes at
// once and after every list from then, and stops at 0. The list itself is the
// same bytes whoever reads it. The controls: a session that never answers (a
// PS2) is offered once and sent no list of capes, and a server without capes
// offers nothing and takes no answer.
func TestCapesOfferedAfterTheListAndAnswered(t *testing.T) {
	capesOn(t)
	f := newCapeListFixture(t)

	ps2 := f.session(t, 31)
	f.h.SendCharacterSelectEQOA(ps2)
	if got := queuedOps(ps2); len(got) != 3 || got[0] != protocol.OpcodeCharacterSelect || got[1] != protocol.OpcodeLiveLook || got[2] != protocol.OpcodeCapes {
		t.Fatalf("a first list: %v, want the list, the live look's offer, the capes'", got)
	}
	if d := sentOp(ps2, protocol.OpcodeCapes); binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	ps2List := sentOp(ps2, protocol.OpcodeCharacterSelect)
	f.h.SendCharacterSelectEQOA(ps2)
	if got := queuedOps(ps2); len(got) != 4 || got[3] != protocol.OpcodeCharacterSelect {
		t.Fatalf("a second list: %v, want no second offer and no capes", got)
	}

	van := f.session(t, 32)
	f.h.SendCharacterSelectEQOA(van)
	if !bytes.Equal(sentOp(van, protocol.OpcodeCharacterSelect), ps2List) {
		t.Fatal("the list differs by who reads it before any answer")
	}
	f.answer(t, van, 1)
	if !van.Capes() {
		t.Fatal("the answer 1 not taken")
	}
	want := append([]byte{1}, binary.LittleEndian.AppendUint32(nil, f.sid)...)
	want = append(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 0xC01010FF)
	if got := sentOp(van, protocol.OpcodeCapesList); !bytes.Equal(got[2:], want) {
		t.Fatalf("the capes after the answer: %x, want %x", got, want)
	}
	van.Queue().GetPending()
	f.h.SendCharacterSelectEQOA(van)
	if got := queuedOps(van); len(got) != 2 || got[0] != protocol.OpcodeCharacterSelect || got[1] != protocol.OpcodeCapesList {
		t.Fatalf("a list after the answer: %v, want the list then its capes", got)
	}
	if got := sentOp(van, protocol.OpcodeCapesList); !bytes.Equal(got[2:], want) {
		t.Fatalf("the list's capes: %x, want %x", got, want)
	}
	if !bytes.Equal(sentOp(van, protocol.OpcodeCharacterSelect), ps2List) {
		t.Fatal("the list carries something of the cape")
	}
	f.answer(t, van, 0)
	van.Queue().GetPending()
	f.h.SendCharacterSelectEQOA(van)
	if hasOp(van, protocol.OpcodeCapesList) || van.Capes() {
		t.Fatal("capes after the answer 0")
	}

	entity.SetCapes(false)
	off := f.session(t, 33)
	f.h.SendCharacterSelectEQOA(off)
	if hasOp(off, protocol.OpcodeCapes) {
		t.Fatal("a server without capes offered them")
	}
	f.answer(t, off, 1)
	if off.Capes() || hasOp(off, protocol.OpcodeCapesList) {
		t.Fatal("a server without capes took an answer it never asked for")
	}
}

// A client that draws capes wears one and takes it off through the game's own
// messages: its 0x003E is answered 0x003F into slot 22 (nothing displaced: the
// helm stays), the row worn there and the character's look carrying the cape's
// style and colour; again, 0x0040 for slot 22 and the look bare. The controls: a
// client that does not draw capes (a PS2), and one on a server without capes,
// are refused in a red line and told nothing of slot 22, the row unchanged.
func TestACapeIsWornThroughTheGamesMessages(t *testing.T) {
	capesOn(t)
	f := newGearFixture(t)
	cape := f.pattern("Violet Mantle", wearCape, 0, 0)
	if _, err := f.db.Exec(`UPDATE itempattern SET model = 3, color = ? WHERE patternid = ?`, int64(0x7020A0FF), cape); err != nil {
		t.Fatal(err)
	}
	helm := f.pattern("Iron Helm", 1, 0, 0)
	capeRow := f.own(cape, -1)
	helmRow := f.own(helm, 0)
	f.loadList()
	use := func(key uint32) [][]byte {
		return f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
			w.WriteUint32(key)
			w.WriteUint32(0xFFFFFFFF)
			w.WriteByte(0)
		})
	}

	capeSession(f.sess)
	if got := slotsNamed(use(0)); len(got) != 1 || got[0] != 100+equipCape {
		t.Fatalf("worn: %v, want one 0x003F into 22", got)
	}
	if w := f.worn(); w[equipCape] != capeRow || w[0] != helmRow {
		t.Fatalf("the rows worn: %v", w)
	}
	if f.player.CapeStyle != 3 || f.player.CapeColor != 0x7020A0FF {
		t.Fatalf("the look: style %d colour %08x", f.player.CapeStyle, f.player.CapeColor)
	}
	if got := slotsNamed(use(0)); len(got) != 1 || got[0] != equipCape {
		t.Fatalf("taken off: %v, want one 0x0040 for 22", got)
	}
	if w := f.worn(); w[equipCape] != 0 || f.player.CapeStyle != 0 {
		t.Fatalf("off: worn %v, style %d", w, f.player.CapeStyle)
	}

	f.sess.SetCapes(false) // a PS2
	msgs := use(0)
	if got := slotsNamed(msgs); len(got) != 0 || len(msgs) != 1 || opcode(msgs[0]) != protocol.OpcodeColoredChat {
		t.Fatalf("a PS2's ask: %v (%d messages), want only the refusal", got, len(msgs))
	}
	if f.worn()[equipCape] != 0 {
		t.Fatal("a PS2 put a cape on")
	}
	f.sess.SetCapes(true)
	entity.SetCapes(false)
	if got := slotsNamed(use(0)); len(got) != 0 || f.worn()[equipCape] != 0 {
		t.Fatalf("a server without capes: %v, worn %v", got, f.worn())
	}
}

// A worn cape a client that does not draw capes holds as a plain item comes off
// in the database alone when that client destroys it or the bank or a trade takes
// it (TakeOff): no 0x0040 names slot 22. The controls: the same TakeOff for a
// client that draws capes says so, and a PS2 is told of any other slot.
func TestAWornCapeComesOffAPS2Silently(t *testing.T) {
	capesOn(t)
	f := newGearFixture(t)
	cape := f.pattern("Violet Mantle", wearCape, 0, 0)
	helm := f.pattern("Iron Helm", 1, 0, 0)
	f.own(cape, equipCape)
	f.own(helm, 0)
	f.loadList()
	rows := func() []database.CharInventoryEntry {
		r, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	takeOff := func(pattern int) [][]byte {
		for _, r := range rows() {
			if r.Pattern.PatternID == pattern {
				if !f.h.TakeOff(f.sess, f.player, &r) {
					t.Fatal("TakeOff refused")
				}
			}
		}
		var out [][]byte
		for _, m := range f.sess.Queue().GetPending() {
			out = append(out, append([]byte(nil), m.Data()...))
		}
		return out
	}
	if got := slotsNamed(takeOff(cape)); len(got) != 0 || f.worn()[equipCape] != 0 {
		t.Fatalf("a PS2's cape taken off: %v, worn %v", got, f.worn())
	}
	if got := slotsNamed(takeOff(helm)); len(got) != 1 || got[0] != 0 {
		t.Fatalf("the control, a PS2's helm taken off: %v, want 0x0040 for 0", got)
	}

	// destroyed while worn
	if err := f.inv.SetEquipLoc(f.sid, rows()[0].ItemID, equipCape); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	msgs := f.send(protocol.OpcodeRemoveInvItem, func(w *buffer.Writer) {
		w.WriteUint32(0)
		w.WriteUint32(1)
	})
	if got := slotsNamed(msgs); len(got) != 0 {
		t.Fatalf("a PS2's worn cape destroyed: %v", got)
	}
	for _, r := range rows() {
		if r.Pattern.PatternID == cape {
			t.Fatal("the cape was not destroyed")
		}
	}

	cape2 := f.pattern("Nightborne Cloak", wearCape, 0, 0)
	f.own(cape2, equipCape)
	capeSession(f.sess)
	if got := slotsNamed(takeOff(cape2)); len(got) != 1 || got[0] != equipCape {
		t.Fatalf("the control, a client that draws capes: %v, want 0x0040 for 22", got)
	}
}

// A merchant buys a worn cape from a client that holds it as a plain item (a
// PS2), taking it off first without a word of slot 22, as it buys any item
// that client shows unworn. The control: from a client that draws capes, the
// worn cape is refused as any worn item is.
func TestAMerchantBuysAPS2sWornCape(t *testing.T) {
	capesOn(t)
	f := newRehabFixture(t)
	f.item(715001, "Violet Mantle", 1394465037, wearCape, 1)
	merchant := &entity.NPC{ServerID: 3100, Name: "Merchant Yulia", NPCID: 3100, NPCType: 0x81, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5}
	mid := f.tw.add(4, merchant)
	eh := NewEquipHandler(f.tw.mgr, f.inv, nil, nil)
	eh.SetGameWorld(f.tw.w)
	win := NewNPCWindowHandler(f.tw.mgr, f.inv)
	win.SetGameWorld(f.tw.w)
	win.takeOff = eh.TakeOff

	capeSession(f.tw.s)
	f.own(715001, 1, equipCape, 0)
	f.tw.s.Queue().GetPending()
	if win.sell(f.tw.s, mid, 0, 1) || f.held(715001) != 1 {
		t.Fatal("the control: a client that draws capes sold its worn cape")
	}

	f.tw.s.SetCapes(false) // a PS2
	f.tw.s.Queue().GetPending()
	if !win.sell(f.tw.s, mid, 0, 1) || f.held(715001) != 0 {
		t.Fatalf("a PS2's worn cape not bought (held %d)", f.held(715001))
	}
	var msgs [][]byte
	for _, m := range f.tw.s.Queue().GetPending() {
		msgs = append(msgs, m.Data())
	}
	if got := slotsNamed(msgs); len(got) != 0 {
		t.Fatalf("a PS2 told of slot 22: %v", got)
	}
}

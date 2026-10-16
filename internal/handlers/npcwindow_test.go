package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The merchant, bank and blacksmith windows against a temporary database:
// a character, two item patterns, a merchant's catalog, and NPCs the handler
// finds by id. Each window request is fed as the client sends it, and what
// the handler queues is read back off the session.

const (
	shopMerchantID = 0x00100BB9 // ps2_main_capture's merchant, as the client names it
	shopBankerID   = 0x00100BBA
	shopSmithID    = 0x00100BBB
	shopMerchant   = 7 // npcs.id of the merchant's catalog
)

type shopFixture struct {
	t       *testing.T
	db      *database.SQLiteDB
	inv     *database.InventoryStore
	h       *NPCWindowHandler
	sess    *session.Session
	player  *entity.Player
	npcs    map[uint32]*entity.NPC
	bait    int // a stackable pattern (Fishing Bait: value 38, stack 20, hp 100)
	sword   int // a single one (value 100, hp 100)
	charSID uint32
}

func newShopFixture(t *testing.T) *shopFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/shop.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &shopFixture{t: t, db: db, inv: database.NewInventoryStore(db)}

	rec := &database.CharacterRecord{CharName: "Shopper", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f.charSID = rec.ServerID
	f.bait = f.pattern("Fishing Bait", 38, 20, 100, 21)
	f.sword = f.pattern("Rusty Sword", 100, 1, 100, 12)

	f.player = &entity.Player{ServerID: 1001, CharServerID: f.charSID, Name: "Shopper", Tunar: 562}
	f.npcs = map[uint32]*entity.NPC{
		shopMerchantID: {Name: "Merchant", NPCID: shopMerchant, NPCType: 0x81},
		shopBankerID:   {Name: "Banker", NPCID: 8, NPCType: 0x82},
		shopSmithID:    {Name: "Smith", NPCID: 9, NPCType: 0x88},
	}
	f.sess = testSession()
	f.h = NewNPCWindowHandler(nil, f.inv)
	f.h.playerOf = func(*session.Session) *entity.Player { return f.player }
	f.h.find = func(_ *session.Session, id uint32) (entity.Entity, bool) {
		n, ok := f.npcs[id]
		return n, ok
	}
	f.loadLists()
	return f
}

// pattern adds an item pattern and returns its id.
func (f *shopFixture) pattern(name string, value, maxStack, maxHP, wear int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, ?, ?, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
		value, wear, maxStack, maxHP, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// catalog lists n of the pattern for sale, each one with condition hp.
func (f *shopFixture) catalog(pattern, n, hp int) {
	f.t.Helper()
	for i := 0; i < n; i++ {
		if _, err := f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
			VALUES (?, 1, ?, ?, ?)`, shopMerchant, hp, pattern, i); err != nil {
			f.t.Fatal(err)
		}
	}
}

// own gives the character an item row (location 0 inventory, 1 bank).
func (f *shopFixture) own(pattern, stack, hp, location int) int {
	f.t.Helper()
	id, err := f.inv.AddCharItem(f.charSID, pattern, stack, hp, 0, location)
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

// loadLists is what SendMemoryDump leaves: the two mirrors, key = position.
func (f *shopFixture) loadLists() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.charSID)
	if err != nil {
		f.t.Fatal(err)
	}
	var inv, bank []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 1 {
			bank = append(bank, r)
		} else {
			inv = append(inv, r)
		}
	}
	f.sess.SetInventoryList(newInvList(inv))
	f.sess.SetBankList(newInvList(bank))
}

// send feeds a request: its payload after the opcode, as Data() holds it.
func (f *shopFixture) send(op protocol.GameOpcode, fill func(w *buffer.Writer)) {
	f.t.Helper()
	payload := build(op, 256, fill)[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(payload)
	r := NewRegistry()
	f.h.RegisterHandlers(r)
	h, ok := r.Get(op)
	if !ok {
		f.t.Fatalf("no handler for 0x%04X", uint16(op))
	}
	if err := h(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
}

// sent drains what the handler queued: each message's bytes, opcode first.
func (f *shopFixture) sent() [][]byte {
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func (f *shopFixture) rows() []database.CharInventoryEntry {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.charSID)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *shopFixture) storedTunar() (int, int) {
	var a, b int
	if err := f.db.QueryRow(`SELECT tunar, bankTunar FROM characters WHERE serverid = ?`, f.charSID).Scan(&a, &b); err != nil {
		f.t.Fatal(err)
	}
	return a, b
}

func opOf(b []byte) protocol.GameOpcode {
	return protocol.GameOpcode(binary.LittleEndian.Uint16(b))
}

func ops(msgs [][]byte) string {
	s := ""
	for _, m := range msgs {
		s += fmt.Sprintf("%04X ", uint16(opOf(m)))
	}
	return s
}

func mustOps(t *testing.T, msgs [][]byte, want ...protocol.GameOpcode) {
	t.Helper()
	if len(msgs) != len(want) {
		t.Fatalf("sent %s, want %d messages %v", ops(msgs), len(want), want)
	}
	for i := range want {
		if opOf(msgs[i]) != want[i] {
			t.Fatalf("sent %s, want %v", ops(msgs), want)
		}
	}
}

// --- merchant ---

// 0x00B7 against Sandstorm's own (ps2_main_capture, 70.689 s): the merchant's
// id, both rates 100, ten records under counter 10, each record's instance
// fields as Sandstorm writes a catalog entry (1 of it, condition 20, no
// charges, not equipped, no quick slot, key = position), and the greeting.
func TestMerchantOpenIsSandstormsMerchantBox(t *testing.T) {
	f := newShopFixture(t)
	f.catalog(f.bait, 10, 20)
	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeMerchantBox)
	want := []byte{0xB7, 0x00, 0xB9, 0x0B, 0x10, 0x00, 0xC8, 0x01, 0xC8, 0x01, 0x14, 0x0A, 0x00, 0x00, 0x00,
		0x02, 0x28, 0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00}
	if got := msgs[0][:len(want)]; !bytes.Equal(got, want) {
		t.Errorf("0x00B7 starts % X\n                want % X", got, want)
	}
	// The second record is keyed 1, and the box ends with the greeting.
	tail := make([]byte, 4+2*len(merchantGreeting))
	binary.LittleEndian.PutUint32(tail, uint32(len(merchantGreeting)))
	for i, c := range merchantGreeting {
		binary.LittleEndian.PutUint16(tail[4+2*i:], uint16(c))
	}
	if !bytes.HasSuffix(msgs[0], tail) {
		t.Errorf("0x00B7 does not end with the greeting as ReadString16 reads it")
	}
}

// A merchant with no catalog (npcitem is empty on our server) still opens:
// count 0, counter 0.
func TestMerchantOpensEmpty(t *testing.T) {
	f := newShopFixture(t)
	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeMerchantBox)
	if want := []byte{0xB7, 0x00, 0xB9, 0x0B, 0x10, 0x00, 0xC8, 0x01, 0xC8, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}; !bytes.HasPrefix(msgs[0], want) {
		t.Errorf("empty 0x00B7 % X", msgs[0])
	}
}

func TestMerchantRefusesWhatIsNotAMerchant(t *testing.T) {
	f := newShopFixture(t)
	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopBankerID) })
	mustOps(t, f.sent(), protocol.OpcodeCancelPending)
	f.player.X = 500 // out of reach of the merchant at the origin
	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	mustOps(t, f.sent(), protocol.OpcodeCancelPending)
}

// The buy, as the capture has it: 0x004A {0, 1, merchant} with 562 tunar, a
// 38-tunar item → 0x003B (one, new: condition 100) then 0x0052 = 524
// (Sandstorm's 98 08). Bought again, the stack grows on the same record.
func TestMerchantBuy(t *testing.T) {
	f := newShopFixture(t)
	f.catalog(f.bait, 10, 20)
	buy := func() {
		f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(1)
			w.WriteUint32(shopMerchantID)
		})
	}
	buy()
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if want := []byte{0x3B, 0x00, 0x02, 0xC8, 0x01, 0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00}; !bytes.HasPrefix(msgs[0], want) {
		t.Errorf("0x003B % X\nwant prefix % X", msgs[0][:12], want)
	}
	if want := []byte{0x52, 0x00, 0x98, 0x08}; !bytes.Equal(msgs[1], want) {
		t.Errorf("0x0052 % X, want % X", msgs[1], want)
	}
	if got, _ := f.storedTunar(); got != 524 || f.player.Tunar != 524 {
		t.Errorf("tunar %d stored %d, want 524", f.player.Tunar, got)
	}
	buy()
	mustOps(t, f.sent(), protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	rows := f.rows()
	if len(rows) != 1 || rows[0].StackLeft != 2 || rows[0].RemainHP != 100 || rows[0].Location != 0 {
		t.Fatalf("rows %+v, want one stack of 2", rows)
	}
	if l := sessionInvList(f.sess); l.index(0) != 0 || l.nextKey() != 1 {
		t.Errorf("mirror: key 0 at %d, counter %d", l.index(0), l.nextKey())
	}
}

func TestMerchantBuyRefusals(t *testing.T) {
	f := newShopFixture(t)
	f.catalog(f.sword, 1, 0)
	f.player.Tunar = 99 // the sword is 100
	f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(0)
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(shopMerchantID)
	})
	mustOps(t, f.sent(), protocol.OpcodeCancelPending)
	f.player.Tunar = 1000
	f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(5) // no such catalog row
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(shopMerchantID)
	})
	mustOps(t, f.sent(), protocol.OpcodeCancelPending)
	if len(f.rows()) != 0 || f.player.Tunar != 1000 {
		t.Errorf("a refused buy changed something")
	}
}

// The sale, as the capture has it: 0x004B {key, 1, merchant} for a 38-tunar
// item at 524 → 0x003C {its index, sound 1, 1} then 0x0052 = 562 (E4 08).
func TestMerchantSell(t *testing.T) {
	f := newShopFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.own(f.bait, 1, 100, 0)
	f.loadLists()
	f.player.Tunar = 524
	f.send(protocol.OpcodeMerchantSell, func(w *buffer.Writer) {
		w.WriteUint32(1) // the bait, at index 1
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(shopMerchantID)
	})
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	if want := []byte{0x3C, 0x00, 0x01, 0x01, 0x02}; !bytes.Equal(msgs[0], want) {
		t.Errorf("0x003C % X, want % X", msgs[0], want)
	}
	if want := []byte{0x52, 0x00, 0xE4, 0x08}; !bytes.Equal(msgs[1], want) {
		t.Errorf("0x0052 % X, want % X", msgs[1], want)
	}
	if rows := f.rows(); len(rows) != 1 || rows[0].Pattern.PatternID != f.sword {
		t.Errorf("rows after the sale %+v", rows)
	}
	// The mirror packed: the sword keeps key 0, the bait's key is gone.
	if l := sessionInvList(f.sess); l.index(1) != -1 || l.index(0) != 0 {
		t.Errorf("mirror after the sale: key 1 at %d", l.index(1))
	}
}

// Worn half to pieces, an item sells for what GetInvItemSellPrice shows: the
// value less (1 - condition)/2 of it.
func TestSellPriceIsTheClients(t *testing.T) {
	for _, c := range []struct{ value, cur, max, want int }{
		{38, 100, 100, 38}, // new
		{100, 50, 100, 75}, // half worn: wear 0.25
		{100, 0, 100, 50},  // broken: wear 0.5
		{100, 7, 0, 100},   // no condition
		{99, 30, 100, 65},  // 99 - (int)(99 * 0.35) = 99 - 34
	} {
		if got := sellPrice(c.value, c.cur, c.max); got != c.want {
			t.Errorf("sellPrice(%d, %d/%d) = %d, want %d", c.value, c.cur, c.max, got, c.want)
		}
	}
	if merchantPrice(38) != 38 || roundPercent(149, 33) != 49 || roundPercent(150, 33) != 50 {
		t.Error("round half up at the rate")
	}
}

func TestMerchantSellRefusesWornAndMissing(t *testing.T) {
	f := newShopFixture(t)
	id := f.own(f.sword, 1, 100, 0)
	if err := f.inv.SetEquipLoc(f.charSID, id, 14); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	for _, key := range []uint32{0, 9} { // worn; no such key
		f.send(protocol.OpcodeMerchantSell, func(w *buffer.Writer) {
			w.WriteUint32(key)
			w.Write7BitEncodedInt64(1)
			w.WriteUint32(shopMerchantID)
		})
		mustOps(t, f.sent(), protocol.OpcodeCancelPending)
	}
	if len(f.rows()) != 1 {
		t.Error("a refused sale removed the item")
	}
}

// --- bank ---

func TestBankOpens(t *testing.T) {
	f := newShopFixture(t)
	f.send(protocol.OpcodeBankUI, func(w *buffer.Writer) { w.WriteUint32(shopBankerID) })
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeBankUI)
	if len(msgs[0]) != 2 {
		t.Errorf("0x124D carries % X; the dispatcher reads nothing", msgs[0])
	}
	f.send(protocol.OpcodeBankUI, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	mustOps(t, f.sent(), protocol.OpcodeCancelPending)
}

// A whole record into the bank: 0x003C off the inventory (its index, sound
// 0, the count), 0x1251 the record onto the bank; the row changes location
// and keeps its condition. Out again: 0x1250 then 0x003B.
func TestBankItemRoundTrip(t *testing.T) {
	f := newShopFixture(t)
	f.own(f.bait, 3, 100, 0)
	f.own(f.sword, 1, 40, 0)
	f.own(f.bait, 5, 100, 1) // the bank already holds bait
	f.loadLists()

	bankItem := func(fromBank byte, key uint32, n int64) [][]byte {
		f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) {
			w.WriteUint32(shopBankerID)
			w.WriteByte(fromBank)
			w.WriteUint32(key)
			w.Write7BitEncodedInt64(n)
		})
		return f.sent()
	}

	// The sword (inventory key 1) into the bank: a new bank record, key 1.
	msgs := bankItem(0, 1, 1)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	if want := []byte{0x3C, 0x00, 0x01, 0x00, 0x02}; !bytes.Equal(msgs[0], want) {
		t.Errorf("0x003C % X, want % X", msgs[0], want)
	}
	// The record: 1 of it, condition 40 kept, no charges, -1, -1, key 1.
	if want := []byte{0x51, 0x12, 0x02, 0x50, 0x00, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00}; !bytes.HasPrefix(msgs[1], want) {
		t.Errorf("0x1251 % X\nwant prefix % X", msgs[1][:11], want)
	}

	// Two of the inventory's three bait (key 0) onto the bank's stack.
	msgs = bankItem(0, 0, 2)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	rows := f.rows()
	stacks := map[int]int{}
	for _, r := range rows {
		stacks[r.Location*1000+r.Pattern.PatternID] = r.StackLeft
	}
	if stacks[0*1000+f.bait] != 1 || stacks[1*1000+f.bait] != 7 || stacks[1*1000+f.sword] != 1 {
		t.Errorf("after the deposits: %v", stacks)
	}

	// The sword back out (bank key 1, now at bank index 1): 0x1250 {1, 1}, 0x003B.
	msgs = bankItem(1, 1, 1)
	mustOps(t, msgs, protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	if want := []byte{0x50, 0x12, 0x01, 0x02}; !bytes.Equal(msgs[0], want) {
		t.Errorf("0x1250 % X, want % X", msgs[0], want)
	}
	if l := sessionInvList(f.sess); l.index(2) != 1 { // the inventory's counter was 2
		t.Errorf("the sword came back under key 2 at index 1: got index %d", l.index(2))
	}
	for _, r := range f.rows() {
		if r.Pattern.PatternID == f.sword && (r.Location != 0 || r.RemainHP != 40) {
			t.Errorf("the sword is back at location %d with condition %d", r.Location, r.RemainHP)
		}
	}
}

func TestBankItemRefusals(t *testing.T) {
	f := newShopFixture(t)
	id := f.own(f.sword, 1, 100, 0)
	if err := f.inv.SetEquipLoc(f.charSID, id, 14); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	for _, c := range []struct {
		from byte
		key  uint32
		n    int64
	}{{0, 0, 1}, {0, 7, 1}, {1, 0, 1}, {0, 0, 2}} { // worn; no key; empty bank; too many
		f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) {
			w.WriteUint32(shopBankerID)
			w.WriteByte(c.from)
			w.WriteUint32(c.key)
			w.Write7BitEncodedInt64(c.n)
		})
		mustOps(t, f.sent(), protocol.OpcodeCancelPending)
	}
}

func TestBankTunar(t *testing.T) {
	f := newShopFixture(t)
	f.player.Tunar, f.player.BankTunar = 562, 0
	move := func(dir byte, n int64) [][]byte {
		f.send(protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
			w.WriteUint32(shopBankerID)
			w.WriteByte(dir)
			w.Write7BitEncodedInt64(n)
		})
		return f.sent()
	}
	msgs := move(0, 500)
	mustOps(t, msgs, protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	if want := []byte{0x53, 0x12, 0xE8, 0x07}; !bytes.Equal(msgs[1], want) { // zz 500
		t.Errorf("0x1253 % X, want % X", msgs[1], want)
	}
	if c, b := f.storedTunar(); c != 62 || b != 500 {
		t.Errorf("stored %d / %d, want 62 / 500", c, b)
	}
	mustOps(t, move(1, 501), protocol.OpcodeCancelPending) // more than is banked
	mustOps(t, move(0, 63), protocol.OpcodeCancelPending)  // more than is carried
	mustOps(t, move(1, 500), protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	if f.player.Tunar != 562 || f.player.BankTunar != 0 {
		t.Errorf("after the round trip %d / %d", f.player.Tunar, f.player.BankTunar)
	}
}

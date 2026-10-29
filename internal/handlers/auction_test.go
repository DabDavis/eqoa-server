package handlers

import (
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The auction house over the auctionWire (auction.go) against a temporary database:
// characters on their own accounts with sessions, a banker, the house's clock
// and its view of who is in the world stood in for. Each request is fed as
// CLIENT's VIBankControl sends it, and every reply is read back as CLIENT's
// Recv1206 / Recv / the chat read it.

const houseBankerID = 0x00100BBA

type houseChar struct {
	name   string
	sid    uint32
	sess   *session.Session
	player *entity.Player
}

type houseFixture struct {
	t        *testing.T
	db       *database.SQLiteDB
	inv      *database.InventoryStore
	win      *NPCWindowHandler
	house    *AuctionHouse
	reg      *Registry
	now      time.Time
	chars    map[string]*houseChar
	bySess   map[*session.Session]*houseChar
	byID     map[uint32]*houseChar
	online   map[uint32]bool
	replaced map[*entity.Player]bool
	sword    int // a single item, value 50
	gem      int // stacks to 20, value 10
}

func newHouseFixture(t *testing.T, names ...string) *houseFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/house.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &houseFixture{t: t, db: db, inv: database.NewInventoryStore(db),
		now:   time.Date(2026, 10, 29, 12, 0, 30, 0, time.UTC),
		chars: map[string]*houseChar{}, bySess: map[*session.Session]*houseChar{}, byID: map[uint32]*houseChar{},
		online: map[uint32]bool{}, replaced: map[*entity.Player]bool{}}
	store := database.NewSQLiteCharacterStore(db)
	for i, name := range names {
		rec := &database.CharacterRecord{CharName: name, AccountID: i + 1, BankTunar: 1000}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		c := &houseChar{name: name, sid: rec.ServerID, sess: testSession(),
			player: &entity.Player{ServerID: entity.EntityID(1000 + i), CharServerID: rec.ServerID, Name: name, BankTunar: 1000}}
		c.sess.SetCharacterInWorld(true)
		f.chars[name], f.bySess[c.sess], f.byID[c.sid] = c, c, c
		f.online[c.sid] = true
		f.load(c) // the lists its MemoryDump left
	}
	f.sword = f.pattern("Fine Sword", 50, 1)
	f.gem = f.pattern("Rough Gem", 10, 20)

	f.win = NewNPCWindowHandler(nil, f.inv)
	f.win.playerOf = func(s *session.Session) *entity.Player {
		if c := f.bySess[s]; c != nil {
			return c.player
		}
		return nil
	}
	f.win.find = func(_ *session.Session, id uint32) (entity.Entity, bool) {
		if id == houseBankerID {
			return &entity.NPC{Name: "Banker", NPCID: 8, NPCType: 0x82}, true
		}
		return nil, false
	}
	f.house = NewAuctionHouse(db, f.win)
	f.house.now = func() time.Time { return f.now }
	f.house.live = func(id uint32) (*session.Session, *entity.Player) {
		if c := f.byID[id]; c != nil && f.online[id] {
			return c.sess, c.player
		}
		return nil, nil
	}
	f.house.isLive = func(p *entity.Player) bool { return p != nil && !f.replaced[p] }
	f.reg = NewRegistry()
	f.win.RegisterHandlers(f.reg)
	return f
}

func (f *houseFixture) pattern(name string, value, maxStack int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, 0, 12, 0, 0, 1, 0, 1, 0, 0, 10, ?, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, 'Well made.', 0, 0)`,
		value, maxStack, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// give puts an item in a character's bank and loads its lists as a
// MemoryDump leaves them.
func (f *houseFixture) give(c *houseChar, pattern, stack int) int {
	f.t.Helper()
	id, err := f.inv.AddCharItem(c.sid, pattern, stack, 90, 0, 1)
	if err != nil {
		f.t.Fatal(err)
	}
	f.load(c)
	return id
}

func (f *houseFixture) load(c *houseChar) {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(c.sid)
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
	c.sess.SetInventoryList(newInvList(inv))
	c.sess.SetBankList(newInvList(bank))
}

func (f *houseFixture) bankRows(c *houseChar) []database.CharInventoryEntry {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(c.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	var out []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 1 {
			out = append(out, r)
		}
	}
	return out
}

func (f *houseFixture) dbBank(c *houseChar) int {
	f.t.Helper()
	n, _, err := f.db.BankTunar(c.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

// request feeds 0x1205 with the sub-op and fill's fields.
func (f *houseFixture) request(c *houseChar, sub byte, fill func(w *buffer.Writer)) {
	f.t.Helper()
	payload := build(protocol.OpcodeAuctionRequest, 1024, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeAuctionRequest)
	msg.SetData(payload)
	h, ok := f.reg.Get(protocol.OpcodeAuctionRequest)
	if !ok {
		f.t.Fatal("0x1205 is not registered")
	}
	if err := h(c.sess, msg); err != nil {
		f.t.Fatal(err)
	}
}

// sell is VIBankControl::Sell: the banker, the bank record's key, 1, the
// three numbers.
func (f *houseFixture) sell(c *houseChar, key uint32, price, buyout, minutes int) {
	f.request(c, 5, func(w *buffer.Writer) {
		w.WriteUint32(houseBankerID)
		w.WriteUint32(key)
		for _, v := range []int{1, price, buyout, minutes} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
}

// bid is Send06: the lot, the amount, 1.
func (f *houseFixture) bid(c *houseChar, lot, amount int) {
	f.request(c, 6, func(w *buffer.Writer) {
		for _, v := range []int{lot, amount, 1} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
}

type sentMsg struct {
	op   protocol.GameOpcode
	data []byte // after the opcode
}

// drain is what the handler queued, each message whole: a message over
// MaxFragmentSize went as 0xFA fragments and a last 0xFB (queue.go), put back
// together here as the client's DRDP does.
func (f *houseFixture) drain(c *houseChar) []sentMsg {
	var out []sentMsg
	var whole []byte
	for _, m := range c.sess.Queue().GetPending() {
		whole = append(whole, m.Data()...)
		if m.Type == protocol.MessageTypeSegmentReliable {
			continue
		}
		out = append(out, sentMsg{protocol.GameOpcode(binary.LittleEndian.Uint16(whole)), whole[2:]})
		whole = nil
	}
	return out
}

// --- CLIENT's reads ---

type auctionWire struct {
	b   []byte
	p   int
	bad bool
}

func (r *auctionWire) take(n int) []byte {
	if r.bad || n < 0 || len(r.b)-r.p < n {
		r.bad = true
		return nil
	}
	v := r.b[r.p : r.p+n]
	r.p += n
	return v
}
func (r *auctionWire) u8() byte {
	if v := r.take(1); v != nil {
		return v[0]
	}
	return 0
}
func (r *auctionWire) u32() uint32 {
	if v := r.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}
func (r *auctionWire) zz() int64 {
	var u uint64
	for shift := uint(0); shift < 64; shift += 7 {
		b := r.u8()
		if r.bad {
			return 0
		}
		u |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
	}
	return int64(u>>1) ^ -int64(u&1)
}
func (r *auctionWire) str8() string { return string(r.take(int(r.u32()))) }
func (r *auctionWire) wide() string {
	n := int(r.u32())
	b := r.take(2 * n)
	u := make([]uint16, 0, n)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, binary.LittleEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(u))
}

type wireItem struct {
	pattern, value, maxStack int64
	name, desc               string
	stats                    int64
}

// item is VIItem's reader (0x00CBACE8): 23 zz words, the name, the text, the
// stats.
func (r *auctionWire) item() wireItem {
	var w [23]int64
	for i := range w {
		w[i] = r.zz()
	}
	it := wireItem{pattern: w[0], value: w[1], maxStack: w[14]}
	it.name, it.desc = r.wide(), r.wide()
	it.stats = r.zz()
	for i := int64(0); i < it.stats && !r.bad; i++ {
		r.zz()
		r.zz()
	}
	return it
}

type wireLot struct {
	id, qty, hp, charges, min, buyout, end int64
	seller                                 string
	item                                   wireItem
}

// lot is operator>>(VIMemStream &, VIAuctionItem &) (0x00CBB150).
func (r *auctionWire) lot() wireLot {
	l := wireLot{id: r.zz(), qty: r.zz(), hp: r.zz(), charges: r.zz(), min: r.zz(), buyout: r.zz(), end: r.zz()}
	l.seller = r.str8()
	l.item = r.item()
	return l
}

type wirePage struct {
	page, minutes int64
	lots          []wireLot
	cols          [3][]int64
}

// page is RecvResults (129).
func readPage(t *testing.T, d []byte) wirePage {
	t.Helper()
	r := &auctionWire{b: d}
	if sub := r.u8(); sub != 129 {
		t.Fatalf("0x1206 sub-op %d, want 129", sub)
	}
	p := wirePage{page: r.zz(), minutes: r.zz()}
	for n := r.zz(); n > 0 && !r.bad; n-- {
		p.lots = append(p.lots, r.lot())
	}
	for c := range p.cols {
		for n := r.zz(); n > 0 && !r.bad; n-- {
			p.cols[c] = append(p.cols[c], r.zz())
		}
	}
	if r.bad || r.p != len(d) {
		t.Fatalf("page read %d of %d bytes (bad %v)", r.p, len(d), r.bad)
	}
	return p
}

type wireBid struct{ char, lot, max, x0c, escrow int64 }

func readBid(r *auctionWire) wireBid {
	b := wireBid{char: r.zz(), lot: r.zz(), max: r.zz(), x0c: r.zz(), escrow: r.zz()}
	r.u8()
	return b
}

// find is the first message of that opcode (and 0x1206 sub-op, when sub is
// not 0) among them.
func findSent(msgs []sentMsg, op protocol.GameOpcode, sub byte) ([]byte, bool) {
	for _, m := range msgs {
		if m.op == op && (sub == 0 || (len(m.data) > 0 && m.data[0] == sub)) {
			return m.data, true
		}
	}
	return nil, false
}

func sentLines(msgs []sentMsg) []string {
	var out []string
	for _, m := range msgs {
		if m.op == protocol.OpcodeColoredChat || m.op == protocol.OpcodeErrorMessage {
			r := &auctionWire{b: m.data}
			out = append(out, r.wide())
		}
	}
	return out
}

func bankTunarOf(t *testing.T, msgs []sentMsg) (int64, bool) {
	t.Helper()
	d, ok := findSent(msgs, protocol.OpcodeConfirmBankTunar, 0)
	if !ok {
		return 0, false
	}
	r := &auctionWire{b: d}
	return r.zz(), true
}

// clears reports a reply CLIENT's bank wait ends on (VIBankControl::m_state
// back to 0): 0x1206 128, 129, 130, 132, 135, 136, 0x1250, 0x1251, 0x1253,
// 0x00D7.
func sentClears(msgs []sentMsg) bool {
	for _, m := range msgs {
		switch m.op {
		case protocol.OpcodeRemoveBankItem, protocol.OpcodeAddBankItem, protocol.OpcodeConfirmBankTunar, protocol.OpcodeCancelPending:
			return true
		case protocol.OpcodeAuctionStuff:
			switch m.data[0] {
			case 128, 129, 130, 132, 135, 136:
				return true
			}
		}
	}
	return false
}

// --- the tests ---

// A sale over the wire: the bank record leaves the list (0x1250), the fee
// the bank (0x1253), the lot joins m_lots (130), and the row is the house's;
// the same key again is refused in red and nothing moves.
func TestAuctionSellOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller")
	s := f.chars["Seller"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 250, 900, 720)
	msgs := f.drain(s)
	took, ok := findSent(msgs, protocol.OpcodeRemoveBankItem, 0)
	if !ok || took[0] != 0 {
		t.Fatalf("0x1250 %v (%v)", took, ok)
	}
	if n, _ := bankTunarOf(t, msgs); n != 998 || s.player.BankTunar != 998 || f.dbBank(s) != 998 {
		t.Errorf("bank after the fee: sent %d, game %d, kept %d; want 998", n, s.player.BankTunar, f.dbBank(s))
	}
	add, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 130)
	if !ok {
		t.Fatal("no 130")
	}
	r := &auctionWire{b: add[1:]}
	l := r.lot()
	if r.bad || r.p != len(add)-1 || l.min != 250 || l.buyout != 900 || l.seller != "Seller" || l.item.name != "Fine Sword" || l.qty != 1 {
		t.Errorf("the lot as CLIENT reads it: %+v (read %d of %d)", l, r.p, len(add)-1)
	}
	if l.end != (f.now.Unix()+720*60+59)/60 {
		t.Errorf("ends at minute %d", l.end)
	}
	if len(f.bankRows(s)) != 0 || sessionBankList(s.sess).index(0) != -1 {
		t.Error("the item is still in the bank")
	}
	if !sentClears(msgs) {
		t.Error("the sale's replies leave the bank waiting")
	}
	// planted control: the same key again
	f.sell(s, 0, 250, 900, 720)
	msgs = f.drain(s)
	if _, ok := findSent(msgs, protocol.OpcodeCancelPending, 0); !ok {
		t.Error("a refused sale was not ended with 0x00D7")
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "not in your bank") {
		t.Errorf("refusal lines %q", ls)
	}
	if f.dbBank(s) != 998 {
		t.Errorf("a refused sale took a fee: %d", f.dbBank(s))
	}
	// a sale needs a banker
	f.give(s, f.sword, 1)
	f.request(s, 5, func(w *buffer.Writer) {
		w.WriteUint32(0x12345)
		w.WriteUint32(1)
		for _, v := range []int{1, 250, 900, 720} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
	if msgs := f.drain(s); len(f.bankRows(s)) != 1 || !sentClears(msgs) {
		t.Error("a sale without a banker took the item, or left the bank waiting")
	}
}

// Bids over the wire: the first leads (132, the escrow out of the bank), a
// higher one takes the lead (the old leader is told: 0x1253, 134 with
// nothing held, 137, a line), a raise (134 and 0x1253), a bid outbid at once
// by a bidder with a record (134 alone: 0x00D7 ends the wait); the seller sees
// the lot change (137). Every reply ends the bidder's wait.
func TestAuctionBidsOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder", "Rival")
	s, a, b := f.chars["Seller"], f.chars["Bidder"], f.chars["Rival"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 1000, 720)
	f.drain(s)

	f.bid(a, 1, 300)
	msgs := f.drain(a)
	d, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 132)
	if !ok {
		t.Fatalf("no 132: %v", msgs)
	}
	rb := readBid(&auctionWire{b: d[1:]})
	if rb.char != int64(a.sid) || rb.lot != 1 || rb.max != 300 || rb.escrow != 300 {
		t.Errorf("the bid record %+v", rb)
	}
	if n, _ := bankTunarOf(t, msgs); n != 700 || a.player.BankTunar != 700 || f.dbBank(a) != 700 {
		t.Errorf("the bidder's bank: sent %d, game %d, kept %d", n, a.player.BankTunar, f.dbBank(a))
	}
	up, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 137)
	if !ok {
		t.Fatal("no 137")
	}
	r := &auctionWire{b: up[1:]}
	l := r.lot()
	minutes, high, cur, next := r.zz(), r.zz(), r.zz(), r.zz()
	if r.bad || l.id != 1 || high != int64(a.sid) || cur != 100 || next != 101 || minutes <= 0 {
		t.Errorf("137: lot %d, minutes %d, leader %d, bid %d, min %d", l.id, minutes, high, cur, next)
	}
	if !sentClears(msgs) {
		t.Error("the first bid's replies leave the wait")
	}
	if _, ok := findSent(f.drain(s), protocol.OpcodeAuctionStuff, 137); !ok {
		t.Error("the seller was not shown the lot's change")
	}

	f.bid(b, 1, 400)
	msgs = f.drain(b)
	if n, _ := bankTunarOf(t, msgs); n != 600 || !sentClears(msgs) {
		t.Errorf("the new leader's bank %d", n)
	}
	am := f.drain(a)
	if n, ok := bankTunarOf(t, am); !ok || n != 1000 || a.player.BankTunar != 1000 {
		t.Errorf("the old leader's refund: sent %d (%v), game %d", n, ok, a.player.BankTunar)
	}
	if d, ok := findSent(am, protocol.OpcodeAuctionStuff, 134); !ok {
		t.Error("the old leader's record was not updated")
	} else if rb := readBid(&auctionWire{b: d[1:]}); rb.escrow != 0 || rb.max != 300 {
		t.Errorf("the old leader's record %+v", rb)
	}
	if ls := sentLines(am); len(ls) != 1 || !strings.Contains(ls[0], "outbid on Fine Sword") {
		t.Errorf("the old leader's lines %q", ls)
	}
	if n, _ := f.db.AuctionNotices(a.sid); len(n) != 0 {
		t.Errorf("lines told stay waiting: %v", n)
	}

	f.bid(b, 1, 500) // a raise
	msgs = f.drain(b)
	if _, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 134); !ok || !sentClears(msgs) {
		t.Errorf("a raise: %v", msgs)
	}
	if n, _ := bankTunarOf(t, msgs); n != 500 {
		t.Errorf("the raise's bank %d", n)
	}

	f.bid(a, 1, 450) // a record already, outbid at once: nothing moves
	msgs = f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 134); !ok {
		t.Error("the outbid record was not updated")
	}
	if _, ok := bankTunarOf(t, msgs); ok {
		t.Error("a bid outbid at once moved tunar")
	}
	if _, ok := findSent(msgs, protocol.OpcodeCancelPending, 0); !ok {
		t.Error("134 alone leaves the wait: no 0x00D7")
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "stands at 451") {
		t.Errorf("lines %q", ls)
	}
	// the seller cannot bid on its own lot
	f.bid(s, 1, 600)
	msgs = f.drain(s)
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "your own auctions") || !sentClears(msgs) {
		t.Errorf("the seller's own bid: %q", ls)
	}
}

// A buyout over the wire: the item into the buyer's bank list (0x1251) and
// its copy, the seller paid and told, the old leader refunded; what the
// buyer's client and the server's copy hold agree.
func TestAuctionBuyoutOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder", "Buyer")
	s, a, c := f.chars["Seller"], f.chars["Bidder"], f.chars["Buyer"]
	f.give(s, f.sword, 1)
	f.give(c, f.gem, 3)
	f.sell(s, 0, 100, 900, 720)
	f.drain(s)
	f.bid(a, 1, 300)
	f.drain(a)
	f.drain(s)

	f.bid(c, 1, 900)
	msgs := f.drain(c)
	add, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0)
	if !ok {
		t.Fatalf("no 0x1251 to the buyer: %v", msgs)
	}
	r := &auctionWire{b: add}
	stack, hp := r.zz(), r.zz()
	r.zz()
	r.zz()
	r.zz()
	key := r.u32()
	it := r.item()
	if r.bad || stack != 1 || hp != 90 || it.name != "Fine Sword" || key != 1 {
		t.Errorf("0x1251: stack %d hp %d key %d item %+v", stack, hp, key, it)
	}
	bank := sessionBankList(c.sess)
	if i := bank.index(key); i != 1 {
		t.Errorf("the buyer's list copy: key %d at %d", key, i)
	}
	if rows := f.bankRows(c); len(rows) != 2 || rows[1].Pattern.PatternID != f.sword {
		t.Errorf("the buyer's bank rows %+v", rows)
	}
	if n, _ := bankTunarOf(t, msgs); n != 100 || c.player.BankTunar != 100 {
		t.Errorf("the buyer's bank: %d / %d", n, c.player.BankTunar)
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "won the auction of Fine Sword for 900") {
		t.Errorf("the buyer's lines %q", ls)
	}
	sm := f.drain(s)
	if n, _ := bankTunarOf(t, sm); n != 999+900 || s.player.BankTunar != 1899 {
		t.Errorf("the seller's bank: %d / %d", n, s.player.BankTunar)
	}
	if ls := sentLines(sm); len(ls) != 1 || !strings.Contains(ls[0], "sold to Buyer for 900") {
		t.Errorf("the seller's lines %q", ls)
	}
	if n, _ := bankTunarOf(t, f.drain(a)); n != 1000 {
		t.Errorf("the old leader's refund %d", n)
	}
	if problems, _ := f.db.AuctionAudit(); len(problems) != 0 {
		t.Errorf("audit: %v", problems)
	}
}

// The sweep closes a lot at its time and tells who is in the world; one who
// is not finds its bank right and hears its lines as it comes in.
func TestAuctionSweepTellsOnlineAndOffline(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 900, 720)
	f.drain(s)
	f.bid(a, 1, 300)
	f.drain(a)
	f.drain(s)
	f.online[s.sid] = false // the seller logs out
	f.now = f.now.Add(13 * time.Hour)
	f.house.Sweep()

	msgs := f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0); !ok {
		t.Errorf("the winner's item was not filed: %v", msgs)
	}
	if n, _ := bankTunarOf(t, msgs); n != 900 || a.player.BankTunar != 900 {
		t.Errorf("the winner's refund: %d / %d (want 900: 1000 less the price 100)", n, a.player.BankTunar)
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "won the auction") {
		t.Errorf("the winner's lines %q", ls)
	}
	if got := f.drain(s); len(got) != 0 {
		t.Errorf("the seller, not in the world, was sent %d messages", len(got))
	}
	if f.dbBank(s) != 999+100 {
		t.Errorf("the seller's bank, kept: %d", f.dbBank(s))
	}
	// the seller comes back: its Player read from a row before the payment
	s.player = &entity.Player{ServerID: s.player.ServerID, CharServerID: s.sid, Name: "Seller", BankTunar: 999}
	f.online[s.sid] = true
	f.house.BeforeDump(s.sess, s.player)
	if s.player.BankTunar != 1099 {
		t.Errorf("the bank at login %d, want 1099", s.player.BankTunar)
	}
	f.house.playerEntered(s.sess, s.player)
	if ls := sentLines(f.drain(s)); len(ls) != 1 || !strings.Contains(ls[0], "sold to Bidder for 100") {
		t.Errorf("the seller's lines at login %q", ls)
	}
	if n, _ := f.db.AuctionNotices(s.sid); len(n) != 0 {
		t.Errorf("lines told at login stay: %v", n)
	}
	// planted control: the sweep again moves nothing
	f.house.Sweep()
	if f.dbBank(s) != 1099 || f.dbBank(a) != 900 || len(f.bankRows(a)) != 1 {
		t.Errorf("a second sweep moved something: %d %d %d", f.dbBank(s), f.dbBank(a), len(f.bankRows(a)))
	}
}

// A full bank keeps an item waiting; opening the bank with room files it.
func TestAuctionItemWaitsForTheBank(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 900, 720)
	for i := 0; i < database.AuctionBankSlots; i++ {
		if _, err := f.inv.AddCharItem(a.sid, f.gem+0*i, 1, 90, 0, 1); err != nil {
			t.Fatal(err)
		}
	}
	// forty records of one stackable item would stack: give each its own
	for i := 0; i < database.AuctionBankSlots; i++ {
		f.db.Exec(`UPDATE charinventory SET patternid = ? WHERE serverid = ? AND listnumber = ?`, f.pattern("Trinket", 1, 1), a.sid, i)
	}
	f.load(a)
	f.drain(s)
	f.bid(a, 1, 900)
	msgs := f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0); ok {
		t.Fatal("an item filed into a full bank")
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "waits at the auction house") {
		t.Errorf("lines %q", ls)
	}
	// room is made, the bank opened
	rows := f.bankRows(a)
	if err := f.inv.DeleteCharItem(a.sid, rows[0].ItemID); err != nil {
		t.Fatal(err)
	}
	f.load(a)
	payload := build(protocol.OpcodeBankUI, 16, func(w *buffer.Writer) { w.WriteUint32(houseBankerID) })[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeBankUI)
	msg.SetData(payload)
	if err := f.win.HandleBankOpen(a.sess, msg); err != nil {
		t.Fatal(err)
	}
	msgs = f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0); !ok {
		t.Errorf("not filed as the bank opened: %v", msgs)
	}
	if n, _ := f.db.AuctionWaiting(a.sid); n != 0 {
		t.Errorf("%d still waiting", n)
	}
	if got := sessionBankList(a.sess); len(got.recs) != database.AuctionBankSlots {
		t.Errorf("the list copy holds %d", len(got.recs))
	}
}

// Searches as CLIENT pages them: newest first in pages of 20, the next page
// after the last lot, the page before the first (the first page again at the
// start), nothing past the end; the query's filters.
func TestAuctionSearchPages(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Other", "Third")
	s := f.chars["Seller"]
	// 20 lots each at most (AuctionMaxOpenLots): 1-20, 21-40, 41-45
	for _, x := range []struct {
		c     *houseChar
		n     int
		price func(i int) int
	}{
		{s, 20, func(i int) int { return 100 + i }},
		{f.chars["Other"], 20, func(int) int { return 500 }},
		{f.chars["Third"], 5, func(int) int { return 500 }},
	} {
		for i := 0; i < x.n; i++ {
			f.give(x.c, f.sword, 1)
		}
		for i := 0; i < x.n; i++ {
			f.sell(x.c, uint32(i), x.price(i), 1000, 720)
		}
		f.drain(x.c)
	}
	search := func(page, from int, forward bool, q auctionQuery) wirePage {
		t.Helper()
		f.request(s, 7, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(int64(page))
			w.Write7BitEncodedInt64(int64(from))
			if forward {
				w.WriteByte(1)
			} else {
				w.WriteByte(0)
			}
			w.WriteByte(q.flags)
			writeString8(w, q.seller, 24)
			writeWideString(w, q.text, 128)
			w.WriteUint32(q.classes)
			w.WriteUint32(q.races)
			for _, v := range []int{q.minLevel, q.maxLevel, q.minPrice, q.maxPrice, q.wear, q.attack} {
				w.Write7BitEncodedInt64(int64(v))
			}
			if q.open {
				w.WriteByte(1)
			} else {
				w.WriteByte(0)
			}
		})
		d, ok := findSent(f.drain(s), protocol.OpcodeAuctionStuff, 129)
		if !ok {
			t.Fatal("no page")
		}
		return readPage(t, d)
	}
	// the untouched window's query (S2 note §4's bytes, levels 1-50)
	any := auctionQuery{classes: 0xFFFFFFFF, races: 0xFFFFFFFF, minLevel: 1, maxLevel: 50, maxPrice: 999999999, wear: -2, attack: -1, open: true}
	p1 := search(3, -1, true, any)
	if p1.page != 3 || len(p1.lots) != 20 || p1.lots[0].id != 45 || p1.lots[19].id != 26 {
		t.Fatalf("page 1: %d lots, %d..%d", len(p1.lots), p1.lots[0].id, p1.lots[len(p1.lots)-1].id)
	}
	if len(p1.cols[0]) != 20 || p1.cols[1][0] != -1 || p1.cols[2][0] != 500 || p1.cols[0][0] != 0 {
		t.Errorf("columns %v", p1.cols)
	}
	if p1.minutes != f.now.Unix()/60 || p1.lots[0].end-p1.minutes != 721 {
		t.Errorf("minutes %d, end %d", p1.minutes, p1.lots[0].end)
	}
	p2 := search(4, 26, true, any)
	if len(p2.lots) != 20 || p2.lots[0].id != 25 {
		t.Errorf("page 2 starts at %d (%d lots)", p2.lots[0].id, len(p2.lots))
	}
	p3 := search(5, int(p2.lots[19].id), true, any)
	if len(p3.lots) != 5 || p3.lots[4].id != 1 {
		t.Errorf("page 3: %d lots", len(p3.lots))
	}
	if end := search(6, 1, true, any); len(end.lots) != 0 {
		t.Errorf("past the end: %d lots", len(end.lots))
	}
	back := search(7, int(p2.lots[0].id), false, any)
	if len(back.lots) != 20 || back.lots[0].id != 45 {
		t.Errorf("back from page 2: %d lots from %d", len(back.lots), back.lots[0].id)
	}
	if again := search(8, 45, false, any); len(again.lots) != 20 || again.lots[0].id != 45 {
		t.Errorf("back from the first page: %d lots", len(again.lots))
	}
	// filters
	q := any
	q.flags, q.maxPrice = 16, 110
	if p := search(9, -1, true, q); len(p.lots) != 11 {
		t.Errorf("price at most 110: %d lots", len(p.lots))
	}
	q = any
	q.seller = "other"
	if p := search(10, -1, true, q); len(p.lots) != 20 || p.lots[0].seller != "Other" {
		t.Errorf("by seller: %d", len(p.lots))
	}
	q = any
	q.flags, q.text = 12, "SWORD"
	if p := search(11, -1, true, q); len(p.lots) != 20 {
		t.Errorf("keywords: %d", len(p.lots))
	}
	q.text = "axe"
	if p := search(12, -1, true, q); len(p.lots) != 0 {
		t.Errorf("no axe: %d", len(p.lots))
	}
	q = any
	q.minLevel, q.maxLevel = 11, 60
	if p := search(13, -1, true, q); len(p.lots) != 0 {
		t.Errorf("level 11+: %d (the sword is 10)", len(p.lots))
	}
	q = any
	q.flags, q.wear = 2, 13
	if p := search(14, -1, true, q); len(p.lots) != 0 {
		t.Errorf("wear 13: %d (the sword's is 12)", len(p.lots))
	}
	q.wear = 12
	if p := search(15, -1, true, q); len(p.lots) != 20 {
		t.Errorf("wear 12: %d", len(p.lots))
	}
	q = any
	q.open = false
	if p := search(16, -1, true, q); len(p.lots) != 0 {
		t.Errorf("closed: %d", len(p.lots))
	}
	f.now = f.now.Add(13 * time.Hour)
	f.house.Sweep()
	if p := search(17, -1, true, q); len(p.lots) != 20 || p.cols[1][0] != -1 {
		t.Errorf("closed after the end: %d", len(p.lots))
	}
	if p := search(18, -1, true, any); len(p.lots) != 0 {
		t.Errorf("open after the end: %d", len(p.lots))
	}
}

// My Auctions / My Bids (sub-op 8): the lots asked, in the order asked; the
// bid history (9): the bidders, the leader at the current bid.
func TestAuctionLotsByIDAndHistory(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder", "Rival")
	s, a, b := f.chars["Seller"], f.chars["Bidder"], f.chars["Rival"]
	for i := 0; i < 3; i++ {
		f.give(s, f.sword, 1)
	}
	for i := 0; i < 3; i++ {
		f.sell(s, uint32(i), 100, 1000, 720)
	}
	f.bid(a, 2, 300)
	f.bid(b, 2, 200)
	f.drain(s)
	f.drain(a)
	f.drain(b)
	f.request(s, 8, func(w *buffer.Writer) {
		for _, v := range []int{9, 4, 3, 1, 99, 3} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
	d, ok := findSent(f.drain(s), protocol.OpcodeAuctionStuff, 129)
	if !ok {
		t.Fatal("no page")
	}
	p := readPage(t, d)
	if p.page != 9 || len(p.lots) != 2 || p.lots[0].id != 3 || p.lots[1].id != 1 {
		t.Errorf("lots by id: page %d, %+v", p.page, p.lots)
	}
	f.request(a, 9, func(w *buffer.Writer) { w.Write7BitEncodedInt64(2) })
	d, ok = findSent(f.drain(a), protocol.OpcodeAuctionStuff, 135)
	if !ok {
		t.Fatal("no 135")
	}
	r := &auctionWire{b: d[1:]}
	if lot := r.zz(); lot != 2 {
		t.Errorf("history of %d", lot)
	}
	n := r.zz()
	type row struct {
		char  int64
		name  string
		shown int64
	}
	var rows []row
	for i := int64(0); i < n; i++ {
		var x row
		x.char = r.zz()
		r.zz()
		r.zz()
		x.name = r.str8()
		x.shown = r.zz()
		r.zz()
		rows = append(rows, x)
	}
	if r.bad || r.p != len(d)-1 || len(rows) != 2 || rows[0].name != "Bidder" || rows[0].shown != 201 || rows[1].shown != 200 {
		t.Errorf("history rows %+v (the leader's proxy 300 must not show)", rows)
	}
}

// The MemoryDump's m_lots and m_bids read back as VIBankControl::Read reads
// them, each to its last byte.
func TestAuctionDumpListsReadBack(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	f.give(s, f.sword, 1)
	f.give(s, f.gem, 7)
	f.sell(s, 0, 100, 1000, 720)
	f.sell(s, 1, 100, 1000, 720)
	f.bid(a, 1, 300)
	buf := make([]byte, 16384)
	w := buffer.NewWriter(buf)
	f.house.WriteDumpLists(w, s.sid)
	f.house.WriteDumpLists(w, a.sid)
	r := &auctionWire{b: buf[:w.Position()]}
	if n := r.zz(); n != 2 {
		t.Fatalf("the seller's lots: %d", n)
	}
	l2, l1 := r.lot(), r.lot()
	if l2.id != 2 || l2.qty != 7 || l1.id != 1 || l1.item.name != "Fine Sword" || l1.item.desc != "" {
		t.Errorf("the seller's lots %+v %+v", l2, l1)
	}
	if n := r.zz(); n != 0 {
		t.Errorf("the seller's bids %d", n)
	}
	if n := r.zz(); n != 0 {
		t.Errorf("the bidder's lots %d", n)
	}
	if n := r.zz(); n != 1 {
		t.Fatalf("the bidder's bids %d", n)
	}
	if b := readBid(r); b.lot != 1 || b.max != 300 || b.escrow != 300 {
		t.Errorf("the bidder's bid %+v", b)
	}
	if r.bad || r.p != w.Position() {
		t.Errorf("read %d of %d", r.p, w.Position())
	}
	// a house-less dump: two empty lists (the old dump's bytes)
	w = buffer.NewWriter(make([]byte, 16))
	(*AuctionHouse)(nil).WriteDumpLists(w, 1)
	if got := w.Bytes(); len(got) != 2 || got[0] != 0 || got[1] != 0 {
		t.Errorf("no house: % x", got)
	}
}

// A replaced session's Player (its character logged in again) moves nothing
// of the bank.
func TestAuctionReplacedSessionRefused(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 1000, 720)
	f.replaced[a.player] = true
	f.bid(a, 1, 300)
	msgs := f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeCancelPending, 0); !ok || f.dbBank(a) != 1000 {
		t.Errorf("a replaced session's bid: bank %d, %v", f.dbBank(a), msgs)
	}
	// planted control: the live one may
	delete(f.replaced, a.player)
	f.bid(a, 1, 300)
	if f.drain(a); f.dbBank(a) != 700 {
		t.Errorf("the live session's bid: bank %d", f.dbBank(a))
	}
}

// Two buyers at the buyout at once, over the wire: one gets the item and
// pays, the other is refused and keeps its tunar; each client's bank list
// copy agrees with its rows.
func TestAuctionConcurrentBuyoutsOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder", "Rival")
	s, a, b := f.chars["Seller"], f.chars["Bidder"], f.chars["Rival"]
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 900, 720)
	var wg sync.WaitGroup
	for _, c := range []*houseChar{a, b} {
		wg.Add(1)
		go func(c *houseChar) {
			defer wg.Done()
			f.bid(c, 1, 900)
		}(c)
	}
	wg.Wait()
	got := 0
	for _, c := range []*houseChar{a, b} {
		rows := f.bankRows(c)
		list := sessionBankList(c.sess)
		if len(rows) != len(list.recs) {
			t.Errorf("%s: %d rows, %d in the list copy", c.name, len(rows), len(list.recs))
		}
		got += len(rows)
	}
	if got != 1 || f.dbBank(a)+f.dbBank(b) != 2000-900 || f.dbBank(s) != 999+900 {
		t.Errorf("items %d, buyers %d+%d, seller %d", got, f.dbBank(a), f.dbBank(b), f.dbBank(s))
	}
}

// The bank's tunar as kept is not written back from a stale Player: a
// purse's save keeps the purse alone, and a character's row save leaves both.
func TestTunarSavesKeepAnAuctionPayment(t *testing.T) {
	f := newHouseFixture(t, "Seller")
	s := f.chars["Seller"]
	if _, err := f.db.Exec(`UPDATE characters SET bankTunar = 1600 WHERE serverid = ?`, s.sid); err != nil {
		t.Fatal(err)
	}
	s.player.Tunar = 42 // the Player still says 1000 banked
	f.win.saveTunar(s.player)
	if f.dbBank(s) != 1600 {
		t.Errorf("a purse save wrote the bank back: %d", f.dbBank(s))
	}
	var purse int
	f.db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, s.sid).Scan(&purse)
	if purse != 42 {
		t.Errorf("the purse %d", purse)
	}
	store := database.NewSQLiteCharacterStore(f.db)
	rec, err := store.GetByServerID(s.sid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE characters SET bankTunar = 2600 WHERE serverid = ?`, s.sid); err != nil {
		t.Fatal(err)
	}
	rec.X = 5 // a logout's save, from the row read before the payment
	if err := store.Update(rec); err != nil {
		t.Fatal(err)
	}
	if f.dbBank(s) != 2600 {
		t.Errorf("a row save wrote the bank back: %d", f.dbBank(s))
	}
	// planted control: the purse-and-bank save does write the bank
	if err := f.inv.SetTunar(s.sid, 42, 1000); err != nil || f.dbBank(s) != 1000 {
		t.Errorf("SetTunar: %v, %d", err, f.dbBank(s))
	}
}

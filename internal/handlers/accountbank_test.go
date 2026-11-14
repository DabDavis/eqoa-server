package handlers

import (
	"fmt"
	"net"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The account bank (accountbank.go, database/accountbank.go): two characters of an
// account (Ash, Birch) and one of another (Cedar) in a temporary database, each with
// a session in the world that the handler finds through its manager. Every message a
// client is sent is played on a model of CLIENT's bank list (VIItemList's AddItem with
// its capacity, Took), which must equal the server's copy after every step and never
// be sent a record past its capacity. Every test has its control.

type acctClient struct {
	recs []acctRec
	next uint32
	cap  int
}

type acctRec struct {
	key     uint32
	pattern int
	qty     int
}

type acctFixture struct {
	t        *testing.T
	db       *database.SQLiteDB
	inv      *database.InventoryStore
	store    *database.SQLiteCharacterStore
	h        *NPCWindowHandler
	mgr      *session.Manager
	sid      map[string]uint32
	sess     map[string]*session.Session
	players  map[*session.Session]*entity.Player
	clients  map[*session.Session]*acctClient
	maxStack map[int]int
	gem      int // stacks to 20
	sword    int // a single one
	port     int
}

func newAcctFixture(t *testing.T, on bool, slots int) *acctFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/acct.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	database.SetAccountBank(on, slots)
	t.Cleanup(func() { database.SetAccountBank(false, 0) })
	f := &acctFixture{t: t, db: db, inv: database.NewInventoryStore(db), store: database.NewSQLiteCharacterStore(db),
		sid: map[string]uint32{}, sess: map[string]*session.Session{}, players: map[*session.Session]*entity.Player{},
		clients: map[*session.Session]*acctClient{}, maxStack: map[int]int{}, mgr: session.NewManager()}
	for _, c := range []struct {
		name string
		acct int
	}{{"Ash", 1}, {"Birch", 1}, {"Cedar", 2}} {
		rec := &database.CharacterRecord{CharName: c.name, AccountID: c.acct}
		if err := f.store.Create(rec); err != nil {
			t.Fatal(err)
		}
		f.sid[c.name] = rec.ServerID
	}
	f.gem = f.pattern("Rough Gem", 20)
	f.sword = f.pattern("Plain Sword", 1)
	f.h = NewNPCWindowHandler(f.mgr, f.inv)
	f.h.playerOf = func(s *session.Session) *entity.Player { return f.players[s] }
	f.h.find = func(_ *session.Session, id uint32) (entity.Entity, bool) {
		if id == shopBankerID {
			return &entity.NPC{Name: "Banker", NPCID: 8, NPCType: 0x82}, true
		}
		return nil, false
	}
	old := acctBankWindows.Load()
	acctBankWindows.Store(f.h)
	t.Cleanup(func() { acctBankWindows.Store(old) })
	return f
}

func (f *acctFixture) pattern(name string, maxStack int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, ?, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, 'An item.', 0, 0)`,
		maxStack, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	f.maxStack[int(id)] = maxStack
	return int(id)
}

// own gives a character n rows of a pattern at a location, each of stack.
func (f *acctFixture) own(name string, pattern, n, stack, location int) []int {
	f.t.Helper()
	var ids []int
	for range n {
		id, err := f.inv.AddCharItem(f.sid[name], pattern, stack, 100, 0, location)
		if err != nil {
			f.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// login puts a character in the world with a session whose client's bank list
// holds bankCap (40: a PS2's), its lists as its MemoryDump leaves them.
func (f *acctFixture) login(name string, bankCap int) *session.Session {
	f.t.Helper()
	f.port++
	s := f.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7100 + f.port}, uint32(100+f.port), 1, 1, 2, true)
	sid := f.sid[name]
	rec, err := f.store.GetByServerID(sid)
	if err != nil {
		f.t.Fatal(err)
	}
	s.SetAccountID(rec.AccountID)
	s.SetCharacterServerID(sid)
	s.SetInGame(true)
	s.SetCharacterInWorld(true)
	p := &entity.Player{ServerID: entity.EntityID(800 + f.port), CharServerID: sid, Name: name, Tunar: 1000, BankTunar: rec.BankTunar}
	f.players[s] = p
	f.sess[name] = s
	rows, err := f.inv.GetCharacterItems(sid)
	if err != nil {
		f.t.Fatal(err)
	}
	var pack, bank []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == database.LocBank {
			bank = append(bank, r)
		} else {
			pack = append(pack, r)
		}
	}
	var hidden map[int]bool
	if database.AccountBankOn() {
		bank, hidden = accountBankPage(bank, bankCap)
	} else {
		bank = capEntries(bank, bankCap)
	}
	pl, bl := newInvListCap(pack, 0), newInvListCap(bank, bankCap)
	bl.kept = hidden
	pl.bank = bl
	s.SetInventoryList(pl)
	s.SetBankList(bl)
	c := &acctClient{next: uint32(len(bank)), cap: bankCap}
	for i, r := range bank {
		c.recs = append(c.recs, acctRec{key: uint32(i), pattern: r.Pattern.PatternID, qty: r.StackLeft})
	}
	f.clients[s] = c
	s.Queue().GetPending()
	return s
}

func (f *acctFixture) send(name string, op protocol.GameOpcode, fill func(w *buffer.Writer)) {
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
	if err := h(f.sess[name], msg); err != nil {
		f.t.Fatal(err)
	}
}

// bankItem is a 0x1254: n of the record with key out of the bank (fromBank)
// or into it.
func (f *acctFixture) bankItem(name string, fromBank bool, key uint32, n int) {
	f.t.Helper()
	b := byte(0)
	if fromBank {
		b = 1
	}
	f.send(name, protocol.OpcodeBankItem, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(b)
		w.WriteUint32(key)
		w.Write7BitEncodedInt64(int64(n))
	})
}

// sent is what a character's client was sent since last asked, each played
// on its model of the bank list, which must then be the server's copy.
func (f *acctFixture) sent(name string) [][]byte {
	f.t.Helper()
	s := f.sess[name]
	var out [][]byte
	c := f.clients[s]
	for _, m := range s.Queue().GetPending() {
		b := append([]byte(nil), m.Data()...)
		out = append(out, b)
		switch opOf(b) {
		case protocol.OpcodeAddBankItem:
			d, ok := readDumpedItem(buffer.NewReader(b[2:]))
			if !ok {
				f.t.Fatalf("%s: a 0x1251 that does not read: % X", name, b)
			}
			c.add(f.t, name, int(d.pattern), int(d.stack), f.maxStack[int(d.pattern)])
		case protocol.OpcodeRemoveBankItem:
			r := buffer.NewReader(b[2:])
			slot, _ := r.ReadByte()
			n, _ := r.Read7BitEncodedInt64()
			c.took(f.t, name, int(slot), int(n))
		}
	}
	f.same(name)
	return out
}

// add is VIItemList::AddItem(item, maxStack, cap): onto the first record of a
// stackable item, else a new record while there is room. Sent one with no
// room is the failure this guards against (a 41st record on a PS2).
func (c *acctClient) add(t *testing.T, name string, pattern, qty, maxStack int) {
	t.Helper()
	if maxStack >= 2 {
		for i := range c.recs {
			if c.recs[i].pattern == pattern {
				c.recs[i].qty = min(maxStack, c.recs[i].qty+qty)
				return
			}
		}
	}
	if len(c.recs) >= c.cap {
		t.Fatalf("%s's client was sent a record past its list of %d", name, c.cap)
	}
	if maxStack < 2 {
		qty = 1
	}
	c.recs = append(c.recs, acctRec{key: c.next, pattern: pattern, qty: min(qty, max(maxStack, 1))})
	c.next++
}

// took is VIItemList::Took, which asserts the slot and the count.
func (c *acctClient) took(t *testing.T, name string, slot, n int) {
	t.Helper()
	if slot >= len(c.recs) || c.recs[slot].qty < n {
		t.Fatalf("%s's client was told to take %d off slot %d of %v", name, n, slot, c.recs)
	}
	c.recs[slot].qty -= n
	if c.recs[slot].qty == 0 {
		c.recs = append(c.recs[:slot], c.recs[slot+1:]...)
	}
}

// same fails when a client's model and the server's copy of its bank list differ.
func (f *acctFixture) same(name string) {
	f.t.Helper()
	s := f.sess[name]
	l, c := sessionBankList(s), f.clients[s]
	l.mu.Lock()
	defer l.mu.Unlock()
	var srv []acctRec
	for _, r := range l.recs {
		srv = append(srv, acctRec{key: r.key, pattern: r.patternID, qty: r.qty})
	}
	if fmt.Sprint(srv) != fmt.Sprint(c.recs) || l.next != c.next {
		f.t.Fatalf("%s: the server's bank list %v (next %d), its client's %v (next %d)", name, srv, l.next, c.recs, c.next)
	}
	if len(c.recs) > c.cap {
		f.t.Fatalf("%s's client holds %d past its %d", name, len(c.recs), c.cap)
	}
}

// keyOf is the key of a character's bank (or pack) record of a row.
func (f *acctFixture) keyOf(name string, bank bool, itemID int) uint32 {
	f.t.Helper()
	l := sessionInvList(f.sess[name])
	if bank {
		l = sessionBankList(f.sess[name])
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.recs {
		if r.itemID == itemID {
			return r.key
		}
	}
	f.t.Fatalf("%s's %s list has no record of item %d", name, map[bool]string{true: "bank", false: "pack"}[bank], itemID)
	return 0
}

func (f *acctFixture) rowAt(itemID int) (serverID uint32, location int) {
	f.t.Helper()
	if err := f.db.QueryRow(`SELECT serverid, location FROM charinventory WHERE itemid = ?`, itemID).Scan(&serverID, &location); err != nil {
		f.t.Fatal(err)
	}
	return serverID, location
}

// One bank for the account: what Ash banks Birch sees at its login and takes
// out (the row then Birch's), and the tunar Ash banks Birch withdraws. The
// control: Cedar, of another account, sees none of it.
func TestAccountBankSharedBetweenCharacters(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	sword := f.own("Ash", f.sword, 1, 1, 0)[0]
	f.login("Ash", 40)
	f.bankItem("Ash", false, f.keyOf("Ash", false, sword), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	f.send("Ash", protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(0)
		w.Write7BitEncodedInt64(500)
	})
	mustOps(t, f.sent("Ash"), protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	if n, _, _ := f.inv.BankTunarOf(f.sid["Birch"]); n != 500 {
		t.Fatalf("Birch's bank holds %d tunar, want Ash's 500", n)
	}

	f.login("Birch", 40)
	if l := sessionBankList(f.sess["Birch"]); len(l.itemIDs()) != 1 || l.itemIDs()[0] != sword {
		t.Fatalf("Birch's bank at login %v, want Ash's sword %d", l.itemIDs(), sword)
	}
	if p := f.players[f.sess["Birch"]]; p.BankTunar != 500 {
		t.Fatalf("Birch logs in with %d in the bank, want 500", p.BankTunar)
	}
	f.bankItem("Birch", true, f.keyOf("Birch", true, sword), 1)
	mustOps(t, f.sent("Birch"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	if sid, loc := f.rowAt(sword); sid != f.sid["Birch"] || loc != database.LocPack {
		t.Fatalf("the sword is character %d's at location %d, want Birch's pack", sid, loc)
	}
	f.send("Birch", protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(1)
		w.Write7BitEncodedInt64(200)
	})
	mustOps(t, f.sent("Birch"), protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	var acct int
	if err := f.db.QueryRow(`SELECT tunar FROM accountbank WHERE accountid = 1`).Scan(&acct); err != nil || acct != 300 {
		t.Fatalf("the account's bank holds %d (%v), want 300", acct, err)
	}
	// Ash's own column is not the bank's (the merge's record of it)
	var own int
	f.db.QueryRow(`SELECT bankTunar FROM characters WHERE serverid = ?`, f.sid["Ash"]).Scan(&own)
	if own != 0 {
		t.Errorf("Ash's own bankTunar column moved to %d", own)
	}

	// the control: another account's character
	f.own("Ash", f.gem, 1, 3, database.LocBank)
	f.login("Cedar", 40)
	if l := sessionBankList(f.sess["Cedar"]); len(l.itemIDs()) != 0 {
		t.Errorf("Cedar of account 2 sees account 1's bank: %v", l.itemIDs())
	}
	if n, _, _ := f.inv.BankTunarOf(f.sid["Cedar"]); n != 0 {
		t.Errorf("Cedar's bank holds %d tunar", n)
	}
}

// Both of the account's characters in the world at once, a PS2 (40) and a
// larger client (100): each move by one is shown to the other's client, the
// two lists in step with their clients at every message (sent's model), the
// tunar told; a stack grown is grown on the other's record. The control:
// Cedar's client, of another account, is sent nothing.
func TestAccountBankTwoOnlineAtOnce(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	swords := f.own("Ash", f.sword, 2, 1, 0)
	gems := f.own("Ash", f.gem, 1, 5, 0)
	bankGem := f.own("Birch", f.gem, 1, 4, database.LocBank)[0]
	f.login("Ash", 40)
	f.login("Birch", 100)
	f.login("Cedar", 40)

	f.bankItem("Ash", false, f.keyOf("Ash", false, swords[0]), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	mustOps(t, f.sent("Birch"), protocol.OpcodeAddBankItem)
	mustOps(t, f.sent("Cedar"))

	// two of Ash's five gems onto the bank's stack of four (Birch's row)
	f.bankItem("Ash", false, f.keyOf("Ash", false, gems[0]), 2)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	mustOps(t, f.sent("Birch"), protocol.OpcodeAddBankItem)
	var stack int
	f.db.QueryRow(`SELECT stackleft FROM charinventory WHERE itemid = ?`, bankGem).Scan(&stack)
	if stack != 6 {
		t.Fatalf("the bank's gems %d, want 6", stack)
	}

	// Birch takes the sword Ash banked: Ash's page loses it
	f.bankItem("Birch", true, f.keyOf("Birch", true, swords[0]), 1)
	mustOps(t, f.sent("Birch"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveBankItem)
	// and three gems out: both stacks shrink
	f.bankItem("Birch", true, f.keyOf("Birch", true, bankGem), 3)
	mustOps(t, f.sent("Birch"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveBankItem)

	// tunar: Ash banks 300, Birch's client is told and its Player holds it
	f.send("Ash", protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(0)
		w.Write7BitEncodedInt64(300)
	})
	mustOps(t, f.sent("Ash"), protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	mustOps(t, f.sent("Birch"), protocol.OpcodeConfirmBankTunar)
	if p := f.players[f.sess["Birch"]]; p.BankTunar != 300 {
		t.Fatalf("Birch's Player holds %d in the bank, want 300", p.BankTunar)
	}
	// Birch, holding the account's 300, cannot take out 301 (Ash's cached view
	// is not what decides): refused
	f.send("Birch", protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(1)
		w.Write7BitEncodedInt64(301)
	})
	mustOps(t, f.sent("Birch"), protocol.OpcodeCancelPending)
	mustOps(t, f.sent("Cedar"))
}

// 2000 records an account, a page of its client's list shown: a deposit past a
// full page goes in unshown (the pack's 0x003C and 0x1253 to end the wait),
// the 2001st is refused, and /bank turns the page (every record off, the
// page's on), never a record past the client's 40. The control: the same
// deposit with the account bank off is refused at the client's 40.
func TestAccountBankHolds2000(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	if database.AccountBankSlots() != 2000 {
		t.Fatalf("the account bank holds %d, want 2000", database.AccountBankSlots())
	}
	if database.BankSlotsOf(f.sid["Ash"]) != 2000 {
		t.Fatalf("the auction files into %d, want 2000", database.BankSlotsOf(f.sid["Ash"]))
	}
	tx, _ := f.db.Begin()
	for i := 0; i < 1999; i++ {
		owner := f.sid["Ash"]
		if i%2 == 1 {
			owner = f.sid["Birch"]
		}
		if _, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, 1, 100, 0, ?, -1, 1, ?)`, owner, f.sword, i); err != nil {
			t.Fatal(err)
		}
	}
	tx.Commit()
	swords := f.own("Ash", f.sword, 2, 1, 0)
	f.login("Ash", 40)
	f.login("Birch", 40)
	if n := len(sessionBankList(f.sess["Ash"]).itemIDs()); n != 40 {
		t.Fatalf("Ash's page holds %d, want a PS2's 40", n)
	}

	f.bankItem("Ash", false, f.keyOf("Ash", false, swords[0]), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeConfirmBankTunar, protocol.OpcodeColoredChat)
	mustOps(t, f.sent("Birch")) // its page is full: nothing to show
	if sid, loc := f.rowAt(swords[0]); sid != f.sid["Ash"] || loc != database.LocBank {
		t.Fatalf("the sword is %d's at %d, want in the bank", sid, loc)
	}
	if n, _ := f.inv.BankRowCount(f.sid["Birch"]); n != 2000 {
		t.Fatalf("the account's bank holds %d, want 2000", n)
	}
	f.bankItem("Ash", false, f.keyOf("Ash", false, swords[1]), 1)
	msgs := f.sent("Ash")
	if opOf(msgs[len(msgs)-1]) != protocol.OpcodeCancelPending {
		t.Fatalf("the 2001st: %s, want a refusal", ops(msgs))
	}
	if _, loc := f.rowAt(swords[1]); loc != database.LocPack {
		t.Fatal("the 2001st left the pack")
	}

	// the last page: 2000 records of 40 a page, the 50th, which holds the sword
	f.h.bankPageCommand(f.sess["Ash"], "last")
	f.sent("Ash")
	if ids := sessionBankList(f.sess["Ash"]).itemIDs(); len(ids) != 40 || !containsInt(ids, swords[0]) {
		t.Fatalf("the last page %d records, the sword on it %v", len(ids), containsInt(ids, swords[0]))
	}
	// Birch takes one off its first page: Ash's page is not changed (the row was not on it)
	first := sessionBankList(f.sess["Birch"]).itemIDs()[0]
	f.bankItem("Birch", true, f.keyOf("Birch", true, first), 1)
	f.sent("Birch")
	f.sent("Ash")
	f.h.bankPageCommand(f.sess["Ash"], "1")
	f.sent("Ash")
	if ids := sessionBankList(f.sess["Ash"]).itemIDs(); len(ids) != 40 || containsInt(ids, first) {
		t.Fatalf("page 1 after Birch's withdrawal: %d records, the withdrawn one on it %v", len(ids), containsInt(ids, first))
	}

	// the control: no account bank, a PS2's full bank refuses the deposit
	g := newAcctFixture(t, false, 0)
	g.own("Ash", g.sword, 40, 1, database.LocBank)
	s := g.own("Ash", g.sword, 1, 1, 0)[0]
	g.login("Ash", 40)
	g.bankItem("Ash", false, g.keyOf("Ash", false, s), 1)
	mustOps(t, g.sent("Ash"), protocol.OpcodeCancelPending)
	if database.BankSlotsOf(g.sid["Ash"]) != database.AuctionBankSlots {
		t.Errorf("off, the auction files into %d", database.BankSlotsOf(g.sid["Ash"]))
	}
}

// The account bank off is every character's own bank, as before: Ash's
// banked sword and tunar are not Birch's, and both are written to Ash's row.
func TestAccountBankOffIsPerCharacter(t *testing.T) {
	f := newAcctFixture(t, false, 0)
	sword := f.own("Ash", f.sword, 1, 1, 0)[0]
	f.login("Ash", 40)
	f.login("Birch", 40)
	f.bankItem("Ash", false, f.keyOf("Ash", false, sword), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	mustOps(t, f.sent("Birch"))
	f.send("Ash", protocol.OpcodeDepositBankTunar, func(w *buffer.Writer) {
		w.WriteUint32(shopBankerID)
		w.WriteByte(0)
		w.Write7BitEncodedInt64(500)
	})
	mustOps(t, f.sent("Ash"), protocol.OpcodePlayerTunar, protocol.OpcodeConfirmBankTunar)
	mustOps(t, f.sent("Birch"))
	rows, _ := f.inv.GetCharacterItems(f.sid["Birch"])
	if len(rows) != 0 {
		t.Errorf("Birch holds %d rows of Ash's", len(rows))
	}
	if n, _, _ := f.inv.BankTunarOf(f.sid["Birch"]); n != 0 {
		t.Errorf("Birch's bank holds %d tunar", n)
	}
	var own int
	f.db.QueryRow(`SELECT bankTunar FROM characters WHERE serverid = ?`, f.sid["Ash"]).Scan(&own)
	if own != 500 {
		t.Errorf("Ash's bankTunar %d, want 500", own)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM accountbank`).Scan(&n)
	if n != 0 {
		t.Errorf("%d accountbank rows with the account bank off", n)
	}
	if _, err := f.h.inv.GetCharItem(f.sid["Birch"], sword); err == nil {
		t.Error("Birch reaches Ash's banked sword")
	}
}

// A character deleted keeps its bank rows the account's: Birch, and a
// character made after, see what Ash banked. The control: off, Ash's rows are
// no one's.
func TestAccountBankKeptOverADeletion(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := newAcctFixture(t, on, 0)
		banked := f.own("Ash", f.gem, 1, 7, database.LocBank)[0]
		if err := f.store.Delete(f.sid["Ash"]); err != nil {
			t.Fatal(err)
		}
		rows, err := f.inv.GetBankItems(f.sid["Birch"])
		if err != nil {
			t.Fatal(err)
		}
		got := len(rows) == 1 && rows[0].ItemID == banked && rows[0].StackLeft == 7
		if got != on {
			t.Errorf("account bank %v: after Ash's deletion Birch's bank %v", on, rows)
		}
		if !on {
			continue
		}
		rec := &database.CharacterRecord{CharName: "Dogwood", AccountID: 1}
		if err := f.store.Create(rec); err != nil {
			t.Fatal(err)
		}
		if rows, _ := f.inv.GetBankItems(rec.ServerID); len(rows) != 1 {
			t.Errorf("a character made after sees %d of the bank's rows", len(rows))
		}
		// and withdraws it: the row is its own
		f.sid["Dogwood"] = rec.ServerID
		f.login("Dogwood", 40)
		f.bankItem("Dogwood", true, f.keyOf("Dogwood", true, banked), 7)
		mustOps(t, f.sent("Dogwood"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
		if sid, loc := f.rowAt(banked); sid != rec.ServerID || loc != database.LocPack {
			t.Errorf("the gems are %d's at %d", sid, loc)
		}
	}
}

// pageFill never takes a stackable item a record of the page is already of
// (AddItem would stack it onto that record), nor a row AddItem would cut, and
// wraps round from the page's place.
func TestAccountBankPageFill(t *testing.T) {
	row := func(id, pattern, stack, maxStack int) database.CharInventoryEntry {
		e := database.CharInventoryEntry{ItemID: id, StackLeft: stack, EquipLoc: -1}
		e.Pattern.PatternID, e.Pattern.MaxStack = pattern, maxStack
		return e
	}
	rows := []database.CharInventoryEntry{
		row(1, 10, 1, 1), row(2, 20, 5, 20), row(3, 20, 7, 20), row(4, 30, 30, 20), row(5, 40, 2, 1), row(6, 50, 1, 1),
	}
	got := pageFill(rows, 0, nil, nil, 10)
	if fmt.Sprint(got) != "[0 1 5]" {
		t.Errorf("fill from 0: %v, want [0 1 5] (a second gem stack, an overfull stack and a stack of a single item left)", got)
	}
	if got := pageFill(rows, 5, map[int]bool{6: true}, map[int]bool{20: true}, 2); fmt.Sprint(got) != "[0]" {
		t.Errorf("fill from 5 round: %v, want [0]", got)
	}
	if got := pageFill(rows, 0, nil, nil, 0); got != nil {
		t.Errorf("no room: %v", got)
	}
}

// A dump with the account bank (fitItemLists): a PS2 is sent the first 40 of
// an account bank of 100 and no row is kept (location 3); the rest are held
// for the Lore rule. The control: off, the 60 past 40 are kept for it.
func TestAccountBankDumpIsAPage(t *testing.T) {
	for _, on := range []bool{true, false} {
		w := newListsWorld(t)
		database.SetAccountBank(on, 0)
		t.Cleanup(func() { database.SetAccountBank(false, 0) })
		w.own(w.sword, 99, database.LocBank)
		w.own(w.charm, 1, database.LocBank) // the 100th: off the PS2's page
		sess := w.session(0, 0)
		dump, _ := w.dump(sess, true)
		_, bnk := dumpedLists(t, dump, 0, 40)
		if len(bnk) != 40 {
			t.Fatalf("on %v: %d bank records dumped", on, len(bnk))
		}
		kept := len(w.rowsAt()[database.LocBankKept])
		switch {
		case on && kept != 0:
			t.Errorf("the account bank kept %d rows", kept)
		case !on && kept != 60:
			t.Errorf("off: %d kept, want 60", kept)
		}
		if on && !sessionInvList(sess).loreHeld(w.charm) {
			t.Error("a Lore item in the account bank off the page is not held")
		}
	}
}

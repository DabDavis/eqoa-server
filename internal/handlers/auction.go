package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The auction house on the wire: CLIENT's VIBankControl (decomp
// vibankcontrol.cpp), its sell form (viwndauction.cpp) and its search window
// (viwndauctionsearch.cpp). What it holds lives in database/auction.go's
// tables; this is their view and the messages that keep every client in step.
// No capture holds an auction session but searches (claude-notes/
// sandstorm-s2-live-20261015.md §4): the formats are CLIENT's.
//
//	C>S 0x1205 5  u32 the banker, u32 the bank record's key, zz a count (the
//	              constant 1: the whole record goes), zz starting bid, zz
//	              buyout, zz minutes                                  Sell
//	C>S 0x1205 6  zz lot, zz amount (a proxy: the most the bidder will
//	              pay; the buyout to buy outright), zz 1              Send06
//	C>S 0x1205 7  zz page, zz the lot it starts from (-1 a new search), u8
//	              forward, the query (u8 flags, str seller, wide keywords,
//	              u32 classes, u32 races, zz min/max level, zz min/max
//	              price, zz wear slot, zz attack type, u8 open)  Search/Next/PrevPage
//	C>S 0x1205 8  zz page, zz count, zz lot ids                       RequestLots/RequestBids
//	C>S 0x1205 9  zz lot                                              Send09 (the bid history)
//	S>C 0x1206 129 zz page, zz the house's minutes, the lots, then three
//	              arrays of a zz each a lot: the leader's character id (0
//	              none; the player's own shows "(you)"), the current bid (-1
//	              "no bids"; once closed "Sold For"), the least next bid
//	              (word 1710 "Min. Bid")                              RecvResults
//	S>C 0x1206 130 a lot, into the seller's m_lots                    RecvAddLot
//	S>C 0x1206 132/134 a bid record into / over the bidder's m_bids:
//	              zz character, zz lot, zz proxy ("Your Bid"), zz 0, zz held
//	              ("In Escrow"), u8 0                                 RecvAddBid/RecvUpdateBid
//	S>C 0x1206 135 zz lot, the bidders: zz character, zz lot, zz 0, str
//	              name, zz the bid shown, zz 0                        RecvRemoteChars
//	S>C 0x1206 137 a lot, zz minutes, its three columns: one lot changed
//	              wherever a page shows it                            RecvUpdateLot
//
// A lot (operator<<(VIMemStream &, const VIAuctionItem &), 0x00CBB1F0) is
// seven zz words (the lot, the stack, hit points, charges, the starting bid,
// the buyout, the end in the house's minutes), the seller and the item. The
// house's minutes are Unix minutes: a page carries now and each lot its end,
// and CLIENT's GetMinutes adds the minutes since the page came; 137 resets
// that base without resetting the page's arrival, so it carries the minutes
// of the page this session was last sent.
//
// The bank's wait (VIBankControl::m_state) is cleared by 129, 130, 132, 135,
// 0x1250, 0x1251, 0x1253 and 0x00D7, never by 131, 133, 134 or 137: every
// request is answered with one of the first.
//
// Money and items move only in the bank, as EQOA's did (Prima's Frontiers
// guide, "Auctions"): the fee, a bid's escrow and a refund out of or into
// the bank's tunar (0x1253), the item out of the bank list (0x1250) and into
// the winner's or back into the seller's (0x1251). A player in the world is
// told at once (0x1253, 0x1251, 0x1206 134 and 137, a line); one who is not
// finds its bank right at its next login, where the lines wait for it
// (auction_notices). An item a full bank cannot take waits at the house and
// is filed when the player logs in or opens the bank with room for it.
//
// bankMu (bank.go) serializes the bank: its list's copy (invList), its tunar
// (Player.BankTunar) and their rows, here, in the bank's own moves and in a
// MemoryDump. The house's own lock comes first; nothing takes either with the
// World's lock held, and no transaction is open while either the World or a
// session is consulted.

const (
	auctionPageSize   = 20 // a page of lots: Sandstorm's (15 pages of 20, newest first)
	auctionDumpLots   = 20 // the MemoryDump's m_lots, newest first
	auctionDumpBids   = 40 // and m_bids
	auctionIDsAsked   = 100
	auctionSweepEvery = 5 * time.Second
)

// auctionQuiet is how long a session may go unheard and still be told its
// lines: a client in the world sends its state many times a second, and a
// session whose client is gone lingers until it times out (a minute and more
// on the rig), where a line told is a line lost. A quiet one's lines wait for
// its character's next login.
var auctionQuiet = 15 * time.Second

// auctionCyan is CLIENT's colour for the auction's lines (RecvResults'
// AddText); a refusal is red (0x00CE), as the search window's own.
var auctionCyan = [3]uint8{48, 245, 245}

// AuctionHouse runs the auction: 0x1205's requests, the sweep that closes
// lots at their time, and the hooks that keep a player's bank right as it
// logs in.
type AuctionHouse struct {
	db      *database.SQLiteDB
	windows *NPCWindowHandler // the banker a sale needs, the session's player
	world   *entity.World

	isGM func(*session.Session) bool
	now  func() time.Time
	// live is the session and Player of a character in the world, nil when
	// it is not; isLive reports a Player its character's live one (a
	// replaced session's is not, and nothing of the bank is done for it).
	// The World's (SetGameWorld); tests stand in for them.
	live   func(serverID uint32) (*session.Session, *entity.Player)
	isLive func(*entity.Player) bool

	mu      sync.Mutex // one request or sweep at a time
	pageMin map[*session.Session]int64

	stopOnce sync.Once
	stop     chan struct{}
}

// NewAuctionHouse makes the house over a database.
func NewAuctionHouse(db *database.SQLiteDB, windows *NPCWindowHandler) *AuctionHouse {
	a := &AuctionHouse{db: db, windows: windows, now: time.Now, pageMin: map[*session.Session]int64{},
		stop: make(chan struct{})}
	a.isGM = func(*session.Session) bool { return false }
	a.live = a.liveSession
	a.isLive = func(p *entity.Player) bool { return p != nil }
	if windows != nil {
		windows.auction = a
	}
	return a
}

// SetGameWorld lets the house find players in the world and tell them as
// they enter it.
func (a *AuctionHouse) SetGameWorld(gw *entity.World) {
	a.world = gw
	if gw != nil {
		a.isLive = gw.IsLivePlayer
		gw.OnPlayerEntered(a.playerEntered)
	}
}

// auctionHouse is the process's running house (Auctions), as Quests is
// the quest engine: a test that builds the registry stops it.
var auctionHouse atomic.Pointer[AuctionHouse]

// Auctions is the running auction house, nil before one starts.
func Auctions() *AuctionHouse { return auctionHouse.Load() }

// Start runs the sweep: at once (what ended while the server was down), then
// every auctionSweepEvery.
func (a *AuctionHouse) Start() {
	auctionHouse.Store(a)
	go func() {
		t := time.NewTicker(auctionSweepEvery)
		defer t.Stop()
		for {
			a.Sweep()
			select {
			case <-a.stop:
				return
			case <-t.C:
			}
		}
	}()
}

// Stop ends the sweep.
func (a *AuctionHouse) Stop() {
	a.stopOnce.Do(func() { close(a.stop) })
}

// liveSession is the session whose Player is the character's live one.
func (a *AuctionHouse) liveSession(serverID uint32) (*session.Session, *entity.Player) {
	gw := a.world
	if gw == nil || gw.SessionManager == nil || serverID == 0 {
		return nil, nil
	}
	var found *session.Session
	var player *entity.Player
	gw.SessionManager.ForEach(func(s *session.Session) bool {
		if p := gw.SessionPlayer(s); p != nil && p.CharServerID == serverID && gw.IsLivePlayer(p) {
			found, player = s, p
			return false
		}
		return true
	})
	return found, player
}

// --- the messages ---

func auctionMinutes(unix int64) int64 { return unix / 60 }

// writeLot is operator<<(VIMemStream &, const VIAuctionItem &). thin leaves
// the item's text and stats out: the MemoryDump's m_lots serve only HasLot
// and My Auctions' ids, and every page carries the whole lot.
func writeLot(w *buffer.Writer, l *database.AuctionLot, thin bool) {
	for _, v := range []int64{int64(l.ID), int64(l.Item.StackLeft), int64(l.Item.RemainHP), int64(l.Item.Charges),
		int64(l.MinBid), int64(l.Buyout), auctionMinutes(l.EndsAt)} {
		w.Write7BitEncodedInt64(v)
	}
	writeString8(w, l.SellerName, 24)
	e := l.Item
	if thin {
		e.Pattern = database.ItemPatternRecord{
			PatternID: e.Pattern.PatternID, ItemCost: e.Pattern.ItemCost, ItemIcon: e.Pattern.ItemIcon,
			EquipSlot: e.Pattern.EquipSlot, Trade: e.Pattern.Trade, Rent: e.Pattern.Rent,
			LevelReq: e.Pattern.LevelReq, MaxStack: e.Pattern.MaxStack, MaxHP: e.Pattern.MaxHP,
			ClassUse: e.Pattern.ClassUse, RaceUse: e.Pattern.RaceUse, ItemName: e.Pattern.ItemName,
		}
	}
	writeItemPattern(w, &e)
}

// lotBytes is how much room a lot may take on the wire.
func lotBytes(n int) int { return 2048 + 1536*n }

// resultsPayload is 0x1206 129: a page of lots and their three columns.
func resultsPayload(page int, minutes int64, lots []database.AuctionLot) []byte {
	return build(protocol.OpcodeAuctionStuff, lotBytes(len(lots)), func(w *buffer.Writer) {
		w.WriteByte(129)
		w.Write7BitEncodedInt64(int64(page))
		w.Write7BitEncodedInt64(minutes)
		w.Write7BitEncodedInt64(int64(len(lots)))
		for i := range lots {
			writeLot(w, &lots[i], false)
		}
		for col := 0; col < 3; col++ {
			w.Write7BitEncodedInt64(int64(len(lots)))
			for i := range lots {
				w.Write7BitEncodedInt64(int64(lotColumn(&lots[i], col)))
			}
		}
	})
}

// lotColumn is a page's column for a lot: 0 the leader, 1 the current bid,
// 2 the least next bid.
func lotColumn(l *database.AuctionLot, col int) int {
	switch col {
	case 0:
		return int(l.High)
	case 1:
		return l.CurrentBid()
	}
	return l.MinNext()
}

// addLotPayload is 0x1206 130.
func addLotPayload(l *database.AuctionLot) []byte {
	return build(protocol.OpcodeAuctionStuff, lotBytes(1), func(w *buffer.Writer) {
		w.WriteByte(130)
		writeLot(w, l, false)
	})
}

// updateLotPayload is 0x1206 137.
func updateLotPayload(l *database.AuctionLot, minutes int64) []byte {
	return build(protocol.OpcodeAuctionStuff, lotBytes(1), func(w *buffer.Writer) {
		w.WriteByte(137)
		writeLot(w, l, false)
		w.Write7BitEncodedInt64(minutes)
		for col := 0; col < 3; col++ {
			w.Write7BitEncodedInt64(int64(lotColumn(l, col)))
		}
	})
}

// writeBid is operator<<(VIMemStream &, const VIDbCharBid &): the key, the
// proxy (FindBid's first, "Your Bid"), a word no reader shows, what is held
// (FindBid's second, "In Escrow"), a byte.
func writeBid(w *buffer.Writer, b *database.AuctionBid) {
	for _, v := range []int{int(b.Bidder), b.Lot, b.Max, 0, b.Escrow} {
		w.Write7BitEncodedInt64(int64(v))
	}
	w.WriteByte(0)
}

// bidPayload is 0x1206 132 (a new record) or 134 (over the record of the
// same lot).
func bidPayload(sub byte, b *database.AuctionBid) []byte {
	return build(protocol.OpcodeAuctionStuff, 64, func(w *buffer.Writer) {
		w.WriteByte(sub)
		writeBid(w, b)
	})
}

// biddersPayload is 0x1206 135: a lot's bids, each its bidder's highest, the
// leader's shown at the lot's current bid (its proxy is its own).
func biddersPayload(l *database.AuctionLot, bids []database.AuctionBid) []byte {
	return build(protocol.OpcodeAuctionStuff, 64+48*len(bids), func(w *buffer.Writer) {
		w.WriteByte(135)
		w.Write7BitEncodedInt64(int64(l.ID))
		w.Write7BitEncodedInt64(int64(len(bids)))
		for _, b := range bids {
			shown := b.Max
			if b.Bidder == l.High {
				shown = l.Price
			}
			w.Write7BitEncodedInt64(int64(b.Bidder))
			w.Write7BitEncodedInt64(int64(l.ID))
			w.Write7BitEncodedInt64(0)
			writeString8(w, b.Name, 24)
			w.Write7BitEncodedInt64(int64(shown))
			w.Write7BitEncodedInt64(0)
		}
	})
}

// auctionLine is a line in the auction's cyan.
func auctionLine(sess *session.Session, text string) {
	SendColoredChat(sess, text, auctionCyan[0], auctionCyan[1], auctionCyan[2])
}

// --- the requests ---

// HandleAuction is 0x1205.
func (h *NPCWindowHandler) HandleAuction(sess *session.Session, msg *protocol.Message) error {
	if h.auction == nil {
		refuse(sess)
		return nil
	}
	return h.auction.Handle(sess, msg)
}

// Handle is 0x1205: a sub-op byte and its fields.
func (a *AuctionHouse) Handle(sess *session.Session, msg *protocol.Message) error {
	r := newClientReader(msg)
	sub := r.u8()
	if r.err || a.windows == nil || a.windows.playerOf == nil {
		return nil
	}
	player := a.windows.playerOf(sess)
	if player == nil {
		return nil
	}
	switch sub {
	case 5:
		banker, key := r.u32(), r.u32()
		count, price, buyout, minutes := r.signed(), r.signed(), r.signed(), r.signed()
		if r.err || count < 1 {
			refuse(sess)
			return nil
		}
		a.sell(sess, player, banker, key, int(price), int(buyout), int(minutes))
	case 6:
		lot, amount := r.signed(), r.signed()
		if r.err {
			refuse(sess)
			return nil
		}
		a.bid(sess, player, int(lot), int(amount))
	case 7:
		page, from := r.signed(), r.signed()
		forward := r.u8() != 0
		q := readAuctionQuery(r)
		if r.err {
			refuse(sess)
			return nil
		}
		a.search(sess, player, int(page), int(from), forward, q)
	case 8:
		page, n := r.signed(), r.signed()
		var ids []int
		for i := 0; i < int(n) && i < auctionIDsAsked && !r.err; i++ {
			ids = append(ids, int(r.signed()))
		}
		if r.err {
			refuse(sess)
			return nil
		}
		a.lotsByID(sess, player, int(page), ids)
	case 9:
		lot := r.signed()
		if r.err {
			refuse(sess)
			return nil
		}
		a.history(sess, player, int(lot))
	default:
		log.Printf("Auction: %s sent sub-op %d, not handled", player.Name, sub)
		refuse(sess)
	}
	return nil
}

// refused answers a request the house will not carry out: the line in red
// (as the search window's own refusals), 0x00D7 to end the wait.
func (a *AuctionHouse) refused(sess *session.Session, player *entity.Player, what string, err error) {
	line, ok := database.IsAuctionRefusal(err)
	if !ok {
		log.Printf("Auction: %s's %s: %v", player.Name, what, err)
		line = "The auction house cannot do that now."
	} else {
		log.Printf("Auction: %s's %s refused: %s", player.Name, what, line)
	}
	SendErrorMessage(sess, line)
	refuse(sess)
}

// sell is sub-op 5: the bank record with that key up for auction, at a
// banker (Prima: "Go to the bank and speak to one of the bankers").
func (a *AuctionHouse) sell(sess *session.Session, player *entity.Player, banker, key uint32, price, buyout, minutes int) {
	if _, _, ok := a.windows.windowNPC(sess, banker, npcFlagBank, "AuctionSell"); !ok || !a.isLive(player) {
		refuse(sess)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	bank := sessionBankList(sess)
	if bank == nil {
		refuse(sess)
		return
	}
	i := bank.index(key)
	rec, found := bank.at(i)
	if !found {
		a.refused(sess, player, "sale", &database.AuctionRefusal{Line: "That item is not in your bank."})
		return
	}
	lot, after, err := a.db.AuctionList(database.AuctionListing{
		Seller: player.CharServerID, SellerName: player.Name,
		ItemID: rec.itemID, PatternID: rec.patternID, Stack: rec.qty,
		MinBid: price, Buyout: buyout, Minutes: minutes, GM: a.isGM(sess), Now: a.now(),
	})
	if err != nil {
		a.refused(sess, player, "sale", err)
		return
	}
	bank.took(i, rec.qty)
	player.BankTunar = after
	queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, rec.qty))
	queueReliable(sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(after))
	queueReliable(sess, protocol.OpcodeAuctionStuff, addLotPayload(&lot))
	auctionLine(sess, fmt.Sprintf("%s is up for auction. The listing fee was %d tunar.", lot.Item.Pattern.ItemName, lot.Fee))
	log.Printf("Auction: %s put up %d %q as lot %d (%d / %d, %d minutes, fee %d; bank %d)",
		player.Name, lot.Item.StackLeft, lot.Item.Pattern.ItemName, lot.ID, price, buyout, minutes, lot.Fee, after)
}

// bid is sub-op 6: a proxy bid, or the buyout.
func (a *AuctionHouse) bid(sess *session.Session, player *entity.Player, lot, amount int) {
	if !a.isLive(player) {
		refuse(sess)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	res, err := a.db.AuctionBid(database.AuctionBidRequest{
		Bidder: player.CharServerID, Name: player.Name, Lot: lot, Amount: amount, Now: a.now(),
	})
	if err != nil {
		a.refused(sess, player, "bid", err)
		return
	}
	name := res.Lot.Item.Pattern.ItemName
	player.BankTunar = res.BidderBank
	sub := byte(132)
	if res.HadBid {
		sub = 134
	}
	queueReliable(sess, protocol.OpcodeAuctionStuff, bidPayload(sub, &res.Bid))
	cleared := sub == 132
	if res.Taken > 0 || (res.Settled != nil && res.Settled.Refund > 0) {
		queueReliable(sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(res.BidderBank))
		cleared = true
	}
	queueReliable(sess, protocol.OpcodeAuctionStuff, updateLotPayload(&res.Lot, a.pageMinutes(sess)))
	switch res.Kind {
	case database.BidFirst, database.BidTookLead:
		auctionLine(sess, fmt.Sprintf("You are the high bidder on %s at %d tunar. Your proxy bid of %d is held in escrow.",
			name, res.Lot.Price, res.Bid.Max))
	case database.BidRaised:
		auctionLine(sess, fmt.Sprintf("Your proxy bid on %s is now %d tunar, held in escrow.", name, res.Bid.Max))
	case database.BidOutbid:
		auctionLine(sess, fmt.Sprintf("Another bidder's proxy bid on %s is higher. The bid stands at %d tunar.", name, res.Lot.Price))
	}
	if !cleared {
		refuse(sess) // 134 alone leaves the wait: 0x00D7 ends it
	}
	log.Printf("Auction: %s bid %d on lot %d (%s): kind %d, price %d, leader %d, bank %d",
		player.Name, amount, lot, name, res.Kind, res.Lot.Price, res.Lot.High, res.BidderBank)
	told := map[uint32]bool{player.CharServerID: true}
	if res.Refund != nil {
		a.refundedLocked(*res.Refund, &res.Lot)
		told[res.Refund.Bidder] = true
	}
	if res.Settled != nil {
		a.settledLocked(*res.Settled)
		told[res.Settled.Winner], told[res.Settled.Lot.Seller] = true, true
	} else if !told[res.Lot.Seller] {
		a.showLotLocked(res.Lot.Seller, &res.Lot)
	}
	a.tellLocked(sess, player.CharServerID)
}

// pageMinutes is the minutes of the page this session was last sent (137
// resets GetMinutes' base, not the page's arrival), else now.
func (a *AuctionHouse) pageMinutes(sess *session.Session) int64 {
	if m, ok := a.pageMin[sess]; ok {
		return m
	}
	return auctionMinutes(a.now().Unix())
}

// sendPage sends 0x1206 129 and keeps its minutes for the session's 137s.
func (a *AuctionHouse) sendPage(sess *session.Session, page int, lots []database.AuctionLot) {
	m := auctionMinutes(a.now().Unix())
	a.mu.Lock()
	a.pageMin[sess] = m
	a.mu.Unlock()
	queueReliable(sess, protocol.OpcodeAuctionStuff, resultsPayload(page, m, lots))
}

// auctionQuery is VIAuctionQuery as Search (0x00C139B0) fills it.
type auctionQuery struct {
	flags              byte   // 16 a price ceiling, 2 a wear slot, 1 an attack type, 12 keywords
	seller             string // m_name: the seller's name
	text               string // m_text: the keywords
	classes, races     uint32 // 1 << the picked one; all bits ANY; races 0 NONE
	minLevel, maxLevel int
	minPrice, maxPrice int
	wear, attack       int
	open               bool // "Status: Open"; Closed: lots that ended
}

func readAuctionQuery(r *clientReader) auctionQuery {
	var q auctionQuery
	q.flags = r.u8()
	q.seller = r.str8(24)
	q.text = r.str16(64)
	q.classes, q.races = r.u32(), r.u32()
	q.minLevel, q.maxLevel = int(r.signed()), int(r.signed())
	q.minPrice, q.maxPrice = int(r.signed()), int(r.signed())
	q.wear, q.attack = int(r.signed()), int(r.signed())
	q.open = r.u8() != 0
	return q
}

// matches is the query's test of a lot. Ours where CLIENT says nothing of
// the server's: the levels are the item's requirement (0 counts as 1), a
// price is what the lot would cost now (the least next bid; once closed what
// it sold for, else its starting bid), the keywords a part of the item's
// name and the seller its whole name, any case; a class or race is one the
// item allows, and race NONE an item every race may use.
func (q *auctionQuery) matches(l *database.AuctionLot) bool {
	p := &l.Item.Pattern
	level := max(1, p.LevelReq)
	price := l.MinNext()
	if l.State != database.AuctionOpen {
		price = l.MinBid
		if l.State == database.AuctionSold {
			price = l.Price
		}
	}
	switch {
	case level < q.minLevel || level > q.maxLevel:
		return false
	case price < q.minPrice || (q.flags&16 != 0 && price > q.maxPrice):
		return false
	case q.flags&2 != 0 && p.EquipSlot != q.wear:
		return false
	case q.flags&1 != 0 && p.AttackType != q.attack:
		return false
	case q.flags&12 != 0 && q.text != "" && !strings.Contains(strings.ToLower(p.ItemName), strings.ToLower(q.text)):
		return false
	case q.seller != "" && !strings.EqualFold(q.seller, l.SellerName):
		return false
	case q.classes != 0xFFFFFFFF && p.ClassUse != 0 && uint32(p.ClassUse)&q.classes == 0:
		return false
	}
	races := uint32(raceUseForWire(p.RaceUse))
	switch q.races {
	case 0xFFFFFFFF:
		return true
	case 0:
		return races&uint32(allShippedRacesMask) == uint32(allShippedRacesMask)
	}
	return races&q.races != 0
}

// search is sub-op 7: a page of the lots the query matches, newest first;
// forward from a lot is the page after it, back the page before (the first
// page again at the start: CLIENT then says "Start of search results.").
func (a *AuctionHouse) search(sess *session.Session, player *entity.Player, page, from int, forward bool, q auctionQuery) {
	now := a.now()
	var all []database.AuctionLot
	var err error
	if q.open {
		all, err = a.db.AuctionOpenLots(now)
	} else {
		all, err = a.db.AuctionClosedLots(now, now.Add(-database.AuctionHistory))
	}
	if err != nil {
		log.Printf("Auction: %s's search: %v", player.Name, err)
	}
	var hits []database.AuctionLot
	for i := range all {
		if q.matches(&all[i]) {
			hits = append(hits, all[i])
		}
	}
	start := 0
	switch {
	case from < 0:
	case forward:
		start = len(hits)
		for i := range hits {
			if hits[i].ID < from {
				start = i
				break
			}
		}
	default:
		above := 0
		for i := range hits {
			if hits[i].ID > from {
				above++
			}
		}
		start = max(0, above-auctionPageSize)
	}
	lots := hits[start:min(len(hits), start+auctionPageSize)]
	a.sendPage(sess, page, lots)
	log.Printf("Auction: %s searched (open %v, %q), page %d from %d: %d of %d lots",
		player.Name, q.open, q.text, page, from, len(lots), len(hits))
}

// lotsByID is sub-op 8 (My Auctions, My Bids): those lots open, or closed
// within AuctionHistory, in the order asked.
func (a *AuctionHouse) lotsByID(sess *session.Session, player *entity.Player, page int, ids []int) {
	now := a.now()
	found, err := a.db.AuctionLotsByID(ids)
	if err != nil {
		log.Printf("Auction: %s's lots: %v", player.Name, err)
	}
	byID := map[int]database.AuctionLot{}
	since := now.Add(-database.AuctionHistory).Unix()
	for _, l := range found {
		if l.State == database.AuctionOpen || ((l.State == database.AuctionSold || l.State == database.AuctionUnsold) && l.ClosedAt >= since) {
			byID[l.ID] = l
		}
	}
	var lots []database.AuctionLot
	seen := map[int]bool{}
	for _, id := range ids {
		if l, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			lots = append(lots, l)
		}
	}
	a.sendPage(sess, page, lots)
}

// history is sub-op 9: a lot's bids (DrawRankings: name and bid).
func (a *AuctionHouse) history(sess *session.Session, player *entity.Player, lot int) {
	found, err := a.db.AuctionLotsByID([]int{lot})
	var l database.AuctionLot
	var bids []database.AuctionBid
	if err == nil && len(found) == 1 {
		l = found[0]
		bids, err = a.db.AuctionBidders(lot)
	}
	if err != nil {
		log.Printf("Auction: %s's history of %d: %v", player.Name, lot, err)
	}
	l.ID = lot
	queueReliable(sess, protocol.OpcodeAuctionStuff, biddersPayload(&l, bids))
}

// --- telling the others ---

// refundedLocked tells an outbid leader in the world: its bank's tunar, its
// record now holding nothing, the lot.
func (a *AuctionHouse) refundedLocked(rf database.AuctionRefund, lot *database.AuctionLot) {
	s, p := a.live(rf.Bidder)
	if s == nil {
		return
	}
	if !rf.Gone {
		p.BankTunar = rf.Bank
		queueReliable(s, protocol.OpcodeConfirmBankTunar, bankTunarPayload(rf.Bank))
	}
	queueReliable(s, protocol.OpcodeAuctionStuff, bidPayload(134, &rf.Bid))
	if lot != nil && lot.ID == rf.Bid.Lot {
		queueReliable(s, protocol.OpcodeAuctionStuff, updateLotPayload(lot, a.pageMinutes(s)))
	}
	a.tellLocked(s, rf.Bidder)
}

// settledLocked tells a closed lot's winner and seller in the world: their
// banks' tunar, the item filed into the bank it reached, the winner's record
// holding nothing now, the lot, the lines.
func (a *AuctionHouse) settledLocked(st database.AuctionSettlement) {
	l := &st.Lot
	if st.Winner != 0 {
		if s, p := a.live(st.Winner); s != nil {
			p.BankTunar = st.WinnerBank
			queueReliable(s, protocol.OpcodeConfirmBankTunar, bankTunarPayload(st.WinnerBank))
			queueReliable(s, protocol.OpcodeAuctionStuff, bidPayload(134, &st.WinnerBid))
			if st.Delivery != nil && st.Delivery.Holder == st.Winner {
				a.fileLocked(s, *st.Delivery)
			}
			queueReliable(s, protocol.OpcodeAuctionStuff, updateLotPayload(l, a.pageMinutes(s)))
			a.tellLocked(s, st.Winner)
		}
	}
	if s, p := a.live(l.Seller); s != nil && !st.SellerGone {
		p.BankTunar = st.SellerBank
		queueReliable(s, protocol.OpcodeConfirmBankTunar, bankTunarPayload(st.SellerBank))
		if st.Delivery != nil && st.Delivery.Holder == l.Seller {
			a.fileLocked(s, *st.Delivery)
		}
		queueReliable(s, protocol.OpcodeAuctionStuff, updateLotPayload(l, a.pageMinutes(s)))
		a.tellLocked(s, l.Seller)
	}
	log.Printf("Auction: lot %d (%s) closed %s: winner %d at %d, seller %d; item delivered %v",
		l.ID, l.Item.Pattern.ItemName, l.State, st.Winner, l.Price, l.Seller, l.Delivered)
}

// showLotLocked sends a lot's change to a character in the world.
func (a *AuctionHouse) showLotLocked(serverID uint32, lot *database.AuctionLot) {
	if s, _ := a.live(serverID); s != nil {
		queueReliable(s, protocol.OpcodeAuctionStuff, updateLotPayload(lot, a.pageMinutes(s)))
	}
}

// fileLocked files an item that reached a bank in the session's copy of the
// bank list and on its client (0x1251), as the client's AddItem keeps it.
// With bankMu held.
func (a *AuctionHouse) fileLocked(sess *session.Session, d database.AuctionDelivery) {
	bank := sessionBankList(sess)
	if bank == nil {
		return // no MemoryDump yet: the dump carries it
	}
	p := &d.Item.Pattern
	key, took := bank.add(d.Item.ItemID, p.PatternID, d.Item.StackLeft, p.MaxStack)
	if took != d.Item.StackLeft {
		log.Printf("Auction: lot %d's %d %q into a bank list that kept %d", d.Lot, d.Item.StackLeft, p.ItemName, took)
	}
	queueReliable(sess, protocol.OpcodeAddBankItem, bankAddPayload(&d.Item, key))
}

// tellLocked says a character's waiting lines to its session in the world,
// then forgets them; one not in the world yet hears them at its first 0x0014,
// one gone quiet (auctionQuiet) at its next login's.
func (a *AuctionHouse) tellLocked(sess *session.Session, serverID uint32) {
	if sess == nil || !sess.CharacterInWorld() || time.Since(sess.LastActivity()) > auctionQuiet {
		return
	}
	notes, err := a.db.AuctionNotices(serverID)
	if err != nil || len(notes) == 0 {
		return
	}
	ids := make([]int, 0, len(notes))
	for _, n := range notes {
		auctionLine(sess, n.Line)
		ids = append(ids, n.ID)
	}
	if err := a.db.AuctionNoticesDone(ids); err != nil {
		log.Printf("Auction: forgetting %d lines told to %d: %v", len(ids), serverID, err)
	}
}

// --- the sweep and the hooks ---

// Sweep closes the lots whose time is up (and the orphans) and tells whoever
// of theirs is in the world.
func (a *AuctionHouse) Sweep() {
	if a.db == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	sw, err := a.db.AuctionSettleDue(a.now())
	if err != nil {
		log.Printf("Auction: sweep: %v", err)
	}
	for _, rf := range sw.Refunds {
		a.refundedLocked(rf, nil)
	}
	for _, st := range sw.Settled {
		a.settledLocked(st)
	}
	for _, l := range sw.Cancelled {
		log.Printf("Auction: lot %d (%s) withdrawn: seller %d is gone", l.ID, l.Item.Pattern.ItemName, l.Seller)
	}
	if len(a.pageMin) > 256 && a.world != nil && a.world.SessionManager != nil {
		alive := map[*session.Session]bool{}
		a.world.SessionManager.ForEach(func(s *session.Session) bool {
			alive[s] = true
			return true
		})
		for s := range a.pageMin {
			if !alive[s] {
				delete(a.pageMin, s)
			}
		}
	}
}

// BeforeDump runs as a login's MemoryDump is made (SendMemoryDump): the bank's
// tunar read again (an auction may have paid into it after the character's
// row was read) and the items waiting at the house filed, so the dump carries
// both.
func (a *AuctionHouse) BeforeDump(sess *session.Session, player *entity.Player) {
	if a.db == nil || player == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	if !a.isLive(player) {
		return
	}
	if bank, ok, err := a.db.BankTunar(player.CharServerID); err == nil && ok {
		player.BankTunar = bank
	}
	if got, _, err := a.db.AuctionDeliverPending(player.CharServerID, a.now()); err != nil {
		log.Printf("Auction: %s's items at login: %v", player.Name, err)
	} else if len(got) > 0 {
		log.Printf("Auction: %s's %d items from the auction house filed at login", player.Name, len(got))
	}
}

// playerEntered is OnPlayerEntered: the bank's tunar again, what waits filed
// if there is room, and the lines it has not heard.
func (a *AuctionHouse) playerEntered(sess *session.Session, p *entity.Player) {
	if a.db == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	if !a.isLive(p) {
		return
	}
	if bank, ok, err := a.db.BankTunar(p.CharServerID); err == nil && ok && bank != p.BankTunar {
		p.BankTunar = bank
		queueReliable(sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(bank))
	}
	a.deliverLocked(sess, p)
	a.tellLocked(sess, p.CharServerID)
}

// bankOpened runs as a banker opens the bank: what waits at the house filed
// if there is room now.
func (a *AuctionHouse) bankOpened(sess *session.Session, p *entity.Player) {
	if a.db == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bankMu.Lock()
	defer bankMu.Unlock()
	if !a.isLive(p) {
		return
	}
	a.deliverLocked(sess, p)
	a.tellLocked(sess, p.CharServerID)
}

// deliverLocked files what waits for the player at the house, and says how
// much still waits.
func (a *AuctionHouse) deliverLocked(sess *session.Session, p *entity.Player) {
	got, left, err := a.db.AuctionDeliverPending(p.CharServerID, a.now())
	if err != nil {
		log.Printf("Auction: %s's waiting items: %v", p.Name, err)
	}
	for _, d := range got {
		a.fileLocked(sess, d)
	}
	if left > 0 && sess.CharacterInWorld() {
		auctionLine(sess, fmt.Sprintf("The auction house holds %d of your items until your bank has room for them.", left))
	}
}

// WriteDumpLists writes the MemoryDump's m_lots and m_bids (VIBankControl::
// Read, after the bank's list): the character's lots and bids open or closed
// within AuctionHistory, newest first. Without a house, two empty lists.
func (a *AuctionHouse) WriteDumpLists(w *buffer.Writer, serverID uint32) {
	var lots []database.AuctionLot
	var bids []database.AuctionBid
	if a != nil && a.db != nil {
		since := a.now().Add(-database.AuctionHistory)
		var err error
		if lots, err = a.db.AuctionLotsOf(serverID, since, auctionDumpLots); err != nil {
			log.Printf("Auction: the dump's lots of %d: %v", serverID, err)
		}
		if bids, err = a.db.AuctionBidsOf(serverID, since, auctionDumpBids); err != nil {
			log.Printf("Auction: the dump's bids of %d: %v", serverID, err)
		}
	}
	w.Write7BitEncodedInt64(int64(len(lots)))
	for i := range lots {
		writeLot(w, &lots[i], true)
	}
	sort.SliceStable(bids, func(i, j int) bool { return bids[i].Lot > bids[j].Lot })
	w.Write7BitEncodedInt64(int64(len(bids)))
	for i := range bids {
		writeBid(w, &bids[i])
	}
}

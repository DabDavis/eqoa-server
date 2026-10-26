package handlers

import (
	"log"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Trading between two players, as CLIENT runs it (VIClient's trade half and
// VIWndTrade, viwndtrade.cpp):
//
//   - 0x0057 {u32 target} (RequestTrade 0x00BDA028, SendOpU32(87)): A asks B.
//     B gets 0x0057 {u32 partner, u32 id} (Msg0057 0x00BDA1A8): the id every
//     0x0056 it sends and receives carries, and the entity it names the asker
//     by. Both are the asker, as the target's client knows it.
//   - Every other message is 0x0056 {u8 sub, u32 id, ...}. CLIENT drops one
//     whose id is not its partner's, but for subs 1, 2 and 20.
//     From the client: 1 cancel (TradeCancel), 2 yes (the asked side's
//     TradeSend2), 20 busy (Msg0057 outside play), 3 {u32 key, zz n} offer
//     n of the inventory record with that key (TradeSend3), 4 {u32 id} take
//     an offered entry back by its running id (TradeSend4), 5 {zz x} the
//     money offered (TradeSend5), 6 accept (TradeSend6), 7 unaccept
//     (TradeSend7).
//     To the client: 1 cancelled (Trade01), 2 {id} the other said yes
//     (Trade02: the asker sets its partner from it and opens the window), 16
//     done (Trade16), 17/21/22 refused (words 733/734/735), 20 busy
//     (Trade20, word 651), 6 the other accepted, 7 the accepts cleared, and
//     the offer changes. Those are numbered by WHOSE change it is: the
//     asker's are 3/4/5 to both sides, the asked side's 8/9/10 to both
//     (TradeA 0x00BDA528 reads 3 as its own slot {u32 key, zz n} and 8 as
//     the other's item, a whole record; TradeB 0x00BDA688 the other way
//     round). 4/9 carry the running id, 5/10 the sum (zz).
//
// The running ids are the client's: each side numbers its own new entries
// from 0 (TradeAddSlot, m_p2BAF8) and the other's the same way
// (TradeAddItem, m_p2BAFC); an offer onto an entry from the same inventory
// record joins it and takes no id, and a side holds four entries at most.
// The server keeps the same count so a 4/9 names the same entry on both.
//
// Any change clears both accepts (7 to both). When both have accepted, the
// items and the money move (0x003C off the giver, 0x003B onto the taker,
// 0x0052 each purse) and 16 closes both windows.

// tradeMax is the entries a side of a trade holds (m_tradeA/m_tradeB[4]).
const tradeMax = 4

// tradeOffer is one entry of a side's offer.
type tradeOffer struct {
	key uint32 // the inventory record's key
	n   int
	id  int // the running id
}

// tradeSide is one player's half of a trade.
type tradeSide struct {
	sess     *session.Session
	player   *entity.Player
	asker    bool
	offers   []tradeOffer
	nextID   int
	money    int
	accepted bool
}

// trade is a trade, asked for (open false) or under way.
type trade struct {
	a, b *tradeSide
	open bool
}

func (t *trade) other(s *tradeSide) *tradeSide {
	if s == t.a {
		return t.b
	}
	return t.a
}

// TradeHandler runs the trades.
type TradeHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	items          *NPCWindowHandler // giveItem, takeItem, saveTunar

	mu     sync.Mutex
	trades map[*session.Session]*trade
}

// NewTradeHandler creates the handler; items moves what is traded.
func NewTradeHandler(sessionMgr *session.Manager, items *NPCWindowHandler) *TradeHandler {
	return &TradeHandler{sessionManager: sessionMgr, items: items, trades: map[*session.Session]*trade{}}
}

// SetGameWorld sets the game world reference.
func (h *TradeHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers 0x0057 and 0x0056.
func (h *TradeHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeTradeRequest, h.HandleTradeRequest)
	r.Register(protocol.OpcodeTradeUI, h.HandleTradeUI)
}

// tradeSub is 0x0056 {u8 sub, u32 id} and what fill writes after it.
func tradeSub(sub byte, id uint32, fill func(w *buffer.Writer)) []byte {
	return build(protocol.OpcodeTradeUI, 1024, func(w *buffer.Writer) {
		w.WriteByte(sub)
		w.WriteUint32(id)
		if fill != nil {
			fill(w)
		}
	})
}

// seen is the id side s's client knows player p by (ids are per session).
func (h *TradeHandler) seen(s *tradeSide, p *entity.Player) uint32 {
	return h.gameWorld.IDSeenBy(s.sess, p)
}

func (h *TradeHandler) send(s *tradeSide, sub byte, id uint32, fill func(w *buffer.Writer)) {
	queueReliable(s.sess, protocol.OpcodeTradeUI, tradeSub(sub, id, fill))
}

// HandleTradeRequest is 0x0057 {u32 target}: the target is asked (0x0057 to
// it), or told the asker is busy when it is trading already.
func (h *TradeHandler) HandleTradeRequest(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	target, err := r.ReadUint32()
	if err != nil || h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	them := h.gameWorld.PlayerSeenBy(sess, target)
	if me == nil || them == nil || them == me {
		return nil
	}
	theirSess := h.gameWorld.FindSessionByEntityID(them.ServerID)
	if theirSess == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if old := h.trades[sess]; old != nil {
		h.endLocked(old)
	}
	a := &tradeSide{sess: sess, player: me, asker: true}
	if h.trades[theirSess] != nil {
		// Trade20: busy
		h.send(a, 20, h.seen(a, them), nil)
		return nil
	}
	b := &tradeSide{sess: theirSess, player: them}
	t := &trade{a: a, b: b}
	h.trades[sess] = t
	h.trades[theirSess] = t
	queueReliable(theirSess, protocol.OpcodeTradeRequest, build(protocol.OpcodeTradeRequest, 16, func(w *buffer.Writer) {
		id := h.gameWorld.IDSeenBy(theirSess, me)
		w.WriteUint32(id)
		w.WriteUint32(id)
	}))
	log.Printf("Trade: %s asks %s", me.Name, them.Name)
	return nil
}

// endLocked forgets a trade.
func (h *TradeHandler) endLocked(t *trade) {
	if h.trades[t.a.sess] == t {
		delete(h.trades, t.a.sess)
	}
	if h.trades[t.b.sess] == t {
		delete(h.trades, t.b.sess)
	}
}

// HandleTradeUI is 0x0056 {u8 sub, u32 id, ...} from either side.
func (h *TradeHandler) HandleTradeUI(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, e1 := r.ReadByte()
	_, e2 := r.ReadUint32()
	if e1 != nil || e2 != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.trades[sess]
	if t == nil {
		return nil
	}
	me := t.a
	if t.b.sess == sess {
		me = t.b
	}
	you := t.other(me)
	switch sub {
	case 1: // cancel
		h.send(you, 1, h.seen(you, me.player), nil)
		h.endLocked(t)
		log.Printf("Trade: %s cancelled", me.player.Name)
	case 20: // busy
		h.send(you, 20, h.seen(you, me.player), nil)
		h.endLocked(t)
	case 2: // the asked side said yes
		if me.asker || t.open {
			return nil
		}
		t.open = true
		h.send(you, 2, h.seen(you, me.player), nil)
		log.Printf("Trade: %s and %s trade", t.a.player.Name, t.b.player.Name)
	case 3:
		key, ea := r.ReadUint32()
		n, eb := r.Read7BitEncodedInt64()
		if ea != nil || eb != nil || !t.open {
			return nil
		}
		h.offerLocked(t, me, you, key, int(n))
	case 4:
		id, ea := r.ReadUint32()
		if ea != nil || !t.open {
			return nil
		}
		h.withdrawLocked(t, me, you, int(id))
	case 5:
		x, ea := r.Read7BitEncodedInt64()
		if ea != nil || !t.open || x < 0 || int(x) > me.player.Tunar {
			return nil
		}
		me.money = int(x)
		sub := byte(10)
		if me.asker {
			sub = 5
		}
		h.send(me, sub, h.seen(me, you.player), func(w *buffer.Writer) { w.Write7BitEncodedInt64(x) })
		h.send(you, sub, h.seen(you, me.player), func(w *buffer.Writer) { w.Write7BitEncodedInt64(x) })
		h.clearAcceptsLocked(t)
	case 6:
		if !t.open {
			return nil
		}
		me.accepted = true
		h.send(you, 6, h.seen(you, me.player), nil)
		if you.accepted {
			h.completeLocked(t)
		}
	case 7:
		me.accepted = false
		h.send(you, 7, h.seen(you, me.player), nil)
	}
	return nil
}

// clearAcceptsLocked is a change: both accepts cleared, 7 to both.
func (h *TradeHandler) clearAcceptsLocked(t *trade) {
	t.a.accepted = false
	t.b.accepted = false
	h.send(t.a, 7, h.seen(t.a, t.b.player), nil)
	h.send(t.b, 7, h.seen(t.b, t.a.player), nil)
}

// offered is how many of the record with that key the side has offered.
func (s *tradeSide) offered(key uint32) int {
	for _, o := range s.offers {
		if o.key == key {
			return o.n
		}
	}
	return 0
}

// offerLocked is sub 3: n of me's record `key` onto its offer, as
// TradeAddSlot files it, echoed to me as its own slot and to the other as
// the whole record.
func (h *TradeHandler) offerLocked(t *trade, me, you *tradeSide, key uint32, n int) {
	inv := sessionInvList(me.sess)
	if inv == nil || h.items == nil || h.items.inv == nil || n < 1 {
		return
	}
	rec, ok := inv.at(inv.index(key))
	if !ok || rec.qty-me.offered(key) < n {
		return
	}
	row, err := h.items.inv.GetCharItem(me.player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("Trade: %s offers key %d: %v", me.player.Name, key, err)
		return
	}
	if row.EquipLoc != -1 {
		log.Printf("Trade: %s offers %q while wearing it", me.player.Name, row.Pattern.ItemName)
		return
	}
	if row.Pattern.Trade > 0 {
		// itempattern.trade is CLIENT's m_noTrade (VIItem +0x1C): its info
		// window says NO TRADE, and such an item stays its owner's
		log.Printf("Trade: %s offers %q, which is no trade", me.player.Name, row.Pattern.ItemName)
		return
	}
	at := -1
	for i, o := range me.offers {
		if o.key == key {
			at = i
		}
	}
	if at == -1 {
		if len(me.offers) == tradeMax {
			return
		}
		me.offers = append(me.offers, tradeOffer{key: key, n: n, id: me.nextID})
		me.nextID++
	} else {
		me.offers[at].n += n
	}
	sub := byte(8)
	if me.asker {
		sub = 3
	}
	h.send(me, sub, h.seen(me, you.player), func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.Write7BitEncodedInt64(int64(n))
	})
	out := wireRecord(row)
	out.StackLeft = n
	h.send(you, sub, h.seen(you, me.player), func(w *buffer.Writer) { writeSingleItem(w, &out, int(key)) })
	h.clearAcceptsLocked(t)
	log.Printf("Trade: %s offers %d %q", me.player.Name, n, row.Pattern.ItemName)
}

// withdrawLocked is sub 4: me's entry with running id `id` taken back.
func (h *TradeHandler) withdrawLocked(t *trade, me, you *tradeSide, id int) {
	at := -1
	for i, o := range me.offers {
		if o.id == id {
			at = i
		}
	}
	if at == -1 {
		return
	}
	me.offers = append(me.offers[:at], me.offers[at+1:]...)
	sub := byte(9)
	if me.asker {
		sub = 4
	}
	h.send(me, sub, h.seen(me, you.player), func(w *buffer.Writer) { w.WriteUint32(uint32(id)) })
	h.send(you, sub, h.seen(you, me.player), func(w *buffer.Writer) { w.WriteUint32(uint32(id)) })
	h.clearAcceptsLocked(t)
}

// completeLocked moves both offers and both sums, then closes both windows
// (16). An offer the giver no longer holds, or the taker has no room for,
// refuses the trade (17 to both, the accepts cleared) before anything moves.
func (h *TradeHandler) completeLocked(t *trade) {
	type move struct {
		from, to *tradeSide
		key      uint32
		n        int
		row      database.CharInventoryEntry
	}
	var moves []move
	for _, s := range []*tradeSide{t.a, t.b} {
		inv := sessionInvList(s.sess)
		for _, o := range s.offers {
			rec, ok := inv.at(inv.index(o.key))
			if !ok || rec.qty < o.n {
				h.refuseLocked(t)
				return
			}
			row, err := h.items.inv.GetCharItem(s.player.CharServerID, rec.itemID)
			if err != nil || row.EquipLoc != -1 {
				h.refuseLocked(t)
				return
			}
			moves = append(moves, move{from: s, to: t.other(s), key: o.key, n: o.n, row: row})
		}
		if s.money > s.player.Tunar {
			h.refuseLocked(t)
			return
		}
	}
	for _, m := range moves {
		toInv := sessionInvList(m.to.sess)
		if toInv.wouldAdd(m.row.Pattern.PatternID, m.n, m.row.Pattern.MaxStack) < m.n {
			h.refuseLocked(t)
			return
		}
	}
	for _, m := range moves {
		fromInv := sessionInvList(m.from.sess)
		toInv := sessionInvList(m.to.sess)
		i := fromInv.index(m.key)
		rec, _ := fromInv.at(i)
		if err := h.items.takeItem(m.from.player, fromInv, i, rec, m.n); err != nil {
			log.Printf("Trade: taking from %s: %v", m.from.player.Name, err)
			continue
		}
		queueReliable(m.from.sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(i, 0, m.n))
		out, key, err := h.items.giveItem(m.to.player, toInv, m.row, m.n, 0)
		if err != nil {
			log.Printf("Trade: giving to %s: %v", m.to.player.Name, err)
			continue
		}
		queueReliable(m.to.sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, key))
		log.Printf("Trade: %d %q from %s to %s", m.n, m.row.Pattern.ItemName, m.from.player.Name, m.to.player.Name)
	}
	if t.a.money != 0 || t.b.money != 0 {
		for _, s := range []*tradeSide{t.a, t.b} {
			s.player.Tunar += t.other(s).money - s.money
			h.items.saveTunar(s.player)
			queueReliable(s.sess, protocol.OpcodePlayerTunar, tunarPayload(s.player.Tunar))
		}
	}
	h.send(t.a, 16, h.seen(t.a, t.b.player), nil)
	h.send(t.b, 16, h.seen(t.b, t.a.player), nil)
	h.endLocked(t)
}

// refuseLocked is 17 to both: the trade stays, the accepts cleared.
func (h *TradeHandler) refuseLocked(t *trade) {
	t.a.accepted = false
	t.b.accepted = false
	h.send(t.a, 17, h.seen(t.a, t.b.player), nil)
	h.send(t.b, 17, h.seen(t.b, t.a.player), nil)
	log.Printf("Trade: %s and %s refused", t.a.player.Name, t.b.player.Name)
}

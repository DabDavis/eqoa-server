package handlers

import (
	"fmt"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// merctrade.go — a trade with one's own mercenary through the ordinary trade
// window (Heirloom petition 3306, a player: "could you make it to where you
// just send a trade to a merc to trade them items?"; the owner approved it
// 2026-11-27). Until then a merc took items only through its own menu, /merc
// give and vanilla's merc window (mercinv.go, mercwin.go).
//
// Which clients can ask. CLIENT's RequestTrade (0x00BDA028, viclient.cpp)
// sends 0x0057 only for a creature whose m_p256 is 2, a player's:
//
//	VIEntity *e = FindEntity(m_target);
//	if (!e || !e->IsType(130)) return;
//	if (((VICreatureEntity *)e)->m_p256 != 2) return;
//
// and a merc goes out with m_p256 3, a pet's (statesync.go, buf[142]: every
// NPC with an Owner), which its talk, its name plate and the PvP gate read as
// one of a player's side; sending it as 2 would make a merc's Square a
// player's (no 0x0004: its menu gone) and its plate a player's. So the PS2
// never asks a merc to trade and stays as it is; vanilla's request_trade
// (npcw.rs) asks its own merc too (one whose hirer 0x7E5B names as the
// player: Play::own_merc), and this file answers it.
//
// The trade, the player always the asker:
//
//   - 0x0057 naming the player's own merc, out and alive and within a talk's
//     reach (as a trade with a player): the window opens at once (0x0056 sub
//     2, the merc's yes, under the id the player's client gave), and the merc
//     shows its accept (6): it offers nothing and takes what it is given.
//   - Offers (3) and withdrawals (4) are a player trade's: the same running
//     ids, four entries, the player's own slot echoed (sub 3/4); each change
//     clears the accepts (7) and the merc accepts again (6). An offer the
//     merc's give would refuse (giveRefusal: a worn item, a No Rent item that
//     vanishes at logout) is refused here, its line said, and not filed. A
//     NO TRADE or LORE item may go, as the merc's give lets it: it never
//     leaves its owner (it comes back by Take back).
//   - Tunar (5): a merc holds no purse, so the sum is refused (said, and the
//     player's field set back to 0).
//   - The player's accept (6) moves every offer onto the merc, each through
//     the merc's own give (giveN: one transaction a record, 0x003C to the
//     client), once the offers are all still in the pack and the merc can
//     take them all together (roomFor); else 17 (refused, word 733) and the
//     trade stays, its accepts cleared. Then 16 closes the window, the merc's
//     kit and window made again, and a line names each item given.
//   - Cancel (1) or busy (20) forgets it, nothing moved.
//
// Refused with tradeRefusedLine (red, as the out-of-reach line) and the
// player's ask called off (sub 1, so its client asks nothing more): another
// player's merc, a pet, any other NPC, a merc dead or not out, mercs turned
// off. A restart calls a merc trade off as it does a player trade
// (keep.go's Trading).

// tradeRefusedLine is said to a player whose trade target is no player and no
// merc of his out (ours, in the out-of-reach line's words).
const tradeRefusedLine = "You can not trade with your target."

// mercNoTunarLine refuses tunar offered to a merc (ours).
const mercNoTunarLine = "%s takes no tunar."

// mercTrade is a player's trade with his own merc.
type mercTrade struct {
	sess   *session.Session
	player *entity.Player
	slot   int             // the merc's hire's
	npc    entity.EntityID // the merc's entity
	seen   uint32          // the id the player's client knows the merc by
	name   string
	offers []tradeOffer
	nextID int
}

// tradingLocked is whether the session is in a trade, asked or open, with a
// player or its merc. Must be called with h.mu held.
func (h *TradeHandler) tradingLocked(sess *session.Session) bool {
	return h.trades[sess] != nil || h.mercTrades[sess] != nil
}

// trading is tradingLocked under the lock.
func (h *TradeHandler) trading(sess *session.Session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tradingLocked(sess)
}

// refuseAsk is the line refusing a trade asked of target, and the ask called
// off on the asker's client (sub 1, which CLIENT and vanilla take whatever its
// id).
func (h *TradeHandler) refuseAsk(sess *session.Session, target uint32) {
	SendColoredChat(sess, tradeRefusedLine, 255, 63, 63)
	queueReliable(sess, protocol.OpcodeTradeUI, tradeSub(1, target, nil))
}

// mercTradeRequest is 0x0057 naming no player: a trade with the asker's own
// merc (the file's doc), or refused.
func (h *TradeHandler) mercTradeRequest(sess *session.Session, me *entity.Player, target uint32) {
	e, ok := h.gameWorld.FindEntityByObjectID(sess, target)
	npc, isNPC := e.(*entity.NPC)
	if !ok || !isNPC || npc == nil || h.mercs == nil || !MercsOn() {
		log.Printf("Trade: %s asks %08X, no player and no merc of his", me.Name, target)
		h.refuseAsk(sess, target)
		return
	}
	slot, own := h.gameWorld.MercOwnSlot(sess, npc.ServerID) // his, out and alive
	if !own {
		log.Printf("Trade: %s asks %q, which is no merc of his out", me.Name, npc.Name)
		h.refuseAsk(sess, target)
		return
	}
	if !entity.InRange(me, npc, entity.InteractRange) {
		SendColoredChat(sess, tradeOutOfReachLine, 255, 63, 63)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.endAllLocked(sess)
	mt := &mercTrade{sess: sess, player: me, slot: slot, npc: npc.ServerID, seen: target, name: npc.Name}
	h.mercTrades[sess] = mt
	h.mercSend(mt, 2, nil) // its yes: the window opens
	h.mercSend(mt, 6, nil) // and its accept
	log.Printf("Trade: %s trades with his merc %q (slot %d)", me.Name, mt.name, slot)
}

// endAllLocked forgets whatever trade the session is in, as a new ask does.
// Must be called with h.mu held.
func (h *TradeHandler) endAllLocked(sess *session.Session) {
	if old := h.trades[sess]; old != nil {
		h.endLocked(old)
	}
	delete(h.mercTrades, sess)
}

// mercSend is 0x0056 sub to the player, under the merc's id.
func (h *TradeHandler) mercSend(mt *mercTrade, sub byte, fill func(w *buffer.Writer)) {
	queueReliable(mt.sess, protocol.OpcodeTradeUI, tradeSub(sub, mt.seen, fill))
}

// mercChangedLocked is a change: the accepts cleared (7), the merc's given
// again (6).
func (h *TradeHandler) mercChangedLocked(mt *mercTrade) {
	h.mercSend(mt, 7, nil)
	h.mercSend(mt, 6, nil)
}

// mercTradeUILocked is 0x0056 from a player trading with his merc. Must be
// called with h.mu held.
func (h *TradeHandler) mercTradeUILocked(mt *mercTrade, sub byte, r *buffer.Reader) {
	if h.gameWorld != nil && h.gameWorld.SessionPlayer(mt.sess) != mt.player {
		delete(h.mercTrades, mt.sess) // taken over, or gone (heldLocked)
		return
	}
	switch sub {
	case 1, 20: // cancel, busy: nothing moved
		delete(h.mercTrades, mt.sess)
		log.Printf("Trade: %s called off the trade with his merc %q", mt.player.Name, mt.name)
	case 3:
		key, ea := r.ReadUint32()
		n, eb := r.Read7BitEncodedInt64()
		if ea != nil || eb != nil {
			return
		}
		h.mercOfferLocked(mt, key, int(n))
	case 4:
		id, ea := r.ReadUint32()
		if ea != nil {
			return
		}
		at := -1
		for i, o := range mt.offers {
			if o.id == int(id) {
				at = i
			}
		}
		if at == -1 {
			return
		}
		mt.offers = append(mt.offers[:at], mt.offers[at+1:]...)
		h.mercSend(mt, 4, func(w *buffer.Writer) { w.WriteUint32(id) })
		h.mercChangedLocked(mt)
	case 5:
		x, ea := r.Read7BitEncodedInt64()
		if ea != nil || x == 0 {
			return
		}
		SendErrorMessage(mt.sess, fmt.Sprintf(mercNoTunarLine, mt.name))
		h.mercSend(mt, 5, func(w *buffer.Writer) { w.Write7BitEncodedInt64(0) })
		h.mercChangedLocked(mt)
	case 6:
		h.mercCompleteLocked(mt)
	}
}

// mercOfferLocked is sub 3: n of the player's record `key` onto his offer, as
// offerLocked files a player's, unless the merc's give refuses it.
func (h *TradeHandler) mercOfferLocked(mt *mercTrade, key uint32, n int) {
	inv := sessionInvList(mt.sess)
	if inv == nil || h.items == nil || h.items.inv == nil || n < 1 {
		return
	}
	rec, ok := inv.at(inv.index(key))
	if !ok || rec.qty-mtOffered(mt, key) < n {
		return
	}
	row, err := h.items.inv.GetCharItem(mt.player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("Trade: %s offers key %d to his merc: %v", mt.player.Name, key, err)
		return
	}
	if line := h.mercs.giveRefusal(row); line != "" {
		SendErrorMessage(mt.sess, line)
		return
	}
	at := -1
	for i, o := range mt.offers {
		if o.key == key {
			at = i
		}
	}
	if at == -1 {
		if len(mt.offers) == tradeMax {
			return
		}
		mt.offers = append(mt.offers, tradeOffer{key: key, n: n, id: mt.nextID})
		mt.nextID++
	} else {
		mt.offers[at].n += n
	}
	h.mercSend(mt, 3, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.Write7BitEncodedInt64(int64(n))
	})
	h.mercChangedLocked(mt)
	log.Printf("Trade: %s offers %d %q to his merc %q", mt.player.Name, n, row.Pattern.ItemName, mt.name)
}

// mtOffered is how many of the record with that key the player has offered.
func mtOffered(mt *mercTrade, key uint32) int {
	for _, o := range mt.offers {
		if o.key == key {
			return o.n
		}
	}
	return 0
}

// mercRefuseLocked is 17 (word 733): the trade stays, the accepts cleared,
// the merc's given again.
func (h *TradeHandler) mercRefuseLocked(mt *mercTrade, line string) {
	if line != "" {
		SendErrorMessage(mt.sess, line)
	}
	h.mercSend(mt, 17, nil)
	h.mercSend(mt, 6, nil)
	log.Printf("Trade: %s's trade with his merc %q refused: %s", mt.player.Name, mt.name, line)
}

// mercCompleteLocked is the player's accept: every offer onto the merc, then
// 16 (the file's doc).
func (h *TradeHandler) mercCompleteLocked(mt *mercTrade) {
	if slot, own := h.gameWorld.MercOwnSlot(mt.sess, mt.npc); !own || slot != mt.slot {
		// it died, or went, while the window was open
		SendColoredChat(mt.sess, tradeRefusedLine, 255, 63, 63)
		h.mercSend(mt, 1, nil)
		delete(h.mercTrades, mt.sess)
		return
	}
	merc, ok, err := h.mercs.db.Merc(mt.player.CharServerID, mt.slot)
	if err != nil || !ok {
		h.mercRefuseLocked(mt, mercNoneLine)
		return
	}
	inv := sessionInvList(mt.sess)
	type move struct {
		key uint32
		n   int
		row database.CharInventoryEntry
	}
	var moves []move
	var gives []invGive
	for _, o := range mt.offers {
		rec, ok := inv.at(inv.index(o.key))
		if !ok || rec.qty < o.n {
			h.mercRefuseLocked(mt, "That is no longer in your pack.")
			return
		}
		row, err := h.items.inv.GetCharItem(mt.player.CharServerID, rec.itemID)
		if err != nil {
			h.mercRefuseLocked(mt, "That is no longer in your pack.")
			return
		}
		if line := h.mercs.giveRefusal(row); line != "" {
			h.mercRefuseLocked(mt, line)
			return
		}
		moves = append(moves, move{key: o.key, n: o.n, row: row})
		gives = append(gives, invGive{patternID: row.Pattern.PatternID, n: o.n, maxStack: row.Pattern.MaxStack})
	}
	if !h.mercs.roomFor(mt.player, merc, gives) {
		h.mercRefuseLocked(mt, fmt.Sprintf(mercGiveFull, merc.Name, database.MercItemsMax))
		return
	}
	gave := 0
	for _, m := range moves {
		// the index again for each: a record given whole leaves the list
		i := inv.index(m.key)
		if line := h.mercs.giveN(mt.sess, mt.player, merc, inv, i, m.row, m.n); line != "" {
			SendErrorMessage(mt.sess, line) // a race with the check above: what went stays the merc's
			continue
		}
		gave++
		name := m.row.Pattern.ItemName
		if m.n > 1 {
			name = fmt.Sprintf("%d %s", m.n, name)
		}
		SendClientMessage(mt.sess, fmt.Sprintf(mercGiveLine, name, merc.Name))
	}
	h.mercSend(mt, 16, nil)
	delete(h.mercTrades, mt.sess)
	if gave > 0 {
		h.mercs.rekitSlot(mt.player, mt.slot)
		h.mercs.pushWindow(mt.sess, mt.player, mt.slot)
	}
	log.Printf("Trade: %s gave his merc %q %d of %d offers", mt.player.Name, merc.Name, gave, len(moves))
}

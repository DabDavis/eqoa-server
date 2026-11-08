package handlers

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// buyback.go: a merchant sells back what its customer last got rid of (the user, 2026-11-07, after a
// player destroyed an item by accident: "lets give it back and add a buy back to merchants for last 5
// items"; the entries: database/buyback.go). No client change: 0x00B7's list is the server's to fill.
//
// What CLIENT does with the list, read off the decomp:
//
//   - VIItemList::Read writes every record the count names into ItemInternal[40] with no check, so
//     more than 40 overruns the client's memory. The wares come first, and a line goes in only where
//     there is room, which nearly every merchant has for all five.
//   - A request names a record by its place in the list, so lines AFTER the wares leave every ware's
//     index what it always was, and the list still opens on the wares (anyone who has sold five things
//     has five lines at every merchant from then on, and six rows show at a time). VIWndList::Scroll
//     wraps, so Up from the first row is the last row: the newest line goes last, one press away, and
//     the greeting says so.
//   - A record is the client's by value: nothing keys a list's item by its id, so what a line's record
//     says is that line's alone. 0x003B then files the real record. A row shows only the icon; the name
//     and description show in the panels when it is selected, and that is where a line says what it is.
//     VIItem's m_name holds 63 characters and m_desc 255: ReadString keeps one longer as an EMPTY string.
//   - GetMerchantPrice is the record's value at the buy rate, so a line's value is its whole price. The
//     row picked opens VIWndQuantity for a record's m_maxStack and sends the count, so a line says 1: the
//     box shows the price and is the confirmation, and the whole entry comes back whatever count is sent.
//   - VIClient::MerchantOpen (and vanilla's 0x00B7) opens the window only from the game's mode 0: a
//     0x00B7 with the window up replaces the list and the rest in place, with no sound and no return
//     to the menu, and the window redraws from it every frame. So a sale shows its line and a buy-back
//     drops its line at once. It goes ahead of the answer that clears the request-pending flag, so
//     CLIENT cannot send another request off the old list. (A window closed in the round trip of the
//     request opens again on it, as 0x00B7 asks: two presses inside a round trip, and one more
//     closes it.)
//   - A record of RemainHP 0 is a broken item, which catalogRecord would show as new: a line keeps what went.

// merchantWindow is what the session's client was last sent as 0x00B7. A request's index is into that
// list; the entries it names stay what they were sent as, whatever has happened to the table since.
type merchantWindow struct {
	id       uint32  // the id it came under, which each request hands back
	greeting string  // the window's own greeting, before what is said of the lines
	wares    int     // the wares leading the list
	lines    []int64 // the buy-back entries after them: lines[i] at list index wares+i

	mu       sync.Mutex
	served   int       // the list index of the line last bought back
	servedAt time.Time // and when
}

// buyBackRepeat is how long after a line is bought back a request for the same index is taken as a
// copy of that one, though the line after it has moved into its place. A real copy (the client resends
// what it has not seen acknowledged) is stopped before any handler by its number and contents
// (session/reliablecopies.go), and CLIENT sends no second request until the first is answered; this is
// for what got past both. A player needs the round trip and two presses of the button (the row, then the
// quantity box) to buy that next line.
const buyBackRepeat = 500 * time.Millisecond

// sessionMerchantWindow is the note of the list last sent, or nil.
func sessionMerchantWindow(sess *session.Session) *merchantWindow {
	w, _ := sess.MerchantWindow().(*merchantWindow)
	return w
}

// repeated is whether a request for this line index is a copy of the one last served.
func (w *merchantWindow) repeated(index int, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.servedAt.IsZero() && w.served == index && now.Sub(w.servedAt) < buyBackRepeat
}

// serve notes the line index just bought back.
func (w *merchantWindow) serve(index int, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.served, w.servedAt = index, now
}

// buyBackMark begins a line's name.
const buyBackMark = "Buy back: "

// VIItem's strings: what ReadString keeps, a longer one coming out empty.
const (
	itemNameMax = 63
	itemDescMax = 255
	// greetingMax is m_merchantName's
	greetingMax = 127
)

// What the greeting says of the lines, and the refusals, in the merchant's voice.
const (
	buyBackAtTheEnd  = "What you sold or destroyed last is at the end of my list: press up."
	buyBackSomeAtEnd = "The last of what you sold or destroyed is at the end of my list: press up."
	buyBackNoShelf   = "My list is full: any other merchant can sell back what you sold or destroyed."
	buyBackNoRoom    = "You have no room in your pack for all of that."
	buyBackNoTunar   = "You do not have the tunar to buy that back."
	buyBackNotHere   = "That is no longer here to buy back."
)

// cutRunes is s in at most n characters.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// buyBackRecord is an entry as a line of the merchant's list (the notes above).
func buyBackRecord(e *database.BuyBackEntry) database.CharInventoryEntry {
	rec := wireRecord(e.Item)
	p := &rec.Pattern
	name := p.ItemName
	if e.Item.StackLeft > 1 {
		name = fmt.Sprintf("%d %s", e.Item.StackLeft, name)
	}
	p.ItemName = cutRunes(buyBackMark+name, itemNameMax)
	p.ItemDesc = cutRunes(buyBackWords(e)+" "+p.ItemDesc, itemDescMax)
	p.ItemCost = e.Price
	p.MaxStack = 1
	return rec
}

// buyBackWords is what a line's description begins with.
func buyBackWords(e *database.BuyBackEntry) string {
	this, it, comes := "this", "it", "it comes"
	if n := e.Item.StackLeft; n > 1 {
		this, it, comes = fmt.Sprintf("these %d", n), "them all", "they all come"
	}
	var s string
	switch {
	case e.How == database.BuyBackSold && e.Price > 0:
		s = fmt.Sprintf("You sold %s for %d tunar; the same brings %s back.", this, e.Price, it)
	case e.How == database.BuyBackSold:
		s = fmt.Sprintf("You sold %s for nothing; %s back for nothing.", this, comes)
	case e.Price > 0:
		s = fmt.Sprintf("You destroyed %s; %d tunar brings %s back.", this, e.Price, it)
	default:
		s = fmt.Sprintf("You destroyed %s; %s back for nothing.", this, comes)
	}
	if max := e.Item.Pattern.MaxHP; max > 0 && e.Item.RemainHP < max {
		s += fmt.Sprintf(" Condition %d of %d.", e.Item.RemainHP, max)
	}
	return s
}

// merchantList is what a merchant's window lists for a player.
type merchantList struct {
	wares []database.CharInventoryEntry
	lines []database.BuyBackEntry // in list order, the newest last
	held  int                     // the entries the player has, shown or not
}

// merchantList reads npc's catalog and the player's entries, as many as the client's list has room
// for after the wares, the newest kept.
func (h *NPCWindowHandler) merchantList(player *entity.Player, npc *entity.NPC) merchantList {
	var l merchantList
	if h.inv == nil {
		return l
	}
	var err error
	if l.wares, err = h.waresFor(player, npc); err != nil {
		log.Printf("Merchant: catalog of %q (npc %d): %v", npc.Name, npc.NPCID, err)
	}
	for i := range l.wares { // the buyer's own view: an heirloom's figures (heirloom.go)
		h.inv.ViewPattern(player.CharServerID, &l.wares[i].Pattern)
	}
	entries, err := h.inv.BuyBacks(player.CharServerID)
	if err != nil {
		log.Printf("BuyBack: %s's entries: %v", player.Name, err)
		return l
	}
	l.held = len(entries)
	room := invListCap - len(l.wares)
	if len(entries) > room {
		entries = entries[:room]
	}
	for i := len(entries) - 1; i >= 0; i-- {
		l.lines = append(l.lines, entries[i])
	}
	return l
}

// waresFor is npc's wares as player's list shows them, in its order: an
// heirloom shop's the buyer's class's alone, and no more than the client's 40.
// A buy's catalog index is into this list (merchant.go's buy), so the two may
// never differ: until 2026-11-08 the buy indexed the whole catalog and gave
// another class's item (the user: "im buying staff of faykind with Birdie but is
// giving me wrong items": Heirloom Bracers twice).
func (h *NPCWindowHandler) waresFor(player *entity.Player, npc *entity.NPC) ([]database.CharInventoryEntry, error) {
	wares, err := h.inv.GetNPCItems(npc.NPCID)
	if err != nil {
		return nil, err
	}
	if classShop(npc) { // the buyer's class's alone, before the client's 40
		wares = waresForClass(wares, int(player.Class))
	}
	if len(wares) > invListCap {
		wares = wares[:invListCap]
	}
	return wares, nil
}

// heirloomOutfitterName is the Heirloom Outfitters' name (genie.go's shop
// query names them too).
const heirloomOutfitterName = "Heirloom Outfitter"

// heirloomShop reports a merchant whose stock is heirlooms: an Heirloom
// Outfitter, or a genie selling one's (genie.go). Its list is the buyer's
// class's alone (waresForClass): every outfitter stocks every class's set
// (the user, 2026-11-08: a kappa enchanter found none near its start), and
// the client's list holds 40, which one class's set with the items for every
// class keeps within (37 at most).
func heirloomShop(npc *entity.NPC) bool {
	return npc != nil && (npc.Name == heirloomOutfitterName || npc.Name == entity.GenieName)
}

// SpellArchivistName is the Spell Archivists' name: a merchant whose stock is
// every class's spell scrolls past the class vendors' level 49, its list the
// buyer's class's alone (the Heirloom Test Server's; Destiny's petition 1273,
// 2026-11-08: "55+ spell vendors not in game. this is issue for us wanting to
// test various stuff").
const SpellArchivistName = "Spell Archivist"

// classShop reports a merchant whose list is the buyer's class's alone
// (waresForClass): an heirloom shop, or a Spell Archivist.
func classShop(npc *entity.NPC) bool {
	return heirloomShop(npc) || (npc != nil && npc.Name == SpellArchivistName)
}

// waresForClass is the wares a class can use (classuse), in their order.
func waresForClass(wares []database.CharInventoryEntry, class int) []database.CharInventoryEntry {
	out := wares[:0:0]
	for _, w := range wares {
		if class >= 0 && w.Pattern.ClassUse&(1<<uint(class)) != 0 {
			out = append(out, w)
		}
	}
	return out
}

// greetingWith is a window's greeting with what is said of the lines.
func (l *merchantList) greetingWith(greeting string) string {
	var more string
	switch {
	case l.held == 0:
		return greeting
	case len(l.lines) == 0:
		more = buyBackNoShelf
	case len(l.lines) < l.held:
		more = buyBackSomeAtEnd
	default:
		more = buyBackAtTheEnd
	}
	if s := greeting + " " + more; len([]rune(s)) <= greetingMax {
		return s
	}
	return more
}

// sendMerchantList sends the list as 0x00B7 under id and notes it. prev is the note it replaces, for
// the line last served.
func (h *NPCWindowHandler) sendMerchantList(sess *session.Session, id uint32, greeting string, l merchantList, prev *merchantWindow) {
	w := &merchantWindow{id: id, greeting: greeting, wares: len(l.wares)}
	lines := make([]database.CharInventoryEntry, len(l.lines))
	for i := range l.lines {
		lines[i] = buyBackRecord(&l.lines[i])
		w.lines = append(w.lines, l.lines[i].ID)
	}
	if prev != nil {
		prev.mu.Lock()
		w.served, w.servedAt = prev.served, prev.servedAt
		prev.mu.Unlock()
	}
	sess.SetMerchantWindow(w)
	queueReliable(sess, protocol.OpcodeMerchantBox, merchantBoxPayload(id, l.wares, l.greetingWith(greeting), lines...))
}

// refreshMerchant sends the list again in place when the session's window is on this merchant and
// what it lists has changed. It comes before the answer to the request that changed it (the notes above).
func (h *NPCWindowHandler) refreshMerchant(sess *session.Session, player *entity.Player, npc *entity.NPC, id uint32) {
	w := sessionMerchantWindow(sess)
	if w == nil || w.id != id {
		return
	}
	l := h.merchantList(player, npc)
	same := w.wares == len(l.wares) && len(w.lines) == len(l.lines)
	for i := 0; same && i < len(l.lines); i++ {
		same = w.lines[i] == l.lines[i].ID
	}
	if !same {
		h.sendMerchantList(sess, id, w.greeting, l, w)
	}
}

// buyBack sells back the line at list index index of the list last sent: all of it, as it went, or
// nothing.
func (h *NPCWindowHandler) buyBack(sess *session.Session, player *entity.Player, npc *entity.NPC, inv *invList, w *merchantWindow, id uint32, index int) bool {
	slot := index - w.wares
	if slot >= len(w.lines) {
		log.Printf("BuyBack: %s asked %q for item %d of %d + %d", player.Name, npc.Name, index, w.wares, len(w.lines))
		return false
	}
	now := time.Now()
	if w.repeated(index, now) {
		log.Printf("BuyBack: %s asked %q for item %d again at once, not taken", player.Name, npc.Name, index)
		return false
	}
	entries, err := h.inv.BuyBacks(player.CharServerID)
	if err != nil {
		log.Printf("BuyBack: %s: %v", player.Name, err)
		return false
	}
	var e *database.BuyBackEntry
	for i := range entries {
		if entries[i].ID == w.lines[slot] {
			e = &entries[i]
		}
	}
	gone := func() bool {
		log.Printf("BuyBack: %s asked %q for entry %d, which is gone", player.Name, npc.Name, w.lines[slot])
		h.refreshMerchant(sess, player, npc, id)
		SendColoredChat(sess, buyBackNotHere, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		return false
	}
	if e == nil {
		return gone()
	}
	p, n := &e.Item.Pattern, e.Item.StackLeft
	if e.Price > player.Tunar {
		log.Printf("BuyBack: %s cannot afford %d %q (%d > %d)", player.Name, n, p.ItemName, e.Price, player.Tunar)
		SendColoredChat(sess, buyBackNoTunar, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		return false
	}
	if inv.loreRoom(p, inv.wouldAdd(p.PatternID, n, p.MaxStack)) != n { // all of it or none, what AddItem would drop included
		log.Printf("BuyBack: %s has no room for %d %q", player.Name, n, p.ItemName)
		SendColoredChat(sess, buyBackNoRoom, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		return false
	}
	onto, stack := 0, 0
	if i := inv.stackOnto(p.PatternID, p.MaxStack); i >= 0 {
		have, _ := inv.at(i)
		onto, stack = have.itemID, have.qty+n
	}
	itemID, err := h.inv.TakeBuyBack(player.CharServerID, e.ID, onto, stack, player.Tunar-e.Price)
	if errors.Is(err, database.ErrBuyBackGone) {
		return gone()
	}
	if err != nil {
		log.Printf("BuyBack: %s, entry %d: %v", player.Name, e.ID, err)
		return false
	}
	player.Tunar -= e.Price
	key, _ := inv.add(itemID, p.PatternID, n, p.MaxStack)
	rec := wireRecord(e.Item)
	rec.ItemID = itemID
	w.serve(index, now)
	h.refreshMerchant(sess, player, npc, id)
	queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key))
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	log.Printf("BuyBack: %s bought back %d of %q from %q for %d (entry %d, item %d, %s)",
		player.Name, n, p.ItemName, npc.Name, e.Price, e.ID, itemID, e.How)
	return true
}

// keepBuyBack makes the entry for n of row, just sold or destroyed, and logs it: the action stands
// whatever comes of this.
func keepBuyBack(inv *database.InventoryStore, player *entity.Player, row *database.CharInventoryEntry, n, price int, how string) {
	id, kept, err := inv.AddBuyBack(player.CharServerID, row, n, price, how)
	switch {
	case err != nil:
		log.Printf("BuyBack: %s %s %d of %q, not kept: %v", player.Name, how, n, row.Pattern.ItemName, err)
	case kept:
		log.Printf("BuyBack: %s %s %d of %q (item %d), kept as entry %d to buy back for %d",
			player.Name, how, n, row.Pattern.ItemName, row.ItemID, id, price)
	default:
		log.Printf("BuyBack: %s %s %d of %q (item %d), No Rent: not kept", player.Name, how, n, row.Pattern.ItemName, row.ItemID)
	}
}

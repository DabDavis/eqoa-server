package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// accountbank.go — the account bank on the wire (world.account_bank; the server owner,
// 2026-11-14: "make the heirloom server bank for players be account wide and increase
// capacity to 2000"; database/accountbank.go has what is shared and where it is kept).
//
// No client shows 2000 records: CLIENT's bank is a VIItemList of 40 (VIBankControl+8,
// AddItem(item, maxStack, 40), VIItemList::Read writing the dump's count into 40 with no
// check: a 41st overruns VIClient), and vanilla's holds what the item lists' offer says,
// world.bank_slots, at most 255 (0x1250 names a record by a byte). So a session's bank
// list (invList, its cap the client's) is a PAGE of the account's bank, and the page is
// changed with nothing but the bank's own messages, which both clients take whatever
// window is open: 0x1250 (Took) for each record leaving it, 0x1251 (AddItem) for each
// one coming. Never more records than the client's list holds; never a 0x1251 the client
// would stack onto a record other than the one the server means (AddItem joins the FIRST
// record of a stackable item), so the session's copy and the client's stay one.
//
//   - Every change to the bank, by any of the account's characters (a move at a banker,
//     the auction's fee, escrow, payments and deliveries, a GM's #banktunar, a rehab cost,
//     a deposit past the page), is followed by syncAccountBank under bankMu: each of the
//     account's sessions in the world is sent the bank's tunar when it differs (0x1253)
//     and its page brought to the bank as it stands (syncBankPage): records gone or
//     smaller taken off, a stack grown onto its first record, the room left filled from
//     the page's place on. So two characters of an account in the world at once
//     (multibox) each see the other's moves, and a request names a record by its key in
//     its own session's copy, which every message to its client changed in step.
//   - bankMu (bank.go) is the account bank's one lock: the rows, the tunar and every
//     session's copy are changed only under it.
//   - "/bank [page | next | prev]" (a PS2's "/t bank ...") turns the page: every record
//     taken off, the page's records added.
//   - A deposit whose page is full goes into the account's bank all the same while it
//     holds fewer than AccountBankSlots (bankDepositUnshown): the pack's 0x003C, and
//     0x1253 to end the bank's wait (its pending state, which 0x1250, 0x1251 and 0x1253
//     each clear).

// acctBankWindows is the NPC windows' handler the account bank syncs through
// (its sessions, world and store), set as the registry is made.
var acctBankWindows atomic.Pointer[NPCWindowHandler]

// syncAccountBankOf is syncAccountBank on the registry's handler: nothing
// without the account bank. With bankMu held.
func syncAccountBankOf(serverID uint32) {
	if h := acctBankWindows.Load(); h != nil {
		h.syncAccountBank(serverID)
	}
}

// syncAccountBank brings every session in the world of serverID's account to
// the account's bank as the database holds it: its tunar (Player.BankTunar,
// 0x1253 when it differs) and its page (syncBankPage). Nothing without the
// account bank. With bankMu held.
func (h *NPCWindowHandler) syncAccountBank(serverID uint32) {
	if !database.AccountBankOn() || h == nil || h.inv == nil || h.sessionManager == nil || serverID == 0 {
		return
	}
	acct, ok, err := h.inv.AccountOf(serverID)
	if err != nil || !ok {
		return
	}
	tunar, _, err := h.inv.BankTunarOf(serverID)
	if err != nil {
		log.Printf("AccountBank: account %d's tunar: %v", acct, err)
		return
	}
	h.sessionManager.ForEach(func(s *session.Session) bool {
		if s.AccountID() != acct || !s.CharacterInWorld() || s.PendingTermination() {
			return true
		}
		l := sessionBankList(s)
		if l == nil {
			return true
		}
		if h.playerOf != nil {
			if p := h.playerOf(s); p != nil && p.CharServerID == s.CharacterServerID() && p.BankTunar != tunar {
				p.BankTunar = tunar
				queueReliable(s, protocol.OpcodeConfirmBankTunar, bankTunarPayload(tunar))
			}
		}
		rows, err := h.inv.GetBankItems(s.CharacterServerID())
		if err != nil {
			log.Printf("AccountBank: account %d's bank: %v", acct, err)
			return true
		}
		syncBankPage(s, l, rows)
		return true
	})
}

// pageable is whether a bank row can be added to a page as it is: AddItem
// keeps a stackable item's stack up to its maximum and one of anything else,
// so a row holding more is not shown (it stays in the bank, unreachable from
// this client until it changes).
func pageable(e *database.CharInventoryEntry) bool {
	if e.EquipLoc >= 0 || e.StackLeft < 1 {
		return false
	}
	if e.Pattern.MaxStack >= 2 {
		return e.StackLeft <= e.Pattern.MaxStack
	}
	return e.StackLeft == 1
}

// pageFill is which rows (their indexes, in order) fill room on a page that
// shows those in shown, from the row at from on and then from the first: none
// shown already, none the client would stack onto another record (a stackable
// item whose item a record of the page, or one taken before it, is already of:
// pats), none pageable refuses.
func pageFill(rows []database.CharInventoryEntry, from int, shown map[int]bool, pats map[int]bool, room int) []int {
	if room <= 0 || len(rows) == 0 {
		return nil
	}
	from = min(max(from, 0), len(rows))
	var out []int
	taken := map[int]bool{}
	for k := 0; k < len(rows) && len(out) < room; k++ {
		i := (from + k) % len(rows)
		e := &rows[i]
		if shown[e.ItemID] || !pageable(e) {
			continue
		}
		if e.Pattern.MaxStack >= 2 {
			if pats[e.Pattern.PatternID] || taken[e.Pattern.PatternID] {
				continue
			}
			taken[e.Pattern.PatternID] = true
		}
		out = append(out, i)
	}
	return out
}

// accountBankPage is a dump's page of the account's bank (a list of most
// records from its first row) and the items of the rows not on it, which are
// held (a Lore item there is one held: itemlist.go's loreHeld).
func accountBankPage(rows []database.CharInventoryEntry, most int) ([]database.CharInventoryEntry, map[int]bool) {
	idx := pageFill(rows, 0, nil, nil, most)
	on := make(map[int]bool, len(idx))
	page := make([]database.CharInventoryEntry, 0, len(idx))
	for _, i := range idx {
		page = append(page, rows[i])
		on[i] = true
	}
	var hidden map[int]bool
	for i := range rows {
		if !on[i] {
			if hidden == nil {
				hidden = map[int]bool{}
			}
			hidden[rows[i].Pattern.PatternID] = true
		}
	}
	return page, hidden
}

// syncBankPage brings a session's page (l, its copy of the client's bank list)
// to the account's bank rows, telling its client each change: a record whose
// row is gone (or another item) leaves (0x1250, all of it); a smaller one
// loses the difference; a larger one gains it (0x1251, which AddItem stacks
// onto it) when it is the first record of its item and the stack fits, else
// leaves; then the room left is filled (pageFill, from l.page). The items of
// the rows not on the page are kept as held (l.kept). With bankMu held.
func syncBankPage(sess *session.Session, l *invList, rows []database.CharInventoryEntry) {
	capes := readsCapes(sess)
	byID := make(map[int]*database.CharInventoryEntry, len(rows))
	for i := range rows {
		byID[rows[i].ItemID] = &rows[i]
	}
	l.mu.Lock()
	recs := append([]invRec(nil), l.recs...)
	l.mu.Unlock()
	first := func(i int) bool {
		for j := 0; j < i; j++ {
			if recs[j].patternID == recs[i].patternID {
				return false
			}
		}
		return true
	}
	grow := map[int]int{}
	for i := len(recs) - 1; i >= 0; i-- {
		r := recs[i]
		e := byID[r.itemID]
		n := 0
		switch {
		case e == nil || e.Pattern.PatternID != r.patternID:
			n = r.qty
		case e.StackLeft < r.qty:
			n = r.qty - e.StackLeft
		case e.StackLeft > r.qty:
			if e.Pattern.MaxStack >= 2 && e.StackLeft <= e.Pattern.MaxStack && first(i) {
				grow[r.itemID] = e.StackLeft - r.qty
			} else {
				n = r.qty
			}
		}
		if n > 0 && l.took(i, n) {
			queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, n))
		}
	}
	for _, id := range l.itemIDs() {
		more := grow[id]
		if more <= 0 {
			continue
		}
		e := byID[id]
		key, took := l.add(id, e.Pattern.PatternID, more, e.Pattern.MaxStack)
		if took != more {
			log.Printf("AccountBank: %d more of item %d onto a page that kept %d", more, id, took)
		}
		w := wireRecord(*e)
		w.StackLeft = more
		queueReliable(sess, protocol.OpcodeAddBankItem, bankAddPayload(&w, key, capes))
	}
	shown, pats := map[int]bool{}, map[int]bool{}
	l.mu.Lock()
	for _, r := range l.recs {
		shown[r.itemID] = true
		if e := byID[r.itemID]; e == nil || e.Pattern.MaxStack >= 2 {
			pats[r.patternID] = true
		}
	}
	room, from := l.capacity()-len(l.recs), l.page
	l.mu.Unlock()
	for _, i := range pageFill(rows, from, shown, pats, room) {
		e := &rows[i]
		key, took := l.add(e.ItemID, e.Pattern.PatternID, e.StackLeft, e.Pattern.MaxStack)
		if took == 0 {
			break // the list is full (it cannot be: room counted it)
		}
		shown[e.ItemID] = true
		w := wireRecord(*e)
		queueReliable(sess, protocol.OpcodeAddBankItem, bankAddPayload(&w, key, capes))
	}
	var hidden map[int]bool
	for i := range rows {
		if !shown[rows[i].ItemID] {
			if hidden == nil {
				hidden = map[int]bool{}
			}
			hidden[rows[i].Pattern.PatternID] = true
		}
	}
	l.mu.Lock()
	l.kept = hidden
	l.mu.Unlock()
}

// bankPageWord is whether a word names the bank's pages ("/bank", "#bank").
func bankPageWord(word string) bool {
	return strings.EqualFold(word, "/bank") || strings.EqualFold(word, "#bank")
}

// bankPageArg is whether a "/t bank <arg>" is the bank's pages and not a tell
// to someone named Bank: nothing, a page number, next, prev, first or last.
func bankPageArg(arg string) bool {
	arg = strings.ToLower(strings.TrimSpace(arg))
	switch arg {
	case "", "next", "prev", "previous", "first", "last":
		return true
	}
	_, err := strconv.Atoi(arg)
	return err == nil
}

// bankPageCommand is "/bank [page | next | prev | first | last]": the page of
// the account's bank the client is shown (alone: which it is). Only with the
// account bank; under bankMu.
func (h *NPCWindowHandler) bankPageCommand(sess *session.Session, arg string) {
	if !database.AccountBankOn() {
		SendErrorMessage(sess, "Your bank holds what you see at the banker.")
		return
	}
	if h == nil || h.inv == nil || h.playerOf == nil {
		return
	}
	p := h.playerOf(sess)
	l := sessionBankList(sess)
	if p == nil || l == nil {
		return
	}
	bankMu.Lock()
	defer bankMu.Unlock()
	rows, err := h.inv.GetBankItems(p.CharServerID)
	if err != nil {
		log.Printf("AccountBank: %s's bank: %v", p.Name, err)
		return
	}
	per := l.capacity()
	pages := max(1, (len(rows)+per-1)/per)
	l.mu.Lock()
	at := min(l.page/per, pages-1)
	l.mu.Unlock()
	arg = strings.ToLower(strings.TrimSpace(arg))
	to := at
	switch arg {
	case "":
		SendColoredChat(sess, bankPageLine(at, pages, per, len(rows)), 255, 255, 0)
		return
	case "next":
		to = (at + 1) % pages
	case "prev", "previous":
		to = (at - 1 + pages) % pages
	case "first":
		to = 0
	case "last":
		to = pages - 1
	default:
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > pages {
			SendErrorMessage(sess, fmt.Sprintf("Your bank has pages 1 to %d.", pages))
			return
		}
		to = n - 1
	}
	// every record off the page, from the last (each leaves the indexes below it as they were)
	l.mu.Lock()
	recs := append([]invRec(nil), l.recs...)
	l.mu.Unlock()
	for i := len(recs) - 1; i >= 0; i-- {
		if l.took(i, recs[i].qty) {
			queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, recs[i].qty))
		}
	}
	l.mu.Lock()
	l.page = to * per
	l.mu.Unlock()
	syncBankPage(sess, l, rows)
	SendColoredChat(sess, bankPageLine(to, pages, per, len(rows)), 255, 255, 0)
	log.Printf("AccountBank: %s turned to page %d of %d (%d records)", p.Name, to+1, pages, len(rows))
}

// bankPageLine says which page of the bank the client shows.
func bankPageLine(page, pages, per, n int) string {
	if n == 0 {
		return fmt.Sprintf("Your account's bank is empty: it holds up to %d items, shown %d at a time.", database.AccountBankSlots(), per)
	}
	return fmt.Sprintf("Your account's bank: page %d of %d (%d items, up to %d, shown %d at a time). /bank next, /bank prev or /bank <page> to turn it (on a PS2: /t bank next).",
		page+1, pages, n, database.AccountBankSlots(), per)
}

// SetAccountBankTunar sets the bank tunar of a character's account (the GM
// panel's bank_tunar with the account bank) and tells its characters in the
// world; it returns what the bank held before.
func SetAccountBankTunar(serverID uint32, v int) (int, error) {
	h := acctBankWindows.Load()
	if h == nil || h.inv == nil {
		return 0, fmt.Errorf("no bank on this server")
	}
	bankMu.Lock()
	defer bankMu.Unlock()
	before, ok, err := h.inv.BankTunarOf(serverID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("no character %d", serverID)
	}
	if _, _, err := h.inv.AddBankTunarOf(serverID, v-before, false); err != nil {
		return before, err
	}
	h.syncAccountBank(serverID)
	return before, nil
}

// accountBankTunarNow is the bank tunar of a character's account as kept, with
// the account bank (another of its characters may have moved it); else, or
// when it cannot be read, cached (the Player's). With bankMu held.
func accountBankTunarNow(serverID uint32, cached int) int {
	if !database.AccountBankOn() {
		return cached
	}
	if h := acctBankWindows.Load(); h != nil && h.inv != nil {
		if n, ok, err := h.inv.BankTunarOf(serverID); err == nil && ok {
			return n
		}
	}
	return cached
}

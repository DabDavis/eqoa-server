package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// itemlists.go — a pack and a bank past CLIENT's 40 records (the server owner,
// 2026-11-09: "lets increase inventory and bank space to 100"; world.pack_slots,
// world.bank_slots, entity/itemslots.go).
//
// CLIENT holds 40 records in each list, and no more, by construction (entity/itemslots.go
// says where): its every AddItem keeps nothing past 40, and the MemoryDump's lists are read
// into 40 records with no check, so a 41st in a dump writes over the VIClient fields after
// the list (a PS2's crash). So:
//
//   - The offer (protocol.OpcodeItemLists, after a session's first character list, once, only
//     on a server that holds more) is answered by a client that holds more (vanilla) with the
//     records it holds of each; the server takes each between 40 and its own (listCaps). A
//     PS2 ignores the offer and holds 40, and so does every client that never answers.
//   - Before every MemoryDump the client that answered is told the sizes in force
//     (protocol.OpcodeItemListsInForce): the dump's lists hold that many at most, and the
//     session's copies of the lists (itemlist.go's invList, its cap) hold that many, so every
//     room the server works out (loot, a sale, a trade, the bank's moves, a quest's rewards, a
//     craft, a catch, the auction's deliveries) is the client's own.
//   - A character with more records than its client holds (one filled in vanilla, logging in
//     on a PS2) is sent the first the client holds (planItemLists: every worn item, then the
//     rest in the order the pack was arranged in), and the rest are KEPT for it, untouched:
//     moved to the keeping of their list (charinventory location 2 for the pack, 3 for the
//     bank, database/itemlists.go), where nothing in the game reads them: no request, sale,
//     cast, quest or trade can reach a record its client was never sent. The player is told
//     once in the world. A login whose client has room brings them back, the first kept
//     first, at the end of their list (a respawn's or a restart's dump keeps, and never
//     brings back: what a player freed in a session is theirs until the next login).
//     Nothing is lost or made twice: a kept row is the same row, its stack, condition,
//     charges and quick slot as they were.

// listCaps is the records the session's client holds in its pack and its bank,
// for its next MemoryDump: what it answered the offer, between CLIENT's 40 and
// this server's; 40 each for a client that never answered (a PS2).
func listCaps(sess *session.Session) (pack, bank int) {
	pack, bank = entity.ClientItemSlots, entity.ClientItemSlots
	if sess == nil {
		return pack, bank
	}
	most, mostBank := entity.ItemSlots()
	if a, b := sess.ItemLists(); a > 0 || b > 0 {
		pack = max(entity.ClientItemSlots, min(a, most))
		bank = max(entity.ClientItemSlots, min(b, mostBank))
	}
	return pack, bank
}

// listSizes is the records a pack and a bank hold; 0 is CLIENT's 40.
type listSizes struct{ pack, bank int }

// clientLists is CLIENT's: 40 each.
var clientLists = listSizes{invListCap, invListCap}

func (l listSizes) packOf() int {
	if l.pack <= 0 {
		return invListCap
	}
	return l.pack
}

func (l listSizes) bankOf() int {
	if l.bank <= 0 {
		return invListCap
	}
	return l.bank
}

// sessionListSizes is listCaps as a listSizes.
func sessionListSizes(sess *session.Session) listSizes {
	pack, bank := listCaps(sess)
	return listSizes{pack, bank}
}

// packCapOf is the records the session's client's pack holds now: its list's
// (the dump that made it said so), else what its next dump will hold.
func packCapOf(sess *session.Session) int {
	if l := sessionInvList(sess); l != nil {
		return l.capacity()
	}
	pack, _ := listCaps(sess)
	return pack
}

// bankCapOf is packCapOf for the bank.
func bankCapOf(sess *session.Session) int {
	if l := sessionBankList(sess); l != nil {
		return l.capacity()
	}
	_, bank := listCaps(sess)
	return bank
}

// buildItemListsOffer is OpcodeItemLists from the server: a u32 1, then the
// records this server's pack and bank hold.
func buildItemListsOffer(pack, bank int) []byte {
	return build(protocol.OpcodeItemLists, 14, func(w *buffer.Writer) {
		w.WriteUint32(1)
		w.WriteUint32(uint32(pack))
		w.WriteUint32(uint32(bank))
	})
}

// SendItemListsOffer offers the session's client the larger lists, once a
// session, on a server that holds more than CLIENT's 40 (after its first
// character list, as the capes').
func SendItemListsOffer(sess *session.Session) {
	if !entity.ItemSlotsOffered() {
		return
	}
	if sess.OfferItemListsOnce() {
		pack, bank := entity.ItemSlots()
		queueReliable(sess, protocol.OpcodeItemLists, buildItemListsOffer(pack, bank))
	}
}

// buildItemListsInForce is OpcodeItemListsInForce: the records the next
// dump's pack and bank hold.
func buildItemListsInForce(pack, bank int) []byte {
	return build(protocol.OpcodeItemListsInForce, 10, func(w *buffer.Writer) {
		w.WriteUint32(uint32(pack))
		w.WriteUint32(uint32(bank))
	})
}

// sendItemListsInForce tells a client that answered the offer the sizes of the
// lists of the MemoryDump queued next; nothing to one that never answered (a
// PS2, which holds 40 and knows no such message).
func sendItemListsInForce(sess *session.Session, pack, bank int) {
	if a, b := sess.ItemLists(); a == 0 && b == 0 {
		return
	}
	queueReliable(sess, protocol.OpcodeItemListsInForce, buildItemListsInForce(pack, bank))
}

// ItemListsHandler takes a client's answer to the item lists' offer.
type ItemListsHandler struct{}

// NewItemListsHandler makes it.
func NewItemListsHandler() *ItemListsHandler { return &ItemListsHandler{} }

// RegisterHandlers registers it.
func (h *ItemListsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeItemLists, h.HandleItemLists)
}

// HandleItemLists takes the answer: a u32, 1 then the records the client holds
// of the pack and the bank (u32, u32), or 0 to hold CLIENT's 40 again. Taken
// only on a server that holds more and a session it was offered on; the sizes
// count from the session's next MemoryDump (its lists are made by it).
func (h *ItemListsHandler) HandleItemLists(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("ItemLists: short answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if binary.LittleEndian.Uint32(data[:4]) == 0 {
		// told 40 at its next dump (a client that never answered is told nothing)
		sess.SetItemLists(entity.ClientItemSlots, entity.ClientItemSlots)
		log.Printf("ItemLists: %s holds CLIENT's %d again", sess.RemoteAddr(), entity.ClientItemSlots)
		return nil
	}
	if !entity.ItemSlotsOffered() || !sess.ItemListsOffered() {
		log.Printf("ItemLists: %s asked for larger lists, which were not offered", sess.RemoteAddr())
		return nil
	}
	if len(data) < 12 {
		log.Printf("ItemLists: %s answered 1 with no sizes (%d bytes)", sess.RemoteAddr(), len(data))
		return nil
	}
	a := int(min(binary.LittleEndian.Uint32(data[4:8]), entity.MaxItemSlots))
	b := int(min(binary.LittleEndian.Uint32(data[8:12]), entity.MaxItemSlots))
	sess.SetItemLists(max(a, entity.ClientItemSlots), max(b, entity.ClientItemSlots))
	pack, bank := listCaps(sess)
	log.Printf("ItemLists: %s holds %d in its pack and %d in its bank (asked %d and %d)", sess.RemoteAddr(), pack, bank, a, b)
	return nil
}

// itemListPlan is what a MemoryDump keeps of a character's rows and brings back
// (planItemLists): the moves, and the rows each keeping holds after them.
type itemListPlan struct {
	moves              []database.ItemListMove
	keptPack, keptBank int // rows kept after the moves
}

// heldOf is which of a list's rows (in its order) a list of most records
// holds: every worn row (a worn item is never kept: the client must know what
// is worn), then the others in order while there is room.
func heldOf(rows []database.ItemListRow, most int) []bool {
	held := make([]bool, len(rows))
	n := 0
	for i, r := range rows {
		if r.EquipLoc >= 0 {
			held[i] = true
			n++
		}
	}
	for i, r := range rows {
		if n >= most {
			break
		}
		if r.EquipLoc < 0 {
			held[i] = true
			n++
		}
	}
	return held
}

// planItemLists is how a MemoryDump fits a character's rows (rows: every row
// in its four places, in list order) into lists of pack and bank records: a
// list with more keeps the rest (heldOf: worn rows and the first others stay),
// the kept rows after any kept before them; and at a login (restore) a list
// with room brings kept rows back, the first kept first, after its last row.
func planItemLists(rows []database.ItemListRow, pack, bank int, restore bool) itemListPlan {
	var p itemListPlan
	byLoc := map[int][]database.ItemListRow{}
	for _, r := range rows {
		byLoc[r.Location] = append(byLoc[r.Location], r)
	}
	next := func(rs []database.ItemListRow) int {
		n := 0
		for _, r := range rs {
			n = max(n, r.ListNum+1)
		}
		return n
	}
	fit := func(list, keep, most int) int {
		held, kept := byLoc[list], byLoc[keep]
		if len(held) > most {
			at := next(kept)
			k := 0
			for i, h := range heldOf(held, most) {
				if !h {
					p.moves = append(p.moves, database.ItemListMove{ItemID: held[i].ItemID, From: list, To: keep, ListNum: at + k})
					k++
				}
			}
			return len(kept) + k
		}
		if !restore || len(kept) == 0 || len(held) >= most {
			return len(kept)
		}
		at := next(held)
		back := min(len(kept), most-len(held))
		for k := 0; k < back; k++ {
			p.moves = append(p.moves, database.ItemListMove{ItemID: kept[k].ItemID, From: keep, To: list, ListNum: at + k})
		}
		return len(kept) - back
	}
	p.keptPack = fit(database.LocPack, database.LocPackKept, pack)
	p.keptBank = fit(database.LocBank, database.LocBankKept, bank)
	return p
}

// fitItemLists makes the plan for the character's rows and moves them, then
// reads the rows the dump sends, the pack's and the bank's. A row that came in
// since the rows were read (a GM's give) is fitted on a second look; a dump
// never holds more than pack and bank (a third, should one more come, sends the
// first that fit, worn ones always). login brings kept rows back where there is
// room; the moves it made are noted for the player (keptNotes). kept is the
// items of the rows the pack's keeping holds after it (the pack list's kept).
func (h *CharacterHandler) fitItemLists(serverID uint32, pack, bank int, login bool) (inv, bnk []database.CharInventoryEntry, kept map[int]bool) {
	if h.inventoryStore == nil {
		return nil, nil, nil
	}
	var note keptNote
	for look := 0; look < 3; look++ {
		rows, err := h.inventoryStore.ItemListRows(serverID)
		if err != nil {
			log.Printf("ItemLists: character %d's rows: %v", serverID, err)
			break
		}
		plan := planItemLists(rows, pack, bank, login && look == 0)
		note.keptPack, note.keptBank = plan.keptPack, plan.keptBank
		if len(plan.moves) > 0 {
			moved, err := h.inventoryStore.MoveItemListRows(serverID, plan.moves)
			if err != nil {
				log.Printf("ItemLists: character %d's rows not moved: %v", serverID, err)
			}
			for i, m := range plan.moves {
				switch {
				case !moved[i]:
					log.Printf("ItemLists: character %d's item %d was not where the plan found it (%d): left", serverID, m.ItemID, m.From)
				case m.To == database.LocPackKept:
					note.newPack++
				case m.To == database.LocBankKept:
					note.newBank++
				case m.To == database.LocPack:
					note.backPack++
				case m.To == database.LocBank:
					note.backBank++
				}
			}
			if kp, kb, err := h.inventoryStore.KeptItemRows(serverID); err == nil {
				note.keptPack, note.keptBank = kp, kb
			}
			log.Printf("ItemLists: character %d in lists of %d and %d: %d+%d kept now, %d+%d back, %d+%d kept in all",
				serverID, pack, bank, note.newPack, note.newBank, note.backPack, note.backBank, note.keptPack, note.keptBank)
		}
		all, err := h.inventoryStore.GetCharacterItems(serverID)
		if err != nil {
			log.Printf("SendMemoryDump: inventory query error: %v", err)
			return nil, nil, nil
		}
		inv, bnk = nil, nil
		for i := range all {
			if all[i].Location == database.LocBank {
				bnk = append(bnk, all[i])
			} else {
				inv = append(inv, all[i])
			}
		}
		if len(inv) <= pack && len(bnk) <= bank {
			break
		}
		log.Printf("ItemLists: character %d holds %d and %d past lists of %d and %d after a plan: fitted again", serverID, len(inv), len(bnk), pack, bank)
	}
	inv, bnk = capEntries(inv, pack), capEntries(bnk, bank)
	if note.keptPack > 0 {
		var err error
		if kept, _, err = h.inventoryStore.KeptPatterns(serverID); err != nil {
			log.Printf("ItemLists: character %d's kept items: %v", serverID, err)
		}
	}
	if login && (note.keptPack > 0 || note.keptBank > 0 || note.backPack > 0 || note.backBank > 0) {
		note.pack, note.bank = pack, bank
		note.mostPack, note.mostBank = entity.ItemSlots()
		keptNotesMu.Lock()
		keptNotes[serverID] = note
		keptNotesMu.Unlock()
	}
	return inv, bnk, kept
}

// capEntries is a list's rows as a list of most holds them (heldOf), the last
// guard of a dump: CLIENT reads a list's count with no check.
func capEntries(rows []database.CharInventoryEntry, most int) []database.CharInventoryEntry {
	if len(rows) <= most {
		return rows
	}
	at := make([]database.ItemListRow, len(rows))
	for i := range rows {
		at[i] = database.ItemListRow{ItemID: rows[i].ItemID, EquipLoc: rows[i].EquipLoc}
	}
	out := make([]database.CharInventoryEntry, 0, most)
	for i, h := range heldOf(at, most) {
		if h {
			out = append(out, rows[i])
		}
	}
	// never past it, not even for worn rows (there are 23 equip slots, so only
	// a broken table has more worn than a list holds)
	out = out[:min(len(out), most)]
	log.Printf("ItemLists: WARNING a dump's list cut from %d records to %d", len(rows), len(out))
	return out
}

// keptNote is what a login's dump did with the rows kept for the character,
// until its player is in the world to be told (tellKept).
type keptNote struct {
	pack, bank         int // the lists' sizes
	mostPack, mostBank int // this server's, for a client that holds them
	keptPack, keptBank int // kept after it
	newPack, newBank   int // kept by it
	backPack, backBank int // brought back by it
}

var (
	keptNotesMu sync.Mutex
	keptNotes   = map[uint32]keptNote{}
)

// tellKept tells a player entering the world what its login's dump kept and
// brought back (OnPlayerEntered).
func tellKept(sess *session.Session, p *entity.Player) {
	keptNotesMu.Lock()
	n, ok := keptNotes[p.CharServerID]
	delete(keptNotes, p.CharServerID)
	keptNotesMu.Unlock()
	if !ok {
		return
	}
	for _, line := range keptLines(n) {
		SendColoredChat(sess, line, 255, 255, 0)
	}
}

// keptLines are tellKept's lines.
func keptLines(n keptNote) []string {
	items := func(k int) string {
		if k == 1 {
			return "1 item"
		}
		return fmt.Sprintf("%d items", k)
	}
	var out []string
	if n.backPack > 0 {
		out = append(out, fmt.Sprintf("%s kept for you came back to your pack.", items(n.backPack)))
	}
	if n.backBank > 0 {
		out = append(out, fmt.Sprintf("%s kept for you came back to your bank.", items(n.backBank)))
	}
	more := func(k int) string {
		if k == 1 {
			return "1 more of your items is"
		}
		return fmt.Sprintf("%d more of your items are", k)
	}
	// where the server holds more than this client (a PS2's 40), where to go
	elsewhere := func(here, most int) string {
		if most > here {
			return fmt.Sprintf(", or on a game client that holds %d", most)
		}
		return ""
	}
	if n.keptPack > 0 {
		out = append(out, fmt.Sprintf("Your pack holds %d items here, so %s kept safe for you; they come back when you log in with room for them%s.",
			n.pack, more(n.keptPack), elsewhere(n.pack, n.mostPack)))
	}
	if n.keptBank > 0 {
		out = append(out, fmt.Sprintf("Your bank holds %d items here, so %s kept safe for you; they come back when you log in with room for them%s.",
			n.bank, more(n.keptBank), elsewhere(n.bank, n.mostBank)))
	}
	return out
}

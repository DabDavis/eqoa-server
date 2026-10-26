package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The windows an NPC opens: merchant (this file), bank (bank.go) and
// blacksmith (blacksmith.go).
//
// CLIENT's VIClient::InteractWithTarget (0x00BD5B68 [V]) picks the request by
// the target's m_npcFlags (state record bytes 146-147, the low byte of
// npcs.npc_type): 0x004C for a merchant (1), 0x124D for a banker (2), 0x0061
// for a blacksmith (8), and 0x0004 (InteractHandler) for anything else. Each
// request carries the NPC's id, and so does every request made from the
// window, so the server keeps no window state: it checks the NPC again each
// time — that it exists, carries the flag, and is within InteractRange.
//
// The client holds a request-pending flag per window (+0x1B304 merchant,
// VIBankControl +0x78C8 bank) and sends nothing more until a reply clears it.
// A refused request is answered with 0x00D7 (Msg00D7 [V]), which clears both
// and shows nothing.

// NPC flags as the client reads them (VICreatureEntity m_npcFlags).
const (
	npcFlagMerchant   = 0x01
	npcFlagBank       = 0x02
	npcFlagBlacksmith = 0x08
)

// merchantRate is the percentage 0x00B7 gives the client for selling
// (+0x1B308) and for buying (+0x1B30C). Sandstorm sends 100 and 100
// (ps2_main_capture), so an item sells and buys at its value.
const merchantRate = 100

// merchantGreeting is the line 0x00B7 ends with (m_merchantName, at most 127
// UTF-16 characters). Sandstorm sends each merchant's own; npcs carries none.
const merchantGreeting = "Have a look at my wares."

// NPCWindowHandler runs the merchant, bank and blacksmith windows.
type NPCWindowHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	inv            *database.InventoryStore

	// find and playerOf are the World's FindEntityByObjectID and
	// SessionPlayer, corpseLies and clearCorpse its CorpseLies and
	// ClearCorpse (SetGameWorld); tests stand in for them.
	find        func(*session.Session, uint32) (entity.Entity, bool)
	playerOf    func(*session.Session) *entity.Player
	corpseLies  func(entity.EntityID) (time.Time, bool)
	clearCorpse func(entity.EntityID, time.Time) bool

	// loot is the corpses (loot.go), made on first use; coinRoll rolls a
	// corpse's tunar (corpsecoins.go) and lootIntn its loot tables (loot.go's
	// rollLoot), rand.Intn when nil.
	loot     *lootState
	lootOnce sync.Once
	coinRoll func(int) int
	lootIntn func(int) int
}

// NewNPCWindowHandler creates the handler. inv may be nil (no database): the
// windows then refuse everything.
func NewNPCWindowHandler(sessionMgr *session.Manager, inv *database.InventoryStore) *NPCWindowHandler {
	return &NPCWindowHandler{sessionManager: sessionMgr, inv: inv}
}

// SetGameWorld sets the game world reference.
func (h *NPCWindowHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
	h.find = gw.FindEntityByObjectID
	h.playerOf = gw.SessionPlayer
	h.corpseLies = gw.CorpseLies
	h.clearCorpse = gw.ClearCorpse
	gw.OnCorpseGone(h.CorpsesGone)
}

// RegisterHandlers registers the window requests.
func (h *NPCWindowHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeMerchantDiag, h.HandleMerchantOpen)
	r.Register(protocol.OpcodeMerchantBuy, h.HandleMerchantBuy)
	r.Register(protocol.OpcodeMerchantSell, h.HandleMerchantSell)
	r.Register(protocol.OpcodeBankUI, h.HandleBankOpen)
	r.Register(protocol.OpcodeBankItem, h.HandleBankItem)
	r.Register(protocol.OpcodeDepositBankTunar, h.HandleBankTunar)
	r.Register(protocol.OpcodeAuctionRequest, h.HandleAuction)
	r.Register(protocol.OpcodeBlackSmithMenu, h.HandleBlacksmithOpen)
	r.Register(protocol.OpcodeRequestRepair, h.HandleRepair)
	r.Register(protocol.OpcodeCloseBlacksmithMenu, h.HandleBlacksmithClose)
	r.Register(protocol.OpcodeLootCorpse, h.HandleLootCorpse)
	r.Register(protocol.OpcodeClientCloseLoot, h.HandleLootClose)
	r.Register(protocol.OpcodeClientLoot, h.HandleLootItem)
	r.Register(protocol.OpcodeLootTakeAll, h.HandleLootAll)
}

// windowNPC resolves the NPC a window request names: it must be an NPC with
// the flag, within InteractRange of the session's player.
func (h *NPCWindowHandler) windowNPC(sess *session.Session, id uint32, flag int, what string) (*entity.Player, *entity.NPC, bool) {
	if h.find == nil || h.playerOf == nil {
		return nil, nil, false
	}
	player := h.playerOf(sess)
	if player == nil {
		return nil, nil, false
	}
	ent, ok := h.find(sess, id)
	if !ok {
		log.Printf("%s: %s names object %d, which this session does not see", what, player.Name, id)
		return nil, nil, false
	}
	npc, ok := ent.(*entity.NPC)
	if !ok || npc.NPCType&0xFF&flag == 0 {
		log.Printf("%s: %s names %q (object %d), which is not one", what, player.Name, entity.EntityName(ent), id)
		return nil, nil, false
	}
	if !entity.InRange(player, npc, entity.InteractRange) {
		log.Printf("%s: %s too far from %q (%.1f > %.1f)", what, player.Name, npc.Name,
			entity.DistanceBetween(player, npc), entity.InteractRange)
		return nil, nil, false
	}
	return player, npc, true
}

// refuse answers a request the server will not carry out: 0x00D7 clears the
// client's pending flags so the window takes requests again.
func refuse(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeCancelPending, build(protocol.OpcodeCancelPending, 2, nil))
}

// --- the messages ---

// merchantBoxPayload is 0x00B7 as the dispatcher reads it [V]: the merchant's
// id (+0x1B300), the sell and buy rates (zigzag, +0x1B308 and +0x1B30C), the
// catalog as a VIItemList (VIItemList::Read 0x00BBC8D0: zigzag count, u32 key
// counter, the records), then the greeting (VIMemStream::ReadString16: u32
// characters, UTF-16).
func merchantBoxPayload(id uint32, catalog []database.CharInventoryEntry, greeting string) []byte {
	return build(protocol.OpcodeMerchantBox, 64+1024*(len(catalog)+1), func(w *buffer.Writer) {
		w.WriteUint32(id)
		w.Write7BitEncodedInt64(merchantRate)
		w.Write7BitEncodedInt64(merchantRate)
		w.Write7BitEncodedInt64(int64(len(catalog)))
		w.WriteUint32(uint32(len(catalog)))
		for i := range catalog {
			rec := catalogRecord(catalog[i])
			writeSingleItem(w, &rec, i)
		}
		w.WriteStringUnicode(greeting)
	})
}

// addInvItemPayload is 0x003B: one record, ReadNetItem then
// VIItemList::AddItem(item, maxStack, 40) [V]. The client files it under its
// own counter, whatever key it carries; this sends that counter.
func addInvItemPayload(e *database.CharInventoryEntry, key uint32) []byte {
	return build(protocol.OpcodeAddInvItem, 1024, func(w *buffer.Writer) {
		writeSingleItem(w, e, int(key))
	})
}

// removeInvItemPayload is 0x003C: u8 list index, u8 sound, zigzag count →
// Msg003C [V], VIItemList::Took. With the merchant open, a non-zero sound
// plays and clears the merchant's pending flag; Sandstorm sends 1 for a sale.
func removeInvItemPayload(index int, sound byte, n int) []byte {
	return build(protocol.OpcodeRemoveInvItem, 16, func(w *buffer.Writer) {
		w.WriteByte(byte(index))
		w.WriteByte(sound)
		w.Write7BitEncodedInt64(int64(n))
	})
}

// tunarPayload is 0x0052: zigzag, the purse (VIPlayer +0x34).
func tunarPayload(tunar int) []byte {
	return build(protocol.OpcodePlayerTunar, 16, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(tunar))
	})
}

// --- prices, as the client computes them ---

// roundPercent is value × rate / 100, a remainder of 50 or more rounding up
// (GetMerchantPrice 0x00BCC950 [V], GetInvItemSellPrice 0x00BCC818 [A]).
func roundPercent(value, rate int) int {
	v := value * rate
	r := v / 100
	if v%100 >= 50 {
		r++
	}
	return r
}

// merchantPrice is GetMerchantPrice: what one of a catalog item costs.
func merchantPrice(value int) int { return roundPercent(value, merchantRate) }

// itemCondition is GetInvItemCondition (0x00BCC9C0 [V]): cur/max, 1 with no max.
func itemCondition(cur, max int) float32 {
	if max == 0 {
		return 1
	}
	return float32(cur) / float32(max)
}

// itemWear is GetInvItemWear (0x00BCC8C0 [V]).
func itemWear(cur, max int) float32 {
	c := itemCondition(cur, max)
	if c == 0 {
		return 0.5
	}
	if c == 1 {
		return 0
	}
	return (1 - c) * 0.5
}

// sellPrice is GetInvItemSellPrice: what one of an inventory item fetches —
// its value at the sell rate, less the share its wear takes.
func sellPrice(value, cur, max int) int {
	r := roundPercent(value, merchantRate)
	return r - int(float32(r)*itemWear(cur, max))
}

// --- merchant requests ---

// HandleMerchantOpen is 0x004C, u32 the merchant (InteractWithTarget's
// SendOpU32(76, m_target)): the window opens on 0x00B7 (MerchantOpen).
func (h *NPCWindowHandler) HandleMerchantOpen(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, err := r.ReadUint32()
	if err != nil {
		return nil
	}
	player, npc, ok := h.windowNPC(sess, id, npcFlagMerchant, "Merchant")
	if !ok {
		refuse(sess)
		return nil
	}
	// a quest's step to do at it is done first (questevents.go: CLIENT
	// never talks to an NPC with a window)
	if questsAtWindow(sess, player, npc) {
		return nil
	}
	var catalog []database.CharInventoryEntry
	if h.inv != nil {
		if catalog, err = h.inv.GetNPCItems(npc.NPCID); err != nil {
			log.Printf("Merchant: catalog of %q (npc %d): %v", npc.Name, npc.NPCID, err)
		}
	}
	if len(catalog) > invListCap {
		catalog = catalog[:invListCap]
	}
	queueReliable(sess, protocol.OpcodeMerchantBox, merchantBoxPayload(id, catalog, merchantGreeting))
	log.Printf("Merchant: %s opened %q (npc %d): %d items for sale", player.Name, npc.Name, npc.NPCID, len(catalog))
	return nil
}

// HandleMerchantBuy is 0x004A: zigzag catalog index, zigzag count, u32 the
// merchant (MerchantRequest 0x00BCCA38 [V]). Sandstorm answers 0x003B with the
// item, then 0x0052 with the purse.
func (h *NPCWindowHandler) HandleMerchantBuy(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	index, e1 := r.Read7BitEncodedInt64()
	count, e2 := r.Read7BitEncodedInt64()
	id, e3 := r.ReadUint32()
	if e1 != nil || e2 != nil || e3 != nil {
		return nil
	}
	if !h.buy(sess, id, int(index), int(count)) {
		refuse(sess)
	}
	return nil
}

func (h *NPCWindowHandler) buy(sess *session.Session, id uint32, index, count int) bool {
	player, npc, ok := h.windowNPC(sess, id, npcFlagMerchant, "MerchantBuy")
	inv := sessionInvList(sess)
	if !ok || h.inv == nil || inv == nil || count < 1 {
		return false
	}
	catalog, err := h.inv.GetNPCItems(npc.NPCID)
	if err != nil || index < 0 || index >= len(catalog) {
		log.Printf("MerchantBuy: %s asked %q for item %d of %d (%v)", player.Name, npc.Name, index, len(catalog), err)
		return false
	}
	item := catalog[index]
	p := &item.Pattern
	price := merchantPrice(p.ItemCost)

	// A stack goes out as one record of count; anything else one at a time.
	qty, records := count, 1
	if p.MaxStack < 2 {
		qty, records = 1, count
	}
	bought := 0
	for ; bought < records; bought++ {
		took := inv.wouldAdd(p.PatternID, qty, p.MaxStack)
		if took == 0 {
			break // the list is full, or the stack is
		}
		cost := price * took
		if cost > player.Tunar {
			log.Printf("MerchantBuy: %s cannot afford %d %q (%d > %d)", player.Name, took, p.ItemName, cost, player.Tunar)
			break
		}
		// A bought item comes new (Sandstorm's 0x003B carries the full
		// condition where its catalog record does not).
		fresh := item
		fresh.RemainHP = p.MaxHP
		rec, key, err := h.giveItem(player, inv, fresh, took, 0)
		if err != nil {
			log.Printf("MerchantBuy: %s: %v", player.Name, err)
			break
		}
		player.Tunar -= cost
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(rec, key))
		log.Printf("MerchantBuy: %s bought %d %q from %q for %d", player.Name, took, p.ItemName, npc.Name, cost)
	}
	if bought == 0 {
		return false
	}
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	return true
}

// giveItem files n of rec's item into a list the way the client's AddItem
// will (onto the first record of the same item when it stacks, else a new
// record at the end), in the database and in the mirror. location is 0 for
// the inventory, 1 for the bank. rec carries the condition and charges the
// item arrives with. It returns the record to send and the key the client
// files it under.
func (h *NPCWindowHandler) giveItem(player *entity.Player, l *invList, rec database.CharInventoryEntry, n, location int) (*database.CharInventoryEntry, uint32, error) {
	p := &rec.Pattern
	out := wireRecord(rec)
	out.StackLeft = n
	if i := l.stackOnto(p.PatternID, p.MaxStack); i >= 0 {
		have, _ := l.at(i)
		if err := h.inv.SetItemStack(player.CharServerID, have.itemID, have.qty+n); err != nil {
			return nil, 0, err
		}
		key, _ := l.add(have.itemID, p.PatternID, n, p.MaxStack)
		out.ItemID = have.itemID
		return &out, key, nil
	}
	itemID, err := h.inv.AddCharItem(player.CharServerID, p.PatternID, n, rec.RemainHP, rec.Charges, location)
	if err != nil {
		return nil, 0, err
	}
	out.ItemID = itemID
	key, _ := l.add(itemID, p.PatternID, n, p.MaxStack)
	return &out, key, nil
}

// wireRecord is a row as a record the server sends outside the MemoryDump:
// not equipped and on no quick slot — the record's fifth field is the quick
// slot (VINetItem +0x10; QuickSlot in the client's parser), not the database's
// location, and Sandstorm sends -1 for both in 0x00B7 and 0x003B.
func wireRecord(e database.CharInventoryEntry) database.CharInventoryEntry {
	e.EquipLoc = -1
	e.Location = -1
	return e
}

// catalogRecord is a catalog row as 0x00B7 lists it: at least one, and new
// when npcitem gives no condition.
func catalogRecord(e database.CharInventoryEntry) database.CharInventoryEntry {
	e = wireRecord(e)
	if e.StackLeft < 1 {
		e.StackLeft = 1
	}
	if e.RemainHP <= 0 {
		e.RemainHP = e.Pattern.MaxHP
	}
	return e
}

// HandleMerchantSell is 0x004B: u32 the item's key, zigzag count, u32 the
// merchant (SellItem 0x00BCCA80 [V]). Sandstorm answers 0x003C (the list
// index, sound 1, the count) and 0x0052.
func (h *NPCWindowHandler) HandleMerchantSell(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	key, e1 := r.ReadUint32()
	count, e2 := r.Read7BitEncodedInt64()
	id, e3 := r.ReadUint32()
	if e1 != nil || e2 != nil || e3 != nil {
		return nil
	}
	if !h.sell(sess, id, key, int(count)) {
		refuse(sess)
	}
	return nil
}

func (h *NPCWindowHandler) sell(sess *session.Session, id, key uint32, n int) bool {
	player, npc, ok := h.windowNPC(sess, id, npcFlagMerchant, "MerchantSell")
	inv := sessionInvList(sess)
	if !ok || h.inv == nil || inv == nil || n < 1 {
		return false
	}
	i := inv.index(key)
	rec, found := inv.at(i)
	if !found || rec.qty < n {
		log.Printf("MerchantSell: %s sells %d of key %d, which the inventory does not hold", player.Name, n, key)
		return false
	}
	row, err := h.inv.GetCharItem(player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("MerchantSell: %s: %v", player.Name, err)
		return false
	}
	if row.EquipLoc != -1 {
		log.Printf("MerchantSell: %s tried to sell %q while wearing it", player.Name, row.Pattern.ItemName)
		return false
	}
	paid := n * sellPrice(row.Pattern.ItemCost, row.RemainHP, row.Pattern.MaxHP)
	if err := h.takeItem(player, inv, i, rec, n); err != nil {
		log.Printf("MerchantSell: %s: %v", player.Name, err)
		return false
	}
	player.Tunar += paid
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(i, 1, n))
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	log.Printf("MerchantSell: %s sold %d %q to %q for %d", player.Name, n, row.Pattern.ItemName, npc.Name, paid)
	return true
}

// takeItem takes n off the record at list index i, in the database and in
// the mirror (VIItemList::Took: the record leaves the list when none is left).
func (h *NPCWindowHandler) takeItem(player *entity.Player, l *invList, i int, rec invRec, n int) error {
	var err error
	if rec.qty == n {
		err = h.inv.DeleteCharItem(player.CharServerID, rec.itemID)
	} else {
		err = h.inv.SetItemStack(player.CharServerID, rec.itemID, rec.qty-n)
	}
	if err != nil {
		return err
	}
	l.took(i, n)
	return nil
}

// saveTunar stores the purse and the bank's tunar.
// spendTunar takes amount tunar from the player, kept and shown, when it has
// that much (a waysender's fee: interact.go's travel).
func (h *NPCWindowHandler) spendTunar(sess *session.Session, player *entity.Player, amount int) bool {
	if amount > player.Tunar {
		return false
	}
	player.Tunar -= amount
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	return true
}

func (h *NPCWindowHandler) saveTunar(player *entity.Player) {
	if h.inv == nil {
		return
	}
	if err := h.inv.SetTunar(player.CharServerID, player.Tunar, player.BankTunar); err != nil {
		log.Printf("tunar: saving %s's %d / %d: %v", player.Name, player.Tunar, player.BankTunar, err)
	}
}

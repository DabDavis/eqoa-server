package handlers

import (
	"log"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The HUD's item quick slots and weapon sets, as SOE's live client has them
// (claude-notes/live-toolbars-gear-sets-20261024.md, read off the
// `c:/eqoa-live-1500` CLIENT in the retail save states):
//
//   - The item menu's Prepare puts an item on one of four quick slots: the
//     client asks with 0x000A (u32 key, u8 slot) and puts it there only when
//     the server answers 0x000A (u8 list index, u8 slot), as Msg000A reads it.
//     The disc's client takes a potion (type 2) on slots 0-1, a gem (type 7)
//     on 2 and a weapon on 3; the live one a gem on 2-3. Each applies its own
//     rule to the answer, so the server takes what either does.
//   - Prepare on a weapon makes a weapon set (live only): the client equips the
//     weapon if it is not worn, then names set 0-3 and sends 0x0021 (u8 set,
//     the name). The set is the two hands as worn then; the server answers
//     0x0021 (u8 set, the name, u32 main hand's item, u32 off hand's, -1 none,
//     the items by pattern as the client looks them up) and sends all four at
//     world entry. 0x0022 (u8 set) takes a set up; set 4 is bare hands.
//
// The live MemoryDump carries the four sets between the inventory and the bank;
// the disc's does not, so the server never puts them there (a PS2 would read
// the bank out of them). A PS2 has no case for 0x0021 and skips it.

// gearNameMax is the most of a set's name the client keeps: its 32-byte buffer
// (ReadString's MaxBytes) less the terminator, and the 16-character editor's
// limit less its terminator.
const gearNameMax = 15

// bareHands is the set number that takes off everything in the hands.
const bareHands = database.GearSets

// A set's two hands are read from these equip slots, in order (charinventory
// equiploc): the main hand is the two-hander, the primary, the ranged weapon or
// an item held, the off hand the secondary, the shield or an item held in it.
// Msg003F's table lets one of each be worn at a time.
var (
	mainHandSlots = []int{16, 14, 18, 19}
	offHandSlots  = []int{15, 17, 20}
	// handSlots is everything bare hands takes off.
	handSlots = []int{14, 15, 16, 17, 18, 19, 20}
)

// fitsSlot3 is CLIENT's FitsSlot3 (0x00BDC470): the wear slots the disc's
// fourth quick slot takes.
func fitsSlot3(wear int) bool {
	switch wear {
	case 12, 14, 15, 16, 18, 20:
		return true
	}
	return false
}

// quickSlotFits is whether an item may go on quick slot pos, by either client's
// Msg000A: a type-2 item on 0-1, a type-7 on 2, and on 3 a type-7 (the live
// client's) or a weapon (the disc's). The type is VIItem +0x4C, the database's
// procanim.
func quickSlotFits(e *database.CharInventoryEntry, pos int) bool {
	t := e.Pattern.ProcAnim
	switch pos {
	case 0, 1:
		return t == 2
	case 2:
		return t == 7
	case 3:
		return t == 7 || fitsSlot3(e.Pattern.EquipSlot)
	}
	return false
}

// buildPrepareItem is 0x000A to the client: the item's LIST index and its slot.
func buildPrepareItem(index, pos int) []byte {
	return build(protocol.OpcodePrepareItem, 8, func(w *buffer.Writer) {
		w.WriteByte(byte(index))
		w.WriteByte(byte(pos))
	})
}

// buildGearSet is 0x0021 to the client (the live dispatcher's case at
// 0x012CADEC: Read 1, ReadString into 32 bytes, Read 4, Read 4, then
// SetGearSet 0x012C49E8, which keeps sets 0-3).
func buildGearSet(set int, g database.GearSet) []byte {
	return build(protocol.OpcodePrepareWeapon, 64, func(w *buffer.Writer) {
		w.WriteByte(byte(set))
		writeWideString(w, g.Name, 32)
		w.WriteUint32(uint32(int32(g.MainPattern)))
		w.WriteUint32(uint32(int32(g.OffPattern)))
	})
}

// cutWide keeps the first n UTF-16 units of s, never half a surrogate pair.
func cutWide(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	if n > 0 && utf16.IsSurrogate(rune(u[n-1])) && u[n-1] < 0xDC00 {
		n--
	}
	return string(utf16.Decode(u[:n]))
}

// wornIn is the first inventory row worn in one of the slots, in their order.
func wornIn(inv []*database.CharInventoryEntry, slots []int) *database.CharInventoryEntry {
	for _, s := range slots {
		for _, e := range inv {
			if e.EquipLoc == s {
				return e
			}
		}
	}
	return nil
}

// gearSetOfWorn is a set of the hands as they are worn.
func gearSetOfWorn(inv []*database.CharInventoryEntry, name string) database.GearSet {
	g := database.EmptyGearSet
	g.Name = name
	if e := wornIn(inv, mainHandSlots); e != nil {
		g.MainItem, g.MainPattern = e.ItemID, e.Pattern.PatternID
	}
	if e := wornIn(inv, offHandSlots); e != nil {
		g.OffItem, g.OffPattern = e.ItemID, e.Pattern.PatternID
	}
	return g
}

// listIndexOf is an inventory row's index in the client's list (what EquipItem
// names), resolved through the session's list as invItemByKey does; with no
// list yet, its position in the MemoryDump's order.
func listIndexOf(items []database.CharInventoryEntry, list *invList, itemID int) int {
	if list == nil {
		for i, e := range inventoryRows(items) {
			if e.ItemID == itemID {
				return i
			}
		}
		return -1
	}
	list.mu.Lock()
	defer list.mu.Unlock()
	for i := range list.recs {
		if list.recs[i].itemID == itemID {
			return i
		}
	}
	return -1
}

// findSetItem is a set's hand in the inventory: the row it was prepared with,
// else the first other inventory row of the same item (not one of skip).
func findSetItem(inv []*database.CharInventoryEntry, itemID, patternID int, skip int) *database.CharInventoryEntry {
	if itemID < 0 {
		return nil
	}
	for _, e := range inv {
		if e.ItemID == itemID {
			return e
		}
	}
	for _, e := range inv {
		if e.ItemID != skip && e.Pattern.PatternID == patternID {
			return e
		}
	}
	return nil
}

// mainSlotFor is the equip slot a set's main hand goes into: a one-handed
// weapon the primary, anything else the first slot its wear takes.
func mainSlotFor(e *database.CharInventoryEntry) int {
	slots := itemSlotToEquipSlot[e.Pattern.EquipSlot]
	if len(slots) == 0 {
		return -1
	}
	return slots[0]
}

// offSlotFor is the equip slot a set's off hand goes into: a one-handed
// weapon the secondary, a shield its slot, an item held the second hand.
func offSlotFor(e *database.CharInventoryEntry) int {
	switch e.Pattern.EquipSlot {
	case 14:
		return 15
	case 13:
		return 17
	case 18:
		return 20
	}
	slots := itemSlotToEquipSlot[e.Pattern.EquipSlot]
	if len(slots) == 0 {
		return -1
	}
	return slots[len(slots)-1]
}

// HandlePrepareItem takes the item menu's Prepare (RequestQuickSlot,
// 0x00BCADB8: SendMsg(10, key, slot)): the item goes on the quick slot, and
// the client is told so.
func (h *EquipHandler) HandlePrepareItem(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	key, err1 := r.ReadUint32()
	pos, err2 := r.ReadByte()
	if err1 != nil || err2 != nil {
		log.Printf("PrepareItem: short payload %x", msg.Data())
		return nil
	}
	player := h.player(sess)
	if player == nil {
		return nil
	}
	serverID := player.CharServerID
	if int(pos) >= database.QuickSlots {
		log.Printf("PrepareItem: serverID=%d key=%d slot %d out of range", serverID, key, pos)
		return nil
	}
	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("PrepareItem: loading items for serverID=%d: %v", serverID, err)
		return nil
	}
	idx, row, err := invItemByKey(items, sessionInvList(sess), key)
	if err != nil {
		log.Printf("PrepareItem: serverID=%d key=%d: %v", serverID, key, err)
		return nil
	}
	if !quickSlotFits(row, int(pos)) {
		log.Printf("PrepareItem: serverID=%d %q (type %d, wear %d) does not go on quick slot %d",
			serverID, row.Pattern.ItemName, row.Pattern.ProcAnim, row.Pattern.EquipSlot, pos)
		return nil
	}
	if err := h.inventoryStore.SetQuickSlot(serverID, int(pos), row.ItemID); err != nil {
		log.Printf("PrepareItem: serverID=%d item %d -> quick slot %d: %v", serverID, row.ItemID, pos, err)
		return nil
	}
	log.Printf("PrepareItem: serverID=%d %q (list index %d) on quick slot %d", serverID, row.Pattern.ItemName, idx, pos)
	queueReliable(sess, protocol.OpcodePrepareItem, buildPrepareItem(idx, int(pos)))
	return nil
}

// HandlePrepareWeapon takes a weapon set the player named (the live client's
// RequestGearSet, 0x012C33E8: u8 set, the name): the set becomes the two hands
// as worn now, and the client is told it.
func (h *EquipHandler) HandlePrepareWeapon(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	set, err := r.ReadByte()
	name, ok := readWide(r)
	if err != nil || !ok {
		log.Printf("PrepareWeapon: short payload %x", msg.Data())
		return nil
	}
	player := h.player(sess)
	if player == nil {
		return nil
	}
	serverID := player.CharServerID
	if int(set) >= database.GearSets {
		log.Printf("PrepareWeapon: serverID=%d set %d out of range", serverID, set)
		return nil
	}
	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("PrepareWeapon: loading items for serverID=%d: %v", serverID, err)
		return nil
	}
	g := gearSetOfWorn(inventoryRows(items), cutWide(name, gearNameMax))
	if err := h.inventoryStore.SetGearSet(serverID, int(set), g); err != nil {
		log.Printf("PrepareWeapon: serverID=%d set %d: %v", serverID, set, err)
		return nil
	}
	log.Printf("PrepareWeapon: serverID=%d set %d %q = item %d / item %d", serverID, set, g.Name, g.MainItem, g.OffItem)
	queueReliable(sess, protocol.OpcodePrepareWeapon, buildGearSet(int(set), g))
	return nil
}

// HandleUseWeaponSet takes a set up (the live client's UseGearSet, 0x012C3418,
// from the HUD's fourth menu): its main hand into its slot, then its off hand,
// or the off hand emptied when the set has none; set 4 empties the hands.
func (h *EquipHandler) HandleUseWeaponSet(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("UseWeaponSet: empty payload")
		return nil
	}
	set := int(data[0])
	player := h.player(sess)
	if player == nil {
		return nil
	}
	serverID := player.CharServerID
	if set > bareHands {
		log.Printf("UseWeaponSet: serverID=%d set %d out of range", serverID, set)
		return nil
	}
	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("UseWeaponSet: loading items for serverID=%d: %v", serverID, err)
		return nil
	}
	if set == bareHands {
		h.takeOff(sess, serverID, items, handSlots)
		h.charHandler.applyEquipmentFromDB(player, serverID, false)
		return nil
	}
	sets, err := h.inventoryStore.GearSetsOf(serverID)
	if err != nil {
		log.Printf("UseWeaponSet: serverID=%d: %v", serverID, err)
		return nil
	}
	g := sets[set]
	main := findSetItem(inventoryRows(items), g.MainItem, g.MainPattern, -1)
	if main == nil {
		log.Printf("UseWeaponSet: serverID=%d set %d %q: its main hand (item %d) is not in the inventory", serverID, set, g.Name, g.MainItem)
		return nil
	}
	if !h.equipInto(sess, serverID, items, main, mainSlotFor(main)) {
		return nil
	}
	// what the main hand displaced is off now: read the hands again
	if items, err = h.inventoryStore.GetCharacterItems(serverID); err != nil {
		log.Printf("UseWeaponSet: loading items for serverID=%d: %v", serverID, err)
		return nil
	}
	if g.OffItem < 0 {
		h.takeOff(sess, serverID, items, offHandSlots)
	} else if off := findSetItem(inventoryRows(items), g.OffItem, g.OffPattern, main.ItemID); off != nil {
		h.equipInto(sess, serverID, items, off, offSlotFor(off))
	} else {
		log.Printf("UseWeaponSet: serverID=%d set %d %q: its off hand (item %d) is not in the inventory", serverID, set, g.Name, g.OffItem)
	}
	h.charHandler.applyEquipmentFromDB(player, serverID, false)
	return nil
}

// equipInto puts an inventory row into an equip slot (planEquipInto).
func (h *EquipHandler) equipInto(sess *session.Session, serverID uint32, items []database.CharInventoryEntry, e *database.CharInventoryEntry, slot int) bool {
	if slot < 0 || !fitsEquip(e.Pattern.EquipSlot, slot) {
		log.Printf("UseWeaponSet: serverID=%d %q (wear %d) does not go in slot %d", serverID, e.Pattern.ItemName, e.Pattern.EquipSlot, slot)
		return false
	}
	idx := listIndexOf(items, sessionInvList(sess), e.ItemID)
	if idx < 0 {
		log.Printf("UseWeaponSet: serverID=%d %q is not in the client's list", serverID, e.Pattern.ItemName)
		return false
	}
	return h.applyPlan(sess, serverID, planEquipInto(inventoryRows(items), idx, e, slot), "UseWeaponSet")
}

// takeOff empties equip slots (UnequipItem for each one worn).
func (h *EquipHandler) takeOff(sess *session.Session, serverID uint32, items []database.CharInventoryEntry, slots []int) {
	inv := inventoryRows(items)
	for _, s := range slots {
		for _, e := range inv {
			if e.EquipLoc != s {
				continue
			}
			plan := equipPlan{name: e.Pattern.ItemName, unequip: s, changes: []equipChange{{e.ItemID, -1}}}
			if !h.applyPlan(sess, serverID, plan, "UseWeaponSet") {
				return
			}
			break
		}
	}
}

// SendGearSets tells the client its four weapon sets (world entry). A PS2 skips
// them.
func (h *EquipHandler) SendGearSets(sess *session.Session) {
	player := h.player(sess)
	if player == nil || h.inventoryStore == nil {
		return
	}
	sets, err := h.inventoryStore.GearSetsOf(player.CharServerID)
	if err != nil {
		log.Printf("SendGearSets: serverID=%d: %v", player.CharServerID, err)
		return
	}
	for i, g := range sets {
		queueReliable(sess, protocol.OpcodePrepareWeapon, buildGearSet(i, g))
	}
}

// player is the session's player, or nil.
func (h *EquipHandler) player(sess *session.Session) *entity.Player {
	if h.playerOf == nil {
		return nil
	}
	return h.playerOf(sess)
}

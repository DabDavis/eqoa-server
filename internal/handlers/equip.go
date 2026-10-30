package handlers

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// EquipHandler implements InteractItem (0x003E) — the client's equip/unequip
// request. Until this existed the opcode fell through to "Unhandled opcode",
// so nothing could be equipped in game and appearance could only be changed by
// editing charinventory directly.
//
// Wire format, CLIENT's VIClient::UseItem (0x00BCABA0), which writes it with
// SendOpIntIntByte, and live payloads such as `4d000000ffffffff00`:
//
//	uint32  key          the record's key, VIItemList::GetSlotId (+0x2F8) —
//	                     NOT its position in the list once records have left
//	uint32  target       the client's current target (m_target)
//	uint8   interaction  UseItem's second argument: 0 from the item menu, 1 from
//	                     the HUD's item quick slot, both a use or an equip toggle
//
// It is a TOGGLE: equipped -> unequip, not equipped -> equip. The server picks
// the slot (the first free one a pair allows) and tells the client which,
// in EquipItem (0x003F); the client then takes off what that displaces itself.
//
// A spell scroll is not worn: the same request scribes it (scribe.go).
type EquipHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	inventoryStore *database.InventoryStore
	spells         *database.SpellbookStore
	charHandler    *CharacterHandler

	// playerOf is the World's SessionPlayer and eat its Consume
	// (SetGameWorld); tests stand in for them.
	playerOf func(*session.Session) *entity.Player
	eat      func(*session.Session, entity.Consumable, time.Time) error
	consumed func(*session.Session) // the World's Consumed: the cheer

	fishing *FishingHandler // a fishing pole used from the HUD (fishing.go)
	craft   *CraftHandler   // a recipe scroll used (craft.go)
}

// NewEquipHandler creates the handler. spells may be nil: no scroll is then
// scribed.
func NewEquipHandler(sessionMgr *session.Manager, inv *database.InventoryStore, spells *database.SpellbookStore, ch *CharacterHandler) *EquipHandler {
	return &EquipHandler{sessionManager: sessionMgr, inventoryStore: inv, spells: spells, charHandler: ch}
}

// SetGameWorld sets the game world reference.
func (h *EquipHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
	h.playerOf = gw.SessionPlayer
	h.eat = gw.Consume
	h.consumed = gw.Consumed
}

// RegisterHandlers registers the equip handlers.
func (h *EquipHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeInteractItem, h.HandleInteractItem)
	// the HUD's item quick slots and weapon sets (gearsets.go)
	r.Register(protocol.OpcodePrepareItem, h.HandlePrepareItem)
	r.Register(protocol.OpcodePrepareWeapon, h.HandlePrepareWeapon)
	r.Register(protocol.OpcodeUseWeaponSet, h.HandleUseWeaponSet)
}

// itemSlotToEquipSlot maps an item's wear slot (itempattern.equipslot; the
// item's +0x14 on the client) to the equip slots (charinventory.equiploc) it may
// be put in, in the order the server tries them: exactly the slots CLIENT's fit
// check (fitsEquip) accepts. Two entries are a pair, and the server takes
// whichever is free, else the first.
var itemSlotToEquipSlot = map[int][]int{
	1:  {0},      // Head        -> Head
	2:  {21},     // Robe        -> Robe
	3:  {2, 3},   // Earring     -> Left/Right ear
	4:  {4},      // Neck        -> Neck
	5:  {5},      // Chest       -> Chest
	6:  {6, 7},   // Forearm     -> Right/Left bracelet
	7:  {8},      // TwoForearm  -> Bracers
	8:  {9, 10},  // Ring        -> Left/Right ring
	9:  {11},     // Belt        -> Belt
	10: {12},     // Leg         -> Leg
	11: {13},     // Feet        -> Feet
	12: {14},     // Primary     -> Primary
	13: {17},     // Shield      -> Shield
	14: {14, 15}, // Secondary   -> Primary/Secondary
	15: {16},     // TwoHand     -> TwoHand
	16: {18},     // Bow         -> Ranged (CLIENT allows a bow nowhere else)
	17: {18},     // Thrown      -> Ranged
	18: {19, 20}, // Held        -> Held/Held2
	19: {1},      // Hand        -> Hand (gloves)
	20: {14},     // Fishing     -> Primary
	21: {15},     // Bait        -> Secondary
	22: {19},     // Tools (six crafting kinds, 22-27) -> Held
	23: {19},
	24: {19},
	25: {19},
	26: {19},
	27: {19},
}

// fitsEquip is CLIENT's func_00CC61A8, which Msg003F (0x00BCB2F0) asks before
// it equips anything: whether an item of wear slot w may go in equip slot e. A
// 0x003F that fails it is ignored by the client, so the server must never
// choose a slot it rejects (itemSlotToEquipSlot is checked against it in
// tests). Read off its jump table at 0x00CF3E40, wear -1..27.
func fitsEquip(w, e int) bool {
	switch w {
	case 1:
		return e == 0
	case 2:
		return e == 21
	case 3:
		return e == 2 || e == 3
	case 4:
		return e == 4
	case 5:
		return e == 5
	case 6:
		return e == 6 || e == 7
	case 7:
		return e == 8
	case 8:
		return e == 9 || e == 10
	case 9:
		return e == 11
	case 10:
		return e == 12
	case 11:
		return e == 13
	case 12:
		return e == 14
	case 13:
		return e == 17
	case 14:
		return e == 14 || e == 15
	case 15:
		return e == 16
	case 16, 17:
		return e == 18
	case 18:
		return e == 19 || e == 20
	case 19:
		return e == 1
	case 20:
		return e == 14
	case 21:
		return e == 15
	case 22, 23, 24, 25, 26, 27:
		return e == 19
	}
	return false
}

// equipDisplaces is Msg003F's jump table (data_00CDA880, wear 6..27): the equip
// slots the client empties, after the target slot itself, when an item of wear
// slot w goes into slot e. The server empties the same ones in its database,
// or it keeps items equipped that the client shows as taken off.
func equipDisplaces(w, e int) []int {
	switch w {
	case 6: // forearm
		return []int{8}
	case 7: // both forearms
		return []int{7, 6}
	case 12, 20: // primary, fishing
		return []int{16, 18, 19}
	case 13: // shield
		return []int{15, 16, 18, 20}
	case 14, 21: // secondary, bait
		if e == 15 {
			return []int{16, 18, 17, 20}
		}
		return []int{16, 18, 19}
	case 15: // two-handed
		return []int{14, 15, 17, 18, 19, 20}
	case 16: // bow
		return []int{14, 15, 17, 16, 19, 20}
	case 18: // held
		if e != 20 {
			return []int{16, 18, 14}
		}
		return []int{16, 18, 17, 15}
	case 22, 23, 24, 25, 26, 27: // crafting tools
		return []int{16, 18, 14}
	}
	return nil
}

// equipChange is one charinventory row whose equiploc changes.
type equipChange struct {
	itemID   int
	equipLoc int
}

// equipPlan is what one InteractItem toggle does.
type equipPlan struct {
	name    string        // the item, for the log
	key     uint32        // the key the client asked with
	changes []equipChange // in order: displaced items first, the item last
	unequip int           // the slot UnequipItem (0x0040) vacates, or -1
	equip   bool          // send EquipItem (0x003F)
	index   int           // with the item's LIST index
	slot    int           // and the slot it now occupies
}

var errNotEquippable = errors.New("not equippable")

// inventoryRows is the client's inventory list in database order: every row
// that is not in the bank (location 1), the order SendMemoryDump writes them.
func inventoryRows(items []database.CharInventoryEntry) []*database.CharInventoryEntry {
	inv := make([]*database.CharInventoryEntry, 0, len(items))
	for i := range items {
		if items[i].Location == 1 {
			continue // bank is a separate list with its own keys
		}
		inv = append(inv, &items[i])
	}
	return inv
}

// invItemByKey is the record a request names: its list index and its row.
// The key is resolved through the client's list (list), not used as a
// position; with no list (no MemoryDump sent yet) a key is the position the
// MemoryDump would have given it.
func invItemByKey(items []database.CharInventoryEntry, list *invList, key uint32) (int, *database.CharInventoryEntry, error) {
	inv := inventoryRows(items)
	if list == nil {
		if int(key) >= len(inv) {
			return -1, nil, fmt.Errorf("key %d out of range (inventory has %d items)", key, len(inv))
		}
		return int(key), inv[key], nil
	}
	idx := list.index(key)
	if idx < 0 {
		return -1, nil, fmt.Errorf("the client holds no record with key %d", key)
	}
	rec, _ := list.at(idx)
	for _, e := range inv {
		if e.ItemID == rec.itemID {
			return idx, e, nil
		}
	}
	return -1, nil, fmt.Errorf("key %d is item %d, which the inventory no longer holds", key, rec.itemID)
}

// planInteractItem decides one toggle, for the record the key names
// (invItemByKey).
func planInteractItem(items []database.CharInventoryEntry, list *invList, key uint32) (equipPlan, error) {
	plan := equipPlan{key: key, unequip: -1}
	inv := inventoryRows(items)

	idx, found, err := invItemByKey(items, list, key)
	if err != nil {
		return plan, err
	}
	plan.name = found.Pattern.ItemName

	wear := found.Pattern.EquipSlot
	slots, equippable := itemSlotToEquipSlot[wear]
	if !equippable {
		return plan, errNotEquippable
	}

	if found.EquipLoc >= 0 {
		// Equipped -> unequip.
		plan.changes = []equipChange{{found.ItemID, -1}}
		plan.unequip = found.EquipLoc
		return plan, nil
	}

	// Not equipped -> equip, into the first free slot of the pair, else the
	// first.
	worn := map[int]bool{}
	for _, e := range inv {
		if e.EquipLoc >= 0 {
			worn[e.EquipLoc] = true
		}
	}
	chosen := slots[0]
	for _, s := range slots {
		if !worn[s] {
			chosen = s
			break
		}
	}
	into := planEquipInto(inv, idx, found, chosen)
	into.key = key
	return into, nil
}

// planEquipInto is the equip half of a toggle into a slot the caller chose (a
// weapon set's hand, gearsets.go): what the client's Msg003F takes off comes
// off, everything in that slot and then everything in the slots its table
// names for the item's wear, and the item goes in. An item already worn in
// another slot comes off that one first (UnequipItem); one already in the
// slot changes nothing.
func planEquipInto(inv []*database.CharInventoryEntry, idx int, found *database.CharInventoryEntry, slot int) equipPlan {
	plan := equipPlan{name: found.Pattern.ItemName, unequip: -1}
	if found.EquipLoc == slot {
		return plan
	}
	if found.EquipLoc >= 0 {
		plan.unequip = found.EquipLoc
	}
	worn := map[int][]int{}
	for _, e := range inv {
		if e.EquipLoc >= 0 && e.ItemID != found.ItemID {
			worn[e.EquipLoc] = append(worn[e.EquipLoc], e.ItemID)
		}
	}
	for _, s := range append([]int{slot}, equipDisplaces(found.Pattern.EquipSlot, slot)...) {
		for _, id := range worn[s] {
			plan.changes = append(plan.changes, equipChange{id, -1})
		}
		delete(worn, s)
	}
	plan.changes = append(plan.changes, equipChange{found.ItemID, slot})
	plan.equip, plan.index, plan.slot = true, idx, slot
	return plan
}

// parseInteractItem reads the request: u32 key, u32 target, u8 interaction.
func parseInteractItem(data []byte) (key, target uint32, interaction byte, ok bool) {
	if len(data) < 9 {
		return 0, 0, 0, false
	}
	r := buffer.NewReader(data)
	key, e1 := r.ReadUint32()
	target, e2 := r.ReadUint32()
	interaction, e3 := r.ReadByte()
	return key, target, interaction, e1 == nil && e2 == nil && e3 == nil
}

// HandleInteractItem toggles an item between equipped and not.
func (h *EquipHandler) HandleInteractItem(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	key, target, interaction, ok := parseInteractItem(data)
	if !ok {
		log.Printf("InteractItem: short or malformed payload len=%d hex=%x", len(data), data)
		return nil
	}
	if interaction > 1 {
		// 0 is the item menu's use or equip (UseItem(i, 0)) and 1 the HUD's
		// item quick slot (VIWndGame::OnMessage 0x2030000: UseItem(i, 1)),
		// which asks for the same; no other value is sent.
		log.Printf("InteractItem: interaction type %d (key=%d target=0x%08X) — not handled",
			interaction, key, target)
		return nil
	}

	if h.playerOf == nil {
		return nil
	}
	player := h.playerOf(sess)
	if player == nil {
		return nil
	}
	serverID := player.CharServerID

	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("InteractItem: loading items for serverID=%d: %v", serverID, err)
		return nil
	}

	// A fishing pole used from the HUD (an item quick slot: UseItem(i, 1))
	// fishes (fishing.go): one in the hand stays there, one not yet worn is
	// put in it first. The item menu's Equip and Unequip stay a toggle.
	fishAfter := false
	if _, row, err := invItemByKey(items, sessionInvList(sess), key); err == nil &&
		interaction == 1 && row.Pattern.EquipSlot == wearFishingPole && h.fishing != nil {
		if row.EquipLoc == equipPole {
			h.fishing.StartFishing(sess)
			return nil
		}
		fishAfter = true
	}

	// A recipe scroll (one some recipe's scroll names) puts its recipes in
	// the book (craft.go), a spell scroll is scribed, and a meal or a drink
	// taken (consume.go), not worn.
	if idx, row, err := invItemByKey(items, sessionInvList(sess), key); err == nil {
		if row.EquipLoc < 0 && h.craft.UseScroll(sess, serverID, idx, row) {
			return nil
		}
		if isScroll(&row.Pattern) {
			h.scribe(sess, player, idx, row)
			return nil
		}
		if c, ok := consumableOf(&row.Pattern); ok {
			h.consume(sess, player, idx, row, c)
			return nil
		}
		// a potion is drunk or thrown (potion.go)
		if pe, ok := h.potionOf(&row.Pattern); ok && usesPotion(&pe, &row.Pattern, interaction) {
			h.usePotion(sess, player, idx, row, pe, target)
			return nil
		}
	}

	plan, err := planInteractItem(items, sessionInvList(sess), key)
	if err != nil {
		log.Printf("InteractItem: serverID=%d key=%d %q: %v", serverID, key, plan.name, err)
		return nil
	}
	if plan.equip {
		if _, row, err := invItemByKey(items, sessionInvList(sess), key); err == nil && !mayWear(sess, player, &row.Pattern) {
			return nil
		}
	}

	if !h.applyPlan(sess, serverID, plan, "InteractItem") {
		return nil
	}

	// Refresh the visual equipment bytes. The state channel is serialized from
	// the Player struct every tick, so this is what actually makes the change
	// show up on the character. (A test's handler may have no CharacterHandler.)
	if h.charHandler != nil {
		h.charHandler.applyEquipmentFromDB(player, serverID, false)
	}
	if fishAfter {
		h.fishing.StartFishing(sess)
	} else if _, row, err := invItemByKey(items, sessionInvList(sess), key); plan.equip && h.fishing != nil &&
		err == nil && row.Pattern.EquipSlot == wearFishingPole {
		// the item menu's Equip cannot open the window (the menu is up): say how
		fishSayGood(sess, fishHowTo)
	}
	return nil
}

// cannotWear is why the player may not put the item on, "" when it may:
// CLIENT's CanUse (0x00BCA8F0: the class mask, then the race mask, then the
// level), in the lines Sandstorm refuses an item with (scribe.go). CLIENT
// marks such an item in the item menu (CanUseListItem) but sends the request
// all the same, so the refusal is the server's; until 2026-10-29 it put on
// anything (the user: "though the bracers are level 42 my level 2 sk can
// equip them"). A class or race mask of 0 is not checked (ours): 2,189 of
// this server's wearable items, nearly all the old dump's, carry no class
// mask, which CLIENT would read as no class.
func cannotWear(player *entity.Player, p *database.ItemPatternRecord) string {
	switch {
	case p.ClassUse != 0 && p.ClassUse&(1<<uint(player.Class)) == 0:
		return scribeWrongClass
	case p.RaceUse != 0 && raceUseForWire(p.RaceUse)&(1<<uint(player.Race)) == 0:
		return scribeWrongRace
	case player.Level < p.LevelReq:
		return scribeTooLow
	}
	return ""
}

// mayWear is cannotWear's refusal said in Sandstorm's red, and whether the
// item may go on.
func mayWear(sess *session.Session, player *entity.Player, p *database.ItemPatternRecord) bool {
	why := cannotWear(player, p)
	if why == "" {
		return true
	}
	log.Printf("Equip: %s (level %d), %q (level %d): %s", player.Name, player.Level, p.ItemName, p.LevelReq, why)
	SendColoredChat(sess, why, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	return false
}

// applyPlan writes a plan's equip slots and tells the client: UnequipItem for
// the slot it vacates, then EquipItem. False when the database refused one.
func (h *EquipHandler) applyPlan(sess *session.Session, serverID uint32, plan equipPlan, who string) bool {
	for _, c := range plan.changes {
		if err := h.inventoryStore.SetEquipLoc(serverID, c.itemID, c.equipLoc); err != nil {
			log.Printf("%s: item %d -> equiploc %d: %v", who, c.itemID, c.equipLoc, err)
			return false
		}
	}
	if plan.unequip >= 0 {
		log.Printf("%s: serverID=%d UNEQUIP %q (key=%d) from slot %d",
			who, serverID, plan.name, plan.key, plan.unequip)
		sess.Queue().AddWithSequence(unequipItemMessage(plan.unequip))
	}
	if plan.equip {
		log.Printf("%s: serverID=%d EQUIP %q (key=%d, list index %d) to slot %d, %d displaced",
			who, serverID, plan.name, plan.key, plan.index, plan.slot, len(plan.changes)-1)
		sess.Queue().AddWithSequence(equipItemMessage(plan.index, plan.slot))
	}
	return true
}

// equipItemMessage is EquipItem (0x003F), which the client's dispatcher reads
// as two bytes for Msg003F: the item's LIST index, then the equip slot.
func equipItemMessage(index, equipLoc int) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeEquipItem)
	buf := make([]byte, 16)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeEquipItem))
	w.WriteByte(byte(index))
	w.WriteByte(byte(equipLoc))
	msg.SetData(buf[:w.Position()])
	return msg
}

// unequipItemMessage is UnequipItem (0x0040): one byte, the equip slot whose
// records Msg0040 (0x00BCB8A0) takes off.
func unequipItemMessage(equipLoc int) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUnequipItem)
	buf := make([]byte, 16)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeUnequipItem))
	w.WriteByte(byte(equipLoc))
	msg.SetData(buf[:w.Position()])
	return msg
}

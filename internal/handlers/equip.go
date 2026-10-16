package handlers

import (
	"log"

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
// Wire format, from the C# reference
// (Opcodes/Messages/Client/ClientInteractItem.cs ProcessItemInteraction) and
// confirmed against live payloads such as `4d000000ffffffff00`:
//
//	uint32  itemIndex        the item's `key`, i.e. charinventory.itemid, which
//	                         is what SendMemoryDump writes as the item key
//	uint32  target           0xFFFFFFFF in every observed request
//	7bit    interactionType  0 = equip/unequip toggle
//
// It is a TOGGLE: equipped -> unequip, not equipped -> equip. The client picks
// the slot itself only in the sense that it expects us to choose the item's
// natural one and tell it which we used.
type EquipHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	inventoryStore *database.InventoryStore
	charHandler    *CharacterHandler
}

// NewEquipHandler creates the handler.
func NewEquipHandler(sessionMgr *session.Manager, inv *database.InventoryStore, ch *CharacterHandler) *EquipHandler {
	return &EquipHandler{sessionManager: sessionMgr, inventoryStore: inv, charHandler: ch}
}

// SetGameWorld sets the game world reference.
func (h *EquipHandler) SetGameWorld(gw *entity.World) { h.gameWorld = gw }

// RegisterHandlers registers the equip handlers.
func (h *EquipHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeInteractItem, h.HandleInteractItem)
}

// itemSlotToEquipSlot maps an item's category (itempattern.equipslot, the C#
// ItemSlot enum) to the physical slot it occupies (charinventory.equiploc, the
// C# EquipSlot enum). The two are different enumerations and equal only by
// coincidence for chest.
//
// Every armor entry here is corroborated by real rows: coif 1->0, robe 2->21,
// earring 3->2/3, neck 4->4, tunic 5->5, bracers 7->8, ring 8->9/10, belt
// 9->11, leggings 10->12, boots 11->13, gloves 19->1.
//
// A nil second entry means the slot is unique; two entries means a pair, and we
// take whichever is free.
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
	16: {14},     // Bow         -> Primary
	17: {18},     // Thrown      -> Thrown
	18: {19, 20}, // Held        -> Held/Held2
	19: {1},      // Hand        -> Hand (gloves)
}

// HandleInteractItem toggles an item between equipped and not.
func (h *EquipHandler) HandleInteractItem(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 9 {
		log.Printf("InteractItem: short payload len=%d hex=%x", len(data), data)
		return nil
	}
	r := buffer.NewReader(data)
	key, e1 := r.ReadUint32()
	target, e2 := r.ReadUint32()
	interaction, e3 := r.Read7BitEncodedInt64()
	if e1 != nil || e2 != nil || e3 != nil {
		log.Printf("InteractItem: malformed payload hex=%x", data)
		return nil
	}

	if interaction != 0 {
		// The C# reference treats this as an error worth surfacing rather than
		// guessing at, and we have never seen a non-zero value on the wire.
		log.Printf("InteractItem: non-zero interaction type %d (key=%d target=0x%08X) — not handled",
			interaction, key, target)
		return nil
	}

	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}
	serverID := player.CharServerID

	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("InteractItem: loading items for serverID=%d: %v", serverID, err)
		return nil
	}

	// `key` is the item's POSITION in the client's inventory container, because
	// that is what SendMemoryDump writes as each row's key (the C# ClientIndex).
	// It is NOT charinventory.itemid. Build the same list — GetCharacterItems
	// order with bank rows (Location == 1) removed — and index straight into it.
	// This must stay in step with SendMemoryDump.
	inv := make([]*database.CharInventoryEntry, 0, len(items))
	for i := range items {
		if items[i].Location == 1 {
			continue // bank is a separate container with its own 0-based keys
		}
		inv = append(inv, &items[i])
	}
	idx, found := -1, (*database.CharInventoryEntry)(nil)
	if int(key) < len(inv) {
		idx, found = int(key), inv[key]
	}
	if found == nil {
		log.Printf("InteractItem: serverID=%d key=%d out of range (inventory has %d items)",
			serverID, key, len(inv))
		return nil
	}
	log.Printf("InteractItem: REQ key=%d -> invIndex=%d %q (equipslot=%d model=%d equiploc=%d)",
		key, idx, found.Pattern.ItemName, found.Pattern.EquipSlot,
		found.Pattern.Model, found.EquipLoc)

	slots, equippable := itemSlotToEquipSlot[found.Pattern.EquipSlot]
	if !equippable {
		log.Printf("InteractItem: %q (equipslot=%d) is not equippable — consumable?",
			found.Pattern.ItemName, found.Pattern.EquipSlot)
		return nil
	}

	if found.EquipLoc >= 0 {
		// Equipped -> unequip.
		old := found.EquipLoc
		if err := h.inventoryStore.SetEquipLoc(serverID, found.ItemID, -1); err != nil {
			log.Printf("InteractItem: unequip %q: %v", found.Pattern.ItemName, err)
			return nil
		}
		log.Printf("InteractItem: serverID=%d UNEQUIP %q (key=%d) from slot %d",
			serverID, found.Pattern.ItemName, key, old)
		h.sendUnequip(sess, old)
	} else {
		// Not equipped -> equip. Take the first free slot of the pair; if both
		// are taken, displace the first, which is what the client shows when a
		// player equips over a full pair.
		occupied := map[int]int{}
		for i := range items {
			if items[i].EquipLoc >= 0 {
				occupied[items[i].EquipLoc] = items[i].ItemID
			}
		}
		chosen := slots[0]
		for _, s := range slots {
			if _, taken := occupied[s]; !taken {
				chosen = s
				break
			}
		}
		if displaced, taken := occupied[chosen]; taken {
			if err := h.inventoryStore.SetEquipLoc(serverID, displaced, -1); err != nil {
				log.Printf("InteractItem: displacing item %d from slot %d: %v", displaced, chosen, err)
				return nil
			}
			log.Printf("InteractItem: serverID=%d displaced item %d from slot %d", serverID, displaced, chosen)
			h.sendUnequip(sess, chosen)
		}
		if err := h.inventoryStore.SetEquipLoc(serverID, found.ItemID, chosen); err != nil {
			log.Printf("InteractItem: equip %q: %v", found.Pattern.ItemName, err)
			return nil
		}
		log.Printf("InteractItem: serverID=%d EQUIP %q (key=%d, model=%d) to slot %d",
			serverID, found.Pattern.ItemName, key, found.Pattern.Model, chosen)
		h.sendEquip(sess, idx, chosen)
	}

	// Refresh the visual equipment bytes. The state channel is serialized from
	// the Player struct every tick, so this is what actually makes the change
	// show up on the character.
	h.charHandler.applyEquipmentFromDB(player, serverID)
	return nil
}

// sendEquip sends EquipItem (0x003F): the item's inventory index, then the slot
// it now occupies. Matches ServerEquipItem.ProcessServerEquipItem.
func (h *EquipHandler) sendEquip(sess *session.Session, index, equipLoc int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeEquipItem)
	buf := make([]byte, 16)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeEquipItem))
	w.WriteByte(byte(index))
	w.WriteByte(byte(equipLoc))
	msg.SetData(buf[:w.Position()])
	sess.Queue().AddWithSequence(msg)
}

// sendUnequip sends UnequipItem (0x0040): just the slot that was vacated.
// Matches ServerUnequipItem.ProcessServerUnequipItem.
func (h *EquipHandler) sendUnequip(sess *session.Session, equipLoc int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUnequipItem)
	buf := make([]byte, 16)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeUnequipItem))
	w.WriteByte(byte(equipLoc))
	msg.SetData(buf[:w.Position()])
	sess.Queue().AddWithSequence(msg)
}

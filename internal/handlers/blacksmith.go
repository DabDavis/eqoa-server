package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The blacksmith's repair window [A]: no capture holds one.
//
//	C>S 0x0061  u32 the blacksmith               InteractWithTarget [V]
//	S>C 0x0061  u32 → Msg0061 [V]: idle, +0x22B78 ← it and the window opens
//	C>S 0x0062  u32 key, u32 +0x22B78            Send0062 0x00BCCC70 [V], one item a request
//	S>C 0x0060  u32 key, zz condition → Msg0060 [V]: Find(key), Op0060, sound 1, refresh
//	C>S 0x0063  (empty)                          Send0063 0x00BCCB88 [V]: +0x22B78 ← -1 first
//
// The window shows each item's GetInvItemRepairCost (0x00BCCC00 [V]):
// (int)(value × (1 − condition)). A repair costs that and restores the item
// to its pattern's maximum; 0x0052 then carries the purse. The request has no
// pending flag, so a refused repair needs no answer. Sandstorm's 0x0060 also
// reports wear in combat (ps2_main_capture), which is not this window's.

// adjustItemHPPayload is 0x0060: an item's condition, by key.
func adjustItemHPPayload(key uint32, hp int) []byte {
	return build(protocol.OpcodeAdjustItemHP, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.Write7BitEncodedInt64(int64(hp))
	})
}

// blacksmithPayload is 0x0061: the id the client hands back with each repair.
func blacksmithPayload(id uint32) []byte {
	return build(protocol.OpcodeBlackSmithMenu, 8, func(w *buffer.Writer) {
		w.WriteUint32(id)
	})
}

// repairCost is GetInvItemRepairCost.
func repairCost(value, cur, max int) int {
	return int(float32(value) * (1 - itemCondition(cur, max)))
}

// HandleBlacksmithOpen is 0x0061, u32 the blacksmith: answered with the same
// id, which opens the window.
func (h *NPCWindowHandler) HandleBlacksmithOpen(sess *session.Session, msg *protocol.Message) error {
	id, err := buffer.NewReader(msg.Data()).ReadUint32()
	if err != nil {
		return nil
	}
	player, npc, ok := h.windowNPC(sess, id, npcFlagBlacksmith, "Blacksmith")
	if !ok {
		return nil
	}
	queueReliable(sess, protocol.OpcodeBlackSmithMenu, blacksmithPayload(id))
	log.Printf("Blacksmith: %s opened %q", player.Name, npc.Name)
	return nil
}

// HandleRepair is 0x0062: u32 the item's key, u32 the blacksmith.
func (h *NPCWindowHandler) HandleRepair(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	key, e1 := r.ReadUint32()
	id, e2 := r.ReadUint32()
	if e1 != nil || e2 != nil {
		return nil
	}
	player, npc, ok := h.windowNPC(sess, id, npcFlagBlacksmith, "Repair")
	inv := sessionInvList(sess)
	if !ok || h.inv == nil || inv == nil {
		return nil
	}
	rec, found := inv.at(inv.index(key))
	if !found {
		log.Printf("Repair: %s asks for key %d, which the inventory does not hold", player.Name, key)
		return nil
	}
	row, err := h.inv.GetCharItem(player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("Repair: %s: %v", player.Name, err)
		return nil
	}
	p := &row.Pattern
	if p.MaxHP <= 0 || row.RemainHP >= p.MaxHP {
		return nil // nothing to mend
	}
	cost := repairCost(p.ItemCost, row.RemainHP, p.MaxHP)
	if cost > player.Tunar {
		log.Printf("Repair: %s cannot afford %q (%d > %d)", player.Name, p.ItemName, cost, player.Tunar)
		return nil
	}
	if err := h.inv.SetItemRemainHP(player.CharServerID, rec.itemID, p.MaxHP); err != nil {
		log.Printf("Repair: %s: %v", player.Name, err)
		return nil
	}
	player.Tunar -= cost
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodeAdjustItemHP, adjustItemHPPayload(key, p.MaxHP))
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	log.Printf("Repair: %q mended %s's %q (%d/%d) for %d", npc.Name, player.Name, p.ItemName, row.RemainHP, p.MaxHP, cost)
	return nil
}

// HandleBlacksmithClose is 0x0063: the client has closed the window itself.
func (h *NPCWindowHandler) HandleBlacksmithClose(sess *session.Session, msg *protocol.Message) error {
	return nil
}

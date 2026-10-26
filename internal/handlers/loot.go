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

// Looting a corpse, as CLIENT asks for it:
//
//   - 0x0015 {u32 id}, the attack button on a target whose channel carries
//     the corpse bit (ToggleAutoAttack: byte 26's 2, which a creature's
//     channel takes at its death, entity/visibility.go), is the game's way,
//     as Sandstorm answers it: the corpse opened (HandleLootCorpse). One out
//     of reach is answered "Out of range." (ours: no record of Sandstorm's);
//     one that is no corpse, or gone, 0x0085 2.
//   - 0x0004 (InteractWithTarget's default) on a dead NPC opens its corpse too
//     (InteractWithTarget sends nothing for a creature the game shows as
//     attackable, byte 142 2 or 3, so this is not the game's way for those):
//     0x0018, the corpse as a VIItemList (buildLoot), each record keyed by
//     its position. A client not in play answers 0x0016 at once
//     (LootNoWindow).
//   - 0x0019 {u32 key, u32 count} (LootOne 0x00BCAB?? → SendLoot(25, ...)):
//     that record into the inventory (0x003B), and the corpse sent again
//     (0x0016, then 0x0018 while anything is left). CLIENT's 0x0018 asserts
//     no corpse is held, so 0x0016 always goes first.
//   - 0x001A, no payload (RequestLootAll): 0x001A back, and CLIENT moves every
//     record it has room for itself (Msg001A 0x00BD98B0: AddItem, then Took on
//     the corpse); the server mirrors that in the database and the list, then
//     closes (0x0016) or sends what is left.
//   - 0x0016, no payload (LootNoWindow): 0x0016 back, which is what makes
//     CLIENT let go of the corpse (HaveCorpse = 0) and leave the window.
//
// A corpse's items are its NPC's npcitem rows (the same table a merchant's
// wares are in), taken when it is first opened after a death and gone with
// the corpse. Anyone may loot a corpse. One with nothing on it opens too, as
// the game's "Empty" (word 641, CLIENT opens the window whatever the count),
// and a corpse whose window closes with nothing left on it goes at once
// (World.ClearCorpse: out of every view, its creature's respawn time running
// from then; F3 20261025-044229); one left with items lies until it decays
// (entity/corpse.go), and every window open on a corpse that goes is closed.
// A corpse already gone answers 0x0085 2, "That entity is not lootable"
// (word 674): the client may show one for a visibility round after.

// corpse is one dead NPC's loot.
type corpse struct {
	died  time.Time // the death it is the loot of
	items []database.CharInventoryEntry
	tunar int // its coins until the first opener takes them (corpsecoins.go)
}

// lootState is the corpses and who has which one open.
type lootState struct {
	mu      sync.Mutex
	corpses map[entity.EntityID]*corpse
	open    map[*session.Session]entity.EntityID
	drops   map[entity.EntityID]*questDrops // loot steps' drops (questloot.go)
}

func (h *NPCWindowHandler) loots() *lootState {
	h.lootOnce.Do(func() {
		h.loot = &lootState{corpses: map[entity.EntityID]*corpse{}, open: map[*session.Session]entity.EntityID{},
			drops: map[entity.EntityID]*questDrops{}}
	})
	return h.loot
}

// closeLootPayload is 0x0016, no payload: LootClose, and the corpse forgotten.
func closeLootPayload() []byte {
	return build(protocol.OpcodeClientCloseLoot, 2, nil)
}

// OpenCorpse is 0x0004 on a dead NPC (InteractHandler hands it over): its
// corpse, if it has anything on it, as 0x0018. It reports whether the NPC was
// a corpse.
func (h *NPCWindowHandler) OpenCorpse(sess *session.Session, npc *entity.NPC) bool {
	if !npc.Dead || h.inv == nil {
		return false
	}
	died, lies := h.corpseOf(npc)
	// a pet's corpse carries nothing and is no one's to loot (pet.go)
	if !lies || npc.Owner != 0 {
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootNotLootable))
		return true
	}
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c := ls.corpses[npc.ServerID]
	if c == nil || !c.died.Equal(died) {
		items, err := h.inv.GetNPCItems(npc.NPCID)
		if err != nil {
			log.Printf("Loot: %q (npc %d): %v", npc.Name, npc.NPCID, err)
		}
		for i := range items {
			items[i] = catalogRecord(items[i])
		}
		c = &corpse{died: died, items: items, tunar: corpseCoins(npc, h.coinRoll)}
		ls.corpses[npc.ServerID] = c
	}
	ls.open[sess] = npc.ServerID
	h.giveCorpseCoins(sess, npc, c.tunar)
	c.tunar = 0
	view := ls.viewLocked(npc.ServerID, c, h.charOf(sess))
	SendLoot(sess, view)
	log.Printf("Loot: %s opened %q: %d items", sessName(h, sess), npc.Name, len(view))
	return true
}

// corpseOf is whether the NPC's corpse lies to be looted, and the death it
// is the corpse of (the World's CorpseLies; without a world, its death).
func (h *NPCWindowHandler) corpseOf(npc *entity.NPC) (time.Time, bool) {
	if h.corpseLies != nil {
		return h.corpseLies(npc.ServerID)
	}
	return npc.DeathTime, npc.Dead
}

// emptiedLocked is a corpse whose window closed with nothing left on it:
// the World takes it away (ClearCorpse) and every other window open on it
// is closed. Must be called with ls.mu held.
func (h *NPCWindowHandler) emptiedLocked(ls *lootState, id entity.EntityID, c *corpse) {
	if h.clearCorpse == nil || !h.clearCorpse(id, c.died) {
		return
	}
	h.corpseGoneLocked(ls, id)
}

// corpseGoneLocked forgets a corpse that went and closes every window open
// on it. Must be called with ls.mu held.
func (h *NPCWindowHandler) corpseGoneLocked(ls *lootState, id entity.EntityID) {
	for s, open := range ls.open {
		if open == id {
			delete(ls.open, s)
			queueReliable(s, protocol.OpcodeClientCloseLoot, closeLootPayload())
		}
	}
	delete(ls.corpses, id)
	delete(ls.drops, id)
}

// CorpsesGone is the World's hook for corpses that decayed (OnCorpseGone):
// each forgotten, and the windows open on it closed.
func (h *NPCWindowHandler) CorpsesGone(ids []entity.EntityID) {
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, id := range ids {
		h.corpseGoneLocked(ls, id)
	}
}

func sessName(h *NPCWindowHandler, sess *session.Session) string {
	if h.playerOf != nil {
		if p := h.playerOf(sess); p != nil {
			return p.Name
		}
	}
	return sess.RemoteAddr().String()
}

// resend is the corpse again after a change: 0x0016, then 0x0018 while
// anything is left; else the corpse is closed for this session, and emptied
// and closed it goes (emptiedLocked).
func (h *NPCWindowHandler) resendCorpse(sess *session.Session, ls *lootState, c *corpse) {
	queueReliable(sess, protocol.OpcodeClientCloseLoot, closeLootPayload())
	id, open := ls.open[sess]
	view := ls.viewLocked(id, c, h.charOf(sess))
	if len(view) == 0 {
		delete(ls.open, sess)
		if open && ls.emptyLocked(id, c) {
			h.emptiedLocked(ls, id, c)
		}
		return
	}
	SendLoot(sess, view)
}

// HandleLootCorpse is 0x0015 {u32 id}: the attack button on a corpse
// (CLIENT's ToggleAutoAttack). The corpse opens as talking to it does
// (OpenCorpse) when it lies within reach: InteractRange from its centre, and a
// large or huge creature's allowance beyond (entity.NPC.ReachExtra), so a
// dragon's corpse is reached where its body was fought.
func (h *NPCWindowHandler) HandleLootCorpse(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.find == nil || h.playerOf == nil {
		return nil
	}
	id, _ := buffer.NewReader(data).ReadUint32()
	player := h.playerOf(sess)
	if player == nil {
		return nil
	}
	ent, found := h.find(sess, id)
	npc, isNPC := ent.(*entity.NPC)
	if !found || !isNPC || !npc.Dead {
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootNotLootable))
		return nil
	}
	if reach := entity.InteractRange + npc.ReachExtra(); !entity.InRange(player, npc, reach) {
		log.Printf("Loot: %s too far from %q's corpse (%.1f > %.1f)", player.Name, npc.Name,
			entity.DistanceBetween(player, npc), reach)
		SendColoredChat(sess, "Out of range.", 255, 63, 63)
		return nil
	}
	h.OpenCorpse(sess, npc)
	return nil
}

// HandleLootClose is 0x0016 (LootNoWindow): answered with 0x0016. A corpse
// closed with nothing left on it goes (emptiedLocked).
func (h *NPCWindowHandler) HandleLootClose(sess *session.Session, msg *protocol.Message) error {
	ls := h.loots()
	ls.mu.Lock()
	id, open := ls.open[sess]
	delete(ls.open, sess)
	queueReliable(sess, protocol.OpcodeClientCloseLoot, closeLootPayload())
	if c := ls.corpses[id]; open && c != nil && ls.emptyLocked(id, c) {
		h.emptiedLocked(ls, id, c)
	}
	ls.mu.Unlock()
	return nil
}

// HandleLootItem is 0x0019 {u32 key, u32 count} (SendLoot): the record with
// that key into the inventory, as many as it holds.
func (h *NPCWindowHandler) HandleLootItem(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	key, e1 := r.ReadUint32()
	n, e2 := r.ReadUint32()
	if e1 != nil || e2 != nil {
		return nil
	}
	var looted []lootTake
	var player *entity.Player
	defer func() { tellQuestsLooted(sess, player, looted) }() // after the lock (questloot.go)
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c, player, inv := h.openCorpse(sess, ls)
	if c == nil {
		queueReliable(sess, protocol.OpcodeClientCloseLoot, closeLootPayload())
		return nil
	}
	id, char := ls.open[sess], player.CharServerID
	view := ls.viewLocked(id, c, char)
	i := int(key)
	if i < 0 || i >= len(view) || n == 0 {
		h.resendCorpse(sess, ls, c)
		return nil
	}
	rec := view[i]
	count := min(int(n), rec.StackLeft)
	took := inv.wouldAdd(rec.Pattern.PatternID, count, rec.Pattern.MaxStack)
	if took > 0 {
		out, k, err := h.giveItem(player, inv, rec, took, 0)
		if err != nil {
			log.Printf("Loot: %s: %v", player.Name, err)
		} else {
			queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, k))
			ls.tookLocked(id, c, char, i, took)
			looted = append(looted, lootTake{rec.Pattern.PatternID, took})
			log.Printf("Loot: %s took %d %q", player.Name, took, rec.Pattern.ItemName)
		}
	}
	h.resendCorpse(sess, ls, c)
	return nil
}

// HandleLootAll is 0x001A (RequestLootAll): 0x001A back, and the server does
// what Msg001A does on the client, record by record.
func (h *NPCWindowHandler) HandleLootAll(sess *session.Session, msg *protocol.Message) error {
	var looted []lootTake
	var player *entity.Player
	defer func() { tellQuestsLooted(sess, player, looted) }() // after the lock (questloot.go)
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c, player, inv := h.openCorpse(sess, ls)
	if c == nil {
		queueReliable(sess, protocol.OpcodeClientCloseLoot, closeLootPayload())
		return nil
	}
	queueReliable(sess, protocol.OpcodeLootTakeAll, buildLootTakeAll())
	id, char := ls.open[sess], player.CharServerID
	for i := 0; ; {
		view := ls.viewLocked(id, c, char)
		if i >= len(view) {
			break
		}
		rec := view[i]
		took := inv.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack)
		if took > 0 {
			if _, _, err := h.giveItem(player, inv, rec, took, 0); err != nil {
				log.Printf("Loot: %s: %v", player.Name, err)
				took = 0
			}
		}
		if took > 0 {
			ls.tookLocked(id, c, char, i, took)
			looted = append(looted, lootTake{rec.Pattern.PatternID, took})
		}
		if took != rec.StackLeft {
			i++
		}
	}
	log.Printf("Loot: %s took everything; %d records left", player.Name, len(ls.viewLocked(id, c, char)))
	h.resendCorpse(sess, ls, c)
	return nil
}

// openCorpse is the session's open corpse, its player and inventory list, or
// nils (with the session's corpse forgotten).
func (h *NPCWindowHandler) openCorpse(sess *session.Session, ls *lootState) (*corpse, *entity.Player, *invList) {
	id, ok := ls.open[sess]
	if !ok || h.playerOf == nil || h.inv == nil {
		return nil, nil, nil
	}
	c := ls.corpses[id]
	player := h.playerOf(sess)
	inv := sessionInvList(sess)
	if c == nil || player == nil || inv == nil {
		delete(ls.open, sess)
		return nil, nil, nil
	}
	return c, player, inv
}

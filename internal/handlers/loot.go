package handlers

import (
	"fmt"
	"log"
	"math/rand"
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
// the corpse. A corpse is its kill group's to loot, one looter at a time
// (lootrights.go). One with nothing on it opens too, as
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
	// raid is its raid's hold on it: rolls, winners, a master looter (raidloot.go);
	// nil for every other corpse
	raid *raidCorpse
}

// lootState is the corpses and who has which one open.
type lootState struct {
	mu      sync.Mutex
	corpses map[entity.EntityID]*corpse
	open    map[*session.Session]entity.EntityID
	since   map[*session.Session]time.Time  // each open window's last opening or take (lootrights.go)
	drops   map[entity.EntityID]*questDrops // loot steps' drops (questloot.go)
	// rolls are a raid's need before greed rolls going on, by id, and nextRoll the
	// last id given (raidloot.go)
	rolls    map[int]*lootRoll
	nextRoll int
	// listed is the corpse each master looter was last listed, by character
	listed map[uint32]entity.EntityID
}

func (h *NPCWindowHandler) loots() *lootState {
	h.lootOnce.Do(func() {
		h.loot = &lootState{corpses: map[entity.EntityID]*corpse{}, open: map[*session.Session]entity.EntityID{},
			since: map[*session.Session]time.Time{},
			drops: map[entity.EntityID]*questDrops{}, rolls: map[int]*lootRoll{}}
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
	// a pet's corpse carries nothing and is no one's to loot (pet.go), nor
	// is a creature's the town guards hit (entity/guards.go: jib's "no loot
	// or xp should be given to the player if the guards hit it")
	if !lies || npc.Owner != 0 || npc.CorpseOfNothing() || (h.guardsKill != nil && h.guardsKill(npc.ServerID)) { // an add's, one dead with its master (entity/masterdeath.go)
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootNotLootable))
		return true
	}
	// a Metal Moogie's is his challenger's alone (the GM Jibrish's petition 1379: "If they defeat
	// it they can loot a single "Moogie Tribute""; entity/metalmoogie.go)
	if h.duelLoot != nil {
		if duel, mine, line := h.duelLoot(sess, npc); duel && !mine {
			SendColoredChat(sess, line, 255, 63, 63)
			queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootNotLootable))
			return true
		}
	}
	// the kill group's alone (lootrights.go)
	var opener *entity.Player
	if h.playerOf != nil {
		opener = h.playerOf(sess)
	}
	if !h.mayLoot(opener, npc.ServerID, died) {
		log.Printf("Loot: %s may not open %q's corpse: another's kill", sessName(h, sess), npc.Name)
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootAnotherOwner))
		return true
	}
	var won []winTake
	defer func() { tellWinsQuests(won) }() // after the lock: a raid's won items taken (raidloot.go)
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	h.takeRaidKillLocked(ls, npc, died) // a raid's kill, rolled at its death (raidloot.go)
	c := ls.corpses[npc.ServerID]
	if c == nil || !c.died.Equal(died) {
		c = h.rollCorpse(npc, died, h.openerLevel(sess))
		ls.corpses[npc.ServerID] = c
	}
	answered := false
	if answered, won = h.raidOpenLocked(sess, ls, npc, c); answered { // its raid's rules first (raidloot.go)
		return true
	}
	now := h.lootClock()
	if holder, held := h.heldByOtherLocked(ls, sess, npc, now); held { // one looter at a time (lootrights.go)
		log.Printf("Loot: %s may not open %q's corpse: %s is looting it", sessName(h, sess), npc.Name, holder)
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootBeingLooted))
		return true
	}
	ls.open[sess] = npc.ServerID
	ls.since[sess] = now
	h.giveCorpseCoins(sess, npc, c.tunar)
	c.tunar = 0
	view := ls.viewLocked(npc.ServerID, c, h.charOf(sess))
	SendLoot(sess, view)
	h.raidOpenedLocked(sess, ls, npc, c)
	log.Printf("Loot: %s opened %q: %d items", sessName(h, sess), npc.Name, len(view))
	return true
}

// rollCorpse is a corpse's loot at the death died, rolled for an opener of level (0:
// anyone): its NPC's own items, its loot tables, its categories, and its coins.
func (h *NPCWindowHandler) rollCorpse(npc *entity.NPC, died time.Time, level int) *corpse {
	items, err := h.inv.GetNPCItems(npc.NPCID)
	if err != nil {
		log.Printf("Loot: %q (npc %d): %v", npc.Name, npc.NPCID, err)
	}
	// equipment no more than lootLevelReach above the opener's level
	items, cut := fitItems(items, level)
	// and what its loot tables roll at this death (loottables, npcloot),
	// then the tables every creature's corpse rolls (npcloot's "*")
	for _, name := range lootNames(npc) {
		if rolls, err := h.inv.NPCLootRolls(name, npc.Level); err != nil {
			log.Printf("Loot: %q's loot tables (%q): %v", npc.Name, name, err)
		} else if len(rolls) > 0 {
			for i := range rolls {
				var n int
				rolls[i].Choices, n = fitChoices(rolls[i].Choices, level)
				cut += n
			}
			got := rollLoot(rolls, h.lootIntn)
			items = append(items, got...)
			log.Printf("Loot: %q (level %d) rolled %d of %q's tables' items", npc.Name, npc.Level, len(got), name)
		}
	}
	// and a roll a category (lootcategory.go), but for a pet's, a
	// charmed creature's or a training dummy's corpse
	if rollsCategories(npc) {
		if rolls, err := h.inv.CategoryRolls(npc.Name, npc.Level); err != nil {
			log.Printf("Loot: %q's loot categories: %v", npc.Name, err)
		} else if len(rolls) > 0 {
			for i := range rolls {
				var n int
				rolls[i].Choices, n = fitChoices(rolls[i].Choices, level)
				cut += n
			}
			got := rollCategories(rolls, h.lootFloat, h.lootIntn)
			items = append(items, got...)
			log.Printf("Loot: %q (level %d) rolled %d items of its categories", npc.Name, npc.Level, len(got))
		}
	}
	if cut > 0 {
		log.Printf("Loot: %q rolled for a level %d opener: %d pieces of equipment above level %d left out",
			npc.Name, level, cut, level+lootLevelReach)
	}
	for i := range items {
		items[i] = catalogRecord(items[i])
	}
	return &corpse{died: died, items: items, tunar: corpseCoins(npc, h.coinRoll)}
}

// lootEveryCreature is the npcloot name whose tables every creature's corpse
// rolls, after its own: the modifier drops of the tradeskills (content in
// data: a GM's or a tool's rows), and anything else a GM gives every kill.
const lootEveryCreature = "*"

// lootNames is the npcloot names a corpse rolls the tables of, in order: its
// own, then every creature's, which a pet's, a charmed creature's (whose
// corpse is no one's: OpenCorpse) and a training dummy's do not roll.
func lootNames(npc *entity.NPC) []string {
	if npc.Owner != 0 || npc.Charm != nil || npc.Dummy != entity.NotDummy || npc.DuelSpawn() {
		return []string{npc.Name}
	}
	return []string{npc.Name, lootEveryCreature}
}

// lootLevelReach is how far above its opener's level a piece of equipment a
// corpse rolls may be (the user, 2026-11-07: "equipment drops from mobs for
// players should be within the players level range, with in 10 levels
// higher/lower"). Gear of any level below the opener's stays (jib, 2026-11-08,
// of Cazic-Thule's Rubicite armour, levels 47-48, which no level 59 or 60 could
// find: "level 60 players should absolutely be able to farm the gear", and the
// choice of no lower bound anywhere). A corpse is rolled when it is first
// opened (OpenCorpse), so by that player's level. Equipment is what is worn in the gear's slots, 1 to
// lootGearSlots (armour, jewellery, weapons, bows, held items, gloves) but a
// thrown potion; the fishing pole, bait, the Book of Power and the tradeskill
// tools (slots 20-25) are not, nor is anything else (potions, materials,
// quest items). An item with no level of its own (10 droppable pieces) is not
// judged.
const lootLevelReach = 10

// lootGearSlots is the last equip slot of the gear (19, the gloves).
const lootGearSlots = 19

// lootFits reports an item a level's opener may find on a corpse (level 0:
// no opener known, anything).
func lootFits(p *database.ItemPatternRecord, level int) bool {
	if level <= 0 || p.EquipSlot < 1 || p.EquipSlot > lootGearSlots || p.AttackType == 10 || p.LevelReq <= 0 {
		return true
	}
	return p.LevelReq <= level+lootLevelReach
}

// fitChoices is a roll's choices a level's opener may find, and how many it
// left out: a roll then lands only on these, by their weights (none left: the
// table gives nothing).
func fitChoices(cs []database.LootChoice, level int) ([]database.LootChoice, int) {
	out := make([]database.LootChoice, 0, len(cs))
	for _, c := range cs {
		if lootFits(&c.Item.Pattern, level) {
			out = append(out, c)
		}
	}
	return out, len(cs) - len(out)
}

// fitItems is a creature's own items a level's opener may find, and how many
// it left out.
func fitItems(items []database.CharInventoryEntry, level int) ([]database.CharInventoryEntry, int) {
	out := items[:0:0]
	for _, it := range items {
		if lootFits(&it.Pattern, level) {
			out = append(out, it)
		}
	}
	return out, len(items) - len(out)
}

// openerLevel is the level of the player sess plays, or 0.
func (h *NPCWindowHandler) openerLevel(sess *session.Session) int {
	if h.playerOf == nil {
		return 0
	}
	if p := h.playerOf(sess); p != nil {
		return p.Level
	}
	return 0
}

// rollLoot is what a creature's loot tables give its corpse at a death: each
// table rolled its rolls times, each roll at its chance (percent) taking one
// of the table's items for the creature's level by weight. intn is the dice
// (rand.Intn when nil).
func rollLoot(rolls []database.LootRoll, intn func(int) int) []database.CharInventoryEntry {
	if intn == nil {
		intn = rand.Intn
	}
	var out []database.CharInventoryEntry
	for _, r := range rolls {
		total := 0
		for _, c := range r.Choices {
			total += max(c.Weight, 0)
		}
		if total == 0 {
			continue
		}
		for n := 0; n < r.Rolls; n++ {
			if intn(100) >= r.Chance {
				continue
			}
			pick := intn(total)
			for _, c := range r.Choices {
				if pick -= max(c.Weight, 0); pick < 0 {
					out = append(out, c.Item)
					break
				}
			}
		}
	}
	return out
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
			delete(ls.since, s)
			queueReliable(s, protocol.OpcodeClientCloseLoot, closeLootPayload())
		}
	}
	h.raidCorpseGoneLocked(ls, id) // its rolls end with it (raidloot.go)
	delete(ls.corpses, id)
	delete(ls.drops, id)
	h.killRights.Delete(id) // its kill group with it (lootrights.go)
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
		delete(ls.since, sess)
		if open && ls.emptyLocked(id, c) {
			h.emptiedLocked(ls, id, c)
		}
		return
	}
	if open {
		ls.since[sess] = h.lootClock() // a take: still looting (lootrights.go)
	}
	SendLoot(sess, view)
	h.masterListLocked(sess, c) // after every 0x0018 to its master looter (raidloot.go)
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
	delete(ls.since, sess)
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
	room := inv.wouldAdd(rec.Pattern.PatternID, count, rec.Pattern.MaxStack)
	took := inv.loreRoom(&rec.Pattern, room)
	if took == 0 && room > 0 { // a Lore item held already (itemlist.go)
		SendClientMessage(sess, fmt.Sprintf(loreLine, rec.Pattern.ItemName))
		log.Printf("Loot: %s holds a %q already (Lore)", player.Name, rec.Pattern.ItemName)
		return nil
	}
	if took == 0 {
		// no room: said, and the window left as it is (resent, it closed
		// and opened again and said nothing: petition 1030)
		SendClientMessage(sess, lootNoRoom)
		log.Printf("Loot: %s has no room for %q", player.Name, rec.Pattern.ItemName)
		return nil
	}
	if took < count {
		SendClientMessage(sess, lootNoRoom) // the rest of the stack stays
	}
	if took > 0 {
		out, k, err := h.giveItem(player, inv, rec, took, 0)
		if err != nil {
			log.Printf("Loot: %s: %v", player.Name, err)
		} else {
			queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, k, readsCapes(sess)))
			ls.tookLocked(id, c, char, i, took)
			looted = append(looted, lootTake{rec.Pattern.PatternID, took})
			h.tellLooted(sess, player.Name, rec, took)
			h.tellGroupLooted(player, rec, took)
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
	id, char := ls.open[sess], player.CharServerID
	if loreStopsTakeAll(inv, ls.viewLocked(id, c, char)) {
		looted = h.lootAllByOne(sess, ls, c, player, inv)
		return nil
	}
	queueReliable(sess, protocol.OpcodeLootTakeAll, buildLootTakeAll())
	left := false // a record or part of one stays for want of room
	for i := 0; ; {
		view := ls.viewLocked(id, c, char)
		if i >= len(view) {
			break
		}
		rec := view[i]
		took := inv.loreRoom(&rec.Pattern, inv.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack))
		if took > 0 {
			if _, _, err := h.giveItem(player, inv, rec, took, 0); err != nil {
				log.Printf("Loot: %s: %v", player.Name, err)
				took = 0
			}
		}
		if took > 0 {
			ls.tookLocked(id, c, char, i, took)
			looted = append(looted, lootTake{rec.Pattern.PatternID, took})
			h.tellLooted(sess, player.Name, rec, took)
			h.tellGroupLooted(player, rec, took)
		}
		if took != rec.StackLeft {
			left = true
			i++
		}
	}
	log.Printf("Loot: %s took everything; %d records left", player.Name, len(ls.viewLocked(id, c, char)))
	if left {
		SendClientMessage(sess, lootNoRoom)
	}
	if len(looted) == 0 {
		// nothing taken: Msg001A took nothing either (the same rule), so
		// the window is as it was; resent, it only blinked
		return nil
	}
	h.resendCorpse(sess, ls, c)
	return nil
}

// loreStopsTakeAll is whether a take-all of these records would leave one on
// the corpse under its Lore flag: one held already, or a second of the same on
// the corpse. CLIENT's Msg001A knows no Lore (VIItemList::AddItem keeps every
// record it has room for), so a 0x001A back would put the refused item in the
// client's list, not the server's, and leave it on the corpse to be "taken"
// again (petition 1498, Waldorf: "it still puts that item in your inventory
// and also leaves it on the corpse").
func loreStopsTakeAll(inv *invList, view []database.CharInventoryEntry) bool {
	seen := map[int]bool{}
	for i := range view {
		p := &view[i].Pattern
		if p.Lore == 0 {
			continue
		}
		if seen[p.PatternID] || inv.loreHeld(p.PatternID) {
			return true
		}
		seen[p.PatternID] = true
	}
	return false
}

// lootAllByOne is a take-all with no 0x001A back: each record the character may
// take comes as a single take's does (0x0016), a Lore one held already stays
// and is said once, and the window is sent again with what is left. Must be
// called with the loot lock held; what was taken is returned for the quests.
func (h *NPCWindowHandler) lootAllByOne(sess *session.Session, ls *lootState, c *corpse, player *entity.Player, inv *invList) []lootTake {
	var looted []lootTake
	id, char := ls.open[sess], player.CharServerID
	left, said := false, map[int]bool{}
	for i := 0; ; {
		view := ls.viewLocked(id, c, char)
		if i >= len(view) {
			break
		}
		rec := view[i]
		room := inv.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack)
		took := inv.loreRoom(&rec.Pattern, room)
		switch {
		case took == 0 && room > 0:
			if !said[rec.Pattern.PatternID] {
				said[rec.Pattern.PatternID] = true
				SendClientMessage(sess, fmt.Sprintf(loreLine, rec.Pattern.ItemName))
			}
		case took < rec.StackLeft:
			left = true
		}
		if took > 0 {
			out, k, err := h.giveItem(player, inv, rec, took, 0)
			if err != nil {
				log.Printf("Loot: %s: %v", player.Name, err)
				took = 0
			} else {
				queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, k, readsCapes(sess)))
				ls.tookLocked(id, c, char, i, took)
				looted = append(looted, lootTake{rec.Pattern.PatternID, took})
				h.tellLooted(sess, player.Name, rec, took)
				h.tellGroupLooted(player, rec, took)
			}
		}
		if took != rec.StackLeft {
			i++
		}
	}
	log.Printf("Loot: %s took everything but a Lore item held; %d records left", player.Name, len(ls.viewLocked(id, c, char)))
	if left {
		SendClientMessage(sess, lootNoRoom)
	}
	h.resendCorpse(sess, ls, c)
	return looted
}

// tellLooted says what a corpse gave, unless the client's Loot Messages option
// is off (faction.go). The words are ours, after the coins' Sandstorm line
// (corpsecoins.go): no recording shows an item's.
//
// The item is a link to a client that takes links (lootlinks.go), the line otherwise as
// it was.
func (h *NPCWindowHandler) tellLooted(sess *session.Session, who string, rec database.CharInventoryEntry, n int) {
	if !sess.LootMessagesOff() {
		h.lootStackLinked(who, rec, n).client(sess)
	}
}

// tellGroupLooted tells the looter's group what it took, each member by its own Loot
// Messages option (petition 1145, Larrybird: "i cant see what my other grp is loooting
// could be uber rare gear oooo no").
func (h *NPCWindowHandler) tellGroupLooted(looter *entity.Player, rec database.CharInventoryEntry, n int) {
	if h.groupOf == nil || looter == nil {
		return
	}
	var line *serverLine // one line, its link made once, for the whole group
	for _, s := range h.groupOf(looter) {
		if !s.LootMessagesOff() {
			if line == nil {
				line = h.groupLootLinked(looter.Name, rec, n)
			}
			line.client(s)
		}
	}
}

// groupLootLinked is groupLootLine with the item linked (lootlinks.go).
func (h *NPCWindowHandler) groupLootLinked(name string, rec database.CharInventoryEntry, n int) *serverLine {
	if n > 1 {
		return h.linkedf(name, "%s receives %d %s from the corpse.", name, n, lootItem(rec, n))
	}
	return h.linkedf(name, "%s receives %s from the corpse.", name, lootItem(rec, n))
}

// lootStackLinked is lootStackLine with the item linked (lootlinks.go).
func (h *NPCWindowHandler) lootStackLinked(who string, rec database.CharInventoryEntry, n int) *serverLine {
	if n > 1 {
		return h.linkedf(who, "You receive %d %s from the corpse.", n, lootItem(rec, n))
	}
	return h.linkedf(who, "You receive %s from the corpse.", lootItem(rec, n))
}

// groupLootLine is a group member's line for what another took.
func groupLootLine(name string, n int, item string) string {
	if n > 1 {
		return fmt.Sprintf("%s receives %d %s from the corpse.", name, n, item)
	}
	return fmt.Sprintf("%s receives %s from the corpse.", name, item)
}

// lootStackLine is the line for n of an item.
func lootStackLine(n int, item string) string {
	if n > 1 {
		return fmt.Sprintf("You receive %d %s from the corpse.", n, item)
	}
	return fmt.Sprintf("You receive %s from the corpse.", item)
}

// lootNoRoom is what a take the pack has no room for is answered: CLIENT's own
// word 823, the line it shows itself when an item will not go in its list
// (StatWithItem, 0x00BCA2E8); its Msg001A takes what fits and says nothing,
// and its LootOne only sends the 0x0019, so the server says it.
const lootNoRoom = "You have no room in your inventory for that item."

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
		delete(ls.since, sess)
		return nil, nil, nil
	}
	return c, player, inv
}

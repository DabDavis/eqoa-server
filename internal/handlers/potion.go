package handlers

import (
	"errors"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Potions (entity/potion.go; F3 20261025-223151, -223841, -223959): an item
// with a row in itemeffect is used, not worn. The item menu's Use
// (UseItem(i, 0)) or an item quick slot's (UseItem(i, 1)) drinks one; one
// leaves the stack by 0x003C as a used scroll does. A thrown potion is a
// ranged weapon (F3 20261026-014336, jib: "equips in the range attack slot,
// like a bow, and fires as a range auto attack. Currently its a useable
// potion"): wear 16, a bow's (and the rusty throwing knife's), so every Use
// wears it in the ranged slot and takes the hands off as a bow does, and the
// ranged auto-attack throws it; no Use throws one. Only a thrown potion worn
// nowhere (a GM's) is thrown by Use, at the client's target (0x003E carries
// m_target).
//
// An alchemist's spell gives potions at its landing (summonItem), as a
// merchant's sale gives an item (GiveItemNow, 0x003B), three of a potion
// (jib: "stacks of 3"); an auto-attack's throw takes one off the worn stack
// (potionThrown).

// potionOf is the item's use, if it has one: its itemeffect row, read now, so
// a GM's change counts at the next use.
func (h *EquipHandler) potionOf(p *database.ItemPatternRecord) (entity.PotionEffect, bool) {
	if h.inventoryStore == nil {
		return entity.PotionEffect{}, false
	}
	r, ok, err := h.inventoryStore.ItemEffect(p.PatternID)
	if err != nil {
		log.Printf("Potion: item %d's use: %v", p.PatternID, err)
		return entity.PotionEffect{}, false
	}
	if !ok {
		return entity.PotionEffect{}, false
	}
	return potionFromRow(r, p.ItemName), true
}

func potionFromRow(r database.ItemEffectRow, itemName string) entity.PotionEffect {
	return entity.NewPotionEffect(r.PatternID, itemName, r.Line, r.Name, r.Icon, r.Breaks, r.Wear, r.Area)
}

// usesPotion is whether this Use is the potion's use rather than the wear: a
// potion drunk is used from the item menu and a quick slot alike; a thrown
// one is a ranged weapon, worn by every Use (the ranged auto-attack throws
// it), unless it is worn nowhere. interaction (the menu's 0, a quick slot's
// 1) changes nothing.
func usesPotion(pe *entity.PotionEffect, p *database.ItemPatternRecord, interaction byte) bool {
	return !pe.Thrown() || p.EquipSlot < 0
}

// usePotion answers a Use of the potion at list index idx (row): used and one
// taken off the stack, or refused with a red line and kept.
func (h *EquipHandler) usePotion(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry, pe entity.PotionEffect, target uint32) {
	refuse := func(text string) {
		log.Printf("Potion: %s, %q: %s", player.Name, row.Pattern.ItemName, text)
		SendColoredChat(sess, text, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	}
	if player.Level < row.Pattern.LevelReq {
		refuse(scribeTooLow)
		return
	}
	if h.gameWorld == nil {
		return
	}
	// a vampire drinks no invisibility or speed potion (entity/vampire.go)
	if text := h.gameWorld.VampirePotionRefusal(sess, pe); text != "" {
		refuse(text)
		return
	}
	refusal, err := h.gameWorld.UsePotion(sess, pe, target, time.Now())
	switch {
	case errors.Is(err, entity.ErrPotionNoPlayer):
		return
	case err != nil:
		log.Printf("Potion: %s, %q: %v", player.Name, row.Pattern.ItemName, err)
		return
	case refusal != "":
		refuse(refusal)
		return
	}
	if err := h.takeOne(sess, player, idx, row); err != nil {
		log.Printf("Potion: %s, %q: taking one: %v", player.Name, row.Pattern.ItemName, err)
		return
	}
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
}

// LoadPotions gives the World every item's use (at the start).
func (h *EquipHandler) LoadPotions() {
	if h.inventoryStore == nil || h.gameWorld == nil {
		return
	}
	rows, err := h.inventoryStore.ItemEffects()
	if err != nil {
		log.Printf("Potion: loading the items' uses: %v", err)
		return
	}
	list := make([]entity.PotionEffect, 0, len(rows))
	for _, r := range rows {
		list = append(list, potionFromRow(r, r.ItemName))
	}
	h.gameWorld.SetPotionEffects(list)
	log.Printf("Potion: %d items have a use", len(list))
}

// thrownPotion is whether a worn item is a potion an auto-attack throws: as
// the World knows it, else as its itemeffect row says now (a GM's new potion),
// which the World then keeps.
func thrownPotion(gw *entity.World, inv *database.InventoryStore, p *database.ItemPatternRecord) bool {
	if gw == nil {
		return false
	}
	if gw.ThrownPotion(p.PatternID) {
		return true
	}
	if inv == nil || p.AttackType != 10 {
		return false
	}
	r, ok, err := inv.ItemEffect(p.PatternID)
	if err != nil || !ok {
		return false
	}
	pe := potionFromRow(r, p.ItemName)
	gw.NotePotionEffect(pe)
	return pe.Thrown()
}

// summonItem gives a character the items a spell summoned (entity's
// SummonItemFunc): the item by the name the spell's words give, else by the
// spell's own name (a database seeded before 2026-10-27 has Neriak Cocktail's
// "Grobb Cocktail", as its words and the game's own record name it, under the
// spell's name: tools/fix_archived_spells.py renames it); count the words'
// xN, else summonCount's. A lore item (itempattern.lore 1) is held once (the
// owner's ruling of 2026-10-29: the alchemist's ogre pill "is lore"): one
// at most, and none to a character that holds one already, worn, in the
// inventory or in the bank, who is told so in red (the words ours).
func (h *EquipHandler) summonItem(sess *session.Session, charServerID uint32, item string, count int, spell string) {
	if h.inventoryStore == nil || h.gameWorld == nil {
		return
	}
	pattern, ok, err := h.inventoryStore.ItemPatternByName(item)
	if err == nil && !ok && spell != "" {
		pattern, ok, err = h.inventoryStore.ItemPatternByName(spell)
	}
	if err != nil {
		log.Printf("Potion: %q summons %q: %v", spell, item, err)
		return
	}
	if !ok {
		log.Printf("Potion: %q summons %q: no item of that name", spell, item)
		return
	}
	if count <= 0 {
		_, potion := h.potionOf(&pattern)
		count = summonCount(&pattern, potion)
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil || p.CharServerID != charServerID {
		return // gone, or another character on the session now
	}
	if pattern.Lore != 0 {
		held, err := h.inventoryStore.HoldsPattern(charServerID, pattern.PatternID)
		if err != nil {
			log.Printf("Potion: %q summons lore %q for %s: %v", spell, pattern.ItemName, p.Name, err)
			return
		}
		if held {
			SendErrorMessage(sess, loreHeldLine(pattern.ItemName))
			log.Printf("Potion: %q summons lore %q for %s, who holds one: none given", spell, pattern.ItemName, p.Name)
			return
		}
		count = 1
	}
	n, err := GiveItemNow(sess, h.gameWorld, h.inventoryStore, pattern, count)
	if err != nil {
		log.Printf("Potion: %q summons %d %q for %s: %v", spell, count, pattern.ItemName, p.Name, err)
		return
	}
	log.Printf("Potion: %q summons %d of %d %q for %s", spell, n, count, pattern.ItemName, p.Name)
}

// loreHeldLine is what a summoning of a lore item its caster holds says.
func loreHeldLine(item string) string {
	return "You already have " + item + ", a lore item: you may hold only one."
}

// summonCount is how many of an item a spell summons when its words say no
// number: three of a potion (an item with a use: jib, "a stack of 3" of a
// draught, a quickening or a sublimation, and of the thrown mixtures, "It
// also should come in stacks of 3, not one", F3 20261026-014336) or of
// anything else that stacks to three, one of anything else (the magician's
// Phase Staff).
func summonCount(p *database.ItemPatternRecord, potion bool) int {
	if potion || p.MaxStack == 3 {
		return 3 // GiveItemNow makes three of an item that does not stack
	}
	return 1
}

// potionThrown takes one of a thrown potion off the character's worn stack
// (entity's ThrownPotionFunc): an auto-attack threw it.
func (h *EquipHandler) potionThrown(sess *session.Session, charServerID uint32, patternID int) {
	if h.inventoryStore == nil || h.gameWorld == nil {
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil || p.CharServerID != charServerID {
		return
	}
	items, err := h.inventoryStore.GetCharacterItems(charServerID)
	if err != nil {
		log.Printf("Potion: %s's thrown %d: %v", p.Name, patternID, err)
		return
	}
	pick := -1
	for i := range items {
		e := &items[i]
		if e.Pattern.PatternID != patternID || e.Location != 0 {
			continue
		}
		if pick < 0 || e.EquipLoc >= 0 {
			pick = i // the worn stack first
		}
	}
	if pick < 0 {
		log.Printf("Potion: %s threw %d and holds none", p.Name, patternID)
		return
	}
	row := &items[pick]
	idx := -1
	if l := sessionInvList(sess); l != nil {
		idx = l.indexOfItem(row.ItemID)
	}
	if idx < 0 {
		log.Printf("Potion: %s's thrown %q is not in the client's list", p.Name, row.Pattern.ItemName)
		return
	}
	if err := h.takeOne(sess, p, idx, row); err != nil {
		log.Printf("Potion: %s's thrown %q: %v", p.Name, row.Pattern.ItemName, err)
		return
	}
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
}

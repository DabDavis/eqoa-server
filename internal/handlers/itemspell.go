package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// itemspell.go — an item whose Use casts a spell (table itemspell, database/itemspell.go: work board 737's
// code ask 4; the owner, 2026-11-18, Lord Valkanith's Rune of the Bonewing and Rune of Defilement,
// tools/valkanith_loot_20261118.py).
//
// The item, worn nowhere, is asked as a worn one is (cannotWear: its class and race masks, its level),
// then:
//   - a summoning spell's pet comes at once, with no cast, as the Stone of Guenhwyvar's does
//     (entity.SummonByItem, guenhwyvar.go): a pet of another kind out goes, as at any summoning; with the
//     item's own pet out already the Use does nothing and spends nothing. The pet is its petstats row's
//     (its body, its numbers, and the buffs it keeps on its owner while it is out: pettemplate.go's
//     while_out);
//   - any other spell is cast through the World's casting as an item's (CastSpell.ItemCast): its cast
//     time, its visual, its landing and area as the spell's, no power, no lockout and no 0x00CD (no
//     ability of the book); a cast refused (another cast on, no target it needs) spends nothing.
// Each Use that summons or begins its cast spends one of the item's charges (itemeffect.charges, the
// copy's remaincharge; spendCharge: the last takes it); a record of no charges is never spent; a copy of a
// charged record that holds none (one made before the record had charges) is taken whole, as a potion's
// last is. After a Use the item rests for its row's rest_s (one character, one item record; ours, kept
// in memory: a restart forgets it).

// The lines an item's spell says.
const (
	itemPetCalled  = "%s answers the %s."
	itemPetOutLine = "%s is already at your side."
	itemSpellRests = "The %s is not ready yet."
)

// itemSpellKey is one character's one item record, for its rest.
type itemSpellKey struct {
	char    uint32
	pattern int
}

// useItemSpell is the Use of an item that casts a spell (the file's doc). It reports whether the item is
// one (handled, whatever came of it); false lets the Use go on as any other item's.
func (h *EquipHandler) useItemSpell(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry,
	target uint32) bool {
	if h.inventoryStore == nil || row == nil {
		return false
	}
	is, ok, err := h.inventoryStore.ItemSpell(row.Pattern.PatternID)
	if err != nil {
		log.Printf("ItemSpell: item %d: %v", row.Pattern.PatternID, err)
		return false
	}
	if !ok {
		return false
	}
	item := strings.TrimSpace(row.Pattern.ItemName)
	if h.gameWorld == nil || h.spells == nil {
		return true
	}
	if !mayWear(sess, player, &row.Pattern) {
		return true
	}
	now := time.Now()
	key := itemSpellKey{player.CharServerID, row.Pattern.PatternID}
	h.itemSpellMu.Lock()
	last, used := h.itemSpellUsed[key]
	h.itemSpellMu.Unlock()
	if used && is.Rest > 0 && now.Before(last.Add(is.Rest)) {
		SendErrorMessage(sess, fmt.Sprintf(itemSpellRests, item))
		return true
	}
	sp, err := h.spells.SpellPattern(is.SpellID)
	if err != nil || sp == nil {
		log.Printf("ItemSpell: %q's spell %d: %v", item, is.SpellID, err)
		return true
	}
	fx, _, err := h.spells.SpellEffectRow(is.SpellID)
	if err != nil {
		log.Printf("ItemSpell: %q's spell %d's effect: %v", item, is.SpellID, err)
		return true
	}
	spell := castSpellRow(&database.CharSpellEntry{Pattern: *sp}, fx)
	spell.PowerCost, spell.ItemCast = 0, true
	cast := false
	if spell.Def.Summons() {
		switch h.gameWorld.SummonByItem(sess, spell, now, time.Time{}) {
		case entity.ItemSummoned:
			cast = true
			if name, ok := h.gameWorld.PetName(sess); ok {
				SendClientMessage(sess, fmt.Sprintf(itemPetCalled, name, item))
			}
		case entity.ItemPetOut:
			if name, ok := h.gameWorld.PetName(sess); ok {
				SendClientMessage(sess, fmt.Sprintf(itemPetOutLine, name))
			}
		}
	} else {
		cast = h.gameWorld.StartCast(sess, spell, target, now)
	}
	if !cast {
		return true
	}
	h.itemSpellMu.Lock()
	if h.itemSpellUsed == nil {
		h.itemSpellUsed = make(map[itemSpellKey]time.Time)
	}
	h.itemSpellUsed[key] = now
	h.itemSpellMu.Unlock()
	log.Printf("ItemSpell: %s uses %q: spell %d %q (%d charges left before)", player.Name, item, is.SpellID, spell.Name, row.Charges)
	switch {
	case neverSpent(&row.Pattern) || h.inventoryStore.ItemCharges(row.Pattern.PatternID) <= 0:
		// a record of no charges: never spent
	case row.Charges > 0:
		if err := h.spendCharge(sess, player, idx, row); err != nil {
			log.Printf("ItemSpell: %s, %q: a charge: %v", player.Name, item, err)
		}
	default:
		// a copy of a charged record that holds none: its one use
		if err := h.takeOne(sess, player, idx, row); err != nil {
			log.Printf("ItemSpell: %s, %q: taking it: %v", player.Name, item, err)
			return true
		}
		queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
	}
	return true
}

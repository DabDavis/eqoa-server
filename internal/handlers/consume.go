package handlers

import (
	"errors"
	"log"
	"regexp"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Food and drink: the item menu's Use (UseItem(i, 0)) or an item quick slot's
// (UseItem(i, 1)) on a grocer's meal or drink, the 0x003E equip.go takes,
// eats or drinks one of it (F3 20261025-172627): its HoT (a meal) or PoT (a
// drink) is added to the player's regen for a minute and the effects channel
// shows it by the item's name and a buff's icon (entity/consumable.go); one
// leaves the stack as a used scroll does, by 0x003C (index, sound 0, one).
//
// Not the item's own icon: an item's (itempattern.itemicon) is an ITEMICON.ESF
// surface of 128x64, the inventory's whole tile, and VIWndInfluence blits an
// effect's icon whole at its own size into a cell 34 high, so a meal's tile
// covered the effects beside it. What lasts of a spell goes on 0x43 as the
// ability's own icon (Sandstorm's, claude-notes/casting-sandstorm-20261021.md),
// a UI.CSF surface of 32x32. Sandstorm showed a meal and a drink alike with
// the health-over-time one (lane-spellfx24, 2026-10-30: Elven Wine, Spiced
// Jerky and mine rations all 0x828563AF), and named a plain ration's effect
// "Food" where a grocer's meal or drink kept its item's name ("Elven Wine",
// "Spiced Jerky"). With the item taken it played its eating-and-drinking
// visual on the player (entity.Consumed: the cheer).
//
// A grocer's wares say what they are: every meal of the food line is "A
// hearty meal that will increase health regeneration." and every drink "A
// refreshing beverage that increases (aids in) power regeneration." The
// amount is the item's own HoT or PoT; an item that has none gives its level
// (tools/stock_grocers.py writes that into itempattern, so a GM sees and can
// change it). None of it is measured.
//
// A new character's rations are meals too (the user, 2026-10-29: "should be
// magic slot as usable"): Sandstorm's (tools/fix_start_item_records.py) are
// of CLIENT's usable type, 2 (the record's +0x4C: VIInfoText::IsUsable), and
// described as the food they are ("A slice of toasted bread with mushroom
// infused butter, ..."), with no stat and no words for what eating one does.
// An item of type 2 named a ration, cuisine or snacks (the ten: field, trail,
// mine, tundra, plains, jungle, thicket rations, Gnomish Ration, travel
// cuisine, toad snacks) is eaten as a meal: OURS, its level's HoT (1).

var (
	mealDesc   = regexp.MustCompile(`(?i)\bmeal\b.*\bhealth regeneration\b`)
	drinkDesc  = regexp.MustCompile(`(?i)\bbeverage\b.*\bpower regeneration\b`)
	rationName = regexp.MustCompile(`(?i)\b(rations?|cuisine|snacks)\b`)
)

// usableType is CLIENT's item type of the things it offers to use and the
// HUD's first two quick slots take: food, drink, potions (VIInfoText::IsUsable).
const usableType = 2

// consumableEffectIcon is a meal's and a drink's effect icon, spellpattern.icon
// as it reaches the wire: Regenerative Aura's ("Increase HP per tic", the
// whole healing line's icon: Minor Healing, Healing, the Celebrations), as
// Sandstorm showed both (a drink here had Breeze's, 0xA2774C47, until
// 2026-10-30).
const consumableEffectIcon uint32 = 0x828563AF

// rationEffectName is what a plain ration's effect is called, as Sandstorm
// called a mine ration's.
const rationEffectName = "Food"

// Refusals. The level one is the line Sandstorm gives an item used below its
// level (scribe.go); the fight one is ours, a meal's alone: a meal ends in a
// fight (entity/consumable.go), so one taken in a fight would be gone at
// once. A drink is taken mid-fight, as Sandstorm took one.
const (
	consumeFighting = "You can't eat while you are fighting."
	consumeTooLow   = scribeTooLow
)

// consumableOf is the meal or drink an item is, if it is one: worn nowhere,
// and described as one.
func consumableOf(p *database.ItemPatternRecord) (entity.Consumable, bool) {
	if p.EquipSlot != -1 {
		return entity.Consumable{}, false
	}
	c := entity.Consumable{Name: p.ItemName}
	own := func(v *int) int {
		if v == nil {
			return 0
		}
		return *v
	}
	switch {
	case mealDesc.MatchString(p.ItemDesc):
		c.Icon, c.Amount = consumableEffectIcon, own(p.HoT)
	case p.ProcAnim == usableType && rationName.MatchString(p.ItemName):
		c.Name, c.Icon, c.Amount = rationEffectName, consumableEffectIcon, own(p.HoT)
	case drinkDesc.MatchString(p.ItemDesc):
		c.Drink, c.Icon, c.Amount = true, consumableEffectIcon, own(p.PoT)
	default:
		return entity.Consumable{}, false
	}
	if c.Amount <= 0 {
		c.Amount = max(1, p.LevelReq)
	}
	return c, true
}

// consume answers a Use of the meal or drink at list index idx (row): taken,
// or refused with a red line and kept.
func (h *EquipHandler) consume(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry, c entity.Consumable) {
	refuse := func(text string) {
		log.Printf("Consume: %s, %q: %s", player.Name, row.Pattern.ItemName, text)
		SendColoredChat(sess, text, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	}
	if player.Level < row.Pattern.LevelReq {
		refuse(consumeTooLow)
		return
	}
	if h.eat == nil {
		return
	}
	switch err := h.eat(sess, c, time.Now()); {
	case errors.Is(err, entity.ErrConsumeFighting):
		refuse(consumeFighting)
		return
	case err != nil:
		log.Printf("Consume: %s, %q: %v", player.Name, row.Pattern.ItemName, err)
		return
	}
	// The meal first: should taking it then fail, the player has had one
	// for nothing, rather than losing one for nothing.
	if err := h.takeOne(sess, player, idx, row); err != nil {
		log.Printf("Consume: %s, %q: taking one: %v (the meal stays on)", player.Name, row.Pattern.ItemName, err)
		return
	}
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
	// then, in the same moment, Sandstorm's eating-and-drinking visual: the
	// cheer, its sound and its particles (entity.Consumed)
	if h.consumed != nil {
		h.consumed(sess)
	}
}

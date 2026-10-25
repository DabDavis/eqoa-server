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
// shows it by the item's name and icon (entity/consumable.go); one leaves the
// stack as a used scroll does, by 0x003C (index, sound 0, one).
//
// A grocer's wares say what they are: every meal of the food line is "A
// hearty meal that will increase health regeneration." and every drink "A
// refreshing beverage that increases (aids in) power regeneration." The
// amount is the item's own HoT or PoT; an item that has none gives its level
// (tools/stock_grocers.py writes that into itempattern, so a GM sees and can
// change it). None of it is measured.

var (
	mealDesc  = regexp.MustCompile(`(?i)\bmeal\b.*\bhealth regeneration\b`)
	drinkDesc = regexp.MustCompile(`(?i)\bbeverage\b.*\bpower regeneration\b`)
)

// Refusals. The level one is the line Sandstorm gives an item used below its
// level (scribe.go); the fight one is ours: a meal ends in a fight, so one
// taken in a fight would be gone at once.
const (
	consumeFighting = "You can't eat or drink while you are fighting."
	consumeTooLow   = scribeTooLow
)

// consumableOf is the meal or drink an item is, if it is one: worn nowhere,
// and described as one.
func consumableOf(p *database.ItemPatternRecord) (entity.Consumable, bool) {
	if p.EquipSlot != -1 {
		return entity.Consumable{}, false
	}
	c := entity.Consumable{Name: p.ItemName, Icon: uint32(int32(p.ItemIcon))}
	own := func(v *int) int {
		if v == nil {
			return 0
		}
		return *v
	}
	switch {
	case mealDesc.MatchString(p.ItemDesc):
		c.Amount = own(p.HoT)
	case drinkDesc.MatchString(p.ItemDesc):
		c.Drink, c.Amount = true, own(p.PoT)
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
}

package entity

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mercmeal.go — a merc's food and drink as its hirer's, shown, and its word when its power
// runs low (petition 3061, 2026-11-25: "For Mercs. When giving them drinks or food, it would
// be nice to see that they're currently consuming the food or drink in their buff bar. Also
// if a merc could mention 'out of mana' when getting low as a reminder to give them time to
// consume food or drink."; the owner approved).
//
//   - Every merc eats the meals it carries (/merc give: handlers/mercinv.go), as an
//     alchemist did alone until then (mercbrew.go's mercEatTurnLocked: out of a fight,
//     below mercEatBelow of its health, a fight ending it); only an alchemist with none
//     eats a ration's figures, as before.
//   - Every merc with power drinks the grocer's drinks it carries (a beverage of power
//     regeneration: handlers' consumableOf) below mercDrinkBelow of its power with no drink
//     on it, in a fight or out of one, as a player's drink is taken (consumable.go): the
//     item's PoT a tick for ConsumableDuration, its first tick at once, its
//     eating-and-drinking visual shown round it.
//   - The meal and the drink are effects on the merc named as the item, with a meal's icon
//     (DrinkEffectID and FoodEffectID, as a player's): the group members' effects
//     (groupeffects.go, 0x7EC0/0x7EC1) carry them to its owner's group, their time left
//     with them, for as long as they last.
//   - A caster or a hybrid (mercSaysPower) says one line to its owner's group when its
//     power falls under mercLowPowerSay of its most, once a dip: it says it again only
//     after its power came back over mercLowPowerRearm, and never twice within
//     mercLowPowerQuiet. The line goes as group chat (0x069E, which CLIENT shows as its
//     text), to its owner alone out of a group.

// When a merc drinks and speaks of its power (ours).
const (
	// mercDrinkBelow: it drinks a carried drink below this share of its power
	mercDrinkBelow = 30
	// mercLowPowerSay: it says its power is low below this share of it
	mercLowPowerSay = 10
	// mercLowPowerRearm: its power back over this share, it may say so again
	mercLowPowerRearm = 30
	// mercLowPowerQuiet: never twice within this
	mercLowPowerQuiet = time.Minute
)

// mercLowPowerLine is its word to the group (CLIENT's group chat shows the text whole).
const mercLowPowerLine = "%s tells the group: I'm nearly out of power! Give me a moment to rest, eat or drink."

// mercDrinking is the merc's drink on it, nil none.
func mercDrinking(n *NPC) *ActiveEffect {
	for _, e := range n.Effects {
		if e.SpellID == DrinkEffectID {
			return e
		}
	}
	return nil
}

// mercGrocerDrinkTurnLocked is the merc's drink of a grocer's drink it carries when its
// power runs low (the file's doc); it reports one drunk. Must be called with the lock held.
func (w *World) mercGrocerDrinkTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if m.MaxPower <= 0 || m.Power*100 >= m.MaxPower*mercDrinkBelow || mercDrinking(m) != nil {
		return false
	}
	var drink *MercCarried
	for i := range ms.kit.Carried {
		c := &ms.kit.Carried[i]
		if c.Drink && c.Count > 0 && (drink == nil || c.PoT > drink.PoT) {
			drink = c
		}
	}
	if drink == nil {
		return false
	}
	w.mercGrocerDrinkLocked(ms, *drink, now)
	return true
}

// mercGrocerDrinkLocked puts a carried grocer's drink on the merc (its turn's,
// or its owner's Use: MercUse): the last drink's place taken, its first tick
// at once, one used, shown. Must be called with the lock held.
func (w *World) mercGrocerDrinkLocked(ms *mercState, drink MercCarried, now time.Time) {
	m := ms.npc
	name, pot, id := drink.Name, max(drink.PoT, 1), drink.ID
	w.dropNPCEffect(m, DrinkEffectID)
	ae := &ActiveEffect{SpellID: DrinkEffectID, Name: name, Icon: mercMealIcon, Caster: m.ServerID, Until: now.Add(ConsumableDuration)}
	ae.Mods.PoT = pot
	m.Effects = append(m.Effects, ae)
	m.Power = min(m.Power+pot, m.MaxPower) // its first tick at once (consumable.go)
	syncMercBody(ms)
	w.mercUsedLocked(ms, id, false, 0)
	w.broadcastNPCCastLocked(m, m.ServerID, 0, fxConsumed)
	log.Printf("Merc: %s's %q drinks %q: %d power a tick (%d/%d power)", ms.owner.Name, m.Name, name, pot, m.Power, m.MaxPower)
}

// Why a Use of a merc's carried food or drink was refused (MercUse).
var (
	// ErrMercUseNotOut: the merc of the slot is not out, or is dead
	ErrMercUseNotOut = errors.New("merc not out")
	// ErrMercUseNone: it carries no record of the id
	ErrMercUseNone = errors.New("merc carries none")
	// ErrMercUseNotFood: the record is neither a meal nor a grocer's drink
	ErrMercUseNotFood = errors.New("not food or drink")
	// ErrMercUseNoPower: a drink for a merc with no power to fill
	ErrMercUseNoPower = errors.New("merc has no power")
)

// MercUse is the owner's Use of a meal or a grocer's drink the merc of the slot
// carries (petition 3285, 2026-11-26: food and drink traded to a merc "Use"d
// from its inventory window, so it eats or drinks it now; the owner approved):
// the same effect its own turn would put on it (mercMealLocked,
// mercGrocerDrinkLocked), whatever its health or power, the last meal's or
// drink's place taken. As a player's (consumable.go) a meal is eaten in a fight
// too, its first tick at once, and the fight ends it at the merc's next turn;
// a drink runs on. It reports whether the record was a drink.
func (w *World) MercUse(sess *session.Session, slot, itemID int, now time.Time) (drink bool, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot)
	if ms == nil {
		return false, ErrMercUseNotOut
	}
	var c *MercCarried
	for i := range ms.kit.Carried {
		if ms.kit.Carried[i].ID == itemID && ms.kit.Carried[i].Count > 0 {
			c = &ms.kit.Carried[i]
			break
		}
	}
	switch {
	case c == nil:
		return false, ErrMercUseNone
	case c.Drink:
		if ms.npc.MaxPower <= 0 {
			return true, ErrMercUseNoPower
		}
		w.mercGrocerDrinkLocked(ms, *c, now)
		return true, nil
	case c.Food:
		hot := c.HoT
		if hot <= 0 {
			hot, _ = RationRestore(ms.body) // a ration's figures (the eat turn's)
		}
		w.mercMealLocked(ms, c.Name, hot, c.ID, now)
		return false, nil
	}
	return false, ErrMercUseNotFood
}

// mercSaysPower is whether a merc of the class speaks of its power: a caster's or a
// hybrid's, not a warrior's, a monk's or a rogue's.
func mercSaysPower(class int) bool {
	switch class {
	case 0, 4, 6:
		return false
	}
	return true
}

// mercLowPowerLocked is the merc's word when its power runs low (the file's doc). Must be
// called with the lock held.
func (w *World) mercLowPowerLocked(ms *mercState, now time.Time) {
	m := ms.npc
	if m.Dead || m.MaxPower <= 0 || !mercSaysPower(ms.kit.Class) {
		return
	}
	if m.Power*100 >= m.MaxPower*mercLowPowerRearm {
		ms.lowPowerSaid = false
		return
	}
	if ms.lowPowerSaid || m.Power*100 >= m.MaxPower*mercLowPowerSay {
		return
	}
	if !ms.lowPowerAt.IsZero() && now.Before(ms.lowPowerAt.Add(mercLowPowerQuiet)) {
		return
	}
	ms.lowPowerSaid, ms.lowPowerAt = true, now
	w.mercGroupLineLocked(ms, fmt.Sprintf(mercLowPowerLine, m.Name))
	log.Printf("Merc: %s's %q says its power is low (%d/%d)", ms.owner.Name, m.Name, m.Power, m.MaxPower)
}

// mercGroupLineLocked says a merc's line to its owner's group as group chat (0x069E: a u32
// CLIENT does not use, the merc's id, then the line), to each member who has not hidden
// the group's chat (lfg.go), its owner always; to its owner alone out of a group. Must be
// called with the lock held.
func (w *World) mercGroupLineLocked(ms *mercState, line string) {
	o := ms.owner
	to := []*Player{o}
	if g := w.groups[o.GroupID]; o.GroupID != 0 && g != nil {
		for _, id := range g.Members {
			if p := w.Players[id]; p != nil && p != o && !p.GroupChatHidden {
				to = append(to, p)
			}
		}
	}
	for _, p := range to {
		sendGroupMsg(w.sessionOfLocked(p), protocol.OpcodeGroupChat, func(b *buffer.Writer) {
			b.WriteUint32(uint32(ms.npc.ServerID))
			protocol.WriteWideString(b, line)
		})
	}
}

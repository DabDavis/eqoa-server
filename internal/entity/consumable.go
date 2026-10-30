package entity

import (
	"errors"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// What Consume refuses: no player (or a dead one), and a meal for one in a
// fight, which would end at once (consumableSystem).
var (
	ErrConsumeNoPlayer = errors.New("no living player")
	ErrConsumeFighting = errors.New("in a fight")
)

// consumable.go — food and drink (F3 20261025-172627, jib's account of the
// live game): a grocer's food gives health over time and its drink power over
// time, for about a minute, more for higher-level food and drink, and either
// ends once its eater takes an aggressive action or is hit.
//
// Nothing here is measured but what Sandstorm showed of a use (lane-spellfx24,
// 2026-10-30: Elven Wine, Spiced Jerky, mine rations): the item taken, the
// player's own 0x00B1 of the eating-and-drinking visual (Consumed), the
// effect on 0x43 with the heal-over-time icon, and a drink drunk mid-fight
// (its owner's pet and a training dummy at it) giving its power on, +196 a
// tick, through the 13 s recorded. So a drink is taken in a fight and a
// fight does not end it; a meal keeps jib's rule (refused in a fight, ended
// by one). The amount is the item's own HoT or PoT (itempattern, so a GM can
// change it; tools/stock_grocers.py sets each to the item's level), added to
// the six-second regen (regen.go) and so to the stat channel's HoT and PoT;
// the time is jib's minute.

// FoodEffectID and DrinkEffectID key the two effects among a player's
// spells': a new meal replaces the last, a drink the last drink, and a meal
// and a drink are on together. No spell has a negative id.
const (
	FoodEffectID  = -1001
	DrinkEffectID = -1002
)

// ConsumableDuration is how long a meal or a drink lasts (jib: "about 1
// minute").
const ConsumableDuration = time.Minute

// Consumable is a meal or a drink being taken: its item's name and icon, as
// the effects channel shows them, and its HoT (a meal) or PoT (a drink).
type Consumable struct {
	Drink  bool
	Name   string
	Icon   uint32
	Amount int
}

func (c Consumable) effectID() int {
	if c.Drink {
		return DrinkEffectID
	}
	return FoodEffectID
}

// Consume puts a meal or a drink on the session's player for
// ConsumableDuration: its effect replaces the last of its kind. It refuses
// no player or a dead one (ErrConsumeNoPlayer) and a meal with auto-attack
// on (ErrConsumeFighting); a drink is taken in a fight, as Sandstorm took
// one.
func (w *World) Consume(sess *session.Session, c Consumable, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return ErrConsumeNoPlayer
	}
	if p.Attacking() && !c.Drink {
		return ErrConsumeFighting
	}
	ae := &ActiveEffect{SpellID: c.effectID(), Name: c.Name, Icon: c.Icon, Caster: p.ServerID,
		Until: now.Add(ConsumableDuration)}
	if c.Drink {
		ae.Mods.PoT = c.Amount
	} else {
		ae.Mods.HoT = c.Amount
	}
	w.addEffectLocked(p, ae, now)
	log.Printf("Consume: %s has %q (%s %d for %v)", p.Name, c.Name, map[bool]string{false: "HoT", true: "PoT"}[c.Drink], c.Amount, ConsumableDuration)
	return nil
}

// consumableSystem ends a player's meal once it takes an aggressive action
// or is hit after eating: auto-attack engaged, or a fight marked since
// (LastCombatTime: its swing or harmful spell landing, a creature's or a
// dummy's hit on it). A drink runs on through a fight (Sandstorm's did).
// Every tick; must be called with the lock held.
func (w *World) consumableSystem(now time.Time) {
	for _, p := range w.Players {
		for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
			if ae.SpellID != FoodEffectID {
				continue
			}
			since := ae.Until.Add(-ConsumableDuration)
			if !p.Attacking() && !p.LastCombatTime.After(since) {
				continue
			}
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Consume: %s's %q ends in a fight", p.Name, ae.Name)
		}
	}
}

// fxConsumed is the visual Sandstorm played on whoever ate or drank (Elven
// Wine, Spiced Jerky, mine rations): the player's own 0x00B1 at itself, cast
// 0, in the moment the item was taken, to it and to those around, as a cast
// is. The effect's own events (SPELLFX.CSF) play the cheer (SetAction 61,
// claude-notes/anim-slot-semantics.md), a sound and its particles: no other
// message carries them.
const fxConsumed uint32 = 0xCF84F260

// Consumed plays the eating-and-drinking visual on the session's player.
func (w *World) Consumed(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && !p.IsDead() {
		w.broadcastCastLocked(p, p.ServerID, 0, fxConsumed)
	}
}

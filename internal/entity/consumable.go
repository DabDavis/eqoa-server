package entity

import (
	"errors"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// What Consume refuses: no player (or a dead one), and one in a fight, whose
// meal would end at once (consumableSystem).
var (
	ErrConsumeNoPlayer = errors.New("no living player")
	ErrConsumeFighting = errors.New("in a fight")
)

// consumable.go — food and drink (F3 20261025-172627, jib's account of the
// live game): a grocer's food gives health over time and its drink power over
// time, for about a minute, more for higher-level food and drink, and either
// ends once its eater takes an aggressive action or is hit.
//
// Nothing here is measured: no capture, recording or page gives a food's
// amount or time. The amount is the item's own HoT or PoT (itempattern, so a
// GM can change it; tools/stock_grocers.py sets each to the item's level),
// added to the six-second regen (regen.go) and so to the stat channel's HoT
// and PoT; the time is jib's minute.

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

// isConsumableEffect is whether an effect is a meal's or a drink's.
func isConsumableEffect(id int) bool { return id == FoodEffectID || id == DrinkEffectID }

// Consume puts a meal or a drink on the session's player for
// ConsumableDuration: its effect replaces the last of its kind. It refuses
// no player or a dead one (ErrConsumeNoPlayer) and one with auto-attack on
// (ErrConsumeFighting).
func (w *World) Consume(sess *session.Session, c Consumable, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return ErrConsumeNoPlayer
	}
	if p.AttackTarget != 0 {
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

// consumableSystem ends a player's meal and drink once it takes an
// aggressive action or is hit after taking them: auto-attack engaged, or a
// fight marked since (LastCombatTime: its swing or harmful spell landing, a
// creature's or a dummy's hit on it). Every tick; must be called with the
// lock held.
func (w *World) consumableSystem(now time.Time) {
	for _, p := range w.Players {
		for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
			if !isConsumableEffect(ae.SpellID) {
				continue
			}
			since := ae.Until.Add(-ConsumableDuration)
			if p.AttackTarget == 0 && !p.LastCombatTime.After(since) {
				continue
			}
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Consume: %s's %q ends in a fight", p.Name, ae.Name)
		}
	}
}

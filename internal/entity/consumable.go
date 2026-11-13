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
//
// An item with a foodeffect row (database/foodeffect.go; EQOAEmu's workbook,
// the import of 2026-11-01: "yes add the food too") lasts its row's time and
// ends as its row says: on any spell its eater casts (ends_on_cast, the
// workbook's DisspellOnCast) and in a fight (ends_on_contact, its
// DisspellOnContact: the meal's rule above, a drink's too then, and refused
// in a fight as a meal is); a meal or drink with no row keeps the minute and
// the rules above. An item with both a HoT and a PoT (Blessed Fish) gives
// both from its one slot: a meal's (its description says what it is), so a
// drink beside it adds its own PoT and another meal replaces it whole.

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
// the effects channel shows them, and its HoT (a meal) or PoT (a drink); Also
// the other of the two an item has both of (a meal's PoT, a drink's HoT).
// Duration and Rule are its foodeffect row's: 0 and nil without one.
type Consumable struct {
	Drink    bool
	Name     string
	Icon     uint32
	Amount   int
	Also     int
	Duration time.Duration
	Rule     *FoodRule
	// Ration is a plain ration's (handlers/consume.go): its HoT and PoT are
	// its eater's, RationRestore's, in place of Amount and Also
	Ration bool
}

// RationShare is the share of its eater's most health and power a ration
// gives each regen tick: a tenth (ours; the user, 2026-11-06: "scale with
// level, and doesnt take too long to restore"), so with the regen's own
// fiftieth an empty bar is full within the meal's minute.
const RationShare = 10

// RationRestore is what a ration gives its eater a tick: a tenth of its most
// health and of its most power with its effects (MaxHPNow, MaxPowerNow), at
// least its level each; no power to one that has none (a warrior). Must be
// called with the lock held.
func RationRestore(p *Player) (hot, pot int) {
	hot = max(p.MaxHPNow()/RationShare, p.Level, 1)
	if most := p.MaxPowerNow(); most > 0 {
		pot = max(most/RationShare, p.Level)
	}
	return hot, pot
}

// FoodRule is what ends a meal or a drink, as its item's foodeffect row
// says: a spell its eater casts, a fight. Since is when it was taken (a
// fight from then on ends it); a rule kept over a logout starts again at the
// login.
type FoodRule struct {
	EndsOnCast    bool      `json:",omitempty"`
	EndsOnContact bool      `json:",omitempty"`
	Since         time.Time `json:"-"`
}

// endsInFight is whether a fight ends it: its row's word, else a meal's rule.
func (c Consumable) endsInFight() bool {
	if c.Rule != nil {
		return c.Rule.EndsOnContact
	}
	return !c.Drink
}

func (c Consumable) effectID() int {
	if c.Drink {
		return DrinkEffectID
	}
	return FoodEffectID
}

// Consume puts a meal or a drink on the session's player for
// ConsumableDuration: its effect replaces the last of its kind. It refuses
// no player or a dead one (ErrConsumeNoPlayer) and a meal in a fight
// (ErrConsumeFighting): auto-attack on, or a creature fighting it (its threat
// on it, or it its enemy: linkdead.go's fightingLocked); a drink is taken in
// a fight, as Sandstorm took one.
func (w *World) Consume(sess *session.Session, c Consumable, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return ErrConsumeNoPlayer
	}
	// a meal or a drink is taken in a fight, and runs on through it (the
	// owner, 2026-11-13: "players arent allowed to eat/drink during a fight
	// but should be allowed"; petition 1919 had refused a meal in a fight)
	d := c.Duration
	if d <= 0 {
		d = ConsumableDuration
	}
	if c.Ration {
		c.Amount, c.Also = RationRestore(p)
	}
	ae := &ActiveEffect{SpellID: c.effectID(), Name: c.Name, Icon: c.Icon, Caster: p.ServerID, Until: now.Add(d)}
	if c.Drink {
		ae.Mods.PoT, ae.Mods.HoT = c.Amount, c.Also
	} else {
		ae.Mods.HoT, ae.Mods.PoT = c.Amount, c.Also
	}
	if c.Rule != nil {
		r := *c.Rule
		r.Since = now
		ae.Food = &r
	}
	w.addEffectLocked(p, ae, now)
	log.Printf("Consume: %s has %q (HoT %d, PoT %d for %v)", p.Name, c.Name, ae.Mods.HoT, ae.Mods.PoT, d)
	return nil
}

// endFoodOnCastLocked ends the eater's meal and drink whose rule says a spell
// cast ends it (FoodRule.EndsOnCast), as the cast begins (cast.go). Must be
// called with the lock held.
func (w *World) endFoodOnCastLocked(p *Player) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if (ae.SpellID == FoodEffectID || ae.SpellID == DrinkEffectID) && ae.Food != nil && ae.Food.EndsOnCast {
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Consume: %s's %q ends with a cast", p.Name, ae.Name)
		}
	}
}

// foodRunsThroughFights keeps meals and drinks through a fight (the owner,
// 2026-11-13: eating and drinking allowed in a fight); false brings back the
// meal ending at a fight's first contact.
var foodRunsThroughFights = true

// consumableSystem ends a player's meal once it takes an aggressive action
// or is hit after eating: auto-attack engaged, or a fight marked since
// (LastCombatTime: its swing or harmful spell landing, a creature's or a
// dummy's hit on it). A drink runs on through a fight (Sandstorm's did); a
// meal or a drink with a rule (FoodRule) ends so as its rule says. Every
// tick; must be called with the lock held.
func (w *World) consumableSystem(now time.Time) {
	for _, p := range w.Players {
		for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
			if ae.SpellID != FoodEffectID && ae.SpellID != DrinkEffectID {
				continue
			}
			ends, since := ae.SpellID == FoodEffectID, ae.Until.Add(-ConsumableDuration)
			if ae.Food != nil {
				ends, since = ae.Food.EndsOnContact, ae.Food.Since
			}
			ends = ends && !foodRunsThroughFights
			if !ends || !p.Attacking() && !p.LastCombatTime.After(since) {
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

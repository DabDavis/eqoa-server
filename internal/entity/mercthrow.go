package entity

import (
	"log"
	"strings"
	"time"
)

// mercthrow.go — an alchemist merc throws its potions (the owner, 2026-11-15:
// "the alchemist mercs should be throwing their pots but arent currently").
// Until then its potion attacks were spells: the book's "Qeynos Fire Potion"
// and the like went out as a 2 s cast (0x00B1 with the spell's cast time and
// visual), and the potions its mixtures summon (Volatile Potion, the
// cocktails) were never used, a summoning being nothing a merc casts.
//
// A player alchemist throws a potion as the thrown weapon of its ranged
// auto-attack (potion.go: worn in the ranged slot, each round's throw one of
// the stack, thrownPotionHitLocked): CLIENT's GetSwing gives an alchemist with
// slot 18 worn Range1 (attack.go), whose SPELLFX effect plays the throw on the
// second hand and looses the second hand's item at the target (npcshoot.go),
// its round 3.5 s (SwingDelay), its hit 0.5 s into it (swingHits), its power
// a throw's (throwPowerCost: DefaultThrowPowerPct of the thrower's most). The
// merc throws the same way: its second hand holds the potion bottle (its
// kit's look, handlers/merc.go), each throw is 0x00B1 from the merc at its
// target with Range1's effect and the round's delay to everyone who sees it,
// as a player's round is shown to others (broadcastPlayerSwingLocked), and the
// potion lands 0.5 s on as throwLocked lands one: what its target alone takes
// (TargetShare) and its line on the target, its line on every creature within
// its splash (itemeffect.area) the merc may fight, each a hit of the merc's
// (mercHarmLocked: its figures, the balance, its owner's kill). No cast time,
// no spell lockout.
//
// What it throws (mercThrowPickLocked) is the best of: the thrown potions its
// book's summonings make (a mixture's "Summon Volatile Potion x3", a
// cocktail's), and its book's own potion spells ("Freeport Fire Potion",
// which it no longer casts: mercPotionSpell), each by what it is worth on its
// body (mercWorth). A spell its hirer turned off is not thrown. Its other
// spells (the sprays, roots, buffs) are cast as before, between its throws.

// classAlchemist is the alchemist's class.
const classAlchemist = 14

// mercThrowLand is the potion's hit, 0.5 s into the round as Range1's
// (swingHits).
var mercThrowLand = swingHits(8)[0]

// mercThrowable is a potion a merc may throw: an item's use, or a potion
// spell of its book thrown as one.
type mercThrowable struct {
	pe PotionEffect
	// spell is the book's spell id it stands for (a potion spell), 0 an item
	spell int
	worth float64
}

// mercThrowing is a throw on its way: its target and when it lands.
type mercThrowing struct {
	target EntityID
	lands  time.Time
	what   mercThrowable
}

// mercPotionSpell is whether s is an alchemist's potion spell, thrown and not
// cast (the file's doc): harmful, at a target, named a potion.
func mercPotionSpell(s *CastSpell) bool {
	return s.Def.Harmful && s.Scope == 1 && strings.HasSuffix(strings.ToLower(strings.TrimSpace(s.Name)), " potion")
}

// mercThrowsPotions is whether the merc throws potions: an alchemist.
func (ms *mercState) mercThrowsPotions() bool { return ms.kit.Class == classAlchemist }

// mercThrowSwing is the merc's round with a potion in its ranged slot alone:
// CLIENT's GetSwing for its class and level, Range1 for an alchemist.
func (ms *mercState) mercThrowSwing() Swing {
	var worn [21]bool
	worn[18] = true
	return SwingFor(ms.kit.Class, ms.body.Level, worn, true, 10, PotionRange)
}

// mercThrowablesLocked is every potion the merc may throw, best first (the
// file's doc). Must be called with the lock held.
func (w *World) mercThrowablesLocked(ms *mercState) []mercThrowable {
	if !ms.mercThrowsPotions() {
		return nil
	}
	byName := make(map[string]*PotionEffect, len(w.potions.byPattern))
	for _, pe := range w.potions.byPattern {
		byName[strings.ToLower(strings.TrimSpace(pe.ItemName))] = pe
	}
	seen := map[int]bool{}
	var out []mercThrowable
	add := func(t mercThrowable) {
		cs := t.pe.castSpell()
		u := mercUseOf(&cs)
		if !t.pe.Thrown() || (u != useNuke && u != useDoT) {
			return
		}
		t.worth = mercWorth(&cs, u, ms.body)
		out = append(out, t)
	}
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if containsInt(ms.kit.SpellsOff, s.SpellID) {
			continue
		}
		if mercPotionSpell(s) {
			add(mercThrowable{pe: PotionEffect{ItemName: s.Name, Name: s.Name, Icon: s.Icon, Def: s.Def}, spell: s.SpellID})
			continue
		}
		var made []SpellItem
		if !s.Def.Summons() {
			made = append(append(made, s.Def.Items...), unreadSummons(&s.Def)...)
		}
		for _, it := range made {
			pe := byName[strings.ToLower(strings.TrimSpace(it.Name))]
			if pe == nil || seen[pe.PatternID] {
				continue
			}
			seen[pe.PatternID] = true
			add(mercThrowable{pe: *pe})
		}
	}
	// the biggest first; of two as big, the item's (a real potion) first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].worth > out[j-1].worth || out[j].worth == out[j-1].worth && out[j].spell == 0 && out[j-1].spell != 0); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// mercThrowPickLocked is the potion the merc throws at t now, false for none:
// the best of mercThrowablesLocked that may land on t (a spell for undead
// alone only at undead). Must be called with the lock held.
func (w *World) mercThrowPickLocked(ms *mercState, t *NPC) (mercThrowable, bool) {
	for _, c := range w.mercThrowablesLocked(ms) {
		cs := c.pe.castSpell()
		if w.petUndeadMayLocked(&cs, t) {
			return c, true
		}
	}
	return mercThrowable{}, false
}

// mercThrowTurnLocked is the merc's throw at t when its round allows one (the
// file's doc): in a potion's reach and its sight, the power for it; the round
// shown and the potion on its way. It reports a throw begun. Must be called
// with the lock held.
func (w *World) mercThrowTurnLocked(ms *mercState, t *NPC, now time.Time) bool {
	if !ms.mercThrowsPotions() || t == nil || t.Dead || ms.throwing != nil || now.Before(ms.throwReady) {
		return false
	}
	m := ms.npc
	if !mercMayFight(m, t) || !ms.mercReactedLocked(t, now) {
		return false
	}
	s := ms.mercThrowSwing()
	if dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) > s.Reach+npcReachExtra(t) || !w.inSightLocked(m.World, m.X, m.Y, m.Z, t.X, t.Y, t.Z) {
		return false
	}
	what, ok := w.mercThrowPickLocked(ms, t)
	if !ok {
		return false
	}
	if cost := w.potions.throwPowerCost(ms.body); m.Power < cost {
		return false // its spells may still be cast
	} else {
		m.Power -= cost
	}
	m.stopMoving()
	m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
	ms.throwReady = now.Add(s.Delay)
	ms.throwing = &mercThrowing{target: t.ServerID, lands: now.Add(mercThrowLand), what: what}
	ms.busy = now.Add(mercThrowLand) // it stands for its throw
	breakNPCInvis(m)
	w.broadcastNPCSwingLocked(m, t.ServerID, uint32(s.Delay/time.Millisecond), HashResourceID(SwingEffectName(s.Kind)))
	log.Printf("Merc: %s's %q throws %q at %s (%d power left)", ms.owner.Name, m.Name, what.pe.ItemName, t.Name, m.Power)
	return true
}

// mercThrowLandLocked lands the merc's potion whose time has come, as
// throwLocked lands a player's: its target's own share and its line on its
// target, its line on each creature in its splash the merc may fight. It
// reports its target killed. Must be called with the lock held.
func (w *World) mercThrowLandLocked(ms *mercState, now time.Time) bool {
	c := ms.throwing
	if c == nil || now.Before(c.lands) {
		return false
	}
	ms.throwing = nil
	m := ms.npc
	t := w.NPCs[c.target]
	if t == nil || t.Dead || t.World != m.World || !mercMayFight(m, t) {
		return false
	}
	if r := ms.mercThrowSwing().Reach + npcReachExtra(t); dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) > r {
		return false // it went out of the potion's reach
	}
	pe := &c.what.pe
	s := pe.castSpell()
	if c.what.spell != 0 {
		s.SpellID = c.what.spell // a potion spell's own id: its lasting effect's
	}
	own := s
	if len(pe.TargetShare) > 0 {
		own.Def.Effects = append(append([]SpellEffect(nil), pe.TargetShare...), s.Def.Effects...)
	}
	// the splash, found before the target's hit may kill it
	var splash []*NPC
	if pe.Area > 0 {
		place := areaPlace{world: t.World, caster: [3]float32{m.X, m.Y, m.Z}, centre: [3]float32{t.X, t.Y, t.Z}, onOther: true}
		for _, id := range w.SpatialIndex.QueryRadius(t.X, t.Z, pe.Area) {
			o := w.NPCs[id]
			if o == nil || o == t || o.Dead || o.World != t.World || !mercMayFight(m, o) || o.NPCType&0x80 != 0 || o.hiddenAt(now) {
				continue
			}
			if dist3(o.X, o.Y, o.Z, t.X, t.Y, t.Z) > pe.Area || !w.reachesLocked(place, o.X, o.Y, o.Z) {
				continue
			}
			splash = append(splash, o)
		}
	}
	w.mercHarmLocked(ms, &own, t, now)
	for _, o := range splash {
		if !o.Dead && w.mercLiveLocked(ms) {
			w.mercHarmLocked(ms, &s, o, now)
		}
	}
	return t.Dead || t.HP <= 0
}

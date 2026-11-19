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
// What it throws (mercThrowPickLocked), in order: a thrown potion it carries
// (given it: handlers/mercinv.go, the owner's "that is also a way to switch
// pots on an alc merc"), used up one a throw; then one of its owner's stock
// (mercbrew.go), one a throw; then its path's own, conjured as its spells
// conjure them and never used up: the best of the thrown potions its book's
// summonings make (a mixture's "Summon Volatile Potion x3", a cocktail's) and
// its book's own potion spells ("Freeport Fire Potion", which it no longer
// casts: mercPotionSpell), each by what it is worth on its body (mercWorth).
// A spell its hirer turned off is not thrown. Its other spells (the sprays,
// roots, buffs) are cast as before, between its throws, and a throw that
// comes due while one is cast goes on through the cast on its round's beat
// (mercfight.go), as a player's auto-attack throws through its casts
// (CLIENT's AutoAttackTick runs every frame, a cast or not). Until 2026-11-17
// a due throw waited for the cast's end, and the next round was counted from
// that late throw: the Heirloom mercs threw every 5.3 s on average (median 5
// to 6 s over 2,270 throws, 2026-11-14..17), not Range1's 3.5.
//
// A Cocktail alchemist's throws are its hirer's choice (the owner: "if
// player chooses cocktail path should have a toggle for aoe cocktail thrown
// pots or the single target pots like the knack alc"): /merc aoe, its area
// cocktails (itemeffect.area) first; /merc single (the default, as it threw
// until the toggle: a cocktail was never thrown), the single-target potions
// a Knack alchemist throws, its book's cocktails left alone. A potion of the
// other kind it carries or its owner's stock holds is thrown only when none
// of its own kind is. An area potion pulls nothing (as a merc casts no
// harmful area spell): it is thrown only at a creature already fighting its
// team (its owner, its group, the mercs), and its splash lands only on
// creatures fighting its team; else a single-target potion goes.

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
	// carried is the merc's charmercitem record it is one of (0 none);
	// stock, one of its owner's stock
	carried int
	stock   bool
	worth   float64
	// debuff: its Master Class's thrown debuff (mercselfpotion.go)
	debuff bool
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

// throwWorth is what a potion thrown is worth on the merc's body, false for
// one that is no harm at a creature (a charm, a debuff alone).
func (ms *mercState) throwWorth(pe *PotionEffect) (float64, bool) {
	cs := pe.castSpell()
	u := mercUseOf(&cs)
	if !pe.Thrown() || (u != useNuke && u != useDoT) {
		return 0, false
	}
	return mercWorth(&cs, u, ms.body), true
}

// sortThrowables puts the best first; of two as big, an item's (a real
// potion) before a potion spell's.
func sortThrowables(out []mercThrowable) {
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].worth > out[j-1].worth || out[j].worth == out[j-1].worth && out[j].spell == 0 && out[j-1].spell != 0); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
}

// mercThrowablesLocked is every potion of its path the merc may throw (its
// book's: the file's doc), best first. Must be called with the lock held.
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
		var ok bool
		if t.worth, ok = ms.throwWorth(&t.pe); ok {
			out = append(out, t)
		}
	}
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if containsInt(ms.kit.SpellsOff, s.SpellID) {
			continue // turned off
		}
		if mercPotionSpell(s) {
			if !ms.slotted(s.SpellID) {
				continue // a potion spell left off its ten slots (mercloadout.go)
			}
			add(mercThrowable{pe: PotionEffect{ItemName: s.Name, Name: s.Name, Icon: s.Icon, Def: s.Def}, spell: s.SpellID})
			continue
		}
		// a refinement's potions whatever its ten slots hold (petition 2195,
		// a player: "when you use a alc merc and change its class MC they
		// dont make any of thore potions"): a refinement is never cast, so its
		// window never lists it and no slot can hold it (mercWindowBook); with
		// its slots set it brewed none, threw none of its path's and kept its
		// draughts alone
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
	sortThrowables(out)
	return out
}

// mercAoE is whether the merc throws its area cocktails (/merc aoe; a
// Cocktail alchemist's alone).
func (ms *mercState) mercAoE() bool { return ms.kit.Cocktail && ms.kit.AoE }

// mercCarriedThrowsLocked is the thrown potions the merc carries and those of
// its owner's stock, best first, each marked where it is from. Must be called
// with the lock held.
func (w *World) mercCarriedThrowsLocked(ms *mercState) (carried, stock []mercThrowable) {
	for _, c := range ms.kit.Carried {
		pe := w.potions.byPattern[c.PatternID]
		if pe == nil || c.Count <= 0 || c.Food {
			continue
		}
		if v, ok := ms.throwWorth(pe); ok {
			carried = append(carried, mercThrowable{pe: *pe, carried: c.ID, worth: v})
		}
	}
	for id, n := range w.mercSupply.stockOf(ms.owner) {
		pe := w.potions.byPattern[id]
		if pe == nil || n <= 0 {
			continue
		}
		if v, ok := ms.throwWorth(pe); ok {
			stock = append(stock, mercThrowable{pe: *pe, stock: true, worth: v})
		}
	}
	sortThrowables(carried)
	sortThrowables(stock)
	return carried, stock
}

// mercTeamIDsLocked is the ids of the merc's team (mercTeamLocked). Must be
// called with the lock held.
func (w *World) mercTeamIDsLocked(ms *mercState) map[EntityID]bool {
	out := map[EntityID]bool{}
	for _, mm := range w.mercTeamLocked(ms) {
		out[mm.id()] = true
	}
	for _, om := range w.mercsOfLocked(ms.owner) {
		out[om.npc.ServerID] = true
		if om.pet != nil && om.pet.npc != nil {
			out[om.pet.npc.ServerID] = true
		}
	}
	if ps := w.pets[ms.owner.ServerID]; ps != nil && ps.npc != nil {
		out[ps.npc.ServerID] = true
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

// mercThrowPickLocked is the potion the merc throws at t now, false for none
// (the file's doc): what it carries, then its owner's stock, then its path's,
// each of its kind (/merc aoe or single) first; an area potion only at a
// creature fighting its team; a spell for undead alone only at undead. Must
// be called with the lock held.
func (w *World) mercThrowPickLocked(ms *mercState, t *NPC) (mercThrowable, bool) {
	return w.mercThrowPickAtLocked(ms, t, time.Time{})
}

// mercThrowPickAtLocked is mercThrowPickLocked at now: a debuff thrown a
// moment before is not thrown again (mercselfpotion.go).
func (w *World) mercThrowPickAtLocked(ms *mercState, t *NPC, now time.Time) (mercThrowable, bool) {
	aoe := ms.mercAoE()
	engaged := t != nil && w.mercTeamIDsLocked(ms)[t.CurrentEnemy]
	may := func(c *mercThrowable) bool {
		if c.pe.Area > 0 && !engaged {
			return false // it would pull what it splashes
		}
		cs := c.pe.castSpell()
		return t == nil || w.petUndeadMayLocked(&cs, t)
	}
	first := func(list []mercThrowable, kind func(*mercThrowable) bool) (mercThrowable, bool) {
		for i := range list {
			if (kind == nil || kind(&list[i])) && may(&list[i]) {
				return list[i], true
			}
		}
		return mercThrowable{}, false
	}
	// its Master Class's thrown debuff first, at a creature fighting its team
	// with none of it on (a Transmuter's Transmutive Potion: mercselfpotion.go)
	if engaged {
		for _, c := range w.mercBookDebuffsLocked(ms) {
			id := PotionEffectID(c.pe.PatternID)
			if until, ok := ms.ready[id]; ok && now.Before(until) {
				continue // thrown a moment ago (a creature it did not take on)
			}
			if !npcHasEffect(t, id) && may(&c) {
				return c, true
			}
		}
	}
	own := func(c *mercThrowable) bool { return (c.pe.Area > 0) == aoe }
	carried, stock := w.mercCarriedThrowsLocked(ms)
	book := w.mercThrowablesLocked(ms)
	for _, try := range []struct {
		list []mercThrowable
		kind func(*mercThrowable) bool
	}{{carried, own}, {stock, own}, {book, own}, {carried, nil}, {stock, nil}} {
		if c, ok := first(try.list, try.kind); ok {
			return c, true
		}
	}
	if aoe { // no cocktail may go: its single-target potions
		return first(book, nil)
	}
	return mercThrowable{}, false
}

// mercUsedLocked takes one of what the merc threw or drank off where it came
// from: its carried record, else its owner's stock (the handlers told, to
// keep it); its path's own is never used up. Must be called with the lock
// held.
func (w *World) mercUsedLocked(ms *mercState, carried int, stock bool, patternID int) {
	switch {
	case carried != 0:
		for i := range ms.kit.Carried {
			c := &ms.kit.Carried[i]
			if c.ID != carried {
				continue
			}
			c.Count--
			if c.Count <= 0 {
				ms.kit.Carried = append(ms.kit.Carried[:i:i], ms.kit.Carried[i+1:]...)
			}
			break
		}
		w.noteMercLocked(MercNote{Kind: MercNoteUsed, Owner: ms.owner, Slot: ms.slot, Name: ms.npc.Name, ItemID: carried, PatternID: patternID, Count: 1})
	case stock:
		if w.mercSupply.take(ms.owner, patternID, 1) > 0 {
			w.noteMercLocked(MercNote{Kind: MercNoteStockUsed, Owner: ms.owner, Slot: ms.slot, Name: ms.npc.Name, PatternID: patternID, Count: 1})
		}
	}
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
	what, ok := w.mercThrowPickAtLocked(ms, t, now)
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
	w.mercUsedLocked(ms, what.carried, what.stock, what.pe.PatternID)
	if what.debuff {
		if ms.ready == nil {
			ms.ready = map[int]time.Time{}
		}
		ms.ready[PotionEffectID(what.pe.PatternID)] = now.Add(max(what.pe.duration()/2, mercSelfPotionAgain/5))
	}
	ms.throwReady = mercNextThrow(ms.throwReady, s.Delay, now)
	ms.throwing = &mercThrowing{target: t.ServerID, lands: now.Add(mercThrowLand), what: what}
	if land := now.Add(mercThrowLand); ms.busy.Before(land) {
		ms.busy = land // it stands for its throw (a cast's own time kept)
	}
	breakNPCInvis(m)
	w.broadcastNPCSwingLocked(m, t.ServerID, uint32(s.Delay/time.Millisecond), HashResourceID(SwingEffectName(s.Kind)))
	log.Printf("Merc: %s's %q throws %q at %s (%d power left)", ms.owner.Name, m.Name, what.pe.ItemName, t.Name, m.Power)
	return true
}

// mercThrowOnBeat is how late a throw may come and keep its round's beat: the
// next throw is then due a round after the last one was, not a round after
// this tick, so the tick's lateness does not add up round after round.
const mercThrowOnBeat = 300 * time.Millisecond

// mercNextThrow is when the merc's next throw is due after one thrown now, the
// last one due at was (zero: none): a round on from the last one's due time
// while the throw came on its beat, else a round from now.
func mercNextThrow(was time.Time, delay time.Duration, now time.Time) time.Time {
	if !was.IsZero() && !now.Before(was) && now.Sub(was) <= mercThrowOnBeat {
		return was.Add(delay)
	}
	return now.Add(delay)
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
		team := w.mercTeamIDsLocked(ms)
		place := areaPlace{world: t.World, caster: [3]float32{m.X, m.Y, m.Z}, centre: [3]float32{t.X, t.Y, t.Z}, onOther: true}
		for _, id := range w.SpatialIndex.QueryRadius(t.X, t.Z, pe.Area) {
			o := w.NPCs[id]
			if o == nil || o == t || o.Dead || o.World != t.World || !mercMayFight(m, o) || o.NPCType&0x80 != 0 || o.hiddenAt(now) ||
				!team[o.CurrentEnemy] { // what fights its team alone: it pulls nothing (the file's doc)
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

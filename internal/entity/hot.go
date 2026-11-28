package entity

import "time"

// hot.go — a heal over time on a player, as a GM has the live game's (F3
// 20261025-222354; the production server's rules):
//
//   - with no heal over time on the player, the first tick lands at once and
//     the next at the regular tick, then every regular tick ("2 seconds into
//     a 5 second tick, the first tick of the heal over time will be instant,
//     then the next tick at normal time");
//   - with a stronger one on, a weaker one does nothing;
//   - over a weaker one with ticks left, no tick at once: the next regular
//     tick is the new one's, and it has all its ticks; the workbook's first
//     tick twice is a landing at once's alone, so that tick lands once
//     (Live petition 5643).
//
// The regular tick is the server's six-second regen tick (regenSystem,
// regen.go). The reference server's recorded Minor Healing ticked at its landing and
// then every six seconds from it; with no regen tick yet (a world that has
// not run one, the tests) the ticks keep that rhythm. A heal over time has
// as many ticks as its duration holds (24 s: four, "63 x4"), the first one
// at once included. Strength is a tick's hit points, as each landed from its
// caster; an equal one replaces (a recast keeps a heal going).
//
// Slots (Heirloom petition 2756: "Marr's Blessing, froglok mc mca, it doesnt
// stack with cleric tick heals ... the mca is set to target slot when it should
// be group slot. also, the higher heal spell cleric heal over rides the same
// slot mca so that works, but when the cleric spell wears off the lower spell
// with lower effects and longer duration should still be there"): the rules
// above hold within a heal's slot (HoTSlot: its workbook stack type, else its
// scope's, hotSlot), so a single target's heal and a group's tick side by side.
// Within a slot a stronger one landing over a weaker no longer takes it off: the
// weaker stays on, its time running, and does not tick while the stronger does
// (tickEffectLocked: hotHeldLocked); once the stronger ends, the weaker ticks
// again at the regular tick for whatever of its time is left. It is kept over a
// logout and a restart as any effect (buffsave.go: its slot with it).

// hotOwnSlots are the heals over time that tick in a slot of their own, beside every other heal over time on the
// bearer and never held under a stronger one (the owner, 2026-11-28: "elder, faithful, bard heals should stack", then
// "savage heal should stack also with other HoT"): the cleric's Faithful Healing, the shaman's Savage Healing, and the
// bard's heal songs, which share one bard slot (a higher Hymn or Celebration still takes a lower's place, as one
// line). Tribal Ward's heal, the shaman's Elder mastery, is set off by a hit and has its proc's own slot
// (workbookmore.go's procHoTSlot). A slot here is clear of the workbook's stack types (1-4) and of every proc's.
var hotOwnSlots = map[int]int{
	701138: 701138, // Faithful Healing (cleric 60)
	700643: 700643, // Savage Healing (shaman 49)
	// the bard's heal songs, one slot
	144: hotBardSlot, 700034: hotBardSlot, // Celebration / Hymn of Vigor
	166: hotBardSlot, 700234: hotBardSlot, // of Health
	181: hotBardSlot, 700380: hotBardSlot, // of Life
	197: hotBardSlot, 700524: hotBardSlot, // of Vitae
	701117: hotBardSlot, 700788: hotBardSlot, // of Nife
}

// hotBardSlot is the bard's heal songs' slot (hotOwnSlots).
const hotBardSlot = 1_000_005

// hotTicks is how many ticks a heal over time of that duration has.
func hotTicks(dur time.Duration) int {
	return max(1, int((dur+effectTick/2)/effectTick))
}

// hotSlot is the slot a heal over time of the spell ticks in (the file's doc):
// the workbook's stack type, else its scope's (scopeStack: 1 a single target, 2
// self, 3 a group).
func hotSlot(s *CastSpell) int {
	if slot, ok := hotOwnSlots[s.SpellID]; ok {
		return slot
	}
	if s.Def.Workbook && s.Def.Stack > 0 {
		return s.Def.Stack
	}
	return scopeStack(s.Scope)
}

// activeHoT is the player's heal over time of that slot with the most each
// tick that is still ticking, nil when none is.
func activeHoT(p *Player, slot int, now time.Time) *ActiveEffect {
	var best *ActiveEffect
	for _, e := range p.Effects {
		if e.HPTick <= 0 || e.NextTick.IsZero() || !now.Before(e.Until) || e.HoTSlot != slot {
			continue
		}
		if best == nil || e.HPTick > best.HPTick {
			best = e
		}
	}
	return best
}

// placeHoTOfLocked is placeHoTLocked for a heal of the spell, its slot set.
func (w *World) placeHoTOfLocked(t *Player, ae *ActiveEffect, s *CastSpell, now time.Time) bool {
	ae.HoTSlot = hotSlot(s)
	return w.placeHoTLocked(t, ae, now)
}

// placeHoTLocked sets a new heal over time's ticks by the rules above, and
// reports whether it goes on at all. A heal it replaces comes off here.
func (w *World) placeHoTLocked(t *Player, ae *ActiveEffect, now time.Time) bool {
	ticks := hotTicks(ae.Until.Sub(now))
	if ae.Ticks > 0 {
		ticks = ae.Ticks // its line's own count ("3 ticks")
	}
	old := activeHoT(t, ae.HoTSlot, now)
	switch {
	case old == nil:
		ae.NextTick, ae.Ticks = now, ticks // addEffectLocked ticks it at once
		return true
	case ae.HPTick < old.HPTick:
		return false
	}
	next := old.NextTick
	if !next.After(now) {
		next = w.nextRegularTick(now, now)
	}
	// the workbook's first tick twice is a landing at once's alone: over one
	// ticking, its first tick is the old one's next and lands once (Live
	// petition 5643, "all healers heals stack with each other": kept, every
	// heal cast over another, or recast, added a whole tick at the next
	// regular one, so two healers taking turns healed half again as much as
	// the stronger alone; petition 5561 had found it in Tribal Ward's proc)
	ae.Rules.Twice = false
	// an equal one of its slot is replaced; a weaker one stays on, held while
	// this one ticks (hotHeldLocked)
	for _, e := range append([]*ActiveEffect(nil), t.Effects...) {
		if e.HPTick > 0 && e.SpellID != ae.SpellID && !e.NextTick.IsZero() && e.HoTSlot == ae.HoTSlot && e.HPTick >= ae.HPTick {
			w.removeEffectLocked(t, e.SpellID)
		}
	}
	ae.NextTick, ae.Ticks = next, ticks
	if last := next.Add(time.Duration(ticks-1) * effectTick); !last.Before(ae.Until) {
		ae.Until = last.Add(time.Millisecond) // its last tick before it ends
	}
	return true
}

// hotHeldLocked reports whether ae, a heal over time on t, is held at now: a
// stronger one of its slot ticks (the file's doc). Its tick then passes, its
// count kept. Must be called with the lock held.
func hotHeldLocked(t *Player, ae *ActiveEffect, now time.Time) bool {
	if ae.HPTick <= 0 || ae.PowerTick != 0 {
		return false
	}
	best := activeHoT(t, ae.HoTSlot, now)
	return best != nil && best != ae && best.HPTick > ae.HPTick
}

// nextRegularTick is the first regular tick after now, for an effect whose
// tick was due at prev: the regen tick's rhythm once there is one, else six
// seconds after prev.
func (w *World) nextRegularTick(prev, now time.Time) time.Time {
	if w.regenAt.IsZero() {
		return prev.Add(effectTick)
	}
	n := w.regenAt.Add(RegenInterval)
	for !n.After(now) {
		n = n.Add(RegenInterval)
	}
	return n
}

// advanceTickLocked moves an effect past the tick it is running: a counted
// heal to the next regular tick, or no more once its ticks are spent; any
// other effect six seconds on.
func (w *World) advanceTickLocked(ae *ActiveEffect, now time.Time) {
	if ae.Ticks <= 0 {
		ae.NextTick = ae.NextTick.Add(effectTick)
		return
	}
	ae.Ticks--
	if ae.Ticks == 0 {
		ae.NextTick = time.Time{}
		return
	}
	prev := ae.NextTick
	next := w.nextRegularTick(prev, now)
	// never a second tick within half a tick of the last (Heirloom petition
	// 2221, a player, Marr's Blessing: "it will double tick a lot, have
	// ticks go in between the tick, and then stop ticking a while before it
	// wears off"). The regen tick falls every 60th world tick
	// (regenSystem), later than six seconds whenever a world tick runs long,
	// and regenAt is when it last fell: a tick due just before a late regen
	// tick was set from the regenAt before it, and the next from the late
	// one, a moment after it, so the counted ticks ran out early, two at a
	// time.
	for next.Sub(prev) < RegenInterval/2 {
		next = next.Add(RegenInterval)
	}
	ae.NextTick = next
}

// activeNPCHoT is activeHoT for a pet's (or a merc's) effects: its heal over
// time of that slot with the most each tick that is still ticking.
func activeNPCHoT(n *NPC, slot int, now time.Time) *ActiveEffect {
	var best *ActiveEffect
	for _, e := range n.Effects {
		if e.HPTick <= 0 || e.NextTick.IsZero() || !now.Before(e.Until) || e.HoTSlot != slot {
			continue
		}
		if best == nil || e.HPTick > best.HPTick {
			best = e
		}
	}
	return best
}

// npcHoTHeld is hotHeldLocked for a pet's or a merc's heal over time: a
// stronger one of its slot ticks, so this one waits (Heirloom petition 3364).
func npcHoTHeld(n *NPC, ae *ActiveEffect, now time.Time) bool {
	if ae.HPTick <= 0 || ae.PowerTick != 0 {
		return false
	}
	best := activeNPCHoT(n, ae.HoTSlot, now)
	return best != nil && best != ae && best.HPTick > ae.HPTick
}

// placePetHoTLocked is placeHoTLocked for a pet (Heirloom petitions 2496 and 2499, 2026-11-20: a druid's Celestial
// Healing on an enchanter's pet showed "the initial tick" and none after, and "every time i get a 1500 stack ... on
// live if you casted a tick on a player that already has a tick on them it wouldn't start it over"): a pet's heal
// over time was put on as any effect, its first tick (twice, by the workbook's rule) due at once, so a recast every
// cast time landed that first tick again and the regular ones never came. Now as on a player: with none ticking the
// first tick lands at once; a weaker one does nothing; over one ticking, the next tick is the old one's, with all
// the new one's ticks. Since petition 3364 by slots as a player's (hotSlot): within a slot an equal one is replaced
// and a weaker one held (npcHoTHeld), and another slot's ticks beside it. It reports whether the new one goes on.
func (w *World) placePetHoTLocked(pet *NPC, ae *ActiveEffect, now time.Time) bool {
	old := activeNPCHoT(pet, ae.HoTSlot, now)
	if old == nil {
		ae.NextTick = now
		return true
	}
	if ae.HPTick < old.HPTick {
		return false
	}
	ae.Rules.Twice = false // its first tick is the old one's next, once (placeHoTLocked; Live petition 5643)
	next := old.NextTick
	if !next.After(now) {
		next = w.nextRegularTick(now, now)
	}
	// an equal one of its slot is replaced; a weaker one stays on, held while
	// this one ticks (npcHoTHeld), and a heal of another slot (a group's beside
	// a single target's) ticks beside it, as on a player (Heirloom petition
	// 3364: Radiance on a merc took Celestial Healing off it)
	left := pet.Effects[:0]
	for _, e := range pet.Effects {
		if e.HPTick > 0 && !e.NextTick.IsZero() && e.SpellID != ae.SpellID && e.HoTSlot == ae.HoTSlot && e.HPTick >= ae.HPTick {
			continue
		}
		left = append(left, e)
	}
	for i := len(left); i < len(pet.Effects); i++ {
		pet.Effects[i] = nil
	}
	pet.Effects = left
	ae.NextTick = next
	if ticks := hotTicks(ae.Until.Sub(now)); ae.Ticks <= 0 {
		if last := next.Add(time.Duration(ticks-1) * effectTick); !last.Before(ae.Until) {
			ae.Until = last.Add(time.Millisecond)
		}
	}
	return true
}

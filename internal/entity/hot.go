package entity

import "time"

// hot.go — a heal over time on a player, as jib has the live game's (F3
// 20261025-222354; eqoaclassic's rules):
//
//   - with no heal over time on the player, the first tick lands at once and
//     the next at the regular tick, then every regular tick ("2 seconds into
//     a 5 second tick, the first tick of the heal over time will be instant,
//     then the next tick at normal time");
//   - with a stronger one on, a weaker one does nothing;
//   - over a weaker one with ticks left, no tick at once: the next regular
//     tick is the new one's, and it has all its ticks.
//
// The regular tick is the server's six-second regen tick (regenSystem,
// regen.go). Sandstorm's recorded Minor Healing ticked at its landing and
// then every six seconds from it; with no regen tick yet (a world that has
// not run one, the tests) the ticks keep that rhythm. A heal over time has
// as many ticks as its duration holds (24 s: four, "63 x4"), the first one
// at once included. Strength is a tick's hit points, as each landed from its
// caster; an equal one replaces (a recast keeps a heal going).

// hotTicks is how many ticks a heal over time of that duration has.
func hotTicks(dur time.Duration) int {
	return max(1, int((dur+effectTick/2)/effectTick))
}

// activeHoT is the player's heal over time with the most each tick that is
// still ticking, nil when none is.
func activeHoT(p *Player, now time.Time) *ActiveEffect {
	var best *ActiveEffect
	for _, e := range p.Effects {
		if e.HPTick <= 0 || e.NextTick.IsZero() || !now.Before(e.Until) {
			continue
		}
		if best == nil || e.HPTick > best.HPTick {
			best = e
		}
	}
	return best
}

// placeHoTLocked sets a new heal over time's ticks by the rules above, and
// reports whether it goes on at all. A heal it replaces comes off here.
func (w *World) placeHoTLocked(t *Player, ae *ActiveEffect, now time.Time) bool {
	ticks := hotTicks(ae.Until.Sub(now))
	old := activeHoT(t, now)
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
	for _, e := range append([]*ActiveEffect(nil), t.Effects...) {
		if e.HPTick > 0 && e.SpellID != ae.SpellID && !e.NextTick.IsZero() {
			w.removeEffectLocked(t, e.SpellID)
		}
	}
	ae.NextTick, ae.Ticks = next, ticks
	if last := next.Add(time.Duration(ticks-1) * effectTick); !last.Before(ae.Until) {
		ae.Until = last.Add(time.Millisecond) // its last tick before it ends
	}
	return true
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
	ae.NextTick = w.nextRegularTick(ae.NextTick, now)
}

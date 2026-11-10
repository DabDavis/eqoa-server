package entity

import (
	"fmt"
	"log"
	"time"
)

// creaturedot.go — a creature's damage over time (database.AbilityDoT; the owner, 2026-11-10: Innoruuk
// with "same abilities as red swarming death", whose kit's first ability is its 300 a tick damage over
// time, a half second's cast, kept up; until now a dot was a pet's alone, a creature's being its debuff,
// which lands on players alone).
//
// A creature's dot lands on what it fights: a player, as its debuff's damage tick does (classtemplate.go's
// templateDebuffLocked: each tick its figure at its ability's percent, less the player's resist to its
// element on the creature's template's dice, a 0x00DB and a line, a death), its ticks alone, no stat of
// its spell; or the creature of a scripted fight (scriptedFoes), each tick less the creature's template
// resists of its element (creatureResists) and landing as the fight's hit (castFoeHitLocked: its hate,
// its heal at a hit, its death). It ticks at its landing and every effectTick after, for its spell's time
// (a tick's line with no time: tickDuration). It is kept up as a pet's is (pettemplate.go): due on its
// clock, it is cast again only once the one on its target has no more than its cast's time and its
// landing left, so the new one lands as the old ends, never two of one creature's on one target (the
// old one is replaced); at a pet it waits, due. Its creature dead, gone or in another world, a tick does
// nothing: what is left of it runs out.

// creatureDoTWantedLocked is whether npc's damage over time a is wanted at target now: not on its way
// there, nor on it from npc with more than its cast and landing to run, nor at a pet. Must be called with
// the lock held.
func (w *World) creatureDoTWantedLocked(npc *NPC, a *templateAbility, target EntityID, now time.Time) bool {
	src := npc.figures()
	s := a.spellFor(src.Class, src.Level)
	if s == nil {
		return false
	}
	for _, c := range w.npcCasts {
		if c.npc == npc && c.target == target && c.spell.SpellID == s.SpellID {
			return false
		}
	}
	lead := a.castTime(s) + castLand
	if p := w.Players[target]; p != nil {
		for _, e := range p.Effects {
			if e.SpellID == s.SpellID && e.npcFoe == npc.ServerID && e.Until.Sub(now) > lead {
				return false
			}
		}
		return true
	}
	foe := w.castFoeLocked(npc, target)
	if foe == nil {
		return false // a pet: it waits, due
	}
	for _, e := range foe.Effects {
		if e.SpellID == s.SpellID && e.npcCaster == npc.ServerID && e.Until.Sub(now) > lead {
			return false
		}
	}
	return true
}

// templateDoTLocked lands a creature's damage over time on what it fights (the file's doc). Must be
// called with the lock held.
func (w *World) templateDoTLocked(c *npcCast, now time.Time) {
	npc := c.npc
	if p := w.Players[c.target]; p != nil {
		w.templateDebuffLocked(c, now) // its ticks alone (c.dot)
		return
	}
	foe := w.castFoeLocked(npc, c.target)
	if foe == nil {
		return
	}
	d := &c.spell.Def
	tick := 0
	for _, e := range d.Effects {
		if e.Kind == EffectHPTick && e.Amount < 0 && !e.Self {
			tick += templateTickFigure(e, c.pct)
		}
	}
	if tick <= 0 {
		return
	}
	dur := d.Duration
	if dur <= 0 {
		dur = tickDuration // a tick's line with no time (cast.go)
	}
	keep := foe.Effects[:0]
	for _, e := range foe.Effects {
		if e.npcCaster != npc.ServerID || e.SpellID != c.spell.SpellID {
			keep = append(keep, e)
		}
	}
	for i := len(keep); i < len(foe.Effects); i++ {
		foe.Effects[i] = nil
	}
	resist := c.spell.resist
	if c.spell.unresistable {
		resist = StatNone
	}
	ae := &ActiveEffect{SpellID: c.spell.SpellID, Name: c.spell.Name, Icon: c.spell.Icon, Caster: npc.ServerID, Until: now.Add(dur),
		NextTick: now, HPTick: -tick, npcCaster: npc.ServerID, Rules: EffectRules{Resist: resist, Harm: true}}
	foe.Effects = append(keep, ae)
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[foe.ServerID] = foe
	log.Printf("Template: %s's %q on %s: %d a tick for %v", npc.Name, c.spell.Name, foe.Name, tick, dur)
	w.tickNPCEffectLocked(foe, ae, now)
}

// npcDoTTickLocked is a tick of a creature's damage over time on the creature t it fights
// (tickNPCEffectLocked): the tick less t's template resists of its element, as the fight's hit. A tick
// whose creature is dead, gone or in another world does nothing. Must be called with the lock held.
func (w *World) npcDoTTickLocked(t *NPC, ae *ActiveEffect, now time.Time) {
	ae.NextTick = ae.NextTick.Add(effectTick)
	src := w.NPCs[ae.npcCaster]
	if src == nil || src.Dead || t.Dead || src.World != t.World || ae.HPTick >= 0 {
		return
	}
	n := -creatureResists(ae.Rules.Resist, t, ae.HPTick, now)
	ae.ticking = true // its own tick is no contact for itself (contactNPCLocked)
	defer func() { ae.ticking = false }()
	w.castFoeHitLocked(src, t, n, fmt.Sprintf("%q tick", ae.Name), now)
}

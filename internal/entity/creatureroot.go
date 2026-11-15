package entity

import (
	"fmt"
	"log"
	"time"
)

// creatureroot.go — a creature's root on the player it fights (database.AbilityRoot on a class template:
// work board #688, the GM Jibrish's Moogie Gearling, "a root ... pinned to spell 460 "Root" for its look").
// Until then a template's root was a pet's alone (pettemplate.go's petOnly).
//
// It is cast as the creature's other harmful casts are (classtemplate.go's templateTurnLocked: on its own
// clock, its every and first, from where it stands in its reach, its spell's 0x00B1 at the player with the
// spell's visual), at a player only (at a pet it waits, due), and lands as a harmful single-target spell: the
// player cannot move for CreatureRootHold (its run speed 0, told in 0x00F8, as the TEST menu's Freeze holds
// it), then moves again (the effect's end tells its speed back). Its spell is its look and its name and icon on
// the effects list, nothing else of it: no resist, no hit, no time of its record's. A death ends it (as every
// harmful effect), and the end of a metal duel ends a duel's creature's (metalv5.go); a logout drops it, a
// strip leaves it on (it is harm), and a second one lands in its place (a recast, whole again).

// CreatureRootHold is how long a creature's root holds a player (the GM Jibrish's 6 s).
const CreatureRootHold = 6 * time.Second

// creatureRootLine is the player's line as it lands (ours).
const creatureRootLine = "%s's %s holds you in place."

// templateRootLocked lands a templated NPC's root on the player it fights (the file's doc). Must be called
// with the lock held.
func (w *World) templateRootLocked(c *npcCast, now time.Time) {
	npc := c.npc
	p := w.Players[c.target]
	if p == nil || p.IsDead() || p.respawnPending() || !w.npcSharesLocked(npc, p) {
		return
	}
	ae := &ActiveEffect{SpellID: c.spell.SpellID, Name: c.spell.Name, Icon: c.spell.Icon, Caster: npc.ServerID,
		Until: now.Add(CreatureRootHold), Root: true, npcFoe: npc.ServerID}
	ae.Rules.Harm = true // a creature's harm: a strip leaves it on (strip.go)
	p.LastCombatTime = now
	w.addEffectLocked(p, ae, now)
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(creatureRootLine, npc.Name, c.spell.Name))
	}
	log.Printf("Template: %s's %q roots %s for %v (speed %.0f)", npc.Name, c.spell.Name, p.Name, CreatureRootHold, p.SpeedNow())
}

// creatureRootsOffLocked takes off p every creature's root whose creature (by its id) gone reports, its run
// speed told again. Must be called with the lock held.
func (w *World) creatureRootsOffLocked(p *Player, gone func(EntityID) bool) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if ae.Root && ae.npcFoe != 0 && ae.duelFoe == 0 && gone(ae.npcFoe) {
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Template: %s's root (%q) on %s lets go", w.entityNameLocked(ae.npcFoe), ae.Name, p.Name)
		}
	}
}

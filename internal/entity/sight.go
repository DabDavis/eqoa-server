package entity

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// sight.go — a line of sight between two bodies, off the game's collision
// (internal/ground's Sees): the user, 2026-11-07, "we need aggro working off
// LoS of the mob to the player not through walls, cielings" and "the rocks and
// trees should block aoes, also walls". It holds for:
//
//   - a creature noticing a player, on sight or as it heals itself (aggro.go);
//   - a social NPC coming to help, which sees the one attacked or its attacker
//     (socialaggro.go);
//   - an area spell, whoever casts it (area.go, classtemplate.go,
//     pettemplate.go, potion.go): it reaches a body only with nothing between
//     the body and its caster, nor between the body and its centre, looked at
//     as it begins and again as it lands, so a body that gets behind a rock in
//     time is spared. Its caster's line counts for its own target too. The user,
//     the same night, of Veeshan's Thunder and Fury, which lie round the boss's
//     target: "some were behind trees and rocks and got hit": they were hidden
//     from the boss and in plain sight of whoever it was hitting. A group's
//     spell is no area here: it finds its members wherever they stand in its
//     radius.
//
//   - a heal or a buff (any spell that is not harmful) on another player or a
//     pet: its caster must see the body, as it begins and again as it lands
//     (petition 1243, Dock: "healing allows LOS through obstructions. do not
//     allow heals through obstructions or buffs"). One aimed at a body out of
//     sight is refused before its power is spent; a group's spell leaves out
//     the members its caster cannot see; a body that gets behind something
//     before it lands is spared. Either line clear is enough (a low wall hides
//     no one from a healer). A resurrection is not held to it.
//
//   - a ranged weapon's shot or a throw (a bow, a thrown weapon or potion: the
//     ranged swings, kinds 8-11) at a creature: its shooter must see it as
//     each shot is loosed (Hoofbite's petition 1924: "Able to pull Tinael
//     Windstream through wall with ranged attack"); out of sight nothing
//     lands, no hate, no power paid, castNotSeen in red, the fight let go,
//     as a shot short of power is (rangedpower.go).
//
// Harmful single-target spells, melee, a fight once it has begun and a town
// guard's help stay as they were.
//
// Two lines are drawn, at a man's head (a body is the walk's capsule, 2 tall)
// and at his waist, the same heights at both ends. Either clear is enough for a
// creature to notice someone: the head sees over a low wall, and the waist is
// there for a creature whose head is in a low ceiling. An area spell asks the
// same (petition 1882, Brother, 2026-11-13: "Blade Dance for Bravo MC sometimes
// does not AoE attack"): it needed both until then, so anything over a man's
// waist sheltered whoever stood behind it, and the knee-high stones of the
// tenacious frost giants' camp in Tunaria (1.4 tall, the walls file's, 27 of
// them round the camp) spared the giant behind each from every Blade Dance at
// it, 13 times in an hour. Something as high as a man's head (a boulder, a
// trunk, a wall, a ceiling) still spares him. A big creature looks from
// the same heights, so a rock hides a player from a dragon as it would from a
// man. Ours, not measured:
// CLIENT has no such test (it never decides what attacks), and no capture
// could show one, for a creature on Sandstorm attacks through a floor or does
// not by rules no one recorded.

// Sight is what can be seen past (internal/ground's Sees): whether the straight
// line between two points meets nothing of the game's collision. A Ground that
// is not Sight blocks nothing.
type Sight interface {
	Sees(world uint8, x0, y0, z0, x1, y1, z1 float32) bool
}

// The heights over its feet a body's lines of sight are drawn at.
const (
	sightHead  float32 = 1.7
	sightWaist float32 = 0.8
)

// inSightLocked reports a line of sight between bodies with their feet at the
// two places in a world (the file's doc). Must be called with the lock held.
func (w *World) inSightLocked(world uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	s, ok := w.ground.(Sight)
	if !ok {
		return true
	}
	return s.Sees(world, x0, y0+sightHead, z0, x1, y1+sightHead, z1) ||
		s.Sees(world, x0, y0+sightWaist, z0, x1, y1+sightWaist, z1)
}

// clearLineLocked is what an area spell asks of the line between two bodies:
// a line of sight between them, at either height (inSightLocked; the file's
// doc). Must be called with the lock held.
func (w *World) clearLineLocked(world uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	return w.inSightLocked(world, x0, y0, z0, x1, y1, z1)
}

// castNotSeen refuses a heal or a buff on a body its caster cannot see (ours,
// as EverQuest's; no recording has one).
const castNotSeen = "You cannot see your target."

// helpSeenLocked reports whether a helpful spell (not harmful, no resurrection)
// from caster reaches a body at (x, y, z) in world: a line of sight between
// them (inSightLocked); its caster always, and every body for a harmful spell
// or a resurrection, which are not held to it here. Must be called with the
// lock held.
func (w *World) helpSeenLocked(caster *Player, d *SpellDef, world uint8, x, y, z float32, self bool) bool {
	if self || d.Harmful || d.Resurrects() > 0 {
		return true
	}
	return caster.World == world && w.inSightLocked(world, caster.X, caster.Y, caster.Z, x, y, z)
}

// helpHiddenLocked spares a body a helpful spell lands on out of its caster's
// sight (helpSeenLocked), with a log and, when the body is the cast's one
// target, castNotSeen to its caster. It reports the body spared. Must be called
// with the lock held.
func (w *World) helpHiddenLocked(cs *castState, caster *Player, casterSess *session.Session, name string, world uint8, x, y, z float32, self bool) bool {
	if w.helpSeenLocked(caster, &cs.spell.Def, world, x, y, z, self) {
		return false
	}
	log.Printf("LoS: %s's %q spares %s at (%.1f, %.1f, %.1f), out of sight from (%.1f, %.1f, %.1f)", caster.Name, cs.spell.Name, name, x, y, z, caster.X, caster.Y, caster.Z)
	if casterSess != nil && len(cs.targets) == 1 {
		w.sendColoredLocked(casterSess, castNotSeen, castRefusalRGB)
	}
	return true
}

// shotUnseenLocked reports a ranged swing (a bow's shot, a throw) at npc that
// its shooter cannot see (inSightLocked), told castNotSeen in red (sess nil:
// untold) and logged; a melee swing, never. Must be called with the lock held.
func (w *World) shotUnseenLocked(sess *session.Session, p *Player, npc *NPC) bool {
	if p.Swing.Kind < swingRange1 || p.Swing.Kind > swingRange4 {
		return false
	}
	if p.World == npc.World && w.inSightLocked(p.World, p.X, p.Y, p.Z, npc.X, npc.Y, npc.Z) {
		return false
	}
	log.Printf("LoS: %s's shot at %s at (%.1f, %.1f, %.1f) refused, out of sight from (%.1f, %.1f, %.1f)", p.Name, npc.Name, npc.X, npc.Y, npc.Z, p.X, p.Y, p.Z)
	if sess != nil {
		w.sendColoredLocked(sess, castNotSeen, castRefusalRGB)
	}
	return true
}

// areaPlace is where an area spell stands: its caster's feet and its centre's.
type areaPlace struct {
	world   uint8
	caster  [3]float32
	centre  [3]float32
	onOther bool // the centre is not the caster
}

// reachesLocked reports the area reaching a body at (x, y, z): a clear line from
// its caster, and from its centre when that is another's (the file's doc). Must
// be called with the lock held.
func (w *World) reachesLocked(a areaPlace, x, y, z float32) bool {
	if !w.clearLineLocked(a.world, a.caster[0], a.caster[1], a.caster[2], x, y, z) {
		return false
	}
	return !a.onOther || w.clearLineLocked(a.world, a.centre[0], a.centre[1], a.centre[2], x, y, z)
}

// areaLog writes what an area spell reached and spared, with where everyone
// stood, for finding a wrong call afterwards (the user: "lets look at LoS logs").
type areaLog struct {
	reached, spared []string
}

func (l *areaLog) note(reached bool, name string, x, y, z float32) {
	s := fmt.Sprintf("%s (%.1f, %.1f, %.1f)", name, x, y, z)
	if reached {
		l.reached = append(l.reached, s)
	} else {
		l.spared = append(l.spared, s)
	}
}

// write logs the line, a landing's or a beginning's; nothing when it holds no
// one.
func (l *areaLog) write(when, caster, spell string, a areaPlace) {
	if len(l.reached) == 0 && len(l.spared) == 0 {
		return
	}
	log.Printf("LoS: %s's %q %s from (%.1f, %.1f, %.1f), centre (%.1f, %.1f, %.1f): reached %s; out of sight %s", caster, spell, when,
		a.caster[0], a.caster[1], a.caster[2], a.centre[0], a.centre[1], a.centre[2], strings.Join(l.reached, ", "), strings.Join(l.spared, ", "))
}

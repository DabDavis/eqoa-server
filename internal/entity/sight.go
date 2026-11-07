package entity

import (
	"fmt"
	"log"
	"strings"
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
// Single-target spells, melee, a fight once it has begun and a town guard's
// help stay as they were.
//
// Two lines are drawn, at a man's head (a body is the walk's capsule, 2 tall)
// and at his waist, the same heights at both ends. Either clear is enough for a
// creature to notice someone: the head sees over a low wall, and the waist is
// there for a creature whose head is in a low ceiling. An area spell needs both,
// so a low rock shelters whoever crouches behind it. A big creature looks from
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

// clearLineLocked reports nothing between two bodies at either height (the
// file's doc: what an area spell asks). Must be called with the lock held.
func (w *World) clearLineLocked(world uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	s, ok := w.ground.(Sight)
	if !ok {
		return true
	}
	return s.Sees(world, x0, y0+sightHead, z0, x1, y1+sightHead, z1) &&
		s.Sees(world, x0, y0+sightWaist, z0, x1, y1+sightWaist, z1)
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

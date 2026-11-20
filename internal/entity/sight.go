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
//     spell is no area here: no centre's line, its caster's alone (below). A creature's line is looked at from its own heights too, a man's
//     times its body (clearFromBodyLocked), so a big boss's feet low in its
//     body do not hide everyone round it (the owner, 2026-11-17).
//
//   - a heal or a buff (any spell that is not harmful) on another player or a
//     pet: its caster must see the body as it begins (petition 1243:
//     "healing allows LOS through obstructions. do not allow heals through
//     obstructions or buffs"). One aimed at a body out of sight is refused
//     before its power is spent; a group's spell leaves out the members its
//     caster cannot see. Once begun it lands wherever either has gone since
//     (petition 2494, 2026-11-20: "in live that you just needed los when you
//     casted and could move out of the way"; it had spared a body gone behind
//     something before it landed). Either line clear is enough (a low wall
//     hides no one from a healer). A resurrection is not held to it.
//
//   - every other heal on another (the owner, 2026-11-18: "also healing
//     shouldnt fire if there is no Line of sight"): a group's spell with a
//     radius (a group heal, groupNearLocked) leaves out a member, pet or merc
//     its caster cannot see as it begins (as it lands too until petition
//     2494); a tap's take or a
//     drain over time's tick shared with the group (drainersLocked) and a
//     weapon's restoring proc (restoreProcLocked) go to the members their
//     giver sees; a pet heals its owner, its group or a member only in its
//     sight (npcSeesLocked: from its own heights too), else heals itself or
//     another it sees, or waits; a familiar heals its owner only in its sight;
//     a boss's raid heal (bossmoments.go) lands on those it sees. A merc's
//     heals were held to it before (merccast.go). A heal over time already on
//     a body keeps ticking out of its caster's sight: it is the bearer's now,
//     as it keeps ticking out of range. A heal on oneself, a creature's on
//     itself and a heal by a hit (a mark's, a worn item's, a proc's on its
//     wielder) need no line. No creature heals another creature (a template's
//     heal is its own), so none is held to it there.
//
//   - a ranged weapon's shot or a throw (a bow, a thrown weapon or potion: the
//     ranged swings, kinds 8-11) at a creature: its shooter must see it as
//     each shot is loosed (a player's petition 1924: "Able to pull Tinael
//     Windstream through wall with ranged attack"); out of sight nothing
//     lands, no hate, no power paid, castNotSeen in red, the fight let go,
//     as a shot short of power is (rangedpower.go).
//
// Harmful single-target spells, melee, a fight once it has begun and a town
// guard's help stay as they were.
//
// A player's shot or harmful spell at a creature bigger than a man looks at
// its body where it stands up (seesCreatureLocked): the target end's heights
// are a man's times its body (bodyScale, at most bodyKneeMost), the shooter's
// his own. Heirloom petition 2202 (2026-11-16, the Isle of Dread):
// "Megaladon fight doesnt seem right with range in the water I had to get on
// top of him to actually shoot him with a bow ability/attack". The shark (size
// 4, its body's edge 33) swims in the channel along the beach with its feet at
// 50.8, under the sand's 54.1, so a man's head over its feet (52.5) was under
// the beach from where a player stood on it: Burning Shot within its reach was
// refused out of sight while its back stood out of the water, and only a
// player over it could shoot. What hides a man from a man still hides it
// (anything as high as its scaled head: at most 3 times a man's).
//
// Two lines are drawn, at a man's head (a body is the walk's capsule, 2 tall)
// and at his waist, the same heights at both ends. Either clear is enough for a
// creature to notice someone: the head sees over a low wall, and the waist is
// there for a creature whose head is in a low ceiling. An area spell asks the
// same (petition 1882, 2026-11-13: "Blade Dance for Bravo MC sometimes
// does not AoE attack"): it needed both until then, so anything over a man's
// waist sheltered whoever stood behind it, and the knee-high stones of the
// tenacious frost giants' camp in Tunaria (1.4 tall, the walls file's, 27 of
// them round the camp) spared the giant behind each from every Blade Dance at
// it, 13 times in an hour. Something as high as a man's head (a boulder, a
// trunk, a wall, a ceiling) still spares him. A big creature looks from
// the same heights, so a rock hides a player from a dragon as it would from a
// man. Ours, not measured:
// CLIENT has no such test (it never decides what attacks), and no capture
// could show one, for a creature on the reference server attacks through a floor or does
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

// seesCreatureLocked reports a player's line of sight to a creature, as he
// shoots it or begins a harmful spell at it (the file's doc): either line
// from him to its body at a man's heights (inSightLocked), or, for a body
// bigger than a man's, to its own heights (bodyScale). Must be called with
// the lock held.
func (w *World) seesCreatureLocked(p *Player, npc *NPC) bool {
	if p.World != npc.World {
		return false
	}
	if w.inSightLocked(p.World, p.X, p.Y, p.Z, npc.X, npc.Y, npc.Z) {
		return true
	}
	k := bodyScale(npc)
	s, ok := w.ground.(Sight)
	if !ok || k <= 1 {
		return false
	}
	return s.Sees(p.World, p.X, p.Y+sightHead, p.Z, npc.X, npc.Y+sightHead*k, npc.Z) ||
		s.Sees(p.World, p.X, p.Y+sightWaist, p.Z, npc.X, npc.Y+sightWaist*k, npc.Z)
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

// npcSeesLocked reports a creature's line of sight (a pet's, a familiar's, a
// boss's) to a body at (x, y, z) in world it would heal: either of a man's
// lines, or from its own heights when it is bigger (clearFromBodyLocked). Must
// be called with the lock held.
func (w *World) npcSeesLocked(n *NPC, world uint8, x, y, z float32) bool {
	return n.World == world && w.clearFromBodyLocked(world, bodyScale(n), n.X, n.Y, n.Z, x, y, z)
}

// playerSeesLocked reports a player's line of sight to another (inSightLocked),
// itself always: what a heal from p (a proc's, a drain's share) needs to reach
// t. Must be called with the lock held.
func (w *World) playerSeesLocked(p, t *Player) bool {
	return p == t || p.World == t.World && w.inSightLocked(p.World, p.X, p.Y, p.Z, t.X, t.Y, t.Z)
}

// shotUnseenLocked reports a ranged swing (a bow's shot, a throw) at npc that
// its shooter cannot see (seesCreatureLocked), told castNotSeen in red (sess nil:
// untold) and logged; a melee swing, never. Must be called with the lock held.
func (w *World) shotUnseenLocked(sess *session.Session, p *Player, npc *NPC) bool {
	if p.Swing.Kind < swingRange1 || p.Swing.Kind > swingRange4 {
		return false
	}
	if w.seesCreatureLocked(p, npc) && // its body, as big as it is
		!w.playerWalledOffLocked(p, npc, "shot") { // nor through a quest wall (questwallfight.go, petition 2198)
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
	// casterBody is a creature caster's body against a man's (bodyScale): its
	// line to a body is looked at from its own heights too (clearFromBodyLocked).
	// 0 or 1: a man's, a player's cast.
	casterBody float32
}

// npcAreaPlace is where a creature's area spell round centre stands: its feet,
// the centre, and its body (casterBody).
func npcAreaPlace(npc *NPC, cx, cy, cz float32, onOther bool) areaPlace {
	return areaPlace{world: npc.World, caster: [3]float32{npc.X, npc.Y, npc.Z}, centre: [3]float32{cx, cy, cz},
		onOther: onOther, casterBody: bodyScale(npc)}
}

// clearFromBodyLocked is an area spell's line from a caster's body k times a
// man's (bodyScale) at its feet (x0, y0, z0) to a body at (x1, y1, z1): either
// of a man's lines clear (clearLineLocked), or for a body bigger than a man's
// either from its own heights, a man's times k, to the other's (the owner,
// 2026-11-17: "the raid bosses aoes should be able to hit all targets within
// range"). A dragon's or a shark's feet sit low in its body, under the ledge
// or the sand round it (the megaladon's at 50.8 under the beach's 54.1,
// seesCreatureLocked's doc), so a man's head over them was under the ground
// everyone round it stood on and its breath reached no one there. What hides
// a man from it still hides him: a wall or a boulder as high as its eyes.
// Must be called with the lock held.
func (w *World) clearFromBodyLocked(world uint8, k, x0, y0, z0, x1, y1, z1 float32) bool {
	if w.clearLineLocked(world, x0, y0, z0, x1, y1, z1) {
		return true
	}
	s, ok := w.ground.(Sight)
	if !ok || k <= 1 {
		return false
	}
	return s.Sees(world, x0, y0+sightHead*k, z0, x1, y1+sightHead, z1) ||
		s.Sees(world, x0, y0+sightWaist*k, z0, x1, y1+sightWaist, z1)
}

// reachesLocked reports the area reaching a body at (x, y, z): a clear line from
// its caster (from a creature's own heights too, clearFromBodyLocked), and from
// its centre when that is another's (the file's doc). Must be called with the
// lock held.
func (w *World) reachesLocked(a areaPlace, x, y, z float32) bool {
	if !w.clearFromBodyLocked(a.world, a.casterBody, a.caster[0], a.caster[1], a.caster[2], x, y, z) {
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

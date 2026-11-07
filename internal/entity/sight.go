package entity

// sight.go — a line of sight between two bodies, off the game's collision
// (internal/ground's Sees): the user, 2026-11-07, "we need aggro working off
// LoS of the mob to the player not through walls, cielings" and "the rocks and
// trees should block aoes, also walls". It holds for:
//
//   - a creature noticing a player, on sight or as it heals itself (aggro.go);
//   - a social NPC coming to help, which sees the one attacked or its attacker
//     (socialaggro.go);
//   - an area spell's others, from its centre, whoever casts it (area.go,
//     classtemplate.go, pettemplate.go, potion.go), looked at as it begins and
//     again as it lands, so a body that gets behind a rock in time is spared.
//     A caster's area spares its own target too; one round its target has that
//     target at its centre. A group's spell is no area here: it finds its
//     members wherever they stand in its radius.
//
// Single-target spells, melee, a fight once it has begun and a town guard's
// help stay as they were.
//
// Two lines are drawn, at a man's head (a body is the walk's capsule, 2 tall)
// and at his waist, the same heights at both ends, and either clear is sight:
// the head sees over a low wall, and the waist is there for a creature whose
// head is in a low ceiling. A big creature looks from the same heights, so a
// rock hides a player from a dragon as it would from a man. Ours, not measured:
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

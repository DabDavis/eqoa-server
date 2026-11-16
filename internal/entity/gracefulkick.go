package entity

import "strings"

// gracefulkick.go — the bard's Graceful Strike delivered as a kick (the
// owner, 2026-11-16: "lets add a 0 cost cm for bards graceful strike to kick
// animation"). A mastery rank whose effects carry GracefulKick (cmcatalog's
// effects, "GracefulKick:1": the rank "Graceful Kick", the bard's class list,
// cost 0, from level 16, the art's own; tools/seed_graceful_kick.py) makes
// its owner's Graceful Strike play the Kick art's visual in place of its own:
// the 0x00B1 its cast begins with names SPELLFX.CSF's "Kick" (0x0A286E2E)
// where the art's record names "Attack1" (0xDA9BEA11, a swing of the weapon
// in hand: event 15, PlayAttack). Kick's caster list is SetAction 43, the
// CPlayList's kick slot (claude-notes/anim-slot-semantics.md), then the hit
// on the target (its list: SetAction 34, a spark, a sound), read off the
// disc's SPELLFX record. Only the visual changes: the art's damage, its
// lockout, its cooldown and its timing are the art's. Removing the rank
// (cost 0: nothing lost) plays the swing again at the next use.

// fxKick is the Kick art's visual, HashResourceID("Kick").
const fxKick = 0x0A286E2E

// gracefulStrike is the art the rank changes, in lower case.
const gracefulStrike = "graceful strike"

// cmGracefulKick is the effects key of the rank (parseStatList's spelling
// of "GracefulKick").
const cmGracefulKick = "gracefulkick"

// castVisual is the 0x00B1 visual the player's cast of spell begins with:
// the spell's own, but a kick for Graceful Strike on a player with the rank.
func (p *Player) castVisual(spell *CastSpell) uint32 {
	if p.GracefulKick && strings.EqualFold(strings.TrimSpace(spell.Name), gracefulStrike) {
		return fxKick
	}
	return spell.visual()
}

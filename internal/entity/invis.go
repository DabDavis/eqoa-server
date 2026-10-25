package entity

import "log"

// invis.go — what ends invisibility, and invisibility on a pet.
//
// jib's F3 20261025-180117: "Casting a spell, or taking an action such as
// auto attacking should remove the invisibility buff. Invis should also be
// castable on friendly targets, such as pets or players." Invisibility at
// another player already landed on it (cast.go); at a pet it landed on the
// pet's effects but showed nothing: a creature's state byte 154 is set now
// as a player's is. What ends it here: its bearer beginning a cast, engaging
// a fight, and every round it swings; a pet's, every round it swings. No
// recording says what Sandstorm or SOE sent when invisibility broke; the
// effect simply goes from the effects channel.

// breakInvisLocked ends every invisibility on a player: casting a spell or
// swinging at something shows it (F3 20261025-180117, jib: "Casting a
// spell, or taking an action such as auto attacking should remove the
// invisibility buff").
func (w *World) breakInvisLocked(p *Player, why string) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Invis {
			w.removeEffectLocked(p, e.SpellID)
			log.Printf("Cast: %q ends on %s: %s", e.Name, p.Name, why)
		}
	}
}

// breakNPCInvis is breakInvisLocked for a creature (a pet that fights).
func breakNPCInvis(npc *NPC) {
	left := npc.Effects[:0]
	for _, e := range npc.Effects {
		if !e.Invis {
			left = append(left, e)
		}
	}
	for i := len(left); i < len(npc.Effects); i++ {
		npc.Effects[i] = nil
	}
	npc.Effects = left
}

// invisible is whether an effect hides the creature (a pet made invisible:
// state byte 154, as a player's).
func (n *NPC) invisible() bool {
	for _, e := range n.Effects {
		if e.Invis {
			return true
		}
	}
	return false
}

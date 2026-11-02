package entity

// invis.go — what ends invisibility, and invisibility on a pet.
//
// jib's F3 20261025-180117: "Casting a spell, or taking an action such as
// auto attacking should remove the invisibility buff. Invis should also be
// castable on friendly targets, such as pets or players." Invisibility at
// another player already landed on it (cast.go); at a pet it landed on the
// pet's effects but showed nothing: a creature's state byte 154 is set now
// as a player's is. What ends it here: its bearer beginning a cast
// (cast.go), engaging a fight and every round it swings (attack.go, duel.go),
// a potion it drinks or throws (potion.go: a drink before its own effects
// land, so an invisibility potion's stays), and every hit on it, whatever
// gave the invisibility (a creature's swing, spell or special, a training or
// sparring dummy's swing, a duel's hit: the workbook's own rule ends at a
// creature's swing only the parts it marks, which left the EQOA-Data lines'
// invisibilities on, Sneak's, Stalk's and Improved Invisibility's); a pet's,
// every round it swings. Nothing else of its bearer's ends it (the user,
// 2026-11-02: "invisibility was dropping when emoting, pressing square, but
// only should drop if casting a spell, or attacking a mob", then "potions
// should still break it, hits too"): from F3 20261026-014527 (jib:
// "Effectively any action other than movement, targeting and chat should
// break invis") until that day the handlers' every action message ended it
// before it was handled (an item used, talking to someone, the emotes,
// merchants, the bank, trades, looting, pet commands). A GM's Toggle Invis
// (GMHidden) is no effect, and nothing ends it.
// No recording says what Sandstorm or SOE sent when invisibility broke; the
// effect simply goes from the effects channel. An effect with more than its
// invisibility (Spirit Walk's speed, Superior Invisibility's skeleton) loses
// the invisibility alone, its invisible body with it, the rest staying to its
// time (the user, 2026-11-01: "yes only end the marked part"; parts.go).

// breakInvisLocked ends every invisibility on a player: casting a spell or
// swinging at something shows it (F3 20261025-180117, jib: "Casting a
// spell, or taking an action such as auto attacking should remove the
// invisibility buff"); an effect with more than its invisibility keeps the
// rest (parts.go).
func (w *World) breakInvisLocked(p *Player, why string) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Invis {
			w.endOnPlayerLocked(p, e, partInvisible, why)
		}
	}
}

// breakNPCInvis is breakInvisLocked for a creature (a pet that fights).
func breakNPCInvis(npc *NPC) {
	left := npc.Effects[:0]
	for _, e := range npc.Effects {
		if !e.Invis || endOnNPC(npc, e, partInvisible, "a swing") {
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

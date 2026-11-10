package entity

import "github.com/eqoa/server/internal/session"

// hitheal.go — a creature's heal at every hit on it (database.AbilityHitHeal; the GM Jibrish's petition
// 1439: "Give mr moogie a heal damage shield that heals mr moogie for 150 every single time he is hit").
//
// Every hit that lands on it, taking hit points, and leaves it standing heals it its hit heal's figure
// (an add's at its share: summon.go), up to its most, the hit itself landing whole as ever: a player's
// melee hit and bow shot (attack.go), a player's spell hit, damage over time tick, proc, weapon proc,
// worn item's answer and damage shield's (cast.go's spellHitHateLocked, which they all land through), a
// thrown potion's hit (a spell's), every hit of a pet's (its melee, spells, ticks, procs, taunts and
// damage shield: pet.go's petLandHitLocked), and an NPC's in a fight with it (its melee: guards.go; a
// scripted fight's casts, specials, kicks, procs and rampage: castFoeHitLocked). Under a heal shield the
// hit heals it whole and 1 lands, and that 1 is a hit that landed: this heal follows it too. Not a hit that
// kills it, one that takes nothing (a dummy's figures, a resist that left nothing), nor a player's price or
// a GM's kill. The heal is told to the hitter's client as a creature's heal on itself is (tellNPCHealLocked:
// the figure over it), when the hitter has one.

// hitHealLocked is npc's heal after a hit of n that landed on it and left it standing (the file's doc),
// told to sess (the hitter's, or its owner's: nil for none). Must be called with the lock held.
func (w *World) hitHealLocked(npc *NPC, n int, sess *session.Session) {
	t := npc.tmpl
	if t == nil || t.hitHeal <= 0 || n <= 0 || npc.Dead || npc.HP <= 0 || npc.HP >= npc.MaxHP {
		return
	}
	heal := min(npc.shared(t.hitHeal), npc.MaxHP-npc.HP)
	npc.HP += heal
	w.tellNPCHealLocked(sess, npc, heal)
}

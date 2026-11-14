package entity

// pettaunttank.go — a player tank's taunt takes a creature from a pet, and a pet leaves a grouped
// tank's creature to him (Live petition 5055, Innoruuk, a level 17 shadow knight, 2026-11-14: "Mage
// earth pet either has way too much aggro or Delf sk taunt not strong enough/accumulating enough
// aggro"; Live petition 5022's option (a)).
//
// Since 2026-11-14 a pet's taunt puts the pet over the top of the creature's hate by
// petTauntMargin: the larger of 10% of the top and 20 a pet level (pettauntmargin.go), 320 for a
// level 16 earth elementalkin. A player's taunt put its caster over the top by the taunt's own hate
// (taunt.go): Taunt's ~106 + 16% STR, ~120 at level 17, so the pet's next taunt every 12 s took the
// creature back over a margin the shadow knight's could not pass. Now:
//   - a player's taunt puts him over the top by at least the margin a pet of his level would get
//     (playerTauntFloor: the larger of the taunt's hate and petTauntMargin at his level), the tank's
//     105% (petition 1935) on it as before;
//   - a pet does not taunt (neither path: pet.go petTauntLocked, pettemplate.go's taunt ability) a
//     creature whose current enemy is a player tank (warrior, paladin, shadow knight) in its owner's
//     group, the owner not counted: the pet fights it, the tank keeps it. A creature on the pet's own
//     owner, or on a groupmate who is no tank (a healer, a caster), is taunted as before.

// playerTauntFloor is the hate a player's taunt at level puts its caster over the top by: its own
// hate, or the margin a pet of that level would take the creature by, whichever is more.
func playerTauntFloor(hate int, level int, top int64) int64 {
	m := petTauntMargin(&NPC{Level: level}, top)
	if int64(hate) > m {
		return int64(hate)
	}
	return m
}

// petSparesTankLocked reports whether the pet leaves t to the player tank of its owner's group whom
// t is on (the file's doc). Must be called with the lock held.
func (w *World) petSparesTankLocked(ps *petState, t *NPC) bool {
	if ps == nil || ps.owner == nil || ps.owner.GroupID == 0 || t == nil || t.CurrentEnemy == ps.owner.ServerID {
		return false
	}
	p := w.Players[t.CurrentEnemy]
	return p != nil && p.GroupID == ps.owner.GroupID && archetypeOf(p.Class) == archTank
}

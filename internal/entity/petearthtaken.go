package entity

// petearthtaken.go — a magician's earth pet takes a creature's melee as a player tank of its owner's
// level does (Live petition 5057, Innoruuk, 2026-11-14: "mage earth pet squishy, can't take hits very
// well").
//
// A player's hit from a creature is the creature's roll, the balance's level and con shares
// (npcHitLocked), then the share his class takes at his level (npcHitPlayerLocked: balanceclasslevel's
// melee_taken_pct, a level 19 warrior's 55%, a shadow knight's 52%). A pet's was the first two alone:
// on Live 2026-11-14 08:14-08:33 a level 24 crocodile hit Odium's earth elementalkin (owner level 19,
// 1885 HP, its sheet's AC 750 already at the roll's floor) for 55-62, the level 19 players beside it
// for 30-40. An earth pet (petState.earth: the "pet earth" template's, pettaunthate.go) now takes
// the warrior's share at its owner's level (its template's class is the warrior's), after which its
// template's taken_pct as before. Mercs keep their own (petTakenLocked), other pets as they were.

// earthPetTakenLocked is n of a creature's melee on an earth pet at the warrior's share of its
// owner's level (the file's doc); n as it is for any other pet. Must be called with the lock held.
func (w *World) earthPetTakenLocked(ps *petState, n int) int {
	if !ps.earth || ps.owner == nil || !balanced(ps.npc) || n <= 0 {
		return n
	}
	return pct(n, w.balance.MeleeTakenPctAt(classWarrior, ps.owner.Level))
}

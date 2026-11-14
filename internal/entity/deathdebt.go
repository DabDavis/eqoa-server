package entity

// deathdebt.go — the experience debt a death leaves, as the live game's (the
// user: "add xp debt on death like live"). Each death adds its level's
// "unrezzed debt number" to the character's debt, and half of every award
// pays it off until it is gone (splitXP).
//
// The number is CLIENT's ExpCost (decomp modules/client/src/viexperience.cpp,
// 0x00CC6E30, beside the kill experience model killexp.go gives): none below
// level 6, else LevelExp(level) times one more for each level past 19. The live
// server worked it on an x86 (double arithmetic, the 0.02f constant widened,
// the worth truncated), and that is the players' chart to the unit: the
// "XP to next Level" chart of eqoa.forumotion.com/t279-calculations (the same
// as eqoa.fandom.com/wiki/Formulas) at 47 of its 51 levels, its 51, 53, 55 and
// 60 (13,752,774) included; and SOE's retail save state of Corsten
// (claude-notes/frontiersforge-layouts-verified-20260819.md), a debt of
// 30,689,845 at level 49, five deaths of 6,137,969. The chart's other four
// (15: 14,778, 22: 129,735, 37: 1,934,308, 39: 2,429,523) are off the
// formula by 1.3 to 11% where every other level agrees to the unit, so they
// are taken as mis-copied. Until 2026-11-14 this server copied the chart, its
// four odd ones too, and made up 51, 53 and 55 to 60 (60's 13,809,547).
// A resurrection gives back its share of what the latest death added
// (resurrect.go).

// expCost is CLIENT's ExpCost(level) as the live server's x86 worked it.
func expCost(level int) int {
	if level < 6 {
		return 0
	}
	base := float64((level+1)*(level+1)) * 40
	worth := int(float64(float64(level)*float64(float32(0.02))*base) + base) // no fused multiply-add
	return (max(level-19, 0) + 1) * worth
}

// DeathDebt is the experience debt a death at level leaves.
func DeathDebt(level int) int {
	return expCost(min(level, MaxLevel))
}

// DebtOf is a player's experience debt.
func (w *World) DebtOf(id EntityID) (int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil {
		return 0, false
	}
	return p.Debt, true
}

// SetDebt sets a player's experience debt (a GM's #debt; 0 clears it) and
// tells its client (0x00D9). It reports whether the player is in the world.
func (w *World) SetDebt(id EntityID, debt int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil {
		return false
	}
	p.Debt = max(0, debt)
	sendExpDebt(w.findSessionByEntityIDLocked(id), p.Debt)
	return true
}

package entity

import "math"

// deathdebt.go — the experience debt a death leaves, as the live game's (the
// user: "add xp debt on death like live"). Each death adds its level's
// "unrezzed debt number" to the character's debt (the live game's chart,
// eqoa.fandom.com/wiki/Formulas, "XP to next Level Chart": levels 6-50, 52
// and 54), and half of every award pays it off until it is gone (splitXP).
// The chart is the live server's own: SOE's retail save state of Corsten
// (claude-notes/frontiersforge-layouts-verified-20260819.md) holds a debt of
// 30,689,845 at level 49, exactly five deaths of the chart's 6,137,969.
//
// Ours: no debt below level 6, where the chart starts; its blanks, 51 and 53
// the geometric mean of their neighbours, 55 to 60 the chart's own curve on
// from 54 (from 49 to 54 the debt grows as the fourth power of the level,
// within 0.2%). A resurrection, which the live game's restored part of the
// debt with, is not in this server yet.

// deathDebtChart is the live game's unrezzed debt by level.
var deathDebtChart = map[int]int{
	6: 2195, 7: 2918, 8: 3758, 9: 4719, 10: 5807, 11: 7027, 12: 8382, 13: 9878, 14: 11519,
	15: 14778, 16: 15259, 17: 17366, 18: 19638, 19: 22079, 20: 49390, 21: 82473, 22: 129735,
	23: 168190, 24: 221994, 25: 283913, 26: 354584, 27: 434646, 28: 524780, 29: 625669,
	30: 738036, 31: 862615, 32: 1000132, 33: 1151370, 34: 1317104, 35: 1498159, 36: 1695366,
	37: 1934308, 38: 2141560, 39: 2429523, 40: 2662682, 41: 2953637, 42: 3266064, 43: 3600950,
	44: 3959254, 45: 4342005, 46: 4750228, 47: 5184910, 48: 5647140, 49: 6137969, 50: 6658528,
	52: 7793276, 54: 9060444,
}

// DeathDebt is the experience debt a death at level leaves.
func DeathDebt(level int) int {
	level = min(level, MaxLevel)
	if d, ok := deathDebtChart[level]; ok {
		return d
	}
	switch {
	case level < 6:
		return 0
	case level == 51 || level == 53:
		return int(math.Round(math.Sqrt(float64(deathDebtChart[level-1]) * float64(deathDebtChart[level+1]))))
	}
	return int(math.Round(float64(deathDebtChart[54]) * math.Pow(float64(level)/54, 4)))
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

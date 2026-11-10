package entity

// ownfight.go — the fights a player wins or loses alone (the shard's GM
// Jibrish's petition 1491: "Correct it so mercenarys and healbot did not
// assist you in duels, pvp, or for mr moogies challenge").
//
// A mercenary (merc*.go) and a familiar (familiar.go: Healbot) already kept out
// of a duel and of a PvP fight as far as their own turns went: neither fights a
// player or a duel's pet, a merc never takes on a Metal Moogie, and no heal or
// buff of theirs was begun on a player flagged for PvP or in a duel. What still
// helped:
//   - Mr. Moogie's challenge (metalmoogie.go) was no duel to them: Healbot's
//     600 and a merc's heals and buffs landed on the challenger all through it
//     (Mr. Moogie himself refuses his challenger his buffs: MetalLineDueling);
//   - a merc's heal begun before the fight landed in it (a cast lands its cast
//     time and 1.4 s after it began: mercHelpLocked looked at nothing);
//   - a merc's heal over time put on before the fight went on healing through
//     it, tick by tick.
// Now one test says whose fight is its own (ownFightLocked): a player flagged
// for PvP, in a duel that is on, or dueling a Metal Moogie; and on such a
// player Healbot heals nothing, a merc begins no heal or buff (its own or
// another group member's merc: mercTeamLocked), a merc's cast on its way lands
// nothing, and a merc's heal over time already on it ticks nothing until the
// fight is over (the effect stays and runs out in its own time). What a player
// cast on itself, its own class pet (a duel's and a PvP fight's by their own
// rules: duelfight.go, pvp.go; Metal Moogie's challenger's own pet may hurt
// him) and other players' heals are as they were.

// ownFightLocked reports a player in a fight no mercenary or familiar helps in:
// flagged for PvP, in a duel that is on, or dueling a Metal Moogie (Mr. Moogie's
// challenge). Must be called with the lock held.
func (w *World) ownFightLocked(p *Player) bool {
	return p != nil && (p.pvp.on || w.duelOnLocked(p.ServerID) != nil || w.metalDuelOfLocked(p.ServerID))
}

// helperTickHeldLocked reports a heal over time's tick that does nothing now:
// a mercenary's, on a player whose fight is its own (ownFightLocked). Must be
// called with the lock held.
func (w *World) helperTickHeldLocked(t *Player, ae *ActiveEffect) bool {
	if n := w.NPCs[ae.Caster]; n == nil || !n.isMerc() {
		return false
	}
	return w.ownFightLocked(t)
}

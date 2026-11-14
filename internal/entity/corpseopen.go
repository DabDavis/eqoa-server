package entity

import (
	"time"
)

// corpseopen.go — a kill's corpse is its owners' for six minutes, then anyone's (the GM
// Jibrish's Live petition 5035, the owner's "use jibrish rule", 2026-11-14: "All NPC's should
// lock non members of the kill (solo), group or raid from looting a corpse for 6 minutes ...
// After 8 minutes, the corpse can be looted by anyone until minute 20, where it decays"; read as
// locked to 6:00, anyone's from 6:00 to 20:00, the "8" taken for the 6). It replaced the
// owner's earlier rule of the day (anyone's after five minutes, a raid's never).
//
// A corpse a player is credited with is its kill group's (handlers/lootrights.go): the killer,
// its group, its raid. From DefaultCorpseOpenAfter after its death (world.corpse_open_after) it
// opens for anyone, whatever is left on it then, a raid's too (its rolls and its master looter
// run within the lock as before: handlers/raidloot.go). A Metal Moogie's stays his challenger's
// (metalmoogie.go). It lies to 20 minutes after the death (corpselinger.go).

// DefaultCorpseOpenAfter is how long after its death a kill's corpse is its owners' alone.
const DefaultCorpseOpenAfter = 6 * time.Minute

// SetCorpseOpenAfter sets how long after its death a kill's corpse is its owners'
// alone: zero keeps the default, below zero always (every corpse its kill group's
// for as long as it lies).
func (w *World) SetCorpseOpenAfter(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if d != 0 {
		w.corpseOpenAfter = d
	}
}

// corpseOpenLocked is when the corpse of npc is anyone's (the file's doc), zero for never.
// Must be called with the lock held, at the death.
func (w *World) corpseOpenLocked(npc *NPC) time.Time {
	if w.corpseOpenAfter <= 0 || npc.DeathTime.IsZero() || npc.Owner != 0 || npc.CorpseOfNothing() ||
		npc.Charm != nil || npc.Dummy != NotDummy || npc.DuelSpawn() ||
		npc.challenge != nil || npc.Name == MetalMoogieName || npc.Name == MetalMoogieV2Name {
		return time.Time{}
	}
	return npc.DeathTime.Add(w.corpseOpenAfter)
}

// CorpseOpenAfter is how long after its death a kill's corpse is its owners' alone (0 or
// below: always).
func (w *World) CorpseOpenAfter() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.corpseOpenAfter
}

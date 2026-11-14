package entity

import (
	"time"
)

// corpseopen.go — a kill's corpse anyone's after five minutes, unless a raid's (the
// owner, 2026-11-14: "corpse if not looted for 5 mins should be open to all if not a
// raid").
//
// A corpse a player is credited with is its kill group's (handlers/lootrights.go).
// From DefaultCorpseOpenAfter after its death (world.corpse_open_after) one killed
// outside a raid opens for anyone, whatever is left on it then: a corpse partly
// looted included, the count running from the death, not from the last take. A
// raid's kill (the killer's group a raid at the death: flagged, or past four,
// raidrules.go) keeps its kill group and its raid's rules for as long as it lies.
// A Metal Moogie's stays his challenger's (metalmoogie.go).
//
// A corpse lies its decay (corpse.go, 3 minutes) unless emptied and closed first,
// so for the rule to be met at all such a corpse lies at least until
// CorpseOpenFor after it opens to anyone (HoldCorpse's hold): one not emptied comes
// back that much later; one emptied goes at once as ever.

// DefaultCorpseOpenAfter is how long after its death a non-raid kill's corpse is
// anyone's, and CorpseOpenFor how long it lies open to anyone at the least.
const (
	DefaultCorpseOpenAfter = 5 * time.Minute
	CorpseOpenFor          = time.Minute
)

// SetCorpseOpenAfter sets how long after its death a non-raid kill's corpse is
// anyone's: zero keeps the default, below zero never (every corpse its kill
// group's for as long as it lies, as before 2026-11-14).
func (w *World) SetCorpseOpenAfter(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if d != 0 {
		w.corpseOpenAfter = d
	}
}

// corpseOpenLocked is when the corpse of npc, killed by p, is anyone's (the file's
// doc), zero for never; a corpse that opens is held lying until CorpseOpenFor after.
// Must be called with the lock held, at the death.
func (w *World) corpseOpenLocked(p *Player, npc *NPC) time.Time {
	if w.corpseOpenAfter <= 0 || npc.DeathTime.IsZero() || npc.Owner != 0 || npc.CorpseOfNothing() ||
		npc.Charm != nil || npc.Dummy != NotDummy || npc.DuelSpawn() ||
		npc.challenge != nil || npc.Name == MetalMoogieName || npc.Name == MetalMoogieV2Name ||
		w.raidOfLocked(p) != nil {
		return time.Time{}
	}
	at := npc.DeathTime.Add(w.corpseOpenAfter)
	if until := at.Add(CorpseOpenFor); until.After(npc.corpseHeld) {
		npc.corpseHeld = until
	}
	return at
}

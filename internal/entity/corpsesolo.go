package entity

import (
	"time"
)

// corpsesolo.go — a corpse outside a raid (a GM's Heirloom petition 2752, the owner's "build
// it as asked", 2026-11-23: "Corpses should instead decay after 10 minutes, unlock after 7
// minutes unless in a raid ... If the player opens the corpse to check it, but doesnt loot
// it, reset the decay timer to 10 minutes each time they do it, maximum of 10 though. When
// they full loot it, clear the corpse. Repop timers for mobs should start after its
// corresponding mob corpse is looted or cleared. For raids, the current ... decay and unlock
// times are fine").
//
// A lootable corpse (lootableCorpse: not a pet's, an add's, a dummy's, a duel's, an
// instance's, the guards', ...) of a kill that is not a raid's:
//
//   - lies DefaultSoloCorpseDecay (world.corpse_decay_solo, 10 minutes) from the death, as one
//     corpse: no copy is left in its place (corpselinger.go), so its creature stays down with
//     it and its respawn time (world.respawn_after, npcrespawn) runs only from when it goes;
//   - is its kill group's alone for DefaultSoloCorpseOpenAfter (world.corpse_open_after_solo,
//     7 minutes), then anyone's (corpseopen.go, handlers/lootrights.go);
//   - each opening (RefreshCorpse, handlers/loot.go) puts its decay back to the full 10
//     minutes from then, at most DefaultCorpseChecksMax (world.corpse_checks_max, 10) times
//     a death; after that it decays as it stands;
//   - emptied and closed, it goes at once (ClearCorpse) and the respawn time runs from then.
//
// The rule is put on the corpse at the death (killNPC: soloDecay) and taken off by a raid's
// kill (creditKillLocked), whose corpse keeps the rules before it: world.corpse_decay (3
// minutes) then a copy lying to world.corpse_decay_after (20 minutes) while the creature
// comes back, anyone's after world.corpse_open_after (6 minutes), an opening holding it 3
// minutes from then with no limit. Both come through a keep restart (keepnpc.go, keepcopy.go).
// A World made by NewWorld has the rule off (as before it, which the older tests measure);
// the server's start turns it on (SetSoloCorpse with the config's figures).

// The rule's figures when the config leaves them unset.
const (
	DefaultSoloCorpseDecay     = 10 * time.Minute
	DefaultSoloCorpseOpenAfter = 7 * time.Minute
	DefaultCorpseChecksMax     = 10
)

// SetSoloCorpse sets the corpse rule outside a raid: its decay (zero the default, below zero
// the rule off: every corpse under the raid's rules, as before 2026-11-23), when it is anyone's
// (zero the default, below zero as a raid's: world.corpse_open_after) and how many openings
// put its decay back (zero the default, below zero none).
func (w *World) SetSoloCorpse(decay, openAfter time.Duration, checks int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case decay == 0:
		decay = DefaultSoloCorpseDecay
	case decay < 0:
		decay = 0
	}
	switch {
	case openAfter == 0:
		openAfter = DefaultSoloCorpseOpenAfter
	case openAfter < 0:
		openAfter = 0
	}
	switch {
	case checks == 0:
		checks = DefaultCorpseChecksMax
	case checks < 0:
		checks = 0
	}
	w.corpseDecaySolo, w.corpseOpenSolo, w.corpseChecksMax = decay, openAfter, checks
}

// SoloCorpse is the corpse rule outside a raid: its decay (0: off), when it is anyone's and
// how many openings put its decay back.
func (w *World) SoloCorpse() (decay, openAfter time.Duration, checks int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.corpseDecaySolo, w.corpseOpenSolo, w.corpseChecksMax
}

// soloDecayLocked is the corpse time npc's corpse takes at its death under the rule outside
// a raid, zero for none (the rule off, or a corpse no one loots). Must be called with the
// lock held.
func (w *World) soloDecayLocked(npc *NPC) time.Duration {
	if w.corpseDecaySolo <= 0 || !lootableCorpse(npc) {
		return 0
	}
	return w.corpseDecaySolo
}

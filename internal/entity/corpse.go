package entity

import (
	"log"
	"time"
)

// A creature's corpse and its return (F3 20261025-044229, the user's
// times):
//
//   - it lies where the creature fell for the corpse decay (world.corpse_decay,
//     3 minutes), unless it is emptied and closed first: a corpse whose loot
//     window is closed with nothing left on it, an empty one included, goes
//     at once (ClearCorpse, from handlers/loot.go);
//   - once it has gone it is in no one's view, and the creature comes back
//     on its spawn point its respawn time later (world.respawn_after, 60 s);
//   - a GM's /spawn leaves the world when its corpse goes.
//
// Until 2026-10-25 a corpse stayed in view 30 s and every creature came back
// 60 s after its death, looted or not.

// DefaultCorpseDecay is how long a corpse lies when nothing empties it, and
// DefaultRespawnAfter how long after it goes its creature comes back.
const (
	DefaultCorpseDecay  = 3 * time.Minute
	DefaultRespawnAfter = 60 * time.Second
)

// CorpseGoneFunc is told the creatures whose corpses decayed in a tick, with
// the world's lock let go (the loot windows open on them are closed).
type CorpseGoneFunc func(ids []EntityID)

// SetCorpseTimes sets the corpse decay and the respawn after it (a zero
// keeps the default), the respawn on every NPC already in the world too.
func (w *World) SetCorpseTimes(decay, respawnAfter time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if decay > 0 {
		w.corpseDecay = decay
	}
	if respawnAfter > 0 {
		w.respawnAfter = respawnAfter
	}
	for _, npc := range w.NPCs {
		npc.RespawnTime = w.respawnAfter
	}
}

// CorpseTimes is the corpse decay and the respawn after it.
func (w *World) CorpseTimes() (decay, respawnAfter time.Duration) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.corpseDecay, w.respawnAfter
}

// OnCorpseGone sets the hook told which corpses decayed.
func (w *World) OnCorpseGone(fn CorpseGoneFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onCorpseGone = fn
}

// applyNPCSettingsLocked gives an NPC just made from its row the world's
// respawn time and wander radius. Must be called with the lock held.
func (w *World) applyNPCSettingsLocked(npc *NPC) {
	npc.RespawnTime = w.respawnAfter
	npc.RoamRadius = w.roamRadius
}

// corpseLies reports a dead NPC whose corpse is still where it fell at now:
// not emptied and closed, not decayed (its decay, the world's `decay` or its
// own, after its death).
func (n *NPC) corpseLies(now time.Time, decay time.Duration) bool {
	return n.Dead && !n.DeathTime.IsZero() && n.CorpseGone.IsZero() && now.Before(n.DeathTime.Add(n.decayOf(decay)))
}

// decayOf is how long the NPC's corpse lies: its own CorpseDecay (a pet's,
// pet.go), else the world's.
func (n *NPC) decayOf(world time.Duration) time.Duration {
	if n.CorpseDecay > 0 {
		return n.CorpseDecay
	}
	return world
}

// CorpseLies reports whether the NPC's corpse is there to loot now, and the
// death it is the corpse of.
func (w *World) CorpseLies(id EntityID) (died time.Time, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc, found := w.NPCs[id]
	if !found || !npc.corpseLies(time.Now(), w.corpseDecay) {
		return time.Time{}, false
	}
	return npc.DeathTime, true
}

// ClearCorpse takes the corpse of the death died away now (its loot window
// closed with nothing left on it): it leaves every view at the next
// visibility round, and the creature's respawn time runs from now. A corpse
// of another death, or one gone already, is left alone. It reports whether
// the corpse went.
func (w *World) ClearCorpse(id EntityID, died time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, ok := w.NPCs[id]
	now := time.Now()
	if !ok || !npc.DeathTime.Equal(died) || !npc.corpseLies(now, w.corpseDecay) {
		return false
	}
	npc.CorpseGone = now
	log.Printf("Corpse: '%s' (entity ID=%d) emptied and closed; back in %v", npc.Name, npc.ServerID, npc.RespawnTime)
	return true
}

// corpseSystem is the corpses' decay and the creatures' return: a corpse
// whose decay is up goes (its time noted, the hook told after the tick), a
// GM's spawn with it, and a creature whose corpse went its respawn time ago
// comes back on its spawn point. Every tick; must be called with the lock
// held.
func (w *World) corpseSystem(now time.Time) []EntityID {
	var gone []EntityID
	for _, npc := range w.NPCs {
		if !npc.Dead {
			continue
		}
		if npc.DeathTime.IsZero() {
			continue
		}
		if npc.CorpseGone.IsZero() {
			if npc.corpseLies(now, w.corpseDecay) {
				continue
			}
			npc.CorpseGone = npc.DeathTime.Add(npc.decayOf(w.corpseDecay))
			gone = append(gone, npc.ServerID)
		}
		// A GM's /spawn leaves once its corpse is gone (devspawn.go).
		if npc.Temporary {
			w.removeNPCLocked(npc)
			log.Printf("CorpseSystem: removed spawned NPC '%s' (entity ID=%d)", npc.Name, npc.ServerID)
			continue
		}
		if now.Sub(npc.CorpseGone) < npc.RespawnTime || npc.GMHeld {
			continue // a GM's Kill Spawner holds it (gmchannel.go)
		}
		w.respawnNPCLocked(npc)
		log.Printf("CorpseSystem: respawned NPC '%s' (entity ID=%d)", npc.Name, npc.ServerID)
	}
	return gone
}

// respawnNPCLocked brings an NPC back alive on its spawn point, at full
// health, out of any fight, standing. Must be called with the lock held.
func (w *World) respawnNPCLocked(npc *NPC) {
	oldX, oldZ := npc.X, npc.Z
	// one that died as a charmed pet comes back itself, in its own world
	// (charm.go)
	npc.World = w.unCharmLocked(npc)
	npc.X = npc.SpawnX
	npc.Y = npc.SpawnY
	npc.Z = npc.SpawnZ
	npc.Facing = npc.SpawnFacing
	npc.HP = npc.MaxHP
	npc.Dead = false
	npc.DeathTime = time.Time{}
	npc.CorpseGone = time.Time{}
	npc.Threats = nil
	npc.CurrentEnemy = 0
	npc.stopMoving()
	npc.roam = roamState{}
	npc.path = pathState{}
	w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.SpawnX, npc.SpawnZ)
}

// runCorpsesGone tells the hook which corpses decayed, the lock let go.
func (w *World) runCorpsesGone(gone []EntityID) {
	if len(gone) == 0 {
		return
	}
	w.mu.RLock()
	fn := w.onCorpseGone
	w.mu.RUnlock()
	if fn != nil {
		fn(gone)
	}
}

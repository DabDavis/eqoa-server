package entity

import (
	"log"
	"time"
)

// corpselinger.go — a corpse lies 20 minutes from the death, the creature coming back as
// before (the GM Jibrish's Live petition 5035, the owner's "use jibrish rule", 2026-11-14:
// "All NPC's should lock non members of the kill (solo), group or raid from looting a corpse
// for 6 minutes ... the corpse can be looted by anyone until minute 20, where it decays").
//
// A creature's corpse and the creature are one NPC, and the creature comes back only when its
// corpse has gone (corpse.go). So that a corpse may lie 20 minutes without its spawn waiting
// 20 minutes for it, a lootable corpse of a row's creature that is still there at the creature's
// own corpse time (world.corpse_decay, 3 minutes, or later while a raid's rolls hold it) is LEFT:
// a copy of the creature (spawnCopyAsLocked: Temporary, CopyOf its row), dead with the same
// death, takes its place where it fell and lies until corpseLinger after the death
// (world.corpse_decay_after, 20 minutes: its own CorpseDecay), and the creature goes as its
// corpse went before and comes back its respawn time later, as it did before corpses lingered.
// What the corpse holds (rolled or not, its coins, its kill group and its open time, a loot
// step's drops, a raid's rolls, winners and master looter) is moved to the copy's id by the
// handlers (CorpseLeftFunc: handlers/loot.go's CorpsesLeft), the windows open on it closed;
// until they have it the copy is no one's to open (corpseLeft). A copy's own corpse (a GM's
// /spawn, a quest's or a trigger's spawn) lies on in place to the same time. An emptied
// corpse still goes at once (ClearCorpse), and one never lootable (a pet's, an add's, one the
// guards hit, a dummy's, a duel's, an instance's) goes at its corpse time as before.

// DefaultCorpseLinger is how long a lootable corpse lies from its death.
const DefaultCorpseLinger = 20 * time.Minute

// CorpseLeft is a corpse a creature left as a copy: the creature's id, the copy's, and the
// death.
type CorpseLeft struct {
	From, To EntityID
	Died     time.Time
}

// CorpseLeftFunc hears the corpses left in a tick, with the World's lock let go; it says
// each has moved (CorpseMoved) once its loot is the copy's.
type CorpseLeftFunc func([]CorpseLeft)

// OnCorpseLeft sets the hook told the corpses left as copies.
func (w *World) OnCorpseLeft(fn CorpseLeftFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onCorpseLeft = fn
}

// SetCorpseLinger sets how long a lootable corpse lies from its death: zero keeps the
// default; at or below the corpse time (or below zero) none lingers (each goes at its
// creature's corpse time, as before 2026-11-14).
func (w *World) SetCorpseLinger(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if d != 0 {
		w.corpseLinger = d
	}
}

// CorpseLinger is how long a lootable corpse lies from its death (0 or below: none lingers).
func (w *World) CorpseLinger() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.corpseLinger
}

// lingersLocked is whether npc's corpse, at its creature's corpse time, lies on to the linger
// (the file's doc). Must be called with the lock held.
func (w *World) lingersLocked(npc *NPC) bool {
	if w.corpseLinger <= w.corpseDecay || npc.CorpseDecay != 0 || npc.withMaster || npc.Owner != 0 || npc.Charm != nil ||
		npc.CorpseOfNothing() || npc.Dummy != NotDummy || npc.DuelSpawn() || npc.Instance != 0 || npc.challenge != nil ||
		npc.siege != nil || npc.guardHit || npc.GenieOf != 0 || npc.PortalOf != 0 || npc.isMerc() || npc.isFamiliar() ||
		npc.Name == MetalMoogieName || npc.Name == MetalMoogieV2Name {
		return false
	}
	return npc.DeathTime.Add(w.corpseLinger).After(npc.corpseEnds(w.corpseDecay))
}

// leaveCorpseLocked leaves npc's corpse where it fell as a copy (the file's doc), the hook
// told after the tick. Must be called with the lock held.
func (w *World) leaveCorpseLocked(npc *NPC) *NPC {
	c := w.spawnCopyAsLocked(npc, npc.X, npc.Y, npc.Z, npc.Facing, npc.World, 0)
	c.HP, c.Dead = 0, true
	c.DeathTime, c.corpseHeld = npc.DeathTime, npc.corpseHeld
	c.CorpseDecay = w.corpseLinger
	c.corpseLeft = true
	w.wakeNPCLocked(c) // the corpses' set, for its decay
	w.corpsesLeft = append(w.corpsesLeft, CorpseLeft{From: npc.ServerID, To: c.ServerID, Died: npc.DeathTime})
	log.Printf("Corpse: '%s' (entity ID=%d) left its corpse as %d, lying to %v after its death; back in %v",
		npc.Name, npc.ServerID, c.ServerID, w.corpseLinger, npc.RespawnTime)
	return c
}

// runCorpsesLeft tells the hook the corpses left in the tick, the lock let go; with no hook
// each is the copy's at once.
func (w *World) runCorpsesLeft() {
	w.mu.Lock()
	left := w.corpsesLeft
	w.corpsesLeft = nil
	fn := w.onCorpseLeft
	w.mu.Unlock()
	if len(left) == 0 {
		return
	}
	if fn != nil {
		fn(left)
		return
	}
	for _, l := range left {
		w.CorpseMoved(l.To)
	}
}

// CorpseMoved says the left corpse id's loot is its own now: it opens.
func (w *World) CorpseMoved(id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if npc := w.NPCs[id]; npc != nil {
		npc.corpseLeft = false
	}
}

// CorpseDecaysAt is when the corpse of that id goes, a lingering one's linger counted (the
// file's doc), and whether it lies.
func (w *World) CorpseDecaysAt(id EntityID) (time.Time, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc, ok := w.NPCs[id]
	if !ok || !npc.corpseLies(time.Now(), w.corpseDecay) {
		return time.Time{}, false
	}
	end := npc.corpseEnds(w.corpseDecay)
	if w.lingersLocked(npc) {
		end = npc.DeathTime.Add(w.corpseLinger)
	}
	return end, true
}

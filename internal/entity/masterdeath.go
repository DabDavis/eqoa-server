package entity

import (
	"log"
	"time"
)

// masterdeath.go — a pet dies with its master (the owner, 2026-11-10: "when the summoned pet's master dies,
// so does the pet, goes for NPCs and players").
//
//   - A player's summoned pet already did, since petition 1398: its owner dead (or gone), petSystem lets
//     it go (dismissPetLocked), a living one laid down dead where it stands (petDiesLocked: its death
//     played on every client, its body gone petDismissedLies later, no one's kill, nothing on it). A
//     charmed creature is no summoned pet: its charm breaks (releaseCharmLocked) and it lives on, as
//     ever.
//   - A creature's: the adds its summon called (summon.go) die with it, and a creature's own pets paired
//     to it off the rows (companion.go: a Companion Spirit by its froglok shaman, a Servant of Vorin by
//     Vorin), and a GM trigger's resummoned spawn whose caller dies (handlers' gmtriggers.go: Tunare's
//     Grace and Wrath of the Mother, trigger 141), each laid down dead as a player's pet is
//     (killWithMasterLocked): its death played on every client, its body lying petDismissedLies, a
//     corpse of nothing no one loots (handlers' loot: CorpseOfNothing), no one's kill. A companion comes
//     back with its owner, as before (companionsBackLocked); until 2026-11-10 it was out of the world at
//     its owner's death with no death seen.
//
// No NPC's Owner is an NPC on this server (Owner is a player's alone): those three are the masters an NPC
// has.

// killWithMasterLocked lays npc down dead where it stands, its master dead (the file's doc): off every
// fight, its death played, its body for petDismissedLies, a corpse of nothing. One dead already is left
// as it lies. Must be called with the lock held.
func (w *World) killWithMasterLocked(npc *NPC, now time.Time) {
	if npc == nil || npc.Dead {
		return
	}
	w.dropAggroLocked(npc.ServerID)
	npc.HP = 0
	npc.withMaster = true
	w.killNPC(npc)
	npc.DeathTime = now
}

// KillWithMaster lays the NPC id down dead with its master (killWithMasterLocked): a GM trigger's spawn
// whose caller died. It reports the NPC's name, false for one gone or dead already.
func (w *World) KillWithMaster(id EntityID) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil || npc.Dead {
		return "", false
	}
	w.killWithMasterLocked(npc, time.Now())
	log.Printf("Pets: %q (%d) dies with its master", npc.Name, id)
	return npc.Name, true
}

// addsDieLocked lays down dead every living add a summon of npc's called (summon.go), npc dead. Must be
// called with the lock held.
func (w *World) addsDieLocked(npc *NPC, now time.Time) {
	for _, a := range w.adds {
		if a.add != nil && a.add.by == npc && !a.Dead && w.NPCs[a.ServerID] == a {
			w.killWithMasterLocked(a, now)
			log.Printf("Template: %s (%d) dies with %s", a.Name, a.ServerID, npc.Name)
		}
	}
}

// CorpseOfNothing reports a corpse no one loots and nothing is on (handlers' loot): an add a summon called
// (summon.go: "a dead servant gives no loot"), or one that died with its master (the file's doc).
func (n *NPC) CorpseOfNothing() bool {
	return n != nil && (n.add != nil || n.withMaster)
}

package entity

import "log"

// pethate.go — a pet's hate falls to its master (petition 1656, Waldorf, a magician 60 who multiboxes
// petters: "the hate doesn't transfer from the pet when it dies to the caster. my other boxes don't cast
// now ... so when main box dies, and their pets die, the mob just runs away").
//
// A creature's hate list (its Threats) held only who hurt or angered it. A pet sent in by an owner who
// never cast or swung put the pet alone on the list; when the pet went (killed, dismissed, let go with
// its master's death) dropAggroLocked took it off, the list was empty and the creature gave up and went
// home (aiSystem, chase.go). As EverQuest's (and EQOA's) pets, the master is the one the creature turns
// on: whenever a pet leaves the hate lists, each living creature that hated it adds the pet's hate to its
// master's entry (at least 1, so a master who never touched it is on the list) before the pet is taken
// off, and a creature whose enemy the pet was chooses again from its list as ever (pickEnemyLocked): the
// master, unless another's hate is higher. Each pet of each owner passes to its own owner, so one
// creature fought by several boxes' pets turns on each box in turn as its pet falls.
//
// The master is:
//   - a player's pet's (summoned or charmed: Owner), its owner;
//   - a merc's pet's (mercpet.go), its merc;
//   - a merc itself has none: it is the group's, hired to fight on its own, and its hirer did not send it
//     at that creature; its hate goes with it as before.
//
// No hate passes to a master who is dead or gone, in another world, more than npcGiveUp from the
// creature (it would give up its whole list chasing them: chase.go), or barred from it (a Metal Moogie's
// duel); a dummy keeps its own list (dummy.go), a familiar is never on one. Those creatures go on as ever:
// the next on their list, or home.

// hateMasterLocked is who a pet's hate falls to (the file's doc): its living master's id, world and place;
// id 0 for an NPC that is no pet, a merc, or a pet with no living master. Must be called with the lock
// held.
func (w *World) hateMasterLocked(pet *NPC) (EntityID, uint8, float32, float32) {
	if pet == nil || pet.isMerc() {
		return 0, 0, 0, 0
	}
	if ms := pet.mercPet; ms != nil {
		m := ms.npc
		if m == nil || m.Dead || w.NPCs[m.ServerID] != m {
			return 0, 0, 0, 0
		}
		return m.ServerID, m.World, m.X, m.Z
	}
	if pet.Owner == 0 {
		return 0, 0, 0, 0
	}
	o := w.Players[pet.Owner]
	if o == nil || o.IsDead() {
		return 0, 0, 0, 0
	}
	return o.ServerID, o.World, o.X, o.Z
}

// petHateToMasterLocked passes the hate on every creature's list for the NPC id to its master (the file's
// doc), before dropAggroLocked takes it off. Nothing for an id that is no pet. Must be called with the
// lock held.
func (w *World) petHateToMasterLocked(id EntityID) {
	pet := w.NPCs[id]
	if pet == nil {
		return
	}
	master, world, mx, mz := w.hateMasterLocked(pet)
	if master == 0 {
		return
	}
	for _, npc := range w.NPCs {
		th, ok := npc.Threats[id]
		if !ok || npc == pet || npc.ServerID == master || npc.Dead || npc.Dummy != NotDummy {
			continue
		}
		if npc.World != world || distXZ(npc.X, npc.Z, mx, mz) > npcGiveUp || challengeBars(npc, master) {
			continue
		}
		npc.Threats[master] += max(th, 1)
		log.Printf("Pet: %q's hate on %s (%d) falls to its master %d (now %d)", pet.Name, npc.Name, th, master, npc.Threats[master])
	}
}

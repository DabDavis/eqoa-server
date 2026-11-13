package entity

import "testing"

// Ticket 2008 (Waldorf, 2026-11-13, the world-14 dungeons): a creature's pet killed before its caster could
// be looted for tunar and items, killed after it not ("you can't loot that entity"). The operator's ruling:
// a creature's pet carries nothing, whichever dies first. Both orders, as companion_test.go's dshinn
// necromancer and its spectral minion; the controls: the griffawn beside them (no one's pet), a player's pet
// and a charmed creature named as pets (their own rules, never paired), and the minion's next life.
func TestACreaturesPetCarriesNothingWhicheverDiesFirst(t *testing.T) {
	// the pet first
	w, owner, pet, other, _ := companionWorld(t)
	w.KillNPCNow(pet.ServerID)
	if !pet.Dead || !pet.CorpseOfNothing() {
		t.Errorf("the minion killed first: dead %v, a corpse of nothing %v; want nothing on it", pet.Dead, pet.CorpseOfNothing())
	}
	w.KillNPCNow(owner.ServerID)
	w.KillNPCNow(other.ServerID)
	if owner.CorpseOfNothing() || other.CorpseOfNothing() {
		t.Errorf("the controls: the necromancer's corpse of nothing %v, the griffawn's %v; want their loot", owner.CorpseOfNothing(), other.CorpseOfNothing())
	}
	w.mu.Lock()
	w.respawnNPCLocked(pet)
	w.mu.Unlock()
	if pet.CorpseOfNothing() {
		t.Error("the minion's next life is marked before its death")
	}

	// the master first: its pet dies with it, a corpse of nothing as before
	w, owner, pet, _, _ = companionWorld(t)
	w.KillNPCNow(owner.ServerID)
	if !pet.Dead || !pet.CorpseOfNothing() || owner.CorpseOfNothing() {
		t.Errorf("the necromancer killed first: minion dead %v, of nothing %v; the necromancer's of nothing %v",
			pet.Dead, pet.CorpseOfNothing(), owner.CorpseOfNothing())
	}

	// a player's pet and a charmed creature named as pets keep their own rules (handlers' loot: Owner, Charm)
	w, _, _, _, p := companionWorld(t)
	add := func(n *NPC) *NPC {
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		n.HP, n.MaxHP = 100, 100
		w.NPCs[n.ServerID] = n
		return n
	}
	mine := add(&NPC{ServerID: 4101, Name: "decaying minion", Owner: p.ServerID, X: 100.5, Z: 100.5})
	charmed := add(&NPC{ServerID: 4102, Name: "undead minion", Charm: &charmHold{}, X: 100.2, Z: 99.5})
	w.KillNPCNow(mine.ServerID)
	w.KillNPCNow(charmed.ServerID)
	if mine.CorpseOfNothing() || charmed.CorpseOfNothing() {
		t.Errorf("marked: the player's pet %v, the charmed creature %v; want neither (their own rules)", mine.CorpseOfNothing(), charmed.CorpseOfNothing())
	}
}

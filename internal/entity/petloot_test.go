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

// Ticket 2009 (Waldorf, 2026-11-13): "Guk instance - Howling Death isn't tethered to Rubik and is neutral faced".
// Rubbip and his Howling Death 2.05 off him, as the rows stand, with the seed's npcaggro row for the pet's name
// (aggressive 0, "a player's pet recorded beside its owner"): in the instanced world 14 the Howling Death is
// Rubbip's pet, hostile as he is, joins his fight, and carries nothing killed first; the control, the same pair in
// the open world, is paired but keeps its row's nature (the open world's rules unchanged).
func TestRubbipsHowlingDeathIsHisPet(t *testing.T) {
	dungeonWorlds[14].Store(true)
	t.Cleanup(func() { dungeonWorlds[14].Store(false) })
	pair := func(world uint8) (*World, *NPC, *NPC, *Player) {
		w := NewWorld()
		w.SetAggroRules(map[string]AggroRule{"howling death": {Aggressive: false}})
		add := func(n *NPC) *NPC {
			n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
			n.HP, n.MaxHP, n.World, n.ModelID = 40000, 40000, world, 3637112914
			w.NPCs[n.ServerID] = n
			w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
			return n
		}
		rub := add(&NPC{ServerID: 101678, Name: "Rubbip", Level: 64, X: 24876.1, Y: -11, Z: 28849.1})
		rub.ModelID = 962587005
		hd := add(&NPC{ServerID: 101679, Name: "Howling Death", Level: 64, X: 24877.5, Y: -11, Z: 28847.6})
		add(&NPC{ServerID: 101680, Name: "ghoul urd wizard", Level: 62, X: 24873.3, Y: -11, Z: 28845.2})
		p := &Player{ServerID: 1001, Name: "Waldorf", Level: 60, HP: 7611, MaxHP: 7611, World: world, X: 24890, Z: 28849}
		w.Players[p.ServerID] = p
		return w, rub, hd, p
	}

	w, rub, hd, p := pair(14)
	w.mu.Lock()
	master := w.companionMasterLocked(hd)
	hostile := w.hostileNatureLocked(hd)
	w.updateAggro(rub, p.ServerID, 50)
	w.mu.Unlock()
	if master != rub || !hostile {
		t.Errorf("the instance: Howling Death's master %v, hostile %v; want Rubbip's, hostile", master, hostile)
	}
	if hd.Threats[p.ServerID] == 0 {
		t.Errorf("Rubbip hit: Howling Death's threats %v, want it at his side", hd.Threats)
	}
	w.KillNPCNow(hd.ServerID)
	if !hd.CorpseOfNothing() {
		t.Error("Howling Death killed before Rubbip carries loot")
	}

	w, rub, hd, _ = pair(0)
	w.mu.Lock()
	master = w.companionMasterLocked(hd)
	hostile = w.hostileNatureLocked(hd)
	w.mu.Unlock()
	if master != rub || hostile {
		t.Errorf("the open world: master %v, hostile %v; want Rubbip's, by its row (not hostile)", master, hostile)
	}
}

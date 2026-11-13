package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Ticket 2008 (Waldorf, 2026-11-13, the world-14 dungeons): "If you kill the pet first you can loot it. If you
// kill the caster first, you get a message stating 'you can't loot that entity'. Should be consistent
// regardless of order." The ruling: a creature's pet carries nothing either way. A ghoul urd necromancer and
// its Mummified Guardian 1.9 off it (as Waldorf's view had them), the guardian's NPC row holding an item:
// killed first or after its necromancer, its corpse answers "not lootable"; the control, the necromancer
// itself on the same row, opens with the item.
func TestACreaturesPetIsNotLootableInEitherOrder(t *testing.T) {
	setup := func(t *testing.T) (*corpseFixture, *entity.World, *entity.NPC, *entity.NPC) {
		f := newCorpseFixture(t, 1)
		w := entity.NewWorld()
		add := func(n *entity.NPC) *entity.NPC {
			n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
			n.HP, n.MaxHP, n.NPCID, n.World = 15000, 15000, corpseNPCID, 14
			w.NPCs[n.ServerID] = n
			w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
			return n
		}
		nec := add(&entity.NPC{ServerID: 101685, Name: "ghoul urd necromancer", Level: 62, X: 24854.4, Z: 29046})
		pet := add(&entity.NPC{ServerID: 101686, Name: "Mummified Guardian", Level: 61, X: 24854.3, Z: 29047.9})
		return f, w, nec, pet
	}
	open := func(t *testing.T, f *corpseFixture, npc *entity.NPC) [][]byte {
		t.Helper()
		f.npc = npc
		f.lies = true
		if !f.h.OpenCorpse(f.sess, npc) {
			t.Fatalf("%s's corpse not taken as one", npc.Name)
		}
		return f.sent()
	}
	notLootable := func(t *testing.T, got [][]byte, what string) {
		t.Helper()
		mustOps(t, got, protocol.OpcodeLootOptions)
		if got[0][2] != lootNotLootable {
			t.Errorf("%s: 0x0085 %d, want not lootable", what, got[0][2])
		}
	}

	// the pet first
	f, w, nec, pet := setup(t)
	w.KillNPCNow(pet.ServerID)
	notLootable(t, open(t, f, pet), "the guardian killed first")
	w.KillNPCNow(nec.ServerID)
	got := open(t, f, nec)
	if last := got[len(got)-1]; opOf(last) != protocol.OpcodeLoot || lootCount(t, last) != 1 {
		t.Errorf("the control: the necromancer's corpse sent %x, want its coins and its 1 record", got)
	}

	// the master first
	f, w, nec, pet = setup(t)
	w.KillNPCNow(nec.ServerID)
	if !pet.Dead {
		t.Fatal("the guardian lives on after its necromancer")
	}
	notLootable(t, open(t, f, pet), "the guardian dead with its necromancer")
}

// A GM trigger's resummoned spawn is its caller's pet (gmtriggers.go's resummonLocked): killed before its
// caller, a corpse of nothing too; the control, the caller, is not.
func TestATriggersResummonedPetKilledFirstCarriesNothing(t *testing.T) {
	w := newTrigWorld(t)
	gp := w.gp
	for _, n := range []*entity.NPC{
		{ServerID: 701, Name: "Orc Chief", HP: 500, MaxHP: 500, Level: 20, NPCType: 0x80, X: 100, Y: 50, Z: 120},
	} {
		w.gw.NPCs[n.ServerID] = n
	}
	target := &entity.NPC{ServerID: 702, Name: "Mr. Moogie", HP: 100000, MaxHP: 100000, Level: 50, X: gp.X + 10, Y: gp.Y, Z: gp.Z, World: gp.World}
	w.gw.NPCs[target.ServerID] = target
	w.set(31, "Chief and pawn", "near", 60, `[{"do":"spawn","npc":"Orc Chief","attackable":true,"minutes":60,"spread":1},
		{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":60,"spread":1},
		{"do":"resummon","npc":"Orc Pawn","by":"Orc Chief","seconds":10,"minutes":60},
		{"do":"awaitdead","npc":"Orc Chief","seconds":3600}]`)
	if _, err := w.fire(31); err != nil {
		t.Fatal(err)
	}
	of := func(name string) *entity.NPC {
		for _, n := range w.gw.NPCs {
			if n.Temporary && n.Name == name {
				return n
			}
		}
		return nil
	}
	chief, pawn := of("Orc Chief"), of("Orc Pawn")
	if chief == nil || pawn == nil {
		t.Fatalf("the spawns: chief %v, pawn %v", chief != nil, pawn != nil)
	}
	w.at(time.Second)
	w.gw.KillNPCNow(pawn.ServerID)
	if !pawn.Dead || !pawn.CorpseOfNothing() {
		t.Errorf("the pawn killed first: dead %v, a corpse of nothing %v", pawn.Dead, pawn.CorpseOfNothing())
	}
	w.gw.KillNPCNow(chief.ServerID)
	if chief.CorpseOfNothing() {
		t.Error("the control: the chief's corpse is one of nothing")
	}
}

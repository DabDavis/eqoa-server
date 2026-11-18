package entity

import "testing"

// A row's loot seed is its spawn's: the same until it respawns, another after; a
// keep restart's salt and generations give a new world the same seeds; a temporary
// has its own, drawn once a spawn.
func TestALootSeedIsItsSpawns(t *testing.T) {
	w := NewWorld()
	row := &NPC{ServerID: 4001, Name: "orc pawn", HP: 10, MaxHP: 10}
	tmp := &NPC{ServerID: 4002, Name: "orc pawn", HP: 10, MaxHP: 10, Temporary: true}
	w.NPCs[row.ServerID], w.NPCs[tmp.ServerID] = row, tmp
	a := w.NPCLootSeed(row)
	if a == 0 || w.NPCLootSeed(row) != a {
		t.Fatalf("a row's seed %x, then %x", a, w.NPCLootSeed(row))
	}
	ta := w.NPCLootSeed(tmp)
	if ta == 0 || w.NPCLootSeed(tmp) != ta || ta == a {
		t.Fatalf("a temporary's seed %x then %x (the row's %x)", ta, w.NPCLootSeed(tmp), a)
	}
	for _, n := range []*NPC{row, tmp} {
		n.Dead = true
		w.mu.Lock()
		w.respawnNPCLocked(n)
		w.mu.Unlock()
	}
	b := w.NPCLootSeed(row)
	if b == a {
		t.Errorf("a respawn kept the seed %x", a)
	}
	if tb := w.NPCLootSeed(tmp); tb == ta || tb == 0 {
		t.Errorf("a temporary's respawn kept its seed %x (now %x)", ta, tb)
	}
	// a keep restart: a new world of the same rows, given the salt and generations
	salt, gens := w.LootSeeds()
	if gens[uint32(row.ServerID)] != 1 || len(gens) != 1 {
		t.Fatalf("generations kept %v, want the row's 1 alone", gens)
	}
	w2 := NewWorld()
	row2 := &NPC{ServerID: 4001, Name: "orc pawn", HP: 10, MaxHP: 10}
	w2.NPCs[row2.ServerID] = row2
	if n := w2.RestoreLootSeeds(salt, gens); n != 1 {
		t.Fatalf("restored %d generations", n)
	}
	if got := w2.NPCLootSeed(row2); got != b {
		t.Errorf("after the restart %x, want %x", got, b)
	}
	// a file without them changes nothing
	w3 := NewWorld()
	if n := w3.RestoreLootSeeds(0, gens); n != 0 || w3.lootSalt != 0 {
		t.Errorf("an old file restored %d (salt %x)", n, w3.lootSalt)
	}
}

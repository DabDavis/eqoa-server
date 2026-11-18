package entity

import (
	"math/rand"
	"time"
)

// spawnloot.go — a creature's loot is determined at its spawn, not at its death
// (the owner, 2026-11-18, ticket 2366: "loot drops should be determined at spawn
// not death"). What its corpse holds is rolled (handlers/loot.go's rollCorpse) by
// dice seeded by its spawn alone: the world's salt, its id and its spawn's
// generation (spawnGen, one more at each respawn: respawnNPCLocked). So whoever
// kills it, whenever, and whoever opens the corpse first, it holds the same; a new
// spawn rolls anew. A keep restart carries the salt and every spawnGen past 0
// (LootSeeds, RestoreLootSeeds), so a spawn's loot is the same after one. A copy or
// a temporary (a trigger's, a GM's, a dungeon's, a quest's) has a seed of its own,
// drawn the first time it is asked (its id may have been another's): a keep
// restart gives such a creature a new id, and an unopened corpse of one rolls anew.

// lootSaltLocked is the world's salt, drawn at its first use. Must be called with
// the lock held (write).
func (w *World) lootSaltLocked() uint64 {
	if w.lootSalt == 0 {
		w.lootSalt = rand.New(rand.NewSource(time.Now().UnixNano())).Uint64() | 1
	}
	return w.lootSalt
}

// NPCLootSeed is npc's loot seed for this spawn (the file's doc).
func (w *World) NPCLootSeed(npc *NPC) uint64 {
	if npc == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	salt := w.lootSaltLocked()
	if npc.Temporary || npc.SpawnOnly || npc.lootSeed != 0 {
		if npc.lootSeed == 0 {
			npc.lootSeed = mixSeed(salt^uint64(time.Now().UnixNano()), uint64(npc.ServerID), uint64(npc.spawnGen))
		}
		return npc.lootSeed
	}
	return mixSeed(salt, uint64(npc.ServerID), uint64(npc.spawnGen))
}

// mixSeed is splitmix64 over the three: a well-spread seed of them.
func mixSeed(salt, id, gen uint64) uint64 {
	x := salt ^ (id * 0x9E3779B97F4A7C15) ^ (gen << 40) ^ gen
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// LootSeeds is what a keep restart carries of the spawns' loot: the salt and every
// row's spawnGen past 0, by id.
func (w *World) LootSeeds() (uint64, map[uint32]uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	gens := map[uint32]uint32{}
	for id, npc := range w.NPCs {
		if npc.spawnGen != 0 && !npc.Temporary && !npc.SpawnOnly {
			gens[uint32(id)] = npc.spawnGen
		}
	}
	return w.lootSaltLocked(), gens
}

// RestoreLootSeeds gives the world the salt and the spawns' generations a keep
// restart carried (salt 0: a file from before them, nothing changed).
func (w *World) RestoreLootSeeds(salt uint64, gens map[uint32]uint32) int {
	if salt == 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lootSalt = salt
	n := 0
	for id, g := range gens {
		if npc := w.NPCs[EntityID(id)]; npc != nil && !npc.Temporary && !npc.SpawnOnly {
			npc.spawnGen = g
			n++
		}
	}
	return n
}

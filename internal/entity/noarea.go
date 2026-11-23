package entity

import (
	"fmt"

	"github.com/eqoa/server/internal/database"
)

// noarea.go — the creatures no area spell touches (Heirloom petition 2740,
// 2026-11-23: "Unmarked Headstone in ZP is affected by AoE": the ten headstones
// by Tallon's way in are clicked or killed on purpose, each death a chance at
// its raid's named, and a raid's area spells broke them by the score). A
// creature whose name table npcnoarea holds is passed by every area: a player's
// area spell's others (areaCreaturesLocked), a thrown or drunk potion's splash
// (potion.go, mercthrow.go), a pet's area cast's others (petAreaFoesLocked) and a
// mercenary's (merccast.go). What is cast at it, its own target, still lands.

// SetNoAreaNames sets table npcnoarea's names, keyed as database.NPCClassKey
// keys a name.
func (w *World) SetNoAreaNames(names map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.noArea = names
}

// LoadNoAreaNames reads table npcnoarea into the world: at the start and after
// a GM's edit. Call it without the database held. It reports how many names it
// read.
func (w *World) LoadNoAreaNames(db *database.SQLiteDB) (int, error) {
	names, err := db.NPCNoAreaNames()
	if err != nil {
		return 0, fmt.Errorf("npcnoarea: %w", err)
	}
	w.SetNoAreaNames(names)
	return len(names), nil
}

// areaSparesLocked is whether no area touches the creature. Must be called
// with the lock held.
func (w *World) areaSparesLocked(npc *NPC) bool {
	return npc != nil && len(w.noArea) > 0 && w.noArea[database.NPCClassKey(npc.Name)]
}

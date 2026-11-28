package handlers

import (
	"errors"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// lootdroptrigger.go — an item a corpse rolls fires a GM trigger where the corpse lies (table
// lootdroptrigger, database/lootdroptrigger.go; the owner, 2026-11-18, Ronson's t222: "As soon as a Zek
// Scout drops a Portal Rune, the portal will spawn by the campfire"). The loot is rolled at the corpse's
// first opening (rollCorpse), so that is when it fires: once a corpse, quietly, at the corpse's place, on
// a goroutine of its own (the trigger engine's lock is never taken under the loot's). A trigger whose
// lock a run holds does not fire again.

// fireDropTriggers fires the triggers the items npc's corpse rolled name.
func (h *NPCWindowHandler) fireDropTriggers(npc *entity.NPC, items []database.CharInventoryEntry) {
	if h.inv == nil || npc == nil || len(items) == 0 {
		return
	}
	ids := make([]int, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.Pattern.PatternID)
	}
	names, err := h.inv.LootDropTriggers(ids)
	if err != nil {
		log.Printf("Loot: %q's drop triggers: %v", npc.Name, err)
		return
	}
	e := Triggers()
	if e == nil || len(names) == 0 {
		return
	}
	at := &triggerPlaceAt{world: npc.World, x: npc.X, y: npc.Y, z: npc.Z, inst: npc.Instance} // a heroic raid's instance too
	who := npc.Name + "'s loot"
	for _, name := range names {
		go func(name string) {
			if _, err := e.FireNamed(name, TriggerFirer{Name: who, Quiet: true, At: at}); err != nil {
				if !errors.Is(err, ErrTriggerBusy) {
					log.Printf("Loot: %s fires %q: %v", who, name, err)
				}
				return
			}
			log.Printf("Loot: %s fires trigger %q", who, name)
		}(name)
	}
}

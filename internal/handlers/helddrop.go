package handlers

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// helddrop.go — a creature killed holding a weapon or a shield drops it (the
// owner, 2026-11-17: "if a mob is holding a weapon/shield they will drop it as
// loot", "always drop what it holds"): beside its loot, every corpse holds, for
// each piece the creature carried at its death (entity/heldgear.go: what its
// spawn's roll gave it), the catalog's record of that look for its level
// (database.HeldDrops: the model's, a hand's slot or a shield's, the nearest
// levelreq at or below its level, the same for every creature of its name).
// A name with a row of table npcheld for a hand drops that very record of
// it, No Trade or Lore as it is (database.HeldDrops.Named; the owner,
// 2026-11-18: Vallon Zek's Sapphyra's Sword, "put sword in hand if he will drop it").
// A look no droppable record has, or none of its level, drops nothing extra,
// said once a look in the log. Off (world.held_gear false): nothing extra.

// heldDrops is the model map, nil while held drops are off.
var heldDrops atomic.Pointer[database.HeldDrops]

// SetHeldDrops turns held drops on with the model map d, or off (nil).
func SetHeldDrops(d *database.HeldDrops) { heldDrops.Store(d) }

// heldMissed is the looks already said to drop nothing, by model and kind.
var heldMissed sync.Map

// heldDropItems is what npc's corpse holds of what it carried at its death.
func heldDropItems(npc *entity.NPC) []database.CharInventoryEntry {
	d := heldDrops.Load()
	if d == nil || !rollsCategories(npc) {
		return nil
	}
	var out []database.CharInventoryEntry
	for hand, p := range []struct {
		model  uint32
		shield bool
	}{{uint32(npc.PrimaryWeapon), false}, {uint32(npc.SecondaryWeapon), false}, {uint32(npc.Shield), true}} {
		if p.model == 0 {
			continue
		}
		// a name's own record of that hand (table npcheld: Vallon Zek's Sapphyra's Sword, No Trade
		// as it is), else the catalog's of its look
		if e, ok := d.Named(npc.Name, hand); ok {
			out = append(out, e)
			continue
		}
		if e, ok := d.Pick(p.model, p.shield, npc.Level, npc.Name); ok {
			out = append(out, e)
			continue
		}
		why := "no droppable record has its look"
		if d.Has(p.model, p.shield) {
			why = "no record of its look is of its level or below"
		}
		if _, said := heldMissed.LoadOrStore([3]any{p.model, p.shield, why}, true); !said {
			log.Printf("Loot: %q (level %d) held %08X (shield %v): nothing extra drops: %s", npc.Name, npc.Level, p.model, p.shield, why)
		}
	}
	return out
}

// HeldDropReport is how the held looks of the NPCs given map to records.
type HeldDropReport struct {
	Models, Mapped   int               // distinct held looks (hand and shield apart), of which some record has
	Rows, RowsMapped int               // pieces held over the rows, of which by a look some record has
	Unmapped         map[[2]uint32]int // a look no record has (model, 1 a shield): the pieces holding it
}

// HeldDropsReport is how the held looks of the NPCs' kits (model, shield) map.
func HeldDropsReport(d *database.HeldDrops, held [][2]uint32) HeldDropReport {
	r := HeldDropReport{Unmapped: map[[2]uint32]int{}}
	seen := map[[2]uint32]bool{}
	for _, h := range held {
		if h[0] == 0 {
			continue
		}
		has := d.Has(h[0], h[1] != 0)
		r.Rows++
		if has {
			r.RowsMapped++
		} else {
			r.Unmapped[h]++
		}
		if !seen[h] {
			seen[h] = true
			r.Models++
			if has {
				r.Mapped++
			}
		}
	}
	return r
}

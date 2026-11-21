package handlers

import (
	"log"
	"math/rand"
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
//
// A rare record only 1 in 500 (the owner, 2026-11-21: "mobs are dropping rare
// items because they are holding those items, the chances of them holding rare
// items should be like 1 in 500", "and let it be around that mobs level"): each
// piece rolls once at the death, by the corpse's own dice (heldDice: seeded by
// its spawn, as its loot's), and draws the look's rare record of its level 1 in
// world.held_rare_one_in, else its plain one (database.HeldDrops.Pick, its doc).
// A name's npcheld record is no roll's: it drops as it did.

// heldDrops is the model map, nil while held drops are off.
var heldDrops atomic.Pointer[database.HeldDrops]

// SetHeldDrops turns held drops on with the model map d, or off (nil), each record of
// it with a proc by its name or description (entity.WeaponProcOf) made a rare one.
func SetHeldDrops(d *database.HeldDrops) {
	d.MarkRare(heldHasProc)
	heldDrops.Store(d)
}

// heldHasProc reports whether a record has a proc by its name or description.
func heldHasProc(p *database.ItemPatternRecord) bool {
	_, ok := entity.WeaponProcOf(p.PatternID, p.ItemName, p.ItemDesc)
	return ok
}

// heldDiceSalt sets a held piece's dice apart from its corpse's loot dice (same seed, another
// stream), so the held roll moves none of the loot's.
const heldDiceSalt = 0x48454C4452415245 // "HELDRARE"

// heldDice is the dice npc's held pieces roll with, in [0, 1): the handler's own where set (a
// test's), else one source seeded by the creature's spawn (lootSeed, as corpseDice's: the
// same corpse whoever kills and opens it), else unseeded.
func (h *NPCWindowHandler) heldDice(npc *entity.NPC) func() float64 {
	if h.heldFloat != nil {
		return h.heldFloat
	}
	if h.lootSeed == nil {
		return rand.Float64
	}
	return rand.New(rand.NewSource(int64(h.lootSeed(npc) ^ heldDiceSalt))).Float64
}

// heldMissed is the looks already said to drop nothing, by model and kind.
var heldMissed sync.Map

// heldDropItems is what npc's corpse holds of what it carried at its death, each
// piece of a look drawing its rare record when roll (in [0, 1); nil: unseeded)
// says (database.HeldDrops.RollsRare).
func heldDropItems(npc *entity.NPC, roll func() float64) []database.CharInventoryEntry {
	d := heldDrops.Load()
	if d == nil || !rollsCategories(npc) {
		return nil
	}
	if roll == nil {
		roll = rand.Float64
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
		rare := d.RollsRare(roll())
		if e, ok := d.Pick(p.model, p.shield, npc.Level, npc.Name, rare); ok {
			if rare && d.Rare(e.Pattern.PatternID) {
				log.Printf("Loot: %q (level %d) held %08X: the 1 in %d rare record, %q", npc.Name, npc.Level, p.model, d.RareOneIn, e.Pattern.ItemName)
			}
			out = append(out, e)
			continue
		}
		why := "no droppable record has its look"
		switch {
		case rare:
			why = "no record of its look is near its level"
		case d.HasPlain(p.model, p.shield):
			why = "no plain record of its look is near its level"
		case d.Has(p.model, p.shield):
			why = "its look has rare records alone (they drop 1 in held_rare_one_in)"
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
	Plain, RareOnly  int               // of the Mapped: a plain record has, rare records alone have
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
				if d.HasPlain(h[0], h[1] != 0) {
					r.Plain++
				} else {
					r.RareOnly++
				}
			}
		}
	}
	return r
}

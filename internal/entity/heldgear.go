package entity

import (
	"log"
	"math/rand"
	"strings"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/database"
)

// heldgear.go — not every creature of a camp carries the weapon and shield its
// row holds (the owner, 2026-11-17: "always drop what it holds, but mobs
// shouldnt always spawn with weapons/shields"; the drop is handlers/helddrop.go).
//
// An NPC's row names what it holds in each hand and on its arm (npcs
// primary_weapon, secondary_weapon, shield: the channel's items 0-2,
// claude-notes/held-items-channel-20261022.md); that is its kit (NPC.kit).
// At each spawn, its first (ArmNPCs, once everything the rows make is loaded)
// and every respawn after a death (respawnNPCLocked, makeKindLocked), a
// monster rolls for each piece of its kit: each hand's weapon at the weapon's
// chance, the shield at the shield's (HeldGearRules; world.held_weapon_chance
// and world.held_shield_chance, 50 and 40 unset; world.held_chances by name).
// What it does not carry is off its channel (both the item and the hand's
// attack type), off its swing (rounds.go reads what it carries), and off its
// corpse; what it carries stays for that life, a keep restart too (KeptNPC's
// Unarmed). A GM's edit or reload of its row makes it with its whole kit
// until its next death.
//
// Always its whole kit: anything not a monster of the world's rows (a pet, a
// charmed creature, a mercenary, a familiar, a dummy, a GM's or a trigger's or
// a quest's copy, a spawn-only original, a duel's or a siege's, a dungeon's
// creature), an NPC (npc_type's 0x80: merchants, quest givers, townsfolk), one
// no one may select (a decoration), a town's guard, a rare standing as its
// rare, a ranger of a template shooting the bow it holds (npcshoot.go), a
// creature of a loot profile of its own (lootcreature: boss, named and the
// rest, but xp and trash), and a NAMED one: a name with a capital first
// letter on at most world.held_named_rows rows (3 unset). A capital alone does
// not make a name a named creature's: "Ice goblin warrior" stands on 96 rows.
// A name in world.held_chances rolls at its own chances whatever its name or
// profile (but the first list).

// HeldChance is the chances (percent, 0-100) a monster spawns carrying each
// weapon of its kit, and its shield.
type HeldChance struct {
	Weapon float64
	Shield float64
}

// HeldGearRules is the spawn roll's rules (the file's doc).
type HeldGearRules struct {
	// On: monsters roll; off, every NPC carries its whole kit, and corpses
	// drop no held item (handlers/helddrop.go)
	On bool
	// Default is every monster's chances
	Default HeldChance
	// Names is a name's own chances, by database.NPCClassKey
	Names map[string]HeldChance
	// NamedRows: a capitalised name on at most this many rows is a named
	// creature's
	NamedRows int
	// Profiles is the names of a loot profile of their own (lootcreature:
	// boss, named ...), by database.NPCClassKey: their whole kit
	Profiles map[string]bool
}

// DefaultHeldGear is the owner's rules unset (2026-11-17): on, a weapon at
// 50%, a shield at 40%, a name on at most 3 rows named.
var DefaultHeldGear = HeldGearRules{On: true, Default: HeldChance{Weapon: 50, Shield: 40}, NamedRows: 3}

// npcKit is what an NPC's row holds: each hand's model and attack type, and
// the shield's model.
type npcKit struct {
	Primary, Secondary         int
	PrimaryType, SecondaryType byte
	Shield                     int
}

// any reports a kit holding anything.
func (k npcKit) any() bool { return k.Primary != 0 || k.Secondary != 0 || k.Shield != 0 }

// The pieces of a kit, as KeptNPC's Unarmed carries what an NPC does not.
const (
	heldPrimary   uint8 = 1
	heldSecondary uint8 = 2
	heldShield    uint8 = 4
)

// heldGearState is the world's rules and their roll.
type heldGearState struct {
	rules HeldGearRules
	set   bool // SetHeldGear called: until then DefaultHeldGear
	// rows is how many rows of the world's carry each name (NPCClassKey), for
	// NamedRows
	rows map[string]int
	rnd  *rand.Rand
}

// heldRand is the spawn roll, in percent [0, 100) (a test's own).
var heldRand = func(w *World) float64 {
	if w.held.rnd == nil {
		w.held.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return w.held.rnd.Float64() * 100
}

// SetHeldGear sets the spawn roll's rules (the file's doc); ArmNPCs rolls by
// them.
func (w *World) SetHeldGear(r HeldGearRules) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.held.rules, w.held.set = r, true
}

// HeldGear is the spawn roll's rules.
func (w *World) HeldGear() HeldGearRules {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.heldRulesLocked()
}

func (w *World) heldRulesLocked() HeldGearRules {
	if !w.held.set {
		return DefaultHeldGear
	}
	return w.held.rules
}

// ArmedCount is what ArmNPCs did.
type ArmedCount struct {
	Kits    int // living monsters of the rows with a kit
	Rolled  int // of which rolled (the rest exempt: their whole kit)
	Whole   int // rolled and carrying their whole kit
	Bare    int // rolled and carrying none of it
	Weapons [2]int
	Shields [2]int // pieces rolled: carried, not
}

// ArmNPCs rolls every NPC of the rows for what it carries at this spawn, once
// everything the rows make is loaded (the templates, the guards, the rares):
// the server's start, before a keep restart's file is read (RestoreNPCs). It
// counts the names' rows for NamedRows first.
func (w *World) ArmNPCs() ArmedCount {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.held.rows = map[string]int{}
	for _, npc := range w.NPCs {
		if !npc.Temporary && npc.Owner == 0 {
			w.held.rows[database.NPCClassKey(npc.Name)]++
		}
	}
	var c ArmedCount
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Owner != 0 || !npc.kit.any() {
			continue
		}
		c.Kits++
		if !w.armSpawnLocked(npc) {
			continue
		}
		c.Rolled++
		switch npc.unarmed() {
		case 0:
			c.Whole++
		case npc.kitPieces():
			c.Bare++
		}
		for _, p := range []struct {
			bit   uint8
			count *[2]int
		}{{heldPrimary, &c.Weapons}, {heldSecondary, &c.Weapons}, {heldShield, &c.Shields}} {
			if npc.kitPieces()&p.bit == 0 {
				continue
			}
			if npc.unarmed()&p.bit == 0 {
				p.count[0]++
			} else {
				p.count[1]++
			}
		}
	}
	r := w.heldRulesLocked()
	log.Printf("Held gear: on %v, weapon %.0f%%, shield %.0f%% (%d names their own), named at most %d rows, %d loot profiles kept whole: "+
		"%d monsters with a kit, %d rolled (%d whole, %d bare); weapons %d carried, %d not; shields %d carried, %d not",
		r.On, r.Default.Weapon, r.Default.Shield, len(r.Names), r.NamedRows, len(r.Profiles),
		c.Kits, c.Rolled, c.Whole, c.Bare, c.Weapons[0], c.Weapons[1], c.Shields[0], c.Shields[1])
	return c
}

// kitPieces is the pieces npc's kit holds.
func (n *NPC) kitPieces() uint8 {
	var b uint8
	if n.kit.Primary != 0 {
		b |= heldPrimary
	}
	if n.kit.Secondary != 0 {
		b |= heldSecondary
	}
	if n.kit.Shield != 0 {
		b |= heldShield
	}
	return b
}

// unarmed is the pieces of its kit npc does not carry.
func (n *NPC) unarmed() uint8 {
	var b uint8
	if n.kit.Primary != 0 && n.PrimaryWeapon == 0 {
		b |= heldPrimary
	}
	if n.kit.Secondary != 0 && n.SecondaryWeapon == 0 {
		b |= heldSecondary
	}
	if n.kit.Shield != 0 && n.Shield == 0 {
		b |= heldShield
	}
	return b
}

// carriesOfKit reports npc carrying nothing but pieces of its kit: what else
// set its hands (a pet's or a mercenary's look) is not the roll's.
func (n *NPC) carriesOfKit() bool {
	return (n.PrimaryWeapon == 0 || n.PrimaryWeapon == n.kit.Primary) &&
		(n.SecondaryWeapon == 0 || n.SecondaryWeapon == n.kit.Secondary) &&
		(n.Shield == 0 || n.Shield == n.kit.Shield)
}

// carryKit makes npc carry its kit but the pieces of without.
func (n *NPC) carryKit(without uint8) {
	n.PrimaryWeapon, n.PrimaryType = n.kit.Primary, n.kit.PrimaryType
	n.SecondaryWeapon, n.SecondaryType = n.kit.Secondary, n.kit.SecondaryType
	n.Shield = n.kit.Shield
	if without&heldPrimary != 0 {
		n.PrimaryWeapon, n.PrimaryType = 0, 0
	}
	if without&heldSecondary != 0 {
		n.SecondaryWeapon, n.SecondaryType = 0, 0
	}
	if without&heldShield != 0 {
		n.Shield = 0
	}
}

// heldKeepsWholeLocked reports npc carrying its whole kit at every spawn (the
// file's doc), and the chances it rolls at otherwise. Must be called with the
// lock held.
func (w *World) heldKeepsWholeLocked(npc *NPC, r HeldGearRules) (bool, HeldChance) {
	if !r.On || npc.Owner != 0 || npc.Charm != nil || npc.Temporary || npc.SpawnOnly || npc.Dummy != NotDummy ||
		npc.isMerc() || npc.isFamiliar() || npc.isMercPet() || npc.isMonkey() || npc.DuelSpawn() || npc.Summoned() ||
		npc.NPCType&0x80 != 0 || npc.NoSelect || IsDungeonWorld(npc.World) {
		return true, HeldChance{}
	}
	key := database.NPCClassKey(npc.Name)
	if c, ok := r.Names[key]; ok {
		return false, c
	}
	if r.Profiles[key] || w.townGuardLocked(npc) || w.rareKindChangedLocked(npc) || w.heldNamedLocked(npc, r) {
		return true, HeldChance{}
	}
	// a ranger of a template shooting its bow (npcshoot.go) keeps it
	if npc.tmpl != nil && npc.tmpl.Class == classRanger && npc.kit.Primary == 0 && npc.kit.Secondary != 0 {
		switch npc.kit.SecondaryType {
		case 7, 9, 10:
			return true, HeldChance{}
		}
	}
	return false, r.Default
}

// heldNamedLocked reports a named creature: a name with a capital first
// letter on at most NamedRows rows. Must be called with the lock held.
func (w *World) heldNamedLocked(npc *NPC, r HeldGearRules) bool {
	name := strings.TrimSpace(npc.Name)
	if name == "" {
		return false
	}
	if first := []rune(name)[0]; !unicode.IsUpper(first) {
		return false
	}
	rows := w.held.rows[database.NPCClassKey(name)]
	return rows <= r.NamedRows // a name no row carries (0): its own row's gone
}

// armSpawnLocked rolls what npc carries of its kit at a spawn, reporting
// whether it rolled (false: it carries what it did, its whole kit for one made
// from its row). Must be called with the lock held.
func (w *World) armSpawnLocked(npc *NPC) bool {
	if !npc.kit.any() || !npc.carriesOfKit() {
		return false
	}
	whole, c := w.heldKeepsWholeLocked(npc, w.heldRulesLocked())
	if whole {
		return false
	}
	var without uint8
	for _, p := range []struct {
		bit    uint8
		chance float64
	}{{heldPrimary, c.Weapon}, {heldSecondary, c.Weapon}, {heldShield, c.Shield}} {
		if npc.kitPieces()&p.bit != 0 && heldRand(w) >= p.chance {
			without |= p.bit
		}
	}
	npc.carryKit(without)
	return true
}

// HeldKits is every held piece of the rows' NPCs' kits, as (model, 1 for a
// shield), for a report of the looks the held drops map (handlers/helddrop.go).
func (w *World) HeldKits() [][2]uint32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out [][2]uint32
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Owner != 0 {
			continue
		}
		for _, p := range [][2]uint32{{uint32(npc.kit.Primary), 0}, {uint32(npc.kit.Secondary), 0}, {uint32(npc.kit.Shield), 1}} {
			if p[0] != 0 {
				out = append(out, p)
			}
		}
	}
	return out
}

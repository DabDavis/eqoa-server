package gmweb

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// itemenums.go — the item record's coded columns by name (the owner,
// 2026-11-11: "for attack type in gm menu for weapons 1hb is 3 etc. lets put
// that also like Pierce(4) and any other such QoL for the item editor"). Each
// list is built from the code that reads the column, so the item editor's
// picker, the list's cells and describe_table (an agent's) say what the
// server and CLIENT do:
//   - attacktype: CLIENT's own words for 0-10 (entity.AttackTypeWords: the
//     table gAttackWords, read off a live game), each with what the server
//     does with it (entity.AttackTypeReading: its reach, its melee kind);
//   - itemtype (procanim): CLIENT's item type (database.ItemTypeWords);
//   - wearslot: CLIENT's wear slots (sWearSlotWords, viwearslot.cpp: their
//     words), and the cape's of this server past them (handlers/capes.go);
//   - ammokind (unk3): CLIENT's HasAmmoFor (0x00BDC8F8).

// attackTypeEnum is CLIENT's eleven attack types and the empty hand's.
func attackTypeEnum() []EnumValue {
	out := make([]EnumValue, 0, len(entity.AttackTypeWords)+1)
	for _, a := range entity.AttackTypeWords {
		help := "the item window shows ATT: " + a.Word
		if a.Value != 0 {
			help += "; " + entity.AttackTypeReading(a.Value)
		}
		if a.Value == 8 {
			help += " (no record here has it)"
		}
		out = append(out, EnumValue{Value: a.Value, Label: a.Name, Help: help})
	}
	return append(out, EnumValue{Value: entity.AttackTypeEmptyHand, Label: "Empty hand (not an item's)",
		Help: "CLIENT's type of a hand holding nothing (VICSprite's default); no item word: the window shows ATT: 11; an item of it " +
			entity.AttackTypeReading(entity.AttackTypeEmptyHand)})
}

// itemTypeEnum is CLIENT's item types.
func itemTypeEnum() []EnumValue {
	out := make([]EnumValue, 0, len(database.ItemTypeWords))
	for _, t := range database.ItemTypeWords {
		out = append(out, EnumValue{Value: t.Value, Label: t.Name, Help: t.Help})
	}
	return out
}

// wearSlotEnum is CLIENT's wear slots with the words its item window shows
// ("Equip: HEAD"), and the cape's.
func wearSlotEnum() []EnumValue {
	return []EnumValue{
		{Value: -1, Label: "Not worn", Help: "CLIENT's NONE: a potion, a scroll, loot, a token"},
		{Value: 0, Label: "0 (no wear slot)", Help: "CLIENT names no slot 0 (its window shows Equip: 0) and nothing wears it: use Not worn (-1)"},
		{Value: 1, Label: "Head", Help: "HEAD"}, {Value: 2, Label: "Robe", Help: "ROBE"}, {Value: 3, Label: "Earring", Help: "EAR"},
		{Value: 4, Label: "Neck", Help: "NECK"}, {Value: 5, Label: "Chest", Help: "TORSO"}, {Value: 6, Label: "Forearm", Help: "FOREARM: a bracelet"},
		{Value: 7, Label: "Both forearms", Help: "2FOREARM: bracers"}, {Value: 8, Label: "Ring", Help: "RING"}, {Value: 9, Label: "Belt", Help: "WAIST"},
		{Value: 10, Label: "Legs", Help: "LEGS"}, {Value: 11, Label: "Feet", Help: "FEET"}, {Value: 12, Label: "Primary", Help: "PRIMARY: a main-hand weapon"},
		{Value: 13, Label: "Shield", Help: "SHIELD"}, {Value: 14, Label: "Secondary", Help: "SECONDARY: a one-handed weapon either hand may hold"},
		{Value: 15, Label: "Two-handed", Help: "2HAND"}, {Value: 16, Label: "Bow", Help: "BOW: the ranged slot"},
		{Value: 17, Label: "Thrown", Help: "THROWN: the ranged slot"}, {Value: 18, Label: "Held", Help: "HELD: a totem, an orb"},
		{Value: 19, Label: "Hands", Help: "GLOVES"}, {Value: 20, Label: "Fishing pole", Help: "FISHING"}, {Value: 21, Label: "Bait", Help: "BAIT"},
		{Value: 22, Label: "Tool: Weapon Crafting", Help: "WEAPON CRAFT"}, {Value: 23, Label: "Tool: Armor Crafting", Help: "ARMOR CRAFT"},
		{Value: 24, Label: "Tool: Tailoring", Help: "TAILORING"}, {Value: 25, Label: "Tool: Jewel Crafting", Help: "JEWEL CRAFT"},
		{Value: 26, Label: "Tool: Carpentry", Help: "CARPENTRY"}, {Value: 27, Label: "Tool: Alchemy", Help: "ALCHEMY"},
		{Value: 28, Label: "Cape", Help: "this server's own (handlers/capes.go): a client that draws no capes is sent Not worn"},
	}
}

// ammoKindEnum is VIItem +0x18 (itempattern.unk3) as CLIENT's HasAmmoFor
// reads it on the items of the pack.
func ammoKindEnum() []EnumValue {
	return []EnumValue{
		{Value: 0, Label: "Not ammunition"},
		{Value: 1, Label: "Arrows", Help: "a Bow (attack type 7) shoots only with an unworn item of kind 1 in the pack, else 'Out of arrows.'"},
		{Value: 2, Label: "Bolts", Help: "a 2H Crossbow (attack type 9) shoots only with an unworn item of kind 2 in the pack, else 'Out of bolts.'"},
	}
}

// itemRaceBits are the race bits of an item's race mask (itempattern.raceuse):
// the game's ten and the froglok, each with the custom races (racedefs) that
// pass as it, which an item takes by that race's bit (handlers/equip.go
// cannotWear: entity.WireRace).
func itemRaceBits() []FlagBit {
	passing := map[int][]string{}
	for _, d := range entity.RaceDefs() {
		passing[d.Wire] = append(passing[d.Wire], d.Name)
	}
	var out []FlagBit
	for _, e := range raceEnum()[:11] {
		help := ""
		if e.Value == 10 {
			help = "given by the server to any item that names all ten others (handlers raceUseForWire)"
		}
		if names := passing[e.Value]; len(names) > 0 {
			sort.Strings(names)
			if help != "" {
				help += "; "
			}
			help += "also " + strings.Join(names, ", ") + " (custom races that pass as " + e.Label + ")"
		}
		out = append(out, FlagBit{Bit: 1 << e.Value, Label: e.Label, Help: help})
	}
	return out
}

// attackTypeName is an attack type's name and number, "1H Blunt (3)".
func attackTypeName(v int) string {
	for _, e := range attackTypeEnum() {
		if e.Value == v {
			return fmt.Sprintf("%s (%d)", e.Label, v)
		}
	}
	return fmt.Sprintf("unknown (%d)", v)
}

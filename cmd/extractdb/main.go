// extractdb reads parsed MemoryDump JSON files and imports items/spells into eqoa.db.
//
// Usage:
//
//	go run ./cmd/extractdb/
//	go run ./cmd/extractdb/ -items /tmp/memdump_items.json -spells /tmp/memdump_spells.json -db eqoa.db
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/eqoa/server/internal/database"
)

// ParsedItem mirrors the client's game.ParsedItem struct for JSON deserialization.
type ParsedItem struct {
	StackLeft     int64            `json:"StackLeft"`
	RemainingHP   int64            `json:"RemainingHP"`
	Charges       int64            `json:"Charges"`
	EquipLocation int64            `json:"EquipLocation"`
	Location      int64            `json:"Location"`
	Key           uint32           `json:"Key"`
	ItemID        int64            `json:"ItemID"`
	ItemCost      int64            `json:"ItemCost"`
	Unk1          int64            `json:"Unk1"`
	ItemIcon      int64            `json:"ItemIcon"`
	Unk2          int64            `json:"Unk2"`
	Slot          int64            `json:"Slot"`
	Unk3          int64            `json:"Unk3"`
	NoTrade       int64            `json:"NoTrade"`
	NoRent        int64            `json:"NoRent"`
	Unk4          int64            `json:"Unk4"`
	AttackType    int64            `json:"AttackType"`
	WeaponDamage  int64            `json:"WeaponDamage"`
	Unk5          int64            `json:"Unk5"`
	LevelReq      int64            `json:"LevelReq"`
	MaxStack      int64            `json:"MaxStack"`
	MaxHP         int64            `json:"MaxHP"`
	Duration      int64            `json:"Duration"`
	ClassUse      int64            `json:"ClassUse"`
	RaceUse       int64            `json:"RaceUse"`
	ProcAnim      int64            `json:"ProcAnim"`
	IsLore        int64            `json:"IsLore"`
	Unk6          int64            `json:"Unk6"`
	IsCraft       int64            `json:"IsCraft"`
	Name          string           `json:"Name"`
	Desc          string           `json:"Desc"`
	Stats         map[string]int64 `json:"Stats"` // JSON keys are strings
}

// ParsedSpell mirrors the client's game.ParsedSpell struct for JSON deserialization.
type ParsedSpell struct {
	SpellID       int64  `json:"SpellID"`
	AddedOrder    int64  `json:"AddedOrder"`
	OnHotBar      int64  `json:"OnHotBar"`
	WhereOnHotBar int64  `json:"WhereOnHotBar"`
	Unk1          int64  `json:"Unk1"`
	ShowHide      int64  `json:"ShowHide"`
	AbilityLevel  int64  `json:"AbilityLevel"`
	Unk2          int64  `json:"Unk2"`
	Unk3          int64  `json:"Unk3"`
	Range         int    `json:"Range"`
	CastTime      int64  `json:"CastTime"`
	Power         int64  `json:"Power"`
	IconColor     int64  `json:"IconColor"`
	Icon          int64  `json:"Icon"`
	Scope         int64  `json:"Scope"`
	Recast        int64  `json:"Recast"`
	EqpRequire    int64  `json:"EqpRequire"`
	Name          string `json:"Name"`
	Desc          string `json:"Desc"`
}

// statInt returns a *int for a stat key from the item's Stats map, or nil if absent.
func statInt(stats map[string]int64, key string) *int {
	v, ok := stats[key]
	if !ok {
		return nil
	}
	val := int(v)
	return &val
}

func main() {
	itemsFile := flag.String("items", "/tmp/memdump_items.json", "Path to items JSON file")
	spellsFile := flag.String("spells", "/tmp/memdump_spells.json", "Path to spells JSON file")
	dbPath := flag.String("db", "eqoa.db", "Path to eqoa.db SQLite database")
	flag.Parse()

	// Open database
	db, err := database.NewSQLite(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database %s: %v", *dbPath, err)
	}
	defer db.Close()

	itemStore := database.NewItemPatternStore(db)
	spellStore := database.NewSpellPatternStore(db)

	// Import items
	itemCount := 0
	if data, err := os.ReadFile(*itemsFile); err == nil {
		var items []ParsedItem
		if err := json.Unmarshal(data, &items); err != nil {
			log.Fatalf("Failed to parse items JSON: %v", err)
		}

		seen := make(map[int]bool)
		for _, item := range items {
			id := int(item.ItemID)
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true

			rec := &database.ItemPatternRecord{
				PatternID:    id,
				ItemCost:     int(item.ItemCost),
				Unk1:         int(item.Unk1),
				ItemIcon:     item.ItemIcon,
				Unk2:         int(item.Unk2),
				EquipSlot:    int(item.Slot),
				Unk3:         int(item.Unk3),
				Trade:        int(item.NoTrade),
				Rent:         int(item.NoRent),
				Unk4:         int(item.Unk4),
				AttackType:   int(item.AttackType),
				WeaponDamage: int(item.WeaponDamage),
				Unk5:         int(item.Unk5),
				LevelReq:     int(item.LevelReq),
				MaxStack:     int(item.MaxStack),
				MaxHP:        int(item.MaxHP),
				Duration:     int(item.Duration),
				ClassUse:     int(item.ClassUse),
				RaceUse:      int(item.RaceUse),
				ProcAnim:     int(item.ProcAnim),
				Lore:         int(item.IsLore),
				Unk6:         int(item.Unk6),
				Craft:        int(item.IsCraft),
				ItemName:     item.Name,
				ItemDesc:     item.Desc,
				Model:        0,
				Color:        0,
				// Stats: index → column mapping (C# StatModifiers)
				Str:          statInt(item.Stats, "0"),
				Sta:          statInt(item.Stats, "1"),
				Agi:          statInt(item.Stats, "2"),
				Dex:          statInt(item.Stats, "3"),
				Wis:          statInt(item.Stats, "4"),
				Cha:          statInt(item.Stats, "6"),
				Intelligence: statInt(item.Stats, "5"),
				HPMAX:        statInt(item.Stats, "8"),
				POWMAX:       statInt(item.Stats, "10"),
				PoT:          statInt(item.Stats, "12"),
				HoT:          statInt(item.Stats, "13"),
				AC:           statInt(item.Stats, "14"),
				PR:           statInt(item.Stats, "22"),
				DR:           statInt(item.Stats, "23"),
				FR:           statInt(item.Stats, "24"),
				CR:           statInt(item.Stats, "25"),
				LR:           statInt(item.Stats, "26"),
				AR:           statInt(item.Stats, "27"),
			}
			if err := itemStore.UpsertItemPattern(rec); err != nil {
				fmt.Printf("  Item %d (%s) error: %v\n", id, item.Name, err)
				continue
			}
			itemCount++
		}
		fmt.Printf("Imported %d unique items (from %d total)\n", itemCount, len(items))
	} else {
		fmt.Printf("No items file at %s: %v\n", *itemsFile, err)
	}

	// Import spells
	spellCount := 0
	if data, err := os.ReadFile(*spellsFile); err == nil {
		var spells []ParsedSpell
		if err := json.Unmarshal(data, &spells); err != nil {
			log.Fatalf("Failed to parse spells JSON: %v", err)
		}

		seen := make(map[int]bool)
		for _, sp := range spells {
			id := int(sp.SpellID)
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true

			rec := &database.SpellPatternRecord{
				SpellID:    id,
				AbilityLvl: int(sp.AbilityLevel),
				Unk2:       int(sp.Unk2),
				Unk3:       int(sp.Unk3),
				Range:      sp.Range,
				CastTime:   int(sp.CastTime),
				Power:      int(sp.Power),
				IconColor:  sp.IconColor,
				Icon:       sp.Icon,
				Scope:      int(sp.Scope),
				Recast:     int(sp.Recast),
				EqpRequire: int(sp.EqpRequire),
				SpellName:  sp.Name,
				SpellDesc:  sp.Desc,
			}
			if err := spellStore.UpsertSpellPattern(rec); err != nil {
				fmt.Printf("  Spell %d (%s) error: %v\n", id, sp.Name, err)
				continue
			}
			spellCount++
		}
		fmt.Printf("Imported %d unique spells (from %d total)\n", spellCount, len(spells))
	} else {
		fmt.Printf("No spells file at %s: %v\n", *spellsFile, err)
	}

	fmt.Printf("\nTotal: %d items, %d spells inserted into %s\n", itemCount, spellCount, *dbPath)
}

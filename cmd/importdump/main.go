// importdump parses a captured the reference server MemoryDump binary and imports the
// character's items and spells into eqoa.db. Creates pattern entries in
// itempattern/spellpattern and per-character entries in charinventory/spells.
//
// Usage:
//
//	go run ./cmd/importdump/ -bin /tmp/opcode_000D_1.bin -db eqoa.db -serverid 7
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/eqoa/server/internal/database"
)

// Parsed item/spell structs (subset of client's game.ParsedItem/ParsedSpell)
type parsedItem struct {
	StackLeft     int64
	RemainingHP   int64
	Charges       int64
	EquipLocation int64
	Location      int64
	Key           uint32
	ItemID        int64
	ItemCost      int64
	Unk1          int64
	ItemIcon      int64
	Unk2          int64
	Slot          int64
	Unk3          int64
	NoTrade       int64
	NoRent        int64
	Unk4          int64
	AttackType    int64
	WeaponDamage  int64
	Unk5          int64
	LevelReq      int64
	MaxStack      int64
	MaxHP         int64
	Duration      int64
	ClassUse      int64
	RaceUse       int64
	ProcAnim      int64
	IsLore        int64
	Unk6          int64
	IsCraft       int64
	Name          string
	Desc          string
	Stats         map[int]int64
}

type parsedSpell struct {
	SpellID       int64
	AddedOrder    int64
	OnHotBar      int64
	WhereOnHotBar int64
	Unk1          int64
	ShowHide      int64
	AbilityLevel  int64
	Unk2          int64
	Unk3          int64
	Range         int
	CastTime      int64
	Power         int64
	IconColor     int64
	Icon          int64
	Scope         int64
	Recast        int64
	EqpRequire    int64
	Name          string
	Desc          string
}

func main() {
	binPath := flag.String("bin", "/tmp/opcode_000D_1.bin", "Path to captured MemoryDump binary")
	dbPath := flag.String("db", "eqoa.db", "Path to eqoa.db")
	serverID := flag.Uint("serverid", 0, "Character server_id to assign items/spells to (0 = pattern-only import)")
	flag.Parse()

	data, err := os.ReadFile(*binPath)
	if err != nil {
		log.Fatalf("Failed to read %s: %v", *binPath, err)
	}
	fmt.Printf("Loaded MemoryDump: %d bytes\n", len(data))

	// Parse the MemoryDump
	if uint64(*serverID) > math.MaxUint32 {
		log.Fatal("serverid is outside the character ID range")
	}
	items, bankItems, spells, charName, err := parseMemoryDump(data)
	if err != nil {
		log.Fatalf("cannot import capture: %v", err)
	}
	fmt.Printf("Character: %s\n", charName)
	fmt.Printf("Parsed: %d inventory items, %d bank items, %d spells\n",
		len(items), len(bankItems), len(spells))

	// Open database
	db, err := database.NewSQLite(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database %s: %v", *dbPath, err)
	}
	defer db.Close()

	if err := importRecords(db.DB, uint32(*serverID), items, bankItems, spells); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Import committed.")
}

func importRecords(db *sql.DB, sid uint32, items, bankItems []parsedItem, spells []parsedSpell) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if sid != 0 {
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM characters WHERE serverid = ?)", sid).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("unknown character serverid %d", sid)
		}
		if _, err := tx.Exec("DELETE FROM charinventory WHERE serverid = ?", sid); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM spells WHERE serverid = ?", sid); err != nil {
			return err
		}
	}

	seenItems := make(map[int64]bool)
	for location, list := range [][]parsedItem{items, bankItems} {
		for i, item := range list {
			if item.ItemID != 0 && !seenItems[item.ItemID] {
				if err := database.UpsertItemPattern(tx, itemToRecord(&item)); err != nil {
					return fmt.Errorf("item pattern %d: %w", item.ItemID, err)
				}
				seenItems[item.ItemID] = true
			}
			if sid != 0 {
				_, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) VALUES (?,?,?,?,?,?,?,?)`,
					sid, item.StackLeft, item.RemainingHP, item.Charges, item.ItemID, item.EquipLocation, location, i)
				if err != nil {
					return fmt.Errorf("character item %d: %w", i, err)
				}
			}
		}
	}
	seenSpells := make(map[int64]bool)
	for _, sp := range spells {
		if sp.SpellID != 0 && !seenSpells[sp.SpellID] {
			if err := database.UpsertSpellPattern(tx, spellToRecord(&sp)); err != nil {
				return fmt.Errorf("spell pattern %d: %w", sp.SpellID, err)
			}
			seenSpells[sp.SpellID] = true
		}
		if sid != 0 {
			_, err := tx.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?,?,?,?,?,?,?)`,
				sid, sp.SpellID, sp.AddedOrder, sp.OnHotBar, sp.WhereOnHotBar, sp.Unk1, sp.ShowHide)
			if err != nil {
				return fmt.Errorf("character spell %d: %w", sp.SpellID, err)
			}
		}
	}
	return tx.Commit()
}

func itemToRecord(item *parsedItem) *database.ItemPatternRecord {
	rec := &database.ItemPatternRecord{
		PatternID:    int(item.ItemID),
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
	}
	// Stats
	setStatPtr := func(idx int) *int {
		if v, ok := item.Stats[idx]; ok && v != 0 {
			val := int(v)
			return &val
		}
		return nil
	}
	rec.Str = setStatPtr(0)
	rec.Sta = setStatPtr(1)
	rec.Agi = setStatPtr(2)
	rec.Dex = setStatPtr(3)
	rec.Wis = setStatPtr(4)
	rec.Intelligence = setStatPtr(5)
	rec.Cha = setStatPtr(6)
	rec.HPMAX = setStatPtr(8)
	rec.POWMAX = setStatPtr(10)
	rec.HoT = setStatPtr(12)
	rec.PoT = setStatPtr(13)
	rec.AC = setStatPtr(14)
	rec.PR = setStatPtr(22)
	rec.DR = setStatPtr(23)
	rec.FR = setStatPtr(24)
	rec.CR = setStatPtr(25)
	rec.LR = setStatPtr(26)
	rec.AR = setStatPtr(27)
	return rec
}

func spellToRecord(sp *parsedSpell) *database.SpellPatternRecord {
	return &database.SpellPatternRecord{
		SpellID:    int(sp.SpellID),
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
}

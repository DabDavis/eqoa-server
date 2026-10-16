// importdump parses a captured Sandstorm MemoryDump binary and imports the
// character's items and spells into eqoa.db. Creates pattern entries in
// itempattern/spellpattern and per-character entries in charinventory/spells.
//
// Usage:
//
//	go run ./cmd/importdump/ -bin /tmp/opcode_000D_1.bin -db eqoa.db -serverid 7
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
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
	items, bankItems, spells, charName := parseMemoryDump(data)
	fmt.Printf("Character: %s\n", charName)
	fmt.Printf("Parsed: %d inventory items, %d bank items, %d spells\n",
		len(items), len(bankItems), len(spells))

	// Open database
	db, err := database.NewSQLite(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database %s: %v", *dbPath, err)
	}
	defer db.Close()

	itemStore := database.NewItemPatternStore(db)
	spellStore := database.NewSpellPatternStore(db)

	// Import item patterns (dedup by ItemID)
	allItems := append(items, bankItems...)
	seenItems := make(map[int64]bool)
	patternCount := 0
	for _, item := range allItems {
		if item.ItemID == 0 || seenItems[item.ItemID] {
			continue
		}
		seenItems[item.ItemID] = true

		rec := itemToRecord(&item)
		if err := itemStore.UpsertItemPattern(rec); err != nil {
			fmt.Printf("  Item %d (%s) error: %v\n", item.ItemID, item.Name, err)
			continue
		}
		patternCount++
	}
	fmt.Printf("Imported %d item patterns into itempattern\n", patternCount)

	// Import spell patterns (dedup by SpellID)
	seenSpells := make(map[int64]bool)
	spellPatternCount := 0
	for _, sp := range spells {
		if sp.SpellID == 0 || seenSpells[sp.SpellID] {
			continue
		}
		seenSpells[sp.SpellID] = true

		rec := spellToRecord(&sp)
		if err := spellStore.UpsertSpellPattern(rec); err != nil {
			fmt.Printf("  Spell %d (%s) error: %v\n", sp.SpellID, sp.Name, err)
			continue
		}
		spellPatternCount++
	}
	fmt.Printf("Imported %d spell patterns into spellpattern\n", spellPatternCount)

	// Populate per-character tables if serverid provided
	if *serverID == 0 {
		fmt.Println("\nNo --serverid specified, skipping charinventory/spells population.")
		fmt.Println("Use --serverid N to assign items/spells to a character.")
		return
	}

	sid := uint32(*serverID)

	// Clear existing entries for this character
	db.Exec("DELETE FROM charinventory WHERE serverid = ?", sid)
	db.Exec("DELETE FROM spells WHERE serverid = ?", sid)

	// Insert inventory items (location=0)
	invCount := 0
	for i, item := range items {
		_, err := db.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) VALUES (?,?,?,?,?,?,?,?)`,
			sid, item.StackLeft, item.RemainingHP, item.Charges,
			item.ItemID, item.EquipLocation, 0, i)
		if err != nil {
			fmt.Printf("  charinventory insert error (item %d %s): %v\n", item.ItemID, item.Name, err)
			continue
		}
		invCount++
	}

	// Insert bank items (location=1)
	bankCount := 0
	for i, item := range bankItems {
		_, err := db.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) VALUES (?,?,?,?,?,?,?,?)`,
			sid, item.StackLeft, item.RemainingHP, item.Charges,
			item.ItemID, item.EquipLocation, 1, i)
		if err != nil {
			fmt.Printf("  charinventory bank insert error (item %d %s): %v\n", item.ItemID, item.Name, err)
			continue
		}
		bankCount++
	}
	fmt.Printf("Inserted %d inventory + %d bank items into charinventory for serverid=%d\n",
		invCount, bankCount, sid)

	// Insert spells
	spellInsertCount := 0
	for _, sp := range spells {
		_, err := db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?,?,?,?,?,?,?)`,
			sid, sp.SpellID, sp.AddedOrder, sp.OnHotBar, sp.WhereOnHotBar, sp.Unk1, sp.ShowHide)
		if err != nil {
			fmt.Printf("  spells insert error (spell %d %s): %v\n", sp.SpellID, sp.Name, err)
			continue
		}
		spellInsertCount++
	}
	fmt.Printf("Inserted %d spells into spells for serverid=%d\n", spellInsertCount, sid)

	fmt.Printf("\nDone! Character serverid=%d now has %d items and %d spells.\n",
		sid, invCount+bankCount, spellInsertCount)
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
	rec.PoT = setStatPtr(12)
	rec.HoT = setStatPtr(13)
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

// parseMemoryDump parses the Sandstorm MemoryDump binary.
// Returns inventory items, bank items, spells, and character name.
func parseMemoryDump(data []byte) (invItems, bankItems []parsedItem, spells []parsedSpell, charName string) {
	// Skip opcode (2 bytes)
	reader := buffer.NewReader(data[2:])

	// Unknown byte
	reader.ReadByte()

	// Zone
	pathLen, _ := reader.ReadInt32()
	reader.ReadBytes(int(pathLen)) // skip zone path

	// Character header
	reader.Read7BitEncodedInt64() // CharServerID
	nameLen, _ := reader.ReadInt32()
	nameBytes, _ := reader.ReadBytes(int(nameLen))
	charName = string(nameBytes)
	reader.Read7BitEncodedInt64() // Class
	reader.Read7BitEncodedInt64() // Race
	reader.Read7BitEncodedInt64() // Level
	reader.Read7BitEncodedInt64() // XP
	reader.Read7BitEncodedInt64() // Debt
	reader.ReadByte()             // Breath
	reader.Read7BitEncodedInt64() // Tunar
	reader.Read7BitEncodedInt64() // BankTunar
	reader.Read7BitEncodedInt64() // UnusedTP
	reader.Read7BitEncodedInt64() // BaseMaxStat
	reader.Read7BitEncodedInt64() // World

	// Position
	reader.ReadFloat32()
	reader.ReadFloat32()
	reader.ReadFloat32()
	reader.ReadFloat32()

	// Two unknown floats
	reader.ReadFloat32()
	reader.ReadFloat32()

	// Hotkeys
	hotkeyCount, _ := reader.Read7BitEncodedInt64()
	for i := int64(0); i < hotkeyCount; i++ {
		reader.Read7BitEncodedInt64() // direction
		for slot := 0; slot < 4; slot++ {
			reader.ReadByte() // slotID
			labelLen, _ := reader.ReadInt32()
			if labelLen > 0 && labelLen < 500 {
				reader.ReadBytes(int(labelLen) * 2)
			}
			msgLen, _ := reader.ReadInt32()
			if msgLen > 0 && msgLen < 500 {
				reader.ReadBytes(int(msgLen) * 2)
			}
		}
	}

	// Two int32 zeros
	reader.ReadInt32()
	reader.ReadInt32()

	// Quests
	questCount, _ := reader.ReadUint32()
	for i := uint32(0); i < questCount; i++ {
		textLen, _ := reader.ReadInt32()
		if textLen > 0 && textLen < 5000 {
			reader.ReadBytes(int(textLen) * 2)
		}
	}

	// Inventory
	invCount, _ := reader.Read7BitEncodedInt64()
	reader.ReadUint32() // raw count
	for i := int64(0); i < invCount; i++ {
		item, err := readItem(reader)
		if err != nil {
			log.Printf("inventory item %d parse error: %v", i, err)
			return
		}
		invItems = append(invItems, item)
	}

	// Bank
	bankCount, _ := reader.Read7BitEncodedInt64()
	reader.ReadUint32() // raw count
	for i := int64(0); i < bankCount; i++ {
		item, err := readItem(reader)
		if err != nil {
			log.Printf("bank item %d parse error: %v", i, err)
			return
		}
		bankItems = append(bankItems, item)
	}

	// Transition bytes
	reader.ReadByte()
	reader.ReadByte()
	reader.ReadByte()
	reader.ReadByte()

	// Spells
	spellCount, _ := reader.Read7BitEncodedInt64()
	for i := int64(0); i < spellCount; i++ {
		sp, err := readSpell(reader)
		if err != nil {
			log.Printf("spell %d parse error: %v", i, err)
			return
		}
		spells = append(spells, sp)
	}

	return
}

func readItem(reader *buffer.Reader) (parsedItem, error) {
	var item parsedItem
	item.StackLeft, _ = reader.Read7BitEncodedInt64()
	item.RemainingHP, _ = reader.Read7BitEncodedInt64()
	item.Charges, _ = reader.Read7BitEncodedInt64()
	item.EquipLocation, _ = reader.Read7BitEncodedInt64()
	item.Location, _ = reader.Read7BitEncodedInt64()
	item.Key, _ = reader.ReadUint32()
	item.ItemID, _ = reader.Read7BitEncodedInt64()
	item.ItemCost, _ = reader.Read7BitEncodedInt64()
	item.Unk1, _ = reader.Read7BitEncodedInt64()
	item.ItemIcon, _ = reader.Read7BitEncodedInt64()
	item.Unk2, _ = reader.Read7BitEncodedInt64()
	item.Slot, _ = reader.Read7BitEncodedInt64()
	item.Unk3, _ = reader.Read7BitEncodedInt64()
	item.NoTrade, _ = reader.Read7BitEncodedInt64()
	item.NoRent, _ = reader.Read7BitEncodedInt64()
	item.Unk4, _ = reader.Read7BitEncodedInt64()
	item.AttackType, _ = reader.Read7BitEncodedInt64()
	item.WeaponDamage, _ = reader.Read7BitEncodedInt64()
	item.Unk5, _ = reader.Read7BitEncodedInt64()
	item.LevelReq, _ = reader.Read7BitEncodedInt64()
	item.MaxStack, _ = reader.Read7BitEncodedInt64()
	item.MaxHP, _ = reader.Read7BitEncodedInt64()
	item.Duration, _ = reader.Read7BitEncodedInt64()
	item.ClassUse, _ = reader.Read7BitEncodedInt64()
	item.RaceUse, _ = reader.Read7BitEncodedInt64()
	item.ProcAnim, _ = reader.Read7BitEncodedInt64()
	item.IsLore, _ = reader.Read7BitEncodedInt64()
	item.Unk6, _ = reader.Read7BitEncodedInt64()
	item.IsCraft, _ = reader.Read7BitEncodedInt64()

	// Name
	nameLen, err := reader.ReadInt32()
	if err != nil || nameLen < 0 || nameLen > 200 {
		return item, fmt.Errorf("bad name len: %d", nameLen)
	}
	if nameLen > 0 {
		item.Name = readUTF16(reader, int(nameLen))
	}

	// Desc
	descLen, err := reader.ReadInt32()
	if err != nil || descLen < 0 || descLen > 500 {
		return item, fmt.Errorf("bad desc len: %d", descLen)
	}
	if descLen > 0 {
		item.Desc = readUTF16(reader, int(descLen))
	}

	// Stats
	statSize, _ := reader.Read7BitEncodedInt64()
	item.Stats = make(map[int]int64)
	for i := int64(0); i < statSize; i++ {
		idx, _ := reader.Read7BitEncodedInt64()
		val, err := reader.Read7BitEncodedInt64()
		if err != nil {
			break
		}
		item.Stats[int(idx)] = val
	}

	return item, nil
}

func readSpell(reader *buffer.Reader) (parsedSpell, error) {
	var sp parsedSpell
	sp.SpellID, _ = reader.Read7BitEncodedInt64()
	sp.AddedOrder, _ = reader.Read7BitEncodedInt64()
	sp.OnHotBar, _ = reader.Read7BitEncodedInt64()
	sp.WhereOnHotBar, _ = reader.Read7BitEncodedInt64()
	sp.Unk1, _ = reader.Read7BitEncodedInt64()
	sp.ShowHide, _ = reader.Read7BitEncodedInt64()
	sp.AbilityLevel, _ = reader.Read7BitEncodedInt64()
	sp.Unk2, _ = reader.Read7BitEncodedInt64()
	sp.Unk3, _ = reader.Read7BitEncodedInt64()

	rangeRaw, _ := reader.ReadUint16()
	sp.Range = int(rangeRaw)

	sp.CastTime, _ = reader.Read7BitEncodedInt64()
	sp.Power, _ = reader.Read7BitEncodedInt64()
	sp.IconColor, _ = reader.Read7BitEncodedInt64()
	sp.Icon, _ = reader.Read7BitEncodedInt64()
	sp.Scope, _ = reader.Read7BitEncodedInt64()
	sp.Recast, _ = reader.Read7BitEncodedInt64()
	sp.EqpRequire, _ = reader.Read7BitEncodedInt64()

	nameLen, err := reader.ReadInt32()
	if err != nil || nameLen < 0 || nameLen > 200 {
		return sp, fmt.Errorf("bad spell name len: %d", nameLen)
	}
	if nameLen > 0 {
		sp.Name = readUTF16(reader, int(nameLen))
	}

	descLen, err := reader.ReadInt32()
	if err != nil || descLen < 0 || descLen > 2000 {
		return sp, fmt.Errorf("bad spell desc len: %d", descLen)
	}
	if descLen > 0 {
		sp.Desc = readUTF16(reader, int(descLen))
	}

	reader.ReadByte() // terminator

	return sp, nil
}

// readUTF16 reads charCount UTF-16LE code units and returns a Go string.
func readUTF16(reader *buffer.Reader, charCount int) string {
	raw, err := reader.ReadBytes(charCount * 2)
	if err != nil {
		return ""
	}
	u16 := make([]uint16, charCount)
	for i := 0; i < charCount; i++ {
		u16[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(u16))
}

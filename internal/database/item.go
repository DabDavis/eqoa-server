package database

import "strings"

// ItemPatternRecord represents a row from the itempattern table.
// All 44+2 columns from the schema (patternid through fish).
type ItemPatternRecord struct {
	PatternID    int
	ItemCost     int
	Unk1         int
	ItemIcon     int64
	Unk2         int
	EquipSlot    int
	Unk3         int
	Trade        int
	Rent         int
	Unk4         int
	AttackType   int
	WeaponDamage int
	Unk5         int
	LevelReq     int
	MaxStack     int
	MaxHP        int
	Duration     int
	ClassUse     int
	RaceUse      int
	ProcAnim     int
	Lore         int
	Unk6         int
	Craft        int
	ItemName     string
	ItemDesc     string
	Model        int64
	Color        int64
	// Stat columns (nullable in schema)
	Str          *int
	Sta          *int
	Agi          *int
	Wis          *int
	Dex          *int
	Cha          *int
	Intelligence *int
	HPMAX        *int
	POWMAX       *int
	PoT          *int
	HoT          *int
	AC           *int
	PR           *int
	DR           *int
	FR           *int
	CR           *int
	LR           *int
	AR           *int
	WeaponProc   *int
	Fish         *int
}

// ItemPatternStore queries the itempattern table.
type ItemPatternStore struct {
	db *SQLiteDB
}

// NewItemPatternStore creates a new item pattern store.
func NewItemPatternStore(db *SQLiteDB) *ItemPatternStore {
	return &ItemPatternStore{db: db}
}

// GetItemPattern retrieves an item pattern by patternid.
func (s *ItemPatternStore) GetItemPattern(patternID int) (*ItemPatternRecord, error) {
	row := s.db.QueryRow(`
		SELECT patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3,
			trade, rent, unk4, attacktype, weapondamage, unk5,
			levelreq, maxstack, maxhp, duration, classuse, raceuse,
			procanim, lore, unk6, craft, itemname, itemdesc, model, color,
			str, sta, agi, wis, dex, cha, intelligence,
			HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR,
			weaponproc, fish
		FROM itempattern WHERE patternid = ?
	`, patternID)

	var i ItemPatternRecord
	err := row.Scan(
		&i.PatternID, &i.ItemCost, &i.Unk1, &i.ItemIcon, &i.Unk2,
		&i.EquipSlot, &i.Unk3, &i.Trade, &i.Rent, &i.Unk4,
		&i.AttackType, &i.WeaponDamage, &i.Unk5,
		&i.LevelReq, &i.MaxStack, &i.MaxHP, &i.Duration,
		&i.ClassUse, &i.RaceUse, &i.ProcAnim, &i.Lore, &i.Unk6, &i.Craft,
		&i.ItemName, &i.ItemDesc, &i.Model, &i.Color,
		&i.Str, &i.Sta, &i.Agi, &i.Wis, &i.Dex, &i.Cha, &i.Intelligence,
		&i.HPMAX, &i.POWMAX, &i.PoT, &i.HoT, &i.AC,
		&i.PR, &i.DR, &i.FR, &i.CR, &i.LR, &i.AR,
		&i.WeaponProc, &i.Fish,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// UpsertItemPattern inserts or replaces an item pattern by patternid.
func (s *ItemPatternStore) UpsertItemPattern(r *ItemPatternRecord) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO itempattern (
			patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3,
			trade, rent, unk4, attacktype, weapondamage, unk5,
			levelreq, maxstack, maxhp, duration, classuse, raceuse,
			procanim, lore, unk6, craft, itemname, itemdesc, model, color,
			str, sta, agi, wis, dex, cha, intelligence,
			HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR,
			weaponproc, fish
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`,
		r.PatternID, r.ItemCost, r.Unk1, r.ItemIcon, r.Unk2,
		r.EquipSlot, r.Unk3, r.Trade, r.Rent, r.Unk4,
		r.AttackType, r.WeaponDamage, r.Unk5,
		r.LevelReq, r.MaxStack, r.MaxHP, r.Duration,
		r.ClassUse, r.RaceUse, r.ProcAnim, r.Lore, r.Unk6, r.Craft,
		r.ItemName, r.ItemDesc, r.Model, r.Color,
		r.Str, r.Sta, r.Agi, r.Wis, r.Dex, r.Cha, r.Intelligence,
		r.HPMAX, r.POWMAX, r.PoT, r.HoT, r.AC,
		r.PR, r.DR, r.FR, r.CR, r.LR, r.AR,
		r.WeaponProc, r.Fish,
	)
	return err
}

// Count returns the total number of item patterns.
func (s *ItemPatternStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM itempattern`).Scan(&count)
	return count, err
}

// WearablePatterns is every item of the wear slots given that a character of
// class and race may wear at level, as CLIENT's VIPlayer::CanUse judges it:
// its class's bit in classuse (1 << class), its race's in raceuse (1 <<
// race), and levelreq at most the level. In patternid order.
func (s *InventoryStore) WearablePatterns(class, race, level int, wear []int) ([]ItemPatternRecord, error) {
	if len(wear) == 0 || class < 0 || class > 30 || race < 0 || race > 30 {
		return nil, nil
	}
	in := "?" + strings.Repeat(", ?", len(wear)-1)
	args := []any{1 << class, 1 << race, level}
	for _, w := range wear {
		args = append(args, w)
	}
	rows, err := s.db.Query(`
		SELECT patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3,
			trade, rent, unk4, attacktype, weapondamage, unk5,
			levelreq, maxstack, maxhp, duration, classuse, raceuse,
			procanim, lore, unk6, craft, itemname, itemdesc, model, color,
			str, sta, agi, wis, dex, cha, intelligence,
			HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR,
			weaponproc, fish
		FROM itempattern
		WHERE (classuse & ?) != 0 AND (raceuse & ?) != 0 AND levelreq <= ? AND equipslot IN (`+in+`)
		ORDER BY patternid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemPatternRecord
	for rows.Next() {
		var i ItemPatternRecord
		if err := rows.Scan(
			&i.PatternID, &i.ItemCost, &i.Unk1, &i.ItemIcon, &i.Unk2,
			&i.EquipSlot, &i.Unk3, &i.Trade, &i.Rent, &i.Unk4,
			&i.AttackType, &i.WeaponDamage, &i.Unk5,
			&i.LevelReq, &i.MaxStack, &i.MaxHP, &i.Duration,
			&i.ClassUse, &i.RaceUse, &i.ProcAnim, &i.Lore, &i.Unk6, &i.Craft,
			&i.ItemName, &i.ItemDesc, &i.Model, &i.Color,
			&i.Str, &i.Sta, &i.Agi, &i.Wis, &i.Dex, &i.Cha, &i.Intelligence,
			&i.HPMAX, &i.POWMAX, &i.PoT, &i.HoT, &i.AC,
			&i.PR, &i.DR, &i.FR, &i.CR, &i.LR, &i.AR,
			&i.WeaponProc, &i.Fish,
		); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

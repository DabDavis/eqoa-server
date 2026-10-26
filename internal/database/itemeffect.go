package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// itemeffect: what using an item does (entity/potion.go), the alchemists'
// potions first (F3 20261025-223151, -223841, -223959). One row an item:
//
//   - line: what it does, in spelleffect's words (entity/spelltext.go); its
//     first word says on whom: "Self" (drunk: a draught, a speed or an
//     invisibility potion) or "Target" (thrown at a creature);
//   - name: the effect's name on the effects list (” is the item's own);
//   - icon: the effect's icon there, an ability's front icon (a UI.CSF
//     surface, spellpattern.icon), 0 for none;
//   - breaks: what ends it early, a comma list: "attack" (the bearer's own
//     auto-attack, harmful cast or throw), "damage" (a hit on the bearer);
//   - wear: the percent chance each six-second tick that it wears off;
//   - area: a thrown potion's splash, how far round its target other
//     creatures take it too (0: the target alone);
//   - source: who wrote it (tools/seed_potions.py's rows say "potions").
const itemEffectSchema = `
	CREATE TABLE IF NOT EXISTS itemeffect (
		patternid INTEGER PRIMARY KEY,
		line      TEXT NOT NULL,
		name      TEXT NOT NULL DEFAULT '',
		icon      INTEGER NOT NULL DEFAULT 0,
		breaks    TEXT NOT NULL DEFAULT '',
		wear      INTEGER NOT NULL DEFAULT 0,
		area      REAL NOT NULL DEFAULT 0,
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureItemEffects makes table itemeffect (initSchema).
func (db *SQLiteDB) ensureItemEffects() error {
	if _, err := db.Exec(itemEffectSchema); err != nil {
		return fmt.Errorf("itemeffect: %w", err)
	}
	return nil
}

// ItemEffectRow is one row of itemeffect, with its item's name.
type ItemEffectRow struct {
	PatternID int
	ItemName  string
	Line      string
	Name      string
	Icon      uint32
	Breaks    string
	Wear      int
	Area      float64
}

const itemEffectSelect = `SELECT e.patternid, COALESCE(p.itemname, ''), e.line, e.name, e.icon, e.breaks, e.wear, e.area
	FROM itemeffect e LEFT JOIN itempattern p ON p.patternid = e.patternid`

func scanItemEffect(sc interface{ Scan(...any) error }) (ItemEffectRow, error) {
	var r ItemEffectRow
	var icon int64
	err := sc.Scan(&r.PatternID, &r.ItemName, &r.Line, &r.Name, &icon, &r.Breaks, &r.Wear, &r.Area)
	r.Icon = uint32(icon)
	return r, err
}

// ItemEffects is every row of itemeffect.
func (db *SQLiteDB) ItemEffects() ([]ItemEffectRow, error) {
	rows, err := db.Query(itemEffectSelect)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemEffectRow
	for rows.Next() {
		r, err := scanItemEffect(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ItemEffects is every row of itemeffect.
func (s *InventoryStore) ItemEffects() ([]ItemEffectRow, error) {
	return s.db.ItemEffects()
}

// ItemEffect is the item's row of itemeffect, ok false when it has none: read
// at every use, so a GM's change counts at once.
func (s *InventoryStore) ItemEffect(patternID int) (ItemEffectRow, bool, error) {
	r, err := scanItemEffect(s.db.QueryRow(itemEffectSelect+` WHERE e.patternid = ?`, patternID))
	if errors.Is(err, sql.ErrNoRows) {
		return ItemEffectRow{}, false, nil
	}
	if err != nil {
		return ItemEffectRow{}, false, err
	}
	return r, true, nil
}

// ScrollArt is a spell scroll's icon: an unworn item with it is scribed
// (handlers/scribe.go's scrollIcon), and named after its spell.
const ScrollArt = -607649328

// ItemPatternByName is the item a spell's words summon by that name, the case
// not counted, never a spell scroll (a summoning spell's own scroll shares its
// name, and a cocktail's spell its potion's): of several, one that has a use
// (an itemeffect row) first, then the lowest id. ok false when none has it.
func (s *InventoryStore) ItemPatternByName(name string) (ItemPatternRecord, bool, error) {
	var id int
	err := s.db.QueryRow(`SELECT p.patternid FROM itempattern p LEFT JOIN itemeffect e ON e.patternid = p.patternid
		WHERE lower(p.itemname) = ? AND NOT (p.itemicon = ? AND p.equipslot = -1)
		ORDER BY e.patternid IS NULL, p.patternid LIMIT 1`,
		strings.ToLower(strings.TrimSpace(name)), ScrollArt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ItemPatternRecord{}, false, nil
	}
	if err != nil {
		return ItemPatternRecord{}, false, err
	}
	p, err := NewItemPatternStore(s.db).GetItemPattern(id)
	if err != nil {
		return ItemPatternRecord{}, false, err
	}
	return *p, true, nil
}

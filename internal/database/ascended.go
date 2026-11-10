package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ascended.go — Ascended gear: what a tuned raid boss drops in place of its
// regular gear (petition 1446, the GM Jibrish, 2026-11-10: "For all tuned raid
// bosses, and also when we tune new ones going forward, they should drop
// "Ascended" versions of their regular gear in place of their normal loot
// table. These should be the regular items except with a flat +10% to all of
// their stats. The tuned raid bosses should no longer drop their regular drops
// outside of spells, only the Ascended versions. These are NOT no rent. ... The
// ascended stats should impact all stats on the items except durability,
// vendor cost.").
//
// An Ascended record is an itempattern row of its own (ids from
// ascendedFirst), made once a base (AscendedCopyTx) and listed in
// itemascended with its base, as an import's copy is (imported.go): named
// "Ascended <its base's name>", and the same figures an import's copy scales
// (importedScaled: the seven stats, the hit point and power maxima and their
// ticks, AC, the six resists, a weapon's damage) each a tenth more
// (AscendedFigure: a figure above 0 times 110 / 100 in whole numbers, half up,
// so 4 stays 4, 5 is 6 and 15 is 17; a figure of 0 or below, a penalty, as it
// is, since a tenth more never makes an item worse); its use effects
// (itemeffect) with each "by N" so. Everything else is its base's: its
// durability (maxhp) and its price (itemcost), as the petition says, its look,
// icon, slot, level, classes and races, its NO TRADE and Lore flags; but never
// NO RENT: a base of rent 0 (Lord Nagafen's while they were made to vanish,
// norent.go) makes a copy of rent 1, and a copy is never in norentvanish. What a
// record's figures do not hold, a weapon's proc and a worn item's hit effect
// read by the record's name, is scaled where it is read (entity/ascended.go).
//
// The records made are gear (IsAscendableGear): what is worn in the gear's
// slots 1-19 (armour, jewellery, weapons, a bow, a held item, gloves: the
// equipment of a corpse's level rule, handlers/loot.go), not a stack, not a
// thrown potion (attack type 10), not an Heirloom, not a copy (an import's or
// an Ascended one). A spell scroll (wear slot -1, ScrollArt) never is.
//
// A copy follows its base: at every start SyncAscendedCopies makes each copy
// its base again by the rule (a fixed look or figure on the base reaches its
// copy at the next start: fix the base, never a copy). The bosses' loot points
// at the copies (tools/ascend_boss_loot.py, which makes them in this file's
// SQL, held to it by TestAscendBossLootToolKeepsTheRule); a quest hand-in takes
// an Ascended item for its base and rewards Ascended (handlers/questascended.go),
// making a reward's copy here when it has none. The records the reference kits
// are made of leave them out (WearablePatterns: the heirlooms' figures and the
// balance's kits are the regular items').

// AscendedPrefix is what an Ascended record's name is its base's after.
const AscendedPrefix = "Ascended "

// AscendedPercent is how much more each of an Ascended record's figures is
// than its base's.
const AscendedPercent = 10

// ascendedFirst and ascendedLast bound the Ascended records' ids (below the
// imported copies' 990001-999999, used by no tool or import).
const (
	ascendedFirst = 980001
	ascendedLast  = 989999
)

// MaxItemNameChars is the longest item name CLIENT shows: VIItem's m_name
// holds 64 UTF-16 units with its NUL, and VIMemStream::ReadString skips a
// longer one whole and leaves the name empty (decomp vimemstream.cpp).
const MaxItemNameChars = 63

// The gear's wear slots (itempattern.equipslot), and the attack type of a
// thrown potion: what handlers/loot.go's level rule calls equipment.
const (
	ascendedGearFirstSlot = 1
	ascendedGearLastSlot  = 19
	ascendedThrownPotion  = 10
)

// ErrAscendedNameTooLong is a base whose Ascended name CLIENT would show empty.
var ErrAscendedNameTooLong = errors.New("its Ascended name is longer than the client shows")

// AscendedSchema is table itemascended: each Ascended record (patternid), the
// record it was made from (base), when and by what.
const AscendedSchema = `CREATE TABLE IF NOT EXISTS itemascended (
	patternid INTEGER PRIMARY KEY,
	base      INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)`

// AscendedFigure is a figure as an Ascended record has it.
func AscendedFigure(v int) int {
	if v <= 0 {
		return v
	}
	return (v*(100+AscendedPercent) + 50) / 100
}

// ascendedFigureSQL is AscendedFigure on column q, in whole numbers as SQLite
// divides them (an empty figure stays empty).
const ascendedFigureSQL = `CASE WHEN %[1]s IS NULL THEN NULL WHEN CAST(%[1]s AS INTEGER) > 0 ` +
	`THEN (CAST(%[1]s AS INTEGER) * %[2]d + 50) / 100 ELSE %[1]s END`

// ascendedNameSQL and ascendedRentSQL are the name and the rent an Ascended
// record takes from its base's.
const (
	ascendedNameSQL = `'` + AscendedPrefix + `' || trim(%[1]s)`
	ascendedRentSQL = `CASE WHEN %[1]s = 0 THEN 1 ELSE %[1]s END`
)

// AscendedName is the name of base's Ascended record.
func AscendedName(base string) string { return AscendedPrefix + strings.TrimSpace(base) }

// AscendedBaseName is the name of the record an Ascended record's name was
// made from: name without the prefix, any case; name as it is without one.
func AscendedBaseName(name string) string {
	s := strings.TrimSpace(name)
	if len(s) >= len(AscendedPrefix) && strings.EqualFold(s[:len(AscendedPrefix)], AscendedPrefix) {
		return strings.TrimSpace(s[len(AscendedPrefix):])
	}
	return s
}

// IsAscendableGear reports whether a record is gear an Ascended record is made
// of: worn in the gear's slots, not a stack, not a thrown potion, not an
// Heirloom (that it is no copy is AscendedCopyTx's to see).
func IsAscendableGear(equipSlot, maxStack, attackType int, name string) bool {
	return equipSlot >= ascendedGearFirstSlot && equipSlot <= ascendedGearLastSlot && maxStack <= 1 &&
		attackType != ascendedThrownPotion && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "heirloom ")
}

// AscendedMap is every Ascended record and its base (copy -> base).
func AscendedMap(db *sql.DB) (map[int]int, error) {
	if _, err := db.Exec(AscendedSchema); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT patternid, base FROM itemascended`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int]int{}
	for rows.Next() {
		var c, b int
		if err := rows.Scan(&c, &b); err != nil {
			return nil, err
		}
		m[c] = b
	}
	return m, rows.Err()
}

// ascendedLine is an effect line with each "by N" a tenth more.
func ascendedLine(line string) string {
	return importedBy.ReplaceAllStringFunc(line, func(s string) string {
		n, err := strconv.Atoi(s[3:])
		if err != nil {
			return s
		}
		return s[:3] + strconv.Itoa(AscendedFigure(n))
	})
}

// ascendedColumns are itempattern's columns but patternid, quoted (set), and
// what an Ascended record takes for each from its base (from).
func ascendedColumns(cols []string) (set, from string) {
	var s, f []string
	for _, c := range cols {
		low := strings.ToLower(c)
		if low == "patternid" {
			continue
		}
		q := `"` + c + `"`
		s = append(s, q)
		switch {
		case importedScaled[low]:
			f = append(f, fmt.Sprintf(ascendedFigureSQL, q, 100+AscendedPercent))
		case low == "itemname":
			f = append(f, fmt.Sprintf(ascendedNameSQL, q))
		case low == "rent":
			f = append(f, fmt.Sprintf(ascendedRentSQL, q))
		default:
			f = append(f, q)
		}
	}
	return strings.Join(s, ", "), strings.Join(f, ", ")
}

// AscendedCopyTx is base's Ascended record: the one made already, or one made
// now (made true) in tx, with source saying what made it. A base that is not
// gear (IsAscendableGear), a copy (an import's or an Ascended one), or no
// record at all is given back as it is; ErrAscendedNameTooLong for one whose
// Ascended name CLIENT would show empty.
func AscendedCopyTx(tx *sql.Tx, base int, source string, now time.Time) (id int, made bool, err error) {
	if _, err := tx.Exec(AscendedSchema); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(ImportedSchema); err != nil {
		return 0, false, err
	}
	if err := tx.QueryRow(`SELECT patternid FROM itemascended WHERE base = ?`, base).Scan(&id); err == nil {
		return id, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	var copies int
	if err := tx.QueryRow(`SELECT (SELECT count(*) FROM itemascended WHERE patternid = ?) + (SELECT count(*) FROM itemimported WHERE patternid = ?)`,
		base, base).Scan(&copies); err != nil {
		return 0, false, err
	}
	var equip, stack, attack int
	var name string
	switch err := tx.QueryRow(`SELECT equipslot, COALESCE(maxstack, 0), COALESCE(attacktype, 0), itemname FROM itempattern WHERE patternid = ?`, base).
		Scan(&equip, &stack, &attack, &name); {
	case errors.Is(err, sql.ErrNoRows):
		return base, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("record %d: %w", base, err)
	}
	if copies > 0 || !IsAscendableGear(equip, stack, attack, name) {
		return base, false, nil
	}
	if utf8.RuneCountInString(AscendedName(name)) > MaxItemNameChars {
		return 0, false, fmt.Errorf("record %d, %q: %w (%d characters)", base, name, ErrAscendedNameTooLong, MaxItemNameChars)
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?`,
		ascendedFirst-1, ascendedFirst, ascendedLast).Scan(&id); err != nil {
		return 0, false, err
	}
	if id > ascendedLast {
		return 0, false, fmt.Errorf("no id left for an Ascended record (%d-%d)", ascendedFirst, ascendedLast)
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, false, err
	}
	set, from := ascendedColumns(cols)
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO itempattern ("patternid", %s) SELECT %d, %s FROM itempattern WHERE patternid = ?`,
		set, id, from), base); err != nil {
		return 0, false, fmt.Errorf("copying record %d: %w", base, err)
	}
	if err := copyItemEffects(tx, base, id, ascendedLine); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(`INSERT INTO itemascended (patternid, base, made, source) VALUES (?, ?, ?, ?)`,
		id, base, now.UTC().Format(time.RFC3339), source); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// SyncAscendedCopies makes every Ascended record its base again by the rule
// (the server's start): a base a GM or a petition's fix changes (its look, its
// figures, its name) takes its Ascended record with it, its use effects too.
// It says how many records changed.
func SyncAscendedCopies(db *sql.DB, now time.Time) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(AscendedSchema); err != nil {
		return 0, err
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, err
	}
	set, from := ascendedColumns(cols)
	pairs := make([][2]int, 0)
	rows, err := tx.Query(`SELECT a.patternid, a.base FROM itemascended a JOIN itempattern b ON b.patternid = a.base`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var c, b int
		if err := rows.Scan(&c, &b); err != nil {
			rows.Close()
			return 0, err
		}
		pairs = append(pairs, [2]int{c, b})
	}
	rows.Close()
	changed := 0
	for _, p := range pairs {
		// only a record that differs from what its base makes is written
		res, err := tx.Exec(fmt.Sprintf(`UPDATE itempattern SET (%s) = (SELECT %s FROM itempattern WHERE patternid = ?)
			WHERE patternid = ? AND (%s) IS NOT (SELECT %s FROM itempattern WHERE patternid = ?)`,
			set, from, set, from), p[1], p[0], p[1])
		if err != nil {
			return 0, fmt.Errorf("Ascended record %d of record %d: %w", p[0], p[1], err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
		if _, err := tx.Exec(`DELETE FROM itemeffect WHERE patternid = ?`, p[0]); err != nil && !strings.Contains(err.Error(), "no such table") {
			return 0, err
		}
		if err := copyItemEffects(tx, p[1], p[0], ascendedLine); err != nil {
			return 0, err
		}
	}
	return changed, tx.Commit()
}

// AscendedMap is ItemPatternStore's view of table itemascended (copy -> base).
func (s *ItemPatternStore) AscendedMap() (map[int]int, error) { return AscendedMap(s.db.DB) }

// AscendedOf is base's Ascended record, made now (made true) when it has none
// (AscendedCopyTx; source says what made it): base itself when it is no gear.
func (s *ItemPatternStore) AscendedOf(base int, source string) (id int, made bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	if id, made, err = AscendedCopyTx(tx, base, source, time.Now()); err != nil {
		return 0, false, err
	}
	return id, made, tx.Commit()
}

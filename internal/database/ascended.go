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
// regular gear (petition 1446, 2026-11-10: "For all tuned raid
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
// The records made here are gear (IsAscendableGear): what is worn in the
// gear's slots 1-19 (armour, jewellery, weapons, a bow, a held item, gloves:
// the equipment of a corpse's level rule, handlers/loot.go), not a stack, not
// a thrown potion (attack type 10), not an Heirloom, not a copy (an import's
// or an Ascended one). tools/ascend_boss_loot.py also makes one, by the same
// SQL, of a tuned boss's quest item that a quest's show or give step turns
// into gear (the petition's own example, the Ring of Veeshan, is no gear: the
// Plane of Sky's armourers take it for their breastplates), so the quest can
// tell an Ascended one. A spell scroll (wear slot -1, ScrollArt) never is.
//
// A copy follows its base: at every start SyncAscendedCopies makes each copy
// its base again by the rule (a fixed look or figure on the base reaches its
// copy at the next start: fix the base, never a copy; a copy's race mask a GM
// widened past its base's is kept, petition 3235, and a look a GM gave the copy
// itself, the owner 2026-11-28). The bosses' loot points
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

// The look a GM gives an Ascended record is its own (the owner, 2026-11-28: Skywalker's icons and models for the
// Ascended weapons went back to their bases' at every start). basemodel and baseicon are the base's model and icon as
// the last start's sync left them: a copy whose model and icon are still those follows its base's look (a fixed look
// on the base reaches it); one whose look differs from them was given its own, which the sync keeps
// (ascendedOwnLookSQL), with its colour and unk2 (the icon's second word). NULL (never synced): the base's. The
// two columns are added at the start (ascendedLookMigrate), so the table's SCHEMA stays the tools' own.
var ascendedLookColumns = map[string]bool{"model": true, "itemicon": true, "color": true, "unk2": true}

// ascendedOwnLookSQL is true when the copy (row cp) was given a look of its own since the last start.
const ascendedOwnLookSQL = `EXISTS (SELECT 1 FROM itemascended a WHERE a.patternid = cp.patternid AND a.basemodel IS NOT NULL ` +
	`AND (CAST(cp."model" AS INTEGER), CAST(cp."itemicon" AS INTEGER)) IS NOT (a.basemodel, a.baseicon))`

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

// ascendedRaceKeptSQL is the race mask the start's sync (SyncAscendedCopies)
// gives an Ascended record, the copy being row cp and %[1]s its base's mask:
// the copy's own when it lets every race wear it that its base's does and
// more (Heirloom petition 3235: the owner's "just race all" set the eleven
// Ascended records of a narrower base to 1023 on 2026-11-26, and the next
// start's sync put the Ascended Breastplate of the Anointed back to its base's
// 1439, no ogre), else the base's. A mask is read as the server reads it
// (ascendedRaceReach): 0 is every race, and one naming the ten shipped races
// takes the added ones' bits too (handlers raceUseForWire).
const ascendedRaceKeptSQL = `CASE WHEN cp."raceuse" IS NOT NULL AND %[1]s IS NOT NULL AND ` +
	`(` + ascendedRaceReachCP + ` & ` + ascendedRaceReachBase + `) = ` + ascendedRaceReachBase + ` THEN cp."raceuse" ELSE %[1]s END`

// ascendedRaceReach* are ascendedRaceReach on the copy's mask and the base's.
const (
	ascendedRaceReachCP   = `(CASE WHEN CAST(cp."raceuse" AS INTEGER) = 0 THEN -1 WHEN (CAST(cp."raceuse" AS INTEGER) & 1023) = 1023 THEN (CAST(cp."raceuse" AS INTEGER) | -1024) ELSE CAST(cp."raceuse" AS INTEGER) END)`
	ascendedRaceReachBase = `(CASE WHEN CAST(%[1]s AS INTEGER) = 0 THEN -1 WHEN (CAST(%[1]s AS INTEGER) & 1023) = 1023 THEN (CAST(%[1]s AS INTEGER) | -1024) ELSE CAST(%[1]s AS INTEGER) END)`
)

// ascendedRaceReach is the races a record's race mask lets wear it, as bits:
// every one for 0, the ten shipped races' mask with every added race's bit
// (the mask a GM sets as "every race", 1023), else the mask itself.
func ascendedRaceReach(mask int64) int64 {
	switch {
	case mask == 0:
		return -1
	case mask&1023 == 1023:
		return mask | -1024
	}
	return mask
}

// AscendedKeepsRace reports whether the start's sync leaves an Ascended
// record's race mask as it is (ascendedRaceKeptSQL): it reaches every race its
// base's does.
func AscendedKeepsRace(copyMask, baseMask int64) bool {
	c, b := ascendedRaceReach(copyMask), ascendedRaceReach(baseMask)
	return c&b == b
}

// ascendedColumns are itempattern's columns but patternid, quoted (set), and
// what an Ascended record takes for each from its base (from): what a new
// one's are made of (AscendedCopyTx). ascendedSyncColumns are the start's.
func ascendedColumns(cols []string) (set, from string) { return ascendedColumnsFor(cols, false) }

// ascendedSyncColumns is ascendedColumns for the start's sync, the copy being
// row cp: a race mask wider than its base's kept (ascendedRaceKeptSQL).
func ascendedSyncColumns(cols []string) (set, from string) { return ascendedColumnsFor(cols, true) }

func ascendedColumnsFor(cols []string, sync bool) (set, from string) {
	var s, f []string
	have := map[string]bool{}
	for _, c := range cols {
		have[strings.ToLower(c)] = true
	}
	ownLook := have["model"] && have["itemicon"] // the rule reads both
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
		case low == "raceuse" && sync:
			f = append(f, fmt.Sprintf(ascendedRaceKeptSQL, q))
		case ascendedLookColumns[low] && sync && ownLook:
			f = append(f, `CASE WHEN `+ascendedOwnLookSQL+` THEN cp.`+q+` ELSE `+q+` END`)
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
// A look a GM gave the copy itself is kept (ascendedOwnLookSQL).
// Its race mask alone a GM may widen on the copy: one that lets every race
// wear it that its base's does, and more, is kept (ascendedRaceKeptSQL).
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
	if err := ascendedLookMigrate(tx); err != nil {
		return 0, err
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, err
	}
	set, from := ascendedSyncColumns(cols)
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
	// the two statements parsed once for the 751 pairs, not twice a pair (most of
	// this step's time at a start, 2026-11-19)
	// the copy is cp: a race mask wider than its base's is kept (ascendedRaceKeptSQL)
	upd, err := tx.Prepare(fmt.Sprintf(`UPDATE itempattern AS cp SET (%s) = (SELECT %s FROM itempattern WHERE patternid = ?)
			WHERE cp.patternid = ? AND (%s) IS NOT (SELECT %s FROM itempattern WHERE patternid = ?)`,
		set, from, set, from))
	if err != nil {
		return 0, err
	}
	defer upd.Close()
	del, delErr := tx.Prepare(`DELETE FROM itemeffect WHERE patternid = ?`)
	if delErr != nil && !strings.Contains(delErr.Error(), "no such table") {
		return 0, delErr
	}
	if del != nil {
		defer del.Close()
	}
	for _, p := range pairs {
		// only a record that differs from what its base makes is written
		res, err := upd.Exec(p[1], p[0], p[1])
		if err != nil {
			return 0, fmt.Errorf("Ascended record %d of record %d: %w", p[0], p[1], err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
		if del != nil {
			if _, err := del.Exec(p[0]); err != nil && !strings.Contains(err.Error(), "no such table") {
				return 0, err
			}
		}
		if err := copyItemEffects(tx, p[1], p[0], ascendedLine); err != nil {
			return 0, err
		}
	}
	if err := ascendedLookRemember(tx, cols); err != nil {
		return 0, err
	}
	return changed, tx.Commit()
}

// ascendedLookMigrate adds basemodel and baseicon to an itemascended made before them.
func ascendedLookMigrate(tx *sql.Tx) error {
	have, err := tableColumns(tx, "itemascended")
	if err != nil {
		return err
	}
	got := map[string]bool{}
	for _, c := range have {
		got[strings.ToLower(c)] = true
	}
	for _, c := range []string{"basemodel", "baseicon"} {
		if !got[c] {
			if _, err := tx.Exec(`ALTER TABLE itemascended ADD COLUMN ` + c + ` INTEGER`); err != nil {
				return err
			}
		}
	}
	return nil
}

// ascendedLookRemember records each base's model and icon as this start leaves them (ascendedOwnLookSQL's mark).
func ascendedLookRemember(tx *sql.Tx, cols []string) error {
	have := map[string]bool{}
	for _, c := range cols {
		have[strings.ToLower(c)] = true
	}
	if !have["model"] || !have["itemicon"] {
		return nil
	}
	_, err := tx.Exec(`UPDATE itemascended SET
		basemodel = (SELECT CAST(model AS INTEGER) FROM itempattern WHERE patternid = itemascended.base),
		baseicon = (SELECT CAST(itemicon AS INTEGER) FROM itempattern WHERE patternid = itemascended.base)`)
	return err
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

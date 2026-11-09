package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// imported.go — imported gear's own records: gear a character brought in
// through a character transfer is a copy of its record at ImportedShare of
// its stats (petition 1314, jibrish, 2026-11-09: "Any gear that was imported
// from the character transfer tool should be blanket reduced in stats by
// 30%. The real world server drop equivelents should be the full thing. This
// includes any special effects on the gear, like procs. Regular dropped items
// in game should remain unchanged."; the user: "you can do what jib says for
// the 30% imported gear damper, add Imported Tag to gear that was imported").
//
// A copy is an itempattern row of its own (ids from importedFirst), so it
// keeps its damper wherever the item goes (a trade, the bank, an auction, a
// merchant's buyback), and the record it was made from goes on as every drop
// and merchant has it. Table itemimported names each copy's base. The copy's
// name and description are its base's: the "(Imported)" a client is shown
// beside the name (ImportedTag) is put on as a record is sent, behind a
// switch (the user: "the imported add a note is only temporary for testing
// purposes"), so taking it away again changes no record. What the copy's
// figures do not hold, a proc or a worn item's hit effect read by the
// record's name, is scaled where it is read (entity: ImportedBase).
//
// The gear copied (IsImportedGear) is what is worn: a record with a wear slot
// that does not stack, not an Heirloom (the server's own, never imported).
// Its stats, AC, resists, hit point and power maximums and their ticks, and a
// weapon's damage, are each times ImportedShare, rounded half away from zero;
// everything else is its base's. Its use effects (itemeffect) are copied with
// each "by N" of their lines scaled the same way.

// ImportedShare is the share of its record's figures an imported copy keeps.
const ImportedShare = 0.7

// ImportedTag is what a client is shown after an imported copy's name.
const ImportedTag = " (Imported)"

// GMGrantSource is the from_server of a give a GM queues as an approved
// transfer (chartransfers; ~/charmigrate's grant scripts): its gear is given
// as it is, never as an import's copies.
const GMGrantSource = "GM grant"

// importedFirst and importedLast bound the copies' ids.
const (
	importedFirst = 990001
	importedLast  = 999999
)

// ImportedSchema is table itemimported: each copy (patternid), the record it
// was made from (base), when and by what.
const ImportedSchema = `CREATE TABLE IF NOT EXISTS itemimported (
	patternid INTEGER PRIMARY KEY,
	base      INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)`

// importedScaled are the itempattern columns a copy scales, in lower case.
var importedScaled = map[string]bool{
	"str": true, "sta": true, "agi": true, "wis": true, "dex": true, "cha": true, "intelligence": true,
	"hpmax": true, "powmax": true, "pot": true, "hot": true, "ac": true,
	"pr": true, "dr": true, "fr": true, "cr": true, "lr": true, "ar": true,
	"weapondamage": true,
}

// ImportedFigure is a figure as an imported copy has it.
func ImportedFigure(v int) int { return int(math.Round(float64(v) * ImportedShare)) }

// IsImportedGear reports whether a record is gear an import's damper copies:
// worn (a wear slot), not a stack, not an Heirloom.
func IsImportedGear(equipSlot, maxStack int, name string) bool {
	return equipSlot >= 0 && maxStack <= 1 && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "heirloom ")
}

// ImportedMap is every copy and its base (copy -> base).
func ImportedMap(db *sql.DB) (map[int]int, error) {
	if _, err := db.Exec(ImportedSchema); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT patternid, base FROM itemimported`)
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

var importedBy = regexp.MustCompile(`(?i)\bby (\d+)`)

// importedLine is an effect line with each "by N" scaled.
func importedLine(line string) string {
	return importedBy.ReplaceAllStringFunc(line, func(s string) string {
		n, err := strconv.Atoi(s[3:])
		if err != nil {
			return s
		}
		return s[:3] + strconv.Itoa(ImportedFigure(n))
	})
}

// SyncImportedCopies makes every imported copy its base again at
// ImportedShare (the server's start): a record a GM or a petition's fix
// changes (its look, its figures, its name) takes its copies with it, its
// use effects too. It says how many copies changed.
func SyncImportedCopies(db *sql.DB, now time.Time) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(ImportedSchema); err != nil {
		return 0, err
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, err
	}
	set, from := importedColumns(cols)
	pairs := make([][2]int, 0)
	rows, err := tx.Query(`SELECT i.patternid, i.base FROM itemimported i JOIN itempattern b ON b.patternid = i.base`)
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
		// only a copy that differs from what its base makes is written
		res, err := tx.Exec(fmt.Sprintf(`UPDATE itempattern SET (%s) = (SELECT %s FROM itempattern WHERE patternid = ?)
			WHERE patternid = ? AND (%s) IS NOT (SELECT %s FROM itempattern WHERE patternid = ?)`,
			set, from, set, from), p[1], p[0], p[1])
		if err != nil {
			return 0, fmt.Errorf("copy %d of record %d: %w", p[0], p[1], err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
		if _, err := tx.Exec(`DELETE FROM itemeffect WHERE patternid = ?`, p[0]); err != nil && !strings.Contains(err.Error(), "no such table") {
			return 0, err
		}
		if err := copyItemEffects(tx, p[1], p[0]); err != nil {
			return 0, err
		}
	}
	return changed, tx.Commit()
}

// importedColumns are itempattern's columns but patternid, quoted (set), and
// what a copy takes for each from its base (from): a scaled one at
// ImportedShare, the rest as they are.
func importedColumns(cols []string) (set, from string) {
	var s, f []string
	for _, c := range cols {
		if strings.EqualFold(c, "patternid") {
			continue
		}
		q := `"` + c + `"`
		s = append(s, q)
		if importedScaled[strings.ToLower(c)] {
			f = append(f, fmt.Sprintf("CASE WHEN %s IS NULL THEN NULL ELSE CAST(ROUND(%s * %g) AS INTEGER) END", q, q, ImportedShare))
		} else {
			f = append(f, q)
		}
	}
	return strings.Join(s, ", "), strings.Join(f, ", ")
}

// ImportedCopyTx is base's imported copy: the one made already, or one made
// now (made true) in tx, with source saying what made it. A base that is not
// gear (IsImportedGear), or a copy itself, is given back as it is.
func ImportedCopyTx(tx *sql.Tx, base int, source string, now time.Time) (id int, made bool, err error) {
	if _, err := tx.Exec(ImportedSchema); err != nil {
		return 0, false, err
	}
	if err := tx.QueryRow(`SELECT patternid FROM itemimported WHERE base = ?`, base).Scan(&id); err == nil {
		return id, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	var isCopy int
	if err := tx.QueryRow(`SELECT count(*) FROM itemimported WHERE patternid = ?`, base).Scan(&isCopy); err != nil {
		return 0, false, err
	}
	var equip, stack int
	var name string
	if err := tx.QueryRow(`SELECT equipslot, COALESCE(maxstack, 0), itemname FROM itempattern WHERE patternid = ?`, base).
		Scan(&equip, &stack, &name); err != nil {
		return 0, false, fmt.Errorf("record %d: %w", base, err)
	}
	if isCopy > 0 || !IsImportedGear(equip, stack, name) {
		return base, false, nil
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?`,
		importedFirst-1, importedFirst, importedLast).Scan(&id); err != nil {
		return 0, false, err
	}
	if id > importedLast {
		return 0, false, fmt.Errorf("no id left for an imported copy (%d-%d)", importedFirst, importedLast)
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, false, err
	}
	set, from := importedColumns(cols)
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO itempattern ("patternid", %s) SELECT %d, %s FROM itempattern WHERE patternid = ?`,
		set, id, from), base); err != nil {
		return 0, false, fmt.Errorf("copying record %d: %w", base, err)
	}
	if err := copyItemEffects(tx, base, id); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(`INSERT INTO itemimported (patternid, base, made, source) VALUES (?, ?, ?, ?)`,
		id, base, now.UTC().Format(time.RFC3339), source); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// copyItemEffects gives a copy its base's use effects (itemeffect), each line
// scaled; a database without the table has none.
func copyItemEffects(tx *sql.Tx, base, id int) error {
	cols, err := tableColumns(tx, "itemeffect")
	if err != nil || len(cols) == 0 {
		return err
	}
	rows, err := tx.Query(`SELECT * FROM itemeffect WHERE patternid = ?`, base)
	if err != nil {
		return err
	}
	var copies [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			rows.Close()
			return err
		}
		for i, c := range cols {
			switch strings.ToLower(c) {
			case "patternid":
				vals[i] = id
			case "line", "target_line":
				if s, ok := vals[i].(string); ok {
					vals[i] = importedLine(s)
				} else if b, ok := vals[i].([]byte); ok {
					vals[i] = importedLine(string(b))
				}
			}
		}
		copies = append(copies, vals)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
	}
	for _, vals := range copies {
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO itemeffect (%s) VALUES (%s)`, strings.Join(quoted, ", "),
			strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")), vals...); err != nil {
			return fmt.Errorf("copying record %d's use effect: %w", base, err)
		}
	}
	return nil
}

// tableColumns is a table's columns in order, none when it does not exist.
func tableColumns(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

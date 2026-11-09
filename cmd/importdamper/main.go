// Command importdamper puts the gear character transfers brought in on its
// imported copies (database/imported.go: each record copied once at 70% of
// its stats; petition 1314, jibrish: "Any gear that was imported from the
// character transfer tool should be blanket reduced in stats by 30%. The real
// world server drop equivelents should be the full thing."; the user: "you
// can do what jib says for the 30% imported gear damper").
//
// The rows are the ones charmigrate gave (charmigrate_log's items, not
// undone) and still held, wherever they are now: every transfer's, the form's
// (applied at a login) and those a GM ran by hand, but not a GM's grant
// (chartransfers.from_server "GM grant") and not a run named in -skip-run (a
// give that was no import). Of them only gear is moved
// (database.IsImportedGear: worn, not a stack, not an Heirloom); a row on a
// copy already, or on another record since, is left. Each move is kept in
// importdamper_log, so a second -apply moves only rows given since and -undo
// puts every row back on the record it was on and takes out the copies no
// row holds any more (one a transfer made at a login stays). A report by
// default; -apply and -undo write, in one transaction. Run it with the
// server held (a keep restart's before.sh) or stopped: the server reads the
// copies at its start.
//
//	importdamper -db eqoa.db [-apply | -undo] [-skip-run run1,run2]
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

const logSchema = `CREATE TABLE IF NOT EXISTS importdamper_log (
	id       INTEGER PRIMARY KEY,
	itemid   INTEGER NOT NULL,
	serverid INTEGER NOT NULL,
	run      TEXT NOT NULL,
	before   INTEGER NOT NULL,
	after    INTEGER NOT NULL,
	at       TEXT NOT NULL,
	undone   TEXT NOT NULL DEFAULT ''
)`

// source is what the copies this makes say made them.
const source = "importdamper (petition 1314)"

func main() {
	dbPath := flag.String("db", "eqoa.db", "the database")
	apply := flag.Bool("apply", false, "move the rows (else a report)")
	undo := flag.Bool("undo", false, "put every row this moved back")
	skip := flag.String("skip-run", "", "charmigrate runs whose items are no import, comma separated")
	flag.Parse()
	if *apply && *undo {
		log.Fatal("-apply or -undo, not both")
	}
	db, err := sql.Open("sqlite", *dbPath+"?"+database.BusyTimeoutDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	for _, s := range []string{logSchema, database.ImportedSchema} {
		if _, err := tx.Exec(s); err != nil {
			log.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if *undo {
		err = undoMoves(tx, now)
	} else {
		err = move(tx, skipRuns(*skip), now)
	}
	if err != nil {
		log.Fatal(err)
	}
	if !*apply && !*undo {
		fmt.Println("report only: nothing written")
		return
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("written")
}

func skipRuns(s string) map[string]bool {
	out := map[string]bool{}
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out[r] = true
		}
	}
	return out
}

type given struct {
	run    string
	itemID int64
}

func move(tx *sql.Tx, skip map[string]bool, now time.Time) error {
	grants := map[string]bool{}
	rows, err := tx.Query(`SELECT run FROM chartransfers WHERE from_server = ? AND run != ''`, database.GMGrantSource)
	if err == nil {
		for rows.Next() {
			var r string
			if err := rows.Scan(&r); err != nil {
				rows.Close()
				return err
			}
			grants[r] = true
		}
		rows.Close()
	}
	var items []given
	rows, err = tx.Query(`SELECT run, row_id FROM charmigrate_log WHERE kind = 'item' AND tbl = 'charinventory' AND undone = '' AND row_id > 0 ORDER BY id`)
	if err != nil {
		return fmt.Errorf("charmigrate_log: %w", err)
	}
	for rows.Next() {
		var g given
		if err := rows.Scan(&g.run, &g.itemID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, g)
	}
	rows.Close()
	moved := map[int64]bool{}
	rows, err = tx.Query(`SELECT itemid FROM importdamper_log WHERE undone = ''`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		moved[id] = true
	}
	rows.Close()

	counts := map[string]int{}
	perChar := map[string]int{}
	made := 0
	for _, g := range items {
		switch {
		case grants[g.run]:
			counts["a GM's grant"]++
			continue
		case skip[g.run]:
			counts["a run skipped"]++
			continue
		case moved[g.itemID]:
			counts["moved already"]++
			continue
		}
		var serverID uint32
		var pattern, equip, stack int
		var name, char string
		err := tx.QueryRow(`SELECT i.serverid, i.patternid, p.equipslot, COALESCE(p.maxstack, 0), p.itemname, COALESCE(c.charName, '')
			FROM charinventory i JOIN itempattern p ON p.patternid = i.patternid LEFT JOIN characters c ON c.serverid = i.serverid
			WHERE i.itemid = ?`, g.itemID).Scan(&serverID, &pattern, &equip, &stack, &name, &char)
		if err == sql.ErrNoRows {
			counts["no longer held"]++
			continue
		}
		if err != nil {
			return err
		}
		var isCopy int
		if err := tx.QueryRow(`SELECT count(*) FROM itemimported WHERE patternid = ?`, pattern).Scan(&isCopy); err != nil {
			return err
		}
		if isCopy > 0 {
			counts["on a copy already"]++
			continue
		}
		if !database.IsImportedGear(equip, stack, name) {
			counts["not gear"]++
			continue
		}
		copyID, isNew, err := database.ImportedCopyTx(tx, pattern, source, now)
		if err != nil {
			return err
		}
		if isNew {
			made++
		}
		if _, err := tx.Exec(`UPDATE charinventory SET patternid = ? WHERE itemid = ? AND patternid = ?`, copyID, g.itemID, pattern); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO importdamper_log (itemid, serverid, run, before, after, at) VALUES (?, ?, ?, ?, ?, ?)`,
			g.itemID, serverID, g.run, pattern, copyID, now.Format(time.RFC3339)); err != nil {
			return err
		}
		counts["moved"]++
		perChar[fmt.Sprintf("%s (%d)", char, serverID)]++
	}
	fmt.Printf("%d rows charmigrate gave; %d copies made\n", len(items), made)
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, counts[k])
	}
	names := make([]string, 0, len(perChar))
	for k := range perChar {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Printf("    %s: %d\n", k, perChar[k])
	}
	return audit(tx, "update", map[string]any{"moved": counts["moved"], "copies": made}, now)
}

func undoMoves(tx *sql.Tx, now time.Time) error {
	rows, err := tx.Query(`SELECT id, itemid, before, after FROM importdamper_log WHERE undone = '' ORDER BY id DESC`)
	if err != nil {
		return err
	}
	type mv struct {
		id, itemID    int64
		before, after int
	}
	var moves []mv
	for rows.Next() {
		var m mv
		if err := rows.Scan(&m.id, &m.itemID, &m.before, &m.after); err != nil {
			rows.Close()
			return err
		}
		moves = append(moves, m)
	}
	rows.Close()
	back, kept := 0, 0
	for _, m := range moves {
		res, err := tx.Exec(`UPDATE charinventory SET patternid = ? WHERE itemid = ? AND patternid = ?`, m.before, m.itemID, m.after)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			back++
		} else {
			kept++ // gone, or on another record since
		}
		if _, err := tx.Exec(`UPDATE importdamper_log SET undone = ? WHERE id = ?`, now.Format(time.RFC3339), m.id); err != nil {
			return err
		}
	}
	// the copies this made that no row holds now
	res, err := tx.Exec(`DELETE FROM itemeffect WHERE patternid IN (SELECT patternid FROM itemimported WHERE source = ?
		AND patternid NOT IN (SELECT patternid FROM charinventory))`, source)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	_ = res
	res, err = tx.Exec(`DELETE FROM itempattern WHERE patternid IN (SELECT patternid FROM itemimported WHERE source = ?
		AND patternid NOT IN (SELECT patternid FROM charinventory))`, source)
	if err != nil {
		return err
	}
	gone, _ := res.RowsAffected()
	if _, err := tx.Exec(`DELETE FROM itemimported WHERE source = ? AND patternid NOT IN (SELECT patternid FROM itempattern)`, source); err != nil {
		return err
	}
	fmt.Printf("undone: %d rows back on their records, %d left (gone or moved since); %d copies taken out\n", back, kept, gone)
	return audit(tx, "update", map[string]any{"undone": back, "copies_out": gone}, now)
}

// audit writes the run to gm_audit, when the table is there.
func audit(tx *sql.Tx, action string, what map[string]any, now time.Time) error {
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'gm_audit'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	b, _ := json.Marshal(what)
	_, err := tx.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, 1, 'dab', 'tool', '', ?, 'charinventory', ?, NULL, ?, ?)`,
		now.Format(time.RFC3339), action, `{"importdamper":"`+now.Format("20060102T150405Z")+`"}`, string(b),
		"imported gear on its 70% copies (petition 1314, the user's yes); cmd/importdamper")
	return err
}

func init() { log.SetFlags(0); log.SetOutput(os.Stderr) }

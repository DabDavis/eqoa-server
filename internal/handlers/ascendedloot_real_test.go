package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// TestRealAscendedLoot runs tools/ascend_boss_loot.py on a copy of a server
// database (EQOA_REAL_DB) for the bosses of its TUNED list that stand in it,
// and holds what it wrote to the server: the records it made are the
// server's own rule (its start's sync changes none of them); no tuned boss's
// loot tables hold regular gear any more; a second run writes nothing; and
// --undo leaves every table as it was. Run it on a copy of each shard before
// applying the tool there.
func TestRealAscendedLoot(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "real.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	// the server's own tables first (the server opens a database so), then
	// the bosses of the tool's TUNED list this database has
	db0, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	db0.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	for _, b := range []string{"Lord Nagafen", "Noroxus", "Bertoxxulous", "Uzrathixius", "Poxathixius", "Quintessence of Veeshan", "Yhafiran"} {
		var n int
		raw.QueryRow(`SELECT count(*) FROM npcloot WHERE npc_name = ?`, database.NPCClassKey(b)).Scan(&n)
		if n > 0 {
			args = append(args, "--boss", b)
		}
	}
	before := tableDigest(t, raw)
	raw.Close()
	if len(args) == 0 {
		t.Skip("no tuned boss has loot in this database")
	}
	report := ascendTool(t, path, append(args, "--apply")...)
	t.Logf("the tool:\n%s", report)

	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := database.SyncAscendedCopies(db.DB, time.Now()); err != nil || n != 0 {
		t.Errorf("the server's sync rewrote %d of the tool's records (%v): the tool's rule is not the server's", n, err)
	}
	m, err := database.AscendedMap(db.DB)
	if err != nil || len(m) == 0 {
		t.Fatalf("Ascended records: %v %v", m, err)
	}
	inv := database.NewInventoryStore(db)
	for i := 1; i < len(args); i += 2 {
		rolls, err := inv.NPCLootRolls(args[i], 65)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rolls {
			for _, c := range r.Choices {
				p := c.Item.Pattern
				if _, asc := m[p.PatternID]; !asc && database.IsAscendableGear(p.EquipSlot, p.MaxStack, p.AttackType, p.ItemName) {
					t.Errorf("%s's table %q still holds regular gear: %d %s", args[i], r.Table, p.PatternID, p.ItemName)
				}
			}
		}
	}
	db.Close()
	if again := ascendTool(t, path, append(args, "--apply")...); !strings.Contains(again, "nothing to do") {
		t.Errorf("a second run:\n%s", again)
	}
	undo := ascendTool(t, path, "--undo")
	if !strings.Contains(undo, " 0 left as they are") {
		t.Errorf("the undo left rows:\n%s", undo)
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	after := tableDigest(t, raw)
	for tbl, d := range before {
		if after[tbl] != d {
			t.Errorf("table %s after --undo is not as it was", tbl)
		}
	}
	for tbl := range after {
		if _, ok := before[tbl]; !ok {
			t.Errorf("table %s is new after --undo", tbl)
		}
	}
}

// tableDigest is every table's rows as text, by table: gm_audit (a history the
// tool and its undo add to) and the tool's log aside.
func tableDigest(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()
	out := map[string]string{}
	for _, tbl := range tables {
		if tbl == "gm_audit" || tbl == "ascended_loot_log" {
			continue
		}
		rs, err := db.Query(`SELECT * FROM "` + tbl + `" ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rs.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		var b strings.Builder
		for rs.Next() {
			if err := rs.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			if tbl == "sqlite_sequence" && fmt.Sprint(vals[0]) == "gm_audit" {
				continue
			}
			for _, v := range vals {
				if bs, ok := v.([]byte); ok {
					v = string(bs)
				}
				fmt.Fprintf(&b, "%v\x1f", v)
			}
			b.WriteByte('\n')
		}
		rs.Close()
		out[tbl] = b.String()
	}
	return out
}

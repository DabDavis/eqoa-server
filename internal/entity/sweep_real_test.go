package entity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The lines tools/sweep_spell_pages.py wrote on a real server's database
// (EQOA_REAL_DB names it, a copy of which is read), each read as the server
// reads it: a line of prose it replaced reads now (an effect, a summoning,
// an item summoned or a gate: the test lanes' report of 2026-10-29 found
// Pouncing Tiger's prose read as nothing, and so taken for a help), and a
// line it gave a time reads no less than before. It names the swept records
// that are harmful self spells, which land on their casters (a pact's, a
// proc's own lines). Skipped without a database, and on one the tool has not
// swept: none is in this repository.
func TestRealSweptLinesRead(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "swept.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var logged int
	if db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'spell_pages_sweep_log'`).Scan(&logged); logged == 0 {
		t.Skip("tools/sweep_spell_pages.py has not swept this database")
	}
	gates := map[string]bool{}
	if rows, err := db.Query(`SELECT spell FROM spellgates`); err == nil {
		for rows.Next() {
			var s string
			rows.Scan(&s)
			gates[s] = true
		}
		rows.Close()
	}
	// reads is how much of a line the server reads: its effects, and one
	// more for a summoning, a charm, an illusion, an item or a gate
	reads := func(name, line, desc string) int {
		d := ParseSpellNamed(name, line, desc)
		n := len(d.Effects)
		if d.Summons() || d.Charms() || d.FormKind != FormNone || gates[database.SpellGateKey(name)] {
			n++
		}
		for _, u := range d.Unread {
			if reSummonItem.MatchString(strings.TrimSpace(reParens.ReplaceAllString(u, ""))) && !reSummonCreature.MatchString(u) {
				n++
			}
		}
		return n
	}
	rows, err := db.Query(`SELECT l.rowkey, l.before, l.after, l.why, p.spellname, p.spelldesc FROM spell_pages_sweep_log l
		JOIN spellpattern p ON p.spellid = CAST(json_extract(l.rowkey, '$.spellid') AS INTEGER) WHERE l.tbl = 'spelleffect'`)
	if err != nil {
		t.Fatal(err)
	}
	type lineRow struct{ Line string }
	n := 0
	for rows.Next() {
		var key, before, after, why, name, desc string
		var b, a lineRow
		var bs *string
		if err := rows.Scan(&key, &bs, &after, &why, &name, &desc); err != nil {
			t.Fatal(err)
		}
		if bs != nil {
			before = *bs
			json.Unmarshal([]byte(before), &b)
		}
		json.Unmarshal([]byte(after), &a)
		n++
		was, now := reads(name, b.Line, desc), reads(name, a.Line, desc)
		switch {
		case strings.Contains(why, "prose") && now == 0:
			t.Errorf("%s %q: the page's line %q reads as nothing", key, name, a.Line)
		case now < was:
			t.Errorf("%s %q: %q reads less than %q did (%d, %d)", key, name, a.Line, b.Line, now, was)
		}
	}
	rows.Close()
	var selfHarm []string
	rs, err := db.Query(`SELECT p.spellname, p.spelldesc, e.line FROM spellpattern p JOIN spelleffect e ON e.spellid = p.spellid
		WHERE p.scope = 0 AND p.spellid IN (SELECT CAST(json_extract(rowkey, '$.spellid') AS INTEGER) FROM spell_pages_sweep_log)`)
	if err != nil {
		t.Fatal(err)
	}
	for rs.Next() {
		var name, desc, line string
		rs.Scan(&name, &desc, &line)
		if ParseSpellNamed(name, line, desc).Harmful {
			selfHarm = append(selfHarm, name+": "+line)
		}
	}
	rs.Close()
	t.Logf("%d swept lines read; swept harmful self spells (they land on their casters): %q", n, selfHarm)
}

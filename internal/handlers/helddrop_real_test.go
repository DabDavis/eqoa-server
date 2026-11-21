package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The held gear and drops over a server's database (EQOA_REAL_DB, copied): how
// many rows hold a weapon or a shield, which of them roll and which keep their
// kit, how the held looks map to records, with samples. A report: it fails
// only on a load's error or a pick outside its rules.
func TestRealHeldGearReport(t *testing.T) {
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
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := database.NewInventoryStore(db)

	w := entity.NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Logf("templates: %v", err)
	}
	if _, err := w.LoadSocialRules(db); err != nil {
		t.Logf("social: %v", err)
	}
	if _, err := w.LoadRareSpawns(db); err != nil {
		t.Logf("rares: %v", err)
	}
	rules := entity.DefaultHeldGear
	if rules.Profiles, err = inv.OwnLootProfiles(); err != nil {
		t.Fatal(err)
	}
	w.SetHeldGear(rules)
	c := w.ArmNPCs()
	t.Logf("spawn roll: %d rows' NPCs with a kit; %d roll (%d whole, %d bare this start); %d keep the whole kit; weapons %d carried %d not, shields %d carried %d not; %d loot profiles kept whole",
		c.Kits, c.Rolled, c.Whole, c.Bare, c.Kits-c.Rolled, c.Weapons[0], c.Weapons[1], c.Shields[0], c.Shields[1], len(rules.Profiles))

	d, err := inv.HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("records with a proc by name or description made rare: %d", d.MarkRare(heldHasProc))
	rep := HeldDropsReport(d, w.HeldKits())
	t.Logf("held looks with a record: %d have a plain record, %d rare records alone", rep.Plain, rep.RareOnly)
	t.Logf("records by look: %d (left out: %v)", d.Records, d.Left)
	t.Logf("held looks: %d, of which %d have a record; pieces held on the rows: %d, %d of a look with a record", rep.Models, rep.Mapped, rep.Rows, rep.RowsMapped)

	type un struct {
		look [2]uint32
		n    int
	}
	var uns []un
	for k, n := range rep.Unmapped {
		uns = append(uns, un{k, n})
	}
	sort.Slice(uns, func(i, j int) bool { return uns[i].n > uns[j].n })
	var lines []string
	for _, u := range uns {
		lines = append(lines, fmt.Sprintf("%08X shield=%v x%d", u.look[0], u.look[1] != 0, u.n))
	}
	t.Logf("looks with no record (%d): %s", len(uns), strings.Join(lines, "; "))

	// what the rows' creatures drop: a pick per row, by its name and level
	rows, err := db.Query(`SELECT npc_name, npc_level, primary_weapon, secondary_weapon, shield, npc_type FROM npcs
		WHERE primary_weapon <> 0 OR secondary_weapon <> 0 OR shield <> 0 ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type sample struct {
		name  string
		level int
		look  uint32
		item  string
		req   int
	}
	var picked, below, none int
	var monster [3]int // a monster's pieces (no NPC bit): drop a record, all above its level, no record
	samples := map[string]sample{}
	for rows.Next() {
		var name string
		var level int
		var p, s, h int64
		var npcType int
		if err := rows.Scan(&name, &level, &p, &s, &h, &npcType); err != nil {
			t.Fatal(err)
		}
		for _, piece := range []struct {
			m      int64
			shield bool
		}{{p, false}, {s, false}, {h, true}} {
			if piece.m == 0 {
				continue
			}
			look := uint32(piece.m)
			e, ok := d.Pick(look, piece.shield, level, name, false)
			switch {
			case ok:
				picked++
				if npcType&0x80 == 0 {
					monster[0]++
				}
				if e.Pattern.LevelReq > level+d.LevelAbove || e.Pattern.LevelReq < level-d.LevelBelow || uint32(e.Pattern.Model) != look || d.Rare(e.Pattern.PatternID) {
					t.Errorf("%s (%d) %08X: %s (req %d)", name, level, look, e.Pattern.ItemName, e.Pattern.LevelReq)
				}
				key := database.NPCClassKey(name)
				if _, seen := samples[key]; !seen && len(samples) < 40 {
					samples[key] = sample{name, level, look, e.Pattern.ItemName, e.Pattern.LevelReq}
				}
			case d.Has(look, piece.shield):
				below++
				if npcType&0x80 == 0 {
					monster[1]++
				}
			default:
				none++
				if npcType&0x80 == 0 {
					monster[2]++
				}
			}
		}
	}
	t.Logf("pieces held over the rows: %d drop a plain record (window -%d..+%d), %d of a look with records but no plain one in the window, %d of a look with no record",
		picked, d.LevelBelow, d.LevelAbove, below, none)
	t.Logf("of them a monster's (no NPC bit): %d drop a plain record, %d none in the window, %d no record", monster[0], monster[1], monster[2])
	keys := make([]string, 0, len(samples))
	for k := range samples {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := samples[k]
		t.Logf("  %s (level %d) holding %08X drops %q (level %d)", s.name, s.level, s.look, s.item, s.req)
	}
	heldRuleReport(t, db, d)
}

// heldRuleReport logs, over the monsters' held pieces (no NPC bit), the looks by how often
// held: the record each dropped before the 1 in 500 (the highest levelreq at or below the
// creature's level, any record) and what it drops now (plain, and on the rare roll), at the
// look's median level; and how many looks have a plain record within the window at their
// holders' levels.
func heldRuleReport(t *testing.T, db *database.SQLiteDB, d *database.HeldDrops) {
	rows, err := db.Query(`SELECT npc_level, primary_weapon, 0 FROM npcs WHERE primary_weapon <> 0 AND (npc_type & 128) = 0
		UNION ALL SELECT npc_level, secondary_weapon, 0 FROM npcs WHERE secondary_weapon <> 0 AND (npc_type & 128) = 0
		UNION ALL SELECT npc_level, shield, 1 FROM npcs WHERE shield <> 0 AND (npc_type & 128) = 0`)
	if err != nil {
		t.Fatal(err)
	}
	levels := map[[2]uint32][]int{}
	for rows.Next() {
		var level int
		var m int64
		var sh uint32
		if err := rows.Scan(&level, &m, &sh); err != nil {
			t.Fatal(err)
		}
		k := [2]uint32{uint32(m), sh}
		levels[k] = append(levels[k], level)
	}
	rows.Close()
	label := func(e database.CharInventoryEntry, ok bool) string {
		if !ok {
			return "-"
		}
		return fmt.Sprintf("%s (L%d)", e.Pattern.ItemName, e.Pattern.LevelReq)
	}
	below, above := d.LevelBelow, d.LevelAbove
	before := func(model uint32, shield bool, level int) (database.CharInventoryEntry, bool) {
		d.LevelBelow, d.LevelAbove = 1000, 0
		defer func() { d.LevelBelow, d.LevelAbove = below, above }()
		r, rok := d.Pick(model, shield, level, "x", true)
		if rok && !d.Rare(r.Pattern.PatternID) {
			rok = false // the plain fallback: the plain pick below has it
		}
		p, pok := d.Pick(model, shield, level, "x", false)
		switch {
		case !rok:
			return p, pok
		case !pok:
			return r, rok
		case r.Pattern.LevelReq > p.Pattern.LevelReq || (r.Pattern.LevelReq == p.Pattern.LevelReq && r.Pattern.PatternID < p.Pattern.PatternID):
			return r, true
		}
		return p, true
	}
	type look struct {
		k      [2]uint32
		n, med int
	}
	var looks []look
	var withRec, plainAny, plainMed, rareOnly, pieces, piecesPlain, piecesRareOnly, piecesBeforeRare int
	for k, ls := range levels {
		sort.Ints(ls)
		looks = append(looks, look{k, len(ls), ls[len(ls)/2]})
		sh := k[1] != 0
		if !d.Has(k[0], sh) {
			continue
		}
		withRec++
		if d.HasPlain(k[0], sh) {
			plainAny++
		} else {
			rareOnly++
		}
		if _, ok := d.Pick(k[0], sh, ls[len(ls)/2], "x", false); ok {
			plainMed++
		}
		for _, lv := range ls {
			pieces++
			if _, ok := d.Pick(k[0], sh, lv, "x", false); ok {
				piecesPlain++
			} else if !d.HasPlain(k[0], sh) {
				piecesRareOnly++
			}
			if e, ok := before(k[0], sh, lv); ok && d.Rare(e.Pattern.PatternID) {
				piecesBeforeRare++
			}
		}
	}
	t.Logf("monsters' held looks: %d, %d with a record: %d have a plain record (%d of them one in the window at the look's median holder level), %d rare records alone",
		len(levels), withRec, plainAny, plainMed, rareOnly)
	t.Logf("monsters' held pieces of a look with a record: %d; a plain record in the window at the holder's level: %d; of a look with rare records alone: %d; dropped a rare record before: %d",
		pieces, piecesPlain, piecesRareOnly, piecesBeforeRare)
	sort.Slice(looks, func(i, j int) bool { return looks[i].n > looks[j].n })
	for i, l := range looks {
		if i == 25 {
			break
		}
		sh := l.k[1] != 0
		b, bok := before(l.k[0], sh, l.med)
		p, pok := d.Pick(l.k[0], sh, l.med, "x", false)
		r, rok := d.Pick(l.k[0], sh, l.med, "x", true)
		t.Logf("  %08X shield=%v held %d, median L%d: before %s; now %s, 1 in %d %s", l.k[0], sh, l.n, l.med,
			label(b, bok), label(p, pok), d.RareOneIn, label(r, rok))
	}
}

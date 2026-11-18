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
	rep := HeldDropsReport(d, w.HeldKits())
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
			e, ok := d.Pick(look, piece.shield, level, name)
			switch {
			case ok:
				picked++
				if npcType&0x80 == 0 {
					monster[0]++
				}
				if e.Pattern.LevelReq > level || uint32(e.Pattern.Model) != look || e.Pattern.Trade != 0 || e.Pattern.Lore != 0 {
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
	t.Logf("pieces held over the rows: %d drop a record, %d of a look whose records are all above the creature's level, %d of a look with no record",
		picked, below, none)
	t.Logf("of them a monster's (no NPC bit): %d drop a record, %d all above its level, %d no record", monster[0], monster[1], monster[2])
	keys := make([]string, 0, len(samples))
	for k := range samples {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := samples[k]
		t.Logf("  %s (level %d) holding %08X drops %q (level %d)", s.name, s.level, s.look, s.item, s.req)
	}
}

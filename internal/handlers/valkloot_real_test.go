package handlers

import (
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// TestRealValkanithLoot runs tools/valkanith_loot_20261118.py on a copy of a server database (EQOA_REAL_DB) and
// holds what it wrote to the server: Lord Valkanith rolls his six (four Ascended, the two runes) and the waves
// the thirteen Thexian pieces; the server's start rewrites none of the Ascended records; the Rune of the
// Bonewing calls spell 760353 with 100 charges, whose pet sheet the World loads with no problem and whose buff
// reads as a 40 damage shield, a 20 heal at every hit taken, PR and DR 45, while the pet is out; the Rune of
// Defilement's spell is a power drain round its user; the mace procs on the undead alone; a second run writes
// nothing; --undo leaves every table as it was. Run it on a copy of each shard before applying the tool there.
func TestRealValkanithLoot(t *testing.T) {
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
	db0, err := database.NewSQLite(path) // the server's own tables first (itemspell), as a start makes them
	if err != nil {
		t.Fatal(err)
	}
	db0.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	before := tableDigest(t, raw)
	raw.Close()
	valkTool(t, path, "--apply", "--no-backup")
	if again := valkTool(t, path, "--apply", "--no-backup"); !strings.Contains(again, "nothing to do") {
		t.Errorf("a second run: %s", again)
	}

	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := database.SyncAscendedCopies(db.DB, time.Now()); err != nil || n != 0 {
		t.Errorf("the server's sync rewrote %d of the tool's Ascended records (%v)", n, err)
	}
	inv := database.NewInventoryStore(db)
	names := func(npc string, level int, table string, chance float64, rolls int) []string {
		t.Helper()
		rs, err := inv.NPCLootRolls(npc, level)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			if r.Table != table {
				continue
			}
			if r.Chance != chance || r.Rolls != rolls {
				t.Errorf("%s's %q: %v%% x %d", npc, table, r.Chance, r.Rolls)
			}
			var out []string
			for _, c := range r.Choices {
				out = append(out, c.Item.Pattern.ItemName)
			}
			sort.Strings(out)
			return out
		}
		t.Fatalf("%s rolls no %q (%+v)", npc, table, rs)
		return nil
	}
	got := strings.Join(names("Lord Valkanith", 60, "boss lord valkanith", 100, 1), "; ")
	if want := "Ascended Crown of Tal'thex; Ascended Positronian Mace; Ascended phylactery of valkanith; " +
		"Ascended positronian energy shield; Rune of Defilement; Rune of the Bonewing"; got != want {
		t.Errorf("his table: %s\nwant %s", got, want)
	}
	for _, kind := range []string{"Tal'thex knight", "Tal'thex soldier", "Tal'thex sorcerer"} {
		if n := names(kind, 57, "tal'thex armour", 10, 1); len(n) != 13 {
			t.Errorf("%s's armour: %d pieces %v", kind, len(n), n)
		}
	}
	id := func(name string) int {
		var pid int
		if err := db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? ORDER BY patternid LIMIT 1`, name).Scan(&pid); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return pid
	}
	rune := id("Rune of the Bonewing")
	is, ok, err := inv.ItemSpell(rune)
	if err != nil || !ok || is.SpellID != 760353 || inv.ItemCharges(rune) != 100 {
		t.Errorf("the rune: %+v %v %v, %d charges", is, ok, err, inv.ItemCharges(rune))
	}
	if is, ok, _ := inv.ItemSpell(id("Rune of Defilement")); !ok || is.SpellID != 760355 || is.Rest != time.Minute {
		t.Errorf("the Rune of Defilement: %+v", is)
	}

	w := entity.NewWorld()
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "760353") || strings.Contains(p, "Thexian") {
			t.Errorf("the pet sheet: %s", p)
		}
	}
	spells := database.NewSpellbookStore(db)
	spellOf := func(id int) entity.CastSpell {
		t.Helper()
		sp, err := spells.SpellPattern(id)
		if err != nil {
			t.Fatal(err)
		}
		row, _, err := spells.SpellEffectRow(id)
		if err != nil {
			t.Fatal(err)
		}
		return castSpellRow(&database.CharSpellEntry{Pattern: *sp}, row)
	}
	if s := spellOf(760353); !s.Def.Summons() {
		t.Errorf("760353 summons nothing: %+v", s.Def)
	}
	var shield, heal, pr, dr int
	for _, e := range spellOf(760354).Def.Effects {
		switch {
		case e.Kind == entity.EffectShield:
			shield = e.Amount
		case e.Kind == entity.EffectProcHeal && e.Defense && e.Chance == 100:
			heal = e.Amount
		case e.Kind == entity.EffectStat && e.Stat == entity.StatPR:
			pr = e.Amount
		case e.Kind == entity.EffectStat && e.Stat == entity.StatDR:
			dr = e.Amount
		}
	}
	if shield != 40 || heal != 20 || pr != 45 || dr != 45 {
		t.Errorf("the bonewing's buff: shield %d, heal %d, PR %d, DR %d", shield, heal, pr, dr)
	}
	d := spellOf(760355)
	drains := false
	for _, e := range d.Def.Effects {
		drains = drains || e.Kind == entity.EffectPower && e.Drain == entity.DrainCaster
	}
	if !d.Def.Harmful || d.Area.Centre != entity.AreaOnCaster || !drains {
		t.Errorf("Defilement: harmful %v, area %+v, drains %v (%+v)", d.Def.Harmful, d.Area, drains, d.Def.Effects)
	}
	var desc string
	db.QueryRow(`SELECT itemdesc FROM itempattern WHERE patternid = ?`, id("Positronian Mace")).Scan(&desc)
	if wp, ok := entity.WeaponProcFromText(desc); !ok || wp.Damage != 500 || wp.Only != "undead" {
		t.Errorf("the mace's proc: %+v %v", wp, ok)
	}
	db.Close()

	t.Logf("undo:\n%s", valkTool(t, path, "--undo", "--no-backup"))
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	after := tableDigest(t, raw)
	for tbl, h := range before {
		if after[tbl] != h {
			t.Errorf("table %s differs after --undo", tbl)
		}
	}
	for tbl := range after {
		if _, ok := before[tbl]; !ok && tbl != "valkanith_loot_1118_log" {
			t.Errorf("table %s left after --undo", tbl)
		}
	}
}

// valkTool runs tools/valkanith_loot_20261118.py on the database.
func valkTool(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-E", "-s", "../../tools/valkanith_loot_20261118.py", "--db", path}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the tool %v: %v\n%s", args, err, b)
	}
	return string(b)
}

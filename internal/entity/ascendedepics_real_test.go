package entity

import (
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// TestRealAscendedEpics runs tools/ascended_epics_20261111.py (petition 1629:
// the epic quests reward Ascended weapons and spells) on a copy of a server
// database (EQOA_REAL_DB) and holds what it wrote to the server:
//
//   - every Ascended spell does a tenth more than its base, as the server reads
//     the two (SpellDefFrom): each figure of each effect, its procs' and its
//     area's, its stat and level scales, is its base's a tenth more
//     (database.AscendedFigure on the size, the sign kept) or, where it is the
//     same, one the petition leaves (the caster's own price, a speed, a set
//     figure, a mitigation's percent); its duration, target, harm, stack and
//     level are its base's; what it makes is the Ascended copy of what its base
//     makes, and that copy's use or drink is a tenth more;
//   - its pet's row is its base's a tenth more, its abilities' spells Ascended;
//   - the Ascended records are the server's rule (its start's sync changes none);
//   - every epic quest's reward option names only Ascended gear and (the level
//     60 epics') Ascended spells, and the quests load;
//   - a second run writes nothing, and --undo takes away every table it made.
func TestRealAscendedEpics(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "real.db")
	copyRealDB(t, src, path)
	db0, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	db0.Close()
	tool, _ := filepath.Abs("../../tools/ascended_epics_20261111.py")
	run := func(args ...string) string {
		out, err := exec.Command("python3", append([]string{"-E", "-s", tool, "--db", path, "--no-backup"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("--apply")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	// the spells
	type spellRow struct{ name, desc, line, effects string }
	read := func(id int) spellRow {
		var r spellRow
		if err := raw.QueryRow(`SELECT p.spellname, COALESCE(p.spelldesc, ''), COALESCE(e.line, ''), COALESCE(e.effects, '')
			FROM spellpattern p LEFT JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellid = ?`, id).
			Scan(&r.name, &r.desc, &r.line, &r.effects); err != nil {
			t.Fatalf("spell %d: %v", id, err)
		}
		return r
	}
	rows, err := raw.Query(`SELECT spellid, base FROM spellascended ORDER BY base`)
	if err != nil {
		t.Fatal(err)
	}
	pairs := [][2]int{}
	for rows.Next() {
		var c, b int
		if err := rows.Scan(&c, &b); err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, [2]int{c, b})
	}
	rows.Close()
	if len(pairs) == 0 {
		t.Fatal("no Ascended spell made")
	}
	scaled := 0
	for _, pr := range pairs {
		a, b := read(pr[0]), read(pr[1])
		if a.name != database.AscendedName(b.name) {
			t.Errorf("spell %d is %q, not %q", pr[0], a.name, database.AscendedName(b.name))
		}
		da := SpellDefFrom(a.name, a.line, a.desc, a.effects)
		db := SpellDefFrom(b.name, b.line, b.desc, b.effects)
		who := a.name
		if da.Duration != db.Duration || da.Target != db.Target || da.Harmful != db.Harmful || da.Stack != db.Stack ||
			da.StackLevel != db.StackLevel || da.Workbook != db.Workbook || da.Pet != db.Pet {
			t.Errorf("%s: not its base's but in its figures: %+v against %+v", who, da, db)
			continue
		}
		n := compareEffects(t, who, da.Effects, db.Effects)
		if (da.Area == nil) != (db.Area == nil) {
			t.Errorf("%s: area %v, its base's %v", who, da.Area, db.Area)
		} else if da.Area != nil {
			n += compareEffects(t, who+" (area)", da.Area.Effects, db.Area.Effects)
		}
		if len(da.ProcSpells) != len(db.ProcSpells) {
			t.Errorf("%s: %d proc spells, its base %d", who, len(da.ProcSpells), len(db.ProcSpells))
		} else {
			for i := range da.ProcSpells {
				n += compareEffects(t, who+" (proc spell)", da.ProcSpells[i].Effects, db.ProcSpells[i].Effects)
			}
		}
		if len(da.Items) != len(db.Items) {
			t.Errorf("%s makes %v, its base %v", who, da.Items, db.Items)
		}
		for i := range da.Items {
			if da.Items[i].Name != database.AscendedName(db.Items[i].Name) || da.Items[i].Count != db.Items[i].Count {
				t.Errorf("%s makes %v, its base %v", who, da.Items[i], db.Items[i])
				continue
			}
			n += compareMade(t, raw, who, da.Items[i].Name, db.Items[i].Name)
		}
		if da.Summons() {
			n += comparePet(t, raw, who, pr[0], pr[1])
		}
		if n == 0 {
			t.Errorf("%s: no figure of it is a tenth more than its base's", who)
		}
		scaled += n
	}
	t.Logf("%d Ascended spells, %d figures a tenth more", len(pairs), scaled)

	// the records: the server's rule
	if n, err := database.SyncAscendedCopies(raw, time.Now()); err != nil || n != 0 {
		t.Errorf("the start's sync changed %d Ascended records (%v): the tool's are not the rule's", n, err)
	}

	// the quests: their options name only Ascended rewards, and they load
	asc := map[int]bool{}
	ascRows, _ := raw.Query(`SELECT patternid FROM itemascended`)
	for ascRows.Next() {
		var id int
		ascRows.Scan(&id)
		asc[id] = true
	}
	ascRows.Close()
	ascSpell := map[int]bool{}
	for _, pr := range pairs {
		ascSpell[pr[0]] = true
	}
	quests, err := database.NewQuestStore(realStore(t, path)).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	epics := 0
	for _, q := range quests {
		key := database.QuestImportKey(q.Source)
		epic60 := strings.HasPrefix(key, "60/") && strings.Contains(key, "/reward-")
		epic49 := strings.HasPrefix(key, "30,40,49/") && strings.Contains(key, "/L49-")
		if !epic60 && !(epic49 && strings.HasSuffix(q.Name, "Reward, Level 49")) {
			continue
		}
		epics++
		for _, st := range q.Steps {
			for _, o := range st.Choices.Options {
				for _, it := range o.Reward.Items {
					if !asc[it.PatternID] && isEpicGear(t, raw, it.PatternID) {
						t.Errorf("quest %d %q: %q gives regular gear %d", q.ID, q.Name, o.Answer, it.PatternID)
					}
				}
				for _, s := range o.Reward.Spells {
					if epic60 && !ascSpell[s] {
						t.Errorf("quest %d %q: %q teaches regular spell %d", q.ID, q.Name, o.Answer, s)
					}
					if epic49 && ascSpell[s] {
						t.Errorf("quest %d %q: %q teaches Ascended spell %d (a level 49 path spell)", q.ID, q.Name, o.Answer, s)
					}
				}
				if len(o.Answer) > 63 {
					t.Errorf("quest %d: answer %q is longer than CLIENT shows", q.ID, o.Answer)
				}
			}
			for _, it := range st.RewardItems {
				if !asc[it.PatternID] && isEpicGear(t, raw, it.PatternID) {
					t.Errorf("quest %d %q step %d gives regular gear %d", q.ID, q.Name, st.Step, it.PatternID)
				}
			}
		}
	}
	if epics == 0 {
		t.Error("no epic quest found")
	}

	// again, then back
	if out := run("--apply"); !strings.Contains(out, "nothing to do") {
		t.Errorf("a second run wrote something:\n%s", out)
	}
	run("--undo")
	var n int
	raw.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name IN ('spellascended', 'ascended_epics_log')`).Scan(&n)
	if n != 0 {
		t.Errorf("--undo left %d of its tables", n)
	}
}

func copyRealDB(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
}

func realStore(t *testing.T, path string) *database.SQLiteDB {
	t.Helper()
	d, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func isEpicGear(t *testing.T, raw *sql.DB, pid int) bool {
	var slot, stack, attack int
	var name string
	if err := raw.QueryRow(`SELECT equipslot, COALESCE(maxstack, 0), COALESCE(attacktype, 0), itemname FROM itempattern WHERE patternid = ?`, pid).
		Scan(&slot, &stack, &attack, &name); err != nil {
		return false
	}
	return database.IsAscendableGear(slot, stack, attack, name)
}

// tenthMore reports whether a is b a tenth more (the size, the sign kept).
func tenthMore(a, b int) bool {
	if b < 0 {
		return a == -database.AscendedFigure(-b)
	}
	return a == database.AscendedFigure(b)
}

// keptFigure reports whether an effect's figure the petition leaves may stay its base's.
func keptFigure(e SpellEffect, field string) bool {
	switch {
	case e.Self && e.Amount < 0 && field != "pct":
		return true // the caster's own price
	case e.Kind == EffectSpeed || e.Kind == EffectSnare || e.Kind == EffectRoot || e.Kind == EffectPowerCap || e.Set:
		return true
	case e.Kind == EffectMitigate && field == "amount":
		return true // its percent of a hit
	}
	return false
}

// compareEffects holds a's effects to b's a tenth more; it says how many figures were.
func compareEffects(t *testing.T, who string, a, b []SpellEffect) int {
	t.Helper()
	if len(a) != len(b) {
		t.Errorf("%s: %d effects, its base %d", who, len(a), len(b))
		return 0
	}
	n := 0
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.Stat != y.Stat || x.Self != y.Self || x.Scale != y.Scale || x.Resist != y.Resist {
			t.Errorf("%s: effect %d is %+v, its base's %+v", who, i, x, y)
			continue
		}
		for _, f := range []struct {
			name string
			a, b int
		}{{"amount", x.Amount, y.Amount}, {"most", x.AmountMax, y.AmountMax}, {"pct", x.ScalePct, y.ScalePct},
			{"level", x.LevelPct, y.LevelPct}} {
			switch {
			case f.b == 0 && f.a == 0:
			case tenthMore(f.a, f.b) && f.a != f.b:
				n++
			case f.a == f.b && (keptFigure(y, f.name) || tenthMore(f.a, f.b)):
			default:
				t.Errorf("%s: effect %d (kind %v) %s %d, its base's %d: not a tenth more", who, i, x.Kind, f.name, f.a, f.b)
			}
		}
	}
	return n
}

// compareMade holds an Ascended potion, wine or stone to its base: its HoT and
// PoT a tenth more, its use's figures (but a drinker's price) a tenth more.
func compareMade(t *testing.T, raw *sql.DB, who, name, base string) int {
	t.Helper()
	type it struct {
		id       int
		hot, pot sql.NullInt64
		line     string
	}
	get := func(n string) (r it) {
		// database.ItemPatternByName: of the name, no scroll, one with a use first
		if err := raw.QueryRow(`SELECT p.patternid, p.HoT, p.PoT, COALESCE(e.line, '') FROM itempattern p
			LEFT JOIN itemeffect e ON e.patternid = p.patternid
			WHERE lower(p.itemname) = lower(?) AND NOT (p.itemicon = ? AND p.equipslot = -1)
			ORDER BY e.patternid IS NULL, p.patternid LIMIT 1`, n, database.ScrollArt).Scan(&r.id, &r.hot, &r.pot, &r.line); err != nil {
			t.Errorf("%s: no item %q: %v", who, n, err)
		}
		return
	}
	a, b := get(name), get(base)
	n := 0
	for _, f := range [][2]sql.NullInt64{{a.hot, b.hot}, {a.pot, b.pot}} {
		if f[1].Valid && f[1].Int64 > 0 {
			if !tenthMore(int(f[0].Int64), int(f[1].Int64)) {
				t.Errorf("%s: %s's tick %d, its base's %d", who, name, f[0].Int64, f[1].Int64)
			}
			n++
		}
	}
	na, nb := lineNumbers(a.line), lineNumbers(b.line)
	if len(na) != len(nb) {
		t.Errorf("%s: %q against %q", who, a.line, b.line)
		return n
	}
	for i := range na {
		if na[i] != nb[i] {
			if !tenthMore(na[i], nb[i]) {
				t.Errorf("%s: %q against %q", who, a.line, b.line)
			}
			n++
		}
	}
	return n
}

// lineNumbers are a line's whole numbers in order.
func lineNumbers(s string) []int {
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		v, _ := strconv.Atoi(f)
		out = append(out, v)
	}
	return out
}

// comparePet holds an Ascended summon's pet to its base's: a tenth more, its
// abilities' spells Ascended copies of its base's.
func comparePet(t *testing.T, raw *sql.DB, who string, id, base int) int {
	t.Helper()
	cols := []string{"hp", "power", "ac", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "wpn1_dmg", "wpn2_dmg"}
	get := func(sid int) ([]int, string, bool) {
		vals := make([]int, len(cols))
		ptr := make([]any, 0, len(cols)+1)
		for i := range vals {
			ptr = append(ptr, &vals[i])
		}
		var ab string
		ptr = append(ptr, &ab)
		err := raw.QueryRow(`SELECT `+strings.Join(cols, ", ")+`, abilities FROM petstats WHERE spellid = ?`, sid).Scan(ptr...)
		return vals, ab, err == nil
	}
	a, aab, ok := get(id)
	b, bab, okb := get(base)
	if !okb {
		return 0
	}
	if !ok {
		t.Errorf("%s: no pet row, its base has one", who)
		return 0
	}
	n := 0
	for i := range cols {
		if b[i] > 0 {
			if !tenthMore(a[i], b[i]) {
				t.Errorf("%s: its pet's %s %d, its base's %d", who, cols[i], a[i], b[i])
			}
			n++
		}
	}
	var pa, pb []database.TemplateAbility
	json.Unmarshal([]byte(aab), &pa)
	json.Unmarshal([]byte(bab), &pb)
	if len(pa) != len(pb) {
		t.Errorf("%s: its pet's abilities %s, its base's %s", who, aab, bab)
		return n
	}
	for i := range pa {
		if pb[i].SpellID == 0 {
			continue
		}
		var b2 int
		if err := raw.QueryRow(`SELECT base FROM spellascended WHERE spellid = ?`, pa[i].SpellID).Scan(&b2); err != nil || b2 != pb[i].SpellID {
			t.Errorf("%s: its pet's ability %d casts %d, not an Ascended %d", who, i+1, pa[i].SpellID, pb[i].SpellID)
		}
	}
	return n
}

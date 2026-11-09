package handlers

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The Spell Archivist's paths on a real server's data (EQOA_REAL_DB names its
// database, a copy of which is opened): archivistPathLines against the path
// reward quests, class by class, and every class's paths written and changed
// through his boxes. Skipped without a database.
//
// For each class: every spell its 30, 40 and 49 reward quests' options give,
// and its level 20 class quests' picks (their scrolls' spells; petition 1361),
// is in exactly one path, or sold to the class by a merchant (a basic spell);
// every step's own spell is every path's or sold; each line's every name is
// a spell those quests give; no spell is in two paths; the paths number the
// lines. The table is logged as the GM reads it (petition 1325's report).
func TestRealArchivistPathsAreTheQuests(t *testing.T) {
	db := realRehabDB(t)
	inv := database.NewInventoryStore(db)
	qe, err := NewQuestEngine(db, nil, nil, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	arch, err := loadSpellArchive(database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	for class := 0; class <= database.MaxClassID; class++ {
		slug := database.TransferClassSlugs[class]
		paths, common := classPaths(qe, arch, class)
		lines := archivistPathLines[class]
		if len(paths) != len(lines) {
			t.Errorf("%s: %d paths, %d lines", slug, len(paths), len(lines))
		}
		where := map[int]int{} // spell -> path
		names := map[string]int{}
		for i, pa := range paths {
			for _, s := range pa.spells {
				if j, ok := where[s.id]; ok {
					t.Errorf("%s: %s (%d) in the paths of %s and %s", slug, s.name, s.id, paths[j].name, pa.name)
				}
				if j, ok := names[exactKey(s.name)]; ok && j != i {
					t.Errorf("%s: %s's name in the paths of %s and %s", slug, s.name, paths[j].name, pa.name)
				}
				where[s.id], names[exactKey(s.name)] = i, i
			}
		}
		isCommon := map[int]bool{}
		for _, s := range common {
			isCommon[s.id] = true
		}
		rewards := qe.pathRewards(class)
		levels := map[int]bool{}
		has := func(id int) bool { _, ok := arch.sources[id][class]; return ok }
		for _, r := range rewards {
			levels[r.level] = true
			for _, id := range arch.optionSpells(r, has) {
				name := arch.names[id].Name
				if _, ok := where[id]; !ok && !arch.soldTo(class, name) {
					t.Errorf("%s: the level %d option %q (%d) is in no path and no merchant sells it", slug, r.level, name, id)
				}
			}
			for _, id := range r.own {
				if name := arch.names[id].Name; !isCommon[id] && !arch.soldTo(class, name) {
					t.Errorf("%s: the level %d step's own %q (%d) is not every path's", slug, r.level, name, id)
				}
			}
		}
		if len(rewards) == 0 || !levels[20] || !levels[30] || !levels[40] || !levels[49] {
			t.Errorf("%s: %d reward quests, levels %v", slug, len(rewards), levels)
		}
		for _, line := range lines {
			for _, name := range line {
				if _, ok := names[exactKey(name)]; !ok {
					t.Errorf("%s: the line of %s names %q, which no reward quest gives", slug, line[0], name)
				}
			}
		}
		var parts []string
		for _, pa := range paths {
			var sp []string
			for _, s := range pa.spells {
				sp = append(sp, fmt.Sprintf("%s %d (%d)", s.name, s.level, s.id))
			}
			parts = append(parts, fmt.Sprintf("[%s: %s]", pa.name, strings.Join(sp, ", ")))
		}
		var all []string
		for _, s := range common {
			all = append(all, fmt.Sprintf("%s %d (%d)", s.name, s.level, s.id))
		}
		t.Logf("%s, %d reward quests: %s; every path: %v", slug, len(rewards), strings.Join(parts, " "), all)
	}
}

// Every class at level 60, a book of Return Home alone, through his boxes:
// its first path written at once, then each path in turn, each change asked
// and agreed, the book after each holding Return Home, the path's spells and
// the common ones, nothing of another path; the enchanter's (the petition's
// example) by name, its level 20 picks with them (petition 1361) and the Boon
// path's 49s (petition 1366: Fujiko's book too). Then a level 35 enchanter:
// its 20's and 30's alone; a 20: its 20's; a 19: nothing.
func TestRealArchivistChangesEveryClassesPaths(t *testing.T) {
	db := realRehabDB(t)
	f := &pathRig{archivistFixture: newArchivistFixture(t, db)}
	qe, err := NewQuestEngine(db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	f.qe = qe
	f.a.quests = func() *QuestEngine { return qe }
	arch, err := loadSpellArchive(f.spells)
	if err != nil {
		t.Fatal(err)
	}
	reset := func(class, level int) {
		t.Helper()
		if _, err := db.Exec(`DELETE FROM spells WHERE serverid = ?`, f.p.CharServerID); err != nil {
			t.Fatal(err)
		}
		f.p.Class, f.p.Level = class, level
		f.know(spReturnHome, 0, 0)
	}
	ids := func() []int {
		var out []int
		for _, r := range f.book() {
			var id, index int
			fmt.Sscanf(r, "%d@%d", &id, &index)
			out = append(out, id)
		}
		sort.Ints(out)
		return out
	}
	for class := 0; class <= database.MaxClassID; class++ {
		slug := database.TransferClassSlugs[class]
		paths, common := classPaths(qe, arch, class)
		reset(class, 60)
		var answers []string
		for _, pa := range paths {
			answers = append(answers, fmt.Sprintf(archivistPathAnswer, pa.name))
		}
		answers = append(answers, archivistPathNeither)
		for n := 0; n <= len(paths); n++ { // each in turn, then the first again
			k := n % len(paths)
			_, box := f.paths(answers...)
			got := f.pick(box, byte(k))
			if len(got) > 0 && strings.HasPrefix(got[len(got)-1], "ask#") {
				got = f.pick(boxID(got), 0) // the change agreed
			}
			want := []int{spReturnHome}
			for _, s := range append(paths[k].spells, common...) {
				want = append(want, s.id)
			}
			sort.Ints(want)
			if have := ids(); fmt.Sprint(have) != fmt.Sprint(want) {
				t.Errorf("%s, the path of %s: the book %v, want %v (said %q)", slug, paths[k].name, have, want, got)
			}
		}
	}

	reset(12, 60)
	enc := []string{"The path of Expansive Mind", "The path of Power Jet", archivistPathNeither}
	_, box := f.paths(enc...)
	f.wantMsgs("self power", f.pick(box, 0), "add:700215@1 flag 0", "add:700328@2 flag 0", "add:700462@3 flag 0", "add:700575@4 flag 0",
		"add:700618@5 flag 0",
		"box:Spell Archivist: There: Spacious Mind, Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind written into your book. The path of Expansive Mind is yours.")
	_, box = f.paths(enc...)
	got := f.pick(box, 1)
	f.wantMsgs("power for others", f.pick(boxID(got), 0), "remove:1", "remove:2", "remove:3", "remove:4", "remove:5",
		"add:700207@1 flag 0", "add:700340@2 flag 0", "add:700468@3 flag 0", "add:701032@4 flag 0", "add:701031@5 flag 0",
		"box:*It is done: the path of Power Jet is yours now.*", // past a box's 217 with the 49s: cut between sentences
		"box:Into it: Power Boon, Power Jet, Power Surge, Power Stream, Power Flux.")
	_, box = f.paths(enc...)
	got = f.pick(box, 0)
	f.wantMsgs("self power again", f.pick(boxID(got), 0), "remove:1", "remove:2", "remove:3", "remove:4", "remove:5",
		"add:700215@1 flag 0", "add:700328@2 flag 0", "add:700462@3 flag 0", "add:700575@4 flag 0", "add:700618@5 flag 0",
		"box:*It is done: the path of Expansive Mind*", "box:Into it: Spacious Mind, Expansive Mind, Limitless Mind, Boundless Mind, Infinite Mind.")

	// petition 1366, on a database petfix166_20261109.py has made right (the
	// level 49 reward offering the Boon path's Power Stream and Power Flux):
	// Fujiko's book, his path of Power Jet as he wrote it her before them,
	// Power Jet and Power Surge; at 60 the paths said to their 49s, and hers
	// written in at once, what she lacks of it
	reset(12, 60)
	f.know(700340, 1, -1)
	f.know(700468, 2, -1)
	got, box = f.paths(enc...)
	f.wantMsgs("Fujiko's paths", got,
		"box:Spell Archivist: The path of Expansive Mind: Spacious Mind, Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind. The path of Power Jet: Power Boon, Power Jet, Power Surge, Power Stream and Power Flux.",
		"box:Spell Archivist: Your book holds the path of Power Jet. "+archivistPathOne, "ask#*:"+archivistPathAsk+"|"+strings.Join(enc, "|"))
	f.wantMsgs("Fujiko's path of Power Jet", f.pick(box, 1), "add:700207@3 flag 0", "add:701032@4 flag 0", "add:701031@5 flag 0",
		"box:Spell Archivist: There: Power Boon, Power Stream and Power Flux written into your book. The path of Power Jet is yours.")

	// petition 1361: from level 20, the level 20 picks alone; below it nothing
	reset(12, 35)
	got, _ = f.paths(enc...)
	f.wantMsgs("a level 35 enchanter's paths", got,
		"box:Spell Archivist: The path of Expansive Mind: Spacious Mind and Expansive Mind. The path of Power Jet: Power Boon and Power Jet.",
		"ask#*:"+archivistPathAsk+"|"+strings.Join(enc, "|"))
	reset(12, 20)
	got, _ = f.paths(enc...)
	f.wantMsgs("a level 20 enchanter's paths", got,
		"box:Spell Archivist: The path of Expansive Mind: Spacious Mind. The path of Power Jet: Power Boon.", "ask#*:"+archivistPathAsk+"|"+strings.Join(enc, "|"))
	reset(12, 19)
	f.wantMsgs("a level 19 enchanter", f.pick(f.menu(), archivistPath), "box:Spell Archivist: "+fmt.Sprintf(archivistPathsTooLow, 20))
}

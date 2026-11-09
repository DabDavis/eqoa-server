package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The quests' side spells in the basic grant (petition 1361) on a real
// server's data (EQOA_REAL_DB names its database, a copy of which is opened),
// every class open to every race as the Heirloom Test Server runs: each
// class's side spells at level 60 and what they add to the merchants' grant,
// logged as the GM reads them; the counts are a separate Python reading of the
// same rule on the Heirloom server's data of 2026-11-09 (side_check.py beside
// the petition's report). For every class: no side spell is a path's (the
// level 20 picks and the 30/40/49 options, by record or name), a were's, a
// mastery's, an epic's or a master class's; the merchants' grant is in the
// combined grant whole; and the combined grant is ordered by level. By name,
// the quests' Kick, Taunt or Crawling Skin are granted, the level 20 picks
// (Spacious Mind, Field Dress) are not (the path rule) and neither is anything
// the merchants' grant leaves out (Tranquility, Djinni Bolt); and through his
// box nothing comes out of a book holding both paths' spells. Skipped
// without a database.
func TestRealArchivistSideSpells(t *testing.T) {
	db := realRehabDB(t)
	was := entity.Creation()
	defer entity.SetCreationRules(was)
	entity.SetCreationRules(entity.CreationRules{Races: true, AllClasses: true})
	qe, err := NewQuestEngine(db, nil, nil, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}
	arch, err := loadSpellArchive(database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	names := [15]string{"WAR", "RAN", "PAL", "SK", "MNK", "BRD", "ROG", "DRU", "SHM", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"}
	// the side spells at 60 by the separate reading (names, one each)
	want := [15]int{15, 14, 15, 17, 15, 16, 13, 17, 16, 14, 15, 16, 15, 17, 17}
	for class := 0; class <= database.MaxClassID; class++ {
		side := sideSpells(qe, arch, class, 60)
		paths, _ := classPaths(qe, arch, class)
		inPath := map[string]bool{}
		for _, pa := range paths {
			for _, s := range pa.spells {
				inPath[exactKey(s.name)] = true
			}
		}
		var list []string
		for _, s := range side {
			if inPath[exactKey(s.Name)] {
				t.Errorf("%s: %s (%d) is a path's", names[class], s.Name, s.SpellID)
			}
			if database.IsWereAbility(s.Name, arch.descs[s.SpellID]) || !grantedTo(arch.sources[s.SpellID], class) {
				t.Errorf("%s: %s (%d) is a were's, a mastery's, an epic's or a master class's", names[class], s.Name, s.SpellID)
			}
			list = append(list, fmt.Sprintf("%s %d", s.Name, s.Level))
		}
		if len(side) != want[class] {
			t.Errorf("%s: %d side spells at 60, want %d", names[class], len(side), want[class])
		}
		merchants := arch.basic(class, 0, 60)
		all := basicSpells(qe, arch, class, 0, 60)
		if got := spellsMinus(merchants, all); len(got) != 0 {
			t.Errorf("%s: the merchants' %v are not in the combined grant", names[class], got)
		}
		var added []string
		for _, s := range spellsMinus(all, merchants) {
			added = append(added, fmt.Sprintf("%s %d", s.Name, s.Level))
		}
		for i := 1; i < len(all); i++ {
			if all[i].Level < all[i-1].Level {
				t.Errorf("%s: the grant is out of level order at %s", names[class], all[i].Name)
			}
		}
		t.Logf("%s: %d side spells (%s); the grant at 60 %d = the merchants' %d + %d (%s)", names[class], len(side),
			strings.Join(list, ", "), len(all), len(merchants), len(all)-len(merchants), strings.Join(added, ", "))
	}

	// by name: the class quests' and the ability quests' in, the paths' level
	// 20 picks out (the path rule), and what the merchants' grant leaves out
	// left out still
	in := map[int][]string{
		0:  {"Kick", "Taunt", "Rapid Strike", "Stomp", "Reckless Force", "Frantic Defense"},
		12: {"Crawling Skin", "Heavy Arms", "Alarming Visage", "Power Flow", "Ghastly Visage"},
		13: {"Shock of Frost", "Nerius Portal", "Highpass Evacuation"},
	}
	out := map[int][]string{
		0:  {"Bellow", "Pillar of Might", "Roar", "Tranquility", "Dropkick"},
		4:  {"Freezing Strike", "Stone Stance", "Stasis Strike"},
		6:  {"Silent Blade Burglar", "Virulent Blade Assassin", "Remove Mask", "Avoidance", "Minor Wound"},
		9:  {"Field Dress", "Disease Ward", "Elemental Ward", "First Aid"},
		10: {"Djinni Bolt", "Summon Djinn", "Frozen Mark", "Lava Stone"},
		12: {"Spacious Mind", "Power Boon", "Expansive Mind", "Power Jet"},
	}
	for class, spells := range in {
		got := map[string]bool{}
		for _, s := range basicSpells(qe, arch, class, 0, 60) {
			got[exactKey(s.Name)] = true
		}
		for _, n := range spells {
			if !got[exactKey(n)] {
				t.Errorf("%s: %s is not granted", names[class], n)
			}
		}
	}
	for class, spells := range out {
		for _, s := range basicSpells(qe, arch, class, 0, 60) {
			for _, no := range spells {
				if exactKey(s.Name) == exactKey(no) {
					t.Errorf("%s is granted %s (%d)", names[class], s.Name, s.SpellID)
				}
			}
		}
	}

	// through his box: a level 60 enchanter whose book holds Return Home and
	// both paths' level 20 and 30 spells (as #template leaves one) is granted
	// the merchants' and the quests' spells it lacks, and nothing comes out
	f := newArchivistFixture(t, db)
	f.a.quests = func() *QuestEngine { return qe }
	f.p.Level, f.p.Class = 60, 12
	f.know(spReturnHome, 0, 0)
	for i, id := range []int{700215, 700207, 700328, 700340} {
		f.know(id, 1+i, -1)
	}
	before := f.book()
	all := basicSpells(qe, arch, 12, 0, 60)
	var wantAdds int
	for _, s := range all {
		switch s.SpellID {
		case 700215, 700207, 700328, 700340:
		default:
			if !database.IsReturnHome(s.Name) {
				wantAdds++
			}
		}
	}
	got := f.grant()
	adds := 0
	for _, m := range got {
		switch {
		case strings.HasPrefix(m, "add:"):
			adds++
		case strings.HasPrefix(m, "remove:"):
			t.Errorf("the grant took something out: %q", m)
		}
	}
	if adds != wantAdds || got[len(got)-1] != fmt.Sprintf("box:Spell Archivist: "+archivistGranted, spellCount(wantAdds), 60) {
		t.Errorf("the enchanter's grant: %d added, the last %q; want %d", adds, got[len(got)-1], wantAdds)
	}
	if b := f.book(); strings.Join(b[:len(before)], " ") != strings.Join(before, " ") {
		t.Errorf("the book's rows before the grant: %v, were %v", b[:len(before)], before)
	}
	f.wantMsgs("the enchanter's second grant", f.grant(), "box:Spell Archivist: "+fmt.Sprintf(archivistHasAll, 60))
}

// spellsMinus is the spells of a that b holds neither by record nor by exact
// name (lacking's rule over two lists).
func spellsMinus(a, b []database.SpellName) []database.SpellName {
	ids, names := map[int]bool{}, map[string]bool{}
	for _, s := range b {
		ids[s.SpellID], names[exactKey(s.Name)] = true, true
	}
	var out []database.SpellName
	for _, s := range a {
		if !ids[s.SpellID] && !names[exactKey(s.Name)] {
			out = append(out, s)
		}
	}
	return out
}

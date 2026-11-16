package handlers

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// On a real database (EQOA_REAL_DB, a copy): every Master Class of every
// class (a human's, and a gnome's for Tinker) at 60, what its ranks put in
// the merc's book and what the merc casts each for (MercUseName: "" never).
// A gnome warrior summons Invention from 6 and Tinker's Construct at 60 (not
// its Rat or Drachnid look), a gnome magician its elementals until Tinker,
// then the Construct; a Geomancer magician its Badi Elder.
func TestRealMercMasterClasses(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gw := entity.NewWorld()
	if _, err := gw.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	m := NewMercs(gw, db, NewHeirlooms(db))
	kit := func(class, race, level int, master, pick string) entity.MercKit {
		t.Helper()
		k, err := m.kit(database.MercRow{Name: "Real", Class: class, Race: race, Path: -1, Master: master, Pet: pick}, level)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	petOf := func(k entity.MercKit) string {
		if s := entity.MercPetFor(k); s != nil {
			return s.Name
		}
		return ""
	}
	unused := map[string]bool{}
	for class := 0; class <= 14; class++ {
		for _, race := range []int{0, 3} {
			for _, mc := range gw.MercMasterClasses(class, race) {
				if race == 3 && mc.Kind != entity.MercMasterOfRace {
					continue
				}
				plain := kit(class, race, 60, "", "")
				k := kit(class, race, 60, mc.Name, "")
				if k.Master != mc.Name || len(k.MasterRanks) == 0 {
					t.Errorf("%s %s: master %q, %d ranks", entity.ClassName(class), mc.Name, k.Master, len(k.MasterRanks))
				}
				had := map[int]bool{}
				for _, s := range plain.Spells {
					had[s.SpellID] = true
				}
				var got []string
				for i := range k.Spells {
					s := &k.Spells[i]
					if had[s.SpellID] {
						continue
					}
					use := entity.MercUseName(s)
					if use == "" {
						use = entity.MercPetUseName(s)
					}
					if s.MercSplash {
						use += ", splash"
					}
					if slices.Contains(k.MasterPets, s.SpellID) {
						use = "pet"
					}
					if use == "" || use == ", splash" {
						unused[mc.Name+": "+s.Name] = true
						use = "-"
					}
					got = append(got, s.Name+" ("+use+")")
				}
				t.Logf("%-13s %-6s %-12s %-20s +%s; pet %q", entity.ClassName(class), entity.RaceName(race), mc.Kind, mc.Name,
					strings.Join(got, ", "), petOf(k))
			}
		}
	}
	for n := range unused {
		t.Logf("never cast: %s", n)
	}
	for _, c := range []struct {
		class, race, level int
		master, pick, want string
	}{
		{0, 3, 5, "", "", ""}, {0, 3, 6, "", "", "Invention"}, {0, 3, 60, "", "", "Invention"},
		{0, 3, 60, "Tinker", "", "Construct"}, {0, 3, 57, "Tinker", "", "Invention"},
		{10, 3, 10, "", "", "Fire Elementaling"}, {10, 3, 60, "Tinker", "", "Construct"},
		{10, 3, 60, "Tinker", "air", "Summon Djinni Vizier (60)"}, {10, 0, 60, "Geomancer", "", "Summon Badi Elder"},
		{10, 0, 60, "Aeromancer", "", "Summon Djinni Vizier (60)"}, {3, 0, 60, "Death Knight", "", "Summon Servant"},
		{0, 0, 60, "Tinker", "", ""}, // not a gnome: no Tinker
	} {
		if got := petOf(kit(c.class, c.race, c.level, c.master, c.pick)); got != c.want {
			t.Errorf("%s %s %d %q %q: pet %q, want %q", entity.ClassName(c.class), entity.RaceName(c.race), c.level, c.master, c.pick, got, c.want)
		}
	}
	// Tinker's ranks at 60: its first, Construct and Repair (the pet's heal),
	// not the Rat or the Drachnid
	k := kit(0, 3, 60, "Tinker", "")
	var names []string
	for _, r := range k.MasterRanks {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "Tinker,Construct,Repair" {
		t.Errorf("Tinker's ranks: %v", names)
	}
	if !slices.ContainsFunc(k.Spells, func(s entity.CastSpell) bool { return s.Name == "Repair" }) {
		t.Errorf("no Repair in the book")
	}
}

// Petition 2195 on a real database (EQOA_REAL_DB, a copy): an alchemist of
// every Master Class it may take (a human's, a gnome's and a barbarian's
// racial ones), its ten slots set as Lonewulff's Cardor's were (buffs, a
// potion spell, a nuke: no refinement, which the window never lists), keeps
// the same stacks as one with no Master Class and no slots: its
// single-target potion, its cocktail (a Cocktail alchemist) and its draught.
func TestRealMercAlchemistMasterClassesBrew(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gw := entity.NewWorld()
	if _, err := gw.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	eh := NewEquipHandler(nil, database.NewInventoryStore(db), nil, nil)
	eh.gameWorld = gw
	eh.LoadPotions()
	m := NewMercs(gw, db, NewHeirlooms(db))
	slots := []int{700730, 900782, 900127, 700606, 700484, 700792, 900124, 0, 0, 0}
	tried := 0
	for _, race := range []int{0, 3, 6} {
		for _, path := range []int{0, 1} {
			plain, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: path}, 60)
			if err != nil {
				t.Fatal(err)
			}
			want := gw.MercBrewsFor(plain)
			if len(want) < 2 {
				t.Fatalf("race %d path %d: a plain alchemist keeps %v", race, path, want)
			}
			for _, mc := range gw.MercMasterClasses(14, race) {
				k, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: path, Master: mc.Name, SpellSlots: slots}, 60)
				if err != nil {
					t.Fatal(err)
				}
				if k.Master != mc.Name {
					t.Errorf("%s: master %q", mc.Name, k.Master)
				}
				want := want
				want = append(slices.Clone(want), realKeepsakes[mc.Name]...) // its keepsakes, after them (mercpill_test.go)
				if got := gw.MercBrewsFor(k); !slices.Equal(got, want) {
					t.Errorf("race %d path %d %s with its slots set keeps %v, want %v", race, path, mc.Name, got, want)
				}
				tried++
			}
		}
	}
	if tried < 6*5 {
		t.Errorf("only %d Master Class kits tried", tried)
	}
}

// Petition 2216 on a real database (EQOA_REAL_DB, a copy): an alchemist's
// Master Class mixtures are used (entity's mercselfpotion.go), its ten slots
// set as Felthious's Ornar's were: a Transmuter takes its Transformative Pill
// and throws its Transmutive Potion, a Metabolist drinks its Miraculous
// Draught; with no Master Class none of them.
func TestRealMercAlchemistMasterMixtures(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gw := entity.NewWorld()
	if _, err := gw.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	eh := NewEquipHandler(nil, database.NewInventoryStore(db), nil, nil)
	eh.gameWorld = gw
	eh.LoadPotions()
	m := NewMercs(gw, db, NewHeirlooms(db))
	slots := []int{700606, 700792, 700719, 700773, 900127, 900782, 700730, 700484, 0, 0}
	const pill, transmutive, miraculous = 709301, 50106, 1203
	for _, race := range []int{0, 3, 6} {
		for _, p := range []int{0, 1} {
			for _, master := range []string{"", "Transmuter", "Metabolist", "Sage"} {
				k, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: p, Master: master, SpellSlots: slots}, 60)
				if err != nil {
					t.Fatal(err)
				}
				buffs, heals, debuffs := gw.MercPotionUsesFor(k)
				t.Logf("race %d path %d %-10q buffs %v heals %v debuffs %v", race, p, master, buffs, heals, debuffs)
				has := func(list []int, id int) bool { return slices.Contains(list, id) }
				switch master {
				case "Transmuter":
					if !has(buffs, pill) || !has(debuffs, transmutive) {
						t.Errorf("race %d path %d Transmuter: buffs %v debuffs %v", race, p, buffs, debuffs)
					}
				case "Metabolist":
					if !has(heals, miraculous) {
						t.Errorf("race %d path %d Metabolist: heals %v", race, p, heals)
					}
				default:
					if has(buffs, pill) || has(debuffs, transmutive) || has(heals, miraculous) {
						t.Errorf("race %d path %d %q: a Master Class's mixture without it", race, p, master)
					}
				}
			}
		}
	}
}

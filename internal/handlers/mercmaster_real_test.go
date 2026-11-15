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

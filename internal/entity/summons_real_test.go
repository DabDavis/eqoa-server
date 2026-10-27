package entity

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The summons a real server holds (EQOA_REAL_DB): every spell whose line
// summons an item names one the server has, as handlers' summonItem finds it
// (by the words' name, else by the spell's own). jib's bug report of
// 2026-10-27 found eight that gave nothing (the ranger's Forage spells, the
// alchemist's master class potions), put right by tools/fix_archived_spells.py
// from the game's own records; this holds a database to that. Skipped
// without one: none is in this repository.
//
// summonGaps are the ones eqoaclassic's lines still leave with nothing, by
// spell, logged and not failed: what they make is not in this server yet.
func TestRealSummonsNameTheirItems(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "summons.db")
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

	type spell struct{ name, line, desc string }
	var spells []spell
	rows, err := db.Query(`SELECT p.spellname, e.line, p.spelldesc FROM spellpattern p JOIN spelleffect e USING(spellid)
		WHERE e.line <> '' AND p.spellid IN (SELECT spellid FROM classspells)`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s spell
		if err := rows.Scan(&s.name, &s.line, &s.desc); err != nil {
			t.Fatal(err)
		}
		spells = append(spells, s)
	}
	rows.Close() // one connection: no lookup while the rows are open

	inv := database.NewInventoryStore(db)
	var missing []string
	n := 0
	for _, s := range spells {
		d := ParseSpellNamed(s.name, s.line, s.desc)
		if d.Summons() {
			continue
		}
		for _, u := range d.Unread {
			// a line that makes food and names none (EQOA-Data's Forage lines)
			if strings.Contains(u, "creates") && strings.Contains(u, "food") {
				n++
				missing = append(missing, s.name+": "+u)
				continue
			}
			m := reSummonItem.FindStringSubmatch(strings.TrimSpace(reParens.ReplaceAllString(u, "")))
			if m == nil || reSummonCreature.MatchString(u) {
				continue
			}
			n++
			_, ok, err := inv.ItemPatternByName(m[1])
			if err == nil && !ok {
				_, ok, err = inv.ItemPatternByName(s.name)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				missing = append(missing, s.name+": "+m[1])
			}
		}
	}
	sort.Strings(missing)
	t.Logf("%d summons, %d naming no item here", n, len(missing))
	for _, m := range missing {
		name, _, _ := strings.Cut(m, ": ")
		if why, ok := summonGaps[name]; ok {
			t.Logf("known gap: %s (%s)", m, why)
			continue
		}
		t.Errorf("no item: %s", m)
	}
	for _, want := range []string{"Forage Berries", "Forage Tuber", "Miraculous Mixture", "Wonderous Vaccination", "Transmutive Mixture", "Neriak Cocktail"} {
		found := false
		for _, s := range spells {
			found = found || s.name == want
		}
		if !found {
			t.Errorf("%s has no line here", want)
		}
	}
}

// summonGaps: the magician's charged items and others whose lines name no
// item ("Summons an item, 3 charges"; an item that casts as it is used is
// not in this server), the magician's four elemental pets whose lines say no
// pet ("Summons a Ghilan": summonLine needs "pet", "to aid you" or "to your
// defense"), a call of a player, and the alchemist's breathing potion.
var summonGaps = map[string]string{
	"Alchemic Condensation": "no Aqueous Potion item yet",
	"Aura Stone":            "a charged item",
	"Burning Blade":         "an item it names only as a sword",
	"Call of the Hero":      "a player, not an item",
	"Fiery Pebble":          "a charged item",
	"Frost Orb":             "a charged item",
	"Ice Shard":             "a charged item",
	"Lava Stone":            "a charged item",
	"Mana Stone":            "a charged item",
	"Phase Crown":           "an item it names only as a cap",
	"Phase Gladius":         "an item it names not at all",
	"Phase Ring":            "a charged item",
	"Phase Shield":          "an item it names only as a shield",
	"Scoria Stone":          "an item it names not at all",
	"Summon Badi Elder":     "a pet the line does not call one",
	"Summon Efreeti Lord":   "a pet the line does not call one",
	"Summon Ghilan":         "a pet the line does not call one",
	"Summon Ghilan Noble":   "a pet the line does not call one",
	"Summon Shadow Knife":   "a charged item and throwing knives",
}

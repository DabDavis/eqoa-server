package database

import (
	"os"
	"path/filepath"
	"testing"
)

// Over a real database after tools/ranger_start_20261127.py --apply
// (EQOA_RANGER_DB names a copy): a new ranger of every race but the elves,
// the froglok and a human of each city, starts where the western human ranger
// does (Surefall Glade), as startPlace puts it, and a race of racedefs's
// mainland place is there too (but one whose wire race is the elves'); the
// elves' rangers keep their own start, and no other class starts there.
func TestRangerStartsRealDB(t *testing.T) {
	src := os.Getenv("EQOA_RANGER_DB")
	if src == "" {
		t.Skip("EQOA_RANGER_DB names a server database after tools/ranger_start_20261127.py --apply")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ranger.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	var gx, gz float64
	var gw int
	if err := db.QueryRow(`SELECT x, z, world FROM defaultcharacter WHERE race = 0 AND humType = 2 AND tclass = 1`).Scan(&gx, &gz, &gw); err != nil {
		t.Fatal(err)
	}
	at := func(c *CharacterRecord) bool {
		return c.World == gw && float64(c.X) == float64(float32(gx)) && float64(c.Z) == float64(float32(gz))
	}
	wire := map[int]int{}
	rows, err := db.Query(`SELECT DISTINCT race, race FROM defaultcharacter UNION SELECT id, wire_race FROM racedefs ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r, w int
		rows.Scan(&r, &w)
		if _, ok := wire[r]; !ok || r != w {
			wire[r] = w
		}
	}
	rows.Close()
	wire[raceFroglok] = 0
	n := 0
	for race, wr := range wire {
		elf := race == 1
		for hum := 0; hum <= 2; hum++ {
			c := &CharacterRecord{Race: race, HumType: hum, TClass: 1}
			city, ok := s.startPlace(c)
			if !ok || at(c) == elf {
				t.Errorf("race %d city %d ranger: %v at world %d (%.1f, %.1f) (%s), want the glade unless an elf", race, hum, ok, c.World, c.X, c.Z, city)
			}
			n++
			m := &CharacterRecord{Race: race, HumType: hum, TClass: 1}
			if s.MainlandPlace(m) && wr != 1 && !at(m) {
				t.Errorf("race %d city %d ranger: mainland place world %d (%.1f, %.1f)", race, hum, m.World, m.X, m.Z)
			}
			for _, cls := range []int{0, 2, 7} {
				o := &CharacterRecord{Race: race, HumType: hum, TClass: cls}
				if _, ok := s.startPlace(o); ok && at(o) {
					t.Errorf("race %d city %d class %d starts at the glade", race, hum, cls)
				}
			}
		}
	}
	t.Logf("%d race and city combos' rangers checked (the glade: world %d, %.1f, %.1f)", n, gw, gx, gz)
}

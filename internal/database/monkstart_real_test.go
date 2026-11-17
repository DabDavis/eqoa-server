package database

import (
	"os"
	"path/filepath"
	"testing"
)

// Over a real database after tools/monk_start.py --apply --starts (EQOA_MONK_DB
// names a copy): a new monk of every race, the froglok and a human of each
// city, starts where the western human monk does (the monastery), as
// startPlace puts it, and a race of racedefs's mainland place is there too.
func TestMonkStartsRealDB(t *testing.T) {
	src := os.Getenv("EQOA_MONK_DB")
	if src == "" {
		t.Skip("EQOA_MONK_DB names a server database after tools/monk_start.py --apply --starts")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "monk.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	var mx, mz float64
	var mw int
	if err := db.QueryRow(`SELECT x, z, world FROM defaultcharacter WHERE race = 0 AND humType = 2 AND tclass = 4`).Scan(&mx, &mz, &mw); err != nil {
		t.Fatal(err)
	}
	races := []int{}
	rows, err := db.Query(`SELECT DISTINCT race FROM defaultcharacter UNION SELECT id FROM racedefs ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r int
		rows.Scan(&r)
		races = append(races, r)
	}
	rows.Close()
	races = append(races, raceFroglok)
	n := 0
	for _, race := range races {
		for hum := 0; hum <= 2; hum++ {
			c := &CharacterRecord{Race: race, HumType: hum, TClass: 4}
			city, ok := s.startPlace(c)
			if !ok || c.World != mw || float64(c.X) != float64(float32(mx)) || float64(c.Z) != float64(float32(mz)) {
				t.Errorf("race %d city %d monk: %v at world %d (%.1f, %.1f) (%s), want the monastery", race, hum, ok, c.World, c.X, c.Z, city)
			}
			n++
			m := &CharacterRecord{Race: race, HumType: hum, TClass: 4}
			if s.MainlandPlace(m) && (m.World != mw || float64(m.X) != float64(float32(mx))) {
				t.Errorf("race %d city %d monk: mainland place world %d (%.1f, %.1f)", race, hum, m.World, m.X, m.Z)
			}
			w := &CharacterRecord{Race: race, HumType: hum, TClass: 0}
			if _, ok := s.startPlace(w); ok && race != 0 && w.World == mw && w.X == float32(mx) && w.Z == float32(mz) {
				t.Errorf("race %d city %d warrior starts at the monastery", race, hum)
			}
		}
	}
	t.Logf("%d race and city combos' monks at the monastery (world %d, %.1f, %.1f)", n, mw, mx, mz)
}

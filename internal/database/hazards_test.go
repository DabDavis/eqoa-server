package database

import "testing"

// Table hazards is made with the database, and a row the game cannot use is left out and named.
func TestHazardsTable(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/h.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO hazards (id, name, world, x1, y1, z1, x2, y2, z2, radius, damage, words, source) VALUES
			(1, ' Spiky vines ', 2, 7300, -20, 7180, 7312, -20, 7182, 1.8, 1000, 'The spiky vines tear at you for %d.', 'seed')`,
		`INSERT INTO hazards (id, name, world, damage) VALUES (2, 'no damage', 2, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	hs, bad, err := db.Hazards()
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || len(bad) != 1 || hs[0].Name != "Spiky vines" || hs[0].World != 2 || hs[0].EveryOr() != HazardDefaultEvery ||
		!HazardHolds(hs[0], 7306, -20, 7182) {
		t.Errorf("read %+v, bad %v", hs, bad)
	}
}

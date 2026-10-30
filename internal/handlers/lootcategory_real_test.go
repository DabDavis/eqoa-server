package handlers

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A real server's loot categories (EQOA_REAL_DB names its database, a copy of
// which is read) after tools/seed_loot_categories.py: a rat, a skeleton and
// a frost giant roll the active preset's categories from pools that hold
// items at their levels (the rat and the frost giant their own trash, and
// after tools/seed_trash_items.py the skeleton its own too), and
// over 120,000 seeded kills of each, every category whose pool holds items
// drops within five standard deviations of its rate, whatever the pool's size
// (the skeleton's uncommon pool is ten times the rat's). Skipped without a
// database, and on one the tool has not been applied to: none is in this
// repository.
func TestRealLootCategoriesHoldTheirRates(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "categories.db")
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
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'loot_category_seed'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/seed_loot_categories.py has not been applied to this database")
	}
	var trashSeeded int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'trash_seed_log'`).Scan(&trashSeeded); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	sizes := map[string]int{}
	for _, c := range []struct {
		name  string
		level int
		trash bool
	}{{"rat", 3, true}, {"skeleton", 23, false}, {"tenacious frost giant", 58, true}} {
		rolls, err := inv.CategoryRolls(c.name, c.level)
		if err != nil {
			t.Fatal(err)
		}
		if len(rolls) == 0 {
			t.Fatalf("%s rolls no category", c.name)
		}
		byCat := map[string]database.CategoryRoll{}
		for _, r := range rolls {
			byCat[r.Category] = r
		}
		if got := len(byCat["trash"].Choices) > 0; got != (c.trash || trashSeeded > 0) {
			t.Errorf("%s: trash of its own %v, want %v", c.name, got, c.trash || trashSeeded > 0)
		}
		if len(byCat["uncommon"].Choices) == 0 || len(byCat["global"].Choices) == 0 || len(byCat["gems"].Choices) == 0 {
			t.Errorf("%s at %d: an empty uncommon, global or gem pool", c.name, c.level)
		}
		sizes[c.name] = len(byCat["uncommon"].Choices)
		rng := rand.New(rand.NewSource(int64(c.level)))
		const kills = 120000
		got := map[string]int{}
		for k := 0; k < kills; k++ {
			for _, r := range rolls {
				if len(r.Counts) > 0 || len(r.Choices) == 0 {
					continue
				}
				if items := rollCategories([]database.CategoryRoll{r}, rng.Float64, rng.Intn); len(items) > 0 {
					got[r.Category]++
				}
			}
		}
		for _, r := range rolls {
			if len(r.Counts) > 0 || len(r.Choices) == 0 || r.Rate*kills < 5 {
				continue
			}
			if !within(got[r.Category], kills, r.Rate, 5) {
				t.Errorf("%s: %s %d in %d kills, want about %.0f (a pool of %d)", c.name, r.Category, got[r.Category],
					kills, r.Rate*kills, len(r.Choices))
			}
		}
	}
	if sizes["skeleton"] < 5*sizes["rat"] {
		t.Logf("pools: %v (the rate check above holds whatever their sizes)", sizes)
	}
}

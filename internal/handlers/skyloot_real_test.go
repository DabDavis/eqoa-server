package handlers

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// TestRealSkyLootRates rolls the corpse's own dice on a database with the
// Plane of Sky's loot in it (tools/quests/data/plane-of-sky-armor.json's
// deploy): what each kind of creature's kill gives, against what its rates and
// weights say. A common creature never gets a named creature's list; a named
// one gets the common creatures' lists too; the Quintessence always gets her
// ring and one of her weapons; a creature of another world that shares a name
// with one of the Plane's gets none of it at its own level.
func TestRealSkyLootRates(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "sky.db")
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
	sky := map[int]string{} // the Plane's items of its own lists: patternid -> category
	rows, err := db.Query(`SELECT patternid, category FROM lootpool WHERE region = 'plane of sky'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		var cat string
		if err := rows.Scan(&id, &cat); err != nil {
			t.Fatal(err)
		}
		sky[id] = cat
	}
	rows.Close()
	if len(sky) == 0 {
		t.Skip("the Plane of Sky's loot has not been applied to this database")
	}
	named := map[int]bool{}
	rows, err = db.Query(`SELECT patternid FROM lootpool WHERE category = 'sky named'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		named[id] = true
	}
	rows.Close()
	if len(named) == 0 {
		t.Fatal("no sky named list")
	}
	inv := database.NewInventoryStore(db)
	const kills = 600000
	roll := func(name string, level int) (map[int]int, map[int]string, []database.CategoryRoll) {
		t.Helper()
		rolls, err := inv.CategoryRolls(name, level)
		if err != nil {
			t.Fatal(err)
		}
		rng := rand.New(rand.NewSource(int64(level) + int64(len(name))))
		got, names := map[int]int{}, map[int]string{}
		for k := 0; k < kills; k++ {
			for _, it := range rollCategories(rolls, rng.Float64, rng.Intn) {
				got[it.Pattern.PatternID]++
				names[it.Pattern.PatternID] = it.Pattern.ItemName
			}
		}
		return got, names, rolls
	}
	// checkItems holds what came to what the rates and weights say, a category's
	// items within a few standard deviations of theirs; and says it.
	checkItems := func(name string, got map[int]int, rolls []database.CategoryRoll) map[string]int {
		t.Helper()
		lands := map[string]int{}
		for _, r := range rolls {
			if len(r.Counts) > 0 || len(r.Choices) == 0 || r.Rate <= 0 {
				continue
			}
			total := 0
			for _, c := range r.Choices {
				total += max(c.Weight, 0)
			}
			var lines []string
			for _, c := range r.Choices {
				id := c.Item.Pattern.PatternID
				if sky[id] != r.Category {
					continue
				}
				lands[r.Category] += got[id]
				want := r.Rate * float64(c.Weight) / float64(total)
				if want*kills >= 25 && !within(got[id], kills, want, 5) {
					t.Errorf("%s: %s (%s): %d in %d kills, want about %.0f", name, c.Item.Pattern.ItemName, r.Category, got[id], kills, want*kills)
				}
				lines = append(lines, fmt.Sprintf("1/%.0f", 1/want))
			}
			if len(lines) == 0 {
				continue
			}
			sort.Strings(lines)
			t.Logf("%s: %s at %.4f a kill: %d of the Plane's among %d items, one by one %s .. %s", name, r.Category, r.Rate, len(lines), len(r.Choices), lines[0], lines[len(lines)-1])
		}
		return lands
	}

	common, cnames, crolls := roll("dark servant", 64)
	clands := checkItems("dark servant", common, crolls)
	for id, n := range common {
		if named[id] {
			t.Errorf("dark servant dropped %s, a named creature's, %d times in %d kills", cnames[id], n, kills)
		}
	}
	if clands["sky parts"] == 0 || clands["rare"] == 0 || clands["super rare"] == 0 {
		t.Errorf("dark servant: %v in %d kills: a list of the Plane's that never landed", clands, kills)
	}

	boss, bnames, brolls := roll("Arch Bishop Bofa", 64)
	blands := checkItems("Arch Bishop Bofa", boss, brolls)
	namedGot := 0
	for id := range named {
		namedGot += boss[id]
	}
	if namedGot == 0 || blands["sky parts"] == 0 {
		t.Errorf("Arch Bishop Bofa: %d of the named list and %v in %d kills", namedGot, blands, kills)
	}
	_ = bnames

	// Not of the Plane, though they share a name with some of it.
	for _, c := range []struct {
		name  string
		level int
	}{{"flesh fiend", 25}, {"aviak villager", 18}} {
		got, names, _ := roll(c.name, c.level)
		for id, n := range got {
			if _, ok := sky[id]; ok {
				t.Errorf("%s at %d dropped %s %d times: a list of the Plane's", c.name, c.level, names[id], n)
			}
		}
	}

	// The Quintessence's own tables: the ring every kill, one weapon every kill.
	tables, err := inv.NPCLootRolls("Quintessence of Veeshan", 64)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(64))
	const qkills = 60000
	got, names := map[int]int{}, map[int]string{}
	counts := map[int]int{}
	for k := 0; k < qkills; k++ {
		items := rollLoot(tables, rng.Intn)
		counts[len(items)]++
		for _, it := range items {
			got[it.Pattern.PatternID]++
			names[it.Pattern.PatternID] = it.Pattern.ItemName
		}
	}
	if counts[2] != qkills {
		t.Errorf("the Quintessence's tables: %v items a kill, want 2 every time", counts)
	}
	const ring = 713088
	if got[ring] != qkills {
		t.Errorf("Ring of Veeshan %d times in %d kills", got[ring], qkills)
	}
	if len(got) != 7 {
		t.Errorf("the Quintessence's tables gave %d items, want her ring and six weapons: %v", len(got), names)
	}
	for id, n := range got {
		if id != ring && !within(n, qkills, 1.0/6, 5) {
			t.Errorf("the Quintessence: %s %d times in %d kills, want about %d", names[id], n, qkills, qkills/6)
		}
	}
	t.Logf("the Quintessence's tables, %d kills: %v", qkills, got)
}

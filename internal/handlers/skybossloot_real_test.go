package handlers

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Live petition 5511 ("PoS rares and bosses are not dropping items"): on a
// server database (EQOA_REAL_DB), Ahriaana the Ultimate's own table gives a
// piece every kill, and each of Veeshan's twins gives one or two pieces of
// its gear every kill (the owner-approved 30% one, 70% two), Ascended on
// Heirloom, the pieces they were made from on Live
// (tools/sky_loot_live_20261123.py); none of it Ascended where the shard keeps
// Ascended loot off (no Ascended record in any table or pool of the Plane).
func TestRealSkyBossesDropTheirGear(t *testing.T) {
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
	var ahr int
	db.QueryRow(`SELECT count(*) FROM npcloot WHERE npc_name = 'ahriaana the ultimate'`).Scan(&ahr)
	if ahr == 0 {
		t.Skip("no Ahriaana here")
	}
	inv := database.NewInventoryStore(db)
	rng := rand.New(rand.NewSource(5511))
	const kills = 20000

	tables, err := inv.NPCLootRolls("Ahriaana the Ultimate", 65)
	if err != nil {
		t.Fatal(err)
	}
	none := 0
	for k := 0; k < kills; k++ {
		if len(rollLoot(tables, rng.Intn)) == 0 {
			none++
		}
	}
	if none > 0 {
		t.Errorf("Ahriaana's tables gave nothing in %d of %d kills", none, kills)
	}

	for _, twin := range []string{"Veeshan's Talon", "Veeshan's Bulwark"} {
		rolls, err := inv.CategoryRolls(twin, 65)
		if err != nil {
			t.Fatal(err)
		}
		tabs, err := inv.NPCLootRolls(twin, 65)
		if err != nil {
			t.Fatal(err)
		}
		pieces := map[int]int{}
		for k := 0; k < kills; k++ {
			n := 0
			for _, it := range append(rollCategories(rolls, rng.Float64, rng.Intn), rollLoot(tabs, rng.Intn)...) {
				if it.Pattern.EquipSlot >= 0 && it.Pattern.EquipSlot != 255 && !strings.Contains(strings.ToLower(it.Pattern.ItemName), "scale") &&
					!strings.Contains(strings.ToLower(it.Pattern.ItemName), "gem of the sky") {
					n++
				}
			}
			pieces[n]++
		}
		if pieces[0] > kills/100 {
			t.Errorf("%s gave no gear in %d of %d kills: %v", twin, pieces[0], kills, pieces)
		}
		t.Logf("%s: gear pieces a kill over %d kills: %v", twin, kills, pieces)
	}
}

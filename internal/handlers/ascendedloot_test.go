package handlers

import (
	"math/rand"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// ascendTool runs tools/ascend_boss_loot.py on the database at path.
func ascendTool(t *testing.T, path string, args ...string) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 for tools/ascend_boss_loot.py")
	}
	cmd := exec.Command(py, append([]string{"-E", "-s", "../../tools/ascend_boss_loot.py", "--db", path, "--no-backup"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ascend_boss_loot.py %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// lootDrops is what n kills of a creature of that name and level drop from
// its loot tables (the corpse's own dice, handlers/loot.go), by item name.
func lootDrops(t *testing.T, inv *database.InventoryStore, name string, level, n int) map[string]int {
	t.Helper()
	rolls, err := inv.NPCLootRolls(name, level)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(int64(level)))
	got := map[string]int{}
	for k := 0; k < n; k++ {
		for _, it := range rollLoot(rolls, rng.Intn) {
			got[it.Pattern.ItemName]++
		}
	}
	return got
}

// Petition 1446, end to end: tools/ascend_boss_loot.py on a server database,
// then the corpse's own rolls. A tuned boss drops its gear as Ascended
// records and its spell as it was, from its own table and from its copy of
// the table it shares; the creature that shares it (not tuned) drops the
// regular gear. The records read through the server are the bases' a tenth
// more. --undo gives the boss its regular drops back.
func TestATunedBossDropsAscendedGearAndItsSpells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boss.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc,
			model, color, str, AC, FR)
			VALUES (7001, 4000, 0, 5, 0, 12, 0, 1, 0, 0, 1, 61, 0, 60, 1, 250, 0, 32767, 1023, 0, 0, 0, 0, 'Fang of Testgafen', '', 9, 0, 20, 0, 50),
			       (7002, 10, 0, -607649328, 0, -1, 0, 0, 1, 0, 0, 0, 0, 60, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Searing Test', '', 0, 0, NULL, NULL, NULL),
			       (7003, 600, 0, 6, 0, 13, 0, 0, 1, 0, 0, 0, 0, 55, 1, 400, 0, 32767, 1023, 0, 0, 0, 0, 'Giant Shield', '', 8, 0, NULL, 60, NULL),
			       (7004, 2, 0, 7, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'ember gem', '', 0, 0, NULL, NULL, NULL)`,
		`INSERT INTO npcs (id, npc_name, world, npc_level, npc_type) VALUES (1, 'Lord Testgafen', 3, 65, 0), (2, 'fire giant', 3, 55, 0)`,
		`INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('lord testgafen', 'boss lord testgafen', 100, 2),
			('lord testgafen', 'giants', 100, 1), ('fire giant', 'giants', 100, 1)`,
		`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('boss lord testgafen', 7001, 1, 1, 70),
			('boss lord testgafen', 7002, 1, 1, 70), ('giants', 7003, 1, 1, 70), ('giants', 7004, 1, 1, 70)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	db.Close()

	out := ascendTool(t, path, "--boss", "Lord Testgafen", "--apply")
	if !strings.Contains(out, `rolls a copy of it, "giants ascended"`) || !strings.Contains(out, "wrote") {
		t.Fatalf("the tool said:\n%s", out)
	}
	db, err = database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := database.NewInventoryStore(db)
	const kills = 2000
	boss := lootDrops(t, inv, "Lord Testgafen", 65, kills)
	for _, name := range []string{"Ascended Fang of Testgafen", "Searing Test", "Ascended Giant Shield", "ember gem"} {
		if boss[name] == 0 {
			t.Errorf("Lord Testgafen never dropped %s in %d kills: %v", name, kills, boss)
		}
	}
	for _, name := range []string{"Fang of Testgafen", "Giant Shield"} {
		if boss[name] != 0 {
			t.Errorf("Lord Testgafen dropped the regular %s %d times", name, boss[name])
		}
	}
	giant := lootDrops(t, inv, "fire giant", 55, kills)
	if giant["Giant Shield"] == 0 || giant["ember gem"] == 0 || giant["Ascended Giant Shield"] != 0 {
		t.Errorf("the fire giant, not tuned, dropped %v: want the regular shield and the gem", giant)
	}
	m, err := database.AscendedMap(db.DB)
	if err != nil || len(m) != 2 {
		t.Fatalf("Ascended records %v (%v), want the fang's and the shield's", m, err)
	}
	items := database.NewItemPatternStore(db)
	for copyID, base := range m {
		c, err1 := items.GetItemPattern(copyID)
		b, err2 := items.GetItemPattern(base)
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		fig := func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		}
		if c.ItemName != database.AscendedName(b.ItemName) || c.WeaponDamage != database.AscendedFigure(b.WeaponDamage) ||
			fig(c.Str) != database.AscendedFigure(fig(b.Str)) || fig(c.AC) != database.AscendedFigure(fig(b.AC)) ||
			fig(c.FR) != database.AscendedFigure(fig(b.FR)) || c.MaxHP != b.MaxHP || c.ItemCost != b.ItemCost || c.Rent != 1 ||
			c.EquipSlot != b.EquipSlot || c.Lore != b.Lore || c.Trade != b.Trade || c.ItemIcon != b.ItemIcon || c.Model != b.Model {
			t.Errorf("Ascended record %d of %d: %+v\nits base: %+v", copyID, base, c, b)
		}
	}
	db.Close()

	out = ascendTool(t, path, "--undo")
	if !strings.Contains(out, "put back") {
		t.Fatalf("the undo said:\n%s", out)
	}
	db, err = database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	inv = database.NewInventoryStore(db)
	boss = lootDrops(t, inv, "Lord Testgafen", 65, kills)
	if boss["Fang of Testgafen"] == 0 || boss["Giant Shield"] == 0 || boss["Ascended Fang of Testgafen"] != 0 {
		t.Errorf("after --undo Lord Testgafen dropped %v: want his regular gear back", boss)
	}
	if m, err := database.AscendedMap(db.DB); err != nil || len(m) != 0 {
		t.Errorf("after --undo the Ascended records %v (%v), want none", m, err)
	}
}

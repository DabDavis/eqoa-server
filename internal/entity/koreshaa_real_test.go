package entity

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Lord Koreshaa over a real server's data after tools/koreshaa_20261113.py
// --apply (EQOA_KORESHAA_DB names that database; a copy is read; skipped
// without one): his own row stands out of the world; his placeholder, the
// Welahi-nai myrmidon on his spot (132354), comes back as itself on a miss
// and as him on a hit, on its own spot, in his body at level 62 with his
// template's 150,042 hit points, his hit heal of 15 and his nuke held below
// 34%; and a miss after makes it a myrmidon again.
func TestRealKoreshaaOnHisPlaceholder(t *testing.T) {
	src := os.Getenv("EQOA_KORESHAA_DB")
	if src == "" {
		t.Skip("EQOA_KORESHAA_DB names a server database with tools/koreshaa_20261113.py applied")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "koreshaa.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	const ph EntityID = 132354
	var own EntityID
	if err := db.QueryRow(`SELECT id FROM npcs WHERE npc_name = 'Lord Koreshaa'`).Scan(&own); err != nil {
		t.Fatal(err)
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if n, err := w.LoadRareSpawns(db); err != nil || n != 1 {
		t.Fatalf("rules: %d, %v", n, err)
	}
	for _, p := range w.ClassTemplateProblems() {
		t.Errorf("a template problem: %s", p)
	}
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	defer func(f func(*World) float64) { rareRand = f }(rareRand)

	if o := w.NPCs[own]; o == nil || !o.SpawnOnly {
		t.Fatalf("his own row %d: %+v, want out of the world", own, o)
	}
	npc := w.NPCs[ph]
	if npc == nil || npc.Name != "welahi-nai myrmidon" {
		t.Fatalf("the placeholder: %+v", npc)
	}
	rareRand = func(*World) float64 { return 99 }
	killAndBack(t, w, ph)
	if npc.Name != "welahi-nai myrmidon" {
		t.Fatalf("a miss: %q", npc.Name)
	}
	rareRand = func(*World) float64 { return 0 }
	killAndBack(t, w, ph)
	if npc.Name != "Lord Koreshaa" || npc.Level != 62 || npc.ModelID != 0x5D40C8AB || npc.SpawnOnly {
		t.Fatalf("a hit: %q level %d body %x out of the world %v", npc.Name, npc.Level, npc.ModelID, npc.SpawnOnly)
	}
	if npc.X < 5156 || npc.X > 5157 || npc.Z < 7012 || npc.Z > 7013 || npc.ServerID != ph {
		t.Fatalf("off the placeholder's spot: (%v, %v) id %d", npc.X, npc.Z, npc.ServerID)
	}
	if npc.MaxHP != 150_042 || npc.HP != npc.MaxHP {
		t.Errorf("hit points %d of %d, want 150,042", npc.HP, npc.MaxHP)
	}
	tm := npc.tmpl
	if tm == nil || tm.hitHeal != 15 || len(tm.abilities) != 1 || tm.abilities[0].Damage != 3500 ||
		tm.abilities[0].Resist != "AR" || tm.abilities[0].Below != 34 {
		t.Fatalf("his template: %+v", tm)
	}
	rareRand = func(*World) float64 { return 99 }
	killAndBack(t, w, ph)
	if npc.Name != "welahi-nai myrmidon" || npc.ModelID != 0x78FA8F20 {
		t.Fatalf("a miss after him: %q body %x", npc.Name, npc.ModelID)
	}
	var crown int
	if err := db.QueryRow(`SELECT l.patternid FROM npcloot n JOIN loottables l ON l.tablename = n.tablename
		WHERE n.npc_name = 'lord koreshaa' AND n.chance = 100`).Scan(&crown); err != nil || crown != 6248 {
		t.Errorf("his loot: %d, %v", crown, err)
	}
}

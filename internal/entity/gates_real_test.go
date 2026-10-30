package entity

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// The gates a real server holds (EQOA_REAL_DB, spellgates filled by
// tools/seed_spell_gates.py) over the baked ground (EQOA_GROUND_DIR,
// cmd/groundbake's) and the server's own creatures: each lands where the
// server would set a player, on a pyramid's top as written, else on the
// terrain within 200 of its place, under the open sky and out of the water,
// in one of the disc's six worlds; a place under a roof with no open ground
// near (the vampires' Sanctuary) on itself. Skipped without both: neither is
// in this repository.
func TestRealSpellGatesLand(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	src := os.Getenv("EQOA_REAL_DB")
	if dir == "" || src == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database with spellgates")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, _ := ground.OpenEnv(dir)
	w := realWorld(t)
	w.ground = g
	if env != nil {
		w.env = env
	}
	// the gates, out of a copy (opening a database writes its schema)
	path := filepath.Join(t.TempDir(), "gates.db")
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
	gates, bad, err := db.SpellGates()
	if err != nil || len(bad) != 0 {
		t.Fatalf("spellgates: %v, left out %v", err, bad)
	}
	if len(gates) == 0 {
		t.Skip("no spellgates rows: run tools/seed_spell_gates.py on the database first")
	}
	for _, row := range gates {
		if row.World >= DiscWorlds {
			t.Errorf("%s: world %d, on no disc", row.Spell, row.World)
			continue
		}
		if row.Kind == database.GateEvac {
			continue // its caster's home
		}
		w.mu.Lock()
		x, y, z := w.gateLandingLocked(row)
		w.mu.Unlock()
		under, onTerrain := g.Floor(row.World, row.X, row.Y+gateRise, row.Z)
		if onTerrain && under < row.Y-gateAbove {
			if x != row.X || y != row.Y || z != row.Z {
				t.Errorf("%s: a place %.1f above the terrain landed at (%.1f, %.1f, %.1f), want it as written", row.Spell, row.Y-under, x, y, z)
			}
			t.Logf("%-24s %s: on its top as written (%.1f above the terrain)", row.Spell, row.Place, row.Y-under)
			continue
		}
		d := distXZ(x, z, row.X, row.Z)
		fy, ok := g.Floor(row.World, x, y+0.1, z)
		top, _ := g.Floor(row.World, x, y+1000, z)
		switch {
		case !ok || math.Abs(float64(fy-y)) > 0.05:
			t.Errorf("%s: landed at (%.1f, %.1f, %.1f), off the terrain (%.1f)", row.Spell, x, y, z, fy)
		case top > y+gateOpen && d > 0:
			t.Errorf("%s: landed under a floor %.1f above", row.Spell, top-y)
		case top > y+gateOpen:
			// a place under a roof with no open ground near enough (the
			// vampires' Sanctuary, in the rock): the search's last resort,
			// the place itself
			t.Logf("%-24s %s: under a roof, no open ground within the search: on its place", row.Spell, row.Place)
			continue
		case env != nil && w.waterAtLocked(row.World, x, y+bodyAbove, z):
			t.Errorf("%s: landed in the water", row.Spell)
		case d > gateSteps[len(gateSteps)-1]:
			t.Errorf("%s: landed %.0f from its place", row.Spell, d)
		}
		t.Logf("%-24s %s: lands %.0f off its place, at (%.0f, %.1f, %.0f)", row.Spell, row.Place, d, x, y, z)
	}
}

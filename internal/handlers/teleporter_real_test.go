package handlers

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

// The Teleporter on a real server's data (EQOA_REAL_DB, a copy opened) over
// its baked ground (EQOA_GROUND_DIR): every raid of its table is offered (one
// of its bosses stands in its world), and each lands on a floor at its own
// height, out of the water, the lava and the slime, at least 50 from each of
// its bosses' rows and with no creature of level 10 or more (alive,
// attackable, in the world) spawned within 30 and 12 of its height; every
// camp it offers lands. Skipped without both.
func TestRealTeleporterLandings(t *testing.T) {
	dir, src := os.Getenv("EQOA_GROUND_DIR"), os.Getenv("EQOA_REAL_DB")
	if dir == "" || src == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tele.db")
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
	w := entity.NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	w.SetGround(g)
	w.SetEnv(env)

	// creatures is how many fighting creatures spawn within 30 of (x, y, z)
	creatures := func(world uint8, x, y, z float32) int {
		n := 0
		for _, c := range w.NPCs {
			// below level 10 no threat to a raid: a rook, a pike, the plateau's ZP1 marker by its port-in
			if c.World != world || c.NPCType&0x80 != 0 || c.Dead || c.SpawnOnly || c.Temporary || c.Level < 10 {
				continue
			}
			if math.Hypot(float64(c.SpawnX-x), float64(c.SpawnZ-z)) < 30 && math.Abs(float64(c.SpawnY-y)) < 12 {
				n++
			}
		}
		return n
	}
	// the control: on Lady Vox's own spot her court stands round her
	if x, y, z, _, world, ok := w.SpawnOfNPC("Lady Vox"); ok && creatures(world, x, y, z) == 0 {
		t.Errorf("no creature within 30 of Lady Vox herself: the check sees nothing")
	}

	spots := readTeleSpots(w)
	offered := map[string]bool{}
	camps := 0
	for i := range spots {
		sp := &spots[i]
		dest, ok := teleLanding(w, sp)
		if !ok {
			t.Errorf("%s: no landing", sp.label)
			continue
		}
		if sp.raid == nil {
			camps++
			// a camp's landing out of the lava (petition 1895: the moots' landed in it)
			for _, y := range []float32{0.5, 1.5} {
				if f, ok := env.Flags(dest.World, dest.X, dest.Y+y, dest.Z); ok && f&ground.EnvLava != 0 {
					t.Errorf("%s: in the lava at %.1f up (%.1f, %.2f, %.1f)", sp.label, y, dest.X, dest.Y, dest.Z)
				}
			}
			if sp.camp == "moots" {
				t.Logf("%s: world %d (%.1f, %.2f, %.1f) facing %.2f", sp.label, dest.World, dest.X, dest.Y, dest.Z, dest.Facing)
			}
			continue
		}
		offered[sp.raid.name] = true
		fy, on := g.Floor(dest.World, dest.X, dest.Y+2, dest.Z)
		if !on || math.Abs(float64(fy-dest.Y)) > 1.5 {
			t.Errorf("%s: lands at %.2f over a floor at %.2f (%v)", sp.label, dest.Y, fy, on)
		}
		for _, y := range []float32{0.5, 1.5} {
			if f, ok := env.Flags(dest.World, dest.X, dest.Y+y, dest.Z); ok && f != 0 {
				t.Errorf("%s: in water, lava or slime (%d) at %.1f up", sp.label, f, y)
			}
		}
		for _, b := range w.NPCSpawnsNamed(func(name string) bool {
			for _, n := range sp.raid.bosses {
				if sameName(n, name) {
					return true
				}
			}
			return false
		}) {
			if b.World == dest.World && math.Hypot(float64(b.X-dest.X), float64(b.Z-dest.Z)) < 50 {
				t.Errorf("%s: %s's row %.0f off", sp.label, b.Name, math.Hypot(float64(b.X-dest.X), float64(b.Z-dest.Z)))
			}
		}
		if n := creatures(dest.World, dest.X, dest.Y, dest.Z); n > 0 {
			t.Errorf("%s: %d creatures spawn within 30 of its landing", sp.label, n)
		}
		t.Logf("%s: world %d (%.1f, %.2f, %.1f) facing %.2f", sp.label, dest.World, dest.X, dest.Y, dest.Z, dest.Facing)
	}
	for _, rd := range teleRaids {
		if !offered[rd.name] {
			t.Errorf("%s is not offered: no row of %v stands by its landing", rd.name, rd.bosses)
		}
	}
	for k := range campDests {
		found := false
		for _, sp := range spots {
			found = found || sp.camp == k
		}
		if !found {
			t.Logf("the camp %s is not offered: no %q stands by it", k, campDests[k].NPC)
		}
	}
	if camps < len(campDests)-5 {
		t.Errorf("only %d camps of %d offered", camps, len(campDests))
	}
	t.Logf("%d raids and %d camps", len(offered), camps)
}

// sameName is NPC names as the world compares them: any case, no edge spaces.
func sameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

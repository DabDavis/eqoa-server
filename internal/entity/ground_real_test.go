package entity

import (
	"math"
	"os"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/ground"
)

// The baked ground (EQOA_GROUND_DIR, cmd/groundbake's) under a real
// server's creatures (EQOA_REAL_DB): their spawns' heights are the live
// game's, standing on the terrain, so the floor under each must be within
// the 1.5 CLIENT sets a creature down from (0x00CBCE40). Skipped without
// both: neither is in this repository.
func TestTheGroundUnderARealWorldsCreatures(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := realWorld(t)
	var miss []float64
	within, none, all := 0, 0, 0
	for _, npc := range w.NPCs {
		if !w.wanders(npc) {
			continue
		}
		all++
		y, ok := g.Floor(npc.World, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
		switch d := math.Abs(float64(y - npc.SpawnY)); {
		case !ok:
			none++
		case d <= 1.5:
			within++
		default:
			miss = append(miss, d)
		}
	}
	sort.Float64s(miss)
	t.Logf("%d creatures: %d stand within 1.5 of the ground, %d have no terrain under them, %d farther (median %.1f), %d zone loads",
		all, within, none, len(miss), median(miss), g.Loads)
	if float64(within) < 0.9*float64(all) {
		t.Errorf("only %d of %d creatures on the ground", within, all)
	}
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[len(v)/2]
}

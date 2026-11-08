package entity

import (
	"bytes"
	"log"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/ground"
)

// The Elephant Graveyard's Xrags on the real ground (the user, 2026-11-08: "the
// xrags when they are aggroed will run through the ledge instead of using the
// ramp pathing"): each chases an enemy standing at another's spawn on another
// floor (8 to 60 off across the ground, 5 or more above or below). With the
// routes, no chase falls past a ledge's drop or climbs through rock where a way
// was found, and nearly every one reaches its enemy on its floor; the control,
// the same chases with the routes off, goes over the ledges. Skipped without
// EQOA_GROUND_DIR and EQOA_REAL_DB.
func TestXragChasesKeepToTheGround(t *testing.T) {
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
	w.SetGround(g)
	var xrags []*NPC
	for _, npc := range w.NPCs {
		if strings.HasSuffix(npc.Name, "of Xrag") && npc.World == 0 {
			xrags = append(xrags, npc)
		}
	}
	if len(xrags) < 20 {
		t.Fatalf("only %d Xrags", len(xrags))
	}
	type pair struct{ a, b *NPC }
	var pairs []pair
	for _, a := range xrags {
		for _, b := range xrags {
			d := distXZ(a.SpawnX, a.SpawnZ, b.SpawnX, b.SpawnZ)
			if a != b && d >= 8 && d <= 60 && absf(a.SpawnY-b.SpawnY) >= 5 {
				pairs = append(pairs, pair{a, b})
			}
		}
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	run := func(routes bool, p pair) (reached, bad bool, took time.Duration) {
		was := chaseRoutes
		chaseRoutes = routes
		defer func() { chaseRoutes = was }()
		npc := p.a
		npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
		npc.chase = chaseState{}
		npc.swimming = false
		start := time.Now()
		ok, at := chaseOver(w, npc, p.b.SpawnX, p.b.SpawnY, p.b.SpawnZ, 600)
		_, bad = fallsOrClimbs(at)
		return ok, bad, time.Since(start)
	}
	var reached, bad, noWay, controlBad, mostSearches int
	var took, worst, searchTime, slowest time.Duration
	var searchCount int
	for _, p := range pairs {
		buf.Reset()
		ok, b, d := run(true, p)
		took += d
		worst = max(worst, d)
		found := strings.Contains(buf.String(), "goes round")
		searches := strings.Count(buf.String(), "goes round") + strings.Count(buf.String(), "finds no way")
		mostSearches = max(mostSearches, searches)
		if strings.Contains(buf.String(), "finds no way") && !found {
			noWay++
			if os.Getenv("EQOA_TRACE") != "" {
				t.Logf("no way: %s", lastLines(buf.String(), 1))
			}
			continue
		}
		if ok {
			reached++
		}
		for _, l := range strings.Split(buf.String(), "\n") {
			if i := strings.LastIndex(l, " tried, "); i >= 0 {
				if d, err := time.ParseDuration(strings.TrimRight(strings.SplitN(l[i+len(" tried, "):], ")", 2)[0], " ")); err == nil {
					searchTime += d
					searchCount++
					slowest = max(slowest, d)
				}
			}
		}
		if b {
			bad++
			t.Logf("%s from (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f): fell or climbed\n%s", p.a.Name, p.a.SpawnX, p.a.SpawnY, p.a.SpawnZ,
				p.b.SpawnX, p.b.SpawnY, p.b.SpawnZ, lastLines(buf.String(), 6))
		}
		if _, cb, _ := run(false, p); cb {
			controlBad++
		}
	}
	routed := len(pairs) - noWay
	t.Logf("%d chases between floors: %d with a way, %d of them reached their enemy, %d fell or climbed; %d with no way found (straight); "+
		"the control (no routes) fell or climbed in %d; %v a chase on average, %v the most; %d searches at most in one chase", len(pairs), routed, reached, bad, noWay,
		controlBad, took/time.Duration(len(pairs)), worst, mostSearches)
	if searchCount > 0 {
		t.Logf("%d searches, %v each on average, %v the slowest", searchCount, searchTime/time.Duration(searchCount), slowest)
	}
	if bad > 0 {
		t.Errorf("%d chases with a way fell or climbed", bad)
	}
	if routed == 0 || reached < routed*9/10 {
		t.Errorf("%d of %d chases with a way reached their enemy", reached, routed)
	}
	if controlBad == 0 {
		t.Error("the control never went over a ledge: the pairs test nothing")
	}
}

// lastLines is s's last n lines.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Ordinary chases across Tunaria are as they were (a measure that the routes
// cost little where nothing stands in the way): 400 creatures that wander,
// each chasing an enemy 15 off on its own floor, reach it as often as with the
// routes off, and search seldom. Skipped without EQOA_GROUND_DIR and
// EQOA_REAL_DB.
func TestOrdinaryChasesAreAsTheyWere(t *testing.T) {
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
	w.SetGround(g)
	var picked []*NPC
	for _, npc := range w.NPCs {
		if npc.World == 0 && w.wanders(npc) && len(picked) < 400 {
			picked = append(picked, npc)
		}
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	type goal struct{ x, y, z float32 }
	goals := map[*NPC]goal{}
	for i, npc := range picked {
		a := float64(i) * 2.399 // spread round
		x, z := npc.SpawnX+15*float32(math.Sin(a)), npc.SpawnZ+15*float32(math.Cos(a))
		if y, ok := g.Floor(0, x, npc.SpawnY+3, z); ok && absf(y-npc.SpawnY) <= 3 {
			goals[npc] = goal{x, y, z}
		}
	}
	run := func(routes bool) (reached, searches int, took time.Duration) {
		was := chaseRoutes
		chaseRoutes = routes
		defer func() { chaseRoutes = was }()
		buf.Reset()
		start := time.Now()
		for npc, gl := range goals {
			npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
			npc.chase = chaseState{}
			npc.swimming = false
			if ok, _ := chaseOver(w, npc, gl.x, gl.y, gl.z, 100); ok {
				reached++
			}
		}
		return reached, strings.Count(buf.String(), "goes round") + strings.Count(buf.String(), "finds no way"), time.Since(start)
	}
	on, searches, tookOn := run(true)
	off, _, tookOff := run(false)
	t.Logf("%d chases 15 off on their floor: %d reached with the routes (%d searches, %v), %d without (%v)", len(goals), on, searches, tookOn, off, tookOff)
	if on < off-len(goals)/50 {
		t.Errorf("with the routes %d reached, without %d", on, off)
	}
}

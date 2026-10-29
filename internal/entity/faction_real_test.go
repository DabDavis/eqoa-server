package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Factions over a real server's NPCs (EQOA_REAL_DB names its database; a copy
// is opened), once tools/seed_npc_factions.py has written npcfaction: no NPC
// on Neriak's cavern floor attacks a new dark elf of any of its classes on
// sight, the user's new shadow knight at its start spot (2026-10-29: the
// ghoul servant) least of all, and the members of each faction are counted.
// Once tools/seed_faction_starts.py has written the measured starts, no NPC
// attacks a new character at any start spot. Skipped without a database:
// none is in this repository.
func TestFactionsOverARealWorld(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realWorld(t)
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "factions.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	spots, err := db.StartSpots()
	if err != nil {
		t.Fatal(err)
	}
	var seededStarts, members int
	db.QueryRow(`SELECT COUNT(*) FROM factionstart WHERE source = 'seed'`).Scan(&seededStarts)
	db.QueryRow(`SELECT COUNT(*) FROM npcfaction`).Scan(&members)
	t.Logf("%d measured starts (factionstart, source seed), %d npcfaction rows", seededStarts, members)

	// the members of each faction, and those that may attack on sight
	w.mu.Lock()
	of, attackers := map[string]int{}, map[string]int{}
	for _, npc := range w.NPCs {
		if f := w.npcFactionLocked(npc); f != "" {
			of[f]++
			if _, ok := w.factionAggroLocked(npc); ok {
				attackers[f]++
			}
		}
	}
	w.mu.Unlock()
	var names []string
	for f := range of {
		names = append(names, f)
	}
	sort.Slice(names, func(i, j int) bool {
		return of[names[i]] > of[names[j]] || of[names[i]] == of[names[j]] && names[i] < names[j]
	})
	var lines []string
	for _, f := range names {
		lines = append(lines, fmt.Sprintf("%s: %d (%d attackable)", f, of[f], attackers[f]))
	}
	t.Logf("members by faction: %s", strings.Join(lines, "; "))
	if members == 0 {
		t.Skip("npcfaction is empty: tools/seed_npc_factions.py --apply first")
	}

	// the user's: a new dark elf shadow knight at its start, by the coach
	sk := database.StartSpot{Race: 2, Class: 3, HumType: 0, World: 0, X: 24659.8, Y: 5.77, Z: 8836.5}
	if th := w.StartThreats([]database.StartSpot{sk}); len(th) != 0 {
		t.Errorf("a new dark elf shadow knight at Neriak's start is attacked on sight: %+v", th)
	}
	// every dark elf class, anywhere in the city: among its people (the
	// Citizens of Neriak and its houses, within 450 of its middle: not the
	// Nektulos outpost 450 north) as tools/seed_npc_factions.py reads it,
	// within 40 of one in 3D (not wildlife) or inside the hull of those within
	// 150 of it at its height (within 30). What attacks elsewhere on the floor
	// (its people stand at -2.4 to 14.6; the forest overhead is 54 up) is said.
	w.mu.Lock()
	type person struct{ x, y, z float32 }
	var city []person
	for _, npc := range w.NPCs {
		f := w.npcFactionLocked(npc)
		if npc.World == 0 && (f == "Citizens of Neriak" || strings.HasPrefix(f, "House ")) &&
			distXZ(npc.SpawnX, npc.SpawnZ, 24970, 8790) <= 450 && w.playerModels[uint32(npc.ModelID)] {
			city = append(city, person{npc.SpawnX, npc.SpawnY, npc.SpawnZ})
		}
	}
	among := func(npc *NPC) bool {
		_, wild := w.wildlifeLocked(npc) // wildlife only inside the hull
		var near [][2]float32
		for _, c := range city {
			if !wild && dist3(c.x, c.y, c.z, npc.SpawnX, npc.SpawnY, npc.SpawnZ) <= 40 {
				return true
			}
			if distXZ(c.x, c.z, npc.SpawnX, npc.SpawnZ) <= 150 && c.y-npc.SpawnY <= 30 && npc.SpawnY-c.y <= 30 {
				near = append(near, [2]float32{c.x, c.z})
			}
		}
		return inHull(convexHull(near), npc.SpawnX, npc.SpawnZ)
	}
	t.Logf("Neriak: %d people", len(city))
	for _, s := range spots {
		if s.Race != 2 {
			continue
		}
		p := &Player{Name: "a new dark elf", Level: 1, Race: s.Race, Class: s.Class, HumType: s.HumType}
		for _, npc := range w.NPCs {
			if npc.World != 0 || distXZ(npc.SpawnX, npc.SpawnZ, 24970, 8790) > 450 || npc.SpawnY < -20 || npc.SpawnY > 30 ||
				!w.threatensLocked(npc, p) {
				continue
			}
			f, _ := w.factionAggroLocked(npc)
			what := fmt.Sprintf("a new dark elf %s: %q (level %d) at (%.0f, %.0f, %.0f) attacks it on sight (faction %q)",
				nameIn(classNames, s.Class), npc.Name, npc.Level, npc.SpawnX, npc.SpawnY, npc.SpawnZ, f)
			if among(npc) {
				t.Error(what)
			} else if s.Class == 3 {
				t.Log("outside the city: " + what)
			}
		}
	}
	w.mu.Unlock()

	// the NPCs quests talk to that no town takes in (tools/seed_npc_factions.py's
	// NAMED): none attacks a new character of any combo on sight, and the Fire
	// Giant Magus stays attackable (a quest kills him)
	w.mu.Lock()
	named := map[string]bool{"sartheret": true, "mark of the majestic": true, "aviak elder": true,
		"essence of day": true, "essence of desolation": true, "essence of night": true, "essence of rain": true,
		"essence of snow": true, "graumord citizen": true, "fire giant magus": true}
	seen := 0
	for _, npc := range w.NPCs {
		if !named[database.NPCClassKey(npc.Name)] {
			continue
		}
		seen++
		if w.npcFactionLocked(npc) == "" {
			t.Errorf("%q (world %d) is of no faction", npc.Name, npc.World)
		}
		if database.NPCClassKey(npc.Name) == "fire giant magus" && npc.NPCType&0x80 != 0 {
			t.Errorf("the Fire Giant Magus has the NPC bit: no quest can kill him")
		}
		for _, s := range spots {
			p := &Player{Name: "a new character", Level: 1, Race: s.Race, Class: s.Class, HumType: s.HumType}
			if w.threatensLocked(npc, p) {
				f, _ := w.factionAggroLocked(npc)
				t.Errorf("%q (world %d) attacks a new %s %s on sight (%s %d)", npc.Name, npc.World,
					nameIn(raceNames, s.Race), nameIn(classNames, s.Class), f, w.standingLocked(p, f))
				break
			}
		}
	}
	w.mu.Unlock()
	t.Logf("%d NPCs quests talk to checked", seen)

	// every start spot, strictly with EQOA_STARTS_MEASURED set: once every
	// combo's measured starts are in (tools/seed_faction_starts.py)
	threats := w.StartThreats(spots)
	for _, l := range w.StartThreatLines(spots) {
		t.Log(l)
	}
	if os.Getenv("EQOA_STARTS_MEASURED") != "" && len(threats) > 0 {
		t.Errorf("with the measured starts, %d NPCs attack a new character at its start spot", len(threats))
	}
}

// convexHull is the hull of points on the ground (monotone chain), its
// corners anticlockwise.
func convexHull(pts [][2]float32) [][2]float32 {
	p := append([][2]float32(nil), pts...)
	sort.Slice(p, func(i, j int) bool { return p[i][0] < p[j][0] || p[i][0] == p[j][0] && p[i][1] < p[j][1] })
	if len(p) < 3 {
		return p
	}
	cross := func(o, a, b [2]float32) float32 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	var lo, hi [][2]float32
	for _, q := range p {
		for len(lo) >= 2 && cross(lo[len(lo)-2], lo[len(lo)-1], q) <= 0 {
			lo = lo[:len(lo)-1]
		}
		lo = append(lo, q)
	}
	for i := len(p) - 1; i >= 0; i-- {
		q := p[i]
		for len(hi) >= 2 && cross(hi[len(hi)-2], hi[len(hi)-1], q) <= 0 {
			hi = hi[:len(hi)-1]
		}
		hi = append(hi, q)
	}
	return append(lo[:len(lo)-1], hi[:len(hi)-1]...)
}

// inHull reports (x, z) inside or on an anticlockwise hull.
func inHull(h [][2]float32, x, z float32) bool {
	if len(h) < 3 {
		return false
	}
	for i := range h {
		a, b := h[i], h[(i+1)%len(h)]
		if (b[0]-a[0])*(z-a[1])-(b[1]-a[1])*(x-a[0]) < 0 {
			return false
		}
	}
	return true
}

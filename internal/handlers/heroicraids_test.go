package handlers

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// The Heroic Raids board and Mythic Lord Nagafen's island (work board 2429): the request's "Before publishing"
// checks at the board, and its layout on the ground.

// mythicRecipe is the repository's recipe of the island (tools/worlds/mythic_nagafen.json).
func mythicRecipe(t *testing.T) worlds.Recipe {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "tools", "worlds", "mythic_nagafen.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r worlds.Recipe
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// mythicLayout is the recipe's layout block.
type mythicLayout struct {
	Boss      [3]float32 `json:"boss"`
	Landing   [3]float32 `json:"landing"`
	Exit      [3]float32 `json:"exit"`
	EastValve [3]float32 `json:"east_valve"`
	WestValve [3]float32 `json:"west_valve"`
	Guard     [3]float32 `json:"guard"`
	Pool      [3]float32 `json:"pool"`
	Lanes     struct {
		West [3]float32 `json:"west"`
		East [3]float32 `json:"east"`
	} `json:"lanes"`
	Board struct {
		World   int
		X, Y, Z float32
	} `json:"-"`
}

func readMythicLayout(t *testing.T, r worlds.Recipe) mythicLayout {
	t.Helper()
	var ds []struct {
		Layout json.RawMessage `json:"layout"`
	}
	if err := json.Unmarshal(r.Dungeons, &ds); err != nil || len(ds) != 1 {
		t.Fatalf("the dungeons block: %v (%d)", err, len(ds))
	}
	var l mythicLayout
	if err := json.Unmarshal(ds[0].Layout, &l); err != nil {
		t.Fatal(err)
	}
	var b map[string]float32
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(ds[0].Layout, &raw)
	_ = json.Unmarshal(raw["heroic_raids"], &b)
	l.Board.World, l.Board.X, l.Board.Y, l.Board.Z = int(b["world"]), b["x"], b["y"], b["z"]
	return l
}

func mythicDistXZ(a, b [3]float32) float32 {
	return float32(math.Hypot(float64(a[0]-b[0]), float64(a[2]-b[2])))
}

// The recipe: world 18, Lavastorm's zone record 0 (its cell (2, 2)), one hidden dungeon entered only at the Heroic
// Raids board, its encounter, the guard and the pool left to it; every staged place inside its box, the landing
// and the way out beyond the boss's aggro (21), the valves on opposite sides of him, the lanes clear of the pads a
// pool keeps off.
func TestMythicNagafenRecipe(t *testing.T) {
	r := mythicRecipe(t)
	if r.World != 18 || len(r.Zones) != 1 || r.Zones[0].From != "LAVASTM.ESF" || r.Zones[0].Zone != 0 {
		t.Fatalf("the recipe: world %d zones %+v", r.World, r.Zones)
	}
	defs, err := ReadDungeons(worlds.Built{Recipe: r})
	if err != nil || len(defs) != 1 {
		t.Fatalf("its dungeons: %v %+v", err, defs)
	}
	d := defs[0]
	if d.Key != "mythic_nagafen" || d.Name != "Mythic Lord Nagafen" || d.World != 18 || d.Via != HeroicRaidsName ||
		d.Encounter != entity.EncounterMythicNagafen || len(d.Spawned) != 2 || !d.Hidden() {
		t.Fatalf("the dungeon: %+v", d)
	}
	l := readMythicLayout(t, r)
	in := func(p [3]float32) bool {
		for _, b := range d.Boxes {
			if p[0] >= b.X0 && p[0] <= b.X1 && p[2] >= b.Z0 && p[2] <= b.Z1 {
				return true
			}
		}
		return false
	}
	for name, p := range map[string][3]float32{"boss": l.Boss, "landing": l.Landing, "exit": l.Exit, "east valve": l.EastValve,
		"west valve": l.WestValve, "guard": l.Guard, "pool": l.Pool, "west lane": l.Lanes.West, "east lane": l.Lanes.East} {
		if !in(p) {
			t.Errorf("%s %v outside the dungeon's box", name, p)
		}
	}
	if e := d.Entrance; e.X != l.Landing[0] || e.Z != l.Landing[2] {
		t.Errorf("the entrance %+v is not the layout's landing %v", e, l.Landing)
	}
	for name, p := range map[string][3]float32{"landing": l.Landing, "exit": l.Exit} {
		if dd := mythicDistXZ(p, l.Boss); dd < entity.DefaultAggroRadius+20 {
			t.Errorf("the %s is %.1f from the boss: in his aggro", name, dd)
		}
	}
	if !(l.WestValve[0] < l.Boss[0] && l.EastValve[0] > l.Boss[0]) || mythicDistXZ(l.EastValve, l.WestValve) < 60 {
		t.Errorf("the valves are not on opposite sides: west %v east %v boss %v", l.WestValve, l.EastValve, l.Boss)
	}
	if !(l.Lanes.West[0] < l.Boss[0] && l.Lanes.East[0] > l.Boss[0]) {
		t.Errorf("the lanes are not on opposite flanks: %v %v", l.Lanes.West, l.Lanes.East)
	}
	pads := map[string][4]float32{"landing": {l.Landing[0], 0, l.Landing[2], 6}, "exit": {l.Exit[0], 0, l.Exit[2], 5},
		"east valve": {l.EastValve[0], 0, l.EastValve[2], 5}, "west valve": {l.WestValve[0], 0, l.WestValve[2], 5}}
	for _, lane := range [][3]float32{l.Lanes.West, l.Lanes.East} {
		for name, p := range pads {
			if dd := mythicDistXZ(lane, [3]float32{p[0], 0, p[2]}); dd < p[3]+5+0.5 {
				t.Errorf("the lane %v is %.1f from the %s pad: a pool there would reach it", lane, dd, name)
			}
		}
		if dd := mythicDistXZ(lane, l.Boss); dd > float32(entity.RunSpeed)*5 {
			t.Errorf("the lane %v is %.1f from him: more than 5 s at a run", lane, dd)
		}
	}
	if l.Board.World != 0 || l.Board.Z < 4000 || math.Hypot(float64(l.Board.X-5520), float64(l.Board.Z-3950)) > 100 {
		t.Errorf("the board %+v is not by (5520, 3950) on Tunaria", l.Board)
	}
}

// mythicPortalRig is the portal rig with the dungeons on: a test dungeon in world 14 and the island in world 18 (its
// boss, valves and way out as rows), the Heroic Raids board beside Anna, a Dungeon Warden beside her too.
func mythicPortalRig(t *testing.T) (*portalRig, *entity.NPC, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	island := mythicRecipe(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}},
		{Recipe: island}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
		entity.SetInstancedWorld(18, false)
	})
	for i, n := range []*entity.NPC{
		{ServerID: 7801, Name: entity.MythicBossName, X: 4841.72, Y: -28.25, Z: 4369.27, World: 18, Level: 65, HP: entity.MythicBossHP, MaxHP: entity.MythicBossHP},
		{ServerID: 7802, Name: entity.MythicEastValveName, X: 4884, Y: -28.72, Z: 4398, World: 18, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: 7803, Name: entity.MythicWestValveName, X: 4798, Y: -29.09, Z: 4398, World: 18, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: 7804, Name: entity.DungeonWardenName, X: 4852, Y: -29.08, Z: 4430, World: 18, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1},
	} {
		_ = i
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		r.gw.NPCs[n.ServerID] = n
	}
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true, Worlds: []int{18}}, nil) {
		t.Fatal("the dungeons are not on")
	}
	if !r.gw.InDungeonWorld(18) {
		t.Fatal("world 18 is not a dungeon's")
	}
	for _, s := range []*session.Session{r.anna, r.cora, r.bert} {
		s.SetServerWorlds([]uint8{14, 18})
	}
	board := &entity.NPC{ServerID: 7810, Name: HeroicRaidsName, X: 5001, Y: 50, Z: 5002, World: 0, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	warden := &entity.NPC{ServerID: 7811, Name: entity.DungeonWardenName, X: 5002, Y: 50, Z: 5001, World: 0, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	r.gw.NPCs[board.ServerID], r.gw.NPCs[warden.ServerID] = board, warden
	r.drain()
	return r, board, warden
}

// askOf is the board's question as Anna was sent it ("ask#id:text|answers").
func askOf(t *testing.T, got []string) string {
	t.Helper()
	for _, m := range got {
		if strings.HasPrefix(m, "ask#") {
			return m
		}
	}
	t.Fatalf("no question: %q", got)
	return ""
}

// An ordinary player at the board: its box offers Mythic Lord Nagafen alone; the answer takes her (and her group's
// member beside her) into their instance on the island; the board's box is its own mark.
func TestHeroicRaidsBoardEntry(t *testing.T) {
	r, board, _ := mythicPortalRig(t)
	r.group(t, r.anna, r.pa, r.cora, r.pc)
	r.drain()
	if !r.ih.heroicRaidsTalk(r.anna, r.pa, board) {
		t.Fatal("the board did not answer")
	}
	ask := askOf(t, r.msgs(r.anna))
	if !strings.HasSuffix(ask, "|Mythic Lord Nagafen") || strings.Count(ask, "|") != 1 {
		t.Fatalf("the board's box: %q", ask)
	}
	heroicRaids.mu.Lock()
	asked := heroicRaids.asked[r.pa.CharServerID]
	heroicRaids.mu.Unlock()
	if asked.box&0xFF000000 != heroicRaidsBoxMark || len(asked.keys) != 1 || asked.keys[0] != "mythic_nagafen" {
		t.Fatalf("asked: %+v", asked)
	}
	r.ih.answerHeroicRaids(r.anna, r.pa, asked.box, 0)
	r.travel()
	if r.pa.World != 18 || r.pa.InstanceKey() == 0 {
		t.Fatalf("Anna not on the island: world %d instance %d (%q)", r.pa.World, r.pa.InstanceKey(), r.msgs(r.anna))
	}
	if r.pc.World != 18 || r.pc.InstanceKey() != r.pa.InstanceKey() {
		t.Fatalf("Cora not taken along: world %d instance %d", r.pc.World, r.pc.InstanceKey())
	}
	if st, ok := r.gw.MythicStateOf(r.pa.InstanceKey()); !ok || st.Phase != "staging" || st.Boss == 0 {
		t.Fatalf("the fight: %+v %v", st, ok)
	}
	// inside, the warden standing there offers the way out alone
	inner := r.gw.NPCByID(7804)
	r.drain()
	r.pa.X, r.pa.Z = inner.X+1, inner.Z+1
	dungeonsH.Load().askRoot(r.anna, r.pa, inner, 0)
	dungeonsH.Load().mu.Lock()
	in := dungeonsH.Load().asked[r.pa.CharServerID]
	dungeonsH.Load().mu.Unlock()
	if len(in.answers) != 1 || in.answers[0].do != dungeonsDoOut {
		t.Fatalf("the island's warden offers %+v", in.answers)
	}
}

// A dialogue left open and answered from afar does nothing (the answer's own distance check); a talk from beyond
// reach gets no box.
func TestHeroicRaidsStaleDialogueDistance(t *testing.T) {
	r, board, _ := mythicPortalRig(t)
	r.ih.heroicRaidsTalk(r.anna, r.pa, board)
	heroicRaids.mu.Lock()
	asked := heroicRaids.asked[r.pa.CharServerID]
	heroicRaids.mu.Unlock()
	if asked.box == 0 {
		t.Fatal("no box asked")
	}
	r.pa.X += heroicRaidsReach + 5 // walked off with the box open
	r.drain()
	r.ih.answerHeroicRaids(r.anna, r.pa, asked.box, 0)
	r.travel()
	if r.pa.World != 0 || r.pa.InstanceKey() != 0 {
		t.Fatalf("an answer from %.0f away took her in", heroicRaidsReach+5)
	}
	if got := r.msgs(r.anna); !hasMsg(got, "colour:"+heroicRaidsTooFar) {
		t.Fatalf("not told: %q", got)
	}
	// and a talk from beyond reach is no talk
	r.drain()
	r.ih.heroicRaidsTalk(r.anna, r.pa, board)
	for _, m := range r.msgs(r.anna) {
		if strings.HasPrefix(m, "ask#") {
			t.Fatalf("a talk from afar got a box: %q", m)
		}
	}
}

// Nothing else names the raid or lets a party in by its key: /dungeon enter by key or name is answered as no dungeon
// at all, /dungeon's help names no hidden key, a Dungeon Warden's and the Heroic Dungeons board's boxes leave it out.
func TestHeroicRaidsRemoteKeyRefused(t *testing.T) {
	r, _, warden := mythicPortalRig(t)
	for _, key := range []string{"mythic_nagafen", "Mythic Lord Nagafen"} {
		r.drain()
		dungeonCommand(r.gw, r.anna, "enter "+key)
		r.travel()
		if r.pa.World != 0 {
			t.Fatalf("/dungeon enter %s took her in", key)
		}
		if got := r.msgs(r.anna); !hasMsg(got, "colour:There is no dungeon named*") {
			t.Fatalf("/dungeon enter %s: %q", key, got)
		}
	}
	if line := dungeonsHelpLine(r.gw); strings.Contains(line, "mythic") || !strings.Contains(line, "bb") {
		t.Fatalf("/dungeon's help: %q", line)
	}
	board := &entity.NPC{ServerID: 7812, Name: entity.DungeonRaidBoardName, X: 5000, Y: 50, Z: 5001, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	r.gw.NPCs[board.ServerID] = board
	for _, npc := range []*entity.NPC{warden, board} {
		dungeonsH.Load().askRoot(r.anna, r.pa, npc, 0)
		dungeonsH.Load().mu.Lock()
		a := dungeonsH.Load().asked[r.pa.CharServerID]
		dungeonsH.Load().mu.Unlock()
		for _, x := range a.answers {
			if strings.Contains(x.label, "Nagafen") || x.key == "mythic_nagafen" {
				t.Fatalf("%s offers %+v", npc.Name, x)
			}
		}
	}
}

// The island's places on the baked ground (EQOA_GROUND_DIR with world 18 baked; world 0's too for the board):
// each stands on the terrain, dry (no lava at its feet), the landing reaches every valve, the way out and both lanes
// over dry ground a player walks, from the ground round him a lane is reached in the brand's 6 s at a run, and
// from every dry place in his breath's cone (aimed at each staged place) dry ground outside it is reached in the
// warning's 7 s.
func TestMythicNagafenLayoutOnTheGround(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names the baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !env.Knows(18) {
		t.Skip("world 18 is not baked here")
	}
	l := readMythicLayout(t, mythicRecipe(t))
	dry := func(w uint8, x, y, z float32) (float32, bool) {
		h, placed, roofed, ok := g.Stand(w, x, y+2, z)
		if !ok || placed || roofed {
			return h, false
		}
		f, known := env.Flags(w, x, h+0.1, z)
		return h, known && f&4 == 0
	}
	spots := map[string][3]float32{"boss": l.Boss, "landing": l.Landing, "exit": l.Exit, "east valve": l.EastValve,
		"west valve": l.WestValve, "guard": l.Guard, "west lane": l.Lanes.West, "east lane": l.Lanes.East}
	for name, p := range spots {
		h, ok := dry(18, p[0], p[1], p[2])
		if !ok || math.Abs(float64(h-p[1])) > 0.5 {
			t.Errorf("the %s %v: floor %.2f dry %v", name, p, h, ok)
		}
	}
	if env.Knows(0) {
		if h, ok := dry(0, l.Board.X, l.Board.Y, l.Board.Z); !ok || math.Abs(float64(h-l.Board.Y)) > 0.5 {
			t.Errorf("the Heroic Raids board %+v: floor %.2f dry %v", l.Board, h, ok)
		}
	}
	// the dry ground a player walks from the landing: a grid of 1, a step no steeper than 1:1 plus 1.3
	const step = 1.0
	type cell struct{ i, j int }
	x0, z0 := float32(4740), float32(4280)
	at := func(c cell) (float32, float32) { return x0 + float32(c.i)*step, z0 + float32(c.j)*step }
	of := func(x, z float32) cell {
		return cell{int(math.Round(float64((x - x0) / step))), int(math.Round(float64((z - z0) / step)))}
	}
	height := map[cell]float32{}
	dist := map[cell]float32{}
	start := of(l.Landing[0], l.Landing[2])
	height[start], _ = dry(18, l.Landing[0], l.Landing[1], l.Landing[2])
	dist[start] = 0
	queue := []cell{start}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := cell{c.i + d[0], c.j + d[1]}
			if _, seen := height[n]; seen || n.i < 0 || n.j < 0 || n.i > 220 || n.j > 200 {
				continue
			}
			x, z := at(n)
			h, ok := dry(18, x, height[c], z)
			if !ok || h-height[c] > step*1.0+1.3 {
				continue
			}
			if cx, cz := at(c); !g.Sees(18, cx, height[c]+1.2, cz, x, h+1.2, z) {
				continue // a rock between
			}
			height[n], dist[n] = h, dist[c]+step
			queue = append(queue, n)
		}
	}
	for name, p := range spots {
		if name == "boss" || name == "guard" {
			continue
		}
		if _, ok := dist[of(p[0], p[2])]; !ok {
			t.Errorf("the %s %v is not reached over dry ground from the landing", name, p)
		}
	}
	t.Logf("%d dry places reached from the landing", len(dist))
	// a walk's length between two places over the dry ground (a breadth-first search on the grid)
	walk := func(from cell, done func(cell) bool, most float32) (float32, bool) {
		d := map[cell]float32{from: 0}
		q := []cell{from}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			if done(c) {
				return d[c], true
			}
			if d[c] >= most {
				continue
			}
			for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := cell{c.i + o[0], c.j + o[1]}
				if _, ok := dist[n]; !ok {
					continue
				}
				if _, seen := d[n]; seen {
					continue
				}
				d[n] = d[c] + step
				q = append(q, n)
			}
		}
		return 0, false
	}
	run := float32(entity.RunSpeed)
	// from the ground round him (8 to 20 off his middle) a lane in 6 s at a run (less a second to react)
	for c := range dist {
		x, z := at(c)
		r := math.Hypot(float64(x-l.Boss[0]), float64(z-l.Boss[2]))
		if r < 8 || r > 20 || c.i%4 != 0 || c.j%4 != 0 {
			continue
		}
		lane := func(n cell) bool {
			nx, nz := at(n)
			return math.Hypot(float64(nx-l.Lanes.West[0]), float64(nz-l.Lanes.West[2])) <= 2 ||
				math.Hypot(float64(nx-l.Lanes.East[0]), float64(nz-l.Lanes.East[2])) <= 2
		}
		if d, ok := walk(c, lane, run*5); !ok {
			t.Errorf("from (%.0f, %.0f) no lane within %.0f (5 s at a run)", x, z, run*5)
		} else if d > run*5 {
			t.Errorf("from (%.0f, %.0f) a lane is %.0f off", x, z, d)
		}
	}
	// the breath's cone aimed at each staged place: from each dry place in it, out of it in 6 s
	for name, p := range spots {
		if name == "boss" {
			continue
		}
		dx, dz := p[0]-l.Boss[0], p[2]-l.Boss[2]
		n := float32(math.Hypot(float64(dx), float64(dz)))
		dx, dz = dx/n, dz/n
		cos := float32(math.Cos(50 * math.Pi / 180))
		inCone := func(c cell) bool {
			x, z := at(c)
			ox, oz := x-l.Boss[0], z-l.Boss[2]
			d := float32(math.Hypot(float64(ox), float64(oz)))
			return d <= 35 && (d < 0.01 || (ox*dx+oz*dz)/d >= cos)
		}
		for c := range dist {
			if !inCone(c) || c.i%3 != 0 || c.j%3 != 0 {
				continue
			}
			if _, ok := walk(c, func(n cell) bool { return !inCone(n) }, run*6); !ok {
				x, z := at(c)
				t.Errorf("breath toward the %s: from (%.0f, %.0f) no dry way out of the cone in 6 s", name, x, z)
			}
		}
	}
}

package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Plane of Sky's teleporters on a real server's data (EQOA_REAL_DB, a copy
// opened) over its baked ground (EQOA_GROUND_DIR, world 4's files): the owner,
// 2026-11-13, "lets look at Plane of Sky teleporters make sure they work".
// Skipped without both.

// skyTrip is one way onto or across the Plane of Sky (world 4): an essence's
// passage, a waysender's trip, a GM portal's, a quest step's or a trigger's
// teleport, the Teleporter's.
type skyTrip struct {
	label string
	at    wayPlace
}

// openRealSky opens a copy of EQOA_REAL_DB over EQOA_GROUND_DIR, the NPCs
// loaded.
func openRealSky(t *testing.T) (*database.SQLiteDB, *entity.World, *ground.Ground, *ground.Env) {
	t.Helper()
	dir, src := os.Getenv("EQOA_GROUND_DIR"), os.Getenv("EQOA_REAL_DB")
	if dir == "" || src == "" {
		t.Skip("EQOA_GROUND_DIR and EQOA_REAL_DB name a baked ground and a server database")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
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
	t.Cleanup(func() { db.Close() })
	w := entity.NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	w.SetGround(g)
	w.SetEnv(env)
	return db, w, g, env
}

// skyWalk is whether a walker set down at from reaches within reach of to
// (horizontally, and within 8 of its height) over world 4's floors, the
// terrain's and what is placed on it: steps of 2 that climb at most
// ground.StepUp, drop at most maxDrop and meet no wall.
func skyWalk(g *ground.Ground, from, to [3]float32, reach, maxDrop float32, limit int) bool {
	const step = 2
	type node struct{ ix, iz, iy int32 }
	key := func(x, y, z float32) node {
		return node{int32(math.Round(float64(x / step))), int32(math.Round(float64(z / step))), int32(math.Floor(float64(y / 3)))}
	}
	fy, ok := g.Support(4, from[0], from[1], from[2], ground.StepUp, 6)
	if !ok {
		return false
	}
	type pt struct{ x, y, z float32 }
	q := []pt{{from[0], fy, from[2]}}
	seen := map[node]bool{key(from[0], fy, from[2]): true}
	dirs := [][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for len(q) > 0 && len(seen) < limit {
		c := q[0]
		q = q[1:]
		if math.Hypot(float64(c.x-to[0]), float64(c.z-to[2])) <= float64(reach) && math.Abs(float64(c.y-to[1])) <= 8 {
			return true
		}
		for _, d := range dirs {
			nx, nz := c.x+d[0]*step, c.z+d[1]*step
			ny, ok := g.Support(4, nx, c.y, nz, ground.StepUp, maxDrop)
			if !ok {
				continue
			}
			k := key(nx, ny, nz)
			if seen[k] || g.Wall(4, c.x, c.y+1, c.z, nx, ny+1, nz) {
				continue
			}
			seen[k] = true
			q = append(q, pt{nx, ny, nz})
		}
	}
	return false
}

// skyOrdinals are "first".."fifth" as 1..5.
var skyOrdinals = map[string]int{"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5}

// skyNextKey is the key the isle a passage lands on wants ("the third isle of
// rain": Rain Key 3); "" for the Isle of Wonder.
func skyNextKey(to string) string {
	f := strings.Fields(strings.ToLower(to))
	if len(f) != 5 || f[0] != "the" || f[2] != "isle" {
		return ""
	}
	n, ok := skyOrdinals[f[1]]
	if !ok {
		return ""
	}
	chain := map[string]string{"rain": "Rain", "day": "Day", "night": "Night", "snow": "Snow", "desolation": "Desolation"}[f[4]]
	return fmt.Sprintf("%s Key %d", chain, n)
}

// skyJumps are the isles whose way on starts with a jump (no fall hurts on
// this server): "From where you port in, turn right and you'll need to jump
// off this island, to reach the pyramid of the king" (plane-of-sky, night 5).
var skyJumps = map[string]float32{"the fifth isle of night": 150}

// skyTeleports are the teleports of a JSON events or trigger steps list into
// world 4.
func skyTeleports(text string) []wayPlace {
	var raw []map[string]any
	if json.Unmarshal([]byte(text), &raw) != nil {
		return nil
	}
	var out []wayPlace
	for _, e := range raw {
		if e["do"] != "teleport" {
			continue
		}
		if wd, _ := e["world"].(float64); int(wd) != 4 {
			continue
		}
		x, _ := e["x"].(float64)
		y, _ := e["y"].(float64)
		z, _ := e["z"].(float64)
		out = append(out, wayPlace{World: 4, X: float32(x), Y: float32(y), Z: float32(z)})
	}
	return out
}

// Every way onto and across the Plane of Sky lands on a floor of world 4 at
// its height (the terrain's or a hut's), out of the water; each passage's
// essence stands where the passage says and its key is a record a creature
// carries or a quest gives; each isle's landing walks to the essence that
// takes the isle's key (the fifth isle of night's by the guide's jump).
func TestRealSkyTeleporters(t *testing.T) {
	db, w, g, env := openRealSky(t)

	var trips []skyTrip
	for _, p := range skyPasses {
		trips = append(trips, skyTrip{fmt.Sprintf("pass %s (%.0f, %.0f) -> %s", p.NPC, p.X, p.Z, p.To), p.At})
	}
	var names []string
	for n := range wayRoutes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, r := range wayRoutes[n] {
			if r.At != nil && r.At.World == 4 {
				trips = append(trips, skyTrip{"way " + n + " -> " + r.To, *r.At})
			}
		}
	}
	for _, p := range telePlaces {
		if p.at.World == 4 {
			trips = append(trips, skyTrip{"Teleporter -> " + p.label, p.at})
		}
	}
	for _, r := range teleRaids {
		if r.at != nil && r.at.World == 4 {
			trips = append(trips, skyTrip{"Teleporter -> " + r.name, *r.at})
		}
	}
	portals, err := db.EnabledGMPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		if p.World == 4 {
			trips = append(trips, skyTrip{fmt.Sprintf("portal %s (row %d) -> %s", p.NPCName, p.NPCID, p.Place),
				wayPlace{World: 4, X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)}})
		}
	}
	rows, err := db.Query(`SELECT questid, step, target, events FROM queststeps WHERE events LIKE '%teleport%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var qid, step int
		var target, ev string
		if err := rows.Scan(&qid, &step, &target, &ev); err != nil {
			t.Fatal(err)
		}
		for _, at := range skyTeleports(ev) {
			trips = append(trips, skyTrip{fmt.Sprintf("quest %d step %d at %s", qid, step, target), at})
		}
	}
	rows.Close()
	rows, err = db.Query(`SELECT questid, name, events FROM quests WHERE events LIKE '%teleport%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var qid int
		var name, ev string
		if err := rows.Scan(&qid, &name, &ev); err != nil {
			t.Fatal(err)
		}
		for _, at := range skyTeleports(ev) {
			trips = append(trips, skyTrip{fmt.Sprintf("quest %d (%s)", qid, name), at})
		}
	}
	rows.Close()
	rows, err = db.Query(`SELECT slot, name, steps FROM gmtriggers WHERE steps LIKE '%teleport%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slot int
		var name, steps string
		if err := rows.Scan(&slot, &name, &steps); err != nil {
			t.Fatal(err)
		}
		for _, at := range skyTeleports(steps) {
			trips = append(trips, skyTrip{fmt.Sprintf("trigger %d (%s)", slot, name), at})
		}
	}
	rows.Close()
	if len(trips) < 40 {
		t.Fatalf("only %d trips into world 4", len(trips))
	}

	for _, tr := range trips {
		a := tr.at
		fy, on := g.Support(4, a.X, a.Y, a.Z, 2, 40)
		if !on {
			t.Errorf("%s: no floor under (%.1f, %.2f, %.1f)", tr.label, a.X, a.Y, a.Z)
		} else if d := a.Y - fy; d > 3 || d < -2 {
			t.Errorf("%s: lands at %.2f over a floor at %.2f", tr.label, a.Y, fy)
		}
		for _, y := range []float32{0.5, 1.5} {
			if f, ok := env.Flags(4, a.X, a.Y+y, a.Z); ok && f != 0 {
				t.Errorf("%s: in water, lava or slime (%d)", tr.label, f)
			}
		}
		if _, err := ValidateTeleport(a.X, a.Y, a.Z, a.World); err != nil {
			t.Errorf("%s: %v", tr.label, err)
		}
		t.Logf("%-66s (%.1f, %.2f, %.1f)", tr.label, a.X, a.Y, a.Z)
	}

	// world 4's NPCs by name
	spawnOf := func(name string, x, z float32) ([3]float32, bool) {
		for _, n := range w.NPCs {
			if n.World == 4 && !n.Temporary && sameName(n.Name, name) &&
				(x == 0 && z == 0 || math.Hypot(float64(n.SpawnX-x), float64(n.SpawnZ-z)) <= skyPassReach) {
				return [3]float32{n.SpawnX, n.SpawnY, n.SpawnZ}, true
			}
		}
		return [3]float32{}, false
	}
	carried := func(item string) int {
		var n int
		db.QueryRow(`SELECT (SELECT count(*) FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid
				WHERE lower(trim(p.itemname)) = lower(?1))
			+ (SELECT count(*) FROM queststeps s JOIN itempattern p ON lower(trim(p.itemname)) = lower(?1)
				WHERE (',' || replace(s.rewarditems, ' ', '') || ',') LIKE '%,' || p.patternid || ',%')`, item).Scan(&n)
		return n
	}
	for _, p := range skyPasses {
		if _, ok := spawnOf(p.NPC, p.X, p.Z); !ok {
			t.Errorf("pass %s (%.0f, %.0f): no such NPC in world 4", p.NPC, p.X, p.Z)
		}
		if carried(p.Key) == 0 {
			t.Errorf("pass %s (%.0f, %.0f): no creature carries a %s and no quest gives one", p.NPC, p.X, p.Z, p.Key)
		}
	}

	// each isle's landing walks to the essence that takes its key
	for _, p := range skyPasses {
		want := skyNextKey(p.To)
		if want == "" {
			continue
		}
		var next *skyPass
		for j := range skyPasses {
			if strings.EqualFold(skyPasses[j].Key, want) && skyPasses[j].X != 0 {
				next = &skyPasses[j]
			}
		}
		if next == nil {
			t.Errorf("%s: no essence takes a %s", p.To, want)
			continue
		}
		at, ok := spawnOf(next.NPC, next.X, next.Z)
		if !ok {
			continue // said above
		}
		drop := float32(6)
		if j, ok := skyJumps[p.To]; ok {
			drop = j
		}
		if !skyWalk(g, [3]float32{p.At.X, p.At.Y, p.At.Z}, at, 12, drop, 400000) {
			t.Errorf("%s: its landing does not walk to its %s at (%.0f, %.0f)", p.To, next.NPC, next.X, next.Z)
		}
	}
}

// skyTalker is a player in world 4 talking to the real world's NPCs: the
// interact handler with the database's GM portals and quest engine.
type skyTalker struct {
	t *testing.T
	w *entity.World
	h *InteractHandler
	s *session.Session
	p *entity.Player
}

func newSkyTalker(t *testing.T, db *database.SQLiteDB, w *entity.World) *skyTalker {
	t.Helper()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetLoadableWorlds(LoadableWorlds)
	h := NewInteractHandler(mgr)
	h.SetGameWorld(w)
	h.portals = NewGMPortals(db)
	qe, err := NewQuestEngine(db, w, mgr, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7009}, 9, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1009, CharServerID: 990000009, Name: "Skywalker", World: 4, Level: 60, HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
	s.SetInventoryList(newInvList(nil))
	return &skyTalker{t: t, w: w, h: h, s: s, p: p}
}

// talkTo stands the player 3 from the NPC of row id and talks to it; it
// returns the object id the client knows it by.
func (st *skyTalker) talkTo(id entity.EntityID) uint32 {
	st.t.Helper()
	n := st.w.NPCs[id]
	if n == nil {
		st.t.Fatalf("no NPC of row %d", id)
	}
	st.p.World, st.p.X, st.p.Y, st.p.Z = 4, n.SpawnX+3, n.SpawnY, n.SpawnZ
	st.s.Queue().GetPending()
	vs := entity.GetOrCreateVisibility(st.s)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = n.ServerID, 100000
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteract)
	msg.SetData(u32b(100000))
	if err := st.h.HandleInteract(st.s, msg); err != nil {
		st.t.Fatal(err)
	}
	return 100000
}

// The Isle of Wonder is reached from the main isle, and the main isle from it:
// the main isle's Island of Wonder (row 119498, an essence's body beside
// Sartheret) asks Yes or No to the Isle of Wonder and Yes lands where every
// chain's last essence does; the Isle of Wonder's astral essence (row 119523)
// asks the way back to the main isle and lands where the Sky Portal's trip
// does (tools/sky_wonder_portals_20261113.py). No walk joins the two; each
// landing walks to whom it is for.
func TestRealSkyWonderPortals(t *testing.T) {
	db, w, g, _ := openRealSky(t)
	main, wonder := [3]float32{4834.8, 250.1, 5524.7}, [3]float32{skyIsleOfWonder.X, skyIsleOfWonder.Y, skyIsleOfWonder.Z}
	if skyWalk(g, main, wonder, 12, 6, 600000) {
		t.Errorf("the main isle walks to the Isle of Wonder: no portal is needed")
	}
	st := newSkyTalker(t, db, w)
	for _, c := range []struct {
		row      entity.EntityID
		ask      string
		to       [3]float32
		whom     []string
		isOnWord string
	}{
		{119498, "Do you wish to travel to the Isle of Wonder?", wonder, []string{"Mark of the Majestic", "Mark of Desolation", "manifestation of rain", "Manifestation of Dreams"}, "Island of Wonder"},
		{119523, "Do you wish to travel to the main isle?", main, []string{"Sartheret", "Liztin Misseliania", "Island of Wonder"}, "astral essence"},
	} {
		obj := st.talkTo(c.row)
		q, a := questionBox(t, st.s)
		if q != c.ask || len(a) != 2 || a[0] != "Yes" {
			t.Errorf("%s (row %d) asked %q %q, want %q Yes or No", c.isOnWord, c.row, q, a, c.ask)
			continue
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDialogueBoxOption)
		msg.SetData(append(u32b(obj), 0))
		if err := st.h.HandleDialogueOption(st.s, msg); err != nil {
			t.Fatal(err)
		}
		x, z, world, moved := teleported(st.s)
		if !moved || world != 4 || math.Hypot(float64(x-c.to[0]), float64(z-c.to[2])) > 0.5 {
			t.Errorf("%s: Yes sent the player to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f)", c.isOnWord, x, z, world, moved, c.to[0], c.to[2])
			continue
		}
		for _, name := range c.whom {
			var at [3]float32
			found := false
			for _, n := range w.NPCs {
				if n.World == 4 && sameName(n.Name, name) {
					at, found = [3]float32{n.SpawnX, n.SpawnY, n.SpawnZ}, true
				}
			}
			if !found {
				t.Errorf("no %s in world 4", name)
			} else if !skyWalk(g, c.to, at, 12, 6, 400000) {
				t.Errorf("%s's landing does not walk to %s", c.isOnWord, name)
			}
		}
	}
	// the main isle's other astral essence (the twins' quest) is no portal
	st.talkTo(162500)
	if q, _ := questionBox(t, st.s); strings.HasPrefix(q, "Do you wish to travel") {
		t.Errorf("the main isle's astral essence asked %q", q)
	}
}

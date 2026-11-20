package entity

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/session"
)

// The tick rig: a real server's world (EQOA_REAL_DB, a copy of its database,
// and EQOA_GROUND_DIR, its baked ground) with simulated players standing and
// walking where the database's characters last stood, two in three of them
// vanilla clients that asked for every slot and look (204 slots within 1000,
// the live look, armour, auras, decorations) and the rest PS2s; each tick is
// the server's Tick, then each session's object channels are read and sent as
// the listener's sends read them (GenerateXORDelta, acknowledged at once) and
// its queued messages taken. What every client was sent is a digest a tick.
//
//	EQOA_REAL_DB=... EQOA_GROUND_DIR=... go test ./internal/entity/ -run TestTickRig -v
//
// EQOA_RIG_PLAYERS (36), EQOA_RIG_TICKS (600), EQOA_RIG_SEED (1) set the run
// (EQOA_RIG_LOG keeps the server's log lines); EQOA_RIG_DIGESTS=file writes a
// line a tick (the tick, what the clients were sent, the NPCs' places and
// states) for a run of another build to be compared with (cmp), in a run that
// repeats itself: EQOA_RIG_LAYOUT=one (newTickRig). TestTickRigChannels
// checks every channel against its entity at every tick's end. Skipped
// without both variables: neither is in this repository.

// rigPlayer is one simulated player: its session, where it stands, and
// whether it walks (a circle round its place) or stands.
type rigPlayer struct {
	sess    *session.Session
	p       *Player
	cx, cz  float32
	walks   bool
	phase   float64
	vanilla bool
}

type tickRig struct {
	w       *World
	players []*rigPlayer
	now     time.Time
	tick    int
	sent    int // bytes of channel deltas sent, for the figures
	msgs    int
}

// rigEnv is an environment variable as an int, or def.
func rigEnv(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// newTickRig loads the world as cmd/gameserver does (the parts a tick reads:
// the NPCs, the ground and water, paths, camps, rare cycles, factions, social
// and movement rules, titles, spawn-only rows, tints, outfits, decorations,
// dyes, auras, no-roam areas) and puts n players in it.
func newTickRig(tb testing.TB, n int, seed int64) *tickRig {
	tb.Helper()
	src, dir := os.Getenv("EQOA_REAL_DB"), os.Getenv("EQOA_GROUND_DIR")
	if src == "" || dir == "" {
		tb.Skip("EQOA_REAL_DB and EQOA_GROUND_DIR name a server database and its baked ground")
	}
	if os.Getenv("EQOA_RIG_LOG") == "" { // the load's line an NPC, and the systems' lines
		log.SetOutput(io.Discard)
		tb.Cleanup(func() { log.SetOutput(os.Stderr) })
	}
	path := filepath.Join(tb.TempDir(), "eqoa.db")
	in, err := os.Open(src)
	if err != nil {
		tb.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		tb.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	rigChecksOff(tb) // the server's own work, not this package's tests' checks (tickrig_hooks_test.go)
	store := database.NewNPCStore(db)
	w := NewWorld()
	if m, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(m)
	}
	if m, err := store.RaceModels(); err == nil {
		w.SetRaceModels(m)
	}
	if m, err := store.ModelSexes(); err == nil {
		w.SetModelSexes(m)
	}
	if m, err := store.ModelRaces(); err == nil {
		w.SetModelRaces(m)
	}
	w.LoadNPCs(store)
	if _, _, err := w.LoadBalance(db); err != nil {
		tb.Logf("balance: %v", err)
	}
	g, err := ground.Open(dir)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { g.Close() })
	w.SetGround(g)
	if e, err := ground.OpenEnv(dir); err == nil && e.Worlds() > 0 {
		w.SetEnv(e)
	}
	if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
		w.LoadNPCPaths(recs)
	}
	for name, load := range map[string]func() error{
		"camps":       func() error { _, err := w.LoadCampSpawns(db); return err },
		"rare cycles": func() error { _, err := w.LoadRareCycles(db); return err },
		"factions":    func() error { return w.LoadFactions(db) },
		"aggro rules": func() error { _, err := w.LoadAggroRules(db); return err },
		"social":      func() error { _, err := w.LoadSocialRules(db); return err },
		"move rules":  func() error { _, err := w.LoadMoveRules(db); return err },
		"row wander":  func() error { _, err := w.LoadRowWander(db); return err },
		"titles":      func() error { _, err := w.LoadNPCTitles(db); return err },
		"spawn only":  func() error { _, _, err := w.LoadSpawnOnly(db); return err },
		"tints":       func() error { _, _, err := w.LoadNPCTints(db); return err },
		"outfits":     func() error { _, err := w.LoadItemOutfits(db); return err },
		"decor":       func() error { _, _, _, _, err := w.LoadDecorLights(db); return err },
		"dyes":        func() error { _, err := w.LoadNPCDyes(db); return err },
		"pet dyes":    func() error { _, err := w.LoadPetDyeAreas(db); return err },
		"auras":       func() error { _, err := w.LoadItemAuras(db); return err },
		"no roam":     func() error { _, err := w.LoadNoRoam(db); return err },
	} {
		if err := load(); err != nil {
			tb.Logf("%s: %v", name, err)
		}
	}
	w.roamRnd = rand.New(rand.NewSource(seed))

	// where the characters last stood, the most recent first
	rows, err := db.Query(`SELECT serverid, modelid, tclass, race, level, world, x, y, z FROM characters
		WHERE world BETWEEN 0 AND 5 AND x != 0 ORDER BY serverid DESC LIMIT ?`, n)
	if err != nil {
		tb.Fatal(err)
	}
	type spot struct {
		id                 int64
		model              int64
		class, race, level int
		world              int
		x, y, z            float64
	}
	var spots []spot
	for rows.Next() {
		var s spot
		if err := rows.Scan(&s.id, &s.model, &s.class, &s.race, &s.level, &s.world, &s.x, &s.y, &s.z); err != nil {
			tb.Fatal(err)
		}
		spots = append(spots, s)
	}
	rows.Close()
	sort.Slice(spots, func(i, j int) bool { return spots[i].id < spots[j].id })

	mgr := session.NewManager()
	w.SessionManager = mgr
	r := &tickRig{w: w, now: time.Unix(1_800_000_000, 0)}
	rnd := rand.New(rand.NewSource(seed + 1))
	// EQOA_RIG_LAYOUT=one: every player at the first one's place, walking
	// its circle in step, so that the world's own orders (the wanderers
	// found player by player, in the map's order) do not depend on which
	// player comes first: a run whose digests repeat, for comparing builds
	// (EQOA_RIG_SPOT=k: the k-th player's place, 0 by default)
	one := os.Getenv("EQOA_RIG_LAYOUT") == "one"
	at := min(rigEnv("EQOA_RIG_SPOT", 0), len(spots)-1)
	for i, s := range spots {
		if one {
			s.world, s.x, s.y, s.z = spots[at].world, spots[at].x, spots[at].y, spots[at].z
		}
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 1, byte(i/250), byte(i%250+1)), Port: 7000 + i}, uint32(i+1), uint32(i+1), 1, 2, true)
		p := &Player{ServerID: EntityID(2_000_000 + i), CharServerID: uint32(s.id), Name: fmt.Sprintf("Rig%02d", i),
			ModelID: s.model, Class: s.class, Race: s.race, Level: max(s.level, 1), World: uint8(s.world),
			HP: 1000, MaxHP: 1000, Power: 100, MaxPower: 100,
			X: float32(s.x), Y: float32(s.y), Z: float32(s.z)}
		w.mu.Lock()
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		w.mu.Unlock()
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		sess.SetPlayerObjectID(500_000_000 + uint32(i))
		rp := &rigPlayer{sess: sess, p: p, cx: p.X, cz: p.Z, walks: i%3 != 2, phase: rnd.Float64() * 2 * math.Pi, vanilla: i%3 != 1}
		if one {
			// and at level 60 with no hit points to lose: no creature that
			// fights them (a fight's rolls are the global random source's)
			rp.walks, rp.phase = true, 0
			p.Level = 60
		}
		if rp.vanilla {
			conn := sess.Connection()
			conn.SetEntitySlots(session.MaxEntitySlots)
			conn.SetViewDistance(session.MaxViewDistance)
			sess.SetLiveLook(true)
			sess.SetArmorLook(true)
			sess.SetPieceAuras(true)
			sess.SetBodyAura(true)
			sess.SetCapeAura(true)
			sess.SetHandAura(true)
			sess.SetSecondAura(true)
			sess.SetHatAura(true)
			sess.SetDecorLights(DecorPetArtLayout)
		}
		r.players = append(r.players, rp)
	}
	return r
}

// step is one tick: the walkers move along their circles (a walk's pace, a
// 0x40's report each tick), the world ticks, and each session is sent what
// its channels and queue hold. It returns that tick's digest.
func (r *tickRig) step() [32]byte {
	r.tick++
	r.now = r.now.Add(100 * time.Millisecond)
	for _, rp := range r.players {
		if !rp.walks {
			continue
		}
		a := rp.phase + float64(r.tick)*0.02 // 30 units round at ~0.6 a tick
		x := rp.cx + float32(30*math.Cos(a))
		z := rp.cz + float32(30*math.Sin(a))
		r.w.UpdatePosition(rp.p.ServerID, x, rp.p.Y, z, float32(a))
	}
	r.w.Tick(r.now)
	h := sha256.New()
	var b [8]byte
	for _, rp := range r.players {
		conn := rp.sess.Connection()
		for slot, ch := range conn.GetServerObjects() {
			if ch == nil {
				continue
			}
			data, seq, xor := ch.GenerateXORDelta()
			if data == nil {
				continue
			}
			binary.LittleEndian.PutUint32(b[:4], uint32(rp.p.ServerID))
			binary.LittleEndian.PutUint16(b[4:6], uint16(slot))
			binary.LittleEndian.PutUint16(b[6:8], seq)
			h.Write(b[:])
			h.Write([]byte{xor})
			h.Write(data)
			r.sent += len(data)
			ch.UpdateBaseXor(seq) // the client's ack, at once
		}
		for _, m := range rp.sess.Queue().GetPending() {
			binary.LittleEndian.PutUint32(b[:4], uint32(rp.p.ServerID))
			h.Write(b[:4])
			h.Write(m.Data())
			r.msgs++
		}
		rp.sess.Queue().Clear()
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// npcDigest is the NPCs' places and states, by id.
func (r *tickRig) npcDigest() [32]byte {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ids := make([]EntityID, 0, len(r.w.NPCs))
	for id := range r.w.NPCs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	h := sha256.New()
	for _, id := range ids {
		n := r.w.NPCs[id]
		fmt.Fprintf(h, "%d %08x %08x %08x %08x %d %v %v %08x %08x %v\n", id, math.Float32bits(n.X), math.Float32bits(n.Y), math.Float32bits(n.Z),
			math.Float32bits(n.Facing), n.Loco, n.roam.walking, n.Dead, math.Float32bits(n.roam.toX), math.Float32bits(n.roam.toZ), n.roam.restUntil.UnixNano())
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// TestTickRig runs the rig and reports what a tick took, by system, and
// writes the digests (EQOA_RIG_DIGESTS) for an equivalence run.
func TestTickRig(t *testing.T) {
	n, ticks, seed := rigEnv("EQOA_RIG_PLAYERS", 36), rigEnv("EQOA_RIG_TICKS", 600), int64(rigEnv("EQOA_RIG_SEED", 1))
	r := newTickRig(t, n, seed)
	t.Logf("%d NPCs, %d players (%d vanilla)", len(r.w.NPCs), len(r.players), countVanilla(r.players))
	var out *os.File
	if f := os.Getenv("EQOA_RIG_DIGESTS"); f != "" {
		var err error
		if out, err = os.Create(f); err != nil {
			t.Fatal(err)
		}
		defer out.Close()
	}
	warm := min(100, ticks/4)
	var laps [tsCount]time.Duration
	var lapMax [tsCount]time.Duration
	var total, most time.Duration
	var turns []time.Duration
	var mem runtimeMem // the steps' own, the digests' left out
	for i := 0; i < ticks; i++ {
		m0 := readMem()
		t0 := time.Now()
		d := r.step()
		took := time.Since(t0)
		if m1 := readMem(); i >= warm {
			mem.mallocs += m1.mallocs - m0.mallocs
			mem.bytes += m1.bytes - m0.bytes
			mem.gc += m1.gc - m0.gc
		}
		if out != nil {
			nd := r.npcDigest()
			fmt.Fprintf(out, "%d %x %x\n", r.tick, d, nd)
		}
		if i < warm {
			continue
		}
		total += took
		most = max(most, took)
		turns = append(turns, took)
		for k := range laps {
			laps[k] += r.w.ticks.lap[k]
			lapMax[k] = max(lapMax[k], r.w.ticks.lap[k])
		}
	}
	m := ticks - warm
	sort.Slice(turns, func(i, j int) bool { return turns[i] < turns[j] })
	t.Logf("%d ticks measured: %.2f ms a step on average (tick + sends), p50 %.2f, p95 %.2f, most %.2f; %d channel bytes, %d messages",
		m, float64(total)/float64(m)/1e6, ms(turns[len(turns)/2]), ms(turns[len(turns)*95/100]), ms(most), r.sent, r.msgs)
	t.Logf("allocations a tick: %.0f (%.1f KB); GC cycles %d", float64(mem.mallocs)/float64(m), float64(mem.bytes)/float64(m)/1024, mem.gc)
	type lapLine struct {
		name     string
		avg, top time.Duration
	}
	var ls []lapLine
	for k := range laps {
		if laps[k] > 0 {
			ls = append(ls, lapLine{tickLapNames[k], laps[k] / time.Duration(m), lapMax[k]})
		}
	}
	sort.Slice(ls, func(i, j int) bool { return ls[i].avg > ls[j].avg })
	for _, l := range ls[:min(len(ls), 10)] {
		t.Logf("  %-15s %7.3f ms avg  %7.2f ms most", l.name, ms(l.avg), ms(l.top))
	}
}

func countVanilla(ps []*rigPlayer) int {
	n := 0
	for _, p := range ps {
		if p.vanilla {
			n++
		}
	}
	return n
}

func ms(d time.Duration) float64 { return float64(d) / 1e6 }

// BenchmarkTickRig is one rig step (tick + sends) as a benchmark, after a
// warm-up of 100 steps.
func BenchmarkTickRig(b *testing.B) {
	r := newTickRig(b, rigEnv("EQOA_RIG_PLAYERS", 36), int64(rigEnv("EQOA_RIG_SEED", 1)))
	for i := 0; i < 100; i++ {
		r.step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.step()
	}
}

type runtimeMem struct {
	mallocs, bytes uint64
	gc             uint32
}

func readMem() runtimeMem {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return runtimeMem{m.Mallocs, m.TotalAlloc, m.NumGC}
}

// channelsAsNow is the end of a tick's check: every session's every object
// channel holds what serializeEntityForChannel says of its entity now (the
// systems after the channels' change nothing it reads). It returns the
// channels checked and those that differ.
func (r *tickRig) channelsAsNow() (checked, differ int, first string) {
	w := r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rp := range r.players {
		conn := rp.sess.Connection()
		vis := GetOrCreateVisibility(rp.sess)
		for i, id := range vis.ChannelEntity {
			if id == 0 {
				continue
			}
			want := w.serializeEntityForChannel(rp.sess, id, vis.ChannelObjectID[i])
			if want == nil {
				continue
			}
			got := conn.GetServerObject(i + 1).GetData()
			checked++
			if string(got) != string(want) {
				differ++
				if first == "" {
					first = fmt.Sprintf("tick %d, %s slot %d entity %d: %x, want %x", r.tick, rp.p.Name, i+1, id, got, want)
				}
			}
		}
	}
	return checked, differ, first
}

// TestTickRigChannels runs the rig and checks every channel against its
// entity at the end of every tick (channelsAsNow).
func TestTickRigChannels(t *testing.T) {
	r := newTickRig(t, rigEnv("EQOA_RIG_PLAYERS", 36), int64(rigEnv("EQOA_RIG_SEED", 1)))
	rigChecksOn(t)
	ticks := rigEnv("EQOA_RIG_TICKS", 600)
	all, bad := 0, 0
	for i := 0; i < ticks; i++ {
		r.step()
		c, d, first := r.channelsAsNow()
		all += c
		bad += d
		if first != "" && bad == d {
			t.Errorf("%s", first)
		}
	}
	t.Logf("%d ticks: %d channels checked, %d differ", ticks, all, bad)
	if bad > 0 {
		t.Fail()
	}
}

// rigBench is a rig after 100 steps, for a benchmark of one system on it.
func rigBench(b *testing.B) *tickRig {
	r := newTickRig(b, rigEnv("EQOA_RIG_PLAYERS", 36), int64(rigEnv("EQOA_RIG_SEED", 1)))
	for i := 0; i < 100; i++ {
		r.step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	return r
}

// BenchmarkRigEntityChannels is entityChannelSyncSystem over the rig's
// sessions: every channel's record made and put again.
func BenchmarkRigEntityChannels(b *testing.B) {
	r := rigBench(b)
	for i := 0; i < b.N; i++ {
		r.w.mu.Lock()
		r.w.entityChannelSyncSystem()
		r.w.mu.Unlock()
	}
}

// BenchmarkRigVisibility is visibilitySystem's turn over the rig's sessions.
func BenchmarkRigVisibility(b *testing.B) {
	r := rigBench(b)
	for i := 0; i < b.N; i++ {
		r.w.mu.Lock()
		r.w.TickCount = VisibilityTickInterval * (r.w.TickCount/VisibilityTickInterval + 1)
		r.w.visibilitySystem()
		r.w.mu.Unlock()
	}
}

// BenchmarkRigRoam is roamSystem, a tick's time on, over the rig's players'
// creatures.
func BenchmarkRigRoam(b *testing.B) {
	r := rigBench(b)
	for i := 0; i < b.N; i++ {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.mu.Lock()
		r.w.TickCount++
		r.w.roamSystem(r.now)
		r.w.mu.Unlock()
	}
}

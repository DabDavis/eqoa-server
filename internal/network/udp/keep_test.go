//go:build unix

package udp

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
)

// keepListener is a listener on a socket of its own with one lobby session
// (an account, no character: the handlers' part needs none), reading.
func keepListener(t *testing.T) (*Listener, *session.Manager, *net.UDPConn) {
	t.Helper()
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	sm := session.NewManager()
	cfg := config.DefaultConfig()
	l := NewListener(cfg, sm, nil, entity.NewWorld())
	l.Adopt(server)
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Stop)
	s := sm.CreateSession(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061, 0x2EC7D061, 0x1234, 0xFFFE, false)
	s.SetAccountID(110)
	s.SetCharListSent(true)
	for i := 0; i < 5; i++ {
		s.Connection().LastSentPacketSeq()
	}
	return l, sm, client
}

// The game socket goes across the exec: the copy handed over has its
// close-on-exec flag cleared, its number is in the next image's environment
// with the snapshot's file, and the next image (here a second listener in
// this process, given only that environment) takes the socket by that number,
// reads the snapshot back, and gets the session again under its key with its
// numbers, and the datagrams a client sent while nothing read them.
func TestTheGameSocketAndTheSessionsGoAcrossTheExec(t *testing.T) {
	l, _, client := keepListener(t)
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	var env []string
	execed := make(chan struct{})
	o := KeepOptions{File: file, Exe: "/bin/true", Env: os.Environ(), Exec: func(exe string, args, e []string) error {
		env = e
		close(execed)
		select {} // an exec does not return
	}}
	go l.KeepRestart(o)
	select {
	case <-execed:
	case <-time.After(5 * time.Second):
		t.Fatal("no exec")
	}
	if !l.frozen.Load() || !l.paused.Load() {
		t.Fatal("still reading or sending at the exec")
	}
	var fdv, filev string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, KeepFDEnv+"="); ok {
			fdv = v
		}
		if v, ok := strings.CutPrefix(e, KeepFileEnv+"="); ok {
			filev = v
		}
	}
	if fdv == "" || filev != file {
		t.Fatalf("the next image's environment: %s=%q %s=%q", KeepFDEnv, fdv, KeepFileEnv, filev)
	}
	var fd int
	for _, c := range fdv {
		fd = fd*10 + int(c-'0')
	}
	flags, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	if e != 0 || flags&syscall.FD_CLOEXEC != 0 {
		t.Fatalf("descriptor %d: flags %#x (%v): it would close at the exec", fd, flags, e)
	}

	// a client's datagram while no process reads the socket
	if _, err := client.WriteToUDP([]byte("waiting"), l.conn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}

	// the next image
	t.Setenv(KeepFDEnv, fdv)
	conn, err := InheritedConn()
	if err != nil || conn == nil {
		t.Fatalf("InheritedConn: %v %v", conn, err)
	}
	defer conn.Close()
	if os.Getenv(KeepFDEnv) != "" {
		t.Fatal("the variable is left for a later image")
	}
	if conn.LocalAddr().String() != l.conn.LocalAddr().String() {
		t.Fatalf("the socket handed over is %s, the game's %s", conn.LocalAddr(), l.conn.LocalAddr())
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the snapshot is left to be read again")
	}
	sm2 := session.NewManager()
	l2 := NewListener(config.DefaultConfig(), sm2, nil, entity.NewWorld())
	l2.Adopt(conn)
	if resumed, told := l2.ResumeKept(snap, time.Now()); resumed != 1 || told != 0 {
		t.Fatalf("resumed %d, told %d", resumed, told)
	}
	s := sm2.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061)
	if s == nil || s.AccountID() != 110 || !s.CharListSent() || !s.Resumed() {
		t.Fatalf("the session after the exec: %+v", s)
	}
	if got := s.Connection().LastSentPacketSeq(); got != 6 {
		t.Fatalf("its next packet %d, want 6", got)
	}
	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, from, err := conn.ReadFromUDP(buf)
	if err != nil || string(buf[:n]) != "waiting" || from.String() != client.LocalAddr().String() {
		t.Fatalf("the datagram sent across the exec: %q from %v (%v)", buf[:n], from, err)
	}
}

// An exec that fails leaves the server as it was: reading, ticking and
// sending again, its sessions untouched, no snapshot left behind; what was
// saved is saved.
func TestAFailedExecLeavesTheServerRunning(t *testing.T) {
	l, sm, client := keepListener(t)
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	err := l.KeepRestart(KeepOptions{File: file, Exe: "/nonexistent", Exec: func(string, []string, []string) error {
		if _, err := os.Stat(file); err != nil {
			t.Errorf("no snapshot at the exec: %v", err)
		}
		return errors.New("no such file")
	}})
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("KeepRestart: %v", err)
	}
	if l.paused.Load() || l.frozen.Load() {
		t.Fatal("still paused or frozen")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the snapshot is left behind")
	}
	if sm.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061) == nil {
		t.Fatal("the session is gone")
	}
	if err := l.SendTo([]byte("hello"), client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, _, err := client.ReadFromUDP(buf); err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("the listener does not send again: %q %v", buf[:n], err)
	}
}

// A snapshot is read once, and only by this process's next image, in its
// format, within KeepMaxAge: the controls for each.
func TestASnapshotIsReadOnlyFresh(t *testing.T) {
	dir := t.TempDir()
	write := func(s Snapshot) string {
		p := filepath.Join(dir, "k.json")
		if err := s.Write(p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	now := time.Now()
	good := Snapshot{Version: KeepVersion, PID: os.Getpid(), Written: now}
	if _, err := ReadSnapshot(write(good), now); err != nil {
		t.Fatalf("a fresh one: %v", err)
	}
	for name, s := range map[string]Snapshot{
		"another process's": {Version: KeepVersion, PID: os.Getpid() + 1, Written: now},
		"another format's":  {Version: KeepVersion + 1, PID: os.Getpid(), Written: now},
		"a stale one":       {Version: KeepVersion, PID: os.Getpid(), Written: now.Add(-KeepMaxAge - time.Second)},
	} {
		p := write(s)
		if _, err := ReadSnapshot(p, now); err == nil {
			t.Errorf("%s was read", name)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was left to be read again", name)
		}
	}
}

// keepNPCWorld is a new process's world: its NPCs as their rows make them.
func keepNPCWorld(t *testing.T) *entity.World {
	t.Helper()
	w := entity.NewWorld()
	for _, n := range []*entity.NPC{
		{ServerID: 201, Name: "Lord Keeper", Level: 50, HP: 5000, MaxHP: 5000, X: 2000, Y: 10, Z: 2000, SpawnX: 2000, SpawnY: 10, SpawnZ: 2000, RespawnTime: time.Hour},
		{ServerID: 202, Name: "Lady Keeper", Level: 50, HP: 5000, MaxHP: 5000, X: 2100, Y: 10, Z: 2000, SpawnX: 2100, SpawnY: 10, SpawnZ: 2000, RespawnTime: time.Hour},
		{ServerID: 203, Name: "a beetle", Level: 5, HP: 50, MaxHP: 50, X: 2200, Y: 10, Z: 2000, SpawnX: 2200, SpawnY: 10, SpawnZ: 2000, RespawnTime: time.Hour},
	} {
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	return w
}

// The creatures go through a restart in the file with the sessions: one away from
// its spawn at 40%, one dead, the third untouched and not written. The next image
// has them as they were before it reads a datagram.
func TestTheCreaturesGoAcrossTheExec(t *testing.T) {
	cfg := config.DefaultConfig()
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	old := keepNPCWorld(t)
	l := NewListener(cfg, session.NewManager(), nil, old)
	l.Adopt(server)
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	boss, lady := old.NPCs[201], old.NPCs[202]
	boss.X, boss.Z, boss.Facing, boss.HP = 2030, 1990, 1.5, 2000
	boss.VelX, boss.Loco = 5, entity.LocoWalk // walking when the restart comes
	old.SpatialIndex.Update(201, 2000, 2000, 2030, 1990)
	old.KillNPCNow(202)
	died := lady.DeathTime

	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	execed := make(chan struct{})
	go l.KeepRestart(KeepOptions{File: file, Exe: "/bin/true", Exec: func(string, []string, []string) error {
		close(execed)
		select {}
	}})
	select {
	case <-execed:
	case <-time.After(5 * time.Second):
		t.Fatal("no exec")
	}
	if boss.VelX != 0 || boss.Loco != entity.LocoStand {
		t.Fatalf("still walking as it froze: velocity %v, loco %d", boss.VelX, boss.Loco)
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.NPCs) != 2 {
		t.Fatalf("written %+v, want the two that differ from their rows", snap.NPCs)
	}

	now := keepNPCWorld(t)
	l2 := NewListener(cfg, session.NewManager(), nil, now)
	if resumed, told := l2.ResumeKept(snap, time.Now()); resumed != 0 || told != 0 {
		t.Fatalf("resumed %d, told %d", resumed, told)
	}
	got := now.NPCs[201]
	if got.X != 2030 || got.Z != 1990 || got.Y != 10 || got.Facing != 1.5 || got.HP != 2000 || got.Dead {
		t.Fatalf("the one that moved: at (%v, %v, %v) facing %v, %d hit points, dead %v", got.X, got.Y, got.Z, got.Facing, got.HP, got.Dead)
	}
	if corpse := now.NPCs[202]; !corpse.Dead || !corpse.DeathTime.Equal(died) || corpse.HP != 0 {
		t.Fatalf("the dead one: dead %v, died %v want %v", corpse.Dead, corpse.DeathTime, died)
	}
	if untouched := now.NPCs[203]; untouched.Dead || untouched.HP != 50 {
		t.Fatalf("the third: %+v", untouched)
	}
}

// A file from a binary before the creatures were kept has no such section: the sessions
// come back and every creature is as its row makes it. It is read in the same format (the
// first swap into this build reads one, and a rollback out of it reads this build's with
// the section left aside). No file at all is the same world.
func TestAnOldFileWithoutTheCreaturesStillWorks(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	old := `{"version":1,"pid":` + strconv.Itoa(os.Getpid()) + `,"written":"` + time.Now().UTC().Format(time.RFC3339Nano) +
		`","sessions":[{"session":{"addr":"203.0.113.9:6301","instance":7,"session":7,"client_ep":7,"server_ep":2,"state":0,` +
		`"has_instance":true,"server_init":true,"in_world":false,"in_game":false,"char_list_sent":true,"account":110,"char":0,"object":0,` +
		`"disc_version":0,"conn":{"rx_packet":3,"rx_message":2,"rx_seen":0,"tx_packet":5,"tx_message":4,"acked":4,"slots":0,"view":0,` +
		`"objects":null,"stat":{"next":0},"buff":{"next":0},"group":{"next":0},"client":{"seq":0}}}}]}`
	if err := os.WriteFile(file, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if snap.NPCs != nil || snap.Corpses != nil || snap.Dungeons != nil || snap.Triggers != nil {
		t.Fatalf("a section out of nothing: %+v %+v %+v %+v", snap.NPCs, snap.Corpses, snap.Dungeons, snap.Triggers)
	}
	w := keepNPCWorld(t)
	sm := session.NewManager()
	l := NewListener(config.DefaultConfig(), sm, nil, w)
	if resumed, told := l.ResumeKept(snap, time.Now()); resumed != 1 || told != 0 {
		t.Fatalf("resumed %d, told %d", resumed, told)
	}
	for id, npc := range w.NPCs {
		if npc.Dead || npc.HP != npc.MaxHP || npc.X != npc.SpawnX {
			t.Fatalf("%d: %+v", id, npc)
		}
	}
	// and with no file at all there is nothing to resume from: the world stays its rows'
	if _, err := ReadSnapshot(file+".not-there", time.Now()); err == nil {
		t.Fatal("no file, no error")
	}
}

// The file an older binary reads back: the sections it does not know are left aside
// (encoding/json), and the format's number has not moved, or a rollback would lose
// every session.
func TestTheFormatIsTheOneOlderBinariesRead(t *testing.T) {
	if KeepVersion != 1 {
		t.Fatalf("format %d: a binary from before these sections reads only 1", KeepVersion)
	}
	b, err := json.Marshal(&Snapshot{Version: KeepVersion, PID: 1, NPCs: []entity.KeptNPC{{ID: 7, Name: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	var older struct {
		Version  int             `json:"version"`
		PID      int             `json:"pid"`
		Sessions []KeptSession   `json:"sessions"`
		Groups   json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(b, &older); err != nil || older.Version != 1 {
		t.Fatalf("%v, version %d", err, older.Version)
	}
}

// A creature walking as the restart comes is told standing to every client before
// anything is written (World.StandNPCs): a client carries it on along its last
// velocity for as long as the channels are quiet, and it would be dragged back.
func TestTheClientsAreToldTheCreaturesStandBeforeTheFreeze(t *testing.T) {
	cfg := config.DefaultConfig()
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	w := keepNPCWorld(t)
	sm := session.NewManager()
	w.SessionManager = sm
	l := NewListener(cfg, sm, nil, w)
	l.Adopt(server)
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	sess := sm.CreateSession(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061, 0x2EC7D061, 0x1234, 0xFFFE, false)
	sess.SetAccountID(110)
	sess.SetInGame(true)
	sess.SetCharacterInWorld(true)
	sess.SetSendFunc(func(b []byte) error { return l.SendTo(b, client.LocalAddr().(*net.UDPAddr)) })
	p := w.CreatePlayer(sess, &database.CharacterRecord{ServerID: 61, CharName: "Viewer", AccountID: 110, Level: 60, X: 2000, Y: 10, Z: 2010})
	if p == nil {
		t.Fatal("no player")
	}
	walker := w.NPCs[201]
	walker.VelX, walker.VelZ, walker.Loco = 3, 4, entity.LocoWalk
	vs := entity.GetOrCreateVisibility(sess)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = walker.ServerID, 100001
	if n := w.StandNPCs(); n != 1 {
		t.Fatalf("%d stood", n)
	}
	channel := sess.Connection().GetServerObject(1)
	if !channel.IsActive() && channel.DescribesNoEntity() {
		t.Fatal("its channel was not written")
	}
	before := sess.Keep().Conn.Objects[1].Next
	l.standCreatures()
	if walker.VelX != 0 || walker.VelZ != 0 || walker.Loco != entity.LocoStand {
		t.Fatalf("walking on: %v %v loco %d", walker.VelX, walker.VelZ, walker.Loco)
	}
	if after := sess.Keep().Conn.Objects[1].Next; after == before {
		t.Fatalf("its channel went nowhere: next %d", after)
	}
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	if n, _, err := client.ReadFromUDP(buf); err != nil || n == 0 {
		t.Fatalf("nothing reached the client: %d bytes, %v", n, err)
	}
}

// A dungeon's instance goes through the file and ResumeKept (entity/keepdungeon.go; the round trip with players,
// groups and loot is handlers' TestADungeonInstanceGoesThroughAKeepRestart): it is open again under its id with its
// creature where it stood at its hit points, before any session is put back.
func TestADungeonInstanceGoesAcrossTheExec(t *testing.T) {
	w := entity.NewWorld()
	tmpl := &entity.NPC{ServerID: 930001, Name: "a froglok", Level: 60, HP: 5000, MaxHP: 5000, World: 14,
		X: 1000, Y: 50, Z: 1000, SpawnX: 1000, SpawnY: 50, SpawnZ: 1000}
	w.NPCs[tmpl.ServerID] = tmpl
	w.SpatialIndex.Insert(tmpl.ServerID, tmpl.X, tmpl.Z)
	w.SetDungeons(true, 14, []entity.DungeonDef{{Key: "guk", Name: "Test Guk",
		Boxes: []entity.DungeonBox{{X0: 900, Z0: 900, X1: 1100, Z1: 1100}}, Entrance: entity.HomeSpot{X: 1000, Y: 50, Z: 1150}}},
		entity.DungeonSettings{})
	t.Cleanup(func() { w.SetDungeons(false, 14, nil, entity.DungeonSettings{}) })
	place := [4]float32{1020, 50, 990, 1}
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	snap := &Snapshot{Version: KeepVersion, PID: os.Getpid(), Written: time.Now(), Dungeons: &entity.KeptDungeons{Next: 9,
		Insts: []entity.KeptDungeonInst{{ID: 7, Key: "guk", Owner: 1, OwnerName: "Hero", Creatures: 3, Opened: time.Now(), LastIn: time.Now(),
			NPCs: []entity.KeptInstNPC{{Row: 930001, KeptNPC: entity.KeptNPC{ID: 5555, Name: "a froglok", World: 14,
				Spawn: [4]float32{1000, 50, 1000, 0}, Place: &place, HP: 700, MaxHP: 5000}}}}}}}
	if err := snap.Write(file); err != nil {
		t.Fatal(err)
	}
	back, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	l := NewListener(config.DefaultConfig(), session.NewManager(), nil, w)
	l.ResumeKept(back, time.Now())
	c := w.NPCs[5555]
	if c == nil || !c.Temporary || c.Instance != 7 || c.HP != 700 || c.X != 1020 || c.Z != 990 || c.CopyOf != 930001 {
		t.Fatalf("the instance's creature: %+v", c)
	}
	if inst, open := w.DungeonInstanceInfo(5555); inst != 7 || !open {
		t.Fatalf("instance %d open %v", inst, open)
	}
}

// A GM trigger's run goes through the file and ResumeKept (handlers/gmtriggerkeep.go; the round trip from a live run,
// its clock, sic and awaitdead, is handlers' TestATriggerRunGoesThroughAKeepRestart): Rallos Zek's run is under way
// again at its step with its lock, and its boss stands under his id where he stood at his hit points, before any
// session is put back.
func TestATriggerRunGoesAcrossTheExec(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/triggers.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := entity.NewWorld()
	tmpl := &entity.NPC{ServerID: 930001, Name: "Rallos Zek", Level: 60, HP: 5000, MaxHP: 5000, World: 5, NPCType: 0x80,
		X: 4280, Y: 183, Z: 6700, SpawnX: 4280, SpawnY: 183, SpawnZ: 6700}
	w.NPCs[tmpl.ServerID] = tmpl
	w.SpatialIndex.Insert(tmpl.ServerID, tmpl.X, tmpl.Z)
	steps := `[{"do":"lock","lock":"rallos zek"},{"do":"clock","seconds":7200,"empty":600},{"do":"spawn","npc":"Rallos Zek","hostile":true,"spread":0.5,"minutes":120},{"do":"awaitdead","npc":"Rallos Zek"},{"do":"broadcast","text":"He rises again."}]`
	if _, err := db.Exec(`INSERT INTO gmtriggers (slot, name, world, x, y, z, radius, steps) VALUES (115, 'Rallos Zek', 5, 4280, 183, 6700, 220, ?)`, steps); err != nil {
		t.Fatal(err)
	}
	e := handlers.NewTriggerEngine(db, w, session.NewManager(), nil)
	e.Start()
	t.Cleanup(e.Stop)
	now := time.Now()
	trig := e.List()[0]
	place := [4]float32{4290, 183, 6705, 1}
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	snap := &Snapshot{Version: KeepVersion, PID: os.Getpid(), Written: now, Triggers: &handlers.KeptTriggers{Written: now, Seq: 3,
		Runs: []handlers.KeptTriggerRun{{ID: 3, Trig: trig, By: "Lonewulf", Quiet: true, World: 5, X: 4280, Y: 183, Z: 6700, Next: 3,
			Start: now.Add(-20 * time.Minute), Lock: "rallos zek", Deadline: now.Add(100 * time.Minute), EmptyAfter: 600 * time.Second, LastNear: now,
			Await: &database.TriggerStep{Do: database.TriggerAwaitDead, NPC: "Rallos Zek"},
			Spawns: []handlers.KeptTriggerSpawn{{ID: 5555, Name: "Rallos Zek", Until: now.Add(100 * time.Minute)}}}},
		NPCs: []entity.KeptEventNPC{{Row: 930001, Level: 60, FullHP: 5000, Event: true, KeptNPC: entity.KeptNPC{ID: 5555, Name: "Rallos Zek", World: 5,
			Spawn: [4]float32{4280, 183, 6700, 0}, Place: &place, HP: 1200, MaxHP: 5000}}}}}
	if err := snap.Write(file); err != nil {
		t.Fatal(err)
	}
	back, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	l := NewListener(config.DefaultConfig(), session.NewManager(), nil, w)
	l.ResumeKept(back, time.Now())
	c := w.NPCs[5555]
	if c == nil || c.Dead || !c.Temporary || c.HP != 1200 || c.X != 4290 || c.CopyOf != 930001 || !c.EventAggro {
		t.Fatalf("Rallos Zek after the exec: %+v", c)
	}
	runs := e.Runs()
	if len(runs) != 1 || runs[0].Run != 3 || runs[0].Step != 4 || runs[0].Spawns != 1 || !e.BusyNamed("Rallos Zek") {
		t.Fatalf("the run after the exec: %+v (busy %v)", runs, e.BusyNamed("Rallos Zek"))
	}
}

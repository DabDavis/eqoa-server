package handlers

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// respawnFixture is a dead player in a session, the World's respawn hook set
// to the handlers' as RegisterDefaultHandlers sets it, and an NPC in the
// player's view.
type respawnFixture struct {
	w    *entity.World
	sess *session.Session
	p    *entity.Player
	h    *CharacterHandler
	t0   time.Time
}

func newRespawnFixture(t *testing.T, world uint8, x, y, z float32) *respawnFixture {
	t.Helper()
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 6000}, 1, 1, 1, 2, true)
	sess.SetPlayerObjectID(500000001)
	sess.SetCharacterInWorld(true)
	t0 := time.Unix(1_700_000_000, 0)
	p := &entity.Player{ServerID: 9001, CharServerID: 42, Name: "Funyun", Class: 6, Race: 6, Level: 1,
		HP: 0, MaxHP: 49, Power: 0, MaxPower: 88, Breath: 0, Speed: 8.8333,
		World: world, X: x, Y: y, Z: z, Facing: 1.5, DiedAt: t0}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	vis := entity.GetOrCreateVisibility(sess)
	vis.ChannelEntity[0], vis.ChannelObjectID[0] = 7777, 0x01313011
	h := &CharacterHandler{sessionManager: mgr, gameWorld: w}
	w.OnPlayerRespawn(h.respawnPlayer)
	return &respawnFixture{w: w, sess: sess, p: p, h: h, t0: t0}
}

// sent is every reliable message queued for the client, payload with its
// opcode first.
func (f *respawnFixture) sent() [][]byte {
	var out [][]byte
	for _, m := range f.sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, d)
		}
	}
	return out
}

// dumpPlace reads a MemoryDump as Msg000D does up to the player's place: the
// world path, the record, the world index and the position.
func dumpPlace(t *testing.T, d []byte) (path string, world int64, pos [3]float32, name string) {
	t.Helper()
	r := buffer.NewReader(d[2:])
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("dump read at %d: %v", r.Position(), err)
		}
	}
	str := func() string {
		n, err := r.ReadInt32()
		must(err)
		b, err := r.ReadBytes(int(n))
		must(err)
		return string(b)
	}
	zz := func() int64 {
		v, err := r.Read7BitEncodedInt64()
		must(err)
		return v
	}
	_, err := r.ReadByte()
	must(err)
	path = str()
	zz() // the character's server id
	name = str()
	for i := 0; i < 5; i++ { // class, race, level, xp, debt
		zz()
	}
	_, err = r.ReadByte() // breath
	must(err)
	for i := 0; i < 4; i++ { // tunar, bank, training points, base max stat
		zz()
	}
	world = zz()
	for i := range pos {
		pos[i], err = r.ReadFloat32()
		must(err)
	}
	return path, world, pos, name
}

func opcode(d []byte) protocol.GameOpcode { return protocol.GameOpcode(binary.LittleEndian.Uint16(d)) }

// A death in another world (Lavastorm) respawns at the login spawn in
// Tunaria through a MemoryDump — the message that ends a death on the
// client — and no teleport: the dump carries Tunaria's path, index and the
// spawn, the old world's entities go, the stat channel shows full HP, and the
// state channels wait for the load until the client's 0x0014.
func TestRespawnSendsTheDumpAtTheLoginSpawn(t *testing.T) {
	f := newRespawnFixture(t, 3, 4768, 100, 4500)
	f.w.Tick(f.t0.Add(entity.PlayerRespawnDelay))

	var dumps [][]byte
	for _, d := range f.sent() {
		switch opcode(d) {
		case protocol.OpcodeMemoryDump:
			dumps = append(dumps, d)
		case protocol.OpcodeTeleport, protocol.OpcodeExpansionFlags, protocol.OpcodeTime:
			t.Errorf("respawn sent opcode 0x%04X; Sandstorm's respawn is the dump alone", uint16(opcode(d)))
		}
	}
	if len(dumps) != 1 {
		t.Fatalf("%d MemoryDumps queued, want 1", len(dumps))
	}
	dest := teleDests[loginSpawnDest]
	want := [3]float32{dest.X + loginSpawnOffset, dest.Y, dest.Z + loginSpawnOffset}
	path, world, pos, name := dumpPlace(t, dumps[0])
	if path != worldPathFor(0) || world != 0 || pos != want || name != "Funyun" {
		t.Errorf("dump: %q world %d at %v (%s), want %q world 0 at %v", path, world, pos, name, worldPathFor(0), want)
	}
	if f.p.World != 0 || [3]float32{f.p.X, f.p.Y, f.p.Z} != want {
		t.Errorf("player in world %d at (%v, %v, %v), want world 0 at %v", f.p.World, f.p.X, f.p.Y, f.p.Z, want)
	}
	if f.p.HP != f.p.MaxHP || f.p.Power != f.p.MaxPower {
		t.Errorf("respawned at %d/%d HP, %d/%d power", f.p.HP, f.p.MaxHP, f.p.Power, f.p.MaxPower)
	}
	if stat := f.sess.Connection().GetClientStatUpdate().GetData(); len(stat) < 32 {
		t.Error("the stat channel was not written with the respawn")
	} else if hp := int32(binary.LittleEndian.Uint32(stat[28:])); hp != 49 {
		t.Errorf("stat channel HP %d, want 49", hp)
	}
	if vis := entity.GetOrCreateVisibility(f.sess); vis.ChannelEntity[0] != 0 {
		t.Error("Lavastorm's entity is still on a channel after the respawn to Tunaria")
	}

	// The client answers the dump with 0x0014: the hold ends there.
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	if err := f.h.HandleCharacterInWorld(f.sess, msg); err != nil {
		t.Fatal(err)
	}
	if f.sess.ConsumeWorldLoadPending() {
		t.Error("the respawn's world load is still pending after the client's 0x0014")
	}
}

// In the same world the respawn holds nothing and keeps the scene: the
// visibility system moves it on as the player's place changes, as for a warp.
func TestRespawnInTheSameWorldKeepsTheScene(t *testing.T) {
	f := newRespawnFixture(t, 0, 25100, 54, 15500)
	f.w.Tick(f.t0.Add(entity.PlayerRespawnDelay))

	n := 0
	for _, d := range f.sent() {
		if opcode(d) == protocol.OpcodeMemoryDump {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d MemoryDumps, want 1", n)
	}
	if f.sess.ConsumeWorldLoadPending() {
		t.Error("a respawn in the same world held the state channels")
	}
	if vis := entity.GetOrCreateVisibility(f.sess); vis.ChannelEntity[0] != 7777 {
		t.Error("a respawn in the same world cleared the scene")
	}
}

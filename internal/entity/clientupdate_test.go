package entity

import (
	"github.com/eqoa/server/internal/session"
	"math"
	"testing"
	"time"
)

// quantize is CLIENT's WriteQuantized (0x00BC2D28) for one axis, two bytes:
// what a PS2 puts in the 0x40.
func quantize(v, lo, hi float32) (byte, byte) {
	rng := hi - lo
	t := v - lo
	if t < 0 {
		t = 0
	}
	if t > rng {
		t = rng
	}
	q := uint32(t * 65536 / rng)
	if q > 65535 {
		q = 65535
	}
	return byte(q >> 8), byte(q)
}

func clientUpdateWithVelocity(v [3]float32, autoAttack byte) []byte {
	b := make([]byte, 41)
	for i := 0; i < 3; i++ {
		b[10+2*i], b[11+2*i] = quantize(v[i], clientVelLo[i], clientVelHi[i])
	}
	b[36] = autoAttack
	return b
}

func TestClientUpdateVelocityStandingStill(t *testing.T) {
	// A PS2 standing still: 80 00, F7 39, 80 00 (the selfstate lane's
	// "velocity y 0 is 0xF739" in 3,348 captured buffers).
	b := make([]byte, 41)
	copy(b[10:], []byte{0x80, 0x00, 0xF7, 0x39, 0x80, 0x00})
	v, ok := ClientUpdateVelocity(b)
	if !ok {
		t.Fatal("not decoded")
	}
	for i, c := range v {
		if math.Abs(float64(c)) > 0.002 {
			t.Errorf("axis %d = %v, want 0", i, c)
		}
		if velocityToByte(c) != 0 {
			t.Errorf("axis %d goes out as %d, want 0", i, int8(velocityToByte(c)))
		}
	}
}

func TestClientUpdateVelocityRoundTrip(t *testing.T) {
	for _, want := range [][3]float32{{8.83, 0, 0}, {-6.2, -12.5, 6.2}, {0, 2.9, -15.29}} {
		got, _ := ClientUpdateVelocity(clientUpdateWithVelocity(want, 0))
		for i := range want {
			// One bucket: 30.6/65536 across, 87.5/65536 down.
			if math.Abs(float64(got[i]-want[i])) > 0.0015 {
				t.Errorf("%v: axis %d = %v", want, i, got[i])
			}
		}
	}
	if _, ok := ClientUpdateVelocity(make([]byte, 15)); ok {
		t.Error("a short update decoded")
	}
}

func TestVelocityToByteIsCLIENTsScale(t *testing.T) {
	cases := []struct {
		v    float32
		want int8
	}{
		{0, 0},
		{8.83333, 54}, // the run speed: 53.8 bytes; 54 × 21/128 = 8.86 u/s
		{-8.83333, -54},
		{3, 18}, // the walk
		{0.08, 0},
		{30, 127},
		{-30, -128},
	}
	for _, c := range cases {
		if got := int8(velocityToByte(c.v)); got != c.want {
			t.Errorf("velocityToByte(%v) = %d, want %d", c.v, got, c.want)
		}
	}
}

// A client that stops sends one last 0x40 with the walker at rest and then
// nothing at all; the player must carry that zero, not the last motion.
func TestApplyClientUpdateTakesTheWalkersVelocity(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 5}
	w.Players[p.ServerID] = p

	run := clientUpdateWithVelocity([3]float32{8.83, 0, 0}, 1)
	u := ClientUpdate{X: 100, Y: 10, Z: 100, Animation: 3}
	u.Velocity, u.HasVelocity = ClientUpdateVelocity(run)
	u.AutoAttack, u.HasAutoAttack = ClientUpdateAutoAttack(run)
	w.ApplyClientUpdate(p.ServerID, u)
	if int8(velocityToByte(p.VelX)) != 54 || !p.AutoAttack || p.Animation != 3 {
		t.Fatalf("running: vel byte %d, autoAttack %v, anim %d", int8(velocityToByte(p.VelX)), p.AutoAttack, p.Animation)
	}

	stop := clientUpdateWithVelocity([3]float32{}, 0)
	u = ClientUpdate{X: 100.4, Y: 10, Z: 100}
	u.Velocity, u.HasVelocity = ClientUpdateVelocity(stop)
	u.AutoAttack, u.HasAutoAttack = ClientUpdateAutoAttack(stop)
	w.ApplyClientUpdate(p.ServerID, u)
	buf := SerializePlayerUpdate(1, p)
	if buf[40] != 0 || buf[41] != 0 || buf[42] != 0 {
		t.Errorf("stopped player relays velocity % X, want 00 00 00", buf[40:43])
	}
	if p.AutoAttack {
		t.Error("byte 36 at 0 left auto-attack on")
	}
}

// A server-side move (a teleport) arrives standing still.
func TestUpdatePositionArrivesStill(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 6, VelX: 8, VelZ: -3}
	w.Players[p.ServerID] = p
	w.UpdatePosition(p.ServerID, 5000, 20, 5000, 1)
	if p.VelX != 0 || p.VelY != 0 || p.VelZ != 0 {
		t.Errorf("teleported player keeps velocity (%v, %v, %v)", p.VelX, p.VelY, p.VelZ)
	}
}

// A walker's flags in its 0x40, as VIWalkPhysics carries them: on the
// ground, swimming along the surface, and under it. Only the last is out of
// breath's reach (F3 20261020-205003: drowning while swimming on top).
func TestUnderLiquidIsUnderTheSurfaceOnly(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags uint32
		under bool
	}{
		{"on the ground", 0x0401, false},          // on floor, landscape
		{"at the water's surface", 0x000C, false}, // in water + at its surface
		{"under water", 0x0004, true},             // in water
		{"under lava", 0x0010, true},              // in lava
		{"at the mud's surface", 0x0800 | 0x1000, false},
	} {
		b := make([]byte, 41)
		b[31], b[32], b[33], b[34] = byte(c.flags), byte(c.flags>>8), byte(c.flags>>16), byte(c.flags>>24)
		f, ok := ClientUpdateWalkFlags(b)
		if !ok || f != c.flags {
			t.Fatalf("%s: flags %#x, %v; want %#x", c.name, f, ok, c.flags)
		}
		if got := UnderLiquid(f); got != c.under {
			t.Errorf("%s: under liquid %v, want %v", c.name, got, c.under)
		}
	}
	if _, ok := ClientUpdateWalkFlags(make([]byte, 34)); ok {
		t.Error("a buffer that ends before byte 34 has no flags")
	}
}

// The breath system takes the flags when a client sent them: a swim
// animation at the surface keeps breath, under it loses it.
func TestBreathRunsOutUnderTheSurfaceNotOnIt(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 7, HP: 50, MaxHP: 50, Breath: 100, Animation: AnimSwimSlow}
	w.Players[p.ServerID] = p
	tick := func(n int) {
		for range n {
			w.TickCount = 6
			w.breathSystem(time.Now())
		}
	}
	p.WalkFlags, p.HasWalkFlags = 0x000C, true // at the surface
	tick(1)
	if p.Underwater || p.Breath != 104 {
		t.Fatalf("at the surface: underwater %v, breath %d; want false, 104", p.Underwater, p.Breath)
	}
	p.WalkFlags = 0x0004 // under it: a step every 6 s (ten calls)
	tick(9)
	if !p.Underwater || p.Breath != 104 {
		t.Fatalf("under water 5.4 s: underwater %v, breath %d; want true, 104", p.Underwater, p.Breath)
	}
	tick(1)
	if p.Breath != 104-BreathStep {
		t.Fatalf("under water 6 s: breath %d, want %d", p.Breath, 104-BreathStep)
	}
	// negative control: no flags yet, the swim animation decides
	p.HasWalkFlags = false
	tick(1)
	if !p.Underwater {
		t.Fatal("without flags a swim animation is under water")
	}
}

// Sandstorm's breath (pcaps/breath_capture.pcap): full is 0xFF, eight steps
// of 32 to nothing (48 s), then the HP goes; and the stat channel carries it
// at byte 232, where it was a constant 0xFF (F3 20261021-011015: the gauge
// stood full under water).
func TestBreathStepsAsSandstormsAndReachesTheStatChannel(t *testing.T) {
	w := NewWorld()
	w.SessionManager = session.NewManager() // the drowning looks for a session (none here)
	p := &Player{ServerID: 7, HP: 50, MaxHP: 50, Breath: BreathFull, WalkFlags: 0x0004, HasWalkFlags: true}
	w.Players[p.ServerID] = p
	var seen []byte
	for range 8 * 10 {
		w.TickCount = 6
		w.breathSystem(time.Now())
		if b := SerializeStatUpdate(p)[232]; len(seen) == 0 || seen[len(seen)-1] != b {
			seen = append(seen, b)
		}
	}
	want := []byte{0xFF, 0xDF, 0xBF, 0x9F, 0x7F, 0x5F, 0x3F, 0x1F, 0x00}
	if string(seen) != string(want) {
		t.Fatalf("byte 232 over 48 s under water: % x, want % x", seen, want)
	}
	if p.HP != 50 {
		t.Fatalf("HP %d while there was breath, want 50", p.HP)
	}
	for range 10 {
		w.TickCount = 6
		w.breathSystem(time.Now())
	}
	if p.HP != 48 {
		t.Fatalf("HP %d 6 s after the breath ran out, want 48", p.HP)
	}
	// the control: a player out of the water is full on the wire
	q := &Player{Breath: BreathFull}
	if b := SerializeStatUpdate(q)[232]; b != 0xFF {
		t.Fatalf("a full breath on the wire: %#x, want 0xff", b)
	}
}

// A login that replaces a live session takes its player's place (F3
// 20261020-195202: a tab closed and logged in again came back to the place
// saved before): alive, with the save's guards; a dead player has none.
func TestLivePositionIsTheLivePlayersWithTheSaveGuards(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 9, HP: 10, X: 17091.6, Y: 141.1, Z: 15220.2, Facing: 1.5, World: 0}
	w.Players[p.ServerID] = p
	x, y, z, f, world, ok := w.LivePosition(9, 54.1)
	if !ok || x != 17091.6 || y != 141.1 || z != 15220.2 || f != 1.5 || world != 0 {
		t.Fatalf("live: (%v, %v, %v) facing %v world %d ok %v", x, y, z, f, world, ok)
	}
	p.Y = WorldFloorY // a fall keeps the stored height
	if _, y, _, _, _, _ := w.LivePosition(9, 54.1); y != 54.1 {
		t.Errorf("a fall gave y=%v, want the stored 54.1", y)
	}
	p.HP = 0
	if _, _, _, _, _, ok := w.LivePosition(9, 54.1); ok {
		t.Error("a dead player's place is not taken")
	}
	if _, _, _, _, _, ok := w.LivePosition(10, 0); ok {
		t.Error("a character with no player in the world has no live position")
	}
}

package entity

import (
	"math"
	"testing"
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

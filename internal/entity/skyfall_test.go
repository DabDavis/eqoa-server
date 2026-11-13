package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// skyGround is the Plane of Sky in small: an island's floor at 400 over x
// 0-100 and nothing else anywhere (its terrain the world's alone).
type skyGround struct{}

func (skyGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	return skyGround{}.Support(world, x, y, z, 2, 1e6)
}

func (skyGround) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	if world == skyWorld && x >= 0 && x <= 100 && 400 <= y+up && 400 >= y-down {
		return 400, true
	}
	return 0, false
}

func (skyGround) Blocked(world uint8, x0, y0, z0, x1, y1, z1 float32) (string, bool) {
	return "", false
}

func (skyGround) Knows(world uint8) bool { return world == skyWorld }

// A player who steps off a Plane of Sky island and falls is sent home once it
// has fallen skyFallAfter (Hoofbite's petition 1995: "When jumping off plane
// of sky island, you do not return home and continue to fall forever"), told
// why; one standing on the island, walking off and landing again, or flying
// by #fly, is not; and a PS2 parked at the world floor (-1000) saying nothing
// more is sent home by the tick alone. The control: in another world the same
// fall sends no one.
func TestSkyFallSendsHome(t *testing.T) {
	c := newCaster(t)
	c.w.SetGround(skyGround{})
	var sent []string
	c.w.OnGate(func(_ *session.Session, p *Player) {
		sent = append(sent, p.Name)
		c.w.mu.Lock()
		p.X, p.Y = 50, 400 // home: the hook moves it (handlers' Return Home landing)
		c.w.mu.Unlock()
	})
	c.p.World, c.p.X, c.p.Y, c.p.Z = skyWorld, 50, 400, 50
	report := func(d time.Duration, x, y float32) {
		c.w.ApplyClientUpdate(c.p.ServerID, ClientUpdate{X: x, Y: y, Z: 50, At: c.at(d)})
	}
	tick := func(d time.Duration) {
		c.w.mu.Lock()
		due := c.w.skyFallSystem(c.at(d))
		c.w.mu.Unlock()
		c.w.runGates(due)
	}
	// standing on the island a minute: nobody sent
	for d := time.Duration(0); d < time.Minute; d += time.Second {
		report(d, 50, 400)
		tick(d)
	}
	if len(sent) != 0 {
		t.Fatalf("standing on the island: sent %v", sent)
	}
	// off its edge: falling over the void, 24 a second
	t0 := time.Minute
	y := float32(400)
	for d := time.Duration(0); d < 14*time.Second; d += 100 * time.Millisecond {
		y -= 2.4
		report(t0+d, 150, y)
		tick(t0 + d)
	}
	if len(sent) != 0 {
		t.Fatalf("14 s into the fall: sent %v", sent)
	}
	for d := 14 * time.Second; len(sent) == 0 && d <= 16*time.Second; d += 100 * time.Millisecond {
		y -= 2.4
		report(t0+d, 150, y)
		tick(t0 + d)
	}
	if len(sent) != 1 {
		t.Fatalf("16 s into the fall: sent %v, want the player once", sent)
	}
	report(t0+16*time.Second, 50, 400) // its first report at home
	tick(t0 + 40*time.Second)
	if len(sent) != 1 || !c.p.skyFallSince.IsZero() {
		t.Errorf("home: sent %v, its fall's count %v", sent, c.p.skyFallSince)
	}
	want := make([]byte, 0, 2*len(skyFallLine))
	for _, r := range skyFallLine {
		want = append(want, byte(r), byte(r>>8))
	}
	told := false
	for _, m := range c.sess.Queue().GetPending() {
		told = told || bytes.Contains(m.Data(), want)
	}
	if !told {
		t.Error("the player was not told why it went home")
	}
	// a short fall that lands on the island again: nobody sent
	sent = nil
	t1 := 2 * time.Minute
	report(t1, 150, 380)
	tick(t1)
	report(t1+5*time.Second, 50, 400)
	for d := 5 * time.Second; d < 30*time.Second; d += time.Second {
		report(t1+d, 50, 400)
		tick(t1 + d)
	}
	if len(sent) != 0 {
		t.Errorf("a fall that landed: sent %v", sent)
	}
	// a PS2 parked at the world floor, silent: the tick alone sends it
	t2 := 3 * time.Minute
	report(t2, 150, WorldFloorY)
	tick(t2 + 10*time.Second)
	if len(sent) != 0 {
		t.Fatalf("10 s at the world floor: sent %v", sent)
	}
	tick(t2 + 15*time.Second)
	if len(sent) != 1 {
		t.Fatalf("15 s at the world floor, silent: sent %v", sent)
	}
	// flying by #fly over the void: not lost
	sent = nil
	c.p.Flying = true
	t3 := 4 * time.Minute
	for d := time.Duration(0); d < 30*time.Second; d += time.Second {
		report(t3+d, 150, 300)
		tick(t3 + d)
	}
	if len(sent) != 0 {
		t.Errorf("a GM flying: sent %v", sent)
	}
	c.p.Flying = false
	// the control: the same fall in Tunaria sends no one
	c.p.World = 0
	t4 := 5 * time.Minute
	for d := time.Duration(0); d < 30*time.Second; d += time.Second {
		report(t4+d, 150, -2000-float32(d/time.Second)*24)
		tick(t4 + d)
	}
	if len(sent) != 0 {
		t.Errorf("a fall in Tunaria: sent %v", sent)
	}
}

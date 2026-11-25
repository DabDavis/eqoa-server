package entity

import (
	"testing"
	"time"
)

// lavaEnv is lava where x < 200 up to y 10.3, nothing elsewhere.
type lavaEnv struct{}

func (lavaEnv) Flags(world uint8, x, y, z float32) (uint32, bool) {
	if x < 200 && y < 10.3 {
		return envLava, true
	}
	return 0, true
}

// The lava (petition 2281): with a rule, a player with its feet in it is burnt at once, not
// again before the tick, again at it; out of it, a GM, the dead, and with no rule (the
// default, the control) nothing; a death of it is a death.
func TestLavaBurnsAPlayerInIt(t *testing.T) {
	burnt := func(c *caster, at time.Duration) int {
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		before := c.p.HP
		c.w.TickCount = 0
		c.w.lavaSystem(c.at(at))
		return before - c.p.HP
	}
	rig := func(r LavaRule) *caster {
		c := newCaster(t)
		c.w.SetEnv(lavaEnv{})
		c.w.SetLava(r)
		c.p.HP, c.p.MaxHP = 1000, 1000
		c.p.X, c.p.Y, c.p.Z = 100, 10, 100
		return c
	}
	rule := LavaRule{PctMaxHP: 10, Every: 3 * time.Second}

	c := rig(rule)
	if n := burnt(c, 0); n != c.p.MaxHPNow()/10 {
		t.Fatalf("at once: %d, want %d", n, c.p.MaxHPNow()/10)
	}
	if n := burnt(c, 2900*time.Millisecond); n != 0 {
		t.Fatalf("before the tick: %d", n)
	}
	if n := burnt(c, 3*time.Second); n == 0 {
		t.Fatal("not again at the tick")
	}
	c.p.X = 300 // out of it
	if n := burnt(c, 6*time.Second); n != 0 {
		t.Fatalf("out of it: %d", n)
	}
	c.p.X = 100 // back in: at once
	if n := burnt(c, 6500*time.Millisecond); n == 0 {
		t.Fatal("not at once on stepping in again")
	}
	c.p.HP = 5 // a death of it
	burnt(c, 10*time.Second)
	if c.p.HP != 0 || !c.p.IsDead() {
		t.Fatalf("HP %d, dead %v", c.p.HP, c.p.IsDead())
	}
	if n := burnt(c, 20*time.Second); n != 0 {
		t.Fatalf("the dead burnt %d", n)
	}

	g := rig(rule)
	g.p.GM = true
	if n := burnt(g, 0); n != 0 {
		t.Fatalf("a GM burnt %d", n)
	}
	off := rig(LavaRule{})
	if n := burnt(off, 0); n != 0 || off.w.Lava().On() {
		t.Fatalf("the control, no rule: burnt %d", n)
	}
	flat := rig(LavaRule{Flat: 50, FireResist: true})
	if n := burnt(flat, 0); n < 1 || n > 50 {
		t.Fatalf("flat 50 with fire resist: %d", n)
	}
	over := rig(rule)
	over.p.Y = 11 // over the lava, not in it
	if n := burnt(over, 0); n != 0 {
		t.Fatalf("over it: %d", n)
	}
}

func TestLavaOnMovement(t *testing.T) {
	for _, reenter := range []bool{false, true} {
		name := "continuous exposure"
		if reenter {
			name = "quick reentry"
		}
		t.Run(name, func(t *testing.T) {
			c := newCaster(t)
			c.w.SetEnv(lavaEnv{})
			c.w.SetLava(LavaRule{PctMaxHP: 49})
			c.p.HP, c.p.MaxHP = 1000, 1000
			c.p.X = 200.25
			c.w.TickCount = 1 // between environment scans
			move := func(x float32, elapsed time.Duration) {
				c.w.ApplyClientUpdate(c.p.ServerID, ClientUpdate{X: x, Y: c.p.Y, Z: c.p.Z, At: c.at(elapsed)})
			}
			wantHP := func(want int, when string) {
				t.Helper()
				if c.p.HP != want {
					t.Fatalf("%s: HP %d, want %d", when, c.p.HP, want)
				}
			}

			move(199.75, 0)
			wantHP(510, "on entry")
			move(199.5, 100*time.Millisecond)
			wantHP(510, "moving within lava")

			var lastBurn time.Duration
			if reenter {
				move(200.25, 200*time.Millisecond)
				wantHP(510, "on exit")
				lastBurn = 300 * time.Millisecond
				move(199.75, lastBurn)
				wantHP(20, "on reentry")
			} else {
				c.w.lavaSystem(c.at(lavaEveryDefault - time.Millisecond))
				wantHP(510, "before second tick")
				c.w.lavaSystem(c.at(lavaEveryDefault))
				wantHP(20, "after second tick")
				lastBurn = lavaEveryDefault
			}
			c.w.lavaSystem(c.at(lastBurn + lavaEveryDefault))
			wantHP(0, "after third burn")
			if !c.p.IsDead() {
				t.Fatal("the third burn did not kill the player")
			}
		})
	}
}

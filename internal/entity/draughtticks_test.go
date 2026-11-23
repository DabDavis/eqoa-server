package entity

import (
	"testing"
	"time"
)

// Heirloom petition 2729 (the owner, 2026-11-23: the Infused Draught gets a
// 4th tick at 18 s, 3,200 power for 800 health): its line with "4 ticks"
// gives 800 power at the drink and at 6, 12 and 18 s. The control: the line
// without a count ticks three times, 2,400.
func TestTheInfusedDraughtTicksFourTimes(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"Self, Decrease Hitpoints by 800, Increase Power per tic by 800, 4 ticks, 18sec", 3200},
		{"Self, Decrease Hitpoints by 800, Increase Power per tic by 800, 18sec", 2400},
	} {
		c := newCaster(t)
		c.p.MaxPower, c.p.MaxHP, c.p.HP = 9000, 5000, 5000
		sp := buffOf(950028, 54, 0, "Toxic Infusion", tc.line)
		c.castOn(t, sp, 0)
		c.p.Power = 800 // castOn lands it (its first tick) with power full: that tick's 800
		for d := sp.CastTime + castLand + 100*time.Millisecond; d <= sp.CastTime+castLand+20*time.Second; d += 100 * time.Millisecond {
			c.tick(d)
		}
		if c.p.Power != tc.want {
			t.Errorf("%q: %d power, want %d", tc.line, c.p.Power, tc.want)
		}
	}
}

package entity

import "testing"

// Green is exactly "no experience" (Live petition 5690): for every observer
// level and every creature below dark blue, the con is green when a solo kill
// pays nothing and light blue when it pays.
func TestGreenIsNoExperience(t *testing.T) {
	for obs := 1; obs <= 65; obs++ {
		for mob := 1; mob <= obs-3; mob++ {
			c := ConColor(obs, mob)
			pays := killExp(mob, []int{obs}) > 0
			switch {
			case pays && c != ConLightBlue:
				t.Errorf("level %d on a level %d creature: pays experience but cons %d, want light blue", obs, mob, c)
			case !pays && c != ConGreen:
				t.Errorf("level %d on a level %d creature: pays nothing but cons %d, want green", obs, mob, c)
			}
		}
	}
	if c := ConColor(27, 20); c != ConLightBlue {
		t.Errorf("27 on 20 (the petition's lioness) cons %d, want light blue", c)
	}
	if c := ConColor(27, 19); c != ConGreen {
		t.Errorf("27 on 19 cons %d, want green", c)
	}
}

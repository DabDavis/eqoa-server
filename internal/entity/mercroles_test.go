package entity

import "testing"

// A tank merc's base stats go to STA and AGI, a melee fighter's to STR, DEX
// and AGI (mercroles.go), each above what a player of its class spends there
// (SimBaseStats) and within the level's cap; a caster's are a player's (the
// control). A tank's resists come to the cap, what its WIS adds counted.
func TestMercRoleStats(t *testing.T) {
	for _, level := range []int{10, 30, 60} {
		war, sim := MercBaseStats(0, 0, level), SimBaseStats(0, 0, level)
		if war[1] < sim[1] || war[2] <= sim[2] || war[0] > sim[0] {
			t.Errorf("a warrior of %d: %v, a player's %v: want more STA and AGI, no more STR", level, war, sim)
		}
		rog, rsim := MercBaseStats(6, 0, level), SimBaseStats(6, 0, level)
		if rog[0] < rsim[0] || rog[2] <= rsim[2] || rog[3] < rsim[3] || rog[1] > rsim[1] {
			t.Errorf("a rogue of %d: %v, a player's %v: want more STR, AGI and DEX", level, rog, rsim)
		}
		if wiz := MercBaseStats(13, 0, level); wiz != SimBaseStats(13, 0, level) {
			t.Errorf("a wizard of %d: %v, not a player's", level, wiz)
		}
		for i, v := range war {
			if v > StatCapBase(level) {
				t.Errorf("a warrior of %d: stat %d at %d over the cap", level, i, v)
			}
		}
	}
	g := Gear{FR: 400}
	MercTankResists(&g, 70) // 10 from its WIS
	if g.PR != ResistCap-10 || g.FR != 400 || g.AR != ResistCap-10 {
		t.Errorf("resists %+v", g)
	}
	if MercTankAC(60) != 2400 || MercTankAC(10) != 400 {
		t.Errorf("tank AC %d, %d", MercTankAC(60), MercTankAC(10))
	}
}

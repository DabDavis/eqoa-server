package handlers

import "testing"

// Sandstorm's con colours (0x0010's second byte) against the two levels, every
// pair seven PS2 captures hold (pcaps/ps2_*.pcap, breath_capture.pcap; read
// with the entity channels accumulated as CLIENT does:
// claude-notes/aggro-20261025.md). calcConColor gives each one. The yellow
// band above 1 and the light-blue/green edge below 16 are not in them.
func TestConColoursAsSandstormSentThem(t *testing.T) {
	for _, c := range []struct {
		me, them int
		con      byte
	}{
		{51, 49, 3}, {51, 40, 4}, {51, 35, 5}, {51, 34, 5}, {51, 20, 5}, {51, 19, 5},
		{51, 18, 5}, {51, 17, 5}, {51, 10, 5}, {51, 1, 5}, {51, 51, 2}, {51, 54, 0},
		{51, 55, 0}, {51, 60, 0}, {51, 62, 0}, {51, 65, 0},
		{1, 1, 2}, {1, 2, 1}, {1, 35, 0}, {1, 36, 0}, {1, 50, 0},
	} {
		if got := calcConColor(c.me, c.them); got != c.con {
			t.Errorf("a level %d considering a level %d: con %d, Sandstorm's %d", c.me, c.them, got, c.con)
		}
	}
}

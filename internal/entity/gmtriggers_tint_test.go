package entity

import "testing"

// A row's tint (npcs.unk_o) is its armour's on the channel, within CLIENT's table of 15:
// GetTintColor has no bounds check, so anything past it stays 0 (an invisible body under
// S2, armor-tint-system.md).
func TestNPCArmourTintOnTheChannel(t *testing.T) {
	for _, c := range []struct {
		tint int
		want byte
	}{{0, 0}, {13, 13}, {14, 14}, {15, 0}, {-1, 0}, {255, 0}} {
		buf := SerializeNPCUpdate(1, &NPC{ServerID: 1, Name: "x", HP: 1, MaxHP: 1, Tint: c.tint})
		for i := 99; i <= 104; i++ {
			if buf[i] != c.want {
				t.Fatalf("tint %d: byte %d is %d, want %d", c.tint, i, buf[i], c.want)
			}
		}
	}
}

package session

import "testing"

// A kept client's server worlds after a keep restart: a world rebuilt since (another CRC) is not held; one the same
// is; a snapshot from before CRCs were kept keeps every slot but those rebuilt in the release that began keeping them.
func TestKeptServerWorldsRebuilt(t *testing.T) {
	old := ServerWorldCRC
	t.Cleanup(func() { ServerWorldCRC = old })
	ServerWorldCRC = func(slot uint8) (uint32, bool) {
		return map[uint8]uint32{10: 0x5fa4f6a2, 12: 0x2f292767, 13: 0xd47debb1}[slot], slot >= 10 && slot <= 13
	}
	got := stillHeld([]uint8{10, 12, 13}, []uint32{0x5fa4f6a2, 0x2f292767, 0x2c61d844})
	if len(got) != 2 || got[0] != 10 || got[1] != 12 {
		t.Fatalf("rebuilt 13 dropped: %v", got)
	}
	got = stillHeld([]uint8{10, 12, 13}, []uint32{0x5fa4f6a2, 0x2f292767, 0xd47debb1})
	if len(got) != 3 {
		t.Fatalf("the same CRCs: %v", got)
	}
	got = stillHeld([]uint8{10, 12, 13}, nil)
	if len(got) != 2 || got[1] != 12 {
		t.Fatalf("an old snapshot: %v", got)
	}
	if c := serverWorldCRCs([]uint8{10, 13}); len(c) != 2 || c[1] != 0xd47debb1 {
		t.Fatalf("kept CRCs %x", c)
	}
}

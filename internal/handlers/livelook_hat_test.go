package handlers

import (
	"encoding/binary"
	"testing"
)

// The live look's bit 8 (2026-11-16: a hat and an aura stacked on the helm):
// granted only with bit 7 and below, and said in the look in force's third
// u32; without bit 7 it is not.
func TestTheLiveLookHatBit(t *testing.T) {
	third := func(f []byte) uint32 { return binary.LittleEndian.Uint32(f[len(f)-4:]) }
	all := third(buildLiveLookInForce(true, 8, true, true, true, true, true, true, true))
	if all != liveLookArmor|liveLookAuras|liveLookBody|liveLookCape|liveLookHands|liveLookSecond|liveLookHat || liveLookHat != 256 {
		t.Fatalf("all granted: %d", all)
	}
	if v := third(buildLiveLookInForce(true, 8, true, true, true, true, true, false, true)); v&liveLookHat != 0 {
		t.Fatalf("bit 8 without bit 7: %d", v)
	}
	if v := third(buildLiveLookInForce(true, 8, true, true, true, true, true, true, false)); v&liveLookHat != 0 || v&liveLookSecond == 0 {
		t.Fatalf("bit 7 alone: %d", v)
	}
}

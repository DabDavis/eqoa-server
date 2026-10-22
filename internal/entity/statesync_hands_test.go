package entity

import (
	"encoding/binary"
	"testing"
)

// TestPlayerStateCarriesTheHands pins the three items, the three overrides
// and the two hands' attack types at the offsets every player in the
// Sandstorm captures has them (decoded with re-tools/decode_entity_channels.py:
// two rapiers at 67 and 71 with types 5 and 5 at 91 and 92), here a
// two-hander alone in item 0 with type 4 and a bow in item 1 with type 7.
// The control: read one byte early, the types are not there.
func TestPlayerStateCarriesTheHands(t *testing.T) {
	p := &Player{
		ServerID: 42, Name: "Hands", ModelID: 0x70D898C6, Chest: 3,
		Primary: 0x7C7DBEA9, Secondary: 0x0DA9879A, Shield: 0x343CF977, PrimaryType: 4, SecondaryType: 7,
	}
	buf := SerializePlayerUpdate(500000001, p)
	items := [3]uint32{binary.LittleEndian.Uint32(buf[67:71]), binary.LittleEndian.Uint32(buf[71:75]), binary.LittleEndian.Uint32(buf[75:79])}
	if items != [3]uint32{0x7C7DBEA9, 0x0DA9879A, 0x343CF977} {
		t.Errorf("items %08X", items)
	}
	for i := 79; i < 91; i++ {
		if buf[i] != 0 {
			t.Errorf("override byte %d is %d, want 0", i, buf[i])
		}
	}
	if buf[91] != 4 || buf[92] != 7 {
		t.Errorf("types %d %d, want 4 7", buf[91], buf[92])
	}
	if buf[93] != 3 {
		t.Errorf("chest %d, want 3: the armour starts after the types", buf[93])
	}
	if buf[90] == 4 && buf[91] == 7 {
		t.Error("the control: one byte early must not read the types")
	}
}

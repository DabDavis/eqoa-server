// wirediff serialises entities exactly as the server would and prints the
// 162-byte entity state buffer, so it can be diffed field-by-field against a
// retail buffer pulled out of pcaps/ps2_sandstorm.pcap.
//
// No networking and no client: the point is to compare the BYTES WE PRODUCE
// against the bytes retail produces, which is the only way documented offsets
// have ever been settled in this project — claude-notes/state-channel-formats.md
// and the wire disagree in at least two places (byte 110 is the class on the
// wire, not the sex; the doc's "108 Robe" is the face).
//
//	cd go-eqoa-server && go run ./cmd/wirediff
package main

import (
	"fmt"

	"github.com/eqoa/server/internal/entity"
)

// retail buffers lifted from pcaps/ps2_sandstorm.pcap via
//
//	python3 re-tools/drdp_pcap_parser.py pcaps/ps2_sandstorm.pcap --direction s2c --hex \
//	  | python3 re-tools/decode_entity_channels.py --raw
const (
	retailNPC = "00 cd e4 00 00 82 20 c9 73 00 5d fe 1d e2 f3 c3 00 00 00 00 00 00 00 00 00 00 01 ff c6 98 d8 70 00 00 00 00 00 00 80 3f 00 00 00 00 00 00 00 00 00 00 ff fa 58 42 c2 ca 21 c0 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 03 03 03 03 03 00 ff ff ff ff ff ff 00 03 00 03 ff 00 06 00 00 00 00 4d 65 72 63 68 61 6e 74 20 53 68 61 64 65 68 61 6e 64 00 00 00 00 00 00 19 00 00 00 00 00 81 00 ff ff ff ff 00 18 00 00 01 00 74 73 72 71"
	// Mcgregor: a level 51 human cleric in full armor set 7.
	retailPlayer = "01 a2 67 9f 1d 82 20 ca b6 00 5d e8 1d f1 43 2b 00 00 00 00 00 00 00 00 00 00 01 ff c6 98 d8 70 00 00 00 00 00 00 80 3f 00 00 00 00 00 00 00 00 00 00 00 00 00 00 81 d5 ea 40 00 00 00 00 00 00 00 00 00 1a 99 dd f4 1a 99 dd f4 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 05 05 07 07 07 07 07 07 01 01 01 01 01 01 01 02 01 00 02 09 06 00 00 00 00 4d 63 67 72 65 67 6f 72 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 33 00 02 01 00 05 00 00 ff ff ff ff 00 18 00 00 01 00 74 73 72 71"
)

// fields worth naming in the diff output.
var fields = []struct {
	lo, hi int
	name   string
}{
	{0, 1, "update counter"}, {1, 5, "ObjectID"}, {5, 6, "unused"},
	{6, 15, "position XYZ"}, {15, 16, "facing"}, {26, 27, "HP flag"},
	{27, 28, "HP"}, {28, 32, "ModelID"}, {36, 40, "scale"},
	{40, 43, "velocity"}, {58, 59, "animation"}, {59, 63, "target"},
	{67, 71, "primary"}, {71, 75, "secondary"}, {75, 79, "shield"},
	{93, 99, "ARMOR SLOTS"}, {99, 105, "ARMOR TINTS"}, {105, 108, "hair"},
	{108, 109, "face"}, {109, 110, "robe"}, {110, 111, "CLASS (not sex)"},
	{111, 112, "const 6"}, {112, 116, "unknown"}, {116, 140, "name"},
	{140, 141, "level"}, {141, 142, "movement"}, {142, 143, "name colour"},
	{143, 144, "targetable"}, {144, 145, "race"}, {145, 147, "NPCType"},
	{147, 158, "unknown tail"}, {158, 162, "trsq"},
}

func parse(s string) []byte {
	var out []byte
	var b int
	var n int
	for i := 0; i < len(s); i++ {
		c := s[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		default:
			continue
		}
		b = b<<4 | v
		if n++; n == 2 {
			out = append(out, byte(b))
			b, n = 0, 0
		}
	}
	return out
}

func diff(label string, ours, theirs []byte) {
	fmt.Printf("\n=== %s ===\n", label)
	fmt.Printf("%-18s %-26s %-26s\n", "field", "OURS", "RETAIL")
	for _, f := range fields {
		if f.hi > len(ours) || f.hi > len(theirs) {
			continue
		}
		o, t := ours[f.lo:f.hi], theirs[f.lo:f.hi]
		same := string(o) == string(t)
		mark := "  "
		if !same {
			mark = "!!"
		}
		fmt.Printf("%s %-16s %-26s %-26s\n", mark, f.name, hex(o), hex(t))
	}
}

func hex(b []byte) string {
	s := ""
	for i, x := range b {
		if i == 8 && len(b) > 10 {
			s += "… "
			break
		}
		s += fmt.Sprintf("%02x ", x)
	}
	return s
}

func main() {
	n := &entity.NPC{
		Name: "Merchant Shadehand", Level: 25, HP: 100, MaxHP: 100, Size: 1.0,
		ModelID: 0x70D898C6,
		Torso:   3, Forearm: 3, NPCGloves: 3, NPCLegs: 3, Feet: 3, Head: 0,
		HairColor: 0, HairLength: 3, HairStyle: 0, Face: 3,
		Race: "0", NPCType: 0x8100,
	}
	diff("NPC (townsfolk, armor set 3)", entity.SerializeNPCUpdate(0x0000E4CD, n), parse(retailNPC))

	p := &entity.Player{
		Name: "Mcgregor", Level: 51, Race: 0, Class: 9, Sex: 0, Size: 1.0,
		ModelID: 0x70D898C6,
		Chest:   7, Bracer: 7, Gloves: 7, Legs: 7, Boots: 7, Helm: 7,
		HairColor: 1, HairLength: 2, HairStyle: 1, FaceOption: 0, Robe: 2,
	}
	diff("Player (armored)", entity.SerializePlayerUpdate(0x1D9F67A2, p), parse(retailPlayer))
}

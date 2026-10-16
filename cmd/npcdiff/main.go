// npcdiff prints our 162-byte NPC state buffer for an NPC shaped like retail's
// "Menford Crickle", so it can be diffed offset-by-offset against the capture
// in pcaps/ps2_sandstorm.pcap. Retail renders that NPC clothed.
package main

import (
	"encoding/hex"
	"fmt"

	"github.com/eqoa/server/internal/entity"
)

func main() {
	n := &entity.NPC{
		ServerID: 1,
		Name:     "Menford Crickle",
		Level:    25,
		ModelID:  0x70D898C6,
		NPCType:  0x0080, // DB form; serializer swaps to wire 0x8000
		HP:       100, MaxHP: 100,
		HairColor: 0, HairLength: 2, HairStyle: 0, Face: 1,
		Race: "0",
	}
	fmt.Println(hex.EncodeToString(entity.SerializeNPCUpdate(0x013133DC, n)))
}

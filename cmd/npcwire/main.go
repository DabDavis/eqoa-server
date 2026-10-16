// npcwire dumps the state-channel bytes our server actually puts on the wire
// for real NPCs out of eqoa.db, through the real DB -> entity -> serializer
// path. Bytes 145-146 decide NPC vs player; a player-path entity renders with
// no skin, no clothes and invisible hands and feet.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

func main() {
	path := "eqoa.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		log.Fatal(err)
	}
	recs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		log.Fatal(err)
	}

	bad, total := 0, 0
	seen := map[int]bool{}
	fmt.Printf("%-26s %-10s %-14s %s\n", "NAME", "db type", "wire 145,146", "client uint16 LE")
	for _, r := range recs {
		n := &entity.NPC{
			ServerID: 1, Name: r.NPCName, Level: r.NPCLevel,
			ModelID: int64(r.ModelID), NPCType: r.NPCType,
			HP: 100, MaxHP: 100,
		}
		buf := entity.SerializeNPCUpdate(100001, n)
		v := uint16(buf[145]) | uint16(buf[146])<<8
		total++
		if v&0x8000 == 0 {
			bad++
		}
		if !seen[r.NPCType] {
			seen[r.NPCType] = true
			flag := "NPC"
			if v&0x8000 == 0 {
				flag = "*** PLAYER PATH -> NAKED ***"
			}
			name := r.NPCName
			if len(name) > 25 {
				name = name[:25]
			}
			fmt.Printf("%-26s 0x%04X     %02X,%02X          0x%04X  %s\n",
				name, r.NPCType, buf[145], buf[146], v, flag)
		}
	}
	fmt.Printf("\n%d of %d NPCs would render as players\n", bad, total)
}

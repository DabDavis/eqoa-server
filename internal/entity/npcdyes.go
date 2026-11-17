package entity

import (
	"fmt"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// npcdyes.go — dyes on the NPCs of a name (table npcdyes, database/npcdyes.go;
// the owner, 2026-11-16: the Holiday Elf "with red and green dyed clothing", an
// ogre Santa Claus "in a santa suit"). A client of the live look (vanilla) is
// sent the dyes in the channel's colours (LiveChannel, as a character's: each
// armour slot, its trim, the robe); every channel carries the disc tint nearest
// each dyed armour slot and the robe (bytes 99-104 and 110), which is all a PS2
// draws: one of the fifteen tints a slot, no trim.

//
// A robe row may put a robe on the name too (npcdyes.robe: the owner,
// 2026-11-17, Toymaker Tinsel, the Holiday Elf and the Weapon Enchanter in
// Santa's robe), drawn by every client (byte 109); and a row may hold only in
// its season (npcdyes.season), read again as the season turns
// (LoadSeasonLooks).

var npcDyes atomic.Pointer[map[string]*Dyes]

// npcDyeRobes is the robes the npcdyes robe rows put on, by NPCClassKey.
var npcDyeRobes atomic.Pointer[map[string]byte]

// LoadNPCDyes reads table npcdyes into the world: at the start, after a GM's
// edit and as a season turns. A client sees a change when the NPC's channel is
// next sent. It reports how many names it dyes or robes.
func (w *World) LoadNPCDyes(db *database.SQLiteDB) (int, error) {
	rows, robes, err := db.NPCDyeLooks()
	if err != nil {
		return 0, fmt.Errorf("npcdyes: %w", err)
	}
	m := make(map[string]*Dyes, len(rows))
	for k, v := range rows {
		d := Dyes(v)
		m[k] = &d
	}
	r := make(map[string]byte, len(robes))
	n := len(m)
	for k, v := range robes {
		r[k] = byte(v)
		if m[k] == nil {
			n++
		}
	}
	npcDyes.Store(&m)
	npcDyeRobes.Store(&r)
	return n, nil
}

// LoadSeasonLooks reads again what a season's turn changes of the NPCs' looks:
// the body tints (npctint, sent to the clients that draw them) and the dyes
// and robes (npcdyes), each of which may hold only in a season. It reports the
// names tinted, the clients told and the names dyed.
func (w *World) LoadSeasonLooks(db *database.SQLiteDB) (tinted, told, dyed int, err error) {
	if tinted, told, err = w.LoadNPCTints(db); err != nil {
		return
	}
	dyed, err = w.LoadNPCDyes(db)
	return
}

// putNPCDyeRobe puts the robe an npcdyes robe row puts on the NPCs of a name
// in an NPC's channel (byte 109), if one does.
func putNPCDyeRobe(name string, buf []byte) {
	m := npcDyeRobes.Load()
	if m == nil || len(*m) == 0 || len(buf) < 110 {
		return
	}
	if r, ok := (*m)[database.NPCClassKey(name)]; ok {
		buf[109] = r
	}
}

// npcDyesOf is the dyes of the NPCs of a name, or nil.
func npcDyesOf(name string) *Dyes {
	m := npcDyes.Load()
	if m == nil || len(*m) == 0 {
		return nil
	}
	return (*m)[database.NPCClassKey(name)]
}

// putNPCDyeTints puts the disc tint nearest each dyed armour slot and the
// robe in an NPC's channel (bytes 99-104, 110): what a PS2 draws of its dyes.
func putNPCDyeTints(d *Dyes, buf []byte) {
	if d == nil || len(buf) < 111 {
		return
	}
	for i := 0; i < 6; i++ {
		if c := d[database.DyeChest+i]; c != 0 {
			buf[99+i] = nearestTint(c)
		}
	}
	if c := d[database.DyeRobe]; c != 0 {
		buf[110] = nearestTint(c)
	}
}

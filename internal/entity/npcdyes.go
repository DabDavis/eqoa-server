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

var npcDyes atomic.Pointer[map[string]*Dyes]

// LoadNPCDyes reads table npcdyes into the world: at the start and after a
// GM's edit. A client sees a change when the NPC's channel is next sent.
func (w *World) LoadNPCDyes(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCDyes()
	if err != nil {
		return 0, fmt.Errorf("npcdyes: %w", err)
	}
	m := make(map[string]*Dyes, len(rows))
	for k, v := range rows {
		d := Dyes(v)
		m[k] = &d
	}
	npcDyes.Store(&m)
	return len(m), nil
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

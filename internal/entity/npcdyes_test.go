package entity

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// An NPC of a dyed name (npcdyes): a PS2's channel carries the nearest disc
// tint of each dyed slot and of the robe; a live-look client's the colours
// themselves; another NPC is as it was.
func TestAnNPCsDyesReachEachClientAsItDraws(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcdyes (name, slot, color) VALUES ('Holiday Elf', 0, 3358596863), ('Holiday Elf', 3, 512507135), ('Holiday Elf', 9, 3358596863)`); err != nil {
		t.Fatal(err) // chest and robe 0xC8301EFF red, legs 0x1E8C3CFF green
	}
	f := newAggroFix(t, 10)
	elf := f.npc(2001, "Holiday Elf", 20, creationBody, 1010, 1000)
	other := f.npc(2002, "giant beetle", 20, creationBody, 1000, 1010)
	if n, err := f.w.LoadNPCDyes(db); err != nil || n != 1 {
		t.Fatalf("LoadNPCDyes: %d, %v", n, err)
	}
	t.Cleanup(func() { npcDyes.Store(nil) })
	f.w.mu.Lock()
	ps2 := f.w.serializeEntityForChannel(f.sess, elf.ServerID, 5)
	plain := f.w.serializeEntityForChannel(f.sess, other.ServerID, 6)
	f.sess.SetLiveLook(true)
	live := f.w.serializeEntityForChannel(f.sess, elf.ServerID, 5)
	f.w.mu.Unlock()
	if ps2[99] != 8 || ps2[102] != 5 || ps2[110] != 8 || ps2[100] != 0 {
		t.Errorf("a PS2's tints: chest %d legs %d robe %d bracer %d; want red 8, green 5, red 8, none 0", ps2[99], ps2[102], ps2[110], ps2[100])
	}
	if plain[99] != 0 || plain[110] != 0 {
		t.Errorf("the control: %d %d", plain[99], plain[110])
	}
	if len(live) < LiveEntityStateSize || binary.BigEndian.Uint32(live[111:]) != 0xC8301EFF || binary.BigEndian.Uint32(live[111+15:]) != 0x1E8C3CFF ||
		binary.BigEndian.Uint32(live[141:]) != 0xC8301EFF {
		t.Errorf("the live look: chest % X legs % X robe % X", live[111:115], live[126:130], live[141:145])
	}
}

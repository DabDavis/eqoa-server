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

// Season-scoped dyes and a robe put on (the owner, 2026-11-17: "make the
// toymaker tinsel, holiday elf weapon enchanter wear santa hats and santas
// robe"): while Christmas is off the Weapon Enchanter wears its own look, no
// robe; turned on (LoadSeasonLooks) it wears robe 0 in Santa's red, a PS2 the
// red tint, the live look the colour; turned off again it is as it was. Its
// hat (npctint, season Christmas) comes and goes with it.
func TestSeasonalDyesPutSantasRobeOnOnlyInTheSeason(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcdyes (name, slot, color, season, robe) VALUES
		('Weapon Enchanter', 9, 4280295423, 'Christmas', 0), ('Weapon Enchanter', 11, 4294967295, 'Christmas', -1),
		('Weapon Enchanter', 2, 471604479, '', -1);
		INSERT INTO npctint (name, color, aura_kind, aura_source, aura_nodes, season) VALUES ('Weapon Enchanter', 0, 10, 2718273763, 1, 'christmas')`); err != nil {
		t.Fatal(err) // robe 0xFF201FFF, chest trim white, gloves 0x1C1C1CFF always
	}
	f := newAggroFix(t, 10)
	enc := f.npc(2001, "Weapon Enchanter", 20, creationBody, 1010, 1000)
	enc.RobeStyle = "255"
	t.Cleanup(func() { npcDyes.Store(nil); npcDyeRobes.Store(nil) })
	look := func(on bool) ([]byte, []byte, int) {
		t.Helper()
		if _, err := db.SetSeason("Christmas", on, "test"); err != nil {
			t.Fatal(err)
		}
		tinted, _, _, err := f.w.LoadSeasonLooks(db)
		if err != nil {
			t.Fatal(err)
		}
		f.w.mu.Lock()
		defer f.w.mu.Unlock()
		f.sess.SetLiveLook(false)
		ps2 := f.w.serializeEntityForChannel(f.sess, enc.ServerID, 5)
		f.sess.SetLiveLook(true)
		live := f.w.serializeEntityForChannel(f.sess, enc.ServerID, 5)
		return ps2, live, tinted
	}
	for _, on := range []bool{false, true, false} {
		ps2, live, tinted := look(on)
		robe, tint, hats := byte(0xFF), byte(0), 0
		if on {
			robe, tint, hats = 0, nearestTint(0xFF201FFF), 1
		}
		if ps2[109] != robe || ps2[110] != tint || ps2[101] != nearestTint(0x1C1C1CFF) {
			t.Errorf("season %v: a PS2's robe %d tint %d gloves %d; want %d %d %d", on, ps2[109], ps2[110], ps2[101], robe, tint, nearestTint(0x1C1C1CFF))
		}
		if live[149] != robe {
			t.Errorf("season %v: the live look's robe %d, want %d", on, live[149], robe)
		}
		if got := binary.BigEndian.Uint32(live[141:]); on && got != 0xFF201FFF {
			t.Errorf("season on: the live look's robe colour %08X", got)
		}
		if tinted != hats {
			t.Errorf("season %v: %d names tinted, want %d", on, tinted, hats)
		}
	}
}

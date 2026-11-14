package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The GM looks (gmlooks.go): every game race has the hair colour row, the gnome
// its cap and glasses too, each a GM row never offered (RaceOptionsFor) but laid
// as any look (lookParts) on the body's entries; Paint's colours; who shows them.
func TestGMLookRows(t *testing.T) {
	for race := 0; race <= 9; race++ {
		rows := gmLookRows(race)
		want := 1
		if race == raceGnome {
			want = 3
		}
		if len(rows) != want || rows[0].Key != LookHair || !rows[0].GM {
			t.Fatalf("race %d: %+v", race, rows)
		}
		for _, r := range RaceOptionsFor(race, 3) {
			if r.GM {
				t.Fatalf("race %d offers the GM row %s", race, r.Key)
			}
		}
	}
	if gmLookRows(20) != nil {
		t.Fatal("a custom race has the game's hair rows")
	}
	looks := []database.CharLook{{Part: LookGnomeCap, Kind: database.LookColour, Value: Paint(0xFF2020)},
		{Part: LookGlasses, Kind: database.LookColour, Value: Paint(0xFFC040)}, {Part: LookHair, Kind: database.LookColour, Value: Paint(0xFFFFFF)}}
	_, v, p := lookParts(raceGnome, 0xA99C6B15, looks)
	if len(p) != 3 || p[0].Entries[0] != 0 || len(p[0].Entries) != 6 || p[1].Entries[0] != 10 || p[2].Entries[0] != 9 || v[1] != Paint(0xFF2020) {
		t.Fatalf("the gnome man's parts: %+v %x", p, v)
	}
	if _, _, p := lookParts(raceGnome, 0x4C4084FC, looks[:2]); len(p) != 2 || p[0].Entries[0] != 12 || p[1].Entries[0] != 11 {
		t.Fatalf("the gnome woman's: %+v", p)
	}
	if _, _, p := lookParts(0, 0x70D898C6, looks); len(p) != 1 || p[0].Entries[0] != 6 {
		t.Fatalf("a human: the hair alone: %+v", p)
	}
	for _, c := range []struct {
		rgb  uint32
		want Recolour
	}{{0xFF2020, 0x0800FFA0}, {0xFFC040, 0x081CFFB2}, {0xFFFFFF, 0x080000FF}, {0x000000, 0x08000013}} {
		if got := Paint(c.rgb); got != c.want {
			t.Errorf("Paint(%06X) = %08X, want %08X", c.rgb, got, c.want)
		}
	}
	for _, c := range []struct {
		part                     string
		race, sex, style, length int
		ok                       bool
	}{{LookHair, 0, 0, 0, 0, true}, {LookHair, 20, 0, 0, 0, false},
		{LookGnomeCap, 3, 0, 0, 3, true}, {LookGnomeCap, 3, 0, 3, 2, false}, {LookGnomeCap, 0, 0, 3, 3, false},
		{LookGlasses, 3, 0, 2, 0, true}, {LookGlasses, 3, 1, 2, 0, false}, {LookGlasses, 3, 1, 3, 0, true}, {LookGlasses, 2, 0, 3, 3, false}} {
		if got := GMLookShown(c.part, c.race, c.sex, c.style, c.length) == ""; got != c.ok {
			t.Errorf("%+v: shown %v", c, got)
		}
	}
}

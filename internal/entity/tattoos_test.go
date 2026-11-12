package entity

import (
	"bytes"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// withTattoos runs the test with world.skin_shades and world.tattoos as given.
func withTattoos(t *testing.T, skins, tattoos bool) {
	t.Helper()
	was := Creation()
	SetCreationRules(CreationRules{Races: was.Races, AllClasses: was.AllClasses, Skins: skins, Tattoos: tattoos})
	t.Cleanup(func() { SetCreationRules(was) })
}

// keys is the rows' keys in order.
func keys(rows []OptionRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Key)
	}
	return out
}

// The barbarian alone has the tattoo row, on a server that offers it, before
// its skin (a client lays the tattoo, then shades the face); on each body's face
// entry (the man's 9 and his chest 5, the woman's 6); the man's first step the face's own
// (value 0), then Off, Bands, Crest as ModeTattoo; the woman's Off (0), Bands,
// Crest. The control: off, no race has it.
func TestTheBarbarianHasATattooRow(t *testing.T) {
	withTattoos(t, true, false)
	for race := range 10 {
		if slices.Contains(keys(RaceOptions(race)), "tattoo") {
			t.Fatalf("the control: race %d has a tattoo on a server without", race)
		}
	}
	withTattoos(t, true, true)
	for race := range 10 {
		want := []string{"skin"}
		if race == raceBarbarian {
			want = []string{"tattoo", "skin"}
		}
		if got := keys(RaceOptions(race)); !slices.Equal(got, want) {
			t.Fatalf("race %d: %v, want %v", race, got, want)
		}
	}
	row := RaceOptions(raceBarbarian)[0]
	if row.Kind != KindColour || row.Answer != 3 || row.Label != "Tattoo" {
		t.Fatalf("row %+v", row)
	}
	if p := row.PartsOf(0x61CA3EDF); len(p) != 1 || !slices.Equal(p[0].Entries, []uint8{9, 5}) || len(p[0].Rects) != 0 {
		t.Errorf("his face: %+v", p)
	}
	if p := row.PartsOf(0xC44AD516); len(p) != 1 || !slices.Equal(p[0].Entries, []uint8{6}) {
		t.Errorf("her face: %+v", p)
	}
	his := []uint32{0, 4 << 24, 4<<24 | 1<<16, 4<<24 | 2<<16}
	hers := []uint32{0, 4<<24 | 1<<16, 4<<24 | 2<<16}
	for sex, want := range [][]uint32{his, hers} {
		var got []uint32
		for _, s := range row.Steps(sex) {
			got = append(got, s.Value)
		}
		if !slices.Equal(got, want) {
			t.Errorf("sex %d steps %#x, want %#x", sex, got, want)
		}
	}
	// the tattoo alone, without the shades: the barbarian's one row
	withTattoos(t, false, true)
	if got := keys(RaceOptions(raceBarbarian)); !slices.Equal(got, []string{"tattoo"}) {
		t.Fatalf("no shades: %v", got)
	}
	if got := keys(RaceOptions(0)); len(got) != 0 {
		t.Fatalf("no shades: a human has %v", got)
	}
}

// The tattoo goes only to a client that answered the offer 3 (it draws it):
// one that answered 2 is sent the bytes it got before the tattoo, one that
// answered 1 the custom races' alone; with the tattoo and no shades, a client
// that answered 2 is told no game race and one that answered 3 the barbarian.
func TestTheTattooOnlyToAClientThatDrawsIt(t *testing.T) {
	SetRaceDefs(nil)
	withTattoos(t, true, false)
	before2, before1 := EncodeRaceOptionsAt(2), EncodeRaceOptionsAt(1)
	withTattoos(t, true, true)
	if got := EncodeRaceOptionsAt(2); !bytes.Equal(got, before2) || !bytes.Equal(EncodeRaceOptionsFor(true), before2) {
		t.Fatalf("answered 2: %d bytes, want the %d before", len(got), len(before2))
	}
	if got := EncodeRaceOptionsAt(1); !bytes.Equal(got, before1) {
		t.Fatal("answered 1: changed")
	}
	three := EncodeRaceOptionsAt(3)
	if !bytes.Contains(three, []byte("\x06tattoo\x06Tattoo")) || len(three) <= len(before2) || three[1] != 10 {
		t.Fatalf("answered 3: %d bytes, %d races", len(three), three[1])
	}
	withTattoos(t, false, true)
	if !HasRaceOptions() {
		t.Fatal("the tattoo alone: nothing offered")
	}
	if b := EncodeRaceOptionsAt(2); b[1] != 0 {
		t.Fatalf("the tattoo alone, answered 2: %d races", b[1])
	}
	if b := EncodeRaceOptionsAt(3); b[1] != 1 || b[2] != raceBarbarian {
		t.Fatalf("the tattoo alone, answered 3: %x", b[:4])
	}
}

// A barbarian's tattoo is a look: chosen at creation (his Crest, her Bands), a
// charlook row "tattoo"; his first step none; laid on the face entry of the body
// it was chosen on, before the skin's shade whatever order the rows were kept
// in; re-chosen at select's Customize (Off, then the face's own again, which
// takes the row away), the skin kept; her fourth step refused (she has three).
func TestATattooIsALook(t *testing.T) {
	withTattoos(t, true, true)
	_, looks, err := ResolveOptions(raceBarbarian, 0, []OptionChoice{{Key: "tattoo", Step: 3}, {Key: "skin", Step: 8}})
	if err != nil || len(looks) != 2 {
		t.Fatalf("his crest: %+v %v", looks, err)
	}
	tat := looks[slices.IndexFunc(looks, func(l database.CharLook) bool { return l.Part == "tattoo" })]
	if tat.Kind != database.LookColour || tat.Value != Tattoo(tattooCrest) {
		t.Fatalf("his crest: %+v", tat)
	}
	if _, none, err := ResolveOptions(raceBarbarian, 0, []OptionChoice{{Key: "tattoo", Step: 0}}); err != nil || len(none) != 0 {
		t.Fatalf("the face's own: %+v %v", none, err)
	}
	if _, l, err := ResolveOptions(raceBarbarian, 1, []OptionChoice{{Key: "tattoo", Step: 1}}); err != nil || len(l) != 1 || l[0].Value != Tattoo(tattooBands) {
		t.Fatalf("her bands: %+v %v", l, err)
	}
	if _, _, err := ResolveOptions(raceBarbarian, 1, []OptionChoice{{Key: "tattoo", Step: 3}}); err == nil {
		t.Fatal("her fourth step: kept")
	}
	if _, _, err := ResolveOptions(0, 0, []OptionChoice{{Key: "tattoo", Step: 1}}); err == nil {
		t.Fatal("a human's tattoo: kept")
	}
	// kept skin first: the tattoo still goes first
	k, v, p := lookParts(raceBarbarian, 0x61CA3EDF, []database.CharLook{looks[1], looks[0]})
	if len(p) != 2 || v[0] != Tattoo(tattooCrest) || k[0] != KindColour || !slices.Equal(p[0].Entries, []uint8{9, 5}) || v[1]>>24 != ModeShade {
		t.Fatalf("on his body: %v %#x %+v", k, v, p)
	}
	if _, _, p := lookParts(raceBarbarian, 0xC44AD516, []database.CharLook{tat}); len(p) != 1 || !slices.Equal(p[0].Entries, []uint8{6}) {
		t.Fatalf("on her body: %+v", p)
	}
	skin := database.CharLook{Part: "skin", Kind: 2, Value: skinShades[3].Value}
	have := []database.CharLook{skin, tat}
	got, changed, err := ReselectOptions(raceBarbarian, 0, have, []OptionChoice{{Key: "tattoo", Step: 1}})
	if err != nil || !slices.Equal(changed, []string{"tattoo"}) || len(got) != 2 || got[0] != skin || got[1].Value != Tattoo(tattooNone) {
		t.Fatalf("Off at Customize: %+v %v %v", got, changed, err)
	}
	got, _, err = ReselectOptions(raceBarbarian, 0, have, []OptionChoice{{Key: "tattoo", Step: 0}})
	if err != nil || len(got) != 1 || got[0] != skin {
		t.Fatalf("the face's own at Customize: %+v %v", got, err)
	}
	withTattoos(t, true, false)
	if _, _, err := ReselectOptions(raceBarbarian, 0, have, []OptionChoice{{Key: "tattoo", Step: 1}}); err == nil {
		t.Error("the control: no tattoo on the server, yet kept")
	}
}

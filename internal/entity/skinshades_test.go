package entity

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// withSkins runs the test with world.skin_shades as on.
func withSkins(t *testing.T, on bool) {
	t.Helper()
	was := Creation()
	SetCreationRules(CreationRules{Races: was.Races, AllClasses: was.AllClasses, Skins: on})
	t.Cleanup(func() { SetCreationRules(was) })
}

// Every game race has its skin row on its own two bodies (the bodies its
// characters wear: getModelID's) on a server that offers the shades, each
// shade a ModeShade colour but the first (the race's own); none on a server
// that does not (the control), and never a body row (a game race chooses its
// body by sex alone).
func TestEveryGameRaceHasItsSkinShades(t *testing.T) {
	withSkins(t, false)
	for race := range 10 {
		if rows := RaceOptionsFor(race, raceAnswerNewest); len(rows) != 0 { // the offered rows (the GM rows, gmlooks.go, never are)
			t.Fatalf("the control: race %d has rows on a server without shades: %+v", race, rows)
		}
	}
	withSkins(t, true)
	bodies := map[int][2]uint32{
		0: {0x70D898C6, 0x0D537335}, 1: {0xD8D56FF8, 0xE18919F7}, 2: {0x7ED89DAA, 0x8C9B4B39}, 3: {0xA99C6B15, 0x4C4084FC},
		4: {0xAA5B0447, 0x350BFDBE}, 5: {0x4C6FA532, 0x4DE32701}, 6: {0x61CA3EDF, 0xC44AD516}, 7: {0xA3DB3FE2, 0x8A27A731},
		8: {0x320C0B47, 0x4F1A3CBE}, 9: {0x84807490, 0xACC00B4F},
	}
	for race, b := range bodies {
		rows := RaceOptionsFor(race, raceAnswerNewest)
		if len(rows) != 1 || rows[0].Key != "skin" || rows[0].Kind != KindColour {
			t.Fatalf("race %d: %+v", race, rows)
		}
		for sex, body := range b {
			if p := rows[0].PartsOf(body); len(p) != 1 || len(p[0].Entries) < 4 {
				t.Errorf("race %d sex %d: parts %+v", race, sex, p)
			}
			if ChosenBody(race, body) {
				t.Errorf("race %d: its own body read as chosen", race)
			}
		}
		for i, s := range rows[0].Steps(0) {
			if (i == 0) != (s.Value == 0) || (i > 0 && s.Value>>24 != ModeShade) {
				t.Errorf("race %d step %d %q: %#x", race, i, s.Name, s.Value)
			}
		}
	}
	if s := Shade(27, 42, 23); s != 3<<24|19<<16|107<<8|59 {
		t.Errorf("Shade(27, 42, 23) = %#x", s)
	}
}

// The game races' rows go only to a client that shows them under the four
// (EncodeRaceOptionsFor(true)): one that answered 1 is sent the custom races'
// alone, byte for byte as before the shades; a server with neither offers
// nothing (HasRaceOptions).
func TestGameRacesRowsOnlyToAClientThatShowsThem(t *testing.T) {
	SetRaceDefs(nil)
	withSkins(t, false)
	if HasRaceOptions() {
		t.Fatal("the control: no custom race, no shades, yet options")
	}
	old := EncodeRaceOptionsFor(true)
	withSkins(t, true)
	if !HasRaceOptions() {
		t.Fatal("shades on: no options")
	}
	if got := EncodeRaceOptionsFor(false); !bytes.Equal(got, old) || !bytes.Equal(EncodeRaceOptions(), old) {
		t.Fatalf("a client that answered 1: %x, want %x", got, old)
	}
	b := EncodeRaceOptionsFor(true)
	if b[0] != raceOptionsVersion || b[1] != 10 || b[2] != 0 {
		t.Fatalf("a client that answered 2: %x", b[:8])
	}
}

// A new character's shade is a charlook row "skin" (its first step none); a
// human's look lays it on the human's own body's skin entries, on its female
// body's for a woman, and nothing on a body that is not its own (a GM's form,
// an illusion kept as its model: the control).
func TestASkinShadeIsALook(t *testing.T) {
	withSkins(t, true)
	_, looks, err := ResolveOptions(0, 1, []OptionChoice{{Key: "skin", Step: 8}})
	if err != nil || len(looks) != 1 || looks[0].Part != "skin" || looks[0].Kind != database.LookColour || looks[0].Value != skinShades[8].Value {
		t.Fatalf("Dark: %+v %v", looks, err)
	}
	if _, none, err := ResolveOptions(0, 0, []OptionChoice{{Key: "skin", Step: 0}}); err != nil || len(none) != 0 {
		t.Fatalf("Natural: %+v %v", none, err)
	}
	if _, _, err := ResolveOptions(0, 0, []OptionChoice{{Key: "skin", Step: len(skinShades)}}); err == nil {
		t.Fatal("a step past the row's")
	}
	if _, _, err := ResolveOptions(0, 0, []OptionChoice{{Key: "body", Step: 0}}); err == nil {
		t.Fatal("a body row a game race has not")
	}
	k, v, p := lookParts(0, 0x0D537335, looks)
	if len(p) != 1 || k[0] != KindColour || v[0] != skinShades[8].Value || p[0].Body != 0x0D537335 {
		t.Fatalf("on her body: %v %v %+v", k, v, p)
	}
	if _, _, p := lookParts(0, 0x84807490, looks); len(p) != 0 {
		t.Fatalf("the control: on an ogre's body %+v", p)
	}
	// and the list every client that asked is told
	b := EncodeRaceLooks([]database.LookCharacter{{Name: "Zara", Race: 0, Sex: 1, ModelID: 0x0D537335, Looks: looks}})
	if !bytes.Contains(b, []byte("Zara")) {
		t.Fatalf("the looks: %x", b)
	}
}

// A retail skin shade can be set or reset using the legacy protocol.
func TestReselectOptions(t *testing.T) {
	withSkins(t, true)
	body, reset, looks, err := ReselectOptions(8, 0, 2, false, []OptionChoice{{Key: "skin", Step: 9}})
	if err != nil || body != 0 || len(reset) != 0 || len(looks) != 1 || looks[0].Part != "skin" || looks[0].Value != skinShades[9].Value {
		t.Fatalf("Ebony: model %x, reset %v, looks %+v, error %v", body, reset, looks, err)
	}
	_, reset, looks, err = ReselectOptions(8, 0, 2, false, []OptionChoice{{Key: "skin", Step: 0}})
	if err != nil || len(reset) != 1 || reset[0] != "skin" || len(looks) != 0 {
		t.Fatalf("Natural again: %v %+v %v", reset, looks, err)
	}
	for name, c := range map[string][]OptionChoice{
		"no row":   {{Key: "hair", Step: 1}},
		"too far":  {{Key: "skin", Step: 99}},
		"twice":    {{Key: "skin", Step: 1}, {Key: "skin", Step: 2}},
		"negative": {{Key: "skin", Step: -1}},
	} {
		if _, _, _, err := ReselectOptions(8, 0, 2, false, c); err == nil {
			t.Errorf("%s: kept", name)
		}
	}
	withSkins(t, false)
	if _, _, _, err := ReselectOptions(8, 0, 2, false, []OptionChoice{{Key: "skin", Step: 1}}); err == nil {
		t.Error("the control: no shades on the server, yet kept")
	}
}

package entity

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// signed is a body as characters.modelid and racedefs hold it.
func signed(u uint32) int64 { return int64(int32(u)) }

// withOptionRaces makes rows the races of racedefs for a test (the hill giant, the
// kerran and the fungusman on their real bodies), and puts the table back after.
func withOptionRaces(t *testing.T, rows ...database.RaceDefRow) {
	t.Helper()
	if rows == nil {
		rows = []database.RaceDefRow{
			{ID: 11, Name: "Hill Giant", WireRace: 9, BodyMale: -5117396, BodyFemale: -5117396},
			{ID: 12, Name: "Kerran", WireRace: 0, BodyMale: signed(bodyKerranM), BodyFemale: signed(bodyKerranF)},
			{ID: 13, Name: "Goblin", WireRace: 2, BodyMale: signed(bodyGoblin), BodyFemale: signed(bodyGoblin)},
			{ID: 28, Name: "Fungusman", WireRace: 1, BodyMale: signed(bodyFungus), BodyFemale: signed(bodyFungus)},
		}
	}
	SetRaceDefs(rows)
	t.Cleanup(func() { SetRaceDefs(nil) })
}

// Every row of every race's options is one a client can show and a creation
// can choose: a key and a label, at most 255 rows, steps and parts (each a
// byte on the wire), a key once a race; a body row's steps bodies with a size,
// none twice; a colour row's steps after the first a mode the client knows, its
// first the body's own (0); a texture row's steps textures; a colour or texture
// row's parts each of a body the race can wear (its body rows', or its own),
// with palette entries and rects whose corners are in order.
func TestRaceOptionsAreOnesAClientCanShow(t *testing.T) {
	for own, rows := range raceOptionRows {
		if len(rows) == 0 || len(rows) > 255 {
			t.Errorf("%#x: %d rows", own, len(rows))
		}
		bodies := map[uint32]bool{own: true}
		keys := map[string]bool{}
		for _, r := range rows {
			if r.Kind == KindBody {
				for _, list := range [][]OptionStep{r.Male, r.Female} {
					for _, s := range list {
						bodies[s.Value] = true
					}
				}
			}
		}
		for _, r := range rows {
			where := func(f string, a ...any) {
				t.Helper()
				t.Errorf("%#x %s: "+f, append([]any{own, r.Key}, a...)...)
			}
			if r.Key == "" || r.Label == "" || len(r.Key) > 255 || len(r.Label) > 255 || keys[r.Key] {
				where("key %q, label %q", r.Key, r.Label)
			}
			keys[r.Key] = true
			for _, list := range [][]OptionStep{r.Male, r.Female} {
				if list == nil {
					continue
				}
				if len(list) == 0 || len(list) > 255 {
					where("%d steps", len(list))
				}
				seen := map[uint32]bool{}
				for k, s := range list {
					switch {
					case s.Name == "" || len(s.Name) > 255:
						where("step %d named %q", k, s.Name)
					case r.Kind == KindBody && (s.Value == 0 || s.Size <= 0 || seen[s.Value]):
						where("body step %q: %#x at %v", s.Name, s.Value, s.Size)
					case r.Kind == KindColour && k == 0 && s.Value != 0:
						where("the first colour %q is not the body's own", s.Name)
					case r.Kind == KindColour && k > 0 && s.Value>>24 != ModeDye && s.Value>>24 != ModeTone:
						where("colour %q: %#x", s.Name, s.Value)
					case r.Kind == KindTexture && (s.Value == 0 || seen[s.Value]):
						where("texture step %q: %#x", s.Name, s.Value)
					}
					seen[s.Value] = true
				}
			}
			if r.Kind != KindBody && len(r.Parts) == 0 {
				where("no part")
			}
			if len(r.Parts) > 255 {
				where("%d parts", len(r.Parts))
			}
			for _, p := range r.Parts {
				if !bodies[p.Body] {
					where("a part of %#x, a body the race cannot wear", p.Body)
				}
				if len(p.Entries) == 0 {
					where("a part of %#x with no entry", p.Body)
				}
				for _, rc := range p.Rects {
					if rc[0] > rc[2] || rc[1] > rc[3] {
						where("a rect %v", rc)
					}
				}
			}
		}
	}
}

// A creation's choices: the body row's step is the body (the female's own
// steps for her), a colour's or a texture's past the first a look, the first
// none; a row the race has not, a step past its row's, a row twice and a choice
// for a race with no options are errors; no choice at all is no body and no
// look.
func TestResolveOptions(t *testing.T) {
	withOptionRaces(t)
	body, looks, err := ResolveOptions(28, 0, []OptionChoice{{"body", 1}, {"skin", 0}, {"cap", 3}, {"growths", 0}, {"bag", 2}})
	if err != nil || body != signed(bodyFungusRed) {
		t.Fatalf("a red fungusman: %#x %v", uint32(body), err)
	}
	if len(looks) != 2 || looks[0].Part != "cap" || looks[0].Kind != database.LookColour || looks[0].Value != dyes[3].Value || looks[1].Part != "bag" {
		t.Fatalf("its looks: %+v", looks)
	}
	if body, _, err := ResolveOptions(11, 1, []OptionChoice{{"body", 0}}); err != nil || uint32(body) != giantsFemale[0].Value {
		t.Fatalf("a female hill giant: %#x %v", uint32(body), err)
	}
	if body, _, err := ResolveOptions(11, 1, []OptionChoice{{"body", 3}}); err != nil || uint32(body) != giantsFemale[3].Value {
		t.Fatalf("a female cyclops: %#x %v", uint32(body), err)
	}
	for why, c := range map[string][]OptionChoice{
		"a row it has not":       {{"wings", 1}},
		"past its row's steps":   {{"body", 3}},
		"a row twice":            {{"cap", 1}, {"cap", 2}},
		"a negative step":        {{"cap", -1}},
		"the female's past hers": nil,
	} {
		race, sex := 28, 0
		if c == nil {
			race, sex, c = 11, 1, []OptionChoice{{"body", 5}} // six male kinds, four female
		}
		if _, _, err := ResolveOptions(race, sex, c); err == nil {
			t.Errorf("%s: no error", why)
		}
	}
	if _, _, err := ResolveOptions(3, 0, []OptionChoice{{"body", 0}}); err == nil {
		t.Error("a gnome's choice: no error")
	}
	if _, _, err := ResolveOptions(raceFroglok, 0, []OptionChoice{{"body", 3}}); err == nil {
		t.Error("a froglok's choice while creation does not offer the froglok: no error")
	}
	SetCreationRules(CreationRules{Races: true})
	if body, _, err := ResolveOptions(raceFroglok, 0, []OptionChoice{{"body", 3}}); err != nil || uint32(body) != 0x0F239012 {
		t.Errorf("a ghoul froglok while creation offers it: %#x %v", uint32(body), err)
	}
	SetCreationRules(CreationRules{})
	if body, looks, err := ResolveOptions(28, 0, nil); body != 0 || looks != nil || err != nil {
		t.Errorf("no choice: %v %v %v", body, looks, err)
	}
}

// A body a hill giant chose (the ice giant's) is its look in any armour; its race's
// own body takes the giants' armour bodies as before (the control), and so does a
// body no option offers (a GM's). The channel draws a chosen body at its size
// against the race's own (a green goblin's 1.4), its race's own and a GM's form at
// the race's.
func TestAChosenBodyKeepsItsLookAndSize(t *testing.T) {
	withOptionRaces(t)
	w := NewWorld()
	if n := w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "cloth", Body: 0xE617303B}}); n != 1 {
		t.Fatal(n)
	}
	own := &Player{ServerID: 1, Name: "Own", Race: 11, ModelID: -5117396, Size: 1.8}
	ice := &Player{ServerID: 2, Name: "Frosty", Race: 11, ModelID: signed(0x8117C627), Size: 1.8}
	gm := &Player{ServerID: 3, Name: "Odd", Race: 11, ModelID: signed(0xBF6ADFDB), Size: 1.8}
	for _, p := range []*Player{own, ice, gm} {
		w.Players[p.ServerID] = p
		w.SetGear(p.ServerID, Gear{Armor: ArmorCloth}, false)
	}
	if b, s := own.Body(); b != 0xE617303B || s != 1.8 {
		t.Errorf("its race's own body in cloth: %#x at %v, want the sand giant's", b, s)
	}
	if b, s := ice.Body(); b != 0x8117C627 || s != 1.8 {
		t.Errorf("the ice giant chosen, in cloth: %#x at %v", b, s)
	}
	if b, _ := gm.Body(); b != 0xE617303B {
		t.Errorf("a body no option offers, in cloth: %#x, want the race's armour body", b)
	}
	green := &Player{ServerID: 4, Name: "Snot", Race: 13, ModelID: signed(0x96371804), Size: 1}
	w.Players[4] = green
	if b, s := green.Body(); b != 0x96371804 || s != 1.4 {
		t.Errorf("a green goblin: %#x at %v", b, s)
	}
	green.GMForm = 0x12345678
	if _, s := green.Body(); s != 1 {
		t.Errorf("a green goblin under a GM's form: at %v, the race's size", s)
	}
	grey := &Player{ServerID: 5, Name: "Grey", Race: 13, ModelID: signed(bodyGoblin), Size: 1}
	if _, s := grey.Body(); s != 1 {
		t.Errorf("its race's own: at %v", s)
	}
	if !ChosenBody(11, 0x8117C627) || ChosenBody(11, uint32(bodyHillGiant)) || ChosenBody(11, 0xBF6ADFDB) || ChosenBody(3, 0x8117C627) {
		t.Error("ChosenBody")
	}
}

// The options a client is told: the races with options by id (the froglok only
// while creation offers it), each row's key, label, kind, parts and steps, as
// vanilla's net::raceopts reads them back; a server with no race of racedefs has
// none to offer (the control).
func TestRaceOptionsOnTheWire(t *testing.T) {
	SetRaceDefs(nil)
	if HasRaceOptions() {
		t.Fatal("no race: options offered")
	}
	withOptionRaces(t, database.RaceDefRow{ID: 28, Name: "Fungusman", WireRace: 1, BodyMale: signed(bodyFungus), BodyFemale: signed(bodyFungus)})
	if !HasRaceOptions() {
		t.Fatal("a fungusman: none offered")
	}
	b := EncodeRaceOptions()
	if b[0] != 1 || b[1] != 1 || b[2] != 28 || int(b[3]) != len(raceOptionRows[bodyFungus]) {
		t.Fatalf("version, races, the first race and its rows: %v", b[:4])
	}
	p := 4
	str := func() string {
		n := int(b[p])
		p += 1 + n
		return string(b[p-n : p])
	}
	for _, r := range raceOptionRows[bodyFungus] {
		if k, l := str(), str(); k != r.Key || l != r.Label || OptionKind(b[p]) != r.Kind {
			t.Fatalf("row %q: %q %q %d", r.Key, k, l, b[p])
		}
		p++
		parts := int(b[p])
		p++
		for range parts {
			p += 4
			p += 1 + int(b[p])
			p += 1 + 4*int(b[p])
		}
		for _, list := range [][]OptionStep{r.Male, r.Female} {
			n := int(b[p])
			p++
			if n != len(list) {
				t.Fatalf("row %q: %d steps, want %d", r.Key, n, len(list))
			}
			for _, s := range list {
				if name := str(); name != s.Name || binary.LittleEndian.Uint32(b[p:]) != s.Value {
					t.Fatalf("row %q step %q: %q %#x", r.Key, s.Name, name, binary.LittleEndian.Uint32(b[p:]))
				}
				p += 6
			}
		}
	}
	if p != len(b) {
		t.Fatalf("%d bytes past the rows", len(b)-p)
	}
	SetCreationRules(CreationRules{Races: true})
	t.Cleanup(func() { SetCreationRules(CreationRules{}) })
	if b := EncodeRaceOptions(); b[1] != 2 || b[2] != raceFroglok {
		t.Fatalf("creation open: the froglok first: %v", b[:3])
	}
}

// The looks a client is told: a character's colour on the parts of its own body
// (the fungusman's cap: the disc's two pieces, the cap's top and its gills), a
// GM's under both its names; a look of a key its race has not, a look on a body
// with no part of it (a red fungusman's bag) and a character of a race with no
// options lay nothing and are left out; no character, an empty list.
func TestRaceLooksOnTheWire(t *testing.T) {
	withOptionRaces(t)
	gold := dyes[3].Value
	chars := []database.LookCharacter{
		{Name: "Sporey", GM: true, Race: 28, ModelID: signed(bodyFungus), Looks: []database.CharLook{{Part: "cap", Kind: database.LookColour, Value: gold}}},
		{Name: "Reddy", Race: 28, ModelID: signed(bodyFungusRed), Looks: []database.CharLook{{Part: "bag", Kind: database.LookColour, Value: gold}}},
		{Name: "Oddy", Race: 28, ModelID: signed(bodyFungus), Looks: []database.CharLook{{Part: "wings", Kind: database.LookColour, Value: gold}}},
		{Name: "Gnomey", Race: 3, ModelID: 0x70D898C6, Looks: []database.CharLook{{Part: "cap", Kind: database.LookColour, Value: gold}}},
	}
	b := EncodeRaceLooks(chars)
	if b[0] != 1 || binary.LittleEndian.Uint16(b[1:]) != 2 {
		t.Fatalf("version and count: %v", b[:3])
	}
	one := []byte{6, 'S', 'p', 'o', 'r', 'e', 'y'}
	one = binary.LittleEndian.AppendUint32(one, bodyFungus)
	one = append(one, 2)
	one = append(one, 2)
	one = binary.LittleEndian.AppendUint32(one, gold)
	one = append(one, 1, 0, 1, 115, 115, 255, 255)
	one = append(one, 2)
	one = binary.LittleEndian.AppendUint32(one, gold)
	one = append(one, 1, 1, 1, 184, 0, 255, 153)
	if got := string(b[3 : 3+len(one)]); got != string(one) {
		t.Fatalf("\n got %x\nwant %x", got, one)
	}
	if !strings.Contains(string(b), GMTag+"Sporey") || strings.Contains(string(b), "Reddy") || strings.Contains(string(b), "Oddy") || strings.Contains(string(b), "Gnomey") {
		t.Fatalf("who: %q", b)
	}
	if b := EncodeRaceLooks(nil); string(b) != "\x01\x00\x00" {
		t.Fatalf("none: %v", b)
	}
}

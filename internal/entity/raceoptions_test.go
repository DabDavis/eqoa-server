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
// first the body's own (0); a texture row's steps textures, its first one the
// body's own (its texture, or 0 where its bodies wear different ones); a colour or texture
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
					case r.Kind == KindTexture && (s.Value == 0 && k > 0 || seen[s.Value]):
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

// A body row's steps keep their numbers: a creation sends a step's number, and a
// session kept over a restart is not sent the list again, so a new body goes after
// the steps a client may hold (each race's body steps of 2026-11-08, build49, as
// they stand). The rest of a row is free to grow.
func TestRaceOptionStepsKeepTheirNumbers(t *testing.T) {
	was := map[uint32][2][]uint32{
		bodyFroglok:   {{0x141BFE0C, 0x429A04AB, 0x063C5D97, 0x0F239012, 0x6F17325E, 0x1663EC1C}},
		bodyHillGiant: {{0xFFB1EA2C, 0x8117C627, 0xE617303B, 0x72A8FDBB, 0x7F2604DF, 0x05F59B65}, {0xF34501CB, 0x286C6E9E, 0x8E2A6552, 0xE30827CC}},
		bodyKerranM:   {{0x22176258, 0x4D9C53EF, 0x77A7E7B8, 0x78FA8F20, 0x0DDD4087, 0x5D40C8AB}, {0xB749BF57, 0xB62092E4, 0xE700DE53, 0x39FEAA5F, 0x7654AC3C, 0xD8E0F980}},
		bodyGoblin:    {{0x6C1308E0, 0x581B6A80, 0x96371804, 0xBCDF0C04, 0xE96179D4, 0xA5F92F43, 0x6C7C07ED, 0xBE927B85, 0xCEF5874A}},
		bodyAviakM:    {{0x3151DF13, 0x262C572A, 0xDC641055, 0x41EFF500, 0x3C3DE6CA, 0xC04CEC07, 0xB1EEB204, 0x8B332785}, {0x8EFD12EA, 0x262C572A, 0xDC641055, 0x41EFF500, 0x3C3DE6CA, 0xC04CEC07, 0xB1EEB204, 0x8B332785}},
		bodyLizardman: {{0x911220E2, 0xD45B9F1F, 0x2C1D8724, 0x7468EAE7}},
		bodyKappa:     {{0x31A15F17, 0xC3EC4F58, 0xF6747024, 0xF0FAB933}},
		bodyBrownie:   {{0x4376EB52, 0x49468331}},
		bodyGnoll:     {{0xCF415561, 0x223FD6BC, 0xF5F3A450, 0x097BFC70, 0xB7B21DFC, 0x4A9BAB9E, 0x17DA0FEB, 0x7492678E}},
		bodyOrc:       {{0x8214A32D, 0x84F47767, 0x656DD4C5, 0x37FD13F9, 0x3198F5B1}, {0x0A03EB92, 0xDA515962, 0x8214A32D, 0x37FD13F9, 0x3198F5B1}},
		bodyKobold:    {{0xCFC478EE, 0x6B9B7016, 0x597325B5, 0x9CDF3C2D}},
		bodyRatman:    {{0xBADC25C9, 0x0ECC836A, 0x133A79CA}},
		bodySatyrM:    {{0xBE10226F}, {0x097ADB18}},
		bodySkeleton:  {{0x6AF98A81, 0x45A8B064, 0x198BAA64, 0x648DFC11}},
		bodyFungus:    {{0x4A4D88FB, 0x5874ACFC, 0xE8B0ADC8}},
		bodyGargoyle:  {{0xB4D2D132, 0x4E2A945C, 0xAEC5D9F6, 0x001355C8, 0x6D959FD9, 0x9304BAB5}},
	}
	for own, lists := range was {
		var row *OptionRow
		for i := range raceOptionRows[own] {
			if raceOptionRows[own][i].Kind == KindBody {
				row = &raceOptionRows[own][i]
			}
		}
		if row == nil {
			t.Errorf("%08X: no body row", own)
			continue
		}
		for sex, want := range lists {
			got := row.Male
			if sex == 1 {
				if want == nil {
					continue
				}
				got = row.Female
			}
			for k, v := range want {
				if k >= len(got) || got[k].Value != v {
					t.Errorf("%08X sex %d step %d: want %08X", own, sex, k, v)
					break
				}
			}
		}
	}
	// the control: a body put first moves every number after it
	moved := append([]OptionStep{{"New", 0x12345678, 1}}, raceOptionRows[bodyGoblin][0].Male...)
	if moved[1].Value == was[bodyGoblin][0][1] {
		t.Fatal("the control")
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
	gm := &Player{ServerID: 3, Name: "Odd", Race: 11, ModelID: signed(0x87CE5F73), Size: 1.8} // a stone golem: no option's
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
	if !ChosenBody(11, 0x8117C627) || !ChosenBody(11, 0xBF6ADFDB) || ChosenBody(11, uint32(bodyHillGiant)) || ChosenBody(11, 0x87CE5F73) || ChosenBody(3, 0x8117C627) {
		t.Error("ChosenBody")
	}
	// the control: a game race's character (a GM's form aside) wearing a body some race
	// offers is drawn at its own size, and its race's armour rows (none) are as before
	ogre := &Player{ServerID: 6, Name: "Plainogre", Race: 9, ModelID: signed(0x96371804), Size: 1.2}
	w.Players[6] = ogre
	w.SetGear(6, Gear{Armor: ArmorPlate}, false)
	if b, s := ogre.Body(); b != 0x96371804 || s != 1.2 {
		t.Errorf("an ogre in a green goblin's body: %#x at %v, want its own size", b, s)
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

// No triangle is two rows' (a colour and a texture laid on it would depend on
// the order they were laid in): on each body, two rows' parts that name one
// palette entry hold rects whose insides meet nowhere, and a part that is a
// whole entry shares it with no other row's. The check's own control: two rects
// that meet inside are caught, two that only touch are not.
func TestRaceOptionPartsNeverOverlap(t *testing.T) {
	meet := func(a, b [4]uint8) bool { return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3] }
	if !meet([4]uint8{26, 0, 184, 209}, [4]uint8{115, 115, 255, 255}) || meet([4]uint8{0, 0, 115, 255}, [4]uint8{115, 115, 255, 255}) {
		t.Fatal("the check")
	}
	clash := func(p, q OptionPart) bool {
		for _, e := range p.Entries {
			for _, f := range q.Entries {
				if e != f {
					continue
				}
				if len(p.Rects) == 0 || len(q.Rects) == 0 {
					return true
				}
				for _, a := range p.Rects {
					for _, b := range q.Rects {
						if meet(a, b) {
							return true
						}
					}
				}
			}
		}
		return false
	}
	for own, rows := range raceOptionRows {
		for i, r := range rows {
			for _, s := range rows[i+1:] {
				for _, p := range r.Parts {
					for _, q := range s.Parts {
						if p.Body == q.Body && clash(p, q) {
							t.Errorf("race body %08X: %s and %s meet on body %08X: %v %v", own, r.Key, s.Key, p.Body, p, q)
						}
					}
				}
			}
		}
	}
}

// A character's parts go out in its race's row order, as the appearance page
// lays them, whatever order its charlook rows came in (the database's is by
// part, "cap" before "skin"): the skin's two parts, then the cap's two.
func TestRaceLooksInRowOrder(t *testing.T) {
	withOptionRaces(t)
	gold, ash := dyes[3].Value, barks[2].Value
	looks := []database.CharLook{{Part: "cap", Kind: database.LookColour, Value: gold}, {Part: "skin", Kind: database.LookColour, Value: ash}}
	want := []uint32{ash, ash, gold, gold}
	for _, order := range [][]database.CharLook{looks, {looks[1], looks[0]}} {
		_, values, parts := lookParts(28, bodyFungus, order)
		if len(values) != len(want) || parts[0].Entries[0] != 1 || parts[2].Rects[0] != [4]uint8{115, 115, 255, 255} {
			t.Fatalf("values %x parts %v", values, parts)
		}
		for i := range want {
			if values[i] != want[i] {
				t.Fatalf("values %x, want %x", values, want)
			}
		}
	}
}

// The Jal'Raeth (30) choose their robed, channeler's or knight's body, the knight
// at its size against the robed; the Wisp (31) is a human of its sex, and its wisp
// is laid always: chosen or not, and with no block at all (a client that sends
// none), a charlook row of the will o' wisp's body, told to a client on its own body
// (the female's on hers). The control: a plain texture row's first step (the satyr's
// hair) lays nothing.
func TestTheJalRaethAndTheWisp(t *testing.T) {
	withOptionRaces(t,
		database.RaceDefRow{ID: 25, Name: "Satyr", WireRace: 1, BodyMale: signed(bodySatyrM), BodyFemale: signed(bodySatyrM)},
		database.RaceDefRow{ID: 30, Name: "Jal'Raeth", WireRace: 8, BodyMale: signed(bodyJalRaeth), BodyFemale: signed(bodyJalRaeth)},
		database.RaceDefRow{ID: 31, Name: "Wisp", WireRace: 0, BodyMale: signed(bodyHumanMale), BodyFemale: signed(bodyHumanFemale)})
	body, looks, err := ResolveOptions(30, 0, []OptionChoice{{"body", 2}})
	if err != nil || body != signed(bodyJalRaethKnight) || len(looks) != 0 {
		t.Fatalf("a Jal'Raeth knight: %#x %v %v", uint32(body), looks, err)
	}
	if OptionBodySize(30, bodyJalRaethKnight) != 0.94 || !ChosenBody(30, bodyJalRaethChannel) || ChosenBody(30, bodyJalRaeth) {
		t.Error("the knight's size, the channeler chosen, the robes the race's own")
	}
	for _, c := range [][]OptionChoice{nil, {{"wisp", 0}}, {{"body", 0}}} {
		_, looks, err := ResolveOptions(31, 0, c)
		if err != nil || len(looks) != 1 || looks[0] != (database.CharLook{Part: "wisp", Kind: database.LookTexture, Value: bodyWillOWisp}) {
			t.Errorf("a Wisp choosing %v: %v %v", c, looks, err)
		}
	}
	body, looks, err = ResolveOptions(31, 1, []OptionChoice{{"body", 0}, {"wisp", 0}})
	if err != nil || body != signed(bodyHumanFemale) || len(looks) != 1 {
		t.Fatalf("a female Wisp: %#x %v %v", uint32(body), looks, err)
	}
	b := EncodeRaceLooks([]database.LookCharacter{{Name: "Glim", Race: 31, Sex: 1, ModelID: signed(bodyHumanFemale), Looks: looks}})
	want := append([]byte{raceOptionsVersion, 1, 0, 4}, "Glim"...)
	want = binary.LittleEndian.AppendUint32(want, bodyHumanFemale)
	want = append(want, 1, byte(KindTexture))
	want = binary.LittleEndian.AppendUint32(want, bodyWillOWisp)
	want = append(want, 1, 7, 0)
	if string(b) != string(want) {
		t.Errorf("the female Wisp's look\n got % x\nwant % x", b, want)
	}
	if _, looks, _ := ResolveOptions(25, 0, []OptionChoice{{"hair", 0}}); len(looks) != 0 {
		t.Errorf("the control: the satyr's first hair laid %v", looks)
	}
}

// The Mermaid (32, petition 1620): a merman for a man at the mermaid's height, a
// mermaid or a siren of either kind for a woman (each as tall), and skin, hair,
// tail and shell colours on every one of them but the merman's shells, which
// he has none of. A siren chosen is her look, the mermaid her race's own.
func TestTheMermaid(t *testing.T) {
	withOptionRaces(t, database.RaceDefRow{ID: 32, Name: "Mermaid", WireRace: 8, BodyMale: signed(bodyMerman), BodyFemale: signed(bodyMermaid)})
	body, looks, err := ResolveOptions(32, 1, []OptionChoice{{"body", 1}, {"skin", 8}, {"hair", 5}, {"tail", 6}, {"shells", 3}})
	if err != nil || body != signed(bodySiren) || len(looks) != 4 {
		t.Fatalf("a siren in colours: %#x %v %v", uint32(body), looks, err)
	}
	if OptionBodySize(32, bodyMerman) != 0.83 || OptionBodySize(32, bodyMermaid) != 1 || OptionBodySize(32, bodyEnchantingSiren) != 0.667 {
		t.Error("the sizes")
	}
	if !ChosenBody(32, bodySiren) || ChosenBody(32, bodyMermaid) || ChosenBody(32, bodyMerman) {
		t.Error("a siren chosen, the merman and the mermaid the race's own")
	}
	if body, _, err := ResolveOptions(32, 0, []OptionChoice{{"body", 1}}); err == nil {
		t.Errorf("a merman chose a second body: %#x", uint32(body))
	}
	for _, r := range RaceOptions(32) {
		for _, b := range []uint32{bodyMerman, bodyMermaid, bodySiren, bodyEnchantingSiren} {
			if r.Kind == KindColour && len(r.PartsOf(b)) != 1 && !(r.Key == "shells" && b == bodyMerman) {
				t.Errorf("row %s on %08X: %v", r.Key, b, r.PartsOf(b))
			}
		}
	}
	for _, b := range []uint32{bodyMerman, bodyMermaid, bodySiren, bodyEnchantingSiren} {
		if !swimmerBodies[b] {
			t.Errorf("%08X is no swimmer's body: its swing would play nothing", b)
		}
	}
}

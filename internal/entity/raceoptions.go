package entity

import (
	"fmt"
	"math"
	"sort"

	"github.com/eqoa/server/internal/database"
)

// raceoptions.go — a custom race's own character customisation (the user,
// 2026-11-08: "lets add char customization to the new races, if any other faces
// and hair add them, but if not add color swaps. like for fungusman the fungus man
// general color, can change head cap, fungus protrusion colors, alos the bag
// color, for example. for giants, cycle through ice giants, fire giants, hill
// giants, sand giants and colors,etc."; claude-notes/race-options-20261108.md).
//
// The game's create page dresses a player race's body by its text slots (faces,
// hair, the armour's): a creature's body, which every race of racedefs wears, has
// none, so the retail look's four rows change nothing on it. What such a body has
// instead was read off the disc by sight: other bodies of the same creature (the
// giants' kinds, the goblins' tribes, the aviaks' birds: a row of KindBody), a few
// that draw a face or hair out of the player races' own textures (the satyr's, the
// brownie's: KindTexture), and for the rest parts a colour can be laid on
// (KindColour). Each race's rows are below, keyed by the race's own male body (the
// racedefs row's, the froglok's), so a race's options follow its body and a race
// whose body has none has none.
//
// What a character chose is kept as the game keeps it where it can: a body is
// characters.modelid, so every client, a PS2 too, draws it (and its size against
// the race's own goes on the entity channel's scale: Player.Body); a colour or a
// texture is a charlook row (database/charlook.go), which only a client that asked
// is told (OpcodeRaceLooks: handlers/raceoptions.go). A character with no row and
// its race's body is drawn as it always was.

// OptionKind is what a row's steps change.
type OptionKind uint8

// The kinds.
const (
	// KindBody steps are bodies (CHAR.ESF CSprite DictIDs): characters.modelid.
	KindBody OptionKind = 1
	// KindColour steps recolour a part of the body (Recolour): charlook.
	KindColour OptionKind = 2
	// KindTexture steps lay a texture on a part (a face, hair): charlook.
	KindTexture OptionKind = 3
)

// OptionPart is a part of one body a colour or texture row changes: the
// triangles of its palette entries (Entries) whose texture coordinates' centre
// (the first layer's, after its transform) lies in one of Rects, each
// {u0, v0, u1, v1} in 255ths of the texture; no Rects is the entries' whole.
type OptionPart struct {
	Body    uint32
	Entries []uint8
	Rects   [][4]uint8
}

// OptionStep is one step of a row. Value is a body's DictID (KindBody), a
// Recolour (KindColour: 0 the body's own colours) or a texture's DictID
// (KindTexture: the body's own on the first step). Size is a body step's scale
// against the race's own body, so every kind stands as tall (1: as it is).
type OptionStep struct {
	Name  string
	Value uint32
	Size  float32
}

// OptionRow is one row of a race's options: Key names it (on the wire, in
// charlook), Label is the create page's word for it. A body row's steps are by
// sex (Female nil: the male's); a colour or texture row lays its step on the
// Parts of the body the character wears, and changes nothing on a body it has
// no part of (a texture row's Female steps are the female body's own, as the
// satyrs' faces are).
type OptionRow struct {
	Key, Label string
	Kind       OptionKind
	Parts      []OptionPart
	Male       []OptionStep
	Female     []OptionStep
}

// Steps is the row's steps for sex (1 female).
func (r OptionRow) Steps(sex int) []OptionStep {
	if sex == 1 && r.Female != nil {
		return r.Female
	}
	return r.Male
}

// PartsOf is the row's parts of body.
func (r OptionRow) PartsOf(body uint32) []OptionPart {
	var out []OptionPart
	for _, p := range r.Parts {
		if p.Body == body {
			out = append(out, p)
		}
	}
	return out
}

// Recolour is a colour step as a client lays it on a part's texels (each
// texel's hue, saturation and lightness, HSL): the high byte the mode, then
// three bytes. ModeDye takes the hue (byte 2, 256ths of a turn) and the
// saturation (byte 1, 255ths) as given and scales the lightness (byte 0, 128ths:
// 128 keeps it), as an image editor's colourise: one colour over the part's own
// shading. ModeTone turns the hue by byte 2 and scales the saturation (byte 1)
// and lightness (byte 0), each 128ths: the part's own colours, shifted. 0 is the
// part's own colours.
type Recolour = uint32

// The modes.
const (
	ModeDye  uint32 = 1
	ModeTone uint32 = 2
)

// Dye is a ModeDye colour: hue in degrees, saturation and lightness in per cent
// (lightness 100 keeps the part's own).
func Dye(hue, sat, light int) Recolour {
	return ModeDye<<24 | turn(hue)<<16 | optionPct(sat, 255)<<8 | optionPct(light, 128)
}

// Tone is a ModeTone shift: the hue turned by degrees, saturation and lightness
// scaled by per cent (100 keeps them).
func Tone(hue, sat, light int) Recolour {
	return ModeTone<<24 | turn(hue)<<16 | optionPct(sat, 128)<<8 | optionPct(light, 128)
}

// turn is degrees as 256ths of a turn.
func turn(deg int) uint32 {
	return uint32(((deg%360)+360)%360*256/360) & 0xFF
}

// optionPct is a per cent of unit, clamped to a byte.
func optionPct(p, unit int) uint32 {
	return uint32(max(0, min(255, int(math.Round(float64(p)*float64(unit)/100)))))
}

// The race's own body of each race with options, as racedefs and the froglok
// have them (body_male), and the ones its rows offer.
const (
	bodyFroglok   = 0x141BFE0C
	bodyHillGiant = 0xFFB1EA2C
	bodyKerranM   = 0x22176258
	bodyKerranF   = 0xB749BF57
	bodyGoblin    = 0x6C1308E0
	bodyAviakM    = 0x3151DF13
	bodyAviakF    = 0x8EFD12EA
	bodyLizardman = 0x911220E2
	bodyKappa     = 0x31A15F17
	bodyBrownie   = 0x4376EB52
	bodyPixie     = 0x94BA77CD
	bodyFairy     = 0x7A85646C
	bodyGnoll     = 0xCF415561
	bodyOrc       = 0x8214A32D
	bodyKobold    = 0xCFC478EE
	bodyMinotaur  = 0xC5CBF4CF
	bodyRatman    = 0xBADC25C9
	bodySatyrM    = 0xBE10226F
	bodySatyrF    = 0x097ADB18
	bodyDryad     = 0x70EFCE05
	bodySkeleton  = 0x6AF98A81
	bodyFungus    = 0x4A4D88FB
	bodyGargoyle  = 0xB4D2D132

	bodyFungusRed  = 0x5874ACFC
	bodyFungusPale = 0xE8B0ADC8
)

// The giants' kinds, every one on the hill giant's skeleton and as tall (4.81).
var (
	giantsMale = []OptionStep{{"Hill giant", bodyHillGiant, 1}, {"Ice giant", 0x8117C627, 1}, {"Sand giant", 0xE617303B, 1},
		{"Fire giant", 0x72A8FDBB, 1}, {"Fire giant (vest)", 0x7F2604DF, 1}, {"Cyclops", 0x05F59B65, 1}}
	giantsFemale = []OptionStep{{"Hill giant", 0xF34501CB, 1}, {"Ice giant", 0x286C6E9E, 1}, {"Sand giant", 0x8E2A6552, 1},
		{"Cyclops", 0xE30827CC, 1}}
)

// Palettes. dyes are colours over a part's own shading, for a part of one
// colour (a cap, a bag, wings, a dress); tones shift a part's own colours, for
// one of many (a giant's whole body, a creature's hide).
var (
	dyes = []OptionStep{{"Natural", 0, 0}, {"Crimson", Dye(352, 55, 95), 0}, {"Ember", Dye(20, 65, 105), 0},
		{"Gold", Dye(45, 60, 112), 0}, {"Moss", Dye(95, 40, 95), 0}, {"Teal", Dye(175, 45, 100), 0},
		{"Azure", Dye(212, 55, 105), 0}, {"Violet", Dye(275, 42, 100), 0}, {"Ivory", Dye(40, 20, 140), 0},
		{"Charcoal", Dye(220, 8, 55), 0}}
	tones = []OptionStep{{"Natural", 0, 0}, {"Pale", Tone(0, 70, 118), 0}, {"Dark", Tone(0, 80, 70), 0},
		{"Ashen", Tone(0, 25, 100), 0}, {"Ruddy", Tone(-12, 140, 95), 0}, {"Bronze", Tone(15, 125, 90), 0},
		{"Frost", Dye(205, 30, 110), 0}}
	barks = []OptionStep{{"Natural", 0, 0}, {"Umber", Dye(28, 45, 95), 0}, {"Ash", Dye(30, 6, 110), 0},
		{"Moss", Dye(90, 30, 95), 0}, {"Plum", Dye(310, 25, 90), 0}, {"Ochre", Dye(42, 55, 110), 0},
		{"Char", Dye(20, 15, 55), 0}}
)

// hairs are the player races' eight hair textures (VICSpriteCust's $Hair), which
// the satyrs' and the brownie's heads already wear one of.
var hairs = []OptionStep{{"Hair 1", 0x98F86489, 0}, {"Hair 2", 0x221E7298, 0}, {"Hair 3", 0x6DCEF2D0, 0},
	{"Hair 4", 0xFEBCE4D5, 0}, {"Hair 5", 0xD499494B, 0}, {"Hair 6", 0x8931CEE6, 0}, {"Hair 7", 0x9F87C7AF, 0},
	{"Hair 8", 0xDC79B7C3, 0}}

// rotate is steps with the one of value first (a body's own texture is the row's
// first step, its natural one).
func rotate(steps []OptionStep, value uint32) []OptionStep {
	for i, s := range steps {
		if s.Value == value {
			return append(append([]OptionStep{}, steps[i:]...), steps[:i]...)
		}
	}
	return steps
}

// whole is a part that is every triangle of the entries.
func whole(body uint32, entries ...uint8) OptionPart {
	return OptionPart{Body: body, Entries: entries}
}

// within is a part that is the entries' triangles in rects (255ths).
func within(body uint32, entries []uint8, rects ...[4]uint8) OptionPart {
	return OptionPart{Body: body, Entries: entries, Rects: rects}
}

// everyEntry is a part of each body of steps that is every triangle of its palette
// entries 0 to 7 (a body has fewer; an entry it has not is no triangle).
func everyEntry(steps ...[]OptionStep) []OptionPart {
	var out []OptionPart
	seen := map[uint32]bool{}
	for _, list := range steps {
		for _, s := range list {
			if !seen[s.Value] {
				seen[s.Value] = true
				out = append(out, whole(s.Value, 0, 1, 2, 3, 4, 5, 6, 7))
			}
		}
	}
	return out
}

// The red and pale fungusmen's parts: rects of their one texture, which the two
// bodies lay out alike.
var (
	fungusSkin    = [][4]uint8{{31, 0, 184, 128}, {26, 0, 31, 51}, {0, 122, 115, 209}}
	fungusCap     = [][4]uint8{{115, 128, 255, 255}, {184, 0, 255, 140}}
	fungusGrowths = [][4]uint8{{0, 51, 31, 122}, {0, 209, 110, 255}}
)

// raceOptionRows are each race's rows, by its own male body. Measured by eye
// with vanilla's eqoa dress on the bodies' own triangles and textures
// (claude-notes/race-options-20261108.md has the shots).
var raceOptionRows = map[uint32][]OptionRow{
	bodyFroglok: {{Key: "body", Label: "Froglok", Kind: KindBody, Male: []OptionStep{
		{"Froglok", bodyFroglok, 1}, {"Froglok (robed)", 0x429A04AB, 1}, {"Froglok (spiked)", 0x063C5D97, 0.8},
		{"Ghoul", 0x0F239012, 1}, {"Ghoul (robed)", 0x6F17325E, 1}, {"Ghoul (spiked)", 0x1663EC1C, 0.8}}}},
	bodyHillGiant: {
		{Key: "body", Label: "Giant", Kind: KindBody, Male: giantsMale, Female: giantsFemale},
		{Key: "tone", Label: "Tone", Kind: KindColour, Parts: everyEntry(giantsMale, giantsFemale), Male: tones},
	},
	bodyKerranM: {{Key: "body", Label: "Coat", Kind: KindBody,
		Male: []OptionStep{{"Tiger", bodyKerranM, 1}, {"Tiger (robed)", 0x4D9C53EF, 1}, {"White tiger", 0x77A7E7B8, 0.86},
			{"Lion", 0x78FA8F20, 1}, {"Lion (clothed)", 0x0DDD4087, 1}, {"Maned lion", 0x5D40C8AB, 0.89}},
		Female: []OptionStep{{"Tiger", bodyKerranF, 1}, {"Tiger (robed)", 0xB62092E4, 1}, {"Red tiger", 0xE700DE53, 0.82},
			{"Lioness", 0x39FEAA5F, 1}, {"Lioness (robed)", 0x7654AC3C, 1}, {"Lioness (warrior)", 0xD8E0F980, 0.82}}}},
	bodyGoblin: {{Key: "body", Label: "Tribe", Kind: KindBody, Male: []OptionStep{
		{"Grey goblin", bodyGoblin, 1}, {"Grey goblin (robed)", 0x581B6A80, 1}, {"Green goblin", 0x96371804, 1.4},
		{"Green goblin (robed)", 0xBCDF0C04, 1.4}, {"Ice goblin", 0xE96179D4, 0.875}, {"Ice goblin (robed)", 0xA5F92F43, 0.875},
		{"Fire goblin", 0x6C7C07ED, 1.17}, {"Fire goblin (robed)", 0xBE927B85, 1.17}, {"Fire goblin (shaman)", 0xCEF5874A, 1.17}}}},
	bodyAviakM: {{Key: "body", Label: "Bird", Kind: KindBody,
		Male: []OptionStep{{"Hawk", bodyAviakM, 1}, {"Hawk (robed)", 0x262C572A, 1}, {"Raven", 0xDC641055, 1},
			{"Raven (robed)", 0x41EFF500, 1}, {"Vulture", 0x3C3DE6CA, 0.91}, {"Vulture (robed)", 0xC04CEC07, 0.91},
			{"Owl", 0xB1EEB204, 0.91}, {"Owl (robed)", 0x8B332785, 0.91}},
		Female: []OptionStep{{"Hawk", bodyAviakF, 1}, {"Hawk (robed)", 0x262C572A, 1}, {"Raven", 0xDC641055, 1},
			{"Raven (robed)", 0x41EFF500, 1}, {"Vulture", 0x3C3DE6CA, 0.91}, {"Vulture (robed)", 0xC04CEC07, 0.91},
			{"Owl", 0xB1EEB204, 0.91}, {"Owl (robed)", 0x8B332785, 0.91}}}},
	bodyLizardman: {{Key: "body", Label: "Scales", Kind: KindBody, Male: []OptionStep{
		{"Strapped", bodyLizardman, 1}, {"Pauldrons", 0xD45B9F1F, 1.09}, {"Bare", 0x2C1D8724, 1.09}, {"Robed", 0x7468EAE7, 1.09}}}},
	bodyKappa: {{Key: "body", Label: "Shell", Kind: KindBody, Male: []OptionStep{
		{"Brown shell", bodyKappa, 1}, {"Marked shell", 0xC3EC4F58, 0.91}, {"Banded shell", 0xF6747024, 1.05},
		{"Straw hat", 0xF0FAB933, 0.83}}}},
	bodyBrownie: {
		{Key: "body", Label: "Garb", Kind: KindBody, Male: []OptionStep{{"Brownie", bodyBrownie, 1}, {"Elder", 0x49468331, 1}}},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{whole(bodyBrownie, 1, 2)}, Male: hairs},
	},
	bodyPixie: {
		{Key: "wings", Label: "Wings", Kind: KindColour, Parts: []OptionPart{whole(bodyPixie, 1)}, Male: dyes},
		{Key: "dress", Label: "Dress", Kind: KindColour, Parts: []OptionPart{within(bodyPixie, []uint8{0}, [4]uint8{0, 0, 158, 168})}, Male: dyes},
	},
	bodyFairy: {
		{Key: "wings", Label: "Wings", Kind: KindColour, Parts: []OptionPart{whole(bodyFairy, 1)}, Male: tones},
		{Key: "dress", Label: "Dress", Kind: KindColour, Parts: []OptionPart{within(bodyFairy, []uint8{0}, [4]uint8{0, 0, 158, 168})}, Male: dyes},
	},
	bodyGnoll: {{Key: "body", Label: "Pelt", Kind: KindBody, Male: []OptionStep{
		{"Black", bodyGnoll, 1}, {"Black (captain)", 0x223FD6BC, 0.91}, {"Blue-pawed", 0xF5F3A450, 1},
		{"Black (robed)", 0x097BFC70, 1}, {"Black (elder)", 0xB7B21DFC, 0.91}, {"Black and tan", 0x4A9BAB9E, 1},
		{"Black and tan (scout)", 0x17DA0FEB, 1}, {"Black and tan (robed)", 0x7492678E, 1}}}},
	bodyOrc: {{Key: "body", Label: "Clan", Kind: KindBody,
		Male: []OptionStep{{"Orc", bodyOrc, 1}, {"Orc (long hair)", 0x84F47767, 0.92}, {"Orc (grey mail)", 0x656DD4C5, 1},
			{"Deathfist", 0x37FD13F9, 1.02}, {"Deathfist (kilt)", 0x3198F5B1, 1.06}},
		Female: []OptionStep{{"Orc", 0x0A03EB92, 1.01}, {"Orc (grey mail)", 0xDA515962, 1.01}, {"Orc (brute)", bodyOrc, 1},
			{"Deathfist", 0x37FD13F9, 1.02}, {"Deathfist (kilt)", 0x3198F5B1, 1.06}}}},
	bodyKobold: {{Key: "body", Label: "Hide", Kind: KindBody, Male: []OptionStep{
		{"Kobold", bodyKobold, 1}, {"Kobold (augur)", 0x6B9B7016, 0.9}, {"Patched", 0x597325B5, 1}, {"Pauldrons", 0x9CDF3C2D, 0.82}}}},
	// the minotaur's black fur takes no colour worth the name (a dark grey at most): its hide and horns do
	bodyMinotaur: {
		{Key: "hide", Label: "Hide", Kind: KindColour, Parts: []OptionPart{
			within(bodyMinotaur, []uint8{1}, [4]uint8{195, 105, 255, 250}),
			within(bodyMinotaur, []uint8{2}, [4]uint8{90, 0, 255, 229}, [4]uint8{0, 0, 90, 110})}, Male: tones},
		{Key: "horns", Label: "Horns", Kind: KindColour, Parts: []OptionPart{
			within(bodyMinotaur, []uint8{2}, [4]uint8{190, 230, 255, 255})}, Male: dyes},
	},
	bodyRatman: {{Key: "body", Label: "Garb", Kind: KindBody, Male: []OptionStep{
		{"Ratman", bodyRatman, 1}, {"Hooded", 0x0ECC836A, 1.18}, {"Hooded (pauldrons)", 0x133A79CA, 1}}}},
	bodySatyrM: {
		{Key: "body", Label: "Satyr", Kind: KindBody, Male: []OptionStep{{"Satyr", bodySatyrM, 1}},
			Female: []OptionStep{{"Faun", bodySatyrF, 1}}},
		{Key: "face", Label: "Face", Kind: KindTexture, Parts: []OptionPart{whole(bodySatyrM, 3), whole(bodySatyrF, 3)},
			Male:   []OptionStep{{"Face 1", 0x4A7AB44E, 0}, {"Face 2", 0x5C086A9F, 0}, {"Face 3", 0x6D9620F0, 0}, {"Face 4", 0x7F23D741, 0}},
			Female: []OptionStep{{"Face 1", 0x765CFD7D, 0}, {"Face 2", 0x87EAB3CE, 0}, {"Face 3", 0x99786A1F, 0}, {"Face 4", 0xAB062070, 0}}},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{whole(bodySatyrM, 1), whole(bodySatyrF, 1)},
			Male: hairs, Female: rotate(hairs, 0x221E7298)},
	},
	bodyDryad: {
		{Key: "bark", Label: "Bark", Kind: KindColour, Parts: []OptionPart{whole(bodyDryad, 0, 2, 3, 4)}, Male: barks},
		{Key: "leaves", Label: "Leaves", Kind: KindColour, Parts: []OptionPart{whole(bodyDryad, 1, 7)}, Male: dyes},
	},
	bodySkeleton: {{Key: "body", Label: "Bones", Kind: KindBody, Male: []OptionStep{
		{"Bone", bodySkeleton, 1}, {"Old bone", 0x45A8B064, 0.91}, {"Mossy", 0x198BAA64, 0.91}, {"Frost", 0x648DFC11, 1}}}},
	bodyFungus: {
		{Key: "body", Label: "Fungus", Kind: KindBody, Male: []OptionStep{
			{"Blue cap", bodyFungus, 1}, {"Red cap", bodyFungusRed, 1}, {"Pale cap", bodyFungusPale, 0.91}}},
		// a triangle is one row's only (TestRaceOptionPartsNeverOverlap): on the red
		// and pale bodies' one atlas the cap's top runs down to v 133 and the hands'
		// skin up to 120 (the head's bone against the arms'), the hands' growths out to
		// u 30, checked triangle by triangle against their bones
		{Key: "skin", Label: "Skin", Kind: KindColour, Parts: []OptionPart{
			within(bodyFungus, []uint8{1}, [4]uint8{0, 0, 184, 255}), within(bodyFungus, []uint8{0}, [4]uint8{115, 0, 255, 115}),
			within(bodyFungusRed, []uint8{0}, fungusSkin...),
			within(bodyFungusPale, []uint8{0}, fungusSkin...)}, Male: barks},
		{Key: "cap", Label: "Cap", Kind: KindColour, Parts: []OptionPart{
			within(bodyFungus, []uint8{0}, [4]uint8{115, 115, 255, 255}), within(bodyFungus, []uint8{1}, [4]uint8{184, 0, 255, 153}),
			within(bodyFungusRed, []uint8{0}, fungusCap...),
			within(bodyFungusPale, []uint8{0}, fungusCap...)}, Male: dyes},
		{Key: "growths", Label: "Growths", Kind: KindColour, Parts: []OptionPart{
			within(bodyFungus, []uint8{0}, [4]uint8{0, 0, 115, 255}),
			within(bodyFungusRed, []uint8{0}, fungusGrowths...),
			within(bodyFungusPale, []uint8{0}, fungusGrowths...)}, Male: dyes},
		{Key: "bag", Label: "Bag", Kind: KindColour, Parts: []OptionPart{whole(bodyFungus, 2)}, Male: dyes},
	},
	bodyGargoyle: {{Key: "body", Label: "Stone", Kind: KindBody, Male: []OptionStep{
		{"Granite", bodyGargoyle, 1}, {"Moss", 0x4E2A945C, 1.25}, {"Ice", 0xAEC5D9F6, 0.83}, {"Obsidian", 0x001355C8, 1.99},
		{"Shadow", 0x6D959FD9, 1.54}, {"Fire", 0x9304BAB5, 1.33}}}},
}

// RaceOptions is race's rows: those of its own male body (racedefs', the
// froglok's); none for a race with no body of the table's, a game race, or a
// server that offers no races.
func RaceOptions(race int) []OptionRow {
	return raceOptionRows[uint32(raceOwnBody(race, 0))]
}

// raceOwnBody is race's own body for sex as its row has it (a female with none,
// the male's); the froglok's; 0 for any other race.
func raceOwnBody(race, sex int) int64 {
	if race == raceFroglok {
		return FroglokBody
	}
	d, ok := customRace(race)
	if !ok {
		return 0
	}
	if sex == 1 && d.Body[1] != 0 {
		return d.Body[1]
	}
	return d.Body[0]
}

// optionBody is body's step in a body row of race's options, either sex's, and
// whether it is one.
func optionBody(race int, body uint32) (OptionStep, bool) {
	for _, r := range RaceOptions(race) {
		if r.Kind != KindBody {
			continue
		}
		for _, list := range [][]OptionStep{r.Male, r.Female} {
			for _, s := range list {
				if s.Value == body {
					return s, true
				}
			}
		}
	}
	return OptionStep{}, false
}

// ChosenBody reports whether model is a body a character of race wears by its
// own choice: a step of its race's body rows that is not its race's own body
// for either sex. Such a body is its look, kept in any armour (armorbody.go).
func ChosenBody(race int, model uint32) bool {
	if model == 0 || model == uint32(raceOwnBody(race, 0)) || model == uint32(raceOwnBody(race, 1)) {
		return false
	}
	_, ok := optionBody(race, model)
	return ok
}

// OptionBodySize is the scale a character of race wearing model is drawn at
// against its race's own: its body step's size; 1 for any other body.
func OptionBodySize(race int, model uint32) float32 {
	if s, ok := optionBody(race, model); ok && s.Size > 0 {
		return s.Size
	}
	return 1
}

// OptionBodies is every body a race's body rows offer, any race's: the bodies a
// player may wear by its own choice (a client that can lets them hold items as
// its race's own do).
func OptionBodies() []uint32 {
	seen := map[uint32]bool{}
	var out []uint32
	for _, rows := range raceOptionRows {
		for _, r := range rows {
			if r.Kind != KindBody {
				continue
			}
			for _, list := range [][]OptionStep{r.Male, r.Female} {
				for _, s := range list {
					if !seen[s.Value] {
						seen[s.Value] = true
						out = append(out, s.Value)
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// OptionChoice is a step chosen of a row, by the row's key.
type OptionChoice struct {
	Key  string
	Step int
}

// ResolveOptions is what a new character of race and sex wears by its choices:
// its body (0: none chosen, its race's own) and its looks (a charlook row a part
// whose step is not the first). An error names a choice no client of the
// server's own rows could make: a row the race has not (a race with none, the
// froglok's while creation does not offer it), a step past its row's, a row
// chosen twice.
func ResolveOptions(race, sex int, choices []OptionChoice) (body int64, looks []database.CharLook, err error) {
	rows := RaceOptions(race)
	if race == raceFroglok && !Creation().Races {
		rows = nil // the froglok's are offered only while creation offers the froglok (optionRaces)
	}
	seen := map[string]bool{}
	for _, c := range choices {
		var row *OptionRow
		for i := range rows {
			if rows[i].Key == c.Key {
				row = &rows[i]
			}
		}
		switch {
		case row == nil:
			return 0, nil, fmt.Errorf("no row %q for race %d", c.Key, race)
		case seen[c.Key]:
			return 0, nil, fmt.Errorf("row %q twice", c.Key)
		case c.Step < 0 || c.Step >= len(row.Steps(sex)):
			return 0, nil, fmt.Errorf("row %q step %d of %d", c.Key, c.Step, len(row.Steps(sex)))
		}
		seen[c.Key] = true
		step := row.Steps(sex)[c.Step]
		switch row.Kind {
		case KindBody:
			body = int64(int32(step.Value))
		case KindColour, KindTexture:
			if c.Step > 0 {
				looks = append(looks, database.CharLook{Part: row.Key, Kind: int(row.Kind), Value: step.Value})
			}
		}
	}
	return body, looks, nil
}

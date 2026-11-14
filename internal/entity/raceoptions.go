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
	// Always is a texture row whose step is laid even at its first: the Wisp's
	// wisp (a body's sprite, not a texture), which every Wisp wears, so its
	// charlook row is written at any step and every client that asked is told.
	Always bool
	// Answer is the least answer to the options' offer a client must have
	// given to be told the row (0 or 1: any that asked; 3: the barbarians'
	// tattoo, which a client that answered 2 cannot draw: tattoos.go).
	Answer uint32
	// AtSelect is a custom race's colour or texture row character select's
	// Customize re-chooses (SelectRows; a game race's are its skin's): the
	// Wisp's colour.
	AtSelect bool
	// GM is a row a GM sets (/aura <colour> hair|gnomecap|glasses: gmlooks.go),
	// never offered on a create page or Customize (RaceOptionsFor leaves it
	// out), whose charlook value is laid as any row's (lookParts).
	GM bool
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
	// ModePaint is ModeDye with the part's layer colour made white, so the dye
	// is the whole colour (a gnome's glasses' layer is 36 grey, its cap's
	// red): gmlooks.go's. A client before it changes nothing.
	ModePaint uint32 = 8
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

	// the Jal'Raeth (2026-11-09, the owner: "floating monk, alien type creatures"):
	// the commoners' and nobles' robed body, the channelers' bare-chested one, the
	// knights' and champions' armoured one; one 42-node skeleton, all three floating
	// cross-legged
	bodyJalRaeth        = 0xF51BAB09
	bodyJalRaethChannel = 0x4B24FF7E
	bodyJalRaethKnight  = 0x491F6347

	// the Wisp (2026-11-09, the owner: "the armor will form like a human and also the
	// weapons swing like humans. like when humans are invis, but the wisp is inside the
	// helm"): a human's body, which a client that can draws as its armour alone, and the
	// will o' wisp's (the world's wisps, orbs of light and roaming spirits wear it) in
	// the helm
	bodyHumanMale   = 0x70D898C6
	bodyHumanFemale = 0x0D537335
	bodyWillOWisp   = 0x8143DC70

	// the Mermaid (2026-11-11, petition 1620, Jibrish: "Add mermaid as a
	// playable race"): the merfolk's four bodies, one 24-node skeleton and one
	// play list (a sea creature's: swimmerbody.go), the merman the male's, the
	// mermaid the female's, the sirens of Odus her options
	bodyMerman          = 0xA316EB78
	bodyMermaid         = 0x74BA7C3D
	bodySiren           = 0xBF21FEF5
	bodyEnchantingSiren = 0x1740427C
	// the sea king (Quintashi's body, 2026-11-12, the owner: "also give mermaid
	// a male variant"): the merfolk's skeleton and play list, green-scaled, a
	// crown; a client that draws it for a player leaves out the trident its
	// mesh holds (vanilla), so it holds the player's own weapons
	bodySeaKing = 0xD275EE8F
)

// The giants' kinds, every one on the hill giant's skeleton and as tall (4.81).
var (
	giantsMale = []OptionStep{{"Hill giant", bodyHillGiant, 1}, {"Ice giant", 0x8117C627, 1}, {"Sand giant", 0xE617303B, 1},
		{"Fire giant", 0x72A8FDBB, 1}, {"Fire giant (vest)", 0x7F2604DF, 1}, {"Cyclops", 0x05F59B65, 1},
		{"Fire giant (reaver)", 0x2D686972, 0.95}, {"Fire giant king", 0xBF6ADFDB, 0.91}}
	giantsFemale = []OptionStep{{"Hill giant", 0xF34501CB, 1}, {"Ice giant", 0x286C6E9E, 1}, {"Sand giant", 0x8E2A6552, 1},
		{"Cyclops", 0xE30827CC, 1}}
)

// The women of the goblins and the gnolls (the GM Jibrish's petition 1647, 2026-11-12, the owner:
// "a lot of the new races have female variants that we have not implemented yet"), found by sight
// on every CHAR.ESF body (front close-ups; their CSprite header's sex byte is 1, the men's 0): each
// goblin tribe has a woman (a halter or a top over a bust, the men bare-chested), and four gnoll
// bodies wear a bodice over a bust. Each list is the women first, the first her race's own female
// body (racedefs, tools/race_female_bodies2_20261112.py), then every man's body as the male list
// has it (a woman may still choose any), sizes as the male list's.
var (
	goblinsFemale = []OptionStep{{"Grey goblin", 0xEDB4E0AD, 1}, {"Grey goblin (robed)", 0x687E7645, 1},
		{"Green goblin", 0xEC0885EB, 1.4}, {"Green goblin (robed)", 0x1F8E11D3, 1.4}, {"Ice goblin", 0xA5F92F43, 0.875},
		{"Ice goblin (robed)", 0x1245E56B, 0.875}, {"Fire goblin", 0xEE1DDFBA, 1.17}, {"Fire goblin (robed)", 0xCEF5874A, 1.17},
		{"Grey goblin (brute)", bodyGoblin, 1}, {"Grey goblin (brute, robed)", 0x581B6A80, 1}, {"Green goblin (brute)", 0x96371804, 1.4},
		{"Green goblin (brute, robed)", 0xBCDF0C04, 1.4}, {"Ice goblin (brute)", 0xE96179D4, 0.875}, {"Fire goblin (brute)", 0x6C7C07ED, 1.17},
		{"Fire goblin (brute, robed)", 0xBE927B85, 1.17}, {"Grey goblin (overseer)", 0x2ECBB95A, 0.93}, {"Grey goblin (skull mask)", 0xB656DFEA, 0.93},
		{"Green goblin (warlord)", 0x3AB96672, 1.27}, {"Green goblin (warmaster)", 0xF54C2142, 1.27}, {"Ice goblin (wizard)", 0x27A4E554, 0.875},
		{"Ice goblin (mystic)", 0x5937EA22, 0.82}, {"Ice goblin (chieftain)", 0x507C4A52, 0.82}, {"Brown goblin (inquisitor)", 0x1CCDF0EF, 1.08},
		{"Brown goblin (king)", 0x2F34B867, 1.08}}
	gnollsFemale = []OptionStep{{"Black", 0xF5F3A450, 1}, {"Black and tan", 0x17DA0FEB, 1}, {"Grey", 0x71F163CA, 1},
		{"Grey (gown)", 0xB15628DA, 1}, {"Black (breastplate)", bodyGnoll, 1}, {"Black (captain)", 0x223FD6BC, 0.91},
		{"Black (robed)", 0x097BFC70, 1}, {"Black (elder)", 0xB7B21DFC, 0.91}, {"Black and tan (vest)", 0x4A9BAB9E, 1},
		{"Black and tan (robed)", 0x7492678E, 1}, {"Black (teal robe)", 0x585E5ED9, 1}, {"Black and tan (green robe)", 0x16BA0BD3, 1},
		{"Black and tan (packlord)", 0x3C6D3677, 0.91}, {"Black and tan (armour)", 0xC2D7DD2F, 0.91}, {"Grey (armour)", 0xC3B94FA5, 1},
		{"Grey (red scarf)", 0x147AC3EE, 0.91}, {"Grey (robed)", 0x09E0AE7D, 1}, {"Grey (red robe)", 0xCE06735E, 0.91},
		{"Reanimated", 0x2EDA50DC, 1}, {"Reanimated (robed)", 0x16E320F7, 1}}
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
	// wispColours are a wisp's: the will o' wisp's own glow, then colours laid
	// over its shading, kept bright (a glow darkened reads as a hole)
	wispColours = []OptionStep{{"Natural", 0, 0}, {"White", Dye(210, 10, 135), 0}, {"Gold", Dye(48, 85, 120), 0},
		{"Amber", Dye(30, 90, 115), 0}, {"Crimson", Dye(355, 85, 105), 0}, {"Rose", Dye(330, 70, 120), 0},
		{"Violet", Dye(275, 75, 115), 0}, {"Azure", Dye(212, 85, 115), 0}, {"Aqua", Dye(180, 80, 115), 0},
		{"Emerald", Dye(135, 80, 110), 0}, {"Lime", Dye(90, 80, 120), 0}, {"Shadow", Dye(260, 30, 60), 0}}
	barks = []OptionStep{{"Natural", 0, 0}, {"Umber", Dye(28, 45, 95), 0}, {"Ash", Dye(30, 6, 110), 0},
		{"Moss", Dye(90, 30, 95), 0}, {"Plum", Dye(310, 25, 90), 0}, {"Ochre", Dye(42, 55, 110), 0},
		{"Char", Dye(20, 15, 55), 0}}
)

// hairs are the player races' eight hair textures (VICSpriteCust's $Hair), which
// the satyrs' and the brownie's heads already wear one of.
var hairs = []OptionStep{{"Hair 1", 0x98F86489, 0}, {"Hair 2", 0x221E7298, 0}, {"Hair 3", 0x6DCEF2D0, 0},
	{"Hair 4", 0xFEBCE4D5, 0}, {"Hair 5", 0xD499494B, 0}, {"Hair 6", 0x8931CEE6, 0}, {"Hair 7", 0x9F87C7AF, 0},
	{"Hair 8", 0xDC79B7C3, 0}}

// hairSteps are the eight hair textures after the body's own (Natural, 0): a
// row of a race whose bodies wear different hair (an NPC's own strip, B5632329,
// EF0C9AEB, 87BFA9B5, or one of the eight) on parts of the same name. Every one
// is a 64x64 strip of hair with no layout of its own, so any of them lies on
// any hair part as the body's own does.
var hairSteps = append([]OptionStep{{"Natural", 0, 0}}, hairs...)

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

// The merfolk's parts: rects of their one texture (one palette entry), which
// the four bodies lay out alike (read off each triangle's UV centre and bone:
// the hair a strip at the left, the face beside it, the torso and the arms to
// the right of them; below, the tail, the scales of the hips, the fins and,
// on the women, a shell over each breast).
var (
	merfolkBodies = []uint32{bodyMerman, bodyMermaid, bodySiren, bodyEnchantingSiren}
	merfolkSkin   = [][4]uint8{{63, 0, 128, 128}, {128, 0, 224, 128}, {224, 0, 255, 192}}
	merfolkHair   = [][4]uint8{{0, 0, 63, 128}}
	merfolkTail   = [][4]uint8{{0, 128, 128, 255}, {128, 128, 224, 190}, {168, 192, 255, 255}}
	merfolkShells = [][4]uint8{{128, 190, 168, 255}}
	// a mermaid's skin: the tones and the sea's own
	merfolkSkins = append(append([]OptionStep{}, tones...), OptionStep{"Sea green", Dye(150, 28, 100), 0},
		OptionStep{"Deep blue", Dye(215, 30, 95), 0}, OptionStep{"Pearl", Dye(270, 10, 125), 0})
)

// merfolkParts is the part of rects on each merfolk body.
func merfolkParts(rects [][4]uint8) []OptionPart {
	out := make([]OptionPart, 0, len(merfolkBodies))
	for _, b := range merfolkBodies {
		out = append(out, within(b, []uint8{0}, rects...))
	}
	return out
}

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
			{"Lioness", 0x39FEAA5F, 1}, {"Lioness (robed)", 0x7654AC3C, 1}, {"Lioness (warrior)", 0xD8E0F980, 0.82}}},
		{Key: "mane", Label: "Mane", Kind: KindTexture, Parts: []OptionPart{
			whole(0x77A7E7B8, 2), whole(0x5D40C8AB, 1), whole(0xE700DE53, 3), whole(0xD8E0F980, 6)}, Male: hairSteps}},
	bodyGoblin: {{Key: "body", Label: "Tribe", Kind: KindBody, Male: []OptionStep{
		{"Grey goblin", bodyGoblin, 1}, {"Grey goblin (robed)", 0x581B6A80, 1}, {"Green goblin", 0x96371804, 1.4},
		{"Green goblin (robed)", 0xBCDF0C04, 1.4}, {"Ice goblin", 0xE96179D4, 0.875}, {"Ice goblin (robed)", 0xA5F92F43, 0.875},
		{"Fire goblin", 0x6C7C07ED, 1.17}, {"Fire goblin (robed)", 0xBE927B85, 1.17}, {"Fire goblin (shaman)", 0xCEF5874A, 1.17},
		{"Grey goblin (gladiator)", 0xEDB4E0AD, 1}, {"Grey goblin (purple robe)", 0x687E7645, 1}, {"Grey goblin (overseer)", 0x2ECBB95A, 0.93},
		{"Grey goblin (skull mask)", 0xB656DFEA, 0.93}, {"Green goblin (bandit)", 0xEC0885EB, 1.4}, {"Green goblin (high priest)", 0x1F8E11D3, 1.4},
		{"Green goblin (warlord)", 0x3AB96672, 1.27}, {"Green goblin (warmaster)", 0xF54C2142, 1.27}, {"Ice goblin (soothsayer)", 0x1245E56B, 0.875},
		{"Ice goblin (wizard)", 0x27A4E554, 0.875}, {"Ice goblin (mystic)", 0x5937EA22, 0.82}, {"Ice goblin (chieftain)", 0x507C4A52, 0.82},
		{"Fire goblin (poacher)", 0xEE1DDFBA, 1.17}, {"Brown goblin (inquisitor)", 0x1CCDF0EF, 1.08}, {"Brown goblin (king)", 0x2F34B867, 1.08}},
		Female: goblinsFemale},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{
			whole(bodyGoblin, 7), whole(0x581B6A80, 7, 9), whole(0x96371804, 0), whole(0xBCDF0C04, 7), whole(0xE96179D4, 6),
			whole(0xA5F92F43, 0), whole(0xEDB4E0AD, 1), whole(0x687E7645, 0, 4), whole(0xB656DFEA, 1), whole(0xEC0885EB, 0),
			whole(0x1F8E11D3, 4), whole(0xF54C2142, 9), whole(0x1245E56B, 2), whole(0x27A4E554, 7), whole(0x507C4A52, 7)}, Male: hairSteps}},
	bodyAviakM: {{Key: "body", Label: "Bird", Kind: KindBody,
		Male: []OptionStep{{"Hawk", bodyAviakM, 1}, {"Hawk (robed)", 0x262C572A, 1}, {"Raven", 0xDC641055, 1},
			{"Raven (robed)", 0x41EFF500, 1}, {"Vulture", 0x3C3DE6CA, 0.91}, {"Vulture (robed)", 0xC04CEC07, 0.91},
			{"Owl", 0xB1EEB204, 0.91}, {"Owl (robed)", 0x8B332785, 0.91}, {"Raven (gown)", 0x246B5FED, 1},
			{"Raven (black)", 0x5585143C, 1}, {"Hawk (green robe)", 0xF83BD167, 1}},
		// the women's numbers kept (TestRaceOptionStepsKeepTheirNumbers): the hawk is hers, and the
		// three bodies last are women too (the GM Jibrish's petition 1647, by sight: the ravens'
		// breastplates, a bust under the green robe), the rest the men's
		Female: []OptionStep{{"Hawk", bodyAviakF, 1}, {"Hawk (robed)", 0x262C572A, 1}, {"Raven", 0xDC641055, 1},
			{"Raven (robed)", 0x41EFF500, 1}, {"Vulture", 0x3C3DE6CA, 0.91}, {"Vulture (robed)", 0xC04CEC07, 0.91},
			{"Owl", 0xB1EEB204, 0.91}, {"Owl (robed)", 0x8B332785, 0.91}, {"Raven (gown)", 0x246B5FED, 1},
			{"Raven (black)", 0x5585143C, 1}, {"Hawk (green robe)", 0xF83BD167, 1}}}},
	bodyLizardman: {{Key: "body", Label: "Scales", Kind: KindBody, Male: []OptionStep{
		{"Strapped", bodyLizardman, 1}, {"Pauldrons", 0xD45B9F1F, 1.09}, {"Bare", 0x2C1D8724, 1.09}, {"Robed", 0x7468EAE7, 1.09},
		{"Robed (hierophant)", 0x16BB9E24, 1.1}, {"Grey", 0xEEBF6DA5, 1}}}},
	bodyKappa: {{Key: "body", Label: "Shell", Kind: KindBody, Male: []OptionStep{
		{"Brown shell", bodyKappa, 1}, {"Marked shell", 0xC3EC4F58, 0.91}, {"Banded shell", 0xF6747024, 1.05},
		{"Straw hat", 0xF0FAB933, 0.83}}},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{whole(0xC3EC4F58, 6), whole(0xF6747024, 5)}, Male: hairSteps}},
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
		{"Black and tan (scout)", 0x17DA0FEB, 1}, {"Black and tan (robed)", 0x7492678E, 1},
		{"Black (teal robe)", 0x585E5ED9, 1}, {"Black and tan (green robe)", 0x16BA0BD3, 1}, {"Black and tan (packlord)", 0x3C6D3677, 0.91},
		{"Black and tan (armour)", 0xC2D7DD2F, 0.91}, {"Grey", 0x71F163CA, 1}, {"Grey (armour)", 0xC3B94FA5, 1},
		{"Grey (red scarf)", 0x147AC3EE, 0.91}, {"Grey (robed)", 0x09E0AE7D, 1}, {"Grey (gown)", 0xB15628DA, 1},
		{"Grey (red robe)", 0xCE06735E, 0.91}, {"Reanimated", 0x2EDA50DC, 1}, {"Reanimated (robed)", 0x16E320F7, 1}},
		Female: gnollsFemale}},
	bodyOrc: {{Key: "body", Label: "Clan", Kind: KindBody,
		Male: []OptionStep{{"Orc", bodyOrc, 1}, {"Orc (long hair)", 0x84F47767, 0.92}, {"Orc (grey mail)", 0x656DD4C5, 1},
			{"Deathfist", 0x37FD13F9, 1.02}, {"Deathfist (kilt)", 0x3198F5B1, 1.06}, {"Hatebone elder", 0xB2FBD7EF, 0.92},
			{"Deathfist (fire weaver)", 0xB086ADA7, 0.92}},
		Female: []OptionStep{{"Orc", 0x0A03EB92, 1.01}, {"Orc (grey mail)", 0xDA515962, 1.01}, {"Orc (brute)", bodyOrc, 1},
			{"Deathfist", 0x37FD13F9, 1.02}, {"Deathfist (kilt)", 0x3198F5B1, 1.06}, {"Deathfist (healer)", 0x0205D208, 1.07},
			{"Deathfist (slayer)", 0xC76F1FA8, 1.02}, {"Deathfist (chieftain)", 0xE144942F, 0.98}, {"Hatebone elder", 0xB2FBD7EF, 0.92},
			{"Deathfist (fire weaver)", 0xB086ADA7, 0.92}}},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{
			whole(0x84F47767, 1, 2, 3, 4, 5), whole(0x0A03EB92, 0, 1, 2, 3, 4), whole(0xDA515962, 0, 1, 2, 3, 4),
			whole(0xB2FBD7EF, 1, 2, 3, 4, 5), whole(0x0205D208, 4), whole(0xC76F1FA8, 0, 1, 2, 3, 8),
			whole(0xB086ADA7, 3, 5, 6, 7, 8), whole(0xE144942F, 3)}, Male: hairSteps}},
	bodyKobold: {{Key: "body", Label: "Hide", Kind: KindBody, Male: []OptionStep{
		{"Kobold", bodyKobold, 1}, {"Kobold (augur)", 0x6B9B7016, 0.9}, {"Patched", 0x597325B5, 1}, {"Pauldrons", 0x9CDF3C2D, 0.82}}}},
	// the minotaur's black fur takes no colour worth the name (a dark grey at most): its hide and horns do
	bodyMinotaur: {
		{Key: "body", Label: "Minotaur", Kind: KindBody, Male: []OptionStep{{"Black", bodyMinotaur, 1}, {"Brown (elder)", 0x0AE71D02, 0.86}}},
		{Key: "hide", Label: "Hide", Kind: KindColour, Parts: []OptionPart{
			within(bodyMinotaur, []uint8{1}, [4]uint8{195, 105, 255, 250}),
			within(bodyMinotaur, []uint8{2}, [4]uint8{90, 0, 255, 229}, [4]uint8{0, 0, 90, 110})}, Male: tones},
		{Key: "horns", Label: "Horns", Kind: KindColour, Parts: []OptionPart{
			within(bodyMinotaur, []uint8{2}, [4]uint8{190, 230, 255, 255})}, Male: dyes},
	},
	bodyRatman: {{Key: "body", Label: "Garb", Kind: KindBody, Male: []OptionStep{
		{"Ratman", bodyRatman, 1}, {"Hooded", 0x0ECC836A, 1.18}, {"Hooded (pauldrons)", 0x133A79CA, 1},
		{"Trickster", 0x46BA26A9, 1.08}}}},
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
		{Key: "body", Label: "Spirit", Kind: KindBody, Male: []OptionStep{{"Dryad", bodyDryad, 1}, {"Erych guardian", 0x21F311BB, 1.38},
			{"Haliai", 0xACE71E26, 1.5}, {"Erych arbiter", 0xB90ADDD2, 1.38}}},
		{Key: "hair", Label: "Hair", Kind: KindTexture, Parts: []OptionPart{
			whole(0x21F311BB, 0, 1, 6), whole(0xACE71E26, 1, 6), whole(0xB90ADDD2, 0, 1, 8, 9, 10)}, Male: hairSteps},
		{Key: "bark", Label: "Bark", Kind: KindColour, Parts: []OptionPart{whole(bodyDryad, 0, 2, 3, 4)}, Male: barks},
		{Key: "leaves", Label: "Leaves", Kind: KindColour, Parts: []OptionPart{whole(bodyDryad, 1, 7)}, Male: dyes},
	},
	bodySkeleton: {{Key: "body", Label: "Bones", Kind: KindBody, Male: []OptionStep{
		{"Bone", bodySkeleton, 1}, {"Old bone", 0x45A8B064, 0.91}, {"Mossy", 0x198BAA64, 0.91}, {"Frost", 0x648DFC11, 1},
		{"Great bone", 0x6AF98A82, 0.9}, {"Great old bone", 0x197F51C7, 0.82}, {"Great mossy", 0x90F16A6F, 0.82},
		{"Great frost", 0x7D9F41B2, 0.9}}}},
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
	bodyJalRaeth: {{Key: "body", Label: "Garb", Kind: KindBody, Male: []OptionStep{
		{"Robes", bodyJalRaeth, 1}, {"Channeler", bodyJalRaethChannel, 1}, {"Knight", bodyJalRaethKnight, 0.94}}}},
	// the Wisp: a human's body for its sex (one step: not shown, the female's her
	// own), and the wisp laid on it always (a part of a head entry, which is only
	// where it is said to be: a client that draws wisps puts the wisp's body at the
	// helm's node, and one that does not lays nothing, the value being no texture)
	bodyHumanMale: {
		{Key: "body", Label: "Body", Kind: KindBody, Male: []OptionStep{{"Human", bodyHumanMale, 1}},
			Female: []OptionStep{{"Human", bodyHumanFemale, 1}}},
		{Key: "wisp", Label: "Wisp", Kind: KindTexture, Always: true,
			Parts: wispParts,
			Male:  []OptionStep{{"Will o' wisp", bodyWillOWisp, 0}}},
		// the wisp's colour (the owner, 2026-11-12: "also add color customize to
		// wisp char sel"), on the wisp's own part: a client that draws wisps
		// lays the colour on the wisp it hangs there (vanilla's wisp.rs), and the
		// body under it is never drawn (its head is no worn slot's); chosen at
		// creation and again at select's Customize (SelectRows)
		{Key: wispColourKey, Label: "Color", Kind: KindColour, Parts: wispParts, Male: wispColours, AtSelect: true},
	},
	// the Mermaid: the merman or the sea king for a man (at the mermaid's height;
	// the sea king's skin his scales and crown's every entry but the trident's), the
	// mermaid and the two sirens for a woman, each as tall; the colours on every
	// one of them (the shells on the women's alone: the merman has none)
	bodyMerman: {
		{Key: "body", Label: "Kind", Kind: KindBody, Male: []OptionStep{{"Merman", bodyMerman, 0.83}, {"Sea king", bodySeaKing, 0.334}},
			Female: []OptionStep{{"Mermaid", bodyMermaid, 1}, {"Siren", bodySiren, 0.77}, {"Enchanting siren", bodyEnchantingSiren, 0.667}}},
		{Key: "skin", Label: "Skin", Kind: KindColour, Parts: append(merfolkParts(merfolkSkin), whole(bodySeaKing, 0, 1, 2, 3)), Male: merfolkSkins},
		{Key: "hair", Label: "Hair", Kind: KindColour, Parts: merfolkParts(merfolkHair), Male: dyes},
		{Key: "tail", Label: "Tail", Kind: KindColour, Parts: merfolkParts(merfolkTail), Male: dyes},
		{Key: "shells", Label: "Shells", Kind: KindColour, Parts: merfolkParts(merfolkShells)[1:], Male: dyes},
	},
	bodyGargoyle: {{Key: "body", Label: "Stone", Kind: KindBody, Male: []OptionStep{
		{"Granite", bodyGargoyle, 1}, {"Moss", 0x4E2A945C, 1.25}, {"Ice", 0xAEC5D9F6, 0.83}, {"Obsidian", 0x001355C8, 1.99},
		{"Shadow", 0x6D959FD9, 1.54}, {"Fire", 0x9304BAB5, 1.33}}}},
}

// RaceOptions is race's rows: those of its own male body (racedefs', the
// froglok's); a game race's skin shades (skinshades.go) on a server that
// offers them; none for a race with no body of the table's, or a server that
// offers no races.
func RaceOptions(race int) []OptionRow {
	if isGameRace(race) {
		return gameRaceRows(race)
	}
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
			if c.Step > 0 || row.Always {
				looks = append(looks, database.CharLook{Part: row.Key, Kind: int(row.Kind), Value: step.Value})
			}
		}
	}
	// a row laid always and not chosen (a client that sends no block, an older
	// one): its first step
	for _, r := range rows {
		if r.Always && !seen[r.Key] && len(r.Steps(sex)) > 0 {
			looks = append(looks, database.CharLook{Part: r.Key, Kind: int(r.Kind), Value: r.Steps(sex)[0].Value})
		}
	}
	return body, looks, nil
}

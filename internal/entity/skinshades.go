package entity

import (
	"fmt"
	"slices"

	"github.com/eqoa/server/internal/database"
)

// skinshades.go — the game's ten races' skin shades (the owner, 2026-11-11:
// "lets expand the skin shades for races, humans, ogres, gnomes, elfs, etc. can
// have dark skin like erudites, erudites can have light skin like humans, etc.";
// claude-notes/skin-shades-20261111.md).
//
// What makes a player race's skin, read off the disc (CHAR.ESF's bodies,
// CHARFACE.CSF): each body's torso, upper arms, forearms, hands and legs are
// palette entries of two layers, the skin's texture under the armour's (the
// text slots name the second: SetArmorSlot never touches the skin), and its face
// is an entry of one, the race's face texture (SetFace's). The skin textures are
// few and shared: the human's set (the human, the elf, the barbarian and the
// erudite wear it), the gnome's (the gnome, dwarf, halfling and ogre), the dark
// elf's and the troll's own. An erudite is dark because its bodies multiply the
// human's skin by a layer colour (109, 83, 77) and its faces are painted dark; a
// barbarian's by (237, 219, 188). Some races' forearms, hands and the halfling's
// feet are entries of one layer, whose skin texture an armour's replaces.
//
// A shade is a skin colour (Shade: hue, saturation, lightness), the same for
// every race: a client that shows them moves the skin of each skin entry's first
// layer to it (its texels' own shading kept, an eye's white, an iris or a strap
// left as it is) and makes the layer's colour white, only where that layer holds
// the body's own skin or a face (never an armour's or a robe's texture). So a
// human may be as dark as an erudite and an erudite as light as a human. It is a
// race option (raceoptions.go) of the race's own bodies, kept in charlook (part
// "skin"), sent only to a client that answered the options' offer 2 (it shows a
// game race's rows under the four) and, as every look, to every client that
// asked. A PS2, and a client that does not know the mode, draws the race's own.

// ModeShade is a skin shade (Shade): a client lays it on a skin texture.
const ModeShade uint32 = 3

// Shade is a ModeShade colour: hue in degrees, saturation and lightness in per
// cent, all the colour a skin becomes.
func Shade(hue, sat, light int) Recolour {
	return ModeShade<<24 | turn(hue)<<16 | optionPct(sat, 255)<<8 | optionPct(light, 255)
}

// skinRowKey is the skin shades' row (charlook part "skin").
const skinRowKey = "skin"

// skinShades are the shades every game race may pick, the race's own skin
// first. Measured off the disc's skin (median hue, saturation, lightness): the
// human's body (16, 53, 70) and face (16, 43, 64), the erudite's body under its
// colour (12, 38, 26) and face (25, 62, 22), the gnome's (19, 46, 59), the dark
// elf's (214, 10, 46), the troll's (76, 17, 43).
var skinShades = []OptionStep{
	{"Natural", 0, 0},
	{"Porcelain", Shade(22, 40, 80), 0},
	{"Fair", Shade(20, 46, 73), 0},
	{"Light", Shade(18, 48, 67), 0},
	{"Tan", Shade(22, 46, 57), 0},
	{"Olive", Shade(32, 34, 48), 0},
	{"Bronze", Shade(26, 44, 41), 0},
	{"Brown", Shade(26, 40, 32), 0},
	{"Dark", Shade(27, 42, 23), 0},
	{"Ebony", Shade(25, 30, 14), 0},
	{"Ashen", Shade(212, 10, 45), 0},
	{"Moss", Shade(76, 17, 43), 0},
}

// skinBody is one game race body's skin entries: each two-layer limb and the
// face (and the halfling's bare feet), as CHAR.ESF builds it.
type skinBody struct {
	body    uint32
	entries []uint8
}

// gameSkins are each game race's bodies' skin entries, male then female (the
// survey: claude-notes/skin-shades-20261111.md). Left out: every race's feet
// but the halfling's (boots: the body's own art), the elves' forearms (a
// sleeve), the ogre's tusks, hair and eyebrows.
var gameSkins = map[int][2]skinBody{
	0: {{0x70D898C6, []uint8{0, 2, 3, 4, 5}}, {0x0D537335, []uint8{3, 5, 6, 7, 9}}},
	1: {{0xD8D56FF8, []uint8{0, 2, 8, 10}}, {0xE18919F7, []uint8{0, 1, 5, 8}}},
	2: {{0x7ED89DAA, []uint8{0, 1, 3, 4, 5}}, {0x8C9B4B39, []uint8{5, 9, 10, 11, 13}}},
	3: {{0xA99C6B15, []uint8{2, 3, 4, 5, 6}}, {0x4C4084FC, []uint8{2, 5, 6, 7, 8}}},
	4: {{0xAA5B0447, []uint8{2, 3, 4, 5, 6}}, {0x350BFDBE, []uint8{0, 1, 2, 3, 4}}},
	5: {{0x4C6FA532, []uint8{0, 6, 7, 8, 9}}, {0x4DE32701, []uint8{1, 2, 3, 4, 5}}},
	6: {{0x61CA3EDF, []uint8{0, 5, 7, 8, 9}}, {0xC44AD516, []uint8{0, 4, 6, 7, 8}}},
	7: {{0xA3DB3FE2, []uint8{1, 2, 3, 4, 5, 6, 7}}, {0x8A27A731, []uint8{2, 3, 4, 5, 6, 7, 8}}},
	8: {{0x320C0B47, []uint8{0, 6, 8, 9, 10}}, {0x4F1A3CBE, []uint8{0, 4, 6, 8, 9}}},
	9: {{0x84807490, []uint8{1, 4, 5, 6, 7}}, {0xACC00B4F, []uint8{1, 3, 4, 5, 6}}},
}

// gameRaceRows is race's rows as a game race (0-9): the skin shades on its
// own bodies, on a server that offers them (world.skin_shades), after the
// barbarians' tattoo (world.tattoos: tattoos.go); none else.
func gameRaceRows(race int) []OptionRow {
	b, ok := gameSkins[race]
	if !ok {
		return nil
	}
	// the tattoo first: a client lays it on the face before the skin's shade,
	// which then shades the tattooed face as it does the disc's own
	rows := tattooRows(race)
	if Creation().Skins {
		rows = append(rows, OptionRow{Key: skinRowKey, Label: "Skin", Kind: KindColour,
			Parts: []OptionPart{whole(b[0].body, b[0].entries...), whole(b[1].body, b[1].entries...)}, Male: skinShades})
	}
	return append(rows, gmLookRows(race)...)
}

// isGameRace is whether race is one of the game's ten (its rows are shown under
// the create page's four, to a client that answered the offer 2).
func isGameRace(race int) bool {
	_, ok := gameSkins[race]
	return ok
}

func canReselectRow(r OptionRow, race, sex int, extended bool) bool {
	return !r.GM && len(r.Steps(sex)) > 1 && (extended || isGameRace(race) || r.AtSelect)
}

// SelectRows lists the creation appearance rows character select can re-choose.
// Older clients show only the original races' extra rows and the Wisp's colour.
func SelectRows(race, sex int, extended bool) []string {
	if race == raceFroglok && !Creation().Races {
		return nil
	}
	var out []string
	for _, r := range RaceOptions(race) {
		if canReselectRow(r, race, sex, extended) {
			out = append(out, r.Key)
		}
	}
	return out
}

// ReselectOptions validates a selection against the race and sex stored on the
// character. It returns the chosen model (zero if omitted), parts to reset to
// their defaults, and parts to store. Unmentioned parts keep their saved values.
func ReselectOptions(race, sex int, level uint32, extended bool, choices []OptionChoice) (model int64, reset []string, looks []database.CharLook, err error) {
	rows := RaceOptionsFor(race, level)
	if race == raceFroglok && !Creation().Races {
		rows = nil
	}
	seen := make(map[string]bool, len(choices))
	for _, c := range choices {
		i := slices.IndexFunc(rows, func(row OptionRow) bool { return row.Key == c.Key })
		switch {
		case i < 0 || !canReselectRow(rows[i], race, sex, extended):
			return 0, nil, nil, fmt.Errorf("no selectable row %q for race %d", c.Key, race)
		case seen[c.Key]:
			return 0, nil, nil, fmt.Errorf("row %q twice", c.Key)
		}
		seen[c.Key] = true
		row := rows[i]
		steps := row.Steps(sex)
		if c.Step < 0 || c.Step >= len(steps) {
			return 0, nil, nil, fmt.Errorf("row %q step %d of %d", c.Key, c.Step, len(steps))
		}
		switch {
		case row.Kind == KindBody:
			model = int64(int32(steps[c.Step].Value))
		case c.Step == 0 && !row.Always:
			reset = append(reset, row.Key)
		default:
			looks = append(looks, database.CharLook{Part: row.Key, Kind: int(row.Kind), Value: steps[c.Step].Value})
		}
	}
	return model, reset, looks, nil
}

package entity

import (
	"math"

	"github.com/eqoa/server/internal/database"
)

// gmlooks.go — a GM's colour on a character's hair, a gnome's cap and a gnome's
// glasses (the owner, 2026-11-14: "/aura white hair lets add /aura red gnomecap
// (gnomes only) /aura gold glasses (if they are wearing glasses, like gnomes)").
//
// The game's hair is one texture (VICSprite::SetHair's, the colour the player
// chose) on every hair text slot (8-18) the hair's style and length show; a
// gnome's red cap is a hair slot too (13, layer colour 128,0,1, shown at hair
// length 3) and so are its glasses (the man's slot 17 at hair style 2+, the
// woman's 18 at style 3; layer colour 36 and 50 grey), which is why they took
// the hair's colour (claude-notes/char-look-0x002a-glasses-20261107.md). Each is
// a GM row of the race's options (OptionRow.GM: never offered), whose parts are
// those slots' palette entries on each body (read off CHAR.ESF's CSprites), kept
// in charlook as a colour (ModePaint: a dye over the texels' own shading, the
// layer colour white) and laid by every client that shows looks
// (RefreshRaceLooks), on both shards as each keeps its own charlook. A PS2 and
// an older client draw the game's.

// The GM rows' keys (charlook part).
const (
	LookHair     = "haircolour"
	LookGnomeCap = "gnomecap"
	LookGlasses  = "glasses"
)

// raceGnome is the gnome's race.
const raceGnome = 3

// hairEntries are each game race's bodies' hair entries (the palette entries
// of text slots 8-18), male then female; the gnomes' cap and glasses and the
// ogres' slot 8 (the man's nose ring) left out.
var hairEntries = map[int][2][]uint8{
	0: {{6, 7, 8, 9, 10, 11, 12, 13}, {0, 1, 2, 4, 10, 11, 12, 13}},
	1: {{1, 3, 4, 5, 6, 11, 12, 13}, {2, 3, 4, 9, 10, 11, 12, 13}},
	2: {{6, 7, 8, 9, 10, 11, 12, 13}, {0, 1, 2, 3, 4, 6, 7, 8}},
	3: {{0, 1, 8, 11, 12, 13}, {0, 1, 3, 4, 10, 13}},
	4: {{0, 1, 8, 9, 10, 11, 12, 13}, {6, 7, 8, 9, 10, 11, 12, 13}},
	5: {{1, 2, 3, 4, 5, 11, 12, 13}, {0, 7, 8, 9, 10, 11, 12, 13}},
	6: {{1, 2, 3, 4, 10, 11, 12, 13}, {1, 2, 3, 9, 10, 11, 12, 13}},
	7: {{0, 8, 9, 10, 11, 12, 13, 14}, {0, 1, 9, 10, 11, 12, 13, 14}},
	8: {{1, 2, 3, 4, 5, 11, 12, 13}, {1, 2, 3, 5, 10, 11, 12, 13}},
	9: {{0, 3, 9, 10, 12, 13, 14}, {0, 9, 10, 11, 12, 13, 14}},
}

// The gnome's cap (text slot 13) and glasses (17 the man's, 18 the woman's)
// entries, male then female.
var (
	gnomeCapEntries     = [2]uint8{10, 12}
	gnomeGlassesEntries = [2]uint8{9, 11}
)

// gmLookRows are race's GM rows: the hair's colour on every game race, the
// cap's and the glasses' on the gnome's.
func gmLookRows(race int) []OptionRow {
	b, ok := gameSkins[race]
	h, hok := hairEntries[race]
	if !ok || !hok {
		return nil
	}
	natural := []OptionStep{{"Natural", 0, 0}}
	rows := []OptionRow{{Key: LookHair, Label: "Hair colour", Kind: KindColour, GM: true, Male: natural,
		Parts: []OptionPart{whole(b[0].body, h[0]...), whole(b[1].body, h[1]...)}}}
	if race == raceGnome {
		rows = append(rows,
			OptionRow{Key: LookGnomeCap, Label: "Cap colour", Kind: KindColour, GM: true, Male: natural,
				Parts: []OptionPart{whole(b[0].body, gnomeCapEntries[0]), whole(b[1].body, gnomeCapEntries[1])}},
			OptionRow{Key: LookGlasses, Label: "Glasses colour", Kind: KindColour, GM: true, Male: natural,
				Parts: []OptionPart{whole(b[0].body, gnomeGlassesEntries[0]), whole(b[1].body, gnomeGlassesEntries[1])}})
	}
	return rows
}

// Paint is a ModePaint colour of an RGB colour (0xRRGGBB): its hue and
// saturation, and its lightness as a scale of the texels' own (a hair texture
// is about 0.45 light: white 190%, black 15%).
func Paint(rgb uint32) Recolour {
	r, g, b := float64(rgb>>16&0xFF)/255, float64(rgb>>8&0xFF)/255, float64(rgb&0xFF)/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (hi + lo) / 2
	var h, s float64
	if hi != lo {
		d := hi - lo
		if l > 0.5 {
			s = d / (2 - hi - lo)
		} else {
			s = d / (hi + lo)
		}
		switch hi {
		case r:
			h = math.Mod((g-b)/d+6, 6)
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h *= 60
	}
	light := int(math.Round(math.Max(15, math.Min(199, l/0.45*100))))
	return ModePaint<<24 | turn(int(math.Round(h)))<<16 | optionPct(int(math.Round(s*100)), 255)<<8 | optionPct(light, 128)
}

// GMLookShown says why a GM's look on part would show nothing on a character
// of race, sex (1 female), hair style and length: "" when it shows. A gnome's
// cap shows at hair length 3; its glasses at hair style 2 or more (a woman's 3).
func GMLookShown(part string, race, sex, hairStyle, hairLength int) string {
	switch part {
	case LookHair:
		if _, ok := hairEntries[race]; !ok {
			return "only the game's ten races' hair takes a colour"
		}
	case LookGnomeCap:
		if race != raceGnome {
			return "gnomecap is a gnome's"
		}
		if hairLength < 3 {
			return "this gnome wears no cap (it shows at hair length 3)"
		}
	case LookGlasses:
		if race != raceGnome {
			return "only gnomes wear glasses"
		}
		if hairStyle < 2 || (sex == 1 && hairStyle < 3) {
			return "this gnome wears no glasses (they show at hair style 2+, a woman's 3)"
		}
	}
	return ""
}

// lookRowOK keeps database's charlook writes to the GM rows' kind.
var _ = database.LookColour

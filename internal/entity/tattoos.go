package entity

// tattoos.go — the barbarians' tattoo (the owner, 2026-11-12: "for barbarians
// customize character lets add tattoo toggle";
// claude-notes/barbarian-tattoo-20261112.md).
//
// What a barbarian's tattoo is, read off the disc: woad painted on the
// barbarian man's face textures ($Race, CHARFACE.CSF), on two of his four: face
// 1 (EB79B952, "bands": a block over the forehead, a band over the eyes, bars
// down the cheeks) and face 3 (0E9525F4, "crest": framed blocks over the brow
// and the eye band; the bodies' own face EFC0FF3E is the same art). Faces 0 and
// 2 and all four of the woman's have none, and no limb of a barbarian has any:
// their skin is the human's skin set. But the man's bare chest wears a tattoo
// too: a swirl and a knotwork band painted on his chest entry's second layer
// (EB71B584, the layer an armour's chest replaces), so it shows while he wears
// none. The woman's chest takes the same texture's leather top (her texture
// coordinates never reach the tattoo's), so hers has no chest tattoo to show.
// The woman's faces lie as the man's do (the eyes on the same texels), so
// either face pattern lies on them as it does on his.
//
// The row is a colour row (ModeTattoo) on each body's face entry and the man's
// chest: a client that draws it takes the pattern's woad off the face the
// character wears (its own, on faces 1 and 3) and lays the one chosen, or none;
// the face's other art, its scars and eyes, kept. Off takes the man's chest
// tattoo off as well; a pattern keeps it. The man's first step is the face as the disc paints it
// (a character made before keeps its look), the woman's is none (her faces have
// none). It is told only to a client that answered the options' offer 3 (one
// that answered 2 would show a row it does not draw), kept in charlook (part
// "tattoo") and, as every look, told to every client that asked. A PS2 draws the
// face chosen: faces 1 and 3 with their woad.

// ModeTattoo is a tattoo (Tattoo): a client takes the woad off a barbarian face
// and lays a pattern's.
const ModeTattoo uint32 = 4

// The patterns: none (the woad taken off), the face 1's, the face 3's.
const (
	tattooNone  = 0
	tattooBands = 1
	tattooCrest = 2
)

// Tattoo is a ModeTattoo value: the pattern in byte 2, the rest 0 (kept for
// an ink of its own).
func Tattoo(pattern uint32) Recolour {
	return ModeTattoo<<24 | pattern<<16
}

// tattooRowKey is the tattoo's row (charlook part "tattoo").
const tattooRowKey = "tattoo"

// raceBarbarian is the barbarian's race.
const raceBarbarian = 6

// The barbarian bodies' face entries (CHAR.ESF: the man's text slot 7 is
// palette entry 9, the woman's entry 6).
const (
	barbarianFaceMale   = 9
	barbarianFaceFemale = 6
	// the man's chest (text slot 0), whose bare layer bears his chest tattoo
	barbarianChestMale = 5
)

// tattooRows is race's tattoo row, on a server that offers it (world.tattoos):
// the barbarian's alone.
func tattooRows(race int) []OptionRow {
	if race != raceBarbarian || !Creation().Tattoos {
		return nil
	}
	b := gameSkins[raceBarbarian]
	return []OptionRow{{Key: tattooRowKey, Label: "Tattoo", Kind: KindColour,
		Parts: []OptionPart{whole(b[0].body, barbarianFaceMale, barbarianChestMale), whole(b[1].body, barbarianFaceFemale)},
		Male: []OptionStep{
			{"Face's own", 0, 0},
			{"Off", Tattoo(tattooNone), 0},
			{"Bands", Tattoo(tattooBands), 0},
			{"Crest", Tattoo(tattooCrest), 0},
		},
		Female: []OptionStep{
			{"Off", 0, 0},
			{"Bands", Tattoo(tattooBands), 0},
			{"Crest", Tattoo(tattooCrest), 0},
		},
		Answer: 3}}
}

package entity

import "math"

// coachLocationNames is the town each coachman serves, the name a coach menu
// lists it by (F3 20261025-171410: the menus listed "Coachman Droogra" where
// the town belonged). Every coachman of the server's npcs is here but one:
// Coachman Dilga, the only coach in Rathe (world 1), so in no menu. Each was
// placed by where it stands: EQOA-Data's map (github.com/Jadiction/EQOA-Data,
// Map/zones.json's towns, the coach network of Map/coach_map.json) laid over
// Tunaria by a fit to the eight coachmen named before (each within 10 units),
// every coachman then within 120 of its town but Coachman Nioclas (Dark
// Solace, 145) and Coachman Frender (Surefall Glade, 250, outside the glade).
// The names are EQOA-Data's but four: Neriak (its "Neriak, Nektulos"), Fort
// Seriak (its Castle Felstar, aka Fort Seriak), Klik'Anon (the disc spells it
// Klick'Anon; the players do not), and the two Hazinaks, a good one and an
// evil one, told apart as the coach network tells them.
var coachLocationNames = map[string]string{
	"Coachman Billfer":    "Qeynos",
	"Coachman Boblon":     "Bobble-by-Water",
	"Coachman Brody":      "Castle Lightwolf",
	"Coachman Brumley":    "Wyndhaven",
	"Coachman D'Larik":    "Fort Seriak",
	"Coachman Dapkus":     "Blackwater",
	"Coachman Doba":       "Moradhim",
	"Coachman Droogra":    "Hazinak (Evil)",
	"Coachman Dudley":     "Halas",
	"Coachman Farn":       "Muniel's Tea Garden",
	"Coachman Frender":    "Surefall Glade",
	"Coachman Galdah":     "Highbourne",
	"Coachman Huff":       "Rivervale",
	"Coachman Lothi":      "Neriak",
	"Coachman Nioclas":    "Dark Solace",
	"Coachman Norial":     "Honjour",
	"Coachman Opelim":     "Mt. Hatespike",
	"Coachman Quicksteed": "Highpass Hold",
	"Coachman Rizkar":     "Klik'Anon",
	"Coachman Ronks":      "Freeport",
	"Coachman Skeel":      "Zentar's Keep",
	"Coachman Thirtreel":  "Fayspire",
	"Coachman Ungar":      "Gerntar Mines",
	"Coachman Zaris":      "Tethelin",
	"Coachman Zelik":      "Hazinak (Good)",
	"Coachman Zuggug":     "Grobb",
	"Coachwoman Dainei":   "Murnf",
	"Coachwoman Darva":    "Darvar Manor",
	"Coachwoman Dernia":   "Oasis",
	"Coachwoman Holly":    "Forkwatch",
}

// coachTown is a Tunaria town on EQOA-Data's map, where Tunaria has it.
type coachTown struct {
	name string
	x, z float32
}

// coachTowns are Tunaria's coach towns at Tunaria's coordinates (EQOA-Data's
// map by the fit above; Odus's, on the same sheet, left out), for a coachman
// not in coachLocationNames: one a GM adds is named by the town it stands in.
var coachTowns = []coachTown{
	{"Halas", 12963, 4689},
	{"Klik'Anon", 23460, 6488},
	{"Neriak", 24929, 9438},
	{"Zentar's Keep", 5496, 6457},
	{"Fayspire", 19473, 7038},
	{"Tethelin", 18421, 7488},
	{"Bobble-by-Water", 24634, 11861},
	{"Castle Lightwolf", 9281, 6504},
	{"Moradhim", 15352, 9262},
	{"Fort Seriak", 21776, 8390},
	{"Rivervale", 18339, 11350},
	{"Wyndhaven", 4715, 13211},
	{"Murnf", 8929, 10516},
	{"Mt. Hatespike", 13764, 11395},
	{"Freeport", 25270, 15710},
	{"Surefall Glade", 9060, 12978},
	{"Qeynos", 4829, 17154},
	{"Honjour", 9804, 14991},
	{"Darvar Manor", 13606, 14766},
	{"Highpass Hold", 16773, 15363},
	{"Muniel's Tea Garden", 23374, 19638},
	{"Highbourne", 4854, 21477},
	{"Forkwatch", 12538, 18774},
	{"Dark Solace", 12665, 21109},
	{"Hazinak (Good)", 24885, 27594},
	{"Hazinak (Evil)", 25130, 27159},
	{"Blackwater", 15147, 22660},
	{"Oasis", 21120, 25167},
	{"Gerntar Mines", 13038, 27506},
	{"Grobb", 25257, 31226},
	{"Oggok", 10980, 27211},
	{"Kerplunk Outpost", 14815, 29246},
}

// coachTownReach is how far from a town's mark a coachman not in the table
// may stand and still be named for it: the farthest of the table's own (250)
// and some.
const coachTownReach = 400

// TownAt is the Tunaria coach town within coachTownReach of (x, z), the
// nearest, if one is.
func TownAt(world uint8, x, z float32) (string, bool) {
	if world != 0 {
		return "", false
	}
	best, bestD := "", math.MaxFloat64
	for _, t := range coachTowns {
		d := math.Hypot(float64(t.x-x), float64(t.z-z))
		if d < bestD {
			best, bestD = t.name, d
		}
	}
	return best, bestD <= coachTownReach
}

// coachLabel is the name a coachman's town is listed by: its own entry, else
// the town it stands in (TownAt, at its spawn), else its name; ok reports a
// town.
func coachLabel(npc *NPC) (string, bool) {
	if town, ok := coachLocationNames[npc.Name]; ok {
		return town, true
	}
	if town, ok := TownAt(npc.World, npc.SpawnX, npc.SpawnZ); ok {
		return town, true
	}
	return npc.Name, false
}

// CoachTownMark is one of Tunaria's coach towns where it is (coachTowns).
type CoachTownMark struct {
	Name string
	X, Z float32
}

// CoachTownMarks are Tunaria's coach towns, for a map's town names.
func CoachTownMarks() []CoachTownMark {
	out := make([]CoachTownMark, 0, len(coachTowns))
	for _, t := range coachTowns {
		out = append(out, CoachTownMark{Name: t.name, X: t.x, Z: t.z})
	}
	return out
}

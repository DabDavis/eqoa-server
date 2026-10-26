// Code written by tools/gen_coach_routes.py; DO NOT EDIT: change the tool
// and run it again.

package entity

// liveCoachRoutes is the live game's coach network as EQOA-Data maps it
// (Map/coach_map.json): each coach town and the towns its coach ran to, by
// coachtowns.go's names.
var liveCoachRoutes = map[string][]string{
	"Blackwater":                {"Gerntar Mines", "Oasis", "Forkwatch"},
	"Castle Lightwolf":          {"Halas", "Murnf"},
	"Dark Solace":               {"Kerplunk Outpost", "Oasis", "Highpass Hold", "Honjour"},
	"Darvar Manor":              {"Moradhim", "Surefall Glade", "Forkwatch", "Highpass Hold"},
	"Fayspire":                  {"Tethelin", "Klik'Anon", "Rivervale", "Moradhim"},
	"Forkwatch":                 {"Qeynos", "Blackwater", "Highbourne", "Darvar Manor"},
	"Fort Seriak":               {"Neriak", "Highpass Hold", "Mt. Hatespike"},
	"Freeport":                  {"Neriak", "Bobble-by-Water", "Highpass Hold", "Muniel's Tea Garden"},
	"Gerntar Mines":             {"Blackwater"},
	"Grobb":                     {"Hazinak (Evil)", "Kerplunk Outpost"},
	"Halas":                     {"Moradhim", "Castle Lightwolf"},
	"Hazinak (Good)":            {"Oasis"},
	"Highbourne":                {"Qeynos", "Forkwatch"},
	"Highpass Hold":             {"Freeport", "Rivervale", "Dark Solace", "Darvar Manor", "Fort Seriak"},
	"Honjour":                   {"Mt. Hatespike", "Wyndhaven", "Dark Solace"},
	"Kerplunk Outpost":          {"Grobb", "Dark Solace", "Oggok"},
	"Klik'Anon":                 {"Fayspire", "Neriak", "Bobble-by-Water"},
	"Moradhim":                  {"Halas", "Rivervale", "Fayspire", "Darvar Manor"},
	"Mt. Hatespike":             {"Fort Seriak", "Honjour"},
	"Muniel's Tea Garden":       {"Oasis", "Freeport"},
	"Murnf":                     {"Castle Lightwolf", "Wyndhaven"},
	"Oasis":                     {"Dark Solace", "Hazinak (Good)", "Hazinak (Evil)", "Blackwater", "Muniel's Tea Garden"},
	"Oggok":                     {"Dark Solace", "Kerplunk Outpost"},
	"Qeynos":                    {"Wyndhaven", "Forkwatch", "Highbourne", "Surefall Glade"},
	"Rivervale":                 {"Bobble-by-Water", "Moradhim", "Highpass Hold"},
	"Stone of Morathals (Evil)": {"East Plateau"},
	"Stone of Morathals (Good)": {"Arcadin"},
	"Surefall Glade":            {"Wyndhaven", "Darvar Manor", "Qeynos"},
	"Tethelin":                  {"Fayspire"},
	"Wyndhaven":                 {"Surefall Glade", "Murnf", "Qeynos", "Honjour", "Zentar's Keep"},
	"Zentar's Keep":             {"Wyndhaven"},
}

// Code written by tools/gen_camp_teles.py; DO NOT EDIT: change the tool
// and run it again.

package handlers

// campDests are EQOA-Data's XP camps (its map's camps with a note, its
// XP-camp guide), #tele shortcuts anchored on one of each camp's creatures
// in the server's npcs: the one of that name nearest the spawn written here,
// which is where a server without it lands. Levels are the camp's creatures'
// here, not the guide's.
var campDests = map[string]teleDest{
	"dfc":            {NPC: "Deathfist champion", Near: true, X: 21080.5, Y: 54.1, Z: 19078.3, World: 0, Levels: "33-40", Label: "Deathfist Citadel"},
	"alfarak":        {NPC: "Deathfist slayer", Near: true, X: 22377.3, Y: 57.0, Z: 21305.1, World: 0, Levels: "37-40", Label: "Al Farak Ruins' Deathfist slayers and reavers"},
	"hexbones":       {NPC: "hexbone", Near: true, X: 10306.6, Y: 54.1, Z: 8377.7, World: 0, Levels: "43-46", Label: "Zentar's Keep's hexbones and shrieks"},
	"nasehir":        {NPC: "Nasehir Ranger", Near: true, X: 24346.3, Y: 54.1, Z: 16213.8, World: 0, Levels: "11-17", Label: "Northern Ro's Nasehir, the first camp"},
	"nasehir2":       {NPC: "Nasehir Ranger", Near: true, X: 24539.3, Y: 57.7, Z: 17137.9, World: 0, Levels: "13-23", Label: "Northern Ro's Nasehir, the second camp"},
	"hatebone":       {NPC: "hatebone guard", Near: true, X: 19957.6, Y: 54.1, Z: 7422.3, World: 0, Levels: "1-5", Label: "Halas' Hatebone orcs"},
	"hatebone2":      {NPC: "hatebone orc", Near: true, X: 19946.4, Y: 53.3, Z: 7943.2, World: 0, Levels: "3-8", Label: "Halas' Hatebone orcs, the second camp"},
	"hatebonecave":   {NPC: "hatebone oracle", Near: true, X: 19840.2, Y: 49.3, Z: 7727.4, World: 0, Levels: "3-12", Label: "Halas' Hatebone orc cave"},
	"nroundead":      {NPC: "grave rat", Near: true, X: 24123.5, Y: 54.1, Z: 16547.0, World: 0, Levels: "15-20", Label: "Northern Ro's undead"},
	"nroundead2":     {NPC: "Decaying Zombie", Near: true, X: 24329.7, Y: 54.1, Z: 17688.8, World: 0, Levels: "18-20", Label: "Northern Ro's undead, the second camp"},
	"gharial":        {NPC: "Gharial ", Near: true, X: 23325.2, Y: 55.1, Z: 20427.6, World: 0, Levels: "31-35", Label: "the gharial island south of Moradhim"},
	"tendora":        {NPC: "kerran warrior", Near: true, X: 5649.0, Y: 56.3, Z: 4936.1, World: 2, Levels: "45-55", Label: "Tendora-Buki, West Toxulia's kerran village"},
	"kappa":          {NPC: "kappa slave", Near: true, X: 4907.8, Y: 50.3, Z: 2298.7, World: 2, Levels: "45-52", Label: "Slythis Dwell's kappa"},
	"kappawarriors":  {NPC: "kappa warrior", Near: true, X: 4886.8, Y: 57.7, Z: 3786.7, World: 2, Levels: "45", Label: "Slythis Dwell's kappa warriors"},
	"kappaelites":    {NPC: "kappa elite guard", Near: true, X: 4877.1, Y: 73.0, Z: 2346.1, World: 2, Levels: "58", Label: "Slythis Dwell's kappa elites"},
	"drudges":        {NPC: "kappa drudge", Near: true, X: 6198.5, Y: 81.7, Z: 2776.5, World: 2, Levels: "41", Label: "the East Plateau's kappa drudges"},
	"phantoms":       {NPC: "phantom", Near: true, X: 7057.9, Y: 67.6, Z: 3366.3, World: 2, Levels: "50", Label: "the East Plateau's phantoms"},
	"gargoyles":      {NPC: "leering gargoyle", Near: true, X: 10209.5, Y: 98.3, Z: 8966.7, World: 2, Levels: "50", Label: "the South Barren Coast's gargoyles"},
	"harpies":        {NPC: "shiran harpy", Near: true, X: 11764.5, Y: 55.9, Z: 2293.8, World: 2, Levels: "58-63", Label: "Erud's Crossing's harpies"},
	"moots":          {NPC: "guardian of saftemoot", Near: true, X: 10789.3, Y: 91.0, Z: 3335.1, World: 2, Levels: "58", Label: "Erud's Crossing's moots"},
	"blackburrow":    {NPC: "gnoll whitepaw", Near: true, X: 12913.1, Y: -21.0, Z: 12611.0, World: 0, Levels: "22-32", Label: "Blackburrow's gnolls"},
	"bloodfoots":     {NPC: "Bloodfoot bandit", Near: true, X: 12371.0, Y: 54.4, Z: 11155.9, World: 0, Levels: "15-30", Label: "Mt. Hatespike's Bloodfoots"},
	"coldpaw":        {NPC: "coldpaw brawler", Near: true, X: 22462.1, Y: 50.7, Z: 6734.0, World: 0, Levels: "6-10", Label: "Klik'Anon's Coldpaw gnolls"},
	"hillgiants":     {NPC: "Hill giant", Near: true, X: 13711.6, Y: 54.1, Z: 24281.0, World: 0, Levels: "45-48", Label: "Keliner's hill giants"},
	"serpents":       {NPC: "serpent guardian", Near: true, X: 15644.7, Y: 60.2, Z: 15129.5, World: 0, Levels: "28-30", Label: "the Bandit Hills' serpent guardians"},
	"cutthroats":     {NPC: "cutthroat", Near: true, X: 23685.0, Y: 50.0, Z: 13593.2, World: 0, Levels: "18-19", Label: "Mulin's Reach's cutthroats and bandits"},
	"cutthroats2":    {NPC: "cutthroat", Near: true, X: 22377.4, Y: 57.7, Z: 13533.7, World: 0, Levels: "18-19", Label: "Mulin's Reach's cutthroats and bandits, the second camp"},
	"plainstriders":  {NPC: "orc plainstrider", Near: true, X: 23304.3, Y: 53.0, Z: 13235.3, World: 0, Levels: "20-21", Label: "Mulin's Reach's orc plainstriders"},
	"plainsraiders":  {NPC: "orc plainstrider", Near: true, X: 23208.4, Y: 60.0, Z: 12257.3, World: 0, Levels: "20", Label: "Mulin's Reach's orc plainsraiders"},
	"cracktooth":     {NPC: "cracktooth matron", Near: true, X: 24997.8, Y: 59.3, Z: 12411.4, World: 0, Levels: "13-16", Label: "Bobble-by-Water's Cracktooth orcs"},
	"cadavers":       {NPC: "druidic cadaver", Near: true, X: 4309.2, Y: 59.4, Z: 15805.2, World: 0, Levels: "9", Label: "the druidic cadavers west of Qeynos"},
	"slesher":        {NPC: "reanimated gnoll", Near: true, X: 9621.5, Y: 84.6, Z: 13734.5, World: 0, Levels: "15-17", Label: "Surefall Glade's Slesher gnolls"},
	"aviaks":         {NPC: "aviak hatcher", Near: true, X: 14549.2, Y: 140.2, Z: 5296.2, World: 0, Levels: "17-19", Label: "the aviak outpost near Halas"},
	"orctunnel":      {NPC: "orc brute", Near: true, X: 18959.4, Y: -39.0, Z: 9031.0, World: 0, Levels: "12-19", Label: "the Druk's orc tunnel"},
	"gnollfishers":   {NPC: "gnoll fisherman", Near: true, X: 13445.3, Y: 51.0, Z: 13030.1, World: 0, Levels: "25", Label: "the gnoll fishermen"},
	"scarecrows":     {NPC: "Scarecrow", Near: true, X: 12291.4, Y: 58.0, Z: 17005.3, World: 0, Levels: "22-29", Label: "Alseop's Wall's scarecrows"},
	"goblincitadel":  {NPC: "Goblin Excavator", Near: true, X: 16250.8, Y: 50.4, Z: 12219.3, World: 0, Levels: "15-28", Label: "the Goblin Citadel near Runneye"},
	"klickpirates":   {NPC: "deckhand", Near: true, X: 25454.5, Y: 54.1, Z: 6579.2, World: 0, Levels: "14-20", Label: "the Klik'Anon coast's pirates"},
	"cyclops":        {NPC: "cyclops guard", Near: true, X: 11445.0, Y: 80.4, Z: 21368.4, World: 0, Levels: "44-47", Label: "the cyclops"},
	"siloths":        {NPC: "Siloth knight", Near: true, X: 14358.4, Y: 97.0, Z: 18446.7, World: 0, Levels: "39-41", Label: "the Salt Mine's siloths"},
	"icegiants":      {NPC: "Ice giant guard", Near: true, X: 4861.5, Y: 61.6, Z: 4617.2, World: 0, Levels: "45-53", Label: "the ice giant refugees near Northgate"},
	"ghosts":         {NPC: "wandering ghost", Near: true, X: 15548.9, Y: 54.1, Z: 20787.5, World: 0, Levels: "60-61", Label: "the wandering ghosts"},
	"xrag":           {NPC: "child of Xrag", Near: true, X: 15000.4, Y: 21.7, Z: 27406.8, World: 0, Levels: "61-66", Label: "the Elephant Graveyard's children of Xrag"},
	"tombrats":       {NPC: "tomb rat", Near: true, X: 17767.0, Y: 57.8, Z: 8183.4, World: 0, Levels: "6-8", Label: "the tomb rats"},
	"gusts":          {NPC: "freezing gust", Near: true, X: 15734.1, Y: 60.1, Z: 6740.6, World: 0, Levels: "13-16", Label: "the freezing gusts"},
	"guardianforest": {NPC: "Goblin Assassin", Near: true, X: 10182.1, Y: 75.3, Z: 6973.6, World: 0, Levels: "35-60", Label: "Guardian Forest's goblins"},
	"freezeblood":    {NPC: "Freezeblood berserker", Near: true, X: 13435.0, Y: 54.7, Z: 6587.1, World: 0, Levels: "13-19", Label: "Everfrost's Freezeblood goblins"},
	"crocodiles":     {NPC: "young crocodile", Near: true, X: 25751.5, Y: 54.1, Z: 11072.0, World: 0, Levels: "19-20", Label: "Bobble-by-Water's saltwater crocodiles"},
	"minotaurs":      {NPC: "Rustel Minotaur", Near: true, X: 13002.0, Y: 59.0, Z: 26869.2, World: 0, Levels: "40-43", Label: "the Gerntar Mines' Rustel minotaurs"},
}

// campAliases are other names a GM may type for a camp.
var campAliases = map[string]string{
	"deathfist": "dfc",
	"shrieks":   "hexbones",
	"halasorcs": "hatebone",
	"kerrans":   "tendora",
	"slythis":   "kappa",
	"bbgnolls":  "blackburrow",
	"bandits":   "cutthroats",
	"druk":      "orctunnel",
	"pariah":    "xrag",
	"icywind":   "gusts",
}

package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A waysender sends a player to another town for a fee (F3 20261025-173746:
// Waysender Brindel's trip to Lavastorm, for level 55 and up). The routes are
// the ones EQOA-Data's quest guides (github.com/Jadiction/EQOA-Data, Quests/)
// document, each landing just in front of the NPC the guides meet there: the
// other town's waysender, else its coachman, else (Lavastorm) the outpost a
// level-53 guide walks to. Fees are the guides' where they give one (Neriak to
// Freeport and back 100 tunar, Neriak to Grobb 225), else none. A waysender
// with no route here answers nothing, as any NPC with nothing to say. A trip a
// quest in the game unlocks (questtrips.go, table questunlocks) is offered only
// to a player who has finished it.

// wayRoute is one of a waysender's trips.
type wayRoute struct {
	// To is the place, as the menu lists it.
	To string
	// Arrive is the NPC whose spot the player lands in front of; At, when
	// set, is the place itself instead (a point the guides give, no NPC's).
	Arrive string
	At     *wayPlace
	// Fee is its tunar, MinLevel the level it needs (0: none).
	Fee, MinLevel int
	// QuestOnly is a trip only a quest opens (questtrips.go): with no quest
	// in the game unlocking it, it is offered to no one who has not finished
	// one that did. Every other trip is open until a quest in the game gates
	// it.
	QuestOnly bool
	// Vampire is a trip only a vampire is offered (a coffin's: entity/
	// vampire.go); to anyone else the coffin is a coffin and says nothing.
	Vampire bool
}

// wayPlace is where a trip lands: a world and a point in it.
type wayPlace struct {
	World   uint8
	X, Y, Z float32
	// Facing is the heading a player lands with (radians, 0 facing +z);
	// 0 unless the place says.
	Facing float32
}

// podArrival is where a trip to the Plane of Disease lands (the owner,
// 2026-11-13: "where dust is now is where the dead hills PoD portal ports
// to"): Dust's spot then, facing as he did; until then in front of the Portal
// to the Dead Hills.
var podArrival = &wayPlace{World: 5, X: 4644.3, Y: -11.8, Z: 3818.7, Facing: 3.117049}

// wayRoutes are each waysender's trips, by name; the guides' words beside.
var wayRoutes = map[string][]wayRoute{
	// "Talk to Waysender T'Thox, who is outside of the Blind Fish Tavern (you
	// will need 100 tunar to get to Freeport ...)" (neriak-nektulos/enchanter);
	// "Waysender Thi'thox ports you to Grobb" (neriak-nektulos/necromancer);
	// "To return to Grobb, pay 225 tunar to the Dark Elf Waysender"
	// (grobb/warrior)
	"Waysender Ti`Thox": {
		{To: "Freeport", Arrive: "Waysender Abodai", Fee: 100},
		{To: "Grobb", Arrive: "Waysender Zapgit", Fee: 225},
	},
	// "hire Waysender Abodai to teleport you back to Neriak" (... and another
	// 100 tunar to hire the waysender to get back)
	"Waysender Abodai": {
		{To: "Neriak", Arrive: "Waysender Ti`Thox", Fee: 100},
	},
	// "sends you to Scornblade D'Eleth in Neriak via Waysender Zapit";
	// "Waysender Zapgit for a trip to Honjour" (grobb/shadowknight, shaman)
	"Waysender Zapgit": {
		{To: "Neriak", Arrive: "Waysender Ti`Thox"},
		{To: "Honjour", Arrive: "Coachman Norial"},
	},
	// "Waysender Glowbap who will teleport you to Rivervale" (klick-anon/wizard)
	"Waysender Glowbap": {
		{To: "Rivervale", Arrive: "Waysender Windfoot"},
	},
	// "Waysender Windfoot to teleport back to Klick'Anon" (klick-anon/wizard)
	"Waysender Windfoot": {
		{To: "Klik'Anon", Arrive: "Waysender Glowbap"},
	},
	// "speak with Waysender Carowyn in Fayspires ... Carowyn sends you to
	// Moradhim (sign the coach)" (klick-anon/magician)
	"Waysender Carowyn": {
		{To: "Moradhim", Arrive: "Coachman Doba"},
	},
	// "Go to Waysender Naelarie, near the well. Once in Surefall Glade, sign
	// the coach"; "To get to Rivervale, see Waysender Naelarie in Qeynos"
	// (qeynos/bard)
	"Waysender Naelarie": {
		{To: "Surefall Glade", Arrive: "Coachman Frender"},
		{To: "Rivervale", Arrive: "Waysender Windfoot"},
	},
	// "coach to Klick'Anon, from coach, run out the west gate, follow
	// mountains to the west. Soon you will see a shelter of sorts; there is a
	// waysender there, talk to him. Minastickie is in a hut to your right"
	// (53-59/53); for level 55 and up (F3 20261025-173746). Lavastorm is
	// world 3, the outpost its gnomes' camp.
	"Waysender Brindel": {
		{To: "Lavastorm", Arrive: "Explorer Springa", MinLevel: 55},
	},

	// The boats (petition 1034, Zaeire: "Dockmasters in FP arent giving an
	// option to visit either destination they offer. Bogman/hazinak nor the
	// arc/som"): each dockmaster's harbours as EQOA-Data's map marks them
	// (Map/zones.json: "Dockmaster Yarnning to Arc SoM", "Dockmaster Endol to
	// Haz HB BV", "Dockmaster D'Eleth to FP HB BV", "Dockmaster Thinda to FP
	// Arc", "Dockmaster Killeron to FP HB", "Dockmaster Bibbin to FP bv",
	// "Dockmaster Geben to SoM"), each landing in front of the dockmaster of
	// the harbour reached: Freeport's Odus boat Yarnning's, its other one
	// Endol's ("Endol is at the end of the dock you teleport to. He teleports
	// you to the Hazinak Outpost", neriak-nektulos/cleric; "using Dockmaster
	// D'Eleth from Hazniak to Freeport", neriak-nektulos/warrior). Sandstorm's
	// Bibbin offered "Bogman Village." and "Freeport." (pcaps/sandstorm-
	// warp.pcap). Arcadin and the Stone of Morathals are Odus's (world 2),
	// the rest Tunaria's. No fee: no guide gives one.
	"Dockmaster Yarnning": {
		{To: "Arcadin", Arrive: "Dockmaster Killeron"},
		{To: "Stone of Morathals", Arrive: "Dockmaster Bibbin"},
	},
	"Dockmaster Endol": {
		{To: "Hazinak", Arrive: "Dockmaster D'Eleth"},
		{To: "Highbourne", Arrive: "Dockmaster Thinda"},
		{To: "Bogman Village", Arrive: "Dockmaster Geben"},
	},
	"Dockmaster D'Eleth": {
		{To: "Freeport", Arrive: "Dockmaster Endol"},
		{To: "Highbourne", Arrive: "Dockmaster Thinda"},
		{To: "Bogman Village", Arrive: "Dockmaster Geben"},
	},
	"Dockmaster Thinda": {
		{To: "Freeport", Arrive: "Dockmaster Endol"},
		{To: "Arcadin", Arrive: "Dockmaster Killeron"},
	},
	"Dockmaster Killeron": {
		{To: "Freeport", Arrive: "Dockmaster Yarnning"},
		{To: "Highbourne", Arrive: "Dockmaster Thinda"},
	},
	"Dockmaster Bibbin": {
		{To: "Freeport", Arrive: "Dockmaster Yarnning"},
		{To: "Bogman Village", Arrive: "Dockmaster Geben"},
	},
	"Dockmaster Geben": {
		{To: "Stone of Morathals", Arrive: "Dockmaster Bibbin"},
	},

	// The Black Market and the Stone of Morathals (the owner, 2026-11-11:
	// "Waysender Mazaran should be the evil waysender back to Stone of Morthalis
	// and vice versa to their Waysender", "mazaran location is called Black
	// Market"): Mazaran (Odus, among the Jextalan shopkeepers) and the Stone's
	// evil waysender, Iskan, who stands with the Partisans of Morthalis on the
	// town's evil side (EQOA-Data's map marks the evil Stone west and south of
	// the good; Kurah stands out among its hogs). No fee: none was asked.
	"Waysender Mazaran": {
		{To: "Stone of Morathals", Arrive: "Waysender Iskan"},
	},
	"Waysender Iskan": {
		{To: "the Black Market", Arrive: "Waysender Mazaran"},
	},
	// Arcadin and the Stone's good side (petition 1541, Neo: "talking to
	// Waysender Yuresh does not port you to stone of morathalis"): Yuresh
	// stands by Arcadin's docks (Dockmaster Killeron's), and Arcadin is the
	// good Stone's town (the coaches' routes, entity/coachroutes.go: "Stone
	// of Morathals (Good)" to Arcadin), so he lands on the good Stone's
	// waysender, Kurah, among its hogs, and Kurah sends back to him. Free,
	// as Mazaran's and Iskan's.
	"Waysender Yuresh": {
		{To: "Stone of Morathals", Arrive: "Waysender Kurah"},
	},
	"Waysender Kurah": {
		{To: "Arcadin", Arrive: "Waysender Yuresh"},
	},

	// Between Tunaria and the Rathe (the user, 2026-11-07: "Gatekeeper Furryfoot
	// needs to send the player to rathe", "also door in the cyclops wall"; a
	// player: "can we get gatekeeper to oggok area?", "door in cyclops wall also
	// ports there"). EQOA-Data's map marks "Gatekeeper Furyfoot to SP" at Gerntar
	// Mines and "Gatekeeper Kuggin to GM" in the Rathe; "Door to Urglunt's Gate"
	// in the wall of Tunaria's cyclops and "Door to Urglunt's Wall" in the Rathe's.
	// Kuggin takes Furyfoot's trip back from his own list (entity's warpOfferedBy).
	// A door stands in its wall: each lands four units out on the open side of the
	// other, read off the collision (south of Tunaria's, north of the Rathe's).
	// The same again by Oggok (the user: "yes add threm and the moggok door too"):
	// Gatekeeper Threm and "Door to Moggok's Gate" at the south end of his road,
	// Gatekeeper Wavar and "Door to Oggok Gate" in the Rathe; no sighting holds
	// Tunaria's door, so it stands where the map's mark and the wall agree.
	"Gatekeeper Furyfoot": {
		{To: "the Rathe", Arrive: "Gatekeeper Kuggin"},
	},
	"Door to Urglunt's Gate": {
		{To: "Urglunt's Gate", At: &wayPlace{World: 1, X: 9053.8, Y: 52.6, Z: 2110.1}},
	},
	"Door to Urglunt's Wall": {
		{To: "Urglunt's Wall", At: &wayPlace{World: 0, X: 11419.9, Y: 80.1, Z: 21404}},
	},
	"Gatekeeper Threm": {
		{To: "the Rathe", Arrive: "Gatekeeper Wavar"},
	},
	"Door to Moggok's Gate": {
		{To: "Moggok's Gate", At: &wayPlace{World: 1, X: 9825.9, Y: 57.3, Z: 9785}},
	},
	"Door to Oggok Gate": {
		{To: "Oggok Gate", At: &wayPlace{World: 0, X: 12571, Y: 60, Z: 29520.8}},
	},

	// The Archons' towers in Geomancer's Citadel (ticket 1196, Larrybird: "each
	// side fire and earth should have two different portals each one to the
	// archon and the other too the primael boss"). The guide (Misc/archon-aereth-
	// archon-vetash-and-primeval-elements): "The Fire Primeval is located in the
	// north tower, the Earth Primeval is located in the south tower. In both
	// cases, use the left portal if you wish to port up. The right portal takes
	// you to the archon." The portals stand at the back of each tower's ground
	// floor, each trip landing at the far side of its room from what waits there:
	// an Archon on the top floor, a Primeval on the middle one.
	"Portal to Archon Aereth": {
		{To: "Archon Aereth", At: &wayPlace{World: 1, X: 4702, Y: 110.5, Z: 2492}},
	},
	"Earth Primeval Portal": {
		{To: "the Earth Primeval", At: &wayPlace{World: 1, X: 4700, Y: 84, Z: 2496}},
	},
	"Portal to Archon Vetash": {
		{To: "Archon Vetash", At: &wayPlace{World: 1, X: 4700, Y: 106.5, Z: 2357}},
	},
	"Fire Primeval Portal": {
		{To: "the Fire Primeval", At: &wayPlace{World: 1, X: 4700, Y: 85, Z: 2361}},
	},

	// The Plane of Disease (F3 pass 17, "unable to travel to PoD"), the
	// Secrets world's "Last Home" (world 5): "Log the quest by speaking to
	// Necrologist Zizclank and his assistant Necroloigst Xeptip. Then you want
	// to enter the mausoleum in the middle" (portal/plane-of-disease, "Dead
	// Hills (NW)", the Rathe's: world 1). The live game's flag quest is not
	// in, so Zizclank sends anyone of the quest's level, 50, to the portal
	// players arrive at, which sends them back. tools/seed_pod.py places
	// these NPCs.
	"Necrologist Zizclank": {
		{To: "the Plane of Disease", At: podArrival, MinLevel: 50},
	},
	"Mausoleum Portal": {
		// The portal itself (ticket 1197): the guide's flag lets a player "port
		// there anytime", and this talked only on The Last House's own first step.
		// The same trip as Zizclank's, opened by the same quest (questunlocks).
		{To: "the Plane of Disease", At: podArrival, MinLevel: 50},
	},
	"Portal to the Dead Hills": {
		{To: "the Dead Hills", Arrive: "Necrologist Zizclank"},
	},
	// The First Seal (petition 1534, Fullmetal: "Spirit Takers Portal is not
	// responding and will not port to Caretaker"): the guide's black orb
	// portal in Spirit Talkers Woods, to the Heyokah Burial Ground, landing
	// in front of the Caretaker; open to anyone (the quest's amulet and the
	// ground's damage are not in, 30-40-49.json's notes). No guide gives a
	// way back: it is a walk, or Return Home.
	"Spirit Talker's Portal": {
		{To: "the Heyokah Burial Ground", Arrive: "The Caretaker"},
	},
	// Inside PoD: Queen Qy'tha "inside whirling portal within the lake in
	// entrance area" ("be ported to the wasp hive"), and the Twins and
	// Bertoxxulous through the Bloody Portal ("you can use Noroxus Blood to
	// open the Bloody Portal and pass through to the twins"): the guide's
	// Closed Portal, "all the way around the circle past Honcho Ulgurglch"
	// (the ring's south-east arc, at the slime fall), to the middle valley's
	// pad, where the Portal of the Twins stands (F3 pass 21: the twins and
	// Bert were in the north-west lake's lobe, Noroxus's); live's quests
	// open them, here level 60.
	"Whirling Portal": {
		{To: "the wasp hive", Arrive: "Portal to the Lake", MinLevel: 60},
	},
	"Portal to the Lake": {
		{To: "the lake", Arrive: "Whirling Portal"},
	},
	// The Bloody Portal itself is closed (the guide's Closed Portal; the user,
	// 2026-11-08, the twins' thread: "Open it only when the raid is assembled,
	// since it stays open about 15 minutes. It respawns randomly every 45
	// minutes to 3 hours"): Noroxus's blood shown to it (quest "The Bloody
	// Portal") fires the trigger that stands the Open Bloody Portal beside it
	// for 15 minutes, then keeps it shut (tools/guidegaps/pod-twins.json).
	"Open Bloody Portal": {
		{To: "the Twins' valley", Arrive: "Portal of the Twins", MinLevel: 60},
	},
	"Portal of the Twins": {
		{To: "the Bloody Portal", Arrive: "Bloody Portal"},
	},
	// Noroxus "Take the combined item to the big cauldron in the middle of
	// all 3 pools to open fuming portal ... This leads to Noroxus" (F3 pass
	// 21, jib: he stood at "his portal entrance, not his arena"): the
	// Fuming Portal at the cauldron, to his lair, the north-west lake's lobe
	// (eqoa.live's wiki map); live's slime hearts open it, here level 60.
	"Fuming Portal": {
		{To: "Noroxus's lair", Arrive: "Portal to the Cauldron", MinLevel: 60},
	},
	"Portal to the Cauldron": {
		{To: "the cauldron", Arrive: "Fuming Portal"},
	},

	// The Portal guides' trips (tools/quests/data/portal.json; the places the
	// Portal lane read off its guides and the ground: notes qe.md), each only a
	// quest opens (QuestOnly: the quest that unlocks it). No guide gives a
	// way back from any of them (Return Home, a gate or the bind do), so none
	// is wired.
	// "talk to the Sky Portal at the top of the Endless Tower": the Plane of
	// Sky (world 4), beside Sartheret
	"Sky Portal": {
		{To: "the Plane of Sky", At: &wayPlace{World: 4, X: 4834.8, Y: 250.1, Z: 5524.7}, QuestOnly: true},
	},
	// "pay him the 10k (which you have to pay each and every time)"
	// (isle-of-dread). The owner, 2026-11-11: "Norach ... on the little island
	// with a palm tree. he will teleport you to Isle of Dread onto the little
	// raft for 10k"; then "Keep the medallion gate": still only for those who
	// finished The Medallion of the Faceless, 10,000 tunar, landing on the deck of
	// the fishing raft moored in the cove south of Graumord's pier (the deck's
	// top 52.44 off the walls file, the water's 51.36), facing the pier.
	"Norach": {
		{To: "the Isle of Dread", At: &wayPlace{World: 0, X: 24578, Y: 52.6, Z: 3626, Facing: 3.9}, Fee: 10000, QuestOnly: true},
	},
	// "With the five Ruins and 5,000 tunar you can talk to portal NPC and be
	// ported to Zaoran Plateau" (questmasters1.com's Zaoran Plateau page, 2006-07,
	// the owner's guide of 2026-11-11, over EQOA-Data's zaoran-plateau "Bring 1
	// tunar for passage each time!"): 5,000 a trip; the five runes are the
	// quest's that opens it (The Way to Zaoran Plateau). The plateau is zone 5 of
	// the Secrets world (5), at the ZP1 marker.
	"portal guardian": {
		{To: "Zaoran Plateau", At: &wayPlace{World: 5, X: 4899, Y: 148, Z: 7049}, Fee: 5000, QuestOnly: true},
	},
	// the Fire Giant Magus subdued sends raiders to Lord Nagafen's island
	// (soluseks-eye), by the Nagafen Teleport Arrive marker in Lavastorm
	"Subdued Magus": {
		{To: "Lord Nagafen's lair", Arrive: "Nagafen Teleport Arrive", QuestOnly: true},
	},
	// the Plane of Sky's isles (plane-of-sky): each essence's key, or the
	// Mark of the Majestic's, the passage to its isle
	"essence of rain": {
		{To: "Rain 1", At: &wayPlace{World: 4, X: 4228.2, Y: 233.8, Z: 5695.7}, QuestOnly: true},
		// the third isle's lower hut, its armourers' (a set is six hand-ins):
		// no guide's trip, only the chain's second quest, which brings a
		// player there in the first place and needs the first, opens it
		{To: "Rain 3", At: &wayPlace{World: 4, X: 4614.2, Y: 112.97, Z: 4936.5}, QuestOnly: true},
	},
	"essence of sun": {
		{To: "Sun 1", At: &wayPlace{World: 4, X: 5607, Y: 235.1, Z: 5360.3}, QuestOnly: true},
	},
	"essence of night": {
		{To: "Night 1", At: &wayPlace{World: 4, X: 4161, Y: 612.4, Z: 6384}, QuestOnly: true},
	},
	"Mark of the Majestic": {
		{To: "Snow 1", At: &wayPlace{World: 4, X: 5670.8, Y: 552.9, Z: 6358.8}, QuestOnly: true},
	},
	// "For those flaggged, to get to deso 1, port up to the main PoS isle ...
	// then pass through to the Isle of Wonder. You speak to an npc up there, and
	// then you'll be ported off to Deso 1" (plane-of-sky, desolation): the Mark
	// of Desolation, for those who finished the chain of snow (questunlocks:
	// tools/sky_keys_20261111.py), to the north end of the first isle, clear of
	// the erych (the isle's essence, its way on, stands at the south end:
	// skypass.go)
	"Mark of Desolation": {
		{To: "Desolation 1", At: &wayPlace{World: 4, X: 4763.5, Y: 565.5, Z: 6038.5, Facing: 6.25}, QuestOnly: true},
	},
	// A raid's portals (tools/guidegaps/): each stands only while a trigger's run has
	// spawned it, so no quest gate. "There's Rallos", through the portal the Mark of
	// Rallos wakes (zaoran-plateau): the raid first; the isle's own pad.
	"Portal of Rallos": {
		{To: "Rallos Zek", At: &wayPlace{World: 5, X: 4356.4, Y: 183.0, Z: 6720}, MinLevel: 60},
	},
	// "you recieve Razeal Raid Teleporter. Port up and own." (zaoran-plateau): the isle
	// whose south side the raid sets up on
	"Razeal Raid Teleporter": {
		{To: "Razeal's isle", At: &wayPlace{World: 5, X: 5334.7, Y: 336.3, Z: 7590}, MinLevel: 60},
	},

	// The vampires' coffins (entity/vampire.go): EQOA-Data's vampire guide's
	// Sanctuary "allows porting between connected coffin locations", nine of
	// them: Freeport (SE), Guardian Forest (C), Vastly Deep (C), Solusek's Eye
	// (SE), Sslathis (S), Spider Mines (E), Collinridge Cemetery (C), West
	// Feerrott (C), Zaoran Plateau (NW). Three stand in this server's data
	// as Return Home placed them: the coffin in Freeport's south-east, the one
	// in the middle of Collonridge Cemetery and the Lavastorm's Coffin1 (its
	// south-east, by Solusek's Eye); each takes a vampire to the Sanctuary,
	// and the Sanctuary's own (tools/seed_vampires.py) to each of them. A
	// trip lands three steps off its coffin, on the floor, clear of walls.
	"coffin": {
		{To: "The Sanctuary", At: &sanctuaryLanding, Vampire: true},
	},
	"Coffin1": {
		{To: "The Sanctuary", At: &sanctuaryLanding, Vampire: true},
	},
	"Sanctuary coffin": {
		{To: "Freeport", At: &wayPlace{World: 0, X: 25519.8, Y: 54.09, Z: 15529.8}, Vampire: true},
		{To: "Collonridge Cemetery", At: &wayPlace{World: 0, X: 22841.9, Y: 54.09, Z: 8844.9}, Vampire: true},
		{To: "Solusek's Eye", At: &wayPlace{World: 3, X: 5704.62, Y: 75.25, Z: 5932.4}, Vampire: true},
	},
}

// sanctuaryLanding is where the vampires' Sanctuary is reached: Sandstorm's
// landing for its Sanctuary spell (claude-notes/sanctuary-memcard-patch-
// 20261025.md: S2's world 7 at (4322.19, 274.134, 6179.78), the Frontiers
// disc's world 5 file), on the floor there, under 48 of rock (ground.Roof).
var sanctuaryLanding = wayPlace{World: 5, X: 4322.19, Y: 273.62, Z: 6179.78}

// The waysender's words. A trip is asked as Sandstorm's dockmasters ask one
// (pcaps/sandstorm-warp.pcap: "Where do you wish to go?", then "Very
// well."); one place alone as Yes or No.
const (
	wayAskMany = "Where do you wish to go?"
	wayGo      = "Very well."
)

// IsWaysender is whether the NPC of that name is a waysender: one so named,
// or one with trips here (a portal, the Plane of Disease's necrologist).
func IsWaysender(name string) bool {
	return strings.HasPrefix(name, "Waysender ") || len(wayRoutes[name]) > 0
}

// wayAskOne is the question for a waysender's one trip.
func wayAskOne(d entity.CoachDest) string {
	if d.Fee > 0 {
		return fmt.Sprintf("Do you wish to travel to %s for %d tunar?", d.Name, d.Fee)
	}
	return fmt.Sprintf("Do you wish to travel to %s?", d.Name)
}

// tooWeak is a waysender's answer to a player below every trip's level.
func tooWeak(place string) string {
	return fmt.Sprintf("You are too weak to travel to %s. Come back when you are stronger.", place)
}

// notUnlocked is a travel NPC's answer to a player its every other trip is
// closed to and a quest keeps from this one (questtrips.go; no recording
// has one: ours).
func notUnlocked(place, quest string) string {
	if quest == "" {
		return fmt.Sprintf("You may not travel to %s yet.", place)
	}
	return fmt.Sprintf("You may not travel to %s until you have finished %s.", place, quest)
}

// lockedTrip is a trip a quest keeps the player from: the place and the
// quest to finish.
type lockedTrip struct{ place, quest string }

// wayDests are the waysender's trips this session's player can take: each
// landing on its NPC's spot, in a world the game loads; the ones its level
// allows and no quest keeps it from (TripBarred); the first place a level
// kept it from (or "" if none did) and the first a quest did.
func wayDests(gw *entity.World, sess *session.Session, waysender string, player *entity.Player) (dests []entity.CoachDest, barred string, locked lockedTrip) {
	for _, r := range wayRoutes[waysender] {
		var x, y, z, facing float32
		var world uint8
		if r.At != nil {
			x, y, z, world, facing = r.At.X, r.At.Y, r.At.Z, r.At.World, r.At.Facing
		} else {
			var ok bool
			if x, y, z, facing, world, ok = gw.SpawnOfNPC(r.Arrive); !ok {
				log.Printf("Waysender: %s's trip to %s lands on %s, who is not here", waysender, r.To, r.Arrive)
				continue
			}
		}
		if !CanLoadWorld(sess, world) {
			continue
		}
		if r.Vampire && !gw.VampireOf(sess) {
			continue // a coffin opens for vampires alone
		}
		if r.MinLevel > 0 && player.Level < r.MinLevel {
			if barred == "" {
				barred = r.To
			}
			continue
		}
		if quest, no := Quests().TripBarred(player, waysender, r.To); no {
			if locked.place == "" {
				locked = lockedTrip{place: r.To, quest: quest}
			}
			continue
		}
		// in front of them, not inside them: a body is solid (a place: on it);
		// on dry ground by them where in front is the sea (petition 1349: the
		// dockmasters face the harbour from their piers' ends)
		fx, fz, face := x, z, facing
		if r.At == nil {
			fx, fz, face = inFrontOf(x, z, facing)
			if lx, ly, lz, lf, ok := gw.ArrivalLanding(world, x, y, z, facing, bindReach); ok {
				fx, y, fz, face = lx, ly, lz, lf
			}
		}
		dests = append(dests, entity.CoachDest{Name: r.To, X: fx, Y: y, Z: fz, Facing: face,
			World: world, Fee: r.Fee, MinLevel: r.MinLevel, Via: waysender})
	}
	return dests, barred, locked
}

// destLabel is a trip as a menu of several lists it: the place, and its fee.
func destLabel(d entity.CoachDest) string {
	if d.Fee > 0 {
		return fmt.Sprintf("%s (%d tunar)", d.Name, d.Fee)
	}
	return d.Name
}

// offerWay opens a waysender's trips, or says the player is too weak for
// them or has a quest to finish first; it reports whether the waysender had
// anything to say. A trip a quest keeps the player from is left off the list
// of the others, as one its level does.
func (h *InteractHandler) offerWay(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	dests, barred, locked := wayDests(h.gameWorld, sess, npc.Name, player)
	if len(dests) == 0 {
		if barred == "" && locked.place == "" {
			return false
		}
		forgetDialogs(player)
		if barred != "" {
			h.sendDialogueBox(sess, tooWeak(barred))
			log.Printf("Waysender: %s is too weak (%d) for %s's trip to %s", player.Name, player.Level, npc.Name, barred)
		}
		if locked.place != "" {
			h.sendDialogueBox(sess, notUnlocked(locked.place, locked.quest))
			log.Printf("Waysender: %s has not finished %q for %s's trip to %s", player.Name, locked.quest, npc.Name, locked.place)
		}
		return true
	}
	forgetDialogs(player)
	player.PendingCoachDests = dests
	if len(dests) == 1 {
		player.PendingYesNo = true
		h.sendOptionBox(sess, wayAskOne(dests[0]), []string{"Yes", "No"})
	} else {
		h.sendCoachPage(sess, player)
	}
	log.Printf("Waysender: %s offers %s %d trips", npc.Name, player.Name, len(dests))
	return true
}

// forgetDialogs closes whatever box an NPC had open for the player: a new one
// replaces it, and a pick must answer the box on the screen.
func forgetDialogs(player *entity.Player) {
	player.PendingCoachDests = nil
	player.CoachPage = 0
	player.PendingYesNo = false
	player.PendingBind = entity.Bind{}
	player.PendingSpar = entity.SparPick{}
	if player.PendingHDDMenu != HDDMenuNone {
		player.PendingHDDMenu = HDDMenuNone
		clearFormatArm(player.ServerID)
	}
}

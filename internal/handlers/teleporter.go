package handlers

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// teleporter.go — the Teleporter (the owner, 2026-11-09: "lets add a portal npc
// named Teleporter. has destinations of all raids and camps", and "put it next
// to dabrish"). Talked to, it asks where to, a box at a time, and sends the
// player on as a waysender does (waysender.go's travel: "Very well.", the
// warp, the world loaded): for nothing, at any level, and whatever quest a
// raid's own way in asks first (a test server's convenience: the Heirloom Test
// Server sets world.teleporter). Every box holds at most maxDialogOptions
// answers, which CLIENT shows three a page with its own More and Back; the
// first is "Raids" and the camps by their levels:
//
//   - Raids: by where they are (teleRaidGroups), then the raid, named with its
//     bosses' levels as their rows have them. A raid is EQOA-Data's map's
//     raid mob (Map/zones.json, category raid-mob), petition 1240's raid
//     bosses (their twelve hours' respawn) or one of the scripted raids
//     (tools/guidegaps/), offered only where a row of one of its bosses
//     stands in its world (kept out of the world till its event, or dead,
//     counts). Each lands at its approach: the way in the game's own trip
//     takes (a portal's or a marker's spot, as a waysender's trip lands by its
//     NPC), else a place read off the ground files, on dry ground the boss's
//     walk reaches, out of its notice and clear of every creature by 40 (the
//     table's notes); facing the boss (or the pad its portal wakes on).
//   - Camps: #tele's XP camps (camps.go, tools/gen_camp_teles.py) and the GMs'
//     own (/camp, gmcamps.go) but one beside a #tele camp, sorted by their
//     creatures' levels and folded into boxes of at most six (levelFold), each
//     a range of levels; a camp is offered where its creatures stand, and lands
//     where #tele's does, beside it out of its creatures' notice
//     (World.CampLanding), facing it.
//
// Only worlds the player's client loads are offered (CanLoadWorld), and a box
// is answered only within teleReach of the Teleporter. The list is read again
// at most every teleCatalogFor, so a GM's new camp is offered within a minute.
// Any NPC named Teleporter is it; tools/place_teleporter.py stands one beside
// Dabrish. Its questions go under box mark 0x5E000000 (beside Dabrish's 0x5C
// and the archivists' 0x5D). The words are ours.

// TeleporterName is its name: an NPC of it is the Teleporter.
const TeleporterName = "Teleporter"

// teleporterBoxMark marks its questions' ids.
const teleporterBoxMark = 0x5E000000

// teleporterOn is world.teleporter: off unless set.
var teleporterOn atomic.Bool

// SetTeleporter turns the Teleporter on or off (world.teleporter).
func SetTeleporter(on bool) { teleporterOn.Store(on) }

// TeleporterOn reports world.teleporter.
func TeleporterOn() bool { return teleporterOn.Load() }

// IsTeleporter is whether an NPC of the name is the Teleporter.
func IsTeleporter(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), TeleporterName)
}

const (
	// teleReach is how near the Teleporter a box is answered
	teleReach = 30
	// teleRaidReach is how far from a raid's landing its bosses' rows count
	// (Rallos Zek's isle is 1,478 from the corner his portal wakes in; the two
	// Siliskors stand 3,400 apart)
	teleRaidReach = 2500
	// teleCatalogFor is how long what it offers is kept before it is read again
	teleCatalogFor = time.Minute
	// teleCampFold is how near a #tele camp's creatures a GM's camp counts as
	// that camp (petition 1293's statics on Xrag's rock are the xrag camp)
	teleCampFold = campAnchorReach
)

// Its words.
const (
	teleAskRoot     = "Teleporter: 'I open the way to every raid and every experience camp. Where to?'"
	teleAskRaids    = "Teleporter: 'Raids, by where they are. Which?'"
	teleAskRaid     = "Teleporter: 'Which raid? I set you down at its way in, out of its notice.'"
	teleAskLevels   = "Teleporter: 'Which levels?'"
	teleAskCamp     = "Teleporter: 'Which camp? I set you down beside it, out of its creatures' notice.'"
	teleClosed      = "My portals are closed on this server."
	teleNowhere     = "I know of no raid or camp you could reach from here."
	teleTooFar      = "Come back to me to go: my portals open beside me."
	teleLost        = "I cannot find that place now. Ask me again."
	teleAnswerRaids = "Raids"
)

// teleRaid is one of its raids.
type teleRaid struct {
	group  int      // teleRaidGroups' index
	name   string   // as its answer names it, before its levels
	bosses []string // its bosses by name: their rows' levels name it, and one must stand
	face   string   // what its landing faces when not its bosses (a portal's pad)
	arrive string   // the NPC whose spot it lands by (World.ArrivalLanding), as a waysender's trip
	at     *wayPlace
}

// teleRaidGroups are the raids' first box.
var teleRaidGroups = []string{
	"Tunaria and the Isle of Dread",
	"Odus, Lavastorm and the Plane of Sky",
	"The Plane of Disease",
	"Zaoran Plateau",
}

// telePlaces are places it sends to by name, each an answer of its first box
// after Raids, for nothing as every trip of it: the Isle of Dread (the owner,
// 2026-11-11: "lets add isle of dread free to teleporter npc freeport"),
// landing as Norach's trip does, on the fishing raft in the cove south of
// Graumord's pier, facing the pier; and the Plane of Sky (petition 1605, the GM
// Jibrish: "a teleport option on the main page of the teleporter that goes to
// plane of sky and lands exactly where im standing"), where he stood.
var telePlaces = []struct {
	label string
	at    wayPlace
}{
	{"Isle of Dread", wayPlace{World: 0, X: 24578, Y: 52.6, Z: 3626, Facing: 3.9}},
	{"Plane of Sky", wayPlace{World: 4, X: 5000.09, Y: 333.54, Z: 4998.23, Facing: 3.068}},
}

// teleRaids are its raids, each with where it lands (measured 2026-11-09 over
// the box's ground and environment files and its creatures: a scratch walk
// from each boss on the terrain and what is placed on it).
var teleRaids = []teleRaid{
	// Permafrost's walkway, 56 north of its outer gate guard: the way in, which
	// walks to the floor under her
	{group: 0, name: "Lady Vox, Permafrost", bosses: []string{"Lady Vox"}, at: &wayPlace{World: 0, X: 4860, Y: 61.62, Z: 4765}},
	// the north rim of his basin, 125 from him, a walk down; the other row of
	// the name, 3,400 away, is talked to (the NPC bit)
	{group: 0, name: "Siliskor", bosses: []string{"Siliskor"}, at: &wayPlace{World: 0, X: 14520, Y: 54.19, Z: 26900}},
	// the canyon by his cave ("Skahyir (Inside Cave)"), 144 from him
	{group: 0, name: "Skahyir's cave", bosses: []string{"Skahyir"}, at: &wayPlace{World: 0, X: 21640, Y: 6.47, Z: 21380}},
	// before his Estate's main door, 56 from The Guardians: "have your raid
	// party assemble in IoD, at CT's Estate" (Portal/isle-of-dread)
	{group: 0, name: "Cazic-Thule's Estate, Isle of Dread", bosses: []string{"CazicThule"}, at: &wayPlace{World: 0, X: 25660, Y: 54.12, Z: 3235}},
	// the isle's west shore, 80 from where it swims ("In ocean west of Sirens")
	{group: 0, name: "Megaladon, Isle of Dread", bosses: []string{"Megaladon"}, at: &wayPlace{World: 0, X: 24254.7, Y: 51.03, Z: 3058.2}},

	// in her den ("Toxxulia (In Den)"), 82 south of her on its walk out
	{group: 1, name: "Toxxulia's den, Odus", bosses: []string{"Toxxulia"}, at: &wayPlace{World: 2, X: 7324.8, Y: -32.9, Z: 7215.8}},
	// his island, where the Subdued Magus sends raiders (waysender.go), 63 off
	{group: 1, name: "Lord Nagafen, Lavastorm", bosses: []string{"Lord Nagafen"}, arrive: "Nagafen Teleport Arrive"},
	// her isle, 70 east of her
	{group: 1, name: "Quintessence of Veeshan", bosses: []string{"Quintessence of Veeshan"}, at: &wayPlace{World: 4, X: 4798, Y: 858.7, Z: 7616.6}},
	// by the main isle's Astral Light, whose talk takes the raid to the twins'
	// isle (tools/guidegaps/sky-twins.json; its run sends them back here)
	{group: 1, name: "Veeshan's twins, Plane of Sky", bosses: []string{"Talon's Veeshan", "Veeshan's Bulwark"}, face: "Astral Light",
		at: &wayPlace{World: 4, X: 5133, Y: 250.06, Z: 5445}},

	// his lair, 70 north of him
	{group: 2, name: "Noroxus", bosses: []string{"Noroxus"}, at: &wayPlace{World: 5, X: 4188, Y: -18.19, Z: 2169}},
	// the middle valley's pad, where the Bloody Portal lands (103 from
	// Uzrathixius; Bertoxxulous's throne beyond them)
	{group: 2, name: "Bertoxxulous and the twins", bosses: []string{"Bertoxxulous", "Poxathixius", "Uzrathixius"}, arrive: "Portal of the Twins"},
	// the wasp hive's pad, where the Whirling Portal lands, 82 from her
	{group: 2, name: "Queen Qy'tha", bosses: []string{"Queen Qy'tha"}, arrive: "Portal to the Lake"},

	// the back corner his portal wakes in (the Signpost of Zek's), 172 from the
	// bridge guards: the Mark's run opens it (tools/guidegaps/rallos.json)
	{group: 3, name: "Rallos Zek", bosses: []string{"Rallos Zek"}, face: "Portal of Rallos", at: &wayPlace{World: 5, X: 5730, Y: 147.62, Z: 6412}},
	// the plateau's port-in (the portal guardian's), his castle beside it: a
	// talk to him atop it starts the summoning (tools/guidegaps/razeal.json)
	{group: 3, name: "Razeal Moloch", bosses: []string{"Razeal Moloch"}, arrive: "ZP1"},
	// where the Portal of Tallon's quest sets the raid down, below the way up
	{group: 3, name: "Tallon Zek's fortress", bosses: []string{"Tallon Zek"}, at: &wayPlace{World: 5, X: 4750, Y: 147.62, Z: 7380}},
	// where the Portal of Zek's quest sets the raid down, his training
	// grounds' near side ("speak with Vallon Zek at its far side")
	{group: 3, name: "Vallon Zek's training grounds", bosses: []string{"Vallon Zek"}, at: &wayPlace{World: 5, X: 4377.8, Y: 253.5, Z: 7052.8}},
}

// teleCampNames are the #tele camps as its answers name them (camps.go's
// labels are a GM's list's); a camp without one is named by its label.
var teleCampNames = map[string]string{
	"hatebone": "Hatebone orcs, Halas", "hatebone2": "Hatebone orcs 2, Halas", "hatebonecave": "Hatebone cave, Halas",
	"tombrats": "Tomb rats", "coldpaw": "Coldpaw gnolls, Klik'Anon", "cadavers": "Druidic cadavers, Qeynos",
	"nasehir": "Nasehir, Northern Ro", "nasehir2": "Nasehir 2, Northern Ro", "orctunnel": "The Druk's orc tunnel",
	"cracktooth": "Cracktooth orcs", "gusts": "Freezing gusts", "freezeblood": "Freezeblood goblins",
	"klickpirates": "Klik'Anon pirates", "slesher": "Slesher gnolls, Surefall", "nroundead": "Undead, Northern Ro",
	"nroundead2": "Undead 2, Northern Ro", "goblincitadel": "The Goblin Citadel", "bloodfoots": "Bloodfoots, Hatespike",
	"aviaks": "Aviak outpost, Halas", "cutthroats": "Cutthroats, Mulin's Reach", "cutthroats2": "Cutthroats 2, Mulin's Reach",
	"crocodiles": "Saltwater crocodiles", "plainsraiders": "Orc plainsraiders", "plainstriders": "Orc plainstriders",
	"scarecrows": "Scarecrows, Alseop's Wall", "blackburrow": "Blackburrow gnolls", "gnollfishers": "Gnoll fishermen",
	"serpents": "Serpent guardians", "gharial": "Gharial island", "dfc": "Deathfist Citadel", "guardianforest": "Guardian Forest goblins",
	"alfarak": "Al Farak Ruins", "siloths": "Salt Mine siloths", "minotaurs": "Rustel minotaurs", "drudges": "Kappa drudges, Odus",
	"hexbones": "Hexbones, Zentar's Keep", "cyclops": "The cyclops", "hillgiants": "Keliner's hill giants",
	"icegiants": "Ice giant refugees", "kappa": "Kappa, Slythis Dwell", "kappawarriors": "Kappa warriors, Odus",
	"tendora": "Tendora-Buki kerrans", "gargoyles": "Gargoyles, Barren Coast", "phantoms": "Phantoms, East Plateau",
	"kappaelites": "Kappa elites, Odus", "moots": "Moots, Erud's Crossing", "harpies": "Harpies, Erud's Crossing",
	"ghosts": "Wandering ghosts", "xrag": "Children of Xrag",
}

// teleSpot is a place it offers: its answer, its creatures' levels (0: not
// known), its world and how it lands.
type teleSpot struct {
	label     string
	low, high int
	world     uint8
	raid      *teleRaid
	bx, bz    float32 // what a raid's landing faces
	camp      string  // a #tele camp's key
	gm        *entity.GMCampSpot
	place     *wayPlace // a place of telePlaces: lands there, facing as it says
}

// teleNode is a box's answer: a further box (kids, asked under ask) or a trip.
type teleNode struct {
	label     string
	order     string // between camps of the same levels: a #tele camp's key, a GM's after them
	ask       string
	low, high int
	kids      []*teleNode
	spot      *teleSpot
}

// teleAsk is a question asked: its box, its answers and the Teleporter's id.
type teleAsk struct {
	box  uint32
	node *teleNode
	npc  entity.EntityID
}

// Teleporter keeps the questions it asked, by character, and what it offers.
type Teleporter struct {
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]teleAsk
	spots []teleSpot
	read  time.Time
	now   func() time.Time // tests
}

func (t *Teleporter) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// levelsLabel is "61-66", or one level, or "" when none is known.
func levelsLabel(low, high int) string {
	switch {
	case low <= 0 && high <= 0:
		return ""
	case low == high || high <= 0:
		return strconv.Itoa(low)
	case low <= 0:
		return strconv.Itoa(high)
	}
	return fmt.Sprintf("%d-%d", low, high)
}

// withLevels is an answer: the name and its levels in brackets.
func withLevels(name string, low, high int) string {
	if l := levelsLabel(low, high); l != "" {
		return clipRunes(name+" ("+l+")", questAnswerChars)
	}
	return clipRunes(name, questAnswerChars)
}

// parseLevels reads camps.go's "33-40" or "45".
func parseLevels(s string) (low, high int) {
	a, b, two := strings.Cut(s, "-")
	low, _ = strconv.Atoi(strings.TrimSpace(a))
	high = low
	if two {
		high, _ = strconv.Atoi(strings.TrimSpace(b))
	}
	return low, high
}

// upFirst is s with its first letter a capital.
func upFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 && r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// catalog is what it offers now: read again after teleCatalogFor.
func (t *Teleporter) catalog(gw *entity.World) []teleSpot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now := t.clock(); t.spots == nil || now.Sub(t.read) >= teleCatalogFor {
		t.spots, t.read = readTeleSpots(gw), now
	}
	return t.spots
}

// readTeleSpots reads the raids and camps the world holds: one pass over its
// NPCs for every name they are known by, then the GMs' camps.
func readTeleSpots(gw *entity.World) []teleSpot {
	want := map[string]bool{}
	key := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	for _, r := range teleRaids {
		for _, b := range r.bosses {
			want[key(b)] = true
		}
		if r.arrive != "" {
			want[key(r.arrive)] = true
		}
		if r.face != "" {
			want[key(r.face)] = true
		}
	}
	for _, d := range campDests {
		want[key(d.NPC)] = true
	}
	byName := map[string][]entity.NPCSpawn{}
	for _, s := range gw.NPCSpawnsNamed(func(name string) bool { return want[key(name)] }) {
		byName[key(s.Name)] = append(byName[key(s.Name)], s)
	}
	nearest := func(name string, world uint8, x, z, reach float32) (entity.NPCSpawn, bool) {
		var best entity.NPCSpawn
		found, bd := false, reach*reach
		for _, s := range byName[key(name)] {
			if s.World != world {
				continue
			}
			if d := (s.X-x)*(s.X-x) + (s.Z-z)*(s.Z-z); d <= bd {
				best, bd, found = s, d, true
			}
		}
		return best, found
	}

	var out []teleSpot
	for i := range teleRaids {
		r := &teleRaids[i]
		var world uint8
		var x, z float32
		switch {
		case r.at != nil:
			world, x, z = r.at.World, r.at.X, r.at.Z
		case r.arrive != "":
			a := byName[key(r.arrive)]
			if len(a) == 0 {
				continue // its way in is not here
			}
			world, x, z = a[0].World, a[0].X, a[0].Z
		default:
			continue
		}
		sp := teleSpot{raid: r, world: world}
		boss := false
		for _, b := range r.bosses {
			for _, s := range byName[key(b)] {
				if s.World != world || (s.X-x)*(s.X-x)+(s.Z-z)*(s.Z-z) > teleRaidReach*teleRaidReach {
					continue
				}
				if !boss {
					sp.bx, sp.bz = s.X, s.Z
				}
				boss = true
				if s.Level > 0 && (sp.low == 0 || s.Level < sp.low) {
					sp.low = s.Level
				}
				sp.high = max(sp.high, s.Level)
			}
		}
		if !boss {
			continue // no boss of it stands here
		}
		if r.face != "" {
			if f, ok := nearest(r.face, world, x, z, teleRaidReach); ok {
				sp.bx, sp.bz = f.X, f.Z
			}
		}
		sp.label = withLevels(r.name, sp.low, sp.high)
		out = append(out, sp)
	}

	// the #tele camps whose creatures stand, then the GMs' that are none of them
	type anchor struct {
		world uint8
		x, z  float32
	}
	var anchors []anchor
	keys := make([]string, 0, len(campDests))
	for k := range campDests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := campDests[k]
		a, ok := nearest(d.NPC, d.World, d.X, d.Z, campAnchorReach)
		if !ok {
			continue
		}
		anchors = append(anchors, anchor{d.World, a.X, a.Z})
		low, high := parseLevels(d.Levels)
		name := teleCampNames[k]
		if name == "" {
			name = upFirst(d.Label)
		}
		out = append(out, teleSpot{label: withLevels(name, low, high), low: low, high: high, world: d.World, camp: k})
	}
	for _, g := range gw.GMCampSpots() {
		if len(g.Kinds) == 0 {
			continue
		}
		folded := false
		for _, a := range anchors {
			if a.world == g.World && (a.x-g.X)*(a.x-g.X)+(a.z-g.Z)*(a.z-g.Z) <= teleCampFold*teleCampFold {
				folded = true
				break
			}
		}
		if folded {
			continue
		}
		g := g
		out = append(out, teleSpot{label: withLevels(upFirst(strings.Join(g.Kinds, ", ")), g.MinLevel, g.MaxLevel),
			low: g.MinLevel, high: g.MaxLevel, world: g.World, gm: &g})
	}
	return out
}

// tree is the boxes for this session: its spots in the worlds its client loads.
func (t *Teleporter) tree(gw *entity.World, sess *session.Session) *teleNode {
	root := &teleNode{ask: teleAskRoot}
	raids := &teleNode{label: teleAnswerRaids, ask: teleAskRaids}
	groups := make([]*teleNode, len(teleRaidGroups))
	var camps []*teleNode
	spots := t.catalog(gw) // replaced whole when read again, never written in place
	for i := range spots {
		sp := &spots[i]
		if !CanLoadWorld(sess, sp.world) {
			continue
		}
		leaf := &teleNode{label: sp.label, low: sp.low, high: sp.high, spot: sp, order: sp.camp}
		if sp.gm != nil {
			leaf.order = "~" + sp.label
		}
		if sp.raid == nil {
			camps = append(camps, leaf)
			continue
		}
		g := sp.raid.group
		if groups[g] == nil {
			groups[g] = &teleNode{label: teleRaidGroups[g], ask: teleAskRaid}
		}
		groups[g].kids = append(groups[g].kids, leaf)
	}
	for _, g := range groups {
		if g != nil {
			raids.kids = append(raids.kids, g)
		}
	}
	if len(raids.kids) == 1 {
		raids = raids.kids[0] // one place: its raids at once
		raids.label = teleAnswerRaids
	}
	if len(raids.kids) > 0 {
		root.kids = append(root.kids, raids)
	}
	for i := range telePlaces {
		pl := &telePlaces[i]
		if !CanLoadWorld(sess, pl.at.World) {
			continue
		}
		at := pl.at
		root.kids = append(root.kids, &teleNode{label: pl.label,
			spot: &teleSpot{label: pl.label, world: at.World, place: &at}})
	}
	sort.SliceStable(camps, func(i, j int) bool {
		a, b := camps[i], camps[j]
		if a.low != b.low {
			return a.low < b.low
		}
		if a.high != b.high {
			return a.high < b.high
		}
		return a.order < b.order
	})
	for _, n := range camps {
		n.high = n.low // a camp's part goes by the levels its camps start at
	}
	parts := levelFold(camps, maxDialogOptions-len(root.kids))
	for _, p := range parts {
		if p.spot == nil { // a range of camps: named as the first box's
			p.label = "Camps, " + strings.ToLower(p.label[:1]) + p.label[1:]
		}
	}
	root.kids = append(root.kids, parts...)
	return root
}

// levelFold puts nodes (sorted by level) in boxes: as they are when they fit
// in room answers; else in parts of at most maxDialogOptions each, split
// where the levels change where it can (levelParts), folded again until the
// parts fit in room.
func levelFold(nodes []*teleNode, room int) []*teleNode {
	if len(nodes) <= room {
		return nodes
	}
	parts := levelParts(nodes, maxDialogOptions)
	for len(parts) > room {
		parts = levelParts(parts, maxDialogOptions)
	}
	return parts
}

// levelParts splits nodes into the fewest parts of at most most each, as even
// as they come, cutting where the levels change rather than within a level
// when it can: each part a box of camps (or of parts) named "Levels a-b".
func levelParts(nodes []*teleNode, most int) []*teleNode {
	n := len(nodes)
	k := (n + most - 1) / most
	ideal := float64(n) / float64(k)
	// cost[j][i]: the best way to put the first i nodes in j parts
	const inf = math.MaxFloat64
	cost := make([][]float64, k+1)
	from := make([][]int, k+1)
	for j := range cost {
		cost[j] = make([]float64, n+1)
		from[j] = make([]int, n+1)
		for i := range cost[j] {
			cost[j][i] = inf
		}
	}
	cost[0][0] = 0
	for j := 1; j <= k; j++ {
		for i := 1; i <= n; i++ {
			for s := 1; s <= most && s <= i; s++ {
				prev := cost[j-1][i-s]
				if prev == inf {
					continue
				}
				c := prev + (float64(s)-ideal)*(float64(s)-ideal)
				if i < n && nodes[i-1].low == nodes[i].low {
					c += 10 // a level split across two parts
				}
				if c < cost[j][i] {
					cost[j][i], from[j][i] = c, i-s
				}
			}
		}
	}
	var cuts []int
	for j, i := k, n; j > 0; j-- {
		cuts = append([]int{from[j][i]}, cuts...)
		i = from[j][i]
	}
	cuts = append(cuts, n)
	parts := make([]*teleNode, 0, k)
	for p := 0; p < k; p++ {
		kids := nodes[cuts[p]:cuts[p+1]]
		part := &teleNode{ask: teleAskCamp, kids: kids, low: kids[0].low, high: kids[0].high}
		if kids[0].spot == nil {
			part.ask = teleAskLevels
		}
		for _, c := range kids {
			part.low, part.high = min(part.low, c.low), max(part.high, c.high)
		}
		if part.low == part.high {
			part.label = "Level " + strconv.Itoa(part.low)
		} else {
			part.label = "Levels " + levelsLabel(part.low, part.high)
		}
		parts = append(parts, part)
	}
	return parts
}

// teleSay is its line in a dialogue box.
func teleSay(sess *session.Session, line string) {
	for _, part := range splitSay(TeleporterName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// ask opens a box of node's answers for p.
func (t *Teleporter) ask(sess *session.Session, p *entity.Player, node *teleNode, npc entity.EntityID) {
	t.mu.Lock()
	t.seq++
	id := teleporterBoxMark | (t.seq & 0xFFFFFF)
	if t.asked == nil {
		t.asked = map[uint32]teleAsk{}
	}
	t.asked[p.CharServerID] = teleAsk{box: id, node: node, npc: npc}
	t.mu.Unlock()
	answers := make([]string, 0, len(node.kids))
	for _, k := range node.kids {
		answers = append(answers, clipRunes(k.label, questAnswerChars))
	}
	SendQuestPopupMulti(sess, id, clipRunes(node.ask, questAskChars), answers)
}

// teleporterTalk is a talk to npc: its first box when npc is the Teleporter,
// else false.
func (h *InteractHandler) teleporterTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsTeleporter(npc.Name) {
		return false
	}
	forgetDialogs(p)
	if !TeleporterOn() || h.gameWorld == nil {
		teleSay(sess, teleClosed)
		return true
	}
	root := h.tele.tree(h.gameWorld, sess)
	if len(root.kids) == 0 {
		teleSay(sess, teleNowhere)
		return true
	}
	h.tele.ask(sess, p, root, npc.ServerID)
	return true
}

// answerTeleporter is HandleDialogueOption's question for its box: a further
// box or a trip. It reports whether the box was its (an old one, or a close,
// does nothing).
func (h *InteractHandler) answerTeleporter(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != teleporterBoxMark {
		return false
	}
	if option == 255 {
		return true // a close: a PS2 closes one box before the pick in the next
	}
	t := &h.tele
	t.mu.Lock()
	asked, ok := t.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(t.asked, p.CharServerID)
	}
	t.mu.Unlock()
	if !ok || asked.box != box || int(option) >= len(asked.node.kids) || h.gameWorld == nil {
		return true
	}
	if !TeleporterOn() {
		teleSay(sess, teleClosed)
		return true
	}
	if w, x, _, z, here := h.gameWorld.NPCSpot(asked.npc); !here || w != p.World ||
		(x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) > teleReach*teleReach {
		teleSay(sess, teleTooFar)
		return true
	}
	kid := asked.node.kids[option]
	if kid.spot == nil {
		t.ask(sess, p, kid, asked.npc)
		return true
	}
	dest, ok := teleLanding(h.gameWorld, kid.spot)
	if !ok {
		log.Printf("Teleporter: %s's trip to %s: no landing", p.Name, kid.label)
		teleSay(sess, teleLost)
		return true
	}
	if _, err := ValidateTeleport(dest.X, dest.Y, dest.Z, dest.World); err != nil || !CanLoadWorld(sess, dest.World) {
		log.Printf("Teleporter: %s's trip to %s refused: %v (world %d)", p.Name, kid.label, err, dest.World)
		teleSay(sess, teleLost)
		return true
	}
	log.Printf("Teleporter: %s to %s", p.Name, kid.label)
	h.travel(sess, p, dest)
	h.teleGroup(p, asked.npc, dest, kid.label)
	return true
}

// teleGroupWords is what a member the trip takes along is told (the
// picker's name, the trip's answer).
const teleGroupWords = "Teleporter: %s takes the group to %s."

// teleGroup takes p's group along on p's trip (petition 1408, Waldorf: "should
// take the group at same time to same spot. I went to Xrags and took each of
// my 4 group members to different parts of the place"): every member standing
// by the Teleporter, as near as a box is answered from, alive, whose client
// loads the trip's world, goes at once to p's landing, facing as p does. A
// camp's landing is worked out from where its creatures stand at the moment,
// so a trip a member took on their own landed apart. A member away from the
// Teleporter stays where they are. Each is moved as a GM trigger's teleport
// moves players (BeginWarp under the World's lock), not by travel, which is
// for the session's own player.
func (h *InteractHandler) teleGroup(p *entity.Player, npc entity.EntityID, dest entity.CoachDest, label string) {
	h.moveGroup(p, npc, dest, fmt.Sprintf(teleGroupWords, p.Name, label), "Teleporter", nil)
}

// moveGroup is teleGroup's move for any NPC's trip: each member by the NPC
// (teleReach), alive, whose client loads the world, told words and moved to
// dest; each then given to also (nil: nothing more). who names the mover in
// the log.
func (h *InteractHandler) moveGroup(p *entity.Player, npc entity.EntityID, dest entity.CoachDest, words, who string, also func(*session.Session, *entity.Player)) {
	label := dest.Name
	gw := h.gameWorld
	w, x, _, z, here := gw.NPCSpot(npc)
	if !here {
		return
	}
	for _, m := range gw.GroupMembers(p) {
		if m.ServerID == p.ServerID {
			continue
		}
		mw, mx, _, mz, dead, ok := gw.PlayerSpot(m)
		if !ok || dead || mw != w || (mx-x)*(mx-x)+(mz-z)*(mz-z) > teleReach*teleReach {
			continue
		}
		ms := gw.FindSessionByEntityID(m.ServerID)
		if ms == nil || !ms.CharacterInWorld() || ms.PendingTermination() {
			continue
		}
		if !CanLoadWorld(ms, dest.World) {
			log.Printf("%s: %s stays: their client cannot load world %d (%s's trip to %s)", who, m.Name, dest.World, p.Name, label)
			continue
		}
		_, counter, changed, ok := gw.BeginWarp(m.ServerID, dest.World)
		if !ok {
			continue
		}
		SendClientMessage(ms, words)
		sendTeleportWarp(ms, dest.World, dest.X, dest.Y, dest.Z, dest.Facing, counter)
		beginWorldLoad(ms, changed)
		gw.UpdatePosition(m.ServerID, dest.X, dest.Y, dest.Z, dest.Facing)
		log.Printf("%s: %s goes with %s to %s", who, m.Name, p.Name, label)
		if also != nil {
			also(ms, m)
		}
	}
}

// heading is CLIENT's heading from (x, z) toward (tx, tz) (VIVect3::HPR: the
// direction (sin h, cos h)), in [0, 2*pi).
func heading(x, z, tx, tz float32) float32 {
	h := math.Atan2(float64(tx-x), float64(tz-z))
	if h < 0 {
		h += 2 * math.Pi
	}
	return float32(h)
}

// teleLanding is where a picked spot lands now: a raid's way in, a camp
// beside its creatures (as #tele's), facing what it goes to.
func teleLanding(gw *entity.World, sp *teleSpot) (entity.CoachDest, bool) {
	dest := entity.CoachDest{Name: sp.label, World: sp.world, Via: TeleporterName}
	switch {
	case sp.place != nil:
		dest.X, dest.Y, dest.Z, dest.World, dest.Facing = sp.place.X, sp.place.Y, sp.place.Z, sp.place.World, sp.place.Facing
		return dest, true
	case sp.raid != nil && sp.raid.at != nil:
		dest.X, dest.Y, dest.Z = sp.raid.at.X, sp.raid.at.Y, sp.raid.at.Z
	case sp.raid != nil:
		x, y, z, facing, world, ok := gw.SpawnOfNPC(sp.raid.arrive)
		if !ok {
			return dest, false
		}
		fx, fz, _ := inFrontOf(x, z, facing)
		if lx, ly, lz, _, ok := gw.ArrivalLanding(world, x, y, z, facing, bindReach); ok {
			fx, y, fz = lx, ly, lz
		}
		dest.X, dest.Y, dest.Z, dest.World = fx, y, fz, world
	case sp.camp != "":
		d, ok := campDests[sp.camp]
		if !ok {
			return dest, false
		}
		x, y, z, ok := gw.CampLanding(d.NPC, d.World, d.X, d.Z, campAnchorReach)
		if !ok {
			return dest, false
		}
		dest.X, dest.Y, dest.Z = x, y, z
		sp = &teleSpot{bx: d.X, bz: d.Z}
	case sp.gm != nil:
		landed := false
		for _, kind := range sp.gm.Kinds {
			if x, y, z, ok := gw.CampLanding(kind, sp.gm.World, sp.gm.X, sp.gm.Z, campAnchorReach); ok {
				dest.X, dest.Y, dest.Z, landed = x, y, z, true
				break
			}
		}
		if !landed {
			return dest, false
		}
		sp = &teleSpot{bx: sp.gm.X, bz: sp.gm.Z}
	default:
		return dest, false
	}
	if dest.X != sp.bx || dest.Z != sp.bz {
		dest.Facing = heading(dest.X, dest.Z, sp.bx, sp.bz)
	}
	return dest, true
}

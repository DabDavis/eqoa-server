// Command wildspread spaces the open world's wildlife out and fills the
// ground none of it reaches (jib's F3 20261029-024920: "Wildlife density
// throughout the world is concentrated in tiny parts. There will be maybe 10
// mobs in a small spot, then huge swathes of empty spaces ... Fill in all very
// empty spots with sporadic roaming wildlife spawns that attempt to stay
// within their area and not go into cities"; the user, 2026-10-31: "we have
// gaps in mob population", "re do the wild life spawns so they are spaced
// correctly also").
//
// The npcs rows came from captures of the live game's spawn points, so a
// spawn's creatures stand where they were seen, in a heap: on Tunaria 39% of
// the open land was more than 300 from any creature to fight. This tool does
// two things, in each world it is given (Tunaria, Rathe and Odus by default),
// and nothing else:
//
//  1. Spread. A clump is wildlife the server wanders (the server's own
//     verdict, entity's movement: an animal by its name, not undead, or a
//     roamer's npcmove row) whose spawns chain within clumpLink of each
//     other. Its creatures are spread evenly (Lloyd's relaxation over the
//     land samples) over the clump's own ground: the land nearer its middle
//     than any other clump's, within territory of it, reachable from it
//     without crossing water or a wall. Each keeps its name, level, body and
//     every other column; only its spawn moves (x, y, z: y the floor there).
//     A creature stays put when it is: one of a kind (a named creature), a
//     players' race body, in the water or keeping to it, off the open ground
//     (a cave, a cellar, a bridge: more than onGround from the highest
//     floor), in or near a town, within campKeep of an EQOA-Data camp (an
//     XP camp players go to: the grizzlies, the griffons, the crocodiles),
//     under a place-bound faction rule, or a row another tool placed or a
//     path, a sign or a quest names.
//  2. Fill. Land still farther than gapFill from any creature gets new
//     creatures, farthest gap first, none nearer than fillSpacing to any
//     other: each a copy of a namesake row (as tools/seed_wildlife.py makes
//     its seeds: the most common body's nearest row, every column but the
//     place, the level and npc_id 0) of a kind drawn from the wildlife
//     round it (weighted by nearness, within kindReach, then twice and
//     three times that), at those creatures' levels. A gap with no wildlife
//     anywhere near is left, and said.
//
// The land is the world's ground files (internal/ground: the highest floor
// at every step units) less the water, slime, lava and mud (the environment
// files), the towns (entity/towns.go's towns, townClear round their people)
// and Tunaria's boundary and sea zones (CLIENT's zone grid, when -words gives
// WORDS.TXT), and only what is reachable: samples join when the rise between
// them is under maxRise and no wall stands between, from every creature and
// town on the open ground.
//
// Without -apply it reports: before.png and after.png per world, plan.json,
// moves.csv and fills.csv in -out. -apply writes the plan in one transaction,
// logged in wildspread_meta, wildspread_moves and wildspread_adds; a second
// -apply is refused while they are there. -undo puts every moved row back
// where it was (a row moved since, by a GM, is left and said), deletes every
// added row, puts sqlite_sequence back when nothing was added after, and
// drops the log. The plan comes from a fixed random seed: the same database
// and ground give the same plan. The server reads npcs at its start: apply
// with the server stopped (a swap's before.sh), or restart after.
//
//	wildspread -db eqoa.db -ground worlds/ground [-words WORDS.TXT] [-eqoa-data DIR]
//	    [-prima prima-frontiers-layout.txt] [-make-kinds 1] [-out DIR] [-worlds 0,1,2] [-seed N]
//	    [-apply | -undo]
//
// -make-kinds makes the bestiary's beasts a world has no row of from the
// bodies of kinds it has (made.go: Rathe's), for the fill where nothing round
// will do.
//
// A run round a few places (2026-11-03, the towns' newbie ground: the user,
// "we also have bare areas around grobb ... use prima guide to fill with
// correct mobs for the levels", of a troll warrior's pythons "too few and
// spread out"): -near x,z,r[;x,z,r...] (or @file, a circle a line) keeps the
// run to those circles (a creature outside them stays, nothing is placed
// outside them), -no-spread moves nothing, -gap and -spacing set the fill's
// two distances (the world's run filled land 250 from any creature with one
// 260 from any other; Sandstorm's newbie ground holds about one creature of
// level 1-12 to every 10,000 square units within 500 of a town, as Halas
// walked on the ground showed), and -quest-weight counts a kind a quest's
// kill or loot step names that many times in a fill within questReach of
// the quest's giver. -tag NAME logs the run in wildspread_meta_NAME,
// wildspread_moves_NAME, wildspread_adds_NAME and wildspread_kinds_NAME,
// applied and undone apart from the world's run (each refuses a second of
// its own tag), and a tagged run's fills stay where it put them in any
// later run.
//
// -group size:weight,... makes a fill a group (the others within
// -group-radius of the first, each kind drawn again: Sandstorm's newbie
// creatures stand in twos and threes, a creature's nearest another a median
// 22 off round Halas and Qeynos, where one creature every 100 read as bare
// ground: the user, 2026-11-03, of Grobb's "the mobs are spread out too much
// or too sparse"), and -quest-min N keeps each newbie quest's kill or loot
// step's creatures (its target and queststeptargets' others) to N within
// -quest-min-reach of its giver, in groups of the step's wildlife kinds.
//
//	wildspread ... -worlds 0 -near @towns.txt -no-spread -gap 90 -spacing 95
//	    -group 1:20,2:28,3:22,4:14,5:9,6:7 -group-radius 28 -quest-weight 3 -quest-min 10
//	    -layers 0:2,125:2,375:3,625:5,875:6,1250:11 -tag towns [-apply | -undo]
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// The plan's numbers, in world units.
const (
	step        = 25.0  // the land's sample step
	maxRise     = 25.0  // the most two samples a step apart may differ in height and join (45°)
	onGround    = 3.0   // a spawn on the open ground: within this of the highest floor
	townClear   = 120.0 // a placed creature from a town's people (tools/seed_wildlife.py's TOWN_CLEAR)
	npcClear    = 25.0  // a placed creature from any NPC not moved with it
	campKeep    = 200.0 // wildlife this near an EQOA-Data camp stays
	clumpLink   = 60.0  // wildlife this close chains into one clump
	territory   = 450.0 // how far from its clump's middle a creature is spread
	lloydRounds = 12
	gapFill     = 250.0  // land farther than this from any creature is filled
	fillSpacing = 260.0  // a filled creature from any other creature
	kindReach   = 1500.0 // a fill's kinds: the wildlife within this (then 2x, 3x)
	minPiece    = 40     // samples: the smallest piece of reachable land a creature is placed on
	jitter      = 10.0   // a placed creature's offset from its sample, at most
	envBad      = ground.EnvWater | ground.EnvSlime | ground.EnvLava | ground.EnvMud
	defaultSeed = 31
)

func main() {
	dbPath := flag.String("db", "eqoa.db", "the server's database")
	groundDir := flag.String("ground", "worlds/ground", "the ground and environment files (cmd/groundbake)")
	wordsPath := flag.String("words", "", "the disc's DATA/WORDS.TXT: Tunaria's zones by name (boundary and sea zones left out)")
	eqoaData := flag.String("eqoa-data", "", "EQOA-Data's checkout (Map/zones.json): its camps' wildlife stays, its zones' biomes")
	primaPath := flag.String("prima", "", "Prima's Frontiers guide as pdftotext -layout writes it: its bestiary, the wildlife each biome has")
	out := flag.String("out", "wildspread", "where the report goes")
	worldsFlag := flag.String("worlds", "0,1,2", "the worlds to spread and fill")
	makeFlag := flag.String("make-kinds", "", "the worlds whose missing bestiary beasts are made from bodies (made.go), e.g. 1 for Rathe")
	seed := flag.Int64("seed", defaultSeed, "the plan's random seed")
	apply := flag.Bool("apply", false, "write the plan into the database")
	undo := flag.Bool("undo", false, "take an applied plan out again")
	tag := flag.String("tag", "", "a run of its own: its log in wildspread_*_<tag>, applied and undone apart from the others")
	nearFlag := flag.String("near", "", "only round these places, x,z,r[;x,z,r...] or @file (one a line): a creature outside them stays, nothing is placed outside them")
	noSpread := flag.Bool("no-spread", false, "fill only: no creature moves")
	gap := flag.Float64("gap", gapFill, "land farther than this from any creature is filled")
	spacing := flag.Float64("spacing", fillSpacing, "a filled creature from any other creature")
	questWeight := flag.Float64("quest-weight", 1, "a kind a quest kills or loots, its giver within questReach, counts this many times in a fill")
	layersFlag := flag.String("layers", "", "a fill's level by its distance from a -near circle's town (its fourth number): d:level[,d:level...]")
	groupFlag := flag.String("group", "", "a fill is a group: its size by weight, size:weight[,size:weight...] (the others of a kind drawn again)")
	groupRadius := flag.Float64("group-radius", 25, "how far a group's others stand from its first")
	questMin := flag.Int("quest-min", 0, "a newbie quest's kill or loot step keeps this many of its creatures within -quest-min-reach of its giver")
	questMinReach := flag.Float64("quest-min-reach", 1250, "the reach -quest-min counts within")
	flag.Parse()
	for _, c := range *tag {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			log.Fatalf("-tag %q: letters, digits and _ only", *tag)
		}
	}
	logs = tablesFor(*tag)
	near, err := readNear(*nearFlag)
	if err != nil {
		log.Fatalf("-near: %v", err)
	}
	layers, err := readLayers(*layersFlag)
	if err != nil {
		log.Fatalf("-layers: %v", err)
	}
	if len(layers) > 0 && len(near) == 0 {
		log.Fatal("-layers counts from -near's circles")
	}
	sizes, weights, err := readGroup(*groupFlag)
	if err != nil {
		log.Fatalf("-group: %v", err)
	}
	if *groupRadius <= groupGap || *questMin < 0 || *questMinReach <= 150 {
		log.Fatalf("-group-radius over %v, -quest-min 0 or more, -quest-min-reach over 150", groupGap)
	}
	if *gap <= 0 || *spacing <= 0 || *questWeight <= 0 {
		log.Fatal("-gap, -spacing and -quest-weight are more than 0")
	}

	db, err := database.NewSQLite(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if *undo {
		if err := undoPlan(db); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *apply {
		if applied, err := hasLog(db); err != nil {
			log.Fatal(err)
		} else if applied {
			log.Fatalf("a plan is applied already (%s): -undo it first", logs.meta)
		}
	}
	var worlds []int
	for _, s := range strings.Split(*worldsFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 || n > 255 {
			log.Fatalf("-worlds: %q", s)
		}
		worlds = append(worlds, n)
	}
	g, err := ground.Open(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	env, err := ground.OpenEnv(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	var words []string
	if *wordsPath != "" {
		if words, err = readWords(*wordsPath); err != nil {
			log.Fatal(err)
		}
	}
	var camps []campPoint
	if *eqoaData != "" {
		if camps, err = readCamps(*eqoaData); err != nil {
			log.Fatal(err)
		}
	}
	in, err := load(db, g, env)
	if err != nil {
		log.Fatal(err)
	}
	p := newPlan(in, g, env, *groundDir, words, camps, *seed)
	p.near, p.noSpread, p.gapFill, p.fillSpacing, p.questWeight = near, *noSpread, *gap, *spacing, *questWeight
	p.layers = layers
	p.groupSizes, p.groupWeights, p.groupRadius = sizes, weights, *groupRadius
	p.questMin, p.questMinReach = *questMin, *questMinReach
	if *questWeight != 1 || *questMin > 0 {
		if p.questGivers, p.questSteps, err = readQuestGivers(db, in); err != nil {
			log.Fatalf("the quests: %v", err)
		}
	}
	if *primaPath != "" {
		if p.prima, err = readPrima(*primaPath); err != nil {
			log.Fatal(err)
		}
		say("Prima's bestiary: %d names", len(p.prima))
	}
	if *eqoaData != "" {
		if p.biomes, err = readZoneBiomes(*eqoaData); err != nil {
			log.Fatal(err)
		}
	}
	if *makeFlag != "" {
		makeWorlds := map[int]bool{}
		for _, s := range strings.Split(*makeFlag, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				log.Fatalf("-make-kinds: %q", s)
			}
			makeWorlds[n] = true
		}
		p.makeKinds(makeWorlds)
	}
	for _, w := range worlds {
		p.world(w)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := p.report(*out); err != nil {
		log.Fatal(err)
	}
	if *apply {
		if err := applyPlan(db, p, *seed); err != nil {
			log.Fatal(err)
		}
	}
}

// quiet runs f with the log discarded (the world's loading says every NPC).
func quiet(f func()) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	f()
}

func readWords(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(string(b), "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return append([]string{""}, strings.Split(s, "\n")...), nil
}

// readNear reads -near's circles: x,z,r[,r0[,sections]] (r0 the town's own
// reach, from which -layers counts, 0 without it; sections the bestiary's
// sections its wildlife comes from, by '+', "Swamp" or "Tunaria/Swamp")
// separated by ';' or, from @file, a line each ('#' starts a comment).
func readNear(s string) ([]circle, error) {
	if strings.HasPrefix(s, "@") {
		b, err := os.ReadFile(s[1:])
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	var out []circle
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' }) {
		if i := strings.Index(part, "#"); i >= 0 {
			part = part[:i]
		}
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		f := strings.Split(part, ",")
		if len(f) < 3 || len(f) > 5 {
			return nil, fmt.Errorf("%q: x,z,r[,r0[,sections]]", part)
		}
		var secs []string
		if len(f) == 5 {
			for _, s := range strings.Split(f[4], "+") {
				if s = strings.TrimSpace(s); s == "" {
					continue
				}
				if !strings.Contains(s, "/") {
					s = "Tunaria/" + s
				}
				secs = append(secs, s)
			}
			f = f[:4]
		}
		var v [4]float64
		for i := range f {
			n, err := strconv.ParseFloat(strings.TrimSpace(f[i]), 64)
			if err != nil {
				return nil, fmt.Errorf("%q: %v", part, err)
			}
			v[i] = n
		}
		if v[2] <= 0 || v[3] < 0 || v[3] >= v[2] {
			return nil, fmt.Errorf("%q: a radius more than 0 and the town's within it", part)
		}
		out = append(out, circle{v[0], v[1], v[2], v[3], secs})
	}
	return out, nil
}

// readGroup reads -group: size:weight pairs by ',' (none: a group of one).
func readGroup(s string) ([]int, []float64, error) {
	var sizes []int
	var weights []float64
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		sw := strings.Split(part, ":")
		if len(sw) != 2 {
			return nil, nil, fmt.Errorf("%q: size:weight", part)
		}
		n, err1 := strconv.Atoi(strings.TrimSpace(sw[0]))
		w, err2 := strconv.ParseFloat(strings.TrimSpace(sw[1]), 64)
		if err1 != nil || err2 != nil || n < 1 || n > 20 || w <= 0 {
			return nil, nil, fmt.Errorf("%q: a size of 1-20 and a weight over 0", part)
		}
		sizes, weights = append(sizes, n), append(weights, w)
	}
	return sizes, weights, nil
}

// readLayers reads -layers: d:level pairs by ',', d rising.
func readLayers(s string) ([]layer, error) {
	var out []layer
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		dl := strings.Split(part, ":")
		if len(dl) != 2 {
			return nil, fmt.Errorf("%q: d:level", part)
		}
		d, err1 := strconv.ParseFloat(strings.TrimSpace(dl[0]), 64)
		l, err2 := strconv.ParseFloat(strings.TrimSpace(dl[1]), 64)
		if err1 != nil || err2 != nil || d < 0 || l < 1 || (len(out) > 0 && d <= out[len(out)-1].D) {
			return nil, fmt.Errorf("%q: d:level, d rising from 0, level 1 or more", part)
		}
		out = append(out, layer{d, l})
	}
	return out, nil
}

func say(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

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
//	    [-prima prima-frontiers-layout.txt] [-out DIR] [-worlds 0,1,2] [-seed N] [-apply | -undo]
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
	seed := flag.Int64("seed", defaultSeed, "the plan's random seed")
	apply := flag.Bool("apply", false, "write the plan into the database")
	undo := flag.Bool("undo", false, "take an applied plan out again")
	flag.Parse()

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
			log.Fatal("a plan is applied already (wildspread_meta): -undo it first")
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

func say(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

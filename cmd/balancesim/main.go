// Command balancesim fights simulated fights on the server's own systems
// (internal/entity/balancesim.go) and prints how each class fares by level
// against NPCs of each con: the share of fights won, the hit points left in
// a fight won and how long it took. A GM's check of the balance tables
// (balancelevel, balancecon) before they go in.
//
//	balancesim -db eqoa.db [-balance db|seed|none] [-classes 0,6,9,13]
//	    [-levels 1,10,20,30,40,50,60] [-diffs 0,1,2,3,4,5] [-fights 40]
//	    [-tier 0.5] [-pets] [-limit 10m]
//	    [-try-level 1:300:200:0,60:500:300:100] [-try-con 1:115:95,5:200:60]
//
// -try-level and -try-con try rows before they are written: level:hp_pct:
// damage_pct[:con_pct[:pet_hp_pct:pet_damage_pct]] and diff:damage_pct:
// taken_pct, in place of the tables' (balance db or seed) rows.
//
// The database is copied first and the copy opened (opening runs the schema
// setup); the original is never written. Players are kitted from its own
// tables (entity.SimKitFromDB): the item at -tier's place of the last ten
// levels' in each slot, its training points spent, its class's spells; with
// -pets the classes that summon one also fight with their pet.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

func ints(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			log.Fatalf("%q: %v", f, err)
		}
		out = append(out, n)
	}
	return out
}

var classNames = [...]string{"WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"}

func main() {
	dbPath := flag.String("db", "", "the server database (copied, never written)")
	which := flag.String("balance", "db", "db: the database's tables; seed: the seed rows; none: no balance")
	classes := flag.String("classes", "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14", "classes (WAR=0 ... ALC=14)")
	levels := flag.String("levels", "1,5,10,15,20,25,30,35,40,45,50,55,60", "player levels")
	diffs := flag.String("diffs", "-1,0,1,2,3,4,5", "NPC levels above the player")
	fights := flag.Int("fights", 40, "fights a cell")
	tier := flag.Float64("tier", 0.5, "the gear's place among the last ten levels' items (0 worst, 1 best)")
	pets := flag.Bool("pets", false, "classes with a summoning also fight with their pet")
	petAlone := flag.Bool("petalone", false, "with -pets, the pet fights alone (its owner casts nothing): won is the pet's")
	limit := flag.Duration("limit", 10*time.Minute, "longest fight")
	tryLevel := flag.String("try-level", "", "level rows to try: level:hp:damage[:pethp:petdamage],...")
	tryCon := flag.String("try-con", "", "con rows to try: diff:damage:taken,...")
	flag.Parse()
	if *dbPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	db, dir := openCopy(*dbPath)
	defer os.RemoveAll(dir)
	var b *entity.Balance
	switch *which {
	case "db":
		ls, err := db.BalanceLevels()
		if err != nil {
			log.Fatal(err)
		}
		cs, err := db.BalanceCons()
		if err != nil {
			log.Fatal(err)
		}
		b = entity.NewBalance(ls, cs)
	case "seed":
		b = entity.SeedBalance()
	case "none":
	default:
		log.Fatalf("-balance %q: db, seed or none", *which)
	}
	if *tryLevel != "" || *tryCon != "" {
		ls, cs := b.Rows()
		if *tryLevel != "" {
			ls = nil
			for _, r := range rows(*tryLevel, 3) {
				l := database.BalanceLevel{Level: int(r[0]), HPPct: r[1], DamagePct: r[2], ConPct: 100, PetHPPct: 100, PetDamagePct: 100}
				if len(r) >= 4 {
					l.ConPct = r[3]
				}
				if len(r) >= 6 {
					l.PetHPPct, l.PetDamagePct = r[4], r[5]
				}
				ls = append(ls, l)
			}
		}
		if *tryCon != "" {
			cs = nil
			for _, r := range rows(*tryCon, 3) {
				cs = append(cs, database.BalanceCon{Diff: int(r[0]), DamagePct: r[1], TakenPct: r[2]})
			}
		}
		b = entity.NewBalance(ls, cs)
	}
	ds := ints(*diffs)
	fmt.Printf("%-4s %-6s %-3s %-4s", "", "", "lvl", "pet")
	for _, d := range ds {
		fmt.Printf(" | %+d won  hp  secs", d)
	}
	fmt.Println()
	for _, class := range ints(*classes) {
		for _, level := range ints(*levels) {
			pl, err := entity.SimKitFromDB(db, class, 0, level, *tier)
			if err != nil {
				log.Fatal(err)
			}
			modes := []bool{false}
			if *pets && entity.SimHasPet(pl) {
				modes = append(modes, true)
			}
			for _, pet := range modes {
				k := pl
				if pet {
					k.Play.Pet, k.Play.Melee = true, false
					if *petAlone {
						k.Play.Spells = false
					}
				}
				fmt.Printf("%-4s %-6s %3d %-4v", classNames[class], entity.ArchetypeName(class), level, pet)
				for _, d := range ds {
					c := entity.SimCellOf(k, entity.SimNPC{}, d, b, *fights, 1, *limit)
					fmt.Printf(" | %3.0f%% %3.0f%% %4.0f", 100*c.WinShare(), 100*c.HPLeft, c.Secs)
				}
				fmt.Println()
			}
		}
	}
}

// rows reads "a:b:c,..." rows of at least min numbers.
func rows(s string, min int) [][]float64 {
	var out [][]float64
	for _, f := range strings.Split(s, ",") {
		var r []float64
		for _, v := range strings.Split(strings.TrimSpace(f), ":") {
			x, err := strconv.ParseFloat(v, 64)
			if err != nil {
				log.Fatalf("%q: %v", f, err)
			}
			r = append(r, x)
		}
		if len(r) < min {
			log.Fatalf("%q: %d numbers, want %d", f, len(r), min)
		}
		out = append(out, r)
	}
	return out
}

// openCopy opens a copy of the database in a directory of its own (under
// TMPDIR), and gives that directory for removing.
func openCopy(path string) (*database.SQLiteDB, string) {
	dir, err := os.MkdirTemp("", "balancesim")
	if err != nil {
		log.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	cp := filepath.Join(dir, "eqoa.db")
	out, err := os.Create(cp)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		log.Fatal(err)
	}
	out.Close()
	log.SetOutput(io.Discard) // the schema setup's lines
	db, err := database.NewSQLite(cp)
	log.SetOutput(os.Stderr)
	if err != nil {
		log.Fatal(err)
	}
	return db, dir
}

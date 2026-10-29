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
//	    [-try-art 1:0,30:1500,60:2000] [-try-class 1:80,5:80,6:80]
//	    [-root] [-try-root 0:0:5,3:35:20,5:65:30] [-try-regen 1:5,60:5]
//	    [-charm -6,-3,0] [-only false|true|root|charm]
//	balancesim -db eqoa.db -lines [-classes ...]
//	balancesim -db eqoa.db -reset 20s [-try-regen 1:5] [-classes ...]
//
// -reset fights each fight that long and then breaks it off: the player
// runs out of the NPC's reach for good and rests, and it prints how long the
// NPC took to be whole again (its heal out of a fight, regen_pct).
//
// -lines prints each class's buff lines instead: the spells of one line, of
// which only the best counts on a player (entity/buffline.go).
//
// -try-level and -try-con try rows before they are written: level:hp_pct:
// damage_pct[:con_pct[:pet_hp_pct:pet_damage_pct]] and diff:damage_pct:
// taken_pct, in place of the tables' (balance db or seed) rows; -try-art
// the arts' cooldown by the player's level (level:ms), the line through them;
// -try-class class:melee_taken_pct rows in place of the class table's;
// -try-root a root's resist and break chances by the con (diff:resist:break).
// With -root the classes that have a root also fight as a rooting caster
// does (entity.SimPlay.Root): the root first, then their spells from range,
// the root again whenever it is off; the column "pet" reads "root". With
// -charm the classes with a charm that takes a creature that many levels
// from their own also fight with one charmed (entity.SimPlay.Charm), the
// column reading c-3 and so on.
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
	"sort"
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
	lines := flag.Bool("lines", false, "print each class's buff lines (only the best of a line counts) and stop")
	tryArt := flag.String("try-art", "", "the arts' cooldown to try, by the player's level: level:ms,...")
	tryClass := flag.String("try-class", "", "class rows to try: class:melee_taken_pct,... (in place of the table's)")
	tryRoot := flag.String("try-root", "", "a root's resist and break chances to try, by the con: diff:resist:break,...")
	tryRegen := flag.String("try-regen", "", "a creature's heal out of a fight to try, by its level: level:pct,...")
	rootPlay := flag.Bool("root", false, "classes with a root also fight rooting and casting from range, no pet")
	only := flag.String("only", "", "only this play's rows: false (hand to hand), true (the pet) or root")
	reset := flag.Duration("reset", 0, "instead: fight this long, then run off for good and rest; how long the NPC takes to be whole")
	charms := flag.String("charm", "", "classes with a charm also fight with a charmed creature this many levels from their own: -6,-3,0,...")
	flag.Parse()
	if *dbPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	db, dir := openCopy(*dbPath)
	defer os.RemoveAll(dir)
	if *lines {
		printLines(db, ints(*classes))
		return
	}
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
		cls, err := db.BalanceClasses()
		if err != nil {
			log.Fatal(err)
		}
		b = entity.NewBalance(ls, cs).WithClasses(cls)
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
		b = entity.NewBalance(ls, cs).WithClasses(b.ClassRows())
	}
	if *tryClass != "" {
		var cls []database.BalanceClass
		for _, r := range rows(*tryClass, 2) {
			cls = append(cls, database.BalanceClass{Class: int(r[0]), MeleeTakenPct: r[1]})
		}
		b = b.WithClasses(cls)
	}
	if *tryRoot != "" {
		var resist, brk [][2]float64
		for _, r := range rows(*tryRoot, 3) {
			resist = append(resist, [2]float64{r[0], r[1]})
			brk = append(brk, [2]float64{r[0], r[2]})
		}
		b = b.WithConColumn(resist, func(r *database.BalanceCon, v float64) { r.RootResistPct = v })
		b = b.WithConColumn(brk, func(r *database.BalanceCon, v float64) { r.RootBreakPct = v })
	}
	if *tryRegen != "" {
		b = b.WithLevelColumn(points(*tryRegen), func(r *database.BalanceLevel, v float64) { r.RegenPct = v })
	}
	if *tryArt != "" {
		b = b.WithLevelColumn(points(*tryArt), func(r *database.BalanceLevel, v float64) { r.ArtCooldownMS = v })
	}
	ds := ints(*diffs)
	if *reset > 0 {
		printResets(db, b, ints(*classes), ints(*levels), ds, *fights, *tier, *reset)
		return
	}
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
			modes := []string{"false"}
			if *pets && entity.SimHasPet(pl) {
				modes = append(modes, "true")
			}
			if *rootPlay && entity.SimHasRoot(pl) {
				modes = append(modes, "root")
			}
			if *charms != "" {
				for _, c := range ints(*charms) {
					if entity.SimCharmable(pl, c) {
						modes = append(modes, fmt.Sprintf("c%+d", c))
					}
				}
			}
			for _, mode := range modes {
				if *only != "" && mode != *only && !(*only == "charm" && strings.HasPrefix(mode, "c")) {
					continue
				}
				k := pl
				switch mode {
				case "true":
					k.Play.Pet, k.Play.Melee = true, false
					if *petAlone {
						k.Play.Spells = false
					}
				case "root":
					k.Play.Root, k.Play.Melee = true, false
				default:
					if strings.HasPrefix(mode, "c") {
						c, _ := strconv.Atoi(mode[1:])
						k.Play.Charm, k.Play.CharmDiff, k.Play.Melee = true, c, false
					}
				}
				fmt.Printf("%-4s %-6s %3d %-4v", classNames[class], entity.ArchetypeName(class), level, mode)
				for _, d := range ds {
					c := entity.SimCellOf(k, entity.SimNPC{}, d, b, *fights, 1, *limit)
					fmt.Printf(" | %3.0f%% %3.0f%% %4.0f", 100*c.WinShare(), 100*c.HPLeft, c.Secs)
				}
				fmt.Println()
			}
		}
	}
}

// printResets fights each cell's fights for after, then breaks each off
// (entity.SimulateReset) and prints the NPC's hit points left at the break,
// the seconds it took to be whole again, and the player's hit points then
// (a fight that ended before the break is left out).
func printResets(db *database.SQLiteDB, b *entity.Balance, classes, levels, ds []int, fights int, tier float64, after time.Duration) {
	fmt.Printf("broken off after %v: the NPC's HP left at the break, seconds to whole, the player's HP at the break -> when the NPC is whole\n", after)
	for _, class := range classes {
		for _, level := range levels {
			pl, err := entity.SimKitFromDB(db, class, 0, level, tier)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%-4s %3d", classNames[class], level)
			for _, d := range ds {
				var npc, whole, hp0, hp1 float64
				n := 0
				for i := range fights {
					r := entity.SimulateReset(pl, entity.SimNPC{Level: max(level+d, 1)}, b, int64(1+i), after, 10*time.Minute)
					if r.Ended || r.Whole == 0 {
						continue
					}
					n++
					npc += r.NPCShare
					whole += r.Whole.Seconds()
					hp0 += r.HPShare
					hp1 += r.HPShareWhenNPCWhole
				}
				if n == 0 {
					fmt.Printf(" | %+d     -", d)
					continue
				}
				k := float64(n)
				fmt.Printf(" | %+d npc %3.0f%% %4.0fs you %3.0f%%->%3.0f%%", d, 100*npc/k, whole/k, 100*hp0/k, 100*hp1/k)
			}
			fmt.Println()
		}
	}
}

// printLines prints every buff line of each class's book (every spell of
// the class, masteries' and epics' too) with more than one spell in it: the
// spells only the best of counts on a player (entity/buffline.go).
func printLines(db *database.SQLiteDB, classes []int) {
	for _, class := range classes {
		book, err := entity.SimBookFromDB(db, class, 100, true)
		if err != nil {
			log.Fatal(err)
		}
		byLine := entity.SimBuffLines(book)
		var keys []string
		single := 0
		for k, ss := range byLine {
			if ss[0].Def.Harmful && !strings.Contains(k, "@self") {
				continue // a debuff on a creature: no player bears it
			}
			if len(ss) > 1 {
				keys = append(keys, k)
			} else {
				single++
			}
		}
		sort.Strings(keys)
		fmt.Printf("== %s: %d lines of two or more spells, %d of one\n", classNames[class], len(keys), single)
		for _, k := range keys {
			var names []string
			for _, s := range byLine[k] {
				names = append(names, fmt.Sprintf("%s (%d)", s.Name, s.Level))
			}
			fmt.Printf("  %-40s %s\n", k, strings.Join(names, ", "))
		}
	}
}

// points reads "x:y,..." as points of a line.
func points(s string) [][2]float64 {
	var out [][2]float64
	for _, r := range rows(s, 2) {
		out = append(out, [2]float64{r[0], r[1]})
	}
	return out
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

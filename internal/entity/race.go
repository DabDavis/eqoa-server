package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// race.go — the races the server has and CLIENT does not, each a row of table
// racedefs (the user, 2026-11-07: "the hill giant race addition is a test of future
// playable races"; claude-notes/custom-races-20261107.md is how to add one).
//
// CLIENT's gRaces has names, cameras, a create page and every item's race bit
// for the races it ships, and a slot past them (or an item's mask without its
// bit) leaves a character with no camera and nothing it may wear, in vanilla and
// on a PS2 alike. So a row's race passes as one of CLIENT's on the wire (WireRace:
// the Hill Giant as the Ogre, the nearest build) and is its own race here: its
// stats (LookupBaseStats), its masteries (cmcatalog.races, its own bit), its
// standings (factionstart) and its name in what the server writes itself (a GM's
// lines, the target box's title, the race labels a client that asks is sent:
// racelabels.go). Its body is the character's own model id, as an illusion's is;
// the row's two say which it should be. Character creation takes one where the
// server opens it (world.open_races: creation.go, which a client that can is told
// of before the world); elsewhere only a GM's hand makes a character of one.
//
// A race with no row is what it always was: the retail races' tables are below
// and in stats.go, and any other number goes on the wire as it is.

// FirstCustomRace and LastCustomRace bound a racedefs row's id: past CLIENT's ten
// and the froglok, and no further than a race's bit in a mask the GM panel's page
// can hold (JavaScript's bit operators stop at 32 bits).
const (
	FirstCustomRace = 11
	LastCustomRace  = 30
)

// RaceDef is a racedefs row as the game reads it.
type RaceDef struct {
	ID    int
	Name  string
	Short string // the short columns' word: never empty (raceDefOf)
	Wire  int    // the race of CLIENT's it passes as
	Bonus [7]int // STR STA AGI DEX WIS INT CHA
	Body  [2]int64
	// Desc is the create page's text, ClassMask the classes creation offers it
	// when every class is not open to every race (0: its wire race's, the game's
	// masks), City the human city a race passing as a human starts in (1 east,
	// 2 west; 0 none): creation.go.
	Desc      string
	ClassMask int
	City      int
}

// raceDefs is the table as last read: the free functions below read it from
// any goroutine, with no World at hand.
var raceDefs atomic.Pointer[map[int]RaceDef]

// CheckRaceDef is why a row cannot be a race, "" when it can: the rows the game
// could not use, which the GM panel refuses and a load leaves out.
func CheckRaceDef(id int, name string, wire int) string {
	switch {
	case id < FirstCustomRace || id > LastCustomRace:
		return fmt.Sprintf("id is %d to %d (0 to 10 are the game's and the froglok's)", FirstCustomRace, LastCustomRace)
	case strings.TrimSpace(name) == "":
		return "a race needs a name"
	case len(name) > raceLabelMax:
		return fmt.Sprintf("a race's name is at most %d bytes", raceLabelMax)
	case wire < 0 || wire > raceOgre:
		return fmt.Sprintf("wire_race is a race of the game's, 0 to %d: a character passes as it on every client", raceOgre)
	}
	return ""
}

// raceDefOf is a row as the game reads it: its short name, its own or the first
// letters of its name.
func raceDefOf(r database.RaceDefRow) RaceDef {
	short := strings.TrimSpace(r.ShortName)
	if short == "" {
		letters := []rune(strings.ReplaceAll(r.Name, " ", ""))
		short = strings.ToUpper(string(letters[:min(3, len(letters))]))
	}
	if len(short) > raceLabelMax {
		short = short[:raceLabelMax]
	}
	return RaceDef{ID: r.ID, Name: r.Name, Short: short, Wire: r.WireRace, Bonus: r.Bonus, Body: [2]int64{r.BodyMale, r.BodyFemale},
		Desc: r.Desc, ClassMask: r.ClassMask, City: r.City}
}

// CheckRaceDefCreation is why a row's creation columns cannot be, "" when they
// can: a class mask of the game's fifteen classes (0 its wire race's), and a
// city of the humans' (1 east, 2 west) only for a race passing as a human.
func CheckRaceDefCreation(wire, classMask, city int) string {
	switch {
	case classMask < 0 || classMask > allClassesMask:
		return fmt.Sprintf("class_mask is the classes' bits, 0 to %d (0: the wire race's)", allClassesMask)
	case city < 0 || city > 2:
		return "city is 0, or for a race passing as a human 1 (Freeport) or 2 (Qeynos)"
	case city != 0 && wire != 0:
		return "city is a human's: 0 for a race passing as any other"
	}
	return ""
}

// SetRaceDefs makes these rows the races, leaving out with a line each one the
// game cannot use (CheckRaceDef): its characters are then as with no row.
func SetRaceDefs(rows []database.RaceDefRow) {
	defs := make(map[int]RaceDef, len(rows))
	for _, r := range rows {
		if why := CheckRaceDef(r.ID, r.Name, r.WireRace); why != "" {
			log.Printf("racedefs: row %d (%q) left out: %s", r.ID, r.Name, why)
			continue
		}
		defs[r.ID] = raceDefOf(r)
	}
	raceDefs.Store(&defs)
}

// LoadRaceDefs reads table racedefs: at the start and after a GM's edit. It says
// which character of such a race wears another body than the race's, a GM's choice
// or a step missed. Call it without the database held.
func LoadRaceDefs(db *database.SQLiteDB) error {
	rows, err := db.RaceDefRows()
	if err != nil {
		return fmt.Errorf("racedefs: %w", err)
	}
	SetRaceDefs(rows)
	chars, err := db.CharactersOfRaces(CustomRaceIDs())
	if err != nil {
		return fmt.Errorf("racedefs: its characters: %w", err)
	}
	for _, c := range chars {
		d, _ := customRace(c.Race)
		if body := d.Body[min(max(c.Sex, 0), 1)]; body != 0 && body != c.ModelID {
			log.Printf("racedefs: %s (%s) wears %#x, the race's body is %#x", c.Name, d.Name, uint32(c.ModelID), uint32(body))
		}
	}
	return nil
}

// customRace is race's row, if it has one.
func customRace(race int) (RaceDef, bool) {
	if m := raceDefs.Load(); m != nil {
		d, ok := (*m)[race]
		return d, ok
	}
	return RaceDef{}, false
}

// IsCustomRace reports a race with a row: one WireRace changes.
func IsCustomRace(race int) bool {
	_, ok := customRace(race)
	return ok
}

// RaceDefs is the races with a row, by id.
func RaceDefs() []RaceDef {
	m := raceDefs.Load()
	if m == nil {
		return nil
	}
	out := make([]RaceDef, 0, len(*m))
	for _, d := range *m {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CustomRaceIDs is their ids, in order.
func CustomRaceIDs() []int {
	defs := RaceDefs()
	out := make([]int, len(defs))
	for i, d := range defs {
		out[i] = d.ID
	}
	return out
}

// RaceDefSummary is the races in a line, for the start.
func RaceDefSummary() string {
	defs := RaceDefs()
	if len(defs) == 0 {
		return "none past the game's (racedefs has no row)"
	}
	parts := make([]string, len(defs))
	for i, d := range defs {
		parts[i] = fmt.Sprintf("%s (%d, %s, as the %s)", d.Name, d.ID, d.Short, nameIn(raceNames, d.Wire))
	}
	return strings.Join(parts, ", ")
}

// WireRace is the race CLIENT is told, and an item's mask is asked about, for a
// race of the server's.
func WireRace(race int) int {
	if d, ok := customRace(race); ok {
		return d.Wire
	}
	return race
}

// allRacesMask is cmcatalog.races' "every race": the eleven bits CLIENT's races
// and the froglok fill.
const allRacesMask = 2047

// raceInMask reports a race in a bit set of races: its own bit, or a race past
// the eleven in a set of every race. It needs no row: a new race's number is its
// bit, and "every race" takes it in.
func raceInMask(mask, race int) bool {
	if race > raceFroglok && mask&allRacesMask == allRacesMask {
		return true
	}
	return mask&(1<<uint(race)) != 0
}

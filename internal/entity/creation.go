package entity

import (
	"fmt"
	"sync/atomic"
)

// creation.go — what character creation takes past the game's own races and
// classes (the user, 2026-11-07: "lets add to the char sel for heirloom server
// other races like kerran, froglok, hill giant, goblin, aviak, Lizardman. also
// lets open all classes to all races on the heirloom server", then "add kappa to
// playable chars also"; world.open_races and world.all_classes, both off unless
// set: claude-notes/custom-races-20261107.md).
//
// The game's create pages offer its ten races (the humans by city), each with
// its classes (gRaces' masks), and a PS2's are the disc's whatever a server says.
// A client that can is told before the world what else this server takes
// (handlers/creation.go: the froglok and the racedefs rows, and whether every
// class is open to every race), and the server takes them (CheckCreation). A
// race of racedefs is made as its row says: its body (the character's model),
// its bonuses, its wire race's city and quests (it passes as that race), and its
// own start rows (database.startPlace, StartItems) and standings (factionstart).
// The froglok is the S2's race 10, a human of the east's in all but its body.

// CreationRules is what creation takes past the game's own.
type CreationRules struct {
	// Races: the froglok and every race of racedefs may be made, and a client
	// that can is told them (world.open_races).
	Races bool
	// AllClasses: every class for every race (world.all_classes).
	AllClasses bool
	// Skins: the game's ten races' skin shades (world.skin_shades,
	// raceoptions.go's gameRaceRows), offered in the races' options to a
	// client that shows them; nothing to do with creation's own offer.
	Skins bool
}

// On reports anything past the game's own: a client that can is to be told.
func (r CreationRules) On() bool { return r.Races || r.AllClasses }

var creationRules atomic.Value // CreationRules

// SetCreationRules sets what creation takes: at the start, from config.
func SetCreationRules(r CreationRules) { creationRules.Store(r) }

// Creation is what creation takes now.
func Creation() CreationRules {
	r, _ := creationRules.Load().(CreationRules)
	return r
}

// allClassesMask is every class's bit: WAR 0 to ALC 14.
const allClassesMask = 0x7FFF

// gameClassMasks is each race's classes in the game, gRaces' m_classMask
// (decomp src/virace.cpp), on the network race enum: bit N class N.
var gameClassMasks = [...]int{
	0: 0x7EFF, // human of no city (the table's; the pages offer only the two below)
	1: 0x76E6, // elf
	2: 0x7E49, // dark elf
	3: 0x7E41, // gnome
	4: 0x0245, // dwarf: warrior, paladin, rogue, cleric
	5: 0x0109, // troll: warrior, shadow knight, shaman
	6: 0x0141, // barbarian: warrior, rogue, shaman
	7: 0x02C1, // halfling: warrior, rogue, druid, cleric
	8: 0x7E0C, // erudite
	9: 0x0909, // ogre: warrior, shadow knight, shaman, necromancer
}

// GameClassMask is the classes the game lets a race of its own be, a human's by
// its city as IsValidRaceClassCityCombo has them (1 east 0x7E69, 2 west 0x76F7);
// the froglok's is the S2's, a human of the east's. A race past them: none.
func GameClassMask(race, city int) int {
	switch {
	case race == 0 && city == 1, race == raceFroglok:
		return 0x7E69
	case race == 0 && city == 2:
		return 0x76F7
	case race >= 0 && race < len(gameClassMasks):
		return gameClassMasks[race]
	}
	return 0
}

// CreationRace is a race the create pages are told of past the game's own: the
// froglok, and each row of racedefs.
type CreationRace struct {
	ID int
	// Like is the game's race whose place on the create pages' stage, cameras,
	// music and starting city it takes (a racedefs race's wire race; the
	// froglok's a human's), City that human's city (1 east, 2 west) when Like is
	// a human, else 0.
	Like, City  int
	Name, Short string
	Desc        string
	Bonus       [7]int // STR STA AGI DEX WIS INT CHA
	// ClassMask is the classes it may be when every class is not open to every
	// race: its row's, else its wire race's.
	ClassMask int
	Body      [2]int64 // male, female: CHAR.ESF DictIDs, as characters.modelid
}

// FroglokBody is the froglok's CHAR.ESF CSprite, both sexes (there is no female
// froglok): handlers.FrogloklModelID.
const FroglokBody int64 = 0x141BFE0C

// froglok is the S2's race 10 as creation offers it: a human of the east's
// (Freeport, its classes and bonuses) in the froglok's body.
var froglok = CreationRace{
	ID: raceFroglok, Like: 0, City: 1, Name: "Froglok", Short: "FRG",
	Desc:      "Frogloks are an earnest and devout people of the swamps, brave beyond their size, who have found a home among the humans of Freeport.",
	Bonus:     [7]int{5, 5, 5, 5, 5, 5, 5},
	ClassMask: 0x7E69,
	Body:      [2]int64{FroglokBody, FroglokBody},
}

// CreationRaces is every race creation takes past the game's own, by id: the
// froglok, then the rows of racedefs, while Races is on; none otherwise.
func CreationRaces() []CreationRace {
	if !Creation().Races {
		return nil
	}
	out := []CreationRace{froglok}
	for _, d := range RaceDefs() {
		mask := d.ClassMask
		if mask == 0 {
			mask = GameClassMask(d.Wire, d.City)
		}
		city := d.City
		if d.Wire != 0 {
			city = 0
		}
		out = append(out, CreationRace{ID: d.ID, Like: d.Wire, City: city, Name: d.Name, Short: d.Short, Desc: d.Desc,
			Bonus: d.Bonus, ClassMask: mask, Body: d.Body})
	}
	return out
}

// creationRace is race's entry of CreationRaces.
func creationRace(race int) (CreationRace, bool) {
	for _, c := range CreationRaces() {
		if c.ID == race {
			return c, true
		}
	}
	return CreationRace{}, false
}

// CheckCreation is why a new character of race, class and city (a human's: 1
// east, 2 west) cannot be made, "" when it can. The game's ten races and the
// froglok take any class 0 to 14 and city 0 to 2 as they always did (a client's
// own pages hold it to its race's; the server never did). A race of racedefs is
// taken only while Races is on: any class while AllClasses is on, else its row's
// classes (or its wire race's).
func CheckCreation(race, class, city int) string {
	switch {
	case class < 0 || class > 14:
		return fmt.Sprintf("class %d", class)
	case city < 0 || city > 2:
		return fmt.Sprintf("city %d", city)
	case race >= 0 && race <= raceFroglok:
		return ""
	}
	r := Creation()
	if !r.Races {
		return fmt.Sprintf("race %d (no race past the game's here: world.open_races)", race)
	}
	c, ok := creationRace(race)
	if !ok {
		return fmt.Sprintf("race %d (racedefs has no such row)", race)
	}
	if !r.AllClasses && c.ClassMask&(1<<uint(class)) == 0 {
		return fmt.Sprintf("class %d is not the %s's (world.all_classes is off)", class, c.Name)
	}
	return ""
}

// CreationBody is the body a new character of race and sex wears, a race of
// creation's past the game's own: its row's (a female with none the male's), the
// froglok's; 0 when creation knows no such race, or the row has no body.
func CreationBody(race, sex int) int64 {
	if race == raceFroglok {
		return FroglokBody
	}
	d, ok := customRace(race)
	if !ok {
		return 0
	}
	if sex == 1 && d.Body[1] != 0 {
		return d.Body[1]
	}
	return d.Body[0]
}

// labelledRace is a race whose characters a client with race labels is told
// the words of (racelabels.go): a row of racedefs, and the froglok while
// creation offers it (no client but a PS2 running the froglok's S2 has a word
// for race 10, and a server that offers it serves no S2).
func labelledRace(race int) (RaceDef, bool) {
	if d, ok := customRace(race); ok {
		return d, true
	}
	if race == raceFroglok && Creation().Races {
		return RaceDef{ID: raceFroglok, Name: froglok.Name, Short: froglok.Short, Wire: raceFroglok,
			Bonus: froglok.Bonus, Body: froglok.Body, Desc: froglok.Desc, ClassMask: froglok.ClassMask, City: froglok.City}, true
	}
	return RaceDef{}, false
}

// HasRaceLabel reports a race whose characters' race clients with labels are
// told (labelledRace): its login refreshes their lists.
func HasRaceLabel(race int) bool {
	_, ok := labelledRace(race)
	return ok
}

// labelledRaces is every race of labelledRace, by id.
func labelledRaces() []RaceDef {
	defs := RaceDefs()
	if d, ok := labelledRace(raceFroglok); ok {
		defs = append([]RaceDef{d}, defs...)
	}
	return defs
}

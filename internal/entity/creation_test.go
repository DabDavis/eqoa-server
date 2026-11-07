package entity

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// withCreation sets creation's rules and two races of racedefs for a test: a
// giant passing as an ogre with no classes of its own, a cat folk passing as a
// western human with its own (warrior and ranger) and a description.
func withCreation(t *testing.T, r CreationRules) {
	t.Helper()
	SetRaceDefs([]database.RaceDefRow{
		{ID: 11, Name: "Hill Giant", ShortName: "HGT", WireRace: raceOgre, Bonus: [7]int{25, 20, -5, -5}, BodyMale: -5117396, BodyFemale: -5117396},
		{ID: 12, Name: "Kerran", ShortName: "KER", WireRace: 0, Bonus: [7]int{5, 5, 15, 10}, BodyMale: 0x22176258, BodyFemale: 0,
			Desc: "cats", ClassMask: 1<<0 | 1<<1, City: 2},
		{ID: 13, Name: "Goblin", WireRace: 2, City: 1}, // a city on a race passing as no human: none
	})
	SetCreationRules(r)
	t.Cleanup(func() { SetRaceDefs(nil); SetCreationRules(CreationRules{}) })
}

// Off (the Live Server's): the game's ten races and the froglok take any class,
// as they always did, and no race of racedefs is made; on, those are taken, each
// with its classes (its row's, else its wire race's) unless every class is open.
func TestCheckCreation(t *testing.T) {
	type c struct {
		race, class, city int
		ok                bool
	}
	run := func(name string, r CreationRules, cases []c) {
		t.Helper()
		withCreation(t, r)
		for _, k := range cases {
			if why := CheckCreation(k.race, k.class, k.city); (why == "") != k.ok {
				t.Errorf("%s: race %d class %d city %d: %q, want ok %v", name, k.race, k.class, k.city, why, k.ok)
			}
		}
	}
	run("off", CreationRules{}, []c{
		{0, 3, 2, true}, {5, 13, 0, true}, {raceFroglok, 1, 0, true}, // as before: the pages decide
		{11, 0, 0, false}, {12, 0, 0, false}, {0, 15, 0, false}, {0, 0, 3, false}, {31, 0, 0, false},
	})
	run("races", CreationRules{Races: true}, []c{
		{11, 0, 0, true}, {11, 3, 0, true}, {11, 13, 0, false}, // the ogre's: WAR SK SHA NEC
		{12, 1, 0, true}, {12, 2, 0, false}, // its own: WAR RAN
		{13, 3, 0, true}, {13, 1, 0, false}, // the dark elf's
		{14, 0, 0, false}, // no row
		{5, 13, 0, true},  // the game's races as before
	})
	run("all classes", CreationRules{Races: true, AllClasses: true}, []c{
		{11, 13, 0, true}, {12, 2, 0, true}, {13, 14, 0, true}, {14, 0, 0, false}, {11, 15, 0, false},
	})
	run("all classes, no races", CreationRules{AllClasses: true}, []c{
		{11, 0, 0, false}, {5, 13, 0, true},
	})
}

// The races creation tells a client of: none while races are not open; else the
// froglok (a human of the east's, Freeport) and each row, its classes its row's
// or its wire race's, a city only for one passing as a human.
func TestCreationRaces(t *testing.T) {
	withCreation(t, CreationRules{AllClasses: true})
	if got := CreationRaces(); got != nil {
		t.Fatalf("races not open: %v", got)
	}
	withCreation(t, CreationRules{Races: true})
	got := CreationRaces()
	var lines []string
	for _, r := range got {
		lines = append(lines, fmt.Sprintf("%d %s %s like %d city %d mask %#x body %#x/%#x", r.ID, r.Name, r.Short, r.Like, r.City, r.ClassMask,
			uint32(r.Body[0]), uint32(r.Body[1])))
	}
	want := []string{
		"10 Froglok FRG like 0 city 1 mask 0x7e69 body 0x141bfe0c/0x141bfe0c",
		"11 Hill Giant HGT like 9 city 0 mask 0x909 body 0xffb1ea2c/0xffb1ea2c",
		"12 Kerran KER like 0 city 2 mask 0x3 body 0x22176258/0x0",
		"13 Goblin GOB like 2 city 0 mask 0x7e49 body 0x0/0x0",
	}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("races:\n%s\nwant\n%s", lines, want)
	}
	if got[2].Desc != "cats" || got[0].Bonus != [7]int{5, 5, 5, 5, 5, 5, 5} || got[1].Bonus != [7]int{25, 20, -5, -5} {
		t.Errorf("text and bonuses: %+v", got[:3])
	}
}

// A new character of a race of creation's wears its row's body by sex, a female
// with none the male's; the froglok its own; a race of the game's none here (the
// handler's table).
func TestCreationBody(t *testing.T) {
	withCreation(t, CreationRules{Races: true})
	for _, c := range []struct {
		race, sex int
		want      int64
	}{
		{11, 1, -5117396}, {12, 0, 0x22176258}, {12, 1, 0x22176258}, {raceFroglok, 1, FroglokBody}, {3, 0, 0}, {14, 0, 0},
	} {
		if got := CreationBody(c.race, c.sex); got != c.want {
			t.Errorf("race %d sex %d: %#x, want %#x", c.race, c.sex, got, c.want)
		}
	}
}

// The froglok has race labels (and the title line names it) only while creation
// offers it: a server that serves the S2 knowing race 10 offers no creation.
func TestTheFroglokIsLabelledWhereCreationOffersIt(t *testing.T) {
	withCreation(t, CreationRules{AllClasses: true})
	if HasRaceLabel(raceFroglok) || len(labelledRaces()) != 3 {
		t.Errorf("races not open: labelled %v, %d races", HasRaceLabel(raceFroglok), len(labelledRaces()))
	}
	withCreation(t, CreationRules{Races: true})
	d := labelledRaces()
	if !HasRaceLabel(raceFroglok) || len(d) != 4 || d[0].Name != "Froglok" || d[0].Short != "FRG" {
		t.Errorf("races open: %+v", d)
	}
	w := NewWorld()
	viewer := &Player{Name: "Viewer"}
	frog := &Player{Name: "Ribbit", Race: raceFroglok, Class: 3}
	// no master class: the race alone, as a hill giant's "Hill Giant"
	if title, _ := w.TargetInfoLines(viewer, frog); title != "Froglok" {
		t.Errorf("the title to a viewer without labels: %q", title)
	}
	viewer.RaceLabels = true
	if title, _ := w.TargetInfoLines(viewer, frog); title != "" {
		t.Errorf("the title to a viewer with labels: %q", title)
	}
	if !HasRaceLabel(11) || HasRaceLabel(3) {
		t.Error("the control: a row's race labelled, a game race not")
	}
}

// A row's creation columns: a class mask of the fifteen classes, a city only for
// a race passing as a human.
func TestCheckRaceDefCreation(t *testing.T) {
	for _, c := range []struct {
		wire, mask, city int
		ok               bool
	}{
		{0, 0, 2, true}, {9, 0x909, 0, true}, {0, 0x7FFF, 1, true},
		{9, 0, 1, false}, {0, 0x8000, 0, false}, {0, -1, 0, false}, {0, 0, 3, false},
	} {
		if why := CheckRaceDefCreation(c.wire, c.mask, c.city); (why == "") != c.ok {
			t.Errorf("wire %d mask %#x city %d: %q", c.wire, c.mask, c.city, why)
		}
	}
}

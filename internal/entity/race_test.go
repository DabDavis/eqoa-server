package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// hillGiantRow is the row racedefs holds for the Hill Giant, as ~/charmigrate's
// script and database.raceDefFromCode make it.
func hillGiantRow() database.RaceDefRow {
	return database.RaceDefRow{ID: 11, Name: "Hill Giant", ShortName: "HGT", WireRace: raceOgre,
		Bonus: [7]int{25, 20, -5, -5, 0, 0, 0}, BodyMale: -5117396, BodyFemale: -5117396}
}

// withRaces makes rows the races for one test.
func withRaces(t *testing.T, rows ...database.RaceDefRow) {
	t.Helper()
	SetRaceDefs(rows)
	t.Cleanup(func() { SetRaceDefs(nil) })
}

// A Hill Giant (race.go) is an Ogre on the wire and its own race here: the
// state channel, the race's stats and name, the masteries offered it, and the
// target box's title, the one place CLIENT shows what the server writes.
func TestAHillGiantIsAnOgreOnTheWire(t *testing.T) {
	const hillGiant = 11
	withRaces(t, hillGiantRow())
	if WireRace(hillGiant) != raceOgre || WireRace(3) != 3 || WireRace(10) != 10 {
		t.Errorf("wire races %d %d %d", WireRace(hillGiant), WireRace(3), WireRace(10))
	}
	p := &Player{ServerID: 1, Name: "Lagerbottoms", Race: hillGiant, Class: 9, Level: 60, HP: 100, MaxHP: 100}
	if b := SerializePlayerUpdate(1, p); b[144] != raceOgre {
		t.Errorf("the channel's race byte %d, want the Ogre's", b[144])
	}
	want := baseStats{60 + 25 + 0, 60 + 20 + 0, 60 - 5 - 5, 60 - 5 - 10, 60 + 0 + 15, 60 + 0 - 10, 60 + 0 + 10}
	if got := LookupBaseStats(hillGiant, 9); got != want {
		t.Errorf("a Hill Giant cleric's base stats %+v, want %+v", got, want)
	}
	if RaceName(hillGiant) != "Hill Giant" {
		t.Errorf("its name %q", RaceName(hillGiant))
	}
	total := 0
	for _, v := range hillGiantRow().Bonus {
		total += v
	}
	if total != 35 {
		t.Errorf("its bonuses total %d, every race's 35", total)
	}

	offered := func(races int, race int) bool {
		r := &CMRank{Enabled: true, Classes: 32767, Races: races}
		return r.offered(&Player{Class: 9, Race: race})
	}
	for _, c := range []struct {
		what        string
		races, race int
		want        bool
	}{
		{"a general mastery, a Hill Giant", allRacesMask, hillGiant, true},
		{"its own master class", 1 << hillGiant, hillGiant, true},
		{"a gnome's, a Hill Giant", 8, hillGiant, false},
		{"an ogre's, a Hill Giant", 512, hillGiant, false},
		{"its master class, a gnome", 1 << hillGiant, 3, false},
		{"its master class, an ogre", 1 << hillGiant, raceOgre, false},
		{"a gnome's, a gnome", 8, 3, true},
	} {
		if got := offered(c.races, c.race); got != c.want {
			t.Errorf("%s: offered %v, want %v", c.what, got, c.want)
		}
	}

	w := NewWorld()
	w.cms = &cmCatalog{byID: map[int]*CMRank{1951: {ID: 1951, Category: CMCategoryMaster, Mastery: "Titan"}}}
	if title, _ := w.TargetInfoLines(nil, p); title != "Hill Giant" {
		t.Errorf("the title with no master class: %q", title)
	}
	p.CMs = map[int]int{1951: 200}
	if title, _ := w.TargetInfoLines(nil, p); title != "Hill Giant Titan" {
		t.Errorf("the title with one: %q", title)
	}
	gnome := &Player{Race: 3, CMs: map[int]int{1951: 200}}
	if title, _ := w.TargetInfoLines(nil, gnome); title != "Titan" {
		t.Errorf("another race's title: %q", title)
	}

	// a viewer whose client asked for race labels reads the race on its race
	// line (racelabels.go), so the title drops it: both ways, for it and for one
	// that did not ask; an ordinary character's reads the same to both
	shows, ps2 := &Player{RaceLabels: true}, &Player{}
	if title, _ := w.TargetInfoLines(shows, p); title != "Titan" {
		t.Errorf("the title to a client with labels: %q", title)
	}
	if title, _ := w.TargetInfoLines(ps2, p); title != "Hill Giant Titan" {
		t.Errorf("the title to a client without: %q", title)
	}
	if a, _ := w.TargetInfoLines(shows, gnome); a != "Titan" {
		t.Errorf("an ordinary character's title to a client with labels: %q", a)
	}
	if a, _ := w.TargetInfoLines(ps2, gnome); a != "Titan" {
		t.Errorf("an ordinary character's title to a client without: %q", a)
	}
	p.CMs = nil
	if title, _ := w.TargetInfoLines(shows, p); title != "" {
		t.Errorf("no master class, to a client with labels: %q (the client shows the class)", title)
	}
}

// A new race is a row: a second one, with nothing in code, reads through every
// function its number goes to. A race with no row is what it always was: its
// number on the wire as it is, no bonus, no name, and the title as it stands. The
// control is the first race's row in place beside it.
func TestANewRaceIsARow(t *testing.T) {
	const stone, none = 12, 13
	withRaces(t, hillGiantRow(), database.RaceDefRow{ID: stone, Name: "Stone Giant", ShortName: "SGT", WireRace: 5, Bonus: [7]int{10, 0, 0, 0, 10, 0, 15}})
	if WireRace(stone) != 5 || WireRace(11) != raceOgre {
		t.Errorf("wire races %d %d", WireRace(stone), WireRace(11))
	}
	if RaceName(stone) != "Stone Giant" || raceName(stone) != "Stone Giant" {
		t.Errorf("its name %q %q", RaceName(stone), raceName(stone))
	}
	if got, want := LookupBaseStats(stone, 0), (baseStats{60 + 10 + 10, 60 + 15, 60, 60, 60 + 10 - 5, 60 - 10, 60 + 15 - 10}); got != want {
		t.Errorf("its stats %+v, want %+v", got, want)
	}
	if !IsCustomRace(stone) || IsCustomRace(none) || IsCustomRace(9) {
		t.Errorf("custom: %v %v %v", IsCustomRace(stone), IsCustomRace(none), IsCustomRace(9))
	}
	if !raceInMask(allRacesMask, stone) || !raceInMask(1<<stone, stone) || raceInMask(1<<11, stone) {
		t.Errorf("its masks: %v %v %v", raceInMask(allRacesMask, stone), raceInMask(1<<stone, stone), raceInMask(1<<11, stone))
	}
	if got := CustomRaceIDs(); len(got) != 2 || got[0] != 11 || got[1] != stone {
		t.Errorf("the ids %v", got)
	}

	if WireRace(none) != none || RaceName(none) != "#13" || raceName(none) != "race 13" {
		t.Errorf("no row: wire %d, names %q %q", WireRace(none), RaceName(none), raceName(none))
	}
	if got, want := LookupBaseStats(none, 0), LookupBaseStats(-1, 0); got != want {
		t.Errorf("no row: stats %+v, want no race's %+v", got, want)
	}
	w := NewWorld()
	if title, _ := w.TargetInfoLines(nil, &Player{Race: none}); title != "" {
		t.Errorf("no row: the title %q", title)
	}
	// the retail races' tables are as they were
	if RaceName(9) != "Ogre" || RaceName(10) != "Froglok" || LookupBaseStats(9, 0) != (baseStats{60 + 20 + 10, 60 + 15 + 15, 60, 60, 55, 50, 50}) {
		t.Errorf("the Ogre: %q %+v, the Froglok %q", RaceName(9), LookupBaseStats(9, 0), RaceName(10))
	}
}

// A row the game cannot use is left out, and its race is as with no row: a retail
// race's number, past the last, no name, a wire race no client has.
func TestARaceRowTheGameCannotUseIsLeftOut(t *testing.T) {
	good := hillGiantRow()
	retail := good
	retail.ID = 9
	past := good
	past.ID = LastCustomRace + 1
	unnamed := good
	unnamed.ID, unnamed.Name = 12, "  "
	froglok := good
	froglok.ID, froglok.WireRace = 13, raceFroglok
	self := good
	self.ID, self.WireRace = 14, 14
	withRaces(t, good, retail, past, unnamed, froglok, self)
	if got := CustomRaceIDs(); len(got) != 1 || got[0] != 11 {
		t.Fatalf("kept %v", got)
	}
	if RaceName(9) != "Ogre" || WireRace(9) != 9 {
		t.Errorf("a row for the Ogre took: %q %d", RaceName(9), WireRace(9))
	}
	// the short name, when the row has none: its first letters
	short := good
	short.ShortName = ""
	withRaces(t, short)
	if d, _ := customRace(11); d.Short != "HIL" {
		t.Errorf("the short name from its name: %q", d.Short)
	}
}

// A summoning item's Use brings the spell's pet with no cast; with it out it
// brings nothing, and within its rest nothing; a dead owner brings nothing.
func TestAnItemSummonsItsPet(t *testing.T) {
	r := newPetRig(t)
	now := r.at(time.Second)
	spell := fireElemental()
	if got := r.w.SummonByItem(r.sess, spell, now, time.Time{}); got != ItemSummoned {
		t.Fatalf("the first use: %d", got)
	}
	ps := r.w.pets[r.p.ServerID]
	if ps == nil || ps.npc == nil || ps.npc.Name != "Fire Elemental" {
		t.Fatalf("the pet: %+v", ps)
	}
	if got := r.w.SummonByItem(r.sess, spell, now, time.Time{}); got != ItemPetOut {
		t.Errorf("with it out: %d", got)
	}
	ps.npc.Dead = true
	if got := r.w.SummonByItem(r.sess, spell, now, now.Add(time.Minute)); got != ItemPetResting {
		t.Errorf("within its rest: %d", got)
	}
	if got := r.w.SummonByItem(r.sess, spell, now.Add(time.Minute), now.Add(time.Minute)); got != ItemSummoned {
		t.Errorf("after its rest: %d", got)
	}
	r.p.HP = 0
	if got := r.w.SummonByItem(r.sess, spell, now.Add(2*time.Minute), time.Time{}); got != ItemSummonNone {
		t.Errorf("a dead owner: %d", got)
	}
}

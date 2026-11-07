package entity

import (
	"testing"
	"time"
)

// A Hill Giant (race.go) is an Ogre on the wire and its own race here: the
// state channel, the race's stats and name, the masteries offered it, and the
// target box's title, the one place CLIENT shows what the server writes.
func TestAHillGiantIsAnOgreOnTheWire(t *testing.T) {
	if WireRace(RaceHillGiant) != raceOgre || WireRace(3) != 3 || WireRace(10) != 10 {
		t.Errorf("wire races %d %d %d", WireRace(RaceHillGiant), WireRace(3), WireRace(10))
	}
	p := &Player{ServerID: 1, Name: "Lagerbottoms", Race: RaceHillGiant, Class: 9, Level: 60, HP: 100, MaxHP: 100}
	if b := SerializePlayerUpdate(1, p); b[144] != raceOgre {
		t.Errorf("the channel's race byte %d, want the Ogre's", b[144])
	}
	want := baseStats{60 + 25 + 0, 60 + 20 + 0, 60 - 5 - 5, 60 - 5 - 10, 60 + 0 + 15, 60 + 0 - 10, 60 + 0 + 10}
	if got := LookupBaseStats(RaceHillGiant, 9); got != want {
		t.Errorf("a Hill Giant cleric's base stats %+v, want %+v", got, want)
	}
	if RaceName(RaceHillGiant) != "Hill Giant" {
		t.Errorf("its name %q", RaceName(RaceHillGiant))
	}
	total := 0
	for _, v := range raceBonus[RaceHillGiant] {
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
		{"a general mastery, a Hill Giant", allRacesMask, RaceHillGiant, true},
		{"its own master class", 1 << RaceHillGiant, RaceHillGiant, true},
		{"a gnome's, a Hill Giant", 8, RaceHillGiant, false},
		{"an ogre's, a Hill Giant", 512, RaceHillGiant, false},
		{"its master class, a gnome", 1 << RaceHillGiant, 3, false},
		{"its master class, an ogre", 1 << RaceHillGiant, raceOgre, false},
		{"a general mastery, a gnome", allRacesMask, 3, true},
		{"a gnome's, a gnome", 8, 3, true},
	} {
		if got := offered(c.races, c.race); got != c.want {
			t.Errorf("%s: offered %v, want %v", c.what, got, c.want)
		}
	}

	w := NewWorld()
	w.cms = &cmCatalog{byID: map[int]*CMRank{1951: {ID: 1951, Category: CMCategoryMaster, Mastery: "Titan"}}}
	if title, _ := w.TargetInfoLines(p); title != "Hill Giant" {
		t.Errorf("the title with no master class: %q", title)
	}
	p.CMs = map[int]int{1951: 200}
	if title, _ := w.TargetInfoLines(p); title != "Hill Giant Titan" {
		t.Errorf("the title with one: %q", title)
	}
	gnome := &Player{Race: 3, CMs: map[int]int{1951: 200}}
	if title, _ := w.TargetInfoLines(gnome); title != "Titan" {
		t.Errorf("another race's title: %q", title)
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

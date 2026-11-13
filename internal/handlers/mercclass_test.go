package handlers

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The same class up to three times (petition 1920, Hoggieson: "2 or 3 of the
// same class in our grp for merc requits, like 2 monks"): three clerics are
// hired, each a name of its own, a fourth refused as any fourth hire; the
// board's healers' box offers the class as ever, and choosing it again goes
// on to the race.
func TestMercSameClassUpToThree(t *testing.T) {
	r, m, db := newMercFixture(t)
	for i := 0; i < 3; i++ {
		m.hire(r.anna, r.pa, 9, 3, 0)
	}
	rows, _ := db.Mercs(r.pa.CharServerID)
	if len(rows) != 3 {
		t.Fatalf("three clerics: rows %+v", rows)
	}
	names := map[string]bool{}
	for _, row := range rows {
		if row.Class != 9 {
			t.Errorf("row %+v", row)
		}
		names[strings.ToLower(row.Name)] = true
	}
	if len(names) != 3 {
		t.Errorf("names %+v", rows)
	}
	r.drain()
	m.hire(r.anna, r.pa, 9, 3, 0)
	if got := r.msgs(r.anna); !hasMsg(got, "box:Mercenary Recruitment: "+mercFullLine) {
		t.Errorf("a fourth: %q", got)
	}
	// through the board, with one cleric hired: the box is plain, and the
	// cleric chosen again goes on (its path asked)
	r2, m2, _ := newMercFixture(t)
	m2.hire(r2.anna, r2.pa, 9, 3, 0)
	lia := r2.liaison()
	r2.drain()
	r2.talk(r2.anna, lia)
	got := r2.answer(r2.msgs(r2.anna), "A healer")
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Cleric|Druid|Shaman") {
		t.Fatalf("the healers' box: %q", got)
	}
	got = r2.answer(got, "Cleric")
	if len(got) != 1 || !strings.Contains(got[0], "Which path does your Cleric walk") {
		t.Errorf("the cleric chosen again: %q", got)
	}
}

// Each hire's name is its own: mercName passes over the names taken, and a
// login names again a hire whose name an earlier slot's has (the GM Jibrish's
// three "Jorador"), its row kept.
func TestMercNamesDistinct(t *testing.T) {
	at := time.Date(2026, 11, 12, 9, 59, 25, 0, time.UTC)
	first := mercName(44, 9, 3, at)
	second := mercName(44, 9, 3, at, "Jibrish", first)
	third := mercName(44, 9, 3, at, "Jibrish", first, second)
	if first == second || second == third || first == third {
		t.Fatalf("names %q %q %q", first, second, third)
	}
	r, m, db := newMercFixture(t)
	for slot := 1; slot <= 3; slot++ {
		if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Slot: slot, Name: "Jorador", Class: 9, Race: 0, Stance: 2, Hired: at}); err != nil {
			t.Fatal(err)
		}
	}
	m.comeBack(r.anna, r.pa)
	rows, _ := db.Mercs(r.pa.CharServerID)
	seen := map[string]bool{}
	for _, row := range rows {
		seen[strings.ToLower(row.Name)] = true
	}
	if len(rows) != 3 || rows[0].Name != "Jorador" || len(seen) != 3 {
		t.Fatalf("after the login: %+v", rows)
	}
	names := map[string]bool{}
	for _, info := range r.gw.MercInfosOf(r.anna) {
		names[info.Name] = true
	}
	if len(names) != len(r.gw.MercInfosOf(r.anna)) {
		t.Errorf("out: %v", r.gw.MercInfosOf(r.anna))
	}
}

// A healer's mode (the owner, 2026-11-12): defensive when hired, kept in its
// row by /merc offensive and defensive, said by /merc; a warrior has none.
func TestMercHealerMode(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 9, 0, 0)
	m.hire(r.anna, r.pa, 0, 0, 0)
	r.gw.AskMercGroup(r.anna)
	rows, _ := db.Mercs(r.pa.CharServerID)
	if len(rows) != 2 || rows[0].Offensive {
		t.Fatalf("rows %+v", rows)
	}
	r.drain()
	r.say(r.anna, "/merc 1 offensive")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); !row.Offensive {
		t.Error("offensive not kept")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+mercModeWords[true]) {
		t.Errorf("/merc 1 offensive: %q", got)
	}
	r.say(r.anna, "/merc 2 offensive")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+mercModeHealersLine) {
		t.Errorf("a warrior offensive: %q", got)
	}
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !slices.ContainsFunc(got, func(m string) bool {
		return strings.HasPrefix(m, "chat:1. Your mercenary "+rows[0].Name) && strings.HasSuffix(m, ", assist, offensive.")
	}) {
		t.Errorf("/merc: %q", got)
	}
	r.say(r.anna, "/merc defensive")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Offensive {
		t.Error("defensive not kept")
	}
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if info.Offensive {
			t.Errorf("slot %d offensive", info.Slot)
		}
	}
}

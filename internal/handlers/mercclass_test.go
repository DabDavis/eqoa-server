package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// One of each class a player (the owner, 2026-11-12: "for mercs players can
// only choose 1 of each class"): a second cleric is refused, named after the
// first, and the liaison's class box marks the class held; another class is
// hired.
func TestMercOneOfEachClass(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 9, 3, 0)
	rows, _ := db.Mercs(r.pa.CharServerID)
	if len(rows) != 1 {
		t.Fatalf("rows %+v", rows)
	}
	r.drain()
	m.hire(r.anna, r.pa, 9, 3, 0)
	want := "box:Mercenary Recruitment: " + fmt.Sprintf(mercClassTakenLine, "Cleric", rows[0].Name, rows[0].Name)
	if got := r.msgs(r.anna); !hasMsg(got, want) {
		t.Errorf("a second cleric: %q, want %q", got, want)
	}
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 1 {
		t.Fatalf("a second cleric made a row: %+v", rows)
	}
	// through the board: the healers' box marks the cleric, and choosing it
	// is refused before the race is asked
	lia := r.liaison()
	r.drain()
	r.talk(r.anna, lia)
	got := r.answer(r.msgs(r.anna), "A healer")
	held := "Cleric (yours: " + rows[0].Name + ")"
	if len(got) != 1 || !strings.HasSuffix(got[0], "|"+held+"|Druid|Shaman") {
		t.Fatalf("the healers' box: %q", got)
	}
	got = r.answer(got, held)
	if !hasMsg(got, want) {
		t.Errorf("the cleric chosen: %q", got)
	}
	m.hire(r.anna, r.pa, 7, 3, 0)
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 2 || rows[1].Class != 7 {
		t.Fatalf("a druid: %+v", rows)
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

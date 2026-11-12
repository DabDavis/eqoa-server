package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Three mercs a character (the owner, 2026-11-12): the liaison hires into the
// first free slot and no fourth; /merc lists them by slot; an order names one
// by its slot or its name (first or last), else every one out; a dismissal of
// several asks which, and one named ends its row alone.
func TestThreeMercsByCommand(t *testing.T) {
	r, m, db := newMercFixture(t)
	for _, class := range []int{0, 9, 13} {
		m.hire(r.anna, r.pa, class, 0, 0)
	}
	r.gw.AskMercGroup(r.anna)
	rows, err := db.Mercs(r.pa.CharServerID)
	if err != nil || len(rows) != 3 || rows[0].Slot != 1 || rows[1].Slot != 2 || rows[2].Slot != 3 || rows[2].Class != 13 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	if got := r.gw.MercInfosOf(r.anna); len(got) != 3 {
		t.Fatalf("%d out, want 3", len(got))
	}
	r.drain()
	m.hire(r.anna, r.pa, 4, 0, 0)
	if got := r.msgs(r.anna); !hasMsg(got, "box:Mercenary Recruitment: "+mercFullLine) {
		t.Errorf("a fourth hire: %q", got)
	}
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 3 {
		t.Fatalf("a fourth hire made a row: %d", len(rows))
	}
	r.drain()
	r.say(r.anna, "/merc")
	got := r.msgs(r.anna)
	if !hasMsg(got, "chat:Your mercenaries, 3 of 3:") || !hasMsg(got, "chat:2. Your mercenary "+rows[1].Name+"*") ||
		!hasMsg(got, "chat:3. Your mercenary "+rows[2].Name+"*") {
		t.Errorf("/merc: %q", got)
	}
	// "/merc 2 stance passive": the second alone, its row alone
	r.say(r.anna, "/merc 2 stance passive")
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if want := info.Slot == 2; (info.Stance == 1) != want {
			t.Errorf("after /merc 2 stance passive, slot %d is %v", info.Slot, info.Stance)
		}
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 2); row.Stance != 1 {
		t.Errorf("slot 2's stance not kept: %d", row.Stance)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Stance == 1 {
		t.Error("slot 1's row took slot 2's stance")
	}
	// by name, last: "/merc stance aggressive <name>"
	r.say(r.anna, "/merc stance aggressive "+strings.ToLower(rows[2].Name))
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if want := info.Slot == 3; (info.Stance == 3) != want {
			t.Errorf("after the stance by name, slot %d is %v", info.Slot, info.Stance)
		}
	}
	// no name: every one out
	r.say(r.anna, "/merc stance assist")
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if info.Stance != 2 {
			t.Errorf("/merc stance assist left slot %d %v", info.Slot, info.Stance)
		}
	}
	// a dismissal of several asks which
	r.drain()
	r.say(r.anna, "/merc dismiss")
	if got := r.msgs(r.anna); len(r.gw.MercInfosOf(r.anna)) != 3 || !hasMsg(got, "red:Name which mercenary: /merc <name or 1-3> dismiss.*") {
		t.Errorf("/merc dismiss with three: %q", got)
	}
	r.say(r.anna, "/t merc 2 dismiss")
	if got := r.gw.MercInfosOf(r.anna); len(got) != 2 || got[0].Slot != 1 || got[1].Slot != 3 {
		t.Fatalf("after /t merc 2 dismiss: %+v", got)
	}
	if _, kept, _ := db.Merc(r.pa.CharServerID, 2); kept {
		t.Error("slot 2's row kept")
	}
	r.say(r.anna, "/merc dismiss "+rows[0].Name)
	if got := r.gw.MercInfosOf(r.anna); len(got) != 1 || got[0].Slot != 3 {
		t.Fatalf("after the dismissal by name: %+v", got)
	}
	// a new hire takes the first free slot
	m.hire(r.anna, r.pa, 7, 0, 0)
	if row, kept, _ := db.Merc(r.pa.CharServerID, 1); !kept || row.Class != 7 {
		t.Errorf("the new hire's slot: %+v %v", row, kept)
	}
	// one hired: /merc dismiss needs no name
	r.say(r.anna, "/merc dismiss 1")
	r.say(r.anna, "/merc dismiss")
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 0 || len(r.gw.MercInfosOf(r.anna)) != 0 {
		t.Errorf("after the last dismissals: %d rows", len(rows))
	}
}

// A login brings every hire back, by slot; one with no place in its owner's
// group goes on standby, and /merc says so.
func TestMercLoginBringsEveryHire(t *testing.T) {
	r, m, db := newMercFixture(t)
	for i, name := range []string{"Ralthas", "Ullin", "Mirra"} {
		if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Slot: i + 1, Name: name, Class: 0, Race: 0, Stance: 2}); err != nil {
			t.Fatal(err)
		}
	}
	// Anna in a group with two other players: a place for one merc
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pb))
	r.gw.GroupAccept(r.bert, r.gw.IDSeenBy(r.bert, r.pa))
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pc))
	r.gw.GroupAccept(r.cora, r.gw.IDSeenBy(r.cora, r.pa))
	r.drain()
	m.comeBack(r.anna, r.pa)
	out := r.gw.MercInfosOf(r.anna)
	if len(out) != 1 || out[0].Name != "Ralthas" {
		t.Fatalf("out: %+v", out)
	}
	sb := r.gw.MercStandby(r.anna)
	if len(sb) != 2 || sb[2] != "Ullin" || sb[3] != "Mirra" {
		t.Fatalf("standby: %v", sb)
	}
	r.drain()
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:3. Your mercenary Mirra, a Human Warrior, is on standby*") {
		t.Errorf("/merc: %q", got)
	}
}

package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// /merc were (petition 3069): only the were forms its hirer's own book holds; given, kept
// in its row and on its kit (the World puts it on: entity/mercwere.go); none takes it
// away; with no merc named, every hire takes it.
func TestMercWereCommand(t *testing.T) {
	r, m, db := newMercFixture(t)
	r.pa.Level = 60
	m.hire(r.anna, r.pa, 0, 0, 0) // a human warrior
	r.gw.AskMercGroup(r.anna)
	rows, _ := db.Mercs(r.pa.CharServerID)
	name := rows[0].Name
	for _, s := range []struct {
		id   int
		name string
	}{{901824, "Wolfwere"}, {4412, "Dire Wolf Form"}, {901804, "Bearwere"}} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, 30, 0, 0, 0, 3, 0, 0, 0, 0, 3, 0, ?, 'Assume the form of a beast.')`, s.id, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Self, Gives Form, Increase STR by 45', 0, 'test')`, s.id); err != nil {
			t.Fatal(err)
		}
	}
	r.drain()

	r.say(r.anna, "/merc were wolf")
	if got := r.msgs(r.anna); !hasMsg(got, "red:Your book holds no were form: *") {
		t.Fatalf("no form in the book: %q", got)
	}
	store := database.NewSpellbookStore(db)
	for i, id := range []int{901824, 4412} {
		if err := store.AddSpell(r.pa.CharServerID, id, 100+i); err != nil {
			t.Fatal(err)
		}
	}
	r.say(r.anna, "/merc were")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+" takes no were form.") ||
		!hasMsg(got, "chat:Your mercenaries may take: Wolfwere, Dire Wolf Form. *") {
		t.Fatalf("the list: %q", got)
	}
	r.say(r.anna, "/merc were wolf") // both hold "wolf"
	if got := r.msgs(r.anna); !hasMsg(got, `red:No were form of yours is called "wolf". Yours: Wolfwere, Dire Wolf Form.`) {
		t.Fatalf("an unclear name: %q", got)
	}
	r.say(r.anna, "/merc were bearwere") // not in the book
	if got := r.msgs(r.anna); !hasMsg(got, `red:No were form of yours is called "bearwere". *`) {
		t.Fatalf("a form not held: %q", got)
	}
	r.say(r.anna, "/merc were dire wolf")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+" takes the form of Dire Wolf Form.") {
		t.Fatalf("given: %q", got)
	}
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if row.Were != "Dire Wolf Form" {
		t.Fatalf("its row's form %q", row.Were)
	}
	k, _ := m.kit(row, 60)
	if k.Were == nil || k.Were.SpellID != 4412 {
		t.Fatalf("its kit's form %+v", k.Were)
	}
	if info, ok := r.gw.MercInfoOf(r.anna); !ok || info.Were != "Dire Wolf Form" {
		t.Errorf("the merc out: %+v", info)
	}
	r.say(r.anna, "/merc")
	said := false
	got := r.msgs(r.anna)
	for _, l := range got {
		said = said || strings.Contains(l, ", in Dire Wolf Form")
	}
	if !said {
		t.Errorf("/merc says no form: %q", got)
	}
	// out of the book: kept in its row, not worn
	if _, err := db.Exec(`DELETE FROM spells WHERE serverid = ? AND spellid = 4412`, r.pa.CharServerID); err != nil {
		t.Fatal(err)
	}
	if k, _ := m.kit(row, 60); k.Were != nil {
		t.Errorf("a form out of its hirer's book still on its kit: %q", k.Were.Name)
	}
	r.say(r.anna, "/merc were")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+"'s were form Dire Wolf Form is not in your book now: *") {
		t.Errorf("a form no longer held: %q", got)
	}
	r.say(r.anna, "/merc were none")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+" leaves its were form and is itself again.") {
		t.Fatalf("none: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Were != "" {
		t.Errorf("its row's form %q after none", row.Were)
	}
}

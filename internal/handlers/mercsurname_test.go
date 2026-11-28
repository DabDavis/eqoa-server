package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// mercsurname.go: a merc's surname (/merc surname, petition 3430): kept in its
// row and audited, held to a character's surname's rules, logged, told to the
// clients that draw surnames by the merc's name, carried over a rename, and in
// the list a client is sent when it answers the offer.
func TestMercSurname(t *testing.T) {
	r, m, db := newMercFixture(t)
	s := NewSurnames(r.gw, db, r.sm)
	s.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	was := surnamesH.Load()
	surnamesH.Store(s)
	t.Cleanup(func() { surnamesH.Store(was) })
	r.cora.SetSurnames(true) // a viewer whose client draws surnames
	m.hire(r.anna, r.pa, 0, 0, 0)
	info, ok := r.gw.MercInfoOf(r.anna)
	if !ok {
		t.Fatal("no merc hired")
	}
	r.drain()

	r.say(r.anna, "/merc surname stoneheart")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+fmt.Sprintf(mercSurnameTaken, info.Name, "Stoneheart")) {
		t.Fatalf("/merc surname stoneheart: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Surname != "Stoneheart" {
		t.Errorf("its row: %q", row.Surname)
	}
	if got := surnamesSent(t, r.cora); len(got) != 1 || got[0] != info.Name+"=Stoneheart" {
		t.Errorf("told the viewers: %q", got)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'charmerc' AND via = 'game' AND detail LIKE '/merc surname:%Stoneheart%'`).Scan(&audits); err != nil || audits != 1 {
		t.Errorf("its audit lines: %d %v", audits, err)
	}
	if pairs := s.inWorld(); !hasPair(pairs, info.Name, "Stoneheart") {
		t.Errorf("the list a client is sent: %+v", pairs)
	}

	// the refusals: its own name, its owner's, not one word, the filter's
	for _, c := range []struct{ asked, line string }{
		{info.Name, fmt.Sprintf(mercSurnameOwnName, info.Name)},
		{"anna", fmt.Sprintf(renameCharacter, "Anna")},
		{"two words", ""},
		{"x1", ""},
	} {
		r.drain()
		r.say(r.anna, "/merc 1 surname "+c.asked)
		got := r.msgs(r.anna)
		if c.line != "" && !hasMsg(got, "red:"+c.line) {
			t.Errorf("/merc 1 surname %s: %q", c.asked, got)
		}
		if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Surname != "Stoneheart" {
			t.Errorf("after %q its surname is %q", c.asked, row.Surname)
		}
	}

	// a rename carries it, its old name's pair emptied
	r.drain()
	r.say(r.anna, "/merc rename Rennis")
	if got := surnamesSent(t, r.cora); strings.Join(got, ",") != info.Name+"=,Rennis=Stoneheart" {
		t.Errorf("told at a rename: %q", got)
	}

	// cleared, by its name
	r.drain()
	r.say(r.anna, "/merc Rennis surname clear")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+fmt.Sprintf(mercSurnameCleared, "Rennis", "Stoneheart")) {
		t.Fatalf("/merc Rennis surname clear: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Surname != "" {
		t.Errorf("cleared, its row: %q", row.Surname)
	}
	if got := surnamesSent(t, r.cora); len(got) != 1 || got[0] != "Rennis=" {
		t.Errorf("told at the clear: %q", got)
	}
	if !mercWords("Rennis surname Vale") || !mercWords("surname Vale") {
		t.Error("a PS2's tell of a merc surname not taken as /merc's")
	}
	log := renameLog(t, db, r.pa.CharServerID)
	if len(log) == 0 || log[0] != "renamed merc surname stoneheart: " {
		t.Errorf("the surnames logged: %q", log)
	}
}

// hasPair reports whether the pairs hold name's surname.
func hasPair(pairs []surnamePair, name, surname string) bool {
	for _, p := range pairs {
		if p.name == name && p.surname == surname {
			return true
		}
	}
	return false
}

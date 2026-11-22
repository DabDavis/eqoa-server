package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// ascendedFixture is a scribe fixture with a base spell 9001 "Fingers of Test"
// and its Ascended spell 9002 (table spellascended), both level 4, and the two
// scrolls (base and Ascended) for a druid.
func ascendedFixture(t *testing.T) (f *scribeFixture, baseScroll, ascScroll int) {
	f = newScribeFixture(t)
	f.spell(1000, "Known A", 1)
	f.know(1000, 0)
	f.spell(9001, "Fingers of Test", 4)
	f.spell(9002, "Ascended Fingers of Test", 4)
	if _, err := f.db.Exec(`CREATE TABLE IF NOT EXISTS spellascended (spellid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE,
		made TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO spellascended (spellid, base) VALUES (9002, 9001)`); err != nil {
		t.Fatal(err)
	}
	return f, f.item("Fingers of Test", true, druidMask, 4, 1), f.item("Ascended Fingers of Test", true, druidMask, 4, 1)
}

func chatOf(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		if len(m) > 6 && protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == protocol.OpcodeColoredChat {
			text, _ := readWideLine(m[2:])
			out = append(out, text)
		}
	}
	return out
}

// Scribing an Ascended scroll with its base in the book (board 1645): the base's
// row out, the Ascended spell in at its display index and hot bar place, the
// scroll taken once, 0x00B8 then 0x00B3 told, and the line said.
func TestAnAscendedScrollReplacesItsBase(t *testing.T) {
	f, _, asc := ascendedFixture(t)
	if _, err := f.db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
		VALUES (?, 9001, 1, 1, 3, 0, -1)`, f.sid); err != nil {
		t.Fatal(err)
	}
	f.know(1000+1, 2) // a row after it stays
	f.own(asc, 1)
	f.loadList()
	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAbilityRemove, protocol.OpcodeAbilityAdd, protocol.OpcodeColoredChat)
	if got := chatOf(msgs); !slices.Equal(got, []string{"Ascended Fingers of Test replaces Fingers of Test in your spell book."}) {
		t.Errorf("said %q", got)
	}
	want := []database.SpellRow{{SpellID: 1000, AddedOrder: 0, WhereOnHotBar: -1}, {SpellID: 9002, AddedOrder: 1, OnHotBar: 1, WhereOnHotBar: 3},
		{SpellID: 1001, AddedOrder: 2, WhereOnHotBar: -1}}
	got := f.book()
	slices.SortFunc(got, func(a, b database.SpellRow) int { return a.AddedOrder - b.AddedOrder })
	if !slices.Equal(got, want) {
		t.Errorf("the book %+v, want %+v", got, want)
	}
	if n := len(f.rowsOf()); n != 0 {
		t.Errorf("%d items left, want the scroll taken", n)
	}
}

// Once the Ascended spell is known, the base's scroll is refused ("You already
// know this spell.") and kept; with no base in the book an Ascended scroll is
// added at the end as any scroll is.
func TestTheBaseScrollIsRefusedAndAPlainAdd(t *testing.T) {
	f, base, asc := ascendedFixture(t)
	f.own(asc, 1)
	f.own(base, 1)
	f.loadList()
	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAbilityAdd)
	if r := f.book(); len(r) != 2 || r[1].SpellID != 9002 || r[1].AddedOrder != 1 {
		t.Fatalf("the plain add: %+v", r)
	}
	f.loadList()
	msgs = f.use(0)
	if got := chatOf(msgs); !slices.Equal(got, []string{scribeKnown}) {
		t.Errorf("the base scroll: said %q", got)
	}
	if len(f.rowsOf()) != 1 || len(f.book()) != 2 {
		t.Errorf("the base scroll: %d items, %d spells; want it kept and the book as it was", len(f.rowsOf()), len(f.book()))
	}
}

// The swap rolls back whole: an insert of the Ascended spell refused (a trigger)
// leaves the base's row and the scroll.
func TestTheAscendedSwapRollsBackWhole(t *testing.T) {
	f, _, asc := ascendedFixture(t)
	f.know(9001, 1)
	if _, err := f.db.Exec(`CREATE TRIGGER no_asc BEFORE INSERT ON spells WHEN NEW.spellid = 9002 BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatal(err)
	}
	f.own(asc, 1)
	f.loadList()
	f.use(0)
	if r := f.book(); len(r) != 2 || !slices.ContainsFunc(r, func(s database.SpellRow) bool { return s.SpellID == 9001 }) {
		t.Errorf("after the failed swap: %+v", r)
	}
	if len(f.rowsOf()) != 1 {
		t.Error("the scroll was taken")
	}
}

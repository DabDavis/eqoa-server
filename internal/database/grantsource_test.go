package database

import (
	"fmt"
	"testing"
)

func TestGotByTheSource(t *testing.T) {
	for _, c := range []struct{ source, got, master string }{
		{"eqoa-data cm", "mastery", ""},
		{"EQOA-Data CM", "mastery", ""},
		{"cm", "mastery", ""},
		{"eqoa-data epic", "epic", ""},
		{"eqoa-data mca", "master class", ""},
		{"eqoa-data master Lich", "master class", "Lich"},
		{"eqoa-data master Storm Caller", "master class", "Storm Caller"},
		{"master Hero", "master class", "Hero"},
		// a spell a character of the class gets by level
		{"eqoa-data", "", ""}, {"dump", "", ""}, {"vendor", "", ""}, {"", "", ""}, {"test", "", ""},
		{"eqoa-data cleric", "", ""}, {"mastermind", "", ""},
	} {
		if got := GotBy(c.source); got != c.got {
			t.Errorf("GotBy(%q) = %q, want %q", c.source, got, c.got)
		}
		if m := MasterOf(c.source); m != c.master {
			t.Errorf("MasterOf(%q) = %q, want %q", c.source, m, c.master)
		}
		if Granted(c.source) != (c.got == "") {
			t.Errorf("Granted(%q) = %v", c.source, Granted(c.source))
		}
	}
}

// A mastery's, an epic's or a master class's row in a class's list is never
// given by a class grant (#template, start_all_spells): none can be bought
// or earned here. The control: the same spell on a leveled row is given.
func TestGrantClassSpellsSkipsWhatNoLevelGives(t *testing.T) {
	for _, c := range []struct {
		source string
		want   string
	}{
		{"eqoa-data cm", "[875]"},
		{"eqoa-data epic", "[875]"},
		{"eqoa-data master Lich", "[875]"},
		{"eqoa-data mca", "[875]"},
		{"eqoa-data", "[875 700900]"}, // the control: a leveled row is given
	} {
		t.Run(c.source, func(t *testing.T) {
			db, s := newCharDB(t)
			for _, q := range []string{
				`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
				 VALUES (875, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Disease Cloud', 'A cloud of infection harms your enemy.'),
				        (700900, 60, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Lich', 'Self, Increase Power per tic by 40')`,
				`INSERT INTO classspells (class, spellid, source) VALUES (11, 875, 'dump')`,
			} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (11, 700900, ?)`, c.source); err != nil {
				t.Fatal(err)
			}
			ch := &CharacterRecord{CharName: "Necro", AccountID: 1, TClass: 11}
			if err := s.Create(ch); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GrantClassSpells(ch.ServerID, 11); err != nil {
				t.Fatal(err)
			}
			var ids []int
			for _, row := range book(t, db, ch.ServerID) {
				ids = append(ids, row.spell)
			}
			if fmt.Sprint(ids) != c.want {
				t.Errorf("book %v, want %s", ids, c.want)
			}
		})
	}
}

// A pet taps with what its master gets by level: a mastery's tap on the
// class's list is not one of ClassSpellLines'. The control: a leveled one is.
func TestClassSpellLinesLeaveOutWhatNoLevelGives(t *testing.T) {
	db, _ := newCharDB(t)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (1, 20, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Siphon Life', ''),
		        (700901, 55, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Group Life Tap', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (11, 1, 'eqoa-data'), (11, 700901, 'eqoa-data cm')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := db.ClassSpellLines(11)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].SpellID != 1 {
		t.Errorf("lines %+v, want Siphon Life alone", lines)
	}
}

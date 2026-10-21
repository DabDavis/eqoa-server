package database

import (
	"reflect"
	"testing"
)

// Kept effects round the table: their order and every field, a second save
// replacing the first, an empty one leaving none.
func TestBuffsRoundTrip(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/buffs.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	rows := []BuffRow{
		{SpellID: 900, Name: "Stoneskin", Icon: 0x727C44F8, Caster: 7, LeftMS: 1792400, TickMS: -1, Mods: `{"AC":150}`},
		{SpellID: 457, Name: "Mend", Icon: 0x828563AF, Caster: 8, LeftMS: 22400, TickMS: 4400, HPTick: 35, PowerTick: 2, Mods: "{}", Flags: 12},
	}
	if err := s.SaveBuffs(7, rows); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBuffs(9, rows[:1]); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadBuffs(7)
	if err != nil || !reflect.DeepEqual(got, rows) {
		t.Fatalf("loaded %+v (%v)", got, err)
	}
	if err := s.SaveBuffs(7, rows[1:]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadBuffs(7); len(got) != 1 || got[0].SpellID != 457 {
		t.Errorf("after a second save: %+v", got)
	}
	if err := s.SaveBuffs(7, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadBuffs(7); len(got) != 0 {
		t.Errorf("after an empty save: %+v", got)
	}
	if got, _ := s.LoadBuffs(9); len(got) != 1 {
		t.Errorf("another character's: %+v", got)
	}
}

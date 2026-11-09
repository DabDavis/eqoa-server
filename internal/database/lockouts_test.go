package database

import (
	"testing"
	"time"
)

// A character's lockouts kept by spell, each by when it ends: a load reads
// those still running at its moment, so the time between a save and a load
// (the character logged out) counts, and one over by the load is nothing. A
// save replaces the character's set (one over or gone from it is gone from
// the table) and leaves another character's alone (the control), a spell
// given twice keeps the later end, and deleting the character deletes its
// rows and no one else's.
func TestLockoutsAreKeptBySpellOnTheWallClock(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/lockouts.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewSQLiteCharacterStore(db)
	a := &CharacterRecord{CharName: "Recaster", AccountID: 1, Level: 50}
	b := &CharacterRecord{CharName: "Bystander", AccountID: 1, Level: 50}
	for _, c := range []*CharacterRecord{a, b} {
		if err := store.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	now := time.UnixMilli(time.Now().UnixMilli())
	if err := store.SaveLockouts(a.ServerID, []LockoutRow{
		{SpellID: 901, Until: now.Add(10 * time.Minute)},
		{SpellID: 902, Until: now.Add(2 * time.Second)},
		{SpellID: 901, Until: now.Add(5 * time.Minute)}, // the same spell again: the later end stays
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLockouts(b.ServerID, []LockoutRow{{SpellID: 901, Until: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}

	got, err := store.LoadLockouts(a.ServerID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SpellID != 901 || !got[0].Until.Equal(now.Add(10*time.Minute)) ||
		got[1].SpellID != 902 || !got[1].Until.Equal(now.Add(2*time.Second)) {
		t.Fatalf("read at once: %+v; want 901 for 10 minutes and 902 for 2 s", got)
	}
	// logged out 3 s: 902 is over, 901 still runs
	if got, _ = store.LoadLockouts(a.ServerID, now.Add(3*time.Second)); len(got) != 1 || got[0].SpellID != 901 {
		t.Fatalf("read 3 s on: %+v; want 901 alone", got)
	}
	// logged out 11 minutes: nothing is locked out any more
	if got, _ = store.LoadLockouts(a.ServerID, now.Add(11*time.Minute)); len(got) != 0 {
		t.Fatalf("read 11 minutes on: %+v; want none", got)
	}

	// a save of fewer replaces the set
	if err := store.SaveLockouts(a.ServerID, []LockoutRow{{SpellID: 902, Until: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.LoadLockouts(a.ServerID, now); len(got) != 1 || got[0].SpellID != 902 || !got[0].Until.Equal(now.Add(time.Hour)) {
		t.Fatalf("after a save of 902 alone: %+v", got)
	}
	// and a save of none leaves none
	if err := store.SaveLockouts(a.ServerID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.LoadLockouts(a.ServerID, now); len(got) != 0 {
		t.Fatalf("after a save of none: %+v", got)
	}
	// the control: the other character's lockout was never touched
	if got, _ = store.LoadLockouts(b.ServerID, now); len(got) != 1 || got[0].SpellID != 901 || !got[0].Until.Equal(now.Add(time.Minute)) {
		t.Fatalf("the other character's: %+v", got)
	}

	// deleting a character deletes its rows, and no one else's
	if err := store.SaveLockouts(a.ServerID, []LockoutRow{{SpellID: 903, Until: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(a.ServerID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charlockouts WHERE serverid = ?`, a.ServerID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("a deleted character's rows: %d (%v)", left, err)
	}
	if got, _ = store.LoadLockouts(b.ServerID, now); len(got) != 1 {
		t.Fatalf("deleting one character took another's: %+v", got)
	}
}

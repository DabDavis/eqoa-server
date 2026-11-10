package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

func newSafeDB(t *testing.T) (*database.SQLiteDB, *database.SQLiteCharacterStore) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/safe.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); SetTestInvis(false) })
	return db, database.NewSQLiteCharacterStore(db)
}

// Petition 1424 (the GM Jibrish): /testinvis is everyone's. A player with no GM
// or tester standing turns safe mode on and off with each spelling
// ("/testinvis", "#testinvis", a PS2's "/t testinvis", any case), told so to
// itself alone, the mode kept in charsafemode; "/t testinvis hello" is a tell.
func TestTestInvisForEveryone(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetSafeModeStore(db)
	s, p := c.join(5, "Plain", 100, 100, 0)
	other, _ := c.join(6, "Near", 101, 100, 0)
	SetTestInvis(true)
	want := true
	for _, word := range []string{"/testinvis", "#testinvis", "/t testinvis", "/TESTINVIS", "/tell testinvis", "#TestInvis"} {
		c.say(s, word)
		line := safeOffLine
		if want {
			line = safeOnLine
		}
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != line {
			t.Fatalf("%s: told %q, want %q", word, got, line)
		}
		if got := heard(t, other); len(got) != 0 {
			t.Fatalf("%s reached another player: %q", word, texts(got))
		}
		if on, ok := c.gw.SafeMode(s); !ok || on != want {
			t.Fatalf("%s: safe mode %v, want %v", word, on, want)
		}
		if kept, err := db.SafeMode(p.CharServerID); err != nil || kept != want {
			t.Fatalf("%s: kept %v (%v), want %v", word, kept, err, want)
		}
		s.Queue().GetPending()
		want = !want
	}
	c.say(s, "/t testinvis hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "testinvis is not connected." {
		t.Fatalf("a tell read as /testinvis: %q", got)
	}
	if on, _ := c.gw.SafeMode(s); on {
		t.Error("the tell changed safe mode")
	}
}

// With world.testinvis off the command says so and changes nothing.
func TestTestInvisOffOnTheServer(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetSafeModeStore(db)
	s, p := c.join(5, "Plain", 100, 100, 0)
	SetTestInvis(false)
	c.say(s, "/testinvis")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != safeOffServer {
		t.Fatalf("told %q, want %q", got, safeOffServer)
	}
	if on, _ := c.gw.SafeMode(s); on {
		t.Error("safe mode turned on with the switch off")
	}
	if kept, _ := db.SafeMode(p.CharServerID); kept {
		t.Error("safe mode kept with the switch off")
	}
}

// A login in safe mode (its charsafemode row) is in it, and told so once in the
// world; one with no row is not; with world.testinvis off a login leaves the
// mode, its row deleted.
func TestASafeModeOutlivesALogin(t *testing.T) {
	db, chars := newSafeDB(t)
	c := newChatWorld(t)
	h := &CharacterHandler{store: chars, gameWorld: c.gw}
	SetTestInvis(true)
	s, p := c.join(5, "Plain", 100, 100, 0)
	plain, _ := c.join(6, "Other", 101, 100, 0)
	if err := db.SetSafeMode(p.CharServerID, true); err != nil {
		t.Fatal(err)
	}
	h.restoreSafeMode(s, &database.CharacterRecord{ServerID: p.CharServerID, CharName: p.Name})
	h.restoreSafeMode(plain, &database.CharacterRecord{ServerID: 6, CharName: "Other"})
	if on, _ := c.gw.SafeMode(s); !on {
		t.Fatal("a login with its row not in safe mode")
	}
	if on, _ := c.gw.SafeMode(plain); on {
		t.Fatal("a login with no row in safe mode")
	}
	tell := tellSafeMode(c.gw)
	tell(s, p)
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != safeLoginLine {
		t.Fatalf("the login told %q, want %q", got, safeLoginLine)
	}
	if got := heard(t, plain); len(got) != 0 {
		t.Fatalf("a login out of safe mode told %q", texts(got))
	}

	// the switch off: the next login leaves it
	c.gw.SetSafeMode(s, false)
	SetTestInvis(false)
	h.restoreSafeMode(s, &database.CharacterRecord{ServerID: p.CharServerID, CharName: p.Name})
	if on, _ := c.gw.SafeMode(s); on {
		t.Error("a login with the switch off put in safe mode")
	}
	if kept, _ := db.SafeMode(p.CharServerID); kept {
		t.Error("the row kept by a login with the switch off")
	}
}

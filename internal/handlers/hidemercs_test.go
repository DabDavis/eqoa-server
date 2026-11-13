package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 1858 (the GM Jibrish): /hide mercs and /show mercs are everyone's.
// Each spelling ("/hide mercs", "#hide mercs", a PS2's "/t hide mercs", any
// case, and the same for show) turns the viewer's hide the way it names,
// answered to the player alone and kept in charhidemercs; "/t hide hello" and
// "/t show me" are tells, and "/hide" with no "mercs" changes nothing.
func TestHideMercsSpellings(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetMercsHiddenStore(db)
	s, p := c.join(5, "Plain", 100, 100, 0)
	other, _ := c.join(6, "Near", 101, 100, 0)
	for _, tc := range []struct {
		line string
		hide bool
	}{
		{"/hide mercs", true}, {"/show mercs", false},
		{"#hide mercs", true}, {"#show mercs", false},
		{"/t hide mercs", true}, {"/t show mercs", false},
		{"/HIDE Mercs", true}, {"/tell show MERCS", false},
		{"#Hide mercs", true}, {"#SHOW mercs", false},
	} {
		c.say(s, tc.line)
		want := mercsShownLine
		if tc.hide {
			want = mercsHiddenLine
		}
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != want {
			t.Fatalf("%s: told %q, want %q", tc.line, got, want)
		}
		if got := heard(t, other); len(got) != 0 {
			t.Fatalf("%s reached another player: %q", tc.line, texts(got))
		}
		if on, ok := c.gw.MercsHidden(s); !ok || on != tc.hide {
			t.Fatalf("%s: hidden %v, want %v", tc.line, on, tc.hide)
		}
		if kept, err := db.MercsHidden(p.CharServerID); err != nil || kept != tc.hide {
			t.Fatalf("%s: kept %v (%v), want %v", tc.line, kept, err, tc.hide)
		}
		s.Queue().GetPending()
	}
	c.say(s, "/t hide hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "hide is not connected." {
		t.Fatalf("a tell read as /hide mercs: %q", got)
	}
	s.Queue().GetPending()
	c.say(s, "/hide")
	c.say(s, "/hide everything")
	if on, _ := c.gw.MercsHidden(s); on {
		t.Error("a /hide without mercs hid them")
	}
}

// A login with no charhidemercs row has them hidden (the owner, 2026-11-13:
// on by default) and is told so once in the world; one that chose /show mercs
// keeps them shown and is told nothing; one that chose /hide mercs hides them.
func TestHideMercsOutlivesALogin(t *testing.T) {
	db, chars := newSafeDB(t)
	c := newChatWorld(t)
	h := &CharacterHandler{store: chars, gameWorld: c.gw}
	s, p := c.join(5, "Plain", 100, 100, 0)
	plain, _ := c.join(6, "Other", 101, 100, 0)
	chose, cp := c.join(7, "Chooser", 102, 100, 0)
	if err := db.SetMercsHidden(6, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMercsHidden(cp.CharServerID, true); err != nil {
		t.Fatal(err)
	}
	h.restoreMercsHidden(s, &database.CharacterRecord{ServerID: p.CharServerID, CharName: p.Name})
	h.restoreMercsHidden(plain, &database.CharacterRecord{ServerID: 6, CharName: "Other"})
	h.restoreMercsHidden(chose, &database.CharacterRecord{ServerID: cp.CharServerID, CharName: cp.Name})
	if on, _ := c.gw.MercsHidden(s); !on {
		t.Fatal("a login with no row (no choice made) shows the mercs")
	}
	if on, _ := c.gw.MercsHidden(chose); !on {
		t.Fatal("a login that chose /hide mercs shows them")
	}
	if on, _ := c.gw.MercsHidden(plain); on {
		t.Fatal("a login that chose /show mercs hides them")
	}
	tell := tellMercsHidden(c.gw)
	tell(s, p)
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != mercsHiddenLoginLine {
		t.Fatalf("the login told %q, want %q", got, mercsHiddenLoginLine)
	}
	tell(plain, nil)
	if got := heard(t, plain); len(got) != 0 {
		t.Fatalf("a login showing the mercs told %q", texts(got))
	}
}

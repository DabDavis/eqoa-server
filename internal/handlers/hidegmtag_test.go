package handlers

import (
	"testing"
)

// The owner, 2026-11-13: a GM may hide its [GM] tag. "/hide gmtag" (and its
// other spellings) takes the tag off the name the entity channel carries,
// "/show gmtag" puts it back, each kept in charhidegmtag and brought back at
// the GM's next world entry; a player who is no GM is told it has none and
// nothing changes.
func TestHideGMTag(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetGMTagHiddenStore(db)
	s, p := c.join(5, "Dabgm", 100, 100, 0)
	c.gw.SetPlayerGM(s, true)
	if got := p.ShownName(); got != "[GM] Dabgm" {
		t.Fatalf("a GM's name %q, want the tag", got)
	}
	for _, tc := range []struct {
		line string
		hide bool
	}{
		{"/hide gmtag", true}, {"/show gmtag", false},
		{"#hide gm tag", true}, {"#show GM", false},
		{"/t hide gmtag", true}, {"/t show gmtag", false},
	} {
		c.say(s, tc.line)
		want, name := gmTagShownLine, "[GM] Dabgm"
		if tc.hide {
			want, name = gmTagHiddenLine, "Dabgm"
		}
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != want {
			t.Fatalf("%s: told %q, want %q", tc.line, got, want)
		}
		if got := p.ShownName(); got != name {
			t.Fatalf("%s: name %q, want %q", tc.line, got, name)
		}
		if kept, err := db.GMTagHidden(p.CharServerID); err != nil || kept != tc.hide {
			t.Fatalf("%s: kept %v (%v), want %v", tc.line, kept, err, tc.hide)
		}
		s.Queue().GetPending()
	}

	// kept: a new world entry of a GM with it hidden is drawn bare
	c.say(s, "/hide gmtag")
	s.Queue().GetPending()
	p.HideGMTag = false
	c.h.restoreGMTagHidden(s)
	if got := p.ShownName(); got != "Dabgm" {
		t.Fatalf("after its world entry %q, want the tag still hidden", got)
	}

	// a player who is no GM has no tag
	s2, p2 := c.join(6, "Plain", 101, 100, 0)
	c.say(s2, "/hide gmtag")
	if got := texts(heard(t, s2)); len(got) != 1 || got[0] != gmTagNoneLine {
		t.Fatalf("a player's /hide gmtag told %q", got)
	}
	if p2.HideGMTag || p2.ShownName() != "Plain" {
		t.Fatal("a player's /hide gmtag changed it")
	}
	if kept, _ := db.GMTagHidden(p2.CharServerID); kept {
		t.Fatal("a player's /hide gmtag was kept")
	}
}

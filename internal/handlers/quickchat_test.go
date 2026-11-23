package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Work board 1960 (petition 2624): /quickchat reset, #quickchat reset and a
// PS2's "/t quickchat reset" delete the player's own sets (the next login
// gets the defaults, hotkeysOf) and say so to it alone; with none left it says
// they are the defaults already. "/quickchat" alone only says how, "/t
// quickchat hello" is a tell, and "/qu ?" finds the help line.
func TestQuickChatReset(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetQuickChatStore(db)
	s, p := c.join(5, "Plain", 100, 100, 0)
	other, o := c.join(6, "Near", 101, 100, 0)
	set := func(id uint32) {
		t.Helper()
		if err := db.SetHotkeyChoice(id, defaultHotkeySets(), 6, 1, database.HotkeyPair{Text: "|16", Title: "Shout!"}); err != nil {
			t.Fatal(err)
		}
	}
	set(o.CharServerID)
	for _, word := range []string{"/quickchat reset", "#quickchat reset", "/t quickchat reset", "/QuickChat RESET"} {
		set(p.CharServerID)
		c.say(s, word)
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != quickChatDoneLine {
			t.Fatalf("%s: told %q, want %q", word, got, quickChatDoneLine)
		}
		if rows, _ := db.Hotkeys(p.CharServerID); len(rows) != 0 {
			t.Fatalf("%s: %d sets left", word, len(rows))
		}
		if got := heard(t, other); len(got) != 0 {
			t.Fatalf("%s reached another player: %q", word, texts(got))
		}
		s.Queue().GetPending()
	}
	if rows, _ := db.Hotkeys(o.CharServerID); len(rows) == 0 {
		t.Fatal("the other character's sets were reset")
	}
	c.say(s, "/quickchat reset")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != quickChatSameLine {
		t.Fatalf("a second reset: told %q", got)
	}
	s.Queue().GetPending()
	set(p.CharServerID)
	for _, word := range []string{"/quickchat", "#quickchat", "/quickchat please", "/t quickchat"} {
		c.say(s, word)
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != quickChatHelpLine {
			t.Fatalf("%s: told %q, want the help line", word, got)
		}
		s.Queue().GetPending()
	}
	if rows, _ := db.Hotkeys(p.CharServerID); len(rows) == 0 {
		t.Fatal("the help line reset the sets")
	}
	c.say(s, "/t quickchat hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "quickchat is not connected." {
		t.Fatalf("a tell read as /quickchat: %q", got)
	}
	if rows, _ := db.Hotkeys(p.CharServerID); len(rows) == 0 {
		t.Fatal("the tell reset the sets")
	}
	found := false
	for _, l := range commandsBeginning("/qu", false) {
		found = found || strings.Contains(l, "/quickchat reset")
	}
	if !found {
		t.Errorf("/qu ? does not find /quickchat reset: %q", commandsBeginning("/qu", false))
	}
}

package handlers

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// muteClock fixes the mutes' clock at 2026-11-18 12:00 UTC, moved by the test.
type muteClock struct{ at time.Time }

func (m *muteClock) now() time.Time { return m.at }

func muteWorld(t *testing.T) (*chatWorld, *muteClock, *session.Session, *session.Session, *session.Session) {
	c, gs, _, ps, _ := gmWorld(t)
	clk := &muteClock{at: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	c.h.mutes.now = clk.now
	by, _ := c.join(3, "Bystander", 25020, 15020, 0)
	by.SetAccountID(3)
	return c, clk, gs, ps, by
}

// drainAll empties the sessions' queues.
func drainAll(ss ...*session.Session) {
	for _, s := range ss {
		s.Queue().GetPending()
	}
}

// heardText reports whether s was sent a line holding text since its last drain.
func heardText(t *testing.T, s *session.Session, text string) bool {
	for _, l := range heard(t, s) {
		if strings.Contains(l.text, text) {
			return true
		}
	}
	return false
}

func shout0C01(c *chatWorld, s *session.Session, text string) {
	c.send(s, protocol.OpcodeClientShout, append([]byte{2}, wide(text, 0)[:4+2*len(text)]...))
}

func TestMuteHoursAndSpans(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want time.Duration
	}{{"26", 26 * time.Hour}, {".5", 30 * time.Minute}, {"0", 0}, {" 4H ", 4 * time.Hour}, {"8760", 8760 * time.Hour}} {
		if got, why := parseMuteHours(c.raw); why != "" || got != c.want {
			t.Errorf("%q: %v %q", c.raw, got, why)
		}
	}
	for _, raw := range []string{"", "x", "-1", "8761", "NaN", "inf", "0.00001"} {
		if _, why := parseMuteHours(raw); why == "" {
			t.Errorf("%q taken", raw)
		}
	}
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{26 * time.Hour, "1 day 2 hours"}, {30 * time.Minute, "30 minutes"}, {75 * time.Minute, "1 hour 15 minutes"},
		{48 * time.Hour, "2 days"}, {8760 * time.Hour, "365 days"}, {20 * time.Second, "less than a minute"},
		{25*time.Hour + 59*time.Minute + 50*time.Second, "1 day 2 hours"}} {
		if got := muteSpan(c.d); got != c.want {
			t.Errorf("%v: %q, want %q", c.d, got, c.want)
		}
	}
}

// /mute Player 26: the GM's line, the player told; its shouts (0x0C01 and shout
// mode) unheard and answered, its say heard; a bystander's shout reaches it;
// near the end the answer is what is left, after the end its shout is heard.
func TestMuteShoutsByTheHour(t *testing.T) {
	c, clk, gs, ps, by := muteWorld(t)
	drainAll(gs, ps, by)
	c.say(gs, "/mute Player 26")
	if !heardText(t, gs, "Player's account is muted from shouting for 1 day 2 hours, until 2026-11-19 14:00 UTC: every character of it.") {
		t.Fatalf("the GM's line: %q", texts(heard(t, gs)))
	}
	if !heardText(t, ps, "A GM has muted you from shouting for 1 day 2 hours.") {
		t.Fatalf("the player told: %q", texts(heard(t, ps)))
	}
	drainAll(gs, ps, by)
	shout0C01(c, ps, "wts")
	c.mode(ps, ChatModeShout)
	c.say(ps, "lfg")
	if heardText(t, by, "wts") || heardText(t, by, "lfg") {
		t.Fatal("a muted shout was heard")
	}
	if got := heard(t, ps); len(got) != 2 || got[0].text != "You are muted from shouting for another 1 day 2 hours." {
		t.Fatalf("the answers: %+v", got)
	}
	c.mode(ps, 0)
	drainAll(ps, by)
	c.say(ps, "hello")
	if !heardText(t, by, "hello") {
		t.Fatal("a say was stopped by a shout mute")
	}
	drainAll(ps, by)
	shout0C01(c, by, "anyone")
	if !heardText(t, ps, "anyone") {
		t.Fatal("a bystander's shout did not reach the muted player")
	}
	clk.at = clk.at.Add(25*time.Hour + 30*time.Minute)
	drainAll(ps, by)
	shout0C01(c, ps, "wts")
	if !heardText(t, ps, "another 30 minutes.") || heardText(t, by, "wts") {
		t.Fatalf("at +25 h 30: %q", texts(heard(t, ps)))
	}
	clk.at = clk.at.Add(31 * time.Minute)
	drainAll(ps, by)
	shout0C01(c, ps, "wts")
	if !heardText(t, by, "wts") {
		t.Fatal("after the end the shout was not heard")
	}
}

// /muteall Player .5: a say, a shout, a tell and a group line unheard and
// answered with the all-chat line; /unmute: told, a say heard again.
func TestMuteAllAndUnmute(t *testing.T) {
	c, _, gs, ps, by := muteWorld(t)
	pp, pb := c.gw.PlayerByName("Player"), c.gw.PlayerByName("Bystander")
	c.group(t, ps, pp, by, pb)
	c.say(gs, "/muteall Player .5")
	drainAll(gs, ps, by)
	c.say(ps, "hi")
	shout0C01(c, ps, "wts")
	c.say(ps, "/t Bystander psst")
	c.mode(ps, ChatModeGroup)
	c.say(ps, "grp")
	c.mode(ps, 0)
	for _, w := range []string{"hi", "wts", "psst", "grp"} {
		if heardText(t, by, w) {
			t.Errorf("%q heard", w)
		}
	}
	n := 0
	for _, l := range heard(t, ps) {
		if l.text == "You are muted from all chat for another 30 minutes." {
			n++
		}
	}
	if n != 4 {
		t.Errorf("answered %d times: %q", n, texts(heard(t, ps)))
	}
	drainAll(ps, by)
	c.say(gs, "/unmute Player")
	if !heardText(t, ps, "A GM has lifted your chat mutes: you may chat again.") {
		t.Fatal("not told of the unmute")
	}
	drainAll(ps, by)
	c.say(ps, "back")
	if !heardText(t, by, "back") {
		t.Fatal("a say after the unmute unheard")
	}
}

// A mute is the account's: a second character of it is told, its shout
// stopped, told again at its world entry; another account is told nothing.
func TestMuteIsTheWholeAccount(t *testing.T) {
	c, _, gs, ps, by := muteWorld(t)
	alt, altP := c.join(4, "Alt", 25030, 15030, 0)
	alt.SetAccountID(2)
	drainAll(gs, ps, by, alt)
	c.say(gs, "/mute Player 2")
	if !heardText(t, alt, "A GM has muted you from shouting for 2 hours.") || heardText(t, by, "A GM has muted") {
		t.Fatal("the account's other character not told, or another account told")
	}
	drainAll(alt, by)
	shout0C01(c, alt, "alt here")
	if heardText(t, by, "alt here") {
		t.Fatal("the other character's shout was heard")
	}
	drainAll(alt, by)
	c.h.muteEntered(alt, altP)
	c.h.muteEntered(by, nil)
	if !heardText(t, alt, "You are muted from shouting for another 2 hours.") || len(heard(t, by)) != 0 {
		t.Fatal("the world entry's line")
	}
}

// The two mutes apart: a short all over a long shout leaves the shout after it;
// #mute <name> lists both; 0 lifts the shout's.
func TestMuteTwoApart(t *testing.T) {
	c, clk, gs, ps, by := muteWorld(t)
	c.say(gs, "/mute Player 24")
	c.say(gs, "/muteall Player 1")
	drainAll(gs)
	c.say(gs, "/mute Player")
	if !heardText(t, gs, "Player's account is muted from shouting for another 1 day.") ||
		!heardText(t, gs, "Player's account is muted from all chat for another 1 hour.") {
		t.Fatalf("the status: %q", texts(heard(t, gs)))
	}
	clk.at = clk.at.Add(2 * time.Hour)
	drainAll(ps, by)
	c.say(ps, "hi")
	shout0C01(c, ps, "wts")
	if !heardText(t, by, "hi") || heardText(t, by, "wts") || !heardText(t, ps, "another 22 hours.") {
		t.Fatalf("at +2 h: %q", texts(heard(t, ps)))
	}
	c.say(gs, "/mute Player 0")
	drainAll(ps, by)
	shout0C01(c, ps, "wts")
	if !heardText(t, by, "wts") {
		t.Fatal("the shout after the lift unheard")
	}
}

// Only a GM, only on an account below its level; bad words mute nothing.
func TestMuteLevels(t *testing.T) {
	c, _, gs, ps, by := muteWorld(t)
	drainAll(ps)
	c.say(ps, "/mute Gm 5")
	if !heardText(t, ps, `Unrecognized command "/mute".`) {
		t.Fatal("a player's /mute")
	}
	drainAll(gs)
	c.say(gs, "/mute Gm 5")
	if !heardText(t, gs, "Gm's account is at or above your level: you cannot mute it.") {
		t.Fatalf("the GM's own: %q", texts(heard(t, gs)))
	}
	for _, bad := range []string{"/mute Nobody 5", "/mute Player x", "/mute Player -2", "/mute Player 5 more", "/unmute Player 5", "/mute"} {
		c.say(gs, bad)
	}
	drainAll(ps, by)
	shout0C01(c, ps, "free")
	if !heardText(t, by, "free") {
		t.Fatal("a bad command muted")
	}
	// another GM at the same level: refused; an admin (2) mutes a GM (1)
	c.h.SetAccessLevels(levels{1: 1, 2: 1, 3: 0})
	drainAll(gs)
	c.say(gs, "/mute Player 1")
	if !heardText(t, gs, "at or above your level") {
		t.Fatal("a GM muted a GM")
	}
	c.h.SetAccessLevels(levels{1: 2, 2: 1, 3: 0})
	drainAll(gs)
	c.say(gs, "/mute Player 1")
	if !heardText(t, gs, "Player's account is muted from shouting") {
		t.Fatalf("an admin's mute of a GM: %q", texts(heard(t, gs)))
	}
}

// fakeMutes is a chatmutes table in memory.
type fakeMutes struct {
	rows  []database.ChatMute
	fail  bool
	reads int
}

func (f *fakeMutes) ChatMutes() ([]database.ChatMute, error) {
	f.reads++
	return append([]database.ChatMute(nil), f.rows...), nil
}
func (f *fakeMutes) SetChatMute(m database.ChatMute) error {
	if f.fail {
		return errors.New("disk")
	}
	f.ClearChatMutes(m.AccountID, m.Scope)
	f.rows = append(f.rows, m)
	return nil
}
func (f *fakeMutes) ClearChatMutes(acct int, scope string) error {
	if f.fail {
		return errors.New("disk")
	}
	var keep []database.ChatMute
	for _, r := range f.rows {
		if r.AccountID != acct || (scope != "" && r.Scope != scope) {
			keep = append(keep, r)
		}
	}
	f.rows = keep
	return nil
}

// The store: the row written; a row added there holds after 30 s, its deletion
// lifts after another 30 s; a refused write mutes nothing and says so.
func TestMuteStore(t *testing.T) {
	c, clk, gs, ps, by := muteWorld(t)
	f := &fakeMutes{}
	c.h.SetChatMuteStore(f)
	c.say(gs, "/mute Player 3")
	if len(f.rows) != 1 || f.rows[0].AccountID != 2 || f.rows[0].Scope != "shout" || f.rows[0].GM != "Gm" ||
		!f.rows[0].Until.Equal(clk.at.Add(3*time.Hour)) {
		t.Fatalf("the row: %+v", f.rows)
	}
	c.say(by, "warm") // the first line reads the store
	f.rows = append(f.rows, database.ChatMute{AccountID: 3, Scope: "all", Until: clk.at.Add(time.Hour)})
	drainAll(by, ps)
	c.say(by, "early")
	if !heardText(t, ps, "early") {
		t.Fatal("a row read before its 30 s")
	}
	clk.at = clk.at.Add(31 * time.Second)
	drainAll(by, ps)
	c.say(by, "later")
	if heardText(t, ps, "later") {
		t.Fatal("a row added to the store did not hold after 30 s")
	}
	f.rows = f.rows[:1]
	clk.at = clk.at.Add(31 * time.Second)
	drainAll(by, ps)
	c.say(by, "free")
	if !heardText(t, ps, "free") {
		t.Fatal("a deleted row still held")
	}
	f.fail = true
	drainAll(gs)
	c.say(gs, "/muteall Bystander 1")
	if !heardText(t, gs, "Bystander's account's mute could not be kept just now: try again.") {
		t.Fatalf("a refused write: %q", texts(heard(t, gs)))
	}
	drainAll(by, ps)
	c.say(by, "still")
	if !heardText(t, ps, "still") {
		t.Fatal("a refused write muted")
	}
}

// The table: replace by account and scope, another scope refused, a bad row
// left out, rows kept across a close, an ended row gone at the next set,
// one scope cleared and both.
func TestChatMutesTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	set := func(acct int, scope string, until time.Time) error {
		return db.SetChatMute(database.ChatMute{AccountID: acct, Scope: scope, Until: until, GM: "Gm", Since: at})
	}
	if set(1, "shout", at.Add(time.Hour)) != nil || set(1, "shout", at.Add(2*time.Hour)) != nil || set(1, "all", at.Add(time.Minute)) != nil {
		t.Fatal("a set refused")
	}
	if set(1, "say", at.Add(time.Hour)) == nil {
		t.Fatal("another scope taken")
	}
	if _, err := db.Exec(`INSERT INTO chatmutes VALUES (9, 'shout', 'soon', '', 'x')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = database.NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.ChatMutes()
	if err != nil || len(rows) != 2 || !rows[0].Until.Equal(at.Add(2*time.Hour)) && !rows[1].Until.Equal(at.Add(2*time.Hour)) {
		t.Fatalf("after a close: %+v %v", rows, err)
	}
	at = at.Add(30 * time.Minute) // the all row ended
	if err := set(2, "shout", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.ChatMutes(); len(rows) != 2 {
		t.Fatalf("the ended row kept: %+v", rows)
	}
	db.ClearChatMutes(2, "shout")
	db.ClearChatMutes(1, "")
	if rows, _ := db.ChatMutes(); len(rows) != 0 {
		t.Fatalf("cleared: %+v", rows)
	}
}

// A /muteall'ed Hardcore character's /hc line is not heard, answered with the
// all-chat line.
func TestMuteHardcoreChannel(t *testing.T) {
	r := newHCWorld(t)
	r.sg.SetAccountID(7)
	r.c.h.mutes.set(7, database.ChatMuteAll, time.Now().Add(time.Hour), "Gm")
	r.sg.Queue().GetPending()
	r.c.say(r.sg, "#hc anyone?")
	if hasOp(r.sg, protocol.OpcodeColoredChat) {
		t.Fatal("a muted Hardcore line was heard")
	}
	if got := hcWide(sentOp(r.sg, protocol.OpcodeErrorMessage)); !strings.HasPrefix(got, "You are muted from all chat for another") {
		t.Fatalf("answered %q", got)
	}
}

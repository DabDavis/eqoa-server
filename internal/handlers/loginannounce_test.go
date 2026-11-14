package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// announceClock is LoginAnnounces' clock and timers as a test runs them: a timer waits in due until the test fires it.
type announceClock struct {
	now time.Time
	due []announceTimer
}

type announceTimer struct {
	after time.Duration
	f     func()
}

func (c *announceClock) use(a *LoginAnnounces) {
	a.now = func() time.Time { return c.now }
	a.later = func(d time.Duration, f func()) { c.due = append(c.due, announceTimer{d, f}) }
}

// fire moves the clock on by the first waiting timer's time and runs it; false when none waits.
func (c *announceClock) fire() (time.Duration, bool) {
	if len(c.due) == 0 {
		return 0, false
	}
	t := c.due[0]
	c.due = c.due[1:]
	c.now = c.now.Add(t.after)
	t.f()
	return t.after, true
}

// announceWorld is bugWorld (Ann a player, account 2; Bob a GM, account 1) with the login announcement over the same
// database, its timers the test's.
func announceWorld(t *testing.T) (*chatWorld, *SupportHandler, *database.SQLiteDB, *session.Session, *session.Session, *announceClock) {
	t.Helper()
	c, h, db, as, bs := bugWorld(t)
	a := NewLoginAnnounces(db)
	clock := &announceClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	clock.use(a)
	c.h.announces = a
	c.gw.OnPlayerEntered(a.Entered)
	return c, h, db, as, bs, clock
}

// The announcement goes after every timed line of the login: the line on reporting (15 s, or the offer's answer)
// and the boon's (20 s) come before its 22 s, and it waits for no more than two minutes.
func TestLoginAnnounceIsTimedAfterTheLoginsOtherLines(t *testing.T) {
	if loginAnnounceAfter <= boonNoticeAfter || loginAnnounceAfter <= reportNoticeAfter {
		t.Fatalf("the announcement at %v, the boon's line at %v, the report line at %v", loginAnnounceAfter, boonNoticeAfter, reportNoticeAfter)
	}
	if loginAnnounceGiveUp <= loginAnnounceAfter {
		t.Fatalf("it gives up (%v) before it is first tried (%v)", loginAnnounceGiveUp, loginAnnounceAfter)
	}
}

// A login's lines in order: a GM's answer to a petition goes at once, the announcement not before its time; tried
// while the client has yet to answer the bug report offer (vanilla, its world loading), it waits; F3's line goes at
// the answer; and the announcement goes then, the last line the login was sent, in its own colour, once.
func TestLoginAnnounceIsTheLoginsLastLine(t *testing.T) {
	c, _, db, as, _, clock := announceWorld(t)
	if _, err := db.SetLoginAnnounceAudited("Double experience this weekend!", database.GMEdit{AccountID: 1, Via: "game"}, "test"); err != nil {
		t.Fatal(err)
	}
	p, err := db.FilePetition(database.Petition{ServerID: 1, CharName: "Ann", AccountID: 2, Category: 1, Subcategory: 10, Text: "stuck"}, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AnswerPetition(p.ID, "GM Jib", "Moved you.", database.GMEdit{AccountID: 1, Via: "web"}, clock.now); err != nil {
		t.Fatal(err)
	}

	as.Queue().GetPending()
	c.gw.PlayerEntered(as)
	said := texts(heard(t, as))
	if !hasLine(said, PetitionAnsweredText(p.ID)) {
		t.Fatalf("the login heard no answer: %q", said)
	}
	if hasLine(said, loginAnnouncePrefix) {
		t.Fatalf("the announcement went at once: %q", said)
	}
	if d, ok := clock.fire(); !ok || d != loginAnnounceAfter {
		t.Fatalf("the announcement's timer: %v %v", d, ok)
	}
	if hasLine(texts(heard(t, as)), loginAnnouncePrefix) {
		t.Fatal("the announcement went before the line on reporting")
	}
	if len(clock.due) != 1 || clock.due[0].after != loginAnnounceRetry {
		t.Fatalf("it is not tried again: %+v", clock.due)
	}

	// the client answers the offer: F3's line
	c.h.support.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, []byte{1, 0, 0, 0}))
	// and the boon's line, at 20 s, is long out (boonNoticeAfter < loginAnnounceAfter): one stands in for it here
	SendClientMessage(as, "The boon is at every town's announcements.")
	clock.fire()

	ls := heard(t, as)
	last := ls[len(ls)-1]
	if last.op != protocol.OpcodeColoredChat || last.text != "Announcement: Double experience this weekend!" || last.rgb != loginAnnounceColour {
		t.Fatalf("the last line: %+v (all: %q)", last, texts(ls))
	}
	n := 0
	for _, l := range ls {
		if strings.HasPrefix(l.text, loginAnnouncePrefix) {
			n++
		}
	}
	if n != 1 || !hasLine(texts(ls), reportNoticeF3) {
		t.Fatalf("%d announcements, F3's line %v: %q", n, hasLine(texts(ls), reportNoticeF3), texts(ls))
	}
	if len(clock.due) != 0 {
		t.Fatalf("still waiting: %+v", clock.due)
	}
}

// A client whose line on reporting never goes out is sent the announcement at loginAnnounceGiveUp all the same;
// with none set, or the player gone from the world, nothing is sent.
func TestLoginAnnounceGivesUpWaitingAndSendsNothingUnset(t *testing.T) {
	c, _, db, as, bs, clock := announceWorld(t)
	db.SetLoginAnnounceAudited("Server maintenance at 04:00 UTC", database.GMEdit{AccountID: 1, Via: "game"}, "test")
	as.Queue().GetPending()
	c.gw.PlayerEntered(as)
	var waited time.Duration
	for {
		d, ok := clock.fire()
		if !ok {
			break
		}
		waited += d
		if waited > 10*time.Minute {
			t.Fatal("it never gave up")
		}
	}
	if waited < loginAnnounceGiveUp {
		t.Fatalf("sent or dropped after %v", waited)
	}
	if said := texts(heard(t, as)); !hasLine(said, "Announcement: Server maintenance at 04:00 UTC") {
		t.Fatalf("not sent after giving up: %q", said)
	}

	// none set
	db.SetLoginAnnounceAudited("", database.GMEdit{AccountID: 1, Via: "game"}, "test")
	bs.TakeReportNotice()
	bs.Queue().GetPending()
	c.gw.PlayerEntered(bs)
	for _, ok := clock.fire(); ok; _, ok = clock.fire() {
	}
	if said := texts(heard(t, bs)); hasLine(said, loginAnnouncePrefix) {
		t.Fatalf("an announcement with none set: %q", said)
	}

	// gone before its time
	db.SetLoginAnnounceAudited("Hello", database.GMEdit{AccountID: 1, Via: "game"}, "test")
	bs.Queue().GetPending()
	c.gw.PlayerEntered(bs)
	bs.SetCharacterInWorld(false)
	for _, ok := clock.fire(); ok; _, ok = clock.fire() {
	}
	if said := texts(heard(t, bs)); hasLine(said, loginAnnouncePrefix) {
		t.Fatalf("an announcement to a player gone: %q", said)
	}
}

// #announce and /announce are a GM's: a player is answered as for any GM command and nothing is kept; the GM sets
// it (shown as it will read), says it, cuts what a PS2 cannot show, and clears it, each change a gm_audit line.
func TestAnnounceCommandIsAGMs(t *testing.T) {
	c, _, db, as, bs, _ := announceWorld(t)
	for _, line := range []string{"#announce Free tunar for all", "/announce Free tunar for all"} {
		chat(t, c, as, line)
		if cur, _ := db.LoginAnnounce(); cur.Text != "" {
			t.Fatalf("a player's %q set it: %+v", line, cur)
		}
	}
	chat(t, c, as, "#announce Free tunar for all")
	if said := sentText(t, as); !strings.Contains(said, `Unrecognized command "#announce"`) {
		t.Fatalf("the player was answered %q", said)
	}

	chat(t, c, bs, "/announce   Welcome to Heirloom Test!  ")
	cur, err := db.LoginAnnounce()
	if err != nil || cur.Text != "Welcome to Heirloom Test!" || cur.SetBy == "" || cur.Changed == "" {
		t.Fatalf("after /announce: %+v %v", cur, err)
	}
	if said := sentText(t, bs); !strings.Contains(said, "Announcement: Welcome to Heirloom Test!") {
		t.Fatalf("the GM was not shown it: %q", said)
	}
	chat(t, c, bs, "#announce")
	if said := sentText(t, bs); !strings.Contains(said, "Announcement: Welcome to Heirloom Test!") {
		t.Fatalf("#announce alone said %q", said)
	}
	chat(t, c, bs, "#announce Patch|+ notes [up]")
	if cur, _ := db.LoginAnnounce(); cur.Text != "Patch+ notes up" {
		t.Fatalf("kept %q, want what a PS2 shows", cur.Text)
	}
	if said := sentText(t, bs); !strings.Contains(said, "Cut to what a PS2 shows") {
		t.Fatalf("the cut was not said: %q", said)
	}
	chat(t, c, bs, "#announce OFF")
	if cur, _ := db.LoginAnnounce(); cur.Text != "" {
		t.Fatalf("off left %+v", cur)
	}
	if said := sentText(t, bs); !strings.Contains(said, "Login announcement cleared") {
		t.Fatalf("off said %q", said)
	}
	chat(t, c, bs, "#announce clear")
	if said := sentText(t, bs); !strings.Contains(said, "No login announcement is set") {
		t.Fatalf("clearing none said %q", said)
	}
	var lines int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'loginannounce' AND accountid = 1 AND via = 'game'`).Scan(&lines)
	if lines != 3 {
		t.Fatalf("%d audit lines, want 3 (set, change, clear)", lines)
	}
}

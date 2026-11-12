package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The ready check (readycheck.go; the GM Jibrish's petition 1711): who may ask, who is
// asked and how, and what the asker is told. Each rule beside its control.

// readyWindows is each OpcodeReadyCheck session i was queued, drained: (id, seconds, name).
type readyWin struct {
	id      uint32
	seconds int
	name    string
}

func (r *raidRig) readyWindows(i int) (wins []readyWin, lines []string) {
	msgs := drain(r.sess[i])
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeReadyCheck && len(m) >= 8 {
			n := int(m[7])
			wins = append(wins, readyWin{binary.LittleEndian.Uint32(m[2:]), int(m[6]), string(m[8 : 8+n])})
		}
	}
	return wins, linesOf(msgs)
}

// at runs the ready checks' system at now.
func (r *raidRig) readyAt(now time.Time) {
	r.w.mu.Lock()
	r.w.readySystem(now)
	r.w.mu.Unlock()
}

// A plain group's member, not its leader, may ask; the others are asked in a line (no
// window answered), the asker is not; all yes: "Everyone is ready", at once.
func TestReadyCheckPlainGroupAllYes(t *testing.T) {
	r := newRaidRig(t, 4, 30)
	r.form(3)
	r.drainAll()
	r.w.ReadyCheck(r.sess[1]) // Bob, no leader
	if l := r.heard(1); !saidPart(l, "Ready check sent to your group (2 asked, 30 seconds).") {
		t.Fatalf("Bob's line: %q", l)
	}
	for _, i := range []int{0, 2} {
		if l := r.heard(i); !saidPart(l, "Bob asks: Are you ready?") || !saidPart(l, "/t ready") {
			t.Fatalf("%s's line: %q", r.p[i].Name, l)
		}
	}
	if l := r.heard(3); len(l) != 0 { // Dee, in no group
		t.Fatalf("Dee was told %q", l)
	}
	if !r.w.ReadyAnswer(r.sess[0], 0, true, true) {
		t.Fatal("Ann's /ready not taken")
	}
	if l := r.heard(1); len(l) != 0 { // a yes is never told
		t.Fatalf("Bob told %q before everyone answered", l)
	}
	r.w.ReadyAnswer(r.sess[2], 0, true, true)
	if l := r.heard(1); len(l) != 1 || l[0] != "Everyone is ready" {
		t.Fatalf("Bob's report: %q", l)
	}
	// over: another /ready has nothing to answer, a new check may begin
	if r.w.ReadyAnswer(r.sess[0], 0, true, true) || r.w.HasReadyCheck(r.sess[0]) {
		t.Fatal("the check still open")
	}
	r.w.ReadyCheck(r.sess[2])
	if !r.w.HasReadyCheck(r.sess[0]) || r.w.HasReadyCheck(r.sess[2]) {
		t.Fatal("Cal's check: Ann asked, Cal not")
	}
}

// Not in a group: refused; a plain group of the asker and nobody else to ask: refused.
func TestReadyCheckRefusals(t *testing.T) {
	r := newRaidRig(t, 2, 30)
	r.w.ReadyCheck(r.sess[0])
	if l := r.heard(0); !saidPart(l, "You are not in a group.") {
		t.Fatalf("Ann alone: %q", l)
	}
	if r.w.ReadyAnswer(r.sess[1], 0, true, true); !saidPart(r.heard(1), "There is no ready check to answer.") {
		t.Fatal("Bob's /ready with none")
	}
}

// A raid: only its leader or helper (the co-leader) may ask; one check at a time; a
// window to a client that answered the offer, its reply taken; the end at 30 seconds:
// "Raid not ready", who said no and who did not answer, the windows still open closed.
func TestReadyCheckRaidNotReady(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.w.RaidHelper(r.sess[0], "Bob", 0)
	r.w.SetReadyWindows(r.sess[1], true) // Bob and Eve show the window
	r.w.SetReadyWindows(r.sess[4], true)
	r.drainAll()
	r.w.ReadyCheck(r.sess[2]) // Cal: neither leads nor helps
	if l := r.heard(2); !saidPart(l, "Only your raid's leader or its helper can start a ready check.") {
		t.Fatalf("Cal: %q", l)
	}
	if r.w.HasReadyCheck(r.sess[1]) {
		t.Fatal("Cal's refused check began")
	}
	start := time.Now()
	r.w.ReadyCheck(r.sess[1]) // the helper may
	if l := r.heard(1); !saidPart(l, "Ready check sent to your raid (5 asked, 30 seconds).") {
		t.Fatalf("Bob: %q", l)
	}
	r.w.ReadyCheck(r.sess[0]) // the leader, while Bob's is on
	if l := r.heard(0); !saidPart(l, "A ready check is already under way") {
		t.Fatalf("Ann's second check: %q", l)
	}
	wins, lines := r.readyWindows(4)
	if len(wins) != 1 || wins[0].seconds != ReadyCheckSeconds || wins[0].name != "Bob" || len(lines) != 0 {
		t.Fatalf("Eve's window %+v, lines %q", wins, lines)
	}
	if _, l := r.readyWindows(3); !saidPart(l, "Bob asks: Are you ready?") {
		t.Fatalf("Dee's line %q", l)
	}
	id := wins[0].id
	if r.w.ReadyAnswer(r.sess[4], id+1, true, false) { // a stale id
		t.Fatal("a reply to another check taken")
	}
	r.w.ReadyAnswer(r.sess[4], id, true, false) // Eve: yes, by the window
	r.w.ReadyAnswer(r.sess[2], 0, false, true)  // Cal: /notready
	r.w.ReadyAnswer(r.sess[0], 0, false, true)  // Ann: /notready
	r.readyAt(start.Add(29 * time.Second))
	if l := r.heard(1); len(l) != 0 {
		t.Fatalf("Bob told %q before the time was up", l)
	}
	r.readyAt(start.Add(31 * time.Second))
	got := r.heard(1)
	want := []string{"Raid not ready", "Not ready: Ann, Cal", "No answer: Dee, Fay"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Bob's report %q, want %q", got, want)
	}
	// Eve answered (its window closed when it did); none closed again
	if w, _ := r.readyWindows(4); len(w) != 1 || w[0].seconds != 0 {
		t.Fatalf("Eve's windows after: %+v", w)
	}
	if r.w.HasReadyCheck(r.sess[3]) {
		t.Fatal("the check still open")
	}
	// the leader may ask now
	r.w.ReadyCheck(r.sess[0])
	if !r.w.HasReadyCheck(r.sess[1]) {
		t.Fatal("Ann's check did not begin")
	}
	if w, _ := r.readyWindows(1); len(w) != 1 || w[0].name != "Ann" || w[0].seconds != 30 {
		t.Fatalf("Bob's window %+v", w)
	}
	// expiry closes Bob's open window
	r.readyAt(time.Now().Add(31 * time.Second))
	if w, _ := r.readyWindows(1); len(w) != 1 || w[0].seconds != 0 {
		t.Fatalf("Bob's window not closed at the end: %+v", w)
	}
	if l := r.heard(0); !saidPart(l, "Raid not ready") || !saidPart(l, "No answer: Bob, Cal, Dee, Eve, Fay") {
		t.Fatalf("Ann's report %q", l)
	}
}

// A member who leaves the group is neither ready nor not; the asker leaving ends the check
// unsaid.
func TestReadyCheckMembersLeaving(t *testing.T) {
	r := newRaidRig(t, 4, 30)
	r.form(4)
	r.w.ReadyCheck(r.sess[0])
	r.w.ReadyAnswer(r.sess[1], 0, true, true)
	r.w.ReadyAnswer(r.sess[2], 0, true, true)
	r.w.RaidLeave(r.sess[3]) // Dee goes, unanswered: everyone left answered
	r.readyAt(time.Now())
	if l := r.heard(0); !saidPart(l, "Everyone is ready") {
		t.Fatalf("Ann's report %q", l)
	}
	r.w.ReadyCheck(r.sess[1])
	r.drainAll()
	r.w.RaidLeave(r.sess[1])
	r.readyAt(time.Now())
	if r.w.HasReadyCheck(r.sess[0]) {
		t.Fatal("the check outlived its asker's leaving")
	}
}

// A merc in a raid is never asked nor named (the control: its owner and the other
// player are both asked).
func TestReadyCheckSkipsMercs(t *testing.T) {
	r, _, g, ss, _ := mercRaid(t, "Other")
	r.w.ReadyCheck(ss[0]) // not the leader: refused
	if c := r.w.ready.byGroup[g.ID]; c != nil {
		t.Fatal("the member's check began")
	}
	r.w.mu.Lock()
	r.w.readyCheckLocked(r.sess, time.Now())
	c := r.w.ready.byGroup[g.ID]
	r.w.mu.Unlock()
	if c == nil || len(c.asked) != 1 || c.asked[0] != r.w.sessionPlayerLocked(ss[0]).ServerID {
		t.Fatalf("asked %+v", c)
	}
	r.w.ReadyAnswer(ss[0], 0, true, true)
	if l := ownerLines(r.sess); !saidPart(l, "Everyone is ready") {
		t.Fatalf("the owner's lines %q", l)
	}
}

func TestReadyNameLines(t *testing.T) {
	names := []string{"Abcdefghij", "Bcdefghijk", "Cdefghijkl", "Defghijklm", "Efghijklmn", "Fghijklmno", "G"}
	got := readyNameLines("No answer: ", names)
	if len(got) != 2 || !strings.HasPrefix(got[1], "  ") || strings.HasSuffix(got[1], ",") || !strings.HasSuffix(got[0], ",") {
		t.Fatalf("%q", got)
	}
	for _, l := range got {
		if len(l) > readyNameLinesWidth+12 {
			t.Fatalf("a long line %q", l)
		}
	}
	if readyNameLines("x", nil) != nil {
		t.Fatal("lines for none")
	}
}

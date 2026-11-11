package handlers

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// crashBody is OpcodeCrashReport's body: version 1, number, count, text.
func crashBody(number uint32, text string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, 1)
	b = binary.LittleEndian.AppendUint32(b, number)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(text)))
	return append(b, text...)
}

// crashAnswers is each OpcodeCrashReportFiled s was sent: (number, ticket).
func crashAnswers(s *session.Session) [][2]uint32 {
	var out [][2]uint32
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 10 && opcode(d) == protocol.OpcodeCrashReportFiled {
			out = append(out, [2]uint32{binary.LittleEndian.Uint32(d[2:]), binary.LittleEndian.Uint32(d[6:])})
		}
	}
	return out
}

const sampleCrash = "build: dev-0bd027f\nplatform: Linux x86_64\ngraphics: AMD Radeon (Vulkan)\n" +
	"when: 2026-11-11T04:52:13Z\n\npanic: crates/eqoa/src/play.rs:1234:56: index out of bounds: the len is 4 but the index is 7\n" +
	"stack backtrace:\n   0: std::panicking::begin_panic\n   1: eqoa::play::Play::frame\n"

// A crash report is filed as the character's petition, Bug Report / Crash,
// open and not waiting on a GM, its text the panic's line, its server view
// the report whole and the server's record of the client stopping; the
// client is answered with its number and the ticket, the player told, a GM
// told. The controls: a petition filed by /bug waits on a GM; a report that
// does not read, an empty one and one past the account's limit are answered
// 0 and filed nowhere.
func TestCrashReportFiledAsAPetition(t *testing.T) {
	c, h, db, as, bs := bugWorld(t)
	now := time.Now()
	// the server saw Ann's client stop before (clientstopped.go)
	as.SetLastActivity(now.Add(-40 * time.Second))
	if !session.NoteClientStopped(as, "silent in the world", now) {
		t.Fatal("the stop was not noted")
	}
	as.SetLastActivity(now)
	as.Queue().GetPending()
	bs.Queue().GetPending()
	if err := h.handleCrashReport(as, incoming(protocol.OpcodeCrashReport, crashBody(7, sampleCrash))); err != nil {
		t.Fatal(err)
	}
	ans := crashAnswers(as)
	if len(ans) != 1 || ans[0][0] != 7 || ans[0][1] == 0 {
		t.Fatalf("the answer: %v", ans)
	}
	ticket := int64(ans[0][1])
	var cat, sub int64
	var waiting bool
	var status, text string
	if err := db.QueryRow(`SELECT category, subcategory, waiting, status, text FROM petitions WHERE id = ?`, ticket).
		Scan(&cat, &sub, &waiting, &status, &text); err != nil {
		t.Fatal(err)
	}
	if cat != 1 || sub != crashSubcategory || waiting || status != "open" {
		t.Errorf("filed as %d/%d waiting %v %s", cat, sub, waiting, status)
	}
	if !strings.Contains(text, "Bug Report / Crash") || !strings.Contains(text, "index out of bounds: the len is 4 but the index is 7") ||
		strings.Contains(text, "stack backtrace") {
		t.Errorf("its text:\n%s", text)
	}
	v := views(t, db)[ticket]
	if v.Crash != strings.TrimSpace(sampleCrash) || v.Via != "crash report" {
		t.Errorf("the view's crash %q via %q", v.Crash, v.Via)
	}
	if v.Stopped == nil || v.Stopped.Name != "Ann" || v.Stopped.How != "silent in the world" {
		t.Errorf("the view's stop: %+v", v.Stopped)
	}
	if s := sentText(t, as); !strings.Contains(s, "Ticket #") {
		t.Errorf("Ann was told: %q", s)
	}
	if s := sentText(t, bs); !strings.Contains(s, "[Petition]") {
		t.Errorf("the GM was told: %q", s)
	}

	// the control: /bug's petition waits on a GM
	chat(t, c, as, "/bug the merchant is gone")
	var bugWaiting bool
	if err := db.QueryRow(`SELECT waiting FROM petitions WHERE subcategory = ?`, bugSubcategory).Scan(&bugWaiting); err != nil || !bugWaiting {
		t.Errorf("/bug's petition waiting %v (%v)", bugWaiting, err)
	}

	// refused: one that does not read, an empty one, then past the limit
	count := func() (n int) {
		db.QueryRow(`SELECT COUNT(*) FROM petitions WHERE subcategory = ?`, crashSubcategory).Scan(&n)
		return n
	}
	for i, body := range [][]byte{{1, 0, 0, 0, 9, 0, 0, 0}, crashBody(9, "  \n ")} {
		as.Queue().GetPending()
		h.handleCrashReport(as, incoming(protocol.OpcodeCrashReport, body))
		if a := crashAnswers(as); len(a) != 1 || a[0][0] != 9 || a[0][1] != 0 {
			t.Errorf("refusal %d answered %v", i, a)
		}
	}
	h.crashLimit.mu.Lock()
	h.crashLimit.seen = nil // the limit alone below: no gap
	h.crashLimit.mu.Unlock()
	for i := range crashReportsPerHour + 1 {
		h.crashLimit.mu.Lock()
		for k, ts := range h.crashLimit.seen {
			for j := range ts {
				ts[j] = ts[j].Add(-reportGap) // past the gap between two
			}
			h.crashLimit.seen[k] = ts
		}
		h.crashLimit.mu.Unlock()
		as.Queue().GetPending()
		h.handleCrashReport(as, incoming(protocol.OpcodeCrashReport, crashBody(uint32(100+i), sampleCrash)))
		a := crashAnswers(as)
		if len(a) != 1 || (i < crashReportsPerHour) != (a[0][1] != 0) {
			// the count started again above: crashReportsPerHour more are taken
			t.Errorf("report %d answered %v", i, a)
		}
	}
	if n := count(); n != crashReportsPerHour+1 {
		t.Errorf("%d crash petitions filed, want %d", n, crashReportsPerHour+1)
	}
}

// The report as kept: no control characters but line ends and tabs, at most
// CrashReportMost bytes, cut on a character; its summary the panic's line,
// else its first.
func TestCrashReportCleanAndSummary(t *testing.T) {
	if got := cleanCrash("a\x00b\r\n\tc\x1b"); got != "a b\n\tc" {
		t.Errorf("cleaned %q", got)
	}
	long := cleanCrash(strings.Repeat("é", CrashReportMost))
	if len(long) > CrashReportMost || !strings.HasSuffix(long, "é") {
		t.Errorf("cut to %d bytes", len(long))
	}
	if s := crashSummary(sampleCrash); !strings.HasPrefix(s, "crates/eqoa/src/play.rs:1234:56: index out of bounds") {
		t.Errorf("summary %q", s)
	}
	if s := crashSummary("The client stopped: RuntimeError: unreachable\nat wasm"); s != "The client stopped: RuntimeError: unreachable" {
		t.Errorf("a page's summary %q", s)
	}
	if _, _, ok := readCrashReport(crashBody(1, strings.Repeat("x", CrashReportMost+1))); ok {
		t.Error("a report past the most was read")
	}
}

// The offer: version 1 and the most bytes taken.
func TestCrashReportOffer(t *testing.T) {
	b := buildCrashReportOffer()
	if opcode(b) != protocol.OpcodeCrashReportOffer || binary.LittleEndian.Uint32(b[2:]) != 1 ||
		binary.LittleEndian.Uint32(b[6:]) != CrashReportMost {
		t.Errorf("offer % x", b)
	}
}

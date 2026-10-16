package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// payloadSrc is the SD payload the verbs must agree with. The two live in
// different languages in different directories and are shipped as a pair; the
// only thing that keeps them in step is this test reading one against the other.
// THIS USED TO READ re-tools/hdd_sd.S AND SKIP WHEN IT COULD NOT.
//
// The assembly payload was retired on 2026-08-14 and the file deleted, so from
// that day the check below did not run at all -- it reported PASS by skipping,
// which is the one failure mode a test must not have. The verbs are in
// re-tools/hdd_sd/layout.h now, as #defines, and a missing file is FATAL: if
// this cannot find the payload's source it cannot say whether the server and
// the console agree, and "I could not check" is not "they agree".
func payloadSrc(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "re-tools", "hdd_sd", "layout.h")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("cannot read the payload's layout (%v) -- this test exists to "+
			"hold the server's verbs to the console's, and it cannot do that "+
			"against a file it cannot open", err)
	}
	return string(b)
}

// A verb the server sends and the payload does not implement is not an error at
// either end: the payload counts it in ST_CMDBAD and carries on, the server
// says "working" and nothing happens. That is precisely why it is checked here.
func TestHDDVerbsMatchPayload(t *testing.T) {
	src := payloadSrc(t)
	want := map[string]rune{
		"CMD_CHECK":  HDDCheck,
		"CMD_CREATE": HDDCreate,
		"CMD_FORMAT": HDDFormat,
		"CMD_REPAIR": HDDRepair,
	}
	for name, verb := range want {
		re := regexp.MustCompile(`(?m)^#define\s+` + name + `\s+(0x[0-9A-Fa-f]+)`)
		m := re.FindStringSubmatch(src)
		if m == nil {
			t.Errorf("hdd_sd/layout.h no longer defines %s", name)
			continue
		}
		got, err := strconv.ParseInt(m[1][2:], 16, 32)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if rune(got) != verb {
			t.Errorf("%s is %#x in the payload and %#x here", name, got, verb)
		}
	}
}

// The marker must be the payload's CMD_MARK, and must survive UTF-16 encoding
// as ONE unit — the hook reads a single halfword. A marker outside the BMP
// would encode as a surrogate pair and never match.
func TestHDDMarkMatchesPayload(t *testing.T) {
	src := payloadSrc(t)
	m := regexp.MustCompile(`(?m)^#define\s+CMD_MARK\s+0x([0-9A-Fa-f]+)`).
		FindStringSubmatch(src)
	if m == nil {
		t.Fatal("hdd_sd/layout.h no longer defines CMD_MARK")
	}
	got, _ := strconv.ParseInt(m[1], 16, 32)
	if rune(got) != hddMark {
		t.Errorf("CMD_MARK is %#x in the payload and %#x here", got, hddMark)
	}
	if u := utf16.Encode([]rune{hddMark}); len(u) != 1 {
		t.Errorf("the marker encodes to %d UTF-16 units; the hook reads one", len(u))
	}
}

// Without a token the control string is exactly two UTF-16 units, marker then
// verb — the shape a payload older than the nonce expects, and the shape the
// server falls back to if it cannot generate one.
func TestHDDControlStringShape(t *testing.T) {
	u := utf16.Encode([]rune(hddControlString(HDDFormat, 0)))
	if len(u) != 2 || u[0] != uint16(hddMark) || u[1] != uint16(HDDFormat) {
		t.Errorf("control string encodes to %v, want [%#x %#x]",
			u, uint16(hddMark), uint16(HDDFormat))
	}
}

// The format arm is the whole guard against a mis-picked dialogue line
// formatting a drive, so its refusals are tested rather than assumed.
func TestFormatArmMustBeTakenOnce(t *testing.T) {
	const id = 4242
	clearFormatArm(id)
	if takeFormatArm(id) {
		t.Error("an unarmed player passed the format guard")
	}
	armFormat(id)
	if !takeFormatArm(id) {
		t.Error("a freshly armed player was refused")
	}
	if takeFormatArm(id) {
		t.Error("the same arm authorised a SECOND format")
	}
}

func TestFormatArmIsClearedByOtherOptions(t *testing.T) {
	const id = 4243
	armFormat(id)
	clearFormatArm(id) // what check/create/help do
	if takeFormatArm(id) {
		t.Error("asking for something else left the format arm live")
	}
}

// The NPC is wired by NAME, so a rename in one file and not the other leaves a
// technician standing in Freeport with nothing to say — and no error anywhere.
func TestHDDSetupNPCNameMatchesSQL(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "sql", "freeport_hdd_setup.sql"))
	if err != nil {
		t.Skipf("sql unreadable (%v)", err)
	}
	if !regexp.MustCompile(`'` + regexp.QuoteMeta(HDDSetupNPC) + `'`).Match(b) {
		t.Errorf("sql/freeport_hdd_setup.sql does not insert %q", HDDSetupNPC)
	}
}

// The destructive line must not be the one under the cursor when the box opens,
// nor the last (where a held button lands), and the confirm menu's SAFE answer
// must be first. This is layout as a safety property, so it is asserted rather
// than left to whoever next edits the strings.
func TestHDDMenuPutsDangerOutOfReach(t *testing.T) {
	main := HDDMenuFor(HDDMenuMain)
	danger := -1
	for i, s := range main {
		if regexp.MustCompile(`(?i)format`).MatchString(s) {
			danger = i
		}
	}
	if danger <= 0 || danger >= len(main)-1 {
		t.Errorf("the format line is at index %d of %d — it must be neither "+
			"first nor last", danger, len(main))
	}
	confirm := HDDMenuFor(HDDMenuConfirmFormat)
	if len(confirm) < 2 || !regexp.MustCompile(`(?i)^no\b`).MatchString(confirm[0]) {
		t.Errorf("the confirm menu's first option is %q; the safe answer must "+
			"be first", confirm[0])
	}
}

// Every pick must be read against the menu that produced it. A number alone
// means nothing: 2 is "create" on the main menu and out of range on the confirm
// menu, and reading one against the other is the mistake that formats a disk.
func TestHDDMenuPicksAreBounded(t *testing.T) {
	for _, menu := range []int{HDDMenuNone, HDDMenuMain, HDDMenuConfirmFormat} {
		items := HDDMenuFor(menu)
		for _, idx := range []int{-1, len(items), 255} {
			if got := HandleHDDMenuPick(nil, &entity.Player{ServerID: 1}, menu,
				idx, func(string) {}); got != HDDMenuNone {
				t.Errorf("menu %d index %d returned menu %d, want none",
					menu, idx, got)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Who a report reaches.
//
// The console's address is not an identity: it is shared by everyone behind one
// router and it changes under a player who reconnects. These tests are the
// specification of what replaced it, and each one is a way the address alone
// got it wrong.
// ---------------------------------------------------------------------------

// resetHDDRequests empties the table between tests. Package-level state shared
// by every test in the file, so a leftover ask would make the NEXT test pass
// for the wrong reason.
func resetHDDRequests() {
	hddReqMu.Lock()
	defer hddReqMu.Unlock()
	hddReqs = nil
}

// hddTestSession builds a session at an address with an account on it. The
// port only has to be unique — sessions are keyed by it, and the reporting
// datagram arrives on a different port anyway, which is exactly why only the
// host is ever compared.
func hddTestSession(sm *session.Manager, ip string, port int, account int) *session.Session {
	addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	sess := sm.CreateSession(addr, uint32(port), uint32(port), 1, 2, false)
	sess.SetAccountID(account)
	return sess
}

func hddTestPlayer(id entity.EntityID, name string) *entity.Player {
	return &entity.Player{ServerID: id, Name: name}
}

// The bug this whole mechanism exists for: two players behind one address, one
// of whom asked. Before, both were told.
func TestHDDReportGoesOnlyToTheAccountThatAsked(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	asker := hddTestSession(sm, "203.0.113.7", 5000, 11)
	bystander := hddTestSession(sm, "203.0.113.7", 5001, 22)

	noteHDDRequest(asker, hddTestPlayer(1, "Asker"), HDDCheck, 0)
	before := bystander.Queue().PendingCount()

	told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.7", Verb: HDDCheck, Describe: "the disk is fine",
	})
	if told != 1 {
		t.Fatalf("told %d players, want exactly 1", told)
	}
	if asker.Queue().PendingCount() == 0 {
		t.Error("the player who asked was not told")
	}
	if got := bystander.Queue().PendingCount(); got != before {
		t.Errorf("the bystander was sent %d messages about someone else's "+
			"hard disk", got-before)
	}
}

// The account is the identity precisely so that this works: a format takes long
// enough that reconnecting during one is ordinary, and the session that asked
// is gone by the time the answer arrives.
func TestHDDReportSurvivesAReconnect(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	first := hddTestSession(sm, "203.0.113.9", 5100, 33)
	noteHDDRequest(first, hddTestPlayer(2, "Reconnector"), HDDFormat, 0)

	// Reconnected: same account, new session, new port.
	sm.RemoveSession(first)
	second := hddTestSession(sm, "203.0.113.9", 5101, 33)

	told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.9", Verb: HDDFormat, Describe: "the disk was formatted",
	})
	if told != 1 || second.Queue().PendingCount() == 0 {
		t.Errorf("told %d players; the reconnected session got %d messages",
			told, second.Queue().PendingCount())
	}
}

// A datagram no one asked for is a stale duplicate, a forgery, or a report that
// outlived its window. None of those is worth telling a bystander about.
func TestHDDUnsolicitedReportIsDropped(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	sess := hddTestSession(sm, "203.0.113.11", 5200, 44)
	before := sess.Queue().PendingCount()

	if told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.11", Verb: HDDCheck, Describe: "the disk is fine",
	}); told != 0 {
		t.Errorf("an unsolicited report reached %d players", told)
	}
	if got := sess.Queue().PendingCount(); got != before {
		t.Errorf("an unsolicited report queued %d messages", got-before)
	}
}

// One ask, one answer. The payload sends its report best-effort with no retry,
// but nothing stops a datagram being duplicated in the network.
func TestHDDRequestIsConsumedOnce(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	sess := hddTestSession(sm, "203.0.113.13", 5300, 55)
	noteHDDRequest(sess, hddTestPlayer(3, "Once"), HDDCheck, 0)

	rep := HDDReport{IP: "203.0.113.13", Verb: HDDCheck, Describe: "fine"}
	if told := ReportHDDStatus(sm, nil, rep); told != 1 {
		t.Fatalf("the first report told %d players, want 1", told)
	}
	if told := ReportHDDStatus(sm, nil, rep); told != 0 {
		t.Errorf("a DUPLICATE report told %d players, want 0", told)
	}
}

// An ask from one console must never answer a report from another, or the
// address check has bought nothing at all.
func TestHDDReportRequiresTheSameConsole(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	asker := hddTestSession(sm, "203.0.113.15", 5400, 66)
	noteHDDRequest(asker, hddTestPlayer(4, "Elsewhere"), HDDCheck, 0)

	if told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "198.51.100.2", Verb: HDDCheck, Describe: "fine",
	}); told != 0 {
		t.Errorf("a report from a different address answered the ask (%d told)", told)
	}
	if asker.Queue().PendingCount() != 0 {
		t.Error("the asker was told about a different console's disk")
	}
}

// The verb is a hint, not a key: the payload reports the verb it ACTED on, and
// one it does not implement is counted and dropped. The ask is still real.
func TestHDDVerbMismatchStillAnswersTheAsk(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	asker := hddTestSession(sm, "203.0.113.17", 5500, 77)
	noteHDDRequest(asker, hddTestPlayer(5, "Mismatch"), HDDCreate, 0)

	if told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.17", Verb: 99, Describe: "something happened",
	}); told != 1 {
		t.Errorf("a verb the server never sent left the ask unanswered (%d told)", told)
	}
}

// Two asks from one console are answered oldest-first, and the exact verb wins
// over mere recency — otherwise a check issued after a format would collect the
// format's answer.
func TestHDDExactVerbWinsOverOrder(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	sess := hddTestSession(sm, "203.0.113.19", 5600, 88)
	noteHDDRequest(sess, hddTestPlayer(6, "First"), HDDCheck, 0)
	noteHDDRequest(sess, hddTestPlayer(6, "First"), HDDFormat, 0)

	req, ok := takeHDDRequest("203.0.113.19", HDDFormat, 0)
	if !ok || req.verb != HDDFormat {
		t.Fatalf("took %v (ok=%v), want the FORMAT ask", req.verb, ok)
	}
	if req, ok := takeHDDRequest("203.0.113.19", HDDCheck, 0); !ok || req.verb != HDDCheck {
		t.Errorf("the check ask did not survive the format being taken")
	}
}

// An ask that has been waiting longer than a disk format could possibly take is
// not the ask a report answers.
func TestHDDRequestExpires(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	sess := hddTestSession(sm, "203.0.113.21", 5700, 99)
	noteHDDRequest(sess, hddTestPlayer(7, "Stale"), HDDCheck, 0)

	hddReqMu.Lock()
	hddReqs[0].at = time.Now().Add(-hddRequestWindow - time.Second)
	hddReqMu.Unlock()

	if _, ok := takeHDDRequest("203.0.113.21", HDDCheck, 0); ok {
		t.Error("an expired ask was still answerable")
	}
}

// Asks that nothing will ever answer are the normal case — no payload, no disk,
// a console switched off mid-format — so the table must not grow without bound.
func TestHDDRequestTableIsBounded(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	sess := hddTestSession(sm, "203.0.113.23", 5800, 100)
	for i := 0; i < hddMaxRequests*3; i++ {
		noteHDDRequest(sess, hddTestPlayer(8, "Flood"), HDDCheck, 0)
	}
	hddReqMu.Lock()
	n := len(hddReqs)
	hddReqMu.Unlock()
	if n > hddMaxRequests {
		t.Errorf("the request table holds %d entries, cap is %d", n, hddMaxRequests)
	}
}

// ---------------------------------------------------------------------------
// The nonce.
//
// It crosses two boundaries that no compiler checks: a UTF-16 encoder that
// silently rewrites anything in the surrogate range, and an assembler source
// file in another language in another directory. Both are checked here.
// ---------------------------------------------------------------------------

// payloadEqu reads one #define out of hdd_sd/layout.h.
func payloadEqu(t *testing.T, src, name string) int64 {
	t.Helper()
	m := regexp.MustCompile(`(?m)^#define\s+` + name + `\s+(0x[0-9A-Fa-f]+|\d+)`).
		FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("hdd_sd/layout.h no longer defines %s", name)
	}
	v, err := strconv.ParseInt(strings.TrimPrefix(m[1], "0x"), map[bool]int{
		true: 16, false: 10,
	}[strings.HasPrefix(m[1], "0x")], 64)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}

// The unit count, the private-use base and the mask are the contract between
// the encoder here and the four range tests in _hook4. Disagree on any of them
// and every token is silently discarded as garbage — which looks exactly like
// an older payload, and so would never be investigated.
func TestHDDTokenConstantsMatchPayload(t *testing.T) {
	src := payloadSrc(t)
	for _, c := range []struct {
		equ  string
		mine int64
	}{
		{"CMD_TOKN", hddTokenUnits},
		{"CMD_TOKV", hddTokenBase},
		{"CMD_TOKM", hddTokenMask},
	} {
		if got := payloadEqu(t, src, c.equ); got != c.mine {
			t.Errorf("%s is %#x in the payload and %#x here", c.equ, got, c.mine)
		}
	}
}

// EVERY unit must survive utf16.Encode unchanged. A rune in the surrogate range
// is replaced with U+FFFD, and a token that encoded to U+FFFD would arrive as
// four identical units that the payload rejects — the failure would be total,
// silent, and indistinguishable from an old payload.
func TestHDDTokenUnitsSurviveUTF16(t *testing.T) {
	// The extremes of each chunk, plus a value with every bit pattern in it.
	for _, tok := range []uint64{1, hddTokenMax, 0xFFF, 0xFFF000,
		0xA5A5A5A5A5A5, 0x5A5A5A5A5A5A} {
		u := utf16.Encode([]rune(hddControlString(HDDCheck, tok)))
		if len(u) != 2+hddTokenUnits {
			t.Fatalf("token %#x encoded to %d units, want %d",
				tok, len(u), 2+hddTokenUnits)
		}
		for i, unit := range u[2:] {
			if unit&hddTokenMask != hddTokenBase {
				t.Errorf("token %#x unit %d is %#04x — outside the range "+
					"_hook4 accepts", tok, i, unit)
			}
		}
	}
}

// The round trip, over exactly the arithmetic the two ends actually perform:
// this side splits a token into four 12-bit units, _hook4 folds them into two
// 24-bit words, and ParseStatus puts them back together. A shift or an endian
// mistake anywhere in that chain is a token that never matches.
func TestHDDTokenRoundTripsThroughThePayloadsArithmetic(t *testing.T) {
	for _, tok := range []uint64{1, 0xFFF, 0x123456789ABC, hddTokenMax} {
		u := utf16.Encode([]rune(hddControlString(HDDCheck, tok)))

		// What _hook4 does: mask each unit to 12 bits and pack them in pairs.
		w0 := uint32(u[2]&0x0FFF)<<12 | uint32(u[3]&0x0FFF)
		w1 := uint32(u[4]&0x0FFF)<<12 | uint32(u[5]&0x0FFF)

		// What _hdd_report sends and udp.ParseStatus reads back.
		if got := uint64(w0)<<24 | uint64(w1); got != tok {
			t.Errorf("token %#x came back as %#x", tok, got)
		}
	}
}

// The case the pending table alone cannot decide, and the reason the nonce
// exists: two players behind ONE address asking for the SAME verb. Only the
// token separates them.
func TestHDDTokenSeparatesTwoAskersAtOneAddress(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	first := hddTestSession(sm, "203.0.113.25", 5900, 111)
	second := hddTestSession(sm, "203.0.113.25", 5901, 222)

	const tokFirst, tokSecond = 0x0A0A0A0A0A0A, 0x0B0B0B0B0B0B
	noteHDDRequest(first, hddTestPlayer(9, "First"), HDDCheck, tokFirst)
	noteHDDRequest(second, hddTestPlayer(10, "Second"), HDDCheck, tokSecond)

	// The SECOND player's console answers first. Ordering would have given this
	// to the first player, who is still waiting.
	told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.25", Verb: HDDCheck, Token: tokSecond,
		Describe: "the disk is fine",
	})
	if told != 1 {
		t.Fatalf("told %d players, want 1", told)
	}
	if second.Queue().PendingCount() == 0 {
		t.Error("the answer went to the wrong player: the second asker got nothing")
	}
	if first.Queue().PendingCount() != 0 {
		t.Error("the first asker was told about the second asker's disk")
	}
}

// A token the server never issued is not a weaker match — it is no match at
// all. Falling back to the verb here would mean a forged report could collect
// someone else's answer just by supplying a wrong token.
func TestHDDWrongTokenIsNotAFallback(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	asker := hddTestSession(sm, "203.0.113.27", 6000, 333)
	noteHDDRequest(asker, hddTestPlayer(11, "Asker"), HDDCheck, 0x0C0C0C0C0C0C)

	if told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.27", Verb: HDDCheck, Token: 0x0D0D0D0D0D0D,
		Describe: "the disk is fine",
	}); told != 0 {
		t.Errorf("a report with an unissued token was delivered to %d players", told)
	}
	// ...and the real ask must still be there for the real report.
	if _, ok := takeHDDRequest("203.0.113.27", HDDCheck, 0x0C0C0C0C0C0C); !ok {
		t.Error("the forged report consumed the genuine ask")
	}
}

// An older payload echoes nothing, and must still be answered — otherwise
// shipping the new server would silently stop talking to every console that
// has not been updated yet.
func TestHDDTokenlessReportStillMatches(t *testing.T) {
	resetHDDRequests()
	sm := session.NewManager()
	asker := hddTestSession(sm, "203.0.113.29", 6100, 444)
	noteHDDRequest(asker, hddTestPlayer(12, "OldPayload"), HDDCheck, 0x0E0E0E0E0E0E)

	if told := ReportHDDStatus(sm, nil, HDDReport{
		IP: "203.0.113.29", Verb: HDDCheck, Token: 0,
		Describe: "the disk is fine",
	}); told != 1 {
		t.Errorf("a console that echoes no token was told %d times, want 1", told)
	}
}

// A nonce that repeats is not a nonce. This cannot prove randomness, but it
// does catch the mistakes that actually happen: a zero token, a constant, and
// a mask that throws away most of the width.
func TestHDDTokensAreDistinctAndInRange(t *testing.T) {
	seen := map[uint64]bool{}
	var all uint64
	for i := 0; i < 512; i++ {
		tok := newHDDToken()
		if tok == 0 {
			t.Fatal("newHDDToken returned 0, which means 'no token'")
		}
		if tok > hddTokenMax {
			t.Fatalf("token %#x is wider than the %d bits the wire carries",
				tok, hddTokenBits)
		}
		if seen[tok] {
			t.Fatalf("token %#x was issued twice in 512 draws", tok)
		}
		seen[tok] = true
		all |= tok
	}
	if all != hddTokenMax {
		t.Errorf("512 tokens never set bits %#x — the generator is not using "+
			"the full width", hddTokenMax&^all)
	}
}

// ---------------------------------------------------------------------------
// The console's font is ASCII.
//
// An em dash draws as "?" on the PS2 — seen on a real screen: the format line
// read "FORMAT THE HARD DISK ? erases everything". Nothing in the pipeline
// complains, because UTF-16 carries the character perfectly and it is the FONT
// that has no glyph, so the only way this is caught is by asserting it here.
//
// Comments and log lines are exempt and are not scanned: they go to a terminal
// and a journal, neither of which is the PS2's font.
// ---------------------------------------------------------------------------

// asciiOnly reports the first character a PS2 font cannot draw.
func asciiOnly(s string) (rune, bool) {
	for _, r := range s {
		// Printable ASCII plus the newline the option box never sees anyway.
		if r < 0x20 || r > 0x7E {
			return r, false
		}
	}
	return 0, true
}

func TestPlayerFacingTextIsASCII(t *testing.T) {
	cases := map[string][]string{
		"HDDGreeting":          {HDDGreeting},
		"hddMenuMain":          hddMenuMain,
		"hddMenuConfirmFormat": hddMenuConfirmFormat,
		"hddChatConfirmHint":   {hddChatConfirmHint},
	}
	for name, list := range cases {
		for _, s := range list {
			if r, ok := asciiOnly(s); !ok {
				t.Errorf("%s: %q contains %q (U+%04X), which the console draws "+
					"as '?'", name, s, r, r)
			}
		}
	}
}

// Everything the player reads goes through one of a handful of sinks, so the
// literals handed to them are found in the AST rather than listed here — a list
// would be a second place to forget. The whole package is scanned, not just this
// file: the em dash that started this was in hddsetup.go, and sixteen more were
// sitting in chat.go's #help.
func TestPlayerFacingSinksAreASCII(t *testing.T) {
	sinks := map[string]bool{
		"gmReply": true, "reply": true, "npcSay": true,
		"buildClientMessage": true, "buildColoredMessage": true,
		"sendOptionBox": true,
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}
	checked := 0
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var name string
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if !sinks[name] {
					return true
				}
				for _, arg := range call.Args {
					ast.Inspect(arg, func(m ast.Node) bool {
						lit, ok := m.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							return true
						}
						str, err := strconv.Unquote(lit.Value)
						if err != nil {
							return true
						}
						checked++
						if r, ok := asciiOnly(str); !ok {
							t.Errorf("%s: %s(%q) contains %q (U+%04X), which "+
								"the console draws as '?'",
								fset.Position(lit.Pos()), name, str, r, r)
						}
						return true
					})
				}
				return true
			})
		}
	}
	if checked == 0 {
		t.Fatal("found no sink string literals — the scan is not working")
	}
	t.Logf("scanned %d literals across the package", checked)
}

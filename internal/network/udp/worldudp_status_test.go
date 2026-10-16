package udp

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The magic the PAYLOAD stamps on a report, read out of its layout header. The
// two ends are built by different toolchains from different files and shipped
// as a pair; nothing but this test keeps them agreeing on four bytes.
func TestStatusMagicMatchesPayload(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"re-tools", "hdd_sd", "layout.h"))
	if err != nil {
		t.Skipf("payload source unreadable (%v)", err)
	}
	m := regexp.MustCompile(`(?m)^#define\s+RPT_MAGIC\s+0x([0-9A-Fa-f]+)`).
		FindSubmatch(b)
	if m == nil {
		t.Fatal("hdd_sd/layout.h no longer defines RPT_MAGIC")
	}
	v, _ := strconv.ParseUint(string(m[1]), 16, 32)
	// The payload stores the word little-endian, so the bytes on the wire are
	// its low byte first — which is what the magic must equal.
	var got [4]byte
	binary.LittleEndian.PutUint32(got[:], uint32(v))
	if got != statusMagic {
		t.Errorf("payload RPT_MAGIC %#08x puts %q on the wire, server wants %q",
			v, got, statusMagic)
	}
}

func statusDatagram(verb, result uint32, id int32) []byte {
	b := make([]byte, ReqSize)
	copy(b[0:4], statusMagic[:])
	binary.LittleEndian.PutUint32(b[4:8], verb)
	binary.LittleEndian.PutUint32(b[8:12], result)
	binary.LittleEndian.PutUint32(b[12:16], uint32(id))
	return b
}

func TestParseStatusAcceptsOnlyItsOwn(t *testing.T) {
	if _, ok := ParseStatus(statusDatagram(1, 0x5100FF01, 47)); !ok {
		t.Error("a well-formed report was rejected")
	}
	// A world REQUEST must not be read as a report: same size, same socket, and
	// only the magic separates them.
	req := make([]byte, ReqSize)
	copy(req[0:4], reqMagic[:])
	if _, ok := ParseStatus(req); ok {
		t.Error("a world request was accepted as a status report")
	}
	if _, ok := ParseStatus(statusDatagram(1, 0, 0)[:ReqSize-1]); ok {
		t.Error("a short datagram was accepted")
	}
}

// Describe is what the player is told, so each class of outcome must produce a
// distinguishable sentence — especially the two that both mean "nothing
// happened" for completely different reasons.
func TestStatusDescribe(t *testing.T) {
	cases := []struct {
		name      string
		rep       StatusReport
		mustMatch string
	}{
		{"already there", StatusReport{Result: 0x5100FF01}, `already there`},
		{"created", StatusReport{Result: 0x5101FF01}, `created and formatted`},
		{"no disk", StatusReport{Result: 0x5111FF01}, `NO HARD DISK`},
		{"needs format", StatusReport{Result: 0x5113FF01}, `not formatted`},
		{"module missing", StatusReport{ID: -201}, `would not load`},
		{"not our word", StatusReport{Result: 0x12345678}, `did not run`},
		{"unknown step", StatusReport{Result: 0x517FFF01}, `does not know`},
	}
	for _, c := range cases {
		if got := c.rep.Describe(); !regexp.MustCompile(c.mustMatch).MatchString(got) {
			t.Errorf("%s: %q does not match %q", c.name, got, c.mustMatch)
		}
	}
}

// A failure must carry its errno: "formatting failed" without one is a bug
// report nobody can act on.
func TestStatusDescribeCarriesErrno(t *testing.T) {
	got := StatusReport{Result: 0x51152C01}.Describe()
	if !regexp.MustCompile(`error 44`).MatchString(got) {
		t.Errorf("%q does not name the errno (0x2C = 44)", got)
	}
}

// statusDatagramWithToken is statusDatagram plus the nonce, laid out the way
// _hdd_report writes it: two 24-bit words at +0x10 and +0x14, most significant
// half first.
func statusDatagramWithToken(verb uint32, token uint64) []byte {
	b := statusDatagram(verb, 0x5100FF01, 47)
	binary.LittleEndian.PutUint32(b[16:20], uint32(token>>24)&0xFFFFFF)
	binary.LittleEndian.PutUint32(b[20:24], uint32(token)&0xFFFFFF)
	return b
}

// The nonce is what tells two consoles behind one address apart, so its two
// halves must go back together in the right order and the right width.
func TestParseStatusReadsTheToken(t *testing.T) {
	for _, tok := range []uint64{1, 0xFFF, 0x123456789ABC, 1<<TokenBits - 1} {
		rep, ok := ParseStatus(statusDatagramWithToken(1, tok))
		if !ok {
			t.Fatalf("token %#x: the datagram was rejected", tok)
		}
		if rep.Token != tok {
			t.Errorf("token %#x came back as %#x", tok, rep.Token)
		}
	}
}

// ZERO IS THE COMPATIBILITY CASE, not an error: it is what a payload older than
// the nonce sends, and the server has to read it as "match this the old way"
// rather than reject the report and tell the player nothing.
func TestParseStatusTreatsAbsentAndImpossibleTokensAsNone(t *testing.T) {
	rep, ok := ParseStatus(statusDatagram(1, 0x5100FF01, 47))
	if !ok || rep.Token != 0 {
		t.Errorf("an old payload's report parsed as ok=%v token=%#x", ok, rep.Token)
	}
	// Wider than 24 bits in either half: not our encoder's output, so not a
	// token. Accepting it would let a forger claim any 48-bit value with a
	// datagram our own payload could never have produced.
	for _, bad := range [][2]uint32{{0x1000000, 0}, {0, 0x1000000}} {
		b := statusDatagram(1, 0x5100FF01, 47)
		binary.LittleEndian.PutUint32(b[16:20], bad[0])
		binary.LittleEndian.PutUint32(b[20:24], bad[1])
		rep, ok := ParseStatus(b)
		if !ok || rep.Token != 0 {
			t.Errorf("halves %#x/%#x parsed as ok=%v token=%#x",
				bad[0], bad[1], ok, rep.Token)
		}
	}
}

// TokenBits is not a free choice: it is CMD_TOKN units of 12 bits each, and the
// payload's decoder is what fixes both numbers. Widening one end alone produces
// tokens that never match, which is indistinguishable from an old payload and
// so would degrade silently rather than fail.
func TestTokenBitsMatchPayload(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"re-tools", "hdd_sd.S"))
	if err != nil {
		t.Skipf("payload source unreadable (%v)", err)
	}
	m := regexp.MustCompile(`(?m)^\.equ\s+CMD_TOKN,\s*(\d+)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("hdd_sd.S no longer defines CMD_TOKN")
	}
	units, _ := strconv.Atoi(string(m[1]))
	if units*12 != TokenBits {
		t.Errorf("the payload carries %d units = %d bits; TokenBits is %d",
			units, units*12, TokenBits)
	}
}

// Every step EQPREP can return must have wording here. The two files are a C
// header in another language in another directory and a Go map; the first
// successful in-game run returned OK_ABSENT (0x04) and the player was told the
// server "does not know" it, which is a correct outcome rendered as a fault.
func TestPrepStepNamesCoverEQPREP(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"open-irx", "eqprep", "src", "eqprep.c"))
	if err != nil {
		t.Skipf("eqprep source unreadable (%v)", err)
	}
	// #define OK_ABSENT 0x04 /* ... */   and the ERR_ ones alongside them.
	re := regexp.MustCompile(`(?m)^#define\s+(OK_\w+|ERR_\w+|PAYLOAD_\w+)\s+0x([0-9A-Fa-f]{2})`)
	found := 0
	for _, m := range re.FindAllSubmatch(b, -1) {
		name := string(m[1])
		step, _ := strconv.ParseInt(string(m[2]), 16, 32)
		found++
		if _, ok := prepStepNames[int(step)]; !ok {
			t.Errorf("eqprep.c defines %s = %#02x and this server has no "+
				"wording for it", name, step)
		}
	}
	if found == 0 {
		t.Fatal("no step defines matched — the header's shape changed")
	}
	// And nothing invented here that the payload cannot produce.
	defined := map[int]bool{}
	for _, m := range re.FindAllSubmatch(b, -1) {
		v, _ := strconv.ParseInt(string(m[2]), 16, 32)
		defined[int(v)] = true
	}
	for step := range prepStepNames {
		if !defined[step] {
			t.Errorf("this server describes step %#02x, which eqprep.c does "+
				"not define", step)
		}
	}
}

// A refusal is not a fault. Steps the PAYLOAD synthesises carry no errno, so
// decorating them with one would render "the game is using the world partition
// right now (error 255)" — which reads as the refusal having gone wrong.
func TestSyntheticStepsCarryNoErrno(t *testing.T) {
	got := StatusReport{Result: 0x5120FF01}.Describe()
	if regexp.MustCompile(`error`).MatchString(got) {
		t.Errorf("%q decorates a payload refusal with an errno", got)
	}
	// ...while a real EQPREP failure still must carry one.
	if !regexp.MustCompile(`error 44`).MatchString(
		StatusReport{Result: 0x51152C01}.Describe()) {
		t.Error("a real failure lost its errno")
	}
}

// The PS2 font has no glyph for an em dash and draws '?' instead — measured on
// a real screen. These sentences are spoken by the technician, so they are
// subject to the same limit as anything else the console prints.
func TestStepDescriptionsAreASCII(t *testing.T) {
	bad := func(s string) (rune, bool) {
		for _, r := range s {
			if r < 0x20 || r > 0x7E {
				return r, true
			}
		}
		return 0, false
	}
	for step, name := range prepStepNames {
		if r, is := bad(name); is {
			t.Errorf("step %#02x: %q contains %q (U+%04X), drawn as '?'",
				step, name, r, r)
		}
	}
	// Describe's own fallbacks reach the player too, and only on the paths
	// nobody exercises by hand — which is exactly where a stray dash survives.
	for _, rep := range []StatusReport{
		{Result: 0x5100FF01}, {Result: 0x5120FF01}, {Result: 0x51152C01},
		{ID: -201}, {Result: 0x12345678}, {Result: 0x517FFF01},
	} {
		if r, is := bad(rep.Describe()); is {
			t.Errorf("Describe(%#08x, id=%d) = %q contains %q (U+%04X)",
				rep.Result, rep.ID, rep.Describe(), r, r)
		}
	}
}

// payloadEquInt resolves a `.equ NAME, <expr>` from hdd_sd.S over integers and
// other .equ names, which is every form these take (`48`, `DLREQ_LEN-12`).
func payloadEquInt(t *testing.T, src, name string, depth int) int {
	t.Helper()
	if depth > 8 {
		t.Fatalf(".equ %s recurses", name)
	}
	m := regexp.MustCompile(`(?m)^\.equ\s+` + regexp.QuoteMeta(name) +
		`\s*,\s*([^#\n]+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("hdd_sd.S has no .equ %s", name)
	}
	total, sign := 0, 1
	for _, tok := range regexp.MustCompile(`[+\-]|\w+`).
		FindAllString(strings.TrimSpace(m[1]), -1) {
		switch tok {
		case "+":
			sign = 1
		case "-":
			sign = -1
		default:
			v, err := strconv.ParseInt(strings.TrimPrefix(tok, "0x"), 0, 64)
			if strings.HasPrefix(tok, "0x") {
				v, err = strconv.ParseInt(tok[2:], 16, 64)
			}
			if err != nil {
				v = int64(payloadEquInt(t, src, tok, depth+1))
			}
			total += sign * int(v)
			sign = 1
		}
	}
	return total
}

// THE WIRE CONSTANTS, pinned to the payload that has to agree with them.
//
// These four carried "must match udp.X" comments on both sides and nothing
// checked either. A mismatch does not fail loudly: the stager sizes its buffer
// from its own DL_CHUNK and reads a SHORT reply as end-of-file, so a server
// that disagreed would hand a console a truncated world of plausible length —
// which is the one outcome MaxChunk's own comment says must never happen.
func TestWireConstantsMatchPayload(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"re-tools", "hdd_sd.S"))
	if err != nil {
		t.Skipf("payload source unreadable (%v)", err)
	}
	src := string(b)
	for _, c := range []struct {
		equ  string
		mine int
	}{
		{"DLREQ_LEN", ReqSize},
		{"DL_NAMELEN", NameLen},
		{"DL_HDR", ReplyHeader},
		{"DL_CHUNK", MaxChunk},
	} {
		if got := payloadEquInt(t, src, c.equ, 0); got != c.mine {
			t.Errorf("%s is %d in the payload and %d here", c.equ, got, c.mine)
		}
	}
}

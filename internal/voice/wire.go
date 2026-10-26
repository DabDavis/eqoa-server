package voice

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The signaling as both of its carriers frame it (0x7E09 on the game
// connection, {"signal"} on the demo's WebSocket): a message is cut into
// parts of at most MaxChunk bytes, each part {seq, kind, part, parts, chunk},
// and put back together by seq.

const (
	// KindOffer is the client's SDP offer; KindAnswer the server's answer;
	// KindCandidate an ICE candidate (the JSON of RTCIceCandidateInit), both
	// ways; KindBye ends the connection (from the server: offer again).
	KindOffer     = 1
	KindAnswer    = 2
	KindCandidate = 3
	KindBye       = 4

	// MaxChunk is the most text one part carries; MaxParts the most parts a
	// message has (an offer is a few KB).
	MaxChunk = 900
	MaxParts = 64

	// a sender's assemblies: at most this many in flight and bytes held, each
	// dropped this long after its first part
	maxAssemblies  = 4
	maxAssembled   = 64 * 1024
	assemblyMaxAge = 10 * time.Second

	// a session's allowance of signaling, from its client: messages and bytes
	// a second, a second's worth at once
	RateMsgs  = 50
	RateBytes = 64 * 1024
)

// Split cuts text into parts of at most MaxChunk bytes, each ending on a
// whole UTF-8 character (a client may decode each part on its own); an empty
// text is one empty part.
func Split(text string) []string {
	var out []string
	for len(text) > MaxChunk {
		n := MaxChunk
		for n > 0 && !utf8.RuneStart(text[n]) {
			n--
		}
		if n == 0 {
			n = MaxChunk
		}
		out = append(out, text[:n])
		text = text[n:]
	}
	return append(out, text)
}

// ErrSignal is a part that does not belong to any message.
var ErrSignal = errors.New("malformed signal part")

// CheckPart is a client's part as both carriers accept it: a kind a client
// sends (an offer, a candidate or a bye), part of 1..MaxParts parts, a chunk
// of at most MaxChunk bytes.
func CheckPart(kind byte, part, parts int, chunk string) error {
	switch {
	case kind != KindOffer && kind != KindCandidate && kind != KindBye:
		return fmt.Errorf("%w: kind %d", ErrSignal, kind)
	case parts < 1 || parts > MaxParts || part < 0 || part >= parts:
		return fmt.Errorf("%w: part %d of %d", ErrSignal, part, parts)
	case len(chunk) > MaxChunk:
		return fmt.Errorf("%w: a %d-byte chunk", ErrSignal, len(chunk))
	}
	return nil
}

// Assembler puts one sender's parts back together, by seq.
type Assembler struct {
	inFlight map[uint32]*assembly
	held     int
}

type assembly struct {
	kind   byte
	chunks []string
	have   []bool
	got    int
	size   int
	first  time.Time
}

// Add takes one part (CheckPart'ed). done is true with the whole text once
// every part of its message is in; a part that contradicts its message (its
// kind or count), or one past the limits, drops the message.
func (a *Assembler) Add(seq uint32, kind byte, part, parts int, chunk string, now time.Time) (text string, done bool) {
	if parts == 1 {
		return chunk, true
	}
	if a.inFlight == nil {
		a.inFlight = map[uint32]*assembly{}
	}
	for s, m := range a.inFlight {
		if now.Sub(m.first) > assemblyMaxAge {
			a.drop(s)
		}
	}
	m := a.inFlight[seq]
	if m == nil {
		if len(a.inFlight) >= maxAssemblies {
			var oldest uint32
			var at time.Time
			for s, o := range a.inFlight {
				if at.IsZero() || o.first.Before(at) {
					oldest, at = s, o.first
				}
			}
			a.drop(oldest)
		}
		m = &assembly{kind: kind, chunks: make([]string, parts), have: make([]bool, parts), first: now}
		a.inFlight[seq] = m
	}
	if m.kind != kind || len(m.chunks) != parts || a.held+len(chunk) > maxAssembled {
		a.drop(seq)
		return "", false
	}
	if m.have[part] {
		return "", false // a repeat
	}
	m.chunks[part], m.have[part] = chunk, true
	m.got++
	m.size += len(chunk)
	a.held += len(chunk)
	if m.got < parts {
		return "", false
	}
	a.drop(seq)
	return strings.Join(m.chunks, ""), true
}

func (a *Assembler) drop(seq uint32) {
	if m := a.inFlight[seq]; m != nil {
		a.held -= m.size
		delete(a.inFlight, seq)
	}
}

// Rate is a session's allowance: RateMsgs messages and RateBytes bytes a
// second, up to a second's worth at once.
type Rate struct {
	msgs, bytes float64
	at          time.Time
}

// Take spends one message of n bytes, false when the allowance has not got
// it (nothing is spent then).
func (r *Rate) Take(n int, now time.Time) bool {
	if r.at.IsZero() {
		r.msgs, r.bytes = RateMsgs, RateBytes
	} else if dt := now.Sub(r.at).Seconds(); dt > 0 {
		r.msgs = min(RateMsgs, r.msgs+dt*RateMsgs)
		r.bytes = min(RateBytes, r.bytes+dt*RateBytes)
	}
	r.at = now
	if r.msgs < 1 || r.bytes < float64(n) {
		return false
	}
	r.msgs--
	r.bytes -= float64(n)
	return true
}

// Credential is a TURN server's REST credential for username (coturn's
// use-auth-secret, and this server's own TURN): base64(HMAC-SHA1(secret,
// username)).
func Credential(secret, username string) string {
	m := hmac.New(sha1.New, []byte(secret))
	m.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// Username is the TURN username of the session with voice id id, good until
// expiry: "<unix expiry>:v<id as 8 hex>" (no name in it).
func Username(expiry time.Time, id uint32) string {
	return fmt.Sprintf("%d:v%08x", expiry.Unix(), id)
}

// usernameExpiry is a TURN username's expiry, false when it has none.
func usernameExpiry(username string) (time.Time, bool) {
	ts, _, _ := strings.Cut(username, ":")
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	return time.Unix(n, 0), true
}

// ChannelID is a well-formed client-made channel id with that prefix ("d:"
// a demo channel, "r:" a passworded room): the prefix and exactly 32 hex
// digits, given back in lower case; false otherwise.
func ChannelID(prefix, id string) (string, bool) {
	if len(id) != len(prefix)+32 || !strings.HasPrefix(id, prefix) {
		return "", false
	}
	for _, c := range id[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(id), true
}

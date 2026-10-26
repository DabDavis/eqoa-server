package entity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Group voice (voice.go) on the assist world (assist_test.go): the asker and
// the leader grouped, and a third player of no group.

// testVoice is a voice of made-up URLs and secret.
var testVoice = VoiceSettings{
	TurnURLs: []string{"turn:203.0.113.5:3479?transport=udp", "turn:203.0.113.5:3479?transport=tcp"},
	Secret:   "a-test-secret-only",
	TTL:      2 * time.Hour,
}

var voiceT0 = time.Unix(1_800_000_000, 0)

// voiceMsgs takes the bodies of the messages of op queued for s.
func voiceMsgs(s *session.Session, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == op {
			out = append(out, d[2:])
		}
	}
	return out
}

func drainVoice(ss ...*session.Session) {
	for _, s := range ss {
		s.Queue().GetPending()
	}
}

// voiceStr reads a voice string (u16 length, UTF-8) off b.
func voiceStr(t *testing.T, b []byte) (string, []byte) {
	t.Helper()
	if len(b) < 2 {
		t.Fatalf("no string in %x", b)
	}
	n := int(binary.LittleEndian.Uint16(b))
	if len(b) < 2+n {
		t.Fatalf("a %d-byte string in %d bytes", n, len(b)-2)
	}
	return string(b[2 : 2+n]), b[2+n:]
}

type peerRow struct {
	id       uint32
	name     string
	offerer  bool
	channels byte
}

// peersOf decodes an OpcodeVoicePeers body.
func peersOf(t *testing.T, b []byte) []peerRow {
	t.Helper()
	n := int(b[0])
	b = b[1:]
	var out []peerRow
	for i := 0; i < n; i++ {
		id := binary.LittleEndian.Uint32(b)
		name, rest := voiceStr(t, b[4:])
		out = append(out, peerRow{id, name, rest[0] == 1, rest[1]})
		b = rest[2:]
	}
	if len(b) != 0 {
		t.Fatalf("%d bytes past the peers", len(b))
	}
	return out
}

type iceRow struct{ url, user, cred string }

// configOf decodes an OpcodeVoiceConfig body.
func configOf(t *testing.T, b []byte) ([]iceRow, uint32) {
	t.Helper()
	n := int(b[0])
	b = b[1:]
	var out []iceRow
	for i := 0; i < n; i++ {
		var r iceRow
		r.url, b = voiceStr(t, b)
		r.user, b = voiceStr(t, b)
		r.cred, b = voiceStr(t, b)
		out = append(out, r)
	}
	if len(b) != 4 {
		t.Fatalf("%d bytes for the seconds", len(b))
	}
	return out, binary.LittleEndian.Uint32(b)
}

// newVoiceWorld is the assist world with voice, its two grouped, and a third
// player of no group on its own session.
func newVoiceWorld(t *testing.T) (*assistWorld, *session.Session, *Player) {
	t.Helper()
	aw := newAssistWorld(t)
	aw.group()
	aw.w.SetVoice(testVoice)
	c := &Player{ServerID: 1003, Name: "Loner", Level: 20}
	s := aw.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7010}, 10, 1, 1, 2, true)
	aw.w.Players[c.ServerID] = c
	s.SetECSEntity(c)
	s.SetCharacterInWorld(true)
	return aw, s, c
}

func (aw *assistWorld) voicePass(now time.Time) {
	aw.w.mu.Lock()
	aw.w.TickCount += 5
	aw.w.voiceSyncSystem(now)
	aw.w.mu.Unlock()
}

// coturn's REST credential, against a vector made with Python's hmac; the
// control: another secret gives another.
func TestTurnCredentialIsCoturns(t *testing.T) {
	if got := TurnCredential("a-test-secret-only", "1700000000:v0000002a"); got != "fL+E108VuYhsBuWvBPKRHO+mbiA=" {
		t.Fatalf("credential %q", got)
	}
	if got := TurnCredential("another-secret", "1700000000:v0000002a"); got != "cFEbMLOsDCLnJBmBOQ0Zc24eseI=" || got == TurnCredential("a-test-secret-only", "1700000000:v0000002a") {
		t.Fatalf("another secret: %q", got)
	}
}

// Voice on: the credentials at once (every URL, the username's expiry the
// TTL from now, the credential coturn's), then the peers: none while the
// other member's voice is off, each other once it is on, exactly one of the
// two making the offer; a player of no group, its voice on, in no one's set.
// Off: the player's own set emptied at once, and the other's at the next pass.
func TestVoicePeersAreTheGroupsVoiceOnMembers(t *testing.T) {
	aw, loner, c := newVoiceWorld(t)
	if !aw.w.SetVoiceOn(loner, true, voiceT0) {
		t.Fatal("the loner has a player")
	}
	if !aw.w.SetVoiceOn(aw.asker, true, voiceT0) {
		t.Fatal("the asker has a player")
	}
	cfg := voiceMsgs(aw.asker, protocol.OpcodeVoiceConfig)
	if len(cfg) != 1 {
		t.Fatalf("%d configs, want 1", len(cfg))
	}
	ice, secs := configOf(t, cfg[0])
	if len(ice) != 2 || secs != 7200 || ice[0].url != testVoice.TurnURLs[0] || ice[1].url != testVoice.TurnURLs[1] {
		t.Fatalf("ice %+v, %d s", ice, secs)
	}
	want := fmt.Sprintf("%d:v%08x", voiceT0.Add(2*time.Hour).Unix(), aw.a.VoiceID)
	if ice[0].user != want || ice[0].cred != TurnCredential(testVoice.Secret, want) || ice[1].user != want {
		t.Fatalf("username %q (want %q), credential %q", ice[0].user, want, ice[0].cred)
	}
	if got := voiceMsgs(aw.asker, protocol.OpcodeVoicePeers); len(got) != 1 || len(peersOf(t, got[0])) != 0 {
		t.Fatalf("the leader's voice is off: peers %x", got)
	}
	drainVoice(aw.asker, aw.leader, loner)

	aw.w.SetVoiceOn(aw.leader, true, voiceT0)
	aw.voicePass(voiceT0)
	pa, pb := voiceMsgs(aw.asker, protocol.OpcodeVoicePeers), voiceMsgs(aw.leader, protocol.OpcodeVoicePeers)
	if len(pa) != 1 || len(pb) != 1 {
		t.Fatalf("peers sent %d and %d times, want once each", len(pa), len(pb))
	}
	ra, rb := peersOf(t, pa[0]), peersOf(t, pb[0])
	if len(ra) != 1 || ra[0].id != aw.b.VoiceID || ra[0].name != "Leader" || len(rb) != 1 || rb[0].id != aw.a.VoiceID || rb[0].name != "Asker" {
		t.Fatalf("asker's peers %+v, leader's %+v", ra, rb)
	}
	if ra[0].channels != voiceChanGroup || rb[0].channels != voiceChanGroup {
		t.Fatalf("channels %d and %d, want the group's", ra[0].channels, rb[0].channels)
	}
	if ra[0].offerer == rb[0].offerer || ra[0].offerer != (aw.a.VoiceID < aw.b.VoiceID) {
		t.Fatalf("offerers: asker %v, leader %v (ids %08x, %08x)", ra[0].offerer, rb[0].offerer, aw.a.VoiceID, aw.b.VoiceID)
	}
	if got := voiceMsgs(loner, protocol.OpcodeVoicePeers); len(got) != 0 {
		t.Fatalf("the loner's set changed: %x", got)
	}
	for _, r := range append(ra, rb...) {
		if r.id == c.VoiceID {
			t.Fatal("a player of no group is a peer")
		}
	}
	drainVoice(aw.asker, aw.leader, loner)

	aw.voicePass(voiceT0) // unchanged: nothing
	if got := append(voiceMsgs(aw.asker, protocol.OpcodeVoicePeers), voiceMsgs(aw.leader, protocol.OpcodeVoicePeers)...); len(got) != 0 {
		t.Fatalf("unchanged peers sent again: %x", got)
	}

	aw.w.SetVoiceOn(aw.asker, false, voiceT0)
	if got := voiceMsgs(aw.asker, protocol.OpcodeVoicePeers); len(got) != 1 || len(peersOf(t, got[0])) != 0 {
		t.Fatalf("off: the asker's own set %x, want one empty", got)
	}
	aw.voicePass(voiceT0)
	if got := voiceMsgs(aw.leader, protocol.OpcodeVoicePeers); len(got) != 1 || len(peersOf(t, got[0])) != 0 {
		t.Fatalf("the asker off: the leader's set %x, want one empty", got)
	}
	drainVoice(aw.asker)
	aw.voicePass(voiceT0)
	if got := voiceMsgs(aw.asker, protocol.OpcodeVoicePeers); len(got) != 0 {
		t.Fatalf("voice off, sent peers: %x", got)
	}
}

// signal builds a client's OpcodeVoiceSignal (after the opcode).
func signal(to, seq uint32, kind byte, part, parts uint16, chunk string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, to)
	b = binary.LittleEndian.AppendUint32(b, seq)
	b = append(b, kind)
	b = binary.LittleEndian.AppendUint16(b, part)
	b = binary.LittleEndian.AppendUint16(b, parts)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(chunk)))
	return append(b, chunk...)
}

// A signal goes to the peer it names, named in its turn by the sender: a
// three-part offer arrives whole, in order; dropped are a signal to a player
// of another group, one from a voice-off sender, to a voice id no peer has,
// a chunk over 900 bytes, a length that does not add up, a kind of none of
// four, and the 51st message of one second.
func TestVoiceSignalsGoOnlyToAPeer(t *testing.T) {
	aw, loner, c := newVoiceWorld(t)
	for _, s := range []*session.Session{aw.asker, aw.leader, loner} {
		aw.w.SetVoiceOn(s, true, voiceT0)
	}
	drainVoice(aw.asker, aw.leader, loner)
	sdp := strings.Repeat("v=0 o=- 46117 2 IN IP4 127.0.0.1 ", 80) // ~2.6 KB
	var parts []string
	for i := 0; i < len(sdp); i += 900 {
		parts = append(parts, sdp[i:min(i+900, len(sdp))])
	}
	for i, p := range parts {
		if why := aw.w.RelayVoiceSignal(aw.asker, signal(aw.b.VoiceID, 7, voiceKindOffer, uint16(i), uint16(len(parts)), p), voiceT0); why != "" {
			t.Fatalf("part %d dropped: %s", i, why)
		}
	}
	got := voiceMsgs(aw.leader, protocol.OpcodeVoiceSignal)
	if len(got) != len(parts) {
		t.Fatalf("%d parts relayed, want %d", len(got), len(parts))
	}
	var whole []byte
	for i, m := range got {
		if from := binary.LittleEndian.Uint32(m); from != aw.a.VoiceID {
			t.Fatalf("part %d from %08x, want the asker's %08x", i, from, aw.a.VoiceID)
		}
		if part := binary.LittleEndian.Uint16(m[9:]); int(part) != i {
			t.Fatalf("part %d came as %d", i, part)
		}
		chunk, rest := voiceStr(t, m[13:])
		if len(rest) != 0 {
			t.Fatalf("part %d: %d bytes past its chunk", i, len(rest))
		}
		whole = append(whole, chunk...)
	}
	if !bytes.Equal(whole, []byte(sdp)) {
		t.Fatal("the offer did not arrive whole")
	}
	drainVoice(aw.leader)

	cases := []struct {
		name string
		from *session.Session
		data []byte
	}{
		{"to a player of no group", aw.asker, signal(c.VoiceID, 8, voiceKindOffer, 0, 1, "x")},
		{"from a player of no group", loner, signal(aw.a.VoiceID, 1, voiceKindOffer, 0, 1, "x")},
		{"to no peer's id", aw.asker, signal(aw.a.VoiceID^0xFFFF, 9, voiceKindOffer, 0, 1, "x")},
		{"a 901-byte chunk", aw.asker, signal(aw.b.VoiceID, 10, voiceKindOffer, 0, 1, strings.Repeat("y", 901))},
		{"a length that does not add up", aw.asker, signal(aw.b.VoiceID, 11, voiceKindAnswer, 0, 1, "xy")[:16]},
		{"kind 5", aw.asker, signal(aw.b.VoiceID, 12, 5, 0, 1, "x")},
		{"part past its parts", aw.asker, signal(aw.b.VoiceID, 13, voiceKindCandidate, 2, 2, "x")},
	}
	for _, k := range cases {
		if why := aw.w.RelayVoiceSignal(k.from, k.data, voiceT0); why == "" {
			t.Errorf("%s: relayed", k.name)
		}
	}
	if got := append(append(voiceMsgs(aw.asker, protocol.OpcodeVoiceSignal), voiceMsgs(aw.leader, protocol.OpcodeVoiceSignal)...), voiceMsgs(loner, protocol.OpcodeVoiceSignal)...); len(got) != 0 {
		t.Fatalf("dropped signals reached someone: %d", len(got))
	}

	aw.w.SetVoiceOn(aw.leader, false, voiceT0)
	if why := aw.w.RelayVoiceSignal(aw.asker, signal(aw.b.VoiceID, 14, voiceKindBye, 0, 1, ""), voiceT0); why == "" {
		t.Error("to a member whose voice is off: relayed")
	}
	if why := aw.w.RelayVoiceSignal(aw.leader, signal(aw.a.VoiceID, 1, voiceKindBye, 0, 1, ""), voiceT0); why == "" {
		t.Error("from a member whose voice is off: relayed")
	}

	aw.w.SetVoiceOn(aw.leader, true, voiceT0)
	drainVoice(aw.asker, aw.leader)
	later := voiceT0.Add(time.Minute) // a full allowance
	n := 0
	for i := 0; i < voiceMsgsPerSec+1; i++ {
		if aw.w.RelayVoiceSignal(aw.asker, signal(aw.b.VoiceID, uint32(100+i), voiceKindCandidate, 0, 1, "{}"), later) == "" {
			n++
		}
	}
	if n != voiceMsgsPerSec {
		t.Fatalf("%d of %d relayed in one second, want %d", n, voiceMsgsPerSec+1, voiceMsgsPerSec)
	}
	if aw.w.RelayVoiceSignal(aw.asker, signal(aw.b.VoiceID, 999, voiceKindCandidate, 0, 1, "{}"), later.Add(time.Second)) != "" {
		t.Fatal("a second later the allowance did not refill")
	}
}

// Credentials go again before they run out (a quarter of a 2 h TTL, 30
// minutes, before), not sooner.
func TestVoiceCredentialsAreRenewedBeforeTheyRunOut(t *testing.T) {
	aw, _, _ := newVoiceWorld(t)
	aw.w.SetVoiceOn(aw.asker, true, voiceT0)
	drainVoice(aw.asker)
	aw.voicePass(voiceT0.Add(89 * time.Minute))
	if got := voiceMsgs(aw.asker, protocol.OpcodeVoiceConfig); len(got) != 0 {
		t.Fatalf("renewed 89 minutes in: %d", len(got))
	}
	aw.voicePass(voiceT0.Add(91 * time.Minute))
	got := voiceMsgs(aw.asker, protocol.OpcodeVoiceConfig)
	if len(got) != 1 {
		t.Fatalf("not renewed 91 minutes in: %d", len(got))
	}
	ice, _ := configOf(t, got[0])
	if want := fmt.Sprintf("%d:", voiceT0.Add(91*time.Minute+2*time.Hour).Unix()); !strings.HasPrefix(ice[0].user, want) {
		t.Fatalf("renewed as %q, want a username from %q", ice[0].user, want)
	}
}

// A server without voice (no URL or no secret) sends nothing and relays
// nothing, whatever a client asks.
func TestNoVoiceWithoutASecret(t *testing.T) {
	aw := newAssistWorld(t)
	aw.group()
	aw.w.SetVoice(VoiceSettings{TurnURLs: testVoice.TurnURLs})
	aw.w.SetVoiceOn(aw.asker, true, voiceT0)
	aw.w.SetVoiceOn(aw.leader, true, voiceT0)
	aw.voicePass(voiceT0)
	for _, op := range []protocol.GameOpcode{protocol.OpcodeVoiceConfig, protocol.OpcodeVoicePeers} {
		if got := append(voiceMsgs(aw.asker, op), voiceMsgs(aw.leader, op)...); len(got) != 0 {
			t.Fatalf("no secret: sent %v %x", op, got)
		}
	}
	if aw.w.RelayVoiceSignal(aw.asker, signal(aw.b.VoiceID, 1, voiceKindOffer, 0, 1, "x"), voiceT0) == "" {
		t.Fatal("no secret: relayed")
	}
}

// told counts the chat lines queued for s that say text.
func told(s *session.Session, text string) int {
	n := 0
	for _, m := range voiceMsgs(s, protocol.OpcodeClientMessage) {
		if bytes.Contains(m, wide16(text)) {
			n++
		}
	}
	return n
}

// guildFolk are n players of guild g on sessions of their own, no group,
// their entity ids from base.
func guildFolk(t *testing.T, w *World, g uint32, n int, base EntityID) ([]*Player, []*session.Session) {
	t.Helper()
	var ps []*Player
	var ss []*session.Session
	for i := 0; i < n; i++ {
		p := &Player{ServerID: base + EntityID(i), Name: fmt.Sprintf("Guildie%d", int(base)+i), Level: 30, GuildID: g}
		s := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, byte(base>>8), byte(base), byte(i+1)), Port: 8000 + i}, uint32(base)+uint32(i), 1, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		ps, ss = append(ps, p), append(ss, s)
	}
	return ps, ss
}

// A guild's voice channel holds the first ten to ask (the user's "also add
// guild voice chat", a channel of at most 10): each told it is in and each a
// peer of the other nine, on the guild's channel bit; the eleventh told once
// that it is full and no one's peer, its signal to one of them dropped; the
// first leaving lets it in, told so, and the first is then in no one's set.
// A member of another guild asking is in another channel.
func TestAGuildsVoiceChannelHoldsTen(t *testing.T) {
	aw := newAssistWorld(t)
	aw.w.SetVoice(testVoice)
	ps, ss := guildFolk(t, aw.w, 5, 11, 3000)
	for i, s := range ss {
		aw.w.SetVoiceFlags(s, VoiceFlagOn|VoiceFlagGuild, voiceT0.Add(time.Duration(i)*time.Millisecond))
	}
	aw.voicePass(voiceT0.Add(time.Second))
	for i := 0; i < 10; i++ {
		if told(ss[i], voiceGuildIn) != 1 || told(ss[i], voiceGuildFull) != 0 {
			t.Fatalf("guildie %d: told in %d times, full %d", i, told(ss[i], voiceGuildIn), told(ss[i], voiceGuildFull))
		}
	}
	if told(ss[10], voiceGuildFull) != 1 || told(ss[10], voiceGuildIn) != 0 {
		t.Fatalf("the eleventh: told full %d times, in %d", told(ss[10], voiceGuildFull), told(ss[10], voiceGuildIn))
	}
	last := func(s *session.Session) []peerRow {
		got := voiceMsgs(s, protocol.OpcodeVoicePeers)
		if len(got) == 0 {
			return nil
		}
		return peersOf(t, got[len(got)-1])
	}
	first := last(ss[0])
	if len(first) != 9 {
		t.Fatalf("the first has %d peers, want the other nine", len(first))
	}
	for _, r := range first {
		if r.channels != voiceChanGuild || r.id == ps[10].VoiceID {
			t.Fatalf("the first's peer %+v", r)
		}
	}
	if got := last(ss[10]); len(got) != 0 {
		t.Fatalf("the eleventh, waiting, has peers %+v", got)
	}
	if aw.w.RelayVoiceSignal(ss[10], signal(ps[1].VoiceID, 1, voiceKindOffer, 0, 1, "x"), voiceT0.Add(time.Second)) == "" {
		t.Fatal("the eleventh, waiting, signaled one of the ten")
	}
	for _, s := range ss {
		drainVoice(s)
	}

	aw.w.SetVoiceFlags(ss[0], VoiceFlagOn, voiceT0.Add(2*time.Second)) // the first leaves the channel
	aw.voicePass(voiceT0.Add(3 * time.Second))
	if told(ss[10], voiceGuildIn) != 1 {
		t.Fatalf("a place freed: the eleventh told in %d times", told(ss[10], voiceGuildIn))
	}
	if got := last(ss[10]); len(got) != 9 {
		t.Fatalf("the eleventh in: %d peers, want nine", len(got))
	}
	if got := last(ss[1]); len(got) != 9 {
		t.Fatalf("the second: %d peers, want nine (the first out, the eleventh in)", len(got))
	} else {
		for _, r := range got {
			if r.id == ps[0].VoiceID {
				t.Fatal("the first, out of the channel, is still a peer")
			}
		}
	}
	if got := last(ss[0]); len(got) != 0 {
		t.Fatalf("the first, out of the channel and in no group, has peers %+v", got)
	}
	if aw.w.RelayVoiceSignal(ss[10], signal(ps[1].VoiceID, 2, voiceKindOffer, 0, 1, "x"), voiceT0.Add(3*time.Second)) != "" {
		t.Fatal("in the channel, the eleventh's signal was dropped")
	}

	_, os := guildFolk(t, aw.w, 6, 1, 3900)
	if len(aw.w.Players) != 2+11+1 {
		t.Fatalf("%d players, want 14", len(aw.w.Players))
	}
	aw.w.SetVoiceFlags(os[0], VoiceFlagOn|VoiceFlagGuild, voiceT0.Add(4*time.Second))
	aw.voicePass(voiceT0.Add(5 * time.Second))
	if told(os[0], voiceGuildIn) != 1 || len(last(os[0])) != 0 {
		t.Fatal("another guild's member: not in its own, empty channel")
	}
	if aw.w.RelayVoiceSignal(os[0], signal(ps[1].VoiceID, 1, voiceKindOffer, 0, 1, "x"), voiceT0.Add(5*time.Second)) == "" {
		t.Fatal("another guild's member signaled into guild 5's channel")
	}
}

// A groupmate also in the guild's channel is one peer of both channels; gone
// to another guild it is the group's alone at the next pass (in its new
// guild's channel, not the old one's).
func TestAGroupmateInTheGuildChannelIsOnePeer(t *testing.T) {
	aw := newAssistWorld(t)
	aw.group()
	aw.w.SetVoice(testVoice)
	aw.a.GuildID, aw.b.GuildID = 9, 9
	aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn|VoiceFlagGuild, voiceT0)
	aw.w.SetVoiceFlags(aw.leader, VoiceFlagOn|VoiceFlagGuild, voiceT0)
	aw.voicePass(voiceT0)
	got := voiceMsgs(aw.asker, protocol.OpcodeVoicePeers)
	if rows := peersOf(t, got[len(got)-1]); len(rows) != 1 || rows[0].channels != voiceChanGroup|voiceChanGuild {
		t.Fatalf("the asker's peers %+v, want the leader once, of both channels", rows)
	}
	drainVoice(aw.asker)
	aw.b.GuildID = 10
	aw.voicePass(voiceT0.Add(time.Second))
	got = voiceMsgs(aw.asker, protocol.OpcodeVoicePeers)
	if len(got) != 1 {
		t.Fatalf("out of the guild: %d sets, want one", len(got))
	}
	if rows := peersOf(t, got[0]); len(rows) != 1 || rows[0].channels != voiceChanGroup {
		t.Fatalf("out of the guild: %+v, want the leader of the group alone", rows)
	}
	if aw.b.voiceGuildIn != 10 {
		t.Fatalf("the leader is in guild %d's channel, want its new guild's", aw.b.voiceGuildIn)
	}
}

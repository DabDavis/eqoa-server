package entity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/voice"
)

// Voice's game half (voice.go) on the assist world (assist_test.go): the
// asker and the leader, grouped, and a third player of no group, against a
// real forwarding server (no media here: internal/voice's tests have that)
// whose signals are recorded.

// recSFU is the forwarding server with its signals and leaves recorded.
type recSFU struct {
	*voice.Server
	mu     sync.Mutex
	sigs   []recSig
	leaves []recLeave
}

type recSig struct {
	id   uint32
	kind byte
	text string
}

type recLeave struct {
	id  uint32
	bye bool
}

func (r *recSFU) Signal(id uint32, kind byte, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sigs = append(r.sigs, recSig{id, kind, text})
}

func (r *recSFU) Leave(id uint32, bye bool) {
	r.mu.Lock()
	r.leaves = append(r.leaves, recLeave{id, bye})
	r.mu.Unlock()
	r.Server.Leave(id, bye)
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
	s, rest, ok := readVoiceString(b)
	if !ok {
		t.Fatalf("no string in %x", b)
	}
	return s, rest
}

type rosterRow struct {
	id       uint32
	name     string
	channels byte
}

// rosterOf decodes a 0x7E12 body.
func rosterOf(t *testing.T, b []byte) []rosterRow {
	t.Helper()
	n := int(b[0])
	b = b[1:]
	out := []rosterRow{}
	for i := 0; i < n; i++ {
		id := binary.LittleEndian.Uint32(b)
		name, rest := voiceStr(t, b[4:])
		out = append(out, rosterRow{id, name, rest[0]})
		b = rest[1:]
	}
	if len(b) != 0 {
		t.Fatalf("%d bytes past the roster", len(b))
	}
	return out
}

// lastRoster is the last roster queued for s (nil: none).
func lastRoster(t *testing.T, s *session.Session) []rosterRow {
	t.Helper()
	got := voiceMsgs(s, protocol.OpcodeVoiceRoster)
	if len(got) == 0 {
		return nil
	}
	return rosterOf(t, got[len(got)-1])
}

type iceRow struct{ url, user, cred string }

// configOf decodes a 0x7E11 body.
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

// newVoiceWorld is the assist world, grouped, with a recorded forwarding
// server and a third player of no group on its own session.
func newVoiceWorld(t *testing.T, turn bool) (*assistWorld, *recSFU, *session.Session, *Player) {
	t.Helper()
	aw := newAssistWorld(t)
	aw.group()
	srv, err := voice.New(voice.Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	rec := &recSFU{Server: srv}
	v := VoiceSettings{SFU: rec}
	if turn {
		v.TurnURLs = []string{"turn:203.0.113.5:3481?transport=udp", "turn:203.0.113.5:3481?transport=tcp"}
		v.Secret, v.TTL = "a-test-secret-only", 2*time.Hour
	}
	aw.w.SetVoice(v)
	c := &Player{ServerID: 1003, Name: "Loner", Level: 20}
	s := aw.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7010}, 10, 1, 1, 2, true)
	aw.w.Players[c.ServerID] = c
	s.SetECSEntity(c)
	s.SetCharacterInWorld(true)
	return aw, rec, s, c
}

func (aw *assistWorld) voicePass(now time.Time) {
	aw.w.mu.Lock()
	aw.w.TickCount += 5
	aw.w.voiceSyncSystem(now)
	aw.w.mu.Unlock()
}

// Voice on: in the forwarding server under a voice id, its ICE servers sent
// at once: none without a TURN server (and 0 seconds), with one its URLs,
// the username the TTL from now and the credential coturn's formula's,
// renewed half an hour before a 2-hour TTL runs out and not sooner.
func TestVoiceOnSendsTheICEServers(t *testing.T) {
	aw, _, _, _ := newVoiceWorld(t, false)
	if !aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn, voiceT0) || !aw.a.VoiceOn || aw.a.VoiceID == 0 {
		t.Fatalf("voice on: %v %08x", aw.a.VoiceOn, aw.a.VoiceID)
	}
	cfg := voiceMsgs(aw.asker, protocol.OpcodeVoiceConfig)
	if len(cfg) != 1 {
		t.Fatalf("%d configs", len(cfg))
	}
	if ice, secs := configOf(t, cfg[0]); len(ice) != 0 || secs != 0 {
		t.Fatalf("no TURN: %+v, %d s", ice, secs)
	}

	aw2, _, _, _ := newVoiceWorld(t, true)
	aw2.w.SetVoiceFlags(aw2.asker, VoiceFlagOn, voiceT0)
	ice, secs := configOf(t, voiceMsgs(aw2.asker, protocol.OpcodeVoiceConfig)[0])
	want := fmt.Sprintf("%d:v%08x", voiceT0.Add(2*time.Hour).Unix(), aw2.a.VoiceID)
	if len(ice) != 2 || secs != 7200 || ice[0].url != "turn:203.0.113.5:3481?transport=udp" || ice[1].user != want ||
		ice[0].user != want || ice[0].cred != voice.Credential("a-test-secret-only", want) {
		t.Fatalf("TURN: %+v, %d s (want %q)", ice, secs, want)
	}
	drainVoice(aw2.asker)
	aw2.voicePass(voiceT0.Add(89 * time.Minute))
	if got := voiceMsgs(aw2.asker, protocol.OpcodeVoiceConfig); len(got) != 0 {
		t.Fatal("renewed 89 minutes into 2 hours")
	}
	aw2.voicePass(voiceT0.Add(91 * time.Minute))
	if got := voiceMsgs(aw2.asker, protocol.OpcodeVoiceConfig); len(got) != 1 {
		t.Fatal("not renewed half an hour before they run out")
	}
}

// A roster: the groupmate with voice on (bit 0), not a voice-off one (it
// appears when its voice comes on) nor a player of no group; out of the
// group, at the next pass, gone; voice off, the player's own roster emptied.
func TestVoiceRostersFollowTheGroup(t *testing.T) {
	aw, _, loner, c := newVoiceWorld(t, false)
	aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn, voiceT0)
	aw.w.SetVoiceFlags(loner, VoiceFlagOn, voiceT0)
	aw.voicePass(voiceT0)
	if r := lastRoster(t, aw.asker); len(r) != 0 {
		t.Fatalf("the leader's voice is off: %+v", r)
	}
	aw.w.SetVoiceFlags(aw.leader, VoiceFlagOn, voiceT0)
	aw.voicePass(voiceT0)
	if r := lastRoster(t, aw.asker); len(r) != 1 || r[0] != (rosterRow{aw.b.VoiceID, "Leader", 1}) {
		t.Fatalf("the asker's roster %+v", r)
	}
	if r := lastRoster(t, aw.leader); len(r) != 1 || r[0].id != aw.a.VoiceID {
		t.Fatalf("the leader's roster %+v", r)
	}
	if r := lastRoster(t, loner); len(r) != 0 {
		t.Fatalf("a player of no group: %+v", r)
	}
	c.GroupID = 8 // another group: no one the asker's
	aw.voicePass(voiceT0)
	if r := lastRoster(t, aw.asker); len(r) != 1 || r[0].id != aw.b.VoiceID {
		t.Fatalf("a player of another group in the asker's roster: %+v", r)
	}
	c.GroupID = 0
	aw.voicePass(voiceT0)
	drainVoice(aw.asker, aw.leader)
	aw.b.GroupID = 0
	aw.voicePass(voiceT0)
	if r := lastRoster(t, aw.asker); r == nil || len(r) != 0 {
		t.Fatalf("the leader out of the group: %+v", r)
	}
	aw.b.GroupID = 7
	aw.voicePass(voiceT0)
	drainVoice(aw.asker, aw.leader)
	aw.w.SetVoiceFlags(aw.leader, 0, voiceT0)
	if r := lastRoster(t, aw.leader); r == nil || len(r) != 0 {
		t.Fatalf("voice off: its own roster %+v", r)
	}
	if r := lastRoster(t, aw.asker); r == nil || len(r) != 0 {
		t.Fatalf("the leader's voice off: the asker's roster %+v", r)
	}
	if aw.b.VoiceOn || aw.b.VoiceID != 0 {
		t.Fatal("voice off kept its id")
	}
}

// signalPart builds a client's 0x7E13 (after the opcode).
func signalPart(seq uint32, kind byte, part, parts uint16, chunk string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, seq)
	b = append(b, kind)
	b = binary.LittleEndian.AppendUint16(b, part)
	b = binary.LittleEndian.AppendUint16(b, parts)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(chunk)))
	return append(b, chunk...)
}

// A client's offer in three parts reaches the forwarding server whole, once;
// dropped are a part from a voice-off player, an answer (a client never
// sends one), a part past its count, a chunk of 901 bytes, a length that does
// not add up, and the 51st message in a second.
func TestVoiceSignalsReachTheForwardingServer(t *testing.T) {
	aw, rec, _, _ := newVoiceWorld(t, false)
	aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn, voiceT0)
	offer := strings.Repeat("v=0 o=- 46117 2 IN IP4 127.0.0.1 ", 80)
	parts := voice.Split(offer)
	for i, p := range parts {
		if why := aw.w.VoiceSignalIn(aw.asker, signalPart(9, voice.KindOffer, uint16(i), uint16(len(parts)), p), voiceT0); why != "" {
			t.Fatalf("part %d dropped: %s", i, why)
		}
	}
	if len(rec.sigs) != 1 || rec.sigs[0] != (recSig{aw.a.VoiceID, voice.KindOffer, offer}) {
		t.Fatalf("the forwarding server got %d signals", len(rec.sigs))
	}
	for name, c := range map[string]struct {
		s *session.Session
		b []byte
	}{
		"from a voice-off player": {aw.leader, signalPart(1, voice.KindOffer, 0, 1, "x")},
		"an answer":               {aw.asker, signalPart(10, voice.KindAnswer, 0, 1, "x")},
		"a part past its count":   {aw.asker, signalPart(11, voice.KindCandidate, 1, 1, "x")},
		"901 bytes":               {aw.asker, signalPart(12, voice.KindOffer, 0, 1, strings.Repeat("y", 901))},
		"a length short":          {aw.asker, signalPart(13, voice.KindCandidate, 0, 1, "xy")[:12]},
	} {
		if why := aw.w.VoiceSignalIn(c.s, c.b, voiceT0); why == "" {
			t.Errorf("%s: taken", name)
		}
	}
	if len(rec.sigs) != 1 {
		t.Fatalf("dropped parts reached the forwarding server: %d", len(rec.sigs))
	}
	later := voiceT0.Add(time.Minute)
	n := 0
	for i := 0; i < voice.RateMsgs+1; i++ {
		if aw.w.VoiceSignalIn(aw.asker, signalPart(uint32(100+i), voice.KindCandidate, 0, 1, "{}"), later) == "" {
			n++
		}
	}
	if n != voice.RateMsgs || len(rec.sigs) != 1+voice.RateMsgs {
		t.Fatalf("%d of %d taken in a second (%d signals)", n, voice.RateMsgs+1, len(rec.sigs))
	}
}

// The forwarding server's answer goes down in parts of at most 900 bytes,
// one seq, put back whole; the next message is the next seq.
func TestVoiceSinkSendsSignalsInParts(t *testing.T) {
	aw, _, _, _ := newVoiceWorld(t, false)
	aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn, voiceT0)
	drainVoice(aw.asker)
	answer := strings.Repeat("a=candidate:1 1 udp 2130706431 127.0.0.1 3480 typ host\r\n", 40)
	aw.a.voiceSink.Signal(voice.KindAnswer, answer)
	aw.a.voiceSink.Signal(voice.KindBye, "")
	msgs := voiceMsgs(aw.asker, protocol.OpcodeVoiceSignal)
	if len(msgs) < 4 {
		t.Fatalf("%d messages", len(msgs))
	}
	var whole bytes.Buffer
	var seq uint32
	for i, m := range msgs[:len(msgs)-1] {
		s, k := binary.LittleEndian.Uint32(m), m[4]
		part, parts := binary.LittleEndian.Uint16(m[5:]), binary.LittleEndian.Uint16(m[7:])
		chunk, rest := voiceStr(t, m[9:])
		if k != voice.KindAnswer || int(part) != i || int(parts) != len(msgs)-1 || len(chunk) > voice.MaxChunk || len(rest) != 0 {
			t.Fatalf("part %d: kind %d, %d of %d, %d bytes", i, k, part, parts, len(chunk))
		}
		if i > 0 && s != seq {
			t.Fatalf("part %d of another seq", i)
		}
		seq = s
		whole.WriteString(chunk)
	}
	if whole.String() != answer {
		t.Fatal("not put back whole")
	}
	bye := msgs[len(msgs)-1]
	if binary.LittleEndian.Uint32(bye) != seq+1 || bye[4] != voice.KindBye {
		t.Fatalf("the bye: seq %d kind %d", binary.LittleEndian.Uint32(bye), bye[4])
	}
}

// roomOp builds a client's 0x7E14 (after the opcode).
func roomOp(op byte, name, hash string, flags ...byte) []byte {
	b := []byte{op}
	if op == voiceRoomJoin {
		b = appendVoiceString(b, name, 1000)
		b = appendVoiceString(b, hash, 1000)
		b = append(b, flags...)
	}
	return b
}

type lobbyRow struct {
	name    string
	members int
	locked  bool
}

func lobbyOf(t *testing.T, b []byte) []lobbyRow {
	t.Helper()
	n := int(binary.LittleEndian.Uint16(b))
	b = b[2:]
	var out []lobbyRow
	for i := 0; i < n; i++ {
		name, rest := voiceStr(t, b)
		out = append(out, lobbyRow{name, int(binary.LittleEndian.Uint16(rest)), rest[2] == 1})
		b = rest[3:]
	}
	return out
}

// roomState is the last 0x7E16 queued for s.
func roomState(t *testing.T, s *session.Session) (byte, string) {
	t.Helper()
	got := voiceMsgs(s, protocol.OpcodeVoiceRoomState)
	if len(got) == 0 {
		t.Fatal("no room state")
	}
	name, _ := voiceStr(t, got[len(got)-1][1:])
	return got[len(got)-1][0], name
}

func lastLobby(t *testing.T, s *session.Session) []lobbyRow {
	t.Helper()
	got := voiceMsgs(s, protocol.OpcodeVoiceLobby)
	if len(got) == 0 {
		t.Fatal("no lobby")
	}
	return lobbyOf(t, got[len(got)-1])
}

// The lobby over the game's opcodes: a join answered with the room's state
// and the lobby; a second player in by the name; the roster's bit 1; a
// locked room refusing a wrong H; an unlisted room in no list; a leave; a
// list with voice off answered, a join with voice off dropped.
func TestVoiceLobbyOverTheGame(t *testing.T) {
	aw, _, loner, c := newVoiceWorld(t, false)
	for _, s := range []*session.Session{aw.asker, aw.leader, loner} {
		aw.w.SetVoiceFlags(s, VoiceFlagOn, voiceT0)
	}
	if why := aw.w.VoiceRoomOp(loner, roomOp(voiceRoomJoin, "Tavern", "", 0), voiceT0); why != "" {
		t.Fatal(why)
	}
	if r, n := roomState(t, loner); r != voice.RoomIn || n != "Tavern" {
		t.Fatalf("making it: %d %q", r, n)
	}
	if lb := lastLobby(t, loner); len(lb) != 1 || lb[0] != (lobbyRow{"Tavern", 1, false}) {
		t.Fatalf("the lobby %+v", lb)
	}
	aw.w.VoiceRoomOp(aw.asker, roomOp(voiceRoomJoin, "tavern", ""), voiceT0) // no flags byte: none
	if r, _ := roomState(t, aw.asker); r != voice.RoomIn {
		t.Fatalf("joining it: %d", r)
	}
	if r := lastRoster(t, aw.asker); len(r) != 2 || r[1] != (rosterRow{c.VoiceID, "Loner", 2}) {
		t.Fatalf("the asker's roster %+v", r)
	}
	h := strings.Repeat("ab", 32)
	aw.w.VoiceRoomOp(aw.leader, roomOp(voiceRoomJoin, "Vault", h, 1), voiceT0)
	aw.w.VoiceRoomOp(loner, roomOp(voiceRoomJoin, "Vault", strings.Repeat("cd", 32), 0), voiceT0)
	if r, _ := roomState(t, loner); r != voice.RoomWrongPassword {
		t.Fatalf("a wrong password: %d", r)
	}
	aw.w.VoiceRoomOp(aw.asker, roomOp(voiceRoomList, "", ""), voiceT0)
	if lb := lastLobby(t, aw.asker); len(lb) != 1 || lb[0].name != "Tavern" || lb[0].members != 2 {
		t.Fatalf("an unlisted room listed, or a refused join counted: %+v", lb)
	}
	aw.w.VoiceRoomOp(aw.asker, roomOp(voiceRoomLeave, "", ""), voiceT0)
	if r, n := roomState(t, aw.asker); r != voice.RoomLeft || n != "Tavern" {
		t.Fatalf("leaving: %d %q", r, n)
	}
	aw.w.SetVoiceFlags(aw.asker, 0, voiceT0)
	drainVoice(aw.asker)
	if why := aw.w.VoiceRoomOp(aw.asker, roomOp(voiceRoomList, "", ""), voiceT0); why != "" || len(lastLobby(t, aw.asker)) != 1 {
		t.Fatalf("a list with voice off: %q", why)
	}
	if why := aw.w.VoiceRoomOp(aw.asker, roomOp(voiceRoomJoin, "Tavern", ""), voiceT0); why == "" {
		t.Fatal("a join with voice off was taken")
	}
	if why := aw.w.VoiceRoomOp(aw.asker, []byte{9}, voiceT0); why == "" {
		t.Fatal("op 9 was taken")
	}
}

// A voice-on player leaving the world leaves the voice at once, its
// connection ended with a bye; its groupmate's roster drops it.
func TestVoiceLeavesWithThePlayer(t *testing.T) {
	aw, rec, _, _ := newVoiceWorld(t, false)
	aw.w.SetVoiceFlags(aw.asker, VoiceFlagOn, voiceT0)
	aw.w.SetVoiceFlags(aw.leader, VoiceFlagOn, voiceT0)
	id := aw.b.VoiceID
	drainVoice(aw.asker)
	aw.w.RemovePlayer(aw.b.ServerID)
	if len(rec.leaves) != 1 || rec.leaves[0] != (recLeave{id, true}) {
		t.Fatalf("leaves %+v", rec.leaves)
	}
	if r := lastRoster(t, aw.asker); r == nil || len(r) != 0 {
		t.Fatalf("the asker's roster %+v", r)
	}
	aw.voicePass(voiceT0)
	if len(rec.leaves) != 1 {
		t.Fatalf("left twice: %+v", rec.leaves)
	}
}

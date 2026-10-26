package entity

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/voice"
)

// Voice chat's game half: the forwarding server (internal/voice) carries the
// media and keeps the lobby; the world tells it who is in (a player whose
// client turned voice on, 0x7E10), in which group, and carries its messages
// over the game connection: the client's signaling (0x7E13) and lobby ops
// (0x7E14) to it, and its answers, candidates and byes (0x7E13), the
// player's roster (0x7E12), the lobby (0x7E15) and room states (0x7E16)
// back. A client also gets its ICE servers (0x7E11): this server's TURN,
// when it has one, with credentials minted here, again before they run out.
// A PS2 never turns voice on, and is sent nothing but the offer.

// VoiceSFU is the forwarding server as the world drives it (voice.Server).
type VoiceSFU interface {
	Join(name, group string, sink voice.Sink) uint32
	SetGroup(id uint32, group string)
	Leave(id uint32, bye bool)
	Signal(id uint32, kind byte, text string)
	JoinRoom(id uint32, name, hash string, unlisted bool) (byte, string)
	LeaveRoom(id uint32) (string, bool)
	Lobby() []voice.Room
}

// VoiceSettings is the world's voice: its forwarding server, and its TURN
// server's URLs and secret (none: no TURN), its credentials lasting TTL.
type VoiceSettings struct {
	SFU      VoiceSFU
	TurnURLs []string
	Secret   string
	TTL      time.Duration
}

const (
	// DefaultVoiceTTL is how long TURN credentials last unless configured.
	DefaultVoiceTTL = 24 * time.Hour

	VoiceFlagOn = 1 // 0x7E10's bit 0

	voiceRoomJoin  = 1
	voiceRoomLeave = 2
	voiceRoomList  = 3

	// a dropped message is logged once every 10 s a session
	voiceDropLogEvery = 10 * time.Second
	// a name in a roster, and a room's in the lobby, at most (bytes)
	voiceMaxName = 64
	// a roster's or the lobby's message, at most (bytes): DRDP fragments it
	voiceMaxBody = 8 * 1024
	// a signal part's header: seq, kind, part, parts, the chunk's length
	voiceSignalHead = 11
)

// SetVoice gives the world its voice (main.go, once).
func (w *World) SetVoice(v VoiceSettings) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if v.TTL <= 0 {
		v.TTL = DefaultVoiceTTL
	}
	w.voice = v
	w.voiceIn = map[uint32]*Player{}
}

// VoiceEnabled says whether this server has voice.
func (w *World) VoiceEnabled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.voice.SFU != nil
}

// voiceGroupOf is p's group's channel ("" in no group).
func voiceGroupOf(p *Player) string {
	if p.GroupID == 0 {
		return ""
	}
	return fmt.Sprintf("g:%d", p.GroupID)
}

// SetVoiceFlags makes sess's player's voice what its client's 0x7E10 says
// (bit 0 on; the old bit 1, a guild's channel, means nothing now). On, it
// joins the forwarding server and is sent its ICE servers; its roster follows
// from there. Off, it leaves (its roster emptied). False when sess has no
// player.
func (w *World) SetVoiceFlags(sess *session.Session, flags uint32, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	if w.voice.SFU == nil {
		return true
	}
	on := flags&VoiceFlagOn != 0
	switch {
	case on && !p.VoiceOn:
		k := &voiceSink{sess: sess}
		id := w.voice.SFU.Join(p.Name, voiceGroupOf(p), k)
		if id == 0 {
			return true // the server is going down
		}
		p.VoiceOn, p.VoiceID, p.voiceGroup, p.voiceSink = true, id, voiceGroupOf(p), k
		p.voiceAsm = voice.Assembler{}
		w.voiceIn[id] = p
		w.sendVoiceConfigLocked(sess, p, now)
	case !on && p.VoiceOn:
		w.voiceLeaveLocked(p, false)
	}
	return true
}

// voiceLeaveLocked takes p out of the voice: with bye its client is told the
// server ended its connection (it offers again when it may).
func (w *World) voiceLeaveLocked(p *Player, bye bool) {
	if !p.VoiceOn {
		return
	}
	w.voice.SFU.Leave(p.VoiceID, bye)
	delete(w.voiceIn, p.VoiceID)
	p.VoiceOn, p.VoiceID, p.voiceGroup, p.voiceSink = false, 0, "", nil
	p.voiceCredsUntil = time.Time{}
	p.voiceAsm = voice.Assembler{}
}

// voiceGoneLocked takes a player leaving the world out of the voice
// (leavingLocked): a logout, or a login from elsewhere ending this one.
func (w *World) voiceGoneLocked(p *Player) {
	if w.voice.SFU != nil {
		w.voiceLeaveLocked(p, true)
	}
}

// voiceSyncSystem keeps the forwarding server's groups the world's (a group
// joined or left: the rosters follow), takes out a voice-on player who is
// no longer in the world, and sends each client its credentials again before
// they run out. Every 5 ticks. Must be called with the lock held.
func (w *World) voiceSyncSystem(now time.Time) {
	if w.TickCount%5 != 0 || w.voice.SFU == nil {
		return
	}
	for id, p := range w.voiceIn {
		if w.Players[p.ServerID] != p || !p.VoiceOn || p.VoiceID != id {
			w.voice.SFU.Leave(id, true)
			delete(w.voiceIn, id)
			continue
		}
		if g := voiceGroupOf(p); g != p.voiceGroup {
			p.voiceGroup = g
			w.voice.SFU.SetGroup(id, g)
		}
		if !p.voiceCredsUntil.IsZero() && now.After(p.voiceCredsUntil.Add(-voiceRefresh(w.voice.TTL))) {
			if s := w.findSessionByEntityIDLocked(p.ServerID); s != nil {
				w.sendVoiceConfigLocked(s, p, now)
			}
		}
	}
}

// voiceRefresh is how long before they run out credentials are sent again:
// a quarter of their life, at most an hour.
func voiceRefresh(ttl time.Duration) time.Duration {
	return min(time.Hour, ttl/4)
}

// sendVoiceConfigLocked sends p's client its ICE servers: this server's TURN
// URLs, each with the credentials minted now for p's voice id, and the
// seconds they last; none (and 0) without a TURN server.
func (w *World) sendVoiceConfigLocked(sess *session.Session, p *Player, now time.Time) {
	v := w.voice
	if len(v.TurnURLs) == 0 || v.Secret == "" {
		p.voiceCredsUntil = time.Time{}
		queueVoiceBody(sess, protocol.OpcodeVoiceConfig, voiceConfigBody(nil, "", "", 0))
		return
	}
	p.voiceCredsUntil = now.Add(v.TTL)
	user := voice.Username(p.voiceCredsUntil, p.VoiceID)
	queueVoiceBody(sess, protocol.OpcodeVoiceConfig,
		voiceConfigBody(v.TurnURLs, user, voice.Credential(v.Secret, user), uint32(v.TTL/time.Second)))
}

// voiceConfigBody is 0x7E11's body: a u8 count of ICE servers, each a url,
// a username and a credential, then a u32 of seconds.
func voiceConfigBody(urls []string, user, cred string, secs uint32) []byte {
	b := []byte{byte(len(urls))}
	for _, u := range urls {
		b = appendVoiceString(b, u, 512)
		b = appendVoiceString(b, user, 128)
		b = appendVoiceString(b, cred, 128)
	}
	return binary.LittleEndian.AppendUint32(b, secs)
}

// appendVoiceString appends s as a u16 byte length and its UTF-8, cut to
// at most max bytes on a whole character.
func appendVoiceString(b []byte, s string, max int) []byte {
	if len(s) > max {
		n := max
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = s[:n]
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

// queueVoiceBody sends a voice message whole, however long (the queue cuts
// one past a datagram into fragments).
func queueVoiceBody(sess *session.Session, op protocol.GameOpcode, body []byte) {
	if sess == nil {
		return
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(append(binary.LittleEndian.AppendUint16(nil, uint16(op)), body...))
	sess.Queue().AddWithSequence(msg)
}

// voiceSink is how the forwarding server reaches one session's client: its
// signals in parts (0x7E13, its own message numbers) and its roster
// (0x7E12). It only queues, so it never blocks.
type voiceSink struct {
	sess *session.Session
	seq  atomic.Uint32
}

func (k *voiceSink) Signal(kind byte, text string) {
	seq := k.seq.Add(1)
	parts := voice.Split(text)
	for i, c := range parts {
		sendGroupMsg(k.sess, protocol.OpcodeVoiceSignal, func(b *buffer.Writer) {
			b.WriteUint32(seq)
			b.WriteByte(kind)
			b.WriteUint16(uint16(i))
			b.WriteUint16(uint16(len(parts)))
			b.WriteUint16(uint16(len(c)))
			b.WriteBytes([]byte(c))
		})
	}
}

func (k *voiceSink) Roster(ms []voice.Member) {
	queueVoiceBody(k.sess, protocol.OpcodeVoiceRoster, encodeVoiceRoster(ms))
}

// encodeVoiceRoster is 0x7E12's body: a u8 count, then each member's u32
// voice id, name and u8 of channels; as many as fit voiceMaxBody and 255.
func encodeVoiceRoster(ms []voice.Member) []byte {
	b := []byte{0}
	n := 0
	for _, m := range ms {
		e := binary.LittleEndian.AppendUint32(nil, m.ID)
		e = appendVoiceString(e, m.Name, voiceMaxName)
		e = append(e, m.Channels)
		if n == 255 || len(b)+len(e) > voiceMaxBody {
			break
		}
		b = append(b, e...)
		n++
	}
	b[0] = byte(n)
	return b
}

// encodeVoiceLobby is 0x7E15's body: a u16 count, then each room's name, u16
// members and u8 locked; as many as fit voiceMaxBody.
func encodeVoiceLobby(rooms []voice.Room) []byte {
	b := []byte{0, 0}
	n := 0
	for _, r := range rooms {
		e := appendVoiceString(nil, r.Name, voiceMaxName)
		e = binary.LittleEndian.AppendUint16(e, uint16(min(r.Members, 65535)))
		if r.Locked {
			e = append(e, 1)
		} else {
			e = append(e, 0)
		}
		if len(b)+len(e) > voiceMaxBody {
			break
		}
		b = append(b, e...)
		n++
	}
	binary.LittleEndian.PutUint16(b, uint16(n))
	return b
}

// voiceDropLocked logs a dropped voice message for p, at most once every
// 10 s, and gives back why.
func (w *World) voiceDropLocked(p *Player, sess *session.Session, why string, now time.Time) string {
	if p != nil {
		if now.Sub(p.voiceDropLogged) < voiceDropLogEvery {
			return why
		}
		p.voiceDropLogged = now
	}
	log.Printf("Voice: %s's message dropped: %s", sess.RemoteAddr(), why)
	return why
}

// VoiceSignalIn takes a client's 0x7E13 (data: after the opcode) and hands
// its message, once every part is in, to the forwarding server: only from a
// voice-on player, well-formed and within its allowance. It says why a part
// was dropped ("" when it was not).
func (w *World) VoiceSignalIn(sess *session.Session, data []byte, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !p.VoiceOn || w.voice.SFU == nil {
		return w.voiceDropLocked(p, sess, "voice is off", now)
	}
	if len(data) < voiceSignalHead {
		return w.voiceDropLocked(p, sess, "short", now)
	}
	seq := binary.LittleEndian.Uint32(data)
	kind := data[4]
	part := int(binary.LittleEndian.Uint16(data[5:]))
	parts := int(binary.LittleEndian.Uint16(data[7:]))
	n := int(binary.LittleEndian.Uint16(data[9:]))
	if len(data) != voiceSignalHead+n {
		return w.voiceDropLocked(p, sess, "its length does not add up", now)
	}
	chunk := string(data[voiceSignalHead:])
	if err := voice.CheckPart(kind, part, parts, chunk); err != nil {
		return w.voiceDropLocked(p, sess, err.Error(), now)
	}
	if !p.voiceRate.Take(len(data), now) {
		return w.voiceDropLocked(p, sess, "past its allowance", now)
	}
	if text, done := p.voiceAsm.Add(seq, kind, part, parts, chunk, now); done {
		w.voice.SFU.Signal(p.VoiceID, kind, text)
	}
	return ""
}

// VoiceRoomOp takes a client's 0x7E14: join a room (its name, its lock's H
// or empty, flags: bit 0 unlisted), leave it, or list the lobby; a join or a
// leave is answered with 0x7E16 and then 0x7E15, a list with 0x7E15. A join
// or a leave needs voice on; a list does not. It says why it was dropped.
func (w *World) VoiceRoomOp(sess *session.Session, data []byte, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.voice.SFU == nil {
		return w.voiceDropLocked(p, sess, "no voice", now)
	}
	if len(data) < 1 {
		return w.voiceDropLocked(p, sess, "short", now)
	}
	if !p.voiceRate.Take(len(data), now) {
		return w.voiceDropLocked(p, sess, "past its allowance", now)
	}
	switch data[0] {
	case voiceRoomJoin:
		name, rest, ok := readVoiceString(data[1:])
		if !ok {
			return w.voiceDropLocked(p, sess, "a join without a name", now)
		}
		hash, rest, ok := readVoiceString(rest)
		if !ok {
			return w.voiceDropLocked(p, sess, "a join without its H", now)
		}
		unlisted := len(rest) > 0 && rest[0]&1 != 0
		if !p.VoiceOn {
			return w.voiceDropLocked(p, sess, "a join with voice off", now)
		}
		res, rname := w.voice.SFU.JoinRoom(p.VoiceID, name, hash, unlisted)
		queueVoiceBody(sess, protocol.OpcodeVoiceRoomState, appendVoiceString([]byte{res}, rname, voiceMaxName))
	case voiceRoomLeave:
		if !p.VoiceOn {
			return w.voiceDropLocked(p, sess, "a leave with voice off", now)
		}
		rname, _ := w.voice.SFU.LeaveRoom(p.VoiceID)
		queueVoiceBody(sess, protocol.OpcodeVoiceRoomState, appendVoiceString([]byte{voice.RoomLeft}, rname, voiceMaxName))
	case voiceRoomList:
	default:
		return w.voiceDropLocked(p, sess, fmt.Sprintf("op %d", data[0]), now)
	}
	queueVoiceBody(sess, protocol.OpcodeVoiceLobby, encodeVoiceLobby(w.voice.SFU.Lobby()))
	return ""
}

// readVoiceString reads a u16 length and that many bytes of UTF-8 off b.
func readVoiceString(b []byte) (string, []byte, bool) {
	if len(b) < 2 {
		return "", nil, false
	}
	n := int(binary.LittleEndian.Uint16(b))
	if len(b) < 2+n || !utf8.Valid(b[2:2+n]) {
		return "", nil, false
	}
	return string(b[2 : 2+n]), b[2+n:], true
}

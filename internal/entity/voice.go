package entity

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Group voice chat (protocol.OpcodeVoiceOffer .. OpcodeVoiceSignal,
// handlers/voice.go; F3 pass 18, the user's choice: a group's voice, push to
// talk, off until a player turns it on). The clients speak WebRTC to each
// other, every packet through the TURN server (coturn, use-auth-secret; the
// clients use relay candidates alone), so no player learns another's address.
// The server's part: each voice-on session gets its own TURN credentials
// (coturn's REST form, TurnCredential), told again before they run out; the
// set of its peers (each with the voice id the signaling names it by, which
// of the two makes the offer and the channels they share), sent whenever it
// changes; and the signaling itself, relayed only between two voice-on
// players who share a channel, rate-limited and size-capped. The channels: a
// player's group (its other members with voice on), and its guild's voice
// channel (the user's "also add guild voice chat"), which a player asks to be
// in (0x7E06 bit 1) and which holds the first voiceGuildCap to get in, by the
// server's clock: one more waits, told so once, and gets in as soon as a
// place frees. Groups and guilds may span worlds: a member in another world
// is a peer all the same, as their chat reaches it.

// VoiceSettings is the server's voice: the TURN URLs clients are given, the
// TURN server's shared secret and how long credentials last.
type VoiceSettings struct {
	TurnURLs []string
	Secret   string
	TTL      time.Duration
}

// usable is whether voice can be offered: a URL and the secret.
func (v VoiceSettings) usable() bool { return len(v.TurnURLs) > 0 && v.Secret != "" }

const (
	// DefaultVoiceTTL is how long credentials last when the config says not.
	DefaultVoiceTTL = 24 * time.Hour

	voiceKindOffer     = 1
	voiceKindAnswer    = 2
	voiceKindCandidate = 3
	voiceKindBye       = 4

	// a signal's chunk and how many make one message (clients chunk an SDP
	// of a few KB into parts of at most voiceMaxChunk bytes)
	voiceMaxChunk   = 900
	voiceMaxParts   = 64
	voiceSignalHead = 4 + 4 + 1 + 2 + 2 + 2

	// what one session may signal: a second's worth of each, refilled as
	// it goes (an offer, its answer and a dozen candidates a peer is a few
	// messages and a few KB)
	voiceMsgsPerSec  = 50
	voiceBytesPerSec = 64 * 1024

	// a dropped signal is logged at most this often a session
	voiceDropLogEvery = 10 * time.Second

	// a name in OpcodeVoicePeers, at most
	voiceMaxName = 64

	// voiceGuildCap is how many a guild's voice channel holds (the user's
	// choice); the words a player is told about it (ours)
	voiceGuildCap  = 10
	voiceGuildIn   = "You are in your guild's voice channel."
	voiceGuildFull = "Your guild's voice channel is full (10)."

	// 0x7E06's flags and 0x7E08's channels
	VoiceFlagOn    = 1
	VoiceFlagGuild = 2
	voiceChanGroup = 1
	voiceChanGuild = 2
)

// voiceRate is a session's signaling allowance: messages and bytes, each
// refilled at its rate a second up to a second's worth.
type voiceRate struct {
	msgs, bytes float64
	at          time.Time
}

// take spends one message of n bytes, refilled since the last; false (and
// nothing spent) when the session has signaled too much.
func (r *voiceRate) take(n int, now time.Time) bool {
	if r.at.IsZero() {
		r.msgs, r.bytes = voiceMsgsPerSec, voiceBytesPerSec
	} else if d := now.Sub(r.at).Seconds(); d > 0 {
		r.msgs = min(voiceMsgsPerSec, r.msgs+d*voiceMsgsPerSec)
		r.bytes = min(voiceBytesPerSec, r.bytes+d*voiceBytesPerSec)
	}
	r.at = now
	if r.msgs < 1 || r.bytes < float64(n) {
		return false
	}
	r.msgs--
	r.bytes -= float64(n)
	return true
}

// SetVoice sets the server's voice (main.go, from config.yaml's voice): with
// no URL or no secret, voice is off.
func (w *World) SetVoice(v VoiceSettings) {
	if v.TTL <= 0 {
		v.TTL = DefaultVoiceTTL
	}
	w.mu.Lock()
	w.voice = v
	w.mu.Unlock()
}

// VoiceEnabled is whether this server offers voice.
func (w *World) VoiceEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.voice.usable()
}

// TurnCredential is coturn's REST credential for a username under the TURN
// server's static-auth-secret: base64(HMAC-SHA1(secret, username)), the
// username "<unix time it expires>:<anything>".
func TurnCredential(secret, username string) string {
	m := hmac.New(sha1.New, []byte(secret))
	m.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// SetVoiceOn is SetVoiceFlags with the voice on (and no guild channel) or
// off.
func (w *World) SetVoiceOn(sess *session.Session, on bool, now time.Time) bool {
	var flags uint32
	if on {
		flags = VoiceFlagOn
	}
	return w.SetVoiceFlags(sess, flags, now)
}

// SetVoiceFlags makes sess's player's voice what its client's 0x7E06 says:
// bit 0 on, bit 1 in its guild's channel (alone, nothing). On, it gets a voice
// id, its credentials and its peers at once, and into its guild's channel when
// it asked and a place is free (else told the channel is full); off, an empty
// set of peers, so its client hangs up, and its peers drop it at the next pass.
// False when sess has no player.
func (w *World) SetVoiceFlags(sess *session.Session, flags uint32, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	if !w.voice.usable() {
		return true
	}
	on := flags&VoiceFlagOn != 0
	guild := on && flags&VoiceFlagGuild != 0
	if guild && !p.VoiceGuild {
		p.voiceGuildAsked, p.voiceGuildToldFull = now, false
	}
	p.VoiceGuild = guild
	switch {
	case on && !p.VoiceOn:
		if p.VoiceID == 0 {
			p.VoiceID = w.newVoiceIDLocked()
		}
		p.VoiceOn = true
		w.sendVoiceConfigLocked(sess, p, now)
		p.voicePeersSent, p.voicePeersDue = "", true
	case !on && p.VoiceOn:
		p.VoiceOn = false
		p.voiceCredsUntil = time.Time{}
		p.voiceGuildIn = 0
		body := encodeVoicePeers(nil)
		p.voicePeersSent, p.voicePeersDue = "", false
		sendGroupMsg(sess, protocol.OpcodeVoicePeers, func(b *buffer.Writer) { b.WriteBytes(body) })
	}
	w.voiceGuildChannelsLocked(now)
	if p.VoiceOn {
		w.sendVoicePeersLocked(sess, p)
	}
	return true
}

// voiceGuildChannelsLocked keeps every guild's voice channel: a player in one
// who no longer may be (its voice or its wish off, out of the guild, gone)
// leaves it, and those waiting get in, the first to ask first, while it holds
// fewer than voiceGuildCap, each told so; one that cannot is told once that
// the channel is full. Must be called with the lock held.
func (w *World) voiceGuildChannelsLocked(now time.Time) {
	in := map[uint32]int{}
	waiting := map[uint32][]*Player{}
	for _, p := range w.Players {
		wants := p.VoiceOn && p.VoiceGuild && p.GuildID != 0
		if p.voiceGuildIn != 0 && (!wants || p.voiceGuildIn != p.GuildID) {
			p.voiceGuildIn = 0
		}
		switch {
		case p.voiceGuildIn != 0:
			in[p.voiceGuildIn]++
		case wants:
			waiting[p.GuildID] = append(waiting[p.GuildID], p)
		}
	}
	for g, ws := range waiting {
		sort.Slice(ws, func(i, j int) bool {
			if !ws[i].voiceGuildAsked.Equal(ws[j].voiceGuildAsked) {
				return ws[i].voiceGuildAsked.Before(ws[j].voiceGuildAsked)
			}
			return ws[i].VoiceID < ws[j].VoiceID
		})
		for _, p := range ws {
			s := w.findSessionByEntityIDLocked(p.ServerID)
			if in[g] < voiceGuildCap {
				in[g]++
				p.voiceGuildIn, p.voiceGuildSince, p.voiceGuildToldFull = g, now, false
				if s != nil {
					w.sendChatToSession(s, voiceGuildIn)
				}
				continue
			}
			if !p.voiceGuildToldFull && s != nil {
				w.sendChatToSession(s, voiceGuildFull)
			}
			p.voiceGuildToldFull = true
		}
	}
}

// newVoiceIDLocked is a voice id no player in the world has: random, so a
// player logged in again is a new peer to its group. Must be called with the
// lock held.
func (w *World) newVoiceIDLocked() uint32 {
	for {
		id := rand.Uint32()
		if id == 0 {
			continue
		}
		taken := false
		for _, o := range w.Players {
			if o.VoiceID == id {
				taken = true
				break
			}
		}
		if !taken {
			return id
		}
	}
}

// voicePeer is one of a player's voice peers.
type voicePeer struct {
	id       uint32
	name     string
	offerer  bool // this player makes the offer to it
	channels byte // voiceChanGroup, voiceChanGuild
	p        *Player
}

// voicePeersLocked is p's peers, each once: its group's other members whose
// voice is on, in the roster's order, then the others in its guild's voice
// channel while it is in it, in the order they got in; one in both is one
// peer of both channels. Must be called with the lock held.
func (w *World) voicePeersLocked(p *Player) []voicePeer {
	if !p.VoiceOn {
		return nil
	}
	var out []voicePeer
	add := func(m *Player, ch byte) {
		for i := range out {
			if out[i].p == m {
				out[i].channels |= ch
				return
			}
		}
		out = append(out, voicePeer{id: m.VoiceID, name: m.Name, offerer: p.VoiceID < m.VoiceID, channels: ch, p: m})
	}
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		for _, mid := range g.Members {
			if m := w.Players[mid]; m != nil && m != p && m.VoiceOn && m.VoiceID != 0 && m.GroupID == p.GroupID {
				add(m, voiceChanGroup)
			}
		}
	}
	if p.voiceGuildIn != 0 {
		var guild []*Player
		for _, m := range w.Players {
			if m != p && m.VoiceOn && m.VoiceID != 0 && m.voiceGuildIn == p.voiceGuildIn {
				guild = append(guild, m)
			}
		}
		sort.Slice(guild, func(i, j int) bool {
			if !guild[i].voiceGuildSince.Equal(guild[j].voiceGuildSince) {
				return guild[i].voiceGuildSince.Before(guild[j].voiceGuildSince)
			}
			return guild[i].VoiceID < guild[j].VoiceID
		})
		for _, m := range guild {
			add(m, voiceChanGuild)
		}
	}
	return out
}

// writeVoiceString is a voice message's string: a u16 byte length, then the
// UTF-8 bytes (at most max of them).
func writeVoiceString(b []byte, s string, max int) []byte {
	if len(s) > max {
		s = s[:max]
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

// encodeVoicePeers is OpcodeVoicePeers' body: a u8 count, then each peer's
// u32 voice id, its name, a u8, 1 where this client makes the offer, and a u8
// of the channels they share (bit 0 the group, bit 1 the guild's).
func encodeVoicePeers(peers []voicePeer) []byte {
	if len(peers) > 255 {
		peers = peers[:255]
	}
	out := []byte{byte(len(peers))}
	for _, v := range peers {
		out = binary.LittleEndian.AppendUint32(out, v.id)
		out = writeVoiceString(out, v.name, voiceMaxName)
		if v.offerer {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
		out = append(out, v.channels)
	}
	return out
}

// sendVoicePeersLocked sends p's peers when they differ from the last set
// sent (or one is due). Must be called with the lock held.
func (w *World) sendVoicePeersLocked(sess *session.Session, p *Player) {
	body := encodeVoicePeers(w.voicePeersLocked(p))
	if !p.voicePeersDue && string(body) == p.voicePeersSent {
		return
	}
	p.voicePeersSent, p.voicePeersDue = string(body), false
	sendGroupMsg(sess, protocol.OpcodeVoicePeers, func(b *buffer.Writer) { b.WriteBytes(body) })
}

// voiceConfigBody is OpcodeVoiceConfig's body for one username: a u8 count,
// each URL with the username and its credential, then the u32 seconds they
// last.
func voiceConfigBody(v VoiceSettings, username string) []byte {
	cred := TurnCredential(v.Secret, username)
	urls := v.TurnURLs
	if len(urls) > 255 {
		urls = urls[:255]
	}
	out := []byte{byte(len(urls))}
	for _, u := range urls {
		out = writeVoiceString(out, u, 1024)
		out = writeVoiceString(out, username, 256)
		out = writeVoiceString(out, cred, 256)
	}
	return binary.LittleEndian.AppendUint32(out, uint32(v.TTL/time.Second))
}

// sendVoiceConfigLocked mints p's credentials, good for the TTL from now, and
// sends them. The username names the player by its voice id alone. Must be
// called with the lock held.
func (w *World) sendVoiceConfigLocked(sess *session.Session, p *Player, now time.Time) {
	until := now.Add(w.voice.TTL)
	body := voiceConfigBody(w.voice, fmt.Sprintf("%d:v%08x", until.Unix(), p.VoiceID))
	p.voiceCredsUntil = until
	sendGroupMsg(sess, protocol.OpcodeVoiceConfig, func(b *buffer.Writer) { b.WriteBytes(body) })
}

// voiceRefresh is how long before they run out credentials are sent again.
func voiceRefresh(ttl time.Duration) time.Duration {
	return min(time.Hour, ttl/4)
}

// voiceSyncSystem keeps the guilds' voice channels (a place freed lets one
// waiting in), sends each voice-on client its credentials again before they
// run out, and its peers whenever they changed (a member grouped or gone, a
// voice turned on or off, a guild left, a logout). Every 5 ticks, after the
// group's roster. Must be called with the lock held.
func (w *World) voiceSyncSystem(now time.Time) {
	if w.TickCount%5 != 0 || w.SessionManager == nil || !w.voice.usable() {
		return
	}
	w.voiceGuildChannelsLocked(now)
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.VoiceOn {
			return true
		}
		if !now.Before(p.voiceCredsUntil.Add(-voiceRefresh(w.voice.TTL))) {
			w.sendVoiceConfigLocked(s, p, now)
		}
		w.sendVoicePeersLocked(s, p)
		return true
	})
}

// RelayVoiceSignal takes a client's OpcodeVoiceSignal (data: the message
// after the opcode) and sends it on to the peer it names, named in its turn
// by the sender's voice id: only from a voice-on player to one of its peers
// (a voice-on groupmate, or another in its guild's voice channel while it is
// in it), a well-formed message, within the sender's allowance.
// Anything else is dropped; it returns why ("" when relayed), logged at most
// every voiceDropLogEvery a session.
func (w *World) RelayVoiceSignal(sess *session.Session, data []byte, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "no player"
	}
	why := w.relayVoiceSignalLocked(p, data, now)
	if why != "" && now.Sub(p.voiceDropLogged) >= voiceDropLogEvery {
		p.voiceDropLogged = now
		log.Printf("Voice: %s's signal dropped: %s", p.Name, why)
	}
	return why
}

func (w *World) relayVoiceSignalLocked(p *Player, data []byte, now time.Time) string {
	if !w.voice.usable() {
		return "voice is off on this server"
	}
	if !p.VoiceOn {
		return "its voice is off"
	}
	if len(data) < voiceSignalHead {
		return fmt.Sprintf("short (%d bytes)", len(data))
	}
	to := binary.LittleEndian.Uint32(data[0:4])
	kind := data[8]
	part := binary.LittleEndian.Uint16(data[9:11])
	parts := binary.LittleEndian.Uint16(data[11:13])
	n := int(binary.LittleEndian.Uint16(data[13:15]))
	switch {
	case kind < voiceKindOffer || kind > voiceKindBye:
		return fmt.Sprintf("kind %d", kind)
	case parts == 0 || parts > voiceMaxParts || part >= parts:
		return fmt.Sprintf("part %d of %d", part, parts)
	case n > voiceMaxChunk:
		return fmt.Sprintf("a %d-byte chunk", n)
	case len(data) != voiceSignalHead+n:
		return fmt.Sprintf("%d bytes for a %d-byte chunk", len(data), n)
	}
	var peer *Player
	for _, v := range w.voicePeersLocked(p) {
		if v.id == to {
			peer = v.p
			break
		}
	}
	if peer == nil {
		return fmt.Sprintf("voice id %08x is no voice peer of its", to)
	}
	if !p.voiceRate.take(len(data), now) {
		return "too much signaling"
	}
	ts := w.findSessionByEntityIDLocked(peer.ServerID)
	if ts == nil {
		return "its peer has no session"
	}
	out := make([]byte, len(data))
	copy(out, data)
	binary.LittleEndian.PutUint32(out[0:4], p.VoiceID)
	sendGroupMsg(ts, protocol.OpcodeVoiceSignal, func(b *buffer.Writer) { b.WriteBytes(out) })
	return ""
}

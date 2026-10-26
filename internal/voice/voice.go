// Package voice is group, guild and room voice chat's forwarding server.
// Every client holds one WebRTC connection to it: its microphone on one
// sendonly audio track, and four recvonly ones on which it hears the four
// loudest of those talking to a channel it shares. It forwards RTP and never
// decodes it, so no player sees another's address, and one media port (UDP,
// and ICE-TCP on the same number for networks that block UDP) carries it
// all; an optional TURN server (turn.go) is there for networks that allow
// little else.
//
// The game server drives it (and the demo service, demo.go): Join a session
// with its group's channel, SetGroup when that changes, Leave, and Signal
// what its client sends (the offer, its candidates, a bye); the server's
// answer, its candidates, a bye and the session's roster go back through
// the session's Sink. A session has two channels: its group's (bit 0 of its
// client's {"send": B} and of its roster; "g:<group id>" in the game,
// "d:<32 hex>" a demo's) and its room of the lobby (bit 1; lobby.go), where
// demo and game players meet.
package voice

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

// chans are a session's channels: its group's and its room's ("r:" + the
// room's key), channel i its client's send bit i and roster bit i; "" none.
type chans [2]string

// has says whether c is one of the session's channels.
func (ch *chans) has(c string) bool {
	return c != "" && (ch[0] == c || ch[1] == c)
}

// Member is one of a session's roster: another session in a channel of its,
// with the bits of the channels they share (bit i: its channel i).
type Member struct {
	ID       uint32
	Name     string
	Channels byte
}

// Sink is how the server reaches one session's client. Its methods are
// called with the server's lock held, so they must not block or call back.
type Sink interface {
	// Signal sends the client a signaling message (KindAnswer,
	// KindCandidate, KindBye) whole; the carrier splits it.
	Signal(kind byte, text string)
	// Roster sends the client who it may hear (nil: none).
	Roster(members []Member)
}

// Config is the server's.
type Config struct {
	// BindIP is the address the ports are opened on ("": every address);
	// PublicIP the address the clients are told to reach it at.
	BindIP, PublicIP string
	// Port is the media port, UDP and TCP; 0 a free one (tests).
	Port int
	// TURN is the optional TURN server; its Port 0: none.
	TURN TURNConfig
	// RefuseRoom says whether a room's name is refused (the server's
	// profanity filter); nil: none is.
	RefuseRoom func(name string) bool
}

// Server is the forwarding server.
type Server struct {
	cfg  Config
	api  *webrtc.API
	udp  net.PacketConn
	tcp  net.Listener
	port int
	turn *turnServer

	clock func() time.Time // the lobby's (tests move it)

	mu     sync.Mutex
	peers  map[uint32]*peer
	rooms  map[string]*room // by key
	closed bool
	stop   chan struct{}
	wg     sync.WaitGroup
}

// opusCap is the one codec: Opus, as browsers offer it.
var opusCap = webrtc.RTPCodecCapability{
	MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2,
	SDPFmtpLine: "minptime=10;useinbandfec=1",
}

// New opens the media port (and the TURN server's, when it has one) and
// starts the server.
func New(cfg Config) (*Server, error) {
	public := net.ParseIP(cfg.PublicIP).To4()
	if public == nil {
		return nil, fmt.Errorf("voice: public address %q is not an IPv4 address", cfg.PublicIP)
	}
	udp, tcp, port, err := listenPair(cfg.BindIP, cfg.Port)
	if err != nil {
		return nil, err
	}
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: opusCap, PayloadType: 111}, webrtc.RTPCodecTypeAudio); err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sdp.AudioLevelURI}, webrtc.RTPCodecTypeAudio); err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.ConfigureRTCPReports(ir); err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	var se webrtc.SettingEngine
	se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udp))
	se.SetICETCPMux(webrtc.NewICETCPMux(nil, tcp, 8))
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4})
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	// every candidate names the public address: the ports may be opened on
	// every address or behind a 1:1 NAT, and the ICE-TCP candidates come off
	// the interfaces' own addresses even when the port is opened on one
	if err := se.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
		External: []string{public.String()}, AsCandidateType: webrtc.ICECandidateTypeHost,
		Mode: webrtc.ICEAddressRewriteReplace,
	}); err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	bind := net.ParseIP(cfg.BindIP)
	if public.IsLoopback() || bind != nil && bind.IsLoopback() {
		se.SetIncludeLoopbackCandidate(true) // a private server on 127.x, the tests
	}
	s := &Server{
		cfg:   cfg,
		api:   webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se)),
		udp:   udp,
		tcp:   tcp,
		port:  port,
		clock: time.Now,
		peers: map[uint32]*peer{},
		rooms: map[string]*room{},
		stop:  make(chan struct{}),
	}
	if cfg.TURN.Port != 0 {
		if s.turn, err = startTURN(cfg.TURN, cfg.BindIP, public); err != nil {
			udp.Close()
			tcp.Close()
			return nil, err
		}
	}
	s.wg.Add(1)
	go s.loop()
	return s, nil
}

// listenPair opens UDP and TCP on one port of bindIP (0: any free one).
func listenPair(bindIP string, port int) (net.PacketConn, net.Listener, int, error) {
	for try := 0; ; try++ {
		udp, err := net.ListenPacket("udp4", net.JoinHostPort(bindIP, fmt.Sprint(port)))
		if err != nil {
			return nil, nil, 0, fmt.Errorf("voice: UDP %s:%d: %w", bindIP, port, err)
		}
		p := udp.LocalAddr().(*net.UDPAddr).Port
		tcp, err := net.Listen("tcp4", net.JoinHostPort(bindIP, fmt.Sprint(p)))
		if err == nil {
			return udp, tcp, p, nil
		}
		udp.Close()
		if port != 0 || try == 20 {
			return nil, nil, 0, fmt.Errorf("voice: TCP %s:%d: %w", bindIP, p, err)
		}
	}
}

// Port is the media port.
func (s *Server) Port() int { return s.port }

// TURNURLs are the TURN server's URLs (its UDP and TCP), nil without one.
func (s *Server) TURNURLs() []string {
	if s.turn == nil {
		return nil
	}
	return s.turn.urls
}

// TURNSecret is the TURN server's secret ("" without one), for minting its
// credentials.
func (s *Server) TURNSecret() string {
	if s.turn == nil {
		return ""
	}
	return s.cfg.TURN.Secret
}

// Close sends every session with a connection a bye (a restart: its client
// offers again, to the server that comes back) and closes everything.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stop)
	var pcs []*webrtc.PeerConnection
	for id, p := range s.peers {
		if p.pc != nil {
			p.sink.Signal(KindBye, "")
			pcs = append(pcs, p.pc)
			p.pc = nil
		}
		p.gone = true
		close(p.sigs)
		delete(s.peers, id)
	}
	s.mu.Unlock()
	for _, pc := range pcs {
		pc.Close()
	}
	s.wg.Wait()
	if s.turn != nil {
		s.turn.close()
	}
	s.udp.Close()
	s.tcp.Close()
}

// peer is one session.
type peer struct {
	id   uint32
	name string
	ch   chans
	room *room
	// wrongAt is when it last brought a wrong H (roomRetry)
	wrongAt time.Time
	sink    Sink
	sigs    chan sig
	gone    bool

	// the connection; gen counts them, so a callback of an old one is
	// told apart
	pc        *webrtc.PeerConnection
	gen       int
	tcs       [Slots]*webrtc.RTPTransceiver
	tracks    [Slots]*webrtc.TrackLocalStaticRTP
	nslots    int // slots the client's offer has (its recvonly audio m-lines, up to Slots)
	answered  bool
	connected bool
	dc        *webrtc.DataChannel

	// as a talker: its {"send": B}, whether its microphone track came, its
	// last packet and its loudness (127 - the RFC 6464 -dBov, smoothed)
	send     byte
	hasTrack bool
	lastPkt  time.Time
	loud     float64

	// as a listener
	seats      [Slots]seat
	rosterSent string
	slotsSent  string
	levelsSent bool
}

type sig struct {
	kind byte
	text string
}

// sendsTo says whether t's talking now reaches l: a channel t sends to is
// one of l's.
func sendsTo(t, l *peer) bool {
	for i, c := range t.ch {
		if t.send&(1<<i) != 0 && l.ch.has(c) {
			return true
		}
	}
	return false
}

// Join adds a session with its name (as others see it) and its group's
// channel ("" none), and returns its voice id (stable while it is in; 0 once
// the server is closed). Its client's offer comes through Signal.
func (s *Server) Join(name, group string, sink Sink) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0
	}
	id := s.newIDLocked()
	// rosterSent none yet: its first roster goes, empty or not
	p := &peer{id: id, name: name, ch: chans{group}, sink: sink, sigs: make(chan sig, 32), rosterSent: "\x00"}
	s.peers[id] = p
	s.wg.Add(1)
	go s.runPeer(p)
	s.rostersLocked()
	return id
}

// newIDLocked is a random voice id no session has, never 0.
func (s *Server) newIDLocked() uint32 {
	var b [4]byte
	for {
		rand.Read(b[:])
		if id := binary.LittleEndian.Uint32(b[:]); id != 0 && s.peers[id] == nil {
			return id
		}
	}
}

// SetGroup changes a session's group channel (a group joined or left).
func (s *Server) SetGroup(id uint32, group string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peers[id]
	if p == nil || p.ch[0] == group {
		return
	}
	p.ch[0] = group
	s.rostersLocked()
}

// Leave takes a session out. With bye, its client is told the connection
// is over (the server dropped it: the client offers again when it may);
// without, it hung up itself (its voice turned off).
func (s *Server) Leave(id uint32, bye bool) {
	s.mu.Lock()
	p := s.peers[id]
	if p == nil {
		s.mu.Unlock()
		return
	}
	delete(s.peers, id)
	p.gone = true
	close(p.sigs)
	s.leaveRoomLocked(p, s.clock())
	if bye && p.pc != nil {
		p.sink.Signal(KindBye, "")
	}
	p.sink.Roster(nil)
	msgs := s.unseatLocked(p)
	s.rostersLocked()
	s.mu.Unlock()
	send(msgs)
}

// Signal hands the server what a session's client sent, whole: its offer,
// a candidate, a bye. It does not wait; the session's signals are handled in
// order, apart from everyone else's.
func (s *Server) Signal(id uint32, kind byte, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peers[id]
	if p == nil || p.gone {
		return
	}
	select {
	case p.sigs <- sig{kind, text}:
	default:
		log.Printf("Voice: %08x's signals are backed up; one dropped", id)
	}
}

// Connected says whether a session's connection is up.
func (s *Server) Connected(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peers[id]
	return p != nil && p.connected
}

// rostersLocked sends each session whose roster changed its new one: every
// other session in a channel of its, with the channels they share,
// groupmates (bit 0) first, then by name. Must be called with s.mu held.
func (s *Server) rostersLocked() {
	for _, r := range s.peers {
		var ms []Member
		for _, o := range s.peers {
			if o == r {
				continue
			}
			var bits byte
			for i, c := range r.ch {
				if o.ch.has(c) {
					bits |= 1 << i
				}
			}
			if bits != 0 {
				ms = append(ms, Member{ID: o.id, Name: o.name, Channels: bits})
			}
		}
		sort.Slice(ms, func(i, j int) bool {
			if gi, gj := ms[i].Channels&1 != 0, ms[j].Channels&1 != 0; gi != gj {
				return gi
			}
			if ms[i].Name != ms[j].Name {
				return ms[i].Name < ms[j].Name
			}
			return ms[i].ID < ms[j].ID
		})
		var key strings.Builder
		for _, m := range ms {
			fmt.Fprintf(&key, "%d/%s/%d;", m.ID, m.Name, m.Channels)
		}
		if k := key.String(); k != r.rosterSent {
			r.rosterSent = k
			r.sink.Roster(ms)
		}
	}
}

// dcMsg is a message for a listener's data channel, sent outside the lock.
type dcMsg struct {
	dc   *webrtc.DataChannel
	text string
}

func send(msgs []dcMsg) {
	for _, m := range msgs {
		if m.dc != nil {
			m.dc.SendText(m.text) // a channel closing fails quietly
		}
	}
}

// slotsMsgLocked is l's {"slots"} when it changed since the last one sent,
// [[slot, voice id], ...] for each seat taken.
func slotsMsgLocked(l *peer) (dcMsg, bool) {
	var b strings.Builder
	b.WriteString(`{"slots":[`)
	n := 0
	for i := 0; i < l.nslots; i++ {
		if t := l.seats[i].talker; t != 0 {
			if n > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "[%d,%d]", i, t)
			n++
		}
	}
	b.WriteString("]}")
	if b.String() == l.slotsSent || l.dc == nil {
		return dcMsg{}, false
	}
	l.slotsSent = b.String()
	return dcMsg{l.dc, l.slotsSent}, true
}

// unseatLocked takes t off every listener's seats, and returns their new
// {"slots"}.
func (s *Server) unseatLocked(t *peer) []dcMsg {
	var msgs []dcMsg
	for _, l := range s.peers {
		for i := range l.seats {
			if l.seats[i].talker == t.id {
				l.seats[i] = seat{rw: l.seats[i].rw}
				l.seats[i].rw.fresh = true
				if m, ok := slotsMsgLocked(l); ok {
					msgs = append(msgs, m)
				}
			}
		}
	}
	return msgs
}

var errGone = errors.New("voice: session gone")

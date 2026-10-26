package voice

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

// The forwarding server over loopback, against Pion clients that offer as
// the game's clients do (the sendonly microphone first, then the four
// recvonly slots, then the data channel "eqoa") and talk RTP that carries
// the RFC 6464 level, the payload naming the talker.

// testSink is a session's sink, kept for the test to read.
type testSink struct {
	sigs chan sig

	mu     sync.Mutex
	roster []Member
	n      int // rosters had
}

func newTestSink() *testSink { return &testSink{sigs: make(chan sig, 64)} }

func (k *testSink) Signal(kind byte, text string) {
	select {
	case k.sigs <- sig{kind, text}:
	default:
	}
}

func (k *testSink) Roster(ms []Member) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.roster = append([]Member(nil), ms...)
	k.n++
}

func (k *testSink) members() []Member {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]Member(nil), k.roster...)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// heard is a packet a client got on one of its slots.
type heard struct {
	slot    int
	from    uint32 // its CSRC
	payload string
	seq     uint16
	ts      uint32
	ssrc    uint32
}

// testClient is one player's client.
type testClient struct {
	t    *testing.T
	s    *Server
	id   uint32
	sink *testSink
	pc   *webrtc.PeerConnection
	mic  *webrtc.TrackLocalStaticRTP
	ext  uint8
	dc   *webrtc.DataChannel
	got  chan heard
	msgs chan string
	seq  uint16
	ts   uint32
}

func dialClient(t *testing.T, s *Server, name, group string) *testClient {
	t.Helper()
	c := &testClient{t: t, s: s, sink: newTestSink(), got: make(chan heard, 4096), msgs: make(chan string, 1024)}
	c.id = s.Join(name, group, c.sink)
	if c.id == 0 {
		t.Fatal("no voice id")
	}
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: opusCap, PayloadType: 111}, webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sdp.AudioLevelURI}, webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatal(err)
	}
	var se webrtc.SettingEngine
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	pc, err := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	c.pc = pc
	t.Cleanup(func() { pc.Close() })
	c.mic, _ = webrtc.NewTrackLocalStaticRTP(opusCap, "mic", "me")
	tx, err := pc.AddTransceiverFromTrack(c.mic, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	if err != nil {
		t.Fatal(err)
	}
	var rx [Slots]*webrtc.RTPTransceiver
	for i := range rx {
		if rx[i], err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
			t.Fatal(err)
		}
	}
	pc.OnTrack(func(tr *webrtc.TrackRemote, rc *webrtc.RTPReceiver) {
		slot := -1
		for i, x := range rx {
			if x.Receiver() == rc {
				slot = i
			}
		}
		go func() {
			for {
				p, _, err := tr.ReadRTP()
				if err != nil {
					return
				}
				h := heard{slot: slot, payload: string(p.Payload), seq: p.SequenceNumber, ts: p.Timestamp, ssrc: p.SSRC}
				if len(p.CSRC) == 1 {
					h.from = p.CSRC[0]
				}
				select {
				case c.got <- h:
				default:
				}
			}
		}()
	})
	c.dc, err = pc.CreateDataChannel("eqoa", nil)
	if err != nil {
		t.Fatal(err)
	}
	open := make(chan struct{})
	c.dc.OnOpen(func() { close(open) })
	c.dc.OnMessage(func(m webrtc.DataChannelMessage) {
		select {
		case c.msgs <- string(m.Data):
		default:
		}
	})
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gathered
	s.Signal(c.id, KindOffer, pc.LocalDescription().SDP)
	answer := c.waitSignal(KindAnswer)
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-open:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: the data channel never opened", name)
	}
	for _, h := range tx.Sender().GetParameters().HeaderExtensions {
		if h.URI == sdp.AudioLevelURI {
			c.ext = uint8(h.ID)
		}
	}
	if c.ext == 0 {
		t.Fatal("no audio level extension negotiated")
	}
	return c
}

func (c *testClient) waitSignal(kind byte) string {
	c.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-c.sink.sigs:
			if m.kind == kind {
				return m.text
			}
		case <-deadline:
			c.t.Fatalf("no signal of kind %d", kind)
		}
	}
}

func (c *testClient) send(bits int) {
	c.t.Helper()
	b, _ := json.Marshal(map[string]int{"send": bits})
	if err := c.dc.SendText(string(b)); err != nil {
		c.t.Fatal(err)
	}
}

// talk sends one 20 ms packet at that level (-dBov), its payload the name.
func (c *testClient) talk(level uint8, name string) {
	c.seq++
	c.ts += opusFrame
	p := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: c.seq, Timestamp: c.ts}, Payload: []byte(name)}
	al, _ := rtp.AudioLevelExtension{Level: level, Voice: true}.Marshal()
	p.Header.SetExtension(c.ext, al)
	c.mic.WriteRTP(p)
}

// drain empties what a client heard.
func (c *testClient) drain() {
	for {
		select {
		case <-c.got:
		default:
			return
		}
	}
}

// listen collects what c hears for d, by talker.
func (c *testClient) listen(d time.Duration) map[uint32]int {
	out := map[uint32]int{}
	end := time.After(d)
	for {
		select {
		case h := <-c.got:
			out[h.from]++
		case <-end:
			return out
		}
	}
}

// talkFor has each talker talk every 20 ms for d at its level.
func talkFor(d time.Duration, talkers []*testClient, levels []uint8, names []string) {
	tk := time.NewTicker(20 * time.Millisecond)
	defer tk.Stop()
	end := time.After(d)
	for {
		select {
		case <-tk.C:
			for i, c := range talkers {
				c.talk(levels[i], names[i])
			}
		case <-end:
			return
		}
	}
}

// talkWhile talks in the background until stop is closed.
func talkWhile(stop chan struct{}, talkers []*testClient, levels []uint8, names []string) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tk := time.NewTicker(20 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				for i, c := range talkers {
					c.talk(levels[i], names[i])
				}
			case <-stop:
				return
			}
		}
	}()
	return &wg
}

// A talker in group 1 is heard by its groupmate, on a slot its {"slots"}
// names, the talker's voice id the CSRC, its words whole; a player of
// group 2 hears nothing, until it is moved into group 1 (the control).
func TestATalkerIsHeardInItsGroupOnly(t *testing.T) {
	s := newTestServer(t)
	a := dialClient(t, s, "Asker", "g:1")
	b := dialClient(t, s, "Bard", "g:1")
	c := dialClient(t, s, "Crier", "g:2")
	a.send(1)
	time.Sleep(100 * time.Millisecond)
	talkFor(time.Second, []*testClient{a}, []uint8{20}, []string{"Asker's words"})
	gb := b.listen(300 * time.Millisecond)
	if gb[a.id] < 20 {
		t.Fatalf("the groupmate heard %d of the talker's packets: %v", gb[a.id], gb)
	}
	if gc := c.listen(100 * time.Millisecond); len(gc) != 0 {
		t.Fatalf("a player of another group heard %v", gc)
	}
	var slots string
	for len(b.msgs) > 0 {
		if m := <-b.msgs; strings.HasPrefix(m, `{"slots"`) {
			slots = m
		}
	}
	if want := `[` + itoa(0) + `,` + utoa(a.id) + `]`; !strings.Contains(slots, want) {
		t.Fatalf("the groupmate's slots %q do not seat the talker (%s)", slots, want)
	}

	s.SetGroup(c.id, "g:1")
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"Asker's words"})
	if gc := c.listen(300 * time.Millisecond); gc[a.id] < 10 {
		t.Fatalf("moved into the group, it heard %v", gc)
	}
}

// Push to talk up ({"send": 0}) forwards nothing and frees the seat; down
// again ({"send": 1}, the control) it is heard. Talking to the room (bit 1)
// reaches no groupmate.
func TestPushToTalkUpForwardsNothing(t *testing.T) {
	s := newTestServer(t)
	a := dialClient(t, s, "Asker", "g:1")
	b := dialClient(t, s, "Bard", "g:1")
	a.send(0)
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	if g := b.listen(200 * time.Millisecond); len(g) != 0 {
		t.Fatalf("push to talk up, the groupmate heard %v", g)
	}
	a.send(2)
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	if g := b.listen(200 * time.Millisecond); len(g) != 0 {
		t.Fatalf("talking to its room (it has none), the groupmate heard %v", g)
	}
	a.send(1)
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	if g := b.listen(200 * time.Millisecond); g[a.id] < 10 {
		t.Fatalf("push to talk down, the groupmate heard %v", g)
	}
}

// Five talk at once to a listener's group, four loud and the fifth quieter,
// which started first and took a seat: once its hold runs out the four loud
// ones have the four slots and the fifth is not forwarded, without a seat
// flapping; when one of the four stops (the control), the fifth gets the
// seat it left.
func TestTheFourLoudestHaveTheSlots(t *testing.T) {
	s := newTestServer(t)
	l := dialClient(t, s, "Listener", "g:1")
	var ts []*testClient
	names := []string{"Loud1", "Loud2", "Loud3", "Loud4", "Quiet"}
	for _, n := range names {
		c := dialClient(t, s, n, "g:1")
		c.send(1)
		ts = append(ts, c)
	}
	time.Sleep(100 * time.Millisecond)
	quiet := ts[4]
	stopQ := make(chan struct{})
	wq := talkWhile(stopQ, []*testClient{quiet}, []uint8{60}, []string{"Quiet"})
	time.Sleep(400 * time.Millisecond)
	if g := l.listen(200 * time.Millisecond); g[quiet.id] < 5 {
		t.Fatalf("alone, the quiet one is not heard: %v", g)
	}
	stop := make(chan struct{})
	w := talkWhile(stop, ts[:4], []uint8{10, 12, 14, 16}, names[:4])
	time.Sleep(slotHold + 1500*time.Millisecond)
	l.drain()
	g := l.listen(600 * time.Millisecond)
	for i := 0; i < 4; i++ {
		if g[ts[i].id] < 15 {
			t.Errorf("%s: heard %d packets in 600 ms", names[i], g[ts[i].id])
		}
	}
	if g[quiet.id] != 0 {
		t.Errorf("the fifth, quieter, was forwarded %d packets beside four louder", g[quiet.id])
	}
	// the slots hold still
	l.drain()
	slots := map[uint32]int{}
	end := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case h := <-l.got:
			if s, ok := slots[h.from]; ok && s != h.slot {
				t.Errorf("%08x moved from slot %d to %d", h.from, s, h.slot)
			}
			slots[h.from] = h.slot
		case <-end:
			break collect
		}
	}

	ts[0].send(0)
	time.Sleep(500 * time.Millisecond)
	l.drain()
	g = l.listen(500 * time.Millisecond)
	close(stop)
	close(stopQ)
	w.Wait()
	wq.Wait()
	if g[ts[0].id] != 0 || g[quiet.id] < 10 {
		t.Fatalf("one of the four stopped: it %d, the fifth %d packets", g[ts[0].id], g[quiet.id])
	}
}

// A slot's stream runs on across a change of talker: one SSRC, sequence
// numbers one apart, timestamps forward.
func TestASlotIsOneStreamAcrossTalkers(t *testing.T) {
	s := newTestServer(t)
	l := dialClient(t, s, "Listener", "g:1")
	a := dialClient(t, s, "Asker", "g:1")
	b := dialClient(t, s, "Bard", "g:1")
	a.send(1)
	b.send(1)
	b.seq, b.ts = 40000, 3_000_000_000 // far from a's
	time.Sleep(100 * time.Millisecond)
	talkFor(300*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	time.Sleep(100 * time.Millisecond)
	a.send(0)
	time.Sleep(100 * time.Millisecond)
	talkFor(300*time.Millisecond, []*testClient{b}, []uint8{20}, []string{"B"})
	time.Sleep(200 * time.Millisecond)
	var hs []heard
	for len(l.got) > 0 {
		hs = append(hs, <-l.got)
	}
	var last *heard
	switched := false
	for i := range hs {
		h := &hs[i]
		if h.slot != 0 {
			t.Fatalf("one talker at a time, yet slot %d", h.slot)
		}
		if last != nil {
			if h.ssrc != last.ssrc {
				t.Fatalf("the slot's SSRC changed: %08x -> %08x", last.ssrc, h.ssrc)
			}
			if d := h.seq - last.seq; d != 1 {
				t.Fatalf("sequence %d -> %d at a %v -> %v", last.seq, h.seq, last.from, h.from)
			}
			if int32(h.ts-last.ts) < opusFrame {
				t.Fatalf("timestamp %d -> %d", last.ts, h.ts)
			}
			if h.from != last.from {
				switched = true
			}
		}
		last = h
	}
	if !switched || len(hs) < 20 {
		t.Fatalf("%d packets, switched %v", len(hs), switched)
	}
}

// Two in a room hear each other; a player in another room does not; leaving
// the room stops it (each with its control: before leaving it heard).
func TestARoomCarriesItsMembersOnly(t *testing.T) {
	s := newTestServer(t)
	a := dialClient(t, s, "Asker", "")
	b := dialClient(t, s, "Bard", "")
	c := dialClient(t, s, "Crier", "")
	if r, _ := s.JoinRoom(a.id, "Tavern", "", false); r != RoomIn {
		t.Fatalf("making the room: %d", r)
	}
	if r, name := s.JoinRoom(b.id, "  TAVERN ", "", false); r != RoomIn || name != "Tavern" {
		t.Fatalf("joining it: %d %q", r, name)
	}
	if r, _ := s.JoinRoom(c.id, "Cellar", "", false); r != RoomIn {
		t.Fatalf("another room: %d", r)
	}
	a.send(2)
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	if g := b.listen(200 * time.Millisecond); g[a.id] < 10 {
		t.Fatalf("its room's member heard %v", g)
	}
	if g := c.listen(50 * time.Millisecond); len(g) != 0 {
		t.Fatalf("another room's member heard %v", g)
	}
	if ms := b.sink.members(); len(ms) != 1 || ms[0].ID != a.id || ms[0].Channels != 2 {
		t.Fatalf("the member's roster %+v", ms)
	}
	if _, ok := s.LeaveRoom(b.id); !ok {
		t.Fatal("leaving")
	}
	time.Sleep(100 * time.Millisecond)
	b.drain()
	talkFor(600*time.Millisecond, []*testClient{a}, []uint8{20}, []string{"A"})
	if g := b.listen(200 * time.Millisecond); len(g) != 0 {
		t.Fatalf("out of the room, it heard %v", g)
	}
	if ms := b.sink.members(); len(ms) != 0 {
		t.Fatalf("out of the room, its roster %+v", ms)
	}
}

// Closing the server sends every connected session a bye.
func TestARestartSaysBye(t *testing.T) {
	s, err := New(Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	a := dialClient(t, s, "Asker", "g:1")
	s.Close()
	a.waitSignal(KindBye)
}

func itoa(n int) string    { b, _ := json.Marshal(n); return string(b) }
func utoa(n uint32) string { b, _ := json.Marshal(n); return string(b) }

package voice

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

// A session's connection: the client offers (one sendonly audio m-line, its
// microphone; then four recvonly ones, slots 0-3 in m-line order; then the
// data channel "eqoa") and the server answers with every candidate in the
// answer. The talker's packets are forwarded as they come (onPacket); the
// seats are settled ten times a second (tick).

const (
	tickEvery     = 100 * time.Millisecond
	gatherTimeout = 3 * time.Second
	// a data channel message from a client, at most
	maxDCMessage = 256
)

// runPeer handles a session's signals in order until it leaves.
func (s *Server) runPeer(p *peer) {
	defer s.wg.Done()
	for m := range p.sigs {
		switch m.kind {
		case KindOffer:
			s.onOffer(p, m.text)
		case KindCandidate:
			s.onCandidate(p, m.text)
		case KindBye:
			s.hangUp(p, false)
		}
	}
	s.hangUp(p, false)
}

// onOffer answers a client's offer, making its connection first when it has
// none (a renegotiation, or an ICE restart, goes to the one it has).
func (s *Server) onOffer(p *peer, offer string) {
	pc, gen, err := s.ensurePC(p)
	if err != nil {
		if err != errGone {
			log.Printf("Voice: %08x: %v", p.id, err)
		}
		return
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		log.Printf("Voice: %08x's offer refused: %v", p.id, err)
		s.hangUp(p, true)
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("Voice: %08x: no answer: %v", p.id, err)
		s.hangUp(p, true)
		return
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("Voice: %08x: %v", p.id, err)
		s.hangUp(p, true)
		return
	}
	select {
	case <-gathered:
	case <-time.After(gatherTimeout):
	case <-s.stop:
		return
	}
	local := pc.LocalDescription()
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.gone || p.gen != gen || local == nil {
		return
	}
	// slot i is the i-th recvonly audio m-line: the slot transceivers were
	// matched to them in order, as many as the offer has
	n := 0
	for _, t := range p.tcs {
		if t != nil && t.Mid() != "" {
			n++
		}
	}
	p.nslots = n
	p.answered = true
	p.sink.Signal(KindAnswer, local.SDP)
}

// ensurePC is the session's connection, made with its four slot tracks when
// it has none.
func (s *Server) ensurePC(p *peer) (*webrtc.PeerConnection, int, error) {
	s.mu.Lock()
	if p.gone {
		s.mu.Unlock()
		return nil, 0, errGone
	}
	if p.pc != nil {
		pc, gen := p.pc, p.gen
		s.mu.Unlock()
		return pc, gen, nil
	}
	s.mu.Unlock()
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, 0, err
	}
	var tracks [Slots]*webrtc.TrackLocalStaticRTP
	var tcs [Slots]*webrtc.RTPTransceiver
	for i := range tracks {
		tr, err := webrtc.NewTrackLocalStaticRTP(opusCap, fmt.Sprintf("slot%d", i), "eqoa")
		if err == nil {
			tcs[i], err = pc.AddTransceiverFromTrack(tr, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
		}
		if err != nil {
			pc.Close()
			return nil, 0, err
		}
		tracks[i] = tr
		go drainRTCP(tcs[i].Sender())
	}
	s.mu.Lock()
	if p.gone {
		s.mu.Unlock()
		pc.Close()
		return nil, 0, errGone
	}
	p.gen++
	gen := p.gen
	p.pc, p.tcs, p.tracks = pc, tcs, tracks
	p.nslots, p.answered, p.connected, p.dc = 0, false, false, nil
	p.send, p.hasTrack = 0, false // the client says {"send"} again on its new channel
	p.slotsSent, p.levelsSent = "", false
	for i := range p.seats {
		p.seats[i] = seat{}
	}
	s.mu.Unlock()
	pc.OnTrack(func(tr *webrtc.TrackRemote, rc *webrtc.RTPReceiver) { s.onTrack(p, gen, tr, rc) })
	pc.OnDataChannel(func(dc *webrtc.DataChannel) { s.onDataChannel(p, gen, dc) })
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) { s.onState(p, gen, st) })
	pc.OnICECandidate(func(c *webrtc.ICECandidate) { s.onLocalCandidate(p, gen, c) })
	return pc, gen, nil
}

// drainRTCP reads a sender's RTCP (its interceptors need it read) until the
// connection closes.
func drainRTCP(snd *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := snd.Read(buf); err != nil {
			return
		}
	}
}

// onLocalCandidate trickles a candidate found after the answer went (the
// answer carries the others).
func (s *Server) onLocalCandidate(p *peer, gen int, c *webrtc.ICECandidate) {
	if c == nil {
		return
	}
	b, err := json.Marshal(c.ToJSON())
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.gen == gen && p.answered && !p.gone {
		p.sink.Signal(KindCandidate, string(b))
	}
}

// onCandidate adds a client's candidate (an empty one ends its candidates).
func (s *Server) onCandidate(p *peer, text string) {
	var c webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(text), &c); err != nil || strings.TrimSpace(c.Candidate) == "" {
		return
	}
	s.mu.Lock()
	pc := p.pc
	s.mu.Unlock()
	if pc == nil {
		return
	}
	if err := pc.AddICECandidate(c); err != nil {
		log.Printf("Voice: %08x's candidate refused: %v", p.id, err)
	}
}

// hangUp ends the session's connection (it stays joined: a new offer makes
// another); with bye its client is told so, and offers again.
func (s *Server) hangUp(p *peer, bye bool) {
	s.mu.Lock()
	pc := p.pc
	if pc == nil {
		s.mu.Unlock()
		return
	}
	p.pc = nil
	p.gen++
	p.nslots, p.answered, p.connected, p.dc = 0, false, false, nil
	p.send, p.hasTrack = 0, false
	for i := range p.seats {
		p.seats[i] = seat{}
	}
	if bye && !p.gone {
		p.sink.Signal(KindBye, "")
	}
	msgs := s.unseatLocked(p)
	s.mu.Unlock()
	send(msgs)
	go pc.Close()
}

// onState follows a connection: up, or failed (the client is told, and
// offers again) or closed by the client.
func (s *Server) onState(p *peer, gen int, st webrtc.PeerConnectionState) {
	switch st {
	case webrtc.PeerConnectionStateConnected:
		s.mu.Lock()
		if p.gen == gen {
			p.connected = true
		}
		s.mu.Unlock()
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		s.mu.Lock()
		mine := p.gen == gen && p.pc != nil
		s.mu.Unlock()
		if mine {
			s.hangUp(p, st == webrtc.PeerConnectionStateFailed)
		}
	}
}

// onTrack reads a session's microphone (its first audio track) and forwards
// each packet.
func (s *Server) onTrack(p *peer, gen int, tr *webrtc.TrackRemote, rc *webrtc.RTPReceiver) {
	if tr.Kind() != webrtc.RTPCodecTypeAudio {
		return
	}
	s.mu.Lock()
	if p.gen != gen || p.hasTrack {
		s.mu.Unlock()
		return
	}
	p.hasTrack = true
	s.mu.Unlock()
	ext := uint8(0)
	for _, h := range rc.GetParameters().HeaderExtensions {
		if h.URI == sdp.AudioLevelURI {
			ext = uint8(h.ID)
		}
	}
	for {
		pkt, _, err := tr.ReadRTP()
		if err != nil {
			return
		}
		level, ok := audioLevel(pkt, ext)
		if !s.onPacket(p, gen, pkt, level, ok, time.Now()) {
			return
		}
	}
}

// audioLevel is a packet's RFC 6464 level (-dBov, 0 loudest, 127 silence).
func audioLevel(pkt *rtp.Packet, ext uint8) (uint8, bool) {
	if ext == 0 {
		return 0, false
	}
	b := pkt.GetExtension(ext)
	if b == nil {
		return 0, false
	}
	var a rtp.AudioLevelExtension
	if a.Unmarshal(b) != nil {
		return 0, false
	}
	return a.Level, true
}

// out is one packet for one slot track, written outside the lock.
type out struct {
	track *webrtc.TrackLocalStaticRTP
	seq   uint16
	ts    uint32
	mark  bool
}

// onPacket forwards a talker's packet to every listener it reaches that
// has it on a seat (a listener with a seat free takes it there and then),
// rewritten for that seat, the talker's voice id its CSRC and no header
// extension (their ids are each connection's own). Nothing goes while the
// talker sends to no channel ({"send": 0}: push to talk up). False once the
// connection the packet came on is over.
func (s *Server) onPacket(t *peer, gen int, pkt *rtp.Packet, level uint8, hasLevel bool, now time.Time) bool {
	s.mu.Lock()
	if t.gone || t.gen != gen {
		s.mu.Unlock()
		return false
	}
	t.lastPkt = now
	raw := 64.0 // no level: as loud as most
	if hasLevel {
		raw = float64(127 - int(level&0x7F))
	}
	t.loud = t.loud*0.8 + raw*0.2
	if t.send == 0 {
		s.mu.Unlock()
		return true
	}
	var outs []out
	var msgs []dcMsg
	for _, l := range s.peers {
		if l == t || l.nslots == 0 || !sendsTo(t, l) {
			continue
		}
		k := -1
		for i := 0; i < l.nslots; i++ {
			if l.seats[i].talker == t.id {
				k = i
				break
			}
		}
		if k < 0 {
			for i := 0; i < l.nslots; i++ {
				if l.seats[i].talker == 0 {
					k = i
					break
				}
			}
			if k < 0 {
				continue
			}
			seatTalker(&l.seats[k], t.id, now)
			if m, ok := slotsMsgLocked(l); ok {
				msgs = append(msgs, m)
			}
		}
		seq, ts, mark := l.seats[k].rw.next(t.id, pkt.SequenceNumber, pkt.Timestamp, now)
		outs = append(outs, out{l.tracks[k], seq, ts, mark})
	}
	id := t.id
	s.mu.Unlock()
	send(msgs)
	for _, o := range outs {
		o.track.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version: 2, Marker: o.mark, SequenceNumber: o.seq, Timestamp: o.ts,
				CSRC: []uint32{id},
			},
			Payload: pkt.Payload,
		})
	}
	return true
}

// onDataChannel takes the client's "eqoa" channel: its {"send": B} in, the
// {"slots"} and {"levels"} out.
func (s *Server) onDataChannel(p *peer, gen int, dc *webrtc.DataChannel) {
	if dc.Label() != "eqoa" {
		dc.Close()
		return
	}
	dc.OnOpen(func() {
		s.mu.Lock()
		if p.gen != gen {
			s.mu.Unlock()
			return
		}
		p.dc = dc
		p.slotsSent = ""
		m, ok := slotsMsgLocked(p)
		s.mu.Unlock()
		if ok {
			send([]dcMsg{m})
		}
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		if !m.IsString || len(m.Data) > maxDCMessage {
			return
		}
		var v struct {
			Send *int `json:"send"`
		}
		if json.Unmarshal(m.Data, &v) != nil || v.Send == nil || *v.Send < 0 {
			return
		}
		s.setSend(p, gen, byte(*v.Send&7))
	})
}

// setSend makes the channels a talker sends to what its client says; off
// every seat at once when it sends to none. Its changes are held to the
// signaling's allowance.
func (s *Server) setSend(p *peer, gen int, b byte) {
	s.mu.Lock()
	if p.gen != gen || p.gone || p.send == b || !p.dcRate.Take(1, time.Now()) {
		s.mu.Unlock()
		return
	}
	p.send = b
	var msgs []dcMsg
	if b == 0 {
		msgs = s.unseatLocked(p)
	}
	s.mu.Unlock()
	send(msgs)
}

// loop settles the seats ten times a second.
func (s *Server) loop() {
	defer s.wg.Done()
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-t.C:
			send(s.tick(now))
		}
	}
}

// tick lets a quiet talker's loudness fall, settles each listener's seats
// (pick) and gives it its {"slots"} when they changed, and its {"levels"}:
// the RFC 6464 -dBov (0 loudest, 127 silence) of each talker it may hear
// that is talking, or one empty list once none is.
func (s *Server) tick(now time.Time) []dcMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireRoomsLocked(s.clock())
	for _, p := range s.peers {
		if now.Sub(p.lastPkt) > 2*tickEvery {
			p.loud *= 0.5
		}
	}
	var msgs []dcMsg
	for _, l := range s.peers {
		if l.nslots == 0 {
			continue
		}
		cands := s.candidatesLocked(l, now)
		pick(l.seats[:l.nslots], cands, now)
		if m, ok := slotsMsgLocked(l); ok {
			msgs = append(msgs, m)
		}
		if l.dc == nil {
			continue
		}
		var b strings.Builder
		b.WriteString(`{"levels":[`)
		n := 0
		for _, c := range cands {
			if c.loud < 1 {
				continue
			}
			if n > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "[%d,%d]", c.id, 127-int(math.Round(min(c.loud, 127))))
			n++
		}
		b.WriteString("]}")
		if n > 0 || l.levelsSent {
			msgs = append(msgs, dcMsg{l.dc, b.String()})
		}
		l.levelsSent = n > 0
	}
	return msgs
}

// candidatesLocked is who l may hear now, loudest first: every other
// session whose microphone came, heard of lately, talking to a channel of
// l's.
func (s *Server) candidatesLocked(l *peer, now time.Time) []candidate {
	var c []candidate
	for _, t := range s.peers {
		if t == l || !t.hasTrack || t.send == 0 || now.Sub(t.lastPkt) > talkerQuiet || !sendsTo(t, l) {
			continue
		}
		c = append(c, candidate{t.id, t.loud})
	}
	byLoudness(c)
	return c
}

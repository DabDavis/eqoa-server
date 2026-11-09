package session_test

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keptRoundTrip is a session through a restart: Keep, the snapshot's JSON,
// and Resume in another manager (the new process's).
func keptRoundTrip(t *testing.T, s *session.Session) (*session.Session, *session.Manager) {
	t.Helper()
	b, err := json.Marshal(s.Keep())
	if err != nil {
		t.Fatal(err)
	}
	var k session.Kept
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager()
	r, err := m.Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	return r, m
}

func reliable(op protocol.GameOpcode, body ...byte) *protocol.Message {
	m := protocol.NewMessage(protocol.MessageTypeReliable, op)
	m.SetData(append([]byte{byte(uint16(op)), byte(uint16(op) >> 8)}, body...))
	return m
}

// A promoted session in the world comes back under its own key with its
// identity, flags and numbers: the next packet and message it sends are
// numbered on from the old process's, the message the client sent last is
// still the one acknowledged, and a new login is not given its object id.
func TestAKeptSessionResumesWhereItWas(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}
	m := session.NewManager()
	s := m.CreateSession(addr, 0x2EC7D061, 0x2EC7D061, 0x1234, 0xB991, false)
	m.PromoteSessionToMaster(s, 500000042)
	s.SetNeedsNewInstance(false)
	s.SetAccountID(110)
	s.SetCharacterServerID(198)
	s.SetState(session.StateInGame)
	s.SetCharacterInWorld(true)
	s.SetLiveLook(true)
	s.SetBugReports(true)
	s.SetFlyClient(true)
	s.SetNumberTags(true)
	s.SetDiscVersion(0x12)
	s.OfferCapesOnce()
	s.SetCapes(true)
	conn := s.Connection()
	conn.SetEntitySlots(204)
	conn.SetViewDistance(1000)
	for i := 0; i < 7; i++ {
		conn.LastSentPacketSeq()
	}
	conn.SetLastReceivedPacketSeq(31)
	for seq := uint16(1); seq <= 9; seq++ {
		conn.SetLastReceivedMessageSeq(seq) // as the listener records them before a restart
	}
	s.Queue().AddWithSequence(reliable(protocol.OpcodeClientMessage, 1, 2, 3)) // seq 1
	s.Queue().AddWithSequence(reliable(protocol.OpcodeClientMessage, 4, 5))    // seq 2
	conn.SetClientLastReceivedMessage(1)
	s.Queue().GetPending() // both went out; 1 was acknowledged
	s.Queue().RemoveReliables()
	s.Queue().AddWithSequence(reliable(protocol.OpcodeClientMessage, 6)) // seq 3, still to go

	r, m2 := keptRoundTrip(t, s)
	if r.Key() != s.Key() || m2.GetSession(s.Key()) != r {
		t.Fatalf("key %v, want %v, filed: %v", r.Key(), s.Key(), m2.GetSession(s.Key()) == r)
	}
	if r.PlayerObjectID() != 500000042 || r.SessionID() != 500000042 || r.AccountID() != 110 || r.CharacterServerID() != 198 {
		t.Fatalf("ids: object %d session %d account %d char %d", r.PlayerObjectID(), r.SessionID(), r.AccountID(), r.CharacterServerID())
	}
	if !r.DidServerInitiate() || !r.HasInstance() || r.NeedsNewInstance() || !r.InGame() || !r.CharacterInWorld() ||
		r.State() != session.StateInGame || !r.LiveLook() || !r.BugReports() || !r.FlyClient() || r.DiscVersion() != 0x12 {
		t.Fatal("flags not kept")
	}
	// its figures' tags (entity/numbertags.go): still asked for
	if !r.NumberTags() {
		t.Fatal("the number tags' answer not kept")
	}
	// the capes (handlers/capes.go): the client still draws them, and is not
	// offered them again
	if !r.Capes() || !r.CapesOffered() || r.OfferCapesOnce() {
		t.Fatalf("capes: drawn %v, offered %v", r.Capes(), r.CapesOffered())
	}
	rc := r.Connection()
	if rc.EntitySlots() != 204 || rc.ViewDistance() != 1000 {
		t.Fatalf("view %d within %d", rc.EntitySlots(), rc.ViewDistance())
	}
	if got := rc.LastSentPacketSeq(); got != 8 {
		t.Fatalf("next packet %d, want 8", got)
	}
	if rc.LastReceivedPacketSeq() != 31 || rc.LastReceivedMessageSeq() != 9 || rc.ClientLastReceivedMessage() != 1 {
		t.Fatalf("rx packet %d message %d, acked %d", rc.LastReceivedPacketSeq(), rc.LastReceivedMessageSeq(), rc.ClientLastReceivedMessage())
	}
	if !r.Resumed() || s.Resumed() {
		t.Fatal("resumed: the session that came through, not the one before")
	}
	if rc.TakeReliable(9) || rc.TakeReliable(4) || !rc.TakeReliable(10) || rc.TakeReliable(10) {
		t.Fatal("the client's messages: 9 and 4 were taken before the restart, 10 is new once")
	}
	// message 2 went and was not acknowledged, message 3 never went: both
	// come again under their numbers, 3 once, and the next new one is 4
	if !r.Queue().HasRetransmits(0) {
		t.Fatal("the unacknowledged message 2 is not due again")
	}
	re := r.Queue().TakeRetransmits(0, 10)
	op := uint16(protocol.OpcodeClientMessage)
	if len(re) != 1 || re[0].Sequence != 2 || !bytes.Equal(re[0].Data(), []byte{byte(op), byte(op >> 8), 4, 5}) {
		t.Fatalf("retransmits %v", re)
	}
	p := r.Queue().GetPending()
	if len(p) != 1 || p[0].Sequence != 3 {
		t.Fatalf("pending %v", p)
	}
	if seq := r.Queue().AddWithSequence(reliable(protocol.OpcodeClientMessage, 7)); seq != 4 {
		t.Fatalf("the next message is %d, want 4", seq)
	}
	if id := m2.GeneratePlayerObjectID(); id <= 500000042 {
		t.Fatalf("a new login was given %d, the resumed session's or below", id)
	}
}

// The control for the numbers: a session made anew (what the new process had
// before this) starts its packets and messages at 1, which the client drops as
// old, and has no message of the old process to resend.
func TestAFreshSessionStartsOver(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 1, 1, 2, 3, true)
	if got := s.Connection().LastSentPacketSeq(); got != 1 {
		t.Fatalf("next packet %d", got)
	}
	if s.Queue().HasRetransmits(0) {
		t.Fatal("a fresh session has something to resend")
	}
}

// A state channel after the restart: its next message is numbered past every
// one the old process sent and goes out WHOLE, so CLIENT's receive (the
// drdpReceiver of statewindow_test.go, carried through the restart as the
// client is) takes it, and acknowledging it brings the deltas back. The
// control: the same message numbered from 1, as a fresh channel numbers it,
// is dropped as old.
func TestAKeptChannelGoesOutWholeAndNew(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 5, 5, 2, 3, true)
	ch := s.Connection().GetClientStatUpdate()
	var client drdpReceiver
	v := make([]byte, ch.Len())
	for i := 1; i <= 40; i++ {
		v[0], v[7] = byte(i), byte(i*3)
		ch.SetData(v)
		data, seq, ref := ch.GenerateXORDelta()
		got, ok := client.receive(data, seq, ref)
		if !ok || !bytes.Equal(got, v) {
			t.Fatalf("message %d before the restart: %v %x", seq, ok, got)
		}
		ch.UpdateBaseXor(seq)
	}
	r, _ := keptRoundTrip(t, s)
	// the control for the capes kept above: a client that never drew them
	// (a PS2's) comes back without them, and may still be offered them
	if r.Capes() || r.CapesOffered() {
		t.Fatalf("capes on a session that never answered: drawn %v, offered %v", r.Capes(), r.CapesOffered())
	}
	rch := r.Connection().GetClientStatUpdate()
	data, seq, ref := rch.GenerateXORDelta()
	if data == nil {
		t.Fatal("the kept channel sends nothing after the restart")
	}
	if ref != 0 || seq != 41 {
		t.Fatalf("seq %d ref %d, want 41 whole", seq, ref)
	}
	got, ok := client.receive(data, seq, ref)
	if !ok || !bytes.Equal(got, v) {
		t.Fatalf("the client took %v %x, want %x", ok, got, v)
	}
	rch.UpdateBaseXor(seq)
	v[3] = 0x55
	rch.SetData(v)
	data, seq, ref = rch.GenerateXORDelta()
	if ref != 1 {
		t.Fatalf("after its ack a change goes as a delta: ref %d", ref)
	}
	if got, ok := client.receive(data, seq, ref); !ok || !bytes.Equal(got, v) {
		t.Fatalf("the delta: %v %x", ok, got)
	}

	// the control
	fresh := session.NewStateUpdateData(len(v))
	fresh.SetData(v)
	data, seq, ref = fresh.GenerateXORDelta()
	if got, ok := client.receive(data, seq, ref); !ok || got != nil {
		t.Fatalf("a fresh channel's message %d was taken (%v %x): the client drops it as old", seq, ok, got)
	}
}

// An entity slot whose removal went out before the restart stays silent
// after it (a second removal on the now-empty slot would make CLIENT build
// the entity again), one whose removal had not gone yet goes once, and one
// with an entity on it goes out whole.
func TestKeptEntitySlotsRemovalsOnce(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 5, 5, 2, 3, true)
	conn := s.Connection()
	entity := func(id byte) []byte {
		b := make([]byte, 162)
		b[0], b[1], b[5] = 1, id, 0x82
		return b
	}
	sent, unsent, shown := conn.GetServerObject(1), conn.GetServerObject(2), conn.GetServerObject(3)
	for i, ch := range []*session.StateUpdateData{sent, unsent, shown} {
		ch.SetData(entity(byte(10 + i)))
		_, seq, _ := ch.GenerateXORDelta()
		ch.UpdateBaseXor(seq)
	}
	sent.MarkRemoved()
	_, seq, _ := sent.GenerateXORDelta()
	sent.UpdateBaseXor(seq)
	unsent.MarkRemoved() // the restart came before it went

	r, _ := keptRoundTrip(t, s)
	rc := r.Connection()
	if data, _, _ := rc.GetServerObject(1).GenerateXORDelta(); data != nil {
		t.Fatalf("the removal that went out goes again: %x", data[:6])
	}
	data, _, ref := rc.GetServerObject(2).GenerateXORDelta()
	if data == nil || ref != 0 || data[0] != 0 || data[1] != 11 || data[5] != 0x82 {
		t.Fatalf("the removal still to go: %x ref %d", data, ref)
	}
	if again, _, _ := rc.GetServerObject(2).GenerateXORDelta(); again != nil {
		t.Fatal("it goes twice")
	}
	data, _, ref = rc.GetServerObject(3).GenerateXORDelta()
	if data == nil || ref != 0 || data[0] != 1 || data[1] != 12 {
		t.Fatalf("the slot with an entity: %x ref %d", data, ref)
	}
}

// The client's own channel: its next message, XORed against the one the old
// process acknowledged, decodes in the new one; the control: a channel with
// no bases (a fresh session's) cannot.
func TestKeptClientBases(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 5, 5, 2, 3, true)
	cs := s.Connection().GetClientState()
	base := []byte{0, 1, 2, 3, 4, 5}
	cs.AddBase(77, base)
	cs.Took(77)
	r, _ := keptRoundTrip(t, s)
	rcs := r.Connection().GetClientState()
	if !bytes.Equal(rcs.GetBase(77), base) || rcs.Newer(77) || !rcs.Newer(78) || rcs.SeqNum != 77 {
		t.Fatalf("base %x, newer(77) %v, newer(78) %v, seq %d", rcs.GetBase(77), rcs.Newer(77), rcs.Newer(78), rcs.SeqNum)
	}
	if fresh := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 8), Port: 1}, 6, 6, 2, 3, true); fresh.Connection().GetClientState().GetBase(77) != nil {
		t.Fatal("the control has the base")
	}
}

// The game clock the client keeps comes through on the wall clock's terms.
func TestKeptClock(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 5, 5, 2, 3, true)
	now := time.Now()
	s.SetGameClock(session.GameDay/2, now) // noon
	want, ok := s.GameHour(now)
	if !ok {
		t.Fatal("no clock")
	}
	r, _ := keptRoundTrip(t, s)
	got, ok := r.GameHour(now)
	if !ok || got < want-0.01 || got > want+0.01 {
		t.Fatalf("hour %v (%v), want %v", got, ok, want)
	}
}

// TakeReliable: new, a copy, out of order, the edge of the window, and past
// 65535 (a resumed session's copies are the window's, as any session's:
// rxwindow.go).
func TestTakeReliable(t *testing.T) {
	cd := session.NewConnectionData(nil, 1, 2)
	steps := []struct {
		seq  uint16
		want bool
	}{
		{1, true}, {1, false}, {3, true}, {2, true}, {2, false}, {3, false},
		{68, false}, // 65 past the ack (3): beyond the window, not now
		{67, true},  // 64 past it: the window's edge
		{6, true},   // a hole below it, filled
		{0, false},  // before the ack: a copy
	}
	for _, st := range steps {
		if got := cd.TakeReliable(st.seq); got != st.want {
			t.Fatalf("seq %d: %v, want %v", st.seq, got, st.want)
		}
	}
	cd = session.NewConnectionData(nil, 1, 2)
	cd.SetLastReceivedMessageSeq(65533)
	for _, seq := range []uint16{65534, 65535, 0, 1} {
		if !cd.TakeReliable(seq) {
			t.Fatalf("seq %d not new", seq)
		}
	}
	if cd.TakeReliable(65535) || cd.LastReceivedMessageSeq() != 1 {
		t.Fatalf("past 65535: a copy of 65535 taken, or the newest is %d", cd.LastReceivedMessageSeq())
	}
}

// The receive window comes through a restart whole (RxAck, RxWin), and the
// file's RxMessage stays the newest taken, what a binary from before the
// window acknowledged and reads back: given the window's ack it would say
// less than the client was told, with no mask, which CLIENT refuses whole. An
// older binary's file (no RxAck) is taken as acknowledged to its RxMessage.
func TestTheReceiveWindowComesThroughARestart(t *testing.T) {
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 40123}, 5, 5, 2, 3, true)
	conn := s.Connection()
	for _, seq := range []uint16{1, 2, 4, 6} {
		conn.TakeReliable(seq)
	}
	s.SetAckMasks() // our acks carry their mask: kept too (CLIENT refuses one taken back)
	k := s.Keep()
	if k.Conn.RxMessage != 6 || k.Conn.RxAck == nil || *k.Conn.RxAck != 2 {
		t.Fatalf("the file: RxMessage %d (want 6, the newest taken), RxAck %v (want 2)", k.Conn.RxMessage, k.Conn.RxAck)
	}
	r, _ := keptRoundTrip(t, s)
	if !r.AckMasks() {
		t.Fatal("the ack masks did not come through the restart")
	}
	rep := r.Connection().NextPacketReport()
	if rep.Ack != 2 || rep.Mask != 0b101 {
		t.Fatalf("after the restart: ack %d mask %b, want 2 and 101 (4 and 6)", rep.Ack, rep.Mask)
	}
	if r.Connection().TakeReliable(4) || !r.Connection().TakeReliable(3) {
		t.Fatal("after the restart: 4 taken again, or 3 not taken")
	}
	// an older binary's file: acknowledged to RxMessage, no window
	k.Conn.RxAck, k.Conn.RxWin = nil, 0
	old, err := session.NewManager().Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	if rep := old.Connection().NextPacketReport(); rep.Ack != 6 || rep.Mask != 0 {
		t.Fatalf("an older binary's file: ack %d mask %b, want 6 and none", rep.Ack, rep.Mask)
	}
}

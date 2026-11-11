package udp

import (
	"github.com/eqoa/server/internal/debuglog"
	"log"
	"sort"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

const (
	// MaxRetries is the maximum number of retransmission attempts.
	MaxRetries = 5
	// RetransmitDelay is how long a reliable message may go unacknowledged
	// before it is sent again.
	//
	// 200ms (the original dead constant) is below the round trip once the
	// MemoryDump burst is in flight: fragments were resent while their ACKs
	// were still on the way, and world entry went from ~2s to ~5s. The PS2
	// acknowledges within ~60ms in a healthy capture, so 600ms leaves ample
	// headroom while still recovering from a real loss quickly.
	RetransmitDelay = 600 * time.Millisecond

	// MaxRetransmitsPerPacket caps how many unacked messages ride along in a
	// single packet, and ResendBudget the bytes they take of it (the first
	// whatever its size): a packet's resends leave room for what is new. Until
	// 2026-11-09 it was four messages of any size, every RetransmitDelay, the
	// oldest four always, so a stuck backlog went round four a tick forever.
	MaxRetransmitsPerPacket = 16
	ResendBudget            = 700

	// SendWindow is how far past the client's acknowledgement a reliable
	// message may go out: within the 64 its ack's mask can name (so every
	// message in flight is one the client can say it holds), and a client
	// that stops acknowledging is sent no more than this until it does.
	SendWindow = 48

	// SendWindowBytes is how many bytes of reliable messages may be out
	// unacknowledged at once: a slow line (a phone's, a relay's TCP stream
	// behind one) holds no more than this queued, so a message sent behind
	// a burst waits seconds less, and the timeout measured on it stays near
	// the line's. 16 KB in a 150 ms round trip is over 100 KB a second.
	SendWindowBytes = 16 * 1024

	// BurstPackets is how many packets more a tick may send while reliable
	// messages wait within the window (a MemoryDump's 24 fragments, a
	// mastery list's 10): one packet a tick carries one fragment, ten a
	// second, and everything queued after a burst waited behind it.
	BurstPackets = 3
)

// RDPHandler handles reliable delivery protocol operations.
type RDPHandler struct {
	listener *Listener
}

// NewRDPHandler creates a new RDP handler.
func NewRDPHandler(listener *Listener) *RDPHandler {
	return &RDPHandler{
		listener: listener,
	}
}

// ProcessIncoming processes an incoming packet for RDP operations.
func (r *RDPHandler) ProcessIncoming(sess *session.Session, packet *Packet) {
	if packet.Body == nil {
		if debuglog.Packets() {
			log.Printf("ProcessIncoming: packet has nil body, skipping")
		}
		return
	}

	body := packet.Body
	conn := sess.Connection()

	if debuglog.Packets() {
		log.Printf("ProcessIncoming: instanceID=0x%X, bodyFlags=0x%X, arrivalNum=%d, guaranteedAck=%d",
			sess.InstanceID(), body.Flags, body.ArrivalNumber, body.GuaranteedAck)
	}

	// Check for delayed/duplicate packets: at or before the newest taken,
	// by the signed 16-bit difference, as connection.cpp compares arrival
	// numbers. A plain <= stopped every packet of a client whose packet
	// numbers passed 65535 (two to three hours at a page's 6-10 packets a
	// second): its acks never taken again, all of our reliable messages
	// resent until its own gave up on ours (eqoaclassic, 01:39:15 UTC
	// 2026-11-09: arrival 0 to 458 against 65535).
	if !session.SeqAfter(body.ArrivalNumber, conn.LastReceivedPacketSeq()) {
		// A packet with no flush ack is a connection's first: always taken
		if body.FlushAck != 0 {
			log.Printf("ProcessIncoming: dropping delayed/duplicate packet (arrival=%d, lastRecv=%d)",
				body.ArrivalNumber, conn.LastReceivedPacketSeq())
			return
		}
	}

	conn.SetLastReceivedPacketSeq(body.ArrivalNumber)

	// the round trip to the packet it names, and which of our last 64 it
	// had (netstats.go)
	if body.Flags&protocol.BodyFlagFlushAck != 0 {
		conn.Net().NoteFlushAck(body.FlushAck, body.FlushAckMask,
			body.Flags&protocol.BodyFlagFlushAckMask != 0, time.Now())
	}

	// Process guaranteed ack if present
	if body.Flags&protocol.BodyFlagGuaranteedAck != 0 {
		if debuglog.Packets() {
			log.Printf("ProcessIncoming: GuaranteedAck flag is SET, calling processRDPReport")
		}
		r.processRDPReport(sess, body)
	} else {
		if debuglog.Packets() {
			log.Printf("ProcessIncoming: GuaranteedAck flag is NOT set")
		}
		// Still process state channel acks even without GuaranteedAck
		if body.Flags&protocol.BodyFlagClientUpdateAck != 0 {
			r.processStateAcks(sess, body)
		}
	}

	// Process session ack if present
	// For master sessions (didServerInitiate), never clear hasInstance - we always need it
	if body.Flags&protocol.BodyFlagSessionAck != 0 {
		if sess.InstanceID() == uint32(body.InstanceLocal) && !sess.DidServerInitiate() {
			sess.SetHasInstance(false)
		}
	}
}

// processRDPReport processes incoming acknowledgments.
func (r *RDPHandler) processRDPReport(sess *session.Session, body *protocol.SegmentBody) {
	conn := sess.Connection()

	if debuglog.Packets() {
		log.Printf("processRDPReport: received GuaranteedAck=%d from %s (instanceID=0x%X)",
			body.GuaranteedAck, sess.RemoteAddr(), sess.InstanceID())
	}

	// The client's ack of our reliable messages, and its mask: those it
	// holds past the ack (body flag 0x08)
	var mask uint64
	if body.Flags&protocol.BodyFlagGuaranteedAckMask != 0 {
		mask = body.GuaranteedAckMask
	}
	conn.SetClientAck(body.GuaranteedAck, mask)
	conn.Net().NoteAck(body.GuaranteedAck, time.Now())

	// Check if we should trigger the ACK callback (e.g., send character list)
	if debuglog.Packets() {
		hasCallback, target, fired := sess.DebugAckCallback()
		log.Printf("processRDPReport: before CheckAckCallback - hasCallback=%v, target=%d, fired=%v",
			hasCallback, target, fired)
	}

	if sess.CheckAckCallback(body.GuaranteedAck) {
		log.Printf("processRDPReport: ACK callback was triggered!")
	}

	// Remove acknowledged reliable messages from resend queue
	sess.Queue().RemoveReliables()

	// Process state channel acks
	r.processStateAcks(sess, body)
}

// processStateAcks handles state channel acknowledgments from the client.
func (r *RDPHandler) processStateAcks(sess *session.Session, body *protocol.SegmentBody) {
	if body.Flags&protocol.BodyFlagClientUpdateAck == 0 {
		return
	}
	conn := sess.Connection()
	if debuglog.Packets() {
		log.Printf("processStateAcks: %d state acks from %s (bodyFlags=0x%X)",
			len(body.StateAcks), sess.RemoteAddr(), body.Flags)
	}
	for _, stateAck := range body.StateAcks {
		if debuglog.Packets() {
			log.Printf("processStateAcks: channel=%d ack=%d", stateAck.Channel, stateAck.Ack)
		}
		if session.EntitySlot(stateAck.Channel) >= 0 {
			if state := conn.ServerObjectForChannel(stateAck.Channel); state != nil {
				state.UpdateBaseXor(stateAck.Ack)
			}
		} else if stateAck.Channel == byte(protocol.MessageTypeStatUpdate) {
			conn.GetClientStatUpdate().UpdateBaseXor(stateAck.Ack)
		} else if stateAck.Channel == byte(protocol.MessageTypeBuffUpdate) {
			conn.GetClientBuffUpdate().UpdateBaseXor(stateAck.Ack)
		} else if stateAck.Channel == byte(protocol.MessageTypeGroupUpdate) {
			conn.GetServerGroupUpdate().UpdateBaseXor(stateAck.Ack)
		}
	}
}

// mergeReliablesInSequenceOrder combines pending and retransmitted messages so
// that every reliable message goes out in ascending sequence order, while any
// non-reliable entries keep the slot they already had.
func mergeReliablesInSequenceOrder(pending, retrans []*protocol.Message) []*protocol.Message {
	combined := make([]*protocol.Message, 0, len(pending)+len(retrans))
	combined = append(combined, pending...)
	combined = append(combined, retrans...)

	// Positions holding reliable messages, and the messages themselves.
	slots := make([]int, 0, len(combined))
	reliables := make([]*protocol.Message, 0, len(combined))
	for i, msg := range combined {
		if msg.Type.IsReliable() {
			slots = append(slots, i)
			reliables = append(reliables, msg)
		}
	}
	sort.SliceStable(reliables, func(i, j int) bool {
		return session.SeqAfter(reliables[j].Sequence, reliables[i].Sequence)
	})
	for i, slot := range slots {
		combined[slot] = reliables[i]
	}
	return combined
}

// PreparePacket builds an outgoing packet for a session. The session is not
// promoted while it does (session.LockSend): what it drains from the queue
// and the header it writes over it are one connection's.
func (r *RDPHandler) PreparePacket(sess *session.Session) []byte {
	sess.LockSend()
	defer sess.UnlockSend()
	queue := sess.Queue()

	// Check if we need to send anything. A due retransmit counts as work even
	// when nothing new is queued — otherwise a stalled transfer never recovers.
	// The resend timeout: the client's round trip's (netstats.go's RTO)
	rto := sess.Connection().Net().RTO()
	needRetransmit := queue.HasRetransmits(rto)
	if !queue.CheckQueue() && !needRetransmit {
		flags := queue.GetBodyFlags()
		// Send if we have ACK flags OR RdpMessage flag (for responding to new connections)
		if flags&protocol.BodyFlagGuaranteedAck == 0 &&
			flags&protocol.BodyFlagClientUpdateAck == 0 &&
			flags&protocol.BodyFlagRdpMessage == 0 {
			return nil
		}
	}

	// Get body flags and pending messages
	// eqoa.live always includes SessionAck, RdpMessage, GuaranteedAck, FlushAck (0x63)
	rawFlags := queue.ClearBodyFlags()
	bodyFlags := rawFlags | protocol.BodyFlagRdpMessage | protocol.BodyFlagSessionAck
	if bodyFlags&protocol.BodyFlagGuaranteedAck != 0 {
		bodyFlags |= protocol.BodyFlagFlushAck
		if debuglog.Packets() {
			log.Printf("PreparePacket: Including GuaranteedAck! rawFlags=0x%X, bodyFlags=0x%X, lastRecvMsg=%d",
				rawFlags, bodyFlags, sess.Connection().LastReceivedMessageSeq())
		}
	}

	// Collect retransmits BEFORE draining pending. TakeRetransmits identifies a
	// message awaiting its first send by looking for it in the pending queue —
	// draining first empties that queue, so every not-yet-sent fragment looks
	// like a stale unacked one and gets "retransmitted" before it has ever been
	// sent, landing in the packet twice.
	var retrans []*protocol.Message
	if needRetransmit {
		retrans = queue.TakeResends(rto, MaxRetransmitsPerPacket, ResendBudget)
	}

	// Drain messages atomically to avoid races between queue peek and get.
	pending := queue.GetPending()
	if testAfterDrain != nil {
		testAfterDrain(sess)
	}

	// Reliable messages must leave in ascending sequence order. Retransmits and
	// freshly-queued messages are two independent ascending streams, so simply
	// putting one before the other inverts them: a MemoryDump fragment that was
	// requeued after a buffer-full is excluded from the retransmit set (it is
	// still pending), and prepending its successors sent 27 and 28 ahead of 26
	// on every tick. The client cannot close the gap and gives up.
	//
	// Non-reliable entries (state channels, pings) keep their positions; only
	// the reliable ones are ordered among themselves.
	if len(retrans) > 0 {
		pending = mergeReliablesInSequenceOrder(pending, retrans)
	}

	// Build the packet
	builder := NewPacketBuilder()
	conn := sess.Connection()

	// Write endpoints
	builder.WriteEndpoints(conn.ServerID(), conn.ClientID())

	// Use actual session flags for header
	hasInstance := sess.HasInstance()
	didServerInitiate := sess.DidServerInitiate()

	// For master sessions (didServerInitiate), always keep hasInstance true
	if didServerInitiate {
		hasInstance = true
	}

	if debuglog.Packets() {
		log.Printf("PreparePacket: hasInstance=%v, didServerInitiate=%v, needsNewInstance=%v, instanceID=0x%X, sessionID=0x%X, playerObjectID=%d, pendingMsgs=%d",
			hasInstance, didServerInitiate, sess.NeedsNewInstance(), sess.InstanceID(), sess.SessionID(), sess.PlayerObjectID(), len(pending))
	}

	// Build segment header flags (needed for patching body size later)
	segFlags := uint64(0)
	if didServerInitiate {
		segFlags |= uint64(protocol.SegmentFlagDidServerInitiate)
	} else {
		segFlags |= uint64(protocol.SegmentFlagClientInitiated)
	}
	if hasInstance {
		segFlags |= uint64(protocol.SegmentFlagHasInstance)
	}
	if sess.NeedsNewInstance() {
		segFlags |= uint64(protocol.SegmentFlagNewInstance)
		sess.SetNeedsNewInstance(false)
		log.Printf("PreparePacket: setting NewInstance flag (0x80000) for master session first packet")
	}

	// Write segment header with placeholder body size (0x7FF forces max 3-byte varint).
	// We'll patch the correct body size after writing all messages.
	segHeaderPos := builder.Position()
	header := &protocol.SegmentHeader{
		Size:     0x7FF,
		Flags:    protocol.SegmentHeaderFlags(segFlags),
		Instance: sess.InstanceID(),
	}
	builder.WriteSegmentHeader(header, hasInstance, didServerInitiate)

	// Write instance ID if hasInstance (C# RdpComm.AddSession)
	if hasInstance {
		builder.WriteInstance(sess.InstanceID())
	}

	// Write SessionID if server-initiated (C# RdpComm.AddSession line 291)
	// This is how the client receives the ObjectID - from the packet header
	if didServerInitiate {
		sessionID := sess.PlayerObjectID()
		builder.WriteSessionID(sessionID)
		if debuglog.Packets() {
			log.Printf("PreparePacket: writing SessionID=%d (0x%X) to header (ObjectID for client)", sessionID, sessionID)
		}
	}

	// === Body starts here (body size = everything from here to end of messages) ===
	bodyStartPos := builder.Position()

	// The packet's number and what it acknowledges, read at once
	// (session.NextPacketReport); the guaranteed ack's mask when the
	// client's messages came out of order, to a client that answered the
	// bug report offer (0x7E0E, any answer: every vanilla, never a PS2;
	// Session.AckMasks). A PS2's CLIENT
	// reads the mask as its own DRDP writes it, but no PS2 has been shown
	// one; it keeps the plain ack, the newest taken with every one before it
	// (its only cost a resend of what it sent past a hole).
	report := conn.NextPacketReport()
	includeAcks := bodyFlags&protocol.BodyFlagGuaranteedAck != 0
	if includeAcks && report.Mask != 0 && sess.AckMasks() {
		bodyFlags |= protocol.BodyFlagGuaranteedAckMask
	}

	// Write body flags
	builder.WriteBodyFlags(bodyFlags)

	// Write session ack if needed
	if bodyFlags&protocol.BodyFlagSessionAck != 0 {
		builder.WriteSessionAck(sess.InstanceID())
	}

	// Write RDP report
	packetSeq := report.Packet
	builder.WriteRDPReport(packetSeq, report.FlushAck, report.Ack, includeAcks)
	if bodyFlags&protocol.BodyFlagGuaranteedAckMask != 0 {
		builder.WriteAckMask(report.Mask)
	}

	// Write state channel acks if needed
	if bodyFlags&protocol.BodyFlagClientUpdateAck != 0 {
		clientState := conn.GetClientState()
		builder.WriteStateAck(0x40, clientState.SeqNum)
		builder.WriteStateAckEnd()
	}

	// Write pending messages (stop if buffer overflows)
	// Reliable messages that don't fit are re-queued for the next tick, as
	// are those past the window (SendWindow beyond the client's ack).
	clientAck, _ := conn.ClientAck()
	windowEnd := clientAck + SendWindow
	inFlight := queue.InFlightBytes()
	var deferred []*protocol.Message
	for idx, msg := range pending {
		var seq uint16
		if msg.Type.IsReliable() && msg.Acked {
			// acknowledged since it was queued again (a resend put back
			// when its packet was full): nothing to send
			continue
		}
		if msg.Type.IsReliable() && (session.SeqAfter(msg.Sequence, windowEnd) ||
			!session.Sent(msg) && inFlight > 0 && inFlight+msg.Size()+8 > SendWindowBytes) {
			// past the window, or the bytes out already enough: it waits
			// for the client's ack to move
			deferred = append(deferred, msg)
			continue
		}
		if msg.Type.IsReliable() && !session.Sent(msg) {
			inFlight += msg.Size() + 8
		}
		if msg.Type.IsReliable() {
			// The number AddWithSequence gave it (Queued marks one given):
			// 0 is a number like any other once the count passes 65535,
			// and a message numbered again here is one the client waits
			// for and no list resends.
			if !msg.Queued.IsZero() {
				seq = msg.Sequence
			} else {
				seq = conn.NextMessageSeq()
				msg.Sequence = seq
			}
		} else if msg.Type.IsStateChannel() {
			// State channels have their own sequence set in GenerateStateUpdates
			seq = msg.Sequence
		}
		if err := builder.WriteMessage(msg, seq); err != nil {
			if debuglog.Packets() {
				log.Printf("PreparePacket: buffer full writing message type=0x%02X, stopping (remaining=%d)",
					msg.Type, builder.Remaining())
			}
			// Re-queue reliable messages that didn't fit (after those put
			// off by the window, which came before them); drop unreliable
			// ones. Use RequeueBatch to preserve message order
			toRequeue := deferred
			deferred = nil
			for _, remaining := range pending[idx:] {
				if remaining.Type.IsReliable() {
					toRequeue = append(toRequeue, remaining)
				} else {
					protocol.PutMessage(remaining)
				}
			}
			queue.RequeueBatch(toRequeue)
			break
		}

		// Put unreliable messages back in pool after successful write
		if !msg.Type.IsReliable() {
			protocol.PutMessage(msg)
		} else {
			// Stamp when it actually went out. AddWithSequence stamps at
			// ENQUEUE, and a MemoryDump enqueues ~25 fragments at once that
			// then drain over seconds — so without this a fragment is already
			// older than RetransmitDelay the moment it is first transmitted,
			// and gets resent before its ACK could possibly arrive.
			msg.UpdateTime()
		}
	}

	if len(deferred) > 0 {
		queue.RequeueBatch(deferred)
	}

	// === Body ends here — patch segment header with actual body size ===
	actualBodySize := builder.Position() - bodyStartPos
	builder.PatchSegmentHeaderSize(segHeaderPos, segFlags, actualBodySize)

	// Finalize packet with CRC
	packet := builder.Finish()
	conn.Net().NoteSent(packetSeq, len(packet), time.Now())
	if debuglog.Packets() {
		log.Printf("Outgoing packet hex (%d bytes): %x", len(packet), packet)
	}
	return packet
}

// SendPacket sends a packet to a session.
func (r *RDPHandler) SendPacket(sess *session.Session, data []byte) error {
	return r.listener.SendToSession(data, sess)
}

// TickSession handles periodic session operations.
func (r *RDPHandler) TickSession(sess *session.Session) {
	// Check for timeout
	if sess.IsTimedOut() {
		sess.SendPing()
		sess.ResetTimeout()
	}

	// Check for ping timeout
	if sess.CheckPingTimeout() {
		sess.SetPendingTermination(true)
		return
	}

	// A session silent before the world ends after PreGameSilentEnd: its
	// client is gone (CLIENT keeps a connection alive with a keepalive every
	// 30 s, connection_flush), where one lived on for 10 pings of
	// world.session_timeout_pre (300 s on eqoaclassic: 50 minutes)
	silent := time.Since(sess.LastActivity())
	if !sess.InGame() && !sess.CharacterInWorld() && !sess.DidServerInitiate() && silent > PreGameSilentEnd {
		log.Printf("Session: %s silent %.0fs before the world: ended", sess.Label(), silent.Seconds())
		sess.SetPendingTermination(true)
		return
	}
	// One in the world silent ClientStoppedSilence: its client stopped, said
	// once (session/clientstopped.go); heard again, it was hung.
	clientStopped(sess, silent, time.Now())
	// One in the world silent InGameSilentEnd: its client is gone (below).
	if r.listener.silentInWorldOver(sess, silent) {
		return
	}

	// Generate state updates if in world using XOR delta encoding
	// Format: 162-byte state (isActive + ObjectID + ObjectUpdate), matching retail
	//
	// Held while the client is loading a world after a teleport — retail sends
	// nothing across that window either. Deferred, not dropped: the buffers keep
	// being written and go out the moment the client reports in with 0x0049.
	// Held too while the client has been silent StateSilence (a client in the
	// world sends its 0x40 ten times a second, a page in the background too):
	// a dropped client's session lingers its 50 pings with its character in
	// the world, and was sent the world's changes every tick, to a relay port
	// long closed (eqoaclassic's kernel counted ~9 such datagrams a second). A
	// client heard from again gets them from its last acknowledged state on.
	states := sess.CharacterInWorld() && !sess.StateChannelsHeld() && silent < StateSilence
	var round entityRound
	if states {
		round = r.GenerateStateUpdates(sess)
	}

	// Prepare and send packet if needed
	r.sendPrepared(sess)

	// A client keeping more entity slots than CLIENT's 24 (OpcodeEntitySlots)
	// has a packet more in the tick for each 24 slots more, each with up to
	// MaxStateUpdatesPerTick entity channels further round the round robin, so
	// each of its channels is served as often as a PS2's are. A PS2's tick is
	// the one packet above.
	// A burst of reliable messages goes out in more packets this tick, while
	// they wait within the client's window (BurstPackets)
	for p := 0; p < BurstPackets; p++ {
		ack, _ := sess.Connection().ClientAck()
		if !sess.Queue().Sendable(ack, SendWindow, SendWindowBytes) {
			break
		}
		r.sendPrepared(sess)
	}

	if states {
		conn := sess.Connection()
		more := false
		for p := 1; p < statePacketsPerTick(conn.EntitySlots()); p++ {
			if n, _, _ := r.queueEntityUpdates(sess, &round); n == 0 {
				break
			}
			more = true
			r.sendPrepared(sess)
		}
		if more {
			conn.SetStateUpdateOffset((round.start + round.visited) % round.n)
		}
	}
}

// StateSilence is how long a client in the world may be silent before its
// session's state channels wait for it (TickSession), and PreGameSilentEnd how
// long one before the world may be before its session ends.
const (
	StateSilence     = 5 * time.Second
	PreGameSilentEnd = 3 * time.Minute
)

// InGameSilentEnd is how long a client past character select may send
// nothing before its session ends and its character leaves the world.
//
// Until 2026-11-09 only the ping rule ended one: 50 pings unanswered, one
// every world.session_timeout_game, 10 s on eqoaclassic, so a closed tab, a
// crashed or hung page or a dropped line left its character standing in the
// world about 500 s (eqoaclassic that morning: Zaeire's client fell silent
// at 08:10 and the character stood until 08:18:51, other players targeting
// it, its session resending up to 240 messages every ten seconds to a relay
// port long closed), and a login of the character in that time took it over
// from a dead session. A client in the world is never that quiet: CLIENT
// sends its 0x40 ten times a second (SendPosition; the page in a hidden tab
// too, tick.js), and its DRDP sends a keepalive once 30 s pass with nothing
// sent (connection_flush). The longest silence a live client has is a PS2
// reading a world off the disc, about 22 s (claude-notes
// streaming-radius-and-hdd-20260814.md), and CLIENT itself gives up on a
// connection whose messages go 60 s unanswered (connection_set_failure's
// 60000, vanilla's too). 60 s is twice the keepalive: a live client is never
// cut, a gone one goes eight minutes sooner. The same as session.LiveWithin,
// past which a session no longer holds its login.
const InGameSilentEnd = 60 * time.Second

// silentInWorldOver ends a session past character select whose client has
// been silent InGameSilentEnd (and reports so), as a dropped client's end:
// its character saved and taken out of the world at the tick's end. One whose
// character is dead with its respawn to come waits for it, so it goes home to
// its bind point first, as a Quit in a fight does (entity/linkdead.go).
func (l *Listener) silentInWorldOver(sess *session.Session, silent time.Duration) bool {
	if silent <= InGameSilentEnd || sess.PendingTermination() {
		return false
	}
	if _, replaced := sess.ReplacedFor(); replaced {
		return false // its own grace ends it (session.ReplacedGrace)
	}
	if !sess.InGame() && !sess.CharacterInWorld() {
		return false
	}
	if l != nil && l.gameWorld != nil && l.gameWorld.RespawnPending(sess) {
		return false
	}
	log.Printf("Session: %s silent %.0fs in the world: its client is gone, ended", sess.Label(), silent.Seconds())
	sess.SetPendingTermination(true)
	return true
}

// clientStopped says a session in the world whose client has been silent
// session.ClientStoppedSilence stopped (a crash, a hang or a dropped line),
// once, naming its character: never while its world loads (the state channels
// held: a PS2 reads one off the disc in up to 22 s), after its client quit
// (0x09B0) or once it was replaced or is ending. A client heard from again
// under the silence after it was said is said to have been hung.
func clientStopped(sess *session.Session, silent time.Duration, now time.Time) {
	if silent < session.ClientStoppedSilence {
		session.NoteClientHeard(sess, now)
		return
	}
	if sess.PendingTermination() || !sess.CharacterInWorld() || sess.StateChannelsHeld() {
		return
	}
	if _, replaced := sess.ReplacedFor(); replaced {
		return
	}
	if _, quit := sess.QuittingSince(); quit {
		return
	}
	session.NoteClientStopped(sess, "silent in the world", now)
}

// testAfterDrain, set by a test alone, runs in PreparePacket between the
// queue's drain and the packet's header: where a promotion must not come.
var testAfterDrain func(sess *session.Session)

// SendWaiting is the tick's send for a session at character select whose
// client took the offer of the server's own worlds (session.ServerWorldsTaken:
// a client of ours, never a PS2): the reliable messages that wait in its
// queue go out now, in up to 1+BurstPackets packets, while they fit the
// client's window (NextFits), as TickSession sends a burst to a session in
// the world. Until 2026-11-10 a session before the world was sent a packet
// only in answer to one of its client's, so the worlds' recipes (70 KB, 62
// fragments) went one fragment for each of the client's acknowledgements,
// which its DRDP holds 100 ms: 8 s on a loopback and a round trip more a
// fragment on a real line, 10 to 36 s on eqoaclassic, every login, with every
// pick in a page waiting for it (petition 1468). A resend that is due goes
// with the first packet, and alone only while the client was heard from
// within StateSilence (the last fragment lost: the client, sent nothing
// more, says nothing more until its keepalive, 30 s on): a client that is
// gone is sent nothing from here. It reports how many packets went.
func (r *RDPHandler) SendWaiting(sess *session.Session) int {
	queue, conn := sess.Queue(), sess.Connection()
	sent := 0
	for p := 0; p <= BurstPackets; p++ {
		ack, _ := conn.ClientAck()
		if !queue.NextFits(ack, SendWindow, SendWindowBytes) {
			heard := time.Since(sess.LastActivity()) < StateSilence
			if p > 0 || !heard || !queue.HasRetransmits(conn.Net().RTO()) {
				break
			}
		}
		data := r.PreparePacket(sess)
		if data == nil {
			break
		}
		if err := r.SendPacket(sess, data); err != nil {
			log.Printf("Failed to send packet to %s: %v", sess.RemoteAddr(), err)
			break
		}
		sent++
	}
	return sent
}

// sendPrepared sends the packet PreparePacket builds, if there is one.
func (r *RDPHandler) sendPrepared(sess *session.Session) {
	if data := r.PreparePacket(sess); data != nil {
		if err := r.SendPacket(sess, data); err != nil {
			log.Printf("Failed to send packet to %s: %v", sess.RemoteAddr(), err)
		}
	}
}

// statePacketsPerTick is how many packets a tick may send a client keeping
// slots entity slots: one for each RetailEntitySlots, so 1 for a PS2.
func statePacketsPerTick(slots int) int {
	return (slots + session.RetailEntitySlots - 1) / session.RetailEntitySlots
}

// entityRound is a tick's round robin over the entity channels, slots 1 to n:
// slot 1 + (start+k)%n is the k-th visited, and visited of them have been.
type entityRound struct {
	start, visited, n int
}

// MaxStateUpdatesPerTick limits how many entity channel messages are queued for
// one packet (a PS2's tick sends one; see TickSession).
// Each state update is ~100-135 bytes after RLE. With MaxPacketSize=0x530 (1328 bytes)
// and ~100 bytes of header overhead, roughly 9-10 updates fit per packet.
// CRITICAL: This value must not exceed what fits in one packet, because calculateBodySize
// includes all queued messages in the segment header body size. If messages are dropped
// due to buffer overflow, the header body size won't match actual data, corrupting the packet.
// With round-robin, channels beyond this limit are served in subsequent ticks.
const MaxStateUpdatesPerTick = 10

// GenerateStateUpdates generates state channel updates using XOR delta encoding.
// Uses round-robin starting offset to ensure all channels get fair access to the packet.
// It returns the tick's round robin over the entity channels as it left it.
func (r *RDPHandler) GenerateStateUpdates(sess *session.Session) entityRound {
	conn := sess.Connection()

	// Player channel 0
	playerState := conn.GetServerObject(0)
	count := 0
	playerSent := false
	if data, seq, xorByte := playerState.GenerateXORDelta(); data != nil {
		msg := protocol.GetMessage()
		msg.Type = protocol.MessageType(0)
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
		count++
		playerSent = true
		if debuglog.Packets() {
			log.Printf("GenerateStateUpdates: channel 0 (player), seq=%d, xorByte=%d, dataLen=%d",
				seq, xorByte, len(data))
		}
	}

	// The entity slots past the player's, round robin from where the last
	// tick left off
	round := entityRound{start: conn.StateUpdateOffset(), n: conn.EntitySlots() - 1}
	npcCount, activeCount, sentNPC := r.queueEntityUpdates(sess, &round)
	count += npcCount

	// Advance round-robin offset so next tick starts from a different channel
	conn.SetStateUpdateOffset((round.start + MaxStateUpdatesPerTick) % round.n)

	// Diagnostic summary: active channels vs sent channels
	if debuglog.Packets() {
		log.Printf("GenerateStateUpdates: SUMMARY sent=%d player=%v active=%d npcSent=%v",
			count, playerSent, activeCount, sentNPC)
	}

	// Generate stat update (channel 0x42) using XOR delta encoding
	statUpdate := conn.GetClientStatUpdate()
	if data, seq, xorByte := statUpdate.GenerateXORDelta(); data != nil {
		msg := protocol.GetMessage()
		msg.Type = protocol.MessageTypeStatUpdate
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
		if debuglog.Packets() {
			log.Printf("GenerateStateUpdates: stat update, seq=%d, xorByte=%d, dataLen=%d",
				seq, xorByte, len(data))
		}
	}

	// Generate buff update (channel 0x43) using XOR delta encoding
	buffUpdate := conn.GetClientBuffUpdate()
	if data, seq, xorByte := buffUpdate.GenerateXORDelta(); data != nil {
		msg := protocol.GetMessage()
		msg.Type = protocol.MessageTypeBuffUpdate
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
	}

	// Generate group update (channel 0x40) using XOR delta encoding
	groupUpdate := conn.GetServerGroupUpdate()
	if data, seq, xorByte := groupUpdate.GenerateXORDelta(); data != nil {
		msg := protocol.GetMessage()
		msg.Type = protocol.MessageTypeGroupUpdate
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
		if debuglog.Packets() {
			log.Printf("GenerateStateUpdates: group update, seq=%d, xorByte=%d, dataLen=%d",
				seq, xorByte, len(data))
		}
	}
	return round
}

// queueEntityUpdates queues the messages of up to MaxStateUpdatesPerTick entity
// channels, visiting them on round's way round (each at most once a tick),
// each on its slot's state channel (session.EntityChannel). It returns how
// many it queued, how many of the channels it visited were active, and the
// channels it queued.
func (r *RDPHandler) queueEntityUpdates(sess *session.Session, round *entityRound) (queued, active int, sent []int) {
	conn := sess.Connection()
	for ; round.visited < round.n && queued < MaxStateUpdatesPerTick; round.visited++ {
		slot := 1 + (round.start+round.visited)%round.n
		state := conn.GetServerObject(slot)
		if state == nil {
			continue
		}
		if state.IsActive() {
			active++
		}
		if data, seq, xorByte := state.GenerateXORDelta(); data != nil {
			ch := session.EntityChannel(slot)
			msg := protocol.GetMessage()
			msg.Type = protocol.MessageType(ch)
			msg.Sequence = seq
			msg.XORByte = xorByte
			msg.SetData(data)
			sess.Queue().Add(msg)
			queued++
			if debuglog.Packets() {
				sent = append(sent, int(ch)) // for the SUMMARY line alone
				// Log first 16 bytes of delta for rendered-vs-not comparison
				hexLen := min(16, len(data))
				log.Printf("CHANNEL_DELTA: ch=%d seq=%d xorByte=%d dataLen=%d first16=%x",
					ch, seq, xorByte, len(data), data[:hexLen])
			}
		}
	}
	return queued, active, sent
}

// BuildPacketData builds raw packet data from components.
func BuildPacketData(serverID, clientID uint16, instanceID uint32, hasInstance, didServerInitiate bool, bodyData []byte) []byte {
	// Calculate total size
	headerSize := 4 // endpoints
	headerSize += protocol.Size7BitEncoded(uint64(len(bodyData)))
	if hasInstance {
		headerSize += 4
	}
	if didServerInitiate {
		headerSize += protocol.Size7BitEncoded(uint64(instanceID))
	}

	totalSize := headerSize + len(bodyData) + 4 // +4 for CRC

	buf := make([]byte, totalSize)
	writer := buffer.NewWriter(buf)

	// Write endpoints
	writer.WriteUint16(serverID)
	writer.WriteUint16(clientID)

	// Build flags
	var flags uint64
	if hasInstance {
		flags |= uint64(protocol.SegmentFlagNewInstance)
		flags |= uint64(protocol.SegmentFlagHasInstance)
	}
	if didServerInitiate {
		flags |= uint64(protocol.SegmentFlagDidServerInitiate)
	} else {
		flags |= uint64(protocol.SegmentFlagClientInitiated)
	}

	// Combine flags with body size
	flagsLen := flags | uint64(len(bodyData))
	writer.Write7BitEncodedUInt64(flagsLen)

	// Write instance if present
	if hasInstance {
		writer.WriteUint32(instanceID)
	}

	// Write session ID if server initiated
	if didServerInitiate {
		writer.Write7BitEncodedUInt64(uint64(instanceID))
	}

	// Write body data
	writer.WriteBytes(bodyData)

	// Add CRC
	return protocol.AppendCRC(writer.Bytes())
}

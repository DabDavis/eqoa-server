package udp

// keepbeat.go — the hold of a restart that keeps the sessions is never silent
// (2026-11-16: the freeze and takeovers after build306's restart).
//
// CLIENT ends a connection whose oldest unacknowledged reliable message is
// older than its warn time AND from which nothing has arrived for as long
// (connection.cpp, connection_flush: `iTimeout < uiAge && iTimeout <
// uiSilence`, reason 2; viclient.cpp 1707 sets the warn time to 30 s, and
// vanilla's drdp is byte-exact with it). Any segment the client takes counts as
// an arrival (connection_record_segment sets time_last_arrival), an ack-only
// one too. Until this file, nothing went to a client from the moment the old
// process froze its sends (Snapshot written) to the end of ResumeKept in the new
// one: on eqoaclassic that hold had grown with the player count (the new image
// loads the world ~8 s, then builds the characters one after another, ~0.65 s
// each), 25.6 s with 27 players at 00:36, 33.0 s with 30 at 01:44, 31.6 s with
// 33 at 02:18 and 35.2 s with 34 at 02:27 (2026-11-16). Past 30 s every client
// with a reliable message in flight at the freeze (a target, an attack press, a
// chat line: anyone playing) gave up: the web page, a native client and a PS2
// alike. The server never heard from them again ("last heard" at the resume)
// and their players logged in again (a takeover).
//
// So through the whole hold every kept session is sent a beat every
// keepBeatEvery: a packet with no message, the next packet number and the
// acknowledgements as they stood (a repeat of what the client was last told:
// nothing is acknowledged that was not, nothing taken back). Three stretches,
// one counter:
//
//   - the old process, from the freeze to the exec (the swap's before.sh runs
//     there, up to 20 s): its live sessions are beaten, and the number of
//     rounds goes to the next image in KeepBeatsEnv, since the snapshot was
//     written before them (Snapshot.Beaten adds them);
//   - the new process, from reading the snapshot (main, before the world
//     loads) to ResumeKept: copies of the sessions made from the snapshot are
//     beaten (StartSnapshotBeat), and Stop adds the rounds to it;
//   - ResumeKept itself: every session is put back first and beaten while the
//     characters are built.
//
// A client is then never silent past keepBeatEvery plus a load's stall, and
// CLIENT's other limit, a message unsent or unacknowledged past 60 s (its fail
// time), is the hold's only bound.

import (
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keepBeatEvery is how often a held session is sent a beat: well inside
// CLIENT's 30 s, often enough that a stall of the process between two (a
// collection, the world's load) leaves the client a wide margin.
var keepBeatEvery = 2 * time.Second

// KeepBeatsEnv carries to the next image how many beats the old one sent each
// kept session after writing the snapshot.
const KeepBeatsEnv = "EQOA_KEEP_BEATS"

// beatPacket is an ack-only packet for sess: its endpoints and segment header as
// PreparePacket writes them, the next packet number and the acknowledgements as
// the connection holds them, and no message. It takes a packet number, under
// the session's send lock (a PreparePacket at the same time numbers after or
// before it, never the same). Nil for a session that would need a new
// instance's flag: that packet must be PreparePacket's.
func beatPacket(sess *session.Session) []byte {
	sess.LockSend()
	defer sess.UnlockSend()
	if sess.NeedsNewInstance() {
		return nil
	}
	conn := sess.Connection()
	builder := NewPacketBuilder()
	builder.WriteEndpoints(conn.ServerID(), conn.ClientID())
	hasInstance := sess.HasInstance()
	didServerInitiate := sess.DidServerInitiate()
	if didServerInitiate {
		hasInstance = true
	}
	segFlags := uint64(0)
	if didServerInitiate {
		segFlags |= uint64(protocol.SegmentFlagDidServerInitiate)
	} else {
		segFlags |= uint64(protocol.SegmentFlagClientInitiated)
	}
	if hasInstance {
		segFlags |= uint64(protocol.SegmentFlagHasInstance)
	}
	segHeaderPos := builder.Position()
	header := &protocol.SegmentHeader{Size: 0x7FF, Flags: protocol.SegmentHeaderFlags(segFlags), Instance: sess.InstanceID()}
	builder.WriteSegmentHeader(header, hasInstance, didServerInitiate)
	if hasInstance {
		builder.WriteInstance(sess.InstanceID())
	}
	if didServerInitiate {
		builder.WriteSessionID(sess.PlayerObjectID())
	}
	bodyStart := builder.Position()
	report := conn.NextPacketReport()
	flags := protocol.BodyFlagRdpMessage | protocol.BodyFlagSessionAck | protocol.BodyFlagGuaranteedAck | protocol.BodyFlagFlushAck
	if report.Mask != 0 && sess.AckMasks() {
		flags |= protocol.BodyFlagGuaranteedAckMask
	}
	builder.WriteBodyFlags(flags)
	builder.WriteSessionAck(sess.InstanceID())
	builder.WriteRDPReport(report.Packet, report.FlushAck, report.Ack, true)
	if flags&protocol.BodyFlagGuaranteedAckMask != 0 {
		builder.WriteAckMask(report.Mask)
	}
	builder.PatchSegmentHeaderSize(segHeaderPos, segFlags, builder.Position()-bodyStart)
	packet := builder.Finish()
	conn.Net().NoteSent(report.Packet, len(packet), time.Now())
	return packet
}

// keepBeat beats a fixed set of sessions on a socket until stopped. Every
// round beats every session, so the rounds are each one's count.
type keepBeat struct {
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	rounds int
}

// startKeepBeat beats sessions on conn every keepBeatEvery, the first a beat
// from now (the caller has just sent, or the client has just heard).
func startKeepBeat(conn *net.UDPConn, sessions []*session.Session) *keepBeat {
	b := &keepBeat{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(b.done)
		t := time.NewTicker(keepBeatEvery)
		defer t.Stop()
		for {
			select {
			case <-b.stop:
				return
			case <-t.C:
			}
			for _, sess := range sessions {
				if data := beatPacket(sess); data != nil {
					conn.WriteToUDP(data, sess.RemoteAddr())
				}
			}
			b.rounds++
		}
	}()
	return b
}

// Stop ends the beat, waits for a round under way, and reports how many
// rounds went.
func (b *keepBeat) Stop() int {
	if b == nil {
		return 0
	}
	b.once.Do(func() { close(b.stop) })
	<-b.done
	return b.rounds
}

// SnapshotBeat beats the sessions of a snapshot read by a new image while it
// loads the world, before its listener exists.
type SnapshotBeat struct {
	snap     *Snapshot
	beat     *keepBeat
	stopOnce sync.Once
}

// StartSnapshotBeat beats every session of snap on conn, from copies of them
// made from the snapshot alone (a manager of their own: the real ones are
// ResumeKept's), until Stop. Nil with no socket or snapshot.
func StartSnapshotBeat(conn *net.UDPConn, snap *Snapshot) *SnapshotBeat {
	if conn == nil || snap == nil || len(snap.Sessions) == 0 {
		return nil
	}
	m := session.NewManager()
	var copies []*session.Session
	for _, ks := range snap.Sessions {
		s, err := m.Resume(ks.Session)
		if err != nil {
			continue // ResumeKept says why
		}
		copies = append(copies, s)
	}
	// a beat at once: the old image's last packet went before its exec
	for _, s := range copies {
		if data := beatPacket(s); data != nil {
			conn.WriteToUDP(data, s.RemoteAddr())
		}
	}
	snap.Beaten(1)
	log.Printf("Keep: %d sessions beaten every %v while the world loads", len(copies), keepBeatEvery)
	sb := &SnapshotBeat{snap: snap, beat: startKeepBeat(conn, copies)}
	snap.beat = sb
	return sb
}

// Stop ends the beat and numbers the snapshot's packets past it, so ResumeKept
// goes on from the last beat's number; ResumeKept calls it, and a second call
// does nothing.
func (sb *SnapshotBeat) Stop() {
	if sb == nil {
		return
	}
	sb.stopOnce.Do(func() { sb.snap.Beaten(sb.beat.Stop()) })
}

// Beaten counts n beats sent every session after the snapshot was written:
// each one's next packet number moves past them.
func (s *Snapshot) Beaten(n int) {
	if n <= 0 {
		return
	}
	for i := range s.Sessions {
		s.Sessions[i].Session.Conn.TxPacket += uint16(n)
	}
}

// InheritedBeats is how many beats the last image sent each kept session after
// writing its snapshot (KeepBeatsEnv), 0 when none; the variable is cleared.
func InheritedBeats() int {
	v := os.Getenv(KeepBeatsEnv)
	os.Unsetenv(KeepBeatsEnv)
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

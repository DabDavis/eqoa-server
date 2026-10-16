package session

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// MaxFragmentSize is the maximum data size for a single reliable message.
// Messages larger than this are split into 0xFA (SegmentReliable) fragments.
// Matches Java eqoa.live: Connection.sendMessage() fragments at 1156 bytes.
const MaxFragmentSize = 1156

// MessageQueue manages outbound messages for a session.
type MessageQueue struct {
	mu sync.Mutex

	session *Session

	// Pending messages to send
	pending []*protocol.Message

	// Reliable messages awaiting acknowledgment
	reliable []*protocol.Message

	// Body flags for next packet
	bodyFlags protocol.SegmentBodyFlags
}

// NewMessageQueue creates a new message queue.
func NewMessageQueue(session *Session) *MessageQueue {
	return &MessageQueue{
		session:  session,
		pending:  make([]*protocol.Message, 0, 16),
		reliable: make([]*protocol.Message, 0, 16),
	}
}

// Add adds a message to the queue.
func (q *MessageQueue) Add(msg *protocol.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.pending = append(q.pending, msg)
}

// AddWithSequence adds a reliable message with sequence number.
// If the message data exceeds MaxFragmentSize, it is split into 0xFA (SegmentReliable)
// fragments followed by a final 0xFB (Reliable) message, matching Java eqoa.live behavior.
func (q *MessageQueue) AddWithSequence(msg *protocol.Message) uint16 {
	q.mu.Lock()
	defer q.mu.Unlock()

	data := msg.Data()

	// Fragment large reliable messages (Java: Connection.sendMessage)
	if msg.Type == protocol.MessageTypeReliable && len(data) > MaxFragmentSize {
		log.Printf("Fragmenting message opcode=0x%04X size=%d into %d fragments",
			msg.Opcode, len(data), (len(data)+MaxFragmentSize-1)/MaxFragmentSize)

		offset := 0
		var lastSeq uint16

		// Send full-size fragments as 0xFA (SegmentReliable)
		for offset+MaxFragmentSize < len(data) {
			frag := &protocol.Message{
				Type:   protocol.MessageTypeSegmentReliable,
				Opcode: msg.Opcode,
			}
			frag.SetData(data[offset : offset+MaxFragmentSize])

			seq := q.session.Connection().NextMessageSeq()
			frag.Sequence = seq
			frag.UpdateTime()

			q.pending = append(q.pending, frag)
			q.reliable = append(q.reliable, frag)
			lastSeq = seq

			offset += MaxFragmentSize
		}

		// Final fragment stays as 0xFB (Reliable) with remaining data
		msg.SetData(data[offset:])
		seq := q.session.Connection().NextMessageSeq()
		msg.Sequence = seq
		msg.UpdateTime()

		q.pending = append(q.pending, msg)
		q.reliable = append(q.reliable, msg)
		lastSeq = seq

		log.Printf("Fragmented: %d fragments, last seq=%d", (offset/MaxFragmentSize)+1, lastSeq)
		return lastSeq
	}

	// Normal (non-fragmented) path
	seq := q.session.Connection().NextMessageSeq()
	msg.Sequence = seq
	msg.UpdateTime()

	q.pending = append(q.pending, msg)

	// Track reliable messages for retransmission
	if msg.Type.IsReliable() {
		q.reliable = append(q.reliable, msg)
	}

	return seq
}

// Requeue re-adds a message to the front of the pending queue.
// Used when a reliable message didn't fit in the current packet.
func (q *MessageQueue) Requeue(msg *protocol.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Prepend to front so it's sent first next tick
	q.pending = append([]*protocol.Message{msg}, q.pending...)
}

// RequeueBatch re-adds multiple messages to the front of the pending queue,
// preserving their original order.
func (q *MessageQueue) RequeueBatch(msgs []*protocol.Message) {
	if len(msgs) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(msgs, q.pending...)
}

// HasPending returns whether there are pending messages.
func (q *MessageQueue) HasPending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending) > 0
}

// CheckQueue returns whether there are messages to send.
func (q *MessageQueue) CheckQueue() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending) > 0 || q.bodyFlags != 0
}

// GetPending returns and clears pending messages.
func (q *MessageQueue) GetPending() []*protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return nil
	}

	msgs := q.pending
	q.pending = make([]*protocol.Message, 0, 16)
	return msgs
}

// PeekPending returns pending messages without removing them.
func (q *MessageQueue) PeekPending() []*protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return nil
	}

	// Return a copy to avoid holding the lock
	result := make([]*protocol.Message, len(q.pending))
	copy(result, q.pending)
	return result
}

// SetBodyFlags sets the body flags for the next packet.
func (q *MessageQueue) SetBodyFlags(flags protocol.SegmentBodyFlags) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bodyFlags |= flags
}

// ClearBodyFlags clears and returns the body flags.
func (q *MessageQueue) ClearBodyFlags() protocol.SegmentBodyFlags {
	q.mu.Lock()
	defer q.mu.Unlock()
	flags := q.bodyFlags
	q.bodyFlags = 0
	return flags
}

// GetBodyFlags returns the current body flags.
func (q *MessageQueue) GetBodyFlags() protocol.SegmentBodyFlags {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.bodyFlags
}

// RemoveReliables removes acknowledged reliable messages.
func (q *MessageQueue) RemoveReliables() {
	q.mu.Lock()
	defer q.mu.Unlock()

	clientAck := q.session.Connection().ClientLastReceivedMessage()

	// Remove messages that have been acknowledged
	newReliable := q.reliable[:0]
	for _, msg := range q.reliable {
		if msg.Sequence > clientAck {
			newReliable = append(newReliable, msg)
		} else {
			protocol.PutMessage(msg)
		}
	}
	q.reliable = newReliable
}

// HasRetransmits reports whether any unacknowledged reliable message has gone
// unanswered for longer than delay. Non-mutating, so it is safe to use as a
// "do we need to send a packet at all" check.
func (q *MessageQueue) HasRetransmits(delay time.Duration) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.reliable) == 0 {
		return false
	}
	inPending := q.pendingSeqs()
	now := time.Now()
	for _, msg := range q.reliable {
		if inPending[msg.Sequence] {
			continue
		}
		if now.Sub(msg.LastSent) >= delay {
			return true
		}
	}
	return false
}

// TakeRetransmits returns up to max unacknowledged reliable messages that have
// gone unanswered for longer than delay, marking each as sent again.
//
// Without this the server sent every reliable message exactly once. A single
// lost packet stalled the connection permanently: the PS2 dropped one
// MemoryDump fragment during world entry, its guaranteedAck froze at that
// sequence, and the server cheerfully kept sending later fragments that the
// client could never assemble. The Go client never showed it because a
// loopback UDP socket does not drop.
func (q *MessageQueue) TakeRetransmits(delay time.Duration, max int) []*protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.reliable) == 0 || max <= 0 {
		return nil
	}
	inPending := q.pendingSeqs()
	now := time.Now()

	var out []*protocol.Message
	for _, msg := range q.reliable {
		if len(out) >= max {
			break
		}
		// Still queued for its first send — not a retransmit.
		if inPending[msg.Sequence] {
			continue
		}
		if now.Sub(msg.LastSent) < delay {
			continue
		}
		msg.LastSent = now
		out = append(out, msg)
	}
	if len(out) > 0 {
		log.Printf("Queue: retransmitting %d unacked reliable message(s), first seq=%d",
			len(out), out[0].Sequence)
	}
	return out
}

// pendingSeqs is the set of sequence numbers still awaiting their first send.
// Caller must hold q.mu.
func (q *MessageQueue) pendingSeqs() map[uint16]bool {
	set := make(map[uint16]bool, len(q.pending))
	for _, msg := range q.pending {
		set[msg.Sequence] = true
	}
	return set
}

// GetUnacknowledged returns reliable messages that need retransmission.
func (q *MessageQueue) GetUnacknowledged() []*protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.reliable) == 0 {
		return nil
	}

	// Return a copy to avoid holding the lock
	result := make([]*protocol.Message, len(q.reliable))
	copy(result, q.reliable)
	return result
}

// Clear removes all messages from the queue.
func (q *MessageQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Return messages to pool
	for _, msg := range q.pending {
		protocol.PutMessage(msg)
	}
	for _, msg := range q.reliable {
		protocol.PutMessage(msg)
	}

	q.pending = q.pending[:0]
	q.reliable = q.reliable[:0]
	q.bodyFlags = 0
}

// PendingCount returns the number of pending messages.
func (q *MessageQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// ReliableCount returns the number of unacknowledged reliable messages.
func (q *MessageQueue) ReliableCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.reliable)
}

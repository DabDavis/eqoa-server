package session

import (
	"bytes"
	"sync"
)

// MaxServerObjects is the number of object state channels, 0x00-0x17: channel 0
// is the player, 1-23 the entities in view. CLIENT reads an object channel only
// below 24 (VIClient::ProcessStateChannel, 0x00BD3C18: `type < 24` goes to
// ChannelEntity, which keeps m_entities[24]); 0x18 and up reach nothing, so an
// entity put there is invisible to a PS2.
const MaxServerObjects = 24

// entityStateSize and statUpdateSize are the wire sizes of the object channels
// (0x00-0x1C) and the stat channel (0x42), measured off pcaps/ps2_sandstorm.pcap.
//
// These duplicate entity.EntityStateSize / entity.StatUpdateSize because
// internal/entity imports internal/session, so the dependency cannot go the
// other way. TestChannelSizesMatchSerializers pins them together.
const (
	entityStateSize = 162
	statUpdateSize  = 233
)

// ConnectionData tracks sequence numbers and acknowledgments.
type ConnectionData struct {
	mu sync.Mutex

	session *Session

	// Endpoint IDs
	clientID uint16
	serverID uint16

	// Sequence tracking - incoming
	lastReceivedPacketSeq  uint16
	lastReceivedMessageSeq uint16

	// Sequence tracking - outgoing
	lastSentPacketSeq  uint16
	lastSentMessageSeq uint16

	// Client acknowledgment tracking
	clientLastReceivedMessage uint16

	// State channel data
	serverObjects     [MaxServerObjects]*StateUpdateData // Channels 0x00-0x17 (player + up to 23 entities)
	clientStatUpdate  *StateUpdateData                   // Channel 0x42
	clientBuffUpdate  *StateUpdateData                   // Channel 0x43
	serverGroupUpdate *StateUpdateData                   // Channel 0x40
	client            *ClientStateData                   // Client input state

	// Round-robin offset for state update generation (ensures all channels get served)
	stateUpdateOffset int
}

// RefnumOffsetMax is how far back a state message may name its reference:
// connection.cpp's REFNUM_OFFSET_MAX. CLIENT refuses the WHOLE segment that
// carries a state message with a larger refnum, or one whose reference it no
// longer holds, so nothing in that segment arrives and nothing of it is acked.
const RefnumOffsetMax = 32

// StateUpdateData tracks state channel updates with XOR delta encoding, the
// send half of connection.cpp's state channels (connection_append_state,
// connection_record_state_acks).
//
// Until 2026-10-24 it kept C# ServerObjectUpdate.cs's model: one base and the
// XOR deltas sent since, all thrown away at each ack. An ack for a message
// already in flight then found nothing and was ignored, the base lagged, and
// the refnum (seq minus the base) grew without bound and was written as a
// byte. After a client stall (a hidden browser tab, a lag spike) it passed 32,
// so the client refused every segment carrying the channel and acked nothing;
// at 256 it wrapped to 0 and the client took a delta for a whole value. On the
// player's own channel that value's flag, id and type XOR to zero, a removal:
// F3 20261024-001134's player lost its own entity (no health gauge, no
// compass) and kept it lost, every later delta landing on the bad copy.
type StateUpdateData struct {
	mu sync.Mutex

	// Current entity state data (what we want to send)
	data []byte

	// The value the client is known to hold: the newest acknowledged one,
	// zeros before any acknowledgement, nil when the acknowledged message had
	// already left `sent`. Whether data differs from it decides a resend.
	baseXOR []byte

	// Message counter for this channel (the next sequence number)
	messageCounter uint16

	// sent is m_txList: every value sent under a new sequence number within
	// RefnumOffsetMax of the newest, oldest first, with nothing older than
	// the newest acknowledgement. A message is XORed against the head when the
	// head is that acknowledged message (ackSeq, acked), and goes out whole
	// with refnum 0 otherwise.
	sent   []sentValue
	ackSeq uint16
	acked  bool

	// Is this channel active?
	isActive bool

	// Does the data need to be sent?
	dirty bool

	// Tick counter for throttling resends of unchanged data.
	// When data differs from baseXOR (client hasn't ACK'd) but dirty is false,
	// we only resend every ResendInterval ticks to avoid flooding.
	resendTicks uint8

	// lastSentSeq is the sequence number the current data first went out
	// with, and sentOnce whether anything has. A retransmission reuses it
	// (connection_append_state numbers a state message once, on its first
	// send), so the client, which drops a sequence at or below the channel's
	// rx, applies each change exactly once.
	lastSentSeq uint16
	sentOnce    bool
}

// sentValue is one message of StateUpdateData.sent: its sequence number and
// the value it carried.
type sentValue struct {
	seq  uint16
	data []byte
}

// seqBefore reports whether sequence number a comes before b, the way
// connection.cpp compares them (the signed 16-bit difference), so a channel
// may pass 65535.
func seqBefore(a, b uint16) bool {
	return int16(a-b) < 0
}

// ClientStateData tracks client input state.
type ClientStateData struct {
	mu     sync.Mutex
	SeqNum uint16 // the newest message taken, the one acknowledged
	took   bool
	data   []byte

	// XOR base messages for delta reconstruction.
	// The client sends position as XOR deltas against previous messages.
	// When XORByte != 0, we XOR against the base at (Sequence - XORByte).
	// C# stores up to 32 base messages.
	bases map[uint16][]byte
}

// NewConnectionData creates new connection data.
func NewConnectionData(session *Session, clientID, serverID uint16) *ConnectionData {
	cd := &ConnectionData{
		session:  session,
		clientID: clientID,
		serverID: serverID,
	}

	// Initialize state channels (0x00-0x16).
	//
	// THE ALLOCATION IS THE WIRE SIZE. SetData only copy()s into this buffer, so
	// it can never shrink it — a serializer that returns fewer bytes just leaves
	// the tail zeroed, and RunLengthEncode faithfully encodes those trailing
	// zeros (a 0x80 0xNN pair) rather than stopping at the last non-zero byte.
	// The client therefore writes every one of them into its entity struct.
	//
	// Measured, not assumed: every object channel in pcaps/ps2_sandstorm.pcap
	// decodes to exactly 162 bytes — all 24 occupied channels, every update in
	// the capture, never 201. 162 is where our own layout ends too (the "tsrq"
	// signature occupies 158-161).
	//
	// This was 201, which overran the client's entity struct by 39 bytes on
	// every channel every tick. Zeroing the tail in SerializePlayerUpdate did
	// NOT fix it: the bytes were still allocated, still encoded, still sent.
	for i := range cd.serverObjects {
		cd.serverObjects[i] = NewStateUpdateData(entityStateSize)
	}
	// Stat update channel (0x42). Retail decodes to 233 on every capture,
	// ending with the 0xFF at offset 232. Was 0xEC (236) — the same bug as
	// above, and it likewise survived SerializeStatUpdate being corrected to
	// 233, because that only shortened the copy, not the buffer.
	cd.clientStatUpdate = NewStateUpdateData(statUpdateSize)
	// Active effects channel (0x43): as long as its count, 4 bytes and 132 an
	// effect (entity.SerializeBuffUpdate), so it starts at 4, no effect, and
	// SetSizedData moves its length.
	cd.clientBuffUpdate = NewStateUpdateData(4)
	// Group update channel (0x40) - 0x27 bytes
	cd.serverGroupUpdate = NewStateUpdateData(0x27)
	// Client input state
	cd.client = &ClientStateData{
		data:  make([]byte, 256),
		bases: make(map[uint16][]byte),
	}

	return cd
}

// ClientID returns the client endpoint ID.
func (cd *ConnectionData) ClientID() uint16 {
	return cd.clientID
}

// ServerID returns the server endpoint ID.
func (cd *ConnectionData) ServerID() uint16 {
	return cd.serverID
}

// LastReceivedPacketSeq returns the last received packet sequence.
func (cd *ConnectionData) LastReceivedPacketSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.lastReceivedPacketSeq
}

// SetLastReceivedPacketSeq sets the last received packet sequence.
func (cd *ConnectionData) SetLastReceivedPacketSeq(seq uint16) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastReceivedPacketSeq = seq
}

// LastReceivedMessageSeq returns the last received message sequence.
func (cd *ConnectionData) LastReceivedMessageSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.lastReceivedMessageSeq
}

// IncrementReceivedMessageSeq increments the received message sequence.
func (cd *ConnectionData) IncrementReceivedMessageSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastReceivedMessageSeq++
	return cd.lastReceivedMessageSeq
}

// SetLastReceivedMessageSeq sets the last received message sequence.
func (cd *ConnectionData) SetLastReceivedMessageSeq(seq uint16) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if seq > cd.lastReceivedMessageSeq {
		cd.lastReceivedMessageSeq = seq
	}
}

// LastSentPacketSeq increments and returns the sent packet sequence.
// Sequences start at 1, not 0 (matching eqoa.live behavior).
func (cd *ConnectionData) LastSentPacketSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastSentPacketSeq++
	return cd.lastSentPacketSeq
}

// NextMessageSeq returns and increments the sent message sequence.
func (cd *ConnectionData) NextMessageSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastSentMessageSeq++
	return cd.lastSentMessageSeq
}

// CurrentMessageSeq returns the current sent message sequence.
func (cd *ConnectionData) CurrentMessageSeq() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.lastSentMessageSeq
}

// ClientLastReceivedMessage returns the last message the client acknowledged.
func (cd *ConnectionData) ClientLastReceivedMessage() uint16 {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.clientLastReceivedMessage
}

// SetClientLastReceivedMessage sets the last message the client acknowledged.
func (cd *ConnectionData) SetClientLastReceivedMessage(seq uint16) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if seq > cd.clientLastReceivedMessage {
		cd.clientLastReceivedMessage = seq
	}
}

// ResetForMasterSession resets all sequence counters for master session transition.
// Both incoming and outgoing counters must be reset since the master session is
// effectively a new connection — the client starts its sequences from 0/1 again.
func (cd *ConnectionData) ResetForMasterSession() {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.lastSentPacketSeq = 0
	cd.lastSentMessageSeq = 0
	cd.clientLastReceivedMessage = 0
	cd.lastReceivedPacketSeq = 0
	cd.lastReceivedMessageSeq = 0
}

// ProcessPacket processes an incoming packet's sequence data.
func (cd *ConnectionData) ProcessPacket(packet interface{}) {
	// This will be expanded when integrating with the packet parser
}

// UpdateStateChannel updates a state channel with new data.
func (cd *ConnectionData) UpdateStateChannel(channel byte, data []byte) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if int(channel) < len(cd.serverObjects) {
		state := cd.serverObjects[channel]
		state.mu.Lock()
		copy(state.data, data)
		state.dirty = true
		state.mu.Unlock()
	}
}

// GetServerObject returns a server object state channel.
func (cd *ConnectionData) GetServerObject(index int) *StateUpdateData {
	if index >= 0 && index < len(cd.serverObjects) {
		return cd.serverObjects[index]
	}
	return nil
}

// GetServerObjects returns the server object state channels slice.
func (cd *ConnectionData) GetServerObjects() []*StateUpdateData {
	return cd.serverObjects[:]
}

// GetClientStatUpdate returns the client stat update state.
func (cd *ConnectionData) GetClientStatUpdate() *StateUpdateData {
	return cd.clientStatUpdate
}

// GetClientBuffUpdate returns the client buff update state (channel 0x43).
func (cd *ConnectionData) GetClientBuffUpdate() *StateUpdateData {
	return cd.clientBuffUpdate
}

// GetServerGroupUpdate returns the server group update state.
func (cd *ConnectionData) GetServerGroupUpdate() *StateUpdateData {
	return cd.serverGroupUpdate
}

// GetClientState returns the client input state.
func (cd *ConnectionData) GetClientState() *ClientStateData {
	return cd.client
}

// ResendAll re-describes every active state channel to the client, and returns
// how many were re-armed.
//
// This is the answer to a world change, and it is what retail does. A state
// channel is only sent when its data CHANGES, which is right while the client
// keeps its scene and wrong the moment it throws it away: the player arrives in
// a new world, the server has nothing new to say, and the client is left with a
// loaded world containing no player and no NPCs — with the camera parked
// wherever the empty player object sits. That is the bug this exists for.
//
// It marks dirty rather than zeroing baseXOR, because the client's XOR bases
// SURVIVE a world change — proved by pcaps/sandstorm-worldchange-20260807.pcap,
// where every one of the 24 channels retail floods after the load carries
// ref=1, a delta against the message it sent BEFORE the teleport. So an
// ordinary delta is all that is needed, and it is all the client will accept
// without being told its base changed (see ForceFullResend).
// An object channel that describes no entity is skipped, and that is not an
// optimisation — it is the difference between a world change and a client
// assert. Byte 0 of an object channel is the flag, bytes 1-4 the id and byte 5
// the ENTITY TYPE, and CLIENT's ChannelEntity (0x00BD41B8) builds an entity on
// an EMPTY slot whatever the flag, through a factory (CLIENT VIEntity.cpp:476)
// that accepts only 0x81 or 0x82:
//
//	type = a0 & 0xFF
//	if      type == 0x81 -> alloc 0x58,  construct
//	else if type == 0x82 -> alloc 0x2C0, construct + VIWalkControl::Init
//	else                 -> pointer stays NULL
//	if (!pointer) assert("pEntity != NULL", VIEntity.cpp, 476)
//
// Type 0x00 falls off the end and asserts. Measured on the live console
// 2026-08-10 warping to world 9: the client parked in the assert loop with that
// exact string, on a stack rooted in the world-change path — and `callonhit`
// running the guest's own malloc AT the assert returned a 1 MB block, so it was
// never the heap. Re-arming the cleared channels re-delivered 28 type-0 buffers
// at the exact moment the client was rebuilding its scene.
//
// A removal now goes out once as the entity's own id and type with the flag
// cleared (StateUpdateData.MarkRemoved), and a resend keeps its sequence
// number, so the client deletes the entity exactly once. Re-offering that
// buffer here would rebuild the entity on the now-empty slot, hence the skip.
func (cd *ConnectionData) ResendAll() int {
	n := 0
	for _, state := range cd.serverObjects {
		if state != nil && !state.DescribesNoEntity() && state.MarkDirtyIfActive() {
			n++
		}
	}
	for _, state := range []*StateUpdateData{cd.clientStatUpdate, cd.clientBuffUpdate, cd.serverGroupUpdate} {
		if state != nil && state.MarkDirtyIfActive() {
			n++
		}
	}
	return n
}

// StateUpdateOffset returns the round-robin offset for state channel updates.
func (cd *ConnectionData) StateUpdateOffset() int {
	return cd.stateUpdateOffset
}

// SetStateUpdateOffset sets the round-robin offset for state channel updates.
func (cd *ConnectionData) SetStateUpdateOffset(offset int) {
	cd.stateUpdateOffset = offset
}

// StateUpdateData methods

// NewStateUpdateData creates a new state update data with the given size.
func NewStateUpdateData(size int) *StateUpdateData {
	return &StateUpdateData{
		data:           make([]byte, size),
		baseXOR:        make([]byte, size),
		messageCounter: 1, // Start at 1 like C#
	}
}

// SetActive sets whether this channel is active.
func (s *StateUpdateData) SetActive(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isActive = active
}

// IsActive returns whether this channel is active.
func (s *StateUpdateData) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isActive
}

// UpdateBaseXor takes the client's acknowledgement of message msgCounter on
// this channel, as connection_record_state_acks does: only one newer than the
// last moves anything; it becomes the reference, and what was sent before it
// is dropped. Any message still held may be acknowledged, so a burst of acks
// for messages that were in flight together moves the reference to the newest
// (the old model ignored all but the first). The value the client holds is
// the one that message carried, at its own length (connection.cpp's receive
// keeps a state message's own size; the effects channel's changes).
func (s *StateUpdateData) UpdateBaseXor(msgCounter uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.sentOnce || seqBefore(s.lastSentSeq, msgCounter) {
		return // never sent: not the client's to acknowledge
	}
	if s.acked && !seqBefore(s.ackSeq, msgCounter) {
		return // this one or a newer one is acknowledged already
	}
	s.ackSeq, s.acked = msgCounter, true
	for len(s.sent) > 0 && seqBefore(s.sent[0].seq, msgCounter) {
		s.sent = s.sent[1:]
	}
	if len(s.sent) > 0 && s.sent[0].seq == msgCounter {
		s.baseXOR = append([]byte(nil), s.sent[0].data...)
	} else {
		// sent too long ago to be held: its value is unknown here, so the
		// channel counts as unsynced and goes out whole until a newer ack
		s.baseXOR = nil
	}

	// Reset the resend throttle
	s.resendTicks = 0
}

// ForceFullResend makes the next GenerateXORDelta emit the ENTIRE state as a
// KEYFRAME — refnum 0, no XOR reference — as though the client had never seen
// this channel. It is the recovery for a client whose base history is gone, not
// the routine answer to a world change; for that use ConnectionData.ResendAll,
// which is what retail sends.
//
// Forgetting the acknowledgement is the whole point. Zeroing the base alone
// made the SERVER emit the full state under a NON-ZERO refnum, which tells the
// client to XOR that full state against the message it still holds. The client
// computes `its_state XOR full_state` and gets garbage. Thirty channels of
// garbage at a client that is mid-load is how a loading screen becomes
// permanent — observed 2026-08-06, and misread at the time as "a full resend
// cannot work". It can; it has to say refnum 0 while doing it.
func (s *StateUpdateData) ForceFullResend() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.baseXOR = make([]byte, len(s.data))
	s.sent = nil
	s.acked = false
	s.dirty = true
	s.resendTicks = 0
}

// DescribesNoEntity reports whether an object channel holds no live entity:
// it is empty, or its flag byte (byte 0) is 0 — never armed, or cleared by
// MarkRemoved. Re-offering such a buffer would make the client build an
// entity out of it (ChannelEntity creates on an empty slot whatever the
// flag), so ConnectionData.ResendAll skips it.
func (s *StateUpdateData) DescribesNoEntity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data) == 0 || s.data[0] == 0
}

// MarkRemoved tells the client the entity on this object channel is gone, the
// way Sandstorm does and CLIENT's ChannelEntity (0x00BD41B8) reads it: the
// same id and type with the flag byte (byte 0) cleared. A slot whose entity
// has that id and type is then deleted and NOT replaced. The rest of the last
// state stays as it was. An all-zero buffer, the old way, carries type 0,
// which CLIENT's entity factory refuses with an assert and a NULL write
// whenever it lands on a slot that is already empty. A channel that never held
// an entity (flag already 0) sends nothing.
func (s *StateUpdateData) MarkRemoved() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) == 0 || s.data[0] == 0 {
		return
	}
	s.data[0] = 0
	s.dirty = true
	s.isActive = true
	s.resendTicks = 0
}

// MarkDirtyIfActive queues an armed channel for re-transmission and reports
// whether it was armed. An inactive channel has never been described to the
// client and must stay silent — sending one would create an entity out of a
// zeroed buffer.
func (s *StateUpdateData) MarkDirtyIfActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isActive {
		return false
	}
	s.dirty = true
	s.resendTicks = 0
	return true
}

// GetSeqNum returns the current message counter.
func (s *StateUpdateData) GetSeqNum() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageCounter
}

// IsDirty returns whether the state needs to be sent.
func (s *StateUpdateData) IsDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirty
}

// ClearDirty clears the dirty flag.
func (s *StateUpdateData) ClearDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = false
}

// GetData returns a copy of the current state data.
func (s *StateUpdateData) GetData() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result
}

// SetData sets the entity state data.
// The buffer is fixed at allocation time; this only copies into it and cannot resize it.
func (s *StateUpdateData) SetData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if data actually changed
	changed := false
	maxLen := len(s.data)
	if len(data) < maxLen {
		maxLen = len(data)
	}
	for i := 0; i < maxLen; i++ {
		if s.data[i] != data[i] {
			changed = true
			break
		}
	}

	if changed {
		copy(s.data, data)
		s.dirty = true
		s.isActive = true
		s.resendTicks = 0 // Reset throttle so new data is sent immediately
	}
}

// SetSizedData sets the channel's value at data's own length, for the one
// channel whose value is as long as what it holds: the active effects (0x43,
// 4 + 132 bytes an effect). DRDP allows it: connection_append_state XORs a
// state message against its reference over the shorter of the two and sends
// the rest as it is, and the receive keeps each message's own size, so a
// value may grow or shrink by any amount between two messages.
func (s *StateUpdateData) SetSizedData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bytes.Equal(s.data, data) {
		return
	}
	s.data = append([]byte(nil), data...)
	s.dirty = true
	s.isActive = true
	s.resendTicks = 0
}

// ResendInterval is how many round-robin visits to wait before resending
// unchanged data that the client hasn't ACK'd yet. With round-robin visiting
// each channel every ~3 ticks (300ms), 10 visits = ~3 seconds. A resend keeps
// the sequence number of the message it repeats, so it can go on until the
// client acknowledges: the refnum does not grow, and the client applies the
// change once whatever arrives twice.
const ResendInterval = 10

// GenerateXORDelta generates an XOR delta update if the state has changed.
// Returns the XOR'd delta data (to be RLE encoded), the sequence number, and the XOR byte (refnum).
// XORs the current buffer with base, uses refnum for delta reference.
func (s *StateUpdateData) GenerateXORDelta() (data []byte, seq uint16, xorByte byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isActive {
		return nil, 0, 0
	}

	// Check if data differs from base (matches C# ServerObjectUpdate.GenerateUpdate);
	// a value of another length differs (SetSizedData)
	differs := len(s.data) != len(s.baseXOR)
	for i := 0; i < len(s.data) && i < len(s.baseXOR); i++ {
		if s.data[i] != s.baseXOR[i] {
			differs = true
			break
		}
	}

	if !differs && !s.dirty {
		return nil, 0, 0
	}

	// If data hasn't been newly dirtied but differs from base (client hasn't ACK'd),
	// throttle resends to avoid flooding the same data every tick.
	if !s.dirty && differs {
		s.resendTicks++
		if s.resendTicks < ResendInterval {
			return nil, 0, 0
		}
		s.resendTicks = 0
		// A retransmission, not a new message: connection_append_state gives a
		// state message its sequence number once (m_usTx++ on the first send
		// only) and re-encodes a resend against the newest acknowledged copy.
		// A new number per resend made the client apply the same state again,
		// and an entity removal applied twice lands on an empty slot.
		if s.sentOnce {
			return s.retransmitLocked()
		}
	}

	// A new message: numbered once, its value kept in sent for the ack
	seq = s.messageCounter
	s.messageCounter++
	s.sent = append(s.sent, sentValue{seq, append([]byte(nil), s.data...)})
	s.lastSentSeq, s.sentOnce = seq, true
	s.dirty = false
	s.resendTicks = 0

	data, xorByte = s.encodeLocked(seq)
	return data, seq, xorByte
}

// retransmitLocked repeats the last message sent on this channel under the
// sequence number it first had, re-encoded against the newest copy the client
// acknowledged, as connection_append_state does for a state message already
// in flight. The client drops it if it already holds that sequence, and
// re-acknowledges it. Must be called with s.mu held, after a first send; data
// has not changed since (a change dirties the channel: a new message).
func (s *StateUpdateData) retransmitLocked() (data []byte, seq uint16, xorByte byte) {
	seq = s.lastSentSeq
	data, xorByte = s.encodeLocked(seq)
	return data, seq, xorByte
}

// encodeLocked is connection_append_state's encoding of message seq (whose
// value is data): what is more than RefnumOffsetMax behind it leaves sent,
// and when the head of what is left is the client's newest acknowledged
// message, the value goes out XORed against it over the shorter of the two
// (the rest as it is) with the distance back as its refnum; otherwise whole,
// refnum 0. So the refnum is never above RefnumOffsetMax and always names a
// message the client holds: it acked it, and connection.cpp's receive keeps
// every message from a received one's reference on.
func (s *StateUpdateData) encodeLocked(seq uint16) ([]byte, byte) {
	for len(s.sent) > 0 && seqBefore(s.sent[0].seq, seq-RefnumOffsetMax) {
		s.sent = s.sent[1:]
	}
	out := append([]byte(nil), s.data...)
	if !s.acked || len(s.sent) == 0 || s.sent[0].seq != s.ackSeq || s.ackSeq == seq {
		return out, 0
	}
	ref := s.sent[0].data
	for i := 0; i < len(out) && i < len(ref); i++ {
		out[i] ^= ref[i]
	}
	return out, byte(seq - s.ackSeq)
}

// GenerateUpdate generates an update message if dirty (legacy method).
// For backwards compatibility - uses XOR delta internally.
func (s *StateUpdateData) GenerateUpdate() []byte {
	data, _, _ := s.GenerateXORDelta()
	return data
}

// HasData returns true if the channel has active data.
func (s *StateUpdateData) HasData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isActive && len(s.data) > 0
}

// MarkDirty marks the state as needing an update.
func (s *StateUpdateData) MarkDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
}

// AddBase stores a decoded state message as a base for future XOR delta
// reconstruction, and drops those more than RefnumOffsetMax behind it, which
// the client can no longer name. Behind by the sequence numbers' signed
// difference: the old "numerically smallest" eviction, once the client's
// numbers passed 65535, dropped the new small ones and kept the stale large
// ones, so no position update decoded again.
func (c *ClientStateData) AddBase(seq uint16, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	base := make([]byte, len(data))
	copy(base, data)
	c.bases[seq] = base
	for k := range c.bases {
		if seqBefore(k, seq-RefnumOffsetMax) {
			delete(c.bases, k)
		}
	}
}

// Newer reports whether message seq is newer than the last one taken (any
// is, before the first): connection.cpp drops a state message at or below the
// channel's rx as a duplicate and acknowledges rx again.
func (c *ClientStateData) Newer(seq uint16) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.took || seqBefore(c.SeqNum, seq)
}

// Took records message seq as taken: the one acknowledged from now on. Only
// a message that decoded is taken, so the client, which XORs its next message
// against the one acknowledged, never names a base this side lacks.
func (c *ClientStateData) Took(seq uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SeqNum, c.took = seq, true
}

// GetBase retrieves a stored base message by sequence number.
func (c *ClientStateData) GetBase(seq uint16) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bases[seq]
}

// CleanBasesThrough removes all base messages with sequence <= threshold.
func (c *ClientStateData) CleanBasesThrough(threshold uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.bases {
		if k <= threshold {
			delete(c.bases, k)
		}
	}
}

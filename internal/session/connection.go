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

// StateUpdateData tracks state channel updates with XOR delta encoding.
// Implements the same logic as C# ServerObjectUpdate.cs
type StateUpdateData struct {
	mu sync.Mutex

	// Current entity state data (what we want to send)
	data []byte

	// Base state that client has acknowledged - used for XOR delta
	baseXOR []byte

	// Message counter for this channel
	messageCounter uint16

	// The message counter of the last ACK'd message (used to calculate XOR byte)
	baseMessageCounter uint16

	// Pending XOR deltas - stored until client ACKs
	// Maps message counter -> XOR delta data
	pendingDeltas map[uint16][]byte

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

// ClientStateData tracks client input state.
type ClientStateData struct {
	mu     sync.Mutex
	SeqNum uint16
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
		pendingDeltas:  make(map[uint16][]byte),
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

// UpdateBaseXor updates the base state when client ACKs a message.
// This is called when we receive a state channel ACK from the client.
func (s *StateUpdateData) UpdateBaseXor(msgCounter uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the XOR delta for that message
	delta, ok := s.pendingDeltas[msgCounter]
	if !ok || len(delta) == 0 {
		return
	}

	// The base becomes the value the client now holds: the delta XORed onto
	// the old base over the shorter of the two, the rest of the delta as it
	// came, at the delta's length (connection.cpp's receive keeps a state
	// message's own size). For a fixed-size channel that is the old in-place
	// XOR; the effects channel changes length (SetSizedData).
	base := make([]byte, len(delta))
	for i := range delta {
		if i < len(s.baseXOR) {
			base[i] = s.baseXOR[i] ^ delta[i]
		} else {
			base[i] = delta[i]
		}
	}
	s.baseXOR = base

	// Clear pending deltas (client has caught up)
	s.pendingDeltas = make(map[uint16][]byte)

	// Update base message counter
	s.baseMessageCounter = msgCounter

	// Reset the resend throttle
	s.resendTicks = 0
}

// ForceFullResend makes the next GenerateXORDelta emit the ENTIRE state as a
// KEYFRAME — refnum 0, no XOR reference — as though the client had never seen
// this channel. It is the recovery for a client whose base history is gone, not
// the routine answer to a world change; for that use ConnectionData.ResendAll,
// which is what retail sends.
//
// baseMessageCounter is reset here and that reset is the whole point. Zeroing
// baseXOR alone makes the SERVER emit the full state, but leaves the refnum at
// `seq - baseMessageCounter`, i.e. non-zero, which tells the client to XOR that
// full state against the message it still holds. The client computes
// `its_state XOR full_state` and gets garbage. Thirty channels of garbage at a
// client that is mid-load is how a loading screen becomes permanent — observed
// 2026-08-06, and misread at the time as "a full resend cannot work".
// It can; it has to say refnum 0 while doing it.
func (s *StateUpdateData) ForceFullResend() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.baseXOR {
		s.baseXOR[i] = 0
	}
	s.pendingDeltas = make(map[uint16][]byte)
	s.baseMessageCounter = 0
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

// IncrementSeqNum increments and returns the message counter.
func (s *StateUpdateData) IncrementSeqNum() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageCounter++
	return s.messageCounter
}

// GetXORByte returns the XOR reference byte for the current message.
// This is the offset from the base message counter.
func (s *StateUpdateData) GetXORByte() byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseMessageCounter == 0 {
		return 0
	}
	return byte(s.messageCounter - s.baseMessageCounter)
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

	// Generate XOR delta (current data XOR'd with base)
	// For first message, baseXOR is zeros, so delta = data (raw)
	delta := make([]byte, len(s.data))
	for i := 0; i < len(s.data); i++ {
		if i < len(s.baseXOR) {
			delta[i] = s.data[i] ^ s.baseXOR[i]
		} else {
			delta[i] = s.data[i]
		}
	}

	// Store the delta for when client ACKs
	seq = s.messageCounter
	s.pendingDeltas[seq] = delta
	s.messageCounter++
	s.lastSentSeq, s.sentOnce = seq, true

	// Calculate refnum (XOR byte) - difference from base message counter
	if s.baseMessageCounter == 0 {
		xorByte = 0
	} else {
		xorByte = byte(seq - s.baseMessageCounter)
	}

	s.dirty = false
	s.resendTicks = 0

	return delta, seq, xorByte
}

// retransmitLocked repeats the last message sent on this channel under the
// sequence number it first had, re-encoded against the current base (the
// newest copy the client acknowledged), as connection_append_state does for a
// state message already in flight. The client drops it if it already holds
// that sequence, and re-acknowledges it; the delta is kept so that ack can
// still move the base. Must be called with s.mu held, after a first send.
func (s *StateUpdateData) retransmitLocked() (data []byte, seq uint16, xorByte byte) {
	seq = s.lastSentSeq
	delta := make([]byte, len(s.data))
	for i := range s.data {
		if i < len(s.baseXOR) {
			delta[i] = s.data[i] ^ s.baseXOR[i]
		} else {
			delta[i] = s.data[i]
		}
	}
	s.pendingDeltas[seq] = delta
	if s.baseMessageCounter != 0 {
		xorByte = byte(seq - s.baseMessageCounter)
	}
	return delta, seq, xorByte
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

// AddBase stores a decoded state message as a base for future XOR delta reconstruction.
func (c *ClientStateData) AddBase(seq uint16, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	base := make([]byte, len(data))
	copy(base, data)
	c.bases[seq] = base
	// Limit stored bases to 32 (matching C#)
	if len(c.bases) > 32 {
		// Remove oldest entries
		var oldest uint16
		for k := range c.bases {
			if oldest == 0 || k < oldest {
				oldest = k
			}
		}
		delete(c.bases, oldest)
	}
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

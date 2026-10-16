package session

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// State represents the session state.
type State int

const (
	StateConnecting State = iota
	StateAuthenticated
	StateCharacterSelect
	StateInGame
	StateDisconnecting
)

// Session represents a client session.
type Session struct {
	mu sync.RWMutex

	// Network info
	remoteAddr     *net.UDPAddr
	instanceID     uint32
	sessionID      uint32
	clientEndpoint uint16
	serverEndpoint uint16

	// Session state
	state              State
	hasInstance        bool
	didServerInitiate  bool
	needsNewInstance   bool // Set true on first packet after promoting to master session
	characterInWorld   bool
	inGame             bool
	pendingTermination bool
	charListSent       bool // Track if character list was actually sent

	// Connection tracking
	conn *ConnectionData

	// Message queue
	queue *MessageQueue

	// Timing
	lastActivity   time.Time
	timeoutTick    time.Time
	pingCount      int32
	preGameTimeout time.Duration // Timeout for pre-game sessions (default 45s)
	inGameTimeout  time.Duration // Timeout for in-game sessions (default 60s)

	// Account/character info
	accountID      int
	charServerID   uint32      // Character's database server_id (for dedup on reconnect)
	character      interface{} // Will be *entity.Character when implemented
	playerObjectID uint32      // Unique ID for this player in the game world (assigned on enter world)

	// ECS entity reference (stored as any to avoid circular import with ecs package)
	ecsEntity interface{}

	// Visibility state (stored as interface{} to avoid circular import with ecs package)
	visibilityState interface{}

	// inventoryList is the handlers package's copy of the client's inventory
	// list: which record sits at each list index and the key the client files
	// it under. Stored as interface{} for the same reason as visibilityState.
	inventoryList interface{}

	// Send callback - set by UDP listener
	sendFunc func([]byte) error

	// Callback for when client ACKs initial messages (triggers character list)
	onAckCallback     func()
	ackCallbackFired  bool
	ackCallbackTarget uint16 // Message sequence that triggers callback

	// stateChannelsHeldUntil suppresses state channel transmission while the
	// client is loading a world. See HoldStateChannels.
	stateChannelsHeldUntil time.Time

	// worldLoadPending is true from the teleport until the client is seen to be
	// alive at the destination. See ConsumeWorldLoadPending.
	worldLoadPending bool

	// lastWorldMismatch is the client-reported world index already logged as
	// disagreeing with the server's. See NoteWorldMismatch.
	lastWorldMismatch   int16
	worldMismatchLogged bool
}

// HoldStateChannels stops state channel updates from going out until the client
// says it is ready (reliable opcode 0x0049) or until d elapses, whichever comes
// first.
//
// A teleport makes the PS2 tear its scene down and load a world, which takes
// seconds. Retail sends NOTHING on any state channel for that whole window —
// measured at 4.4 s in pcaps/sandstorm-worldchange-20260807.pcap and 3.5 s in
// pcaps/ps2_sandstorm.pcap — and resumes only after the client's 0x0049.
// Updates sent into that window are consumed by a scene that is about to be
// destroyed, and worse, the client ACKs them, so the server's baseXOR advances
// and it then believes the client already holds a state it threw away.
//
// Nothing is lost by holding: the channel buffers keep being written, their
// dirty flags are only cleared by GenerateXORDelta, and that is exactly what is
// being deferred. The deadline exists because a teleport the client can service
// without a reload sends no 0x0049 at all, and a hold with no way out would
// freeze every entity on the screen.
func (s *Session) HoldStateChannels(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateChannelsHeldUntil = time.Now().Add(d)
	s.worldLoadPending = true
}

// ConsumeWorldLoadPending reports — once — that a teleport is still waiting for
// the client to turn up at the destination, and clears the wait.
//
// The caller is the 0x40 position handler, and a position update is the
// strongest evidence there is that the client is alive in the new scene: it
// sends none at all while a world loads. Retail's own resync rides in the same
// packet as the first one.
//
// This exists because 0x0049 alone is not a reliable "load finished" edge on our
// client. Warping to world 9 on 2026-08-10 produced one 0x0049 a SECOND after
// the teleport — far too fast for a world load — and the real one six seconds
// later. Re-arming on the early one and then falling silent would put the
// original bug straight back; re-arming on both, plus on the first 0x40, cannot.
// Re-arming is idempotent: two marks in one packet collapse into one send,
// because the transmit happens on the tick.
func (s *Session) ConsumeWorldLoadPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.worldLoadPending
	s.worldLoadPending = false
	if pending {
		s.stateChannelsHeldUntil = time.Time{}
	}
	return pending
}

// NoteWorldMismatch reports whether a client-reported world of w is worth
// logging, and records it so the next one is not.
//
// The 0x40 arrives several times a second, so an unfiltered log would bury the
// session. One line per distinct disagreement is what makes it readable: the
// interesting events are the transitions, and a mismatch that persists is one
// event, not a stream of them. A return to agreement resets the filter, so a
// second failure later in the session logs again.
func (s *Session) NoteWorldMismatch(w byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.worldMismatchLogged && s.lastWorldMismatch == int16(w) {
		return false
	}
	s.lastWorldMismatch = int16(w)
	s.worldMismatchLogged = true
	return true
}

// ClearWorldMismatch forgets any logged disagreement, so that the next one is
// reported. Called when the client and server agree on the world again.
func (s *Session) ClearWorldMismatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worldMismatchLogged = false
}

// ReleaseStateChannels lifts a hold set by HoldStateChannels.
func (s *Session) ReleaseStateChannels() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateChannelsHeldUntil = time.Time{}
}

// StateChannelsHeld reports whether state channel output is currently suppressed.
func (s *Session) StateChannelsHeld() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.stateChannelsHeldUntil.IsZero() && time.Now().Before(s.stateChannelsHeldUntil)
}

// NewSession creates a new session.
func NewSession(addr *net.UDPAddr, instanceID, sessionID uint32, clientEndpoint, serverEndpoint uint16, serverInitiated bool) *Session {
	now := time.Now()

	s := &Session{
		remoteAddr:        addr,
		instanceID:        instanceID,
		sessionID:         sessionID,
		clientEndpoint:    clientEndpoint,
		serverEndpoint:    serverEndpoint,
		state:             StateConnecting,
		hasInstance:       true,
		didServerInitiate: serverInitiated,
		lastActivity:      now,
		preGameTimeout:    45 * time.Second, // Default, overridden by Manager
		inGameTimeout:     60 * time.Second, // Default, overridden by Manager
	}
	s.timeoutTick = now.Add(s.preGameTimeout)

	s.conn = NewConnectionData(s, clientEndpoint, serverEndpoint)
	s.queue = NewMessageQueue(s)

	return s
}

// RemoteAddr returns the session's remote address.
func (s *Session) RemoteAddr() *net.UDPAddr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remoteAddr
}

// InstanceID returns the session instance ID.
func (s *Session) InstanceID() uint32 {
	return s.instanceID
}

// SessionID returns the session ID.
func (s *Session) SessionID() uint32 {
	return s.sessionID
}

// ClientEndpoint returns the client endpoint ID.
func (s *Session) ClientEndpoint() uint16 {
	return s.clientEndpoint
}

// ServerEndpoint returns the server endpoint ID.
func (s *Session) ServerEndpoint() uint16 {
	return s.serverEndpoint
}

// SetServerEndpoint updates the server endpoint ID.
// Called when client targets a specific endpoint (e.g., game server 0xB991).
func (s *Session) SetServerEndpoint(endpoint uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverEndpoint = endpoint
}

// State returns the current session state.
func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// SetState sets the session state.
func (s *Session) SetState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state

	// Update timeout based on state
	if state == StateInGame {
		s.inGame = true
		s.timeoutTick = time.Now().Add(s.inGameTimeout)
	}
}

// HasInstance returns whether the session has an instance.
func (s *Session) HasInstance() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasInstance
}

// SetHasInstance sets whether the session has an instance.
func (s *Session) SetHasInstance(has bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasInstance = has
}

// DidServerInitiate returns whether the server initiated the session.
func (s *Session) DidServerInitiate() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.didServerInitiate
}

// SetDidServerInitiate sets whether the server initiated the session.
// Called when client connects to game server (0xB991) after selecting a server.
func (s *Session) SetDidServerInitiate(initiated bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.didServerInitiate = initiated
}

// NeedsNewInstance returns whether the next packet should have the NewInstance flag.
func (s *Session) NeedsNewInstance() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.needsNewInstance
}

// SetNeedsNewInstance sets whether the next packet should have the NewInstance flag.
func (s *Session) SetNeedsNewInstance(needs bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needsNewInstance = needs
}

// PromoteToMaster transitions this session into a "master session" for game world communication.
// This matches C#'s CreateMasterSession behavior:
// - New instance ID (so client recognizes a new session)
// - didServerInitiate = true (SessionID written in packet header)
// - needsNewInstance = true (NewInstance flag on first packet)
// - sessionID = playerObjectID (client reads this as ObjectID)
func (s *Session) PromoteToMaster(newInstanceID uint32, playerObjectID uint32) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldInstanceID := s.instanceID

	s.instanceID = newInstanceID
	s.sessionID = playerObjectID
	s.playerObjectID = playerObjectID
	s.didServerInitiate = true
	s.needsNewInstance = true
	s.hasInstance = true

	return oldInstanceID
}

// InGame returns whether the session is in-game.
func (s *Session) InGame() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inGame
}

// SetInGame sets whether the session is in-game.
func (s *Session) SetInGame(inGame bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inGame = inGame
	if inGame {
		s.timeoutTick = time.Now().Add(s.inGameTimeout)
	} else {
		s.timeoutTick = time.Now().Add(s.preGameTimeout)
	}
}

// CharacterInWorld returns whether the character is in the world.
func (s *Session) CharacterInWorld() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.characterInWorld
}

// SetCharacterInWorld sets whether the character is in the world.
func (s *Session) SetCharacterInWorld(inWorld bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.characterInWorld = inWorld
}

// CharListSent returns whether the character list was sent.
func (s *Session) CharListSent() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.charListSent
}

// SetCharListSent sets whether the character list was sent.
func (s *Session) SetCharListSent(sent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.charListSent = sent
}

// PendingTermination returns whether the session is pending termination.
func (s *Session) PendingTermination() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingTermination
}

// SetPendingTermination sets whether the session is pending termination.
func (s *Session) SetPendingTermination(pending bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingTermination = pending
}

// AccountID returns the account ID.
func (s *Session) AccountID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountID
}

// SetAccountID sets the account ID.
func (s *Session) SetAccountID(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountID = id
}

// CharacterServerID returns the character's database server_id.
func (s *Session) CharacterServerID() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.charServerID
}

// SetCharacterServerID sets the character's database server_id.
func (s *Session) SetCharacterServerID(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.charServerID = id
}

// PlayerObjectID returns the player's unique object ID in the game world.
// This is assigned when the player enters the world and is used in state channel 0.
func (s *Session) PlayerObjectID() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.playerObjectID
}

// SetPlayerObjectID sets the player's object ID.
func (s *Session) SetPlayerObjectID(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playerObjectID = id
}

// SetECSEntity stores the ECS entity reference.
// Stored as interface{} to avoid circular import with ecs package.
func (s *Session) SetECSEntity(entity interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ecsEntity = entity
}

// ECSEntity returns the stored ECS entity reference.
func (s *Session) ECSEntity() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ecsEntity
}

// SetVisibilityState stores the visibility state.
func (s *Session) SetVisibilityState(v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visibilityState = v
}

// VisibilityState returns the stored visibility state.
func (s *Session) VisibilityState() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visibilityState
}

// SetInventoryList stores the copy of the client's inventory list.
func (s *Session) SetInventoryList(v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inventoryList = v
}

// InventoryList returns the stored copy of the client's inventory list.
func (s *Session) InventoryList() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inventoryList
}

// Connection returns the connection data.
func (s *Session) Connection() *ConnectionData {
	return s.conn
}

// Queue returns the message queue.
func (s *Session) Queue() *MessageQueue {
	return s.queue
}

// SetSendFunc sets the function used to send packets.
func (s *Session) SetSendFunc(fn func([]byte) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendFunc = fn
}

// Send sends data using the send function.
func (s *Session) Send(data []byte) error {
	s.mu.RLock()
	fn := s.sendFunc
	s.mu.RUnlock()

	if fn == nil {
		return nil
	}
	return fn(data)
}

// UpdateActivity updates the last activity time and resets ping count.
// Called when a real packet is received from the client.
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
	atomic.StoreInt32(&s.pingCount, 0)

	if s.inGame {
		s.timeoutTick = time.Now().Add(s.inGameTimeout)
	} else {
		s.timeoutTick = time.Now().Add(s.preGameTimeout)
	}
}

// ResetTimeout resets the timeout timer without resetting ping count.
// Used after sending a server-initiated ping so IsTimedOut() stops firing every tick,
// while still allowing ping count to accumulate for disconnect detection.
func (s *Session) ResetTimeout() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.inGame {
		s.timeoutTick = time.Now().Add(s.inGameTimeout)
	} else {
		s.timeoutTick = time.Now().Add(s.preGameTimeout)
	}
}

// IsTimedOut returns whether the session has timed out.
func (s *Session) IsTimedOut() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Now().After(s.timeoutTick)
}

// IncrementPingCount increments and returns the ping count.
func (s *Session) IncrementPingCount() int32 {
	return atomic.AddInt32(&s.pingCount, 1)
}

// PingCount returns the current ping count.
func (s *Session) PingCount() int32 {
	return atomic.LoadInt32(&s.pingCount)
}

// CheckPingTimeout returns true if too many pings have been missed.
func (s *Session) CheckPingTimeout() bool {
	count := s.PingCount()
	if s.InGame() {
		return count >= 50
	}
	return count >= 10
}

// ProcessIncoming processes an incoming packet.
func (s *Session) ProcessIncoming(packet interface{}) {
	// This will be implemented in rdp.go
	// For now, delegate to connection data
	s.conn.ProcessPacket(packet)
}

// QueueMessage adds a message to the outbound queue.
func (s *Session) QueueMessage(msg *protocol.Message) {
	s.queue.Add(msg)
}

// SendPing sends a DRDP keepalive: a 0xF9 Ping message carrying the disc
// version byte.
//
// This used to be an unreliable BestEffortPing (0x07D0) opcode. No real server
// sends that opcode — 0 occurrences across all five PS2 captures. Retail keeps
// the link alive with the DRDP message type instead, and its payload is always
// the single byte 0x12 (120 identical pings across three captures), the same
// disc version carried by DiscVersion.
func (s *Session) SendPing() {
	msg := protocol.NewMessage(protocol.MessageTypePing, 0)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteByte(discVersionByte)
	msg.SetSize(writer.Position())
	s.queue.AddWithSequence(msg)
	s.IncrementPingCount()
}

// discVersionByte is the payload every retail server ping carries.
const discVersionByte = 0x12

// SetTimeouts configures the pre-game and in-game timeout durations.
func (s *Session) SetTimeouts(preGame, inGame time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preGameTimeout = preGame
	s.inGameTimeout = inGame
}

// Key returns a unique key for the session.
func (s *Session) Key() SessionKey {
	return SessionKey{
		Addr:       s.remoteAddr.String(),
		InstanceID: s.instanceID,
	}
}

// SessionKey is a unique identifier for a session.
type SessionKey struct {
	Addr       string
	InstanceID uint32
}

// SetAckCallback sets a callback to be triggered when client ACKs the target message sequence.
// Used for sending character list after client ACKs initial messages (sequence 2).
func (s *Session) SetAckCallback(target uint16, callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAckCallback = callback
	s.ackCallbackTarget = target
	s.ackCallbackFired = false
}

// DebugAckCallback returns debug info about the callback state.
func (s *Session) DebugAckCallback() (hasCallback bool, target uint16, fired bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.onAckCallback != nil, s.ackCallbackTarget, s.ackCallbackFired
}

// CheckAckCallback checks if the ACK callback should be triggered and fires it if so.
// Returns true if callback was fired.
func (s *Session) CheckAckCallback(clientAck uint16) bool {
	s.mu.Lock()

	if s.onAckCallback == nil || s.ackCallbackFired {
		s.mu.Unlock()
		return false
	}

	// Fire callback when client has ACKed at least up to our target sequence
	if clientAck >= s.ackCallbackTarget {
		s.ackCallbackFired = true
		callback := s.onAckCallback
		s.mu.Unlock()
		// Call callback without holding the lock to avoid deadlock
		callback()
		return true
	}

	s.mu.Unlock()
	return false
}

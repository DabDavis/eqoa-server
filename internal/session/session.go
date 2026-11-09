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
	replacedAt         time.Time // set when another session took this one's character
	quitAt             time.Time // its client quit (0x09B0) while creatures fought it: it ends when they stop
	charListSent       bool      // Track if character list was actually sent

	// Connection tracking
	conn *ConnectionData

	// Message queue
	queue *MessageQueue

	// shown is what the client was shown lately, for a petition's server
	// view (transcript.go)
	shown transcript

	// Timing
	lastActivity   time.Time
	timeoutTick    time.Time
	pingCount      int32
	preGameTimeout time.Duration // Timeout for pre-game sessions (default 45s)
	inGameTimeout  time.Duration // Timeout for in-game sessions (default 60s)

	// Account/character info
	accountID    int
	charServerID uint32 // Character's database server_id (for dedup on reconnect)
	charName     string // its name, for the log's lines (SetCharacterName)
	// lobbyLate counts the lobby connection's packets that came after the
	// promotion (NoteLobbyLate)
	lobbyLate      int32
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
	// bankList is the same for the client's bank list (VIBankControl+8),
	// which keeps keys of its own.
	bankList interface{}
	// merchantWindow is the handlers package's note of the list it last sent
	// the client as 0x00B7 (handlers/buyback.go).
	merchantWindow interface{}

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

	// discVersion is the first byte of the client's DiscVersion (0x0000):
	// 0x12 from a Frontiers disc (a retail PS2, eqoa-rust-client-vanilla, the
	// web page), 0x13 from a PS2 running Sandstorm's S2 patch on its world
	// session, 0 before one arrives. See DiscVersion.
	discVersion byte

	// factionMessagesOff is the client's Faction Messages option off: 0x00C9's
	// first byte 1 (sent at the world entry and when the option changes).
	factionMessagesOff bool
	// lootMessagesOff is the client's Loot Messages option off: 0x00CA's state
	lootMessagesOff bool

	// mailLiveForms is the client's answer to the mail form offer
	// (OpcodeMailForm, handlers/mail.go): it sends SOE's live mail forms.
	mailLiveForms bool
	// liveLook: the client asked for SOE's live game's look
	// (protocol.OpcodeLiveLook) and was told it is in force: its entity
	// channels are the live 201 bytes and its character lists the live
	// records (entity/livelook.go). liveLookOffered: the offer went out.
	liveLook        bool
	liveLookOffered bool
	// creationOffered: the creation offer went out (protocol.OpcodeCreationOffer,
	// handlers/creation.go), once a session as the live look's.
	creationOffered bool
	// capes: the client answered the cape offer 1 (protocol.OpcodeCapes,
	// handlers/capes.go): it draws capes, so it is sent a cape's wear slot and
	// a worn one's equip slot, may wear one, and is told who wears which.
	// capesOffered: the offer went out, once a session as the live look's.
	capes        bool
	capesOffered bool
	// itemListsPack, itemListsBank: the client's answer to the item lists'
	// offer (protocol.OpcodeItemLists, handlers/itemlists.go), the records it
	// holds of the pack and the bank; 0 for a client that never answered (a
	// PS2), which holds CLIENT's 40. itemListsOffered: the offer went out, once
	// a session as the capes'.
	itemListsPack, itemListsBank int
	itemListsOffered             bool
	// raceOptionsOffered: the race options' offer went out
	// (protocol.OpcodeRaceOptions, handlers/raceoptions.go), once a session.
	// raceLooks: the client answered it, so it is sent each race's options
	// (raceCatalogueSent: once) and the looks the characters chose, whenever
	// they differ from raceLooksSent, the list it was last sent.
	raceOptionsOffered bool
	raceLooks          bool
	raceCatalogueSent  bool
	raceLooksSent      string
	// creationAsked: the client answered creation's offer 1
	// (protocol.OpcodeCreationOffer, handlers/creation.go), so a login's world
	// entry sends it creation's races again (handlers/raceoptions.go)
	creationAsked bool

	// clock is the game clock the session's client keeps, set by each 0x0013
	// it was sent: its time of day (gameclock.go).
	clock gameClock

	// bugReports is the client's answer to the bug report offer
	// (protocol.OpcodeBugReportOffer, handlers/bugreport.go): it opens
	// Support's request on Bug Report from a key of its own and sends a note
	// of its own side with what it files. bugNote is that note for the next
	// petition filed, and bugNoteAt when it came. reportNotice: the line on
	// how to report a bug went out. boonNotice: the line on where the towns'
	// boon is went out (handlers/boon.go).
	bugReports bool
	// ackMasks: the client answered the bug report offer (any answer: a PS2
	// never does), so our acks carry their mask (network/udp/rdp.go); never
	// cleared, as CLIENT refuses a mask taken back (SetAckMasks)
	ackMasks     bool
	bugNote      string
	bugNoteAt    time.Time
	reportNotice bool
	boonNotice   bool

	// flyClient: the client said it flies (protocol.OpcodeFly, handlers/fly.go),
	// so #fly may send it the opcode. A client that never said (a PS2, an older
	// build) is sent none.
	flyClient bool

	// resume is the MemoryDump a session carried through a restart is due,
	// and resumed whether it was carried through one (keep.go)
	resume  resumeDump
	resumed bool
}

// SetMailLiveForms records the client's answer to the mail form offer.
func (s *Session) SetMailLiveForms(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mailLiveForms = on
}

// SetBugReports records the client's answer to the bug report offer.
func (s *Session) SetBugReports(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bugReports = on
}

// SetAckMasks marks the session's client as one of ours (it answered the bug
// report offer, whatever it answered): our acks of its messages carry their
// mask from now on. Once set it stays, through a restart too: a mask stopped
// while its bits are out would read to CLIENT as acks taken back ("some of
// the previously acked packets have become un-acked"), every segment refused.
func (s *Session) SetAckMasks() {
	s.mu.Lock()
	s.ackMasks = true
	s.mu.Unlock()
}

// AckMasks is whether our acks to the client carry their mask (SetAckMasks).
func (s *Session) AckMasks() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ackMasks
}

// BugReports is whether the client said it reports bugs from a key of its own
// with a note of its side. A client that never answered (a PS2) did not.
func (s *Session) BugReports() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bugReports
}

// SetFlyClient records that the client said it flies.
func (s *Session) SetFlyClient(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flyClient = on
}

// FlyClient is whether the client said it flies: only then is it sent
// protocol.OpcodeFly. A client that never said (a PS2) did not.
func (s *Session) FlyClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.flyClient
}

// SetBugNote keeps the client's note for the next petition it files.
func (s *Session) SetBugNote(note string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bugNote, s.bugNoteAt = note, at
}

// TakeBugNote is the client's note for a petition filed at now, when it came
// within the minute before (and none after: it goes with one petition), else
// "".
func (s *Session) TakeBugNote(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	note, at := s.bugNote, s.bugNoteAt
	s.bugNote, s.bugNoteAt = "", time.Time{}
	if note == "" || now.Sub(at) > time.Minute || at.After(now.Add(time.Second)) {
		return ""
	}
	return note
}

// TakeBoonNotice is true the first time it is asked: the line on where the
// towns' boon is goes out once a session.
func (s *Session) TakeBoonNotice() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.boonNotice {
		return false
	}
	s.boonNotice = true
	return true
}

// TakeReportNotice is true the first time it is asked: the line on how to
// report a bug is sent once a session.
func (s *Session) TakeReportNotice() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportNotice {
		return false
	}
	s.reportNotice = true
	return true
}

// MailLiveForms is whether the client said it sends the live mail forms. A
// client that never answered (a PS2) did not.
func (s *Session) MailLiveForms() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mailLiveForms
}

// SetLiveLook records whether the session's client reads the live game's
// look (entity channels and character lists).
func (s *Session) SetLiveLook(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveLook = on
}

// LiveLook is whether the session's client reads the live game's look.
func (s *Session) LiveLook() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liveLook
}

// OfferLiveLookOnce reports whether the live look's offer is still to be
// made on this session, and marks it made.
func (s *Session) OfferLiveLookOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.liveLookOffered
	s.liveLookOffered = true
	return first
}

// OfferRaceOptionsOnce reports whether the race options' offer is still to be
// made to this session, and marks it made.
func (s *Session) OfferRaceOptionsOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.raceOptionsOffered
	s.raceOptionsOffered = true
	return first
}

// SetRaceLooks records the client's answer to the race options' offer: on, it
// is sent the options and the looks; off, nothing more.
func (s *Session) SetRaceLooks(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raceLooks = on
	if !on {
		s.raceLooksSent = ""
	}
}

// RaceLooks is whether the client asked for the races' options and looks. A
// client that never answered (a PS2) did not.
func (s *Session) RaceLooks() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.raceLooks
}

// TakeRaceCatalogue is true the first time it is asked of a session that asked
// for the races' options: their list goes out once a session.
func (s *Session) TakeRaceCatalogue() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.raceLooks || s.raceCatalogueSent {
		return false
	}
	s.raceCatalogueSent = true
	return true
}

// RaceDataAgain forgets that the races' options and the looks went out, so the
// next SendRaceOptions and RefreshRaceLooks send them again: a login's world
// entry, as they went on the lobby's connection (handlers/raceoptions.go).
func (s *Session) RaceDataAgain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raceCatalogueSent = false
	s.raceLooksSent = ""
}

// SetCreationAsked records that the client answered creation's offer 1.
func (s *Session) SetCreationAsked() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creationAsked = true
}

// CreationAsked is whether the client answered creation's offer 1.
func (s *Session) CreationAsked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.creationAsked
}

// TakeRaceLooks reports whether body, a list of the looks, is to go to this
// session (it asked, and was last sent another), and records it as sent.
func (s *Session) TakeRaceLooks(body string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.raceLooks || s.raceLooksSent == body {
		return false
	}
	s.raceLooksSent = body
	return true
}

// OfferCreationOnce reports whether creation's offer is still to be made to
// this session, and marks it made.
func (s *Session) OfferCreationOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.creationOffered
	s.creationOffered = true
	return first
}

// SetCapes records the client's answer to the cape offer: it draws capes.
func (s *Session) SetCapes(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capes = on
}

// Capes is whether the client said it draws capes. A client that never
// answered (a PS2) did not.
func (s *Session) Capes() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capes
}

// CapesOffered is whether the cape offer went out on this session: only then
// is a client's answer taken.
func (s *Session) CapesOffered() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capesOffered
}

// OfferCapesOnce reports whether the cape offer is still to be made to this
// session, and marks it made.
func (s *Session) OfferCapesOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.capesOffered
	s.capesOffered = true
	return first
}

// SetItemLists records the client's answer to the item lists' offer: the
// records it holds of the pack and the bank (0, 0: it stopped, CLIENT's 40).
func (s *Session) SetItemLists(pack, bank int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.itemListsPack, s.itemListsBank = pack, bank
}

// ItemLists is what the client said it holds of the pack and the bank: 0, 0
// for one that never answered (a PS2), which holds CLIENT's 40.
func (s *Session) ItemLists() (pack, bank int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.itemListsPack, s.itemListsBank
}

// ItemListsOffered is whether the item lists' offer went out on this session:
// only then is a client's answer taken.
func (s *Session) ItemListsOffered() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.itemListsOffered
}

// OfferItemListsOnce reports whether the item lists' offer is still to be made
// to this session, and marks it made.
func (s *Session) OfferItemListsOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.itemListsOffered
	s.itemListsOffered = true
	return first
}

// SetFactionMessagesOff records the client's Faction Messages option (0x00C9):
// off, the server sends no faction line to this session.
func (s *Session) SetFactionMessagesOff(off bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.factionMessagesOff = off
}

// SetLootMessagesOff records the client's Loot Messages option (0x00CA): off,
// the server sends no line of what a corpse gave to this session.
func (s *Session) SetLootMessagesOff(off bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lootMessagesOff = off
}

// LootMessagesOff is whether the client asked for no loot lines.
func (s *Session) LootMessagesOff() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lootMessagesOff
}

// FactionMessagesOff is whether the client asked for no faction lines.
func (s *Session) FactionMessagesOff() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.factionMessagesOff
}

// SetDiscVersion records the version byte the client's DiscVersion carried.
func (s *Session) SetDiscVersion(v byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discVersion = v
}

// DiscVersion is the version byte the client's DiscVersion carried, 0 when
// none has arrived. It tells which world table the client loads from: a
// Frontiers disc has six worlds, and only a PS2 running a patch (S2, and our
// hard-disk loader after it) has more.
func (s *Session) DiscVersion() byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.discVersion
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

// SetQuitting records that the client quit (0x09B0) while creatures fought
// its character: the session stays, link-dead, until they stop
// (entity/linkdead.go). The first quit counts.
func (s *Session) SetQuitting(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quitAt.IsZero() {
		s.quitAt = at
	}
}

// QuittingSince is when the client quit in a fight, and false if it did not.
func (s *Session) QuittingSince() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.quitAt, !s.quitAt.IsZero()
}

// MarkReplaced records that another session took this one's character. The
// session is kept only long enough to deliver the disconnect it was sent; see
// Manager.ReplaceSession.
func (s *Session) MarkReplaced() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replacedAt.IsZero() {
		s.replacedAt = time.Now()
	}
}

// ReplacedFor returns how long ago the session was replaced, and false if it
// never was.
func (s *Session) ReplacedFor() (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.replacedAt.IsZero() {
		return 0, false
	}
	return time.Since(s.replacedAt), true
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

// SetCharacterName names the session's character, for the log's lines
// (netstats.go's "Net:").
func (s *Session) SetCharacterName(name string) {
	s.mu.Lock()
	s.charName = name
	s.mu.Unlock()
}

// CharacterName is the session's character's name, "" before one is picked.
func (s *Session) CharacterName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.charName
}

// NoteLobbyLate counts one more of the lobby connection's packets come to
// this promoted session (PromotedOwn), and says how many so far.
func (s *Session) NoteLobbyLate() int {
	return int(atomic.AddInt32(&s.lobbyLate, 1))
}

// Label names the session in a log line: its address and its character's
// name when it has one.
func (s *Session) Label() string {
	if s == nil {
		return "<none>"
	}
	addr := "?"
	if a := s.RemoteAddr(); a != nil {
		addr = a.String()
	}
	if n := s.CharacterName(); n != "" {
		return addr + " '" + n + "'"
	}
	return addr
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

// SetMerchantWindow stores the note of the merchant list last sent.
func (s *Session) SetMerchantWindow(v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.merchantWindow = v
}

// MerchantWindow returns the note of the merchant list last sent.
func (s *Session) MerchantWindow() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.merchantWindow
}

// SetBankList stores the copy of the client's bank list.
func (s *Session) SetBankList(v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bankList = v
}

// BankList returns the stored copy of the client's bank list.
func (s *Session) BankList() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bankList
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

// SetLastActivity sets when the client was last heard from (a test's silence).
func (s *Session) SetLastActivity(t time.Time) {
	s.mu.Lock()
	s.lastActivity = t
	s.mu.Unlock()
}

// LastActivity is when the client last sent a real packet (UpdateActivity).
func (s *Session) LastActivity() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActivity
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

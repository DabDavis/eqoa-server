package session

// keep.go — a session carried through a restart of the server process
// (claude-notes/session-keep-restart-20261106.md; the user, 2026-11-06: "is
// there a way to keep folks in world and not boot with a restart").
//
// The process re-execs itself with the game's UDP socket held open, so a
// client's packets wait in the kernel and none is answered "port unreachable"
// (CLIENT ends a route at once on that: drdp.cpp's drdp_route_terminate). What
// the new process must know to go on with each connection is written here
// before the exec and read back after it: the session's identity and flags,
// both directions' packet and message sequence numbers, every reliable message
// not yet acknowledged, each state channel's next sequence number and what it
// last held, and the client's own channel with its XOR bases. A state channel
// goes out WHOLE after a resume (refnum 0, as StateUpdateData.ForceFullResend
// explains), under a sequence number past every one the client holds, so the
// history of what was acknowledged need not be kept. The reliable stream must
// not gap: every message not yet acknowledged goes again under its own number.

import (
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Kept is a session as a restart carries it.
type Kept struct {
	Addr              string `json:"addr"`
	InstanceID        uint32 `json:"instance"`
	SessionID         uint32 `json:"session"`
	ClientEndpoint    uint16 `json:"client_ep"`
	ServerEndpoint    uint16 `json:"server_ep"`
	State             State  `json:"state"`
	HasInstance       bool   `json:"has_instance"`
	DidServerInitiate bool   `json:"server_init"`
	CharacterInWorld  bool   `json:"in_world"`
	InGame            bool   `json:"in_game"`
	CharListSent      bool   `json:"char_list_sent"`
	AccountID         int    `json:"account"`
	CharServerID      uint32 `json:"char"`
	PlayerObjectID    uint32 `json:"object"`

	// what the client answered or was told (session.go)
	DiscVersion        byte `json:"disc_version"`
	FactionMessagesOff bool `json:"faction_messages_off,omitempty"`
	LootMessagesOff    bool `json:"loot_messages_off,omitempty"`
	MailLiveForms      bool `json:"mail_live_forms,omitempty"`
	LiveLook           bool `json:"live_look,omitempty"`
	LiveLookOffered    bool `json:"live_look_offered,omitempty"`
	CreationOffered    bool `json:"creation_offered,omitempty"`
	Capes              bool `json:"capes,omitempty"`
	CapesOffered       bool `json:"capes_offered,omitempty"`
	ItemListsPack      int  `json:"item_lists_pack,omitempty"`
	ItemListsBank      int  `json:"item_lists_bank,omitempty"`
	ItemListsOffered   bool `json:"item_lists_offered,omitempty"`
	RaceOptionsOffered bool `json:"race_options_offered,omitempty"`
	RaceLooks          bool `json:"race_looks,omitempty"`
	RaceSkins          bool `json:"race_skins,omitempty"`
	RaceCatalogueSent  bool `json:"race_catalogue_sent,omitempty"`
	BugReports         bool `json:"bug_reports,omitempty"`
	AckMasks           bool `json:"ack_masks,omitempty"`
	FlyClient          bool `json:"fly_client,omitempty"`
	NumberTags         bool `json:"number_tags,omitempty"`
	// NumberTagsLayout is the number tags' layout the client asked for, when
	// past 1 (a kept file before it has NumberTags alone: layout 1)
	NumberTagsLayout uint8 `json:"number_tags_layout,omitempty"`
	NPCTints         bool  `json:"npc_tints,omitempty"`
	MobInfo          bool  `json:"mob_info,omitempty"`
	MercsTold        bool  `json:"mercs_told,omitempty"`
	// MercsLayout is the layout of OpcodeMercsData the client asked for (2
	// names each merc's hirer, the client's "<X's Mercenary>" tag): without
	// it a kept session fell back to 1 and every tag went (2026-11-11).
	MercsLayout      uint8 `json:"mercs_layout,omitempty"`
	GroupReg         bool  `json:"group_reg,omitempty"`
	ItemLinks        bool  `json:"item_links,omitempty"`
	ReportNotice     bool  `json:"report_notice,omitempty"`
	BoonNotice       bool  `json:"boon_notice,omitempty"`
	WorldLoadPending bool  `json:"world_load_pending,omitempty"`

	// the server's own worlds offered and taken, and those the client holds
	ServerWorldsOffered bool    `json:"server_worlds_offered,omitempty"`
	ServerWorldsTaken   bool    `json:"server_worlds_taken,omitempty"`
	ServerWorlds        []uint8 `json:"server_worlds,omitempty"`
	// ServerWorldCRCs are the CRCs of those worlds as this server had them built when the session was kept, one a
	// slot of ServerWorlds (none from a server before 2026-11-11): a world rebuilt across the restart (another
	// CRC now) is no longer one the client holds, as its file is the old one (ServerWorldCRC)
	ServerWorldCRCs []uint32 `json:"server_world_crcs,omitempty"`

	Clock *KeptClock `json:"clock,omitempty"`
	Conn  KeptConn   `json:"conn"`

	// Unacked are the reliable messages sent and not yet acknowledged, and
	// those queued for their first send, in sequence order; Pending is what
	// waits for its first send, in the queue's order (a message in both is the
	// same message: its Seq names it, 0 for one numbered only when it goes).
	Unacked []KeptMessage `json:"unacked,omitempty"`
	Pending []KeptMessage `json:"pending,omitempty"`
}

// KeptClock is the game clock the client keeps (gameclock.go), its local
// reading as a wall-clock time: the timer it is counted on starts with each
// process (clockBase).
type KeptClock struct {
	Remote    int64 `json:"remote"`
	LocalUnix int64 `json:"local_unix_ms"`
	Blend     int64 `json:"blend"`
}

// KeptConn is a connection's sequence numbers and channels.
type KeptConn struct {
	RxPacket  uint16 `json:"rx_packet"`
	RxMessage uint16 `json:"rx_message"`
	RxSeen    uint64 `json:"rx_seen"`
	// RxAck and RxWin are the receive window (rxwindow.go): the client's
	// messages taken with every one before them, and those taken past it.
	// RxMessage stays the newest taken, what a binary from before the window
	// acknowledged and reads back: given the window's ack, it would
	// acknowledge less than the client was told, with no mask, and CLIENT
	// refuses every such segment ("some of the previously acked packets have
	// become un-acked"). An older binary's file has neither: its RxMessage
	// was acknowledged whole.
	RxAck     *uint16 `json:"rx_ack,omitempty"`
	RxWin     uint64  `json:"rx_win,omitempty"`
	TxPacket  uint16  `json:"tx_packet"`
	TxMessage uint16  `json:"tx_message"`
	Acked     uint16  `json:"acked"`
	// Fragments is a message the client was sending in pieces, the pieces
	// taken so far (fragments.go)
	Fragments *KeptFragments `json:"fragments,omitempty"`

	Slots   int           `json:"slots"`
	View    int           `json:"view"`
	Objects []KeptChannel `json:"objects"`
	Stat    KeptChannel   `json:"stat"`
	Buff    KeptChannel   `json:"buff"`
	Group   KeptChannel   `json:"group"`
	Client  KeptClient    `json:"client"`
}

// KeptChannel is one state channel: the next sequence number it numbers a
// message with, what it holds, and whether that has gone out (Dirty: not yet).
type KeptChannel struct {
	Next   uint16 `json:"next"`
	Data   []byte `json:"data,omitempty"`
	Active bool   `json:"active,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
}

// KeptClient is the client's own channel (0x40): the message taken last, and
// the decoded messages its next ones may be XORed against.
type KeptClient struct {
	Seq   uint16     `json:"seq"`
	Took  bool       `json:"took,omitempty"`
	Data  []byte     `json:"data,omitempty"`
	Bases []KeptBase `json:"bases,omitempty"`
}

// KeptBase is one of KeptClient's bases.
type KeptBase struct {
	Seq  uint16 `json:"seq"`
	Data []byte `json:"data"`
}

// KeptMessage is one reliable message.
type KeptMessage struct {
	Type   byte   `json:"type"`
	Opcode uint16 `json:"opcode"`
	Seq    uint16 `json:"seq"`
	Data   []byte `json:"data"`
}

// Keep is the session as a restart carries it. The caller has stopped what
// would change it (the listener's packets and ticks); a message another
// goroutine queues meanwhile is in it whole or not at all, with the sequence
// numbers to match.
func (s *Session) Keep() Kept {
	s.mu.RLock()
	k := Kept{
		Addr:               s.remoteAddr.String(),
		InstanceID:         s.instanceID,
		SessionID:          s.sessionID,
		ClientEndpoint:     s.clientEndpoint,
		ServerEndpoint:     s.serverEndpoint,
		State:              s.state,
		HasInstance:        s.hasInstance,
		DidServerInitiate:  s.didServerInitiate,
		CharacterInWorld:   s.characterInWorld,
		InGame:             s.inGame,
		CharListSent:       s.charListSent,
		AccountID:          s.accountID,
		CharServerID:       s.charServerID,
		PlayerObjectID:     s.playerObjectID,
		DiscVersion:        s.discVersion,
		FactionMessagesOff: s.factionMessagesOff,
		LootMessagesOff:    s.lootMessagesOff,
		MailLiveForms:      s.mailLiveForms,
		LiveLook:           s.liveLook,
		LiveLookOffered:    s.liveLookOffered,
		CreationOffered:    s.creationOffered,
		Capes:              s.capes,
		CapesOffered:       s.capesOffered,
		ItemListsPack:      s.itemListsPack,
		ItemListsBank:      s.itemListsBank,
		ItemListsOffered:   s.itemListsOffered,
		RaceOptionsOffered: s.raceOptionsOffered,
		RaceLooks:          s.raceLooks,
		RaceSkins:          s.raceSkins,
		RaceCatalogueSent:  s.raceCatalogueSent,
		BugReports:         s.bugReports,
		AckMasks:           s.ackMasks,
		FlyClient:          s.flyClient,
		NumberTags:         s.numberTags != 0,
		NumberTagsLayout:   s.numberTags,
		NPCTints:           s.npcTints,
		MobInfo:            s.mobInfo,
		MercsTold:          s.mercsTold,
		MercsLayout:        s.mercsLayout,
		GroupReg:           s.groupReg,
		ItemLinks:          s.itemLinks,
		ReportNotice:       s.reportNotice,
		BoonNotice:         s.boonNotice,
		WorldLoadPending:   s.worldLoadPending,

		ServerWorldsOffered: s.serverWorldsOffered,
		ServerWorldsTaken:   s.serverWorldsTaken,
		ServerWorlds:        append([]uint8(nil), s.serverWorlds...),
		ServerWorldCRCs:     serverWorldCRCs(s.serverWorlds),
	}
	if s.clock.ready {
		k.Clock = &KeptClock{Remote: s.clock.remote, LocalUnix: unixOfTimer(s.clock.local), Blend: s.clock.blend}
	}
	s.mu.RUnlock()

	k.Conn = s.conn.keep()
	// the queue and the message numbers together: AddWithSequence numbers a
	// message under the queue's lock, so nothing is numbered and not queued,
	// or queued under a number the snapshot does not count
	k.Unacked, k.Pending, k.Conn.TxMessage = s.queue.keep(s.conn)
	return k
}

// keep is the connection's part of a Kept, but for TxMessage (MessageQueue.keep
// reads it under the queue's lock).
func (cd *ConnectionData) keep() KeptConn {
	cd.mu.Lock()
	ack := cd.lastReceivedMessageSeq
	k := KeptConn{
		RxPacket:  cd.lastReceivedPacketSeq,
		RxMessage: cd.newestTakenLocked(),
		RxAck:     &ack,
		RxSeen:    ^uint64(0),
		RxWin:     cd.rxWin,
		TxPacket:  cd.lastSentPacketSeq,
		TxMessage: cd.lastSentMessageSeq,
		Acked:     cd.clientLastReceivedMessage,
		Fragments: cd.frags.keepLocked(),
	}
	cd.mu.Unlock()
	cd.objMu.RLock()
	k.Slots, k.View = cd.slots, cd.view
	for _, o := range cd.serverObjects {
		k.Objects = append(k.Objects, o.keep())
	}
	cd.objMu.RUnlock()
	k.Stat = cd.clientStatUpdate.keep()
	k.Buff = cd.clientBuffUpdate.keep()
	k.Group = cd.serverGroupUpdate.keep()
	k.Client = cd.client.keep()
	return k
}

func (s *StateUpdateData) keep() KeptChannel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return KeptChannel{Next: s.messageCounter, Data: append([]byte(nil), s.data...), Active: s.isActive, Dirty: s.dirty}
}

func (c *ClientStateData) keep() KeptClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := KeptClient{Seq: c.SeqNum, Took: c.took, Data: append([]byte(nil), c.data...)}
	for seq, b := range c.bases {
		k.Bases = append(k.Bases, KeptBase{Seq: seq, Data: append([]byte(nil), b...)})
	}
	return k
}

// keep is the queue's reliable messages and, read under the same lock, the
// last message number the connection gave out. Unreliable messages queued
// (state channels, made again each tick) are not kept.
func (q *MessageQueue) keep(cd *ConnectionData) (unacked, pending []KeptMessage, txMessage uint16) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, m := range q.reliable {
		unacked = append(unacked, keptMessage(m))
	}
	for _, m := range q.pending {
		if m.Type.IsReliable() {
			pending = append(pending, keptMessage(m))
		}
	}
	cd.mu.Lock()
	txMessage = cd.lastSentMessageSeq
	cd.mu.Unlock()
	return unacked, pending, txMessage
}

func keptMessage(m *protocol.Message) KeptMessage {
	return KeptMessage{Type: byte(m.Type), Opcode: uint16(m.Opcode), Seq: m.Sequence, Data: append([]byte(nil), m.Data()...)}
}

// Resume puts a kept session back into the manager under its own key, its
// connection where it was, and reports it; an address that does not parse or
// a key already taken is an error, and nothing is added.
func (m *Manager) Resume(k Kept) (*Session, error) {
	addr, err := net.ResolveUDPAddr("udp", k.Addr)
	if err != nil {
		return nil, fmt.Errorf("session %s: %w", k.Addr, err)
	}
	s := NewSession(addr, k.InstanceID, k.SessionID, k.ClientEndpoint, k.ServerEndpoint, k.DidServerInitiate)
	s.SetTimeouts(m.preGameTimeout, m.inGameTimeout)
	s.restore(k, time.Now())
	if _, loaded := m.sessions.LoadOrStore(s.Key(), s); loaded {
		return nil, fmt.Errorf("session %s instance 0x%X: one is there already", k.Addr, k.InstanceID)
	}
	atomic.AddInt64(&m.activeCount, 1)
	atomic.AddInt64(&m.totalCount, 1)
	// the next login's object id past every one resumed
	for {
		cur := atomic.LoadUint32(&m.nextPlayerObjectID)
		if k.PlayerObjectID <= cur || atomic.CompareAndSwapUint32(&m.nextPlayerObjectID, cur, k.PlayerObjectID) {
			break
		}
	}
	return s, nil
}

// restore sets a new session to k. Nothing else holds s yet.
func (s *Session) restore(k Kept, now time.Time) {
	s.mu.Lock()
	s.state = k.State
	s.hasInstance = k.HasInstance
	s.didServerInitiate = k.DidServerInitiate
	s.needsNewInstance = false // the client has the instance: a NewInstance flag would start it over
	s.characterInWorld = k.CharacterInWorld
	s.inGame = k.InGame
	s.charListSent = k.CharListSent
	s.accountID = k.AccountID
	s.charServerID = k.CharServerID
	s.playerObjectID = k.PlayerObjectID
	s.discVersion = k.DiscVersion
	s.factionMessagesOff = k.FactionMessagesOff
	s.lootMessagesOff = k.LootMessagesOff
	s.mailLiveForms = k.MailLiveForms
	s.liveLook = k.LiveLook
	s.liveLookOffered = k.LiveLookOffered
	s.creationOffered = k.CreationOffered
	s.serverWorldsOffered = k.ServerWorldsOffered
	s.serverWorldsTaken = k.ServerWorldsTaken
	s.serverWorlds = stillHeld(k.ServerWorlds, k.ServerWorldCRCs)

	s.capes = k.Capes
	s.capesOffered = k.CapesOffered
	s.itemListsPack, s.itemListsBank, s.itemListsOffered = k.ItemListsPack, k.ItemListsBank, k.ItemListsOffered
	// the client keeps its lists; a data step in the hold may have changed the
	// looks, so the next refresh sends them again (handlers/keep.go)
	s.raceOptionsOffered, s.raceLooks, s.raceCatalogueSent, s.raceLooksSent = k.RaceOptionsOffered, k.RaceLooks, k.RaceCatalogueSent, ""
	s.raceSkins = k.RaceSkins
	s.bugReports = k.BugReports
	s.ackMasks = k.AckMasks
	s.flyClient = k.FlyClient
	s.numberTags = k.NumberTagsLayout
	if s.numberTags == 0 && k.NumberTags {
		s.numberTags = 1
	}
	// the client keeps its tints; a data step in the hold may have changed
	// them, so the next refresh sends them whatever they are (handlers/keep.go)
	s.npcTints, s.npcTintsSent = k.NPCTints, ""
	if k.NPCTints {
		s.npcTintsSent = npcTintsUnknown
	}
	s.mobInfo = k.MobInfo
	// what they were sent is sent again at the next refresh (the client
	// keeps it; the world may have changed in the hold)
	s.mercsTold, s.mercsLayout, s.mercsSent = k.MercsTold, k.MercsLayout, ""
	s.groupReg, s.groupRegSent = k.GroupReg, ""
	s.itemLinks = k.ItemLinks
	s.reportNotice = k.ReportNotice
	s.boonNotice = k.BoonNotice
	s.worldLoadPending = k.WorldLoadPending
	if k.WorldLoadPending {
		s.stateChannelsHeldUntil = now.Add(10 * time.Second)
	}
	if c := k.Clock; c != nil {
		s.clock = gameClock{ready: true, remote: c.Remote, local: timerAt(time.UnixMilli(c.LocalUnix)), blend: c.Blend}
	}
	s.lastActivity = now
	s.resumed = true
	if s.inGame {
		s.timeoutTick = now.Add(s.inGameTimeout)
	} else {
		s.timeoutTick = now.Add(s.preGameTimeout)
	}
	s.mu.Unlock()

	s.conn.restore(k.Conn)
	s.queue.restore(k.Unacked, k.Pending)
}

func (cd *ConnectionData) restore(k KeptConn) {
	cd.mu.Lock()
	cd.lastReceivedPacketSeq = k.RxPacket
	cd.lastReceivedMessageSeq = k.RxMessage
	if k.RxAck != nil {
		cd.lastReceivedMessageSeq = *k.RxAck
	}
	// everything at or below the newest message the old process took counts
	// as taken: it acknowledged them all (its acks are cumulative), and a copy
	// of one the client sends again now is one handled already
	cd.rxWin = 0
	if k.RxAck != nil {
		cd.rxWin = k.RxWin
	}
	cd.lastSentPacketSeq = k.TxPacket
	cd.lastSentMessageSeq = k.TxMessage
	cd.clientLastReceivedMessage = k.Acked
	cd.frags.restoreLocked(k.Fragments, cd.lastReceivedMessageSeq, time.Now())
	cd.mu.Unlock()

	cd.objMu.Lock()
	n := max(len(k.Objects), k.Slots)
	cd.growEntitySlots(ClampEntitySlots(n, MaxEntitySlots))
	if k.Slots > 0 {
		cd.slots = ClampEntitySlots(k.Slots, MaxEntitySlots)
	}
	if k.View > 0 {
		cd.view = ClampViewDistance(k.View, MaxViewDistance)
	}
	for i, o := range k.Objects {
		if i < len(cd.serverObjects) {
			cd.serverObjects[i].restore(o, true)
		}
	}
	cd.objMu.Unlock()
	cd.clientStatUpdate.restore(k.Stat, false)
	cd.clientBuffUpdate.restore(k.Buff, false)
	cd.serverGroupUpdate.restore(k.Group, false)
	cd.client.restore(k.Client)
}

// restore sets the channel to what was kept, as a client holds it after a
// restart: its next message numbered past every one sent, and going out
// WHOLE (refnum 0: nothing it acknowledged is known here). What it holds goes
// out again at once when it describes something (the client keeps what it had,
// and a whole value of the same entity is an update to it), and so does a
// removal still to go (an object channel whose flag byte is 0, dirty). A
// removal that went out is NOT sent again: CLIENT's ChannelEntity builds an
// entity on an empty slot whatever the flag (ConnectionData.ResendAll), so a
// second one would make the entity it took away come back.
func (s *StateUpdateData) restore(k KeptChannel, object bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k.Next != 0 {
		s.messageCounter = k.Next
	}
	if k.Data != nil {
		s.data = append([]byte(nil), k.Data...)
	}
	s.sent, s.acked, s.ackSeq = nil, false, 0
	s.sentOnce, s.lastSentSeq = false, 0
	s.isActive = k.Active
	s.resendTicks = 0
	removed := object && (len(s.data) == 0 || s.data[0] == 0)
	if removed && !k.Dirty {
		// gone from the client already (or never shown): nothing to say
		s.baseXOR = append([]byte(nil), s.data...)
		s.dirty = false
		return
	}
	s.baseXOR = make([]byte, len(s.data))
	s.dirty = k.Active
}

func (c *ClientStateData) restore(k KeptClient) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SeqNum, c.took = k.Seq, k.Took
	if len(k.Data) > 0 {
		c.data = append([]byte(nil), k.Data...)
	}
	c.bases = make(map[uint16][]byte, len(k.Bases))
	for _, b := range k.Bases {
		c.bases[b.Seq] = append([]byte(nil), b.Data...)
	}
}

// restore puts the kept reliable messages back: the unacknowledged ones due a
// resend at once (the client may have had none of them since the restart
// began), the pending ones queued as they were. A message in both lists is one
// message in both queues, as AddWithSequence puts it.
//
// Each keeps its number. Every reliable message here took one at its queueing
// (AddWithSequence), and PreparePacket keeps a number only on a message whose
// Queued stamp says it has one: since 2119f18 (2026-11-09 05:20 UTC, which
// made 0 a number like any other), a restored message had no stamp, so it went
// out under the next new number, and its own was sent no more. The client,
// holding every message past one it never had, took nothing reliable again,
// not a chat line, a target's con or a window, while the creatures moved on
// their state channels, until its player logged in again; one it had came
// twice. 0 is a number like any other here too.
func (q *MessageQueue) restore(unacked, pending []KeptMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	bySeq := make(map[uint16]*protocol.Message, len(unacked))
	for _, k := range unacked {
		m := restoredMessage(k, now)
		q.reliable = append(q.reliable, m)
		bySeq[k.Seq] = m
	}
	for _, k := range pending {
		m, ok := bySeq[k.Seq]
		if !ok {
			m = restoredMessage(k, now)
		}
		m.LastSent = m.Queued // queued, never sent, as AddWithSequence leaves one
		q.pending = append(q.pending, m)
	}
}

// restoredMessage is a kept message as the queue holds one: numbered (Queued
// set, at the restore) and never sent since (LastSent zero: due at once).
func restoredMessage(k KeptMessage, now time.Time) *protocol.Message {
	m := &protocol.Message{Type: protocol.MessageType(k.Type), Opcode: protocol.GameOpcode(k.Opcode), Sequence: k.Seq, Created: now, Queued: now}
	m.SetData(k.Data)
	return m
}

// unixOfTimer is the wall-clock time, in Unix milliseconds, of a reading of
// this process's timer (timerAt's inverse, to the millisecond).
func unixOfTimer(t int64) int64 {
	return clockBase.Add(time.Duration(t*100000/98304) * time.Millisecond).UnixMilli()
}

// TakeReliable records the client's reliable message seq on a session that
// came through a restart (Resumed) and reports whether it is new: one already taken is a copy the
// client sent again (it resends what it has not seen acknowledged, every
// second through a restart), acknowledged again but not handled a second time.
// The window is the last 64 numbers below the newest; an older one counts as
// taken. Messages may arrive out of order (a packet each goroutine), so one
// below the newest that was never taken is new. Sequence numbers compare as
// connection.cpp's do (the signed 16-bit difference). 0 is never recorded.
func (cd *ConnectionData) TakeReliable(seq uint16) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.takeLocked(seq)
}

// resumeDump is when a resumed session's client is sent the MemoryDump that
// brings its copy of the character in line with this process's (the
// listener's resume, udp/keep.go): soon after its first report, which moved
// the character to where the client has it, else at the latest.
type resumeDump struct {
	at      time.Time // the latest it goes
	pending bool
	dead    bool // its character was dead when the restart came: it respawns
}

// SetResumeDump marks the session's resume dump due at the latest at at;
// dead, its character respawns with it.
func (s *Session) SetResumeDump(at time.Time, dead bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resume = resumeDump{at: at, pending: true, dead: dead}
}

// NoteResumeReport moves a pending resume dump to soon after now: the client
// reported where it is (its first 0x40 since the resume), and a few more of
// the reports it sent while the server was away are on their way.
func (s *Session) NoteResumeReport(now time.Time, after time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resume.pending && now.Add(after).Before(s.resume.at) {
		s.resume.at = now.Add(after)
	}
}

// TakeResumeDue reports, once, that the session's resume dump is due now, and
// whether its character respawns with it.
func (s *Session) TakeResumeDue(now time.Time) (due, dead bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.resume.pending || now.Before(s.resume.at) {
		return false, false
	}
	s.resume.pending = false
	return true, s.resume.dead
}

// Resumed is whether the session came through a restart: its client's
// reliable messages are taken once each (TakeReliable), as copies of them
// arrive with it.
func (s *Session) Resumed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resumed
}

// ResumePending is whether the session waits for its resume dump.
func (s *Session) ResumePending() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resume.pending
}

// rebuiltBeforeCRCs are the slots rebuilt in the release that began keeping CRCs (2026-11-11: world 13, the
// homes, moved from Tunaria zone 100 to zone 55's rim): a snapshot of the server before it (no CRCs) cannot tell, so
// these are not held after it.
var rebuiltBeforeCRCs = map[uint8]bool{13: true}

// ServerWorldCRC is the CRC of the server's own world in a slot as this process has it built (handlers/serverworlds.go
// sets it with the worlds it loads, before the kept sessions are put back); nil, or ok false, for none.
var ServerWorldCRC func(slot uint8) (crc uint32, ok bool)

// serverWorldCRCs is the CRC of each slot as built now (0: not known).
func serverWorldCRCs(slots []uint8) []uint32 {
	if len(slots) == 0 || ServerWorldCRC == nil {
		return nil
	}
	out := make([]uint32, len(slots))
	for i, w := range slots {
		out[i], _ = ServerWorldCRC(w)
	}
	return out
}

// stillHeld is the slots a kept client holds after the restart: every slot kept, but one whose world was rebuilt
// (kept with one CRC, built now with another): the client's file of it is the old one, and sending it there would
// put it in a world it does not have (the homes moved to a mountain top, 2026-11-11). A snapshot without CRCs (a
// server from before) keeps every slot.
func stillHeld(slots []uint8, crcs []uint32) []uint8 {
	out := make([]uint8, 0, len(slots))
	for i, w := range slots {
		if len(crcs) == 0 && rebuiltBeforeCRCs[w] {
			log.Printf("Keep: the client held world %d, rebuilt since its snapshot (kept without CRCs): not held until it builds it again (its next login)", w)
			continue
		}
		if i < len(crcs) && crcs[i] != 0 && ServerWorldCRC != nil {
			if now, ok := ServerWorldCRC(w); ok && now != crcs[i] {
				log.Printf("Keep: the client held world %d as CRC %08x, built now as %08x: not held until it builds it again (its next login)", w, crcs[i], now)
				continue
			}
		}
		out = append(out, w)
	}
	return out
}

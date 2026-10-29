package udp

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

const (
	// MaxPacketSize is the maximum UDP packet size.
	MaxPacketSize = 0x530
	// ReadBufferSize is the UDP socket read buffer size.
	ReadBufferSize = 1024 * 1024
	// WriteBufferSize is the UDP socket write buffer size.
	WriteBufferSize = 1024 * 1024
)

// Listener handles UDP network communication.
type Listener struct {
	conn           *net.UDPConn
	addr           *net.UDPAddr
	cfg            *config.Config
	sessionManager *session.Manager
	handlers       *HandlerRegistry
	gameWorld      *entity.World
	charStore      *database.SQLiteCharacterStore

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Packet buffer pool
	bufferPool sync.Pool
}

// SetCharStore sets the character store for position persistence on disconnect.
func (l *Listener) SetCharStore(store *database.SQLiteCharacterStore) {
	l.charStore = store
}

// NewListener creates a new UDP listener.
func NewListener(cfg *config.Config, sessionManager *session.Manager, handlers *HandlerRegistry, gameWorld *entity.World) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	return &Listener{
		cfg:            cfg,
		sessionManager: sessionManager,
		handlers:       handlers,
		gameWorld:      gameWorld,
		ctx:            ctx,
		cancel:         cancel,
		bufferPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, MaxPacketSize)
				return &buf
			},
		},
	}
}

// Start begins listening for UDP packets.
func (l *Listener) Start() error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(l.cfg.Server.BindAddress),
		Port: l.cfg.Server.GamePort,
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	// Set socket buffer sizes
	if err := conn.SetReadBuffer(ReadBufferSize); err != nil {
		log.Printf("Warning: failed to set read buffer size: %v", err)
	}
	if err := conn.SetWriteBuffer(WriteBufferSize); err != nil {
		log.Printf("Warning: failed to set write buffer size: %v", err)
	}

	l.conn = conn
	l.addr = addr

	log.Printf("UDP listener started on %s:%d", l.cfg.Server.BindAddress, l.cfg.Server.GamePort)

	// Start read goroutine
	l.wg.Add(1)
	go l.readLoop()

	return nil
}

// Stop gracefully shuts down the listener.
func (l *Listener) Stop() {
	l.cancel()
	if l.conn != nil {
		l.conn.Close()
	}
	l.wg.Wait()
}

// readLoop continuously reads packets from the UDP socket.
func (l *Listener) readLoop() {
	defer l.wg.Done()

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		// Get buffer from pool
		bufPtr := l.bufferPool.Get().(*[]byte)
		buf := *bufPtr

		// Set read deadline
		l.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, remoteAddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			// Return buffer to pool
			l.bufferPool.Put(bufPtr)

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("UDP read error (non-timeout): %v", err)

			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("UDP read error: %v", err)
				continue
			}
		}

		// Copy data and return buffer to pool
		packetData := make([]byte, n)
		copy(packetData, buf[:n])
		l.bufferPool.Put(bufPtr)

		log.Printf("UDP received %d bytes from %s", n, remoteAddr)
		log.Printf("UDP raw hex: %x", packetData)

		// Process packet in a goroutine
		l.wg.Add(1)
		go func(data []byte, addr *net.UDPAddr) {
			defer l.wg.Done()
			l.handlePacket(data, addr)
		}(packetData, remoteAddr)
	}
}

// handlePacket processes a received packet.
func (l *Listener) handlePacket(data []byte, addr *net.UDPAddr) {
	// Parse the packet
	packet, err := ParsePacket(data)
	if err != nil {
		log.Printf("Failed to parse packet from %s: %v", addr, err)
		return
	}

	log.Printf("handlePacket: endpoint=0x%04X, instanceID=0x%X, hasBody=%v, isNew=%v",
		packet.RemoteEndpoint, packet.GetInstanceID(), packet.Body != nil, packet.IsNewConnection())

	// Handle ResetConnection: remove session and stop processing
	if packet.Header.Flags&protocol.SegmentFlagResetConnection != 0 && packet.Body == nil {
		instID := packet.GetInstanceID()
		if instID != 0 {
			// Specific instance — remove that exact session
			if sess := l.sessionManager.GetSessionByAddr(addr, instID); sess != nil {
				log.Printf("handlePacket: ResetConnection from %s, removing session (instance=0x%X)", addr, instID)
				l.sessionManager.RemoveSession(sess)
			}
		} else {
			// instanceID=0: the client is resetting its old login session.
			// Match by address + server endpoint (0xFFFE=login vs 0xB991=game) to find the right session.
			// IMPORTANT: Never match server-initiated (master) sessions — the client's reset targets
			// the old auth/login session, not the promoted master session.
			serverEndpoint := packet.RemoteEndpoint
			addrStr := addr.String()
			var found *session.Session
			l.sessionManager.ForEach(func(s *session.Session) bool {
				if s.RemoteAddr().String() == addrStr && s.ServerEndpoint() == serverEndpoint && !s.InGame() && !s.DidServerInitiate() {
					found = s
					return false
				}
				return true
			})
			if found != nil {
				log.Printf("handlePacket: ResetConnection from %s, removing session (instance=0x%X, serverEP=0x%04X)", addr, found.InstanceID(), serverEndpoint)
				l.sessionManager.RemoveSession(found)
			} else {
				log.Printf("handlePacket: ResetConnection from %s (instanceID=0, serverEP=0x%04X), no matching session", addr, serverEndpoint)
			}
		}
		return
	}

	// Get or create session
	sess := l.sessionManager.GetOrCreateSession(addr, packet)
	if sess == nil {
		log.Printf("Failed to get/create session for %s", addr)
		return
	}

	log.Printf("handlePacket: session instance=0x%X, didServerInit=%v, remoteEndpoint=0x%04X",
		sess.InstanceID(), sess.DidServerInitiate(), packet.RemoteEndpoint)

	// Update server endpoint if client targets a specific endpoint (not FFFE new connection)
	// This detects when client connects to game server (0xB991) vs login server
	if packet.RemoteEndpoint != 0xFFFE && packet.RemoteEndpoint != 0 {
		log.Printf("handlePacket: updating server endpoint to 0x%04X", packet.RemoteEndpoint)
		sess.SetServerEndpoint(packet.RemoteEndpoint)
		// NOTE: didServerInitiate is set later when character enters world (SendMemoryDump)
		// NOT when they first connect to 0xB991 - this matches C# CreateMemoryDumpSession
	}

	// Set send function if not already set
	if sess.Connection() != nil {
		sess.SetSendFunc(func(data []byte) error {
			return l.SendTo(data, addr)
		})
	}

	// Update session activity
	sess.UpdateActivity()

	// Process the packet through RDP handler (handles ACKs and triggers callbacks)
	rdpHandler := NewRDPHandler(l)
	rdpHandler.ProcessIncoming(sess, packet)

	// For new connections or any received packet, always send a response
	// This ensures the client knows we received their packet
	if packet.IsNewConnection() || sess.HasInstance() {
		sess.Queue().SetBodyFlags(protocol.BodyFlagRdpMessage)
	}

	// Dispatch messages to handlers (check for nil body)
	if packet.Body != nil && packet.Body.Messages != nil {
		conn := sess.Connection()
		for _, msg := range packet.Body.Messages {
			// Track reliable message sequence numbers for ACKing
			if msg.Type.IsReliable() && msg.Sequence > 0 {
				conn.SetLastReceivedMessageSeq(msg.Sequence)
				// Set flag to ACK the reliable message back to client
				sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck | protocol.BodyFlagFlushAck)
			}

			// Handle ping messages - respond with ACK (already handled above, but continue)
			if msg.Type == protocol.MessageTypePing {
				continue
			}

			// Handle state channel messages directly (position updates, etc.)
			// Only a message that decodes is taken and acknowledged, and an old
			// one is a duplicate, acknowledged again but not applied (a late
			// position put the player back), as connection.cpp's receive does.
			// Acknowledging one whose base was missing made the client XOR its
			// next against a message never stored here.
			if msg.Type.IsStateChannel() {
				clientState := conn.GetClientState()
				sess.Queue().SetBodyFlags(protocol.BodyFlagClientUpdateAck)

				// Process channel 0x40 (ClientUpdate) - contains position data
				if msg.Type == protocol.MessageTypeClientUpdate && len(msg.Data()) > 0 &&
					clientState.Newer(msg.Sequence) && l.processClientUpdate(sess, msg) {
					clientState.Took(msg.Sequence)
				}
				continue
			}

			// The character pick promotes the session, which clears the queue and
			// restarts the sequence space. Acknowledge the pick on the LOBBY
			// connection first, as Sandstorm does (pcaps/ps2_sandstorm.pcap: the
			// PS2 answers the promotion with a reset of its lobby connection
			// 15 ms later). Unacknowledged, the client's lobby connection never
			// gets in sync, so CLIENT's connection_close leaves it alive, and it
			// keeps resending 0x002A under this same instance: its arrival
			// numbers become the promoted session's, the promoted connection's
			// own packets are dropped as old, and the client rejects every ack
			// that comes back (the decomp's "acking a segment that hasn't been
			// sent"). Found by eqoa-rs's DRDP, which is the decomp's.
			if msg.Type.IsReliable() && msg.Opcode == protocol.OpcodeSelectedChar && sess.PlayerObjectID() == 0 {
				l.flushSession(sess)
			}

			if l.handlers != nil {
				l.handlers.Handle(sess, msg)
			}
		}
	}

	// Flush any queued messages immediately
	l.flushSession(sess)
}

// flushSession sends any queued messages for a session.
// NOTE: Do NOT call GenerateStateUpdates here — state updates are generated
// exclusively by the tick goroutine (tickAllSessions → TickSession) to avoid
// race conditions between packet handler goroutines and the tick goroutine.
// This function only flushes already-queued messages (ACKs, reliable messages).
func (l *Listener) flushSession(sess *session.Session) {
	rdp := NewRDPHandler(l)

	if data := rdp.PreparePacket(sess); data != nil {
		if err := l.SendToSession(data, sess); err != nil {
			log.Printf("Failed to flush session %s: %v", sess.RemoteAddr(), err)
		} else {
			log.Printf("Flushed %d bytes to %s", len(data), sess.RemoteAddr())
		}
	}
}

// SendTo sends a packet to a specific address.
func (l *Listener) SendTo(data []byte, addr *net.UDPAddr) error {
	_, err := l.conn.WriteToUDP(data, addr)
	return err
}

// SendToSession sends a packet to a session's address.
func (l *Listener) SendToSession(data []byte, sess *session.Session) error {
	return l.SendTo(data, sess.RemoteAddr())
}

// StartTickRoutine starts a background goroutine that ticks all sessions periodically.
// This ensures keepalives and state updates are sent even when the client is silent.
func (l *Listener) StartTickRoutine(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			select {
			case <-ticker.C:
				l.tickAllSessions()
			case <-done:
				ticker.Stop()
				return
			case <-l.ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}

// tickAllSessions runs ECS systems and then ticks all active sessions.
func (l *Listener) tickAllSessions() {
	// Run ECS tick: process commands, AI, respawn, regen, state sync
	if l.gameWorld != nil {
		l.gameWorld.Tick(time.Now())
	}

	// Send packets for all sessions
	rdp := NewRDPHandler(l)

	// Collect sessions to remove (can't modify map during iteration)
	var toRemove []*session.Session

	l.sessionManager.ForEach(func(sess *session.Session) bool {
		// Clean up sessions flagged for termination (ping timeout, disconnect)
		if sess.PendingTermination() {
			toRemove = append(toRemove, sess)
			return true
		}

		// Another login took this session's character: it only delivers the
		// disconnect it was queued, then goes. Its Player is no longer the
		// world's, so it gets no full tick (state updates, timeouts).
		if age, replaced := sess.ReplacedFor(); replaced {
			if age >= session.ReplacedGrace {
				toRemove = append(toRemove, sess)
				return true
			}
			if data := rdp.PreparePacket(sess); data != nil {
				if err := rdp.SendPacket(sess, data); err != nil {
					log.Printf("Failed to send disconnect to replaced session %s: %v", sess.RemoteAddr(), err)
				}
			}
			return true
		}

		if sess.CharacterInWorld() {
			// Full tick: timeout checks, state updates, packet send
			rdp.TickSession(sess)
		} else if sess.InGame() {
			// Session B (MemoryDump session) has messages queued before CharacterInWorld.
			// Just send pending packets without full timeout/ping/state-update logic.
			if data := rdp.PreparePacket(sess); data != nil {
				if err := rdp.SendPacket(sess, data); err != nil {
					log.Printf("Failed to send initial packet to %s: %v", sess.RemoteAddr(), err)
				}
			}
		}
		return true
	})

	// Remove terminated sessions and their ECS entities
	for _, sess := range toRemove {
		log.Printf("Cleaning up terminated session %s (instance=0x%X, objectID=%d)",
			sess.RemoteAddr(), sess.InstanceID(), sess.PlayerObjectID())
		if l.gameWorld != nil && sess.CharacterInWorld() {
			// Save player position, spell effects and standings to database before removing
			if l.charStore != nil {
				l.savePlayerPosition(sess)
				l.saveEffects(sess)
				l.saveFactions(sess)
				l.saveMastery(sess)
				l.saveRez(sess)                // rezsave.go
				l.saveLogout(sess, time.Now()) // norentsave.go
			}
			l.gameWorld.RemovePlayerBySession(sess)
		}
		l.sessionManager.RemoveSession(sess)
	}
}

// saveEffects keeps the player's spell effects over the logout, what is left
// of each (entity/buffsave.go): a session whose character logged in again
// holds the OLD Player, whose effects the new login already kept.
func (l *Listener) saveEffects(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	rows, ok := l.gameWorld.SavedEffects(player.ServerID, time.Now())
	if !ok {
		return
	}
	if err := l.charStore.SaveBuffs(player.CharServerID, rows); err != nil {
		log.Printf("saveEffects: '%s' %d effects: %v", player.Name, len(rows), err)
	} else if len(rows) > 0 {
		log.Printf("saveEffects: '%s' keeps %d spell effects", player.Name, len(rows))
	}
}

// saveFactions keeps the player's standings over the logout
// (entity/faction.go): as saveEffects, an old session's Player is not saved
// (the new login kept its standings).
func (l *Listener) saveFactions(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	m, ok := l.gameWorld.SavedFactions(player.ServerID)
	if !ok {
		return
	}
	if err := l.charStore.SaveFactions(player.CharServerID, m); err != nil {
		log.Printf("saveFactions: '%s' %d standings: %v", player.Name, len(m), err)
	} else if len(m) > 0 {
		log.Printf("saveFactions: '%s' keeps its standing with %d factions", player.Name, len(m))
	}
}

// saveMastery keeps the player's mastery points and their share over the
// logout (entity/mastery.go), as saveFactions keeps standings.
func (l *Listener) saveMastery(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	m, ok := l.gameWorld.MasteryOf(player.ServerID)
	if !ok {
		return
	}
	row := database.MasteryRow{Points: m.Points, Spent: m.Spent, Percent: m.Percent, XP: m.XP}
	if err := l.charStore.SaveMastery(player.CharServerID, row); err != nil {
		log.Printf("saveMastery: '%s': %v", player.Name, err)
	}
}

// SaveAll saves every player in the world as a logout would (position, spell
// effects, standings and mastery points), for a shutdown: a restart is a logout for everyone
// in it, and before this one the state since each login was lost.
// NoticeShutdown tells every client (each session is a game connected, in
// the world or before it) that the server is going: 0x07C0, CLIENT's Disconnect(17), "The server has been shutdown by a
// server administrator." (ConnectStatusString's word 1024), so the game goes
// back to its login with that notice. Without it a restart left each game in a
// world that no longer answered until its own timeout, every creature carried
// along its last word (F3 20261026-063102: a pet ran on into the sea). Flushed
// at once, and again past RetransmitDelay for a datagram that was lost, as
// nothing would resend it once the process ends. It returns how many were told.
func (l *Listener) NoticeShutdown() int {
	var told []*session.Session
	l.sessionManager.ForEach(func(sess *session.Session) bool {
		told = append(told, sess)
		return true
	})
	for _, sess := range told {
		handlers.SendDisconnect(sess, protocol.OpcodeServerUpdatingDC)
		l.flushSession(sess)
	}
	if len(told) > 0 {
		time.Sleep(RetransmitDelay + 100*time.Millisecond)
		for _, sess := range told {
			l.flushSession(sess)
		}
	}
	log.Printf("NoticeShutdown: %d clients told", len(told))
	return len(told)
}

func (l *Listener) SaveAll() {
	if l.gameWorld == nil || l.charStore == nil {
		return
	}
	var inWorld []*session.Session
	l.sessionManager.ForEach(func(sess *session.Session) bool {
		if sess.CharacterInWorld() {
			inWorld = append(inWorld, sess)
		}
		return true
	})
	for _, sess := range inWorld {
		l.savePlayerPosition(sess)
		l.saveEffects(sess)
		l.saveFactions(sess)
		l.saveMastery(sess)
		l.saveRez(sess)                // rezsave.go
		l.saveLogout(sess, time.Now()) // norentsave.go
	}
	log.Printf("SaveAll: %d players saved", len(inWorld))
}

// savePlayerPosition saves the player's current position to the database on disconnect.
func (l *Listener) savePlayerPosition(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil {
		return
	}
	// A session whose character logged in again holds the OLD Player, no
	// longer the world's: saving it wrote the stale window's position over
	// the live one's.
	if !l.gameWorld.IsLivePlayer(player) {
		log.Printf("savePlayerPosition: '%s' was replaced by a newer login, not saving the old session's position",
			player.Name)
		return
	}

	// The character's row by its own serverid: the Player's ServerID is its
	// entity id, another number when the character's collided with an NPC's
	// (CreatePlayer).
	record, err := l.charStore.GetByServerID(player.CharServerID)
	if err != nil {
		log.Printf("savePlayerPosition: failed to load record for serverID=%d: %v", player.CharServerID, err)
		return
	}

	// The level and the experience into it, as the game holds them, whatever
	// becomes of the place: until 2026-10-25 nothing wrote them, so every
	// level gained, and a level a GM set, was gone at the next login.
	record.Level = player.Level
	record.TotalXP = entity.TotalXP(player.Level, player.XPInLevel)
	record.Debt = player.Debt // what experience has not yet paid (awardXP)
	record.UnusedTP, record.TotalTP = player.UnusedTP, player.TotalTP

	// A death's place is not where the next login starts: the respawn would
	// have moved the player beside Vaal 15 s later (entity/respawn.go), and the
	// place it died may be the lava or the deep water that killed it. The row
	// keeps the last place it logged out alive.
	if player.IsDead() {
		log.Printf("savePlayerPosition: '%s' logged out dead at (%.1f, %.1f, %.1f); "+
			"keeping the position saved before", player.Name, player.X, player.Y, player.Z)
		if err := l.charStore.Update(record); err != nil {
			log.Printf("savePlayerPosition: failed to save level %d for '%s': %v", player.Level, player.Name, err)
		}
		return
	}

	prevY := record.Y // the last height that was written, before we overwrite it

	// In the air (a fall: the last 0x40's walker flags), the last place it
	// stood, so no login begins in mid-air (entity.SavePlace).
	x, y, z, back := player.SavePlace()
	if back {
		log.Printf("savePlayerPosition: '%s' logged out in the air at (%.1f, %.1f, %.1f); "+
			"saving where it last stood (%.1f, %.1f, %.1f)", player.Name, player.X, player.Y, player.Z, x, y, z)
	}
	record.X = x
	record.Y = y
	record.Z = z
	record.Facing = player.Facing

	// A FALL IS NOT A POSITION. Walking off terrain drops a character to the
	// world floor at y = -1000 (see the world-9 note in entity/types.go), and
	// saving that makes the next login spawn there and fall again — the
	// character is stranded permanently by one bad step. Two rows in this
	// database were already sitting at exactly -1000 when this was written.
	//
	// The X/Z are kept: they are where the player actually was, and the world
	// check below still gets to judge them. Only the height is refused, and it
	// falls back to the last one that was not a fall, which is the height they
	// were standing at before they went over the edge.
	//
	// Same shape as the CoherentWorld guard below, for the same reason: it is
	// better not to persist the bad value than to detect it forever afterwards.
	if safe, ok := entity.SafeSaveHeight(y, prevY); !ok {
		log.Printf("savePlayerPosition: '%s' is at y=%.1f, at or under the world "+
			"floor — keeping the stored height %.1f rather than saving a fall",
			player.Name, y, safe)
		record.Y = safe
	}
	// The world must be saved WITH the position or the pair is incoherent:
	// logging out in guk and back in would place the player at guk's
	// coordinates inside Tunaria, under the map.
	//
	// ...but only if the two AGREE. A cross-world teleport commits its world
	// index immediately (nothing acknowledges the arrival), so a world that
	// failed to load leaves the player reporting their old position under the
	// new index. Writing that pair strands the character: visibility filters to
	// a world they are not in and no NPC ever appears. Caught at load too, but
	// it is better not to persist the bad pair in the first place.
	worldToSave, ok := entity.CoherentWorld(player.World, player.X, player.Z)
	if !ok {
		log.Printf("savePlayerPosition: '%s' is world %d at (%.1f, %.1f), which "+
			"that world does not contain — saving world %d instead",
			player.Name, player.World, player.X, player.Z, worldToSave)
	}
	record.World = int(worldToSave)

	if err := l.charStore.Update(record); err != nil {
		log.Printf("savePlayerPosition: failed to save position for '%s': %v", player.Name, err)
	} else {
		log.Printf("savePlayerPosition: saved position for '%s' (%.1f, %.1f, %.1f) world=%d, level %d (%d xp into it)",
			player.Name, player.X, player.Y, player.Z, player.World, player.Level, player.XPInLevel)
	}
}

// processClientUpdate handles an incoming state channel 0x40 message.
// Reconstructs the full state via XOR delta, extracts position, and queues an ECS command.
// It reports whether the message decoded: false when its base is not held.
func (l *Listener) processClientUpdate(sess *session.Session, msg *protocol.Message) bool {
	clientState := sess.Connection().GetClientState()
	decoded := msg.Data()

	// XOR reconstruction: if XORByte != 0, XOR against base at (Sequence - XORByte)
	if msg.XORByte != 0 {
		baseSeq := msg.Sequence - uint16(msg.XORByte)
		base := clientState.GetBase(baseSeq)
		if base != nil {
			// Reconstructed length = max(delta, base) to preserve unchanged trailing bytes
			maxLen := len(decoded)
			if len(base) > maxLen {
				maxLen = len(base)
			}
			reconstructed := make([]byte, maxLen)
			for i := 0; i < maxLen; i++ {
				var d, b byte
				if i < len(decoded) {
					d = decoded[i]
				}
				if i < len(base) {
					b = base[i]
				}
				reconstructed[i] = d ^ b
			}
			decoded = reconstructed
		} else {
			log.Printf("processClientUpdate: missing XOR base for seq %d (baseSeq=%d), skipping", msg.Sequence, baseSeq)
			return false
		}
	}

	// Store as base for future XOR deltas
	clientState.AddBase(msg.Sequence, decoded)

	// Extract position from channel 0x40 data
	// C# Unreliables.ProcessUnreliableClientUpdate:
	// Bytes 1-3: X (uint24 BE) → 36000 * val / 0xFFFFFF - 4000
	// Bytes 4-6: Y (uint24 BE) → 2000 * val / 0xFFFFFF - 1000
	// Bytes 7-9: Z (uint24 BE) → 36000 * val / 0xFFFFFF - 4000
	// Byte 22: Facing → float32(byte) / 256.0 * 2 * PI
	if len(decoded) < 23 {
		return true
	}

	rawX := uint32(decoded[1])<<16 | uint32(decoded[2])<<8 | uint32(decoded[3])
	rawY := uint32(decoded[4])<<16 | uint32(decoded[5])<<8 | uint32(decoded[6])
	rawZ := uint32(decoded[7])<<16 | uint32(decoded[8])<<8 | uint32(decoded[9])

	posX := float32(36000.0*float64(rawX)/float64(0xFFFFFF) - 4000.0)
	posY := float32(2000.0*float64(rawY)/float64(0xFFFFFF) - 1000.0)
	posZ := float32(36000.0*float64(rawZ)/float64(0xFFFFFF) - 4000.0)

	facing, hasFacing := entity.ClientUpdateFacing(decoded)

	var animByte byte
	if len(decoded) > 35 {
		animByte = decoded[35]
	}
	log.Printf("ClientPosition: seq=%d xor=%d pos=(%.1f, %.1f, %.1f) facing=%.2f anim=0x%02X decodedLen=%d from %s",
		msg.Sequence, msg.XORByte, posX, posY, posZ, facing, animByte, len(decoded), sess.RemoteAddr())

	// Trigger world entry on first state channel 0x40 (matches C# Unreliables.cs)
	if !sess.CharacterInWorld() {
		log.Printf("first state channel from %s - entering world", sess.RemoteAddr())
		sess.SetCharacterInWorld(true)

		// Always use server-side saved position on world entry.
		// Never trust the client's first 0x40 — prevents teleport exploits
		// via PNACH cheats or modified clients spoofing coordinates.
		if l.gameWorld != nil {
			player := l.gameWorld.SessionPlayer(sess)
			if player != nil {
				log.Printf("WORLD ENTRY: ignoring client position (%.0f,%.0f,%.0f), using saved (%.0f,%.0f,%.0f) for %s",
					posX, posY, posZ, player.X, player.Y, player.Z, player.Name)
				posX = player.X
				posY = player.Y
				posZ = player.Z
			}
		}
	}

	// The client is moving again, so it is alive in the scene the teleport sent
	// it to — re-describe that scene. A world load stops the 0x40 stream dead,
	// so the first one after a teleport is the arrival, and retail's own resync
	// rides in the same packet. Doing this as well as on 0x0049 is what makes
	// the re-sync independent of which 0x0049 was the real one; see
	// Session.ConsumeWorldLoadPending.
	if sess.ConsumeWorldLoadPending() {
		if _, _, resend := session.WorldLoadLayers(); !resend {
			log.Printf("World load done (first 0x40 at %.0f,%.0f,%.0f): resend layer disabled, not re-arming for %s",
				posX, posY, posZ, sess.RemoteAddr())
		} else if conn := sess.Connection(); conn != nil {
			log.Printf("World load done (first 0x40 at %.0f,%.0f,%.0f): re-arming %d state channels for %s",
				posX, posY, posZ, conn.ResendAll(), sess.RemoteAddr())
		}
	}

	// Extract animation byte at offset 35 (C# Unreliables.cs: skip 12 after facing → Animation)
	var animation byte
	if len(decoded) > 35 {
		animation = decoded[35]
	}

	// Position, the walker's own velocity (bytes 10-15), the animation and
	// the auto-attack flag (byte 36), in one locked update: see
	// entity/clientupdate.go for why the velocity is read, not worked out.
	if l.gameWorld != nil {
		player := l.gameWorld.SessionPlayer(sess)
		if player != nil {
			reportWorldMismatch(sess, player, decoded[0])
			u := entity.ClientUpdate{X: posX, Y: posY, Z: posZ, Facing: facing, HasFacing: hasFacing, Animation: animation}
			u.Velocity, u.HasVelocity = entity.ClientUpdateVelocity(decoded)
			u.AutoAttack, u.HasAutoAttack = entity.ClientUpdateAutoAttack(decoded)
			u.WalkFlags, u.HasWalkFlags = entity.ClientUpdateWalkFlags(decoded)
			l.gameWorld.ApplyClientUpdate(player.ServerID, u)
		}
	}
	return true
}

// reportWorldMismatch logs when the client says it is in a different world than
// the one the server has it in. Byte 0 of the 0x40 update is the client's own
// world index — the PS2 sets it (a world change shows up as an XOR flip of this
// byte in pcaps/sandstorm-worldchange-20260807.pcap) and so does our Go client
// (game/engine.go buildPositionBuffer). We were reading bytes 1-9, 22 and 35 out
// of this buffer and never this one.
//
// It is only REPORTED, not adopted: the client is not a trustworthy source for
// its own location. But without it a world change that the client failed to
// perform is completely invisible to the server, which keeps the player in the
// destination world, keeps filtering visibility to it, and hands the client
// entities that belong to a world it is not in. That is how a world 9 NPC came
// to be standing in world 0 with nothing anywhere logging a complaint.
//
// CoherentWorld does not cover this. It asks whether the POSITION could exist in
// the recorded world, and worlds we lift out of Tunaria (world 9 is Tunaria zone
// 127) have coordinates that are equally legal in world 0 — so the pair stays
// self-consistent through the whole failure.
func reportWorldMismatch(sess *session.Session, player *entity.Player, clientWorld byte) {
	if clientWorld == player.World {
		sess.ClearWorldMismatch()
		return
	}
	if !sess.CharacterInWorld() {
		return
	}
	if !sess.NoteWorldMismatch(clientWorld) {
		return
	}
	log.Printf("WORLD MISMATCH: '%s' is world %d server-side but the client "+
		"reports world %d at (%.0f, %.0f, %.0f) — the world change did not "+
		"happen on the client, and every entity we send it now belongs to a "+
		"world it is not in",
		player.Name, player.World, clientWorld, player.X, player.Y, player.Z)
}

// HandlerRegistry maps opcodes to handler functions.
type HandlerRegistry struct {
	handlers map[protocol.GameOpcode]Handler
	security *handlers.SecurityMiddleware
	fallback *handlers.Registry
	mu       sync.RWMutex
}

// Handler is a function that handles a message.
type Handler func(sess *session.Session, msg *protocol.Message)

// NewHandlerRegistry creates a new handler registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[protocol.GameOpcode]Handler),
		security: handlers.NewSecurityMiddleware(),
	}
}

// SetFallback sets a fallback registry for opcodes not explicitly registered.
// This allows new handlers in handlers.Registry to work without manual bridging.
func (r *HandlerRegistry) SetFallback(fallback *handlers.Registry) {
	r.fallback = fallback
}

// Register adds a handler for an opcode.
func (r *HandlerRegistry) Register(opcode protocol.GameOpcode, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[opcode] = handler
}

// Handle dispatches a message to the appropriate handler.
func (r *HandlerRegistry) Handle(sess *session.Session, msg *protocol.Message) {
	r.mu.RLock()
	handler, ok := r.handlers[msg.Opcode]
	r.mu.RUnlock()

	if !ok {
		// Fall through to handler registry if set
		if r.fallback != nil {
			r.fallback.Handle(sess, msg)
			return
		}
		log.Printf("Unhandled opcode: 0x%04X (%s)", uint16(msg.Opcode), msg.Opcode.String())
		return
	}

	// Security middleware: rate limiting + validation
	if !r.security.Check(sess, msg.Opcode) {
		return
	}

	handler(sess, msg)
}

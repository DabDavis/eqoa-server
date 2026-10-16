package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// GameServerEndpoint is the endpoint ID for the game server.
// When client connects to this endpoint, they've selected a server.
const GameServerEndpoint = 0xB991

// AuthHandler handles authentication-related messages.
type AuthHandler struct {
	serverHandler    *ServerHandler
	characterHandler *CharacterHandler
	accountStore     *database.SQLiteAccountStore

	// Track IPs that have received the server list
	// Key: IP string, Value: time when server list was sent
	serverListSent   map[string]time.Time
	serverListSentMu sync.RWMutex

	// Track IPs that have received the character list (shared across sessions)
	// Key: IP string, Value: time when char list was sent
	charListSent   map[string]time.Time
	charListSentMu sync.RWMutex

	// loginTickets, when set, requires that a username have completed a TCP
	// login before the UDP handshake will accept it. Nil disables the check.
	loginTickets *auth.TicketStore
}

// SetLoginTickets requires a completed TCP login for each username seen in the
// UDP handshake.
//
// This closes a bypass rather than adding a feature. Authenticate and
// Authenticate2 read the username out of the packet and look the account up, so
// without this a client that never contacted the auth server could name any
// account -- which would make the auth server's password check pointless.
func (h *AuthHandler) SetLoginTickets(t *auth.TicketStore) {
	h.loginTickets = t
}

// resolveAccount looks up the account for an authenticated username.
//
// When the ticket check is enabled the account already exists -- the TCP login
// created it if needed -- so this is a plain lookup and no credential-less row
// can be produced here. With the check disabled it falls back to the old
// get-or-create so an operator who deliberately turned it off keeps the
// previous behaviour.
func (h *AuthHandler) resolveAccount(username string, ticketID int) (*database.AccountRecord, error) {
	if h.loginTickets == nil {
		return h.accountStore.GetOrCreate(username, "")
	}
	account, err := h.accountStore.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	if ticketID != 0 && account.ID != ticketID {
		// The ticket was granted for a different row than the one this lookup
		// found -- a username-normalisation mismatch, not something to paper over.
		log.Printf("Authenticate: username %q resolved to account %d but its login "+
			"ticket was for %d; using the ticket", username, account.ID, ticketID)
	}
	return account, nil
}

// authorizeUsername reports whether a username seen in the UDP handshake is
// backed by a recent TCP login, and the account ID that login resolved to.
func (h *AuthHandler) authorizeUsername(stage, username string) (int, bool) {
	if h.loginTickets == nil {
		return 0, true // check disabled
	}
	accountID, ok := h.loginTickets.Valid(username)
	if !ok {
		log.Printf("%s: REJECTED username %q -- no completed TCP login. "+
			"The client must authenticate on the auth port first; if this is a "+
			"legitimate client, its login may have expired (auth.ticket_ttl).",
			stage, username)
		return 0, false
	}
	return accountID, true
}

// NewAuthHandler creates a new auth handler.
// Deprecated: Use NewAuthHandlerFull instead.
func NewAuthHandler(serverHandler *ServerHandler) *AuthHandler {
	return &AuthHandler{
		serverHandler:    serverHandler,
		characterHandler: nil,
		accountStore:     nil,
		serverListSent:   make(map[string]time.Time),
		charListSent:     make(map[string]time.Time),
	}
}

// NewAuthHandlerWithCharHandler creates a new auth handler with an existing character handler.
// Deprecated: Use NewAuthHandlerFull instead.
func NewAuthHandlerWithCharHandler(serverHandler *ServerHandler, charHandler *CharacterHandler) *AuthHandler {
	return &AuthHandler{
		serverHandler:    serverHandler,
		characterHandler: charHandler,
		accountStore:     nil,
		serverListSent:   make(map[string]time.Time),
		charListSent:     make(map[string]time.Time),
	}
}

// NewAuthHandlerFull creates a new auth handler with all dependencies.
func NewAuthHandlerFull(serverHandler *ServerHandler, charHandler *CharacterHandler, accountStore *database.SQLiteAccountStore) *AuthHandler {
	return &AuthHandler{
		serverHandler:    serverHandler,
		characterHandler: charHandler,
		accountStore:     accountStore,
		serverListSent:   make(map[string]time.Time),
		charListSent:     make(map[string]time.Time),
	}
}

// RegisterHandlers registers auth handlers with the registry.
func (h *AuthHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDiscVersion, h.HandleDiscVersion)
	r.Register(protocol.OpcodeAuthenticate, h.HandleAuthenticate)
	r.Register(protocol.OpcodeAuthenticate2, h.HandleAuthenticate2)
}

// HandleDiscVersion handles disc/client version verification.
// Only responds once per session - subsequent DiscVersion messages are ignored
// since the client sends DiscVersion with every packet.
func (h *AuthHandler) HandleDiscVersion(sess *session.Session, msg *protocol.Message) error {
	// Only respond to DiscVersion if we haven't already (session is still connecting)
	// The client sends DiscVersion with every packet, so we must not flood responses
	if sess.State() != session.StateConnecting {
		return nil // Already responded, just ignore
	}

	data := msg.Data()
	if len(data) < 1 {
		log.Printf("DiscVersion: empty data from %s", sess.RemoteAddr())
		return nil
	}

	reader := buffer.NewReader(data)
	version, _ := reader.ReadByte()

	log.Printf("DiscVersion: client version 0x%02X from %s (first response)", version, sess.RemoteAddr())

	// Determine response based on version
	var responseVersion byte
	switch version {
	case protocol.GameVersionEQOAFrontiers:
		responseVersion = protocol.GameVersionEQOAFrontiers
	case protocol.GameVersionEQOAVanilla:
		responseVersion = protocol.GameVersionEQOAVanilla
	case protocol.GameVersionSandstormS2:
		// Answer with the DISC version, not an echo. 19 is a patched-client
		// marker S2 writes on the OUTGOING path (it rewrote `li $v0,18` at
		// CLIENT 0x00BC1EC8); the disc underneath is still Frontiers. Across all
		// five Sandstorm captures the server answers 0x12 to all 20 DiscVersion
		// messages and 0x13 never appears in either direction, so 0x12 is the
		// only reply with evidence behind it. Echoing 0x13 was tried and the
		// client still refused with string 719.
		responseVersion = protocol.GameVersionEQOAVanilla
	default:
		log.Printf("DiscVersion: UNKNOWN client version 0x%02X from %s — replying "+
			"Unknown, which makes the client report \"We are updating the game\"",
			version, sess.RemoteAddr())
		responseVersion = protocol.GameVersionUnknown
	}

	// Send version response - eqoa.live uses opcode 0x0000 with version as uint32 LE
	response := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDiscVersion)
	writer := buffer.NewWriter(response.Span())
	writer.WriteUint16(uint16(protocol.OpcodeDiscVersion)) // Opcode in data (C# reference format)
	writer.WriteUint32(uint32(responseVersion))            // Version as 4-byte uint32 LE (not just 1 byte)
	response.SetSize(writer.Position())

	sess.Queue().AddWithSequence(response)

	return nil
}

// HandleAuthenticate handles the main authentication request.
func (h *AuthHandler) HandleAuthenticate(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("Authenticate: insufficient data from %s", sess.RemoteAddr())
		return nil
	}

	reader := buffer.NewReader(data)

	// Parse authentication data based on captured packet format:
	// Offset 0-3: unknown
	// Offset 4-7: unknown
	// Offset 8-11: "EQOA"
	// Offset 12-15: username length (7-bit encoded?)
	// Offset 16+: username + encrypted password

	log.Printf("Authenticate: received %d bytes from %s", len(data), sess.RemoteAddr())
	log.Printf("Authenticate: raw hex: %x", data)

	// Try to find username in the data
	// Look for "EQOA" marker followed by username
	username := ""
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 'E' && data[i+1] == 'Q' && data[i+2] == 'O' && data[i+3] == 'A' {
			// Found EQOA marker, username follows
			if i+8 < len(data) {
				// Skip EQOA (4) + length prefix (4)
				nameStart := i + 8
				nameEnd := nameStart
				for nameEnd < len(data) && data[nameEnd] != 0 && data[nameEnd] >= 32 && data[nameEnd] < 127 {
					nameEnd++
				}
				if nameEnd > nameStart {
					username = string(data[nameStart:nameEnd])
				}
			}
			break
		}
	}

	// Also try to read game version
	gameVersion, _ := reader.ReadByte()

	// Get client IP (without port) for tracking
	clientIP := sess.RemoteAddr().IP.String()

	// Check if this client already received the server list
	h.serverListSentMu.RLock()
	serverListTime, hasSeenServerList := h.serverListSent[clientIP]
	h.serverListSentMu.RUnlock()

	// If server list was sent more than 5 minutes ago, treat as expired
	if hasSeenServerList && time.Since(serverListTime) > 5*time.Minute {
		hasSeenServerList = false
	}

	// If this is a brand new session (state=0), don't trust the IP-based hasSeenServerList flag
	// because it might be from a different account's previous session
	if sess.State() == session.StateConnecting && hasSeenServerList {
		log.Printf("Authenticate: new session but hasSeenServerList=true from IP tracking, clearing flag")
		h.serverListSentMu.Lock()
		delete(h.serverListSent, clientIP)
		h.serverListSentMu.Unlock()
		hasSeenServerList = false
	}

	log.Printf("Authenticate: game version=0x%02X, username='%s', serverEndpoint=0x%04X, state=%d, hasSeenServerList=%v",
		gameVersion, username, sess.ServerEndpoint(), sess.State(), hasSeenServerList)

	// Bind the session to its account -- but only if this username actually
	// logged in on the auth port. GetOrCreate used to run unconditionally with
	// an empty password, which both bypassed authentication and created
	// credential-less accounts.
	if username != "" && h.accountStore != nil && sess.AccountID() == 0 {
		ticketID, ok := h.authorizeUsername("Authenticate", username)
		if !ok {
			return nil
		}
		account, err := h.resolveAccount(username, ticketID)
		if err != nil {
			log.Printf("Authenticate: failed to resolve account for '%s': %v", username, err)
		} else {
			sess.SetAccountID(account.ID)
			log.Printf("Authenticate: account '%s' (ID=%d) logged in", username, account.ID)
		}
	}

	// Queue acknowledgment
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)

	// Determine connection type based on ENDPOINT primarily.
	// - 0xFFFE = Login server → send server list
	// - 0xB991 = Game server → send character list
	// - hasSeenServerList = Client already got server list, now connecting to game server
	//
	// Check if character list was already sent for this IP (shared across sessions)
	h.charListSentMu.RLock()
	charListTime, hasCharList := h.charListSent[clientIP]
	h.charListSentMu.RUnlock()
	if hasCharList && time.Since(charListTime) > 5*time.Minute {
		hasCharList = false
	}

	// If this is a brand new session (state=0), clear the hasCharList flag too
	// because it might be from a previous login attempt
	if sess.State() == session.StateConnecting && hasCharList {
		log.Printf("Authenticate: new session but hasCharList=true from IP tracking, clearing flag")
		h.charListSentMu.Lock()
		delete(h.charListSent, clientIP)
		h.charListSentMu.Unlock()
		hasCharList = false
	}

	// Note: Removed "client returned to server select" reset logic.
	// It was causing race conditions where server list was sent during character select flow.
	// If user needs to go back to server select, they should restart the client.

	// HandleAuthenticate (opcode 0x0904) only handles login server (0xFFFE) for server list.
	// Character select flow is handled by HandleAuthenticate2 (opcode 0x01) on game server (0xB991).
	// If we've seen the server list and client sends 0x0904 again, just ACK - don't interfere
	// with the character select flow happening on the game server session.
	if hasSeenServerList || hasCharList {
		log.Printf("Authenticate: already in character select flow (seenList=%v, hasCharList=%v), just ACKing",
			hasSeenServerList, hasCharList)
		return nil
	}

	// Only handle login server connection - send server list
	if sess.ServerEndpoint() == GameServerEndpoint {
		log.Printf("Authenticate: unexpected 0x0904 on game server endpoint, just ACKing")
		return nil
	}

	{
		// Login server connection (endpoint 0xFFFE) - send server list
		// But don't re-send if we already sent it
		if sess.State() >= session.StateAuthenticated {
			log.Printf("Authenticate: already sent server list, just ACKing")
			return nil
		}

		log.Printf("Authenticate: login server connection, sending server list")

		// Set session state
		sess.SetState(session.StateAuthenticated)

		if h.serverHandler != nil {
			h.serverHandler.SendServerList(sess)

			// Track that this IP has received the server list
			h.serverListSentMu.Lock()
			h.serverListSent[clientIP] = time.Now()
			h.serverListSentMu.Unlock()
		}
	}

	return nil
}

// HandleAuthenticate2 handles secondary authentication (game server connection).
// The client sends opcode 0x01 when connecting to the game server after selecting
// a server from the server list (vs 0x0904 for initial login server auth).
func (h *AuthHandler) HandleAuthenticate2(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	log.Printf("Authenticate2: received %d bytes from %s (game server auth)", len(data), sess.RemoteAddr())
	if len(data) > 0 {
		log.Printf("Authenticate2: raw hex: %x", data)
	}

	// Parse username from auth data (same format as Authenticate)
	username := ""
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 'E' && data[i+1] == 'Q' && data[i+2] == 'O' && data[i+3] == 'A' {
			log.Printf("Authenticate2: found EQOA marker at offset %d", i)
			if i+8 < len(data) {
				nameStart := i + 8
				nameEnd := nameStart
				for nameEnd < len(data) && data[nameEnd] != 0 && data[nameEnd] >= 32 && data[nameEnd] < 127 {
					nameEnd++
				}
				if nameEnd > nameStart {
					username = string(data[nameStart:nameEnd])
					log.Printf("Authenticate2: parsed username='%s' (bytes %d-%d)", username, nameStart, nameEnd)
				} else {
					log.Printf("Authenticate2: no username found after EQOA marker (nameStart=%d, nameEnd=%d)", nameStart, nameEnd)
				}
			} else {
				log.Printf("Authenticate2: insufficient data after EQOA marker (i=%d, len=%d)", i, len(data))
			}
			break
		}
	}
	if username == "" {
		log.Printf("Authenticate2: WARNING - no EQOA marker found or username empty!")
	}

	// Get or create account for this user (important for new sessions)
	log.Printf("Authenticate2: checking account lookup - username='%s', accountStore=%v, currentAccountID=%d",
		username, h.accountStore != nil, sess.AccountID())
	if username != "" && h.accountStore != nil && sess.AccountID() == 0 {
		ticketID, ok := h.authorizeUsername("Authenticate2", username)
		if !ok {
			return nil
		}
		account, err := h.resolveAccount(username, ticketID)
		if err != nil {
			log.Printf("Authenticate2: failed to resolve account for '%s': %v", username, err)
		} else {
			sess.SetAccountID(account.ID)
			log.Printf("Authenticate2: *** account '%s' (ID=%d) set on session ***", username, account.ID)
		}
	} else if sess.AccountID() > 0 {
		log.Printf("Authenticate2: session already has accountID=%d, not overwriting", sess.AccountID())
	}

	// Queue acknowledgment
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)

	// Get client IP for tracking
	clientIP := sess.RemoteAddr().IP.String()

	// Check if character list was already sent for this IP (shared across sessions)
	h.charListSentMu.RLock()
	charListTime, hasCharList := h.charListSent[clientIP]
	h.charListSentMu.RUnlock()
	if hasCharList && time.Since(charListTime) > 5*time.Minute {
		hasCharList = false
	}

	// If this is a brand new session (state=0), clear the hasCharList flag
	// because it might be from a previous login attempt
	if sess.State() == session.StateConnecting && hasCharList {
		log.Printf("Authenticate2: new session but hasCharList=true from IP tracking, clearing flag")
		h.charListSentMu.Lock()
		delete(h.charListSent, clientIP)
		h.charListSentMu.Unlock()
		hasCharList = false
	}

	// Only skip if we ACTUALLY sent the character list (IP-based tracking)
	if hasCharList {
		log.Printf("Authenticate2: character list already sent, just ACKing")
		return nil
	}

	log.Printf("Authenticate2: sending initial contact (Camera1)")

	// Reset and update state to character select
	sess.SetState(session.StateCharacterSelect)

	// Camera1 (0x07D1) carries int32 -1, NOT 3. Verified in every Sandstorm
	// capture — `Camera1 payload=4  ff ff ff ff` — on the pre-promotion session
	// as well as the promoted one. The old 0x03 came from a doc line that
	// CLAUDE.md and the memory notes both record as wrong; SendMemoryDump was
	// corrected in 2026-08-03 but these two character-select senders were not.
	// Note: eqoa.live only sends Camera1, never Camera2 (0x07F5)
	msg1 := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeExpansionFlags)
	writer1 := buffer.NewWriter(msg1.Span())
	writer1.WriteUint16(uint16(protocol.OpcodeExpansionFlags))
	writer1.WriteInt32(-1)
	msg1.SetSize(writer1.Position())
	sess.Queue().AddWithSequence(msg1)

	log.Printf("Authenticate2: Camera1 queued with sequence %d", msg1.Sequence)

	// The character list goes out on THIS session — still client-initiated, on
	// the client's own instance, with no sessionID in the header. Verified
	// against pcaps/ps2_sandstorm.pcap: Camera1 at t=1.121998s and
	// CharacterSelect at t=1.141304s both carry flags=ClientInit|HasInstance on
	// instance 0x000267A3, only 19ms apart — the server does not wait for an
	// ACK between them, and it does not promote first.
	//
	// Promotion to a master session now happens in HandleSelectedChar, once a
	// character has actually been picked (Sandstorm does it at t=4.61s). The
	// old code promoted here, which told the PS2 "server-initiated, new
	// instance, here is your session ID" while it was still sitting in
	// character select; it bounced back to the station screen ~3s later.
	if h.characterHandler != nil {
		// NOT sent: ServerLoginResponse (0x0728). Sandstorm never sends it —
		// zero occurrences across all three PS2 captures — and sending it here
		// pushed CharacterSelect from seq 3 to seq 4, which is the only
		// remaining difference between our character-select packet and a
		// working server's.
		h.characterHandler.SendCharacterSelectEQOA(sess)
		h.characterHandler.SendTime(sess)

		// Mark character list as sent for this IP
		h.charListSentMu.Lock()
		h.charListSent[clientIP] = time.Now()
		h.charListSentMu.Unlock()
	}

	return nil
}

// SendBadPassword sends a bad password response.
func (h *AuthHandler) SendBadPassword(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeBadLoginPassword)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeBadLoginPassword)) // Opcode in data
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendNotSubscribed sends a subscription error.
func (h *AuthHandler) SendNotSubscribed(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNotSubscribed)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeNotSubscribed)) // Opcode in data
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendWorldUnavailable sends a world unavailable error.
func (h *AuthHandler) SendWorldUnavailable(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWorldUnavailable)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeWorldUnavailable)) // Opcode in data
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendDisconnect sends a disconnect message.
func (h *AuthHandler) SendDisconnect(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDisconnectClient)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeDisconnectClient)) // Opcode in data
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	sess.SetPendingTermination(true)
}

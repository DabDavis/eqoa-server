package session

import (
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Manager manages all active sessions.
type Manager struct {
	sessions sync.Map // map[SessionKey]*Session

	// Instance ID generator
	nextInstanceID uint32

	// Player object ID generator (for in-game entity IDs)
	nextPlayerObjectID uint32

	// Session statistics
	activeCount int64
	totalCount  int64

	// onReplaced tells a session it lost its character to another login
	// (handlers.SendLoggedInElsewhere); see RemoveSessionForCharacter.
	onReplaced func(*Session)

	// Configurable timeouts
	preGameTimeout time.Duration
	inGameTimeout  time.Duration

	// Dead instance IDs: old auth instances that were promoted to master.
	// Packets for these IDs should be dropped, not create orphan sessions.
	deadInstances sync.Map // map[uint32]bool
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	// Seed instance ID with random value
	// Start player object IDs at a high value to distinguish from other IDs
	return &Manager{
		nextInstanceID:     uint32(rand.Int31()),
		nextPlayerObjectID: 500000000, // Start at 500M like eqoa.live
		preGameTimeout:     45 * time.Second,
		inGameTimeout:      60 * time.Second,
	}
}

// SetTimeouts configures the timeout durations for all new sessions.
func (m *Manager) SetTimeouts(preGame, inGame time.Duration) {
	m.preGameTimeout = preGame
	m.inGameTimeout = inGame
}

// GetSession retrieves a session by key.
func (m *Manager) GetSession(key SessionKey) *Session {
	if val, ok := m.sessions.Load(key); ok {
		return val.(*Session)
	}
	return nil
}

// GetSessionByAddr retrieves a session by remote address and instance ID.
func (m *Manager) GetSessionByAddr(addr *net.UDPAddr, instanceID uint32) *Session {
	key := SessionKey{
		Addr:       addr.String(),
		InstanceID: instanceID,
	}
	return m.GetSession(key)
}

// GetOrCreateSession gets an existing session or creates a new one.
func (m *Manager) GetOrCreateSession(addr *net.UDPAddr, packet interface{}) *Session {
	// Extract packet info - this is a simplified version
	// In practice, we'd extract instance/session IDs from the packet
	type packetInfo interface {
		GetLocalEndpoint() uint16
		GetRemoteEndpoint() uint16
		GetInstanceID() uint32
		IsNewConnection() bool
	}

	// For now, use a simplified approach based on address
	// The real implementation would parse the packet header
	var instanceID uint32
	var sessionID uint32
	var clientEndpoint uint16
	var serverEndpoint uint16
	var isNew bool

	// Try to extract from packet if it has the right interface
	if p, ok := packet.(packetInfo); ok {
		instanceID = p.GetInstanceID()
		clientEndpoint = p.GetLocalEndpoint()
		serverEndpoint = p.GetRemoteEndpoint()
		isNew = p.IsNewConnection()
	} else {
		// Fallback: generate new instance ID
		instanceID = m.generateInstanceID()
		clientEndpoint = uint16(instanceID & 0xFFFF)
		serverEndpoint = uint16((instanceID >> 16) & 0xFFFF)
		if serverEndpoint == 0 {
			serverEndpoint = clientEndpoint + 1
		}
		isNew = true
	}

	// Drop packets for dead instance IDs (old auth sessions that were promoted to master).
	// These are late/queued packets from the client before it learned about the new instance.
	if _, dead := m.deadInstances.Load(instanceID); dead {
		log.Printf("GetOrCreateSession: dropping packet for dead instance 0x%X (promoted away)", instanceID)
		return nil
	}

	// If packet has instanceID=0, try to find existing session by address only
	// This handles game server packets that come without instance ID initially
	if instanceID == 0 {
		var existingSession *Session
		addrStr := addr.String()
		m.sessions.Range(func(key, value interface{}) bool {
			k := key.(SessionKey)
			if k.Addr == addrStr {
				existingSession = value.(*Session)
				return false // Found one, stop iterating
			}
			return true
		})
		if existingSession != nil {
			log.Printf("GetOrCreateSession: instanceID=0, found existing session with instance=0x%X",
				existingSession.InstanceID())
			return existingSession
		}
		// No existing session, generate a new instance ID (don't use 0!)
		instanceID = m.generateInstanceID()
		log.Printf("GetOrCreateSession: instanceID=0, no existing session, generated instance=0x%X", instanceID)
	}

	// Generate session ID based on instance ID
	// Session ID should be non-zero for proper ObjectID generation
	sessionID = instanceID
	if sessionID == 0 {
		sessionID = m.generateSessionID()
	}

	key := SessionKey{
		Addr:       addr.String(),
		InstanceID: instanceID,
	}

	// Try to get existing session
	if sess := m.GetSession(key); sess != nil {
		return sess
	}

	// Instance-keyed lookup failed. Before creating a new session, check if
	// there's a master session for this client address. After promotion, the
	// server stores the session under the SERVER's instance ID, but the client
	// sends its OWN (different) instance. Route to the master session and
	// re-key it so future packets match directly.
	{
		addrStr := addr.String()
		var masterSess *Session
		m.sessions.Range(func(k, v interface{}) bool {
			sk := k.(SessionKey)
			if sk.Addr == addrStr {
				s := v.(*Session)
				if s.DidServerInitiate() {
					masterSess = s
					return false
				}
			}
			return true
		})
		if masterSess != nil {
			log.Printf("GetOrCreateSession: routing client instance 0x%X to master session (server instance 0x%X) for %s",
				instanceID, masterSess.InstanceID(), addrStr)
			// Also store under client's instance for fast future lookups
			m.sessions.Store(key, masterSess)
			return masterSess
		}
	}

	// Create new session if this is a new connection
	if isNew || true { // For now, always allow creation
		// Clean up stale game sessions from previous logins for the same client IP.
		// Without this, old sessions with CharacterInWorld=true continue generating
		// state updates in the tick loop, sending conflicting channel data.
		// Matches on IP only since client uses different source ports per login.
		// Only removes InGame/CharacterInWorld sessions, NOT concurrent login sessions.
		if removed := m.RemoveStaleGameSessions(addr.IP.String(), instanceID); removed > 0 {
			log.Printf("GetOrCreateSession: cleaned up %d stale game sessions for IP %s", removed, addr.IP)
		}

		sess := NewSession(addr, instanceID, sessionID, clientEndpoint, serverEndpoint, false)
		sess.SetTimeouts(m.preGameTimeout, m.inGameTimeout)

		// Store session
		actual, loaded := m.sessions.LoadOrStore(key, sess)
		if loaded {
			return actual.(*Session)
		}

		atomic.AddInt64(&m.activeCount, 1)
		atomic.AddInt64(&m.totalCount, 1)

		log.Printf("Created new session for %s (instance: 0x%X, session: 0x%X)", addr, instanceID, sessionID)
		return sess
	}

	return nil
}

// CreateSession creates a new session with specific parameters.
func (m *Manager) CreateSession(addr *net.UDPAddr, instanceID, sessionID uint32, clientEndpoint, serverEndpoint uint16, serverInitiated bool) *Session {
	sess := NewSession(addr, instanceID, sessionID, clientEndpoint, serverEndpoint, serverInitiated)
	sess.SetTimeouts(m.preGameTimeout, m.inGameTimeout)

	key := sess.Key()
	m.sessions.Store(key, sess)

	atomic.AddInt64(&m.activeCount, 1)
	atomic.AddInt64(&m.totalCount, 1)

	return sess
}

// CreateMasterSession creates a master session from an existing session.
func (m *Manager) CreateMasterSession(existing *Session) *Session {
	instanceID := m.generateInstanceID()
	sessionID := existing.SessionID()

	// Master session has server-initiated flag
	sess := NewSession(
		existing.RemoteAddr(),
		instanceID,
		sessionID,
		existing.ClientEndpoint(),
		existing.ServerEndpoint(),
		true, // Server initiated
	)
	sess.SetTimeouts(m.preGameTimeout, m.inGameTimeout)

	key := sess.Key()
	m.sessions.Store(key, sess)

	atomic.AddInt64(&m.activeCount, 1)
	atomic.AddInt64(&m.totalCount, 1)

	log.Printf("Created master session for %s (instance: 0x%X)", existing.RemoteAddr(), instanceID)
	return sess
}

// PromoteSessionToMaster promotes an existing session to a "master session".
// This matches C#'s CreateMasterSession: generates new instance ID, sets didServerInitiate=true,
// writes SessionID (= playerObjectID) in packet headers so client can read ObjectID.
// The session is registered under both old and new instance IDs for transition.
func (m *Manager) PromoteSessionToMaster(sess *Session, playerObjectID uint32) uint32 {
	// The instance ID belongs to the CLIENT and survives promotion — only the
	// sessionID and the ServerInit/NewInstance flags change. Verified against
	// pcaps/ps2_sandstorm.pcap: the PS2 mints 0x000267A3 itself at t=1.12s and
	// every later server packet, promoted or not, still carries that same
	// instance. Minting a server-side instance here handed the client a
	// NewInstance it had never asked for, and it abandoned the session.
	//
	// Because the instance is unchanged, the session map key is still valid —
	// no re-keying, and no dead-instance marking (that would kill this very
	// session's own key).
	instanceID := sess.InstanceID()

	sess.PromoteToMaster(instanceID, playerObjectID)

	// Reset connection counters for a clean master-session start. Sandstorm's
	// first promoted packet is arrival=1 carrying Camera1 at seq=1, so the
	// sequence space really does restart here.
	sess.Connection().ResetForMasterSession()

	// Clear the message queue (pre-promotion reliable messages)
	sess.Queue().Clear()

	log.Printf("PromoteSessionToMaster: promoted session %s (instance=0x%X kept, sessionID/objectID=%d (0x%X))",
		sess.RemoteAddr(), instanceID, playerObjectID, playerObjectID)

	return instanceID
}

// RemoveSession removes a session.
func (m *Manager) RemoveSession(sess *Session) {
	if sess == nil {
		return
	}

	key := sess.Key()
	if _, loaded := m.sessions.LoadAndDelete(key); loaded {
		atomic.AddInt64(&m.activeCount, -1)
		log.Printf("Removed session for %s (instance: 0x%X)", sess.RemoteAddr(), sess.InstanceID())
	}
}

// RemoveSessionByKey removes a session by key.
func (m *Manager) RemoveSessionByKey(key SessionKey) {
	if _, loaded := m.sessions.LoadAndDelete(key); loaded {
		atomic.AddInt64(&m.activeCount, -1)
	}
}

// ActiveCount returns the number of active sessions.
func (m *Manager) ActiveCount() int64 {
	return atomic.LoadInt64(&m.activeCount)
}

// TotalCount returns the total number of sessions created.
func (m *Manager) TotalCount() int64 {
	return atomic.LoadInt64(&m.totalCount)
}

// ForEach iterates over all sessions.
func (m *Manager) ForEach(fn func(*Session) bool) {
	m.sessions.Range(func(key, value interface{}) bool {
		return fn(value.(*Session))
	})
}

// staleLobbyIdle is how long a same-IP pre-game session must have been silent
// before a new login reaps it. A login in progress exchanges packets every
// few hundred milliseconds; one abandoned by an earlier attempt goes quiet.
const staleLobbyIdle = 10 * time.Second

// RemoveStaleGameSessions flags stale pre-game sessions for termination.
// Only removes lobby/auth sessions (not in-game) from the same IP to avoid
// killing concurrent clients (e.g. headless + PS2 from the same machine).
// In-game cleanup is handled by character-based dedup in World.CreatePlayer.
//
// A pre-game session is stale only once it has been silent for
// staleLobbyIdle: two players behind one address (one household's router, or
// two clients on one machine) log in at the same moment, and flagging every
// lobby session from the IP killed whichever login was still in progress —
// the second client's session creation took out the first's mid-MemoryDump.
func (m *Manager) RemoveStaleGameSessions(ip string, exceptInstanceID uint32) int {
	now := time.Now()
	var count int
	m.sessions.Range(func(key, value interface{}) bool {
		k := key.(SessionKey)
		sess := value.(*Session)
		if k.InstanceID == exceptInstanceID {
			return true
		}
		sessIP := sess.RemoteAddr().IP.String()
		if sessIP == ip {
			// Only terminate pre-game sessions (lobby duplicates).
			// In-game sessions are cleaned up by character-based dedup in CreatePlayer.
			if !sess.InGame() && !sess.CharacterInWorld() && !sess.DidServerInitiate() &&
				now.Sub(sess.LastActivity()) >= staleLobbyIdle {
				log.Printf("RemoveStaleGameSessions: flagging stale pre-game session (addr=%s, instance=0x%X)",
					k.Addr, k.InstanceID)
				sess.SetPendingTermination(true)
				count++
			}
		}
		return true
	})
	return count
}

// LiveWithin is how recently an in-game session must have been heard from to
// hold its login (LiveLogin). An idle page keeps its connection with a DRDP
// keepalive every 30 s; a tab closed, crashed or long hidden goes quiet, and
// must not lock its account or address out until the in-game timeout.
const LiveWithin = 60 * time.Second

// LiveLogin names what a new login would collide with: "account" when that
// account has a live in-game session, "address" when one comes from ip
// (nil: not checked), "" when neither. Live means in the game, not on its way
// out (replaced or terminating), and heard from within LiveWithin. ipOf gives
// a session's client address (a relayed player's is the page's, not the
// relay's; nil: unknown, never a match); nil ipOf takes the source as it is.
func (m *Manager) LiveLogin(accountID int, ip net.IP, ipOf func(*net.UDPAddr) net.IP) string {
	now := time.Now()
	var hit string
	m.sessions.Range(func(_, value interface{}) bool {
		sess := value.(*Session)
		if !holdsLogin(sess, now) {
			return true
		}
		if accountID != 0 && sess.AccountID() == accountID {
			hit = "account"
			return false
		}
		if ip != nil && sess.RemoteAddr() != nil {
			sip := sess.RemoteAddr().IP
			if ipOf != nil {
				sip = ipOf(sess.RemoteAddr())
			}
			if sip == nil || !sip.Equal(ip) {
				return true
			}
			hit = "address"
			return false
		}
		return true
	})
	return hit
}

// LiveCharacter reports whether the character (by serverID) is in the game
// in a session other than except, live as LiveLogin counts one (LiveHolder).
func (m *Manager) LiveCharacter(charServerID uint32, except *Session) bool {
	return m.LiveHolder(charServerID, except) != nil
}

// LiveHolder is the session other than except that has the character (by
// serverID) in the game, live as LiveLogin counts one, or nil. Character
// select takes the character over from it when it is the same account's
// (entity/takeover.go), and refuses it while another account's holds it.
func (m *Manager) LiveHolder(charServerID uint32, except *Session) *Session {
	if charServerID == 0 {
		return nil
	}
	now := time.Now()
	var holder *Session
	m.sessions.Range(func(_, value interface{}) bool {
		sess := value.(*Session)
		if sess != except && sess.CharacterServerID() == charServerID && holdsLogin(sess, now) {
			holder = sess
			return false
		}
		return true
	})
	return holder
}

// holdsLogin reports whether sess holds its login (LiveLogin, LiveCharacter):
// in the game, not on its way out (replaced or terminating), and heard from
// within LiveWithin.
func holdsLogin(sess *Session, now time.Time) bool {
	if !sess.InGame() && !sess.CharacterInWorld() {
		return false
	}
	if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() {
		return false
	}
	return now.Sub(sess.LastActivity()) <= LiveWithin
}

// EndAccount ends every session the account holds: one in the game is told
// it was replaced (0x07A4) and kept ReplacedGrace to hear it, as
// RemoveSessionForCharacter does; one still in the lobby is dropped. Returns
// how many in-game sessions were told.
func (m *Manager) EndAccount(accountID int) int {
	if accountID == 0 {
		return 0
	}
	var n int
	m.sessions.Range(func(_, value interface{}) bool {
		sess := value.(*Session)
		if sess.AccountID() != accountID {
			return true
		}
		if _, done := sess.ReplacedFor(); done || sess.PendingTermination() {
			return true
		}
		if (sess.InGame() || sess.CharacterInWorld()) && m.onReplaced != nil {
			log.Printf("EndAccount: telling session for account %d it was logged out (addr=%s, instance=0x%X)",
				accountID, sess.RemoteAddr(), sess.InstanceID())
			sess.MarkReplaced()
			m.onReplaced(sess)
			n++
			return true
		}
		sess.SetPendingTermination(true)
		return true
	})
	return n
}

// ReplacedGrace is how long a session whose character logged in again is kept
// after it was told so: long enough for the disconnect to go out (and be
// resent once), short of the in-game timeout.
const ReplacedGrace = 3 * time.Second

// SetOnReplaced sets the function that tells a replaced session it was
// replaced. Without one, a replaced session is dropped at once and silently.
func (m *Manager) SetOnReplaced(fn func(*Session)) {
	m.onReplaced = fn
}

// ReplaceSession ends sess because another login took its character: one in
// the game is told (0x07A4, "You have logged in from another location.") and
// kept ReplacedGrace to hear it; one not in the game, or with nothing to tell
// it, is dropped at the next tick. One already replaced is left alone. It
// reports whether sess was told.
func (m *Manager) ReplaceSession(sess *Session) bool {
	if sess == nil {
		return false
	}
	if _, done := sess.ReplacedFor(); done {
		return false
	}
	if m.onReplaced == nil || !sess.InGame() {
		sess.SetPendingTermination(true)
		return false
	}
	sess.MarkReplaced()
	m.onReplaced(sess)
	return true
}

// RemoveSessionForCharacter retires any existing session that holds the given
// character (by serverID). Called from World.CreatePlayer so that reconnecting
// the same character cleans up the ghost, while different characters from the
// same IP can coexist.
//
// The old session is TOLD (0x07A4, "You have logged in from another
// location."), then kept for ReplacedGrace so the message goes out. Dropping
// it silently left the old client running against a server that no longer
// answered it — a stale window that kept sending, so a fresh session was made
// for it and lived until the ping timeout. Two browser tabs on one character
// did exactly that.
func (m *Manager) RemoveSessionForCharacter(charServerID uint32, exceptInstanceID uint32) int {
	var count int
	m.sessions.Range(func(key, value interface{}) bool {
		k := key.(SessionKey)
		sess := value.(*Session)
		if k.InstanceID == exceptInstanceID {
			return true
		}
		if sess.CharacterServerID() == charServerID && charServerID != 0 {
			if _, done := sess.ReplacedFor(); done {
				return true
			}
			count++
			if m.ReplaceSession(sess) {
				log.Printf("RemoveSessionForCharacter: told old session for char serverID=%d it was replaced (addr=%s, instance=0x%X)",
					charServerID, k.Addr, k.InstanceID)
			} else {
				log.Printf("RemoveSessionForCharacter: flagged old session for char serverID=%d (addr=%s, instance=0x%X)",
					charServerID, k.Addr, k.InstanceID)
			}
		}
		return true
	})
	return count
}

// CleanupTimedOut removes timed-out sessions.
func (m *Manager) CleanupTimedOut() int {
	var toRemove []SessionKey

	m.sessions.Range(func(key, value interface{}) bool {
		sess := value.(*Session)
		if sess.InGame() {
			// In-game: IsTimedOut() is a 2s ping-interval trigger, not a disconnect.
			// Only disconnect after 50 missed pings (~100s with no client response).
			if sess.CheckPingTimeout() {
				toRemove = append(toRemove, key.(SessionKey))
			}
		} else {
			// Pre-game: IsTimedOut() means 45s with no activity — safe to remove.
			if sess.IsTimedOut() {
				toRemove = append(toRemove, key.(SessionKey))
			}
		}
		return true
	})

	for _, key := range toRemove {
		m.RemoveSessionByKey(key)
	}

	return len(toRemove)
}

// StartCleanupRoutine starts a background goroutine to clean up timed-out sessions.
func (m *Manager) StartCleanupRoutine(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				if removed := m.CleanupTimedOut(); removed > 0 {
					log.Printf("Cleaned up %d timed-out sessions", removed)
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}

// generateInstanceID generates a new unique instance ID.
func (m *Manager) generateInstanceID() uint32 {
	return atomic.AddUint32(&m.nextInstanceID, 1)
}

// GenerateInstanceID generates a new unique instance ID (public).
func (m *Manager) GenerateInstanceID() uint32 {
	return m.generateInstanceID()
}

// generateSessionID generates a new session ID.
func (m *Manager) generateSessionID() uint32 {
	return uint32(rand.Int31())
}

// GeneratePlayerObjectID generates a new unique player object ID.
// This is called when a player enters the game world and is used as their entity ID.
func (m *Manager) GeneratePlayerObjectID() uint32 {
	return atomic.AddUint32(&m.nextPlayerObjectID, 1)
}

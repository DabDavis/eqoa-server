package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"sync"
	"time"
)

// TicketStore records which usernames have completed a TCP login recently.
//
// It exists to close a bypass. The UDP Authenticate and Authenticate2 handlers
// take the username straight out of the packet and look the account up, so a
// client that never spoke to the auth server at all could name any account and
// be treated as that account. Password checking on the TCP side is worth
// nothing while that path is open.
//
// A ticket is granted only by a successful login and expires on its own. It
// holds what that login's password hashes to, and answers only to the same
// password (ValidFor): every client's handshake carries it in its token, so a
// ticket spares the handshake a second bcrypt and never stands in for the
// password. A ticket that answered to the username alone let anyone who knew it
// sign in within ttl of the account's real login.
type TicketStore struct {
	mu      sync.Mutex
	granted map[string]ticket
	ttl     time.Duration
	salt    [32]byte // this run's: a ticket's proof means nothing outside it

	// now is swappable for tests. Nil means time.Now.
	now func() time.Time
}

type ticket struct {
	accountID int
	expires   time.Time
	proof     [sha256.Size]byte
}

// NewTicketStore creates a store whose tickets live for ttl. A non-positive ttl
// falls back to 15 minutes, which comfortably covers a human sitting on the
// server-select and character-select screens between TCP auth and world entry.
func NewTicketStore(ttl time.Duration) *TicketStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s := &TicketStore{
		granted: make(map[string]ticket),
		ttl:     ttl,
	}
	rand.Read(s.salt[:]) // cannot fail (crypto/rand since Go 1.24)
	return s
}

// proofOf is what a ticket keeps of the password that earned it.
func (s *TicketStore) proofOf(password string) [sha256.Size]byte {
	return sha256.Sum256(append(s.salt[:len(s.salt):len(s.salt)], password...))
}

func (s *TicketStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Grant records a successful login for username with password.
func (s *TicketStore) Grant(username string, accountID int, password string) {
	key := normalize(username)
	if key == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock()
	s.granted[key] = ticket{accountID: accountID, expires: now.Add(s.ttl), proof: s.proofOf(password)}

	// Opportunistic sweep. The map is bounded by the number of distinct
	// usernames seen within one TTL, but a long-lived server with churn should
	// not accumulate dead entries forever.
	for k, t := range s.granted {
		if now.After(t.expires) {
			delete(s.granted, k)
		}
	}
}

// Valid reports whether username holds an unexpired ticket, and the account ID
// it was granted for. Username matching is case-insensitive: the account lookup
// downstream is too, so a case-sensitive check here would reject logins that
// the rest of the server considers the same account.
func (s *TicketStore) Valid(username string) (int, bool) {
	key := normalize(username)
	if key == "" {
		return 0, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.granted[key]
	if !ok {
		return 0, false
	}
	if s.clock().After(t.expires) {
		delete(s.granted, key)
		return 0, false
	}
	return t.accountID, true
}

// ValidFor reports whether username holds an unexpired ticket earned with this
// password, and the account ID it was granted for.
func (s *TicketStore) ValidFor(username, password string) (int, bool) {
	proof := s.proofOf(password)
	key := normalize(username)
	if key == "" {
		return 0, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.granted[key]
	if !ok {
		return 0, false
	}
	if s.clock().After(t.expires) {
		delete(s.granted, key)
		return 0, false
	}
	if subtle.ConstantTimeCompare(t.proof[:], proof[:]) != 1 {
		return 0, false
	}
	return t.accountID, true
}

// Revoke drops any ticket for username.
func (s *TicketStore) Revoke(username string) {
	key := normalize(username)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.granted, key)
}

func normalize(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

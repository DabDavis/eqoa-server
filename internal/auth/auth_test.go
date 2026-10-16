package auth

import (
	"testing"
	"time"
)

const testCost = 4 // bcrypt minimum; these tests are about the decision, not the KDF

func TestVerifyPasswordAgainstABcryptHash(t *testing.T) {
	hash, err := HashPassword("correct", testCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if got := VerifyPassword(hash, "correct"); got != Accepted {
		t.Fatalf("correct password gave %v, want Accepted", got)
	}
	if got := VerifyPassword(hash, "wrong"); got != Rejected {
		t.Fatalf("wrong password gave %v, want Rejected", got)
	}
	if got := VerifyPassword(hash, ""); got != Rejected {
		t.Fatalf("empty password gave %v, want Rejected", got)
	}
}

// Two hashes of the same password must differ -- if they do not, the salt is
// not doing its job and the column is effectively a lookup table.
func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same", testCost)
	b, _ := HashPassword("same", testCost)
	if a == b {
		t.Fatal("two hashes of the same password are identical: no salt")
	}
}

func TestLegacyPlaintextIsAcceptedButFlaggedForRehash(t *testing.T) {
	if got := VerifyPassword("plaintextpw", "plaintextpw"); got != AcceptedNeedsRehash {
		t.Fatalf("matching plaintext gave %v, want AcceptedNeedsRehash", got)
	}
	if got := VerifyPassword("plaintextpw", "other"); got != Rejected {
		t.Fatalf("non-matching plaintext gave %v, want Rejected", got)
	}
}

// An empty stored credential must be reported as such, never treated as a
// password that anything matches.
func TestEmptyCredentialIsNotAMatch(t *testing.T) {
	if got := VerifyPassword("", ""); got != NoCredential {
		t.Fatalf("empty/empty gave %v, want NoCredential", got)
	}
	if got := VerifyPassword("", "anything"); got != NoCredential {
		t.Fatalf("empty stored gave %v, want NoCredential", got)
	}
}

// A 60-character password that merely looks like a hash must not be routed
// through bcrypt comparison, and vice versa.
func TestBcryptDetection(t *testing.T) {
	real, _ := HashPassword("x", testCost)
	if !looksLikeBcrypt(real) {
		t.Fatalf("real bcrypt hash not detected: %q", real)
	}

	for _, s := range []string{
		"",
		"short",
		"$2a$04$tooshort",
		"$1a$04$0123456789012345678901234567890123456789012345678901",   // wrong prefix
		"0123456789012345678901234567890123456789012345678901234567890", // 60 chars, no prefix
	} {
		if looksLikeBcrypt(s) {
			t.Fatalf("%q misdetected as a bcrypt hash", s)
		}
	}
}

// An out-of-range cost must fall back rather than fail a registration outright.
func TestOutOfRangeCostFallsBack(t *testing.T) {
	for _, cost := range []int{0, 3, 99, -1} {
		hash, err := HashPassword("pw", cost)
		if err != nil {
			t.Fatalf("cost %d: %v", cost, err)
		}
		if VerifyPassword(hash, "pw") != Accepted {
			t.Fatalf("cost %d produced an unusable hash", cost)
		}
	}
}

func TestTicketGrantAndValidate(t *testing.T) {
	s := NewTicketStore(time.Minute)

	if _, ok := s.Valid("alice"); ok {
		t.Fatal("unknown username has a valid ticket")
	}

	s.Grant("alice", 42)
	id, ok := s.Valid("alice")
	if !ok {
		t.Fatal("granted ticket is not valid")
	}
	if id != 42 {
		t.Fatalf("ticket carries account %d, want 42", id)
	}
}

// Account lookup downstream is case-insensitive, so ticket matching must be
// too, or a client that logs in as "Alice" and then sends "alice" is refused.
func TestTicketMatchingIsCaseInsensitive(t *testing.T) {
	s := NewTicketStore(time.Minute)
	s.Grant("Alice", 7)

	for _, name := range []string{"alice", "ALICE", "Alice", " alice "} {
		if _, ok := s.Valid(name); !ok {
			t.Fatalf("ticket not found for %q", name)
		}
	}
}

func TestTicketExpires(t *testing.T) {
	s := NewTicketStore(time.Minute)

	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }

	s.Grant("bob", 1)
	if _, ok := s.Valid("bob"); !ok {
		t.Fatal("ticket invalid immediately after being granted")
	}

	now = now.Add(59 * time.Second)
	if _, ok := s.Valid("bob"); !ok {
		t.Fatal("ticket expired early")
	}

	now = now.Add(2 * time.Second) // past the TTL
	if _, ok := s.Valid("bob"); ok {
		t.Fatal("expired ticket still valid")
	}
}

func TestTicketRevoke(t *testing.T) {
	s := NewTicketStore(time.Minute)
	s.Grant("carol", 3)
	s.Revoke("carol")
	if _, ok := s.Valid("carol"); ok {
		t.Fatal("revoked ticket still valid")
	}
}

func TestEmptyUsernameNeverHoldsATicket(t *testing.T) {
	s := NewTicketStore(time.Minute)
	s.Grant("", 1)
	if _, ok := s.Valid(""); ok {
		t.Fatal("empty username holds a ticket")
	}
}

func TestTicketStoreIsConcurrencySafe(t *testing.T) {
	s := NewTicketStore(time.Minute)
	done := make(chan struct{})

	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				s.Grant("user", 1)
				s.Valid("user")
				s.Revoke("other")
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

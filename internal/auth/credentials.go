// Package auth holds password verification and the short-lived record of which
// accounts have completed a TCP login.
//
// It is deliberately free of protocol and database types so that both the TCP
// auth listener and the UDP opcode handlers can depend on it without importing
// each other.
package auth

import (
	"crypto/subtle"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBCryptCost is used when configuration does not set one.
const DefaultBCryptCost = bcrypt.DefaultCost

// HashPassword produces a bcrypt hash suitable for the accountinfo.pwhash
// column. A cost outside bcrypt's supported range falls back to the default
// rather than failing a registration.
func HashPassword(password string, cost int) (string, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = DefaultBCryptCost
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Result describes the outcome of checking a supplied password against what is
// stored for an account.
type Result int

const (
	// Rejected means the password did not match.
	Rejected Result = iota

	// Accepted means the password matched a bcrypt hash.
	Accepted

	// AcceptedNeedsRehash means the password matched, but the stored value was
	// not a bcrypt hash -- it was a legacy plaintext password. The caller
	// should re-store it hashed.
	AcceptedNeedsRehash

	// NoCredential means the account exists but has no password set at all.
	// Older accounts were created this way by the UDP path, which passed an
	// empty string. The caller decides whether to adopt the supplied password
	// or refuse the login; this package will not guess.
	NoCredential
)

// VerifyPassword checks a supplied plaintext password against a stored
// credential.
//
// Stored values come in three shapes because of how accounts were created
// before password checking existed:
//
//	bcrypt hash   the normal case
//	plaintext     written by the account-registration web server
//	empty         written by the UDP auth path, which passed ""
//
// Plaintext comparison is constant time so that a legacy account is not more
// vulnerable to timing analysis than a hashed one.
func VerifyPassword(stored, supplied string) Result {
	if stored == "" {
		return NoCredential
	}

	if looksLikeBcrypt(stored) {
		if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(supplied)); err != nil {
			return Rejected
		}
		return Accepted
	}

	if subtle.ConstantTimeCompare([]byte(stored), []byte(supplied)) == 1 {
		return AcceptedNeedsRehash
	}
	return Rejected
}

// looksLikeBcrypt reports whether a stored credential is a bcrypt hash rather
// than a legacy plaintext password. bcrypt hashes are $2<variant>$<cost>$<salt+digest>
// and always 60 characters.
func looksLikeBcrypt(stored string) bool {
	if len(stored) != 60 {
		return false
	}
	return strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$")
}

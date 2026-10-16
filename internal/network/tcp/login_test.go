package tcp

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// newLoginTestServer builds an AuthServer over a throwaway database.
func newLoginTestServer(t *testing.T, tune func(*config.AuthConfig)) (*AuthServer, *database.SQLiteAccountStore) {
	t.Helper()

	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "auth_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := config.DefaultConfig()
	// Keep hashing cheap; these tests are about the decision, not the KDF.
	cfg.Auth.BCryptCost = 4
	if tune != nil {
		tune(&cfg.Auth)
	}

	store := database.NewSQLiteAccountStore(db)
	return NewAuthServer(cfg, store, auth.NewTicketStore(0)), store
}

// buildLoginPayload assembles the 140-byte Login Request payload the PS2 sends.
func buildLoginPayload(t *testing.T, username, password string) []byte {
	t.Helper()

	payload := make([]byte, 140)
	binary.BigEndian.PutUint32(payload[4:8], 2) // version
	copy(payload[8:40], username)

	field, err := encryptPassword(password)
	if err != nil {
		t.Fatalf("encrypt password: %v", err)
	}
	copy(payload[40:72], field)

	binary.BigEndian.PutUint32(payload[72:76], 1)
	copy(payload[76:80], "EQOA")
	return payload
}

// succeeded reports whether a Login Response says yes. Success and failure
// differ at the error flag and the status word, and both are checked: a
// response that set only one of them would be a half-answer.
func succeeded(t *testing.T, response []byte) bool {
	t.Helper()
	if len(response) != 168 {
		t.Fatalf("login response is %d bytes, want 168", len(response))
	}
	errFlag := binary.BigEndian.Uint32(response[8:12])
	status := binary.BigEndian.Uint32(response[84:88])

	switch {
	case errFlag == 0 && status == 3:
		return true
	case errFlag == 1 && status == 4:
		return false
	default:
		t.Fatalf("inconsistent response: error flag %d with status %d", errFlag, status)
		return false
	}
}

func login(t *testing.T, s *AuthServer, username, password string) bool {
	t.Helper()
	return succeeded(t, s.handleLogin(buildLoginPayload(t, username, password)))
}

// The whole point: the right password gets in and the wrong one does not.
func TestLoginChecksThePassword(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)

	if !login(t, s, "alice", "correct-horse") {
		t.Fatal("first login should register the account and succeed")
	}
	if !login(t, s, "alice", "correct-horse") {
		t.Fatal("correct password rejected on the second login")
	}
	if login(t, s, "alice", "wrong-password") {
		t.Fatal("WRONG PASSWORD ACCEPTED")
	}
	if login(t, s, "alice", "") {
		t.Fatal("empty password accepted")
	}
}

// The password must be stored hashed, not in the clear. Reading it back out of
// the database is the only way to be sure the hashing is real rather than a
// comparison that happens to work.
func TestPasswordIsStoredHashed(t *testing.T) {
	s, store := newLoginTestServer(t, nil)

	const password = "hunter2"
	if !login(t, s, "bob", password) {
		t.Fatal("registration login failed")
	}

	account, err := store.GetByUsername("bob")
	if err != nil {
		t.Fatalf("account not created: %v", err)
	}
	if account.PasswordHash == password {
		t.Fatal("password stored in PLAINTEXT")
	}
	if len(account.PasswordHash) != 60 || account.PasswordHash[:4] != "$2a$" {
		t.Fatalf("stored credential is not a bcrypt hash: %q", account.PasswordHash)
	}
}

// An account created before password checking existed has an empty pwhash. It
// must not be a skeleton key: with auto-register on, the next login adopts a
// password, and from then on the account is protected.
func TestAccountWithNoPasswordAdoptsOneThenEnforcesIt(t *testing.T) {
	s, store := newLoginTestServer(t, nil)

	if _, err := store.Create("legacy", ""); err != nil {
		t.Fatalf("seed legacy account: %v", err)
	}

	if !login(t, s, "legacy", "adopted") {
		t.Fatal("login against a credential-less account should adopt the password")
	}
	if login(t, s, "legacy", "something-else") {
		t.Fatal("account still accepts ANY password after adoption")
	}
	if !login(t, s, "legacy", "adopted") {
		t.Fatal("adopted password no longer works")
	}
}

// A plaintext password left by the old account-registration page must keep
// working, and must be upgraded to bcrypt the first time it is used.
func TestLegacyPlaintextPasswordIsAcceptedThenUpgraded(t *testing.T) {
	s, store := newLoginTestServer(t, nil)

	if _, err := store.Create("olduser", "plaintextpw"); err != nil {
		t.Fatalf("seed legacy account: %v", err)
	}

	if login(t, s, "olduser", "wrong") {
		t.Fatal("wrong password accepted against a plaintext credential")
	}
	if !login(t, s, "olduser", "plaintextpw") {
		t.Fatal("legacy plaintext password rejected")
	}

	account, _ := store.GetByUsername("olduser")
	if account.PasswordHash == "plaintextpw" {
		t.Fatal("plaintext credential was not upgraded after a successful login")
	}
	if !login(t, s, "olduser", "plaintextpw") {
		t.Fatal("password stopped working after the upgrade")
	}
}

func TestAutoRegisterDisabledRefusesUnknownAccounts(t *testing.T) {
	s, _ := newLoginTestServer(t, func(a *config.AuthConfig) {
		a.AllowAutoRegister = false
	})

	if login(t, s, "stranger", "any") {
		t.Fatal("unknown account accepted while auto-register is disabled")
	}
}

// With no account store there is nothing to check a password against, so the
// server must refuse rather than default to yes.
func TestLoginRefusedWithoutAnAccountStore(t *testing.T) {
	s := NewAuthServer(config.DefaultConfig(), nil, nil)
	if login(t, s, "alice", "whatever") {
		t.Fatal("login succeeded with no account store configured")
	}
}

// A password field that was not encrypted with the protocol key -- garbage, or
// a client sending the password in the clear -- must not authenticate.
func TestUnencryptedPasswordFieldIsRejected(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)

	if !login(t, s, "carol", "realpassword") {
		t.Fatal("setup login failed")
	}

	payload := make([]byte, 140)
	copy(payload[8:40], "carol")
	copy(payload[40:72], "realpassword") // plaintext, as the old testlogin sent

	if succeeded(t, s.handleLogin(payload)) {
		t.Fatal("plaintext password field accepted")
	}
}

// A successful login must leave a ticket, or the UDP handshake has nothing to
// check and the whole gate is inert.
func TestSuccessfulLoginGrantsATicket(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)

	if _, ok := s.tickets.Valid("dave"); ok {
		t.Fatal("ticket exists before any login")
	}
	if !login(t, s, "dave", "pw") {
		t.Fatal("login failed")
	}
	if _, ok := s.tickets.Valid("dave"); !ok {
		t.Fatal("no ticket granted after a successful login")
	}
}

func TestFailedLoginGrantsNoTicket(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)

	if !login(t, s, "erin", "right") {
		t.Fatal("setup login failed")
	}
	s.tickets.Revoke("erin")

	if login(t, s, "erin", "wrong") {
		t.Fatal("wrong password accepted")
	}
	if _, ok := s.tickets.Valid("erin"); ok {
		t.Fatal("a FAILED login granted a ticket")
	}
}

// Regression: a freshly created account must not read back as banned.
//
// acctstatus doubles as the active flag and the ban flag, and Create inserts 1.
// Selecting it directly into Banned made every account banned-and-activated at
// once; nothing read Banned, so it surfaced only when the login path began
// honouring it and refused every account on its SECOND login. Registration
// still worked, which is what made it look like a password bug.
func TestNewAccountIsNotBanned(t *testing.T) {
	_, store := newLoginTestServer(t, nil)

	if _, err := store.Create("freshuser", "irrelevant"); err != nil {
		t.Fatalf("create: %v", err)
	}
	account, err := store.GetByUsername("freshuser")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if account.Banned {
		t.Fatal("a newly created account reads back as BANNED")
	}
	if !account.Activated {
		t.Fatal("a newly created account reads back as not activated")
	}
}

// Banning must actually work, or the check in the login path is decorative.
func TestBannedAccountCannotLogIn(t *testing.T) {
	s, store := newLoginTestServer(t, nil)

	if !login(t, s, "villain", "pw") {
		t.Fatal("setup login failed")
	}
	account, _ := store.GetByUsername("villain")
	if err := store.SetBanned(account.ID, true); err != nil {
		t.Fatalf("ban: %v", err)
	}

	if login(t, s, "villain", "pw") {
		t.Fatal("a banned account logged in with the correct password")
	}

	if err := store.SetBanned(account.ID, false); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if !login(t, s, "villain", "pw") {
		t.Fatal("account still refused after being unbanned")
	}
}

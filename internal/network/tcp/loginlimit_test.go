package tcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// The game's login counts every attempt against the server's one limiter
// (auth.Limiter) before the password is checked: by the client's address,
// unless it is the operators' or the relay's own, and by the account name.

func limitedLoginServer(t *testing.T, tune func(*config.AuthConfig)) (*AuthServer, *auth.Limiter) {
	t.Helper()
	s, _ := newLoginTestServer(t, tune)
	l := auth.NewLimiter()
	if err := s.SetLimiter(l, []string{"192.0.2.9", "198.51.100.0/24"}); err != nil {
		t.Fatal(err)
	}
	return s, l
}

func loginFrom(t *testing.T, s *AuthServer, name, password, ip string) []byte {
	t.Helper()
	var addr net.IP
	if ip != "" {
		addr = net.ParseIP(ip)
	}
	return s.handleLoginFrom(buildLoginPayload(t, name, password), addr)
}

func refusalText(r []byte) string { return string(bytes.TrimRight(r[12:76], "\x00")) }

func TestTheGameLoginStopsAGuesserOfAPassword(t *testing.T) {
	s, _ := limitedLoginServer(t, nil)
	if !login(t, s, "victim", "the-password") {
		t.Fatal("setup: the account did not register")
	}
	for i := 0; i < auth.FailsPerName; i++ {
		if r := loginFrom(t, s, "victim", fmt.Sprintf("guess-%d", i), "203.0.113.5"); succeeded(t, r) {
			t.Fatalf("guess %d got in", i)
		}
	}
	// the name is spent: even its right password, from another address, is
	// refused untried, and the refusal says why
	r := loginFrom(t, s, "victim", "the-password", "203.0.113.77")
	if succeeded(t, r) || !strings.HasPrefix(refusalText(r), "Too many failed logins") {
		t.Fatalf("after %d guesses: success=%v, %q", auth.FailsPerName, succeeded(t, r), refusalText(r))
	}
	// and the right password clears a name's count before it is spent
	if !login(t, s, "other", "pw2") {
		t.Fatal("setup: a second account did not register")
	}
	for i := 0; i < auth.FailsPerName-1; i++ {
		loginFrom(t, s, "other", "wrong", "203.0.113.6")
	}
	if r := loginFrom(t, s, "other", "pw2", "203.0.113.6"); !succeeded(t, r) {
		t.Fatalf("the right password, one try before the limit: %q", refusalText(r))
	}
	for i := 0; i < auth.FailsPerName-1; i++ {
		if r := loginFrom(t, s, "other", "wrong", "203.0.113.6"); succeeded(t, r) {
			t.Fatal("a wrong password got in")
		}
	}
	if r := loginFrom(t, s, "other", "pw2", "203.0.113.6"); !succeeded(t, r) {
		t.Fatal("a right password did not clear the name's count")
	}
}

func TestTheGameLoginStopsAnAddressTryingNames(t *testing.T) {
	s, _ := limitedLoginServer(t, nil)
	for i := 0; i < auth.FailsPerAddr; i++ {
		loginFrom(t, s, fmt.Sprintf("nobody%d", i), "x", "203.0.113.9")
	}
	if r := loginFrom(t, s, "nobody-more", "x", "203.0.113.9"); !strings.HasPrefix(refusalText(r), "Too many failed logins") {
		t.Fatalf("an address past its limit: %q", refusalText(r))
	}
	// the operators' addresses and an unnamed relayed player are counted by
	// name only
	for _, ip := range []string{"192.0.2.9", "198.51.100.40", "127.0.0.1", ""} {
		for i := 0; i < auth.FailsPerAddr+2; i++ {
			if r := loginFrom(t, s, fmt.Sprintf("n%s-%d", ip, i), "x", ip); strings.HasPrefix(refusalText(r), "Too many") {
				t.Fatalf("%q was limited by address", ip)
			}
		}
	}
}

func TestAnAddressRegistersNoFasterThanItMayGuess(t *testing.T) {
	s, _ := limitedLoginServer(t, func(a *config.AuthConfig) { a.AllowAutoRegister = true })
	for i := 0; i < auth.FailsPerAddr; i++ {
		if r := loginFrom(t, s, fmt.Sprintf("newbie%d", i), "pw", "203.0.113.20"); !succeeded(t, r) {
			t.Fatalf("registration %d refused: %q", i, refusalText(r))
		}
	}
	if r := loginFrom(t, s, "newbie-more", "pw", "203.0.113.20"); succeeded(t, r) {
		t.Fatal("an address registered past the limit")
	}
	// the address is spent as it would be by as many failures; the accounts
	// it made sign in from elsewhere
	if r := loginFrom(t, s, "newbie0", "pw", "203.0.113.21"); !succeeded(t, r) {
		t.Fatalf("a registered account from another address: %q", refusalText(r))
	}
}

func TestTheAccountPagesShareTheGameLoginsLimit(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "shared.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.DefaultConfig()
	cfg.Auth.BCryptCost = 4
	s := NewAuthServer(cfg, database.NewSQLiteAccountStore(db), auth.NewTicketStore(0))
	l := auth.NewLimiter()
	if err := s.SetLimiter(l, nil); err != nil {
		t.Fatal(err)
	}
	if !login(t, s, "shared", "right") {
		t.Fatal("setup: the account did not register")
	}
	pages := NewAccountServer(db, 0, 4)
	pages.Limiter = l
	pages.EndSessions = func(int) int { return 0 }
	logout := func(password string) map[string]any {
		body := strings.NewReader(fmt.Sprintf(`{"username":"shared","password":%q}`, password))
		req := httptest.NewRequest(http.MethodPost, "/api/logout", body)
		req.RemoteAddr = "203.0.113.30:5555"
		rec := httptest.NewRecorder()
		pages.handleAPILogoutSessions(rec, req)
		var out map[string]any
		json.NewDecoder(rec.Body).Decode(&out)
		return out
	}
	if out := logout("right"); out["success"] != true {
		t.Fatalf("the right password: %v", out)
	}
	// half the guesses in the game, half on the page: they add up
	for i := 0; i < auth.FailsPerName/2; i++ {
		loginFrom(t, s, "shared", "wrong", "203.0.113.31")
		logout("wrong")
	}
	out := logout("right")
	if out["success"] == true || !strings.HasPrefix(fmt.Sprint(out["error"]), "Too many failed logins") {
		t.Fatalf("after guesses spread over the game and the page: %v", out)
	}
	if r := loginFrom(t, s, "shared", "right", "203.0.113.32"); succeeded(t, r) {
		t.Fatal("the game's login let the right password through after the page's guesses")
	}
}

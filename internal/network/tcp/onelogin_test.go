package tcp

import (
	"bytes"
	"net"
	"testing"
)

// One login at a time: an account already in the game is refused, and so is
// an address with a session in it unless the address is exempt. The refusal
// is STATION's failure shape (so a PS2 shows its "Your login failed") with the
// reason in the text field at 12, which the browser client shows.
func TestOneLoginRefusesASecondWindow(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)
	if !login(t, s, "first", "pw") {
		t.Fatal("setup: the account did not register")
	}

	var inGameAccount int
	inGameIP := net.ParseIP("203.0.113.7")
	var gotCheckIP bool
	live := func(accountID int, ip net.IP, _ func(*net.UDPAddr) net.IP) string {
		gotCheckIP = ip != nil
		if accountID == inGameAccount {
			return "account"
		}
		if ip != nil && ip.Equal(inGameIP) {
			return "address"
		}
		return ""
	}
	if err := s.SetOneLogin(live, []string{"198.51.100.0/24", "192.0.2.9"}); err != nil {
		t.Fatal(err)
	}
	try := func(ip string) []byte {
		return s.handleLoginFrom(buildLoginPayload(t, "first", "pw"), net.ParseIP(ip))
	}
	reason := func(r []byte) string { return string(bytes.TrimRight(r[12:76], "\x00")) }

	// control: nobody in the game, any address
	if r := try("203.0.113.50"); !succeeded(t, r) {
		t.Fatal("a login with nothing in the game was refused")
	}

	// the account is in the game: refused from anywhere, exempt or not
	inGameAccount = accountIDOf(t, s, "first")
	for _, ip := range []string{"203.0.113.50", "198.51.100.4"} {
		if r := try(ip); succeeded(t, r) || reason(r) != alreadyLoggedIn {
			t.Errorf("%s: the account is in the game, got success=%v reason %q", ip, succeeded(t, r), reason(r))
		}
	}
	inGameAccount = -1

	// another account plays from 203.0.113.7: refused there, exempt addresses not asked
	if r := try("203.0.113.7"); succeeded(t, r) || reason(r) != alreadyPlayingHere {
		t.Errorf("same address: got success=%v reason %q", succeeded(t, r), reason(r))
	}
	for _, ip := range []string{"198.51.100.4", "192.0.2.9", "127.0.0.1"} {
		if r := try(ip); !succeeded(t, r) || gotCheckIP {
			t.Errorf("%s is exempt: success=%v, address checked=%v", ip, succeeded(t, r), gotCheckIP)
		}
	}
	// control: 192.0.2.9 is exempt as one address, not its neighbours
	if try("192.0.2.10"); !gotCheckIP {
		t.Error("192.0.2.10 was treated as exempt")
	}

	// a bad exemption is an error, not silently ignored
	if err := s.SetOneLogin(live, []string{"not-an-ip"}); err == nil {
		t.Error("a malformed exemption was accepted")
	}
}

func accountIDOf(t *testing.T, s *AuthServer, name string) int {
	t.Helper()
	a, err := s.store.GetByUsername(name)
	if err != nil || a == nil {
		t.Fatalf("account %s: %v", name, err)
	}
	return a.ID
}

// A login through the web relay is refused when its account has the limit's characters in the game through it, in words
// the page shows; one under the limit, and a native client's, are not (auth.browser_characters).
func TestABrowsersCharacterLimit(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)
	if !login(t, s, "first", "pw") {
		t.Fatal("setup: the account did not register")
	}
	relay := net.ParseIP("192.0.2.9")
	s.SetRelay([]net.IP{relay}, func(int) (net.IP, bool) { return net.ParseIP("203.0.113.50"), true })
	live := 0
	var sawRelay, sawOther bool
	s.SetBrowserLimit(4, func(_ int, from func(*net.UDPAddr) bool) int {
		sawRelay = from(&net.UDPAddr{IP: relay})
		sawOther = from(&net.UDPAddr{IP: net.ParseIP("203.0.113.50")})
		return live
	})
	login := func(relayed bool) []byte {
		return s.handleLoginVia(buildLoginPayload(t, "first", "pw"), net.ParseIP("203.0.113.50"), relayed)
	}
	reason := func(r []byte) string { return string(bytes.TrimRight(r[12:76], "\x00")) }
	for live = 0; live < 4; live++ {
		if r := login(true); !succeeded(t, r) {
			t.Errorf("%d in a browser: a fifth place refused: %q", live, reason(r))
		}
	}
	if !sawRelay || sawOther {
		t.Errorf("what it counts as relayed: the relay %v, another %v", sawRelay, sawOther)
	}
	live = 4
	r := login(true)
	if succeeded(t, r) || reason(r) != "This account has 4 characters in a browser already." {
		t.Errorf("4 in a browser: success=%v reason %q", succeeded(t, r), reason(r))
	}
	if len(reason(r)) > 63 {
		t.Errorf("the words run past the field: %d", len(reason(r)))
	}
	if r := login(false); !succeeded(t, r) {
		t.Errorf("a native client past the limit: refused %q", reason(r))
	}
}

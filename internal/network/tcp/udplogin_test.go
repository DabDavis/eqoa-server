package tcp

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/auth"
)

var udpFrom = &net.UDPAddr{IP: net.IPv4(203, 0, 113, 5), Port: 4000}

// A client with no ticket (a restart forgot it) signs in by its token's
// password, checked as a login is, and gets the ticket; a wrong password, an
// account that does not exist (never made over UDP) and a banned one do not.
func TestUDPPasswordSignsInWithNoTicket(t *testing.T) {
	s, store := newLoginTestServer(t, nil)
	if !login(t, s, "dave", "pw") {
		t.Fatal("setup login failed")
	}
	s.tickets.Revoke("dave")

	id, why := s.CheckUDPPassword("dave", "pw", udpFrom)
	if why != "" || id == 0 {
		t.Fatalf("the right password over UDP: account %d, refused %q", id, why)
	}
	if _, ok := s.tickets.Valid("dave"); !ok {
		t.Error("no ticket after a UDP login")
	}
	s.tickets.Revoke("dave")
	if _, why := s.CheckUDPPassword("dave", "wrong", udpFrom); why != "password" {
		t.Errorf("a wrong password over UDP: refused %q", why)
	}
	if _, ok := s.tickets.Valid("dave"); ok {
		t.Error("a refused UDP login granted a ticket")
	}
	if _, why := s.CheckUDPPassword("nobody", "pw", udpFrom); why != "password" {
		t.Errorf("an unknown account over UDP: refused %q", why)
	}
	if _, err := store.GetByUsername("nobody"); err == nil {
		t.Error("a UDP login made an account")
	}
	if err := store.SetBanned(id, true); err != nil {
		t.Fatal(err)
	}
	if _, why := s.CheckUDPPassword("dave", "pw", udpFrom); why != "password" {
		t.Errorf("a banned account over UDP: refused %q", why)
	}
}

// The guessing limit holds over UDP: past the failures an account is allowed,
// the right password is refused too. The control: a fresh limit lets it in.
func TestUDPPasswordCountsAgainstTheLimit(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)
	if !login(t, s, "dave", "pw") {
		t.Fatal("setup login failed")
	}
	s.tickets.Revoke("dave")
	if err := s.SetLimiter(auth.NewLimiter(), nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < auth.FailsPerName; i++ {
		s.CheckUDPPassword("dave", "wrong", udpFrom)
	}
	if _, why := s.CheckUDPPassword("dave", "pw", udpFrom); why != "password" {
		t.Errorf("past the limit the right password over UDP: refused %q", why)
	}
	if err := s.SetLimiter(auth.NewLimiter(), nil); err != nil {
		t.Fatal(err)
	}
	if _, why := s.CheckUDPPassword("dave", "pw", udpFrom); why != "" {
		t.Errorf("the control: a fresh limit refused the right password (%q)", why)
	}
}

// One login at a time holds for the address over UDP: another account playing
// from it refuses ("address"); the account's own session is the player coming
// back, and passes.
func TestUDPPasswordKeepsOneLoginAtAnAddress(t *testing.T) {
	s, _ := newLoginTestServer(t, nil)
	if !login(t, s, "dave", "pw") {
		t.Fatal("setup login failed")
	}
	s.tickets.Revoke("dave")
	hit := "address"
	if err := s.SetOneLogin(func(int, net.IP, func(*net.UDPAddr) net.IP) string { return hit }, nil); err != nil {
		t.Fatal(err)
	}
	if _, why := s.CheckUDPPassword("dave", "pw", udpFrom); why != UDPRefusedAddress {
		t.Errorf("another account at the address: refused %q, want %q", why, UDPRefusedAddress)
	}
	if _, ok := s.tickets.Valid("dave"); ok {
		t.Error("a login refused for its address granted a ticket")
	}
	hit = "account"
	if _, why := s.CheckUDPPassword("dave", "pw", udpFrom); why != "" {
		t.Errorf("the account's own session (coming back): refused %q", why)
	}
}

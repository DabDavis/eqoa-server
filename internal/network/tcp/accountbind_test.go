package tcp

import (
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The account server listens where auth.bind_address says, and on every
// address when it is unset, as it did before the setting.
func TestAccountServerBindAddress(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "bind.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewAccountServer(db, 8080, 4)
	if got := s.ListenAddr(); got != ":8080" {
		t.Fatalf("unset: %q, want every address", got)
	}
	s.BindAddress = "127.0.0.1"
	if got := s.ListenAddr(); got != "127.0.0.1:8080" {
		t.Fatalf("127.0.0.1: %q", got)
	}
	s.BindAddress = "::1"
	if got := s.ListenAddr(); got != "[::1]:8080" {
		t.Fatalf("::1: %q", got)
	}

	// a real start on loopback answers there
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	live := NewAccountServer(db, port, 4)
	live.BindAddress = "127.0.0.1"
	if err := live.Start(); err != nil {
		t.Fatal(err)
	}
	defer live.server.Close()
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	var resp *http.Response
	for i := 0; i < 100; i++ {
		if resp, err = http.Get(url); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("nothing listening on %s: %v", url, err)
	}
	resp.Body.Close()
	if live.server.Addr != "127.0.0.1:"+strconv.Itoa(port) {
		t.Fatalf("listened on %q", live.server.Addr)
	}
}

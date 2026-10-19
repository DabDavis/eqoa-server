package tcp

import (
	"net/http/httptest"
	"testing"
)

// A proxy on loopback names the client; anyone else's headers are ignored.
func TestClientIPTrustsOnlyALoopbackProxy(t *testing.T) {
	s := &AccountServer{}
	req := func(peer, real, fwd string) string {
		r := httptest.NewRequest("POST", "/api/register", nil)
		r.RemoteAddr = peer
		if real != "" {
			r.Header.Set("X-Real-IP", real)
		}
		if fwd != "" {
			r.Header.Set("X-Forwarded-For", fwd)
		}
		return s.getClientIP(r)
	}
	cases := []struct{ peer, real, fwd, want string }{
		{"127.0.0.1:5000", "203.0.113.7", "198.51.100.1, 203.0.113.7", "203.0.113.7"},
		{"127.0.0.1:5000", "", "198.51.100.1, 203.0.113.7", "203.0.113.7"},
		{"[::1]:5000", "203.0.113.8", "", "203.0.113.8"},
		{"203.0.113.9:5000", "198.51.100.1", "198.51.100.2", "203.0.113.9"},
		{"203.0.113.9:5000", "", "", "203.0.113.9"},
	}
	for _, c := range cases {
		if got := req(c.peer, c.real, c.fwd); got != c.want {
			t.Errorf("peer %s real %q fwd %q: %s, want %s", c.peer, c.real, c.fwd, got, c.want)
		}
	}
}

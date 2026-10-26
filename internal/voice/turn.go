package voice

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/turn/v4"
)

// The optional TURN server (voice.turn.port), for a network that lets a
// client reach neither the media port's UDP nor its TCP: its clients
// authenticate with the REST credentials the game mints per session
// (Credential over Username, under voice.turn.secret), and it relays to the
// forwarding server ONLY, so it is no open relay.

// TURNConfig is the TURN server's.
type TURNConfig struct {
	Port   int // UDP and TCP; 0: no TURN server
	Secret string
	Realm  string // "" "eqoa"
	// the relay ports, 49400-50399 when unset
	RelayMinPort, RelayMaxPort int
}

// turnAllocations is how many allocations one username (one session) may
// hold at once: one connection over UDP and TCP, and a reconnect's overlap.
const turnAllocations = 4

type turnServer struct {
	srv  *turn.Server
	urls []string

	mu     sync.Mutex
	allocs map[string]int
}

func startTURN(cfg TURNConfig, bindIP string, public net.IP) (*turnServer, error) {
	if cfg.Secret == "" {
		return nil, fmt.Errorf("voice: voice.turn.port without voice.turn.secret")
	}
	realm := cfg.Realm
	if realm == "" {
		realm = "eqoa"
	}
	lo, hi := cfg.RelayMinPort, cfg.RelayMaxPort
	if lo == 0 && hi == 0 {
		lo, hi = 49400, 50399
	}
	if lo <= 0 || hi > 65535 || lo > hi {
		return nil, fmt.Errorf("voice: TURN relay ports %d-%d", lo, hi)
	}
	udp, tcp, port, err := listenPair(bindIP, cfg.Port)
	if err != nil {
		return nil, err
	}
	relayBind := bindIP
	if relayBind == "" {
		relayBind = "0.0.0.0"
	}
	t := &turnServer{allocs: map[string]int{}}
	gen := func() turn.RelayAddressGenerator {
		return &turn.RelayAddressGeneratorPortRange{
			RelayAddress: public, Address: relayBind, MinPort: uint16(lo), MaxPort: uint16(hi),
		}
	}
	// the forwarding server is the one peer: its public address (and the one
	// its ports are opened on)
	bound := net.ParseIP(bindIP)
	perm := func(_ net.Addr, peer net.IP) bool {
		return peer.Equal(public) || bound != nil && peer.Equal(bound)
	}
	t.srv, err = turn.NewServer(turn.ServerConfig{
		Realm: realm,
		AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
			if exp, ok := usernameExpiry(username); !ok || time.Now().After(exp) {
				return nil, false
			}
			return turn.GenerateAuthKey(username, realm, Credential(cfg.Secret, username)), true
		},
		QuotaHandler: func(username, _ string, _ net.Addr) bool {
			t.mu.Lock()
			defer t.mu.Unlock()
			return t.allocs[username] < turnAllocations
		},
		EventHandler: turn.EventHandler{
			OnAllocationCreated: func(_, _ net.Addr, _, username, _ string, _ net.Addr, _ int) {
				t.mu.Lock()
				t.allocs[username]++
				t.mu.Unlock()
			},
			OnAllocationDeleted: func(_, _ net.Addr, _, username, _ string) {
				t.mu.Lock()
				if t.allocs[username]--; t.allocs[username] <= 0 {
					delete(t.allocs, username)
				}
				t.mu.Unlock()
			},
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen(), PermissionHandler: perm}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: tcp, RelayAddressGenerator: gen(), PermissionHandler: perm}},
	})
	if err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	host := net.JoinHostPort(public.String(), fmt.Sprint(port))
	t.urls = []string{"turn:" + host + "?transport=udp", "turn:" + host + "?transport=tcp"}
	log.Printf("Voice: TURN on %s (UDP and TCP), relay ports %d-%d", host, lo, hi)
	return t, nil
}

func (t *turnServer) close() {
	if t.srv != nil {
		t.srv.Close()
	}
}

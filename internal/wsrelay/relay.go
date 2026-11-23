// Package wsrelay holds a browser page's sockets on a real host.
//
// A page can open neither a UDP socket nor a raw TCP connection, and an EQOA
// client needs both: DRDP over UDP to the game server, TCP to the login
// server and the patch server's HTTP. Each of the page's sockets is one
// WebSocket here, and the relay holds the real socket behind it. The wire
// protocol is eqoa-port's tools/wsrelay.py, unchanged, so eqoa-port's web
// build and eqoa-rs's both use it:
//
//	/udp                      a UDP socket on an ephemeral port; each binary message is one
//	                          datagram, both ways, behind a 6-byte header: the far end's IPv4
//	                          address (first octet first) and port (big-endian)
//	/tcp?host=A.B.C.D&port=N  a TCP connection; the binary messages are its bytes
//	first, a text message: "ok LOCALADDR LOCALPORT", or "err REASON" before it closes
//
//	GET /resolve?name=NAME    a name's IPv4 address, as text
//	GET /local?to=A.B.C.D     this host's own address toward a host
//
// Unlike wsrelay.py it reaches a host only on the PORTS it is given: on a
// host with other services behind loopback, "any port of an allowed host"
// would make it a way in.
package wsrelay

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Config is what a Relay may reach and how long it holds a socket.
type Config struct {
	AllowUDP, AllowTCP AllowList
	// Origins a browser page may connect from ("https://example.org"). Empty:
	// any. A handshake without an Origin header is not a browser page's and
	// is let through: an Origin check keeps other sites' pages off the relay,
	// which is all it can do.
	Origins []string
	// MaxPerIP caps the sockets one client address holds at once (0: 8).
	MaxPerIP int
	// A UDP socket with no datagram either way for UDPIdle is closed (0:
	// 120 s); a TCP connection idle for TCPIdle likewise (0: 10 min).
	UDPIdle, TCPIdle time.Duration
	// ConnectTimeout bounds a TCP connect (0: 10 s).
	ConnectTimeout time.Duration
	// Ping is the interval of the WebSocket pings that notice a client gone
	// silent (a browser answers them itself). 0: 30 s; negative: none.
	Ping time.Duration
	// PingTimeout is how long a ping may go unanswered before the client is
	// dropped (0: Ping). A stalled TCP path under the WebSocket (a lossy
	// line's retransmit backoff) can outlast one interval and still recover;
	// past the game client's own 60 s give-up, its timer decides instead.
	PingTimeout time.Duration
	// WriteWait bounds one write to the WebSocket or TCP peer (0: 10 s): a stalled path
	// that holds a write longer drops the client.
	WriteWait time.Duration
	Verbose   bool
	// Logf receives one line per socket opened, refused and closed (nil:
	// log.Printf).
	Logf func(format string, args ...any)
}

// Relay is an http.Handler serving the protocol above.
type Relay struct {
	cfg   Config
	mu    sync.Mutex
	perIP map[string]int
	// Distinct sockets can share a local port across protocols, or when TCP
	// endpoints have different destinations. Keep each registration until
	// its own close, and answer only when the client is unambiguous.
	owners map[int]map[*socketOwner]struct{}
}

type socketOwner struct {
	network string
	ip      string
}

const maxMessage = 1 << 20

// New fills in Config's defaults.
func New(cfg Config) *Relay {
	if cfg.MaxPerIP <= 0 {
		cfg.MaxPerIP = 8
	}
	if cfg.UDPIdle <= 0 {
		cfg.UDPIdle = 120 * time.Second
	}
	if cfg.TCPIdle <= 0 {
		cfg.TCPIdle = 10 * time.Minute
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.Ping == 0 {
		cfg.Ping = 30 * time.Second
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = cfg.Ping
	}
	if cfg.WriteWait <= 0 {
		cfg.WriteWait = 10 * time.Second
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	for k, o := range cfg.Origins {
		cfg.Origins[k] = normOrigin(o)
	}
	return &Relay{cfg: cfg, perIP: map[string]int{}, owners: map[int]map[*socketOwner]struct{}{}}
}

// own records that the relay's local port carries client ip until the
// returned function is called.
func (r *Relay) own(network string, port int, ip string) func() {
	r.mu.Lock()
	owner := &socketOwner{network: network, ip: ip}
	if r.owners[port] == nil {
		r.owners[port] = make(map[*socketOwner]struct{})
	}
	r.owners[port][owner] = struct{}{}
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.owners[port], owner)
		if len(r.owners[port]) == 0 {
			delete(r.owners, port)
		}
	}
}

// ClientOf serves older callers that only know the port.
func (r *Relay) ClientOf(port int) (string, bool) {
	return r.clientOf("", port)
}

func (r *Relay) clientOf(network string, port int) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found string
	for owner := range r.owners[port] {
		if network != "" && owner.network != network {
			continue
		}
		if found != "" && found != owner.ip {
			return "", false
		}
		found = owner.ip
	}
	return found, found != ""
}

// Lookup serves GET /client?port=N&network=tcp (or udp): the client's address as
// text, or 404. It names players' addresses, so it is served on its own
// listener for the relayed server alone (wsrelay -lookup, on loopback), never
// beside the relay behind a public proxy.
func (r *Relay) Lookup() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		port, err := strconv.Atoi(q.Get("port"))
		if req.URL.Path != "/client" || err != nil {
			http.NotFound(w, req)
			return
		}
		ip, ok := r.clientOf(q.Get("network"), port)
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, ip)
	})
}

func normOrigin(o string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/")
}

func (r *Relay) logf(format string, args ...any) { r.cfg.Logf(format, args...) }

func (r *Relay) vlogf(format string, args ...any) {
	if r.cfg.Verbose {
		r.cfg.Logf(format, args...)
	}
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/udp", "/tcp":
		r.socket(w, req)
	case "/resolve":
		addr, ok := resolve(req.Context(), req.URL.Query().Get("name"))
		r.text(w, addr, ok)
	case "/local":
		to, err := netip.ParseAddr(req.URL.Query().Get("to"))
		addr := "127.0.0.1"
		if err == nil {
			addr = localToward(to)
		}
		r.text(w, addr, true)
	default:
		http.NotFound(w, req)
	}
}

func (r *Relay) text(w http.ResponseWriter, s string, ok bool) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
	}
	fmt.Fprintln(w, s)
}

func resolve(ctx context.Context, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", name)
	if err != nil || len(ips) == 0 {
		return "", false
	}
	return ips[0].String(), true
}

// localToward is this host's source address toward ip, as wsrelay.py's
// local_toward finds it: connect a UDP socket (which sends nothing) and read
// its local address.
func localToward(ip netip.Addr) string {
	c, err := net.Dial("udp4", netip.AddrPortFrom(ip.Unmap(), 9).String())
	if err != nil {
		return "127.0.0.1"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// ClientIP is the address a socket is counted and logged against: the
// peer's, or behind a proxy on loopback (nginx) the one the proxy names.
// X-Real-IP comes first because nginx overwrites it; of X-Forwarded-For only
// the LAST entry is trusted, since $proxy_add_x_forwarded_for appends to
// whatever the client sent and the first entry is the client's to choose.
func ClientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !peer.IsLoopback() {
		return host
	}
	if v := strings.TrimSpace(req.Header.Get("X-Real-IP")); v != "" {
		if a, err := netip.ParseAddr(v); err == nil {
			return a.Unmap().String()
		}
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if a, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
			return a.Unmap().String()
		}
	}
	return host
}

func (r *Relay) acquire(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perIP[ip] >= r.cfg.MaxPerIP {
		return false
	}
	r.perIP[ip]++
	return true
}

func (r *Relay) release(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perIP[ip]--; r.perIP[ip] <= 0 {
		delete(r.perIP, ip)
	}
}

// Held is how many sockets ip holds now.
func (r *Relay) Held(ip string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.perIP[ip]
}

func (r *Relay) originOK(req *http.Request) bool {
	o := req.Header.Get("Origin")
	if len(r.cfg.Origins) == 0 || o == "" {
		return true
	}
	o = normOrigin(o)
	for _, want := range r.cfg.Origins {
		if o == want {
			return true
		}
	}
	return false
}

// socket admits a WebSocket for /udp or /tcp and holds it until it ends.
//
// A session ends through the WebSocket: whatever finishes it (the client, an
// idle timeout, the far server closing, an unanswered ping) closes the
// WebSocket, which ends the read loop, and only then is the context
// cancelled, so a clean end sends the client a close frame rather than
// dropping the connection under it.
func (r *Relay) socket(w http.ResponseWriter, req *http.Request) {
	ip := ClientIP(req)
	kind := strings.TrimPrefix(req.URL.Path, "/")
	if !r.originOK(req) {
		r.logf("wsrelay: %s refused client=%s: origin %q", kind, ip, req.Header.Get("Origin"))
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !r.acquire(ip) {
		r.logf("wsrelay: %s refused client=%s: %d sockets already held", kind, ip, r.cfg.MaxPerIP)
		http.Error(w, "too many sockets", http.StatusTooManyRequests)
		return
	}
	defer r.release(ip)
	// The Origin is checked above; the library's own check compares it with
	// Host, which behind a proxy is the proxy's upstream address.
	c, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		r.vlogf("wsrelay: %s handshake from %s: %v", kind, ip, err)
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gone := new(reason)
	ctx = context.WithValue(ctx, reasonKey{}, gone)
	if r.cfg.Ping > 0 {
		go r.pinger(ctx, c, gone)
	}
	if kind == "udp" {
		r.udp(ctx, cancel, c, ip)
	} else {
		r.tcp(ctx, cancel, c, ip, req)
	}
}

// reason is why the relay itself dropped a client (its ping unanswered, a
// write that timed out), for the close line: "" when it did not.
type reason struct{ v atomic.Value }

type reasonKey struct{}

func (g *reason) set(s string) { g.v.CompareAndSwap(nil, s) }

func (g *reason) get() string {
	s, _ := g.v.Load().(string)
	return s
}

// relayReason is the relay's own reason for dropping the client on ctx, if any.
func relayReason(ctx context.Context) string {
	if g, ok := ctx.Value(reasonKey{}).(*reason); ok {
		return g.get()
	}
	return ""
}

// pinger drops a client that stops answering pings (a browser answers them
// itself; the page never sees one) within PingTimeout.
func (r *Relay) pinger(ctx context.Context, c *websocket.Conn, gone *reason) {
	t := time.NewTicker(r.cfg.Ping)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sent := time.Now()
			pctx, done := context.WithTimeout(ctx, r.cfg.PingTimeout)
			err := c.Ping(pctx)
			done()
			if err != nil {
				if ctx.Err() == nil {
					gone.set(fmt.Sprintf("ping unanswered for %s", time.Since(sent).Round(time.Second)))
					c.CloseNow()
				}
				return
			}
		}
	}
}

// activity is the last time a socket moved anything, for its idle timeout.
type activity struct{ at atomic.Int64 }

func (a *activity) touch()              { a.at.Store(time.Now().UnixNano()) }
func (a *activity) idle() time.Duration { return time.Since(time.Unix(0, a.at.Load())) }

// watchIdle calls fire once when a has been idle past limit.
func watchIdle(ctx context.Context, a *activity, limit time.Duration, fire func()) {
	step := limit / 4
	if step < 10*time.Millisecond {
		step = 10 * time.Millisecond
	}
	t := time.NewTicker(step)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.idle() > limit {
				fire()
				return
			}
		}
	}
}

func putHeader(h []byte, ap netip.AddrPort) {
	a := ap.Addr().Unmap().As4()
	copy(h, a[:])
	binary.BigEndian.PutUint16(h[4:], ap.Port())
}

// why says how a read loop ended, for the close line.
func why(err error, idled bool, other string) string {
	switch {
	case idled:
		return "idle"
	case other != "":
		return other
	case websocket.CloseStatus(err) != -1:
		return fmt.Sprintf("client closed, %d", websocket.CloseStatus(err))
	default:
		return fmt.Sprintf("client gone: %v", err)
	}
}

func (r *Relay) udp(ctx context.Context, cancel context.CancelFunc, c *websocket.Conn, ip string) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		r.logf("wsrelay: udp client=%s: %v", ip, err)
		c.Close(websocket.StatusInternalError, "no socket")
		return
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port
	defer r.own("udp", port, ip)()
	toward, ok := r.cfg.AllowUDP.First()
	if !ok {
		toward, _ = r.cfg.AllowTCP.First()
	}
	local := "127.0.0.1"
	if toward.IsValid() {
		local = localToward(toward)
	}
	if err := r.write(ctx, c, websocket.MessageText, []byte(fmt.Sprintf("ok %s %d", local, port))); err != nil {
		return
	}
	start := time.Now()
	r.logf("wsrelay: udp open client=%s port=%d", ip, port)
	var up, down, upN, downN, dropped atomic.Int64
	var act activity
	act.touch()
	var idled atomic.Bool
	go watchIdle(ctx, &act, r.cfg.UDPIdle, func() {
		idled.Store(true)
		go c.Close(websocket.StatusNormalClosure, "idle")
	})
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	// datagrams in, to the page
	go func() {
		frame := make([]byte, 6+65536)
		for {
			n, from, err := conn.ReadFromUDPAddrPort(frame[6:])
			if err != nil {
				return
			}
			if !r.cfg.AllowUDP.Allows(from.Addr(), from.Port()) {
				dropped.Add(1)
				r.vlogf("wsrelay: udp client=%s port=%d: dropped a datagram from %s", ip, port, from)
				continue
			}
			act.touch()
			putHeader(frame, from)
			if r.write(ctx, c, websocket.MessageBinary, frame[:6+n]) != nil {
				if ctx.Err() == nil {
					c.CloseNow()
				}
				return
			}
			down.Add(int64(n))
			downN.Add(1)
		}
	}()
	// datagrams out, from the page
	for {
		typ, m, rerr := c.Read(ctx)
		if rerr != nil {
			err = rerr
			break
		}
		if typ != websocket.MessageBinary || len(m) < 6 {
			continue
		}
		to := netip.AddrPortFrom(netip.AddrFrom4([4]byte(m[:4])), binary.BigEndian.Uint16(m[4:6]))
		if !r.cfg.AllowUDP.Allows(to.Addr(), to.Port()) {
			dropped.Add(1)
			r.vlogf("wsrelay: udp client=%s port=%d: dropped a datagram to %s", ip, port, to)
			continue
		}
		act.touch()
		if _, werr := conn.WriteToUDPAddrPort(m[6:], to); werr != nil {
			r.vlogf("wsrelay: udp client=%s port=%d: send to %s: %v", ip, port, to, werr)
			continue
		}
		up.Add(int64(len(m) - 6))
		upN.Add(1)
	}
	cancel()
	r.logf("wsrelay: udp close client=%s port=%d up=%d/%d down=%d/%d dropped=%d dur=%s (%s)",
		ip, port, up.Load(), upN.Load(), down.Load(), downN.Load(), dropped.Load(),
		time.Since(start).Round(time.Millisecond), why(err, idled.Load(), relayDropped(ctx)))
}

func (r *Relay) tcp(ctx context.Context, cancel context.CancelFunc, c *websocket.Conn, ip string, req *http.Request) {
	q := req.URL.Query()
	host, portStr := q.Get("host"), q.Get("port")
	target := net.JoinHostPort(host, portStr)
	refuse := func(reason string) {
		r.logf("wsrelay: tcp refused client=%s target=%s: %s", ip, target, reason)
		r.write(ctx, c, websocket.MessageText, []byte("err "+reason))
		c.Close(websocket.StatusNormalClosure, "")
	}
	addr, err := netip.ParseAddr(host)
	port, perr := strconv.ParseUint(portStr, 10, 16)
	if err != nil || !addr.Unmap().Is4() || perr != nil || port == 0 || !r.cfg.AllowTCP.Allows(addr, uint16(port)) {
		refuse(fmt.Sprintf("%s is not a host this relay reaches", target))
		return
	}
	dctx, done := context.WithTimeout(ctx, r.cfg.ConnectTimeout)
	conn, err := (&net.Dialer{}).DialContext(dctx, "tcp4", target)
	done()
	if err != nil {
		refuse(fmt.Sprintf("%s: %v", target, err))
		return
	}
	defer conn.Close()
	me := conn.LocalAddr().(*net.TCPAddr)
	defer r.own("tcp", me.Port, ip)()
	if r.write(ctx, c, websocket.MessageText, []byte(fmt.Sprintf("ok %s %d", me.IP, me.Port))) != nil {
		return
	}
	start := time.Now()
	r.logf("wsrelay: tcp open client=%s target=%s local=%s", ip, target, me)
	var up, down atomic.Int64
	var act activity
	act.touch()
	var idled, serverEOF atomic.Bool
	go watchIdle(ctx, &act, r.cfg.TCPIdle, func() {
		idled.Store(true)
		go c.Close(websocket.StatusNormalClosure, "idle")
	})
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	// the stream in, to the page; the server's end of it ends the WebSocket
	go func() {
		buf := make([]byte, 65536)
		for {
			n, rerr := conn.Read(buf)
			if n > 0 {
				act.touch()
				if r.write(ctx, c, websocket.MessageBinary, buf[:n]) != nil {
					if ctx.Err() == nil {
						c.CloseNow()
					}
					return
				}
				down.Add(int64(n))
			}
			if rerr != nil {
				if ctx.Err() == nil {
					serverEOF.Store(true)
					c.Close(websocket.StatusNormalClosure, "")
				}
				return
			}
		}
	}()
	// the stream out, from the page
	other := ""
	for {
		typ, m, rerr := c.Read(ctx)
		if rerr != nil {
			err = rerr
			break
		}
		if typ != websocket.MessageBinary {
			continue
		}
		act.touch()
		conn.SetWriteDeadline(time.Now().Add(r.cfg.WriteWait))
		if _, werr := conn.Write(m); werr != nil {
			other = "server write failed"
			c.Close(websocket.StatusNormalClosure, "")
			break
		}
		up.Add(int64(len(m)))
	}
	if other == "" && serverEOF.Load() {
		other = "server closed"
	}
	if other == "" {
		other = relayDropped(ctx)
	}
	cancel()
	r.logf("wsrelay: tcp close client=%s target=%s up=%d down=%d dur=%s (%s)",
		ip, target, up.Load(), down.Load(), time.Since(start).Round(time.Millisecond), why(err, idled.Load(), other))
}

func (r *Relay) write(ctx context.Context, c *websocket.Conn, typ websocket.MessageType, p []byte) error {
	wctx, done := context.WithTimeout(ctx, r.cfg.WriteWait)
	defer done()
	err := c.Write(wctx, typ, p)
	if err != nil && !errors.Is(err, context.Canceled) {
		r.vlogf("wsrelay: write: %v", err)
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			if g, ok := ctx.Value(reasonKey{}).(*reason); ok {
				g.set(fmt.Sprintf("a write held past %s", r.cfg.WriteWait))
			}
		}
	}
	return err
}

// relayDropped is relayReason's reason as the close line says it: "" when the
// relay did not drop the client itself.
func relayDropped(ctx context.Context) string {
	if s := relayReason(ctx); s != "" {
		return "dropped by the relay: " + s
	}
	return ""
}

package tcp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// Auth message types (big-endian)
const (
	MsgServerHello            = 0x04
	MsgDisabledFeature        = 0x01
	MsgLoginResponse          = 0x25
	MsgAcctCreateResponse     = 0x29
	MsgChangePasswordResponse = 0x2B

	MsgRequestChangePwd = 0x02
	MsgLoginRequest     = 0x24
	MsgSubmitAcctCreate = 0x38
	MsgChangePassword   = 0x2A
)

// serverHelloBlob is the 32 bytes that follow the length+opcode in the Server
// Hello (0x04).
//
// It was assumed to be a random per-connection challenge, and we generated it
// with crypto/rand. It is NOT random: a live capture of eqoa.live from February
// and a fresh capture of Sandstorm in August are byte-identical, so the real
// servers send a CONSTANT here. Since neither server validates the password
// hash the client derives from it, a random value survives the login exchange —
// but the PS2's station layer opens a SECOND connection to this port after a
// game server is picked, and on that one it read our Hello and then sent
// nothing at all, ending in "Server not ready ... (nodbs)".
//
// Captured from a working server rather than invented.
var serverHelloBlob = []byte{
	0x3c, 0xbd, 0xb3, 0x13, 0xd0, 0x92, 0x03, 0xb7,
	0x41, 0x7e, 0x90, 0x12, 0x3e, 0x32, 0xc2, 0xea,
	0xb3, 0xc8, 0xd6, 0xc3, 0x86, 0xdb, 0xfc, 0x6c,
	0x1d, 0x29, 0x27, 0xf5, 0xb0, 0xb8, 0xb2, 0xcf,
}

// Account status
const (
	StatusInactive = 0x00
	StatusNormal   = 0x01
)

// Login results
const (
	ResultNoUser    = 0x00
	ResultBadPass   = 0x01
	ResultNameTaken = 0x02
	ResultError     = 0x07
	ResultGood      = 0x08
)

// Listener handles TCP connections for the auth server.
type Listener struct {
	listener net.Listener
	cfg      *config.Config
	handler  ConnectionHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ConnectionHandler processes a TCP connection.
type ConnectionHandler interface {
	HandleConnection(conn net.Conn)
}

// NewListener creates a new TCP listener.
func NewListener(cfg *config.Config, handler ConnectionHandler) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	return &Listener{
		cfg:     cfg,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start begins listening for TCP connections.
func (l *Listener) Start() error {
	listenAddr := l.cfg.Server.BindAddress + ":" + itoa(l.cfg.Server.AuthPort)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	l.listener = listener
	log.Printf("TCP auth listener started on %s", listenAddr)

	l.wg.Add(1)
	go l.acceptLoop()

	return nil
}

// Stop gracefully shuts down the listener.
func (l *Listener) Stop() {
	l.cancel()
	if l.listener != nil {
		l.listener.Close()
	}
	l.wg.Wait()
}

// acceptLoop accepts incoming connections.
func (l *Listener) acceptLoop() {
	defer l.wg.Done()

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		if tcpListener, ok := l.listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := l.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		l.wg.Add(1)
		go func(c net.Conn) {
			defer l.wg.Done()
			defer c.Close()

			if l.handler != nil {
				l.handler.HandleConnection(c)
			}
		}(conn)
	}
}

// SeedAccounts creates the accounts listed in config that do not exist yet,
// with their passwords hashed. An account that already exists is left untouched
// — otherwise every restart would reset a password the owner had changed.
func SeedAccounts(store *database.SQLiteAccountStore, seeds []config.AccountSeed, cost int) {
	if store == nil {
		return
	}
	created := 0
	for _, s := range seeds {
		if s.Username == "" {
			continue
		}
		if _, err := store.GetByUsername(s.Username); err == nil {
			continue // already present
		}
		hash, err := auth.HashPassword(s.Password, cost)
		if err != nil {
			log.Printf("Auth: could not hash seed password for %q: %v", s.Username, err)
			continue
		}
		if _, err := store.Create(s.Username, hash); err != nil {
			log.Printf("Auth: could not create seed account %q: %v", s.Username, err)
			continue
		}
		created++
	}
	if created > 0 {
		log.Printf("Auth: created %d seed account(s) from config", created)
	}
}

// AuthServer handles auth server TCP connections.
type AuthServer struct {
	cfg     *config.Config
	store   *database.SQLiteAccountStore
	tickets *auth.TicketStore

	// liveLogin, when set, names what a login collides with: "account" (that
	// account is already in the game) or "address" (a session from that IP
	// is; ip nil when not checked); see SetOneLogin.
	liveLogin func(accountID int, ip net.IP, ipOf func(*net.UDPAddr) net.IP) string
	exempt    []netip.Prefix
	// clientIP, when set, gives the client address behind a source: a
	// relayed browser player's page address (SetRelay); nil when unknown.
	clientIP func(ip net.IP, port int) net.IP

	// autoRegister is auth.allow_auto_register, which the GM panel turns
	// over while the server runs (SetAllowAutoRegister).
	autoRegister atomic.Bool

	// limiter counts every login before its password is checked (SetLimiter);
	// limitExempt are the addresses it does not count by address.
	limiter     *auth.Limiter
	limitExempt []netip.Prefix
}

// SetLimiter counts every login against l (the server's one limiter, which
// the account pages and the GM panel share): by the client's address (a
// relayed browser player's own, SetRelay) and by the account name, BEFORE
// the password is checked, and a refused login is answered as one with no
// password tried. exempt lists the addresses (IPs or CIDR prefixes) not
// counted by address: the operators' own, and the relay's, which every
// browser player comes through when the relay cannot name the page behind
// it. Their names are still counted.
func (s *AuthServer) SetLimiter(l *auth.Limiter, exempt []string) error {
	p, err := parsePrefixes(exempt, "login-limit exemption")
	if err != nil {
		return err
	}
	s.limiter, s.limitExempt = l, p
	return nil
}

// parsePrefixes reads a list of IPs and CIDR prefixes.
func parsePrefixes(list []string, what string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, e := range list {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			a, err := netip.ParseAddr(e)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %v", what, e, err)
			}
			e = netip.PrefixFrom(a, a.BitLen()).String()
		}
		p, err := netip.ParsePrefix(e)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %v", what, e, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// inPrefixes is whether ip is loopback or in one of the prefixes.
func inPrefixes(ip net.IP, prefixes []netip.Prefix) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() {
		return true
	}
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// SetAllowAutoRegister turns auth.allow_auto_register on or off from the next
// login on: an unknown username registering itself.
func (s *AuthServer) SetAllowAutoRegister(on bool) {
	s.autoRegister.Store(on)
}

// AllowAutoRegister is whether an unknown username registers itself now.
func (s *AuthServer) AllowAutoRegister() bool {
	return s.autoRegister.Load()
}

// SetRelay names the web relay's source addresses and how to ask it which
// page is behind one of its ports (wsrelay -lookup), so that the address rule
// sees a browser player's own address rather than the relay's, which every
// browser player shares. A relayed source the relay cannot name is unknown,
// and the address rule passes it.
func (s *AuthServer) SetRelay(addrs []net.IP, lookup func(port int) (net.IP, bool)) {
	s.clientIP = func(ip net.IP, port int) net.IP {
		for _, a := range addrs {
			if a.Equal(ip) {
				if c, ok := lookup(port); ok {
					return c
				}
				return nil
			}
		}
		return ip
	}
}

// RelayLookup asks wsrelay's lookup listener (base: "http://127.0.0.1:8792")
// for the client behind one of its local ports.
func RelayLookup(base string) func(port int) (net.IP, bool) {
	client := &http.Client{Timeout: time.Second}
	base = strings.TrimRight(base, "/")
	return func(port int) (net.IP, bool) {
		resp, err := client.Get(fmt.Sprintf("%s/client?port=%d", base, port))
		if err != nil {
			return nil, false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, false
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		if err != nil {
			return nil, false
		}
		ip := net.ParseIP(strings.TrimSpace(string(b)))
		return ip, ip != nil
	}
}

// SetOneLogin turns on one login at a time: an account already in the game,
// or an address that already has a session in it, is refused at the Station
// login, so the first window keeps playing and a second one is told why (no
// two windows on one character, and no multiboxing). exempt lists the
// addresses (IPs or CIDR prefixes) the address rule does not apply to: the
// operators' own, and the web relay's, which every browser player comes
// through (the relay limits those itself, by the page's address).
func (s *AuthServer) SetOneLogin(live func(accountID int, ip net.IP, ipOf func(*net.UDPAddr) net.IP) string, exempt []string) error {
	p, err := parsePrefixes(exempt, "one-login exemption")
	if err != nil {
		return err
	}
	s.liveLogin, s.exempt = live, p
	return nil
}

func (s *AuthServer) exemptIP(ip net.IP) bool {
	return inPrefixes(ip, s.exempt)
}

// NewAuthServer creates a new auth server handler.
//
// store and tickets may be nil only in tests that exercise framing rather than
// authentication; with a nil store every login is refused, because a server
// that cannot look an account up must not conclude the password was right.
func NewAuthServer(cfg *config.Config, store *database.SQLiteAccountStore, tickets *auth.TicketStore) *AuthServer {
	s := &AuthServer{cfg: cfg, store: store, tickets: tickets}
	s.autoRegister.Store(cfg.Auth.AllowAutoRegister)
	return s
}

// HandleConnection processes an auth server connection.
// Protocol verified against eqoa.live pcap (2026-02-14):
//  1. Server sends Hello (40 bytes: length + opcode 0x04 + 32-byte challenge)
//  2. Client sends Login (148 bytes: length + opcode 0x24 + fields)
//  3. Server sends Response (168 bytes: length + opcode 0x25 + fields)
//
// All values big-endian. Length field is self-inclusive.
func (s *AuthServer) HandleConnection(conn net.Conn) {
	log.Printf("Auth connection from %s", conn.RemoteAddr())

	// Send Server Hello (40 bytes)
	hello := make([]byte, 40)
	binary.BigEndian.PutUint32(hello[0:4], 40)
	binary.BigEndian.PutUint32(hello[4:8], MsgServerHello)
	copy(hello[8:40], serverHelloBlob)

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(hello); err != nil {
		log.Printf("Auth: failed to send hello to %s: %v", conn.RemoteAddr(), err)
		return
	}

	// Loop to handle multiple requests on the same connection.
	// The client reuses the connection when navigating between pages
	// (e.g. login → create account → back → login).
	buf := make([]byte, 512)
	for {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("Auth: client %s disconnected", conn.RemoteAddr())
			return
		}

		if n < 8 {
			log.Printf("Auth: packet too short from %s (%d bytes)", conn.RemoteAddr(), n)
			return
		}

		data := buf[:n]

		// Packet format: [4 length BE self-inclusive] [4 opcode BE] [payload]
		pktLen := int(binary.BigEndian.Uint32(data[0:4]))
		if pktLen != n {
			log.Printf("Auth: length mismatch from %s (header=%d, received=%d)", conn.RemoteAddr(), pktLen, n)
			return
		}

		opcode := binary.BigEndian.Uint32(data[4:8])
		payload := data[8:] // everything after length + opcode

		var response []byte
		switch opcode {
		case MsgLoginRequest:
			var ip net.IP
			if a, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
				ip = a.IP
				if s.clientIP != nil {
					ip = s.clientIP(a.IP, a.Port)
				}
			}
			response = s.handleLoginFrom(payload, ip)
		case MsgSubmitAcctCreate:
			response = s.handleCreateAccount(payload)
		case MsgChangePassword:
			response = s.handleChangePassword(payload)
		default:
			log.Printf("Auth: unknown opcode 0x%08X from %s", opcode, conn.RemoteAddr())
			return
		}

		if response != nil {
			conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := conn.Write(response); err != nil {
				log.Printf("Auth: write error to %s: %v", conn.RemoteAddr(), err)
				return
			}
		}
	}
}

// handleLogin processes a login request (opcode 0x24).
// Payload layout (after length+opcode, 140 bytes):
//
//	[0:4]   Unknown (0)
//	[4:8]   Version (2)
//	[8:40]  Username (32 bytes, null-padded ASCII)
//	[40:72] Password, DES-CBC encrypted (32 bytes) — see authcrypt.go
//	[72:76] Unknown (1)
//	[76:80] GameID ("EQOA")
//	[80:140] Padding (zeros)
func (s *AuthServer) handleLogin(payload []byte) []byte {
	return s.handleLoginFrom(payload, nil)
}

// handleLoginFrom is handleLogin for a connection from ip (nil: unknown, and
// the address rule is not applied).
func (s *AuthServer) handleLoginFrom(payload []byte, ip net.IP) []byte {
	if len(payload) < 72 {
		log.Printf("Auth: login payload too short (%d bytes)", len(payload))
		return nil
	}

	username := extractString(payload[8:40])
	if username == "" {
		log.Printf("Auth: empty username in login")
		return nil
	}

	log.Printf("Auth: login request for '%s'", username)

	// Counted before the password is checked (SetLimiter): a guesser gets
	// FailsPerName tries at an account and FailsPerAddr from an address
	// every FailWindow, here and on the account pages and the GM panel
	// together, however many connections it opens.
	addr := ""
	if ip != nil && !inPrefixes(ip, s.limitExempt) {
		addr = ip.String()
	}
	var release func()
	if s.limiter != nil {
		if wait := s.limiter.Begin(addr, username); wait > 0 {
			log.Printf("Auth: refused '%s' from %v: too many failed logins", username, ip)
			return s.buildLoginRefusedResponse(0, fmt.Sprintf(tooManyLogins, auth.RetryMinutes(wait)))
		}
		var free bool
		if release, free = s.limiter.Slot(10 * time.Second); !free {
			log.Printf("Auth: refused '%s' from %v: no free password check", username, ip)
			return s.buildLoginRefusedResponse(0, serverBusy)
		}
	}
	accountID, ok, registered := s.authenticate(username, payload[40:72])
	if release != nil {
		release()
	}
	if !ok {
		// The account ID is echoed even on failure, matching a real server's
		// response shape. 0 is correct for an account we could not identify.
		return s.buildLoginFailResponse(uint32(accountID))
	}
	if s.limiter != nil {
		if registered {
			s.limiter.Succeed("", username) // the address keeps the attempt
		} else {
			s.limiter.Succeed(addr, username)
		}
	}

	// One login at a time (SetOneLogin): the window already in the game
	// keeps it. Checked after the password, so a refusal says nothing about
	// an account to someone who does not hold it.
	if s.liveLogin != nil {
		checkIP := ip
		if ip != nil && s.exemptIP(ip) {
			checkIP = nil
		}
		var ipOf func(*net.UDPAddr) net.IP
		if s.clientIP != nil {
			ipOf = func(a *net.UDPAddr) net.IP { return s.clientIP(a.IP, a.Port) }
		}
		switch s.liveLogin(accountID, checkIP, ipOf) {
		case "account":
			log.Printf("Auth: refused '%s' (ID: %d) from %s: the account is already in the game", username, accountID, ip)
			return s.buildLoginRefusedResponse(uint32(accountID), alreadyLoggedIn)
		case "address":
			log.Printf("Auth: refused '%s' (ID: %d) from %s: that address already has a session in the game", username, accountID, ip)
			return s.buildLoginRefusedResponse(uint32(accountID), alreadyPlayingHere)
		}
	}

	// Record that this username completed a login, so the UDP handshake can
	// require one. Without this the password check below is decorative.
	if s.tickets != nil {
		s.tickets.Grant(username, accountID)
	}

	// Build response (168 bytes) — matches eqoa.live byte-for-byte
	response := make([]byte, 168)
	binary.BigEndian.PutUint32(response[0:4], 168)              // Length (self-inclusive)
	binary.BigEndian.PutUint32(response[4:8], MsgLoginResponse) // Opcode 0x25
	// [8:12] Error flag: 0=success, 1=bad password (left as 0 for success)
	// [12:80] zeros
	binary.BigEndian.PutUint32(response[80:84], uint32(accountID)) // Account ID
	binary.BigEndian.PutUint32(response[84:88], 0x00000003)        // Status: 3=success, 4=bad password
	binary.BigEndian.PutUint32(response[88:92], 0x001e7f4f)        // Subtime
	binary.BigEndian.PutUint32(response[92:96], 0x001e7f4f)        // Partime
	copy(response[96:112], []byte("EQOA_SESSION_OK!"))             // Session token
	// [112:160] zeros
	binary.BigEndian.PutUint32(response[160:164], 0xFFFFFFFF)
	binary.BigEndian.PutUint32(response[164:168], 0xFFFFFFFF)

	log.Printf("Auth: login success for '%s' (ID: %d) from %v", username, accountID, ip)
	return response
}

// authenticate decrypts the password field and checks it against the account's
// stored credential. It returns the account ID, whether the login stands, and
// whether it registered the account (auth.allow_auto_register).
//
// Every failure returns the same refusal to the client and logs the reason
// server-side: an unknown username and a wrong password are indistinguishable
// on the wire, so the auth port cannot be used to enumerate account names.
func (s *AuthServer) authenticate(username string, passwordField []byte) (int, bool, bool) {
	if s.store == nil {
		log.Printf("Auth: REFUSING login for '%s' — no account store configured", username)
		return 0, false, false
	}

	password, err := decryptPassword(passwordField)
	if err != nil {
		log.Printf("Auth: login for '%s' rejected — %v", username, err)
		return 0, false, false
	}

	cost := s.cfg.Auth.BCryptCost
	autoRegister := s.autoRegister.Load()

	account, err := s.store.GetByUsername(username)
	if err != nil {
		if !errors.Is(err, database.ErrAccountNotFound) {
			log.Printf("Auth: account lookup failed for '%s': %v", username, err)
			return 0, false, false
		}
		if !autoRegister {
			log.Printf("Auth: login for '%s' rejected — no such account (auto-register disabled)", username)
			return 0, false, false
		}
		hash, err := auth.HashPassword(password, cost)
		if err != nil {
			log.Printf("Auth: could not hash password for new account '%s': %v", username, err)
			return 0, false, false
		}
		created, err := s.store.Create(username, hash)
		if err != nil {
			log.Printf("Auth: could not create account '%s': %v", username, err)
			return 0, false, false
		}
		log.Printf("Auth: registered new account '%s' (ID: %d) on first login", username, created.ID)
		return created.ID, true, true
	}

	if account.Banned {
		log.Printf("Auth: login for '%s' (ID: %d) rejected — account is banned", username, account.ID)
		return account.ID, false, false
	}

	switch auth.VerifyPassword(account.PasswordHash, password) {
	case auth.Accepted:
		s.store.UpdateLastLogin(account.ID)
		return account.ID, true, false

	case auth.AcceptedNeedsRehash:
		// The stored value was a legacy plaintext password. It matched, so
		// upgrade it in place -- this is the only moment the plaintext is known.
		if hash, err := auth.HashPassword(password, cost); err == nil {
			if err := s.store.UpdatePasswordHash(account.ID, hash); err != nil {
				log.Printf("Auth: could not upgrade stored password for '%s': %v", username, err)
			} else {
				log.Printf("Auth: upgraded '%s' from a plaintext password to bcrypt", username)
			}
		}
		s.store.UpdateLastLogin(account.ID)
		return account.ID, true, false

	case auth.NoCredential:
		// Created by the UDP path before password checking existed, so it has
		// no password at all. Adopt the one presented now, or refuse.
		if !autoRegister {
			log.Printf("Auth: login for '%s' rejected — account has no password set "+
				"and auto-register is disabled; set one in auth.accounts", username)
			return account.ID, false, false
		}
		if hash, err := auth.HashPassword(password, cost); err == nil {
			if err := s.store.UpdatePasswordHash(account.ID, hash); err != nil {
				log.Printf("Auth: could not set initial password for '%s': %v", username, err)
				return account.ID, false, false
			}
			log.Printf("Auth: adopted a password for '%s' (ID: %d), which had none set",
				username, account.ID)
		}
		s.store.UpdateLastLogin(account.ID)
		return account.ID, true, false

	default:
		log.Printf("Auth: login for '%s' (ID: %d) rejected — wrong password", username, account.ID)
		return account.ID, false, false
	}
}

// buildLoginFailResponse builds a bad password response (168 bytes).
// Differs from success at offset [8:12]=0x01 and [84:88]=0x04.
func (s *AuthServer) buildLoginFailResponse(accountID uint32) []byte {
	response := make([]byte, 168)
	binary.BigEndian.PutUint32(response[0:4], 168)
	binary.BigEndian.PutUint32(response[4:8], MsgLoginResponse)
	binary.BigEndian.PutUint32(response[8:12], 0x00000001) // Error flag: bad password
	binary.BigEndian.PutUint32(response[80:84], accountID)
	binary.BigEndian.PutUint32(response[84:88], 0x00000004) // Status: bad password
	binary.BigEndian.PutUint32(response[88:92], 0x001e7f4f)
	binary.BigEndian.PutUint32(response[92:96], 0x001e7f4f)
	copy(response[96:112], []byte("EQOA_SESSION_OK!"))
	binary.BigEndian.PutUint32(response[160:164], 0xFFFFFFFF)
	binary.BigEndian.PutUint32(response[164:168], 0xFFFFFFFF)
	return response
}

// The refusals' texts. STATION shows its own word 1394 ("Your login failed.")
// for any nonzero result and never reads this field (OnValidateLogin's s12);
// the browser client shows it.
const (
	alreadyLoggedIn    = "You are already logged in."
	alreadyPlayingHere = "Someone is already playing from your address."
	tooManyLogins      = "Too many failed logins. Try again in %d minutes."
	serverBusy         = "The server is busy. Try again in a moment."
)

// buildLoginRefusedResponse is a refusal that says why: the reply STATION
// reads (VIStationReply37) has the result at 8 and a 64-byte text at 12,
// NUL-terminated, so at most 63 characters.
func (s *AuthServer) buildLoginRefusedResponse(accountID uint32, why string) []byte {
	response := s.buildLoginFailResponse(accountID)
	copy(response[12:75], why)
	return response
}

// handleCreateAccount processes an account creation request (opcode 0x38).
// Payload (after length+opcode, 468 bytes):
//
//	[0:4]     Unknown (0)
//	[4:36]    Username (32 bytes, null-padded)
//	[36:68]   Password hash (32 bytes)
//	[68:84]   First name (16 bytes)
//	[132:148] Last name (16 bytes)
//	[148:164] Middle initial (16 bytes)
//	[212:228] Country (16 bytes)
//	[244:260] Postal code (16 bytes)
//	[276:292] Birth month (16 bytes)
//	[292:308] Birth day (16 bytes)
//	[308:324] Birth year (16 bytes)
//	[324:340] Gender (16 bytes)
//	[340:356] Email (16 bytes)
func (s *AuthServer) handleCreateAccount(payload []byte) []byte {
	if len(payload) < 36 {
		log.Printf("Auth: create account payload too short (%d bytes)", len(payload))
		return nil
	}

	username := extractString(payload[4:36])
	log.Printf("Auth: create account for '%s'", username)

	// No account is created here. This opcode carries no password field we have
	// decoded, so creating one would produce an account with no credential --
	// exactly the state that made every login succeed before. Registration
	// happens on first login instead (auth.allow_auto_register), which does
	// carry a password.
	//
	// Success is still reported: eqoa.live answers this way too, and the client
	// treats a failure here as a hard stop at the account-creation screen.
	if username != "" && s.store != nil {
		if _, err := s.store.GetByUsername(username); err == nil {
			log.Printf("Auth: account '%s' already exists", username)
		} else {
			log.Printf("Auth: '%s' will be registered on first login", username)
		}
	}

	response := make([]byte, 276)
	binary.BigEndian.PutUint32(response[0:4], 276)
	binary.BigEndian.PutUint32(response[4:8], MsgAcctCreateResponse)
	binary.BigEndian.PutUint32(response[12:16], 1) // success

	return response
}

// handleChangePassword processes a password change request (opcode 0x2A).
func (s *AuthServer) handleChangePassword(payload []byte) []byte {
	log.Printf("Auth: change password request")

	response := make([]byte, 272)
	binary.BigEndian.PutUint32(response[0:4], 272)
	binary.BigEndian.PutUint32(response[4:8], MsgChangePasswordResponse)
	binary.BigEndian.PutUint32(response[12:16], 1) // success

	return response
}

// itoa converts an int to a string.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}

	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

// extractString extracts a null-terminated string from a byte slice.
func extractString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

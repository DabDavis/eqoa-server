// Package gmweb is the GM panel: a web page for game masters, and the same
// work for an AI agent, over the server's database and its running world.
//
// Everything it does is an operation (ops.go): a name, a description an AI can
// read, a JSON Schema for its arguments and a handler. The page calls them at
// api/op/<name>; an AI agent calls the same ones as MCP tools (mcp.go) or
// through the OpenAPI description (openapi.go), so the three cannot disagree
// about what a GM may do.
//
// WHO: an account whose accountinfo.acctlevel is gm.min_level or more, as the
// in-game # commands are a GM's (handlers/devcmds.go); gm.admin_level for the
// server's own settings, raw SQL, backups, a restart and other accounts'
// levels. The level is read again on every request, so a GM demoted in the
// database is out at once. A person signs in with the account's password
// (and an authenticator code once enrolled, totp.go) and holds a cookie; an AI
// agent holds a bearer token, made in the panel (tokens.go) or through OAuth,
// where the GM signs in on the panel's own page (oauth.go).
//
// EVERY CHANGE IS AUDITED (audit.go) with the row before and after, by whom
// and through what, and a row change can be reverted from the log.
//
// LIVE: a row of npcs edited, added or deleted takes effect in the running
// world at once (live.go), as do a character's level, purse and place when it
// is online; what the game reads only at a login says so in the answer.
package gmweb

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

//go:embed static
var staticFS embed.FS

// Deps is what the panel works on, and the hooks into the rest of the server
// that main.go owns.
type Deps struct {
	Config     *config.Config
	ConfigPath string

	DB         *database.SQLiteDB
	World      *entity.World
	Sessions   *session.Manager
	Characters *database.SQLiteCharacterStore
	Accounts   *database.SQLiteAccountStore
	NPCs       *database.NPCStore

	// ApplyNewCharacterRules gives every creation path new rules
	// (world.start_level, world.start_all_spells).
	ApplyNewCharacterRules func(database.NewCharacterRules)
	// SetAutoRegister and AutoRegister are auth.allow_auto_register, live.
	SetAutoRegister func(bool)
	AutoRegister    func() bool
	// Restart saves everyone and starts the server again; nil: not offered.
	Restart func() error
	// Limiter counts sign-in attempts; the server's own, shared with the
	// game's login and the account pages, so guesses spread over them all add
	// up (nil: the panel keeps its own).
	Limiter *auth.Limiter
}

// Server is the GM panel's HTTP server.
type Server struct {
	d   Deps
	cfg config.GMConfig

	// public is gm.public_url parsed; nil when unset (http on Bind:Port).
	public *url.URL

	srv     *http.Server
	started time.Time
	build   string

	sessions *sessionStore
	limiter  *auth.Limiter
	tokens   *tokenCache
	oauth    *oauthState
	rate     *rateLimiter
	schema   *schemaCache

	// per address: unknown bearer tokens, authorization pages, client
	// registrations (and all of those together); per account: the page's
	// requests
	badTokens, authorizeRate, regRate, regAll, sessRate *bucketLimiter

	ops    map[string]*Op
	opList []*Op

	// settingsMu orders the settings page's writes to config.yaml
	settingsMu sync.Mutex

	dummyOnce sync.Once
	dummy     string
}

// New builds the panel. It creates its own tables (gm_audit, gm_totp,
// gm_tokens, gm_oauth_clients) if the database lacks them.
func New(d Deps) (*Server, error) {
	if d.Config == nil || d.DB == nil {
		return nil, errors.New("gmweb: a config and a database are required")
	}
	cfg := d.Config.GM
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.MinLevel < 1 {
		cfg.MinLevel = 1 // level 0 is every player
	}
	if cfg.AdminLevel < cfg.MinLevel {
		cfg.AdminLevel = cfg.MinLevel
	}
	if cfg.SessionIdle <= 0 {
		cfg.SessionIdle = 2 * time.Hour
	}
	if cfg.TokenRate <= 0 {
		cfg.TokenRate = 240
	}
	if cfg.LogPath == "" {
		cfg.LogPath = "gameserver.log"
	}
	if cfg.BackupDir == "" {
		cfg.BackupDir = "backups"
	}
	s := &Server{
		d:        d,
		cfg:      cfg,
		started:  time.Now(),
		build:    buildVersion(),
		sessions: newSessionStore(cfg.SessionIdle),
		limiter:  d.Limiter,
		rate:     newRateLimiter(cfg.TokenRate, time.Minute),
		oauth:    newOAuthState(),
		schema:   &schemaCache{},

		badTokens:     newBucketLimiter(20, 20),
		authorizeRate: newBucketLimiter(30, 20),
		regRate:       newBucketLimiter(0.5, 10),
		regAll:        newBucketLimiter(2, 60),
		sessRate:      newBucketLimiter(600, 300),
	}
	if s.limiter == nil {
		s.limiter = auth.NewLimiter()
	}
	if cfg.PublicURL != "" {
		u, err := url.Parse(strings.TrimRight(cfg.PublicURL, "/"))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("gm.public_url %q is not an http(s) URL", cfg.PublicURL)
		}
		s.public = u
	}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	s.tokens = newTokenCache(s)
	s.registerOps()
	return s, nil
}

// Handler is the panel's whole HTTP surface, for Start and for tests.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// the page
	mux.HandleFunc("/", s.handleIndex)
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", s.staticHandler(http.StripPrefix("/static/", http.FileServer(http.FS(sub)))))

	// a person's sign-in and the things only a person does
	mux.HandleFunc("/api/auth/session", s.handleSession)
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/totp", s.handleTOTP)
	mux.HandleFunc("/api/auth/token", s.handleTokenLogin)
	mux.HandleFunc("/api/tokens", s.handleTokens)

	// the operations, for the page and for an AI agent
	mux.HandleFunc("/api/op/", s.handleOp)
	mux.HandleFunc("/api/ops", s.handleOpList)
	mux.HandleFunc("/api/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("/api/guide", s.handleGuide)

	// MCP, and the OAuth it signs in through
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/.well-known/", s.handleWellKnown)
	mux.HandleFunc("/oauth/register", s.handleOAuthRegister)
	mux.HandleFunc("/oauth/authorize", s.handleOAuthAuthorize)
	mux.HandleFunc("/oauth/token", s.handleOAuthToken)
	mux.HandleFunc("/oauth/revoke", s.handleOAuthRevoke)

	return s.secure(mux)
}

// Start listens on gm.bind:gm.port.
func (s *Server) Start() error {
	addr := net.JoinHostPort(s.cfg.Bind, fmt.Sprint(s.cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gm panel: %w", err)
	}
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("GM panel: %v", err)
		}
	}()
	where := "http://" + addr + "/"
	if s.public != nil {
		where = s.public.String() + "/ (" + addr + ")"
	}
	totp := "an authenticator for the admin's tools"
	if s.cfg.RequireTOTP {
		totp = "an authenticator required of every GM"
	}
	log.Printf("GM panel on %s for acctlevel %d+ (admin %d+), %s", where, s.cfg.MinLevel, s.cfg.AdminLevel, totp)
	return nil
}

// Stop closes the listener.
func (s *Server) Stop() {
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.srv.Shutdown(ctx)
	}
}

// secure sets the headers every answer carries, and refuses an oversized body.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; "+
			"img-src 'self' data: blob:; connect-src 'self'; worker-src 'self' blob:; "+
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		if s.public != nil && s.public.Scheme == "https" {
			h.Set("Strict-Transport-Security", "max-age=63072000")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		defer func() {
			if v := recover(); v != nil {
				log.Printf("GM panel: panic on %s %s: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the page's files: kept by the browser, since the page
// names them with the build in the query.
func (s *Server) staticHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// handleIndex serves the page itself, with its scripts named by the build so
// a new build is never answered from an old cache.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	page, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "page missing", http.StatusInternalServerError)
		return
	}
	html := strings.ReplaceAll(string(page), "{{BUILD}}", url.QueryEscape(s.build))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html))
}

// publicBase is the panel's address as a client reaches it, without a
// trailing slash: gm.public_url, else the request's own host.
func (s *Server) publicBase(r *http.Request) string {
	if s.public != nil {
		return s.public.String()
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fromProxy(r) {
		if p := r.Header.Get("X-Forwarded-Proto"); p == "https" || p == "http" {
			scheme = p
		}
	}
	return scheme + "://" + r.Host
}

// cookiePath is the path the session cookie is sent on: the panel's own.
func (s *Server) cookiePath() string {
	if s.public != nil && s.public.Path != "" {
		return strings.TrimRight(s.public.Path, "/") + "/"
	}
	return "/"
}

// secureCookies is whether the cookie is kept to https.
func (s *Server) secureCookies() bool {
	return s.public != nil && s.public.Scheme == "https"
}

// fromProxy is whether the request came through a proxy on this machine,
// whose forwarding headers are then believed.
func fromProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// clientIP is the address the request came from: the proxy's X-Real-IP (else
// the last X-Forwarded-For entry) when it came through one on this machine.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if fromProxy(r) {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" && net.ParseIP(real) != nil {
			return real
		}
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
				return last
			}
		}
	}
	return host
}

// writeJSON answers with v as JSON, never cached.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// writeError answers {"error": msg} with the status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// buildVersion names this build: the commit it was built from (and whether
// the tree had changes), else the binary's start time.
func buildVersion() string {
	rev, dirty, at := "", false, ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				rev = kv.Value
			case "vcs.modified":
				dirty = kv.Value == "true"
			case "vcs.time":
				at = kv.Value
			}
		}
	}
	if rev == "" {
		return "dev-" + time.Now().UTC().Format("20060102150405")
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty" + time.Now().UTC().Format("150405")
	}
	_ = at
	return rev
}

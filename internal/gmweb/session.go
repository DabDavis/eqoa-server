package gmweb

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/auth"
)

// Who is asking: a person signed in with a cookie, or an AI agent (any client)
// with a bearer token.

const (
	csrfHeader = "X-CSRF-Token"

	// sessionMaxAge ends a browser session however busy it is.
	sessionMaxAge = 24 * time.Hour
)

// Scopes, as a token carries them; a person's session holds the widest its
// level allows.
const (
	scopeRead  = "read"  // look only
	scopeWrite = "write" // change game data and players
	scopeAdmin = "admin" // and the server's own settings, SQL, backups, levels
)

// actor is who an operation runs as.
type actor struct {
	AccountID int
	Username  string
	Level     int
	// Role is what the account is to the panel: roleAdmin, roleGM, or below a
	// GM the roles it holds (roleTester, roleStoryteller, or both:
	// "tester+storyteller"; roles.go).
	Role string
	// Admin is Level >= gm.admin_level AND an authenticator: the session
	// signed in with its code (or enrolled it), the token was made by such a
	// sign-in, and the account has it still. An admin's tools are the whole
	// server's, so the game password alone never reaches them.
	Admin   bool
	Scope   string // what this credential may do (scopeRead/Write/Admin)
	Via     string // "web", "token:<name>", "oauth:<client>"
	IP      string
	TokenID int64 // 0 for a browser session

	// mustEnrol: gm.require_totp and no authenticator yet; nothing runs
	// until one is enrolled.
	mustEnrol bool

	sess *gmSession // a browser session's, else nil
}

// may is whether the actor's credential and level allow work of that scope.
func (a *actor) may(scope string) bool {
	switch scope {
	case scopeRead:
		return true
	case scopeWrite:
		return a.Scope == scopeWrite || a.Scope == scopeAdmin
	case scopeAdmin:
		return a.Admin && a.Scope == scopeAdmin
	}
	return false
}

// gmSession is a person signed in through the page.
type gmSession struct {
	Token     string
	AccountID int
	Username  string
	CSRF      string
	IP        string
	Created   time.Time
	Seen      time.Time
	// MFA: the sign-in gave an authenticator's code, or the session enrolled
	// one since.
	MFA bool

	// totpSecret is an authenticator being enrolled, not yet confirmed (not kept over
	// a restart: an enrolment begun before one starts again)
	totpSecret string
	// saved is when Seen was last written to gm_sessions
	saved time.Time
}

// sessionStore is the browser sessions: in memory, and kept in table gm_sessions
// (the token by its hash, as gm_tokens keeps a token) so a server restart, a keep
// restart's re-exec included, does not sign everyone out (the user, 2026-11-10: "the
// gm panel is closing the session too soon. testers are trying to update and it boots
// them": every restart had emptied the map). A session's last use is written at most
// once a minute.
type sessionStore struct {
	mu   sync.Mutex
	idle time.Duration
	m    map[string]*gmSession
	db   *sql.DB // nil: memory alone
}

// sessionSeenEvery is how often a busy session's last use is written to gm_sessions.
const sessionSeenEvery = time.Minute

func newSessionStore(idle time.Duration, db *sql.DB) *sessionStore {
	return &sessionStore{idle: idle, m: map[string]*gmSession{}, db: db}
}

// dbExec runs a statement on gm_sessions, a failure logged: a session kept only in
// memory still works until a restart.
func (st *sessionStore) dbExec(query string, args ...any) int64 {
	if st.db == nil {
		return 0
	}
	res, err := st.db.Exec(query, args...)
	if err != nil {
		log.Printf("GM panel: sessions: %v", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// loadLocked is a session gm_sessions keeps for the token, put in memory; nil when it
// has none or it has run out (and is then deleted). Must be called with st.mu held.
func (st *sessionStore) loadLocked(token string, now time.Time) *gmSession {
	if st.db == nil {
		return nil
	}
	var (
		s             gmSession
		created, seen string
		mfa           int
	)
	err := st.db.QueryRow(`SELECT accountid, username, csrf, ip, created, seen, mfa FROM gm_sessions WHERE hash = ?`, hashToken(token)).
		Scan(&s.AccountID, &s.Username, &s.CSRF, &s.IP, &created, &seen, &mfa)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("GM panel: sessions: %v", err)
		}
		return nil
	}
	s.Token, s.Created, s.Seen, s.MFA, s.saved = token, parseStamp(created), parseStamp(seen), mfa != 0, parseStamp(seen)
	if s.Created.IsZero() || now.Sub(s.Seen) > st.idle || now.Sub(s.Created) > sessionMaxAge {
		st.dbExec(`DELETE FROM gm_sessions WHERE hash = ?`, hashToken(token))
		return nil
	}
	st.m[token] = &s
	return &s
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the kernel's random source is gone; nothing is safe to hand out
	}
	return hex.EncodeToString(b)
}

func (st *sessionStore) create(accountID int, username, ip string, mfa bool) *gmSession {
	now := time.Now()
	s := &gmSession{
		Token: randomHex(32), AccountID: accountID, Username: username,
		CSRF: randomHex(24), IP: ip, Created: now, Seen: now, MFA: mfa,
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.pruneLocked(now)
	st.m[s.Token] = s
	s.saved = now
	st.dbExec(`INSERT OR REPLACE INTO gm_sessions (hash, accountid, username, csrf, ip, created, seen, mfa) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hashToken(s.Token), accountID, username, s.CSRF, ip, stamp(now), stamp(now), boolInt(mfa))
	return s
}

// get is the live session for a token, touched.
func (st *sessionStore) get(token string) *gmSession {
	if token == "" {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	s := st.m[token]
	if s == nil {
		// a session from before a restart: as gm_sessions keeps it
		if s = st.loadLocked(token, now); s == nil {
			return nil
		}
	}
	if now.Sub(s.Seen) > st.idle || now.Sub(s.Created) > sessionMaxAge {
		delete(st.m, token)
		st.dbExec(`DELETE FROM gm_sessions WHERE hash = ?`, hashToken(token))
		return nil
	}
	s.Seen = now
	if now.Sub(s.saved) >= sessionSeenEvery {
		s.saved = now
		st.dbExec(`UPDATE gm_sessions SET seen = ? WHERE hash = ?`, stamp(now), hashToken(token))
	}
	return s
}

func (st *sessionStore) end(token string) {
	st.mu.Lock()
	delete(st.m, token)
	st.mu.Unlock()
	st.dbExec(`DELETE FROM gm_sessions WHERE hash = ?`, hashToken(token))
}

// setMFA marks a session signed in with an authenticator (it has just enrolled one), in
// memory and in gm_sessions.
func (st *sessionStore) setMFA(s *gmSession) {
	st.mu.Lock()
	s.totpSecret = ""
	s.MFA = true
	st.mu.Unlock()
	st.dbExec(`UPDATE gm_sessions SET mfa = 1 WHERE hash = ?`, hashToken(s.Token))
}

// endAccount ends every browser session of an account (its password changed,
// it was banned or demoted), but keep.
func (st *sessionStore) endAccount(accountID int, keep string) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for tok, s := range st.m {
		if s.AccountID == accountID && tok != keep {
			delete(st.m, tok)
			n++
		}
	}
	// and those kept from before a restart that nobody has used since
	if st.db != nil {
		if keep == "" {
			n = max(n, int(st.dbExec(`DELETE FROM gm_sessions WHERE accountid = ?`, accountID)))
		} else {
			n = max(n, int(st.dbExec(`DELETE FROM gm_sessions WHERE accountid = ? AND hash != ?`, accountID, hashToken(keep))))
		}
	}
	return n
}

// clearMFA forgets every session's authenticator sign-in for an account (its
// authenticator was removed), so an admin's tools wait for a new one.
func (st *sessionStore) clearMFA(accountID int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.m {
		if s.AccountID == accountID {
			s.MFA = false
		}
	}
	st.dbExec(`UPDATE gm_sessions SET mfa = 0 WHERE accountid = ?`, accountID)
}

func (st *sessionStore) pruneLocked(now time.Time) {
	for tok, s := range st.m {
		if now.Sub(s.Seen) > st.idle || now.Sub(s.Created) > sessionMaxAge {
			delete(st.m, tok)
		}
	}
	// stamps are RFC 3339 in UTC, so they sort as text
	st.dbExec(`DELETE FROM gm_sessions WHERE seen < ? OR created < ?`, stamp(now.Add(-st.idle)), stamp(now.Add(-sessionMaxAge)))
}

// bucketLimiter is a token bucket per key (an address, an account): burst
// requests at once, refilled at rate a second. A key costs a few words
// however busy it is, and there are at most maxKeys of them.
type bucketLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	maxKeys int
	b       map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newBucketLimiter(perMinute float64, burst int) *bucketLimiter {
	return &bucketLimiter{rate: perMinute / 60, burst: float64(burst), maxKeys: 10000, b: map[string]*bucket{}}
}

// take spends n of key's tokens, if it has them.
func (l *bucketLimiter) take(key string, n float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.b[key]
	if b == nil {
		if len(l.b) >= l.maxKeys {
			for k, v := range l.b { // the full ones first: they lose nothing
				if v.tokens+now.Sub(v.last).Seconds()*l.rate >= l.burst {
					delete(l.b, k)
				}
			}
			for k := range l.b {
				if len(l.b) < l.maxKeys {
					break
				}
				delete(l.b, k)
			}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.b[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < n {
		return false
	}
	b.tokens -= n
	return true
}

func (l *bucketLimiter) allow(key string) bool { return l.take(key, 1) }

// empty is whether key has no token left, spending none.
func (l *bucketLimiter) empty(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.b[key]
	if b == nil {
		return false
	}
	return min(l.burst, b.tokens+time.Since(b.last).Seconds()*l.rate) < 1
}

// rateLimiter allows n requests a window per key (a token's id).
type rateLimiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	hits   map[int64][]time.Time
}

func newRateLimiter(n int, window time.Duration) *rateLimiter {
	return &rateLimiter{n: n, window: window, hits: map[int64][]time.Time{}}
}

func (r *rateLimiter) allow(key int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	h := r.hits[key]
	keep := h[:0]
	for _, t := range h {
		if now.Sub(t) < r.window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= r.n {
		r.hits[key] = keep
		return false
	}
	r.hits[key] = append(keep, now)
	return true
}

// accountState is an account's level, status and name as the database has
// them now.
type accountState struct {
	ID       int
	Username string
	Level    int
	Status   int // > 0 active, 0 inactive, < 0 banned
	PwHash   string
}

var errNoAccount = errors.New("no such account")

func (s *Server) accountByID(id int) (accountState, error) {
	var a accountState
	err := s.d.DB.QueryRow(`SELECT accountid, username, COALESCE(acctlevel, 0), COALESCE(acctstatus, 0),
		COALESCE(CAST(pwhash AS TEXT), '') FROM accountinfo WHERE accountid = ?`, id).
		Scan(&a.ID, &a.Username, &a.Level, &a.Status, &a.PwHash)
	if errors.Is(err, sql.ErrNoRows) {
		return a, errNoAccount
	}
	return a, err
}

func (s *Server) accountByName(name string) (accountState, error) {
	var a accountState
	err := s.d.DB.QueryRow(`SELECT accountid, username, COALESCE(acctlevel, 0), COALESCE(acctstatus, 0),
		COALESCE(CAST(pwhash AS TEXT), '') FROM accountinfo WHERE LOWER(username) = LOWER(?)`, name).
		Scan(&a.ID, &a.Username, &a.Level, &a.Status, &a.PwHash)
	if errors.Is(err, sql.ErrNoRows) {
		return a, errNoAccount
	}
	return a, err
}

// isGM is whether an account is a GM now: its level, and not banned. (A tester
// and a storyteller use the panel too: panelRole, roles.go, is who may sign
// in.)
func (s *Server) isGM(a accountState) bool {
	return a.Level >= s.cfg.MinLevel && a.Status >= 0
}

// authError is a refusal with its HTTP status.
type authError struct {
	status int
	msg    string
}

func (e *authError) Error() string { return e.msg }

// authenticate says who a request is from: a bearer token (an AI agent's, or
// any client's) or the page's cookie, whose writes must carry the session's
// CSRF token. The account's level and status are read again here, on every
// request.
func (s *Server) authenticate(r *http.Request) (*actor, error) {
	ip := clientIP(r)
	if h := r.Header.Get("Authorization"); h != "" {
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return nil, &authError{http.StatusUnauthorized, "Authorization must be a Bearer token"}
		}
		// a guesser of tokens is stopped before it costs a database read
		key := auth.LimitKey(ip)
		if s.badTokens.empty(key) {
			return nil, &authError{http.StatusTooManyRequests, "too many unknown tokens from this address; wait a minute"}
		}
		a, err := s.tokenActor(strings.TrimSpace(tok), ip)
		var ae *authError
		if err != nil && errors.As(err, &ae) && ae.status == http.StatusUnauthorized {
			s.badTokens.allow(key)
		}
		return a, err
	}

	c, err := r.Cookie(s.cookieName())
	if err != nil {
		return nil, &authError{http.StatusUnauthorized, "not signed in"}
	}
	sess := s.sessions.get(c.Value)
	if sess == nil {
		return nil, &authError{http.StatusUnauthorized, "not signed in"}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		got := r.Header.Get(csrfHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRF)) != 1 {
			return nil, &authError{http.StatusForbidden, "missing or stale CSRF token; reload the page"}
		}
		if o := r.Header.Get("Origin"); o != "" && !s.sameOrigin(r, o) {
			return nil, &authError{http.StatusForbidden, "cross-origin request refused"}
		}
	}
	acc, err := s.accountByID(sess.AccountID)
	role := ""
	if err == nil {
		role = s.panelRole(acc)
	}
	if role == "" {
		s.sessions.end(sess.Token)
		return nil, &authError{http.StatusUnauthorized, "this account no longer has the panel"}
	}
	if !s.sessRate.allow(fmt.Sprint(acc.ID)) {
		return nil, &authError{http.StatusTooManyRequests, "too many requests from this account; slow down"}
	}
	on, err := s.totpEnabled(acc.ID)
	if err != nil {
		return nil, err
	}
	s.sessions.mu.Lock()
	mfa := sess.MFA
	s.sessions.mu.Unlock()
	a := &actor{
		AccountID: acc.ID, Username: acc.Username, Level: acc.Level, Role: role,
		Admin: acc.Level >= s.cfg.AdminLevel && mfa && on, Scope: scopeWrite, Via: "web", IP: ip, sess: sess,
	}
	if a.Admin {
		a.Scope = scopeAdmin
	}
	if s.cfg.RequireTOTP && !on {
		a.Scope, a.mustEnrol = scopeRead, true // and the page asks for enrolment first
	}
	return a, nil
}

// sameOrigin is whether an Origin header names the panel's own origin.
func (s *Server) sameOrigin(r *http.Request, origin string) bool {
	b, err := url.Parse(s.publicBase(r))
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimRight(origin, "/"), b.Scheme+"://"+b.Host)
}

// cookieName is the session cookie's: on its own https host the panel's is a
// __Host- cookie, which a page of any other host (a parent domain's
// included) cannot set or overwrite.
func (s *Server) cookieName() string {
	if s.secureCookies() && s.cookiePath() == "/" {
		return "__Host-eqoa_gm"
	}
	return "eqoa_gm"
}

// setSessionCookie hands the browser its session.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName(), Value: token, Path: s.cookiePath(), MaxAge: maxAge,
		HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteStrictMode,
	})
}

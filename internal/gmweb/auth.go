package gmweb

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/auth"
)

// A person's sign-in through the page: the game account's own password, and
// the authenticator's code once one is enrolled.

// decodeBody reads a JSON body into v, refusing anything after it. The body
// must be sent as JSON: a page of another site can post a form or plain text
// without asking, but not application/json.
func decodeBody(r *http.Request, v any) error {
	if ct := strings.ToLower(r.Header.Get("Content-Type")); !strings.Contains(ct, "json") {
		return errors.New("send the body as JSON (Content-Type: application/json)")
	}
	return decodeJSON(r, v)
}

// decodeJSON is decodeBody whatever the Content-Type, for the endpoints
// that take only a bearer token (MCP), which no other site's page can send.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("a JSON body is required")
		}
		return fmt.Errorf("the body is not valid JSON: %v", err)
	}
	return nil
}

// writeAuthError answers an authentication failure with its status.
func writeAuthError(w http.ResponseWriter, err error) {
	var ae *authError
	if errors.As(err, &ae) {
		writeError(w, ae.status, ae.msg)
		return
	}
	writeError(w, http.StatusUnauthorized, err.Error())
}

// checkCredentials is a sign-in's decision, shared by the page, a terminal's
// token sign-in and OAuth's own page: the account, its password, its being a
// GM and its authenticator. mfa is whether an authenticator's code was given
// (what an admin's tools need); needCode is true when everything but a
// missing code passed. Every attempt is counted against the address and the
// name before the password is checked.
func (s *Server) checkCredentials(ip, username, password, code string) (acc accountState, mfa, needCode bool, err error) {
	username = strings.TrimSpace(username)
	counted := username
	if len(username) > auth.MaxNameLen || len(password) > 256 {
		counted = "" // counted against the address alone: a name this long is no account's
	}
	if wait := s.limiter.Begin(ip, counted); wait > 0 {
		return acc, false, false, &authError{http.StatusTooManyRequests,
			fmt.Sprintf("too many failed sign-ins; try again in %d minutes", auth.RetryMinutes(wait))}
	}
	refuse := func(why string) (accountState, bool, bool, error) {
		log.Printf("GM panel: sign-in refused for %q from %s: %s", clip(username, auth.MaxNameLen), ip, why)
		return accountState{}, false, false, &authError{http.StatusUnauthorized, "wrong account name or password, or not a GM account"}
	}
	if counted == "" {
		return refuse("empty or overlong name or password")
	}
	if password == "" {
		return refuse("empty password")
	}
	// at most a few password checks at once, whoever asks (the game's
	// logins included)
	release, ok := s.limiter.Slot(10 * time.Second)
	if !ok {
		return acc, false, false, &authError{http.StatusServiceUnavailable, "the server is busy; try again in a moment"}
	}
	defer release()
	acc, err = s.accountByName(username)
	if err != nil {
		if errors.Is(err, errNoAccount) {
			// spend a bcrypt's time anyway, so a missing name is not quicker
			auth.VerifyPassword(s.dummyHash(), password)
			return refuse("no such account")
		}
		return acc, false, false, err
	}
	switch auth.VerifyPassword(acc.PwHash, password) {
	case auth.Accepted, auth.AcceptedNeedsRehash:
	default:
		return refuse("wrong password")
	}
	if !s.isGM(acc) {
		return refuse(fmt.Sprintf("acctlevel %d, status %d", acc.Level, acc.Status))
	}
	on, err := s.totpEnabled(acc.ID)
	if err != nil {
		return acc, false, false, err
	}
	if on {
		if strings.TrimSpace(code) == "" {
			return acc, false, true, &authError{http.StatusUnauthorized, "enter the code from your authenticator app"}
		}
		ok, err := s.totpCheck(acc.ID, code)
		if err != nil {
			return acc, false, false, err
		}
		if !ok {
			return refuse("wrong authenticator code")
		}
	}
	s.limiter.Succeed(ip, username)
	return acc, on, false, nil
}

// crossSite refuses a browser request another site's page made: an Origin
// that is not the panel's. A terminal or an agent sends none.
func (s *Server) crossSite(w http.ResponseWriter, r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" && !s.sameOrigin(r, o) {
		writeError(w, http.StatusForbidden, "cross-origin request refused")
		return true
	}
	return false
}

// handleLogin signs a person in: {"username", "password", "code"}.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if s.crossSite(w, r) {
		return
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ip := clientIP(r)
	acc, mfa, needCode, err := s.checkCredentials(ip, req.Username, req.Password, req.Code)
	if err != nil {
		if needCode {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "needCode": true})
			return
		}
		writeAuthError(w, err)
		return
	}
	sess := s.sessions.create(acc.ID, acc.Username, ip, mfa)
	s.setSessionCookie(w, sess.Token, int(sessionMaxAge.Seconds()))
	a := &actor{AccountID: acc.ID, Username: acc.Username, Level: acc.Level, Via: "web", IP: ip}
	s.audit(a, "login", "", nil, nil, nil, "signed in to the panel")
	log.Printf("GM panel: %s (acctlevel %d) signed in from %s", acc.Username, acc.Level, ip)
	writeJSON(w, http.StatusOK, s.sessionInfo(acc.ID, sess, acc.Level >= s.cfg.AdminLevel && mfa))
}

// handleLogout ends the page's session: only the page's own request, which
// carries the session's CSRF token, so another site cannot sign a GM out.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.crossSite(w, r) {
		return
	}
	if c, err := r.Cookie(s.cookieName()); err == nil {
		if sess := s.sessions.get(c.Value); sess != nil {
			got := r.Header.Get(csrfHeader)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRF)) != 1 {
				writeError(w, http.StatusForbidden, "missing or stale CSRF token; reload the page")
				return
			}
			s.sessions.end(sess.Token)
		}
	}
	s.setSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSession is who is signed in (401 when nobody), with the CSRF token
// the page sends with every change, and what the server is.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	a, err := s.authenticate(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.sessionInfo(a.AccountID, a.sess, a.Admin))
}

// sessionInfo is the page's picture of who it is signed in as. admin is the
// credential's: an admin's level with an authenticator's sign-in.
func (s *Server) sessionInfo(accountID int, sess *gmSession, admin bool) map[string]any {
	acc, _ := s.accountByID(accountID)
	totp, _ := s.totpEnabled(accountID)
	out := map[string]any{
		"user": map[string]any{
			"accountid": acc.ID,
			"username":  acc.Username,
			"level":     acc.Level,
			"admin":     admin,
			// an admin's level, waiting for an authenticator's sign-in
			"adminNeedsTOTP": acc.Level >= s.cfg.AdminLevel && !admin,
		},
		"totp": map[string]any{
			"enabled":  totp,
			"required": s.cfg.RequireTOTP,
		},
		"levels": map[string]any{"gm": s.cfg.MinLevel, "admin": s.cfg.AdminLevel},
		"server": map[string]any{
			"build":   s.build,
			"started": stamp(s.started),
			"restart": s.d.Restart != nil,
		},
	}
	// the spell-visual preview the Spell visuals page frames (gm.preview_url)
	if s.preview != nil {
		out["preview"] = s.preview.String()
	}
	if sess != nil {
		out["csrf"] = sess.CSRF
		out["expires"] = stamp(sess.Created.Add(sessionMaxAge))
		out["idle"] = int(s.cfg.SessionIdle / time.Second)
	}
	return out
}

// dummyHash is a bcrypt hash of nothing anyone knows, compared against when
// an account does not exist so that answer takes a real one's time.
func (s *Server) dummyHash() string {
	s.dummyOnce.Do(func() {
		s.dummy, _ = auth.HashPassword(randomHex(16), s.d.Config.Auth.BCryptCost)
	})
	return s.dummy
}

// handleTokenLogin is a terminal's sign-in (an agent such as Codex, or a
// script): {"username", "password", "code", "name", "scope", "days"} for a
// personal API token, the same token the AI agents page makes, under the same
// checks as the page's sign-in (failed attempts limited, the authenticator's
// code when one is enrolled). The answer is the token, once.
func (s *Server) handleTokenLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST {username, password, code, name, scope, days}")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Days     *int   `json:"days"`
	}
	if s.crossSite(w, r) {
		return
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ip := clientIP(r)
	acc, mfa, needCode, err := s.checkCredentials(ip, req.Username, req.Password, req.Code)
	if err != nil {
		if needCode {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "needCode": true})
			return
		}
		writeAuthError(w, err)
		return
	}
	if s.cfg.RequireTOTP {
		if on, _ := s.totpEnabled(acc.ID); !on {
			writeError(w, http.StatusForbidden, "this server requires an authenticator: enrol one in the panel first")
			return
		}
	}
	scope := req.Scope
	switch scope {
	case "":
		scope = scopeWrite
	case scopeRead, scopeWrite:
	case scopeAdmin:
		if acc.Level < s.cfg.AdminLevel {
			writeError(w, http.StatusForbidden, "only an admin may have an admin token")
			return
		}
		if !mfa {
			writeError(w, http.StatusForbidden, "an admin token needs a sign-in with an authenticator code: enrol one in the panel (Security)")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, `scope is "read", "write" or "admin"`)
		return
	}
	days := 30
	if req.Days != nil {
		days = *req.Days
	}
	if days < 1 || days > maxTokenDays {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("days is 1 to %d: every token expires", maxTokenDays))
		return
	}
	name := cleanName(req.Name, 60)
	if name == "" {
		name = "terminal"
	}
	value, id, err := s.issueToken(acc.ID, name, kindPersonal, scope, "", time.Duration(days)*24*time.Hour, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a := &actor{AccountID: acc.ID, Username: acc.Username, Level: acc.Level, Via: "terminal", IP: ip}
	s.audit(a, "token", "", nil, nil, nil, fmt.Sprintf("signed in from a terminal: API token #%d %q (%s, %d days)", id, name, scope, days))
	exp := stamp(time.Now().Add(time.Duration(days) * 24 * time.Hour))
	writeJSON(w, http.StatusOK, map[string]any{"token": value, "id": id, "scope": scope, "expires": exp,
		"mcp": s.publicBase(r) + "/mcp"})
}

// maxTokenDays is the longest an API token lives.
const maxTokenDays = 365

// cleanName is a name a person or a client gave (a token's, an OAuth
// client's) with no control characters, trimmed to n characters: it is shown
// on pages and written to the log.
func cleanName(name string, n int) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if r := []rune(name); len(r) > n {
		name = string(r[:n])
	}
	return name
}

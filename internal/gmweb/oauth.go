package gmweb

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/auth"
)

// OAuth 2.1 for the MCP endpoint, as MCP's authorization asks: protected
// resource metadata (RFC 9728) pointing at this panel as its own
// authorization server (RFC 8414), open client registration (RFC 7591), the
// authorization code flow with PKCE (S256 only) and refresh tokens. The GM
// signs in on the panel's own page (the account's password and authenticator)
// and chooses what the client may do; the client gets a bearer token that acts
// as that GM (tokens.go). An AI client is added by the MCP address alone.

const (
	oauthCodeTTL    = 10 * time.Minute
	oauthAccessTTL  = 8 * time.Hour
	oauthRefreshTTL = 30 * 24 * time.Hour
	oauthPendingTTL = 15 * time.Minute

	// oauthMaxPending bounds the sign-ins in progress (anyone can start one)
	// and the codes not yet traded.
	oauthMaxPending = 1000
	// refreshGrace: a refresh token used again this soon after it was
	// traded is the same client racing itself, not a stolen copy.
	refreshGrace = 30 * time.Second
	// oauthStaleClient: a registered client that never got a token is
	// forgotten after this.
	oauthStaleClient = 3 * 24 * time.Hour
)

type pendingAuth struct {
	ClientID, ClientName, RedirectURI, State, Challenge, Scope, Resource string
	CSRF                                                                 string
	Expires                                                              time.Time
	// Known: the client returns to ChatGPT, Claude or this computer, not to
	// an address anyone could have registered
	Known bool
}

type authCode struct {
	ClientID, ClientName, RedirectURI, Challenge, Scope, Resource string
	AccountID                                                     int
	Expires                                                       time.Time
}

type oauthState struct {
	mu        sync.Mutex
	pending   map[string]*pendingAuth
	codes     map[string]*authCode
	lastPrune time.Time
}

func newOAuthState() *oauthState {
	return &oauthState{pending: map[string]*pendingAuth{}, codes: map[string]*authCode{}}
}

// prune drops the expired sign-ins and codes: when the maps are big, else at
// most once a minute, so a busy page does not walk them on every request.
func (o *oauthState) prune(now time.Time) {
	if len(o.pending)+len(o.codes) < 256 && now.Sub(o.lastPrune) < time.Minute {
		return
	}
	o.lastPrune = now
	for k, p := range o.pending {
		if now.After(p.Expires) {
			delete(o.pending, k)
		}
	}
	for k, c := range o.codes {
		if now.After(c.Expires) {
			delete(o.codes, k)
		}
	}
}

// oauthCORS: the discovery, registration, token and revocation endpoints are
// called by clients from anywhere, browsers included, and carry no cookie.
func oauthCORS(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
	h.Set("Access-Control-Max-Age", "86400")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// handleWellKnown serves the two discovery documents under every path a
// client may build for them (with or without the resource's path after).
func (s *Server) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	if oauthCORS(w, r) {
		return
	}
	base := s.publicBase(r)
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/.well-known/oauth-protected-resource"):
		writeJSON(w, http.StatusOK, map[string]any{
			"resource":                 base + "/mcp",
			"authorization_servers":    []string{base},
			"scopes_supported":         []string{scopeRead, scopeWrite, scopeAdmin, "offline_access"},
			"bearer_methods_supported": []string{"header"},
			"resource_name":            "EQOA GM panel",
			"resource_documentation":   base + "/#/ai",
		})
	case strings.HasPrefix(p, "/.well-known/oauth-authorization-server"),
		strings.HasPrefix(p, "/.well-known/openid-configuration"):
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                         base,
			"authorization_endpoint":                         base + "/oauth/authorize",
			"token_endpoint":                                 base + "/oauth/token",
			"registration_endpoint":                          base + "/oauth/register",
			"revocation_endpoint":                            base + "/oauth/revoke",
			"response_types_supported":                       []string{"code"},
			"response_modes_supported":                       []string{"query"},
			"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":               []string{"S256"},
			"authorization_response_iss_parameter_supported": true,
			"token_endpoint_auth_methods_supported":          []string{"none"},
			"revocation_endpoint_auth_methods_supported":     []string{"none"},
			"scopes_supported":                               []string{scopeRead, scopeWrite, scopeAdmin, "offline_access"},
			"service_documentation":                          base + "/#/ai",
		})
	default:
		http.NotFound(w, r)
	}
}

// redirectAllowed is whether a client may be sent back to uri: https, a
// loopback address over http (a desktop client), or a desktop app's own
// scheme; never javascript:, data: and the like.
func redirectAllowed(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" || u.Fragment != "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Host != ""
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	case "javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss":
		return false
	}
	// a private-use scheme ("cursor://", "vscode://"): a reverse-domain or a
	// single word, as RFC 8252 7.1 has them
	for _, c := range u.Scheme {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '+') {
			return false
		}
	}
	return true
}

// knownReturns are the hosts (and their subdomains) of the AI clients a GM
// connects: their redirect addresses are theirs. Any other https address may
// be anyone's, since registration is open.
var knownReturns = []string{"chatgpt.com", "openai.com", "claude.ai", "claude.com", "anthropic.com"}

// returnLabel is how the consent page names where a client returns to, and
// whether that is ChatGPT, Claude or a program on the GM's own computer (a
// loopback address, or an app's own scheme).
func returnLabel(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil {
		return uri, false
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "a program on this computer (" + u.Host + ")", true
	case "https":
		host := strings.ToLower(u.Hostname())
		for _, k := range knownReturns {
			if host == k || strings.HasSuffix(host, "."+k) {
				return host, true
			}
		}
		return host, false
	}
	return "an app on this computer (" + u.Scheme + ":)", true
}

// redirectMatches is whether an authorization's redirect is one a client
// registered: the same address, or for a loopback one the same address on any
// port, as RFC 8252 7.3 has it (a desktop client such as Codex listens on a
// port it picks at each login).
func redirectMatches(registered, asked string) bool {
	if registered == asked {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(asked)
	if err1 != nil || err2 != nil || a.Scheme != "http" || b.Scheme != "http" {
		return false
	}
	loop := func(h string) bool { return h == "127.0.0.1" || h == "::1" || h == "localhost" }
	return loop(a.Hostname()) && a.Hostname() == b.Hostname() && a.Path == b.Path && a.RawQuery == b.RawQuery
}

// handleOAuthRegister is open client registration (RFC 7591): a public client
// (no secret) with its redirect addresses.
func (s *Server) handleOAuthRegister(w http.ResponseWriter, r *http.Request) {
	if oauthCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	ip := clientIP(r)
	if !s.regRate.allow(auth.LimitKey(ip)) || !s.regAll.allow("all") {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "invalid_client_metadata",
			"error_description": "too many registrations; try again later"})
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		GrantTypes   []string `json:"grant_types"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_client_metadata", "error_description": "a JSON body is required"})
		return
	}
	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > 10 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_redirect_uri", "error_description": "1 to 10 redirect_uris"})
		return
	}
	for _, u := range req.RedirectURIs {
		if len(u) > 500 || !redirectAllowed(u) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_redirect_uri",
				"error_description": "a redirect is https, http to a loopback address, or an app's own scheme: " + u})
			return
		}
	}
	name := cleanName(req.ClientName, 80)
	if name == "" {
		u, _ := url.Parse(req.RedirectURIs[0])
		name = u.Host
		if name == "" {
			name = u.Scheme
		}
	}
	// a client that never got a token is forgotten after a while
	s.d.DB.Exec(`DELETE FROM gm_oauth_clients WHERE last_used IS NULL AND created < ?`, stamp(time.Now().Add(-oauthStaleClient)))
	id := "gmc_" + randomHex(16)
	uris, _ := json.Marshal(req.RedirectURIs)
	if _, err := s.d.DB.Exec(`INSERT INTO gm_oauth_clients (client_id, name, redirect_uris, created) VALUES (?, ?, ?, ?)`,
		id, name, string(uris), now()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
		return
	}
	log.Printf("GM OAuth: registered client %s %q from %s (%v)", id, name, ip, req.RedirectURIs)
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  id,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                name,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// oauthClient is a registered client's name and redirect addresses.
func (s *Server) oauthClient(id string) (string, []string, bool) {
	var name, uris string
	if err := s.d.DB.QueryRow(`SELECT name, redirect_uris FROM gm_oauth_clients WHERE client_id = ?`, id).Scan(&name, &uris); err != nil {
		return "", nil, false
	}
	var list []string
	json.Unmarshal([]byte(uris), &list)
	return name, list, true
}

// scopeOf is the scope a request asks for: the widest of its words that this
// server knows, write when it names none.
func scopeOf(requested string) string {
	scope := ""
	for _, w := range strings.Fields(requested) {
		switch w {
		case scopeAdmin:
			scope = scopeAdmin
		case scopeWrite:
			if scope != scopeAdmin {
				scope = scopeWrite
			}
		case scopeRead:
			if scope == "" {
				scope = scopeRead
			}
		}
	}
	if scope == "" {
		scope = scopeWrite
	}
	return scope
}

var authorizePage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Allow access · EQOA GM</title>
<link rel="icon" href="{{.Base}}/static/icon.svg" type="image/svg+xml">
<link rel="stylesheet" href="{{.Base}}/static/app.css?v={{.Build}}">
</head>
<body class="oauth">
<main class="oauth-card">
  <div class="brand"><span class="brand-mark">GM</span> EQOA GM panel</div>
  {{if .Fatal}}
    <h1>This sign-in cannot go on</h1>
    <p class="err">{{.Fatal}}</p>
  {{else}}
    <h1>Allow <strong>{{.Client}}</strong> to work as a GM?</h1>
    <p class="muted">It will act as the GM who signs in here, through the same tools as the panel. Everything it
    changes is logged under your name and can be reverted.</p>
    <p>After you sign in, you go back to <strong>{{.Return}}</strong>.</p>
    {{if not .Known}}<p class="notice warn">That is not ChatGPT, Claude or a program on this computer, and the name
    above is only what the application calls itself: anyone can register one. Go on only if you set this
    application up yourself and know that address.</p>{{end}}
    {{if .Error}}<p class="err" role="alert">{{.Error}}</p>{{end}}
    <form method="post" action="{{.Base}}/oauth/authorize" class="stack">
      <input type="hidden" name="req" value="{{.Req}}">
      <input type="hidden" name="csrf" value="{{.CSRF}}">
      <label>GM account <input name="username" autocomplete="username" required value="{{.Username}}" autofocus></label>
      <label>Password <input name="password" type="password" autocomplete="current-password" required></label>
      <label>Authenticator code <span class="muted">(if you have one)</span>
        <input name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]*" maxlength="7"></label>
      <fieldset>
        <legend>It may</legend>
        <label class="radio"><input type="radio" name="scope" value="read"{{if eq .Scope "read"}} checked{{end}}> look only</label>
        <label class="radio"><input type="radio" name="scope" value="write"{{if eq .Scope "write"}} checked{{end}}> look and change game data and players</label>
        <label class="radio"><input type="radio" name="scope" value="admin"{{if eq .Scope "admin"}} checked{{end}}> all of that, and an admin's tools (an admin with an authenticator)</label>
      </fieldset>
      {{if not .Known}}<label class="check"><input type="checkbox" name="trust" value="1" required> <span>I set this application up and know <strong>{{.Return}}</strong></span></label>{{end}}
      <div class="row">
        <button type="submit" name="action" value="allow" class="primary">Sign in and allow</button>
        <button type="submit" name="action" value="deny" formnovalidate>Deny</button>
      </div>
    </form>
  {{end}}
</main>
</body>
</html>`))

type authorizeView struct {
	Base, Build, Client, Return, Req, CSRF, Scope, Username, Error, Fatal string
	Known                                                                 bool
}

// renderAuthorize draws the sign-in page. returnTo is the client's redirect
// address: the page's form-action allows it, since the form's answer is a
// redirect there and browsers hold a form's redirects to form-action.
func (s *Server) renderAuthorize(w http.ResponseWriter, r *http.Request, status int, v authorizeView, returnTo string) {
	v.Base = ""
	if s.public != nil {
		v.Base = strings.TrimRight(s.public.Path, "/")
	}
	v.Build = url.QueryEscape(s.build)
	target := ""
	if u, err := url.Parse(returnTo); err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			target = " " + u.Scheme + "://" + u.Host
		default:
			target = " " + u.Scheme + ":"
		}
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'none'; style-src 'self'; img-src 'self' data:; "+
		"frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'"+target)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	authorizePage.Execute(w, v)
}

// oauthRedirect sends the browser back to the client with the answer.
func oauthRedirect(w http.ResponseWriter, r *http.Request, redirectURI string, params url.Values) {
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// handleOAuthAuthorize shows the sign-in for a client (GET) and takes it (POST).
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.authorizeStart(w, r)
	case http.MethodPost:
		s.authorizeSubmit(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) authorizeStart(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRate.allow(auth.LimitKey(clientIP(r))) {
		s.renderAuthorize(w, r, http.StatusTooManyRequests, authorizeView{Fatal: "Too many sign-in pages from this address. Wait a minute and start again from the application."}, "")
		return
	}
	q := r.URL.Query()
	clientID, redirect := q.Get("client_id"), q.Get("redirect_uri")
	if len(clientID) > 64 || len(redirect) > 500 {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The application's sign-in request is not valid."}, "")
		return
	}
	name, uris, ok := s.oauthClient(clientID)
	if !ok {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The application is not registered here (an unknown client_id). Add the connector again."}, "")
		return
	}
	if redirect == "" && len(uris) == 1 {
		redirect = uris[0]
	}
	registered := false
	for _, u := range uris {
		if redirectMatches(u, redirect) {
			registered = true
		}
	}
	if !registered {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The application asked to return to an address it did not register."}, "")
		return
	}
	// a bad request is answered here, never sent on to the client: a
	// registered address is anyone's, so a redirect would make this server
	// an open redirector
	invalid := func(why string) {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The application's sign-in request is not valid: " + why + "."}, "")
	}
	challenge := q.Get("code_challenge")
	switch {
	case q.Get("response_type") != "code":
		invalid("only the code flow is supported")
		return
	case q.Get("code_challenge_method") != "S256" || !pkceChallenge(challenge):
		invalid("PKCE with S256 is required")
		return
	case len(q.Get("state")) > 512, len(q.Get("scope")) > 200:
		invalid("a parameter is too long")
		return
	}
	resource := q.Get("resource")
	if resource != "" && !s.ourResource(r, resource) {
		invalid("the resource is not this server's MCP address")
		return
	}
	// what the page offers first: write, or read when that is all it asked
	// for; admin only when the GM picks it (clients ask for every scope a
	// server lists); read for a client returning to an address anyone may
	// have registered
	offer := scopeOf(q.Get("scope"))
	if offer == scopeAdmin {
		offer = scopeWrite
	}
	label, known := returnLabel(redirect)
	if !known {
		offer = scopeRead
	}
	now := time.Now()
	p := &pendingAuth{
		ClientID: clientID, ClientName: name, RedirectURI: redirect, State: q.Get("state"),
		Challenge: challenge, Scope: offer, Resource: resource,
		CSRF: randomHex(16), Expires: now.Add(oauthPendingTTL), Known: known,
	}
	id := randomHex(16)
	s.oauth.mu.Lock()
	s.oauth.prune(now)
	full := len(s.oauth.pending) >= oauthMaxPending
	if !full {
		s.oauth.pending[id] = p
	}
	s.oauth.mu.Unlock()
	if full {
		s.renderAuthorize(w, r, http.StatusServiceUnavailable, authorizeView{Fatal: "Too many sign-ins are in progress. Try again in a few minutes."}, "")
		return
	}
	s.renderAuthorize(w, r, http.StatusOK, authorizeView{Client: name, Return: label, Known: known, Req: id, CSRF: p.CSRF, Scope: p.Scope}, redirect)
}

// pkceChallenge is whether c is an S256 code challenge: the base64url
// SHA-256 of a verifier, 43 characters (RFC 7636 allows up to 128).
func pkceChallenge(c string) bool {
	if len(c) < 43 || len(c) > 128 {
		return false
	}
	for _, r := range c {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == '~') {
			return false
		}
	}
	return true
}

// ourResource is whether an authorization's resource (RFC 8707) names this
// panel's MCP endpoint (or the panel).
func (s *Server) ourResource(r *http.Request, resource string) bool {
	base := strings.ToLower(s.publicBase(r))
	res := strings.ToLower(strings.TrimRight(resource, "/"))
	return res == base || res == base+"/mcp"
}

func (s *Server) authorizeSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The form could not be read."}, "")
		return
	}
	id := r.PostForm.Get("req")
	s.oauth.mu.Lock()
	p := s.oauth.pending[id]
	s.oauth.mu.Unlock()
	if p == nil || time.Now().After(p.Expires) {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "This sign-in has expired. Start it again from the application."}, "")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(p.CSRF)) != 1 {
		s.renderAuthorize(w, r, http.StatusBadRequest, authorizeView{Fatal: "The form was not this page's. Start again from the application."}, "")
		return
	}
	label, _ := returnLabel(p.RedirectURI)
	view := authorizeView{Client: p.ClientName, Return: label, Known: p.Known, Req: id, CSRF: p.CSRF,
		Scope: p.Scope, Username: r.PostForm.Get("username")}
	if r.PostForm.Get("action") == "deny" {
		s.oauth.mu.Lock()
		delete(s.oauth.pending, id)
		s.oauth.mu.Unlock()
		if !p.Known {
			s.renderAuthorize(w, r, http.StatusOK, authorizeView{Fatal: "Not allowed. You can close this page."}, "")
			return
		}
		oauthRedirect(w, r, p.RedirectURI, url.Values{"error": {"access_denied"}, "state": {p.State}})
		return
	}
	if sc := r.PostForm.Get("scope"); sc == scopeRead || sc == scopeWrite || sc == scopeAdmin {
		view.Scope = sc
	}
	if !p.Known && r.PostForm.Get("trust") != "1" {
		view.Error = "Tick the box to say you set this application up yourself, or deny it."
		s.renderAuthorize(w, r, http.StatusBadRequest, view, p.RedirectURI)
		return
	}
	acc, mfa, needCode, err := s.checkCredentials(clientIP(r), r.PostForm.Get("username"), r.PostForm.Get("password"), r.PostForm.Get("code"))
	if err != nil {
		view.Error = err.Error()
		if needCode {
			view.Error = "Enter the code from your authenticator app."
		}
		s.renderAuthorize(w, r, http.StatusUnauthorized, view, p.RedirectURI)
		return
	}
	if s.cfg.RequireTOTP {
		if on, _ := s.totpEnabled(acc.ID); !on {
			view.Error = "This server requires an authenticator: enrol one in the GM panel first."
			s.renderAuthorize(w, r, http.StatusForbidden, view, p.RedirectURI)
			return
		}
	}
	scope := view.Scope
	if scope == scopeAdmin {
		switch {
		case acc.Level < s.cfg.AdminLevel:
			view.Error = "You are not an admin: choose one of the other two."
		case !mfa:
			view.Error = "An admin's tools need a sign-in with an authenticator code: enrol one in the GM panel (Security), or choose one of the other two."
		}
		if view.Error != "" {
			s.renderAuthorize(w, r, http.StatusForbidden, view, p.RedirectURI)
			return
		}
	}
	code := randomHex(24)
	s.oauth.mu.Lock()
	delete(s.oauth.pending, id)
	s.oauth.prune(time.Now())
	full := len(s.oauth.codes) >= oauthMaxPending
	if !full {
		s.oauth.codes[code] = &authCode{ClientID: p.ClientID, ClientName: p.ClientName, RedirectURI: p.RedirectURI,
			Challenge: p.Challenge, Scope: scope, Resource: p.Resource, AccountID: acc.ID, Expires: time.Now().Add(oauthCodeTTL)}
	}
	s.oauth.mu.Unlock()
	if full {
		s.renderAuthorize(w, r, http.StatusServiceUnavailable, authorizeView{Fatal: "Too many sign-ins are in progress. Try again in a few minutes."}, "")
		return
	}
	a := &actor{AccountID: acc.ID, Username: acc.Username, Level: acc.Level, Via: "oauth:" + p.ClientName, IP: clientIP(r)}
	s.audit(a, "token", "", nil, nil, nil, fmt.Sprintf("allowed %q (%s) to work as this GM (%s)", p.ClientName, p.ClientID, scope))
	oauthRedirect(w, r, p.RedirectURI, url.Values{"code": {code}, "state": {p.State}, "iss": {s.publicBase(r)}})
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]any{"error": code, "error_description": desc})
}

// handleOAuthToken trades a code (with its PKCE verifier) or a refresh token
// for an access token and a new refresh token.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if oauthCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		oauthError(w, http.StatusMethodNotAllowed, "invalid_request", "POST only")
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "a form body is required")
		return
	}
	f := r.PostForm
	clientID := f.Get("client_id")
	if clientID == "" {
		if u, _, ok := r.BasicAuth(); ok {
			clientID = u
		}
	}
	switch f.Get("grant_type") {
	case "authorization_code":
		code := f.Get("code")
		s.oauth.mu.Lock()
		c := s.oauth.codes[code]
		delete(s.oauth.codes, code) // a code is good once, right or wrong
		s.oauth.mu.Unlock()
		if c == nil || time.Now().After(c.Expires) {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is unknown, used or expired")
			return
		}
		if c.ClientID != clientID {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is another client's")
			return
		}
		if ru := f.Get("redirect_uri"); ru != "" && ru != c.RedirectURI {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri differs from the authorization's")
			return
		}
		sum := sha256.Sum256([]byte(f.Get("code_verifier")))
		if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(c.Challenge)) != 1 {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the code_verifier does not match the challenge")
			return
		}
		s.issueOAuthPair(w, r, c.AccountID, c.ClientID, c.ClientName, c.Scope)
	case "refresh_token":
		t, err := s.refreshTokenOf(f.Get("refresh_token"))
		if err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
			return
		}
		if t.Client != clientID {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is another client's")
			return
		}
		// rotate: the old pair goes, a new pair comes. A refresh token is
		// good once: used again, a copy is in someone else's hands, and
		// every token of that grant goes. Used twice within moments is a
		// client racing itself (its other request got the new pair): that
		// one is refused, and nothing else.
		res, err := s.d.DB.Exec(`UPDATE gm_tokens SET revoked = 1, last_used = ? WHERE id = ? AND revoked = 0`, now(), t.ID)
		if err != nil {
			oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n != 1 && t.LastUsed != "" && time.Since(parseStamp(t.LastUsed)) < refreshGrace {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was just used")
			return
		}
		if n != 1 || t.Revoked {
			res, _ := s.d.DB.Exec(`UPDATE gm_tokens SET revoked = 1 WHERE accountid = ? AND client = ? AND revoked = 0`, t.AccountID, t.Client)
			n := int64(0)
			if res != nil {
				n, _ = res.RowsAffected()
			}
			a := &actor{AccountID: t.AccountID, Username: "", Via: "oauth:" + t.Name, IP: clientIP(r)}
			if acc, err := s.accountByID(t.AccountID); err == nil {
				a.Username, a.Level = acc.Username, acc.Level
			}
			s.audit(a, "security", "", nil, nil, nil, fmt.Sprintf("a refresh token of %q was used twice: its %d live token(s) revoked", t.Name, n))
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was used already; sign in again")
			return
		}
		s.revokeToken(t.ID) // and its access token
		scope := t.Scope
		if want := f.Get("scope"); want != "" && scopeRank(scopeOf(want)) < scopeRank(scope) {
			scope = scopeOf(want)
		}
		s.issueOAuthPair(w, r, t.AccountID, t.Client, t.Name, scope)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "authorization_code or refresh_token")
	}
}

func scopeRank(s string) int {
	switch s {
	case scopeRead:
		return 1
	case scopeWrite:
		return 2
	case scopeAdmin:
		return 3
	}
	return 0
}

func (s *Server) issueOAuthPair(w http.ResponseWriter, r *http.Request, accountID int, clientID, clientName, scope string) {
	acc, err := s.accountByID(accountID)
	if err != nil || !s.isGM(acc) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the account is no longer a GM")
		return
	}
	if scope == scopeAdmin && acc.Level < s.cfg.AdminLevel {
		scope = scopeWrite
	}
	access, aid, err := s.issueToken(accountID, clientName, kindOAuth, scope, clientID, oauthAccessTTL, 0)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	refresh, _, err := s.issueToken(accountID, clientName, kindRefresh, scope, clientID, oauthRefreshTTL, aid)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	s.d.DB.Exec(`UPDATE gm_oauth_clients SET last_used = ? WHERE client_id = ?`, now(), clientID)
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(oauthAccessTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         scope,
	})
}

// handleOAuthRevoke is RFC 7009: the token and its pair go; the answer is the
// same whether it was known or not.
func (s *Server) handleOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if oauthCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	if t, err := s.lookupToken(r.PostForm.Get("token"), kindOAuth, kindRefresh, kindPersonal); err == nil {
		s.revokeToken(t.ID)
	}
	w.WriteHeader(http.StatusOK)
}

// refreshTokenOf finds a refresh token by its value, revoked or not (a
// revoked one used again is a stolen copy), but not expired.
func (s *Server) refreshTokenOf(value string) (tokenInfo, error) {
	var t tokenInfo
	var expires, lastUsed sql.NullString
	var revoked int
	err := s.d.DB.QueryRow(`SELECT id, accountid, name, scope, client, expires, last_used, revoked FROM gm_tokens WHERE hash = ? AND kind = ?`,
		hashToken(value), kindRefresh).Scan(&t.ID, &t.AccountID, &t.Name, &t.Scope, &t.Client, &expires, &lastUsed, &revoked)
	if err != nil {
		return t, errors.New("unknown refresh token")
	}
	t.Expires, t.LastUsed, t.Revoked = expires.String, lastUsed.String, revoked != 0
	if t.Expires != "" && time.Now().After(parseStamp(t.Expires)) {
		return t, errors.New("the refresh token expired; sign in again")
	}
	return t, nil
}

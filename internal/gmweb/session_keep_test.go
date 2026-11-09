package gmweb

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// A browser session outlives a server restart (the user, 2026-11-10: "the gm panel
// is closing the session too soon. testers are trying to update and it boots them":
// every keep restart had emptied the sessions' map): gm_sessions keeps it by its
// token's hash, never the token, and a second panel over the same database takes the
// same cookie and CSRF token. Signing out there ends it for good. The controls: a
// session idle past gm.session_idle is not taken back (and its row goes), nor one of
// an account whose sessions were ended.
func TestASessionOutlivesARestart(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	u, _ := url.Parse(f.ts.URL)
	var token string
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == f.s.cookieName() {
			token = ck.Value
		}
	}
	if token == "" || c.csrf == "" {
		t.Fatalf("no session cookie (%q) or CSRF token (%q)", token, c.csrf)
	}
	count := func(hash string) int {
		var n int
		f.db.QueryRow(`SELECT count(*) FROM gm_sessions WHERE hash = ?`, hash).Scan(&n)
		return n
	}
	if count(token) != 0 || count(hashToken(token)) != 1 {
		t.Fatalf("gm_sessions rows: %d by the token, %d by its hash; want 0 and 1", count(token), count(hashToken(token)))
	}

	// the restart: a new panel over the same database, the browser's cookie on it
	restarted := func() *client {
		t.Helper()
		s2, err := New(f.s.d)
		if err != nil {
			t.Fatal(err)
		}
		ts2 := httptest.NewServer(s2.Handler())
		t.Cleanup(ts2.Close)
		f2 := *f
		f2.s, f2.ts = s2, ts2
		c2 := f2.browser()
		u2, _ := url.Parse(ts2.URL)
		c2.http.Jar.SetCookies(u2, []*http.Cookie{{Name: s2.cookieName(), Value: token, Path: "/"}})
		c2.csrf = c.csrf
		return c2
	}
	c2 := restarted()
	code, out := c2.do("GET", "/api/auth/session", nil)
	user, _ := out["user"].(map[string]any)
	if code != 200 || user == nil || user["username"] != "gm" || out["csrf"] != c.csrf {
		t.Fatalf("after the restart: %d %v, want gm's session with its CSRF token", code, out)
	}
	if code, out := c2.do("POST", "/api/auth/logout", nil); code != 200 {
		t.Fatalf("sign out after the restart: %d %v", code, out)
	}
	if count(hashToken(token)) != 0 {
		t.Errorf("the signed-out session's row is still there")
	}
	if code, _ := restarted().do("GET", "/api/auth/session", nil); code != 401 {
		t.Errorf("a signed-out session after another restart: %d, want 401", code)
	}

	// idle past its time: not taken back, its row gone
	c.login("gm")
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == f.s.cookieName() {
			token = ck.Value
		}
	}
	mustExec(t, f.db, `UPDATE gm_sessions SET seen = ? WHERE hash = ?`, stamp(time.Now().Add(-f.s.cfg.SessionIdle-time.Minute)), hashToken(token))
	if code, _ := restarted().do("GET", "/api/auth/session", nil); code != 401 {
		t.Errorf("an idle session after a restart: %d, want 401", code)
	}
	if count(hashToken(token)) != 0 {
		t.Errorf("the idle session's row is still there")
	}

	// an account's sessions ended while no panel had them in memory
	c.login("gm")
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == f.s.cookieName() {
			token = ck.Value
		}
	}
	var id int
	f.db.QueryRow(`SELECT accountid FROM accountinfo WHERE username = 'gm'`).Scan(&id)
	s3, err := New(f.s.d)
	if err != nil {
		t.Fatal(err)
	}
	if n := s3.sessions.endAccount(id, ""); n < 1 {
		t.Errorf("endAccount on a fresh panel ended %d, want the kept one", n)
	}
	if code, _ := restarted().do("GET", "/api/auth/session", nil); code != 401 {
		t.Errorf("a session of an ended account after a restart: %d, want 401", code)
	}
}

package gmweb

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// The spell-visual preview (gm.preview_url): with it the panel may frame that
// one origin, and the session names it for the Spell visuals page; without it
// nothing may be framed and the session says nothing; a preview_url that is no
// http(s) address is refused at the start.
func TestSpellVisualPreviewFraming(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	csp := func(s *Server) string {
		rec := httptest.NewRecorder()
		s.secure(ok).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		return rec.Header().Get("Content-Security-Policy")
	}
	u, _ := url.Parse("https://eqoaclassic.com/eqoa/play/fxview.html")
	with := &Server{preview: u}
	if got := csp(with); !strings.Contains(got, "frame-src https://eqoaclassic.com;") || !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("with a preview: %s", got)
	}
	if got := csp(&Server{}); !strings.Contains(got, "frame-src 'none';") || strings.Contains(got, "eqoaclassic.com") {
		t.Errorf("without one: %s", got)
	}

	// the session names it (the fixture's server has none)
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if _, out := c.do("GET", "/api/auth/session", nil); out["preview"] != nil {
		t.Errorf("a panel with no preview names one: %v", out["preview"])
	}
	f.s.preview = u
	if _, out := c.do("GET", "/api/auth/session", nil); out["preview"] != u.String() {
		t.Errorf("the session's preview: %v", out["preview"])
	}

	// a bad address is refused at the start (the control: a good one starts)
	db, err := database.NewSQLite(t.TempDir() + "/eqoa.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, bad := range []string{"javascript:alert(1)", "eqoaclassic.com/fxview.html", "https://user:pw@eqoaclassic.com/x"} {
		cfg := config.DefaultConfig()
		cfg.GM.PreviewURL = bad
		if _, err := New(Deps{Config: cfg, DB: db}); err == nil || !strings.Contains(err.Error(), "preview_url") {
			t.Errorf("preview_url %q: %v", bad, err)
		}
	}
	cfg := config.DefaultConfig()
	cfg.GM.PreviewURL = u.String()
	if s, err := New(Deps{Config: cfg, DB: db}); err != nil || s.preview == nil {
		t.Errorf("the control must start: %v", err)
	}
}

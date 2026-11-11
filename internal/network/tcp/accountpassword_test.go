package tcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// The account pages' Forgot password and Change password forms
// (accountpassword.go), and what a temporary password a GM set lets in.

type pwRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	s     *AccountServer
	h     http.Handler
	told  []string
	clock time.Time
}

func newPwRig(t *testing.T) *pwRig {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "pw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &pwRig{t: t, db: db, clock: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
	r.s = NewAccountServer(db, 0, 4)
	r.s.Limiter = auth.NewLimiter()
	r.s.Now = func() time.Time { return r.clock }
	r.s.PwResetTell = func(p database.PwReset, account string) { r.told = append(r.told, fmt.Sprintf("%d %s", p.ID, account)) }
	r.h = r.s.Handler()
	for _, name := range []string{"alice", "bob", "carol", "dave", "erin"} {
		hash, _ := auth.HashPassword("pw-"+name, 4)
		if _, err := database.NewSQLiteAccountStore(db).Create(name, hash); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// post sends JSON from addr with the page's header (header false: without).
func (r *pwRig) post(path, addr string, header bool, body any, cookies ...*http.Cookie) (int, map[string]any, *http.Response) {
	r.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.RemoteAddr = addr + ":5555"
	req.Header.Set("Content-Type", "application/json")
	if header {
		req.Header.Set("X-EQOA-Account", "1")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Result()
}

func (r *pwRig) count(q string, args ...any) int {
	r.t.Helper()
	var n int
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *pwRig) forgot(addr, account string) (int, map[string]any) {
	code, out, _ := r.post("/api/password/forgot", addr, true, map[string]any{"account": account, "contact": "Alice#1234 on Discord"})
	return code, out
}

func TestTheForgotFormFilesARequestAndSaysNothingOfTheAccount(t *testing.T) {
	r := newPwRig(t)
	// without the page's header: refused, nothing kept (another site's form)
	if code, _, _ := r.post("/api/password/forgot", "203.0.113.1", false, map[string]any{"account": "alice", "contact": "x#1"}); code != 400 {
		t.Fatalf("no header: %d", code)
	}
	// a contact is needed
	if code, out, _ := r.post("/api/password/forgot", "203.0.113.1", true, map[string]any{"account": "alice"}); code != 400 || out["ok"] != false {
		t.Fatalf("no contact: %d %v", code, out)
	}
	codeA, real := r.forgot("203.0.113.1", "ALICE") // the name in any case
	codeB, none := r.forgot("203.0.113.2", "nobody")
	if codeA != 200 || codeB != 200 || real["message"] != none["message"] || real["ok"] != true || !strings.Contains(real["message"].(string), "If the account exists") {
		t.Fatalf("the answers differ or fail: %d %v / %d %v", codeA, real, codeB, none)
	}
	if n := r.count(`SELECT COUNT(*) FROM pwresets`); n != 1 {
		t.Fatalf("%d requests kept, want 1 (none for no account)", n)
	}
	p, ok, err := r.db.PwResetByID(1)
	if err != nil || !ok || p.AccountID != 1 || p.Typed != "ALICE" || p.Contact != "Alice#1234 on Discord" || p.Status != "waiting" ||
		p.AddrHash == "" || strings.Contains(p.AddrHash, "203.0.113") || p.Board == 0 {
		t.Fatalf("the request: %+v %v %v", p, ok, err)
	}
	e, ok, err := r.db.WorkEntryByID(p.Board)
	if err != nil || !ok || !strings.Contains(e.Title, "Password reset 1: account alice") || e.Status != database.WorkPlanned ||
		!strings.Contains(e.Details, "Alice#1234") {
		t.Fatalf("the board entry: %+v %v %v", e, ok, err)
	}
	if len(r.told) != 1 || r.told[0] != "1 alice" {
		t.Fatalf("the GMs were told %v", r.told)
	}
	// a second for the account while one waits: the same answer, nothing kept
	if code, out := r.forgot("203.0.113.3", "alice"); code != 200 || out["message"] != real["message"] {
		t.Fatalf("a second: %d %v", code, out)
	}
	// the honeypot: the same answer, nothing kept
	code, out, _ := r.post("/api/password/forgot", "203.0.113.4", true, map[string]any{"account": "bob", "contact": "bob#1", "website": "http://spam"})
	if code != 200 || out["message"] != real["message"] {
		t.Fatalf("the honeypot: %d %v", code, out)
	}
	if n := r.count(`SELECT COUNT(*) FROM pwresets`); n != 1 {
		t.Fatalf("%d requests kept after a second and a honeypot, want 1", n)
	}
}

func TestTheForgotFormIsLimitedByAddress(t *testing.T) {
	r := newPwRig(t)
	addr := "198.51.100.7"
	// three kept a day from one address
	for _, name := range []string{"alice", "bob", "carol"} {
		if code, out := r.forgot(addr, name); code != 200 {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}
	if code, out := r.forgot(addr, "dave"); code != 429 {
		t.Fatalf("a fourth kept from one address: %d %v", code, out)
	}
	if n := r.count(`SELECT COUNT(*) FROM pwresets`); n != 3 {
		t.Fatalf("%d kept", n)
	}
	// and five forms an hour, kept or not (names that are no account)
	other := "198.51.100.8"
	for i := 0; i < forgotPerHour; i++ {
		if code, _ := r.forgot(other, fmt.Sprintf("nobody%d", i)); code != 200 {
			t.Fatalf("form %d: %d", i, code)
		}
	}
	if code, _ := r.forgot(other, "erin"); code != 429 {
		t.Fatalf("past the hour's forms: %d", code)
	}
	r.clock = r.clock.Add(61 * time.Minute)
	if code, _ := r.forgot(other, "erin"); code != 200 {
		t.Fatalf("an hour on: %d", code)
	}
	// a day on, the first address may send again
	r.clock = r.clock.Add(24 * time.Hour)
	if code, _ := r.forgot(addr, "dave"); code != 200 {
		t.Fatalf("a day on: %d", code)
	}
}

// setTemp gives an account a temporary password as an approval does.
func (r *pwRig) setTemp(name, password string) int {
	r.t.Helper()
	acc, err := database.NewSQLiteAccountStore(r.db).GetByUsername(name)
	if err != nil {
		r.t.Fatal(err)
	}
	hash, _ := auth.HashPassword(password, 4)
	if _, err := r.db.Exec(`UPDATE accountinfo SET pwhash = ?, pwtemp = 1 WHERE accountid = ?`, hash, acc.ID); err != nil {
		r.t.Fatal(err)
	}
	return acc.ID
}

func TestATemporaryPasswordMustBeChangedOnThePages(t *testing.T) {
	r := newPwRig(t)
	id := r.setTemp("alice", "otter-maple-lantern-42")
	// the login page's login: no session, mustChange
	code, out, resp := r.post("/api/login", "203.0.113.9", false, map[string]any{"username": "alice", "password": "otter-maple-lantern-42"})
	if code != 200 || out["success"] != false || out["mustChange"] != true || len(resp.Cookies()) != 0 {
		t.Fatalf("login with a temporary password: %d %v cookies %v", code, out, resp.Cookies())
	}
	// the transfer page's sign-in says so too
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "203.0.113.9:1"
	if aid, _, refusal := r.s.CheckLogin(req, "alice", "otter-maple-lantern-42"); aid != 0 || refusal != TempPasswordRefusal {
		t.Fatalf("CheckLogin: %d %q", aid, refusal)
	}
	// the change form: a wrong current password changes nothing
	if _, out, _ := r.post("/api/password/change", "203.0.113.9", true, map[string]any{"account": "alice", "current": "wrong", "password": "my-own-one"}); out["ok"] != false {
		t.Fatalf("a wrong current password: %v", out)
	}
	// without the header: refused
	if code, _, _ := r.post("/api/password/change", "203.0.113.9", false, map[string]any{"account": "alice", "current": "otter-maple-lantern-42", "password": "my-own-one"}); code != 400 {
		t.Fatalf("no header: %d", code)
	}
	if temp, _ := r.db.PasswordTemporary(id); !temp {
		t.Fatal("the flag went without a change")
	}
	if _, out, _ := r.post("/api/password/change", "203.0.113.9", true, map[string]any{"account": "alice", "current": "otter-maple-lantern-42", "password": "my-own-one"}); out["ok"] != true {
		t.Fatalf("the change: %v", out)
	}
	if temp, _ := r.db.PasswordTemporary(id); temp {
		t.Fatal("the flag stayed after the change")
	}
	code, out, resp = r.post("/api/login", "203.0.113.9", false, map[string]any{"username": "alice", "password": "my-own-one"})
	if out["success"] != true || len(resp.Cookies()) == 0 {
		t.Fatalf("login with the new password: %d %v", code, out)
	}
	session := resp.Cookies()[0]
	// a GM's reset afterwards drops the session made before it
	r.setTemp("alice", "another-temp-11")
	req = httptest.NewRequest(http.MethodGet, "/characters", nil)
	req.AddCookie(session)
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("the characters page with a session from before a reset: %d %s", w.Code, w.Header().Get("Location"))
	}
	// the pages
	for _, path := range []string{"/password", "/login"} {
		w := httptest.NewRecorder()
		r.h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Forgot password?") {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

// The game's login takes a temporary password: no game client can change a
// password on this server, so the world says it instead.
func TestTheGameLoginTakesATemporaryPassword(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.DefaultConfig()
	cfg.Auth.BCryptCost = 4
	cfg.Auth.AllowAutoRegister = false
	store := database.NewSQLiteAccountStore(db)
	hash, _ := auth.HashPassword("otter-maple-lantern-42", 4)
	acc, err := store.Create("alice", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accountinfo SET pwtemp = 1 WHERE accountid = ?`, acc.ID); err != nil {
		t.Fatal(err)
	}
	s := NewAuthServer(cfg, store, auth.NewTicketStore(0))
	if !login(t, s, "alice", "otter-maple-lantern-42") {
		t.Fatal("the game's login refused a temporary password")
	}
	if login(t, s, "alice", "the-old-one") {
		t.Fatal("a wrong password got in")
	}
}

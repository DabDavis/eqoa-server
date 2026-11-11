package gmweb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
)

// Password resets on the panel (ops_pwreset.go): a request from the Forgot
// password form waits; a GM's approval sets a temporary password, answered
// once and kept nowhere but as its hash, and flags the account; a decline sets
// nothing; a GM starts one without the form; each is audited and refused below
// a GM and on a shard whose accounts are a copy.

func filePwReset(t *testing.T, f *fixture, accountID int, typed string) database.PwReset {
	t.Helper()
	title, status := "Password reset: "+typed, database.WorkPlanned
	r, err := f.db.FilePwReset(database.PwReset{AccountID: accountID, Typed: typed, Contact: "hero#0001 on Discord",
		Note: "<b>please</b>", AddrHash: "abc"}, time.Now(),
		func(int64) database.WorkPost { return database.WorkPost{Title: &title, Status: &status} },
		database.GMEdit{AccountID: accountID, Via: "pwreset"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pwhashOf(t *testing.T, f *fixture, id int) (string, int) {
	t.Helper()
	var h string
	var temp int
	if err := f.db.QueryRow(`SELECT CAST(pwhash AS TEXT), pwtemp FROM accountinfo WHERE accountid = ?`, id).Scan(&h, &temp); err != nil {
		t.Fatal(err)
	}
	return h, temp
}

func TestAPasswordResetIsApprovedOnce(t *testing.T) {
	f := newFixture(t)
	r := filePwReset(t, f, 3, "Player")
	gm := f.browser()
	gm.login("gm")
	// the list shows it with the account's characters
	code, out := gm.op("pwreset_list", map[string]any{})
	if code != 200 {
		t.Fatalf("list: %d %v", code, out)
	}
	reqs := out["requests"].([]any)
	if len(reqs) != 1 {
		t.Fatalf("listed %d", len(reqs))
	}
	acc := reqs[0].(map[string]any)["account"].(map[string]any)
	chars := acc["characters"].([]any)
	if acc["username"] != "player" || len(chars) != 1 || chars[0].(map[string]any)["name"] != "Hero" ||
		chars[0].(map[string]any)["level"].(float64) != 10 || chars[0].(map[string]any)["class"] != "Warrior" {
		t.Fatalf("the account: %v", acc)
	}
	// a player is no GM: refused
	pl := f.browser()
	if code, _ := pl.do("POST", "/api/auth/login", map[string]any{"username": "player", "password": testPassword}); code == 200 {
		if code, _ := pl.op("pwreset_approve", map[string]any{"id": r.ID}); code == 200 {
			t.Fatal("a player approved a reset")
		}
	}
	before, _ := pwhashOf(t, f, 3)
	code, out = gm.op("pwreset_approve", map[string]any{"id": r.ID, "note": "named Hero, level 10 warrior"})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, out)
	}
	pw, _ := out["password"].(string)
	if len(strings.Split(pw, "-")) != 4 || len(pw) > auth.MaxPasswordLen || !strings.Contains(out["tell"].(string), pw) {
		t.Fatalf("the temporary password: %q %v", pw, out)
	}
	hash, temp := pwhashOf(t, f, 3)
	if hash == before || temp != 1 || auth.VerifyPassword(hash, pw) != auth.Accepted {
		t.Fatalf("the account: hash changed %v, temp %d, verifies %v", hash != before, temp, auth.VerifyPassword(hash, pw))
	}
	got, _, _ := f.db.PwResetByID(r.ID)
	if got.Status != "done" || got.DecidedBy == "" || !strings.Contains(got.Review, "Approved by gm") || !strings.Contains(got.Review, "named Hero") {
		t.Fatalf("the request: %+v", got)
	}
	if e, _, _ := f.db.WorkEntryByID(got.Board); e.Status != database.WorkDone {
		t.Fatalf("the board entry: %+v", e)
	}
	// audited, and the password and its hash are nowhere in the log or the request
	var lines int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl IN ('pwresets', 'accountinfo') AND username = 'gm'`).Scan(&lines)
	if lines < 2 {
		t.Fatalf("%d audit lines", lines)
	}
	var leaks int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE COALESCE(before, '') || COALESCE(after, '') || detail LIKE ?
		OR COALESCE(before, '') || COALESCE(after, '') || detail LIKE ?`, "%"+pw+"%", "%"+hash[:20]+"%").Scan(&leaks)
	if leaks != 0 {
		t.Fatalf("the password or its hash is in %d audit lines", leaks)
	}
	if n := countRows(t, f, `SELECT COUNT(*) FROM pwresets WHERE contact || note || review LIKE ?`, "%"+pw+"%"); n != 0 {
		t.Fatal("the password is in the request")
	}
	// decided once
	if code, _ := gm.op("pwreset_approve", map[string]any{"id": r.ID}); code == 200 {
		t.Fatal("approved twice")
	}
	if code, _ := gm.op("pwreset_decline", map[string]any{"id": r.ID}); code == 200 {
		t.Fatal("declined after approval")
	}
}

func countRows(t *testing.T, f *fixture, q string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAPasswordResetIsDeclinedOrStartedByAGM(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	r := filePwReset(t, f, 3, "player")
	before, _ := pwhashOf(t, f, 3)
	if code, out := gm.op("pwreset_decline", map[string]any{"id": r.ID, "note": "not them"}); code != 200 {
		t.Fatalf("decline: %d %v", code, out)
	}
	if h, temp := pwhashOf(t, f, 3); h != before || temp != 0 {
		t.Fatal("a decline changed the password")
	}
	if got, _, _ := f.db.PwResetByID(r.ID); got.Status != "declined" || !strings.HasSuffix(got.Review, "\nNote: not them") {
		t.Fatalf("declined: %+v", got)
	}
	// a GM's own reset, from a Discord ask
	code, out := gm.op("pwreset_start", map[string]any{"account": "player", "contact": "hero#0001 asked on Discord"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	pw := out["password"].(string)
	if h, temp := pwhashOf(t, f, 3); temp != 1 || auth.VerifyPassword(h, pw) != auth.Accepted {
		t.Fatal("the GM's reset set no temporary password")
	}
	req := out["request"].(map[string]any)
	if req["status"] != "done" || req["filedBy"] != "gm" || req["contact"] != "hero#0001 asked on Discord" {
		t.Fatalf("the started request: %v", req)
	}
	// not an account at or above the GM's level
	if code, _ := gm.op("pwreset_start", map[string]any{"account": "boss", "contact": "x"}); code != 403 {
		t.Fatalf("a GM reset its superior: %d", code)
	}
	r2 := filePwReset(t, f, 1, "boss")
	if code, _ := gm.op("pwreset_approve", map[string]any{"id": r2.ID}); code != 403 {
		t.Fatalf("a GM approved its superior's reset: %d", code)
	}
	if got, _, _ := f.db.PwResetByID(r2.ID); got.Status != "waiting" {
		t.Fatalf("a refused approval decided it: %+v", got)
	}
	// set_account_password: temporary sets the flag, otherwise it is cleared
	if code, out := gm.op("set_account_password", map[string]any{"account": "player", "password": "chosen-one"}); code != 200 {
		t.Fatalf("set: %d %v", code, out)
	}
	if _, temp := pwhashOf(t, f, 3); temp != 0 {
		t.Fatal("set_account_password kept the temporary flag")
	}
	gm.op("set_account_password", map[string]any{"account": "player", "password": "chosen-two", "temporary": true})
	if _, temp := pwhashOf(t, f, 3); temp != 1 {
		t.Fatal("set_account_password's temporary did not flag it")
	}
}

// On a shard whose accounts are a copy, the resets are refused (the owner's
// panel makes them).
func TestPasswordResetsAreRefusedOnAShardCopy(t *testing.T) {
	f := newFixture(t)
	f.s.d.AccountsShared = true
	f.s.holdSharedAccounts()
	gm := f.browser()
	gm.login("gm")
	r := filePwReset(t, f, 3, "player")
	for _, call := range []struct {
		name string
		args map[string]any
	}{{"pwreset_approve", map[string]any{"id": r.ID}}, {"pwreset_decline", map[string]any{"id": r.ID}},
		{"pwreset_start", map[string]any{"account": "player", "contact": "x"}}} {
		if code, _ := gm.op(call.name, call.args); code != 403 {
			t.Errorf("%s on a copy: %d", call.name, code)
		}
	}
	if code, _ := gm.op("pwreset_list", map[string]any{}); code != 200 {
		t.Errorf("the list on a copy: %d", code)
	}
}

// The Password resets page in a browser: it lists the request with the
// account's characters, its note as text; Approve shows the temporary password
// in a dialog, and the request leaves the waiting list.
func TestThePasswordResetsPageInABrowser(t *testing.T) {
	var browser string
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			browser = p
			break
		}
	}
	if browser == "" {
		t.Skip("no Chromium to load the page in")
	}
	f := newFixture(t)
	r := filePwReset(t, f, 3, "player")
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><script type="module">
import { setCSRF } from '/static/lib/api.js';
const out = document.getElementById('out');
const wait = async (ok) => { for (let i = 0; i < 100 && !ok(); i++) await new Promise((r) => setTimeout(r, 100)); return ok(); };
const press = (root, label) => [...root.querySelectorAll('button')].find((b) => b.textContent.trim() === label).click();
(async () => {
  try {
    const r = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'gm', password: ` + "'" + testPassword + "'" + ` }) });
    setCSRF((await r.json()).csrf);
    const m = await import('/static/views/pwresets.js');
    await m.render(out, []);
    if (!(await wait(() => out.textContent.includes('Request #` + fmt.Sprint(r.ID) + `')))) throw new Error('no request listed: ' + out.textContent.slice(0, 300));
    if (!out.textContent.includes('Hero') || !out.textContent.includes('hero#0001')) throw new Error('no characters or contact: ' + out.textContent.slice(0, 600));
    press(out, 'Approve: set a temporary password');
    if (!(await wait(() => document.querySelector('.dialog')))) throw new Error('no confirmation');
    press(document.querySelector('.dialog'), 'Set a temporary password');
    if (!(await wait(() => document.querySelector('.dialog code')))) throw new Error('no password shown');
    document.getElementById('out').dataset.pw = document.querySelector('.dialog code').textContent;
    if (!(await wait(() => out.textContent.includes('No request waits')))) throw new Error('it did not leave the waiting list: ' + out.textContent.slice(0, 600));
    out.dataset.done = '1';
  } catch (e) {
    out.dataset.failed = String(e);
  }
})();
</script>`
	mux := http.NewServeMux()
	mux.HandleFunc("/harness.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(harness))
	})
	mux.Handle("/static/", f.s.Handler())
	mux.Handle("/api/", f.s.Handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=20000", "--user-data-dir="+t.TempDir(), "--dump-dom", ts.URL+"/harness.html")
	tmp, err := os.MkdirTemp("", "pwreset")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS=disabled:", "TMPDIR="+tmp)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	dom := string(out)
	if err != nil || !strings.Contains(dom, `id="out"`) {
		t.Skipf("the browser did not load the page (%v): %.300s %.600s", err, out, stderr.String())
	}
	if !strings.Contains(dom, `data-done="1"`) {
		t.Fatalf("the page did not list and approve the request:\n%.4000s", dom)
	}
	i := strings.Index(dom, `data-pw="`)
	if i < 0 {
		t.Fatal("no password read off the page")
	}
	pw := dom[i+len(`data-pw="`):]
	pw = pw[:strings.Index(pw, `"`)]
	if h, temp := pwhashOf(t, f, 3); temp != 1 || auth.VerifyPassword(h, pw) != auth.Accepted {
		t.Fatalf("the password the page showed (%q) is not the account's", pw)
	}
	if strings.Contains(dom, "<b>please</b>") {
		t.Errorf("the player's note is not shown as text")
	}
}

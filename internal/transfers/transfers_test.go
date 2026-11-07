package transfers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// fixture is a transfer server on a database with account 3 ("player", a
// character Hero) and account 4 (a character Else), its check taking
// "player"/"secret" alone.
func fixture(t *testing.T) (*Server, *httptest.Server, *database.SQLiteDB) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (3, 'player', 'x', 1, 0), (4, 'other', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Else', 4, 8, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
		// a shadow knight's (Hero's class) level 20 path, the vampire's line
		// and a flag
		`INSERT INTO quests (questid, name, enabled, classes, source) VALUES
			(41, 'The Dark Path', 1, '3', 'EQOA-Data 1-20/neriak/shadowknight/L20 (tools/import_quests.py)'),
			(61, 'Razeal Has Won', 1, '', 'EQOA-Data Weres/vampire/p1 (tools/import_quests.py)'),
			(62, 'Creed''s Lessons', 1, '', 'EQOA-Data Weres/vampire/L40 (tools/import_quests.py)'),
			(71, 'The Manticores'' Riddles', 1, '', 'EQOA-Data Portal/plane-of-sky/portal-1 (tools/import_quests.py)')`,
		`INSERT INTO queststeps (questid, step, kind, target, choices) VALUES
			(41, 9, 'talk', 'Someone', '{"ask": "Which do you take?", "options": [{"answer": "Harm Touch"}, {"answer": "Dark Blade"}]}'),
			(61, 1, 'talk', 'Someone', ''), (62, 1, 'talk', 'Someone', ''), (71, 1, 'talk', 'Someone', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := New(db, func(r *http.Request, user, pass string) (int, string, string) {
		if user == "player" && pass == "secret" {
			return 3, "player", ""
		}
		return 0, "", "Invalid username or password"
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/transfer", s.ServePage)
	mux.HandleFunc("/api/transfer/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeAPI(w, r, strings.TrimPrefix(r.URL.Path, "/api/transfer/"))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return s, ts, db
}

type client struct {
	t      *testing.T
	base   string
	cookie *http.Cookie
}

// do sends a request the page's way (JSON, its header) unless bare.
func (c *client) do(method, path string, body any, bare bool) (int, map[string]any, *http.Response) {
	c.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if !bare {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-EQOA-Transfer", "1")
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == cookieName {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp
}

// The page; a sign-in refused, then taken (the cookie HttpOnly, Strict and
// Secure behind https); the state lists the account's own character alone; a
// transfer filed and listed; a write without the page's header refused (no
// other site can send one); another account's character refused; a cancel;
// signed out, the state refused.
func TestTransferRoutes(t *testing.T) {
	_, ts, _ := fixture(t)
	resp, err := http.Get(ts.URL + "/transfer")
	if err != nil || resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("the page: %v %v", err, resp)
	}
	resp.Body.Close()
	c := &client{t: t, base: ts.URL}
	if code, _, _ := c.do("GET", "/api/transfer/state", nil, true); code != http.StatusUnauthorized {
		t.Fatalf("state before signing in: %d", code)
	}
	if code, out, _ := c.do("POST", "/api/transfer/login", map[string]any{"username": "player", "password": "nope"}, false); code != 403 || out["ok"] != false {
		t.Fatalf("a wrong password: %d %v", code, out)
	}
	code, _, r := c.do("POST", "/api/transfer/login", map[string]any{"username": "player", "password": "secret"}, false)
	if code != 200 || c.cookie == nil {
		t.Fatalf("sign in: %d", code)
	}
	set := r.Header.Get("Set-Cookie")
	if !strings.Contains(set, "HttpOnly") || !strings.Contains(set, "SameSite=Strict") || !strings.Contains(set, "Secure") {
		t.Fatalf("the cookie: %s", set)
	}
	_, st, _ := c.do("GET", "/api/transfer/state", nil, true)
	chars := st["characters"].([]any)
	if len(chars) != 1 || chars[0].(map[string]any)["name"] != "Hero" || chars[0].(map[string]any)["class"] != "Shadow Knight" {
		t.Fatalf("characters %v", st["characters"])
	}
	// the catalog: Hero's class's path, the vampire's line, the flag; not
	// for another account's character
	code, cat, _ := c.do("GET", "/api/transfer/catalog?serverid=7", nil, true)
	if code != 200 {
		t.Fatalf("catalog: %d %v", code, cat)
	}
	b, _ := json.Marshal(cat["catalog"])
	for _, want := range []string{`"quest":41`, `"answer":"Dark Blade"`, `"line":"vampire"`, `"label":"Plane of Sky"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the catalog lacks %s: %s", want, b)
		}
	}
	if code, _, _ := c.do("GET", "/api/transfer/catalog?serverid=8", nil, true); code != 400 {
		t.Fatalf("another account's character's catalog: %d", code)
	}
	form := map[string]any{"serverid": 7, "from_server": "Sandstorm", "from_name": "Hero", "from_class": "Shadow Knight",
		"from_level": 60, "from_masteries": 1040, "items": "Tower of Woe\nSlayer's Claw Earring x2", "notes": "<b>hi</b>",
		"claims": map[string]any{"picks": []any{map[string]any{"quest": 41, "step": 9, "option": 1}}, "quests": []any{61, 71},
			"line": "vampire", "other": "The Lost Ring"}}
	if code, _, _ := c.do("POST", "/api/transfer/submit", form, true); code != http.StatusBadRequest {
		t.Fatalf("a write without the page's header: %d", code)
	}
	code, out, _ := c.do("POST", "/api/transfer/submit", form, false)
	if code != 200 || out["ok"] != true {
		t.Fatalf("submit: %d %v", code, out)
	}
	id := out["transfer"].(map[string]any)["id"].(float64)
	if code, out, _ := c.do("POST", "/api/transfer/submit", form, false); code != 400 || !strings.Contains(out["error"].(string), "waiting already") {
		t.Fatalf("a second for the same character: %d %v", code, out)
	}
	// a form on a character of another class is refused, saying which (forms #47 and #60
	// went on a rogue and a warrior for a cleric and a monk)
	wrong := map[string]any{"serverid": 7, "from_server": "Sandstorm", "from_name": "Sontosegara", "from_class": "Monk", "from_level": 60}
	if code, out, _ := c.do("POST", "/api/transfer/submit", wrong, false); code != 400 || !strings.Contains(out["error"].(string), "Hero here is a Shadow Knight and the form says Monk") {
		t.Fatalf("a monk's form on a shadow knight: %d %v", code, out)
	}
	other := map[string]any{"serverid": 8, "from_name": "Else", "from_level": 10}
	if code, out, _ := c.do("POST", "/api/transfer/submit", other, false); code != 400 || !strings.Contains(out["error"].(string), "not one of your characters") {
		t.Fatalf("another account's character: %d %v", code, out)
	}
	_, st, _ = c.do("GET", "/api/transfer/state", nil, true)
	if ts := st["transfers"].([]any); len(ts) != 1 || ts[0].(map[string]any)["notes"] != "<b>hi</b>" || ts[0].(map[string]any)["plan"] != nil ||
		ts[0].(map[string]any)["named"] != float64(3) || ts[0].(map[string]any)["line"] != "vampire" {
		t.Fatalf("the list (the player's words as sent, no plan, the quests named): %v", st["transfers"])
	}
	if code, _, _ := c.do("POST", "/api/transfer/cancel", map[string]any{"id": id}, false); code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	c.do("POST", "/api/transfer/logout", map[string]any{}, false)
	if code, _, _ := c.do("GET", "/api/transfer/state", nil, true); code != http.StatusUnauthorized {
		t.Fatalf("state after signing out: %d", code)
	}
}

// noFrameGuard drops the page's framing guard, for the browser test's frame.
type noFrameGuard struct{ http.ResponseWriter }

func (w noFrameGuard) WriteHeader(code int) {
	w.Header().Del("X-Frame-Options")
	w.Header().Del("Content-Security-Policy")
	w.ResponseWriter.WriteHeader(code)
}

func (w noFrameGuard) Write(b []byte) (int, error) {
	w.Header().Del("X-Frame-Options")
	w.Header().Del("Content-Security-Policy")
	return w.ResponseWriter.Write(b)
}

// The page in a real browser, when one is installed: signed in, its character
// offered, the form sent, the transfer listed waiting with the player's words
// as text (not markup); the control: a wrong password is said on the page.
func TestThePageInABrowser(t *testing.T) {
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
	s, _, db := fixture(t)
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><iframe id="f" src="/transfer" style="width:900px;height:900px"></iframe><script>
const out = document.getElementById('out');
const wait = async (ok) => { for (let i = 0; i < 100 && !ok(); i++) await new Promise((r) => setTimeout(r, 100)); return ok(); };
document.getElementById('f').addEventListener('load', async () => {
  try {
    const d = document.getElementById('f').contentDocument;
    const $ = (id) => d.getElementById(id);
    $('u').value = 'player'; $('p').value = 'wrong';
    $('loginForm').requestSubmit();
    if (!(await wait(() => $('loginMsg').textContent.includes('Invalid')))) throw new Error('no refusal shown');
    $('p').value = 'secret';
    $('loginForm').requestSubmit();
    if (!(await wait(() => !$('signedIn').classList.contains('hidden')))) throw new Error('not signed in');
    if (!$('char').textContent.includes('Hero')) throw new Error('no character: ' + $('char').textContent);
    $('fromName').value = '<i>Hero</i>'; $('fromLevel').value = '60'; $('fromCM').value = '1040';
    $('fromClass').value = 'Shadow Knight';
    $('items').value = 'Tower of Woe\n<b>Slayer Claw Earring</b> x2';
    if (!(await wait(() => !$('questsBox').classList.contains('hidden') && $('choice0')))) throw new Error('no quests offered');
    $('choice0').value = '1';
    $('line').value = 'vampire'; $('line').dispatchEvent(new Event('change'));
    if (!(await wait(() => d.querySelectorAll('#lineQuests input').length === 2))) throw new Error('no line quests');
    d.querySelector('#lineQuests input[value="61"]').checked = true;
    d.querySelector('#flags input[value="71"]').checked = true;
    $('otherQuests').value = 'The Lost Ring';
    $('form').requestSubmit();
    if (!(await wait(() => $('list').textContent.includes('waiting')))) throw new Error('not listed: ' + $('formMsg').textContent);
    if ($('list').querySelector('i') || !$('list').textContent.includes('<i>Hero</i>')) throw new Error('the name shown as markup');
    out.dataset.done = '1';
  } catch (e) {
    out.dataset.failed = String(e);
  }
});
</script>`
	mux := http.NewServeMux()
	mux.HandleFunc("/harness.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(harness))
	})
	mux.HandleFunc("/transfer", func(w http.ResponseWriter, r *http.Request) { s.ServePage(noFrameGuard{w}, r) })
	mux.HandleFunc("/api/transfer/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeAPI(w, r, strings.TrimPrefix(r.URL.Path, "/api/transfer/"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=20000", "--user-data-dir="+t.TempDir(), "--dump-dom", ts.URL+"/harness.html")
	tmp, err := os.MkdirTemp("", "xferpage")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS=disabled:", "TMPDIR="+tmp)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	outb, err := cmd.Output()
	dom := string(outb)
	if err != nil || !strings.Contains(dom, `id="out"`) {
		t.Skipf("the browser did not load the page (%v): %.300s %.600s", err, outb, stderr.String())
	}
	if !strings.Contains(dom, `data-done="1"`) {
		t.Fatalf("the page did not file the transfer:\n%.2000s", dom)
	}
	list, err := db.AccountTransfers(3)
	if err != nil || len(list) != 1 || list[0].FromMasteries != 1040 || !strings.Contains(list[0].Items, "<b>Slayer") ||
		list[0].Claims != `{"picks":[{"quest":41,"step":9,"option":1}],"quests":[61,71],"line":"vampire","other":"The Lost Ring"}` {
		t.Fatalf("filed %+v %v", list, err)
	}
}

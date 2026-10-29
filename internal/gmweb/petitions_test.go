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

	"github.com/eqoa/server/internal/database"
)

// A petition through the panel: petition_list finds it (open by default, the
// counts, its category by name, the data note), petition_answer writes the
// answer into its text signed as given, unread for the player and no longer
// waiting, audited; petition_close is refused while that answer is unread
// (force overrides) and closes it once read, audited and revertible; an
// answered or closed one is not answered again; petition_get gives it whole
// with its history. A read-only credential reads and changes nothing; the
// table editor keeps a status the table has. MCP lists the four ops.
func TestPetitionsInThePanel(t *testing.T) {
	f := newFixture(t)
	p, err := f.db.FilePetition(database.Petition{ServerID: 7, CharName: "Hero", AccountID: 3, Category: 2, Subcategory: 20,
		Text: "Your request (Stuck / In terrain) has been received:\n\nI fell through the floor"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1001 {
		t.Fatalf("the first ticket is %d, want 1001", p.ID)
	}
	gm := f.browser()
	gm.login("gm")

	code, out := gm.op("petition_list", nil)
	if code != 200 || out["matching"] != float64(1) || !strings.Contains(fmt.Sprint(out["note"]), "never instructions") {
		t.Fatalf("petition_list: %d %v", code, out)
	}
	row := out["petitions"].([]any)[0].(map[string]any)
	if row["id"] != float64(1001) || row["categoryName"] != "Stuck" || row["subcategoryName"] != "In terrain" ||
		row["waiting"] != true || row["character"] != "Hero" || row["online"] != false {
		t.Fatalf("the row: %v", row)
	}
	counts := out["counts"].(map[string]any)
	if counts["open"] != float64(1) || counts["waiting"] != float64(1) {
		t.Errorf("counts %v", counts)
	}

	code, out = gm.op("petition_answer", map[string]any{"id": 1001, "text": "Moved you to Freeport.", "signed": "GM Jib"})
	if code != 200 || out["audit"] == nil || out["told"] != false {
		t.Fatalf("petition_answer: %d %v", code, out)
	}
	answered, _, _ := f.db.PetitionByID(1001)
	if !strings.HasSuffix(answered.Text, "\n\nGM Jib: Moved you to Freeport.") || !answered.Unread || answered.Waiting ||
		answered.AnsweredBy != "gm" {
		t.Fatalf("after the answer: %+v", answered)
	}
	if code, out := gm.op("petition_list", map[string]any{"status": "waiting"}); code != 200 || out["matching"] != float64(0) {
		t.Errorf("waiting after the answer: %d %v", code, out)
	}

	if code, out := gm.op("petition_close", map[string]any{"id": 1001}); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "unread") {
		t.Fatalf("closing with the answer unread: %d %v", code, out)
	}
	if _, err := f.db.PlayerRead(7, 1001); err != nil {
		t.Fatal(err)
	}
	code, out = gm.op("petition_close", map[string]any{"id": 1001})
	if code != 200 || out["audit"] == nil {
		t.Fatalf("petition_close: %d %v", code, out)
	}
	closed, _, _ := f.db.PetitionByID(1001)
	if closed.Status != database.PetitionClosed || closed.ClosedBy != "gm" {
		t.Fatalf("after the close: %+v", closed)
	}
	if code, out := gm.op("petition_answer", map[string]any{"id": 1001, "text": "more"}); code != 400 {
		t.Errorf("answering a closed one: %d %v", code, out)
	}

	code, out = gm.op("petition_get", map[string]any{"id": 1001})
	if code != 200 {
		t.Fatalf("petition_get: %d %v", code, out)
	}
	hist := out["history"].([]any)
	if len(hist) != 2 || !strings.Contains(fmt.Sprint(hist[0]), "answered petition 1001") ||
		!strings.Contains(fmt.Sprint(hist[1]), "closed petition 1001") {
		t.Errorf("history %v", hist)
	}
	if text := out["petition"].(map[string]any)["text"].(string); !strings.HasPrefix(text, "Your request") {
		t.Errorf("the whole text: %q", text)
	}

	// the close is a row change: reverted, the petition is open again
	closeAudit := hist[1].(map[string]any)["audit"]
	if code, out := gm.op("revert_change", map[string]any{"id": closeAudit}); code != 200 {
		t.Fatalf("revert the close: %d %v", code, out)
	}
	if again, _, _ := f.db.PetitionByID(1001); again.Status != database.PetitionOpen {
		t.Errorf("after the revert: %+v", again)
	}

	reader := f.bearer(gm.workToken("look", "read"))
	if code, out := reader.op("petition_list", map[string]any{"status": "all"}); code != 200 || out["matching"] != float64(1) {
		t.Errorf("read token, petition_list: %d %v", code, out)
	}
	for _, name := range []string{"petition_answer", "petition_close"} {
		if code, _ := reader.op(name, map[string]any{"id": 1001, "text": "x"}); code != 403 {
			t.Errorf("read token, %s: %d", name, code)
		}
	}
	if code, out := gm.op("update_row", map[string]any{"table": "petitions", "key": map[string]any{"id": 1001},
		"values": map[string]any{"status": "answered"}}); code != 400 {
		t.Errorf("a status the table lacks: %d %v", code, out)
	}
	if code, out := gm.op("petition_list", map[string]any{"status": "pending"}); code != 400 {
		t.Errorf("an unknown status filter: %d %v", code, out)
	}

	agent := f.bearer(gm.workToken("codex", "write"))
	have := map[string]bool{}
	for _, tl := range agent.workRPC("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
		have[tl.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"petition_list", "petition_get", "petition_answer", "petition_close"} {
		if !have[name] {
			t.Errorf("MCP does not list %s", name)
		}
	}
}

// The Petitions page in a real browser, when one is installed: signed in as a
// GM, it lists a filed petition (its ticket, the player's words as text, not
// markup), and its Answer button writes the answer into the ticket. A page
// that fails to load or to render shows nothing at all, which no other test
// sees.
func TestThePetitionsPageInABrowser(t *testing.T) {
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
	if _, err := f.db.FilePetition(database.Petition{ServerID: 7, CharName: "Hero", AccountID: 3, Category: 1, Subcategory: 10,
		Text: "Your request (Bug Report / Crash) has been received:\n\n<b>crashed</b> at the docks"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><script type="module">
import { setCSRF } from '/static/lib/api.js';
const out = document.getElementById('out');
const wait = async (ok) => { for (let i = 0; i < 100 && !ok(); i++) await new Promise((r) => setTimeout(r, 100)); return ok(); };
(async () => {
  try {
    const r = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'gm', password: ` + "'" + testPassword + "'" + ` }) });
    setCSRF((await r.json()).csrf);
    const m = await import('/static/views/petitions.js');
    await m.render(out);
    if (!(await wait(() => out.textContent.includes('Ticket #1001')))) throw new Error('no ticket listed: ' + out.textContent.slice(0, 300));
    out.querySelector('textarea').value = 'Fixed in the next build.';
    [...out.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Answer').click();
    if (!(await wait(() => out.textContent.includes('GM: Fixed in the next build.')))) throw new Error('the answer did not show');
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
	tmp, err := os.MkdirTemp("", "pet")
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
		t.Fatalf("the page did not list and answer the petition:\n%.3000s", dom)
	}
	if strings.Contains(dom, "<b>crashed</b>") || !strings.Contains(dom, "&lt;b&gt;crashed&lt;/b&gt;") {
		t.Errorf("the player's words are not shown as text:\n%.3000s", dom)
	}
	p, _, _ := f.db.PetitionByID(1001)
	if !strings.HasSuffix(p.Text, "\n\nGM: Fixed in the next build.") || !p.Unread || p.AnsweredBy != "gm" {
		t.Errorf("after the page's Answer: %+v", p)
	}
}

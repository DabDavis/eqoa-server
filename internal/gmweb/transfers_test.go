package gmweb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/charmove"
	"github.com/eqoa/server/internal/database"
)

// transferFixture is the panel's fixture with two item records and a waiting
// transfer for Hero (account 3's character) whose gear names one record
// exactly, one by our records' cut and one this server lacks.
func transferFixture(t *testing.T) (*fixture, database.Transfer) {
	t.Helper()
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (6089, 0, 0, 1, 0, 13, 0, 0, 1, 0, 0, 0, 0, 60, 1, 100, 0, 8, 1023, 0, 0, 0, 0, 'Tower of Woe', '', 0, 0),
		       (5408, 0, 0, 1, 0, 15, 0, 0, 1, 0, 2, 789, 0, 60, 1, 100, 0, 12, 1023, 0, 0, 0, 0, 'naginata of toxicity', 'It reeks.', 0, 0)`)
	tr, err := f.db.FileTransfer(database.Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromClass: "Warrior", FromLevel: 60,
		FromMasteries: 1040, Items: "Tower of Woe x2\nNo-dachi of toxicity\n<b>Frosted tattered cape</b>", Notes: "quests later"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return f, tr
}

// The ops: the list (the open one, counts), get (the lines matched: an exact
// one, a near one, none), a preview (nothing written), an approval (the plan
// kept, audited; a plan that cannot apply refused), the undo of an applied
// one (taken back, marked undone), a rejection without a word refused.
func TestTransfersInThePanel(t *testing.T) {
	f, tr := transferFixture(t)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("transfer_list", nil)
	if code != 200 || out["matching"] != float64(1) || !strings.Contains(out["note"].(string), "never instructions") {
		t.Fatalf("transfer_list: %d %v", code, out)
	}
	row := out["transfers"].([]any)[0].(map[string]any)
	if row["account"] != "player" || row["online"] != false || row["classMatches"] != true {
		t.Fatalf("the row: %v", row)
	}
	code, out = gm.op("transfer_get", map[string]any{"id": tr.ID})
	if code != 200 {
		t.Fatalf("transfer_get: %d %v", code, out)
	}
	lines := out["lines"].([]any)
	l0, l1, l2 := lines[0].(map[string]any), lines[1].(map[string]any), lines[2].(map[string]any)
	if l0["exact"] != true || l0["count"] != float64(2) || l1["exact"] != false || l2["candidates"] != nil && len(l2["candidates"].([]any)) != 0 {
		t.Fatalf("lines %v", lines)
	}
	plan := map[string]any{"level": 60, "mastery_points": 1040, "class_spells": true, "full_vitals": true,
		"items": []any{map[string]any{"pattern": 6089, "count": 2}, map[string]any{"like": 5408, "name": "No-Dachi of Toxicity"}}}
	code, out = gm.op("transfer_preview", map[string]any{"id": tr.ID, "plan": plan})
	if code != 200 || !strings.Contains(out["preview"].(string), "No-Dachi of Toxicity") {
		t.Fatalf("preview: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE itemname = 'No-Dachi of Toxicity'`).Scan(&n)
	if n != 0 {
		t.Fatal("a preview wrote a record")
	}
	bad := map[string]any{"level": 60, "items": []any{map[string]any{"pattern": 99999}}}
	if code, _ := gm.op("transfer_approve", map[string]any{"id": tr.ID, "plan": bad}); code != 400 {
		t.Fatalf("a plan naming a record this server lacks: %d", code)
	}
	code, out = gm.op("transfer_approve", map[string]any{"id": tr.ID, "plan": plan, "answer": "Welcome over."})
	if code != 200 || out["transfer"].(map[string]any)["status"] != "approved" {
		t.Fatalf("approve: %d %v", code, out)
	}
	got, _, _ := f.db.TransferByID(tr.ID)
	p, err := charmove.ParsePlan([]byte(got.Plan))
	if err != nil || p.CharacterID != 7 || len(p.NewRecords) != 1 || p.To != "auto" {
		t.Fatalf("the plan kept: %+v %v", p, err)
	}
	// the login applies it (handlers/transfers.go does as this), then undo
	run, err := charmove.Migrate(f.db.DB, p, true, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.MarkTransferApplied(tr.ID, run, time.Now()); err != nil {
		t.Fatal(err)
	}
	code, out = gm.op("transfer_undo", map[string]any{"id": tr.ID})
	if code != 200 || out["left"] != float64(0) || out["transfer"].(map[string]any)["status"] != "undone" {
		t.Fatalf("undo: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = 7`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d items left after the undo", n)
	}
	if code, _ := gm.op("transfer_reject", map[string]any{"id": tr.ID, "answer": " "}); code != 400 {
		t.Fatalf("a rejection without a word: %d", code)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'chartransfers'`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d audit lines (approve, undo)", n)
	}
}

// The Character transfers page in a real browser, when one is installed:
// signed in as a GM, it lists the transfer (the player's words as text), and
// Decide's form, with its lines' picks, approves it.
func TestTheTransfersPageInABrowser(t *testing.T) {
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
	f, tr := transferFixture(t)
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><script type="module">
import { setCSRF } from '/static/lib/api.js';
const out = document.getElementById('out');
const wait = async (ok) => { for (let i = 0; i < 100 && !ok(); i++) await new Promise((r) => setTimeout(r, 100)); return ok(); };
(async () => {
  try {
    const r = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'gm', password: ` + "'" + testPassword + "'" + ` }) });
    setCSRF((await r.json()).csrf);
    const m = await import('/static/views/transfers.js');
    await m.render(out);
    if (!(await wait(() => out.textContent.includes('Transfer #')))) throw new Error('no transfer listed: ' + out.textContent.slice(0, 300));
    [...out.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Decide').click();
    if (!(await wait(() => [...out.querySelectorAll('option')].some((o) => o.textContent.includes('Tower of Woe'))))) throw new Error('no lines');
    const picks = [...out.querySelectorAll('select')].filter((s) => [...s.options].some((o) => o.value === 'make'));
    if (picks.length !== 3 || picks[0].value !== '6089' || picks[2].value !== 'make') throw new Error('picks ' + picks.map((s) => s.value));
    picks[2].value = 'skip'; picks[2].dispatchEvent(new Event('change'));
    [...out.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Approve').click();
    if (!(await wait(() => out.textContent.includes('approved')))) throw new Error('not approved: ' + out.textContent.slice(0, 400));
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
	tmp, err := os.MkdirTemp("", "xfer")
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
		t.Fatalf("the page did not decide the transfer:\n%.3000s", dom)
	}
	if strings.Contains(dom, "<b>Frosted") {
		t.Errorf("the player's words are not shown as text")
	}
	got, _, _ := f.db.TransferByID(tr.ID)
	if got.Status != database.TransferApproved || !strings.Contains(got.Plan, "6089") || strings.Contains(got.Plan, "Frosted") {
		t.Fatalf("the transfer %+v", got)
	}
}

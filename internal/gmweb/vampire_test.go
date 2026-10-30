package gmweb

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The sun's settings (world.vampire_sun_*) are an admin's, live and kept: a
// GM may not change them; the admin's change is the world's at once, shown by
// get_settings and written to config.yaml, which loads them again; a number
// out of its range is refused and changes nothing. The control: before the
// change the world has the defaults.
func TestVampireSunSettingsAreLive(t *testing.T) {
	f := newFixture(t)
	if got := f.world.VampireSun(); got != entity.DefaultVampireSun {
		t.Fatalf("the control: the world starts at %+v", got)
	}
	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("update_settings", map[string]any{"vampire_sun_damage": 80}); code != 403 {
		t.Fatalf("a GM changed the sun: %d", code)
	}
	c := f.browser()
	c.login("boss")
	code, out := c.op("update_settings", map[string]any{
		"vampire_sun_damage": 80, "vampire_sun_interval": 4, "vampire_sun_shade": false, "vampire_sun_gms": false,
	})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	want := entity.VampireSun{Damage: 80, Interval: 4 * time.Second, Shade: false, GMs: false}
	if got := f.world.VampireSun(); got != want {
		t.Fatalf("the world's sun: %+v, want %+v", got, want)
	}
	_, out = c.op("get_settings", nil)
	live, _ := out["live"].(map[string]any)
	if live["vampire_sun_damage"] != float64(80) || live["vampire_sun_interval"] != float64(4) ||
		live["vampire_sun_shade"] != false || live["vampire_sun_gms"] != false {
		t.Errorf("get_settings: %v", live)
	}
	b, _ := os.ReadFile(f.cfg)
	for _, w := range []string{"vampire_sun_damage: 80", "vampire_sun_interval: 4s", "vampire_sun_shade: false", "vampire_sun_gms: false", "# the test server"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("config.yaml lacks %q:\n%s", w, b)
		}
	}
	reloaded, err := config.Load(f.cfg)
	if err != nil || reloaded.World.VampireSunDamage == nil || *reloaded.World.VampireSunDamage != 80 ||
		reloaded.World.VampireSunInterval != 4*time.Second || reloaded.World.VampireSunShade == nil || *reloaded.World.VampireSunShade {
		t.Fatalf("reload: %v %+v", err, reloaded.World)
	}
	for _, bad := range []map[string]any{{"vampire_sun_damage": -1}, {"vampire_sun_interval": 0}, {"vampire_sun_interval": 7200}} {
		if code, out := c.op("update_settings", bad); code != 400 {
			t.Errorf("%v: %d %v, want refused", bad, code, out)
		}
	}
	if got := f.world.VampireSun(); got != want {
		t.Errorf("after the refusals: %+v", got)
	}
}

// A GM's charvampire row is the game's at once (entity/vampire.go): the
// character in the world is a vampire where it stands, the one beside it is
// not; the row deleted, it is none again. The control: before any row,
// neither is one.
func TestVampireEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	online := func(id uint32, name string) *entity.Player {
		sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9200 + int(id)}, id, id, 1, 2, true)
		sess.SetCharacterInWorld(true)
		return f.world.CreatePlayer(sess, &database.CharacterRecord{ServerID: id, CharName: name, X: 25300, Y: 54, Z: 15700})
	}
	bitten, plain := online(51, "Bitten"), online(52, "Plain")
	if bitten.Vampire || plain.Vampire {
		t.Fatal("the control: a vampire before any row")
	}
	code, out := c.op("insert_row", map[string]any{"table": "charvampire", "values": map[string]any{"serverid": 51, "note": "a test"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if !bitten.Vampire || plain.Vampire {
		t.Fatalf("after the insert: %v / %v", bitten.Vampire, plain.Vampire)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "Bitten") {
		t.Errorf("live: %v", out["live"])
	}
	if code, out := c.op("delete_row", map[string]any{"table": "charvampire", "key": map[string]any{"serverid": 51}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if bitten.Vampire {
		t.Error("after the delete: still a vampire")
	}
}

// The Settings page in a browser (Chromium, when installed): an admin sees the
// Vampires in the sun card with the world's values, and its Save sends the
// numbers it holds, which the world takes at once. The control: the world's
// sun before the save is the defaults.
func TestTheSettingsPagesSunInABrowser(t *testing.T) {
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
	var id int
	f.db.QueryRow(`SELECT accountid FROM accountinfo WHERE username = 'boss'`).Scan(&id)
	f.enrolTOTP(id)
	if got := f.world.VampireSun(); got != entity.DefaultVampireSun {
		t.Fatalf("the control: the world starts at %+v", got)
	}
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><script type="module">
import { setCSRF } from '/static/lib/api.js';
import { state } from '/static/lib/nav.js';
const out = document.getElementById('out');
const wait = async (ok) => { for (let i = 0; i < 100 && !ok(); i++) await new Promise((r) => setTimeout(r, 100)); return ok(); };
(async () => {
  try {
    const r = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'boss', password: ` + "'" + testPassword + "'" + `, code: '` + totpNow(testTOTPSecret) + `' }) });
    const s = await r.json();
    setCSRF(s.csrf);
    state.session = { user: { admin: true }, server: {} };
    const m = await import('/static/views/settings.js');
    await m.render(out);
    if (!(await wait(() => out.textContent.includes('Vampires in the sun')))) throw new Error('no card: ' + out.textContent.slice(0, 300));
    const nums = [...out.querySelectorAll('input[type=number]')].filter((i) => i.max === '100000' || i.max === '3600');
    if (nums.length !== 2 || nums[0].value !== '50' || nums[1].value !== '6') throw new Error('the values: ' + nums.map((i) => i.value));
    nums[0].value = '70';
    nums[1].value = '3';
    [...out.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Save settings').click();
    await new Promise((r) => setTimeout(r, 1500));
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
	tmp, err := os.MkdirTemp("", "sun")
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
		t.Fatalf("the page did not show and save the sun:\n%.3000s", dom)
	}
	if got := f.world.VampireSun(); got.Damage != 70 || got.Interval != 3*time.Second || !got.Shade || !got.GMs {
		t.Errorf("after the page's Save the world's sun is %+v, want 70 every 3s", got)
	}
}

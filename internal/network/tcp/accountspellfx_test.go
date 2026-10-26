package tcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// spellFXProbe records what the routes hand the community page's service.
type spellFXProbe struct {
	spells int
	ips    []string
}

func (p *spellFXProbe) ServeSpells(w http.ResponseWriter, r *http.Request) {
	p.spells++
	w.Write([]byte(`{"spells":[]}`))
}

func (p *spellFXProbe) ServeSuggest(w http.ResponseWriter, r *http.Request, clientIP string) {
	p.ips = append(p.ips, clientIP)
	w.Write([]byte(`{"ok":true,"count":1}`))
}

// The two community-page routes reach the service, a suggestion with the
// client's own address as nginx names it from loopback; without a service
// they are not there.
func TestTheSpellFXRoutesTakeTheProxysAddress(t *testing.T) {
	probe := &spellFXProbe{}
	s := &AccountServer{SpellFX: probe}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if resp, err := http.Get(ts.URL + "/api/spellfx/spells"); err != nil || resp.StatusCode != 200 || probe.spells != 1 {
		t.Fatalf("spells: %v %v (served %d)", resp, err, probe.spells)
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/spellfx/suggest", strings.NewReader(`{"spell":1,"visual":"0x1"}`))
	req.Header.Set("X-Real-IP", "198.51.100.7")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("suggest: %v %v", resp, err)
	}
	if len(probe.ips) != 1 || probe.ips[0] != "198.51.100.7" {
		t.Fatalf("the suggestion's sender: %v, want the proxy's 198.51.100.7", probe.ips)
	}

	none := httptest.NewServer((&AccountServer{}).Handler())
	defer none.Close()
	for _, path := range []string{"/api/spellfx/spells", "/api/spellfx/suggest"} {
		if resp, err := http.Get(none.URL + path); err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s with no service: %v %v, want 404", path, resp, err)
		}
	}
}

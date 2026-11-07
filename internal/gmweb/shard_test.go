package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// A shard whose accounts are a copy: a GM signs in and reads them as ever, every
// way of changing one is refused and says where to go, whichever route asks. On the
// owner's panel (the control) the same operation goes through. With no transfers
// the panel neither offers nor lists them; the page is told its shard's name.
func TestASharedAccountsPanel(t *testing.T) {
	f := newFixture(t)
	f.addAccount(70, "owned", 0)
	boss := f.browser()
	boss.login("boss")
	if code, out := boss.op("ban_account", map[string]any{"account": "owned", "banned": true}); code != 200 {
		t.Fatalf("the control, the owner's panel: %d %v", code, out)
	}
	if code, out := boss.op("ban_account", map[string]any{"account": "owned", "banned": false}); code != 200 {
		t.Fatalf("the control, the owner's panel: %d %v", code, out)
	}

	shared := newFixture(t)
	shared.addAccount(70, "owned", 0)
	shared.s.d.AccountsShared = true
	shared.s.holdSharedAccounts()
	shared.s.schema = &schemaCache{}
	shared.s.d.Config.Shards = []config.ShardConfig{{Name: "Heirloom Test Server", Port: 10070}, {Name: "Live Server", Port: 10071, Recommended: true}}
	shared.s.d.Config.Server.GamePort = 10071
	gm := shared.browser()
	gm.login("boss")
	for name, args := range map[string]map[string]any{
		"create_account":       {"username": "newone", "password": "longenough"},
		"set_account_password": {"account": "owned", "password": "longenough"},
		"ban_account":          {"account": "owned", "banned": true},
		"set_account_level":    {"account": "owned", "level": 1},
		"set_account_tester":   {"account": "owned", "tester": true, "note": "no"},
	} {
		code, out := gm.op(name, args)
		if code == 200 || !strings.Contains(asText(out), "a copy of the main shard's") {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	// the generic editor too, a revert included: the table is not written here
	if code, out := gm.op("update_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 70}, "set": map[string]any{"email": "x@example.org"}}); code == 200 {
		t.Errorf("update_row on accountinfo: %d %v", code, out)
	}
	// reading goes on
	if code, out := gm.op("get_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 70}}); code != 200 {
		t.Errorf("reading an account: %d %v", code, out)
	}
	// and the word to the page
	_, out := gm.op("server_status", nil)
	if out["shard"] != "Live Server" {
		t.Errorf("server_status names shard %v", out["shard"])
	}
	info := shared.s.sessionInfo(1, nil, true)["server"].(map[string]any)
	if info["shard"] != "Live Server" || info["accountsShared"] != true {
		t.Errorf("the page's word: %v", info)
	}
}

// NoTransfers: no such operation, and nothing else goes missing with it.
func TestAPanelWithoutTransfers(t *testing.T) {
	f := newFixture(t)
	if _, ok := f.s.ops["transfer_list"]; !ok {
		t.Fatal("the control: a server with the form has them")
	}
	s := &Server{d: f.s.d, cfg: f.s.cfg}
	s.d.NoTransfers = true
	s.registerOps()
	for name := range s.ops {
		if strings.HasPrefix(name, "transfer_") {
			t.Errorf("%s offered", name)
		}
	}
	if _, ok := s.ops["petition_list"]; !ok {
		t.Error("petitions went with them")
	}
}

func asText(m map[string]any) string {
	var b strings.Builder
	for _, v := range m {
		if s, ok := v.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}

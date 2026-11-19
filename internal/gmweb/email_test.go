package gmweb

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/mailer"
)

// The accounts' e-mail on the panel (ops_email.go): masked everywhere, the
// counts, a GM's resend and override (audited), and both refused on a shard
// whose accounts are a copy.
func TestEmailOnThePanelIsMasked(t *testing.T) {
	f := newFixture(t)
	sent := &mailer.Memory{}
	svc := accountmail.NewWithQueue(f.db, config.MailConfig{Accounts: true, BaseURL: "https://example.com"}, mailer.NewQueue(sent, 0, []time.Duration{time.Millisecond}))
	defer svc.Close()
	f.s.d.AccountMail = svc
	if _, err := svc.SetEmail(3, "player.one@example.com", ""); err != nil {
		t.Fatal(err)
	}
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("email_counts", map[string]any{})
	c, _ := out["counts"].(map[string]any)
	if code != 200 || c["accounts"].(float64) != 4 || c["pending"].(float64) != 1 || c["noAddress"].(float64) != 3 {
		t.Fatalf("counts: %d %v", code, out)
	}
	code, out = gm.op("email_list", map[string]any{"filter": "pending"})
	b, _ := json.Marshal(out)
	if code != 200 || strings.Contains(string(b), "player.one") || !strings.Contains(string(b), `p***@e***.com`) {
		t.Fatalf("the list shows: %d %s", code, b)
	}
	// the account's page and the generic row: never the address whole
	for _, call := range []struct {
		op   string
		args map[string]any
	}{{"fetch", map[string]any{"id": "account:3"}}, {"get_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 3}}},
		{"email_get", map[string]any{"account": "player"}}} {
		code, out := gm.op(call.op, call.args)
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "player.one") {
			t.Errorf("%s shows the address: %s", call.op, b)
		}
		if call.op == "fetch" && (code != 200 || !strings.Contains(string(b), `p***@e***.com`)) {
			t.Errorf("fetch: %d %s", code, b)
		}
	}
	// a GM's resend: no wait for a GM
	if code, out := gm.op("email_resend", map[string]any{"account": "player"}); code != 200 || out["to"] != "p***@e***.com" {
		t.Fatalf("resend: %d %v", code, out)
	}
	// the override wants a note, then is audited
	if code, _ := gm.op("email_mark_verified", map[string]any{"account": "player"}); code == 200 {
		t.Fatal("an override with no note")
	}
	code, out = gm.op("email_mark_verified", map[string]any{"account": "player", "note": "named Hero, level 10; no mail reaches them"})
	if code != 200 {
		t.Fatalf("override: %d %v", code, out)
	}
	if ok, _ := f.db.EmailVerified(3); !ok {
		t.Fatal("not verified after the override")
	}
	var detail string
	f.db.QueryRow(`SELECT detail FROM gm_audit WHERE detail LIKE 'marked the e-mail%'`).Scan(&detail)
	if !strings.Contains(detail, "player") || strings.Contains(detail, "player.one") {
		t.Fatalf("the audit line: %q", detail)
	}
	// a player is no GM
	pl := f.browser()
	if code, _ := pl.do("POST", "/api/auth/login", map[string]any{"username": "player", "password": testPassword}); code == 200 {
		if code, _ := pl.op("email_mark_verified", map[string]any{"account": "boss", "note": "x"}); code == 200 {
			t.Fatal("a player marked an account verified")
		}
	}
	// refused on a shard's copy
	f.s.d.AccountsShared = true
	f.s.holdSharedAccounts()
	for _, name := range []string{"email_resend", "email_mark_verified"} {
		if code, _ := gm.op(name, map[string]any{"account": "bannedgm", "note": "x"}); code != 403 {
			t.Errorf("%s on a copy: %d", name, code)
		}
	}
}

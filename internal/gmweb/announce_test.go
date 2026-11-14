package gmweb

import (
	"fmt"
	"strings"
	"testing"
)

// The login announcement in the panel (ops_announce.go): a GM sets it, reads it back as the game will send it, and
// clears it with an empty text, each change an audited row change that revert_change undoes; what a PS2 cannot show
// is cut, and a text with nothing left is refused. A tester may do neither (the role's allowlist names no such op).
func TestLoginAnnounceIsSetReadClearedAndReverted(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("get_login_announce", nil); code != 200 || out["text"] != "" {
		t.Fatalf("none at first: %d %v", code, out)
	}
	code, out := c.op("set_login_announce", map[string]any{"text": "  Double XP | this weekend  "})
	if code != 200 || out["text"] != "Double XP  this weekend" || out["line"] != "Announcement: Double XP  this weekend" {
		t.Fatalf("set: %d %v", code, out)
	}
	if code, out := c.op("get_login_announce", nil); code != 200 || out["text"] != "Double XP  this weekend" || out["set_by"] != "gm" {
		t.Fatalf("get: %d %v", code, out)
	}
	if code, out := c.op("set_login_announce", map[string]any{"text": "|||"}); code != 400 {
		t.Fatalf("nothing left: %d %v", code, out)
	}
	if code, out := c.op("set_login_announce", map[string]any{"text": ""}); code != 200 || out["previous"] != "Double XP  this weekend" {
		t.Fatalf("clear: %d %v", code, out)
	}
	if a, _ := f.db.LoginAnnounce(); a.Text != "" {
		t.Fatalf("cleared, it is %+v", a)
	}
	var id int64
	var via string
	f.db.QueryRow(`SELECT id, via FROM gm_audit WHERE tbl = 'loginannounce' AND action = 'delete' ORDER BY id DESC LIMIT 1`).Scan(&id, &via)
	if id == 0 || via != "web" {
		t.Fatalf("the clear's audit line: %d %q", id, via)
	}
	if code, out := c.op("revert_change", map[string]any{"id": id}); code != 200 {
		t.Fatalf("revert the clear: %d %v", code, out)
	}
	if a, _ := f.db.LoginAnnounce(); a.Text != "Double XP  this weekend" {
		t.Fatalf("reverted, it is %+v", a)
	}

	f.newTester()
	tc := f.browser()
	tc.login("tess")
	code, out = tc.op("set_login_announce", map[string]any{"text": "hi"})
	if code != 403 || !strings.Contains(fmt.Sprint(out["error"]), testerRefused) {
		t.Fatalf("a tester set it: %d %v", code, out)
	}
	if a, _ := f.db.LoginAnnounce(); a.Text != "Double XP  this weekend" {
		t.Fatalf("after the tester: %+v", a)
	}
}

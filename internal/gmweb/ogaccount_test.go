package gmweb

import "testing"

// set_account_og: the GM's way to change an [OG] account's choice (a player
// makes it once, at the account's creation): a GM sets it on a player's
// account, audited and revertible (by an admin); never on an account at or above their own
// level. The column is no editor's (update_row refuses it).
func TestSetAccountOG(t *testing.T) {
	f := newFixture(t)
	f.addAccount(71, "meshuggah", 0)
	ogOf := func(id int) int {
		t.Helper()
		var v int
		if err := f.db.QueryRow(`SELECT og FROM accountinfo WHERE accountid = ?`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("update_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 71},
		"values": map[string]any{"og": 1}}); code == 200 {
		t.Fatal("og written through the editor")
	}
	code, out := gm.op("set_account_og", map[string]any{"account": "meshuggah", "og": true, "reason": "asked at creation, the box missed"})
	if code != 200 || ogOf(71) != 1 {
		t.Fatalf("set: %d %v, og %d", code, out, ogOf(71))
	}
	audit := out["change"].(map[string]any)["audit"].(float64)
	var detail string
	f.db.QueryRow(`SELECT COALESCE(detail, '') FROM gm_audit WHERE id = ?`, int64(audit)).Scan(&detail)
	if detail != "reason: asked at creation, the box missed" {
		t.Fatalf("the audit's reason: %q", detail)
	}
	// reverting a change to an account is an admin's (the panel's rule)
	if code, _ := gm.op("revert_change", map[string]any{"id": audit}); code == 200 {
		t.Fatal("a GM reverted an account change")
	}
	boss := f.browser()
	boss.login("boss")
	if code, out := boss.op("revert_change", map[string]any{"id": audit}); code != 200 || ogOf(71) != 0 {
		t.Fatalf("revert: %d %v, og %d", code, out, ogOf(71))
	}
	if code, _ := gm.op("set_account_og", map[string]any{"account": "boss", "og": true}); code == 200 {
		t.Fatal("a GM made an admin's account [OG]")
	}
}

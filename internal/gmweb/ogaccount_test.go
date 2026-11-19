package gmweb

import (
	"strings"
	"testing"
)

// set_character_og: the GM's way to change a character's [OG] choice (a player
// makes it once, while the character is new): audited with its reason,
// revertible, the choice marked made; the reason is required, and the columns
// are no editor's (update_row refuses them). set_account_og is gone.
func TestSetCharacterOG(t *testing.T) {
	f := newFixture(t)
	ogOf := func() (og, chosen int) {
		t.Helper()
		if err := f.db.QueryRow(`SELECT og, og_chosen FROM characters WHERE serverid = 7`).Scan(&og, &chosen); err != nil {
			t.Fatal(err)
		}
		return
	}
	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("update_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7},
		"set": map[string]any{"og": 1}}); code == 200 {
		t.Fatal("og written through the editor")
	}
	if code, _ := gm.op("set_character_og", map[string]any{"character": "Hero", "og": true}); code == 200 {
		t.Fatal("set with no reason")
	}
	if og, _ := ogOf(); og != 0 {
		t.Fatal("the control: set already")
	}
	code, out := gm.op("set_character_og", map[string]any{"character": "Hero", "og": true, "reason": "asked in the box, the box missed"})
	if og, chosen := ogOf(); code != 200 || og != 1 || chosen != 1 {
		t.Fatalf("set: %d %v, og %d chosen %d", code, out, og, chosen)
	}
	audit := out["change"].(map[string]any)["audit"].(float64)
	var detail string
	f.db.QueryRow(`SELECT COALESCE(detail, '') FROM gm_audit WHERE id = ?`, int64(audit)).Scan(&detail)
	if !strings.Contains(detail, "Hero") || !strings.Contains(detail, "asked in the box, the box missed") {
		t.Fatalf("the audit's reason: %q", detail)
	}
	if code, out := gm.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if og, chosen := ogOf(); og != 0 || chosen != 0 {
		t.Fatalf("reverted: og %d chosen %d", og, chosen)
	}
	if code, _ := gm.op("set_account_og", map[string]any{"account": "player", "og": true}); code == 200 {
		t.Fatal("set_account_og still there")
	}
}

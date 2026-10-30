package gmweb

import (
	"strings"
	"testing"
)

// A character's stable ledgers in the panel (charcoaches): a GM grants a town
// as a row, audited, which the revert takes away again; a town spelled
// otherwise than the coach menus spell it, one no coach menu lists, a source
// the game does not write and a row of no character are refused. Deleting the
// character takes its ledgers with it, each audited. The control: before the
// grant the character has none.
func TestCoachLedgerRowsAreAuditedAndRevertible(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	count := func() int {
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM charcoaches WHERE serverid = 7`).Scan(&n)
		return n
	}
	if count() != 0 {
		t.Fatal("the control: Hero has a ledger before any grant")
	}
	code, out := c.op("describe_table", map[string]any{"table": "charcoaches"})
	if code != 200 || out["label"] != "Character coach ledgers" || out["group"] != "Players" ||
		!strings.Contains(out["help"].(string), "coachman") {
		t.Fatalf("describe_table charcoaches: %d %v", code, out)
	}

	code, out = c.op("insert_row", map[string]any{"table": "charcoaches", "values": map[string]any{"serverid": 7, "town": "Highpass Hold"}})
	if code != 200 {
		t.Fatalf("grant: %d %v", code, out)
	}
	audit, _ := out["audit"].(float64)
	var source string
	f.db.QueryRow(`SELECT source FROM charcoaches WHERE serverid = 7 AND town = 'Highpass Hold'`).Scan(&source)
	if count() != 1 || audit == 0 || source != "gm" {
		t.Fatalf("after the grant: %d rows, audit %v, source %q; want one, audited, gm", count(), audit, source)
	}
	for _, bad := range []map[string]any{
		{"serverid": 7, "town": "highpass hold"},
		{"serverid": 7, "town": "Atlantis"},
		{"serverid": 7, "town": "Neriak", "source": "quest"},
		{"town": "Neriak"},
	} {
		if code, out := c.op("insert_row", map[string]any{"table": "charcoaches", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if _, out := c.op("insert_row", map[string]any{"table": "charcoaches", "values": map[string]any{"serverid": 7, "town": "neriak"}}); !strings.Contains(out["error"].(string), `"Neriak"`) {
		t.Errorf("a town in the wrong case: %v, want its spelling named", out["error"])
	}
	if count() != 1 {
		t.Fatalf("after the refusals: %d rows", count())
	}

	if code, out = c.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert the grant: %d %v", code, out)
	}
	if count() != 0 {
		t.Errorf("the grant reverted, %d rows left", count())
	}

	c.op("insert_row", map[string]any{"table": "charcoaches", "values": map[string]any{"serverid": 7, "town": "Freeport", "source": "talk"}})
	code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}, "dry_run": true})
	if also, _ := out["alsoDeleted"].(map[string]any); code != 200 || also["charcoaches"] != float64(1) {
		t.Fatalf("deleting Hero would take %v (%d), want its ledger too", out["alsoDeleted"], code)
	}
	if code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}}); code != 200 {
		t.Fatalf("delete Hero: %d %v", code, out)
	}
	if count() != 0 {
		t.Errorf("Hero deleted, %d of its ledgers left", count())
	}
}

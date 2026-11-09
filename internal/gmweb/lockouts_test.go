package gmweb

import (
	"strings"
	"testing"
	"time"
)

// A character's ability lockouts in the panel (charlockouts, kept over a
// logout): described under Players; deleting the character takes its rows
// with it, each audited, and no other character's (the control).
func TestCharacterLockoutsInThePanel(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, out := c.op("describe_table", map[string]any{"table": "charlockouts"})
	if code != 200 || out["label"] != "Character lockouts" || out["group"] != "Players" ||
		!strings.Contains(out["help"].(string), "logging out ends no recast") {
		t.Fatalf("describe_table charlockouts: %d %v", code, out)
	}
	until := time.Now().Add(10 * time.Minute).UnixMilli()
	for _, row := range [][2]int64{{7, 901}, {7, 902}, {99, 901}} {
		if _, err := f.db.Exec(`INSERT INTO charlockouts (serverid, spellid, until) VALUES (?, ?, ?)`, row[0], row[1], until); err != nil {
			t.Fatal(err)
		}
	}
	count := func(id int) int {
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM charlockouts WHERE serverid = ?`, id).Scan(&n)
		return n
	}
	code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}, "dry_run": true})
	if also, _ := out["alsoDeleted"].(map[string]any); code != 200 || also["charlockouts"] != float64(2) {
		t.Fatalf("deleting Hero would take %v (%d), want its two lockouts too", out["alsoDeleted"], code)
	}
	if code, out = c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}}); code != 200 {
		t.Fatalf("delete Hero: %d %v", code, out)
	}
	if count(7) != 0 || count(99) != 1 {
		t.Errorf("Hero deleted: %d of its lockouts left, %d of another's (want 0 and 1)", count(7), count(99))
	}
}

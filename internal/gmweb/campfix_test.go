package gmweb

import "testing"

// The players' camp reports in the panel (catalog.go's campfix_reports; board
// entry 513): read-only but status and review. A GM's review is taken, and the
// table's trigger stamps when; a player's marks, note and account are refused,
// and so is a status the review does not have.
func TestCampfixReportsAreReadOnlyButTheReview(t *testing.T) {
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO campfix_reports (id, accountid, charname, marks, created)
		VALUES (1, 3, 'Hero', '[{"kind":"move"}]', '2026-11-10T12:00:00Z')`)
	tb, err := f.s.table("campfix_reports")
	if err != nil {
		t.Fatal(err)
	}
	if tb.Label != "Camp fix reports" || len(tb.Columns) != 17 {
		t.Fatalf("the table: %q, %d columns", tb.Label, len(tb.Columns))
	}
	for _, c := range tb.Columns {
		if writable := c.Name == "status" || c.Name == "review"; c.ReadOnly == writable {
			t.Errorf("%s: read-only %v", c.Name, c.ReadOnly)
		}
	}
	gm := f.browser()
	gm.login("gm")
	if code, out := gm.op("update_row", map[string]any{"table": "campfix_reports", "key": map[string]any{"id": 1},
		"values": map[string]any{"status": "declined", "review": "a test filing"}}); code != 200 {
		t.Fatalf("the review: %d %v", code, out)
	}
	r, ok, err := f.db.CampfixReportByID(1)
	if err != nil || !ok || r.Status != "declined" || r.Review != "a test filing" || r.Reviewed == "" {
		t.Fatalf("after the review: %+v %v %v", r, ok, err)
	}
	for _, v := range []map[string]any{{"marks": "[]"}, {"note": "changed"}, {"accountid": 2}, {"status": "later"}} {
		if code, out := gm.op("update_row", map[string]any{"table": "campfix_reports", "key": map[string]any{"id": 1},
			"values": v}); code == 200 {
			t.Errorf("%v taken: %v", v, out)
		}
	}
	if r, _, _ := f.db.CampfixReportByID(1); r.Marks != `[{"kind":"move"}]` || r.Note != "" || r.AccountID != 3 || r.Status != "declined" {
		t.Fatalf("a refused edit changed the row: %+v", r)
	}
}

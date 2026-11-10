package gmweb

import (
	"strings"
	"testing"
)

// The players' camp reports in the panel (catalog.go's campfix_reports; board
// entry 513): read-only but the review. A GM's review line is taken; the
// status is decided by campfix_approve and campfix_decline alone (the Camp
// reports page), so no report is marked applied with nothing applied; a
// player's marks, note and account are refused too.
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
		if writable := c.Name == "review"; c.ReadOnly == writable {
			t.Errorf("%s: read-only %v", c.Name, c.ReadOnly)
		}
	}
	gm := f.browser()
	gm.login("gm")
	if code, out := gm.op("update_row", map[string]any{"table": "campfix_reports", "key": map[string]any{"id": 1},
		"values": map[string]any{"review": "a test filing"}}); code != 200 {
		t.Fatalf("the review: %d %v", code, out)
	}
	r, ok, err := f.db.CampfixReportByID(1)
	if err != nil || !ok || r.Status != "waiting" || r.Review != "a test filing" || r.Reviewed != "" {
		t.Fatalf("after the review: %+v %v %v", r, ok, err)
	}
	for _, v := range []map[string]any{{"status": "declined"}, {"marks": "[]"}, {"note": "changed"}, {"accountid": 2}} {
		code, out := gm.op("update_row", map[string]any{"table": "campfix_reports", "key": map[string]any{"id": 1}, "values": v})
		if code == 200 {
			t.Errorf("%v taken: %v", v, out)
		}
		if _, ok := v["status"]; ok && !strings.Contains(out["error"].(string), "campfix_approve or campfix_decline decides it") {
			t.Errorf("the status's refusal: %v", out)
		}
	}
	if r, _, _ := f.db.CampfixReportByID(1); r.Marks != `[{"kind":"move"}]` || r.Note != "" || r.AccountID != 3 || r.Status != "waiting" {
		t.Fatalf("a refused edit changed the row: %+v", r)
	}
	// the control: the decline decides it, and the trigger stamps when
	if code, out := gm.op("campfix_decline", map[string]any{"id": 1, "note": "a test filing"}); code != 200 {
		t.Fatalf("decline: %d %v", code, out)
	}
	if r, _, _ := f.db.CampfixReportByID(1); r.Status != "declined" || r.Reviewed == "" || !strings.HasPrefix(r.Review, "Declined by gm on ") ||
		!strings.HasSuffix(r.Review, "\nNote: a test filing") {
		t.Fatalf("after the decline: %+v", r)
	}
}

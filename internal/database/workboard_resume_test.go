package database

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// A board made before next, waiting_on, basis and basis_ref were added gets
// them when the board is opened, its entries kept and empty in each.
func TestAnOldBoardGetsTheResumeColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.NewReplacer(`,
		next       TEXT NOT NULL DEFAULT '',
		waiting_on TEXT NOT NULL DEFAULT '',
		basis      TEXT NOT NULL DEFAULT '',
		basis_ref  TEXT NOT NULL DEFAULT ''`, "").Replace(workBoardSchema)
	if old == workBoardSchema {
		t.Fatal("the test's old schema is the new one")
	}
	if _, err := raw.Exec(GMAuditSchema + old); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO workboard (title, status, owner) VALUES ('An old entry', 'in_progress', 'jibrish')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	for i := 0; i < 2; i++ { // a second open finds them there
		db, err := OpenWorkBoard(path)
		if err != nil {
			t.Fatal(err)
		}
		e, ok, err := db.WorkEntryByID(1)
		if err != nil || !ok || e.Title != "An old entry" || e.Next != "" || e.Basis != "" || e.Version != 1 {
			t.Fatalf("open %d: the old entry %+v %v %v", i, e, ok, err)
		}
		db.Close()
	}
	db, err := OpenWorkBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e, audit, err := db.PostWork(WorkPost{ID: 1, Next: workStr("read back npcs 121689"), WaitingOn: workStr("dab"),
		Basis: workStr("shared fix"), BasisRef: workStr("petition 5590"), Version: 1}, workJib)
	if err != nil || audit == 0 || e.Version != 2 || e.Next != "read back npcs 121689" || e.WaitingOn != "dab" ||
		e.Basis != "shared" || e.BasisRef != "petition 5590" {
		t.Fatalf("set on the old entry: %+v %d %v", e, audit, err)
	}
}

func TestTheResumeFieldsAreSetAuditedAndCleared(t *testing.T) {
	db := newWorkDB(t)
	e, _, err := db.PostWork(WorkPost{Title: workStr("Qeynos guards"), Status: workStr("in_progress"),
		Next: workStr("rename npcs 121689"), WaitingOn: workStr("jibrish"), Basis: workStr("era"),
		BasisRef: workStr("Prima p.212")}, workDab)
	if err != nil || e.Next != "rename npcs 121689" || e.Basis != "era" || e.Version != 1 {
		t.Fatalf("post: %+v %v", e, err)
	}
	// a change to one is a version and an audit line naming it
	e, audit, err := db.PostWork(WorkPost{ID: e.ID, Next: workStr("Final check: read the ids back after the next login"),
		Note: "Verified: renamed, read back (audit 163600)", Version: 1}, workDab)
	if err != nil || audit == 0 || e.Version != 2 {
		t.Fatalf("next: %+v %d %v", e, audit, err)
	}
	lines := workAuditLines(t, db, "workboard", e.ID)
	last := lines[len(lines)-1]
	if !strings.Contains(last["detail"].(string), "next") || last["after"].(map[string]any)["next"] != e.Next {
		t.Fatalf("the audit line: %v", last)
	}
	if !strings.Contains(e.LastVerified, "Verified: renamed, read back (audit 163600)") {
		t.Fatalf("last verified: %q", e.LastVerified)
	}
	// the same values again change nothing
	if _, audit, _ := db.PostWork(WorkPost{ID: e.ID, Basis: workStr("era")}, workDab); audit != 0 {
		t.Fatal("an unchanged basis was a change")
	}
	// bad values are refused
	for _, p := range []WorkPost{
		{ID: e.ID, Basis: workStr("vibes")},
		{ID: e.ID, Next: workStr(strings.Repeat("x", WorkMaxNext+1))},
		{ID: e.ID, WaitingOn: workStr(strings.Repeat("x", WorkMaxWaiting+1))},
		{ID: e.ID, BasisRef: workStr(strings.Repeat("x", WorkMaxBasisRef+1))},
	} {
		if _, _, err := db.PostWork(p, workDab); err == nil {
			t.Errorf("%+v was taken", p)
		}
	}
	// a basis cleared takes its reference with it
	e2, _, err := db.PostWork(WorkPost{ID: e.ID, Basis: workStr("none")}, workDab)
	if err != nil || e2.Basis != "" || e2.BasisRef != "" {
		t.Fatalf("basis cleared: %+v %v", e2, err)
	}
	// done clears next and waiting_on; the basis stays
	db.PostWork(WorkPost{ID: e.ID, Basis: workStr("owner"), BasisRef: workStr("owner 2026-11-24")}, workDab)
	e, _, err = db.PostWork(WorkPost{ID: e.ID, Status: workStr("done"), Result: workStr("renamed")}, workDab)
	if err != nil || e.Next != "" || e.WaitingOn != "" || e.Basis != "owner" || e.BasisRef != "owner 2026-11-24" {
		t.Fatalf("done: %+v %v", e, err)
	}
	// unless the change gives them
	e, _, err = db.PostWork(WorkPost{Title: workStr("x"), Status: workStr("done"), Result: workStr("y"),
		Next: workStr("Final check: Live tester sees it")}, workDab)
	if err != nil || e.Next != "Final check: Live tester sees it" {
		t.Fatalf("done with next: %+v %v", e, err)
	}
}

func TestTheBoardsJSONHasTheResumeFields(t *testing.T) {
	db := newWorkDB(t)
	db.PostWork(WorkPost{Title: workStr("a"), Status: workStr("in_progress"), Basis: workStr("decomp"),
		BasisRef: workStr("decomp viswing.cpp"), Next: workStr("port it")}, workDab)
	db.PostWork(WorkPost{Title: workStr("b"), Status: workStr("planned")}, workDab)
	b, err := db.WorkBoard(WorkQuery{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b.Entries)
	var entries []map[string]any
	json.Unmarshal(raw, &entries)
	for _, e := range entries {
		for _, k := range []string{"next", "waiting_on", "basis", "basis_ref", "owner", "status", "version"} {
			if _, ok := e[k]; !ok {
				t.Errorf("entry %v has no %s", e["id"], k)
			}
		}
	}
	for basis, want := range map[string]int{"decomp": 1, "none": 1, "era": 0, "": 2} {
		b, err := db.WorkBoard(WorkQuery{Basis: basis})
		if err != nil || b.Matching != want {
			t.Errorf("basis %q: %d entries, want %d (%v)", basis, b.Matching, want, err)
		}
	}
	if b, _ := db.WorkBoard(WorkQuery{Search: "viswing"}); b.Matching != 1 {
		t.Errorf("search finds the basis_ref: %d", b.Matching)
	}
	if _, err := db.WorkBoard(WorkQuery{Basis: "vibes"}); err == nil {
		t.Error("an unknown basis was a filter")
	}
}

func TestTheBasisTag(t *testing.T) {
	for _, c := range []struct{ basis, ref, want string }{
		{"shared", "petition 5590", "[basis:shared petition 5590] "},
		{"Owner decision", "", "[basis:owner] "},
		{"era", "Prima [p.212]\n", "[basis:era Prima (p.212)] "},
		{"", "", ""},
	} {
		if got, err := BasisTag(c.basis, c.ref); err != nil || got != c.want {
			t.Errorf("BasisTag(%q, %q) = %q, %v; want %q", c.basis, c.ref, got, err, c.want)
		}
	}
	for _, c := range [][2]string{{"vibes", ""}, {"", "petition 1"}, {"era", strings.Repeat("x", WorkMaxBasisRef+1)}} {
		if _, err := BasisTag(c[0], c[1]); err == nil {
			t.Errorf("BasisTag(%q, %q) was taken", c[0], c[1])
		}
	}
}

func TestLastVerifiedIsTheNewestVerifiedNote(t *testing.T) {
	notes := "- 2026-11-24 05:00 UTC, dab's sessions: Verified: step 1 (audit 100)\n" +
		"- 2026-11-24 05:10 UTC, jibrish: rehearsal posted, receipt r-1124a\n  second line\n" +
		"- 2026-11-24 05:20 UTC, dab's sessions: waiting on the restart"
	if got := lastVerifiedNote(notes); !strings.HasPrefix(got, "2026-11-24 05:10 UTC, jibrish: rehearsal posted, receipt r-1124a") {
		t.Fatalf("got %q", got)
	}
	if got := lastVerifiedNote("- 2026-11-24 05:00 UTC, x: nothing checked"); got != "" {
		t.Fatalf("got %q", got)
	}
}

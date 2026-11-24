package database

import (
	"encoding/json"
	"slices"
	"testing"
)

// rawWorkLines is the before and after text of a row's gm_audit lines,
// oldest first.
func rawWorkLines(t *testing.T, db *SQLiteDB, id int64) [][2]string {
	t.Helper()
	k, _ := json.Marshal(map[string]any{"id": id})
	rows, err := db.Query(`SELECT COALESCE(before, ''), COALESCE(after, '') FROM gm_audit
		WHERE tbl = 'workboard' AND rowkey = ? ORDER BY id`, string(k))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var b, a string
		if err := rows.Scan(&b, &a); err != nil {
			t.Fatal(err)
		}
		out = append(out, [2]string{b, a})
	}
	return out
}

// An unrelated edit's images leave the four empty optional columns out
// (board #334); read back, they are "" again.
func TestAnUnrelatedEditLeavesTheEmptyOptionalFieldsOut(t *testing.T) {
	db := newWorkDB(t)
	e, _, err := db.PostWork(WorkPost{Title: workStr("Move the Freeport gate guards"), Touches: workStr("npcs")}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("in_progress")}, workJib); err != nil {
		t.Fatal(err)
	}
	lines := workAuditLines(t, db, "workboard", e.ID)
	if len(lines) != 2 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, l := range lines {
		for _, side := range []string{"before", "after"} {
			m, _ := l[side].(map[string]any)
			for _, k := range WorkAuditOptional {
				if _, ok := m[k]; ok {
					t.Errorf("line %d %s carries %s: %v", i, side, k, m)
				}
			}
		}
	}
	if lines[1]["before"].(map[string]any)["status"] != "planned" || lines[1]["after"].(map[string]any)["status"] != "in_progress" {
		t.Fatalf("the status change: %v", lines[1])
	}

	// what that saves: the same images with the four keys put back
	raw := rawWorkLines(t, db, e.ID)[1]
	full := 0
	for _, s := range raw {
		var m map[string]any
		json.Unmarshal([]byte(s), &m)
		FillWorkAuditImage("workboard", m)
		b, _ := json.Marshal(m)
		full += len(b)
	}
	compact := len(raw[0]) + len(raw[1])
	t.Logf("a status change's images: %d bytes compact, %d with the four empty fields (%d saved)", compact, full, full-compact)
	if full-compact != 104 {
		t.Fatalf("saved %d bytes, want 104", full-compact)
	}

	// history reads it as the change it was
	hist, err := db.WorkHistory("workboard", e.ID, 10)
	if err != nil || len(hist) != 2 || !slices.Equal(hist[1].Changed, []string{"status"}) {
		t.Fatalf("history: %+v %v", hist, err)
	}
}

// Setting a field and clearing it each put it in both images, so history
// names it and the before holds what a revert writes back.
func TestSettingAndClearingAnOptionalFieldIsInBothImages(t *testing.T) {
	db := newWorkDB(t)
	e, _, err := db.PostWork(WorkPost{Title: workStr("Re-fit the Freeport guards")}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"next": "read the ids back", "waiting_on": "dab", "basis": "era", "basis_ref": "Prima p.212"}
	for _, k := range WorkAuditOptional {
		if k == "basis_ref" { // a basis_ref goes with a basis
			if _, _, err := db.PostWork(WorkPost{ID: e.ID, Basis: workStr("test")}, workDab); err != nil {
				t.Fatal(err)
			}
		}
		set, clear := WorkPost{ID: e.ID}, WorkPost{ID: e.ID}
		v, empty := values[k], ""
		switch k {
		case "next":
			set.Next, clear.Next = &v, &empty
		case "waiting_on":
			set.WaitingOn, clear.WaitingOn = &v, &empty
		case "basis":
			set.Basis, clear.Basis = &v, &empty
		case "basis_ref":
			set.BasisRef, clear.BasisRef = &v, &empty
		}
		for i, p := range []WorkPost{set, clear} {
			if _, audit, err := db.PostWork(p, workDab); err != nil || audit == 0 {
				t.Fatalf("%s post %d: %d %v", k, i, audit, err)
			}
			lines := workAuditLines(t, db, "workboard", e.ID)
			last := lines[len(lines)-1]
			b, a := last["before"].(map[string]any), last["after"].(map[string]any)
			wantB, wantA := "", v
			if i == 1 {
				wantB, wantA = v, ""
			}
			if gb, ok := b[k]; !ok || gb != wantB {
				t.Errorf("%s post %d: before %v", k, i, b)
			}
			if ga, ok := a[k]; !ok || ga != wantA {
				t.Errorf("%s post %d: after %v", k, i, a)
			}
			for _, o := range WorkAuditOptional {
				if v, ok := a[o]; o != k && ok && v == "" {
					t.Errorf("%s post %d carries the untouched %s", k, i, o)
				}
			}
			hist, _ := db.WorkHistory("workboard", e.ID, 100)
			if got := hist[len(hist)-1].Changed; !slices.Contains(got, k) {
				t.Errorf("%s post %d: history names %v", k, i, got)
			}
		}
	}
}

// A line from before the compact images (the four keys there, or not there
// at all) still reads as the change it was.
func TestOldFullBoardLinesStillRead(t *testing.T) {
	db := newWorkDB(t)
	e, _, err := db.PostWork(WorkPost{Title: workStr("Old entry")}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	k, _ := json.Marshal(map[string]any{"id": e.ID})
	full := `{"id":1,"title":"Old entry","status":"planned","next":"","waiting_on":"","basis":"","basis_ref":""}`
	fullAfter := `{"id":1,"title":"Old entry","status":"blocked","next":"","waiting_on":"","basis":"","basis_ref":""}`
	bare := `{"id":1,"title":"Old entry","status":"blocked"}`
	bareAfter := `{"id":1,"title":"Old entry","status":"done"}`
	for _, l := range [][2]string{{full, fullAfter}, {bare, bareAfter}} {
		if _, err := db.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after)
			VALUES ('2026-11-20T00:00:00Z', 0, 'dab''s sessions', 'cli', 'local', 'update', 'workboard', ?, ?, ?)`,
			string(k), l[0], l[1]); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := db.WorkHistory("workboard", e.ID, 10)
	if err != nil || len(hist) != 3 || !slices.Equal(hist[1].Changed, []string{"status"}) ||
		!slices.Equal(hist[2].Changed, []string{"status"}) {
		t.Fatalf("history: %+v %v", hist, err)
	}
}

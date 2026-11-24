package gmweb

import (
	"encoding/json"
	"testing"
)

// Board #334: a board line leaves its empty optional fields out; reverted, a
// set comes back "", and a line from before (the fields all there, or a key
// missing from one image) still reverts exactly.
func TestRevertingACompactBoardLine(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("post_work", map[string]any{"title": "Move the Freeport gate guards"})
	if code != 200 {
		t.Fatalf("post: %d %v", code, out)
	}
	id := workEntryOf(t, out)["id"].(float64)
	entry := func() map[string]any {
		t.Helper()
		code, one := gm.op("work_board", map[string]any{"id": id})
		if code != 200 {
			t.Fatalf("work_board: %d %v", code, one)
		}
		e := workEntryOf(t, one)
		for _, k := range []string{"next", "waiting_on", "basis", "basis_ref"} {
			if _, ok := e[k]; !ok {
				t.Fatalf("work_board leaves %s out: %v", k, e)
			}
		}
		return e
	}
	revert := func(line any) {
		t.Helper()
		if code, out := gm.op("revert_change", map[string]any{"id": line}); code != 200 {
			t.Fatalf("revert %v: %d %v", line, code, out)
		}
	}

	// a set, reverted: "" again
	code, out = gm.op("post_work", map[string]any{"id": id, "next": "read the ids back", "waiting_on": "dab"})
	if code != 200 {
		t.Fatalf("set: %d %v", code, out)
	}
	revert(out["audit"])
	if e := entry(); e["next"] != "" || e["waiting_on"] != "" {
		t.Fatalf("after reverting the set: %v", e)
	}

	// an unrelated edit, reverted: the fields untouched
	gm.op("post_work", map[string]any{"id": id, "basis": "era", "basis_ref": "Prima p.212"})
	code, out = gm.op("post_work", map[string]any{"id": id, "status": "in_progress"})
	if code != 200 {
		t.Fatalf("status: %d %v", code, out)
	}
	revert(out["audit"])
	if e := entry(); e["status"] != "planned" || e["basis"] != "era" || e["basis_ref"] != "Prima p.212" {
		t.Fatalf("after reverting the status: %v", e)
	}

	// old lines, put in by hand as earlier builds and other writers wrote them
	e := entry()
	row := map[string]any{}
	for k, v := range e {
		switch k {
		case "touches", "last_verified":
			continue
		}
		row[k] = v
	}
	row["touches"] = ""
	line := func(before, after map[string]any) int64 {
		t.Helper()
		k, _ := json.Marshal(map[string]any{"id": int64(id)})
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		res, err := f.db.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after)
			VALUES ('2026-11-20T00:00:00Z', 2, 'gm', 'web', 'local', 'update', 'workboard', ?, ?, ?)`,
			string(k), string(b), string(a))
		if err != nil {
			t.Fatal(err)
		}
		n, _ := res.LastInsertId()
		return n
	}
	clone := func(m map[string]any, set map[string]any, drop ...string) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			out[k] = v
		}
		for k, v := range set {
			out[k] = v
		}
		for _, k := range drop {
			delete(out, k)
		}
		return out
	}
	// a full-image line (all four there) that set next
	gm.op("post_work", map[string]any{"id": id, "next": "old next", "version": e["version"]})
	e = entry()
	after := clone(row, map[string]any{"next": "old next", "version": e["version"], "updated": e["updated"], "updated_by": e["updated_by"]})
	revert(line(clone(row, map[string]any{"next": ""}), after))
	if e = entry(); e["next"] != "" || e["basis"] != "era" {
		t.Fatalf("after reverting an old full line: %v", e)
	}
	// a line whose before has no waiting_on at all, its after one set: missing reads ""
	gm.op("post_work", map[string]any{"id": id, "waiting_on": "jib"})
	e = entry()
	after = clone(row, map[string]any{"waiting_on": "jib", "next": "", "version": e["version"], "updated": e["updated"], "updated_by": e["updated_by"]})
	revert(line(clone(row, nil, "next", "waiting_on"), after))
	if e = entry(); e["waiting_on"] != "" || e["basis"] != "era" || e["basis_ref"] != "Prima p.212" {
		t.Fatalf("after reverting a line with no before key: %v", e)
	}
}

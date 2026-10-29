package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newWorkDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func workStr(s string) *string { return &s }

var (
	workJib = GMEdit{AccountID: 2, Username: "jibrish", Via: "oauth:Claude", IP: "203.0.113.9"}
	workDab = GMEdit{Username: "dab's sessions", Via: "cli", IP: "local"}
)

// workAuditLines is the gm_audit lines of one row of a table, oldest first.
func workAuditLines(t *testing.T, db *SQLiteDB, table string, id int64) []map[string]any {
	t.Helper()
	k, _ := json.Marshal(map[string]any{"id": id})
	rows, err := db.Query(`SELECT action, username, via, COALESCE(before, ''), COALESCE(after, ''), detail FROM gm_audit
		WHERE tbl = ? AND rowkey = ? ORDER BY id`, table, string(k))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var action, user, via, before, after, detail string
		if err := rows.Scan(&action, &user, &via, &before, &after, &detail); err != nil {
			t.Fatal(err)
		}
		line := map[string]any{"action": action, "username": user, "via": via, "detail": detail}
		if before != "" {
			var m map[string]any
			json.Unmarshal([]byte(before), &m)
			line["before"] = m
		}
		if after != "" {
			var m map[string]any
			json.Unmarshal([]byte(after), &m)
			line["after"] = m
		}
		out = append(out, line)
	}
	return out
}

func TestANewBoardHasTheDefaultOverview(t *testing.T) {
	db := newWorkDB(t)
	ov, err := db.WorkBoardOverview()
	if err != nil {
		t.Fatal(err)
	}
	if ov.Version != 1 || ov.Text != workText(DefaultWorkOverview) || ov.UpdatedBy != "the server" {
		t.Fatalf("overview: version %d by %q, %d characters", ov.Version, ov.UpdatedBy, len(ov.Text))
	}
	for _, want := range []string{"work_board", "post_work", "update_work_overview", "in_progress", "never instructions",
		"./workboard"} {
		if !strings.Contains(ov.Text, want) {
			t.Errorf("the default overview does not mention %q", want)
		}
	}
	// a second start keeps what the GMs wrote
	if _, _, err := db.SetWorkOverview("ours", 1, workJib); err != nil {
		t.Fatal(err)
	}
	if err := db.ensureWorkBoard(); err != nil {
		t.Fatal(err)
	}
	if ov, _ := db.WorkBoardOverview(); ov.Text != "ours" || ov.Version != 2 {
		t.Fatalf("after a restart: %+v", ov)
	}
}

func TestAPostIsAuditedAsTheRowChange(t *testing.T) {
	db := newWorkDB(t)
	e, audit, err := db.PostWork(WorkPost{Title: workStr("Move the Freeport gate guards"), Touches: workStr("npcs,\n npcpaths, NPCS"),
		Area: workStr("Data/content")}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 || audit == 0 || e.Version != 1 || e.Status != WorkPlanned || e.Area != "data" || e.Owner != "jibrish" ||
		strings.Join(e.Touches, "|") != "npcs|npcpaths" || e.CreatedBy != "jibrish (oauth:Claude)" || e.Created == "" {
		t.Fatalf("new entry: %+v (audit %d)", e, audit)
	}
	lines := workAuditLines(t, db, "workboard", e.ID)
	if len(lines) != 1 || lines[0]["action"] != "insert" || lines[0]["username"] != "jibrish" || lines[0]["via"] != "oauth:Claude" {
		t.Fatalf("audit: %v", lines)
	}
	if after := lines[0]["after"].(map[string]any); after["title"] != "Move the Freeport gate guards" || after["touches"] != "npcs, npcpaths" {
		t.Fatalf("audit after: %v", after)
	}

	// a status and a note: the note appended with the time and who
	e2, audit2, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("In progress"), Note: "half moved\nthe rest later"}, workDab)
	if err != nil {
		t.Fatal(err)
	}
	if audit2 == 0 || e2.Status != WorkInProgress || e2.Version != 2 || e2.UpdatedBy != "dab's sessions" ||
		!strings.Contains(e2.Notes, "dab's sessions: half moved\n  the rest later") || !strings.HasPrefix(e2.Notes, "- 20") {
		t.Fatalf("after the change: %+v", e2)
	}
	if e2.Owner != "jibrish" || e2.CreatedBy != "jibrish (oauth:Claude)" {
		t.Fatalf("a change by another took the entry: %+v", e2)
	}
	lines = workAuditLines(t, db, "workboard", e.ID)
	if len(lines) != 2 || lines[1]["action"] != "update" || lines[1]["username"] != "dab's sessions" {
		t.Fatalf("audit: %v", lines)
	}
	if b, a := lines[1]["before"].(map[string]any), lines[1]["after"].(map[string]any); b["status"] != "planned" ||
		a["status"] != "in_progress" || a["version"] != float64(2) {
		t.Fatalf("audit before/after: %v / %v", b, a)
	}
	if !strings.Contains(lines[1]["detail"].(string), "planned -> in_progress") {
		t.Fatalf("audit detail: %v", lines[1]["detail"])
	}

	// a second note goes below the first
	e3, _, err := db.PostWork(WorkPost{ID: e.ID, Note: "all moved"}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	if i, j := strings.Index(e3.Notes, "half moved"), strings.Index(e3.Notes, "all moved"); i < 0 || j < i {
		t.Fatalf("notes: %q", e3.Notes)
	}

	// the same again changes nothing and logs nothing
	same, audit4, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("in_progress"), Title: workStr(" Move the Freeport gate guards ")}, workJib)
	if err != nil || audit4 != 0 || same.Version != e3.Version {
		t.Fatalf("unchanged post: %v, audit %d, version %d -> %d", err, audit4, e3.Version, same.Version)
	}
	if n := len(workAuditLines(t, db, "workboard", e.ID)); n != 3 {
		t.Fatalf("%d audit lines, want 3", n)
	}
}

func TestDoneNeedsAResultAndKeepsWhenItWasDone(t *testing.T) {
	db := newWorkDB(t)
	e, _, err := db.PostWork(WorkPost{Title: workStr("Seed the coach routes"), Status: workStr("doing")}, workJib)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("done")}, workJib); err == nil || !strings.Contains(err.Error(), "result") {
		t.Fatalf("done with no result: %v", err)
	}
	if _, _, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("done"), Result: workStr("one\ntwo")}, workJib); err != nil {
		t.Fatalf("a result's line break is a space: %v", err)
	}
	d, _, _ := db.WorkEntryByID(e.ID)
	if d.Status != WorkDone || d.DoneAt == "" || d.Result != "one two" {
		t.Fatalf("done: %+v", d)
	}
	back, _, err := db.PostWork(WorkPost{ID: e.ID, Status: workStr("blocked"), Note: "the coach NPCs are missing"}, workJib)
	if err != nil || back.DoneAt != "" || back.Status != WorkBlocked {
		t.Fatalf("reopened: %v %+v", err, back)
	}
	for _, bad := range []WorkPost{
		{Title: workStr("")},
		{Title: workStr(strings.Repeat("x", WorkMaxTitle+1))},
		{Title: workStr("ok"), Status: workStr("someday")},
		{Title: workStr("ok"), Area: workStr("the moon")},
		{Title: workStr("ok"), Details: workStr(strings.Repeat("x", WorkMaxDetails+1))},
		{Title: workStr("ok"), Note: strings.Repeat("x", WorkMaxNote+1)},
		{Title: workStr("ok"), Version: 3},
		{ID: 999, Note: "nobody"},
	} {
		if _, _, err := db.PostWork(bad, workJib); err == nil {
			t.Errorf("post %+v was taken", bad)
		}
	}
}

func TestAStaleVersionIsRefusedWithTheCurrentRow(t *testing.T) {
	db := newWorkDB(t)
	e, _, _ := db.PostWork(WorkPost{Title: workStr("Rename the guards"), Details: workStr("first")}, workJib)
	if _, _, err := db.PostWork(WorkPost{ID: e.ID, Details: workStr("second"), Version: e.Version}, workDab); err != nil {
		t.Fatal(err)
	}
	_, _, err := db.PostWork(WorkPost{ID: e.ID, Details: workStr("jib's, over dab's"), Version: e.Version}, workJib)
	var c *WorkConflict
	if !errors.As(err, &c) || c.Version != 2 || c.Given != 1 || c.Current.(WorkEntry).Details != "second" {
		t.Fatalf("stale entry write: %v", err)
	}
	if !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("the refusal does not say the version: %v", err)
	}

	// the overview the same way, and the text as it is now comes back
	ov, _ := db.WorkBoardOverview()
	if _, _, err := db.SetWorkOverview("dab's rules", ov.Version, workDab); err != nil {
		t.Fatal(err)
	}
	_, _, err = db.SetWorkOverview("jib's rules", ov.Version, workJib)
	if !errors.As(err, &c) || c.Current.(WorkOverview).Text != "dab's rules" || c.Version != ov.Version+1 {
		t.Fatalf("stale overview write: %v", err)
	}
	now, _ := db.WorkBoardOverview()
	if now.Text != "dab's rules" || now.UpdatedBy != "dab's sessions" {
		t.Fatalf("the stale write landed: %+v", now)
	}
	// the same text again is no change
	if same, audit, err := db.SetWorkOverview("dab's rules\n\n", now.Version, workJib); err != nil || audit != 0 || same.Version != now.Version {
		t.Fatalf("unchanged overview: %v %d %+v", err, audit, same)
	}
	lines := workAuditLines(t, db, "workboard_overview", 1)
	if len(lines) != 1 || lines[0]["action"] != "update" || lines[0]["after"].(map[string]any)["text"] != "dab's rules" {
		t.Fatalf("overview audit: %v", lines)
	}
}

func TestAWriteThatKeepsTheVersionStillRaisesIt(t *testing.T) {
	// the panel's table editor and a revert write a row as it was, version and
	// all: the trigger raises it, so a writer holding the old one is refused
	db := newWorkDB(t)
	e, _, _ := db.PostWork(WorkPost{Title: workStr("Fix the bank")}, workJib)
	if _, err := db.Exec(`UPDATE workboard SET details = 'by hand', version = 1 WHERE id = ?`, e.ID); err != nil {
		t.Fatal(err)
	}
	got, _, _ := db.WorkEntryByID(e.ID)
	if got.Version != 2 || got.Details != "by hand" {
		t.Fatalf("after a write keeping the version: %+v", got)
	}
	if _, _, err := db.PostWork(WorkPost{ID: e.ID, Details: workStr("mine"), Version: 1}, workJib); err == nil {
		t.Fatal("a writer holding version 1 overwrote the hand edit")
	}
	if _, err := db.Exec(`UPDATE workboard_overview SET text = 'by hand'`); err != nil {
		t.Fatal(err)
	}
	if ov, _ := db.WorkBoardOverview(); ov.Version != 2 {
		t.Fatalf("overview version after a hand edit: %d", ov.Version)
	}
	if _, err := db.Exec(`INSERT INTO workboard_overview (id, text) VALUES (2, 'a second')`); err == nil {
		t.Fatal("a second overview row was taken")
	}
}

func TestTheBoardInOrderWithOldDoneFolded(t *testing.T) {
	db := newWorkDB(t)
	post := func(title, status, touches string) WorkEntry {
		p := WorkPost{Title: workStr(title), Status: workStr(status), Touches: workStr(touches)}
		if status == WorkDone {
			p.Result = workStr("did it")
		}
		e, _, err := db.PostWork(p, workJib)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	planned := post("plan", WorkPlanned, "quests")
	doing := post("doing", WorkInProgress, "npcs, internal/gmweb")
	blocked := post("stuck", WorkBlocked, "itempattern")
	done := post("done lately", WorkDone, "npcs")
	old := post("done long ago", WorkDone, "npcs")
	long := workStamp(time.Now().AddDate(0, 0, -(WorkDoneDays + 6)))
	if _, err := db.Exec(`UPDATE workboard SET done_at = ?, updated = ? WHERE id = ?`, long, long, old.ID); err != nil {
		t.Fatal(err)
	}

	b, err := db.WorkBoard(WorkQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, e := range b.Entries {
		ids = append(ids, e.ID)
	}
	want := []int64{doing.ID, blocked.ID, planned.ID, done.ID}
	if len(ids) != len(want) {
		t.Fatalf("board: %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("board: %v, want %v", ids, want)
		}
	}
	if b.OlderDone != 1 || b.Counts[WorkDone] != 1 || b.Counts[WorkInProgress] != 1 || b.Overview.Version != 1 {
		t.Fatalf("counts: %v, older done %d", b.Counts, b.OlderDone)
	}
	if b, _ := db.WorkBoard(WorkQuery{Statuses: []string{"done"}, OldDone: true}); b.Matching != 2 {
		t.Fatalf("every done entry: %d", b.Matching)
	}
	if b, _ := db.WorkBoard(WorkQuery{Touches: []string{"internal/gmweb/ops.go"}}); b.Matching != 1 || b.Entries[0].ID != doing.ID {
		t.Fatalf("by what it touches: %+v", b.Entries)
	}
	if b, _ := db.WorkBoard(WorkQuery{Owner: "JIB", Search: "STUCK"}); b.Matching != 1 || b.Entries[0].ID != blocked.ID {
		t.Fatalf("by owner and search: %+v", b.Entries)
	}
	if _, err := db.WorkBoard(WorkQuery{Statuses: []string{"someday"}}); err == nil {
		t.Fatal("a status the board does not have was taken")
	}

	// what an entry touching npcs overlaps: the open ones only, in progress first
	ov, err := db.WorkOverlaps(0, []string{"NPCs.modelid", "spellpattern"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ov) != 1 || ov[0].ID != doing.ID || !ov[0].Taken() || ov[0].Touches[0] != "npcs" {
		t.Fatalf("overlaps: %+v", ov)
	}
	if ov, _ := db.WorkOverlaps(doing.ID, []string{"npcs"}); len(ov) != 0 {
		t.Fatalf("an entry overlaps itself: %+v", ov)
	}

	// its history, oldest first, with what each change changed
	db.PostWork(WorkPost{ID: doing.ID, Status: workStr("blocked"), Note: "waiting"}, workDab)
	h, err := db.WorkHistory("workboard", doing.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 || h[0].Action != "insert" || h[0].By != "jibrish (oauth:Claude)" || h[1].By != "dab's sessions" ||
		strings.Join(h[1].Changed, ",") != "notes,status" {
		t.Fatalf("history: %+v", h)
	}
}

func TestTheWordsAStatusOrAnAreaMayTake(t *testing.T) {
	for in, want := range map[string]string{"In progress": "in_progress", "in-progress": "in_progress", "WIP": "in_progress",
		"Planned": "planned", "todo": "planned", "Blocked": "blocked", "finished": "done"} {
		if got, ok := NormalizeWorkStatus(in); !ok || got != want {
			t.Errorf("status %q: %q %v, want %q", in, got, ok, want)
		}
	}
	for in, want := range map[string]string{"GM tools": "gm_tools", "server code": "code", "Data/content": "data",
		"quest": "quests", "Client": "client", "other": "other"} {
		if got, ok := NormalizeWorkArea(in); !ok || got != want {
			t.Errorf("area %q: %q %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := NormalizeWorkStatus("someday"); ok {
		t.Error("someday is a status")
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"npcs", "NPCS", true}, {"npcs", "npcs.modelid", true}, {"internal/gmweb/", "internal/gmweb/ops.go", true},
		{"`npcs`", "npcs", true}, {"./tools/x.py", "tools/x.py", true}, {"npcs", "npcsocial", false},
		{"internal/gm", "internal/gmweb", false}, {"", "npcs", false},
	} {
		if got := TouchesOverlap(c.a, c.b); got != c.want {
			t.Errorf("TouchesOverlap(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestOpenWorkBoardTouchesNothingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eqoa.db")
	if _, err := OpenWorkBoard(path); err == nil {
		t.Fatal("a missing database was made")
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := OpenWorkBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.PostWork(WorkPost{Title: workStr("From the box")}, workDab); err != nil {
		t.Fatal(err)
	}
	var names []string
	rows, _ := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	if got := strings.Join(names, ","); got != "characters,gm_audit,workboard,workboard_overview" {
		t.Fatalf("tables after OpenWorkBoard: %s", got)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// box is a game database as the server makes it, in a temp directory: a
// bandit camp round (25000, 50, 15000), and a petition filed there by Hero
// (account 3, character 7), facing north-east.
func box(t *testing.T) (string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eqoa.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (401, 'bandit', 0, 25040, 50, 15000, 64, 4660, 1.0, 20, 0, '0'),
		       (402, 'bandit', 0, 25000, 50, 15060, 192, 4660, 1.0, 20, 0, '0'),
		       (403, 'bandit captain', 0, 25070, 50, 15070, 0, 4660, 1.0, 22, 0, '0')`); err != nil {
		t.Fatal(err)
	}
	p, err := db.FilePetition(database.Petition{ServerID: 7, CharName: "Hero", AccountID: 3, Category: 1, Subcategory: 10,
		Text: "kappa guard spawns here", ServerView: `{"filed":"2026-11-09T10:00:00Z","via":"chat","character":{"name":"Hero",` +
			`"serverid":7,"level":14,"class":"Druid","race":"Elf","world":0,"x":25010,"y":50,"z":15000,"facing":0.7854}}`}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return path, p.ID
}

func cf(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	return code, out.String(), errb.String()
}

func reports(t *testing.T, path string) []database.CampfixReport {
	t.Helper()
	db, err := database.OpenCampfix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	l, err := db.CampfixReports(database.CampfixQuery{Status: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return l.Reports
}

// A session on the box files a petition's request as a waiting camp report:
// the marks where the petition's character stood, facing its way, the report
// the petition's player's, its board entry posted as dab's sessions; nothing
// spawns. A dry run files nothing; a usage error or a refusal files nothing
// either and says why (the controls).
func TestCampfixfileFilesAWaitingReport(t *testing.T) {
	path, id := box(t)
	pid := func() string { return itoa(id) }
	if code, out, errs := cf(t, "-db", path, "-petition", pid(), "-add", "a bandit", "-count", "2", "-dry"); code != 0 ||
		!strings.Contains(out, "Would file from petition") || len(reports(t, path)) != 0 {
		t.Fatalf("dry run: %d %q %q", code, out, errs)
	}
	code, out, errs := cf(t, "-db", path, "-petition", pid(), "-add", "a bandit", "-count", "2", "-note", "the camp is empty")
	if code != 0 || !strings.Contains(out, "Filed camp report 1 (board entry ") || !strings.Contains(out, "nothing spawns before that") {
		t.Fatalf("file: %d %q %q", code, out, errs)
	}
	rs := reports(t, path)
	if len(rs) != 1 {
		t.Fatalf("%d reports", len(rs))
	}
	r := rs[0]
	if r.Status != database.CampfixWaiting || r.AccountID != 3 || r.CharName != "Hero" || r.Level != 14 || r.Class != 7 ||
		r.X != 25010 || r.Z != 15000 || r.Note != "From petition "+itoa(id)+": the camp is empty" || r.Board == 0 {
		t.Fatalf("the report: %+v", r)
	}
	var marks []handlers.CampfixMarkRecord
	if err := json.Unmarshal([]byte(r.Marks), &marks); err != nil || len(marks) != 2 {
		t.Fatalf("marks: %v %q", err, r.Marks)
	}
	if m := marks[0]; m.Kind != "add" || m.Name != "bandit" || m.X != 25010 || m.Y != 50 || m.Z != 15000 || m.Facing != 0.7854 || m.Template != 401 {
		t.Fatalf("mark 1: %+v", m)
	}
	if d := math.Hypot(marks[1].X-marks[0].X, marks[1].Z-marks[0].Z); d <= 2 {
		t.Fatalf("the adds are %.2f apart", d)
	}
	db, _ := database.OpenCampfix(path)
	defer db.Close()
	e, ok, _ := db.WorkEntryByID(r.Board)
	var npcs, audit int
	db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&npcs)
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'workboard' AND username = 'dab''s sessions' AND via = 'cli'`).Scan(&audit)
	if !ok || e.CreatedBy != "dab's sessions" || e.Owner != handlers.CampfixBoardOwner || npcs != 3 || audit != 1 {
		t.Fatalf("the entry %+v, %d npcs, %d audit lines", e, npcs, audit)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"-db", path, "-petition", pid()}, exitUsage, "Usage: campfixfile"},
		{[]string{"-db", path, "-petition", pid(), "-add", "bandit", "-move", "bandit"}, exitUsage, "one kind a report"},
		{[]string{"-db", path, "-petition", pid(), "-add", "bandit", "captain"}, exitUsage, "is not a flag"},
		{[]string{"-db", path, "-petition", "9999", "-add", "bandit"}, exitFail, "no petition 9999"},
		{[]string{"-db", path, "-petition", pid(), "-add", "zebra"}, exitFail, "No creature named zebra here"},
		{[]string{"-db", path, "-petition", pid(), "-move", "bandit", "-count", "2"}, exitFail, "a count is an add's"},
	} {
		if code, out, errs := cf(t, tc.args...); code != tc.code || !strings.Contains(errs, tc.want) {
			t.Errorf("%q: %d %q %q, want %d and %q", tc.args, code, out, errs, tc.code, tc.want)
		}
	}
	if n := len(reports(t, path)); n != 1 {
		t.Fatalf("a refusal filed a report: %d", n)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

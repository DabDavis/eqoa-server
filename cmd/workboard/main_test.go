package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// board is a game database as the server makes it, in a temp directory.
func board(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "eqoa.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	return path
}

// wb runs the tool and returns its exit code, stdout and stderr.
func wb(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestTheBoxPostsAndClosesWork(t *testing.T) {
	db := board(t)
	if code, out, errs := wb(t, "", "-db", db, "add", "-title", "Move the Freeport gate guards", "-area", "data",
		"-touches", "npcs, npcpaths", "-status", "in_progress"); code != 0 || !strings.Contains(out, "posted #1") {
		t.Fatalf("add: %d %q %q", code, out, errs)
	}
	// someone else's entry touching the same table is named, as a warning
	if code, out, _ := wb(t, "", "add", "-db", db, "-as", "jibrish", "-title", "Rename guards", "-touches", "NPCs.npc_name",
		"-status", "in_progress"); code != 0 || !strings.Contains(out, "WARNING: entry #1 (in_progress, owner dab's sessions) touches npcs") {
		t.Fatalf("the overlapping add: %d %q", code, out)
	}
	if code, out, _ := wb(t, "", "-db", db, "note", "1", "half", "moved"); code != 0 || !strings.Contains(out, "#1 changed") {
		t.Fatalf("note: %d %q", code, out)
	}
	if code, _, errs := wb(t, "", "-db", db, "done", "1"); code != exitUsage || !strings.Contains(errs, "result") {
		t.Fatalf("done with no result: %d %q", code, errs)
	}
	if code, _, errs := wb(t, "", "-db", db, "block", "2"); code != exitUsage || !strings.Contains(errs, "-note") {
		t.Fatalf("block with no note: %d %q", code, errs)
	}
	if code, out, _ := wb(t, "", "-db", db, "done", "-result", "18 guards at the new gate", "1"); code != 0 || !strings.Contains(out, "(done, v3") {
		t.Fatalf("done: %d %q", code, out)
	}
	// a stale version is refused and changes nothing
	if code, _, errs := wb(t, "", "-db", db, "update", "1", "-details", "x", "-version", "1"); code != exitConflict ||
		!strings.Contains(errs, "version 3") {
		t.Fatalf("a stale update: %d %q", code, errs)
	}

	code, out, _ := wb(t, "", "-db", db, "list")
	if code != 0 || !strings.Contains(out, "IN PROGRESS\n  #2 Rename guards") || !strings.Contains(out, "result: 18 guards at the new gate") ||
		!strings.Contains(out, "1 in progress, 0 blocked, 0 planned, 1 done") {
		t.Fatalf("list:\n%s", out)
	}
	code, out, _ = wb(t, "", "-db", db, "-json", "show", "1")
	var shown struct {
		Entry   database.WorkEntry
		History []database.WorkChange
	}
	if code != 0 || json.Unmarshal([]byte(out), &shown) != nil {
		t.Fatalf("show -json: %d %q", code, out)
	}
	if shown.Entry.Status != database.WorkDone || !strings.Contains(shown.Entry.Notes, "dab's sessions: half moved") ||
		len(shown.History) != 3 || shown.History[0].By != "dab's sessions" {
		t.Fatalf("shown: %+v", shown)
	}

	// each change is the panel's kind of audit line: the panel lists it and reverts it
	d, err := database.OpenWorkBoard(db)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'workboard' AND rowkey = '{"id":1}' AND username = 'dab''s sessions'
		AND via = 'cli' AND after IS NOT NULL`).Scan(&n)
	if n != 3 {
		t.Fatalf("%d audit lines for entry 1 as the panel keys them, want 3", n)
	}
}

func TestTheOverviewFromTheBox(t *testing.T) {
	db := board(t)
	code, out, _ := wb(t, "", "-db", db, "overview")
	if code != 0 || !strings.Contains(out, "## The loop") || !strings.Contains(out, "-- version 1") {
		t.Fatalf("overview: %d %q", code, out)
	}
	if code, out, _ := wb(t, "## Conventions\n\n- no deploys on Fridays\n", "-db", db, "overview", "set", "-version", "1"); code != 0 ||
		!strings.Contains(out, "version 2") {
		t.Fatalf("set from stdin: %d %q", code, out)
	}
	if code, _, errs := wb(t, "", "-db", db, "overview", "set", "-version", "1", "-text", "mine"); code != exitConflict ||
		!strings.Contains(errs, "nothing was written") {
		t.Fatalf("a stale set: %d %q", code, errs)
	}
	if code, _, _ := wb(t, "", "-db", db, "overview", "set", "-text", "mine"); code != exitUsage {
		t.Fatalf("a set with no version: %d", code)
	}
	_, out, _ = wb(t, "", "-db", db, "-json", "overview")
	var ov database.WorkOverview
	if json.Unmarshal([]byte(out), &ov) != nil || ov.Version != 2 || !strings.Contains(ov.Text, "Fridays") || ov.UpdatedBy != "dab's sessions" {
		t.Fatalf("after: %q", out)
	}
	if code, out, _ := wb(t, "", "overview", "default"); code != 0 || out != database.DefaultWorkOverview {
		t.Fatalf("default: %d", code)
	}
}

func TestTheDatabaseIsTheConfigsOrRefused(t *testing.T) {
	db := board(t)
	dir := filepath.Dir(db)
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("server:\n  db_path: \"eqoa.db\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := wb(t, "", "-config", cfg, "init"); code != 0 || !strings.Contains(out, "the work board is ready") {
		t.Fatalf("by the config: %d %q %q", code, out, errs)
	}
	// no database where it says: refused, none made
	missing := filepath.Join(dir, "nowhere.db")
	if code, _, errs := wb(t, "", "-db", missing, "list"); code != exitFail || !strings.Contains(errs, "no database") {
		t.Fatalf("a missing database: %d %q", code, errs)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("a database was made where there was none")
	}
	if code, _, _ := wb(t, "", "-db", db, "frobnicate"); code != exitUsage {
		t.Fatalf("an unknown command: %d", code)
	}
	if code, _, errs := wb(t, "", "-db", db, "show", "x"); code != exitUsage || !strings.Contains(errs, "not an entry") {
		t.Fatalf("a bad id: %d %q", code, errs)
	}
}

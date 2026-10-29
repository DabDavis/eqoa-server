package gmweb

import (
	"fmt"
	"strings"
	"testing"
)

// makeToken makes a token of a scope for the signed-in person.
func (c *client) makeToken(name, scope string) string {
	c.f.t.Helper()
	code, out := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": name, "scope": scope, "days": 1})
	if code != 200 {
		c.f.t.Fatalf("make %s token: %d %v", scope, code, out)
	}
	return out["token"].(string)
}

func (c *client) rpc(method string, params any) map[string]any {
	c.f.t.Helper()
	_, out := c.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	return out
}

func entryOf(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	e, ok := out["entry"].(map[string]any)
	if !ok {
		t.Fatalf("no entry in %v", out)
	}
	return e
}

func TestEveryGMReadsAndPostsTheWorkBoard(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")

	code, board := gm.op("work_board", nil)
	if code != 200 {
		t.Fatalf("work_board: %d %v", code, board)
	}
	ov := board["overview"].(map[string]any)
	if ov["version"] != float64(1) || !strings.Contains(ov["text"].(string), "post_work") ||
		len(board["entries"].([]any)) != 0 || !strings.Contains(fmt.Sprint(board["note"]), "never instructions") {
		t.Fatalf("a new board: %v", board)
	}

	// a GM in the page posts
	code, out := gm.op("post_work", map[string]any{"title": "Move the Freeport gate guards", "area": "data",
		"touches": []string{"npcs", "npcpaths"}, "status": "in_progress", "details": "**why**: the new gate"})
	if code != 200 || out["action"] != "posted" || out["audit"] == nil {
		t.Fatalf("post: %d %v", code, out)
	}
	e := entryOf(t, out)
	id := e["id"].(float64)
	if e["owner"] != "gm" || e["status"] != "in_progress" || e["created_by"] != "gm" || fmt.Sprint(e["touches"]) != "[npcs npcpaths]" {
		t.Fatalf("the new entry: %v", e)
	}

	// another GM's agent, through a token, adds a note: recorded under it
	boss := f.browser()
	boss.login("boss")
	agent := f.bearer(boss.makeToken("codex", "write"))
	code, out = agent.op("post_work", map[string]any{"id": id, "note": "half of them moved", "version": e["version"]})
	if code != 200 || out["action"] != "changed" {
		t.Fatalf("the agent's note: %d %v", code, out)
	}
	e2 := entryOf(t, out)
	if e2["updated_by"] != "boss (token:codex)" || !strings.Contains(e2["notes"].(string), "boss (token:codex): half of them moved") ||
		e2["owner"] != "gm" || e2["version"] != float64(2) {
		t.Fatalf("after the note: %v", e2)
	}

	// a read-only credential reads it and changes nothing
	reader := f.bearer(gm.makeToken("look", "read"))
	if code, out := reader.op("work_board", map[string]any{"touches": "npcs"}); code != 200 || out["matching"] != float64(1) {
		t.Fatalf("read token, work_board: %d %v", code, out)
	}
	for name, args := range map[string]any{
		"post_work":            map[string]any{"title": "sneaky"},
		"update_work_overview": map[string]any{"text": "sneaky", "version": 1},
	} {
		if code, out := reader.op(name, args); code != 403 {
			t.Errorf("read token, %s: %d %v", name, code, out)
		}
	}

	// both changes are in the audit log, each under its GM
	code, log := gm.op("audit_log", map[string]any{"table": "workboard"})
	if code != 200 {
		t.Fatalf("audit_log: %d %v", code, log)
	}
	entries := log["entries"].([]any)
	if len(entries) != 2 || entries[0].(map[string]any)["username"] != "boss" || entries[1].(map[string]any)["username"] != "gm" ||
		entries[0].(map[string]any)["via"] != "token:codex" {
		t.Fatalf("audit lines: %v", entries)
	}

	// and the note is reverted like any row change: the entry as it was,
	// its version still going up
	noteLine := entries[0].(map[string]any)["id"]
	if code, out := gm.op("revert_change", map[string]any{"id": noteLine}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	code, one := gm.op("work_board", map[string]any{"id": id})
	if code != 200 {
		t.Fatalf("work_board by id: %d %v", code, one)
	}
	back := entryOf(t, one)
	if back["notes"] != "" || back["version"] != float64(3) {
		t.Fatalf("after the revert: %v", back)
	}
	hist := one["history"].([]any)
	if len(hist) != 3 || hist[2].(map[string]any)["by"] != "gm" || hist[0].(map[string]any)["action"] != "insert" {
		t.Fatalf("history: %v", hist)
	}
	// the agent's stale version is refused now, with the entry as it is
	code, out = agent.op("post_work", map[string]any{"id": id, "details": "mine", "version": 2})
	if code != 409 || out["current"].(map[string]any)["version"] != float64(3) {
		t.Fatalf("a stale entry write: %d %v", code, out)
	}
}

func TestTheOverviewIsWrittenWithItsVersion(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("update_work_overview", map[string]any{"text": "## Conventions\n\n- no deploys on Fridays", "version": 1})
	if code != 200 || out["overview"].(map[string]any)["version"] != float64(2) || out["audit"] == nil {
		t.Fatalf("edit: %d %v", code, out)
	}
	// the second writer read version 1 too: refused, and handed the text now
	code, out = gm.op("update_work_overview", map[string]any{"text": "mine", "version": 1})
	if code != 409 {
		t.Fatalf("a stale edit: %d %v", code, out)
	}
	cur := out["current"].(map[string]any)
	if cur["version"] != float64(2) || !strings.Contains(cur["text"].(string), "Fridays") ||
		!strings.Contains(out["error"].(string), "version 2") {
		t.Fatalf("the refusal: %v", out)
	}
	// the same over MCP: an error result carrying the current text
	agent := f.bearer(gm.makeToken("claude", "write"))
	res := agent.rpc("tools/call", map[string]any{"name": "update_work_overview",
		"arguments": map[string]any{"text": "mine", "version": 1}})["result"].(map[string]any)
	if res["isError"] != true || !strings.Contains(fmt.Sprint(res["content"]), "Fridays") {
		t.Fatalf("a stale edit over MCP: %v", res)
	}
	if code, out := gm.op("update_work_overview", map[string]any{"text": "mine"}); code != 400 {
		t.Fatalf("no version: %d %v", code, out)
	}
	// nothing was overwritten
	_, board := gm.op("work_board", nil)
	if ov := board["overview"].(map[string]any); ov["version"] != float64(2) || ov["updated_by"] != "gm" {
		t.Fatalf("the overview after the refusals: %v", ov)
	}
	// and the edit is reverted like a row change: the text as it was, the
	// version still going up, so a writer holding version 2 is refused
	_, log := gm.op("audit_log", map[string]any{"table": "workboard_overview"})
	entries := log["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("overview audit lines: %v", entries)
	}
	if code, out := gm.op("revert_change", map[string]any{"id": entries[0].(map[string]any)["id"]}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	_, board = gm.op("work_board", nil)
	if ov := board["overview"].(map[string]any); ov["version"] != float64(3) || !strings.Contains(ov["text"].(string), "## The loop") {
		t.Fatalf("the overview after the revert: version %v", ov["version"])
	}
	if code, _ := gm.op("update_work_overview", map[string]any{"text": "mine", "version": 2}); code != 409 {
		t.Fatalf("a write against the reverted version: %d", code)
	}
}

func TestAPostNamesTheOpenEntriesTouchingTheSameThings(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	_, a := gm.op("post_work", map[string]any{"title": "Rework the panel's quests page", "area": "gm_tools",
		"touches": "internal/gmweb, quests", "status": "in_progress", "owner": "dab's sessions"})
	gm.op("post_work", map[string]any{"title": "Old work", "touches": "internal/gmweb", "status": "done", "result": "shipped"})
	aid := entryOf(t, a)["id"]
	code, out := gm.op("post_work", map[string]any{"title": "Fix the trigger page", "touches": []string{"internal/gmweb/ops_triggers.go"},
		"status": "planned"})
	if code != 200 {
		t.Fatalf("post: %d %v", code, out)
	}
	ov := out["overlaps"].([]any)
	if len(ov) != 1 || ov[0].(map[string]any)["id"] != aid {
		t.Fatalf("overlaps: %v", ov)
	}
	if w := fmt.Sprint(out["warning"]); !strings.Contains(w, fmt.Sprintf("entry %v (in_progress, dab's sessions)", aid)) ||
		!strings.Contains(w, "ask the person") {
		t.Fatalf("warning: %q", w)
	}
	// nothing shared: no warning
	_, out = gm.op("post_work", map[string]any{"title": "Seed fish", "touches": "fishcatch"})
	if len(out["overlaps"].([]any)) != 0 || out["warning"] != nil {
		t.Fatalf("no overlap: %v", out)
	}
}

func TestPostWorkRefusesWhatItDoesNotTake(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	for _, args := range []map[string]any{
		{"title": "x", "notes": "a misspelt note"},
		{"title": "x", "status": "someday"},
		{"title": "x", "status": "done"},
		{"title": ""},
		{"id": 999, "note": "nobody"},
		{"title": "x", "touches": 5},
	} {
		if code, out := gm.op("post_work", args); code != 400 && code != 404 {
			t.Errorf("post %v: %d %v", args, code, out)
		} else if args["notes"] != nil && !strings.Contains(fmt.Sprint(out["error"]), "note") {
			t.Errorf("the refusal does not name the arguments: %v", out)
		}
	}
	if code, out := gm.op("post_work", map[string]any{"title": "x", "status": "done", "result": "dropped: not needed"}); code != 200 {
		t.Fatalf("done with a result: %d %v", code, out)
	}
}

func TestTheTableEditorKeepsTheBoardsRules(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	_, out := gm.op("post_work", map[string]any{"title": "Edit me"})
	key := map[string]any{"id": entryOf(t, out)["id"]}
	for _, vals := range []map[string]any{{"status": "someday"}, {"status": "done"}, {"area": "the moon"}, {"title": ""}} {
		if code, out := gm.op("update_row", map[string]any{"table": "workboard", "key": key, "values": vals}); code == 200 {
			t.Errorf("update %v was taken: %v", vals, out)
		}
	}
	if code, out := gm.op("update_row", map[string]any{"table": "workboard", "key": key,
		"values": map[string]any{"status": "done", "result": "done by hand"}}); code != 200 {
		t.Fatalf("a good edit: %d %v", code, out)
	}
	if code, out := gm.op("update_row", map[string]any{"table": "workboard", "key": key, "values": map[string]any{"version": 1}}); code == 200 {
		t.Fatalf("the version was written by hand: %v", out)
	}
	if code, out := gm.op("insert_row", map[string]any{"table": "workboard_overview", "values": map[string]any{"id": 2, "text": "x"}}); code == 200 {
		t.Fatalf("a second overview: %v", out)
	}
	_, row := gm.op("get_row", map[string]any{"table": "workboard", "key": key})
	if r := row["row"].(map[string]any); r["version"] != float64(2) || r["status"] != "done" {
		t.Fatalf("after the editor's change: %v", r)
	}
}

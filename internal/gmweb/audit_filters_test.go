package gmweb

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// auditLines writes lines as the other writers do: a data tool (its key
// spaced as Python's json writes it), a GM command in the game, an agent.
func auditLines(t *testing.T, f *fixture) {
	t.Helper()
	mustExec(t, f.db, `INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES
		(?, 1, 'dab', 'tool', '', 'delete', 'npcrespawn', '{"npc_row": 502}', '{"npc_row": 502, "seconds": 600}', NULL,
		 'petition 1446 (Jibrish): the skeleton goes; tools/skel_20261111.py'),
		(?, 9, 'forestwood', 'game', '', 'insert', 'npcs', '{"id":600}', NULL, '{"id":600,"npc_name":"Guard Dahvel","world":1}',
		 '/clone in the game: Guard Dahvel (row 500), a copy of NPC 500'),
		('2026-11-01T10:00:00Z', 1, 'jibrish', 'oauth:Claude', '', 'update', 'spellpattern', '{"spellid":101}',
		 '{"spellid":101,"spellname":"Root","power":5}', '{"spellid":101,"spellname":"Greater Root","power":5}', ''),
		(?, 1, 'funyun', 'token:Claude Code (dab sessions)', '', 'insert', 'charinventory', '{"itemid":9001}', NULL,
		 '{"itemid":9001,"serverid":7,"patternid":10,"equiploc":-1}', 'camp report 122, mark 1: a reward'),
		(?, 1, 'dab', 'web', '', 'login', '', '', NULL, NULL, 'signed in')`, now(), now(), now(), now())
}

func auditTotal(t *testing.T, c *client, args map[string]any) (float64, []map[string]any) {
	t.Helper()
	code, out := c.op("audit_log", args)
	if code != 200 {
		t.Fatalf("audit_log %v: %d %v", args, code, out)
	}
	var list []map[string]any
	for _, e := range out["entries"].([]any) {
		list = append(list, e.(map[string]any))
	}
	return out["total"].(float64), list
}

// The log's filters find what a GM asks for: an NPC deleted by name, its
// respawn row by the NPC's id, a tool's run, a petition's changes, an agent's,
// a world's, a day's; and a changed row says only what changed.
func TestTheAuditLogFindsWhatAGMAsksFor(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501}, "values": map[string]any{"npc_level": 21}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out := c.op("delete_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 502}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	auditLines(t, f)

	n, list := auditTotal(t, c, map[string]any{"preset": "npcs_deleted"})
	if n != 1 || list[0]["thing"] != "a decaying skeleton" || list[0]["kind"] != "deleted" || list[0]["source"] != "panel" {
		t.Errorf("NPCs deleted: %v %v", n, list)
	}
	// the respawn row of a deleted NPC is named by the NPC's own line
	if n, list := auditTotal(t, c, map[string]any{"name": "skeleton"}); n != 2 {
		t.Errorf("by name: %v %v", n, list)
	}
	if n, list := auditTotal(t, c, map[string]any{"row_id": 502}); n != 2 {
		t.Errorf("by row id: %v %v", n, list)
	}
	n, list = auditTotal(t, c, map[string]any{"source": "tool"})
	if n != 1 || list[0]["tool"] != "skel_20261111.py" || fmt.Sprint(list[0]["petitions"]) != "[1446]" || list[0]["thing"] != "a decaying skeleton" {
		t.Errorf("a tool's: %v %v", n, list)
	}
	if n, _ := auditTotal(t, c, map[string]any{"tool": "skel_2026"}); n != 1 {
		t.Errorf("by tool: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"petition": 1446}); n != 1 {
		t.Errorf("by petition: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"petition": 1447}); n != 0 {
		t.Errorf("another petition: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"preset": "camp_reports"}); n != 1 {
		t.Errorf("camp reports: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"who": "claude"}); n != 2 {
		t.Errorf("an agent's: %v", n)
	}
	n, list = auditTotal(t, c, map[string]any{"source": "game"})
	if n != 1 || list[0]["tool"] != "/clone" || list[0]["world"] != float64(1) || list[0]["worldName"] == "" {
		t.Errorf("the game's: %v %v", n, list)
	}
	if n, _ := auditTotal(t, c, map[string]any{"world": 1}); n != 1 {
		t.Errorf("by world: %v", n)
	}
	n, list = auditTotal(t, c, map[string]any{"table": "npcs", "action": "changed"})
	ch, _ := list[0]["changes"].([]any)
	if n != 1 || len(ch) != 1 || fmt.Sprint(ch[0]) != "map[after:21 before:20 field:npc_level label:Level]" && !strings.Contains(fmt.Sprint(ch[0]), "field:npc_level") {
		t.Errorf("only what changed: %v %v", n, list)
	}
	// the old name finds a renamed row, as the new one does
	if n, _ := auditTotal(t, c, map[string]any{"name": "root"}); n != 1 {
		t.Errorf("an old name: %v", n)
	}
	if n, list := auditTotal(t, c, map[string]any{"name": "greater"}); n != 1 || list[0]["thing"] != "Greater Root" {
		t.Errorf("a new name: %v %v", n, list)
	}
	if _, list := auditTotal(t, c, map[string]any{"table": "charinventory"}); len(list) != 1 || list[0]["thing"] != "Rusty Sword · Hero" {
		t.Errorf("an item given: %v", list)
	}
	if n, _ := auditTotal(t, c, map[string]any{"until": "2026-11-01"}); n != 1 {
		t.Errorf("until a day: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"since": "2026-11-02", "group": "npcs"}); n != 4 {
		t.Errorf("since a day, NPCs: %v", n)
	}
	if n, _ := auditTotal(t, c, map[string]any{"preset": "today", "table": "npcs,npcrespawn", "action": "deleted"}); n != 2 {
		t.Errorf("a preset with the rest: %v", n)
	}
	if code, out := c.op("audit_log", map[string]any{"since": "last tuesday"}); code != 400 {
		t.Errorf("a bad date: %d %v", code, out)
	}
	if code, out := c.op("audit_log", map[string]any{"preset": "nope"}); code != 400 {
		t.Errorf("a bad preset: %d %v", code, out)
	}

	// counts: each choice with the others held, each preset alone
	code, out := c.op("audit_log", map[string]any{"group": "npcs", "counts": true})
	if code != 200 {
		t.Fatalf("counts: %d %v", code, out)
	}
	counts := out["counts"].(map[string]any)
	kind := counts["kind"].(map[string]any)
	if kind["deleted"] != float64(2) || kind["changed"] != float64(1) || kind["added"] != float64(1) {
		t.Errorf("by kind: %v", kind)
	}
	if g := counts["group"].(map[string]any); g["spells"] != float64(1) || g["npcs"] != float64(4) {
		t.Errorf("by group (its own filter left out): %v", g)
	}
	if p := counts["presets"].(map[string]any); p["npcs_deleted"] != float64(1) || p["items_given"] != float64(1) || p["spells_changed"] != float64(1) {
		t.Errorf("presets: %v", p)
	}
	if fmt.Sprint(counts["tool"]) == "[]" || counts["who"] == nil {
		t.Errorf("tools and who: %v", counts)
	}

	// a revert: the change is reverted, the revert one
	_, list = auditTotal(t, c, map[string]any{"table": "npcs", "action": "changed"})
	if code, out := c.op("revert_change", map[string]any{"id": list[0]["id"]}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if n, list := auditTotal(t, c, map[string]any{"preset": "reverted"}); n != 1 || list[0]["revertedBy"] == nil {
		t.Errorf("reverted: %v %v", n, list)
	}
	if n, list := auditTotal(t, c, map[string]any{"preset": "reverts"}); n != 1 || list[0]["isRevert"] != true || list[0]["thing"] != "Merchant Shadehand" {
		t.Errorf("reverts: %v %v", n, list)
	}
	// the free text still reads the lines' words
	if n, _ := auditTotal(t, c, map[string]any{"search": "a copy of NPC 500"}); n != 1 {
		t.Errorf("free text: %v", n)
	}
	// a line a tool writes after is in at the next read
	mustExec(t, f.db, `INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, 1, 'dab', 'tool', '', 'insert', 'itempattern', '{"patternid": 12}', NULL, '{"patternid": 12, "itemname": "Sword of Tests"}', 'tools/x.py')`, now())
	if n, list := auditTotal(t, c, map[string]any{"preset": "items_added"}); n != 1 || list[0]["thing"] != "Sword of Tests" {
		t.Errorf("a late line: %v %v", n, list)
	}
}

// A tester filters their own tables' lines by what and when, never by who or
// by what the words say.
func TestATestersAuditFiltersStayTheirs(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	gm := f.browser()
	gm.login("gm")
	gm.op("update_row", map[string]any{"table": "itempattern", "key": map[string]any{"patternid": 10}, "values": map[string]any{"itemcost": 50}})
	auditLines(t, f)
	r := f.routesOf("tess")
	if code, _, out := atOp(r.page, "audit_log", `{"name":"root","counts":true}`); code != 200 || out["total"] != float64(1) {
		t.Errorf("by name: %d %v", code, out)
	} else if counts := out["counts"].(map[string]any); counts["who"] != nil || counts["source"] != nil || counts["tool"] != nil {
		t.Errorf("a tester is counted who: %v", counts)
	}
	_, _, out := atOp(r.page, "audit_log", `{"preset":"items_changed"}`)
	if out["total"] != float64(1) {
		t.Errorf("items changed: %v", out)
	}
	if e := out["entries"].([]any)[0].(map[string]any); e["username"] != "staff" || e["source"] != nil || e["tool"] != nil {
		t.Errorf("another's line: %v", e)
	}
	for _, raw := range []string{`{"who":"gm"}`, `{"source":"panel"}`, `{"tool":"x"}`, `{"petition":1446}`, `{"camp_report":-1}`,
		`{"preset":"petitions"}`, `{"table":"spellpattern,npcs"}`, `{"username":"gm"}`, `{"search":"x"}`} {
		if code, _, out := atOp(r.page, "audit_log", raw); code != 403 {
			t.Errorf("%s: %d %v", raw, code, out)
		}
	}
}

// On a copy of a real server's log (EQOA_AUDIT_DB names its database), the
// first read indexes every line, and then each filter a GM uses answers
// fast. Skipped without one: none is in this repository.
func TestTheAuditLogOnARealServersLog(t *testing.T) {
	src := os.Getenv("EQOA_AUDIT_DB")
	if src == "" {
		t.Skip("EQOA_AUDIT_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "real.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	outf, _ := os.Create(path)
	io.Copy(outf, in)
	in.Close()
	outf.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.Server.DBPath = path
	cfg.GM.Port = 1
	s, err := New(Deps{Config: cfg, DB: db})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := s.indexAudit(); err != nil {
		t.Fatal(err)
	}
	var lines, facets int64
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit`).Scan(&lines)
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit_facets`).Scan(&facets)
	t.Logf("indexed %d lines in %v", facets, time.Since(start))
	if lines != facets {
		t.Errorf("%d lines, %d indexed", lines, facets)
	}
	start = time.Now()
	s.indexAudit()
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("nothing new took %v", d)
	}
	var npc int64
	db.QueryRow(`SELECT rid FROM gm_audit_facets WHERE tbl = 'npcs' AND kind = 'deleted' ORDER BY id DESC LIMIT 1`).Scan(&npc)
	for _, f := range []auditFilter{
		{},
		{Counts: true},
		{Preset: "npcs_deleted", Counts: true},
		{Preset: "items_added", Counts: true},
		{Preset: "items_changed"},
		{Preset: "spells_changed", Counts: true},
		{Preset: "camp_reports", Counts: true},
		{Preset: "today", Counts: true},
		{Name: "bat", Counts: true},
		{Who: "claude", Counts: true},
		{Source: "tool", Tool: "ct_trials", Counts: true},
		{Username: "jibrish", Group: "loot", Action: "changed", Counts: true},
		{RowID: &npc, Counts: true},
		{Petition: 1446, Counts: true},
		{Since: "2026-11-01", Until: "2026-11-05", Group: "npcs", Counts: true},
		{Action: "reverted"},
		{Search: "Jibrish"},
		{Search: "Jibrish", Counts: true},
		{Offset: 30000},
	} {
		f.hideSQL = true
		start := time.Now()
		list, total, _, err := s.auditQuery(f)
		d := time.Since(start)
		if err != nil {
			t.Fatalf("%+v: %v", f, err)
		}
		t.Logf("%v total %d, %d on the page, %+v", d.Round(time.Millisecond), total, len(list), f)
		limit := 400 * time.Millisecond
		if f.Search != "" {
			limit = 3 * time.Second // the free text reads the lines themselves
		}
		if d > limit {
			t.Errorf("%+v took %v", f, d)
		}
		if len(list) > 0 && f.Preset == "npcs_deleted" && list[0].Thing == "" {
			t.Errorf("an NPC deleted with no name: %+v", list[0])
		}
	}
	var unnamed, rows int64
	db.QueryRow(`SELECT COUNT(*), SUM(thing = '') FROM gm_audit_facets WHERE tbl != '' AND action != 'sql'`).Scan(&rows, &unnamed)
	t.Logf("%d of %d row lines have no name", unnamed, rows)
	rs, _ := db.Query(`SELECT tbl, COUNT(*) FROM gm_audit_facets WHERE tbl != '' AND thing = '' GROUP BY 1 ORDER BY 2 DESC LIMIT 15`)
	for rs.Next() {
		var tbl string
		var n int64
		rs.Scan(&tbl, &n)
		t.Logf("  unnamed: %s %d", tbl, n)
	}
	rs.Close()
}

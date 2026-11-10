package gmweb

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
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

// campGround is terrain at 39.5 everywhere: Floor the highest floor no more
// than the ground's step (2) above the height asked, as internal/ground's.
type campGround struct{}

func (campGround) Floor(_ uint8, _, y, _ float32) (float32, bool) {
	if 39.5 <= y+2 {
		return 39.5, true
	}
	return y, false
}

// campfixFixture is the panel's fixture with a bandit camp in Tunaria round
// (21000, 40, 12000), on ground at 39.5: bandits 801-805, 803 and 805 spawns
// of camp 7, 805 walking a path; the world loaded with them.
func campfixFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO npcs (id, npc_name, world, zone, x_coord, y_coord, z_coord, facing, modelid, size, npc_level,
		npc_type, race, torso, primary_weapon) VALUES
		(801, 'bandit', 0, 3, 21000, 40, 12000, 64, 4660, 1.0, 20, 0, '0', 3, 77),
		(802, 'bandit', 0, 3, 21000, 40, 12060, 192, 4660, 1.0, 20, 0, '0', 3, 77),
		(803, 'bandit', 0, 3, 21060, 40, 12000, 0, 4661, 1.1, 21, 0, '0', 4, 78),
		(804, 'bandit', 0, 3, 21200, 40, 12200, 0, 4660, 1.0, 20, 0, '0', 3, 77),
		(805, 'bandit', 0, 3, 21030, 40, 12030, 0, 4660, 1.0, 20, 0, '0', 3, 77)`)
	mustExec(t, f.db, `INSERT INTO campspawns (npcid, camp, kinds) VALUES (803, 7, 'bandit'), (805, 7, 'bandit')`)
	mustExec(t, f.db, `INSERT INTO npcpaths (npcid, world, mode, pause, speed, points)
		VALUES (805, 0, 'loop', 0, 'walk', '[{"x":21030,"y":40,"z":12030},{"x":21040,"y":40,"z":12030}]')`)
	f.world.LoadNPCs(database.NewNPCStore(f.db))
	if _, err := f.world.LoadCampSpawns(f.db); err != nil {
		t.Fatal(err)
	}
	f.world.SetGround(campGround{})
	return f
}

// campfixMarksJSON is a report's five marks on the camp: move 801 10 east,
// add a bandit by 803 (a camp spawn), remove 805, turn 802, move 804 10 west.
const campfixMarksJSON = `[` +
	`{"kind":"move","typed":"bandit","name":"bandit","world":0,"x":21010,"y":40,"z":12000,"facing":0.25,"facing_byte":10,"row":801,` +
	`"from_x":21000,"from_y":40,"from_z":12000,"from_facing":64,"level":20,"distance":10,"at":"2026-01-01T00:00:00Z"},` +
	`{"kind":"add","typed":"bandit","name":"bandit","world":0,"x":21062,"y":40,"z":12010,"facing":0.5,"facing_byte":20,"row":0,` +
	`"template":803,"from_x":21060,"from_y":40,"from_z":12000,"from_facing":0,"level":21,"distance":10.2,"at":"2026-01-01T00:00:00Z"},` +
	`{"kind":"remove","typed":"","name":"bandit","world":0,"x":21031,"y":40,"z":12031,"facing":0,"facing_byte":128,"row":805,` +
	`"from_x":21030,"from_y":40,"from_z":12030,"from_facing":0,"level":20,"distance":1.41,"at":"2026-01-01T00:00:00Z"},` +
	`{"kind":"turn","typed":"bandit","name":"bandit","world":0,"x":21000.5,"y":40,"z":12060,"facing":0.75,"facing_byte":30,"row":802,` +
	`"from_x":21000,"from_y":40,"from_z":12060,"from_facing":192,"level":20,"distance":0.5,"at":"2026-01-01T00:00:00Z"},` +
	`{"kind":"move","typed":"bandit","name":"bandit","world":0,"x":21190,"y":40,"z":12200,"facing":1,"facing_byte":40,"row":804,` +
	`"from_x":21200,"from_y":40,"from_z":12200,"from_facing":0,"level":20,"distance":10,"at":"2026-01-01T00:00:00Z"}]`

// campfixFile files a waiting report of marks (JSON) from the player Hero
// (account 3), sent at the start of 2026, with its work board entry.
func campfixFile(t *testing.T, f *fixture, marks string) database.CampfixReport {
	t.Helper()
	title, details, status, owner := "Camp fix report: Hero", "the marks", database.WorkPlanned, handlers.CampfixBoardOwner
	r, _, err := f.db.FileCampfixReport(database.CampfixReport{AccountID: 3, CharName: "Hero", Level: 10, World: 0, X: 21010, Z: 12000,
		Note: "the bandits stand wrong", Marks: marks}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		func(id int64) database.WorkPost {
			return database.WorkPost{Title: &title, Details: &details, Status: &status, Owner: &owner}
		}, database.GMEdit{AccountID: 3, Via: "campfix"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// campfixCamp is the camp's rows as they stand: npcs (but zone), campspawns,
// npcpaths, one line each.
func campfixCamp(t *testing.T, f *fixture) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, torso, primary_weapon, zone FROM npcs ORDER BY id`,
		`SELECT npcid, camp, kinds FROM campspawns ORDER BY npcid`,
		`SELECT npcid, points FROM npcpaths ORDER BY npcid`} {
		rows, err := f.s.rowsOf(q)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			fmt.Fprintf(&b, "%v\n", r)
		}
	}
	return b.String()
}

func campfixResults(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	list, _ := out["results"].([]any)
	var res []map[string]any
	for _, r := range list {
		res = append(res, r.(map[string]any))
	}
	return res
}

// Approve applies the checked marks of every kind, each an audited change by
// the approving GM the world follows at once and revert_change undoes: a move
// sets its row's place and facing (the height on the ground's floor), an add
// copies the nearest row of its name (body, level, gear) with its camp spawn
// row, a removal deletes the row with its path and camp spawn row, a turn the
// facing alone. The unchecked mark is not applied (the control); the report is
// applied, its review saying which marks went, and its board entry closed; a
// decided report is not decided again.
func TestCampfixApproveAppliesEachKind(t *testing.T) {
	f := campfixFixture(t)
	r := campfixFile(t, f, campfixMarksJSON)
	gm := f.browser()
	gm.login("gm")

	code, out := gm.op("campfix_list", map[string]any{})
	if code != 200 || out["matching"] != float64(1) {
		t.Fatalf("list: %d %v", code, out)
	}
	rep := out["reports"].([]any)[0].(map[string]any)
	marks := rep["marks"].([]any)
	if len(marks) != 5 {
		t.Fatalf("marks: %v", marks)
	}
	for i, want := range []string{"green", "green", "yellow", "green", "green"} {
		m := marks[i].(map[string]any)
		if m["colour"] != want || m["checked"] != true || m["stale"] != nil {
			t.Errorf("mark %d: %v %v %v %v, want %s and checked", i+1, m["colour"], m["checked"], m["stale"], m["reasons"], want)
		}
	}
	if p := marks[0].(map[string]any)["plan"].(map[string]any); p["y"] != 39.5 || p["row"] != float64(801) || p["action"] != "update" {
		t.Fatalf("the move's plan: %v", p)
	}
	if p := marks[1].(map[string]any)["plan"].(map[string]any); p["template"] != float64(803) || p["camp"] != float64(7) {
		t.Fatalf("the add's plan: %v", p)
	}

	code, out = gm.op("campfix_approve", map[string]any{"id": r.ID, "marks": []int{1, 2, 3, 4}, "note": "checked against the camp"})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, out)
	}
	res := campfixResults(t, out)
	for i, want := range []string{"applied", "applied", "applied", "applied", "declined"} {
		if res[i]["result"] != want {
			t.Errorf("mark %d: %v, want %s", i+1, res[i], want)
		}
	}
	var newID int64
	if err := f.db.QueryRow(`SELECT id FROM npcs WHERE x_coord = 21062 AND z_coord = 12010`).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	type row struct {
		x, y, z       float64
		facing, model int
		level, torso  int
		primary, zone int
		size          float64
		name          string
	}
	read := func(id int64) (row, bool) {
		var r row
		err := f.db.QueryRow(`SELECT x_coord, y_coord, z_coord, facing, modelid, npc_level, torso, primary_weapon, zone, size, npc_name
			FROM npcs WHERE id = ?`, id).Scan(&r.x, &r.y, &r.z, &r.facing, &r.model, &r.level, &r.torso, &r.primary, &r.zone, &r.size, &r.name)
		return r, err == nil
	}
	if got, _ := read(801); got.x != 21010 || got.y != 39.5 || got.z != 12000 || got.facing != 10 || got.zone != 3 {
		t.Errorf("the move: %+v", got)
	}
	if got, _ := read(newID); got.x != 21062 || got.y != 39.5 || got.facing != 20 || got.model != 4661 || got.level != 21 ||
		got.torso != 4 || got.primary != 78 || got.size != 1.1 || got.name != "bandit" {
		t.Errorf("the add: %+v", got)
	}
	if _, ok := read(805); ok {
		t.Error("the removed row stands")
	}
	if got, _ := read(802); got.x != 21000 || got.z != 12060 || got.y != 40 || got.facing != 30 {
		t.Errorf("the turn: %+v", got)
	}
	if got, _ := read(804); got.x != 21200 || got.facing != 0 {
		t.Errorf("the unchecked mark was applied: %+v", got)
	}
	var camps, paths int
	f.db.QueryRow(`SELECT COUNT(*) FROM campspawns WHERE npcid = ? AND camp = 7 AND kinds = 'bandit'`, newID).Scan(&camps)
	f.db.QueryRow(`SELECT COUNT(*) FROM campspawns WHERE npcid = 805`).Scan(&paths)
	if camps != 1 || paths != 0 {
		t.Errorf("camp spawn rows: the add's %d, the removal's %d", camps, paths)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM npcpaths WHERE npcid = 805`).Scan(&paths)
	if paths != 0 {
		t.Error("the removed row's path stands")
	}
	// the world followed
	if st, ok := f.world.NPCStateOf(801); !ok || st.SpawnX != 21010 || st.SpawnY != 39.5 {
		t.Errorf("the world's 801: %+v %v", st, ok)
	}
	if st, ok := f.world.NPCStateOf(entity.EntityID(newID)); !ok || st.Name != "bandit" || st.SpawnX != 21062 || st.Level != 21 {
		t.Errorf("the world's new bandit: %+v %v", st, ok)
	}
	if _, ok := f.world.NPCStateOf(805); ok {
		t.Error("805 is still in the world")
	}
	if st, _ := f.world.NPCStateOf(802); math.Abs(float64(st.Facing)-30.0/256*2*math.Pi) > 1e-5 {
		t.Errorf("the world's 802 faces %v", st.Facing)
	}
	if !strings.Contains(f.world.CampSummary(), "2 camp spawns") {
		t.Errorf("the camps: %s", f.world.CampSummary())
	}
	// every change the GM's, audited
	var lines int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE accountid = 2 AND username = 'gm' AND detail LIKE ?`,
		fmt.Sprintf("camp report %d, mark %%", r.ID)).Scan(&lines)
	if lines != 7 {
		t.Errorf("%d audit lines, want 7 (move 1, add 2, remove 3, turn 1)", lines)
	}
	// the report and its board entry
	got, _, _ := f.db.CampfixReportByID(r.ID)
	if got.Status != "applied" || !strings.HasPrefix(got.Review, "Approved by gm on ") ||
		!strings.Contains(got.Review, "marks 1, 2, 3 and 4 of 5 applied; mark 5 declined.") || !strings.HasSuffix(got.Review, "\nNote: checked against the camp") {
		t.Errorf("the report: %+v", got)
	}
	e, _, _ := f.db.WorkEntryByID(r.Board)
	if e.Status != database.WorkDone || !strings.Contains(e.Result, "marks 1, 2, 3 and 4 of 5 applied") || !strings.Contains(e.Notes, "Mark 2, add bandit") {
		t.Errorf("the board entry: %+v", e)
	}
	if code, out := gm.op("campfix_approve", map[string]any{"id": r.ID, "marks": []int{5}}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "is applied already") {
		t.Fatalf("approved twice: %d %v", code, out)
	}
	// revert_change undoes the move, and the world follows
	audit := res[0]["changes"].([]any)[0].(map[string]any)["audit"]
	if code, out := gm.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if got, _ := read(801); got.x != 21000 || got.y != 40 || got.facing != 64 {
		t.Errorf("after the revert: %+v", got)
	}
	if st, _ := f.world.NPCStateOf(801); st.SpawnX != 21000 {
		t.Errorf("the world after the revert: %+v", st)
	}
}

// A mark whose row moved since the report was sent is skipped as stale and
// said so; a report whose checked marks are all stale is refused and still
// waits. Decline changes no row. Only a GM decides: a tester and a read-only
// token are refused, and change nothing.
func TestCampfixStaleDeclineAndWho(t *testing.T) {
	f := campfixFixture(t)
	gm := f.browser()
	gm.login("gm")
	r := campfixFile(t, f, campfixMarksJSON)
	if code, out := gm.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 804},
		"values": map[string]any{"npc_level": 22}}); code != 200 {
		t.Fatalf("edit 804: %d %v", code, out)
	}
	code, out := gm.op("campfix_get", map[string]any{"id": r.ID})
	if code != 200 {
		t.Fatalf("get: %d %v", code, out)
	}
	m5 := out["report"].(map[string]any)["marks"].([]any)[4].(map[string]any)
	if !strings.Contains(fmt.Sprint(m5["stale"]), "row 804 was changed since the report was sent: update by gm") || m5["checked"] != false {
		t.Fatalf("mark 5: %v", m5)
	}
	before := campfixCamp(t, f)
	if code, out := gm.op("campfix_approve", map[string]any{"id": r.ID, "marks": []int{5}}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "nothing to apply: every checked mark is stale") {
		t.Fatalf("all stale: %d %v", code, out)
	}
	if got, _, _ := f.db.CampfixReportByID(r.ID); got.Status != "waiting" || campfixCamp(t, f) != before {
		t.Fatalf("the refused approval changed something: %+v", got)
	}
	code, out = gm.op("campfix_approve", map[string]any{"id": r.ID, "marks": []int{4, 5}})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, out)
	}
	if res := campfixResults(t, out); res[4]["result"] != "stale" || res[3]["result"] != "applied" {
		t.Fatalf("results: %v", res)
	}
	if got, _, _ := f.db.CampfixReportByID(r.ID); !strings.Contains(got.Review, "mark 4 of 5 applied; marks 1, 2 and 3 declined; mark 5 skipped as stale") ||
		!strings.Contains(got.Review, "- Mark 5, move bandit: skipped as stale: row 804 was changed") {
		t.Fatalf("the review: %q", got.Review)
	}

	// decline: no row changes
	r2 := campfixFile(t, f, campfixMarksJSON)
	before = campfixCamp(t, f)
	code, out = gm.op("campfix_decline", map[string]any{"id": r2.ID, "note": "a joke", "abuse": true})
	if code != 200 {
		t.Fatalf("decline: %d %v", code, out)
	}
	if campfixCamp(t, f) != before {
		t.Fatal("the decline changed a row")
	}
	got, _, _ := f.db.CampfixReportByID(r2.ID)
	e, _, _ := f.db.WorkEntryByID(r2.Board)
	if got.Status != "declined" || !strings.HasPrefix(got.Review, handlers.CampfixDeclinedAbuse+" by gm on ") || e.Status != database.WorkDone {
		t.Fatalf("after the decline: %+v %+v", got, e)
	}

	// who: a tester and a read-only token are refused; the control, a GM's write token
	r3 := campfixFile(t, f, campfixMarksJSON)
	f.newTester()
	tess := f.browser()
	tess.login("tess")
	read := f.bearer(gm.workToken("r", "read"))
	before = f.digest()
	for who, c := range map[string]*client{"tester": tess, "read token": read} {
		for _, op := range []string{"campfix_approve", "campfix_decline"} {
			if code, out := c.op(op, map[string]any{"id": r3.ID, "marks": []int{1}}); code != 403 {
				t.Errorf("%s %s: %d %v", who, op, code, out)
			}
		}
		if code, out := c.op("campfix_from_petition", map[string]any{"petition": 1001, "kind": "add", "name": "bandit"}); code != 403 {
			t.Errorf("%s campfix_from_petition: %d %v", who, code, out)
		}
	}
	if f.digest() != before {
		t.Fatal("a refused call changed the database")
	}
	write := f.bearer(gm.workToken("w", "write"))
	if code, out := write.op("campfix_decline", map[string]any{"id": r3.ID}); code != 200 {
		t.Fatalf("the control, a GM's write token: %d %v", code, out)
	}
}

// The MCP tool is the page's operation: the same report approved through each
// leaves the same rows, and the world the same NPCs.
func TestCampfixApproveThroughMCPIsThePages(t *testing.T) {
	var camps [2]string
	var states [2]string
	for i, route := range []string{"page", "mcp"} {
		f := campfixFixture(t)
		r := campfixFile(t, f, campfixMarksJSON)
		gm := f.browser()
		gm.login("gm")
		args := map[string]any{"id": r.ID, "marks": []int{1, 2, 3, 4}}
		if route == "page" {
			if code, out := gm.op("campfix_approve", args); code != 200 {
				t.Fatalf("page: %d %v", code, out)
			}
		} else {
			agent := f.bearer(gm.workToken("agent", "write"))
			raw, _ := json.Marshal(args)
			if isErr, text := atMCP(agent, "campfix_approve", string(raw)); isErr || !strings.Contains(text, "of 5 applied") {
				t.Fatalf("mcp: %v %s", isErr, text)
			}
		}
		camps[i] = campfixCamp(t, f)
		var st []string
		for _, n := range f.world.NPCStates(0) {
			st = append(st, fmt.Sprintf("%d %s %.2f %.2f %.2f %.4f", n.ID, n.Name, n.SpawnX, n.SpawnY, n.SpawnZ, n.Facing))
		}
		states[i] = strings.Join(st, "\n")
		if got, _, _ := f.db.CampfixReportByID(r.ID); got.Status != "applied" {
			t.Fatalf("%s: %+v", route, got)
		}
	}
	if camps[0] != camps[1] {
		t.Fatalf("the rows differ:\npage:\n%s\nmcp:\n%s", camps[0], camps[1])
	}
	if states[0] != states[1] {
		t.Fatalf("the world differs:\npage:\n%s\nmcp:\n%s", states[0], states[1])
	}
}

// A petition's spawn request filed from the panel becomes a waiting report at
// the petition's place (nothing spawns); approved, its adds stand there. A
// dry run files nothing, and a petition with no server view is refused.
func TestCampfixFromPetitionOp(t *testing.T) {
	f := campfixFixture(t)
	gm := f.browser()
	gm.login("gm")
	p, err := f.db.FilePetition(database.Petition{ServerID: 7, CharName: "Hero", AccountID: 3, Category: 1, Subcategory: 10,
		Text: "bandit camp not populated", ServerView: `{"filed":"2026-11-09T10:00:00Z","via":"chat","character":{"name":"Hero",` +
			`"serverid":7,"level":10,"class":"Warrior","race":"Human","world":0,"x":21100,"y":40,"z":12100,"facing":3.1416}}`}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	npcsBefore := campfixCamp(t, f)
	args := map[string]any{"petition": p.ID, "kind": "add", "name": "bandi", "count": 2, "note": "bandit camp not populated", "dry_run": true}
	code, out := gm.op("campfix_from_petition", args)
	if code != 200 || out["dryRun"] != true || len(out["marks"].([]any)) != 2 {
		t.Fatalf("dry run: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM campfix_reports`).Scan(&n)
	if n != 0 {
		t.Fatal("the dry run filed a report")
	}
	delete(args, "dry_run")
	code, out = gm.op("campfix_from_petition", args)
	if code != 200 {
		t.Fatalf("file: %d %v", code, out)
	}
	rep := out["report"].(map[string]any)
	id := int64(rep["id"].(float64))
	r, _, _ := f.db.CampfixReportByID(id)
	if r.Status != "waiting" || r.AccountID != 3 || r.CharName != "Hero" || r.X != 21100 || r.Z != 12100 || rep["petition"] != float64(p.ID) ||
		r.Note != fmt.Sprintf("From petition %d: bandit camp not populated", p.ID) || campfixCamp(t, f) != npcsBefore {
		t.Fatalf("the report: %+v %v", r, rep)
	}
	if said := fmt.Sprint(out["said"]); !strings.Contains(said, "Taken as bandit.") {
		t.Errorf("said: %s", said)
	}
	e, _, _ := f.db.WorkEntryByID(r.Board)
	if e.CreatedBy != "gm" || !strings.Contains(e.Details, fmt.Sprintf("**From:** petition %d", p.ID)) {
		t.Errorf("the entry: %+v", e)
	}
	if code, out := gm.op("campfix_approve", map[string]any{"id": id, "marks": []int{1, 2}}); code != 200 {
		t.Fatalf("approve: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM npcs WHERE npc_name = 'bandit' AND y_coord = 39.5 AND ABS(x_coord - 21100) <= 3.01 AND ABS(z_coord - 12100) <= 3.01`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d bandits stand where the petition's character stood, want 2", n)
	}

	p2, _ := f.db.FilePetition(database.Petition{ServerID: 7, CharName: "Hero", AccountID: 3, Category: 1, Subcategory: 10, Text: "x"}, time.Now())
	if code, out := gm.op("campfix_from_petition", map[string]any{"petition": p2.ID, "kind": "add", "name": "bandit"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "has no server view") {
		t.Fatalf("no server view: %d %v", code, out)
	}
}

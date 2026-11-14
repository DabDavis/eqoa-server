package gmweb

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The storyteller role (storyteller.go): a player's account that signs in to
// the panel and changes NPC dialog and the quests' words, and nothing else, by
// every route; and an account with both roles (roles.go), given what either
// gives. As the tester's (tester_test.go), the loops take every registered
// operation and every table, so a new one cannot slip through; GMs, admins,
// testers and plain players are the controls.

// storytellerRefused is in what the gate says to a storyteller of what is not
// its, and in nothing else the panel says.
const storytellerRefused = "is not a storyteller's"

// bothRefused is what it says to an account with both roles of what neither
// gives.
const bothRefused = "is not a tester's or a storyteller's"

// setStoryteller is the admin's op.
func (f *fixture) setStoryteller(name string, on bool) map[string]any {
	f.t.Helper()
	boss := f.browser()
	boss.login("boss")
	code, out := boss.op("set_account_storyteller", map[string]any{"account": name, "storyteller": on, "note": "for the test"})
	if code != 200 {
		f.t.Fatalf("set_account_storyteller %s %v: %d %v", name, on, code, out)
	}
	return out
}

// storyFixture is what a storyteller works on: a quest of two steps (a trip it
// unlocks, a step's other creature, a character who has it under way) and two
// town dialog lines. The names are made up.
func (f *fixture) storyFixture() {
	f.t.Helper()
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, offer, accept, decline, declined, started, source, notes)
		VALUES (40, 'Bread run', 1, 'Guard Dahvel', 1, 60, 'The watch is hungry.
Will you fetch bread?', 'I will.', 'Not today.', 'Then go.', 'Speak to the merchant.', 'gm', 'for the test')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, answer, says, xp, rewardtunar, rewarditems, items)
		VALUES (40, 1, 'talk', 'Merchant Shadehand', 'Speak to the merchant.', 'Dahvel sent me.', 'Take this bread.', 0, 0, '', ''),
		       (40, 2, 'give', 'Guard Dahvel', 'Bring the guard the bread.', '', 'Thank you, %p.', 100, 5, '11*1', '11*1')`)
	f.exec(`INSERT INTO questunlocks (questid, npc, dest) VALUES (40, 'Waysender Test', 'Qeynos')`)
	f.exec(`INSERT INTO queststeptargets (questid, step, target) VALUES (40, 2, 'a decaying skeleton')`)
	f.exec(`INSERT INTO charquests (serverid, questid, step) VALUES (7, 40, 2)`)
	f.exec(`INSERT INTO npcdialog (id, town, role, name, line, weight) VALUES (1, 'Freeport', 'guard', '', 'Move along, %p.', 1),
		(2, '', '', 'Guard Dahvel', 'Keep the peace.', 1)`)
}

// newStoryteller is an account with the role, sage (6), and what it works on.
func (f *fixture) newStoryteller() {
	f.t.Helper()
	f.addAccount(6, "sage", 0)
	f.setStoryteller("sage", true)
	f.storyFixture()
}

// refusedIn checks every route refuses a call, in these words.
func (r testerRoutes) refusedIn(t *testing.T, words, name, raw string) {
	t.Helper()
	for route, c := range map[string]*client{"page": r.page, "openapi": r.token} {
		if code, text, _ := atOp(c, name, raw); code != 403 || !strings.Contains(text, words) {
			t.Errorf("%s %s %.200s: %d %.200s", route, name, raw, code, text)
		}
	}
	for route, c := range map[string]*client{"mcp": r.token, "oauth": r.oauth} {
		if isErr, text := atMCP(c, name, raw); !isErr || !strings.Contains(text, words) {
			t.Errorf("%s %s %.200s: isError %v %.200s", route, name, raw, isErr, text)
		}
	}
}

func (f *fixture) value(q string, args ...any) string {
	f.t.Helper()
	var v sql.NullString
	if err := f.db.QueryRow(q, args...).Scan(&v); err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
	return v.String
}

// The whole of a storyteller's way in with the live server's rule: the
// password alone at first and nothing but the enrolment; then a quest's words,
// a step's words and the town dialog changed, each an audit line under the
// storyteller's name and taken by the running game as a GM's change is (the
// quests reloaded, the journal redrawn, the dialog read again at once); and in
// the game a player still.
func TestAStorytellerEnrolsSignsInAndChangesDialogAndAQuestsWords(t *testing.T) {
	f := newFixture(t)
	f.s.cfg.RequireTOTP = true
	f.addAccount(6, "sage", 0)
	f.storyFixture()
	q := f.questEngine()
	login := map[string]any{"username": "sage", "password": testPassword}
	if code, out := f.browser().do("POST", "/api/auth/login", login); code != 401 || !strings.Contains(fmt.Sprint(out["error"]), "not a GM account") {
		t.Fatalf("before the role: %d %v", code, out)
	}
	f.setStoryteller("sage", true)

	sage := f.browser()
	code, out := sage.do("POST", "/api/auth/login", login)
	if code != 200 {
		t.Fatalf("sign-in with the role: %d %v", code, out)
	}
	sage.csrf, _ = out["csrf"].(string)
	user := out["user"].(map[string]any)
	if user["role"] != "storyteller" || user["level"] != float64(0) || user["admin"] != false || user["adminNeedsTOTP"] != false {
		t.Fatalf("what the sign-in reports: %v", user)
	}
	if code, text, _ := atOp(sage, "quest_list", `{}`); code != 403 || !strings.Contains(text, "authenticator") {
		t.Fatalf("before enrolling: %d %s", code, text)
	}
	if code, _ := sage.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "read", "days": 7}); code != 403 {
		t.Fatalf("a token before enrolling: %d", code)
	}
	code, out = sage.do("POST", "/api/auth/totp", map[string]any{"action": "begin"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, out)
	}
	secret := out["secret"].(string)
	if code, out := sage.do("POST", "/api/auth/totp", map[string]any{"action": "confirm", "code": totpNow(secret)}); code != 200 {
		t.Fatalf("confirm: %d %v", code, out)
	}
	mustExec(t, f.db, `UPDATE gm_totp SET last_step = 0 WHERE accountid = 6`)
	code, out = f.browser().do("POST", "/api/auth/token", map[string]any{"username": "sage", "password": testPassword, "code": totpNow(secret), "name": "codex"})
	if code != 200 || out["scope"] != "write" {
		t.Fatalf("a terminal's token with the code: %d %v", code, out)
	}
	if code, text, _ := atOp(f.bearer(out["token"].(string)), "quest_get", `{"quest":40}`); code != 200 {
		t.Fatalf("the terminal's token: %d %s", code, text)
	}

	// a quest's words, the engine reading them at once
	code, _, res := atOp(sage, "update_row", `{"table":"quests","key":{"questid":40},"values":{"name":"Bread for the watch","offer":"The watch is hungry, %p.\nWill you fetch bread?"},"expect":{"name":"Bread run"}}`)
	if code != 200 {
		t.Fatalf("a quest's words: %d %v", code, res)
	}
	who, via, action, tbl, before, after := auditRow(t, f, res["audit"])
	if who != "sage" || via != "web" || action != "update" || tbl != "quests" || !strings.Contains(before, `"name":"Bread run"`) ||
		!strings.Contains(after, `"name":"Bread for the watch"`) {
		t.Fatalf("the audit line: %s %s %s %s %s %s", who, via, action, tbl, before, after)
	}
	if got := q.InGame()[40]; got != "Bread for the watch" {
		t.Errorf("the engine's name of the quest: %q", got)
	}
	// a step's journal line, in the journal of a character who has it
	if _, err := q.SetCharacterQuest(106, 40, "give", 1); err != nil {
		t.Fatal(err)
	}
	if _, lines := q.Journal(106); !slices.Contains(lines, "Speak to the merchant.") {
		t.Fatalf("the journal before: %v", lines)
	}
	code, _, res = atOp(sage, "update_row", `{"table":"queststeps","key":{"questid":40,"step":1},"values":{"journal":"Find the merchant by the gate."}}`)
	if code != 200 {
		t.Fatalf("a step's words: %d %v", code, res)
	}
	if _, lines := q.Journal(106); !slices.Contains(lines, "Find the merchant by the gate.") {
		t.Errorf("the journal after: %v", lines)
	}
	// a GM's change to a step's words goes the same way
	f.enrolTOTP(2)
	gm := f.browser()
	code, out = gm.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword, "code": totpNow(testTOTPSecret)})
	if code != 200 {
		t.Fatalf("the GM's sign-in: %d %v", code, out)
	}
	gm.csrf, _ = out["csrf"].(string)
	_, _, mine := atOp(sage, "update_row", `{"table":"queststeps","key":{"questid":40,"step":2},"values":{"says":"Thank you, %p. Eat well."}}`)
	_, _, control := atOp(gm, "update_row", `{"table":"queststeps","key":{"questid":40,"step":1},"values":{"says":"Take the bread."}}`)
	if mine["live"] == nil || mine["live"] != control["live"] || !strings.Contains(fmt.Sprint(mine["live"]), "at once") {
		t.Errorf("a storyteller's change in the running game: %v; a GM's: %v", mine["live"], control["live"])
	}

	// town dialog: added, changed, copied and deleted, each said at the next talk
	if _, err := f.db.NPCDialog(); err != nil { // read once: kept for 30 s unless told
		t.Fatal(err)
	}
	code, _, res = atOp(sage, "insert_row", `{"table":"npcdialog","values":{"town":"Freeport","role":"guard","line":"Mind the gate, %p.","weight":2}}`)
	if code != 200 || !strings.Contains(fmt.Sprint(res["live"]), "at once") {
		t.Fatalf("a new line: %d %v", code, res)
	}
	lines, err := f.db.NPCDialog()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(lines, func(l database.NPCDialogLine) bool { return l.Line == "Mind the gate, %p." }) {
		t.Errorf("the dialog read after the new line: %v", lines)
	}
	added := res["key"].(map[string]any)["id"]
	for _, tc := range []struct{ name, raw string }{
		{"update_row", fmt.Sprintf(`{"table":"npcdialog","key":{"id":%v},"values":{"line":"Mind the gate.","mood":"wary"}}`, added)},
		{"clone_row", `{"table":"npcdialog","key":{"id":2},"changes":{"line":"The peace is kept."}}`},
		{"delete_row", `{"table":"npcdialog","key":{"id":1}}`},
	} {
		if code, _, res := atOp(sage, tc.name, tc.raw); code != 200 {
			t.Errorf("%s: %d %v", tc.name, code, res)
		}
	}
	if n := f.value(`SELECT COUNT(*) FROM npcdialog`); n != "3" {
		t.Errorf("%s lines after", n)
	}
	// every row change is a line under the storyteller's name, and a GM reads them
	if n := f.value(`SELECT COUNT(*) FROM gm_audit WHERE username = 'sage' AND tbl != ''`); n != "7" {
		t.Errorf("%s row changes logged under the storyteller, want 7", n)
	}
	code, _, res = atOp(gm, "audit_log", `{"username":"sage","table":"npcdialog"}`)
	if code != 200 || res["total"] != float64(4) {
		t.Fatalf("a GM's log of the storyteller: %d %v", code, res)
	}
	// in the game a player: the level the GM commands read, and the row
	if lvl, err := f.s.d.Accounts.AccessLevel(6); err != nil || lvl != 0 {
		t.Errorf("AccessLevel, what handlers.isGM reads: %d %v", lvl, err)
	}
	if f.s.characterOfGM(106) {
		t.Error("the storyteller's character counts as a GM's")
	}
	if levelOf(t, f, 6) != 0 {
		t.Errorf("acctlevel %d", levelOf(t, f, 6))
	}
}

// Nothing of the game reads the role: it is the panel's alone.
func TestNothingInTheGameReadsTheStorytellers(t *testing.T) {
	for _, dir := range []string{"../handlers", "../entity", "../session", "../network", "../protocol", "../auth", "../charmove", "../transfers", "../database", "../../cmd"} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// the sanitiser's rule is the one place outside the panel that names it
			if strings.HasSuffix(path, "database/snapshot.go") {
				return nil
			}
			// the shared accounts' copy carries the table to a shard as it is (the panel's role there too), reading
			// nothing of it
			if strings.HasSuffix(path, "database/accountmirror.go") || strings.HasSuffix(path, "database/accountmirror_test.go") {
				return nil
			}
			if bytes.Contains(src, []byte("gm_storytellers")) || bytes.Contains(src, []byte("storytellerFlag")) {
				t.Errorf("%s reads the storytellers' table", path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
	}
}

// The allowlist itself, word for word, and the quests' columns that are words
// and that are not: a change to either, or a new column of the two tables, is
// a change to this.
func TestTheStorytellersAllowlistIsTheOneMeant(t *testing.T) {
	f := newFixture(t)
	want := []string{"audit_log", "clone_row", "count_rows", "delete_row", "describe_table", "get_row", "guide", "insert_row",
		"list_tables", "look_names", "lookup", "query_rows", "quest_get", "quest_list", "revert_change", "server_status", "update_row"}
	var got []string
	for name := range storytellerOps {
		got = append(got, name)
		op, ok := f.s.ops[name]
		if !ok {
			t.Errorf("%s is not an operation", name)
			continue
		}
		if op.Scope == scopeAdmin {
			t.Errorf("%s is an admin's", name)
		}
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("the allowlist:\n got %v\nwant %v", got, want)
	}
	if !slices.Equal(storytellerTableList, []string{"npcdialog", "quests", "queststeps", "questunlocks", "queststeptargets", "npcs"}) {
		t.Errorf("the tables: %v", storytellerTableList)
	}
	var writes []string
	for _, name := range storytellerTableList {
		if storytellerWrites(name) {
			writes = append(writes, name)
		}
	}
	if !slices.Equal(writes, []string{"npcdialog", "quests", "queststeps"}) || storytellerWrites("itempattern") || storytellerWrites("") {
		t.Errorf("the tables it writes: %v", writes)
	}
	// the words, and the text columns that are not: what a quest is, and the
	// importer's key
	notWords := map[string][]string{
		"quests":     {"classes", "events", "factionreq", "giver", "races", "requires", "source"},
		"queststeps": {"byclass", "choices", "events", "factions", "items", "kind", "rewarditems", "spells", "target"},
	}
	wantWords := map[string][]string{
		"quests":     {"name", "offer", "accept", "decline", "declined", "started", "notes"},
		"queststeps": {"journal", "answer", "says", "waiting", "notyet", "reminder"},
	}
	if len(storytellerWords) != 2 {
		t.Errorf("the words: %v", storytellerWords)
	}
	for table, words := range wantWords {
		if !slices.Equal(storytellerWords[table], words) {
			t.Errorf("%s's words: %v", table, storytellerWords[table])
		}
		tb, err := f.s.table(table)
		if err != nil {
			t.Fatal(err)
		}
		var text []string
		for _, c := range tb.Columns {
			if c.Affinity == "TEXT" && !slices.Contains(words, c.Name) {
				text = append(text, c.Name)
			}
		}
		sort.Strings(text)
		if !slices.Equal(text, notWords[table]) {
			t.Errorf("%s's text columns that are no words: %v, want %v (a new one is a decision: a word, or not)", table, text, notWords[table])
		}
		for _, w := range words {
			c := tb.col(w)
			if c == nil || c.Affinity != "TEXT" || c.PK > 0 || c.ReadOnly {
				t.Errorf("%s.%s: %+v", table, w, c)
			}
		}
	}
	for _, name := range storytellerTableList {
		tb, err := f.s.table(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if tb.Write != writeGM || tb.Hidden || catalog[name].Label == "" {
			t.Errorf("%s: write %q, hidden %v, in the catalog %v", name, tb.Write, tb.Hidden, catalog[name].Label != "")
		}
		for _, c := range tb.Columns {
			if c.Secret {
				t.Errorf("%s.%s is a secret", name, c.Name)
			}
			// the page looks up the name a reference points at
			if c.Ref != "" && !isStorytellerTable(c.Ref) {
				t.Errorf("%s.%s refers to %s, which a storyteller cannot read", name, c.Name, c.Ref)
			}
		}
	}
	dialog, _ := f.s.table("npcdialog")
	for col := range storytellerNumbers {
		if c := dialog.col(col); c == nil || c.Affinity != "INTEGER" {
			t.Errorf("npcdialog.%s: %+v", col, c)
		}
	}
	// an admin's operation is never a storyteller's, whatever the list comes to say
	storytellerOps["sql_query"] = nil
	defer delete(storytellerOps, "sql_query")
	if storytellerMayCall(f.s.ops["sql_query"]) {
		t.Error("an admin's operation on the list is a storyteller's")
	}
	c := &opCtx{s: f.s, a: &actor{AccountID: 6, Username: "sage", Role: roleStoryteller, Scope: scopeWrite}, raw: json.RawMessage(`{"sql":"SELECT 1"}`)}
	if err := c.roleGate(f.s.ops["sql_query"]); err == nil {
		t.Error("the gate let an admin's operation through")
	}
	// and the roles, as a role string names them
	if gs := grantsOf("tester+storyteller"); len(gs) != 2 || gs[0] != testerGrant || gs[1] != storytellerGrant {
		t.Errorf("both: %v", gs)
	}
	for _, role := range []string{"tester storyteller", "storyteller,tester", "storyteller+storyteller+tester"} {
		if len(grantsOf(role)) != 2 {
			t.Errorf("%q: %v", role, grantsOf(role))
		}
	}
	for _, role := range []string{"", "gm", "admin", "Storyteller", "storytellers", "tester-storyteller"} {
		if len(grantsOf(role)) != 0 && role != "tester-storyteller" {
			t.Errorf("%q names %v", role, grantsOf(role))
		}
	}
}

// Every registered operation not on the list, by every route, with a new one
// added now; nothing changes and nothing runs. A GM and an admin are not
// stopped by the gate (they get the operation's own answer).
func TestAStorytellerIsRefusedEveryOtherOperation(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newStoryteller()
	r := f.routesOf("sage")
	ran := 0
	f.s.addOp(&Op{Name: "zz_brand_new", Title: "New", Description: "added after the list was written", Scope: scopeRead,
		Handler: func(c *opCtx) (any, error) { ran++; return map[string]any{"ran": true}, nil }})
	f.s.addOp(&Op{Name: "zz_brand_new_write", Title: "New", Description: "added after the list was written", Scope: scopeWrite,
		Handler: func(c *opCtx) (any, error) { ran++; return map[string]any{"ran": true}, nil }})
	before := f.digest()
	refused := 0
	for _, op := range f.s.opList {
		if storytellerMayCall(op) {
			continue
		}
		refused++
		r.refusedIn(t, storytellerRefused, op.Name, `{}`)
	}
	// the ones plainly a GM's, with arguments that would do it
	for name, raw := range map[string]string{
		"quest_save":              `{"quest":{"questid":40,"name":"Bread run","minlevel":50},"steps":[{"kind":"talk","target":"Guard Dahvel"}]}`,
		"quest_delete":            `{"quest":40}`,
		"quest_set_enabled":       `{"quests":[40],"enabled":false}`,
		"quest_try":               `{"quest":40}`,
		"character_quest_set":     `{"character":"Hero","quest":40,"action":"complete"}`,
		"character_quests":        `{"character":"Hero"}`,
		"give_item":               `{"character":"sageHero","item":10}`,
		"set_character":           `{"character":"sageHero","level":60}`,
		"search":                  `{"query":"Guard"}`,
		"fetch":                   `{"id":"npc:500"}`,
		"online_players":          `{}`,
		"set_account_storyteller": `{"account":"player","storyteller":true}`,
		"set_account_tester":      `{"account":"sage","tester":true}`,
		"sql_query":               `{"sql":"SELECT * FROM accountinfo"}`,
		"update_rows":             `{"table":"npcdialog","filters":[{"column":"id","value":1}],"values":{"line":"x"}}`,
		"delete_rows":             `{"table":"npcdialog","search":"Move"}`,
	} {
		r.refusedIn(t, storytellerRefused, name, raw)
	}
	if refused < 60 {
		t.Errorf("only %d operations refused: the loop lost them", refused)
	}
	if ran != 0 {
		t.Errorf("a new operation ran %d times", ran)
	}
	if f.digest() != before {
		t.Error("a refused call changed the database")
	}
	// the controls
	gm := f.browser()
	gm.login("gm")
	boss := f.browser()
	boss.login("boss")
	for _, op := range f.s.opList {
		for who, c := range map[string]*client{"gm": gm, "boss": boss} {
			if _, text, _ := atOp(c, op.Name, `{}`); strings.Contains(text, storytellerRefused) {
				t.Errorf("the control %s, %s: %s", who, op.Name, text)
			}
		}
	}
	if ran == 0 {
		t.Error("the control: a GM's new operation did not run")
	}
}

// storytellerWriteOps are the operations that write a row a table names.
var storytellerWriteOps = map[string]bool{"insert_row": true, "clone_row": true, "update_row": true, "delete_row": true}

// Every operation that takes a table, with every table there is and one made
// now, by every route: refused but for a storyteller's own; of those, a write
// to one it reads alone refused, and a quest's or a step's row added, copied or
// deleted refused.
func TestAStorytellerIsRefusedEveryOtherTable(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newStoryteller()
	r := f.routesOf("sage")
	mustExec(t, f.db, `CREATE TABLE zz_brand_new (id INTEGER PRIMARY KEY, secret_plans TEXT); INSERT INTO zz_brand_new VALUES (1, 'x')`)
	f.s.schema.invalidate()
	var tableOps []*Op
	for _, op := range f.s.opList {
		if props, _ := op.Input["properties"].(map[string]any); props["table"] != nil {
			tableOps = append(tableOps, op)
		}
	}
	if len(tableOps) < 12 {
		t.Fatalf("%d operations take a table", len(tableOps))
	}
	all, err := f.s.tables()
	if err != nil {
		t.Fatal(err)
	}
	if all["zz_brand_new"] == nil || all["gm_storytellers"] == nil || all["itempattern"] == nil {
		t.Fatal("the table list lost some")
	}
	before := f.digest()
	checked, own := 0, 0
	for name := range all {
		for _, op := range tableOps {
			raw := fmt.Sprintf(`{"table":%q,"key":{"id":1},"values":{"id":1},"keys":[1],"filters":[{"column":"id","value":1}],"dry_run":true}`, name)
			switch {
			case !isStorytellerTable(name) || !storytellerMayCall(op), storytellerWriteOps[op.Name] && !storytellerWrites(name):
				r.refusedIn(t, storytellerRefused, op.Name, raw)
				checked++
			case storytellerWriteOps[op.Name] && storytellerWords[name] != nil:
				// a quest's or a step's: no row added, copied or deleted, and
				// id is no word of theirs
				for _, c := range []*client{r.page, r.token} {
					if code, text, _ := atOp(c, op.Name, raw); code != 403 {
						t.Errorf("%s %s: %d %s", op.Name, name, code, text)
					}
				}
			default:
				// its own: past the gate (the row need not exist)
				own++
				for _, c := range []*client{r.page, r.token} {
					if code, text, _ := atOp(c, op.Name, raw); code == 403 || strings.Contains(text, storytellerRefused) {
						t.Errorf("%s %s: %d %s", op.Name, name, code, text)
					}
				}
			}
		}
	}
	if checked < (len(all)-len(storytellerTableList))*len(tableOps) || own < 30 {
		t.Errorf("%d refusals and %d of its own, of %d tables by %d operations: the loop lost them", checked, own, len(all), len(tableOps))
	}
	if f.digest() != before {
		t.Error("a refused call, or a dry run, changed the database")
	}
	// a key spelled otherwise, or twice: the handler reads what the gate read
	for _, raw := range []string{
		`{"TABLE":"accountinfo"}`, `{"Table":"accountinfo"}`, `{"tAbLe":"accountinfo"}`,
		`{"table":"npcdialog","TABLE":"accountinfo"}`, `{"table":"quests","table":"accountinfo"}`,
		`{"TABLE":"npcdialog","table":"accountinfo"}`, `{"table":"npcs","table":null,"Table":"accountinfo"}`,
		`null`, `{}`, `{"table":""}`, `{"table":"NPCDIALOG"}`, `{"table":"npcdialog\u0000"}`, `{"table":"quests "}`,
		`{"table":"npcdialog; DROP TABLE accountinfo"}`,
	} {
		for _, name := range []string{"query_rows", "count_rows", "describe_table", "get_row", "lookup"} {
			r.refusedIn(t, storytellerRefused, name, raw)
		}
	}
	code, _, out := atOp(r.page, "query_rows", `{"table":"accountinfo","TABLE":"npcdialog"}`)
	if code != 200 || out["table"] != "npcdialog" {
		t.Errorf("the last spelling: %d %v", code, out)
	}
	// the values spelled otherwise: the handler writes what the gate read
	for _, raw := range []string{
		`{"table":"quests","key":{"questid":40},"VALUES":{"minlevel":50}}`,
		`{"table":"quests","key":{"questid":40},"values":{"offer":"x"},"Values":{"minlevel":50}}`,
		`{"table":"quests","key":{"questid":40},"values":{"minlevel":50,"minlevel":1}}`,
		`{"table":"quests","key":{"questid":40},"values":{"MINLEVEL":50}}`,
	} {
		if code, text, _ := atOp(r.page, "update_row", raw); code != 403 {
			t.Errorf("%s: %d %s", raw, code, text)
		}
	}
	if f.value(`SELECT minlevel FROM quests WHERE questid = 40`) != "1" {
		t.Error("a spelling wrote the quest's level")
	}
	// the controls: a GM reads the tables a storyteller does not
	gm := f.browser()
	gm.login("gm")
	if code, _, out := atOp(gm, "query_rows", `{"table":"characters"}`); code != 200 || out["total"] == float64(0) {
		t.Errorf("the control, a GM: %d %v", code, out)
	}
}

// valueFor is a value a column takes, for a write the gate should refuse.
func valueFor(c Column) string {
	switch c.Affinity {
	case "INTEGER":
		return "5"
	case "REAL":
		return "0.5"
	}
	return `"words"`
}

// A quest's and a step's words alone: every other column, by every route and
// with nothing behind the gate; no row added, copied or deleted; a dry run held
// as a write; and the words written, each with what they were.
func TestAStorytellerChangesAQuestsWordsAlone(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newStoryteller()
	r := f.routesOf("sage")
	keys := map[string]string{"quests": `{"questid":40}`, "queststeps": `{"questid":40,"step":2}`}
	before := f.digest()
	for _, table := range []string{"quests", "queststeps"} {
		tb, err := f.s.table(table)
		if err != nil {
			t.Fatal(err)
		}
		others := 0
		for _, c := range tb.Columns {
			if slices.Contains(storytellerWords[table], c.Name) {
				continue
			}
			others++
			r.refusedIn(t, storytellerRefused, "update_row", fmt.Sprintf(`{"table":%q,"key":%s,"values":{%q:%s}}`, table, keys[table], c.Name, valueFor(c)))
			// with a word beside it, and as a dry run: refused whole
			r.refusedIn(t, storytellerRefused, "update_row", fmt.Sprintf(`{"table":%q,"key":%s,"values":{%q:"new words",%q:%s},"dry_run":true}`,
				table, keys[table], storytellerWords[table][0], c.Name, valueFor(c)))
		}
		if others < 7 {
			t.Errorf("%s: only %d columns that are no word", table, others)
		}
		// no row of it added, copied or deleted
		for name, raw := range map[string]string{
			"insert_row": fmt.Sprintf(`{"table":%q,"values":{"questid":41,"name":"New","step":1}}`, table),
			"clone_row":  fmt.Sprintf(`{"table":%q,"key":%s,"changes":{}}`, table, keys[table]),
			"delete_row": fmt.Sprintf(`{"table":%q,"key":%s}`, table, keys[table]),
		} {
			r.refusedIn(t, "a storyteller adds no "+table+" row", name, raw)
		}
	}
	if f.digest() != before {
		t.Error("a refused write changed the database")
	}
	// the words: each written, by every route, with what it was
	for i, c := range []*client{r.page, r.token} {
		for _, table := range []string{"quests", "queststeps"} {
			for _, w := range storytellerWords[table] {
				text := fmt.Sprintf("%s by route %d, %%p", w, i)
				raw := fmt.Sprintf(`{"table":%q,"key":%s,"values":{%q:%q}}`, table, keys[table], w, text)
				if code, msg, _ := atOp(c, "update_row", raw); code != 200 {
					t.Errorf("%s.%s: %d %s", table, w, code, msg)
				}
			}
		}
	}
	if isErr, text := atMCP(r.oauth, "update_row", `{"table":"queststeps","key":{"questid":40,"step":1},"values":{"says":"by an agent"}}`); isErr {
		t.Errorf("the words by an agent: %s", text)
	}
	for col, want := range map[string]string{"minlevel": "1", "giver": "Guard Dahvel", "enabled": "1", "requires": "", "source": "gm"} {
		if got := f.value(`SELECT ` + col + ` FROM quests WHERE questid = 40`); got != want {
			t.Errorf("quests.%s: %q", col, got)
		}
	}
	for col, want := range map[string]string{"xp": "100", "rewardtunar": "5", "rewarditems": "11*1", "kind": "give", "target": "Guard Dahvel", "items": "11*1"} {
		if got := f.value(`SELECT ` + col + ` FROM queststeps WHERE questid = 40 AND step = 2`); got != want {
			t.Errorf("queststeps.%s: %q", col, got)
		}
	}
	if n := f.value(`SELECT COUNT(*) FROM queststeps WHERE questid = 40`); n != "2" {
		t.Errorf("%s steps", n)
	}
	// a word changed meanwhile: the change is refused, the newer kept
	code, _, out := atOp(r.page, "update_row", `{"table":"quests","key":{"questid":40},"values":{"offer":"Mine."},"expect":{"offer":"what it said once"}}`)
	if code != 409 || out["current"] == nil {
		t.Errorf("a moved row: %d %v", code, out)
	}
	// the row editor alone, whatever calls it: no gate in front of it
	a := &actor{AccountID: 6, Username: "sage", Role: roleStoryteller, Scope: scopeWrite, Via: "test"}
	qt, _ := f.s.table("quests")
	st, _ := f.s.table("queststeps")
	if _, err := f.s.updateRow(a, qt, map[string]any{"questid": int64(40)}, map[string]any{"minlevel": 50}, nil, writeOpts{}); err == nil || !strings.Contains(err.Error(), storytellerRefused) {
		t.Errorf("updateRow of a quest's level: %v", err)
	}
	if _, err := f.s.updateRow(a, qt, map[string]any{"questid": int64(40)}, map[string]any{"minlevel": 50}, nil, writeOpts{Raw: true, Dry: true}); err == nil {
		t.Error("a raw dry run of a quest's level")
	}
	if _, err := f.s.insertRow(a, st, map[string]any{"questid": int64(40), "step": int64(3), "journal": "x"}, writeOpts{}); err == nil {
		t.Error("insertRow of a step")
	}
	if _, err := f.s.deleteRow(a, st, map[string]any{"questid": int64(40), "step": int64(2)}, writeOpts{}); err == nil {
		t.Error("deleteRow of a step")
	}
	if _, err := f.s.updateRow(a, st, map[string]any{"questid": int64(40), "step": int64(2)}, map[string]any{"says": "the row editor's"}, nil, writeOpts{}); err != nil {
		t.Errorf("updateRow of a step's words: %v", err)
	}
	// unchanged columns named beside a word, past the gate: only what changes is written
	if _, err := f.s.updateRow(a, st, map[string]any{"questid": int64(40), "step": int64(2)}, map[string]any{"says": "again", "xp": 100}, nil, writeOpts{}); err != nil {
		t.Errorf("updateRow of a word beside an unchanged xp: %v", err)
	}
	if f.value(`SELECT xp FROM queststeps WHERE questid = 40 AND step = 2`) != "100" {
		t.Error("the step's experience moved")
	}
	// the control: a GM changes what a quest is
	gm := f.browser()
	gm.login("gm")
	if code, text, _ := atOp(gm, "update_row", `{"table":"quests","key":{"questid":40},"values":{"minlevel":5}}`); code != 200 {
		t.Errorf("the control: %d %s", code, text)
	}
}

// A storyteller's words reach every client, and a dialog line's numbers the
// game: no bar, no megabytes, weights the speaker's pick can add up and an away
// the table's read can scan, by every route, every way of writing a row, and
// the row editor on its own.
func TestAStorytellersTextAndNumbersAreWhatTheGameReads(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	r := f.routesOf("sage")
	bars := strings.Repeat("|+", 180)
	long := strings.Repeat("x", storytellerMaxText+1)
	before := f.digest()
	for _, tc := range []struct{ name, raw, words string }{
		{"update_row", fmt.Sprintf(`{"table":"npcdialog","key":{"id":1},"values":{"line":%q}}`, bars), "a storyteller's text"},
		{"update_row", `{"table":"quests","key":{"questid":40},"values":{"offer":"a|b"}}`, "a storyteller's text"},
		{"update_row", `{"table":"queststeps","key":{"questid":40,"step":2},"values":{"says":"a|b"}}`, "a storyteller's text"},
		{"update_row", fmt.Sprintf(`{"table":"queststeps","key":{"questid":40,"step":2},"values":{"journal":%q}}`, long), "a storyteller's text"},
		{"update_row", fmt.Sprintf(`{"table":"npcdialog","key":{"id":1},"VALUES":{"line":%q}}`, bars), "a storyteller's text"},
		{"update_row", fmt.Sprintf(`{"table":"npcdialog","key":{"id":1},"values":{"line":"fine"},"Values":{"town":%q}}`, bars), "a storyteller's text"},
		{"insert_row", fmt.Sprintf(`{"table":"npcdialog","values":{"line":%q}}`, bars), "a storyteller's text"},
		{"clone_row", fmt.Sprintf(`{"table":"npcdialog","key":{"id":1},"changes":{"line":%q}}`, bars), "a storyteller's text"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":1001}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":-1}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":"0x10000"}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":"a lot"}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":null}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":9007199254740993}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"away":2}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"race":300}}`, "for a storyteller"},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"speaker_race":-2}}`, "for a storyteller"},
		{"insert_row", `{"table":"npcdialog","values":{"line":"Hail.","weight":5000}}`, "for a storyteller"},
		{"clone_row", `{"table":"npcdialog","key":{"id":1},"changes":{"away":7}}`, "for a storyteller"},
	} {
		for route, c := range map[string]*client{"page": r.page, "openapi": r.token} {
			if code, text, _ := atOp(c, tc.name, tc.raw); code != 403 || !strings.Contains(text, tc.words) {
				t.Errorf("%s %s %.120s: %d %s", route, tc.name, tc.raw, code, text)
			}
		}
		if isErr, text := atMCP(r.oauth, tc.name, tc.raw); !isErr || !strings.Contains(text, tc.words) {
			t.Errorf("mcp %s %.120s: %v %s", tc.name, tc.raw, isErr, text)
		}
	}
	if f.digest() != before {
		t.Error("a refused write changed the database")
	}
	// at the limits, and what a PS2 draws: written
	for _, raw := range []string{
		fmt.Sprintf(`{"table":"npcdialog","key":{"id":1},"values":{"line":%q,"weight":1000,"away":1,"race":-1,"speaker_race":255}}`, strings.Repeat("y", storytellerMaxText)),
		`{"table":"npcdialog","key":{"id":1},"values":{"weight":"0x10","away":"0","race":"9"}}`,
		`{"table":"quests","key":{"questid":40},"values":{"offer":"Hail, %p! Will you help?"}}`,
	} {
		if code, text, _ := atOp(r.page, "update_row", raw); code != 200 {
			t.Errorf("%.120s: %d %s", raw, code, text)
		}
	}
	if f.value(`SELECT weight FROM npcdialog WHERE id = 1`) != "16" {
		t.Errorf("the weight written: %s", f.value(`SELECT weight FROM npcdialog WHERE id = 1`))
	}
	if _, err := f.db.NPCDialog(); err != nil {
		t.Errorf("the dialog's read after the storyteller's numbers: %v", err)
	}
	// the row editor on its own: a row a GM wrote outside the bounds, copied by
	// the storyteller (the gate sees only what the copy changes), or put back by
	// a revert
	gm := f.browser()
	gm.login("gm")
	if _, _, ch := atOp(gm, "update_row", `{"table":"npcdialog","key":{"id":2},"values":{"weight":5000,"line":"a|b"}}`); ch["audit"] == nil {
		t.Fatalf("the GM's change: %v", ch)
	}
	if code, text, _ := atOp(r.page, "clone_row", `{"table":"npcdialog","key":{"id":2},"changes":{"town":"Freeport"}}`); code != 403 || !strings.Contains(text, "a storyteller's") {
		t.Errorf("a copy of a GM's row out of the bounds: %d %s", code, text)
	}
	_, _, back := atOp(gm, "update_row", `{"table":"npcdialog","key":{"id":2},"values":{"weight":1,"line":"plain"}}`)
	if code, text, _ := atOp(r.page, "revert_change", fmt.Sprintf(`{"id":%d}`, int64(id0(back["audit"])))); code != 403 || !strings.Contains(text, "a storyteller's") {
		t.Errorf("a revert to a GM's row out of the bounds: %d %s", code, text)
	}
	if f.value(`SELECT line FROM npcdialog WHERE id = 2`) != "plain" {
		t.Error("the revert wrote the bar")
	}
	// the control: a GM's text and numbers are a GM's
	if code, text, _ := atOp(gm, "update_row", `{"table":"quests","key":{"questid":40},"values":{"offer":"a|b"}}`); code != 200 {
		t.Errorf("the control: %d %s", code, text)
	}
}

// A storyteller's log: the changes to its tables, others' without who or from
// where, the ones it may not revert marked.
func TestAStorytellersAuditLogIsItsTablesWithoutOtherPeople(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	r := f.routesOf("sage")
	boss := f.browser()
	boss.login("boss")
	atOp(boss, "update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_level":31}}`)
	atOp(boss, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":50}}`)
	atOp(boss, "update_row", `{"table":"quests","key":{"questid":40},"values":{"minlevel":3}}`)
	mustExec(t, f.db, `UPDATE gm_audit SET ip = '203.0.113.9', detail = 'petition 7, Somebody''s words' WHERE username = 'boss'`)
	atOp(r.page, "update_row", `{"table":"npcdialog","key":{"id":1},"values":{"line":"Hold."}}`)

	code, _, out := atOp(r.page, "audit_log", `{}`)
	if code != 200 {
		t.Fatalf("audit_log: %d %v", code, out)
	}
	entries := out["entries"].([]any)
	if len(entries) != 3 || out["total"] != float64(3) {
		t.Fatalf("%d entries of %v: %v", len(entries), out["total"], entries)
	}
	marks := map[string]bool{}
	for _, v := range entries {
		e := v.(map[string]any)
		if !isStorytellerTable(fmt.Sprint(e["table"])) {
			t.Errorf("another table: %v", e)
		}
		marks[fmt.Sprint(e["table"])] = e["noRevert"] == true
		if e["username"] == "sage" {
			if e["via"] != "web" {
				t.Errorf("its own entry: %v", e)
			}
			continue
		}
		if e["username"] != "staff" || e["ip"] != "" || e["via"] != "" || e["accountid"] != float64(0) || e["detail"] != nil {
			t.Errorf("another's entry: %v", e)
		}
		if e["before"] == nil || e["after"] == nil {
			t.Errorf("another's entry says nothing of the change: %v", e)
		}
	}
	// an NPC's change (read alone) and a quest's level (no word) are no reverts of its
	if !marks["npcs"] || !marks["quests"] || marks["npcdialog"] {
		t.Errorf("the marks: %v", marks)
	}
	raw, _ := json.Marshal(entries)
	if text := string(raw); strings.Contains(text, "203.0.113.9") || strings.Contains(text, "boss") || strings.Contains(text, "Somebody") {
		t.Errorf("it shows another person: %s", text)
	}
	if code, _, out := atOp(r.page, "audit_log", `{"table":"npcdialog","key":{"id":1}}`); code != 200 || out["total"] != float64(1) {
		t.Errorf("by row: %d %v", code, out)
	}
	if code, _, out := atOp(r.page, "audit_log", `{"username":"SAGE"}`); code != 200 || out["total"] != float64(1) {
		t.Errorf("its own: %d %v", code, out)
	}
	if _, _, out := atOp(r.page, "audit_log", `{"action":"login"}`); out["total"] != float64(0) {
		t.Errorf("sign-ins: %v", out)
	}
	for _, raw := range []string{`{"table":"itempattern"}`, `{"table":"accountinfo","key":{"accountid":1}}`, `{"username":"boss"}`,
		`{"search":"petition"}`, `{"TABLE":"itempattern"}`, `{"table":"npcdialog","Table":"gm_storytellers"}`} {
		r.refusedIn(t, storytellerRefused, "audit_log", raw)
	}
	// the control: a GM reads who and from where
	gm := f.browser()
	gm.login("gm")
	if _, _, out := atOp(gm, "audit_log", `{"username":"boss","table":"npcs"}`); !strings.Contains(fmt.Sprint(out["entries"]), "203.0.113.9") {
		t.Errorf("the control: %v", out)
	}
}

// Revert: what a storyteller could have changed itself (any dialog line's
// change, a quest's words), never a quest's or a step's row added or taken
// away, nor what a quest is, nor another table's.
func TestAStorytellerRevertsOnlyChangesItCouldMake(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	r := f.routesOf("sage")
	boss := f.browser()
	boss.login("boss")
	entry := func(c *client, name, raw string) string {
		t.Helper()
		code, _, out := atOp(c, name, raw)
		if code != 200 {
			t.Fatalf("%s %s: %d %v", name, raw, code, out)
		}
		id, ok := out["audit"].(float64)
		if !ok {
			id, ok = out["change"].(map[string]any)["audit"].(float64)
		}
		if !ok {
			t.Fatalf("no audit line in %v", out)
		}
		return fmt.Sprintf(`{"id":%d}`, int64(id))
	}
	// not its to revert
	level := entry(boss, "update_row", `{"table":"quests","key":{"questid":40},"values":{"minlevel":3}}`)
	both := entry(boss, "update_row", `{"table":"queststeps","key":{"questid":40,"step":2},"values":{"says":"Ta.","xp":200}}`)
	stepAdded := entry(boss, "insert_row", `{"table":"queststeps","values":{"questid":40,"step":3,"kind":"talk","target":"Guard Dahvel","journal":"Report back."}}`)
	stepGone := entry(boss, "delete_row", `{"table":"queststeps","key":{"questid":40,"step":3}}`)
	npc := entry(boss, "update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_level":31}}`)
	item := entry(boss, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":50}}`)
	trip := entry(boss, "insert_row", `{"table":"questunlocks","values":{"questid":40,"npc":"Waysender Test","dest":"Freeport"}}`)
	before := f.digest()
	for _, raw := range []string{npc, item, trip} {
		r.refusedIn(t, storytellerRefused, "revert_change", raw)
	}
	var login, grant int64
	f.db.QueryRow(`SELECT MIN(id) FROM gm_audit WHERE action = 'login'`).Scan(&login)
	f.db.QueryRow(`SELECT MIN(id) FROM gm_audit WHERE tbl = 'gm_storytellers'`).Scan(&grant)
	r.refusedIn(t, storytellerRefused, "revert_change", fmt.Sprintf(`{"id":%d}`, login))
	r.refusedIn(t, storytellerRefused, "revert_change", fmt.Sprintf(`{"id":%d,"force":true,"dry_run":true}`, grant))
	for words, raw := range map[string]string{"sets quests.minlevel back": level, "sets queststeps.xp back": both,
		"add or take away a whole queststeps row": stepAdded} {
		r.refusedIn(t, words, "revert_change", raw)
	}
	r.refusedIn(t, "add or take away a whole queststeps row", "revert_change", stepGone)
	if f.digest() != before {
		t.Error("a refused revert changed the database")
	}
	if code, _, _ := atOp(r.page, "revert_change", `{"id":99999}`); code != 404 {
		t.Errorf("no such entry: %d", code)
	}
	// past the gate, the row editor holds it: a GM's change of a level
	a := &actor{AccountID: 6, Username: "sage", Role: roleStoryteller, Scope: scopeWrite, Via: "test"}
	var levelID int64
	fmt.Sscanf(level, `{"id":%d}`, &levelID)
	if _, err := f.s.revert(a, levelID, true, false); err == nil || !strings.Contains(err.Error(), storytellerRefused) {
		t.Errorf("revert of a quest's level, no gate: %v", err)
	}
	// its to revert: a GM's dialog lines, added, changed and deleted, and a quest's words
	for _, raw := range []string{
		entry(boss, "update_row", `{"table":"npcdialog","key":{"id":1},"values":{"line":"Halt."}}`),
		entry(boss, "insert_row", `{"table":"npcdialog","values":{"line":"Shoo.","weight":1}}`),
		entry(boss, "delete_row", `{"table":"npcdialog","key":{"id":2}}`),
		entry(boss, "update_row", `{"table":"quests","key":{"questid":40},"values":{"offer":"Bread. Now."}}`),
		entry(r.page, "update_row", `{"table":"queststeps","key":{"questid":40,"step":1},"values":{"says":"Mine."}}`),
	} {
		if code, _, out := atOp(r.page, "revert_change", raw); code != 200 {
			t.Errorf("revert %s: %d %v", raw, code, out)
		}
	}
	if f.value(`SELECT line FROM npcdialog WHERE id = 1`) != "Move along, %p." || f.value(`SELECT COUNT(*) FROM npcdialog`) != "2" ||
		f.value(`SELECT offer FROM quests WHERE questid = 40`) != "The watch is hungry.\nWill you fetch bread?" {
		t.Error("the reverts did not put the rows back")
	}
	// the control: a GM reverts the level
	if code, _, out := atOp(boss, "revert_change", level); code != 200 {
		t.Errorf("the control: %d %v", code, out)
	}
}

// What a storyteller is shown: its tables and operations, by every way of asking.
func TestAStorytellerIsShownOnlyWhatItCanUse(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	r := f.routesOf("sage")
	allowed, readOnly := []string{}, []string{}
	for _, op := range f.s.opList {
		if storytellerMayCall(op) {
			allowed = append(allowed, op.Name)
			if op.Scope == scopeRead {
				readOnly = append(readOnly, op.Name)
			}
		}
	}
	sort.Strings(allowed)
	sort.Strings(readOnly)
	names := func(list []any, key string) []string {
		out := []string{}
		for _, v := range list {
			out = append(out, v.(map[string]any)[key].(string))
		}
		sort.Strings(out)
		return out
	}
	// a server that restarts offers it to a GM, never below one
	f.s.d.Restart = func() error { return nil }
	code, sess := r.page.do("GET", "/api/auth/session", nil)
	if code != 200 || sess["user"].(map[string]any)["role"] != "storyteller" || sess["server"].(map[string]any)["restart"] != false || sess["tester"] != nil {
		t.Fatalf("the session: %d %v", code, sess)
	}
	gmPage := f.browser()
	gmPage.login("gm")
	if _, s := gmPage.do("GET", "/api/auth/session", nil); s["server"].(map[string]any)["restart"] != true || s["storyteller"] != nil {
		t.Errorf("the control, a GM's session: %v", s["server"])
	}
	if tables := sess["storyteller"].(map[string]any)["tables"].([]any); len(tables) != len(storytellerTableList) {
		t.Errorf("the session's tables: %v", tables)
	}
	code, out := r.page.do("GET", "/api/ops", nil)
	if got := names(out["ops"].([]any), "name"); code != 200 || !slices.Equal(got, allowed) {
		t.Errorf("api/ops: %d %v", code, got)
	}
	_, out = r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if got := names(out["result"].(map[string]any)["tools"].([]any), "name"); !slices.Equal(got, allowed) {
		t.Errorf("MCP's tools: %v", got)
	}
	readTok := f.bearer(r.page.workToken("r", "read"))
	_, out = readTok.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if got := names(out["result"].(map[string]any)["tools"].([]any), "name"); !slices.Equal(got, readOnly) {
		t.Errorf("a read token's tools: %v", got)
	}
	if code, text, _ := atOp(readTok, "update_row", `{"table":"npcdialog","key":{"id":1},"values":{"line":"x"}}`); code != 403 || !strings.Contains(text, "read-only") {
		t.Errorf("a read token's write: %d %s", code, text)
	}
	_, out = r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	instr := out["result"].(map[string]any)["instructions"].(string)
	if !strings.Contains(instr, "storyteller") || strings.Contains(instr, "work_board") || strings.Contains(instr, "game master") || strings.Contains(instr, "item and spell") {
		t.Errorf("MCP's instructions: %s", instr)
	}
	paths := func(c *client, q string) []string {
		code, doc := c.do("GET", "/api/openapi.json"+q, nil)
		if code != 200 {
			t.Fatalf("openapi%s: %d", q, code)
		}
		out := []string{}
		for p := range doc["paths"].(map[string]any) {
			out = append(out, strings.TrimPrefix(p, "/api/op/"))
		}
		sort.Strings(out)
		return out
	}
	if got := paths(r.token, ""); !slices.Equal(got, allowed) {
		t.Errorf("openapi with a storyteller's token: %v", got)
	}
	if got := paths(f.browser(), "?role=storyteller"); !slices.Equal(got, allowed) {
		t.Errorf("openapi?role=storyteller: %v", got)
	}
	core := paths(f.browser(), "?role=storyteller&core=1")
	if len(core) == 0 {
		t.Error("openapi?role=storyteller&core=1 is empty")
	}
	for _, n := range core {
		if !slices.Contains(allowed, n) {
			t.Errorf("the storyteller's short list has %s", n)
		}
	}
	if got := paths(f.browser(), ""); len(got) != len(f.s.opList) {
		t.Errorf("the control, the plain document: %d of %d", len(got), len(f.s.opList))
	}
	// the guide is a storyteller's, every operation it names one it has
	_, _, g := atOp(r.page, "guide", `{}`)
	guide := fmt.Sprint(g["guide"])
	if !strings.HasPrefix(guide, "# EQOA storyteller guide") || strings.Contains(guide, "{{") || !strings.Contains(guide, f.ts.URL+"/mcp") ||
		!strings.Contains(guide, "217") || strings.Contains(guide, "# EQOA tester guide") {
		t.Errorf("the guide: %.300s", guide)
	}
	for _, op := range f.s.opList {
		if !storytellerMayCall(op) && strings.Contains(guide, "`"+op.Name+"`") {
			t.Errorf("the guide names %s", op.Name)
		}
	}
	for _, w := range append(slices.Clone(storytellerWords["quests"]), storytellerWords["queststeps"]...) {
		if !strings.Contains(guide, "`"+w+"`") {
			t.Errorf("the guide does not say what %s is", w)
		}
	}
	req, _ := http.NewRequest("GET", f.ts.URL+"/api/guide", nil)
	req.Header.Set("Authorization", "Bearer "+r.token.auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(body), "# EQOA storyteller guide") {
		t.Errorf("api/guide with a storyteller's token: %.200s", body)
	}
	// server_status: who, and no state of the server's
	_, _, st := atOp(r.page, "server_status", `{}`)
	for _, k := range []string{"playersOnline", "npcsInWorld", "sessions", "settings", "databaseBytes", "gmSpawns", "worlds"} {
		if _, there := st[k]; there {
			t.Errorf("server_status says %s", k)
		}
	}
	if c := st["counts"].(map[string]any); c["quests"] != float64(1) || c["dialogLines"] != float64(2) || c["questSteps"] != float64(2) ||
		c["accounts"] != nil || c["items"] != nil {
		t.Errorf("server_status's counts: %v", c)
	}
	if you := st["you"].(map[string]any); you["role"] != "storyteller" || you["username"] != "sage" || you["subgm"] != nil ||
		fmt.Sprint(you["roles"]) != "[storyteller]" {
		t.Errorf("server_status's you: %v", you)
	}
	// the catalog: its six tables, what it changes in each
	_, _, lt := atOp(r.page, "list_tables", `{}`)
	got := names(lt["tables"].([]any), "name")
	want := slices.Clone(storytellerTableList)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("list_tables: %v", got)
	}
	for _, v := range lt["tables"].([]any) {
		tb := v.(map[string]any)
		name := tb["name"].(string)
		wantWrite := map[bool]string{true: "storyteller", false: "none"}[storytellerWrites(name)]
		if tb["write"] != wantWrite || (tb["writeColumns"] != nil) != (storytellerWords[name] != nil) {
			t.Errorf("list_tables' %s: write %v, writeColumns %v", name, tb["write"], tb["writeColumns"])
		}
	}
	for name, want := range map[string][3]any{
		"npcdialog": {"storyteller", true, true}, "quests": {"storyteller", false, false}, "queststeps": {"storyteller", false, false},
		"npcs": {"none", false, false}, "questunlocks": {"none", false, false}, "queststeptargets": {"none", false, false},
	} {
		_, _, d := atOp(r.page, "describe_table", fmt.Sprintf(`{"table":%q}`, name))
		if d["write"] != want[0] || d["insert"] != want[1] || d["delete"] != want[2] {
			t.Errorf("describe_table %s: write %v insert %v delete %v", name, d["write"], d["insert"], d["delete"])
		}
		for _, v := range d["columns"].([]any) {
			c := v.(map[string]any)
			free := name == "npcdialog" || slices.Contains(storytellerWords[name], c["name"].(string))
			if (c["readonly"] == true) == free {
				t.Errorf("describe_table %s.%s: readonly %v", name, c["name"], c["readonly"])
			}
		}
	}
	// the quests: whole, but who has one
	code, _, qg := atOp(r.page, "quest_get", `{"quest":40}`)
	if code != 200 || qg["characters"] != nil || len(qg["steps"].([]any)) != 2 || qg["quest"] == nil {
		t.Errorf("quest_get: %d %v", code, qg)
	}
	if code, _, ql := atOp(r.page, "quest_list", `{"search":"bread"}`); code != 200 || ql["total"] != float64(1) {
		t.Errorf("quest_list: %d %v", code, ql)
	}
	// the control: a GM is told who has it
	gm := f.browser()
	gm.login("gm")
	if _, _, out := atOp(gm, "quest_get", `{"quest":40}`); !strings.Contains(fmt.Sprint(out["characters"]), "Hero") {
		t.Errorf("the control: %v", out["characters"])
	}
}

// Both and the gate's three layers each hold a storyteller on their own: the
// gate with no handler behind it, the handlers and the row editor with no gate
// in front of them.
func TestTheGateAndTheRowEditorEachHoldAStoryteller(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	a := &actor{AccountID: 6, Username: "sage", Role: roleStoryteller, Scope: scopeWrite, Via: "test"}
	boss := f.browser()
	boss.login("boss")
	_, _, itemChange := atOp(boss, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":31}}`)
	_, _, levelChange := atOp(boss, "update_row", `{"table":"quests","key":{"questid":40},"values":{"minlevel":3}}`)
	for _, op := range f.s.opList {
		props, _ := op.Input["properties"].(map[string]any)
		if props["table"] == nil {
			continue
		}
		raw := json.RawMessage(`{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":1},"keys":[10],"filters":[{"column":"patternid","value":10}]}`)
		c := &opCtx{s: f.s, a: a, raw: raw}
		if err := c.roleGate(op); err == nil || errStatus(err) != 403 {
			t.Errorf("the gate alone, %s: %v", op.Name, err)
		}
		if op.Name == "audit_log" {
			continue // its handler narrows the lines instead
		}
		if _, err := op.Handler(&opCtx{s: f.s, a: a, raw: raw}); err == nil || errStatus(err) != 403 {
			t.Errorf("the handler alone, %s: %v", op.Name, err)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"update_row", `{"table":"quests","key":{"questid":40},"values":{"minlevel":50}}`},
		{"update_row", `{"table":"queststeps","key":{"questid":40,"step":2},"values":{"says":"x","rewardtunar":9}}`},
		{"insert_row", `{"table":"quests","values":{"name":"x"}}`},
		{"clone_row", `{"table":"queststeps","key":{"questid":40,"step":2},"changes":{}}`},
		{"delete_row", `{"table":"queststeps","key":{"questid":40,"step":2}}`},
		{"delete_row", `{"table":"npcs","key":{"id":500}}`},
		{"update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_name":"x"}}`},
		{"insert_row", `{"table":"questunlocks","values":{"questid":40,"npc":"x","dest":"y"}}`},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"line":"a|b"}}`},
		{"update_row", `{"table":"npcdialog","key":{"id":1},"values":{"weight":5000}}`},
		{"audit_log", `{"table":"itempattern"}`},
		{"audit_log", `{"username":"boss"}`},
		{"audit_log", `{"search":"x"}`},
		{"revert_change", fmt.Sprintf(`{"id":%d}`, int64(id0(itemChange["audit"])))},
		{"revert_change", fmt.Sprintf(`{"id":%d}`, int64(id0(levelChange["audit"])))},
		{"quest_save", `{"quest":{"questid":40,"name":"x"},"steps":[]}`},
		{"search", `{"query":"a","kinds":["npc"]}`},
		{"fetch", `{"id":"npc:500"}`},
		{"query_rows", `{"table":"characters"}`},
	} {
		c := &opCtx{s: f.s, a: a, raw: json.RawMessage(tc.raw)}
		if err := c.roleGate(f.s.ops[tc.name]); err == nil || errStatus(err) != 403 {
			t.Errorf("the gate alone, %s %s: %v", tc.name, tc.raw, err)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"update_row", `{"table":"quests","key":{"questid":40},"values":{"offer":"x","notes":"y"}}`},
		{"update_row", `{"table":"queststeps","key":{"questid":40,"step":2},"values":{"says":"x","journal":"y"}}`},
		{"insert_row", `{"table":"npcdialog","values":{"line":"x","weight":3}}`},
		{"clone_row", `{"table":"npcdialog","key":{"id":1},"changes":{"line":"x"}}`},
		{"delete_row", `{"table":"npcdialog","key":{"id":1}}`},
		{"query_rows", `{"table":"npcs"}`},
		{"audit_log", `{"table":"questunlocks"}`},
		{"quest_get", `{"quest":40}`},
		{"quest_list", `{}`},
		{"guide", `{}`},
	} {
		c := &opCtx{s: f.s, a: a, raw: json.RawMessage(tc.raw)}
		if err := c.roleGate(f.s.ops[tc.name]); err != nil {
			t.Errorf("the gate alone, %s %s: %v", tc.name, tc.raw, err)
		}
	}
	// the handlers alone: no table but its own read
	for _, raw := range []string{`{"table":"characters"}`, `{"table":"itempattern"}`, `{"table":"gm_storytellers"}`} {
		if _, err := f.s.ops["query_rows"].Handler(&opCtx{s: f.s, a: a, raw: json.RawMessage(raw)}); err == nil || errStatus(err) != 403 {
			t.Errorf("the handler alone, query_rows %s: %v", raw, err)
		}
	}
	if _, err := f.s.ops["query_rows"].Handler(&opCtx{s: f.s, a: a, raw: json.RawMessage(`{"table":"npcs"}`)}); err != nil {
		t.Errorf("the handler alone, its own npcs: %v", err)
	}
	if _, err := f.s.ops["fetch"].Handler(&opCtx{s: f.s, a: a, raw: json.RawMessage(`{"id":"npc:500"}`)}); err == nil || errStatus(err) != 403 {
		t.Errorf("the handler alone, fetch: %v", err)
	}
	// the row editor, whatever calls it
	for _, name := range []string{"accountinfo", "characters", "npcs", "questunlocks", "queststeptargets", "charquests", "itempattern",
		"questnotyet", "signtext", "gm_storytellers", "zz"} {
		if err := mayWrite(a, &Table{Name: name, Write: writeGM}); err == nil || !strings.Contains(err.Error(), storytellerRefused) {
			t.Errorf("mayWrite %s: %v", name, err)
		}
	}
	for _, name := range []string{"npcdialog", "quests", "queststeps"} {
		if err := mayWrite(a, &Table{Name: name, Write: writeGM}); err != nil {
			t.Errorf("mayWrite %s: %v", name, err)
		}
	}
	for _, tc := range []struct {
		table, action string
		vals          map[string]any
		ok            bool
	}{
		{"quests", "update", map[string]any{"offer": "x"}, true},
		{"quests", "update", map[string]any{"offer": "x", "giver": "y"}, false},
		{"quests", "insert", map[string]any{"name": "x"}, false},
		{"quests", "delete", nil, false},
		{"queststeps", "update", map[string]any{"journal": "x"}, true},
		{"queststeps", "update", map[string]any{"spells": "1"}, false},
		{"npcdialog", "insert", map[string]any{"line": "x", "weight": int64(3)}, true},
		{"npcdialog", "delete", nil, true},
		{"npcdialog", "update", map[string]any{"weight": int64(1001)}, false},
		{"npcdialog", "update", map[string]any{"line": "|x"}, false},
		{"npcs", "update", map[string]any{"npc_name": "x"}, false},
	} {
		if err := roleRow(a, tc.table, tc.action, tc.vals); (err == nil) != tc.ok {
			t.Errorf("roleRow %s %s %v: %v", tc.table, tc.action, tc.vals, err)
		}
	}
	if err := mayWrite(&actor{Role: roleGM, Scope: scopeWrite}, &Table{Name: "npcs", Write: writeGM}); err != nil {
		t.Errorf("the control, a GM: %v", err)
	}
	if err := roleRow(&actor{Role: roleGM, Scope: scopeWrite}, "quests", "delete", nil); err != nil {
		t.Errorf("the control, a GM's row: %v", err)
	}
	// anyone below a GM holding no role is held to nothing at all
	for _, role := range []string{"", "player", "Storyteller"} {
		c := &opCtx{s: f.s, a: &actor{AccountID: 3, Username: "player", Role: role, Scope: scopeWrite}, raw: json.RawMessage(`{"table":"npcdialog"}`)}
		if err := c.roleGate(f.s.ops["query_rows"]); err == nil || !strings.Contains(err.Error(), bothRefused) {
			t.Errorf("an actor of role %q at the gate: %v", role, err)
		}
	}
}

// Taking the role away: at once, on a session, a token, an OAuth grant and its
// refresh; a row taken away by a tool too; and a ban.
func TestTakingTheStorytellerRoleAwayCountsAtOnce(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	r := f.routesOf("sage")
	clientID, tok := oauthTokens(t, f, "sage", "write", "")
	for _, c := range []*client{r.page, r.token, r.oauth, f.bearer(tok["access_token"].(string))} {
		if code, text, _ := atOp(c, "server_status", `{}`); code != 200 {
			t.Fatalf("before: %d %s", code, text)
		}
	}
	out := f.setStoryteller("sage", false)
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "no longer a storyteller") || strings.Contains(live, "tester still") {
		t.Errorf("the answer's live: %s", live)
	}
	for route, c := range map[string]*client{"page": r.page, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after: %d", route, code)
		}
	}
	resp, err := http.PostForm(f.ts.URL+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID},
		"refresh_token": {tok["refresh_token"].(string)}})
	if err != nil {
		t.Fatal(err)
	}
	var refreshed map[string]any
	json.NewDecoder(resp.Body).Decode(&refreshed)
	resp.Body.Close()
	if refreshed["access_token"] != nil {
		t.Errorf("a refresh after: %v", refreshed)
	}
	if n := f.value(`SELECT COUNT(*) FROM gm_tokens WHERE accountid = 6 AND revoked = 0`); n != "0" {
		t.Errorf("%s live tokens after", n)
	}
	if code, _ := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "sage", "password": testPassword}); code != 401 {
		t.Errorf("a sign-in after: %d", code)
	}
	var who, detail string
	f.db.QueryRow(`SELECT username, detail FROM gm_audit WHERE tbl = 'gm_storytellers' AND action = 'delete' ORDER BY id DESC LIMIT 1`).Scan(&who, &detail)
	if who != "boss" || !strings.Contains(detail, "took the storyteller role from sage") {
		t.Errorf("the audit line: %s %s", who, detail)
	}
	// a second grant: a new sign-in, the old tokens dead
	f.setStoryteller("sage", true)
	if code, _, _ := atOp(r.token, "server_status", `{}`); code != 401 {
		t.Errorf("an old token after a second grant: %d", code)
	}
	r = f.routesOf("sage")
	// the row taken away by a tool, nothing revoked: out at the next request
	mustExec(t, f.db, `DELETE FROM gm_storytellers WHERE accountid = 6`)
	for route, c := range map[string]*client{"page": r.page, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after the row went: %d", route, code)
		}
	}
	// and a ban
	mustExec(t, f.db, `INSERT INTO gm_storytellers (accountid) VALUES (6)`)
	r = f.routesOf("sage")
	boss := f.browser()
	boss.login("boss")
	if code, _, out := atOp(boss, "ban_account", `{"account":"sage","banned":true}`); code != 200 {
		t.Fatalf("ban: %d %v", code, out)
	}
	for route, c := range map[string]*client{"page": r.page, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after a ban: %d", route, code)
		}
	}
}

// Giving and taking the role is an admin's, audited and revertible; the admin
// scope is never a storyteller's.
func TestTheStorytellerRoleIsAnAdminsToGive(t *testing.T) {
	f := newFixture(t)
	f.addAccount(6, "sage", 0)
	gm := f.browser()
	gm.login("gm")
	if code, _, _ := atOp(gm, "set_account_storyteller", `{"account":"sage","storyteller":true}`); code != 403 {
		t.Errorf("a GM: %d", code)
	}
	if code, _, _ := atOp(gm, "insert_row", `{"table":"gm_storytellers","values":{"accountid":6}}`); code == 200 {
		t.Error("a GM's row edit")
	}
	boss := f.browser()
	boss.login("boss")
	writeTok := f.bearer(boss.workToken("w", "write"))
	if code, _, _ := atOp(writeTok, "set_account_storyteller", `{"account":"sage","storyteller":true}`); code != 403 {
		t.Errorf("an admin's write token: %d", code)
	}
	code, _, out := atOp(boss, "set_account_storyteller", `{"account":"sage","storyteller":true,"note":"writes the closing lines"}`)
	if code != 200 {
		t.Fatalf("an admin: %d %v", code, out)
	}
	ch := out["change"].(map[string]any)
	who, via, action, tbl, before, after := auditRow(t, f, ch["audit"])
	if who != "boss" || via != "web" || action != "insert" || tbl != "gm_storytellers" || before != "" || !strings.Contains(after, `"accountid":6`) {
		t.Errorf("the audit line: %s %s %s %s %s %s", who, via, action, tbl, before, after)
	}
	if detail := f.value(`SELECT detail FROM gm_audit WHERE id = ?`, ch["audit"]); !strings.Contains(detail, "gave sage the storyteller role") ||
		!strings.Contains(detail, "writes the closing lines") {
		t.Errorf("the detail: %s", detail)
	}
	if by, note := f.value(`SELECT granted_by FROM gm_storytellers WHERE accountid = 6`), f.value(`SELECT note FROM gm_storytellers WHERE accountid = 6`); by != "boss" || note != "writes the closing lines" {
		t.Errorf("the row: %s %s", by, note)
	}
	if !strings.Contains(fmt.Sprint(ch["live"]), "as a storyteller") || levelOf(t, f, 6) != 0 || f.s.testerFlag(6) {
		t.Errorf("the grant's live %v, level %d, tester %v", ch["live"], levelOf(t, f, 6), f.s.testerFlag(6))
	}
	if _, _, again := atOp(boss, "set_account_storyteller", `{"account":"sage","storyteller":true}`); again["unchanged"] != true {
		t.Errorf("a second grant: %v", again)
	}
	sage := f.browser()
	sage.login("sage")
	if code, _, _ := atOp(sage, "set_account_storyteller", `{"account":"player","storyteller":true}`); code != 403 {
		t.Errorf("a storyteller gives it: %d", code)
	}
	for _, raw := range []string{`{"account":"gm","storyteller":true}`, `{"account":"boss","storyteller":true}`, `{"account":"bannedgm","storyteller":true}`,
		`{"account":"nobody","storyteller":true}`, `{"account":"sage"}`} {
		if code, _, out := atOp(boss, "set_account_storyteller", raw); code == 200 {
			t.Errorf("%s: %d %v", raw, code, out)
		}
	}
	if _, _, acc := atOp(boss, "fetch", `{"id":"account:6"}`); acc["record"].(map[string]any)["storyteller"] != true || acc["record"].(map[string]any)["tester"] != false {
		t.Errorf("the account's record: %v", acc)
	}
	if _, _, found := atOp(boss, "search", `{"query":"sage","kinds":["account"]}`); !strings.Contains(fmt.Sprint(found["results"]), "player, storyteller") {
		t.Errorf("search: %v", found)
	}
	// the admin scope: refused at every way to a credential
	f.enrolTOTP(6)
	if code, _ := sage.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "admin", "days": 7}); code != 403 {
		t.Errorf("the page's admin token: %d", code)
	}
	if code, _ := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "sage", "password": testPassword,
		"code": totpNow(testTOTPSecret), "scope": "admin"}); code != 403 {
		t.Errorf("a terminal's admin token: %d", code)
	}
	if _, _, err := f.s.issueToken(6, "direct", kindPersonal, scopeAdmin, "", time.Hour, 0); err == nil {
		t.Error("issueToken wrote the admin scope for a storyteller")
	}
	value := newTokenValue("gmt_")
	mustExec(t, f.db, `INSERT INTO gm_tokens (accountid, name, kind, hash, prefix, scope, created, expires)
		VALUES (6, 'old', 'token', ?, 'gmt_xx', 'admin', ?, ?)`, hashToken(value), now(), stamp(time.Now().Add(time.Hour)))
	if code, _, _ := atOp(f.bearer(value), "quest_list", `{}`); code != 401 {
		t.Errorf("an old admin token on a storyteller: %d", code)
	}
	// a revert takes the role back, and counts at once
	if code, _, out := atOp(boss, "revert_change", fmt.Sprintf(`{"id":%d}`, int64(ch["audit"].(float64)))); code != 200 {
		t.Fatalf("revert the grant: %d %v", code, out)
	}
	if code, _, _ := atOp(sage, "server_status", `{}`); code != 401 {
		t.Errorf("a session after the grant's revert: %d", code)
	}
	if tb, err := f.s.table("gm_storytellers"); err != nil || tb.Write != writeAdmin || tb.Hidden {
		t.Errorf("the catalog: %v %v", tb, err)
	}
	// a shard whose accounts are another's gives no role: the owner's panel does
	f.s.d.AccountsShared = true
	f.s.holdSharedAccounts()
	if code, text, _ := atOp(boss, "set_account_storyteller", `{"account":"sage","storyteller":true}`); code != 403 || !strings.Contains(text, "copy") {
		t.Errorf("on a shard: %d %s", code, text)
	}
}

// An account with both rows is given what either gives, each table held to
// its own role's rules, and refused what neither gives in both their words;
// one role taken away, it signs in again with the other.
func TestBothRolesAreGivenWhatEitherGives(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.addAccount(8, "duo", 0)
	f.setTester("duo", true)
	f.setStoryteller("duo", true)
	f.storyFixture()
	r := f.routesOf("duo")
	code, sess := r.page.do("GET", "/api/auth/session", nil)
	if code != 200 || sess["user"].(map[string]any)["role"] != "tester+storyteller" || sess["tester"] == nil || sess["storyteller"] == nil {
		t.Fatalf("the session: %d %v", code, sess)
	}
	// either's
	for name, raw := range map[string]string{
		"update_row": `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":9}}`,
		"insert_row": `{"table":"npcdialog","values":{"line":"Both of us.","weight":1}}`,
		"clone_row":  `{"table":"itempattern","key":{"patternid":10},"changes":{"itemname":"Dull Sword"}}`,
		"quest_get":  `{"quest":40}`,
		"search":     `{"query":"Rusty"}`,
		"fetch":      `{"id":"spell:100"}`,
		"audit_log":  `{"table":"npcdialog"}`,
	} {
		if code, text, _ := atOp(r.page, name, raw); code != 200 {
			t.Errorf("%s %s: %d %s", name, raw, code, text)
		}
	}
	if code, text, _ := atOp(r.page, "update_row", `{"table":"quests","key":{"questid":40},"values":{"started":"Off you go."}}`); code != 200 {
		t.Errorf("a quest's words: %d %s", code, text)
	}
	// each table to its own role's rules, refused in that role's words
	for words, raw := range map[string]string{
		"quests.minlevel is not a storyteller's": `{"table":"quests","key":{"questid":40},"values":{"minlevel":9}}`,
		"a tester's text":                        `{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"a|b"}}`,
		"a storyteller's text":                   `{"table":"npcdialog","key":{"id":1},"values":{"line":"a|b"}}`,
	} {
		r.refusedIn(t, words, "update_row", raw)
	}
	r.refusedIn(t, "does not delete", "delete_row", `{"table":"itempattern","key":{"patternid":10}}`)
	// neither's, in both their words
	for name, raw := range map[string]string{
		"query_rows":     `{"table":"characters"}`,
		"update_row":     `{"table":"npcs","key":{"id":500},"values":{"npc_level":2}}`,
		"give_item":      `{"character":"duoHero","item":10}`,
		"quest_save":     `{"quest":{"questid":40,"name":"x"},"steps":[]}`,
		"audit_log":      `{"username":"boss"}`,
		"play_visual":    `{"visual":"0x1DAEB5B6"}`,
		"fetch":          `{"id":"npc:500"}`,
		"describe_table": `{"table":"gm_testers"}`,
	} {
		r.refusedIn(t, bothRefused, name, raw)
	}
	// what it is shown: either's tables and operations
	_, _, lt := atOp(r.page, "list_tables", `{}`)
	if n := len(lt["tables"].([]any)); n != len(testerTableList)+len(storytellerTableList) {
		t.Errorf("list_tables: %d", n)
	}
	for table, write := range map[string]string{"itempattern": "tester", "npcdialog": "storyteller", "npcs": "none"} {
		if _, _, d := atOp(r.page, "describe_table", fmt.Sprintf(`{"table":%q}`, table)); d["write"] != write {
			t.Errorf("describe_table %s: %v", table, d["write"])
		}
	}
	var union []string
	for _, op := range f.s.opList {
		if testerMayCall(op) || storytellerMayCall(op) {
			union = append(union, op.Name)
		}
	}
	sort.Strings(union)
	code, ops := r.page.do("GET", "/api/ops", nil)
	var got []string
	for _, v := range ops["ops"].([]any) {
		got = append(got, v.(map[string]any)["name"].(string))
	}
	sort.Strings(got)
	if code != 200 || !slices.Equal(got, union) {
		t.Errorf("api/ops: %v, want %v", got, union)
	}
	for _, q := range []string{"?role=tester+storyteller", "?role=tester%2Bstoryteller", "?role=storyteller,tester"} {
		_, doc := f.browser().do("GET", "/api/openapi.json"+q, nil)
		if n := len(doc["paths"].(map[string]any)); n != len(union) {
			t.Errorf("openapi%s: %d of %d", q, n, len(union))
		}
	}
	_, out := r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	if instr := out["result"].(map[string]any)["instructions"].(string); !strings.Contains(instr, "tester and storyteller") || strings.Contains(instr, "work_board") {
		t.Errorf("MCP's instructions: %s", instr)
	}
	_, _, g := atOp(r.page, "guide", `{}`)
	if guide := fmt.Sprint(g["guide"]); !strings.Contains(guide, "# EQOA tester guide") || !strings.Contains(guide, "# EQOA storyteller guide") {
		t.Errorf("the guide: %.200s", guide)
	}
	_, _, st := atOp(r.page, "server_status", `{}`)
	if you := st["you"].(map[string]any); fmt.Sprint(you["roles"]) != "[tester storyteller]" || you["subgm"] != false {
		t.Errorf("server_status's you: %v", you)
	}
	if c := st["counts"].(map[string]any); c["items"] == nil || c["quests"] == nil {
		t.Errorf("server_status's counts: %v", c)
	}
	// one role taken away: its sessions end, and it signs in again with the other
	out = f.setStoryteller("duo", false)
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "tester still") {
		t.Errorf("the answer's live: %s", live)
	}
	if code, _, _ := atOp(r.page, "server_status", `{}`); code != 401 {
		t.Errorf("its session after: %d", code)
	}
	page := f.browser()
	page.login("duo")
	if _, s := page.do("GET", "/api/auth/session", nil); s["user"].(map[string]any)["role"] != "tester" {
		t.Errorf("signed in again: %v", s["user"])
	}
	if code, text, _ := atOp(page, "update_row", `{"table":"npcdialog","key":{"id":1},"values":{"line":"x"}}`); code != 403 || !strings.Contains(text, testerRefused) {
		t.Errorf("a dialog line after: %d %s", code, text)
	}
	// a GM's way in never reads either table: both gone, a GM still signs in
	mustExec(t, f.db, `INSERT INTO gm_storytellers (accountid) VALUES (2)`)
	gm := f.browser()
	gm.login("gm")
	if _, s := gm.do("GET", "/api/auth/session", nil); s["user"].(map[string]any)["role"] != "gm" || s["storyteller"] != nil {
		t.Errorf("a GM with a row: %v", s["user"])
	}
	mustExec(t, f.db, `ALTER TABLE gm_storytellers RENAME TO gm_storytellers_gone`)
	if code, _ := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword}); code != 200 {
		t.Errorf("a GM's sign-in with the table gone: %d", code)
	}
	mustExec(t, f.db, `ALTER TABLE gm_storytellers_gone RENAME TO gm_storytellers`)
}

// A storyteller made a GM: what it made as a storyteller does not act as a GM;
// a GM brought down who holds the role is a storyteller again, and told so.
func TestAStorytellerMadeAGMStartsAgain(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	sage := f.browser()
	sage.login("sage")
	tok := f.bearer(sage.workToken("old", "write"))
	boss := f.browser()
	boss.login("boss")
	code, _, out := atOp(boss, "set_account_level", `{"account":"sage","level":1}`)
	if code != 200 {
		t.Fatalf("promote: %d %v", code, out)
	}
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "revoked") || !strings.Contains(live, "a storyteller") {
		t.Errorf("the answer: %s", live)
	}
	if code, _, _ := atOp(tok, "query_rows", `{"table":"characters"}`); code != 401 {
		t.Errorf("its storyteller token as a GM: %d", code)
	}
	sage = f.browser()
	sage.login("sage")
	if code, text, _ := atOp(sage, "query_rows", `{"table":"characters"}`); code != 200 {
		t.Errorf("a new sign-in as a GM: %d %s", code, text)
	}
	code, _, out = atOp(boss, "set_account_level", `{"account":"sage","level":0}`)
	if code != 200 {
		t.Fatalf("demote: %d %v", code, out)
	}
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "storyteller still (set_account_storyteller takes that away)") {
		t.Errorf("the answer: %s", live)
	}
	sage = f.browser()
	sage.login("sage")
	if code, text, _ := atOp(sage, "query_rows", `{"table":"characters"}`); code != 403 || !strings.Contains(text, storytellerRefused) {
		t.Errorf("after coming down: %d %s", code, text)
	}
}

// The sanitised snapshot leaves the role out: an admin's copy names no
// storyteller.
func TestASnapshotLeavesTheStorytellersOut(t *testing.T) {
	f := newFixture(t)
	f.newStoryteller()
	boss := f.browser()
	boss.login("boss")
	code, out := boss.op("download_snapshot", nil)
	if code != 200 {
		t.Fatalf("download_snapshot: %d %v", code, out)
	}
	resp, gz := fetch(t, "GET", out["url"].(string))
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d %s", resp.StatusCode, gz)
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var n, lines int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM gm_storytellers`).Scan(&n); err != nil || n != 0 {
		t.Errorf("gm_storytellers in the snapshot: %d %v", n, err)
	}
	// the control: the game's content is whole
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM npcdialog`).Scan(&lines); err != nil || lines != 2 {
		t.Errorf("npcdialog in the snapshot: %d %v", lines, err)
	}
}

// browserPage loads the panel in a real browser, as user, and runs script in
// the frame (tester_test.go's harness): "" when it ran to the end, else why
// not; skips when no browser can load it.
func browserPage(t *testing.T, f *fixture, browser, user, code, script string) {
	t.Helper()
	page := `<!doctype html><meta charset="utf-8"><div id="app"></div><script type="module">
const refused = [];
const realFetch = window.fetch;
window.fetch = async (...a) => {
  const r = await realFetch(...a);
  if (r.status === 401 || r.status === 403) refused.push(r.status + ' ' + String(a[0]) + ' ' + (a[1] && a[1].body || '').slice(0, 80));
  return r;
};
const wait = async (ok, what) => { for (let i = 0; i < 300 && !ok(); i++) await new Promise((r) => setTimeout(r, 50));
  if (!ok()) throw new Error('never saw ' + what + ' on ' + location.hash + ': ' + document.getElementById('app').textContent.slice(-900)); };
const text = () => document.querySelector('main').textContent;
const buttons = () => [...document.querySelectorAll('main button')].map((b) => b.textContent.trim());
const nav = () => [...document.querySelectorAll('nav a')].map((a) => a.getAttribute('href'));
const open = async (hash, what) => { location.hash = hash; await wait(() => text().includes(what), what); await new Promise((r) => setTimeout(r, 250)); };
const no = (list, word) => { if (list.some((b) => b.includes(word))) throw new Error('offered ' + word + ' on ' + location.hash + ': ' + list.join(' | ')); };
const yes = (list, word) => { if (!list.some((b) => b.includes(word))) throw new Error('no ' + word + ' on ' + location.hash + ': ' + list.join(' | ')); };
const same = (a, b) => { if (JSON.stringify(a) !== JSON.stringify(b)) throw new Error('got ' + JSON.stringify(a) + ', want ' + JSON.stringify(b)); };
const field = (tag, label) => [...document.querySelectorAll('main ' + tag)].filter((i) => i.getAttribute('aria-label') === label);
const links = () => [...document.querySelectorAll('main a')].map((a) => a.getAttribute('href'));
(async () => {
  try {
    const r = await realFetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: '` + user + `', password: '` + testPassword + `', code: '` + code + `' }) });
    if (!r.ok) throw new Error('sign-in: ' + r.status);
    await import('/static/app.js');
    await wait(() => document.querySelector('nav a'), 'the frame');
` + script + `
    if (refused.length) throw new Error('refused calls: ' + refused.join('; '));
    await realFetch('/result', { method: 'POST', body: 'done' });
  } catch (e) {
    await realFetch('/result', { method: 'POST', body: String(e) });
  }
})();
</script>`
	result := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/harness.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(page))
	})
	mux.HandleFunc("/result", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		select {
		case result <- string(body):
		default:
		}
	})
	mux.Handle("/static/", f.s.Handler())
	mux.Handle("/api/", f.s.Handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+t.TempDir(), ts.URL+"/harness.html")
	tmp, err := os.MkdirTemp("", "storypage")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS=disabled:", "TMPDIR="+tmp)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Skipf("the browser did not start: %v", err)
	}
	gone := make(chan error, 1)
	go func() { gone <- cmd.Wait() }()
	defer func() { cancel(); <-gone }()
	select {
	case got := <-result:
		if got != "done" {
			t.Fatalf("the page, as %s: %.1500s", user, got)
		}
	case err := <-gone:
		gone <- err
		t.Skipf("the browser did not load the page (%v): %.600s", err, stderr.String())
	case <-ctx.Done():
		t.Fatalf("the page, as %s, never finished: %.600s", user, stderr.String())
	}
}

// The page in a real browser, when one is installed: a storyteller is offered
// its pages and buttons and no other, changes a step's closing line through the
// quest's words editor, and no page it can reach asks for anything refused; an
// account with both roles has the pages of both; a GM's and an admin's quests
// and accounts pages keep theirs.
func TestTheStorytellersPageInABrowser(t *testing.T) {
	var browser string
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			browser = p
			break
		}
	}
	if browser == "" {
		t.Skip("no Chromium to load the page in")
	}
	f := newFixture(t)
	f.newStoryteller()
	f.addAccount(8, "duo", 0)
	f.setTester("duo", true)
	f.setStoryteller("duo", true)

	storyteller := `
    same(nav(), ['#/quests', '#/tables', '#/audit', '#/ai', '#/security']);
    if (!document.querySelector('header').textContent.includes('Storyteller')) throw new Error('no Storyteller badge');
    if (document.querySelector('header input[type=search]')) throw new Error('the search box is offered');
    await wait(() => text().includes('Bread run'), 'the quests (the home page)');
    no(buttons(), 'New quest'); no(buttons(), 'Put the'); no(buttons(), 'Take the');
    if (text().includes("A character's quests") || text().includes('Drafts from EQOA-Data')) throw new Error('the GM tabs are offered');
    await open('#/quests/40', 'What the giver says');
    no(buttons(), 'Try it'); no(buttons(), 'Delete'); no(buttons(), 'Add a step'); no(buttons(), 'Remove'); yes(buttons(), 'Save');
    if (text().includes('Hero')) throw new Error('the quest shows who has it');
    if (field('input', 'Giver').length || field('select', 'Kind').length) throw new Error('what the quest is is offered');
    // a line too long for a box is said so, and the closing line changed as a person does
    const says = field('textarea', 'NPC says when done').find((i) => i.value.includes('Thank you'));
    says.value = 'Well done, %p. ' + 'The watch eats tonight. '.repeat(10);
    says.dispatchEvent(new Event('input'));
    await wait(() => text().includes('goes in more than one'), 'the box hint');
    says.value = 'Well done, %p.';
    says.dispatchEvent(new Event('input'));
    [...document.querySelectorAll('main button')].find((b) => b.textContent.trim() === 'Save').click();
    await wait(() => document.body.textContent.includes('Saved'), 'Saved');
    await open('#/tables', 'Dialog and quest tables');
    same(links().filter((l) => l.startsWith('#/tables/')).sort(), ['#/tables/npcdialog', '#/tables/npcs', '#/tables/questunlocks', '#/tables/queststeps', '#/tables/queststeptargets', '#/tables/quests'].sort());
    await open('#/tables/npcdialog', 'NPC dialog');
    yes(buttons(), 'New');
    await open('#/tables/npcdialog/1', 'Move along');
    yes(buttons(), 'Delete'); yes(buttons(), 'Copy');
    await open('#/tables/quests', 'Bread run');
    no(buttons(), 'New');
    await open('#/tables/quests/40', 'Bread run');
    no(buttons(), 'Delete'); no(buttons(), 'Copy');
    if (!field('input', 'Giver')[0]?.disabled) throw new Error('the giver is offered');
    if (field('textarea', 'Giver offers')[0]?.disabled !== false) throw new Error('the offer is not offered');
    await open('#/tables/queststeps', 'Bring the guard');
    await open('#/tables/npcs/500', 'Guard Dahvel');
    no(buttons(), 'Delete'); no(buttons(), 'Copy'); no(buttons(), 'Save');
    await open('#/audit', 'queststeps');
    yes(buttons(), 'Revert');
    if (text().includes('boss')) throw new Error('the log names another: ' + text().slice(0, 400));
    await open('#/ai', 'Your tokens');
    if (!text().includes('openapi.json?role=storyteller')) throw new Error('the AI page offers the whole API');
    await open('#/security', 'Authenticator');
    for (const hash of ['#/dashboard', '#/items', '#/npcs/500', '#/characters/7', '#/accounts/1', '#/settings', '#/quests/characters', '#/quests/new']) {
      await open(hash, 'Bread run');
      if (nav().length !== 5) throw new Error('the navigation moved');
    }
`
	both := `
    same(nav(), ['#/items', '#/spells', '#/quests', '#/visuals', '#/tables', '#/audit', '#/ai', '#/security', '#/disc']);
    if (!document.querySelector('header').textContent.includes('Tester · Storyteller')) throw new Error('no badge of both');
    if (!document.querySelector('header input[type=search]')) throw new Error('no search box for the tester');
    await wait(() => text().includes('Rusty Sword'), 'the items (the home page)');
    await open('#/tables', 'Your tables');
    if (links().filter((l) => l.startsWith('#/tables/')).length !== 12) throw new Error('the tables of both: ' + links().join(' '));
    await open('#/quests/40', 'What the giver says');
    no(buttons(), 'Try it');
    await open('#/ai', 'Your tokens');
    if (!text().includes('openapi.json?role=tester%2Bstoryteller')) throw new Error('the AI page offers another API');
`
	gm := `
    await open('#/quests', 'Bread run');
    yes(buttons(), 'New quest');
    await open('#/quests/40', 'Steps');
    yes(buttons(), 'Try it'); yes(buttons(), 'Delete'); yes(buttons(), 'Add a step');
    await open('#/accounts/6', 'Storyteller');
    no(buttons(), 'Storyteller');
`
	admin := `
    await open('#/accounts/6', 'Storyteller');
    yes(buttons(), 'Storyteller'); yes(buttons(), 'Tester');
    await open('#/accounts/2', 'Account 2');
    no(buttons(), 'Storyteller');
`
	f.enrolTOTP(1)
	for _, tc := range []struct{ name, user, code, script string }{
		{"storyteller", "sage", "", storyteller},
		{"both", "duo", "", both},
		{"gm", "gm", "", gm},
		{"admin", "boss", totpNow(testTOTPSecret), admin},
	} {
		t.Run(tc.name, func(t *testing.T) { browserPage(t, f, browser, tc.user, tc.code, tc.script) })
	}
	// the editor's Save reached the row under the storyteller's name, its words alone
	if says := f.value(`SELECT says FROM queststeps WHERE questid = 40 AND step = 2`); says != "Well done, %p." {
		t.Errorf("the page's Save: says %q", says)
	}
	var who, after string
	f.db.QueryRow(`SELECT username, after FROM gm_audit WHERE tbl = 'queststeps' ORDER BY id DESC LIMIT 1`).Scan(&who, &after)
	if who != "sage" || !strings.Contains(after, `"xp":100`) {
		t.Errorf("the audit line of the page's Save: %q %s", who, after)
	}
	if n := f.value(`SELECT COUNT(*) FROM gm_audit WHERE username = 'sage' AND tbl != ''`); n != "1" {
		t.Errorf("%s row changes by the storyteller's page: one Save, one row", n)
	}
}

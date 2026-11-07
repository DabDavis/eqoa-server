package gmweb

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

	"github.com/eqoa/server/internal/auth"
)

// The tester role (tester.go): a player's account that signs in to the panel
// and reads and changes the item and spell records, and nothing else, by every
// route. The loops take every registered operation and every table, so a new
// one cannot slip through; GMs, admins and plain players are the controls.

// testerRefused is in what the gate says, and in nothing else the panel says.
const testerRefused = "is not a tester's"

// addAccount adds an account of a level, with a character of serverid id+100.
func (f *fixture) addAccount(id int, name string, level int) {
	f.t.Helper()
	hash, _ := auth.HashPassword(testPassword, 4)
	mustExec(f.t, f.db, `INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (?, ?, ?, 1, ?)`,
		id, name, hash, level)
	mustExec(f.t, f.db, `INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level,
		haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar)
		VALUES (?, ?, ?, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 25300, 54, 15700, 100)`, name+"Hero", id, id+100)
}

// setTester is the admin's op.
func (f *fixture) setTester(name string, on bool) map[string]any {
	f.t.Helper()
	boss := f.browser()
	boss.login("boss")
	code, out := boss.op("set_account_tester", map[string]any{"account": name, "tester": on, "note": "for the test"})
	if code != 200 {
		f.t.Fatalf("set_account_tester %s %v: %d %v", name, on, code, out)
	}
	return out
}

// newTester is an account with the role, tess by default.
func (f *fixture) newTester() {
	f.t.Helper()
	f.addAccount(5, "tess", 0)
	f.setTester("tess", true)
}

// unlimit takes off the rate limits, for tests that make thousands of calls.
func (f *fixture) unlimit() {
	f.s.sessRate = newBucketLimiter(1e9, 1e9)
	f.s.rate = newRateLimiter(1<<30, time.Minute)
}

// testerRoutes are one account's every way in: the page's cookie, a personal
// token at the OpenAPI operations and at MCP, and an OAuth grant at MCP.
type testerRoutes struct {
	page, token, oauth *client
}

func (f *fixture) routesOf(name string) testerRoutes {
	f.t.Helper()
	page := f.browser()
	page.login(name)
	_, tok := oauthTokens(f.t, f, name, "write", "")
	access, _ := tok["access_token"].(string)
	if access == "" {
		f.t.Fatalf("no OAuth grant for %s: %v", name, tok)
	}
	return testerRoutes{page: page, token: f.bearer(page.workToken("t", "write")), oauth: f.bearer(access)}
}

// atOp calls an operation at api/op/<name>: (status, error text or whole answer).
func atOp(c *client, name, raw string) (int, string, map[string]any) {
	c.f.t.Helper()
	code, out := c.do("POST", "/api/op/"+name, json.RawMessage(raw))
	return code, fmt.Sprint(out["error"]), out
}

// atMCP calls it as an MCP tool: (whether it is an error, its text).
func atMCP(c *client, name, raw string) (bool, string) {
	c.f.t.Helper()
	_, out := c.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": json.RawMessage(raw)}})
	res, ok := out["result"].(map[string]any)
	if !ok {
		return true, fmt.Sprint(out)
	}
	return res["isError"] == true, fmt.Sprint(res["content"])
}

// refused checks every route refuses a call, in the tester's words.
func (r testerRoutes) refused(t *testing.T, name, raw string) {
	t.Helper()
	for route, c := range map[string]*client{"page": r.page, "openapi": r.token} {
		if code, text, _ := atOp(c, name, raw); code != 403 || !strings.Contains(text, testerRefused) {
			t.Errorf("%s %s %s: %d %.200s", route, name, raw, code, text)
		}
	}
	for route, c := range map[string]*client{"mcp": r.token, "oauth": r.oauth} {
		if isErr, text := atMCP(c, name, raw); !isErr || !strings.Contains(text, testerRefused) {
			t.Errorf("%s %s %s: isError %v %.200s", route, name, raw, isErr, text)
		}
	}
}

// digest is every row of every table but the tokens' (whose last use moves).
func (f *fixture) digest() string {
	f.t.Helper()
	names, err := f.s.rowsOf(`SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'gm_tokens' ORDER BY name`)
	if err != nil {
		f.t.Fatal(err)
	}
	h := sha256.New()
	for _, n := range names {
		table := fmt.Sprint(n["name"])
		rows, err := f.s.rowsOf(`SELECT * FROM ` + quoteIdent(table))
		if err != nil {
			f.t.Fatal(err)
		}
		fmt.Fprintf(h, "%s %v\n", table, rows)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func auditRow(t *testing.T, f *fixture, id any) (user, via, action, tbl, before, after string) {
	t.Helper()
	var b, a sql.NullString
	if err := f.db.QueryRow(`SELECT username, via, action, tbl, before, after FROM gm_audit WHERE id = ?`, id).
		Scan(&user, &via, &action, &tbl, &b, &a); err != nil {
		t.Fatalf("audit line %v: %v", id, err)
	}
	return user, via, action, tbl, b.String, a.String
}

// The whole of a tester's way in with the live server's rule: the password
// alone at first and nothing but the enrolment, then an item and a spell
// changed, an audit line for each under the tester's name, the same in the
// running game as a GM's change, and a player in the game still.
func TestATesterEnrolsSignsInAndChangesAnItemAndASpell(t *testing.T) {
	f := newFixture(t)
	f.s.cfg.RequireTOTP = true
	f.addAccount(5, "tess", 0)
	login := map[string]any{"username": "tess", "password": testPassword}
	if code, out := f.browser().do("POST", "/api/auth/login", login); code != 401 || !strings.Contains(fmt.Sprint(out["error"]), "not a GM account") {
		t.Fatalf("before the role: %d %v", code, out)
	}
	f.setTester("tess", true)

	tess := f.browser()
	code, out := tess.do("POST", "/api/auth/login", login)
	if code != 200 {
		t.Fatalf("sign-in with the role: %d %v", code, out)
	}
	tess.csrf, _ = out["csrf"].(string)
	user := out["user"].(map[string]any)
	if user["role"] != "tester" || user["level"] != float64(0) || user["admin"] != false || user["adminNeedsTOTP"] != false {
		t.Fatalf("what the sign-in reports: %v", user)
	}
	if totp := out["totp"].(map[string]any); totp["required"] != true || totp["enabled"] != false {
		t.Fatalf("totp: %v", totp)
	}
	// nothing but the enrolment, by any route, as a GM
	if code, text, _ := atOp(tess, "query_rows", `{"table":"itempattern"}`); code != 403 || !strings.Contains(text, "authenticator") {
		t.Fatalf("before enrolling: %d %s", code, text)
	}
	if code, _ := tess.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "read", "days": 7}); code != 403 {
		t.Fatalf("a token before enrolling: %d", code)
	}
	if code, _ := f.browser().do("POST", "/api/auth/token", login); code != 403 {
		t.Fatalf("a terminal's token before enrolling: %d", code)
	}
	if _, tok := oauthTokens(t, f, "tess", "write", ""); tok["access_token"] != nil {
		t.Fatalf("an agent's grant before enrolling: %v", tok)
	}
	code, out = tess.do("POST", "/api/auth/totp", map[string]any{"action": "begin"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, out)
	}
	secret := out["secret"].(string)
	if code, out := tess.do("POST", "/api/auth/totp", map[string]any{"action": "confirm", "code": totpNow(secret)}); code != 200 {
		t.Fatalf("confirm: %d %v", code, out)
	}
	// the next sign-in asks for the code
	mustExec(t, f.db, `UPDATE gm_totp SET last_step = 0 WHERE accountid = 5`)
	if code, out := f.browser().do("POST", "/api/auth/login", login); code != 401 || out["needCode"] != true {
		t.Fatalf("a sign-in without the code: %d %v", code, out)
	}

	// with the code, every route's way in works: a terminal's token, an agent's grant
	code, out = f.browser().do("POST", "/api/auth/token", map[string]any{"username": "tess", "password": testPassword, "code": totpNow(secret), "name": "codex"})
	if code != 200 || out["scope"] != "write" {
		t.Fatalf("a terminal's token with the code: %d %v", code, out)
	}
	if code, text, _ := atOp(f.bearer(out["token"].(string)), "query_rows", `{"table":"itempattern"}`); code != 200 {
		t.Fatalf("the terminal's token: %d %s", code, text)
	}
	mustExec(t, f.db, `UPDATE gm_totp SET last_step = 0 WHERE accountid = 5`)
	_, tok := oauthTokens(t, f, "tess", "write", totpNow(secret))
	if tok["access_token"] == nil {
		t.Fatalf("an agent's grant with the code: %v", tok)
	}
	if isErr, text := atMCP(f.bearer(tok["access_token"].(string)), "query_rows", `{"table":"spellpattern"}`); isErr {
		t.Fatalf("the agent's grant at MCP: %s", text)
	}

	// an item
	code, _, res := atOp(tess, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"weapondamage":99,"itemname":"Sharp Sword"}}`)
	if code != 200 {
		t.Fatalf("change an item: %d %v", code, res)
	}
	who, via, action, tbl, before, after := auditRow(t, f, res["audit"])
	if who != "tess" || via != "web" || action != "update" || tbl != "itempattern" ||
		!strings.Contains(before, `"weapondamage":7`) || !strings.Contains(after, `"weapondamage":99`) {
		t.Fatalf("the audit line: %s %s %s %s %s %s", who, via, action, tbl, before, after)
	}
	var dmg int
	f.db.QueryRow(`SELECT weapondamage FROM itempattern WHERE patternid = 10`).Scan(&dmg)
	if dmg != 99 {
		t.Fatalf("the row: %d", dmg)
	}
	// a spell, and what the running game does is what it does for a GM
	code, _, res = atOp(tess, "update_row", `{"table":"spellpattern","key":{"spellid":100},"values":{"power":9}}`)
	if code != 200 {
		t.Fatalf("change a spell: %d %v", code, res)
	}
	if who, via, action, tbl, _, _ := auditRow(t, f, res["audit"]); who != "tess" || via != "web" || action != "update" || tbl != "spellpattern" {
		t.Fatalf("the spell's audit line: %s %s %s %s", who, via, action, tbl)
	}
	f.enrolTOTP(2)
	gm := f.browser()
	code, out = gm.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword, "code": totpNow(testTOTPSecret)})
	if code != 200 {
		t.Fatalf("the GM's sign-in: %d %v", code, out)
	}
	gm.csrf, _ = out["csrf"].(string)
	_, _, control := atOp(gm, "update_row", `{"table":"spellpattern","key":{"spellid":100},"values":{"power":10}}`)
	if res["live"] == nil || res["live"] != control["live"] {
		t.Errorf("a tester's change in the running game: %v; a GM's: %v", res["live"], control["live"])
	}
	// and the other tables of an item and a spell
	for _, raw := range []string{
		`{"table":"spelleffect","values":{"spellid":100,"line":"Self, Increase HP by 10","fxdict":0,"source":"tester"}}`,
		`{"table":"spellarea","values":{"spellid":100,"centre":2,"radius":10,"maxtargets":3,"note":"tried"}}`,
		`{"table":"itemeffect","values":{"patternid":11,"line":"Self, Increase HP by 10"}}`,
		`{"table":"foodeffect","values":{"patternid":11,"duration_s":60}}`,
	} {
		if code, _, res := atOp(tess, "insert_row", raw); code != 200 || res["audit"] == nil {
			t.Errorf("insert %s: %d %v", raw, code, res)
		}
	}
	if code, _, res := atOp(tess, "delete_row", `{"table":"spellarea","key":{"spellid":100}}`); code != 200 {
		t.Errorf("delete a spell's area: %d %v", code, res)
	}
	code, _, res = atOp(tess, "set_spell_visual", `{"spell":101,"visual":"0x1DAEB5B6"}`)
	if code != 200 {
		t.Fatalf("set_spell_visual: %d %v", code, res)
	}
	var source string
	f.db.QueryRow(`SELECT source FROM spelleffect WHERE spellid = 101`).Scan(&source)
	if source != "visual set by a tester" {
		t.Errorf("the row's source: %q", source)
	}
	// a copy of an item, read back
	code, _, res = atOp(tess, "clone_row", `{"table":"itempattern","key":{"patternid":10},"changes":{"itemname":"Sharp Sword II"}}`)
	if code != 200 {
		t.Fatalf("clone_row: %d %v", code, res)
	}
	if code, _, res := atOp(tess, "query_rows", `{"table":"itempattern","search":"Sharp Sword II"}`); code != 200 || res["total"] != float64(1) {
		t.Fatalf("read it back: %d %v", code, res)
	}

	// a GM reads every line under the tester's name
	code, _, res = atOp(gm, "audit_log", `{"username":"tess","table":"itempattern"}`)
	if code != 200 || res["total"] != float64(2) {
		t.Fatalf("a GM's log of the tester: %d %v", code, res)
	}

	// in the game a player: the level the GM commands read, and the row
	if lvl, err := f.s.d.Accounts.AccessLevel(5); err != nil || lvl != 0 {
		t.Errorf("AccessLevel, what handlers.isGM reads: %d %v", lvl, err)
	}
	if f.s.characterOfGM(105) {
		t.Error("the tester's character counts as a GM's")
	}
	if levelOf(t, f, 5) != 0 {
		t.Errorf("acctlevel %d", levelOf(t, f, 5))
	}
}

// Nothing of the game reads the role: it is the panel's alone.
func TestNothingInTheGameReadsTheTesters(t *testing.T) {
	for _, dir := range []string{"../handlers", "../entity", "../session", "../network", "../protocol", "../auth", "../charmove", "../transfers", "../../cmd"} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err == nil && (bytes.Contains(src, []byte("gm_testers")) || bytes.Contains(src, []byte("testerFlag"))) {
				t.Errorf("%s reads the testers' table", path)
			}
			return err
		})
		if err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
	}
}

// The allowlist itself, word for word: a change to it is a change to this.
func TestTheTestersAllowlistIsTheOneMeant(t *testing.T) {
	f := newFixture(t)
	want := []string{"audit_log", "clone_row", "count_rows", "delete_row", "describe_table", "fetch", "get_row", "guide", "insert_row",
		"list_tables", "lookup", "query_rows", "revert_change", "search", "server_status", "set_spell_visual", "spell_visual", "update_row", "visual_uses"}
	var got []string
	for name := range testerOps {
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
	wantTables := []string{"itempattern", "itemeffect", "foodeffect", "spellpattern", "spelleffect", "spellarea"}
	if !slices.Equal(testerTableList, wantTables) {
		t.Errorf("the tables: %v", testerTableList)
	}
	for _, name := range testerTableList {
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
			if c.Ref != "" && !isTesterTable(c.Ref) {
				t.Errorf("%s.%s refers to %s, which a tester cannot read", name, c.Name, c.Ref)
			}
		}
	}
	if !testerRecords["itempattern"] || !testerRecords["spellpattern"] || len(testerRecords) != 2 {
		t.Errorf("the records: %v", testerRecords)
	}
	// an admin's operation is never a tester's, whatever the list comes to say
	testerOps["sql_query"] = nil
	defer delete(testerOps, "sql_query")
	if testerMayCall(f.s.ops["sql_query"]) {
		t.Error("an admin's operation on the list is a tester's")
	}
	c := &opCtx{s: f.s, a: &actor{AccountID: 5, Username: "tess", Role: roleTester, Scope: scopeWrite}, raw: json.RawMessage(`{"sql":"SELECT 1"}`)}
	if err := c.testerGate(f.s.ops["sql_query"]); err == nil {
		t.Error("the gate let an admin's operation through")
	}
}

// Every registered operation not on the list, by every route, with a new one
// added now; nothing changes and nothing runs. A GM and an admin are not
// stopped by the gate (they get the operation's own answer).
func TestATesterIsRefusedEveryOtherOperation(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newTester()
	r := f.routesOf("tess")
	ran := 0
	f.s.addOp(&Op{Name: "zz_brand_new", Title: "New", Description: "added after the list was written", Scope: scopeRead,
		Handler: func(c *opCtx) (any, error) { ran++; return map[string]any{"ran": true}, nil }})
	f.s.addOp(&Op{Name: "zz_brand_new_write", Title: "New", Description: "added after the list was written", Scope: scopeWrite,
		Handler: func(c *opCtx) (any, error) { ran++; return map[string]any{"ran": true}, nil }})
	before := f.digest()
	refused := 0
	for _, op := range f.s.opList {
		if testerMayCall(op) {
			continue
		}
		refused++
		r.refused(t, op.Name, `{}`)
	}
	// the ones plainly a GM's, with arguments that would do it
	for name, raw := range map[string]string{
		"give_item":          `{"character":"tessHero","item":10}`,
		"grant_spell":        `{"character":"tessHero","spell":100}`,
		"set_character":      `{"character":"tessHero","level":60}`,
		"play_visual":        `{"visual":"0x1DAEB5B6"}`,
		"online_players":     `{}`,
		"set_account_level":  `{"account":"tess","level":2}`,
		"set_account_tester": `{"account":"player","tester":true}`,
		"sql_query":          `{"sql":"SELECT * FROM accountinfo"}`,
		"update_rows":        `{"table":"itempattern","filters":[{"column":"patternid","value":10}],"values":{"itemcost":1}}`,
		"delete_rows":        `{"table":"spellarea","search":"x"}`,
	} {
		r.refused(t, name, raw)
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
			if _, text, _ := atOp(c, op.Name, `{}`); strings.Contains(text, testerRefused) {
				t.Errorf("the control %s, %s: %s", who, op.Name, text)
			}
		}
	}
	if ran == 0 {
		t.Error("the control: a GM's new operation did not run")
	}
}

// Every operation that takes a table, with every table there is and one made
// now, by every route: refused but for a tester's own.
func TestATesterIsRefusedEveryOtherTable(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newTester()
	r := f.routesOf("tess")
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
	if all["zz_brand_new"] == nil || all["gm_totp"] == nil || all["accountinfo"] == nil {
		t.Fatal("the table list lost some")
	}
	before := f.digest()
	checked := 0
	for name := range all {
		for _, op := range tableOps {
			raw := fmt.Sprintf(`{"table":%q,"key":{"id":1},"values":{"id":1},"keys":[1],"filters":[{"column":"id","value":1}],"dry_run":true}`, name)
			if !isTesterTable(name) || !testerMayCall(op) {
				r.refused(t, op.Name, raw)
				checked++
				continue
			}
			if op.Name == "delete_row" && testerRecords[name] {
				for _, c := range []*client{r.page, r.token} {
					if code, text, _ := atOp(c, op.Name, raw); code != 403 || !strings.Contains(text, "does not delete") {
						t.Errorf("delete_row %s: %d %s", name, code, text)
					}
				}
				continue
			}
			// its own: past the gate (the row need not exist)
			for _, c := range []*client{r.page, r.token} {
				if code, text, _ := atOp(c, op.Name, raw); code == 403 || strings.Contains(text, testerRefused) {
					t.Errorf("%s %s: %d %s", op.Name, name, code, text)
				}
			}
		}
	}
	if checked < len(all)*len(tableOps)-len(testerTableList)*len(tableOps) {
		t.Errorf("only %d refusals of %d tables by %d operations: the loop lost them", checked, len(all), len(tableOps))
	}
	if f.digest() != before {
		t.Error("a refused call changed the database")
	}

	// a key spelled otherwise, or twice: the handler reads what the gate read
	for _, raw := range []string{
		`{"TABLE":"accountinfo"}`, `{"Table":"accountinfo"}`, `{"tAbLe":"accountinfo"}`,
		`{"table":"itempattern","TABLE":"accountinfo"}`, `{"table":"itempattern","table":"accountinfo"}`,
		`{"TABLE":"itempattern","table":"accountinfo"}`, `{"table":"itempattern","table":null,"Table":"accountinfo"}`,
		`null`, `{}`, `{"table":""}`, `{"table":"ACCOUNTINFO"}`, `{"table":"itempattern\u0000"}`, `{"table":"accountinfo "}`,
		`{"table":"itempattern; DROP TABLE accountinfo"}`,
	} {
		for _, name := range []string{"query_rows", "count_rows", "describe_table", "get_row", "lookup"} {
			r.refused(t, name, raw)
		}
	}
	// the other way about the last spelling wins, for both: its own table's rows
	code, _, out := atOp(r.page, "query_rows", `{"table":"accountinfo","TABLE":"itempattern"}`)
	if code != 200 || out["table"] != "itempattern" {
		t.Errorf("the last spelling: %d %v", code, out)
	}
	// not an object, or a table of the wrong kind: no rows of anything
	for _, raw := range []string{`[]`, `"accountinfo"`, `{"table":["accountinfo"]}`, `{"table":{"a":1}}`, `7`} {
		if code, _, out := atOp(r.page, "query_rows", raw); code == 200 || out["rows"] != nil {
			t.Errorf("%s: %d %v", raw, code, out)
		}
	}
	// the controls: a GM reads the tables a tester does not
	gm := f.browser()
	gm.login("gm")
	if code, _, out := atOp(gm, "query_rows", `{"table":"characters"}`); code != 200 || out["total"] == float64(0) {
		t.Errorf("the control, a GM: %d %v", code, out)
	}
}

// Both layers hold on their own: the gate with no handler behind it, and the
// handlers and the row editor for a tester with no gate in front of them.
func TestTheGateAndTheRowEditorEachHoldATester(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	a := &actor{AccountID: 5, Username: "tess", Role: roleTester, Scope: scopeWrite, Via: "test"}
	boss := f.browser()
	boss.login("boss")
	_, _, npcChange := atOp(boss, "update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_level":31}}`)
	npcEntry := npcChange["audit"]
	if npcEntry == nil {
		t.Fatalf("no audit line: %v", npcChange)
	}
	for _, op := range f.s.opList {
		props, _ := op.Input["properties"].(map[string]any)
		if props["table"] == nil {
			continue
		}
		raw := json.RawMessage(`{"table":"npcs","key":{"id":500},"values":{"npc_level":1},"keys":[500],"filters":[{"column":"id","value":500}]}`)
		c := &opCtx{s: f.s, a: a, raw: raw}
		if err := c.testerGate(op); err == nil || errStatus(err) != 403 {
			t.Errorf("the gate alone, %s: %v", op.Name, err)
		}
		if op.Name == "audit_log" {
			continue // its handler narrows the lines instead: below
		}
		if _, err := op.Handler(&opCtx{s: f.s, a: a, raw: raw}); err == nil || errStatus(err) != 403 {
			t.Errorf("the handler alone, %s: %v", op.Name, err)
		}
	}
	// every rule of the gate on its own, with nothing behind it to say no again
	var grant int64
	f.db.QueryRow(`SELECT MIN(id) FROM gm_audit WHERE tbl = 'gm_testers'`).Scan(&grant)
	_, _, added := atOp(boss, "clone_row", `{"table":"itempattern","key":{"patternid":10},"changes":{"itemname":"Theirs"}}`)
	addedEntry := int64(added["change"].(map[string]any)["audit"].(float64))
	for _, tc := range []struct{ name, raw string }{
		{"search", `{"query":"a","kinds":["character"]}`},
		{"fetch", `{"id":"account:1"}`},
		{"fetch", `{"id":"npc:500"}`},
		{"audit_log", `{"table":"npcs"}`},
		{"audit_log", `{"username":"boss"}`},
		{"audit_log", `{"search":"petition"}`},
		{"revert_change", fmt.Sprintf(`{"id":%d}`, int64(id0(npcEntry)))},
		{"revert_change", fmt.Sprintf(`{"id":%d}`, grant)},
		{"revert_change", fmt.Sprintf(`{"id":%d}`, addedEntry)},
		{"delete_row", `{"table":"itempattern","key":{"patternid":10}}`},
		{"delete_row", `{"table":"npcs","key":{"id":500}}`},
		{"update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"a|b"}}`},
		{"insert_row", `{"table":"npcs","values":{"npc_name":"x"}}`},
		{"give_item", `{"character":"tessHero","item":10}`},
	} {
		c := &opCtx{s: f.s, a: a, raw: json.RawMessage(tc.raw)}
		if err := c.testerGate(f.s.ops[tc.name]); err == nil || errStatus(err) != 403 {
			t.Errorf("the gate alone, %s %s: %v", tc.name, tc.raw, err)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"search", `{"query":"a","kinds":["item","spell"]}`},
		{"search", `{"query":"a"}`},
		{"fetch", `{"id":"item:10"}`},
		{"audit_log", `{"table":"itempattern"}`},
		{"audit_log", `{"username":"tess"}`},
		{"delete_row", `{"table":"spellarea","key":{"spellid":100}}`},
		{"update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"plain words"}}`},
		{"guide", `{}`},
	} {
		c := &opCtx{s: f.s, a: a, raw: json.RawMessage(tc.raw)}
		if err := c.testerGate(f.s.ops[tc.name]); err != nil {
			t.Errorf("the gate alone, %s %s: %v", tc.name, tc.raw, err)
		}
	}
	if _, err := f.s.ops["delete_row"].Handler(&opCtx{s: f.s, a: a, raw: json.RawMessage(`{"table":"characters","key":{"serverid":7}}`)}); err == nil || errStatus(err) != 403 {
		t.Errorf("the handler alone, a character: %v", err)
	}
	for _, id := range []string{"account:1", "character:7", "npc:500"} {
		if _, err := f.s.ops["fetch"].Handler(&opCtx{s: f.s, a: a, raw: json.RawMessage(fmt.Sprintf(`{"id":%q}`, id))}); err == nil || errStatus(err) != 403 {
			t.Errorf("the handler alone, fetch %s: %v", id, err)
		}
	}
	// the row editor, whatever calls it
	for _, name := range []string{"accountinfo", "characters", "charinventory", "npcs", "npcitem", "classspells", "spellgates", "gm_testers", "gm_tokens", "zz"} {
		tb := &Table{Name: name, Write: writeGM}
		if err := mayWrite(a, tb); err == nil || !strings.Contains(err.Error(), testerRefused) {
			t.Errorf("mayWrite %s: %v", name, err)
		}
	}
	for _, name := range testerTableList {
		if err := mayWrite(a, &Table{Name: name, Write: writeGM}); err != nil {
			t.Errorf("mayWrite %s: %v", name, err)
		}
	}
	if err := mayWrite(&actor{Role: roleGM, Scope: scopeWrite}, &Table{Name: "npcs", Write: writeGM}); err != nil {
		t.Errorf("the control, a GM: %v", err)
	}
	// revert, past the gate
	id, _ := npcEntry.(float64)
	if _, err := f.s.revert(a, int64(id), false, false); err == nil || !strings.Contains(err.Error(), testerRefused) {
		t.Errorf("revert of an NPC's change, no gate: %v", err)
	}
	// anyone below a GM is gated at run(), whatever the actor says of itself
	if _, err := f.s.run(&actor{AccountID: 3, Username: "player", Scope: scopeWrite}, "online_players", nil, nil); err == nil || !strings.Contains(err.Error(), testerRefused) {
		t.Errorf("a player's actor at run(): %v", err)
	}
}

// id0 is an audit id as an answer carries it.
func id0(v any) float64 {
	id, _ := v.(float64)
	return id
}

// A player without the role stays out of every way in, and a credential it
// somehow holds does nothing.
func TestAPlayerWithoutTheRoleIsOutEveryWay(t *testing.T) {
	f := newFixture(t)
	f.newTester() // someone else's role changes nothing for the player
	if code, out := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "player", "password": testPassword}); code != 401 ||
		!strings.Contains(fmt.Sprint(out["error"]), "not a GM account") {
		t.Errorf("the page: %d %v", code, out)
	}
	if code, out := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "player", "password": testPassword}); code != 401 {
		t.Errorf("a terminal: %d %v", code, out)
	}
	if _, tok := oauthTokens(t, f, "player", "read", ""); tok["access_token"] != nil {
		t.Errorf("OAuth: %v", tok)
	}
	value, _, err := f.s.issueToken(3, "planted", kindPersonal, scopeWrite, "", time.Hour, 0)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := atOp(f.bearer(value), "query_rows", `{"table":"itempattern"}`); code != 401 {
		t.Errorf("a token it should not have: %d", code)
	}
	sess := f.s.sessions.create(3, "player", "127.0.0.1", true)
	c := f.browser()
	c.csrf = sess.CSRF
	u, _ := url.Parse(f.ts.URL)
	c.http.Jar.SetCookies(u, []*http.Cookie{{Name: f.s.cookieName(), Value: sess.Token, Path: "/"}})
	if code, _, _ := atOp(c, "query_rows", `{"table":"itempattern"}`); code != 401 {
		t.Errorf("a session it should not have: %d", code)
	}
	// and the role on a banned account is nothing
	mustExec(t, f.db, `UPDATE accountinfo SET acctstatus = -1 WHERE accountid = 5`)
	if code, _ := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "tess", "password": testPassword}); code != 401 {
		t.Errorf("a banned tester: %d", code)
	}
}

// What a tester is shown: its tables and operations, by every way of asking.
func TestATesterIsShownOnlyWhatItCanUse(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	r := f.routesOf("tess")
	allowed := []string{}
	var readOnly []string
	for _, op := range f.s.opList {
		if testerMayCall(op) {
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

	// a sign-in's report
	code, sess := r.page.do("GET", "/api/auth/session", nil)
	if code != 200 || sess["user"].(map[string]any)["role"] != "tester" || sess["server"].(map[string]any)["restart"] != false {
		t.Fatalf("the session: %d %v", code, sess)
	}
	if tables := sess["tester"].(map[string]any)["tables"].([]any); len(tables) != len(testerTableList) {
		t.Errorf("the session's tables: %v", tables)
	}
	// the operations
	code, out := r.page.do("GET", "/api/ops", nil)
	if got := names(out["ops"].([]any), "name"); code != 200 || !slices.Equal(got, allowed) {
		t.Errorf("api/ops: %d %v", code, got)
	}
	// MCP
	_, out = r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if got := names(out["result"].(map[string]any)["tools"].([]any), "name"); !slices.Equal(got, allowed) {
		t.Errorf("MCP's tools: %v", got)
	}
	readTok := f.bearer(r.page.workToken("r", "read"))
	_, out = readTok.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if got := names(out["result"].(map[string]any)["tools"].([]any), "name"); !slices.Equal(got, readOnly) {
		t.Errorf("a read token's tools: %v", got)
	}
	if code, text, _ := atOp(readTok, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":2}}`); code != 403 || !strings.Contains(text, "read-only") {
		t.Errorf("a read token's write: %d %s", code, text)
	}
	_, out = r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	instr := out["result"].(map[string]any)["instructions"].(string)
	if !strings.Contains(instr, "tester") || strings.Contains(instr, "work_board") || strings.Contains(instr, "game master") {
		t.Errorf("MCP's instructions: %s", instr)
	}
	// OpenAPI: a tester's credential, or the address a custom GPT imports
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
		t.Errorf("openapi with a tester's token: %v", got)
	}
	if got := paths(f.browser(), "?role=tester"); !slices.Equal(got, allowed) {
		t.Errorf("openapi?role=tester: %v", got)
	}
	core := paths(f.browser(), "?role=tester&core=1")
	if len(core) == 0 {
		t.Error("openapi?role=tester&core=1 is empty")
	}
	for _, n := range core {
		if !slices.Contains(allowed, n) {
			t.Errorf("the tester's short list has %s", n)
		}
	}
	if got := paths(f.browser(), ""); len(got) != len(f.s.opList) {
		t.Errorf("the control, the plain document: %d of %d", len(got), len(f.s.opList))
	}
	// the guide is a tester's, every operation it names one it has, and written
	// whole: the recipes it borrows are all there
	_, _, g := atOp(r.page, "guide", `{}`)
	guide := fmt.Sprint(g["guide"])
	if !strings.HasPrefix(guide, "# EQOA tester guide") || strings.Contains(guide, "{{") || !strings.Contains(guide, f.ts.URL+"/mcp") {
		t.Errorf("the guide: %.300s", guide)
	}
	for _, op := range f.s.opList {
		if !testerMayCall(op) && strings.Contains(guide, "`"+op.Name+"`") {
			t.Errorf("the guide names %s", op.Name)
		}
	}
	for _, title := range testerGuideRecipes {
		if guideBullet(title) == "" || !strings.Contains(guide, "- **"+title) {
			t.Errorf("the guide lacks the recipe %q", title)
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
	if !strings.HasPrefix(string(body), "# EQOA tester guide") {
		t.Errorf("api/guide with a tester's token: %.200s", body)
	}
	resp, _ = http.Get(f.ts.URL + "/api/guide")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "work_board") {
		t.Errorf("the control, the plain guide: %.200s", body)
	}
	// server_status: who, and no state of the server's
	_, _, st := atOp(r.page, "server_status", `{}`)
	for _, k := range []string{"playersOnline", "npcsInWorld", "sessions", "settings", "databaseBytes", "gmSpawns", "worlds"} {
		if _, there := st[k]; there {
			t.Errorf("server_status says %s", k)
		}
	}
	if c := st["counts"].(map[string]any); c["items"] == nil || c["spells"] == nil || c["accounts"] != nil || c["characters"] != nil || c["auditEntries"] != nil {
		t.Errorf("server_status's counts: %v", c)
	}
	if you := st["you"].(map[string]any); you["role"] != "tester" || you["username"] != "tess" {
		t.Errorf("server_status's you: %v", you)
	}
	// the catalog
	_, _, lt := atOp(r.page, "list_tables", `{}`)
	got := names(lt["tables"].([]any), "name")
	want := slices.Clone(testerTableList)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("list_tables: %v", got)
	}
	for _, tb := range lt["tables"].([]any) {
		if tb.(map[string]any)["write"] != "tester" {
			t.Errorf("list_tables' write: %v", tb)
		}
	}
	for _, name := range testerTableList {
		_, _, d := atOp(r.page, "describe_table", fmt.Sprintf(`{"table":%q}`, name))
		if d["write"] != "tester" || d["delete"] != !testerRecords[name] {
			t.Errorf("describe_table %s: write %v delete %v", name, d["write"], d["delete"])
		}
	}
	// search finds items and spells, and nobody and nothing else
	kinds := func(raw string) map[string]int {
		code, text, out := atOp(r.page, "search", raw)
		if code != 200 {
			t.Fatalf("search %s: %d %s", raw, code, text)
		}
		found := map[string]int{}
		for _, v := range out["results"].([]any) {
			found[v.(map[string]any)["kind"].(string)]++
		}
		return found
	}
	if got := kinds(`{"query":"Rusty"}`); got["item"] != 1 || len(got) != 1 {
		t.Errorf("search Rusty: %v", got)
	}
	if got := kinds(`{"query":"Healing"}`); got["spell"] != 1 || len(got) != 1 {
		t.Errorf("search Healing: %v", got)
	}
	for _, q := range []string{"Guard", "Hero", "boss", "player"} {
		if got := kinds(fmt.Sprintf(`{"query":%q}`, q)); len(got) != 0 {
			t.Errorf("search %s: %v", q, got)
		}
	}
	if got := kinds(`{"query":"1"}`); got["account"]+got["character"]+got["npc"] != 0 {
		t.Errorf("search by a number: %v", got)
	}
	for _, kind := range []string{"account", "character", "npc"} {
		r.refused(t, "search", fmt.Sprintf(`{"query":"a","kinds":["item",%q]}`, kind))
	}
	// the control
	gm := f.browser()
	gm.login("gm")
	if _, _, out := atOp(gm, "search", `{"query":"Guard"}`); len(out["results"].([]any)) == 0 {
		t.Error("the control: a GM's search found no NPC")
	}
	// fetch: an item and a spell, and the record alone
	_, _, it := atOp(r.page, "fetch", `{"id":"item:11"}`)
	rec := it["record"].(map[string]any)
	if rec["soldBy"] != nil || rec["heldByCharacters"] != nil || strings.Contains(fmt.Sprint(it["text"]), "Merchant Shadehand") {
		t.Errorf("fetch of an item: %v", it)
	}
	if rec["item"] == nil {
		t.Errorf("fetch of an item has no record: %v", it)
	}
	_, _, sp := atOp(r.page, "fetch", `{"id":"spell:100"}`)
	rec = sp["record"].(map[string]any)
	if rec["knownByCharacters"] != nil || rec["spell"] == nil || !strings.Contains(fmt.Sprint(rec["classes"]), "Warrior") {
		t.Errorf("fetch of a spell: %v", sp)
	}
	for _, id := range []string{"account:1", "character:7", "npc:500", " npc:500", "npc", ""} {
		r.refused(t, "fetch", fmt.Sprintf(`{"id":%q}`, id))
	}
	// the control: a GM sees who sells it
	if _, _, out := atOp(gm, "fetch", `{"id":"item:11"}`); !strings.Contains(fmt.Sprint(out["text"]), "Merchant Shadehand") {
		t.Errorf("the control: %v", out)
	}
}

// A tester's log: the changes to its tables, others' without who or from where.
func TestATestersAuditLogIsItsTablesWithoutOtherPeople(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	r := f.routesOf("tess")
	boss := f.browser()
	boss.login("boss")
	atOp(boss, "update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_level":31}}`)
	atOp(boss, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":50}}`)
	mustExec(t, f.db, `UPDATE gm_audit SET ip = '203.0.113.9', detail = 'petition 7, Somebody''s words' WHERE username = 'boss'`)
	atOp(r.page, "update_row", `{"table":"itempattern","key":{"patternid":11},"values":{"itemcost":60}}`)

	code, _, out := atOp(r.page, "audit_log", `{}`)
	if code != 200 {
		t.Fatalf("audit_log: %d %v", code, out)
	}
	entries := out["entries"].([]any)
	if len(entries) != 2 || out["total"] != float64(2) {
		t.Fatalf("%d entries of %v: %v", len(entries), out["total"], entries)
	}
	for _, v := range entries {
		e := v.(map[string]any)
		if !isTesterTable(fmt.Sprint(e["table"])) {
			t.Errorf("another table: %v", e)
		}
		if e["username"] == "tess" {
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
	raw, _ := json.Marshal(entries)
	if text := string(raw); strings.Contains(text, "203.0.113.9") || strings.Contains(text, "boss") || strings.Contains(text, "Somebody") {
		t.Errorf("it shows another person: %s", text)
	}
	// by row, as the page's history asks
	if code, _, out := atOp(r.page, "audit_log", `{"table":"itempattern","key":{"patternid":10}}`); code != 200 || out["total"] != float64(1) {
		t.Errorf("by row: %d %v", code, out)
	}
	if code, _, out := atOp(r.page, "audit_log", `{"username":"TESS"}`); code != 200 || out["total"] != float64(1) {
		t.Errorf("its own: %d %v", code, out)
	}
	// the sign-ins and tokens are no line of a tester's tables
	if _, _, out := atOp(r.page, "audit_log", `{"action":"login"}`); out["total"] != float64(0) {
		t.Errorf("sign-ins: %v", out)
	}
	for _, raw := range []string{`{"table":"npcs"}`, `{"table":"accountinfo","key":{"accountid":1}}`, `{"username":"boss"}`, `{"search":"petition"}`,
		`{"TABLE":"npcs"}`, `{"table":"itempattern","Table":"npcs"}`} {
		r.refused(t, "audit_log", raw)
	}
	// the control: a GM reads who and from where
	gm := f.browser()
	gm.login("gm")
	if _, _, out := atOp(gm, "audit_log", `{"username":"boss","table":"itempattern"}`); !strings.Contains(fmt.Sprint(out["entries"]), "203.0.113.9") {
		t.Errorf("the control: %v", out)
	}
}

// A tester's text reaches every client: no bar (a run of them kills a PS2 that
// opens it) and no megabytes, by every route and every way of writing a row.
func TestATestersTextIsWhatAClientCanDraw(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	r := f.routesOf("tess")
	bars := strings.Repeat("|+", 180)
	long := strings.Repeat("x", testerMaxText+1)
	before := f.digest()
	for _, raw := range []string{
		fmt.Sprintf(`{"table":"itempattern","key":{"patternid":10},"values":{"itemdesc":%q}}`, bars),
		`{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"a|b"}}`,
		`{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"a\u007cb"}}`,
		fmt.Sprintf(`{"table":"itempattern","key":{"patternid":10},"VALUES":{"itemdesc":%q}}`, bars),
		fmt.Sprintf(`{"table":"itempattern","key":{"patternid":10},"values":{"itemdesc":"fine"},"Values":{"itemname":%q}}`, bars),
		fmt.Sprintf(`{"table":"spellpattern","key":{"spellid":100},"values":{"spelldesc":%q}}`, long),
		fmt.Sprintf(`{"table":"spelleffect","values":{"spellid":100,"line":%q,"fxdict":0,"source":"x"}}`, bars),
		fmt.Sprintf(`{"table":"itempattern","key":{"patternid":10},"changes":{"itemdesc":%q}}`, bars),
	} {
		name := "update_row"
		if strings.Contains(raw, `"changes"`) {
			name = "clone_row"
		} else if strings.Contains(raw, "spelleffect") {
			name = "insert_row"
		}
		for route, c := range map[string]*client{"page": r.page, "openapi": r.token} {
			if code, text, _ := atOp(c, name, raw); code != 403 || !strings.Contains(text, "a tester's text") {
				t.Errorf("%s %s %.100s: %d %s", route, name, raw, code, text)
			}
		}
		if isErr, text := atMCP(r.oauth, name, raw); !isErr || !strings.Contains(text, "a tester's text") {
			t.Errorf("mcp %s %.100s: %v %s", name, raw, isErr, text)
		}
	}
	if f.digest() != before {
		t.Error("a refused write changed the database")
	}
	// up to the limit, and what a PS2 draws, is written
	ok := strings.Repeat("y", testerMaxText)
	if code, text, _ := atOp(r.page, "update_row", fmt.Sprintf(`{"table":"itempattern","key":{"patternid":10},"values":{"itemdesc":%q}}`, ok)); code != 200 {
		t.Errorf("text at the limit: %d %s", code, text)
	}
	// the control: a GM's text is a GM's
	gm := f.browser()
	gm.login("gm")
	if code, text, _ := atOp(gm, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemname":"a|b"}}`); code != 200 {
		t.Errorf("the control: %d %s", code, text)
	}
}

// Revert: changes to a tester's tables, and never one that takes away a record
// it did not add.
func TestATesterRevertsOnlyChangesToItsTables(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	r := f.routesOf("tess")
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
			t.Fatalf("no audit line in %v", out)
		}
		return fmt.Sprintf(`{"id":%d}`, int64(id))
	}
	// not its tables
	npc := entry(boss, "update_row", `{"table":"npcs","key":{"id":500},"values":{"npc_level":31}}`)
	char := entry(boss, "update_row", `{"table":"characters","key":{"serverid":7},"values":{"tunar":5}}`)
	before := f.digest()
	r.refused(t, "revert_change", npc)
	r.refused(t, "revert_change", char)
	var login int64
	f.db.QueryRow(`SELECT MIN(id) FROM gm_audit WHERE action = 'login'`).Scan(&login)
	r.refused(t, "revert_change", fmt.Sprintf(`{"id":%d}`, login))
	var grant int64
	f.db.QueryRow(`SELECT MIN(id) FROM gm_audit WHERE tbl = 'gm_testers'`).Scan(&grant)
	r.refused(t, "revert_change", fmt.Sprintf(`{"id":%d}`, grant))
	r.refused(t, "revert_change", `{"id":`+fmt.Sprint(login)+`,"force":true,"dry_run":true}`)
	if f.digest() != before {
		t.Error("a refused revert changed the database")
	}
	if code, _, _ := atOp(r.page, "revert_change", `{"id":99999}`); code != 404 {
		t.Errorf("no such entry: %d", code)
	}

	// its tables: another's change, and its own
	item := entry(boss, "update_row", `{"table":"itempattern","key":{"patternid":10},"values":{"itemcost":50}}`)
	if code, _, out := atOp(r.page, "revert_change", item); code != 200 {
		t.Fatalf("revert a GM's change to an item: %d %v", code, out)
	}
	var cost int
	f.db.QueryRow(`SELECT itemcost FROM itempattern WHERE patternid = 10`).Scan(&cost)
	if cost != 5 {
		t.Errorf("the item's value: %d", cost)
	}
	own := entry(r.page, "update_row", `{"table":"spellpattern","key":{"spellid":100},"values":{"power":77}}`)
	if code, _, out := atOp(r.page, "revert_change", own); code != 200 {
		t.Fatalf("revert its own: %d %v", code, out)
	}
	// a record it added goes; one a GM added does not
	code, _, out := atOp(r.page, "clone_row", `{"table":"itempattern","key":{"patternid":10},"changes":{"itemname":"Mine"}}`)
	if code != 200 {
		t.Fatalf("clone: %d %v", code, out)
	}
	ch := out["change"].(map[string]any)
	if code, _, out := atOp(r.page, "revert_change", fmt.Sprintf(`{"id":%d}`, int64(ch["audit"].(float64)))); code != 200 {
		t.Fatalf("revert its own new record: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE itemname = 'Mine'`).Scan(&n)
	if n != 0 {
		t.Error("its new record stayed")
	}
	code, _, out = atOp(boss, "clone_row", `{"table":"itempattern","key":{"patternid":10},"changes":{"itemname":"Theirs"}}`)
	if code != 200 {
		t.Fatalf("clone: %d %v", code, out)
	}
	theirs := fmt.Sprintf(`{"id":%d}`, int64(out["change"].(map[string]any)["audit"].(float64)))
	if code, text, _ := atOp(r.page, "revert_change", theirs); code != 403 || !strings.Contains(text, "did not add") {
		t.Errorf("revert a GM's new record: %d %s", code, text)
	}
	_, _, log := atOp(r.page, "audit_log", `{"action":"insert","table":"itempattern"}`)
	noRevert := 0
	for _, v := range log["entries"].([]any) {
		if v.(map[string]any)["noRevert"] == true {
			noRevert++
		}
	}
	if noRevert != 1 {
		t.Errorf("the log marks %d of its lines as not revertible: %v", noRevert, log["entries"])
	}
	// deleting a whole record is a GM's, by every route; a leaf's is its own
	for _, raw := range []string{`{"table":"itempattern","key":{"patternid":10}}`, `{"table":"spellpattern","key":{"spellid":100}}`,
		`{"TABLE":"spellpattern","key":{"spellid":100}}`} {
		if code, text, _ := atOp(r.page, "delete_row", raw); code != 403 || !strings.Contains(text, "does not delete") {
			t.Errorf("delete_row %s: %d %s", raw, code, text)
		}
		if isErr, text := atMCP(r.oauth, "delete_row", raw); !isErr || !strings.Contains(text, "does not delete") {
			t.Errorf("delete_row %s at MCP: %v %s", raw, isErr, text)
		}
	}
	// the control: a GM reverts the NPC's change
	if code, _, out := atOp(boss, "revert_change", npc); code != 200 {
		t.Errorf("the control: %d %v", code, out)
	}
}

// The admin scope: refused at every way to a credential, at the one place they
// all come to, and an old one is not good on a tester's account.
func TestATesterCannotHoldTheAdminScope(t *testing.T) {
	f := newFixture(t)
	f.s.cfg.AdminLevel = 1 // the live server's: every GM an admin with a code
	f.newTester()
	f.enrolTOTP(5)
	page := f.browser()
	code, out := page.do("POST", "/api/auth/login", map[string]any{"username": "tess", "password": testPassword, "code": totpNow(testTOTPSecret)})
	if code != 200 {
		t.Fatalf("sign-in with a code: %d %v", code, out)
	}
	page.csrf, _ = out["csrf"].(string)
	if out["user"].(map[string]any)["admin"] != false {
		t.Errorf("a tester with a code is an admin: %v", out["user"])
	}
	if code, _ := page.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "admin", "days": 7}); code != 403 {
		t.Errorf("the page's token: %d", code)
	}
	f.enrolTOTP(5)
	if code, _ := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "tess", "password": testPassword,
		"code": totpNow(testTOTPSecret), "scope": "admin"}); code != 403 {
		t.Errorf("a terminal's token: %d", code)
	}
	f.enrolTOTP(5)
	if _, tok := oauthTokens(t, f, "tess", "admin", totpNow(testTOTPSecret)); tok["access_token"] != nil || tok["_status"] != 403 {
		t.Errorf("OAuth's: %v", tok["access_token"])
	}
	if _, _, err := f.s.issueToken(5, "direct", kindPersonal, scopeAdmin, "", time.Hour, 0); err == nil {
		t.Error("issueToken wrote the admin scope for a tester")
	}
	// a grant asked for with every scope is offered write; a refresh does not raise it
	f.enrolTOTP(5)
	clientID, tok := oauthTokens(t, f, "tess", "write", totpNow(testTOTPSecret))
	if tok["scope"] != "write" {
		t.Fatalf("the grant: %v", tok)
	}
	resp, err := http.PostForm(f.ts.URL+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID},
		"refresh_token": {tok["refresh_token"].(string)}, "scope": {"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	var again map[string]any
	json.NewDecoder(resp.Body).Decode(&again)
	resp.Body.Close()
	if again["scope"] != "write" {
		t.Errorf("a refresh asking for admin: %v", again)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_tokens WHERE accountid = 5 AND scope = 'admin'`).Scan(&n)
	if n != 0 {
		t.Errorf("%d admin-scope tokens on a tester", n)
	}
	// an admin token from when the account was an admin's, by no route's check
	value := newTokenValue("gmt_")
	mustExec(t, f.db, `INSERT INTO gm_tokens (accountid, name, kind, hash, prefix, scope, created, expires)
		VALUES (5, 'old', 'token', ?, 'gmt_xx', 'admin', ?, ?)`, hashToken(value), now(), stamp(time.Now().Add(time.Hour)))
	if code, _, _ := atOp(f.bearer(value), "query_rows", `{"table":"itempattern"}`); code != 401 {
		t.Errorf("an old admin token on a tester: %d", code)
	}
	// the control: an admin signed in with a code has one
	f.enrolTOTP(1)
	code, out = f.browser().do("POST", "/api/auth/token", map[string]any{"username": "boss", "password": testPassword,
		"code": totpNow(testTOTPSecret), "scope": "admin"})
	if code != 200 || out["scope"] != "admin" {
		t.Errorf("the control, an admin: %d %v", code, out)
	}
}

// Taking the role away: at once, on a session, a token, an OAuth grant and its
// refresh; and what was made under it does not wake at a second grant.
func TestTakingTheRoleAwayCountsAtOnce(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	r := f.routesOf("tess")
	page2 := f.browser()
	page2.login("tess")
	clientID, tok := oauthTokens(t, f, "tess", "write", "")
	for _, c := range []*client{r.page, page2, r.token, r.oauth, f.bearer(tok["access_token"].(string))} {
		if code, text, _ := atOp(c, "server_status", `{}`); code != 200 {
			t.Fatalf("before: %d %s", code, text)
		}
	}
	out := f.setTester("tess", false)
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "no longer a tester") {
		t.Errorf("the answer's live: %s", live)
	}
	for route, c := range map[string]*client{"page": r.page, "second window": page2, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after: %d", route, code)
		}
	}
	if code, _ := r.token.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}); code != 401 {
		t.Errorf("MCP after: %d", code)
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
	var live int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_tokens WHERE accountid = 5 AND revoked = 0`).Scan(&live)
	if live != 0 {
		t.Errorf("%d live tokens after", live)
	}
	if code, _ := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "tess", "password": testPassword}); code != 401 {
		t.Errorf("a sign-in after: %d", code)
	}
	// audited
	var who, via, detail string
	f.db.QueryRow(`SELECT username, via, detail FROM gm_audit WHERE tbl = 'gm_testers' AND action = 'delete' ORDER BY id DESC LIMIT 1`).Scan(&who, &via, &detail)
	if who != "boss" || !strings.Contains(detail, "took the tester role from tess") {
		t.Errorf("the audit line: %s %s %s", who, via, detail)
	}
	// a second grant: a new sign-in, the old tokens dead
	f.setTester("tess", true)
	if code, _, _ := atOp(r.token, "server_status", `{}`); code != 401 {
		t.Errorf("an old token after a second grant: %d", code)
	}
	r = f.routesOf("tess")
	if code, text, _ := atOp(r.page, "query_rows", `{"table":"itempattern"}`); code != 200 {
		t.Errorf("a new sign-in after a second grant: %d %s", code, text)
	}
	// the row taken away by a tool (grant_testers.py --undo), nothing revoked:
	// out at the next request, and a refresh token still live gets nothing
	clientID, tok = oauthTokens(t, f, "tess", "write", "")
	mustExec(t, f.db, `DELETE FROM gm_testers WHERE accountid = 5`)
	for route, c := range map[string]*client{"page": r.page, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after the row went: %d", route, code)
		}
	}
	resp, err = http.PostForm(f.ts.URL+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID},
		"refresh_token": {tok["refresh_token"].(string)}})
	if err != nil {
		t.Fatal(err)
	}
	refreshed = nil
	json.NewDecoder(resp.Body).Decode(&refreshed)
	resp.Body.Close()
	if refreshed["access_token"] != nil {
		t.Errorf("a live refresh token after the row went: %v", refreshed)
	}
	// and a ban
	mustExec(t, f.db, `INSERT INTO gm_testers (accountid) VALUES (5)`)
	r = f.routesOf("tess")
	boss := f.browser()
	boss.login("boss")
	if code, _, out := atOp(boss, "ban_account", `{"account":"tess","banned":true}`); code != 200 {
		t.Fatalf("ban: %d %v", code, out)
	}
	for route, c := range map[string]*client{"page": r.page, "token": r.token, "oauth": r.oauth} {
		if code, _, _ := atOp(c, "server_status", `{}`); code != 401 {
			t.Errorf("%s after a ban: %d", route, code)
		}
	}
}

// Giving and taking the role is an admin's, audited and revertible.
func TestTheRoleIsAnAdminsToGive(t *testing.T) {
	f := newFixture(t)
	f.addAccount(5, "tess", 0)
	gm := f.browser()
	gm.login("gm")
	if code, _, _ := atOp(gm, "set_account_tester", `{"account":"tess","tester":true}`); code != 403 {
		t.Errorf("a GM: %d", code)
	}
	if code, _, _ := atOp(gm, "insert_row", `{"table":"gm_testers","values":{"accountid":5}}`); code == 200 {
		t.Error("a GM's row edit")
	}
	boss := f.browser()
	boss.login("boss")
	writeTok := f.bearer(boss.workToken("w", "write"))
	if code, _, _ := atOp(writeTok, "set_account_tester", `{"account":"tess","tester":true}`); code != 403 {
		t.Errorf("an admin's write token: %d", code)
	}
	code, _, out := atOp(boss, "set_account_tester", `{"account":"tess","tester":true,"note":"tries the shamans"}`)
	if code != 200 {
		t.Fatalf("an admin: %d %v", code, out)
	}
	ch := out["change"].(map[string]any)
	who, via, action, tbl, before, after := auditRow(t, f, ch["audit"])
	if who != "boss" || via != "web" || action != "insert" || tbl != "gm_testers" || before != "" || !strings.Contains(after, `"accountid":5`) {
		t.Errorf("the audit line: %s %s %s %s %s %s", who, via, action, tbl, before, after)
	}
	var detail, by, note string
	f.db.QueryRow(`SELECT detail FROM gm_audit WHERE id = ?`, ch["audit"]).Scan(&detail)
	if !strings.Contains(detail, "gave tess the tester role") || !strings.Contains(detail, "tries the shamans") {
		t.Errorf("the detail: %s", detail)
	}
	f.db.QueryRow(`SELECT granted_by, note FROM gm_testers WHERE accountid = 5`).Scan(&by, &note)
	if by != "boss" || note != "tries the shamans" {
		t.Errorf("the row: %s %s", by, note)
	}
	if levelOf(t, f, 5) != 0 {
		t.Errorf("the role changed the level: %d", levelOf(t, f, 5))
	}
	if _, _, again := atOp(boss, "set_account_tester", `{"account":"tess","tester":true}`); again["unchanged"] != true {
		t.Errorf("a second grant: %v", again)
	}
	// a tester gives nothing
	tess := f.browser()
	tess.login("tess")
	if code, _, _ := atOp(tess, "set_account_tester", `{"account":"player","tester":true}`); code != 403 {
		t.Errorf("a tester gives it: %d", code)
	}
	// who it cannot go on
	for _, raw := range []string{`{"account":"gm","tester":true}`, `{"account":"boss","tester":true}`, `{"account":"bannedgm","tester":true}`,
		`{"account":"nobody","tester":true}`, `{"account":"tess"}`} {
		if code, _, out := atOp(boss, "set_account_tester", raw); code == 200 {
			t.Errorf("%s: %d %v", raw, code, out)
		}
	}
	// the account's page reads it, and search says so
	if _, _, acc := atOp(boss, "fetch", `{"id":"account:5"}`); acc["record"].(map[string]any)["tester"] != true {
		t.Errorf("the account's record: %v", acc)
	}
	if _, _, found := atOp(boss, "search", `{"query":"tess","kinds":["account"]}`); !strings.Contains(fmt.Sprint(found["results"]), "tester") {
		t.Errorf("search: %v", found)
	}
	// a revert takes it back, and counts at once
	id := int64(ch["audit"].(float64))
	if code, _, out := atOp(boss, "revert_change", fmt.Sprintf(`{"id":%d}`, id)); code != 200 {
		t.Fatalf("revert the grant: %d %v", code, out)
	}
	if code, _, _ := atOp(tess, "server_status", `{}`); code != 401 {
		t.Errorf("a session after the grant's revert: %d", code)
	}
	// and the catalog keeps a GM's edit out, an admin's in
	tb, err := f.s.table("gm_testers")
	if err != nil || tb.Write != writeAdmin || tb.Hidden {
		t.Errorf("the catalog: %v %v", tb, err)
	}
}

// A GM's and an admin's powers: what they may call and whom they are, untouched.
func TestGMsAndAdminsAreNotTesters(t *testing.T) {
	f := newFixture(t)
	f.unlimit()
	f.newTester()
	count := func(c *client) int {
		_, out := c.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
		return len(out["result"].(map[string]any)["tools"].([]any))
	}
	admins, writes := 0, 0
	for _, op := range f.s.opList {
		if op.Scope == scopeAdmin {
			admins++
		} else {
			writes++
		}
	}
	gm := f.browser()
	gm.login("gm")
	if got := count(f.bearer(gm.workToken("g", "write"))); got != writes {
		t.Errorf("a GM's tools: %d of %d", got, writes)
	}
	boss := f.browser()
	boss.login("boss")
	if got := count(f.bearer(boss.workToken("b", "admin"))); got != writes+admins {
		t.Errorf("an admin's tools: %d of %d", got, writes+admins)
	}
	for who, c := range map[string]*client{"gm": gm, "boss": boss} {
		_, sess := c.do("GET", "/api/auth/session", nil)
		want := map[string]string{"gm": "gm", "boss": "admin"}[who]
		if sess["user"].(map[string]any)["role"] != want || sess["tester"] != nil {
			t.Errorf("%s's session: %v", who, sess["user"])
		}
		_, _, st := atOp(c, "server_status", `{}`)
		if st["settings"] == nil || st["counts"].(map[string]any)["accounts"] == nil || st["you"].(map[string]any)["role"] != want {
			t.Errorf("%s's server_status: %v", who, st)
		}
		_, _, lt := atOp(c, "list_tables", `{}`)
		if len(lt["tables"].([]any)) < 50 {
			t.Errorf("%s's list_tables: %d", who, len(lt["tables"].([]any)))
		}
		_, _, g := atOp(c, "guide", `{}`)
		if !strings.Contains(fmt.Sprint(g["guide"]), "work_board") {
			t.Errorf("%s's guide", who)
		}
	}
	// an admin's own tools
	if code, text, _ := atOp(boss, "sql_query", `{"sql":"SELECT COUNT(*) FROM gm_testers"}`); code != 200 {
		t.Errorf("an admin's SQL: %d %s", code, text)
	}
	if code, text, _ := atOp(gm, "sql_query", `{"sql":"SELECT 1"}`); code != 403 || strings.Contains(text, testerRefused) {
		t.Errorf("a GM's SQL: %d %s", code, text)
	}

	// a row on a GM's account does nothing: a GM's powers, a GM's way in
	mustExec(t, f.db, `INSERT INTO gm_testers (accountid, note) VALUES (2, 'once a tester')`)
	if code, _, out := atOp(gm, "query_rows", `{"table":"characters"}`); code != 200 {
		t.Errorf("a GM with a row: %d %v", code, out)
	}
	_, sess := gm.do("GET", "/api/auth/session", nil)
	if sess["user"].(map[string]any)["role"] != "gm" {
		t.Errorf("a GM with a row: %v", sess["user"])
	}
	// and the role's table is not on the way of a GM's sign-in at all: gone, GMs still work
	mustExec(t, f.db, `ALTER TABLE gm_testers RENAME TO gm_testers_gone`)
	f.enrolTOTP(1)
	if code, out := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword}); code != 200 {
		t.Errorf("a GM's sign-in with the table gone: %d %v", code, out)
	}
	if code, _ := f.browser().do("POST", "/api/auth/login", map[string]any{"username": "tess", "password": testPassword}); code != 401 {
		t.Errorf("a tester's sign-in with the table gone: %d (it fails closed)", code)
	}
	mustExec(t, f.db, `ALTER TABLE gm_testers_gone RENAME TO gm_testers`)
}

// A tester made a GM: what it made as a tester does not act as a GM. A GM
// brought down who holds the role is a tester again, and told so.
func TestATesterMadeAGMStartsAgain(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	tess := f.browser()
	tess.login("tess")
	tok := f.bearer(tess.workToken("old", "write"))
	boss := f.browser()
	boss.login("boss")
	code, _, out := atOp(boss, "set_account_level", `{"account":"tess","level":1}`)
	if code != 200 {
		t.Fatalf("promote: %d %v", code, out)
	}
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "revoked") {
		t.Errorf("the answer: %s", live)
	}
	if code, _, _ := atOp(tok, "query_rows", `{"table":"characters"}`); code != 401 {
		t.Errorf("its tester token as a GM: %d", code)
	}
	if code, _, _ := atOp(tess, "server_status", `{}`); code != 401 {
		t.Errorf("its tester session as a GM: %d", code)
	}
	tess = f.browser()
	tess.login("tess")
	if code, text, _ := atOp(tess, "query_rows", `{"table":"characters"}`); code != 200 {
		t.Errorf("a new sign-in as a GM: %d %s", code, text)
	}
	code, _, out = atOp(boss, "set_account_level", `{"account":"tess","level":0}`)
	if code != 200 {
		t.Fatalf("demote: %d %v", code, out)
	}
	if live := fmt.Sprint(out["change"].(map[string]any)["live"]); !strings.Contains(live, "tester still") {
		t.Errorf("the answer: %s", live)
	}
	tess = f.browser()
	tess.login("tess")
	if code, text, _ := atOp(tess, "query_rows", `{"table":"characters"}`); code != 403 || !strings.Contains(text, testerRefused) {
		t.Errorf("after coming down: %d %s", code, text)
	}
}

// The sanitised snapshot leaves the role out: an admin's copy names no tester.
func TestASnapshotLeavesTheTestersOut(t *testing.T) {
	f := newFixture(t)
	f.newTester()
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
	var n int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM gm_testers`).Scan(&n); err != nil || n != 0 {
		t.Errorf("gm_testers in the snapshot: %d %v", n, err)
	}
}

// The page in a real browser, when one is installed, the whole frame: a tester
// is offered its pages and buttons and no other, and no page it can reach asks
// for anything refused; a GM's and an admin's frame keep theirs.
func TestTheTestersPageInABrowser(t *testing.T) {
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
	f.newTester()
	mustExec(t, f.db, `INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (100, 'Self, Increase HP by 10', 0, 'seed')`)
	boss := f.browser()
	boss.login("boss")
	atOp(boss, "update_row", `{"table":"itempattern","key":{"patternid":11},"values":{"itemcost":3}}`)

	harness := func(user, code, script string) string {
		return `<!doctype html><meta charset="utf-8"><div id="app"></div><script type="module">
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
const yes = (list, word) => { if (!list.some((b) => b.includes(word))) throw new Error('no ' + word + ' on ` + "${location.hash}" + `: ' + list.join(' | ')); };
const same = (a, b) => { if (JSON.stringify(a) !== JSON.stringify(b)) throw new Error('got ' + JSON.stringify(a) + ', want ' + JSON.stringify(b)); };
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
	}
	tester := `
    same(nav(), ['#/items', '#/spells', '#/visuals', '#/tables', '#/audit', '#/ai', '#/security', '#/disc']);
    if (!document.querySelector('header').textContent.includes('Tester')) throw new Error('no Tester badge');
    await wait(() => text().includes('Rusty Sword'), 'the items (the home page)');
    no(buttons(), 'Broadcast');
    await open('#/items/10', 'Item 10');
    await wait(() => text().includes('Look'), 'the side panel');
    no(buttons(), 'Give to a character'); no(buttons(), 'Delete'); yes(buttons(), 'Copy');
    if (text().includes('Sold by') || text().includes('character item row')) throw new Error('the item page shows who sells or holds it: ' + text().slice(0, 300));
    // change the item through the form, as a person does
    const field = [...document.querySelectorAll('main input')].find((i) => i.getAttribute('aria-label') === 'Weapon DMG');
    field.value = '123';
    field.dispatchEvent(new Event('input'));
    await wait(() => buttons().includes('Save'), 'Save');
    [...document.querySelectorAll('main button')].find((b) => b.textContent.trim() === 'Save').click();
    await wait(() => document.body.textContent.includes('Saved'), 'Saved');
    await open('#/spells', 'Minor Healing');
    await open('#/spells/100', 'What it does');
    no(buttons(), 'Play in game'); no(buttons(), 'Delete'); yes(buttons(), 'Save the effect');
    if (text().includes('know it')) throw new Error('the spell page shows who knows it');
    if (!text().includes('Warrior')) throw new Error('the spell page lost its classes');
    if ([...document.querySelectorAll('main .flagset input')].length) throw new Error('the class boxes are offered');
    await open('#/tables', 'Item and spell tables');
    for (const name of ['itempattern', 'spelleffect', 'foodeffect']) if (!text().includes(name)) throw new Error('no table ' + name);
    for (const name of ['accountinfo', 'characters', 'npcs', 'classspells', 'gm_testers']) if (text().includes(name)) throw new Error('offered table ' + name);
    await open('#/tables/spellarea', 'Spell areas');
    yes(buttons(), 'New');
    await open('#/tables/spelleffect/100', 'What it does');
    no(buttons(), 'Play in game'); yes(buttons(), 'Delete');
    await open('#/audit', 'itempattern 10');
    yes(buttons(), 'Revert');
    if (text().includes('boss')) throw new Error('the log names another: ' + text().slice(0, 400));
    if (document.querySelector('main input[placeholder="GM"]') || document.querySelector('main input[placeholder="text in the entry"]')) throw new Error('the log offers a filter it refuses');
    await open('#/ai', 'Your tokens');
    if (!text().includes('openapi.json?role=tester')) throw new Error('the AI page offers the whole API');
    await open('#/security', 'Authenticator');
    await open('#/disc', 'isc');
    await open('#/visuals', 'isc');
    no(buttons(), 'Play in game');
    // a GM's page by its address: the tester's home, nothing asked for
    for (const hash of ['#/dashboard', '#/npcs/500', '#/characters/7', '#/accounts/1', '#/settings', '#/sql', '#/petitions']) {
      await open(hash, 'Rusty Sword');
      if (nav().length !== 8) throw new Error('the navigation moved');
    }
    // the search box finds what it may
    const box = document.querySelector('header input[type=search]');
    box.value = 'Rusty';
    box.dispatchEvent(new Event('input'));
    await wait(() => document.querySelector('.search-pop')?.textContent.includes('Rusty Sword'), 'the search');
`
	gm := `
    for (const hash of ['#/dashboard', '#/work', '#/npcs', '#/accounts', '#/items', '#/spells', '#/tables', '#/audit', '#/ai', '#/security']) if (!nav().includes(hash)) throw new Error('no ' + hash + ': ' + nav().join(' '));
    if (nav().includes('#/sql')) throw new Error('a GM offered SQL');
    if (!document.querySelector('header').textContent.includes('GM')) throw new Error('no GM badge');
    await open('#/items/10', 'Sold by');
    yes(buttons(), 'Give to a character'); yes(buttons(), 'Delete'); yes(buttons(), 'Copy');
    await open('#/spells/100', 'What it does');
    yes(buttons(), 'Play in game'); yes(buttons(), 'Delete');
    if (!text().includes('know it')) throw new Error('no Known');
    if (![...document.querySelectorAll('main .flagset input')].length) throw new Error('no class boxes');
    await open('#/tables', 'All tables');
    for (const name of ['accountinfo', 'characters', 'gm_testers']) if (!text().includes(name)) throw new Error('no table ' + name);
    await open('#/audit', 'itempattern 11');
    if (!text().includes('boss')) throw new Error('the log names nobody');
    await open('#/ai', 'Your tokens');
    if (text().includes('role=tester')) throw new Error('the AI page offers a GM the tester API');
    await open('#/accounts/5', 'Tester');
    no(buttons(), 'Tester');
`
	admin := `
    if (!nav().includes('#/sql')) throw new Error('an admin not offered SQL');
    await open('#/accounts/5', 'Tester');
    yes(buttons(), 'Tester');
    await open('#/accounts/2', 'Account 2');
    no(buttons(), 'Tester');
    await open('#/accounts/3', 'Account 3');
    yes(buttons(), 'Tester');
`
	f.enrolTOTP(1)
	for _, tc := range []struct{ name, user, code, script string }{
		{"tester", "tess", "", tester},
		{"gm", "gm", "", gm},
		{"admin", "boss", totpNow(testTOTPSecret), admin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// the harness posts its result: no timer here is a virtual one, so a
			// real wait for the disc worker is a real wait (a --dump-dom budget
			// runs out in an instant while nothing but a worker is busy)
			result := make(chan string, 1)
			page := harness(tc.user, tc.code, tc.script)
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
			tmp, err := os.MkdirTemp("", "testerpage")
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
					t.Fatalf("the page, as %s: %.1500s", tc.user, got)
				}
			case err := <-gone:
				gone <- err
				t.Skipf("the browser did not load the page (%v): %.600s", err, stderr.String())
			case <-ctx.Done():
				t.Fatalf("the page, as %s, never finished: %.600s", tc.user, stderr.String())
			}
		})
	}
	// the form's change reached the row under the tester's name
	var dmg int
	f.db.QueryRow(`SELECT weapondamage FROM itempattern WHERE patternid = 10`).Scan(&dmg)
	if dmg != 123 {
		t.Errorf("the page's Save: weapondamage %d", dmg)
	}
	var who string
	f.db.QueryRow(`SELECT username FROM gm_audit WHERE tbl = 'itempattern' AND rowkey = '{"patternid":10}' ORDER BY id DESC LIMIT 1`).Scan(&who)
	if who != "tess" {
		t.Errorf("the audit line of the page's Save is %q's", who)
	}
}

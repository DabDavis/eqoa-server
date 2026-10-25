package gmweb

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
	"rsc.io/qr"
)

// fixture is a panel over a fresh database with three accounts (an admin, a
// GM and a player), a few NPCs, items and spells, and a world with the NPCs
// loaded.
type fixture struct {
	t     *testing.T
	s     *Server
	db    *database.SQLiteDB
	world *entity.World
	ts    *httptest.Server
	cfg   string
}

const testPassword = "correct horse"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	db, err := database.NewSQLite(filepath.Join(dir, "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hash, _ := auth.HashPassword(testPassword, 4)
	mustExec(t, db, `INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES
		(1, 'boss', ?, 1, 2), (2, 'gm', ?, 1, 1), (3, 'player', ?, 1, 0), (4, 'bannedgm', ?, -1, 1)`, hash, hash, hash, hash)
	mustExec(t, db, `INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level,
		haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar)
		VALUES ('Hero', 3, 7, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 25300, 54, 15700, 100)`)
	mustExec(t, db, `INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (500, 'Guard Dahvel', 0, 25000, 50, 15000, 0, 1893243078, 1.0, 30, 128, '0'),
		       (501, 'Merchant Shadehand', 0, 25010, 50, 15010, 0, 1893243078, 1.0, 20, 129, '0'),
		       (502, 'a decaying skeleton', 0, 20000, 40, 12000, 0, 1893243078, 1.0, 3, 0, '0')`)
	mustExec(t, db, `INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent,
		unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore,
		unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 5, 0, 0, 0, 12, 0, 1, 0, 0, 0, 7, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, 'Rusty Sword', '', 0, 0),
		       (11, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Bread', '', 0, 0)`)
	mustExec(t, db, `INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon,
		scope, recast, eqprequire, spellname, spelldesc) VALUES (100, 1, 0, 0, 30, 2000, 5, 0, 0, 0, 6000, 0, 'Minor Healing', ''),
		(101, 5, 0, 0, 30, 2000, 5, 0, 0, 0, 6000, 0, 'Root', '')`)
	mustExec(t, db, `INSERT INTO classspells (class, spellid, source) VALUES (0, 100, 'test'), (0, 101, 'test')`)
	mustExec(t, db, `INSERT INTO npcitem (npcid, patternid, stackleft, listnumber) VALUES (501, 11, 1, 0)`)

	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("# the test server\nserver:\n  server_ip: \"127.0.0.1\" # here\nworld:\n  # what NPCs say\n  npc_text:\n    \"Guard Dahvel\": \"Move along.\"\n"), 0o644)
	cfg := config.DefaultConfig()
	cfg.Server.DBPath = filepath.Join(dir, "eqoa.db")
	cfg.Auth.BCryptCost = 4
	cfg.GM.Port = 1
	cfg.GM.LogPath = filepath.Join(dir, "gameserver.log")
	os.WriteFile(cfg.GM.LogPath, []byte("line one\nGM: something\nline three\n"), 0o644)
	cfg.World.NPCText = map[string]string{"Guard Dahvel": "Move along."}
	handlers.SetNPCText(cfg.World.NPCText)

	world := entity.NewWorld()
	npcs := database.NewNPCStore(db)
	world.LoadNPCs(npcs)
	sm := session.NewManager()
	world.SessionManager = sm
	chars := database.NewSQLiteCharacterStore(db)
	auto := cfg.Auth.AllowAutoRegister
	s, err := New(Deps{
		Config: cfg, ConfigPath: cfgPath, DB: db, World: world, Sessions: sm,
		Characters: chars, Accounts: database.NewSQLiteAccountStore(db), NPCs: npcs,
		ApplyNewCharacterRules: chars.SetNewCharacterRules,
		SetAutoRegister:        func(b bool) { auto = b },
		AutoRegister:           func() bool { return auto },
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &fixture{t: t, s: s, db: db, world: world, ts: ts, cfg: cfgPath}
}

func mustExec(t *testing.T, db *database.SQLiteDB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// client is a browser: a cookie jar and the CSRF token the session gave.
type client struct {
	f    *fixture
	http *http.Client
	csrf string
	auth string // a bearer token instead of the cookie
}

func (f *fixture) browser() *client {
	jar, _ := cookiejar.New(nil)
	return &client{f: f, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func (f *fixture) bearer(token string) *client {
	c := f.browser()
	c.auth = token
	return c
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.f.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.f.ts.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set(csrfHeader, c.csrf)
	}
	if c.auth != "" {
		req.Header.Set("Authorization", "Bearer "+c.auth)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.f.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{"_raw": string(raw)}
	}
	return resp.StatusCode, out
}

// testTOTPSecret is the authenticator login() enrols for an admin: an
// admin's tools need a sign-in with its code.
const testTOTPSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"

// totpNow is the secret's code at this moment.
func totpNow(secret string) string {
	key, _ := b32.DecodeString(secret)
	return hotp(key, uint64(time.Now().Unix()/totpStep))
}

// enrolTOTP gives an account the test authenticator, its step unspent, so a
// test may sign in with the same code more than once.
func (f *fixture) enrolTOTP(accountID int) {
	f.t.Helper()
	mustExec(f.t, f.db, `INSERT OR REPLACE INTO gm_totp (accountid, secret, enabled, last_step) VALUES (?, ?, ?, 0)`,
		accountID, testTOTPSecret, now())
}

// login signs a person in; an admin's account gets the test authenticator
// and signs in with its code, as an admin must for an admin's tools.
func (c *client) login(user string) {
	c.f.t.Helper()
	body := map[string]any{"username": user, "password": testPassword}
	var id, level int
	c.f.db.QueryRow(`SELECT accountid, COALESCE(acctlevel, 0) FROM accountinfo WHERE username = ?`, user).Scan(&id, &level)
	if level >= c.f.s.cfg.AdminLevel {
		c.f.enrolTOTP(id)
		body["code"] = totpNow(testTOTPSecret)
	}
	code, out := c.do("POST", "/api/auth/login", body)
	if code != 200 {
		c.f.t.Fatalf("login %s: %d %v", user, code, out)
	}
	c.csrf, _ = out["csrf"].(string)
}

func (c *client) op(name string, args any) (int, map[string]any) {
	c.f.t.Helper()
	return c.do("POST", "/api/op/"+name, args)
}

func TestSignInRefusesTheWrongPeopleAlike(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct{ user, pass string }{
		{"gm", "wrong"}, {"player", testPassword}, {"nobody", testPassword}, {"bannedgm", testPassword},
	} {
		c := f.browser()
		code, out := c.do("POST", "/api/auth/login", map[string]any{"username": tc.user, "password": tc.pass})
		if code != 401 || !strings.Contains(fmt.Sprint(out["error"]), "not a GM account") {
			t.Errorf("%s/%s: %d %v, want 401 with the one message", tc.user, tc.pass, code, out)
		}
	}
	c := f.browser()
	c.login("gm")
	if code, out := c.do("GET", "/api/auth/session", nil); code != 200 || out["csrf"] == nil {
		t.Fatalf("session: %d %v", code, out)
	}
}

func TestAWriteWithoutTheCSRFTokenIsRefused(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	csrf := c.csrf
	c.csrf = ""
	if code, _ := c.op("broadcast", map[string]any{"text": "hi"}); code != 403 {
		t.Fatalf("no CSRF token: %d, want 403", code)
	}
	c.csrf = "wrong"
	if code, _ := c.op("broadcast", map[string]any{"text": "hi"}); code != 403 {
		t.Fatalf("a wrong CSRF token: %d, want 403", code)
	}
	c.csrf = csrf
	if code, out := c.op("broadcast", map[string]any{"text": "hi"}); code != 200 {
		t.Fatalf("with the token: %d %v", code, out)
	}
}

func TestFailedSignInsAreLimited(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	for i := 0; i < auth.FailsPerName; i++ {
		c.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": "nope"})
	}
	code, _ := c.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword})
	if code != 429 {
		t.Fatalf("after %d failures the right password got %d, want 429", auth.FailsPerName, code)
	}
}

func TestADemotedGMIsOutAtTheNextRequest(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	mustExec(t, f.db, `UPDATE accountinfo SET acctlevel = 0 WHERE username = 'gm'`)
	if code, _ := c.do("GET", "/api/auth/session", nil); code != 401 {
		t.Fatalf("demoted GM: %d, want 401", code)
	}
}

func TestTokensActWithinTheirScope(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	makeToken := func(scope string) string {
		code, out := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "test " + scope, "scope": scope, "days": 1})
		if code != 200 {
			t.Fatalf("make %s token: %d %v", scope, code, out)
		}
		return out["token"].(string)
	}
	if code, _ := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "admin"}); code != 403 {
		t.Fatalf("a GM made an admin token: %d", code)
	}
	read := f.bearer(makeToken("read"))
	if code, out := read.op("query_rows", map[string]any{"table": "npcs"}); code != 200 || out["total"].(float64) != 3 {
		t.Fatalf("read token query: %d %v", code, out)
	}
	if code, _ := read.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "values": map[string]any{"npc_level": 31}}); code != 403 {
		t.Fatalf("read token wrote: %d", code)
	}
	if code, _ := read.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "y", "scope": "read"}); code != 403 {
		t.Fatalf("a token made a token: %d", code)
	}
	writeTok := makeToken("write")
	write := f.bearer(writeTok)
	if code, out := write.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "values": map[string]any{"npc_level": 31}}); code != 200 {
		t.Fatalf("write token: %d %v", code, out)
	}
	if code, _ := write.op("sql_query", map[string]any{"sql": "SELECT 1"}); code != 403 {
		t.Fatalf("a GM's write token ran SQL: %d", code)
	}
	// revoke
	_, list := c.do("GET", "/api/tokens", nil)
	for _, tk := range list["tokens"].([]any) {
		m := tk.(map[string]any)
		if m["name"] == "test write" {
			c.do("POST", "/api/tokens", map[string]any{"action": "revoke", "id": m["id"]})
		}
	}
	if code, _ := write.op("server_status", nil); code != 401 {
		t.Fatalf("revoked token: %d, want 401", code)
	}
	// stored only as a hash
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_tokens WHERE hash = ?`, writeTok).Scan(&n)
	if n != 0 {
		t.Fatal("a token's value is in the database")
	}
}

func TestAnNPCEditIsLiveAuditedAndRevertible(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	// a change moves the NPC in the world
	code, out := c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500},
		"values": map[string]any{"x_coord": 26000, "npc_level": 40}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	st, ok := f.world.NPCStateOf(500)
	if !ok || st.SpawnX != 26000 || st.Level != 40 {
		t.Fatalf("world NPC after the edit: %+v", st)
	}
	if !strings.Contains(fmt.Sprint(out["live"]), "changed in the world") {
		t.Errorf("live: %v", out["live"])
	}
	auditID := out["audit"].(float64)

	// a clone spawns
	code, out = c.op("clone_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500},
		"changes": map[string]any{"npc_name": "Guard Two", "x_coord": 25100}})
	if code != 200 {
		t.Fatalf("clone: %d %v", code, out)
	}
	newID := out["change"].(map[string]any)["key"].(map[string]any)["id"].(float64)
	if st, ok := f.world.NPCStateOf(entity.EntityID(newID)); !ok || st.Name != "Guard Two" {
		t.Fatalf("the clone is not in the world: %+v %v", st, ok)
	}

	// a delete removes it, and reverting the delete brings it back
	code, out = c.op("delete_row", map[string]any{"table": "npcs", "key": map[string]any{"id": newID}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if _, ok := f.world.NPCStateOf(entity.EntityID(newID)); ok {
		t.Fatal("deleted NPC still in the world")
	}
	delAudit := out["audit"].(float64)
	if code, out = c.op("revert_change", map[string]any{"id": delAudit}); code != 200 {
		t.Fatalf("revert delete: %d %v", code, out)
	}
	if _, ok := f.world.NPCStateOf(entity.EntityID(newID)); !ok {
		t.Fatal("reverted delete did not bring the NPC back")
	}

	// reverting the first edit puts the old level back
	if code, out = c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert update: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.Level != 30 || st.SpawnX != 25000 {
		t.Fatalf("after revert: %+v", st)
	}
	// and the same entry cannot be reverted twice
	if code, _ = c.op("revert_change", map[string]any{"id": auditID}); code == 200 {
		t.Fatal("reverted twice")
	}
	_, log := c.op("audit_log", map[string]any{"table": "npcs"})
	if n := log["total"].(float64); n < 5 {
		t.Fatalf("audit entries for npcs: %v", n)
	}
}

func TestAMovedRowIsAConflict(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, out := c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501},
		"values": map[string]any{"npc_level": 25}, "expect": map[string]any{"npc_level": 19}})
	if code != 409 || out["current"] == nil {
		t.Fatalf("stale expect: %d %v", code, out)
	}
}

func TestSecretsNeverLeave(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("boss")
	_, out := c.op("query_rows", map[string]any{"table": "accountinfo"})
	for _, r := range out["rows"].([]any) {
		if _, has := r.(map[string]any)["pwhash"]; has {
			t.Fatal("pwhash came back from query_rows")
		}
	}
	if code, _ := c.op("update_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 3},
		"values": map[string]any{"pwhash": "x"}}); code == 200 {
		t.Fatal("pwhash written through the editor")
	}
	if code, _ := c.op("update_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 3},
		"values": map[string]any{"acctlevel": 2}}); code == 200 {
		t.Fatal("acctlevel written through the editor")
	}
	_, list := c.op("list_tables", nil)
	for _, tb := range list["tables"].([]any) {
		if tb.(map[string]any)["name"] == "gm_totp" {
			t.Fatal("gm_totp listed")
		}
	}
	if code, _ := c.op("query_rows", map[string]any{"table": "gm_totp"}); code == 200 {
		t.Fatal("gm_totp readable")
	}
}

func TestLevelsGuardAccounts(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("ban_account", map[string]any{"account": "boss", "banned": true}); code == 200 {
		t.Fatal("a GM banned an admin")
	}
	if code, _ := gm.op("set_account_level", map[string]any{"account": "player", "level": 1}); code != 403 {
		t.Fatalf("a GM set a level: %d", code)
	}
	if code, out := gm.op("ban_account", map[string]any{"account": "player", "banned": true, "reason": "spam"}); code != 200 {
		t.Fatalf("ban a player: %d %v", code, out)
	}
	boss := f.browser()
	boss.login("boss")
	if code, _ := boss.op("set_account_level", map[string]any{"account": "boss", "level": 3}); code == 200 {
		t.Fatal("an admin raised their own level")
	}
	if code, _ := boss.op("set_account_level", map[string]any{"account": "player", "level": 5}); code == 200 {
		t.Fatal("an admin set a level above their own")
	}
	if code, out := boss.op("set_account_level", map[string]any{"account": "player", "level": 1}); code != 200 {
		t.Fatalf("admin makes a GM: %d %v", code, out)
	}
}

func TestBulkChangesAreBounded(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	filter := []any{map[string]any{"column": "world", "op": "=", "value": 0}}
	if code, out := c.op("update_rows", map[string]any{"table": "npcs", "filters": filter, "values": map[string]any{"npc_level": 1}, "max": 2}); code == 200 {
		t.Fatalf("3 rows changed past max 2: %v", out)
	}
	code, out := c.op("update_rows", map[string]any{"table": "npcs", "filters": filter, "values": map[string]any{"npc_level": 1}, "dry_run": true})
	if code != 200 || out["matched"].(float64) != 3 {
		t.Fatalf("dry run: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.Level != 30 {
		t.Fatal("a dry run changed the world")
	}
	if code, _ := c.op("update_rows", map[string]any{"table": "npcs", "values": map[string]any{"npc_level": 1}}); code == 200 {
		t.Fatal("a bulk change without filters went through")
	}
}

func TestDeletingACharacterTakesItsRows(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("give_item", map[string]any{"character": "Hero", "item": "Bread", "count": 3}); code != 200 {
		t.Fatalf("give: %d %v", code, out)
	}
	if code, out := c.op("grant_spell", map[string]any{"character": "Hero", "spell": "all"}); code != 200 || len(out["added"].([]any)) != 2 {
		t.Fatalf("grant: %d %v", code, out)
	}
	code, out := c.op("delete_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT (SELECT COUNT(*) FROM charinventory WHERE serverid = 7) + (SELECT COUNT(*) FROM spells WHERE serverid = 7)`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows of the deleted character left", n)
	}
}

func TestSettingsChangeLiveAndKeepTheFile(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	if code, _ := gm.op("update_settings", map[string]any{"start_level": 20}); code != 403 {
		t.Fatalf("a GM changed settings: %d", code)
	}
	c := f.browser()
	c.login("boss")
	code, out := c.op("update_settings", map[string]any{
		"npc_text": map[string]any{"Guard Dahvel": "Halt!", "Merchant Shadehand": "Buy something."}, "start_level": 20,
		"allow_auto_register": false,
	})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if handlers.NPCText()["Guard Dahvel"] != "Halt!" {
		t.Fatal("npc_text not applied live")
	}
	if f.s.d.Characters.NewCharacterRules().Level != 20 || f.s.d.AutoRegister() {
		t.Fatal("rules not applied live")
	}
	b, _ := os.ReadFile(f.cfg)
	text := string(b)
	for _, want := range []string{"# the test server", "# here", "start_level: 20", "Halt!", "allow_auto_register: false", "server_ip"} {
		if !strings.Contains(text, want) {
			t.Errorf("config.yaml lost or lacks %q:\n%s", want, text)
		}
	}
	reloaded, err := config.Load(f.cfg)
	if err != nil || reloaded.World.StartLevel != 20 || reloaded.World.NPCText["Merchant Shadehand"] != "Buy something." {
		t.Fatalf("reload: %v %+v", err, reloaded)
	}
}

func TestTOTPIsAskedForOnceEnrolledAndSpentOnce(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, out := c.do("POST", "/api/auth/totp", map[string]any{"action": "begin"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, out)
	}
	secret := out["secret"].(string)
	key, _ := b32.DecodeString(secret)
	codeAt := func(t time.Time) string { return hotp(key, uint64(t.Unix()/totpStep)) }
	if code, out := c.do("POST", "/api/auth/totp", map[string]any{"action": "confirm", "code": codeAt(time.Now())}); code != 200 {
		t.Fatalf("confirm: %d %v", code, out)
	}
	n := f.browser()
	code, out = n.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword})
	if code != 401 || out["needCode"] != true {
		t.Fatalf("no code: %d %v", code, out)
	}
	// the confirm spent this step's code
	code, _ = n.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword, "code": codeAt(time.Now())})
	if code == 200 {
		t.Fatal("a spent code signed in")
	}
	code, _ = n.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword,
		"code": codeAt(time.Now().Add(totpStep * time.Second))})
	if code != 200 {
		t.Fatalf("the next step's code: %d", code)
	}
}

func TestMCPListsAndCallsTools(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	_, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "mcp", "scope": "read", "days": 1})
	read := f.bearer(tok["token"].(string))
	rpc := func(cl *client, method string, params any) map[string]any {
		_, out := cl.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		return out
	}
	init := rpc(read, "initialize", map[string]any{"protocolVersion": "2025-03-26"})
	if init["result"].(map[string]any)["protocolVersion"] != "2025-03-26" {
		t.Fatalf("initialize: %v", init)
	}
	tools := rpc(read, "tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["guide"] || !names["query_rows"] || names["update_row"] || names["sql_query"] {
		t.Fatalf("a read token's tools: %v", names)
	}
	res := rpc(read, "tools/call", map[string]any{"name": "search", "arguments": map[string]any{"query": "guard"}})["result"].(map[string]any)
	if res["isError"] == true || !strings.Contains(fmt.Sprint(res["content"]), "Guard Dahvel") {
		t.Fatalf("search: %v", res)
	}
	res = rpc(read, "tools/call", map[string]any{"name": "update_row", "arguments": map[string]any{}})["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatalf("a read token's write was not an error: %v", res)
	}
	// a notification is accepted with no body
	req, _ := http.NewRequest("POST", f.ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Authorization", "Bearer "+tok["token"].(string))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 202 {
		t.Fatalf("notification: %d", resp.StatusCode)
	}
	// without a token: where to sign in
	resp, _ = http.Post(f.ts.URL+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if resp.StatusCode != 401 || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("no token: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestOAuthSignsAnAgentInAsTheGM(t *testing.T) {
	f := newFixture(t)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// discovery
	resp, _ := http.Get(f.ts.URL + "/.well-known/oauth-protected-resource/mcp")
	var prm map[string]any
	json.NewDecoder(resp.Body).Decode(&prm)
	if prm["resource"] != f.ts.URL+"/mcp" {
		t.Fatalf("PRM: %v", prm)
	}
	resp, _ = http.Get(f.ts.URL + "/.well-known/oauth-authorization-server")
	var asm map[string]any
	json.NewDecoder(resp.Body).Decode(&asm)
	if asm["registration_endpoint"] != f.ts.URL+"/oauth/register" {
		t.Fatalf("AS metadata: %v", asm)
	}

	// registration: a javascript: redirect is refused, an https one taken
	bad, _ := http.Post(f.ts.URL+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"x","redirect_uris":["javascript:alert(1)"]}`))
	if bad.StatusCode == 201 {
		t.Fatal("a javascript: redirect registered")
	}
	resp, _ = http.Post(f.ts.URL+"/oauth/register", "application/json",
		strings.NewReader(`{"client_name":"ChatGPT","redirect_uris":["https://chatgpt.com/connector_platform_oauth_redirect"]}`))
	var reg map[string]any
	json.NewDecoder(resp.Body).Decode(&reg)
	clientID, _ := reg["client_id"].(string)
	if resp.StatusCode != 201 || clientID == "" {
		t.Fatalf("register: %d %v", resp.StatusCode, reg)
	}

	// the sign-in page
	verifier := "a-verifier-of-enough-length-for-pkce-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"https://chatgpt.com/connector_platform_oauth_redirect"},
		"state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "scope": {"write"}}
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(page), "ChatGPT") {
		t.Fatalf("authorize page: %d %s", resp.StatusCode, page)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "form-action 'self' https://chatgpt.com") {
		t.Errorf("the page's form cannot redirect to the client: %s", resp.Header.Get("Content-Security-Policy"))
	}
	field := func(name string) string {
		m := regexp.MustCompile(`name="` + name + `" value="([^"]+)"`).FindStringSubmatch(string(page))
		if m == nil {
			t.Fatalf("no %s field", name)
		}
		return m[1]
	}
	form := url.Values{"req": {field("req")}, "csrf": {field("csrf")}, "username": {"gm"}, "password": {"wrong"},
		"scope": {"write"}, "action": {"allow"}}
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", form)
	if resp.StatusCode != 401 {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}
	form.Set("password", testPassword)
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", form)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	code := loc.Query().Get("code")
	if resp.StatusCode != 302 || code == "" || loc.Query().Get("state") != "xyz" || loc.Host != "chatgpt.com" {
		t.Fatalf("allow: %d %v", resp.StatusCode, loc)
	}

	// the token: a wrong verifier fails (and spends the code)
	tokenReq := func(v url.Values) (int, map[string]any) {
		resp, _ := http.PostForm(f.ts.URL+"/oauth/token", v)
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if st, _ := tokenReq(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {"https://chatgpt.com/connector_platform_oauth_redirect"}, "code_verifier": {"wrong"}}); st == 200 {
		t.Fatal("a wrong verifier got a token")
	}
	// start again and finish right
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	page, _ = io.ReadAll(resp.Body)
	form = url.Values{"req": {field("req")}, "csrf": {field("csrf")}, "username": {"gm"}, "password": {testPassword},
		"scope": {"write"}, "action": {"allow"}}
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", form)
	loc, _ = url.Parse(resp.Header.Get("Location"))
	st, tok := tokenReq(url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")},
		"client_id": {clientID}, "redirect_uri": {"https://chatgpt.com/connector_platform_oauth_redirect"}, "code_verifier": {verifier}})
	if st != 200 || tok["access_token"] == nil || tok["scope"] != "write" {
		t.Fatalf("token: %d %v", st, tok)
	}
	agent := f.bearer(tok["access_token"].(string))
	if code, out := agent.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 502},
		"values": map[string]any{"npc_level": 4}}); code != 200 {
		t.Fatalf("agent write: %d %v", code, out)
	}
	_, log := f.bearer(tok["access_token"].(string)).op("audit_log", map[string]any{"table": "npcs"})
	if e := log["entries"].([]any)[0].(map[string]any); e["username"] != "gm" || !strings.HasPrefix(e["via"].(string), "oauth:ChatGPT") {
		t.Fatalf("audit of the agent's change: %v", e)
	}
	// refresh rotates: the old access token stops, the new one works
	st, tok2 := tokenReq(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)},
		"client_id": {clientID}})
	if st != 200 {
		t.Fatalf("refresh: %d %v", st, tok2)
	}
	if code, _ := agent.op("server_status", nil); code != 401 {
		t.Fatalf("the rotated-out access token: %d", code)
	}
	if code, _ := f.bearer(tok2["access_token"].(string)).op("server_status", nil); code != 200 {
		t.Fatal("the new access token does not work")
	}
	if st, _ := tokenReq(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)},
		"client_id": {clientID}}); st == 200 {
		t.Fatal("a used refresh token worked again")
	}
}

func TestOpenAPIDescribesEveryOperation(t *testing.T) {
	f := newFixture(t)
	resp, _ := http.Get(f.ts.URL + "/api/openapi.json")
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	if len(paths) != len(f.s.opList) {
		t.Fatalf("%d paths for %d operations", len(paths), len(f.s.opList))
	}
	for p, v := range paths {
		post := v.(map[string]any)["post"].(map[string]any)
		if d := post["description"].(string); len(d) > openAPIDescMax {
			t.Errorf("%s: description %d long", p, len(d))
		}
	}
	resp, _ = http.Get(f.ts.URL + "/api/openapi.json?core=1")
	json.NewDecoder(resp.Body).Decode(&doc)
	if n := len(doc["paths"].(map[string]any)); n == 0 || n > 30 {
		t.Fatalf("core operations: %d (want 1-30)", n)
	}
}

func TestSQLIsAnAdminsAndQueriesReadOnly(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("boss")
	if code, out := c.op("sql_query", map[string]any{"sql": "SELECT npc_name FROM npcs ORDER BY id"}); code != 200 || len(out["rows"].([]any)) != 3 {
		t.Fatalf("query: %d %v", code, out)
	}
	if code, _ := c.op("sql_query", map[string]any{"sql": "DELETE FROM npcs"}); code == 200 {
		t.Fatal("sql_query deleted")
	}
	if code, _ := c.op("sql_query", map[string]any{"sql": "SELECT 1; DELETE FROM npcs"}); code == 200 {
		t.Fatal("two statements ran")
	}
	if code, _ := c.op("sql_query", map[string]any{"sql": "WITH x AS (SELECT 1) DELETE FROM npcs"}); code == 200 {
		t.Fatal("a WITH ... DELETE ran in read-only mode")
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&n)
	if n != 3 {
		t.Fatalf("%d NPC rows left", n)
	}
	// the connection is writable again for the game
	mustExec(t, f.db, `UPDATE npcs SET npc_level = 9 WHERE id = 502`)
	code, out := c.op("sql_execute", map[string]any{"sql": "UPDATE npcs SET npc_level = 8 WHERE id = 502"})
	if code != 200 || out["backup"] == "" {
		t.Fatalf("execute: %d %v", code, out)
	}
	if _, err := os.Stat(out["backup"].(string)); err != nil {
		t.Fatalf("no backup: %v", err)
	}
}

func TestTheLogTailFilters(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	_, out := c.op("server_log", map[string]any{"filter": "gm:"})
	lines := out["lines"].([]any)
	if len(lines) != 1 || lines[0] != "GM: something" {
		t.Fatalf("log: %v", lines)
	}
}

func TestSearchAndFetch(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	_, out := c.op("search", map[string]any{"query": "merchant"})
	res := out["results"].([]any)
	if len(res) != 1 || res[0].(map[string]any)["id"] != "npc:501" {
		t.Fatalf("search: %v", res)
	}
	code, out := c.op("fetch", map[string]any{"id": "npc:501"})
	rec := out["record"].(map[string]any)
	if code != 200 || len(rec["stock"].([]any)) != 1 || rec["live"] == nil {
		t.Fatalf("fetch: %d %v", code, out)
	}
	if code, out = c.op("fetch", map[string]any{"id": "character:7"}); code != 200 || out["title"] != "Hero" {
		t.Fatalf("fetch character: %d %v", code, out)
	}
}

func TestTheAuditLogFiltersByRow(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 501}, "values": map[string]any{"npc_level": 21}})
	c.op("update_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 502}, "values": map[string]any{"npc_level": 4}})
	code, out := c.op("audit_log", map[string]any{"table": "npcs", "key": map[string]any{"id": 501}})
	if code != 200 || out["total"].(float64) != 1 {
		t.Fatalf("by row: %d %v", code, out)
	}
}

func TestALoopbackRedirectMatchesOnAnyPort(t *testing.T) {
	for _, tc := range []struct {
		reg, asked string
		want       bool
	}{
		{"http://127.0.0.1:5000/callback", "http://127.0.0.1:61234/callback", true},
		{"http://localhost/cb", "http://localhost:8080/cb", true},
		{"http://127.0.0.1:5000/callback", "http://127.0.0.1:5000/other", false},
		{"http://127.0.0.1:5000/callback", "http://localhost:5000/callback", false},
		{"https://chat.example/cb", "https://chat.example:8443/cb", false},
		{"http://evil.example:80/cb", "http://evil.example:81/cb", false},
	} {
		if got := redirectMatches(tc.reg, tc.asked); got != tc.want {
			t.Errorf("redirectMatches(%q, %q) = %v, want %v", tc.reg, tc.asked, got, tc.want)
		}
	}
}

func TestATerminalSignsInForAToken(t *testing.T) {
	f := newFixture(t)
	post := func(body string) (int, map[string]any) {
		resp, err := http.Post(f.ts.URL+"/api/auth/token", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, _ := post(`{"username":"player","password":"` + testPassword + `"}`); code != 401 {
		t.Fatalf("a player got a token: %d", code)
	}
	if code, _ := post(`{"username":"gm","password":"` + testPassword + `","scope":"admin"}`); code != 403 {
		t.Fatalf("a GM got an admin token: %d", code)
	}
	code, out := post(`{"username":"gm","password":"` + testPassword + `","name":"codex"}`)
	if code != 200 || out["scope"] != "write" {
		t.Fatalf("terminal sign-in: %d %v", code, out)
	}
	agent := f.bearer(out["token"].(string))
	if code, res := agent.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}); code != 200 || res["result"] == nil {
		t.Fatalf("the token on MCP: %d %v", code, res)
	}
	// the audit names the terminal
	_, log := agent.op("audit_log", map[string]any{"action": "token"})
	if e := log["entries"].([]any)[0].(map[string]any); !strings.Contains(e["detail"].(string), "terminal") {
		t.Fatalf("audit: %v", e)
	}
}

func TestTheConsentPageOffersWriteNotAdmin(t *testing.T) {
	f := newFixture(t)
	resp, _ := http.Post(f.ts.URL+"/oauth/register", "application/json",
		strings.NewReader(`{"client_name":"Codex","redirect_uris":["http://127.0.0.1:1455/callback"]}`))
	var reg map[string]any
	json.NewDecoder(resp.Body).Decode(&reg)
	q := url.Values{"response_type": {"code"}, "client_id": {reg["client_id"].(string)},
		"redirect_uri": {"http://127.0.0.1:50999/callback"}, "code_challenge": {strings.Repeat("x", 43)}, "code_challenge_method": {"S256"},
		"scope": {"read write admin offline_access"}}
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("another loopback port refused: %d %s", resp.StatusCode, page)
	}
	if !regexp.MustCompile(`value="write" checked`).Match(page) || regexp.MustCompile(`value="admin" checked`).Match(page) {
		t.Fatalf("the page did not offer write first:\n%s", page)
	}
}

// The enrolment answers with a QR code of the authenticator's address: its
// SVG drawn back into modules is exactly the encoder's, inside a white quiet
// border.
func TestTheEnrolmentQRCodeIsTheAuthenticatorsAddress(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.do("POST", "/api/auth/totp", map[string]any{"action": "begin"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, out)
	}
	uri := out["uri"].(string)
	data, ok := strings.CutPrefix(out["qr"].(string), "data:image/svg+xml;base64,")
	if !ok || !strings.Contains(uri, out["secret"].(string)) {
		t.Fatalf("qr %.40q for %s", out["qr"], uri)
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(raw)
	want, err := qr.Encode(uri, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	side := want.Size + 2*qrQuiet
	if !strings.Contains(svg, fmt.Sprintf(`viewBox="0 0 %d %d"`, side, side)) || !strings.Contains(svg, `fill="#fff"`) {
		t.Fatalf("no white %d-module field: %.200s", side, svg)
	}
	got := map[[2]int]bool{}
	for _, m := range regexp.MustCompile(`M(\d+) (\d+)h(\d+)v1h-(\d+)z`).FindAllStringSubmatch(svg, -1) {
		var x, y, run int
		fmt.Sscan(m[1]+" "+m[2]+" "+m[3], &x, &y, &run)
		for i := 0; i < run; i++ {
			got[[2]int{x + i - qrQuiet, y - qrQuiet}] = true
		}
	}
	dark := 0
	for y := -qrQuiet; y < want.Size+qrQuiet; y++ {
		for x := -qrQuiet; x < want.Size+qrQuiet; x++ {
			if got[[2]int{x, y}] != want.Black(x, y) {
				t.Fatalf("module (%d, %d): drawn %v, encoded %v", x, y, got[[2]int{x, y}], want.Black(x, y))
			}
			if want.Black(x, y) {
				dark++
			}
		}
	}
	if dark == 0 || len(got) != dark {
		t.Fatalf("%d modules drawn, %d dark", len(got), dark)
	}
}

// The panel's teleport names a place by its people ("guk": where Guk's
// frogloks stand in Tunaria, the spot of the one nearest their middle), and
// lists and takes only worlds the character's game loads: an offline one logs
// in in the disc's six alone, so world 8 is refused for it by label and by
// coordinates (F3 20261025-000749: a GM's "Guk" was the August gatekeeper's
// world 8). The control: the world holds that world 8 place, and the list
// leaves it out.
func TestATeleportGoesWhereTheGameLoads(t *testing.T) {
	f := newFixture(t)
	for id, n := range map[entity.EntityID]*entity.NPC{
		900: {Name: "froglok guk wizard", World: 0, SpawnX: 24610, SpawnY: -30, SpawnZ: 28980},
		901: {Name: "froglok guk shaman", World: 0, SpawnX: 24630, SpawnY: -33, SpawnZ: 28955},
		902: {Name: "froglok guk warrior", World: 0, SpawnX: 24630, SpawnY: -33, SpawnZ: 28962},
		903: {Name: "Gatekeeper Krup", World: 8, SpawnX: 1125, SpawnY: -62.2, SpawnZ: 1143.7},
		904: {Name: "Harbormaster Thand", World: 0, SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5},
	} {
		n.ServerID = id
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		f.world.NPCs[id] = n
	}
	c := f.browser()
	c.login("gm")

	if !slices.ContainsFunc(f.world.Places(nil), func(p entity.Place) bool { return p.World == 8 }) {
		t.Fatal("the control: the world has no place in world 8")
	}
	code, out := c.op("teleport_destinations", map[string]any{})
	if code != 200 {
		t.Fatalf("teleport_destinations: %d %v", code, out)
	}
	for _, d := range out["destinations"].([]any) {
		if w := d.(map[string]any)["world"].(float64); w >= 6 {
			t.Errorf("listed %v in world %v", d, w)
		}
	}

	code, out = c.op("teleport_character", map[string]any{"character": "Hero", "place": "guk"})
	if code != 200 || !strings.Contains(fmt.Sprint(out["message"]), "froglok guk warrior") {
		t.Fatalf("to guk: %d %v", code, out)
	}
	var x, z float64
	var world int
	if err := f.db.QueryRow(`SELECT x, z, world FROM characters WHERE serverid = 7`).Scan(&x, &z, &world); err != nil {
		t.Fatal(err)
	}
	if x != 24630 || z != 28962 || world != 0 {
		t.Fatalf("Hero's next login at (%v, %v) world %d, want the warrior's spot in Tunaria", x, z, world)
	}

	// the page's NPC field takes the same word
	if code, out = c.op("teleport_character", map[string]any{"character": "Hero", "npc": "guk"}); code != 200 ||
		!strings.Contains(fmt.Sprint(out["message"]), "3 NPCs called guk") {
		t.Fatalf("npc guk: %d %v", code, out)
	}
	if code, out = c.op("teleport_character", map[string]any{"character": "Hero", "npc": "nobody here"}); code == 200 {
		t.Fatalf("an NPC nobody is: %v", out)
	}
	if code, out = c.op("teleport_character", map[string]any{"character": "Hero", "place": "World 8 (HDD)"}); code == 200 {
		t.Fatalf("world 8 by its label: %v", out)
	}
	code, out = c.op("teleport_character", map[string]any{"character": "Hero", "x": 1125, "y": -62, "z": 1143, "world": 8})
	if code == 200 || !strings.Contains(fmt.Sprint(out), "disc's six") {
		t.Fatalf("world 8 by coordinates: %d %v", code, out)
	}
}

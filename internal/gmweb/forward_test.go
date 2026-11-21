package gmweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// One panel for both shards (forward.go): a Heirloom panel hands calls named
// for Live to Live's own panel, as the GM, judged and audited there.

const testShardKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// shardPair is two panels, each on its own database: heir hands "live" calls to
// live. Live's NPC 500 is named apart, so an answer says whose database gave it.
func shardPair(t *testing.T) (heir, live *fixture) {
	t.Helper()
	key := filepath.Join(t.TempDir(), "gm-shard.key")
	if err := os.WriteFile(key, []byte(testShardKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	live = newFixtureWith(t, func(c *config.Config) {
		c.GM.ShardID, c.GM.ShardLabel, c.GM.ShardKeyFile = "live", "Live", key
	})
	heir = newFixtureWith(t, func(c *config.Config) {
		c.GM.OtherShards = []config.GMShardPanel{{Name: "live", Label: "Live", URL: live.ts.URL, KeyFile: key}}
	})
	mustExec(t, live.db, `UPDATE npcs SET npc_name = 'Live Guard' WHERE id = 500`)
	return heir, live
}

func npcName(out map[string]any) string {
	row, _ := out["row"].(map[string]any)
	name, _ := row["npc_name"].(string)
	return name
}

func TestAShardsCallIsAnsweredAndAuditedThere(t *testing.T) {
	heir, live := shardPair(t)
	c := heir.browser()
	c.login("gm")
	key := map[string]any{"id": 500}

	// omitted, or the panel's own name: here
	for _, args := range []map[string]any{{"table": "npcs", "key": key}, {"table": "npcs", "key": key, "shard": "heirloom"}} {
		if code, out := c.op("get_row", args); code != 200 || npcName(out) != "Guard Dahvel" {
			t.Fatalf("here %v: %d %v", args, code, out)
		}
	}
	// named in the arguments, or the query (the page's switch): Live's database
	if code, out := c.op("get_row", map[string]any{"table": "npcs", "key": key, "shard": "live"}); code != 200 || npcName(out) != "Live Guard" {
		t.Fatalf("shard live: %d %v", code, out)
	}
	if code, out := c.do("POST", "/api/op/get_row?shard=Live", map[string]any{"table": "npcs", "key": key}); code != 200 || npcName(out) != "Live Guard" {
		t.Fatalf("?shard=Live: %d %v", code, out)
	}

	// a change: made on Live's database and world, audited in Live's log as the GM
	code, out := c.op("update_row", map[string]any{"table": "npcs", "key": key, "values": map[string]any{"npc_level": 44}, "shard": "live"})
	if code != 200 {
		t.Fatalf("update on live: %d %v", code, out)
	}
	var lvLive, lvHeir int
	live.db.QueryRow(`SELECT npc_level FROM npcs WHERE id = 500`).Scan(&lvLive)
	heir.db.QueryRow(`SELECT npc_level FROM npcs WHERE id = 500`).Scan(&lvHeir)
	if lvLive != 44 || lvHeir != 30 {
		t.Fatalf("levels: live %d heirloom %d", lvLive, lvHeir)
	}
	if st, ok := live.world.NPCStateOf(500); !ok || st.Level != 44 {
		t.Fatalf("Live's world NPC: %+v", st)
	}
	var who, via string
	var acct int
	if err := live.db.QueryRow(`SELECT accountid, username, via FROM gm_audit WHERE tbl = 'npcs' ORDER BY id DESC LIMIT 1`).Scan(&acct, &who, &via); err != nil {
		t.Fatal(err)
	}
	if acct != 2 || who != "gm" || via != "web via heirloom panel" {
		t.Fatalf("Live's audit: #%d %q %q", acct, who, via)
	}
	var n int
	heir.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'npcs'`).Scan(&n)
	if n != 0 {
		t.Fatalf("Heirloom's log has %d npcs lines", n)
	}
	// a conflict comes back with Live's current row
	code, out = c.op("update_row", map[string]any{"table": "npcs", "key": key, "values": map[string]any{"npc_level": 45},
		"expect": map[string]any{"npc_level": 30}, "shard": "live"})
	if code != 409 || out["current"] == nil {
		t.Fatalf("a conflict on live: %d %v", code, out)
	}

	// an unknown shard says which there are; an operation's own shard keeps its meaning
	if code, out := c.op("get_row", map[string]any{"table": "npcs", "key": key, "shard": "mars"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), `unknown shard "mars"`) || !strings.Contains(fmt.Sprint(out["error"]), "heirloom, live") {
		t.Fatalf("shard mars: %d %v", code, out)
	}
	if code, out := c.op("economy", map[string]any{"shard": "other"}); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "economy_other_db") {
		t.Fatalf("economy's own other: %d %v", code, out)
	}
	if code, out := c.op("economy", map[string]any{"shard": "live"}); code != 200 || out["windows"] == nil {
		t.Fatalf("economy on live: %d %v", code, out)
	}

	// an operation Live lacks says so
	delete(live.s.ops, "search")
	if code, out := c.op("search", map[string]any{"query": "guard", "shard": "live"}); code != 404 ||
		!strings.Contains(fmt.Sprint(out["error"]), "Heirloom's alone") {
		t.Fatalf("an op Live lacks: %d %v", code, out)
	}
	// Live's server not answering
	live.ts.Close()
	if code, out := c.op("get_row", map[string]any{"table": "npcs", "key": key, "shard": "live"}); code != 502 ||
		!strings.Contains(fmt.Sprint(out["error"]), "did not answer") {
		t.Fatalf("Live down: %d %v", code, out)
	}
}

func TestAShardJudgesTheGMByItsOwnAccount(t *testing.T) {
	heir, live := shardPair(t)
	gm := heir.browser()
	gm.login("gm")
	key := map[string]any{"id": 500}

	// below Live's min level: refused there, allowed here
	mustExec(t, live.db, `UPDATE accountinfo SET acctlevel = 0 WHERE accountid = 2`)
	if code, out := gm.op("get_row", map[string]any{"table": "npcs", "key": key, "shard": "live"}); code != 403 ||
		!strings.Contains(fmt.Sprint(out["error"]), "no GM panel on Live") {
		t.Fatalf("a player on Live: %d %v", code, out)
	}
	if code, _ := gm.op("get_row", map[string]any{"table": "npcs", "key": key}); code != 200 {
		t.Fatalf("the same GM here: %d", code)
	}
	// missing on Live
	mustExec(t, live.db, `DELETE FROM accountinfo WHERE accountid = 2`)
	if code, out := gm.op("get_row", map[string]any{"table": "npcs", "key": key, "shard": "live"}); code != 403 ||
		!strings.Contains(fmt.Sprint(out["error"]), "no account #2 on Live") {
		t.Fatalf("no account on Live: %d %v", code, out)
	}

	// admin's tools by Live's level: an admin both places, then a GM on Live
	boss := heir.browser()
	boss.login("boss")
	sql := map[string]any{"sql": "SELECT npc_name FROM npcs WHERE id = 500", "shard": "live"}
	if code, out := boss.op("sql_query", sql); code != 200 || !strings.Contains(fmt.Sprint(out["rows"]), "Live Guard") {
		t.Fatalf("an admin's SQL on live: %d %v", code, out)
	}
	mustExec(t, live.db, `UPDATE accountinfo SET acctlevel = 1 WHERE accountid = 1`)
	if code, out := boss.op("sql_query", sql); code != 403 || !strings.Contains(fmt.Sprint(out["error"]), "needs an admin") {
		t.Fatalf("a GM's SQL on live: %d %v", code, out)
	}
	delete(sql, "shard")
	if code, _ := boss.op("sql_query", sql); code != 200 {
		t.Fatalf("the admin's SQL here: %d", code)
	}

	// an admin on Live alone, signed in here with no authenticator: no admin's tools
	mustExec(t, live.db, `INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (2, 'gm', '', 1, 2)`)
	sql["shard"] = "live"
	if code, out := gm.op("sql_query", sql); code != 403 || !strings.Contains(fmt.Sprint(out["error"]), "authenticator") {
		t.Fatalf("an admin on Live without an authenticator: %d %v", code, out)
	}
}

func TestMCPReachesTheOtherShard(t *testing.T) {
	heir, _ := shardPair(t)
	c := heir.browser()
	c.login("gm")
	_, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "claude", "scope": "write", "days": 1})
	agent := heir.bearer(tok["token"].(string))
	rpc := func(method string, params any) map[string]any {
		_, out := agent.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		return out["result"].(map[string]any)
	}
	if ins := rpc("initialize", map[string]any{}); !strings.Contains(fmt.Sprint(ins["instructions"]), `"live" (Live)`) {
		t.Errorf("instructions: %v", ins["instructions"])
	}
	for _, tl := range rpc("tools/list", nil)["tools"].([]any) {
		tool := tl.(map[string]any)
		props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		if _, ok := props["shard"]; !ok {
			t.Fatalf("%s has no shard", tool["name"])
		}
	}
	res := rpc("tools/call", map[string]any{"name": "get_row", "arguments": map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "shard": "live"}})
	if res["isError"] == true || !strings.Contains(fmt.Sprint(res["content"]), "Live Guard") {
		t.Fatalf("get_row on live: %v", res)
	}
	res = rpc("tools/call", map[string]any{"name": "update_row", "arguments": map[string]any{"table": "npcs", "key": map[string]any{"id": 501},
		"values": map[string]any{"npc_level": 21}, "shard": "live"}})
	if res["isError"] == true {
		t.Fatalf("update_row on live: %v", res)
	}
}

func TestOnlyAKeyedCallFromThisMachineIsTaken(t *testing.T) {
	heir, live := shardPair(t)
	body := func(user string) []byte {
		b, _ := json.Marshal(shardCall{From: "heirloom", AccountID: 2, Username: user, Via: "web", IP: "198.51.100.7", Scope: "write",
			Args: json.RawMessage(`{"table":"npcs","key":{"id":500}}`)})
		return b
	}
	call := func(f *fixture, key string, hdr map[string]string, user string) (int, string) {
		req, _ := http.NewRequest("POST", f.ts.URL+"/api/shard/op/get_row", bytes.NewReader(body(user)))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set(shardKeyHeader, key)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, fmt.Sprint(out)
	}
	if code, out := call(live, testShardKey, nil, "gm"); code != 200 || !strings.Contains(out, "Live Guard") {
		t.Fatalf("the right key: %d %s", code, out)
	}
	if code, _ := call(live, "", nil, "gm"); code != 403 {
		t.Fatalf("no key: %d", code)
	}
	if code, _ := call(live, strings.Repeat("x", 64), nil, "gm"); code != 403 {
		t.Fatalf("a wrong key: %d", code)
	}
	if code, _ := call(live, testShardKey, map[string]string{"X-Real-IP": "203.0.113.9"}, "gm"); code != 403 {
		t.Fatalf("through the proxy: %d", code)
	}
	if code, out := call(live, testShardKey, nil, "boss"); code != 403 || !strings.Contains(out, `is "gm" on Live`) {
		t.Fatalf("another name for the id: %d %s", code, out)
	}
	// a panel that names no key takes none: the path is not there
	if code, _ := call(heir, testShardKey, nil, "gm"); code != 404 {
		t.Fatalf("a panel with no shard key: %d", code)
	}
	// not from this machine
	req := httptest.NewRequest("POST", "/api/shard/op/get_row", bytes.NewReader(body("gm")))
	req.RemoteAddr = "203.0.113.5:4000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(shardKeyHeader, testShardKey)
	rec := httptest.NewRecorder()
	live.s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("from another machine: %d %s", rec.Code, rec.Body)
	}

	// a key file that cannot be read shuts that shard alone, saying why
	c := heir.browser()
	c.login("gm")
	heir.s.peers[0].key, heir.s.peers[0].keyErr = "", "this panel has no key for the Live shard"
	if code, out := c.op("get_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "shard": "live"}); code != 503 ||
		!strings.Contains(fmt.Sprint(out["error"]), "no key") {
		t.Fatalf("no key here: %d %v", code, out)
	}
	if code, _ := c.op("get_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}}); code != 200 {
		t.Fatalf("here, with Live shut: %d", code)
	}
}

func TestThePageIsToldTheShards(t *testing.T) {
	heir, live := shardPair(t)
	c := heir.browser()
	c.login("gm")
	_, out := c.do("GET", "/api/auth/session", nil)
	if s := fmt.Sprint(out["shards"]); !strings.Contains(s, "id:heirloom") || !strings.Contains(s, "id:live") {
		t.Fatalf("shards: %v", out["shards"])
	}
	code, st := c.do("POST", "/api/op/server_status?shard=live", map[string]any{})
	if code != 200 || st["shardId"] != "live" {
		t.Fatalf("Live's status: %d %v", code, st)
	}
	// a panel with no other shards offers no switch, and no shard in its schemas
	l := live.browser()
	l.login("gm")
	if _, out := l.do("GET", "/api/auth/session", nil); out["shards"] != nil {
		t.Fatalf("Live's own page: %v", out["shards"])
	}
	if _, ok := live.s.ops["get_row"].Input["properties"].(map[string]any)["shard"]; ok {
		t.Fatal("a panel with no other shards gave its tools shard")
	}
	// a shard name in the arguments of a panel with none: refused, but its own runs
	if code, _ := l.op("get_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "shard": "live"}); code != 200 {
		t.Fatalf("Live's own name on Live: %d", code)
	}
	if code, out := l.op("get_row", map[string]any{"table": "npcs", "key": map[string]any{"id": 500}, "shard": "heirloom"}); code != 400 ||
		!strings.Contains(fmt.Sprint(out["error"]), "hands calls to no other shard") {
		t.Fatalf("heirloom on Live: %d %v", code, out)
	}
}

func TestTheWorkBoardStaysHere(t *testing.T) {
	heir, live := shardPair(t)
	c := heir.browser()
	c.login("gm")
	for _, path := range []string{"/api/op/post_work?shard=live", "/api/op/post_work"} {
		args := map[string]any{"title": "Moving the guards"}
		if !strings.Contains(path, "?") {
			args["shard"] = "live"
		}
		if code, out := c.do("POST", path, args); code != 200 {
			t.Fatalf("%s: %d %v", path, code, out)
		}
	}
	var here, there int
	heir.db.QueryRow(`SELECT COUNT(*) FROM workboard`).Scan(&here)
	live.db.QueryRow(`SELECT COUNT(*) FROM workboard`).Scan(&there)
	if here != 2 || there != 0 {
		t.Fatalf("work board entries: heirloom %d, live %d", here, there)
	}
}

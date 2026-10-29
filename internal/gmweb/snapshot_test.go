package gmweb

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// snapshotPlanted are secrets and a person's data planted in the fixture's
// every column that holds one; plantSnapshotSecrets adds the ones the panel
// and the database make themselves (the bcrypt hash, the authenticator, the
// token's hash, the salts).
var snapshotPlanted = []string{
	"planted.person@example.org", "203.0.113.77", "Plantedfirstname", "90210-PLANTED",
	"198.51.100.23", "192.0.2.44", "planted-oauth-client-id", "http://127.0.0.1:41234/planted-callback",
	"aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888",
	"9999eeee8888dddd7777cccc6666bbbb5555aaaa4444ffff3333eeee2222dddd",
	"the board's note: the test login is hunter2-planted-password",
}

// plantSnapshotSecrets fills every secret column and answers every secret
// value now in the database, a GM signed in (boss) and its admin token.
func plantSnapshotSecrets(t *testing.T, f *fixture) ([]string, *client, string) {
	t.Helper()
	boss := f.browser()
	boss.login("boss") // enrols the authenticator: gm_totp's secret
	adminTok := boss.workToken("snapshot test", "admin")
	mustExec(t, f.db, `UPDATE accountinfo SET email = 'planted.person@example.org', ipaddress = '203.0.113.77',
		firstname = 'Plantedfirstname', zip = '90210-PLANTED', lastlogin = '2026-10-29 10:00:00' WHERE username = 'player'`)
	mustExec(t, f.db, `UPDATE gm_tokens SET last_ip = '198.51.100.23'`)
	mustExec(t, f.db, `INSERT INTO gm_audit (at, accountid, username, via, ip, action, detail)
		VALUES ('2026-10-29T10:00:00Z', 1, 'boss', 'web', '192.0.2.44', 'login', 'signed in to the panel')`)
	mustExec(t, f.db, `INSERT INTO gm_oauth_clients (client_id, name, redirect_uris, created)
		VALUES ('planted-oauth-client-id', 'Claude Code', '["http://127.0.0.1:41234/planted-callback"]', '2026-10-29T10:00:00Z')`)
	if _, err := f.db.SpellFXSalt(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.MapMarkSalt(); err != nil {
		t.Fatal(err)
	}
	mustExec(t, f.db, `INSERT INTO spellfx_suggestions (spellid, fxdict, name, note, addr_hash, created)
		VALUES (100, 1, 'Hero', 'this one', ?, '2026-10-29T10:00:00Z')`, snapshotPlanted[8])
	mustExec(t, f.db, `INSERT INTO mapmark_suggestions (world, x, z, name, note, client, sender_hash, created)
		VALUES (0, 25000, 15000, 'The well', '', 'vanilla', ?, '2026-10-29T10:00:00Z')`, snapshotPlanted[9])
	if code, out := boss.op("post_work", map[string]any{"title": "Try the snapshot", "note": snapshotPlanted[10]}); code != 200 {
		t.Fatalf("post_work: %d %v", code, out)
	}
	values := append([]string{}, snapshotPlanted...)
	for _, q := range []string{
		`SELECT CAST(pwhash AS TEXT) FROM accountinfo`, `SELECT secret FROM gm_totp`, `SELECT hash FROM gm_tokens`,
		`SELECT value FROM spellfx_meta`, `SELECT value FROM mapmark_meta`,
	} {
		rows, err := f.db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var v string
			rows.Scan(&v)
			values = append(values, v)
			n++
		}
		rows.Close()
		if n == 0 {
			t.Fatalf("%s: nothing planted", q)
		}
	}
	return values, boss, adminTok
}

// fetch GETs a URL as curl does: no cookie, no token.
func fetch(t *testing.T, method, url string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func snapshotFiles(t *testing.T, f *fixture) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(f.s.snapshotDir(), "*"))
	return m
}

func TestAnAdminDownloadsASanitisedSnapshotOnce(t *testing.T) {
	f := newFixture(t)
	values, boss, _ := plantSnapshotSecrets(t, f)
	src, err := os.ReadFile(f.s.d.Config.Server.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range values {
		if !bytes.Contains(src, []byte(v)) {
			t.Fatalf("control: %q is not in the live file, so not finding it in the snapshot would prove nothing", v)
		}
	}

	code, out := boss.op("download_snapshot", nil)
	if code != 200 {
		t.Fatalf("download_snapshot: %d %v", code, out)
	}
	url, _ := out["url"].(string)
	file, _ := out["file"].(string)
	if !strings.HasPrefix(url, f.ts.URL+"/api/snapshot/snap_") || !strings.HasSuffix(url, "/"+file) ||
		!strings.HasSuffix(file, ".db.gz") || out["expiresInSeconds"] != float64(600) || out["singleUse"] != true ||
		!strings.Contains(fmt.Sprint(out["fetch"]), url) {
		t.Fatalf("answer %v", out)
	}
	files := snapshotFiles(t, f)
	if len(files) != 1 || !strings.HasSuffix(files[0], ".db.gz") {
		t.Fatalf("waiting: %v (only the gzipped snapshot)", files)
	}
	if fi, _ := os.Stat(files[0]); fi.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(f.s.snapshotDir()); fi.Mode().Perm() != 0o700 {
		t.Errorf("the directory is %v", fi.Mode().Perm())
	}

	// HEAD leaves the link; GET takes it, as curl would, with no credential
	if resp, _ := fetch(t, "HEAD", url); resp.StatusCode != 200 || resp.Header.Get("Content-Length") == "" {
		t.Fatalf("HEAD: %d", resp.StatusCode)
	}
	resp, gz := fetch(t, "GET", url)
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d %s", resp.StatusCode, gz)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="`+file+`"` {
		t.Errorf("Content-Disposition %q", cd)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type %q", ct)
	}
	sum := sha256.Sum256(gz)
	if hex.EncodeToString(sum[:]) != out["sha256"] || int64(len(gz)) != int64(out["bytes"].(float64)) {
		t.Errorf("the file is not the one the answer describes")
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	dbBytes, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range values {
		if bytes.Contains(dbBytes, []byte(v)) {
			t.Errorf("%q survived the snapshot", v)
		}
	}
	if again, body := fetch(t, "GET", url); again.StatusCode != 404 {
		t.Errorf("the link worked twice: %d %s", again.StatusCode, body)
	}
	if left := snapshotFiles(t, f); len(left) != 0 {
		t.Errorf("the file was not deleted once fetched: %v", left)
	}

	// it opens as the real-data tests open a database, and the game is in it
	path := filepath.Join(t.TempDir(), "snapshot.db")
	os.WriteFile(path, dbBytes, 0o600)
	snap, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	world := entity.NewWorld()
	world.LoadNPCs(database.NewNPCStore(snap))
	for _, id := range []entity.EntityID{500, 501, 502} {
		want, ok1 := f.world.NPCStateOf(id)
		got, ok2 := world.NPCStateOf(id)
		if !ok1 || !ok2 || got.SpawnX != want.SpawnX || got.Level != want.Level {
			t.Errorf("NPC %d: %+v, was %+v", id, got, want)
		}
	}
	chars := database.NewSQLiteCharacterStore(snap)
	if c, err := chars.GetByName("Hero"); err != nil || c.Level != 10 {
		t.Errorf("the character: %+v %v", c, err)
	}
	var name string
	snap.QueryRow(`SELECT username FROM accountinfo WHERE accountid = 3`).Scan(&name)
	if name != "account-3" {
		t.Errorf("the player's login name is %q", name)
	}

	// the audit log says who made it and fetched it
	code, log := boss.op("audit_log", map[string]any{"action": "snapshot"})
	if code != 200 {
		t.Fatal(log)
	}
	all := fmt.Sprint(log["entries"])
	for _, want := range []string{"made the sanitised snapshot " + file, "sanitiser 1", "fetched the sanitised snapshot " + file} {
		if !strings.Contains(all, want) {
			t.Errorf("no audit line with %q in %s", want, all)
		}
	}
}

func TestASnapshotIsAnAdminsAlone(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	if code, out := gm.op("download_snapshot", nil); code != 403 {
		t.Fatalf("a GM (not an admin): %d %v", code, out)
	}
	boss := f.browser()
	boss.login("boss")
	write := f.bearer(boss.workToken("write", "write"))
	if code, _ := write.op("download_snapshot", nil); code != 403 {
		t.Fatalf("an admin's write token: %d", code)
	}
	if tools := fmt.Sprint(write.workRPC("tools/list", nil)); strings.Contains(tools, "download_snapshot") {
		t.Error("a write token is offered the snapshot")
	}
	admin := f.bearer(boss.workToken("admin", "admin"))
	if tools := fmt.Sprint(admin.workRPC("tools/list", nil)); !strings.Contains(tools, "download_snapshot") {
		t.Error("an admin token is not offered the snapshot")
	}
	res := admin.workRPC("tools/call", map[string]any{"name": "download_snapshot", "arguments": map[string]any{}})
	sc, _ := res["result"].(map[string]any)["structuredContent"].(map[string]any)
	if sc == nil || sc["url"] == nil {
		t.Fatalf("MCP: %v", res)
	}
	if code, _ := admin.do("GET", "/api/openapi.json", nil); code != 200 {
		t.Fatal("openapi")
	}
	if resp, _ := fetch(t, "GET", sc["url"].(string)); resp.StatusCode != 200 {
		t.Errorf("the MCP link: %d", resp.StatusCode)
	}
}

func TestASnapshotLinkDiesWithItsHoldersRight(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	tok := boss.workToken("agent", "admin")
	agent := f.bearer(tok)

	// the token that asked is revoked before the fetch
	_, out := agent.op("download_snapshot", nil)
	url := out["url"].(string)
	mustExec(t, f.db, `UPDATE gm_tokens SET revoked = 1 WHERE name = 'agent'`)
	if resp, body := fetch(t, "GET", url); resp.StatusCode != 403 || !strings.Contains(string(body), "revoked") {
		t.Fatalf("a revoked token's link: %d %s", resp.StatusCode, body)
	}
	if left := snapshotFiles(t, f); len(left) != 0 {
		t.Errorf("a refused link's file was kept: %v", left)
	}

	// the admin is demoted before the fetch
	_, out = boss.op("download_snapshot", nil)
	url = out["url"].(string)
	mustExec(t, f.db, `UPDATE accountinfo SET acctlevel = 1 WHERE username = 'boss'`)
	if resp, body := fetch(t, "GET", url); resp.StatusCode != 403 || !strings.Contains(string(body), "no longer an admin") {
		t.Fatalf("a demoted admin's link: %d %s", resp.StatusCode, body)
	}
	mustExec(t, f.db, `UPDATE accountinfo SET acctlevel = 2 WHERE username = 'boss'`)

	// a link nobody made
	if resp, _ := fetch(t, "GET", f.ts.URL+"/api/snapshot/snap_madeup/x.db.gz"); resp.StatusCode != 404 {
		t.Errorf("a made-up link: %d", resp.StatusCode)
	}
}

func TestASnapshotLinkExpiresAndIsReplaced(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")

	// a new snapshot replaces the admin's last, unused
	_, first := boss.op("download_snapshot", nil)
	_, second := boss.op("download_snapshot", nil)
	if resp, _ := fetch(t, "GET", first["url"].(string)); resp.StatusCode != 404 {
		t.Errorf("a replaced link: %d", resp.StatusCode)
	}
	if n := len(snapshotFiles(t, f)); n != 1 {
		t.Errorf("%d files waiting, not the new one's alone", n)
	}
	if resp, _ := fetch(t, "GET", second["url"].(string)); resp.StatusCode != 200 {
		t.Errorf("the new link: %d", resp.StatusCode)
	}

	// one left unused expires, and its file goes with it
	f.s.snap.ttl = 150 * time.Millisecond
	_, out := boss.op("download_snapshot", nil)
	time.Sleep(600 * time.Millisecond)
	if n := len(snapshotFiles(t, f)); n != 0 {
		t.Errorf("an expired link's file was kept")
	}
	if resp, _ := fetch(t, "GET", out["url"].(string)); resp.StatusCode != 404 {
		t.Errorf("an expired link: %d", resp.StatusCode)
	}
	_, log := boss.op("audit_log", map[string]any{"action": "snapshot"})
	all := fmt.Sprint(log["entries"])
	if !strings.Contains(all, "expired unused") || !strings.Contains(all, "replaced by a new one") {
		t.Errorf("the audit log does not say so: %s", all)
	}

	// and an admin makes at most a few at once
	f.s.snap.ttl = snapshotTTL
	refusedAt := 0
	for i := 1; i <= 4 && refusedAt == 0; i++ {
		if code, _ := boss.op("download_snapshot", nil); code == 429 {
			refusedAt = i
		}
	}
	if refusedAt == 0 {
		t.Error("no limit on snapshots")
	}
}

func TestASnapshotRefusedNamesTheTable(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	mustExec(t, f.db, `CREATE TABLE player_logins (id INTEGER PRIMARY KEY, at TEXT, ip TEXT)`)
	code, out := boss.op("download_snapshot", nil)
	if code != 409 || !strings.Contains(fmt.Sprint(out["error"]), "player_logins") {
		t.Fatalf("an unknown table of addresses: %d %v", code, out)
	}
	if left := snapshotFiles(t, f); len(left) != 0 {
		t.Errorf("a refused snapshot left files: %v", left)
	}
	_, log := boss.op("audit_log", map[string]any{"action": "snapshot"})
	if !strings.Contains(fmt.Sprint(log["entries"]), "was refused") {
		t.Error("the refusal is not in the audit log")
	}
}

package gmweb

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
)

// The security review's findings, each held as a test: what it showed could
// be done, refused.

// ownerAndAdmin adds an account above boss (owner, 3) and a second admin
// (adm, 2).
func ownerAndAdmin(t *testing.T, f *fixture) {
	hash, _ := auth.HashPassword(testPassword, 4)
	mustExec(t, f.db, `INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES
		(5, 'owner', ?, 1, 3), (6, 'adm', ?, 1, 2)`, hash, hash)
}

func auditID(t *testing.T, out map[string]any) int64 {
	t.Helper()
	ch, _ := out["change"].(map[string]any)
	if ch == nil {
		ch = out
	}
	id, _ := ch["audit"].(float64)
	if id == 0 {
		t.Fatalf("no audit id in %v", out)
	}
	return int64(id)
}

func levelOf(t *testing.T, f *fixture, id int) int {
	t.Helper()
	l := -99
	f.db.QueryRow(`SELECT COALESCE(acctlevel, -99) FROM accountinfo WHERE accountid = ?`, id).Scan(&l)
	return l
}

func TestARevertCannotRaiseAnAccount(t *testing.T) {
	f := newFixture(t)
	ownerAndAdmin(t, f)
	owner := f.browser()
	owner.login("owner")

	// a demoted admin cannot revert their own demotion
	code, out := owner.op("set_account_level", map[string]any{"account": "adm", "level": 1})
	if code != 200 {
		t.Fatalf("demote: %d %v", code, out)
	}
	demotion := auditID(t, out)
	adm := f.browser()
	adm.login("adm")
	if code, _ := adm.op("revert_change", map[string]any{"id": demotion}); code == 200 {
		t.Fatal("a demoted admin reverted their own demotion")
	}
	if l := levelOf(t, f, 6); l != 1 {
		t.Fatalf("adm is level %d", l)
	}

	// nor can a GM revert an admin's demotion, having set its password, even
	// where every GM is an admin
	if code, out := owner.op("set_account_level", map[string]any{"account": "adm", "level": 2}); code != 200 {
		t.Fatalf("promote back: %d %v", code, out)
	}
	code, out = owner.op("set_account_level", map[string]any{"account": "adm", "level": 0})
	if code != 200 {
		t.Fatalf("demote to 0: %d %v", code, out)
	}
	demotion = auditID(t, out)
	f.s.cfg.AdminLevel = 1
	gm := f.browser()
	f.enrolTOTP(2)
	if code, out := gm.do("POST", "/api/auth/login", map[string]any{"username": "gm", "password": testPassword,
		"code": totpNow(testTOTPSecret)}); code != 200 {
		t.Fatalf("gm sign-in: %d %v", code, out)
	} else {
		gm.csrf, _ = out["csrf"].(string)
	}
	if code, out := gm.op("set_account_password", map[string]any{"account": "adm", "password": "the-gm-knows-it"}); code != 200 {
		t.Fatalf("set a player's password: %d %v", code, out)
	}
	if code, out := gm.op("revert_change", map[string]any{"id": demotion}); code == 200 {
		t.Fatalf("a GM put an admin back above its own level: %v", out)
	}
	if l := levelOf(t, f, 6); l != 0 {
		t.Fatalf("adm is level %d", l)
	}
	f.s.cfg.AdminLevel = 2

	// nor is a deleted account put back, by anyone: its password was never kept
	code, out = owner.op("delete_row", map[string]any{"table": "accountinfo", "key": map[string]any{"accountid": 6}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	deletion := auditID(t, out)
	for _, who := range []*client{gm, owner} {
		if code, _ := who.op("revert_change", map[string]any{"id": deletion}); code == 200 {
			t.Fatal("a deleted account was put back by revert")
		}
	}
	if l := levelOf(t, f, 6); l != -99 {
		t.Fatalf("the deleted account is back at level %d", l)
	}

	// an admin's revert within the rules still works
	code, out = owner.op("set_account_level", map[string]any{"account": "gm", "level": 2})
	if code != 200 {
		t.Fatalf("promote: %d %v", code, out)
	}
	if code, out := owner.op("revert_change", map[string]any{"id": auditID(t, out)}); code != 200 {
		t.Fatalf("revert a promotion: %d %v", code, out)
	}
	if l := levelOf(t, f, 2); l != 1 {
		t.Fatalf("gm is level %d after the revert", l)
	}
}

func TestTheSignInLimiterIsBoundedAndCountsFirst(t *testing.T) {
	f := newFixture(t)

	// overlong names are refused before they are kept under their name
	c := f.browser()
	big := strings.Repeat("A", 100<<10)
	statuses := map[int]int{}
	for i := 0; i < 30; i++ {
		code, _ := c.do("POST", "/api/auth/login", map[string]any{"username": fmt.Sprintf("%d%s", i, big), "password": "x"})
		statuses[code]++
	}
	_, kept := f.s.limiter.Sizes()
	if kept != 0 || statuses[401] != auth.FailsPerAddr || statuses[429] != 30-auth.FailsPerAddr {
		t.Fatalf("statuses %v, %d names kept", statuses, kept)
	}
	// (the limiter's bounds and its IPv6 keys: internal/auth's tests)

	// a burst of concurrent guesses gets no more tries than a slow one
	hash, _ := auth.HashPassword(testPassword, 8)
	mustExec(t, f.db, `UPDATE accountinfo SET pwhash = ? WHERE username = 'gm'`, hash)
	const n = 40
	codes := make(chan int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest("POST", f.ts.URL+"/api/auth/login",
				strings.NewReader(fmt.Sprintf(`{"username":"gm","password":"guess-%d"}`, i)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Real-IP", "198.51.100.7")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes <- 0
				return
			}
			resp.Body.Close()
			codes <- resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for c := range codes {
		got[c]++
	}
	if got[401] > auth.FailsPerName {
		t.Fatalf("%d concurrent guesses: %v (at most %d may be tried)", n, got, auth.FailsPerName)
	}
}

func TestTheAuthorizePageIsBoundedAndRedirectsNoErrors(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	code, out := c.do("POST", "/oauth/register", map[string]any{"client_name": "x", "redirect_uris": []string{"https://evil.example/cb"}})
	if code != 201 {
		t.Fatalf("register: %d %v", code, out)
	}
	id := out["client_id"].(string)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(q url.Values) *http.Response {
		resp, err := noRedirect.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	base := func() url.Values {
		return url.Values{"client_id": {id}, "redirect_uri": {"https://evil.example/cb"}, "response_type": {"code"},
			"code_challenge": {strings.Repeat("c", 43)}, "code_challenge_method": {"S256"}}
	}

	// a bad request is answered on the page: never a redirect to an address
	// anyone could have registered
	for name, q := range map[string]url.Values{
		"implicit flow": func() url.Values { q := base(); q.Set("response_type", "token"); return q }(),
		"no PKCE":       func() url.Values { q := base(); q.Del("code_challenge"); return q }(),
		"short PKCE":    func() url.Values { q := base(); q.Set("code_challenge", "x"); return q }(),
		"huge state":    func() url.Values { q := base(); q.Set("state", strings.Repeat("s", 4000)); return q }(),
		"other server":  func() url.Values { q := base(); q.Set("resource", "https://elsewhere.example/mcp"); return q }(),
	} {
		resp := get(q)
		if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
			t.Fatalf("%s: %d Location %q", name, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	f.s.oauth.mu.Lock()
	held := len(f.s.oauth.pending)
	f.s.oauth.mu.Unlock()
	if held != 0 {
		t.Fatalf("%d bad requests were kept", held)
	}

	// an address is limited, and what is kept is bounded
	limited := 0
	for i := 0; i < 40; i++ {
		if get(base()).StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("40 authorize pages a moment from one address were all served")
	}
	f.s.authorizeRate = newBucketLimiter(1e9, 1<<30)
	full := 0
	for i := 0; i < oauthMaxPending+20; i++ {
		if get(base()).StatusCode == http.StatusServiceUnavailable {
			full++
		}
	}
	f.s.oauth.mu.Lock()
	held = len(f.s.oauth.pending)
	f.s.oauth.mu.Unlock()
	if held > oauthMaxPending || full == 0 {
		t.Fatalf("%d sign-ins held (%d refused)", held, full)
	}
}

// pkcePair is a verifier and its S256 challenge.
func pkcePair() (string, string) {
	v := "a-verifier-of-enough-length-for-pkce-0123456789"
	sum := sha256.Sum256([]byte(v))
	return v, base64.RawURLEncoding.EncodeToString(sum[:])
}

// formField is a hidden field's value on the authorize page.
func formField(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + name + `" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no %s field in %s", name, page)
	}
	return m[1]
}

func TestAnUnknownReturnAddressNeedsTheGMsWord(t *testing.T) {
	f := newFixture(t)
	resp, _ := http.Post(f.ts.URL+"/oauth/register", "application/json",
		strings.NewReader(`{"client_name":"ChatGPT","redirect_uris":["https://chatgpt.com.example.net/cb"]}`))
	var reg map[string]any
	json.NewDecoder(resp.Body).Decode(&reg)
	clientID := reg["client_id"].(string)
	_, challenge := pkcePair()
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"https://chatgpt.com.example.net/cb"},
		"state": {"s"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "scope": {"read write admin"}}
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	raw, _ := io.ReadAll(resp.Body)
	page := string(raw)
	if !strings.Contains(page, "chatgpt.com.example.net") || !strings.Contains(page, "anyone can register one") ||
		!strings.Contains(page, `name="trust"`) {
		t.Fatalf("no warning for an unknown return address: %s", page)
	}
	if !regexp.MustCompile(`value="read" checked`).MatchString(page) {
		t.Fatal("an unknown client is not offered read first")
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := url.Values{"req": {formField(t, page, "req")}, "csrf": {formField(t, page, "csrf")}, "username": {"gm"},
		"password": {testPassword}, "scope": {"write"}, "action": {"allow"}}
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", form)
	if resp.StatusCode == http.StatusFound {
		t.Fatal("allowed without the GM's word for an unknown address")
	}
	form.Set("trust", "1")
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", form)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("with the GM's word: %d", resp.StatusCode)
	}

	// a denial for an unknown address is a page, not a redirect there
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	raw, _ = io.ReadAll(resp.Body)
	page = string(raw)
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", url.Values{"req": {formField(t, page, "req")},
		"csrf": {formField(t, page, "csrf")}, "action": {"deny"}})
	if resp.Header.Get("Location") != "" {
		t.Fatalf("a denial redirected to %s", resp.Header.Get("Location"))
	}
}

// oauthTokens runs the flow for a client returning to ChatGPT and gives the
// token answer.
func oauthTokens(t *testing.T, f *fixture, user, scope, code string) (string, map[string]any) {
	t.Helper()
	ret := "https://chatgpt.com/connector_platform_oauth_redirect"
	resp, _ := http.Post(f.ts.URL+"/oauth/register", "application/json",
		strings.NewReader(`{"client_name":"ChatGPT","redirect_uris":["`+ret+`"]}`))
	var reg map[string]any
	json.NewDecoder(resp.Body).Decode(&reg)
	clientID := reg["client_id"].(string)
	verifier, challenge := pkcePair()
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {ret},
		"state": {"s"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	resp, _ = http.Get(f.ts.URL + "/oauth/authorize?" + q.Encode())
	raw, _ := io.ReadAll(resp.Body)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ = noRedirect.PostForm(f.ts.URL+"/oauth/authorize", url.Values{"req": {formField(t, string(raw), "req")},
		"csrf": {formField(t, string(raw), "csrf")}, "username": {user}, "password": {testPassword}, "code": {code},
		"scope": {scope}, "action": {"allow"}})
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || loc.Query().Get("code") == "" {
		body, _ := io.ReadAll(resp.Body)
		return clientID, map[string]any{"_status": resp.StatusCode, "_page": string(body)}
	}
	resp, _ = http.PostForm(f.ts.URL+"/oauth/token", url.Values{"grant_type": {"authorization_code"},
		"code": {loc.Query().Get("code")}, "client_id": {clientID}, "redirect_uri": {ret}, "code_verifier": {verifier}})
	var tok map[string]any
	json.NewDecoder(resp.Body).Decode(&tok)
	return clientID, tok
}

func TestARefreshTokenIsGoodOnce(t *testing.T) {
	f := newFixture(t)
	clientID, tok := oauthTokens(t, f, "gm", "write", "")
	refresh := func(rt string) (int, map[string]any) {
		resp, _ := http.PostForm(f.ts.URL+"/oauth/token", url.Values{"grant_type": {"refresh_token"},
			"refresh_token": {rt}, "client_id": {clientID}})
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	st, tok2 := refresh(tok["refresh_token"].(string))
	if st != 200 {
		t.Fatalf("refresh: %d %v", st, tok2)
	}
	// at once again: the client racing itself; refused, and nothing else
	if st, out := refresh(tok["refresh_token"].(string)); st == 200 {
		t.Fatalf("a refresh token worked twice: %v", out)
	}
	if code, _ := f.bearer(tok2["access_token"].(string)).op("server_status", nil); code != 200 {
		t.Fatalf("the new access token after a racing refresh: %d", code)
	}
	// later again: a copy in other hands; the grant goes
	mustExec(t, f.db, `UPDATE gm_tokens SET last_used = ? WHERE kind = 'refresh' AND revoked = 1`, stamp(time.Now().Add(-time.Minute)))
	if st, out := refresh(tok["refresh_token"].(string)); st == 200 {
		t.Fatalf("a refresh token worked twice: %v", out)
	}
	if code, _ := f.bearer(tok2["access_token"].(string)).op("server_status", nil); code != 401 {
		t.Fatalf("the grant's newest access token after a reuse: %d", code)
	}
	if st, _ := refresh(tok2["refresh_token"].(string)); st == 200 {
		t.Fatal("the grant's newest refresh token after a reuse still works")
	}
}

func TestAnAdminsToolsNeedAnAuthenticator(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	code, out := boss.do("POST", "/api/auth/login", map[string]any{"username": "boss", "password": testPassword})
	if code != 200 {
		t.Fatalf("sign-in: %d %v", code, out)
	}
	boss.csrf, _ = out["csrf"].(string)
	user := out["user"].(map[string]any)
	if user["admin"] != false || user["adminNeedsTOTP"] != true {
		t.Fatalf("an admin with the password alone: %v", user)
	}
	if code, out := boss.op("sql_query", map[string]any{"sql": "SELECT 1"}); code != 403 || !strings.Contains(fmt.Sprint(out["error"]), "authenticator") {
		t.Fatalf("sql_query with the password alone: %d %v", code, out)
	}
	if code, _ := boss.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "admin", "days": 7}); code != 403 {
		t.Fatalf("an admin token with the password alone: %d", code)
	}
	if code, _ := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "boss", "password": testPassword, "scope": "admin"}); code != 403 {
		t.Fatalf("a terminal's admin token with the password alone: %d", code)
	}
	if _, tok := oauthTokens(t, f, "boss", "admin", ""); tok["access_token"] != nil {
		t.Fatalf("an agent's admin grant with the password alone: %v", tok)
	}

	// enrolling one in this session is an authenticator's sign-in
	code, out = boss.do("POST", "/api/auth/totp", map[string]any{"action": "begin"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, out)
	}
	secret := out["secret"].(string)
	if code, out := boss.do("POST", "/api/auth/totp", map[string]any{"action": "confirm", "code": totpNow(secret)}); code != 200 {
		t.Fatalf("confirm: %d %v", code, out)
	}
	if code, out := boss.op("sql_query", map[string]any{"sql": "SELECT 1"}); code != 200 {
		t.Fatalf("sql_query after enrolling: %d %v", code, out)
	}

	// a terminal's admin token, made with the code, is an admin's while the
	// account keeps its authenticator
	mustExec(t, f.db, `UPDATE gm_totp SET last_step = 0 WHERE accountid = 1`)
	code, out = f.browser().do("POST", "/api/auth/token", map[string]any{"username": "boss", "password": testPassword,
		"code": totpNow(secret), "scope": "admin"})
	if code != 200 {
		t.Fatalf("an admin token with the code: %d %v", code, out)
	}
	admin := f.bearer(out["token"].(string))
	if code, _ := admin.op("sql_query", map[string]any{"sql": "SELECT 1"}); code != 200 {
		t.Fatalf("the admin token: %d", code)
	}
	mustExec(t, f.db, `DELETE FROM gm_totp WHERE accountid = 1`)
	if code, _ := admin.op("sql_query", map[string]any{"sql": "SELECT 1"}); code != 403 {
		t.Fatalf("the admin token once the authenticator is gone: %d", code)
	}
}

func TestCharactersOfHigherAccountsAreGuarded(t *testing.T) {
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level,
		haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar)
		VALUES ('Bossman', 1, 8, 1, 0, 0, 1, 50, 0, 0, 0, 0, 0, 0, 25300, 54, 15700, 100)`)
	mustExec(t, f.db, `INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (900, 8, 1, 100, 0, 10, -1, 0, 0)`)
	gm := f.browser()
	gm.login("gm")
	refused := map[string]func() (int, map[string]any){
		"set_character on an admin's": func() (int, map[string]any) {
			return gm.op("set_character", map[string]any{"character": "Bossman", "level": 1})
		},
		"move a character into an admin's account": func() (int, map[string]any) {
			return gm.op("update_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7}, "values": map[string]any{"accountid": 1}})
		},
		"give an admin's character an item": func() (int, map[string]any) {
			return gm.op("give_item", map[string]any{"character": "Bossman", "item": "Bread"})
		},
		"grant an admin's character a spell": func() (int, map[string]any) {
			return gm.op("grant_spell", map[string]any{"character": "Bossman", "spell": "Root"})
		},
		"add a row to an admin's character": func() (int, map[string]any) {
			return gm.op("insert_row", map[string]any{"table": "charinventory", "values": map[string]any{"serverid": 8, "patternid": 11,
				"stackleft": 1, "remainHP": 0, "remaincharge": 0, "equiploc": -1, "location": 0, "listnumber": 1}})
		},
		"edit an admin's character's item": func() (int, map[string]any) {
			return gm.op("update_row", map[string]any{"table": "charinventory", "key": map[string]any{"itemid": 900}, "values": map[string]any{"stackleft": 5}})
		},
		"move an item to an admin's character": func() (int, map[string]any) {
			mustExec(t, f.db, `INSERT OR IGNORE INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
				VALUES (901, 7, 1, 100, 0, 10, -1, 0, 1)`)
			return gm.op("update_row", map[string]any{"table": "charinventory", "key": map[string]any{"itemid": 901}, "values": map[string]any{"serverid": 8}})
		},
	}
	for name, try := range refused {
		if code, out := try(); code != 403 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	// a player's character is a GM's to change
	if code, out := gm.op("set_character", map[string]any{"character": "Hero", "level": 12}); code != 200 {
		t.Fatalf("a player's character: %d %v", code, out)
	}
}

func TestAnAdminsSQLIsHiddenFromGMs(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	if code, out := boss.op("sql_query", map[string]any{"sql": "SELECT 'marker-7f3a' AS x"}); code != 200 {
		t.Fatalf("sql_query: %d %v", code, out)
	}
	gm := f.browser()
	gm.login("gm")
	_, out := gm.op("audit_log", map[string]any{"action": "sql"})
	entries := out["entries"].([]any)
	if len(entries) == 0 || strings.Contains(fmt.Sprint(entries), "marker-7f3a") {
		t.Fatalf("a GM's view of an admin's SQL: %v", entries)
	}
	if _, out := gm.op("audit_log", map[string]any{"search": "marker-7f3a"}); len(out["entries"].([]any)) != 0 {
		t.Fatal("a GM's search matched an admin's SQL")
	}
	if _, out := boss.op("audit_log", map[string]any{"search": "marker-7f3a"}); len(out["entries"].([]any)) == 0 {
		t.Fatal("an admin's search misses their own SQL")
	}
}

func TestSQLQueryRunsOnlyThePragmasThatRead(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	for q, want := range map[string]int{
		"PRAGMA table_info(npcs)":     200,
		"PRAGMA integrity_check":      200,
		"PRAGMA foreign_keys = ON":    400,
		"PRAGMA synchronous = OFF":    400,
		"PRAGMA query_only = 0":       400,
		"PRAGMA cache_size(1000)":     400,
		"SELECT COUNT(*) FROM npcs":   200,
		"DELETE FROM npcs WHERE 0":    400,
		"PRAGMA journal_mode = WAL":   400,
		"PRAGMA table_info('spells')": 200,
	} {
		if code, out := boss.op("sql_query", map[string]any{"sql": q}); code != want {
			t.Errorf("%s: %d %v", q, code, out["error"])
		}
	}
}

func TestSignInTakesJSONFromThisSiteOnly(t *testing.T) {
	f := newFixture(t)
	post := func(path, ctype, origin, body string) int {
		req, _ := http.NewRequest("POST", f.ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", ctype)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	login := `{"username":"gm","password":"` + testPassword + `"}`
	if code := post("/api/auth/login", "text/plain", "", login); code == 200 {
		t.Fatal("a text/plain sign-in (another site's form) was taken")
	}
	if code := post("/api/auth/login", "application/json", "https://evil.example", login); code != 403 {
		t.Fatalf("a sign-in from another site's page: %d", code)
	}
	if code := post("/api/auth/token", "application/json", "https://evil.example", login); code != 403 {
		t.Fatalf("a token sign-in from another site's page: %d", code)
	}
	// signing out needs the page's CSRF token
	gm := f.browser()
	gm.login("gm")
	csrf := gm.csrf
	gm.csrf = ""
	if code, _ := gm.do("POST", "/api/auth/logout", map[string]any{}); code != 403 {
		t.Fatalf("a sign-out without the CSRF token: %d", code)
	}
	gm.csrf = csrf
	if code, _ := gm.op("server_status", nil); code != 200 {
		t.Fatalf("the session after a refused sign-out: %d", code)
	}
}

func TestTokensExpireAndGoWithThePassword(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	for _, days := range []int{0, -1, maxTokenDays + 1} {
		if code, _ := gm.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "x", "scope": "read", "days": days}); code != 400 {
			t.Fatalf("a token of %d days: %d", days, code)
		}
		if code, _ := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "gm", "password": testPassword, "days": days}); code != 400 {
			t.Fatalf("a terminal token of %d days: %d", days, code)
		}
	}
	code, out := gm.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "agent", "scope": "write", "days": 30})
	if code != 200 {
		t.Fatalf("a 30-day token: %d %v", code, out)
	}
	tok := f.bearer(out["token"].(string))
	boss := f.browser()
	boss.login("boss")
	if code, out := boss.op("set_account_password", map[string]any{"account": "gm", "password": "a-new-one"}); code != 200 || out["tokensRevoked"] != float64(1) {
		t.Fatalf("set the password: %d %v", code, out)
	}
	if code, _ := tok.op("server_status", nil); code != 401 {
		t.Fatalf("a token made with the old password: %d", code)
	}
}

func TestMCPBatchesAreBoundedAndCharged(t *testing.T) {
	f := newFixture(t)
	f.s.rate = newRateLimiter(3, time.Minute)
	code, out := f.browser().do("POST", "/api/auth/token", map[string]any{"username": "gm", "password": testPassword, "name": "t", "scope": "read"})
	if code != 200 {
		t.Fatalf("token: %d %v", code, out)
	}
	tok := out["token"].(string)
	batch := func(n int) int {
		var b []map[string]any
		for i := 0; i < n; i++ {
			b = append(b, map[string]any{"jsonrpc": "2.0", "id": i, "method": "tools/call",
				"params": map[string]any{"name": "count_rows", "arguments": map[string]any{"table": "npcs"}}})
		}
		raw, _ := json.Marshal(b)
		req, _ := http.NewRequest("POST", f.ts.URL+"/mcp", strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := batch(mcpMaxBatch + 1); code != 400 {
		t.Fatalf("a batch of %d: %d", mcpMaxBatch+1, code)
	}
	if code := batch(10); code != http.StatusTooManyRequests {
		t.Fatalf("a batch of 10 under a rate of 3 a minute: %d", code)
	}
}

// A browser drops a __Host- cookie silently unless it is Secure, on Path=/
// and has no Domain: then nobody could sign in, and nothing would say why.
func TestTheSessionCookieKeepsTheHostPrefixRules(t *testing.T) {
	f := newFixture(t)
	u, _ := url.Parse("https://gm.example.test")
	f.s.public = u
	body := strings.NewReader(`{"username":"gm","password":"` + testPassword + `"}`)
	req, _ := http.NewRequest("POST", f.ts.URL+"/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	set := resp.Header.Get("Set-Cookie")
	for _, want := range []string{"__Host-eqoa_gm=", "Path=/;", "Secure", "HttpOnly", "SameSite=Strict"} {
		if !strings.Contains(set+";", want) {
			t.Errorf("Set-Cookie %q lacks %q", set, want)
		}
	}
	if strings.Contains(strings.ToLower(set), "domain=") {
		t.Errorf("Set-Cookie %q names a domain", set)
	}
}

func TestTheCookieIsAHostCookieOnItsOwnHost(t *testing.T) {
	for _, c := range []struct{ public, name, path string }{
		{"https://gm.example.test", "__Host-eqoa_gm", "/"},
		{"https://example.test/eqoa/gm", "eqoa_gm", "/eqoa/gm/"},
		{"", "eqoa_gm", "/"},
	} {
		f := newFixture(t)
		if c.public != "" {
			u, _ := url.Parse(c.public)
			f.s.public = u
		}
		if f.s.cookieName() != c.name || f.s.cookiePath() != c.path {
			t.Errorf("%q: cookie %s on %s", c.public, f.s.cookieName(), f.s.cookiePath())
		}
	}
}

func TestSQLExecutesBackupsArePruned(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	for i := 0; i < keepSQLBackups+3; i++ {
		if code, out := boss.op("sql_execute", map[string]any{"sql": fmt.Sprintf("UPDATE npcs SET npc_level = %d WHERE id = 502", i+1)}); code != 200 {
			t.Fatalf("sql_execute: %d %v", code, out)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(f.s.backupDir(), "*before-sql*.db"))
	if len(matches) != keepSQLBackups {
		t.Fatalf("%d backups kept, not %d", len(matches), keepSQLBackups)
	}
	os.RemoveAll(f.s.backupDir())
}

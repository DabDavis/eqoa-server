package tcp

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

func termsRequest(h http.Handler, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPlayerTermsAgeBeforeCredentialsAndExplicitAccountDecision(t *testing.T) {
	r := newPwRig(t)
	r.s.EnablePlayerTerms(true)
	h := r.s.Handler()
	get := termsRequest(h, "GET", "/terms", nil, nil)
	if get.Code != 200 || !strings.Contains(get.Body.String(), "How old are you?") || strings.Contains(get.Body.String(), `name="username"`) {
		t.Fatal("the public age screen must precede account fields")
	}
	for _, section := range []string{"Version " + PlayerTermsVersion + " ·", `id="ordinary-game-losses"`, `id="player-claim-limit"`,
		"If money damages for an ordinary game loss defined above", "You keep all rights the law gives you as a minor"} {
		if !strings.Contains(get.Body.String(), section) {
			t.Fatalf("signed-out agreement missing %q", section)
		}
	}
	privacy := termsRequest(h, "GET", "/privacy", nil, nil)
	if privacy.Code != 200 {
		t.Fatalf("signed-out privacy notice: %d", privacy.Code)
	}
	for _, disclosure := range []string{"Version 1.1", "up to 24 hours", "detailed network logging", "including tells"} {
		if !strings.Contains(privacy.Body.String(), disclosure) {
			t.Fatalf("privacy notice missing %q", disclosure)
		}
	}
	age := func(value string) *httptest.ResponseRecorder {
		return termsRequest(h, "POST", "/terms", url.Values{"step": {"age"}, "age": {value}}, nil)
	}
	young := age("12")
	if young.Code != 403 || strings.Contains(young.Body.String(), `name="age"`) || strings.Contains(young.Body.String(), `name="username"`) {
		t.Fatal("underage response offered another attempt or requested identifiers")
	}
	if retry := termsRequest(h, "POST", "/terms", url.Values{"step": {"age"}, "age": {"18"}}, young.Result().Cookies()[0]); retry.Code != 403 {
		t.Fatal("same browser immediately retried after an underage answer")
	}
	mature := age("13")
	if mature.Code != 303 {
		t.Fatalf("age boundary: %d", mature.Code)
	}
	cookie := mature.Result().Cookies()[0]
	for _, path := range []string{"/terms", "/terms?mode=create"} {
		page := termsRequest(h, "GET", path, nil, cookie).Body.String()
		box := strings.Index(page, `name="agree" value="`+PlayerTermsVersion+`" required`)
		if box < 0 || strings.Contains(page, " checked") {
			t.Fatalf("%s must require an unchecked box for the current terms", path)
		}
		for _, notice := range []string{"Vanilla Test - Wipes", "will be wiped for testing", "Sandbox Test - No Wipes", "no planned character wipes",
			"Bugs and rollbacks can permanently erase progress on either server", "no guarantee of uptime", "those ordinary game losses",
			"may mute, suspend or permanently ban", "give up money damages for ordinary game losses", "own ordinary negligence",
			"US $250 fallback cap", `href="#ordinary-game-losses"`, `href="#player-claim-limit"`} {
			if !strings.Contains(page[:box], notice) {
				t.Fatalf("%s must show %q before the agreement box", path, notice)
			}
		}
		if !strings.Contains(page[box:], "I accept these gameplay risks and agree") {
			t.Fatalf("%s must name the gameplay risks in the affirmative choice", path)
		}
	}
	f := url.Values{"step": {"accept"}, "username": {"alice"}, "password": {"pw-alice"}, "proof": {cookie.Value}}
	account, err := r.s.accountStore.GetByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	const previousVersion = "1.3"
	if err := r.db.AcceptPlayerTerms(account, previousVersion); err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		got, err := r.db.HasPlayerTerms(account.ID, PlayerTermsVersion)
		if err != nil || got != want {
			t.Fatalf("receipt = %v, %v; want %v", got, err, want)
		}
	}
	check(false)
	for _, bad := range []string{"", "1.0", "1.1", "1.2", previousVersion} {
		f.Set("agree", bad)
		if resp := termsRequest(h, "POST", "/terms", f, cookie); resp.Code != 400 {
			t.Fatalf("accepted %q: %d", bad, resp.Code)
		}
	}
	f.Set("agree", PlayerTermsVersion)
	if resp := termsRequest(h, "POST", "/terms", f, nil); resp.Code != 403 {
		t.Fatal("accepted without eligibility")
	}
	otherBrowser := age("25").Result().Cookies()[0]
	if resp := termsRequest(h, "POST", "/terms", f, otherBrowser); resp.Code != 403 {
		t.Fatal("accepted using a form from another browser")
	}
	crossSite := httptest.NewRequest("POST", "/terms", strings.NewReader(f.Encode()))
	crossSite.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	crossSite.Header.Set("Origin", "https://other.example")
	crossSite.AddCookie(cookie)
	result := httptest.NewRecorder()
	h.ServeHTTP(result, crossSite)
	if result.Code != 403 {
		t.Fatal("a cross-site form recorded acceptance")
	}
	f.Set("step", "decline")
	if resp := termsRequest(h, "POST", "/terms", f, cookie); resp.Code != 200 || !strings.Contains(resp.Body.String(), "You declined") {
		t.Fatal("decline")
	}
	check(false)
	f.Set("step", "accept")
	f.Set("password", "incorrect")
	if resp := termsRequest(h, "POST", "/terms", f, cookie); resp.Code != 403 {
		t.Fatal("accepted with wrong credentials")
	}
	check(false)
	f.Set("password", "pw-alice")
	for range 2 {
		if resp := termsRequest(h, "POST", "/terms", f, cookie); resp.Code != 200 || !strings.Contains(resp.Body.String(), "Agreement recorded") {
			t.Fatalf("accept: %d", resp.Code)
		}
	}
	check(true)
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM account_terms_acceptances WHERE accountid = ? AND version = ?", account.ID, PlayerTermsVersion).Scan(&count); err != nil || count != 1 {
		t.Fatalf("receipt count %d: %v", count, err)
	}
	if ok, err := r.db.HasPlayerTerms(account.ID, previousVersion); err != nil || !ok {
		t.Fatalf("the previous version's receipt must remain on record: %v, %v", ok, err)
	}
	f.Set("step", "create")
	f.Set("username", "newplayer")
	if resp := termsRequest(h, "POST", "/terms", f, cookie); resp.Code != 200 || !strings.Contains(resp.Body.String(), "Agreement recorded") {
		t.Fatalf("signup: %d", resp.Code)
	}
	created, err := r.s.accountStore.GetByUsername("newplayer")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := r.db.HasPlayerTerms(created.ID, PlayerTermsVersion); err != nil || !ok {
		t.Fatalf("new account receipt: %v, %v", ok, err)
	}
	if legacy := termsRequest(h, "POST", "/api/register", nil, nil); legacy.Code != 403 || !strings.Contains(legacy.Body.String(), playerTermsURL) {
		t.Fatal("legacy registration did not redirect players")
	}
	r.s.EnablePlayerTerms(false)
	if resp := termsRequest(r.s.Handler(), "POST", "/terms", f, cookie); resp.Code != 403 {
		t.Fatal("secondary accepted a write")
	}
}

func TestPlayerTermsTCPAndUDPRequireCurrentReceipt(t *testing.T) {
	s, store := newLoginTestServer(t, func(c *config.AuthConfig) {
		c.RequirePlayerTerms = true
		c.AllowAutoRegister = true
	})
	hash, err := auth.HashPassword("password", 4)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.Create("player", hash)
	if err != nil {
		t.Fatal(err)
	}
	reply := s.handleLogin(buildLoginPayload(t, "player", "password"))
	if binary.BigEndian.Uint32(reply[8:12]) != 1 || binary.BigEndian.Uint32(reply[84:88]) != EmailRefusalStatus {
		t.Fatal("Station did not open the account notice")
	}
	if _, ok := s.tickets.Valid("player"); ok {
		t.Fatal("unaccepted player received a ticket")
	}
	if _, why := s.CheckUDPPassword("player", "password", udpFrom); why == "" {
		t.Fatal("UDP admitted an unaccepted player")
	}
	s.handleLogin(buildLoginPayload(t, "newuser", "password"))
	if _, err := store.GetByUsername("newuser"); err == nil {
		t.Fatal("game login registered an account before eligibility")
	}
	s.PlayerTermsAllowed = func(id int) bool { return id == account.ID }
	if !login(t, s, "player", "password") {
		t.Fatal("accepted existing player could not log in")
	}
	s.tickets.Revoke("player")
	if _, why := s.CheckUDPPassword("player", "password", udpFrom); why != "" {
		t.Fatalf("accepted UDP reconnect: %s", why)
	}
}

func TestPlayerTermsOnePortalDecisionForBothShards(t *testing.T) {
	openShard := func(path string) *database.SQLiteDB {
		t.Helper()
		db, err := database.NewSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	dir := t.TempDir()
	primaryPath := filepath.Join(dir, "heirloom.db")
	primary := openShard(primaryPath)
	secondary := openShard(filepath.Join(dir, "live.db"))
	mirror := database.NewAccountMirror(secondary, primaryPath)
	newLogin := func(db *database.SQLiteDB, read func(int, string) (bool, error)) *AuthServer {
		cfg := config.DefaultConfig()
		cfg.Auth.RequirePlayerTerms = true
		cfg.Auth.BCryptCost = 4
		server := NewAuthServer(cfg, database.NewSQLiteAccountStore(db), auth.NewTicketStore(0))
		server.PlayerTermsAllowed = func(id int) bool {
			ok, err := read(id, PlayerTermsVersion)
			return err == nil && ok
		}
		return server
	}
	heirloom := newLogin(primary, primary.HasPlayerTerms)
	live := newLogin(secondary, mirror.HasPlayerTerms)
	live.SetAccountRefresh(mirror.PullOne)
	const password = "fixture-pass"
	hash, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.NewSQLiteAccountStore(primary).CreateWithPlayerTerms("returning", hash, "1.3"); err != nil {
		t.Fatal(err)
	}
	checkEntry := func(username string, want bool) {
		t.Helper()
		for shard, server := range map[string]*AuthServer{"Heirloom": heirloom, "Live": live} {
			_, refusal := server.CheckUDPPassword(username, password, udpFrom)
			if (refusal == "") != want {
				t.Fatalf("%s on %s: refusal %q, want admitted=%v", username, shard, refusal, want)
			}
			if username == "returning" && !want && refusal != "terms" {
				t.Fatalf("%s refused the old agreement for the wrong reason: %q", shard, refusal)
			}
		}
	}
	portal := NewAccountServer(primary, 0, 4)
	portal.EnablePlayerTerms(true)
	h := portal.Handler()
	players := []struct{ name, step string }{
		{"returning", "accept"},
		{"newplayer", "create"},
	}
	for _, player := range players {
		checkEntry(player.name, false)
		age := termsRequest(h, "POST", "/terms", url.Values{"step": {"age"}, "age": {"18"}}, nil)
		if age.Code != http.StatusSeeOther || len(age.Result().Cookies()) != 1 {
			t.Fatalf("age check for %s: %d, cookies %d", player.name, age.Code, len(age.Result().Cookies()))
		}
		cookie := age.Result().Cookies()[0]
		form := url.Values{"step": {player.step}, "username": {player.name}, "password": {password}, "proof": {cookie.Value}}
		if unchecked := termsRequest(h, "POST", "/terms", form, cookie); unchecked.Code != http.StatusBadRequest {
			t.Fatalf("%s unchecked: %d", player.step, unchecked.Code)
		}
		checkEntry(player.name, false)
		form.Set("agree", PlayerTermsVersion)
		if reply := termsRequest(h, "POST", "/terms", form, cookie); reply.Code != http.StatusOK {
			t.Fatalf("%s: %d", player.step, reply.Code)
		}
		checkEntry(player.name, true)
	}
}

func TestPlayerTermsLegacyStationSignupPointsToWeb(t *testing.T) {
	s, _ := newLoginTestServer(t, func(c *config.AuthConfig) { c.RequirePlayerTerms = true })
	server, client := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		s.HandleConnection(server)
	}()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(client, make([]byte, 40)); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 476)
	binary.BigEndian.PutUint32(frame, uint32(len(frame)))
	binary.BigEndian.PutUint32(frame[4:], MsgSubmitAcctCreate)
	if _, err := client.Write(frame); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 276)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(reply[12:16]) != 0 || !bytes.Contains(reply, []byte(playerTermsAddress)) {
		t.Fatal("Station signup must direct the player to the age and acceptance form")
	}
}

func TestPlayerTermsStationPagesPreserveDiscAgreement(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.RequirePlayerTerms = true
	pak := bytes.Join([][]byte{pakEntry("data/ui/IDCreate.xml", "old form"), pakEntry(EmailRefusalPage, "old key"),
		pakEntry("DATA/EULA.TXT", "original disc agreement sentinel")}, nil)
	served := (&PatchServer{cfg: cfg}).repointStationPak(pak)
	if bytes.Count(served, []byte(playerTermsAddress)) < 2 || !bytes.Contains(served, pakEntry("DATA/EULA.TXT", "original disc agreement sentinel")) {
		t.Fatal("account pages must direct players to the project agreement and preserve the existing EULA bytes")
	}
}

package tcp

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
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
	if !strings.Contains(get.Body.String(), "Version "+PlayerTermsVersion+" ·") || !strings.Contains(get.Body.String(), `id="player-claim-limit"`) {
		t.Fatal("the signed-out page must show the full current agreement and its damages limit")
	}
	if privacy := termsRequest(h, "GET", "/privacy", nil, nil); privacy.Code != 200 || !strings.Contains(privacy.Body.String(), "including tells") {
		t.Fatal("privacy must be readable without age or account information")
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
		if !strings.Contains(page[:box], "US $250 limit per player") || !strings.Contains(page[:box], `href="#player-claim-limit"`) {
			t.Fatalf("%s must show the limit and link to its exceptions before the agreement box", path)
		}
	}
	f := url.Values{"step": {"accept"}, "username": {"alice"}, "password": {"pw-alice"}, "proof": {cookie.Value}}
	account, err := r.s.accountStore.GetByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	const previousVersion = "1.1"
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
	for _, bad := range []string{"", "1.0", previousVersion} {
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

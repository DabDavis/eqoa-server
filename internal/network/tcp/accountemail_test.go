package tcp

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

// The account page's e-mail (accountemail.go) and STATION's e-mail forms
// (stationemail.go, stationpages.go).

type mailRig struct {
	*pwRig
	sent *mailer.Memory
	mail *accountmail.Service
	totp map[int]string
}

func newMailRig(t *testing.T) *mailRig {
	t.Helper()
	r := &mailRig{pwRig: newPwRig(t), sent: &mailer.Memory{}, totp: map[int]string{}}
	r.mail = accountmail.NewWithQueue(r.db, config.MailConfig{Accounts: true}, mailer.NewQueue(r.sent, 0, []time.Duration{time.Millisecond}))
	r.mail.Now = func() time.Time { return r.clock }
	t.Cleanup(r.mail.Close)
	r.s.Mail = r.mail
	r.s.TOTP = func(id int, code string) (bool, bool) {
		want, ok := r.totp[id]
		return ok, ok && code == want
	}
	r.h = r.s.Handler()
	return r
}

func (r *mailRig) mails(n int) []mailer.Message {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(r.sent.All()) < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := r.sent.All()
	if len(got) != n {
		r.t.Fatalf("%d mails, want %d", len(got), n)
	}
	return got
}

var reLink = regexp.MustCompile(`#(verify|reset)=([A-Za-z0-9_-]+)`)

func linkToken(t *testing.T, m mailer.Message, kind string) string {
	t.Helper()
	g := reLink.FindStringSubmatch(m.Text)
	if g == nil || g[1] != kind {
		t.Fatalf("no %s link in %q", kind, m.Text)
	}
	return g[2]
}

func (r *mailRig) id(name string) int {
	acc, err := database.NewSQLiteAccountStore(r.db).GetByUsername(name)
	if err != nil {
		r.t.Fatal(err)
	}
	return acc.ID
}

func TestThePageSetsAndVerifiesAnEmail(t *testing.T) {
	r := newMailRig(t)
	// a wrong password: nothing set, nothing sent
	_, out, _ := r.post("/api/password/email", "203.0.113.1", true, map[string]any{"account": "alice", "password": "wrong", "email": "alice@example.com"})
	if out["ok"] != false {
		t.Fatalf("a wrong password: %v", out)
	}
	// without the page's header: refused
	if code, _, _ := r.post("/api/password/email", "203.0.113.1", false, map[string]any{"account": "alice", "password": "pw-alice", "email": "alice@example.com"}); code != 400 {
		t.Fatalf("no header: %d", code)
	}
	// a bad address
	if code, out, _ := r.post("/api/password/email", "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice", "email": "alice"}); code != 400 || out["ok"] != false {
		t.Fatalf("a bad address: %d %v", code, out)
	}
	code, out, _ := r.post("/api/password/email", "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice", "email": "Alice@Example.com"})
	if code != 200 || out["ok"] != true || out["email"] != "a***@e***.com" || strings.Contains(out["message"].(string), "alice@example.com") {
		t.Fatalf("set: %d %v", code, out)
	}
	tok := linkToken(t, r.mails(1)[0], "verify")
	// the status, to the owner: the address whole
	_, out, _ = r.post("/api/password/email/status", "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice"})
	if out["email"] != "alice@example.com" || out["verified"] != false {
		t.Fatalf("status: %v", out)
	}
	// resend at once: the gap
	if code, out, _ := r.post("/api/password/email/resend", "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice"}); code != 429 {
		t.Fatalf("resend at once: %d %v", code, out)
	}
	// the link
	if _, out, _ := r.post("/api/password/email/verify", "203.0.113.5", true, map[string]any{"token": "nope"}); out["ok"] != false {
		t.Fatalf("a bad token: %v", out)
	}
	_, out, _ = r.post("/api/password/email/verify", "203.0.113.5", true, map[string]any{"token": tok})
	if out["ok"] != true || !strings.Contains(out["message"].(string), "a***@e***.com") {
		t.Fatalf("verify: %v", out)
	}
	_, out, _ = r.post("/api/password/email/status", "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice"})
	if out["verified"] != true {
		t.Fatalf("status after: %v", out)
	}
	// an account with an authenticator changes its address with a code
	r.totp[r.id("bob")] = "123456"
	_, out, _ = r.post("/api/password/email", "203.0.113.2", true, map[string]any{"account": "bob", "password": "pw-bob", "email": "bob@example.com"})
	if out["ok"] != false || out["needCode"] != true {
		t.Fatalf("no code: %v", out)
	}
	_, out, _ = r.post("/api/password/email", "203.0.113.2", true, map[string]any{"account": "bob", "password": "pw-bob", "code": "654321", "email": "bob@example.com"})
	if out["ok"] != false {
		t.Fatalf("a wrong code: %v", out)
	}
	_, out, _ = r.post("/api/password/email", "203.0.113.2", true, map[string]any{"account": "bob", "password": "pw-bob", "code": "123456", "email": "bob@example.com"})
	if out["ok"] != true {
		t.Fatalf("the right code: %v", out)
	}
	// the page says the tab is on, and never carries a token
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/password", nil))
	if !strings.Contains(rec.Body.String(), `<body data-mail="1">`) || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("the page with e-mail on")
	}
}

func TestEmailRoutesAreOffWithMailOff(t *testing.T) {
	r := newPwRig(t)
	for _, p := range []string{"/api/password/email", "/api/password/email/status", "/api/password/email/resend",
		"/api/password/email/verify", "/api/password/reset"} {
		if code, _, _ := r.post(p, "203.0.113.1", true, map[string]any{"account": "alice", "password": "pw-alice"}); code != 404 {
			t.Errorf("%s with e-mail off: %d", p, code)
		}
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/password", nil))
	if !strings.Contains(rec.Body.String(), `<body data-mail="0">`) {
		t.Fatal("the page with e-mail off")
	}
}

func TestTheForgotFormMailsAVerifiedAddress(t *testing.T) {
	r := newMailRig(t)
	// alice verified, bob with an address not verified, carol with none
	r.mail.SetEmail(r.id("alice"), "alice@example.com", "")
	r.mail.Verify(linkToken(t, r.mails(1)[0], "verify"))
	r.mail.SetEmail(r.id("bob"), "bob@example.com", "")
	r.mails(2)
	answers := map[string]string{}
	for i, name := range []string{"alice", "bob", "carol", "nobody"} {
		code, out, _ := r.post("/api/password/forgot", "203.0.113."+string(rune('1'+i)), true, map[string]any{"account": name})
		if code != 200 || out["ok"] != true {
			t.Fatalf("%s: %d %v", name, code, out)
		}
		answers[name] = out["message"].(string)
	}
	for n, a := range answers {
		if a != answers["alice"] {
			t.Fatalf("the answer for %s differs: %q / %q", n, a, answers["alice"])
		}
	}
	// one mail (alice's reset), and no GM request without a contact
	got := r.mails(3)
	if got[2].To != "alice@example.com" {
		t.Fatalf("the reset went to %s", mailer.Mask(got[2].To))
	}
	if n := r.count(`SELECT COUNT(*) FROM pwresets`); n != 0 {
		t.Fatalf("%d GM requests kept with no contact", n)
	}
	// with a contact, an account without a verified address still asks a GM
	if _, out, _ := r.post("/api/password/forgot", "203.0.113.9", true, map[string]any{"account": "carol", "contact": "carol#1 on Discord"}); out["message"] != answers["alice"] {
		t.Fatalf("carol with a contact: %v", out)
	}
	if n := r.count(`SELECT COUNT(*) FROM pwresets`); n != 1 {
		t.Fatalf("%d GM requests, want 1", n)
	}
	// alice's link sets a password, keeps the account's authenticator (not
	// touched), clears a temporary flag and ends her page sessions
	_, _, resp := r.post("/api/login", "203.0.113.20", false, map[string]any{"username": "alice", "password": "pw-alice"})
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session")
	}
	r.db.SetPasswordTemporary(r.id("alice"), true)
	tok := linkToken(t, got[2], "reset")
	if _, out, _ := r.post("/api/password/reset", "203.0.113.20", true, map[string]any{"token": tok, "password": "abc"}); out["ok"] != false {
		t.Fatalf("a short password: %v", out)
	}
	_, out, _ := r.post("/api/password/reset", "203.0.113.20", true, map[string]any{"token": tok, "password": "brand-new-pass"})
	if out["ok"] != true {
		t.Fatalf("reset: %v", out)
	}
	if temp, _ := r.db.PasswordTemporary(r.id("alice")); temp {
		t.Fatal("still temporary")
	}
	r.s.sessionsMu.RLock()
	left := r.s.sessions[session.Value]
	r.s.sessionsMu.RUnlock()
	if left != nil {
		t.Fatal("a page session outlived the reset")
	}
	if _, out, _ := r.post("/api/login", "203.0.113.20", false, map[string]any{"username": "alice", "password": "brand-new-pass"}); out["success"] != true {
		t.Fatalf("the new password: %v", out)
	}
	if _, out, _ := r.post("/api/password/reset", "203.0.113.20", true, map[string]any{"token": tok, "password": "another-one"}); out["ok"] != false {
		t.Fatalf("the link twice: %v", out)
	}
}

func TestRegisterRequiresAnEmailWithMailOn(t *testing.T) {
	r := newMailRig(t)
	if _, out, _ := r.post("/api/register", "203.0.113.30", false, map[string]any{"username": "newbie", "password": "pass1234"}); out["success"] != false {
		t.Fatalf("no address: %v", out)
	}
	_, out, _ := r.post("/api/register", "203.0.113.30", false, map[string]any{"username": "newbie", "password": "pass1234", "email": "newbie@example.com"})
	if out["success"] != true || !strings.Contains(out["message"].(string), "Check your e-mail") {
		t.Fatalf("register: %v", out)
	}
	if m := r.mails(1)[0]; m.To != "newbie@example.com" {
		t.Fatalf("mailed %s", mailer.Mask(m.To))
	}
	rec := httptest.NewRecorder()
	r.s.handleRegisterPage(rec, httptest.NewRequest("GET", "/register", nil))
	if !strings.Contains(rec.Body.String(), `<div class="form-group" id="emailGroup">`) {
		t.Fatal("the register page hides the address with e-mail on")
	}
}

// createPayload is STATION's 0x38 after its length and type, as vanilla's
// auth::create_account lays it out (message offsets less 8).
func createPayload(t *testing.T, name, password, email string) []byte {
	t.Helper()
	p := make([]byte, 476-8)
	copy(p[4:36], name)
	enc, err := encryptPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	copy(p[36:68], enc)
	copy(p[68:84], "x")
	copy(p[340:468], email)
	return p
}

func replyText(b []byte) string { return extractString(b[20:276]) }

func newMailAuth(t *testing.T) (*AuthServer, *mailRig) {
	t.Helper()
	r := newMailRig(t)
	cfg := config.DefaultConfig()
	cfg.Auth.BCryptCost = 4
	s := NewAuthServer(cfg, database.NewSQLiteAccountStore(r.db), auth.NewTicketStore(0))
	s.SetAccountMail(r.mail, r.db)
	return s, r
}

func TestStationCreatesTheAccountWithItsEmail(t *testing.T) {
	s, r := newMailAuth(t)
	ip := []byte{198, 51, 100, 40}
	// no address: refused, and said why
	resp := s.handleCreateAccountMail(createPayload(t, "frodo", "ringbearer", ""), ip)
	if binary.BigEndian.Uint32(resp[12:16]) != 0 || !strings.Contains(replyText(resp), "e-mail address is required") || len(resp) != 276 {
		t.Fatalf("no address: %q", replyText(resp))
	}
	if _, err := database.NewSQLiteAccountStore(r.db).GetByUsername("frodo"); err == nil {
		t.Fatal("an account made without an address")
	}
	resp = s.handleCreateAccountMail(createPayload(t, "frodo", "ringbearer", "frodo@example.com"), ip)
	if binary.BigEndian.Uint32(resp[12:16]) != 1 {
		t.Fatalf("create: %q", replyText(resp))
	}
	acc, err := database.NewSQLiteAccountStore(r.db).GetByUsername("frodo")
	if err != nil || auth.VerifyPassword(acc.PasswordHash, "ringbearer") != auth.Accepted {
		t.Fatalf("the account: %+v %v", acc, err)
	}
	if st, _ := r.db.AccountEmail(acc.ID); st.Email != "frodo@example.com" || st.Verified() {
		t.Fatalf("its e-mail: %+v", st)
	}
	if m := r.mails(1)[0]; m.To != "frodo@example.com" || !strings.Contains(m.Text, "#verify=") {
		t.Fatal("no verify link")
	}
	// a name taken: "exists", which STATION answers with its word 1371
	resp = s.handleCreateAccountMail(createPayload(t, "FRODO", "x1234", "f2@example.com"), ip)
	if binary.BigEndian.Uint32(resp[12:16]) != 0 || !strings.Contains(replyText(resp), "exists") {
		t.Fatalf("a taken name: %q", replyText(resp))
	}
	// the login that follows takes the password
	if _, ok, _ := s.checkPassword("frodo", "ringbearer", false); !ok {
		t.Fatal("the new account's login")
	}
	// three a day from an address
	for _, n := range []string{"sam", "merry"} {
		if resp := s.handleCreateAccountMail(createPayload(t, n, "pass1234", n+"@example.com"), ip); binary.BigEndian.Uint32(resp[12:16]) != 1 {
			t.Fatalf("%s: %q", n, replyText(resp))
		}
	}
	if resp := s.handleCreateAccountMail(createPayload(t, "pippin", "pass1234", "pippin@example.com"), ip); binary.BigEndian.Uint32(resp[12:16]) != 0 {
		t.Fatal("a fourth account from one address")
	}
}

func aesEncryptField(t *testing.T, s string, n int) []byte {
	t.Helper()
	key, err := sessionKey()
	if err != nil {
		t.Fatal(err)
	}
	c, _ := aes.NewCipher(key)
	b := make([]byte, n)
	copy(b, s)
	for i := 0; i < n; i += 16 {
		c.Encrypt(b[i:i+16], b[i:i+16])
	}
	return b
}

func TestStationUpdateAndForgotByEmail(t *testing.T) {
	s, r := newMailAuth(t)
	alice := r.id("alice")
	// 0x40 from a connection that logged in as another account: refused
	upd := make([]byte, 200)
	binary.BigEndian.PutUint32(upd[4:8], uint32(alice))
	copy(upd[72:200], "alice@example.com")
	if resp := s.handleAccountUpdate(upd, r.id("bob"), nil); len(resp) != 16 || binary.BigEndian.Uint32(resp[12:16]) != 0 {
		t.Fatalf("another account's update: %v", resp)
	}
	if resp := s.handleAccountUpdate(upd, alice, nil); binary.BigEndian.Uint32(resp[4:8]) != MsgAccountUpdateResponse || binary.BigEndian.Uint32(resp[12:16]) != 1 {
		t.Fatalf("the update: %v", resp)
	}
	r.mail.Verify(linkToken(t, r.mails(1)[0], "verify"))
	// 0x42, AES under the hello's key: the right address gets a link
	forgot := func(user, email string) []byte {
		p := make([]byte, 260)
		copy(p[4:36], aesEncryptField(t, user, 32))
		copy(p[36:68], aesEncryptField(t, "Jan", 32))
		copy(p[68:100], aesEncryptField(t, "1", 32))
		copy(p[100:132], aesEncryptField(t, "1977", 32))
		copy(p[132:260], aesEncryptField(t, email, 128))
		return s.handleForgotPassword(p, nil)
	}
	for _, c := range []struct{ user, email string }{{"alice", "other@example.com"}, {"nobody", "x@example.com"}, {"alice", "ALICE@example.com"}} {
		resp := forgot(c.user, c.email)
		if len(resp) != 16 || binary.BigEndian.Uint32(resp[4:8]) != MsgForgotPasswordReply || binary.BigEndian.Uint32(resp[12:16]) != 1 {
			t.Fatalf("%v: %v", c, resp)
		}
	}
	if m := r.mails(2)[1]; m.To != "alice@example.com" || !strings.Contains(m.Text, "#reset=") {
		t.Fatal("no reset link")
	}
	// an account with an authenticator changes its address on the website
	r.db.Exec(`CREATE TABLE IF NOT EXISTS gm_totp (accountid INTEGER PRIMARY KEY, secret TEXT, enabled INTEGER, last_step INTEGER)`)
	r.db.Exec(`INSERT INTO gm_totp (accountid, secret, enabled, last_step) VALUES (?, 'x', 1, 0)`, r.id("bob"))
	binary.BigEndian.PutUint32(upd[4:8], uint32(r.id("bob")))
	if resp := s.handleAccountUpdate(upd, r.id("bob"), nil); binary.BigEndian.Uint32(resp[12:16]) != 0 {
		t.Fatal("an update for an account with an authenticator")
	}
}

// loginAccepted reads the account a login response accepted.
func TestLoginAccepted(t *testing.T) {
	s := &AuthServer{}
	if loginAccepted(s.buildLoginFailResponse(7)) != 0 || loginAccepted(buildOneReply(0x41, 1)) != 0 {
		t.Fatal("a refusal read as accepted")
	}
	ok := make([]byte, 168)
	binary.BigEndian.PutUint32(ok[4:8], MsgLoginResponse)
	binary.BigEndian.PutUint32(ok[80:84], 42)
	binary.BigEndian.PutUint32(ok[84:88], 3)
	if loginAccepted(ok) != 42 {
		t.Fatal("an accepted login")
	}
}

// The pak served with e-mail on: STATION's account pages trimmed, every field
// its code reads still there, and no billing, postal or birth field shown.
func TestStationPakServesTheEmailPages(t *testing.T) {
	pak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "x\r\n"),
		pakEntry("stationdata/config/options.cfg", "x\r\n"),
		pakEntry("data/ui/IDCreate.xml", "<PS2GUIPage><Text name=\"FirstName\"/></PS2GUIPage>"),
		pakEntry("data/ui/IDCreat2.xml", "<PS2GUIPage><Text name=\"PostalCode\"/></PS2GUIPage>"),
		pakEntry("data/ui/AcctMgmt.xml", "<PS2GUIPage><Button action=\"UpdateBilling\"/></PS2GUIPage>"),
		pakEntry("data/ui/UpAcct.xml", "old"),
		pakEntry("data/ui/FrgtPass.xml", "old"),
		pakEntry("data/ui/login.xml", "login"),
	}, nil)
	cfg := config.DefaultConfig()
	cfg.Server.ServerIP = "203.0.113.10"
	off := (&PatchServer{cfg: cfg}).repointStationPak(pak)
	if !bytes.Contains(off, []byte(`action="UpdateBilling"`)) {
		t.Fatal("the pages changed with e-mail off")
	}
	cfg.Mail.Accounts = true
	got := (&PatchServer{cfg: cfg}).repointStationPak(pak)
	pages := map[string]string{}
	for off := 0; off+4 <= len(got); {
		n := int(binary.BigEndian.Uint32(got[off:]))
		name := string(got[off+4 : off+4+n])
		l := int(binary.BigEndian.Uint32(got[off+4+n:]))
		pages[name] = string(got[off+8+n : off+8+n+l])
		off += 8 + n + l
	}
	if pages["data/ui/login.xml"] != "login" {
		t.Fatal("login.xml changed")
	}
	// what each form's code reads (viwndgenericrenderer.cpp)
	need := map[string][]string{
		"data/ui/IDCreate.xml": {"StationName", "StationPassword", "StationPassword2", "FirstName", "MiddleName", "LastName", "ContinueAcctCreate"},
		"data/ui/IDCreat2.xml": {"Country", "PostalCode", "BirthDay", "BirthMonth", "BirthYear", "Gender", "EmailAddress", "SubmitAcctCreate"},
		"data/ui/UpAcct.xml":   {"Country", "PostalCode", "EmailAddress", "SubmitAcctUpdate"},
		"data/ui/FrgtPass.xml": {"Username", "BirthDay", "BirthMonth", "BirthYear", "EmailAddress", "SubmitForgotPassword"},
		"data/ui/AcctMgmt.xml": {"data/ui/PassChng.xml", "data/ui/UpAcct.xml"},
	}
	reShown := regexp.MustCompile(`<Text name="([A-Za-z0-9]+)" val="">\s*<ButtonPos`)
	reHidden := regexp.MustCompile(`<Text name="([A-Za-z0-9]+)" val="([^"]+)">\s*<Position x="0" y="0"/>`)
	for name, fields := range need {
		p := pages[name]
		for _, f := range fields {
			if !strings.Contains(p, `"`+f+`"`) {
				t.Errorf("%s lacks %s", name, f)
			}
		}
		for _, bad := range []string{"Billing", "Credit", "Cancel Account", "Postal Code", "Birth", "Gender", "Country", "1-858"} {
			if strings.Contains(p, `val="`+bad) || strings.Contains(p, "title=\""+bad) {
				t.Errorf("%s still shows %q", name, bad)
			}
		}
		// a hidden field comes before the backgrounds (which cover it) and
		// holds a value CheckName passes
		if h := reHidden.FindAllStringSubmatchIndex(p, -1); len(h) > 0 {
			bg := strings.Index(p, "stbk00.tga")
			for _, m := range h {
				if m[0] > bg {
					t.Errorf("%s: a hidden field after the background", name)
				}
				if v := p[m[4]:m[5]]; strings.Trim(v, "0123456789") == v && strings.ToLower(v) == strings.ToUpper(v) {
					t.Errorf("%s: a hidden value with no letter or digit: %q", name, v)
				}
			}
		}
		// the grid rows shown are 0..n-1
		rows := regexp.MustCompile(`<ButtonPos x="0" y="(\d+)"/>`).FindAllStringSubmatch(p, -1)
		hm := regexp.MustCompile(`gridHeight="(\d+)"`).FindStringSubmatch(p)
		if hm == nil || len(rows) == 0 || hm[1] != itoa(len(rows)) {
			t.Errorf("%s: gridHeight %v with %d rows", name, hm, len(rows))
		}
		for i, row := range rows {
			if row[1] != itoa(i) {
				t.Errorf("%s: row %d is %s", name, i, row[1])
			}
		}
		_ = reShown
	}
	if !strings.Contains(pages["data/ui/IDCreat2.xml"], "eqoaclassic.com/eqoa/password") {
		t.Error("IDCreat2 names no website")
	}
	if strings.Contains(pages["data/ui/AcctMgmt.xml"], "UpdateBilling") || strings.Contains(pages["data/ui/AcctMgmt.xml"], "CancelAccountLink") {
		t.Error("AcctMgmt still offers billing or cancelling")
	}
}

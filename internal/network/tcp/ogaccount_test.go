package tcp

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// ogOf reads accountinfo.og of the named account.
func ogOf(t *testing.T, db *database.SQLiteDB, name string) bool {
	t.Helper()
	acc, err := database.NewSQLiteAccountStore(db).GetByUsername(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	og, err := db.AccountOG(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	return og
}

// The page with auth.og_choice has the "Vanilla Test" row: Gender as a
// ComboBox (its value Standard, then OG), its label GenderLabel (CheckName's),
// on the grid's middle row with the e-mail above and the button below; every
// field the form's code reads is there, the hidden ones still passing its
// checks; its help fits. Without the choice the page is as before, Gender a
// hidden "x".
func TestStationCreatePageOffersOG(t *testing.T) {
	page := "eqoaclassic.com/eqoa/password"
	plain := string(stationCreate2(page, false))
	if !strings.Contains(plain, `<Text name="Gender" val="x">`) || strings.Contains(plain, "ComboBox") {
		t.Fatal("without the choice: not the plain page")
	}
	if !bytes.Equal(stationEmailPages(page, true)["data/ui/IDCreat2.xml"], []byte(plain)) {
		t.Fatal("the e-mail pages' IDCreat2 is not the plain page")
	}
	og := string(stationCreate2(page, true))
	for _, want := range []string{
		`<PS2GUIPage gridWidth="3" gridHeight="3">`,
		`<ComboBox name="Gender" val="Standard">`, `<Option name="Standard"/>`, `<Option name="OG"/>`,
		`<Text name="GenderLabel" val="Vanilla Test">`,
		`name="EmailAddress" val="">` + "\n\t\t<ButtonPos x=\"0\" y=\"0\"/>",
		`<ComboBox name="Gender" val="Standard">` + "\n\t\t<ButtonPos x=\"0\" y=\"1\"/>",
		`action="SubmitAcctCreate">` + "\n\t\t<ButtonPos x=\"0\" y=\"2\"/>",
	} {
		if !strings.Contains(og, want) {
			t.Errorf("the [OG] page lacks %q", want)
		}
	}
	for _, f := range []string{"Country", "PostalCode", "BirthDay", "BirthMonth", "BirthYear", "Gender", "EmailAddress"} {
		if !strings.Contains(og, `name="`+f+`"`) {
			t.Errorf("the [OG] page lacks %s", f)
		}
	}
	if strings.Count(og, `name="Gender"`) != 1 {
		t.Fatal("Gender twice")
	}
	if m := regexp.MustCompile(`<Text val="([^"]*)">\s*<Position x="450" y="118"/>`).FindStringSubmatch(og); m == nil || len(m[1]) > 150 {
		t.Fatalf("the help: %q", m)
	}
	reHidden := regexp.MustCompile(`<Text name="([A-Za-z0-9]+)" val="([^"]*)">\s*<Position x="0" y="0"/>`)
	for _, m := range reHidden.FindAllStringSubmatch(og, -1) {
		if m[1] == "PostalCode" && m[2] != stationZip {
			t.Errorf("PostalCode %q", m[2])
		}
	}
	// each value fits Gender's 16 bytes with its NUL and passes CheckName
	for _, v := range []string{StationOGStandard, StationOGChoice} {
		if len(v) > 15 || strings.Trim(strings.ToLower(v), "abcdefghijklmnopqrstuvwxyz0123456789") == v {
			t.Errorf("%q", v)
		}
	}

	// served: with auth.og_choice and mail.accounts
	pak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "x\r\n"),
		pakEntry("data/ui/IDCreat2.xml", "<PS2GUIPage><Text name=\"PostalCode\"/></PS2GUIPage>"),
	}, nil)
	served := func(choice bool) string {
		cfg := config.DefaultConfig()
		cfg.Server.ServerIP = "203.0.113.10"
		cfg.Mail.Accounts = true
		cfg.Auth.OGChoice = choice
		got := (&PatchServer{cfg: cfg}).repointStationPak(pak)
		for off := 0; off+4 <= len(got); {
			n := int(binary.BigEndian.Uint32(got[off:]))
			name := string(got[off+4 : off+4+n])
			l := int(binary.BigEndian.Uint32(got[off+4+n:]))
			if name == "data/ui/IDCreat2.xml" {
				return string(got[off+8+n : off+8+n+l])
			}
			off += 8 + n + l
		}
		return ""
	}
	if !strings.Contains(served(true), `<ComboBox name="Gender"`) {
		t.Fatal("og_choice: the row not served")
	}
	if strings.Contains(served(false), `ComboBox`) {
		t.Fatal("no og_choice: the row served")
	}
}

// STATION's Create Account with the row: Gender "OG" makes an [OG] account;
// Standard, the disc's Male or nothing a plain one; without auth.og_choice an
// "OG" there is a plain one too.
func TestStationCreatesAnOGAccount(t *testing.T) {
	s, r := newMailAuth(t)
	make := func(name, gender string) {
		t.Helper()
		p := createPayload(t, name, "pass1234", name+"@example.com")
		copy(p[324:340], gender)
		if resp := s.handleCreateAccountMail(p, []byte{198, 51, 100, byte(len(name))}); binary.BigEndian.Uint32(resp[12:16]) != 1 {
			t.Fatalf("%s: %q", name, replyText(resp))
		}
	}
	make("nochoice", "OG")
	s.cfg.Auth.OGChoice = true
	make("meshuggah", "OG")
	make("standard", "Standard")
	make("discpage", "Male")
	make("lowercase", "og")
	for name, want := range map[string]bool{"nochoice": false, "meshuggah": true, "standard": false, "discpage": false, "lowercase": true} {
		if got := ogOf(t, r.db, name); got != want {
			t.Errorf("%s: og %v, want %v", name, got, want)
		}
	}
}

// The account pages' Register: the checkbox shown only with auth.og_choice;
// an account asked [OG] is made one only there.
func TestRegisterOffersOG(t *testing.T) {
	r := newPwRig(t)
	page := func() string {
		rec := httptest.NewRecorder()
		r.s.handleRegisterPage(rec, httptest.NewRequest("GET", "/register", nil))
		return rec.Body.String()
	}
	if strings.Contains(page(), `<div class="form-group" id="ogGroup">`) {
		t.Fatal("no og_choice: the box shown")
	}
	if _, out, _ := r.post("/api/register", "203.0.113.31", false, map[string]any{"username": "early", "password": "pass1234", "og": true}); out["success"] != true {
		t.Fatalf("register: %v", out)
	}
	r.s.OGChoice = true
	if !strings.Contains(page(), `<div class="form-group" id="ogGroup">`) || !strings.Contains(page(), "Permanent") {
		t.Fatal("og_choice: the box not shown")
	}
	if _, out, _ := r.post("/api/register", "203.0.113.32", false, map[string]any{"username": "Meshuggah", "password": "pass1234", "og": true}); out["success"] != true {
		t.Fatalf("register: %v", out)
	}
	if _, out, _ := r.post("/api/register", "203.0.113.33", false, map[string]any{"username": "plain", "password": "pass1234"}); out["success"] != true {
		t.Fatalf("register: %v", out)
	}
	for name, want := range map[string]bool{"early": false, "Meshuggah": true, "plain": false, "alice": false} {
		if got := ogOf(t, r.db, name); got != want {
			t.Errorf("%s: og %v, want %v", name, got, want)
		}
	}
}

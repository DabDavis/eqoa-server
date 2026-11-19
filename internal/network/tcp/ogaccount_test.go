package tcp

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// [OG] is a character's since 2026-11-19 (database/ogchar.go): no account page
// offers it any more, whatever auth.og_choice says.

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

// The served station.pak's Create Account is the plain page with or without
// auth.og_choice (Gender a hidden "x", no "Account Type" row, no "What is OG?"
// button), and SOE's RentSet.xml goes out as it is. The control: the e-mail
// pages are served (the plain page replaces the disc's).
func TestStationCreatePageHasNoOGRow(t *testing.T) {
	page := "example.com/eqoa/password"
	plain := string(stationCreate2(page))
	if !strings.Contains(plain, `<Text name="Gender" val="x">`) || strings.Contains(plain, "ComboBox") ||
		strings.Contains(plain, "OG") || strings.Contains(plain, "RentSet") {
		t.Fatal("not the plain page")
	}
	pak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "x\r\n"),
		pakEntry("data/ui/IDCreat2.xml", "<PS2GUIPage><Text name=\"PostalCode\"/></PS2GUIPage>"),
		pakEntry("data/ui/RentSet.xml", "<PS2GUIPage><Text name=\"RentalNumber\"/></PS2GUIPage>"),
	}, nil)
	served := func(choice bool, name string) string {
		cfg := config.DefaultConfig()
		cfg.Server.ServerIP = "203.0.113.10"
		cfg.Mail.Accounts = true
		cfg.Mail.BaseURL = "https://example.com"
		cfg.Auth.OGChoice = choice
		got := (&PatchServer{cfg: cfg}).repointStationPak(pak)
		for off := 0; off+4 <= len(got); {
			n := int(binary.BigEndian.Uint32(got[off:]))
			nm := string(got[off+4 : off+4+n])
			l := int(binary.BigEndian.Uint32(got[off+4+n:]))
			if nm == name {
				return string(got[off+8+n : off+8+n+l])
			}
			off += 8 + n + l
		}
		return ""
	}
	for _, choice := range []bool{false, true} {
		if got := served(choice, "data/ui/IDCreat2.xml"); got != plain {
			t.Errorf("og_choice %v: Create Account served %d bytes, not the plain page", choice, len(got))
		}
		if !strings.Contains(served(choice, "data/ui/RentSet.xml"), "RentalNumber") {
			t.Errorf("og_choice %v: SOE's RentSet.xml replaced", choice)
		}
	}
}

// STATION's Create Account makes a plain account whatever the Gender field
// carries: an older card's page with the row sends "OG" there.
func TestStationCreatesNoOGAccount(t *testing.T) {
	s, r := newMailAuth(t)
	s.cfg.Auth.OGChoice = true
	for _, c := range []struct{ name, gender string }{{"meshuggah", "OG"}, {"standard", "Standard"}, {"discpage", "Male"}} {
		p := createPayload(t, c.name, "pass1234", c.name+"@example.com")
		copy(p[324:340], c.gender)
		if resp := s.handleCreateAccountMail(p, []byte{198, 51, 100, byte(len(c.name))}); binary.BigEndian.Uint32(resp[12:16]) != 1 {
			t.Fatalf("%s: %q", c.name, replyText(resp))
		}
		if ogOf(t, r.db, c.name) {
			t.Errorf("%s (Gender %q): made an [OG] account", c.name, c.gender)
		}
	}
}

// The account pages' Register shows no account type and makes a plain account
// even when asked "og": true (an old page's form).
func TestRegisterOffersNoOG(t *testing.T) {
	r := newPwRig(t)
	rec := httptest.NewRecorder()
	r.s.handleRegisterPage(rec, httptest.NewRequest("GET", "/register", nil))
	page := rec.Body.String()
	if !strings.Contains(page, "Create Account") {
		t.Fatal("the control: no Register page")
	}
	for _, gone := range []string{"ogGroup", "accttype", "ogAck", "[OG]", "Vanilla Test"} {
		if strings.Contains(page, gone) {
			t.Errorf("the page still has %q", gone)
		}
	}
	if _, out, _ := r.post("/api/register", "203.0.113.32", false, map[string]any{"username": "Meshuggah", "password": "pass1234", "og": true}); out["success"] != true {
		t.Fatalf("register: %v", out)
	}
	if ogOf(t, r.db, "Meshuggah") {
		t.Fatal("an [OG] account made")
	}
}

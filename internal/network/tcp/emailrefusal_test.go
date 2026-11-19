package tcp

import (
	"bytes"
	"encoding/binary"
	"regexp"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// An account without a verified e-mail past the cutoff is refused at the
// Station login with the reply STATION turns into its page (result 0 at wire
// 12, status 2), after its password and with no ticket; a wrong password is
// answered as before, and a verified account logs in.
func TestTheLoginRefusesAnAccountWithoutAVerifiedEmail(t *testing.T) {
	s, _ := newLoginTestServer(t, func(a *config.AuthConfig) { a.AllowAutoRegister = true })
	if !login(t, s, "nomail", "pw") || !login(t, s, "hasmail", "pw") {
		t.Fatal("the accounts were not made")
	}
	nomail, _ := s.tickets.Valid("nomail")
	s.tickets.Revoke("nomail")
	s.SetEmailRefusal(func(id int) bool { return id == nomail })

	r := s.handleLogin(buildLoginPayload(t, "nomail", "pw"))
	be := func(o int) uint32 { return binary.BigEndian.Uint32(r[o : o+4]) }
	if len(r) != 168 || be(4) != MsgLoginResponse || be(12) != 0 || be(84) != EmailRefusalStatus || be(8) != 1 ||
		int(be(80)) != nomail {
		t.Fatalf("the refusal: %x", r)
	}
	if loginAccepted(r) != 0 {
		t.Fatal("the refusal read as accepted")
	}
	if _, ok := s.tickets.Valid("nomail"); ok {
		t.Fatal("a refused login was given a ticket")
	}
	// the wrong password is told nothing of the e-mail
	if r := s.handleLogin(buildLoginPayload(t, "nomail", "wrong")); binary.BigEndian.Uint32(r[84:88]) != 4 {
		t.Fatalf("a wrong password answered with status %d", binary.BigEndian.Uint32(r[84:88]))
	}
	if !login(t, s, "hasmail", "pw") {
		t.Fatal("a verified account was refused")
	}
}

// With the requirement on, the pak's EnterKey.xml (status 2's page) is the
// reason, naming the website, with nothing to select; and the pak is padded
// past every card's copy, its records whole.
func TestStationPakServesTheEmailRefusalPage(t *testing.T) {
	pak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "x\r\n"),
		pakEntry("stationdata/config/options.cfg", "x\r\n"),
		pakEntry("data/ui/EnterKey.xml", "<PS2GUIPage>Account Key</PS2GUIPage>"),
		pakEntry("data/ui/login.xml", "login"),
	}, nil)
	read := func(got []byte) map[string]string {
		pages := map[string]string{}
		off := 0
		for off+4 <= len(got) {
			n := int(binary.BigEndian.Uint32(got[off:]))
			name := string(got[off+4 : off+4+n])
			l := int(binary.BigEndian.Uint32(got[off+4+n:]))
			pages[name] = string(got[off+8+n : off+8+n+l])
			off += 8 + n + l
		}
		if off != len(got) {
			t.Fatalf("the pak ends mid-record (%d of %d)", off, len(got))
		}
		return pages
	}
	cfg := config.DefaultConfig()
	cfg.Server.ServerIP = "203.0.113.10"
	cfg.Mail.Accounts = true
	cfg.Mail.BaseURL = "https://example.com"
	got := (&PatchServer{cfg: cfg}).repointStationPak(pak)
	if read(got)[EmailRefusalPage] != "<PS2GUIPage>Account Key</PS2GUIPage>" {
		t.Fatal("EnterKey.xml changed without the requirement")
	}
	if len(got) != stationPakFloor {
		t.Fatalf("the pak is %d bytes, want %d", len(got), stationPakFloor)
	}
	cfg.Mail.RequireVerified = true
	got = (&PatchServer{cfg: cfg}).repointStationPak(pak)
	p := read(got)[EmailRefusalPage]
	for _, want := range []string{"Your account needs a verified e-mail. Verify it at", `val="example.com/eqoa/password."`, "StationTri", "stbk00.tga"} {
		if !strings.Contains(p, want) {
			t.Errorf("the refusal page lacks %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "ButtonPos") || strings.Contains(p, "action=") || strings.Contains(p, "AccountKey") {
		t.Errorf("the refusal page has something to select:\n%s", p)
	}
	if len(got) != stationPakFloor {
		t.Fatalf("the pak is %d bytes, want %d", len(got), stationPakFloor)
	}
	// e-mail off: as it was, unpadded
	cfg.Mail.Accounts = false
	if got := (&PatchServer{cfg: cfg}).repointStationPak(pak); len(got) >= stationPakFloor || read(got)[EmailRefusalPage] == p {
		t.Fatal("the pak changed with e-mail off")
	}
}

// The help panel holds about ten short lines and breaks a web address
// mid-word (seen on a PS2): every page's help is short and names no address,
// which goes under the button on a line of its own.
func TestStationHelpPanelsFit(t *testing.T) {
	page := "eqoaclassic.com/eqoa/password"
	reHelp := regexp.MustCompile(`<Text val="([^"]*)">\s*<Position x="450" y="118"/>`)
	for name, p := range stationEmailPages(page, true) {
		m := reHelp.FindStringSubmatch(string(p))
		if m == nil {
			t.Errorf("%s: no help", name)
			continue
		}
		if len(m[1]) > 150 || strings.Contains(m[1], page) {
			t.Errorf("%s: the help is %d characters: %q", name, len(m[1]), m[1])
		}
		if name != "data/ui/IDCreate.xml" && !strings.Contains(string(p), `val="`+page) {
			t.Errorf("%s: the address is not on a line of its own", name)
		}
	}
}

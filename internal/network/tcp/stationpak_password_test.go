package tcp

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
)

// A page's password fields take the login's 31 characters (MaxChars 32, the
// editor counting the NUL), however the pak's Text is written; a field that
// is no password keeps its limit, and one that already takes more is left.
func TestRaisePasswordFields(t *testing.T) {
	page := `<PS2GUIPage gridWidth="1" gridHeight="4">
<Text val="Back"/>
<Text name="StationName" val=""><ButtonPos x="0" y="0"/><MaxChars size="16"/></Text>
<Text name="StationPassword" val=""><ButtonPos x="0" y="1"/><Password mode="true"/><MaxChars size="16"/></Text>
<Text name="StationPassword2" val=""><MaxChars size = "16" /><Password mode="TRUE" /></Text>
<Text name="Wide" val=""><Password mode="true"/><MaxChars size="64"/></Text>
<Text name="Shown" val=""><Password mode="false"/><MaxChars size="16"/></Text>
</PS2GUIPage>`
	out, n := raisePasswordFields([]byte(page))
	if n != 2 {
		t.Fatalf("raised %d fields, want 2:\n%s", n, out)
	}
	want := strings.NewReplacer(
		`<ButtonPos x="0" y="1"/><Password mode="true"/><MaxChars size="16"/>`,
		`<ButtonPos x="0" y="1"/><Password mode="true"/><MaxChars size="32"/>`,
		`<MaxChars size = "16" /><Password mode="TRUE" />`,
		`<MaxChars size="32"/><Password mode="TRUE" />`,
	).Replace(page)
	if string(out) != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
	if auth.MaxPasswordLen+1 != 32 {
		t.Fatalf("MaxPasswordLen %d: the login's field is 32 bytes with its NUL", auth.MaxPasswordLen)
	}
}

func pakEntry(name, body string) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(len(name)))
	b.WriteString(name)
	binary.Write(&b, binary.BigEndian, uint32(len(body)))
	b.WriteString(body)
	return b.Bytes()
}

// The pak served: its login page's password raised, the server's two entries
// repointed as before, every other entry byte for byte (a page outside
// data/ui/ is not a page: the control).
func TestStationPakServesLongPasswords(t *testing.T) {
	login := `<Text name="StationPassword" val=""><Password mode="true"/><MaxChars size="16"/></Text>`
	notPage := `<Text name="x"><Password mode="true"/><MaxChars size="16"/></Text>`
	pak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "5.161.217.188:9735\r\n"),
		pakEntry("stationdata/config/options.cfg", "NewsURL:http://x/news.html\r\n"),
		pakEntry("data/ui/login.xml", login),
		pakEntry("data/strings/notes.txt", notPage),
	}, nil)
	cfg := config.DefaultConfig()
	cfg.Server.ServerIP = "203.0.113.10"
	got := (&PatchServer{cfg: cfg}).repointStationPak(pak)
	wantPak := bytes.Join([][]byte{
		pakEntry("stationdata/servers/stationLiveHosts.txt", "203.0.113.10:"+itoa(cfg.Server.AuthPort)+"\r\n"),
		pakEntry("stationdata/config/options.cfg", "NewsURL:http://203.0.113.10:"+itoa(cfg.Server.PatchPort)+"/patch/web/eqoa/motd/news.html\r\n"),
		pakEntry("data/ui/login.xml", strings.Replace(login, `size="16"`, `size="32"`, 1)),
		pakEntry("data/strings/notes.txt", notPage),
	}, nil)
	if !bytes.Equal(got, wantPak) {
		t.Fatalf("pak served:\n%q\nwant\n%q", got, wantPak)
	}
}

// The account page takes a password the login can carry and refuses a longer
// one before anything is looked up.
func TestAccountPageRefusesAPasswordTheLoginCannotCarry(t *testing.T) {
	s := &AccountServer{}
	err := s.createAccount("192.0.2.1", "longpass", strings.Repeat("p", auth.MaxPasswordLen+1))
	if err == nil || !strings.Contains(err.Error(), "at most 31") {
		t.Fatalf("a 32-character password: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handleRegisterPage(rec, httptest.NewRequest("GET", "/register", nil))
	if !strings.Contains(rec.Body.String(), `id="password" name="password" required minlength="4" maxlength="31"`) {
		t.Fatal("the register form does not stop at 31 characters")
	}
}

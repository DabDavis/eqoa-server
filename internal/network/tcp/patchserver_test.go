package tcp

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// inScratchPatches runs fn with the working directory pointed at a throwaway
// copy of patches/. buildPatchFiles reads from ./patches and bumpProductVersion
// WRITES ./patches/.version, so running the real thing under test would move
// the deployed product version and desynchronise the two-pass Delete dance.
func inScratchPatches(t *testing.T) {
	t.Helper()
	src, err := filepath.Abs("../../../patches")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("no patches/ directory: %v", err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "patches")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	// patches/ itself (the variant pool that overrides name, plus the state
	// files) and patches/serve/ (the manifest). Both, because the whole point of
	// the split is that an override reads from one and the default from the
	// other, and a copy with only the first serves nothing at all.
	copyDir := func(from, to string) {
		t.Helper()
		if err := os.MkdirAll(to, 0o755); err != nil {
			t.Fatal(err)
		}
		ents, err := os.ReadDir(from)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(from, e.Name()))
			if err != nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(to, e.Name()), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	copyDir(src, dst)
	copyDir(filepath.Join(src, "serve"), filepath.Join(dst, "serve"))
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func newTestServer(t *testing.T) *PatchServer {
	t.Helper()
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Server.PatchPort = 7000
	return NewPatchServer(cfg)
}

func find(s *PatchServer, localName string) *patchFile {
	for i := range s.files {
		if s.files[i].LocalName == localName {
			return &s.files[i]
		}
	}
	return nil
}

// TestLoaderFilesAreServed pins the two files of the loader chain that THIS
// server delivers. A missing SS means S2 is written to the memory card and then
// never read — no error, no log line — so "the manifest was served" is not on
// its own evidence of anything.
//
// SD is the third link and is deliberately not checked here: EQOAHDD.ELF
// installs it and this server must not (elfDelivered). That the payload is
// SDP1-shaped is still checked, at the tool's build, by eqscene/tools/verify.py.
func TestLoaderFilesAreServed(t *testing.T) {
	s := newTestServer(t)

	ss := find(s, "SS")
	if ss == nil {
		t.Fatal("no SS entry: without the bootstrap, S2 is downloaded and ignored")
	}
	if len(ss.Content) == 0 {
		t.Fatal("SS is empty")
	}
	if find(s, "S2") == nil {
		t.Fatal("no S2 entry")
	}
	if sd := find(s, "SD"); sd != nil {
		t.Fatal("SD is served: EQOAHDD.ELF installs it, and two writers for one " +
			"filename is a card that keeps whichever landed first")
	}
	// The fixture writes deliberate junk to exercise the verifier; it must
	// never reach a console.
	if sd := find(s, "SD-test"); sd != nil {
		t.Fatal("the verifier fixture SD-test is in the served set")
	}
}

// TestSSCarriesTheDualStageStub checks the served SS actually installs our
// loader rather than Sandstorm's: same hook word, and the stub body must
// contain both stage names. Serving the wrong bootstrap looks identical from
// the manifest.
func TestSSCarriesTheDualStageStub(t *testing.T) {
	s := newTestServer(t)
	ss := find(s, "SS")
	if ss == nil {
		t.Fatal("no SS entry")
	}
	if !strings.Contains(string(ss.Content), "SD") ||
		!strings.Contains(string(ss.Content), "S2") {
		t.Error("served SS does not name both stages; is it Sandstorm's bootstrap?")
	}
	// `jal 0x00157400` = 0x0C055D00, little-endian 00 5d 05 0c.
	if !strings.Contains(string(ss.Content), "\x00\x5d\x05\x0c") {
		t.Error("served SS does not contain the hook jal to 0x00157400")
	}
}

// TestRemoteNamesTrackContent guards the trap that already cost a debugging
// round: the client skips a remote name it has already fetched, so two
// different payloads must never share one.
func TestRemoteNamesTrackContent(t *testing.T) {
	s := newTestServer(t)
	seen := map[string]string{}
	for _, f := range s.files {
		if f.Delete || f.RemoteName == "" {
			continue
		}
		if prev, ok := seen[f.RemoteName]; ok {
			t.Errorf("remote name %q serves both %s and %s", f.RemoteName, prev, f.LocalName)
		}
		seen[f.RemoteName] = f.LocalName
	}
}

// TestDeletePassIsADistinctVersion pins that pass 1 and pass 2 of the dance
// carry different Product Versions. They were thought to have to, or the
// patcher would gate pass 2 out; the decomp shows the patcher never reads the
// number (bumpProductVersion), so this now only keeps each served set
// distinct in the logs.
func TestDeletePassIsADistinctVersion(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"

	pass2 := NewPatchServer(cfg) // no delete set

	t.Setenv("EQOA_PATCH_DELETE", "SS,SD")
	pass1 := NewPatchServer(cfg)

	if pass1.productVersion == pass2.productVersion {
		t.Fatalf("both passes share Product Version %d; pass 2 would download nothing",
			pass1.productVersion)
	}
	if len(pass1.refresh) != 2 {
		t.Fatalf("EQOA_PATCH_DELETE not honoured: %v", pass1.refresh)
	}
}

// TestManifestListsLoaderFiles renders the real XML and checks the loader files
// appear in it, since that is the only thing the console ever sees.
func TestManifestListsLoaderFiles(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/patch/eqoa/eqoa-frontierslive-update.xml.gz", nil)
	s.serveUpdateXML(rec, req)

	if rec.Code != 200 {
		t.Fatalf("manifest returned %d", rec.Code)
	}
	// The path ends in .xml.gz and the server really does gzip it.
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("manifest is not gzipped: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// No world ESF is listed. Worlds reach the console over the HDD partition
	// now, which the patch server does not touch; the memory-card route that
	// needed guk.esf here was retired with the "mc:" prefix it depended on.
	//
	// The driver stack and SD are NOT here, and must not be: EQOAHDD.ELF is
	// their only delivery (elfDelivered). This list held all six until then, and
	// the pairing is checked both ways so a file cannot quietly rejoin the
	// manifest and be delivered twice.
	for _, want := range []string{`LocalName="SS"`, `LocalName="S2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest is missing %s", want)
		}
	}
	for name := range elfDelivered {
		if strings.Contains(body, `LocalName="`+name+`"`) {
			t.Errorf("manifest offers %s, which EQOAHDD.ELF also installs", name)
		}
	}
	// Echo the loader lines under -v: the exact <File> entries are what a
	// deploy has to be checked against, and reading them off the console is
	// otherwise a packet capture.
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, `LocalName="S`) || strings.Contains(line, "guk.esf") ||
			strings.Contains(line, "Product Version") {
			t.Log(strings.TrimSpace(line))
		}
	}
}

// TestManifestUsesCRC pins UseCRC="True" on the distribution. Without it the
// PS2's patcher compares sizes alone (FileDownloader::FileDiscrepancy), and a
// change that keeps a file's length never reaches the card.
func TestManifestUsesCRC(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.serveUpdateXML(rec, httptest.NewRequest("GET", "/patch/eqoa/eqoa-frontierslive-update.xml", nil))
	if !regexp.MustCompile(`<Distribution Name="[^"]*" UseCRC="True"`).MatchString(rec.Body.String()) {
		t.Fatalf("no UseCRC on the distribution: %.200s", rec.Body.String())
	}
}

// The served set no longer carries a world ESF. TestGukWorldIsServable used to
// live here: it gated guk.esf on the two header fields VIESFParse::ParseWorld
// hard-rejects before it parses a record — FileType != 0xAB4F and NumObjects
// != 1 — plus the ~2.8 MB pad.dat reservation on the memory card. All three
// checks were about a world DELIVERED BY THE PATCHER to the card, and that
// route is retired: worlds are streamed off the PS2 hard disk through the SD's
// "hdd:" prefix, onto a partition the patch server never sees.
//
// The gate itself is still worth having — three separate writers once put a
// record count where the format wants the constant 0xAB4F, and the editor read
// its own broken files back happily (claude-notes/memcard-world-loading.md).
// Restore it, retargeted, the day a world is served here again; a header check
// against a file nothing serves would only assert that patches/ still holds a
// copy.

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// Every file the SD payload opens off the memory card has to be one that
// SOMETHING puts there. Nothing enforced that until 2026-08-11, when EQPREP.IRX
// turned out to be staged into patches/ by open-irx/build.sh and NOT served —
// and the gap was invisible on every console we had, because each of them had
// been staged by hand months earlier and already held the file. A fresh card
// would have got the three driver modules, no EQPREP, and a technician answering
// "the disk tool would not load (-201)".
//
// THERE ARE TWO DELIVERIES NOW, and this asks about their union. The patch
// server sends SS and S2; EQOAHDD.ELF installs the driver stack, the setup scene
// and SD. Checking either one alone would reproduce the 2026-08-11 gap exactly —
// a file present in the other half, absent from the one being checked, and no
// console able to tell you which. Each file must be delivered by exactly one of
// them: none is the old bug, both is the two-writers bug elfDelivered exists to
// prevent.
//
// The list is read out of the payload's own strings rather than repeated here,
// so adding a module to the payload fails this test instead of shipping half a
// feature. Paths inside `#ifdef OPEN_STACK` are skipped: that is the
// iomanX/fileXio variant, which `--modules mc0` does not build and we do not
// ship.
func TestPayloadFilesAreServed(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"re-tools", "hdd_sd", "strings.c"))
	if err != nil {
		t.Skipf("payload source unreadable (%v)", err)
	}

	pathRe := regexp.MustCompile(`mc0:/BASLUS-20744EQONLINE/([A-Za-z0-9._]+)`)
	var guards []string
	wanted := map[string]bool{}
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#ifdef"), strings.HasPrefix(trimmed, "#ifndef"):
			f := strings.Fields(trimmed)
			if len(f) > 1 {
				guards = append(guards, f[1])
			}
		case strings.HasPrefix(trimmed, "#endif") && len(guards) > 0:
			guards = guards[:len(guards)-1]
		}
		m := pathRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		open := false
		for _, g := range guards {
			if g == "OPEN_STACK" {
				open = true
			}
		}
		if !open {
			wanted[m[1]] = true
		}
	}
	if len(wanted) == 0 {
		t.Fatal("no mc0: paths found in hdd_sd/strings.c — the scan is not working")
	}

	// The tool's manifest, read before newTestServer chdirs into the scratch
	// copy of patches/. EQSCENE.BIN is not in that directory: it is produced by
	// eqscene/build.sh and embedded straight from out/, so it is credited from
	// elfDelivered rather than from a file on disk.
	byTool := map[string]bool{}
	for name := range elfDelivered {
		byTool[name] = true
	}
	for name := range byTool {
		if name == "EQSCENE.BIN" {
			continue
		}
		if _, err := os.Stat(filepath.Join(cardDir, name)); err != nil {
			t.Errorf("%s is EQOAHDD.ELF's to deliver but is not in %s (%v) — the "+
				"tool cannot be built, so nothing puts it on a card at all",
				name, cardDir, err)
		}
	}

	s := newTestServer(t)
	byServer, byBoth, byNeither := 0, 0, 0
	for name := range wanted {
		f := find(s, name)
		switch {
		case f != nil && byTool[name]:
			byBoth++
			t.Errorf("%s is served AND installed by EQOAHDD.ELF. The patcher "+
				"compares by SIZE, so if the two copies ever differ but match in "+
				"length the card keeps whichever landed first and neither end "+
				"reports it", name)
		case f != nil:
			byServer++
			if len(f.Content) == 0 {
				t.Errorf("%s is served but empty", name)
			}
		case byTool[name]:
			// EQOAHDD.ELF's, and its presence in eqscene/card/ is checked above.
		default:
			byNeither++
			t.Errorf("the payload opens mc0:/BASLUS-20744EQONLINE/%s but NOTHING "+
				"delivers it — the patch server does not serve it and "+
				"EQOAHDD.ELF does not carry it, so a fresh card will not have it",
				name)
		}
	}
	t.Logf("checked %d payload files: %d from the patch server, %d from "+
		"EQOAHDD.ELF, %d from both, %d from neither",
		len(wanted), byServer, len(wanted)-byServer-byNeither-byBoth, byBoth, byNeither)
}

// TestNewsIsPlainText: STATION shows the news a byte a character
// (VIWndGenericRenderer's news poll, then VIGUIDialog::SetText), so a tag in
// it would be on the player's screen as written.
func TestNewsIsPlainText(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleRequest(w, httptest.NewRequest("GET", "/patch/web/eqoa/motd/news.html", nil))
	body := w.Body.String()
	if w.Code != 200 || body == "" {
		t.Fatalf("news: %d %q", w.Code, body)
	}
	if strings.ContainsAny(body, "<>") {
		t.Fatalf("the news carries markup STATION would show as written: %q", body)
	}
}

package tcp

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// manifestFor renders the manifest as one console at `host` would receive it.
// The port varies per HTTP connection and must not affect which pass is served,
// so it is deliberately different on every call.
var manifestPort = 40000

func manifestFor(t *testing.T, s *PatchServer, host string) string {
	t.Helper()
	manifestPort++
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/patch/eqoa/eqoa-frontierslive-update.xml", nil)
	req.RemoteAddr = host + ":" + strconv.Itoa(manifestPort)
	s.serveUpdateXML(rec, req)
	if rec.Code != 200 {
		t.Fatalf("manifest returned %d", rec.Code)
	}
	return rec.Body.String()
}

// diagManifestFor is the same fetch EQOADIAG makes: the patcher's URL, from the
// console's own address, marked as a request that only reads the answer.
func diagManifestFor(t *testing.T, s *PatchServer, host string) string {
	t.Helper()
	manifestPort++
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/patch/eqoa/eqoa-frontierslive-update.xml", nil)
	req.Header.Set("X-EQOA-Diag", "1")
	req.RemoteAddr = host + ":" + strconv.Itoa(manifestPort)
	s.serveUpdateXML(rec, req)
	if rec.Code != 200 {
		t.Fatalf("manifest returned %d", rec.Code)
	}
	return rec.Body.String()
}

func productVersionOf(t *testing.T, xml string) int {
	t.Helper()
	m := regexp.MustCompile(`<Product Version="(\d+)">`).FindStringSubmatch(xml)
	if m == nil {
		t.Fatalf("no Product Version in manifest: %s", xml)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func deletesFile(xml, name string) bool {
	return strings.Contains(xml, `<File LocalName="`+name+`" Delete="True">`)
}

func downloadsFile(xml, name string) bool {
	return regexp.MustCompile(`<File LocalName="` + regexp.QuoteMeta(name) +
		`" RemotePath=`).MatchString(xml)
}

// managedFile follows the production list rather than naming a file. These
// tests are about the SEQUENCER; which file it happens to walk is
// refreshOrder's business, and hardcoding it here is how they came to fail
// wholesale when the managed file changed.
var managedFile = refreshOrder[0]

func deletesManaged(xml string) bool   { return deletesFile(xml, managedFile) }
func downloadsManaged(xml string) bool { return downloadsFile(xml, managedFile) }

// managedDeletes counts how many MANAGED files one manifest asks to delete. The
// always-present words.txt entry and anything EQOA_PATCH_DELETE added are not
// managed and do not count -- the client's one-delete rule is what this is
// about, and it is the sequencer that has to respect it.
func managedDeletes(xml string) []string {
	var out []string
	for _, name := range refreshOrder {
		if deletesFile(xml, name) {
			out = append(out, name)
		}
	}
	return out
}

// walkToDelete fetches manifests as `host` until the one that deletes `name`,
// and returns it. A test about the SD should not also be a test about which
// file the sequence happens to walk first.
func walkToDelete(t *testing.T, s *PatchServer, host, name string) string {
	t.Helper()
	for i := 0; i < len(refreshOrder)+2; i++ {
		x := manifestFor(t, s, host)
		if deletesFile(x, name) {
			return x
		}
	}
	t.Fatalf("%s was never offered a Delete pass for %s", host, name)
	return ""
}

// TestSDRefreshDancesEachConsoleIndependently is the whole point of the file.
//
// The hand-driven dance sequenced the two passes in TIME, which only ever
// worked for consoles that happened to be booting between them: the patcher
// gates on Product Version and never replays one it skipped, so a tester whose
// first contact came after pass 2 kept a stale SD forever, silently. Here the
// two passes are sequenced per console, so a console arriving at any moment
// still gets Delete-then-add.
func TestSDRefreshDancesEachConsoleIndependently(t *testing.T) {
	s := newTestServer(t)
	managed := s.cardSeq.order
	if len(managed) == 0 {
		t.Skip("nothing managed in this patches/")
	}

	// ONE FILE PER PASS, IN ORDER, AND THE FILE BEING DELETED IS NOT ALSO
	// OFFERED. The patcher honours the Delete and ignores the add, so carrying
	// both would remove the file and never restore it.
	for i, name := range managed {
		x := manifestFor(t, s, "10.0.0.1")
		if got := managedDeletes(x); len(got) != 1 || got[0] != name {
			t.Fatalf("pass %d deleted %v, want exactly [%s] — the client honours "+
				"ONE delete per manifest", i+1, got, name)
		}
		if downloadsFile(x, name) {
			t.Errorf("pass %d carries a Delete AND a download for %s", i+1, name)
		}
	}

	// The last pass deletes nothing and delivers everything.
	last := manifestFor(t, s, "10.0.0.1")
	if got := managedDeletes(last); len(got) != 0 {
		t.Errorf("the final pass still deletes %v; the console never receives them", got)
	}
	for _, name := range managed {
		if !downloadsFile(last, name) {
			t.Errorf("the final pass does not carry %s", name)
		}
	}

	// A console that turns up only now — after the operator would long since
	// have finished a manual dance — still gets the whole sequence.
	late := manifestFor(t, s, "10.0.0.2")
	if len(managedDeletes(late)) != 1 {
		t.Error("a console whose first contact is late still holds stale files and " +
			"must be sequenced; this is the case the manual two-pass could not reach")
	}
}

// TestEqahostsIsNeverDeleted is the test that would have stopped a console
// being stranded.
//
// eqahosts.txt WAS sequenced like the payload, for a good reason: it is the
// server's address and it is fifteen bytes for almost every operator, so a card
// patched by a different server keeps pointing there while this operator's log
// looks healthy. The cure was worse. Measured on the console 2026-08-14: the
// pass that deletes this file leaves the game stuck FOREVER on "Checking memory
// card (PS2)... do not reset" — the payload never runs, and the only way out is
// the power cycle that screen forbids.
//
// It is delivered by SIZE now (eqahostsFile pads it), so no Delete is needed and
// none may be offered. See TestEqahostsSizeCarriesItsAddress.
func TestEqahostsIsNeverDeleted(t *testing.T) {
	s := newTestServer(t)
	const host = "10.0.0.20"

	// More fetches than the sequence can possibly need, so a Delete anywhere in
	// the walk is caught rather than just one on the first pass.
	for i := 0; i < len(refreshOrder)+3; i++ {
		if x := manifestFor(t, s, host); deletesFile(x, "eqahosts.txt") {
			t.Fatalf("fetch %d offered a Delete for eqahosts.txt; a console that "+
				"honours it hangs on the memory-card check until it is power-cycled",
				i+1)
		}
	}
	if slices.Contains(refreshOrder, "eqahosts.txt") {
		t.Error("eqahosts.txt is back in refreshOrder")
	}
}

// TestEqahostsSizeCarriesItsAddress is what replaced the Delete.
//
// The client does not see a same-size content change — measured twice on the
// console, with the wrong address left in place both times — so two addresses
// that are equally long must not produce equally long files. These two are the
// pair this project actually runs: Sandstorm and the dev LAN, both thirteen
// characters.
func TestEqahostsSizeCarriesItsAddress(t *testing.T) {
	sand := eqahostsFile("5.161.217.188", hostsState{})
	dev := eqahostsFile("192.168.40.87", hostsState{})
	if len(sand) == len(dev) {
		t.Errorf("both addresses produce %d bytes, so a card patched by one and "+
			"then the other keeps the first — which is the bug this padding is for",
			len(sand))
	}
	// The address still has to be the first thing in the file: _dl_addr parses
	// the leading dotted quad and the game reads the first line.
	if got := string(dev[:len("192.168.40.87\n\n")]); got != "192.168.40.87\n\n" {
		t.Errorf("the file starts %q, not with the address", got)
	}
	// STABLE. A rebuild, a restart, or a config that says the same thing must
	// produce the same bytes, or every console re-downloads it every boot.
	if !bytes.Equal(dev, eqahostsFile("192.168.40.87", hostsState{})) {
		t.Error("the same address produced two different files")
	}
	if !bytes.Equal(dev, eqahostsFile("192.168.40.87", hostsState{IP: "192.168.40.87", Size: len(dev)})) {
		t.Error("re-serving the address we last served changed the file")
	}
}

// TestEqahostsBumpsOffACollidingSize closes the hole the derived pad leaves.
// Two addresses land on one size about once in eqahostsPadMod, and there is no
// Delete to fall back on any more: a file the client cannot see is a file that
// never arrives.
func TestEqahostsBumpsOffACollidingSize(t *testing.T) {
	const ip = "192.168.40.87"
	plain := eqahostsFile(ip, hostsState{})
	// Whatever the derived pad is, claim the previous server served exactly that
	// many bytes for a DIFFERENT address.
	bumped := eqahostsFile(ip, hostsState{IP: "203.0.113.55", Size: len(plain)})
	if len(bumped) == len(plain) {
		t.Errorf("the file is still %d bytes, the same length as the one it "+
			"replaces, so the client will not fetch it", len(bumped))
	}
	if !strings.HasPrefix(string(bumped), ip+"\n\n") {
		t.Error("the bump disturbed the address")
	}
}

// TestOnlyOneManagedDeletePerManifest pins the client's rule. Two deletes in one
// manifest and only one is honoured, so the other file is quietly never
// replaced — which looks exactly like a console that was already up to date.
func TestOnlyOneManagedDeletePerManifest(t *testing.T) {
	s := newTestServer(t)
	const host = "10.0.0.21"
	for i := 0; i < len(refreshOrder)+2; i++ {
		if got := managedDeletes(manifestFor(t, s, host)); len(got) > 1 {
			t.Fatalf("fetch %d asked to delete %v in one manifest", i+1, got)
		}
	}
}

// TestRefreshCanBeStoodDownByEnv is the operator's way out without a rebuild.
// Deleting eqahosts.txt is safe because the patcher reaches this server from the
// address baked into SLUS rather than from that file — but that is a claim about
// a console, and a claim about a console needs an off switch.
func TestRefreshCanBeStoodDownByEnv(t *testing.T) {
	inScratchPatches(t)
	t.Setenv("EQOA_REFRESH_FILES", "none")
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	s := NewPatchServer(cfg)

	if x := manifestFor(t, s, "10.0.0.22"); len(managedDeletes(x)) != 0 {
		t.Errorf("EQOA_REFRESH_FILES=none still sequenced %v", managedDeletes(x))
	}
}

// TestRefreshManagesOnlyServedFiles is the footgun that would be worst of all.
// A Delete for a file this server does not serve takes it off the card and puts
// nothing back, because there is no download entry to follow.
func TestRefreshManagesOnlyServedFiles(t *testing.T) {
	inScratchPatches(t)
	t.Setenv("EQOA_REFRESH_FILES", "eqahosts.txt,NOSUCHFILE.dat")
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	s := NewPatchServer(cfg)

	for _, name := range s.cardSeq.order {
		if find(s, name) == nil {
			t.Errorf("%s is sequenced but not served; a Delete for it would strip "+
				"the card and restore nothing", name)
		}
	}
	for i := 0; i < 4; i++ {
		if x := manifestFor(t, s, "10.0.0.23"); deletesFile(x, "NOSUCHFILE.dat") {
			t.Fatal("a file this server does not serve was deleted from the card")
		}
	}
}

// TestSDRefreshPassesAreDistinctVersions pins the rule the manual dance already
// depended on: the patcher gates the whole distribution on Product Version, so
// two passes sharing one number means pass 2 downloads nothing and the console
// is left with SD deleted and never restored — strictly worse than not dancing.
func TestSDRefreshPassesAreDistinctVersions(t *testing.T) {
	s := newTestServer(t)

	var seen []int
	for i := 0; i <= len(s.cardSeq.order); i++ {
		seen = append(seen, productVersionOf(t, manifestFor(t, s, "10.0.0.3")))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] != seen[i-1]+1 {
			t.Fatalf("pass versions %v are not consecutive; a repeated number is "+
				"gated out and the console is left with the file deleted and "+
				"never restored", seen)
		}
	}
}

// TestVersionsAreAllocatedTwoAtATime is the other half of that rule, across a
// content change rather than within one. Versions are handed out in pairs, so
// the next change's pass 1 cannot reuse a number some console already applied
// as a pass 2 — which the patcher would gate out, downloading nothing and
// leaving no trace but a manifest GET.
func TestVersionsAreAllocatedAWholeSequenceAtATime(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"

	before := NewPatchServer(cfg)
	// The last pass of the current sequence: the highest number any console can
	// already have applied.
	lastPass := before.productVersion + len(before.cardSeq.order)

	// Change the served content the same way a real operator does — a different
	// server_ip rewrites eqahosts.txt and station.pak without touching patches/.
	cfg.Server.ServerIP = "10.9.8.7"
	after := NewPatchServer(cfg)

	if after.productVersion <= lastPass {
		t.Fatalf("the new sequence starts at version %d but a console may already "+
			"have applied %d; the distribution would be silently skipped",
			after.productVersion, lastPass)
	}
}

// TestEngagingTheDanceBumpsTheVersion covers the way this mechanism fails
// silently rather than loudly.
//
// The patcher gates the whole distribution on Product Version. If the dance
// engages under a number the field has already applied — which is exactly what
// happens on the first start after this code is deployed, when the state file
// does not exist yet but every console is holding an unchecked SD — then no
// console fetches either pass, and the server log looks identical to success.
func TestEngagingTheDanceBumpsTheVersion(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	settled := NewPatchServer(cfg)
	published := settled.productVersion
	walkToDelete(t, settled, "10.0.0.7", managedFile)
	manifestFor(t, settled, "10.0.0.7") // settle that console on the last pass

	// Now the SD every console holds becomes wrong, in the way that is hardest
	// to deliver: a rebuilt payload of the SAME LENGTH. The patcher compares by
	// size, so nothing but the dance moves this one.
	flipAByteOfTheManaged(t)
	changed := NewPatchServer(cfg)

	if changed.productVersion <= published {
		t.Fatalf("SD changed but the dance engaged at version %d, which the field "+
			"has already applied (%d); no console would fetch either pass",
			changed.productVersion, published)
	}
	if x := manifestFor(t, changed, "10.0.0.7"); !deletesManaged(x) {
		t.Error("a console that already danced for the OLD SD must dance again for " +
			"the new one; otherwise it keeps the payload it already had")
	}
}

// flipAByteOfTheSD rewrites one byte of the SD in the scratch patches/, leaving
// its LENGTH alone. That is the change the patcher cannot see: it re-downloads
// on size, so a same-size rebuild reaches no console without the dance. Every
// SD rebuild is a candidate — the world table is a fixed-size record.
// flipAByteOfTheManaged changes the served content WITHOUT changing its length,
// which is the only case the patcher cannot see for itself and the whole reason
// the dance exists.
//
// It writes into serve/, the manifest, rather than resolving an override name
// out of the pool. The override form was `filepath.Join("patches",
// resolvePayload("", "", defaultSDFile))`, and defaultSDFile is "" — so it built
// the path "patches", read a DIRECTORY, and took the error as "no payload here"
// and skipped. Every test that called it had been passing without running.
func flipAByteOfTheManaged(t *testing.T) {
	t.Helper()
	path := filepath.Join("patches", "serve", managedFile)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s is not served: %v", managedFile, err)
	}
	// Well past any header, so the file still walks as whatever it is and only
	// its CRC has moved.
	if len(b) < 0x100 {
		t.Skipf("%s is %d bytes, too short to flip inside", managedFile, len(b))
	}
	b[len(b)/2] ^= 0xFF
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSDRefreshTriggersOnAChangedPayload is the bug this was built for.
//
// A rebuilt SD is very often the SAME LENGTH as the one it replaces — the world
// table is a fixed-size record — and the patcher re-downloads on size. So a
// console holding the old payload keeps it, with nothing in either log to say
// so. A changed payload must re-dance every console, including consoles this
// server has never heard from.
func TestSDRefreshTriggersOnAChangedPayload(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	s1 := NewPatchServer(cfg)
	if sd := find(s1, managedFile); sd == nil {
		t.Skip("the managed file is not served")
	}
	before := len(find(s1, managedFile).Content)
	walkToDelete(t, s1, "10.0.0.4", managedFile)
	if x := manifestFor(t, s1, "10.0.0.4"); !downloadsManaged(x) {
		t.Fatal("console did not reach the delivering pass under the original payload")
	}

	flipAByteOfTheManaged(t)
	s2 := NewPatchServer(cfg)
	if after := len(find(s2, managedFile).Content); after != before {
		t.Fatalf("the test changed the SD's length (%d -> %d); it is then a case "+
			"the patcher can see on its own and proves nothing", before, after)
	}
	if x := walkToDelete(t, s2, "10.0.0.4", managedFile); !deletesManaged(x) {
		t.Error("the SD changed under an unchanged length, so every console holds " +
			"the old one — it must be deleted and re-fetched, not left in place")
	}
}

// TestSDRefreshIgnoresAServerIPChange is the inverse, and it is the change
// that retired repointSD.
//
// The SD used to carry the world-download address as DATA, rewritten in place
// as the file went out, so setting server_ip changed the served payload without
// touching patches/ — and every console had to be danced to pick it up. The
// payload now reads the address off the card, out of eqahosts.txt, so a
// server_ip change does not alter one byte of it. Dancing anyway would cost
// every tester a boot for nothing, and worse, would train the operator to
// expect a dance where the real content change may not produce one.
//
// (eqahosts.txt itself is delivered as it always was. A console that does not
// pick up a new one is a console that cannot reach the server at all, which is
// a whole-game failure the player reports in the first minute — not a silent
// one, and not this mechanism's to hide.)
func TestSDRefreshIgnoresAServerIPChange(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	const host = "10.0.0.4"
	s1 := NewPatchServer(cfg)
	if sd := find(s1, managedFile); sd == nil {
		t.Skip("the managed file is not served")
	}
	for i := 0; i <= len(s1.cardSeq.order); i++ {
		manifestFor(t, s1, host) // walk the console all the way through
	}
	served := find(s1, managedFile).CRC

	cfg.Server.ServerIP = "203.0.113.55"
	s2 := NewPatchServer(cfg)
	if got := find(s2, managedFile).CRC; got != served {
		t.Fatalf("server_ip changed the served SD (%08x -> %08x); the payload is "+
			"supposed to carry no address at all", served, got)
	}

	// eqahosts.txt DID change, and it now reaches the card the way the client
	// already works: a different SIZE. No Delete, no pass, no boot spent.
	if before, after := find(s1, "eqahosts.txt"), find(s2, "eqahosts.txt"); len(before.Content) == len(after.Content) {
		t.Errorf("server_ip changed but eqahosts.txt is still %d bytes; the "+
			"client does not see a same-size change, so the card keeps the old "+
			"address", len(after.Content))
	}
	for i := 0; i <= len(s2.cardSeq.order); i++ {
		x := manifestFor(t, s2, host)
		if deletesManaged(x) {
			t.Error("a console was danced for the SD after a server_ip change, " +
				"which no longer alters it; that is a wasted boot for every tester")
		}
		if deletesFile(x, "eqahosts.txt") {
			t.Error("a console was told to delete eqahosts.txt; that hangs it on " +
				"the memory-card check until it is power-cycled")
		}
	}
}

// TestSDRefreshLeavesOtherFilesAlone keeps the cost of this honest. Pass 1
// carries the whole set apart from SD, so a patch that does not touch SD costs
// nobody an extra boot: everything else lands immediately and pass 2 adds only
// the SD.
func TestSDRefreshLeavesOtherFilesAlone(t *testing.T) {
	s := newTestServer(t)
	pass1 := manifestFor(t, s, "10.0.0.5")

	for _, name := range []string{"SS", "S2", "station.pak"} {
		if find(s, name) == nil {
			continue
		}
		if !strings.Contains(pass1, `<File LocalName="`+name+`"`) {
			t.Errorf("%s is missing from pass 1; unrelated files must not wait for "+
				"the SD dance", name)
		}
	}
}

// TestManualDeleteOverrideStandsDown checks that the two mechanisms do not both
// drive at once. EQOA_PATCH_DELETE is an operator running the dance by hand,
// usually while debugging exactly this area, and an automatic pass interleaved
// with a manual one produces a sequence neither of them intended.
func TestManualDeleteOverrideStandsDown(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"

	t.Setenv("EQOA_PATCH_DELETE", managedFile)
	s := NewPatchServer(cfg)

	if s.cardSeq != nil && s.cardSeq.on {
		t.Error("automatic refresh sequencing should stand down while " +
			"EQOA_PATCH_DELETE drives")
	}
	if x := manifestFor(t, s, "10.0.0.6"); !deletesManaged(x) {
		t.Error("the manual override must still serve its Delete pass")
	}
}

// TestMidRefreshIsTheWindowBetweenThePasses pins what the Freeport technician
// asks about. The window opens when a console is told to delete its SD and
// closes when it is SEEN downloading the replacement — a file GET, not a
// manifest GET, because only the download says the payload actually arrived.
func TestMidRefreshIsTheWindowBetweenThePasses(t *testing.T) {
	s := newTestServer(t)
	const host = "10.0.0.8"

	if s.ConsoleMidSDRefresh(host) {
		t.Error("a console we have never heard from must not be called mid-update; " +
			"telling a working player to go and reboot is the worse error")
	}

	walkToDelete(t, s, host, managedFile) // its copy is now deleted
	if !s.ConsoleMidSDRefresh(host) {
		t.Error("after the Delete pass the console has no payload and must be " +
			"reported mid-update")
	}

	manifestFor(t, s, host) // offered, but nothing downloaded yet
	if !s.ConsoleMidSDRefresh(host) {
		t.Error("being OFFERED the SD is not having it — the manifest says what " +
			"was offered, only the file GET says what was taken")
	}

	sd := find(s, managedFile)
	if sd == nil {
		t.Skip("the managed file is not served")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/"+sd.RemoteName, nil)
	req.RemoteAddr = host + ":51000"
	s.servePatchFile(rec, req, sd)

	if s.ConsoleMidSDRefresh(host) {
		t.Error("the console downloaded the current SD; the window must be closed")
	}
}

// TestDiagnosticFetchDoesNotConsumeAPass is the bug a tester hit with EQOADIAG:
// the tool that reports whether the card is current was the reason it was not.
//
// It fetches the patcher's manifest URL from the console's own address to read
// the server's side of the comparison, and it cannot apply a Delete. Counted as
// pass 1, that spent the console's Delete on nothing: the next real fetch got
// pass 2, whose SD entry the patcher skips because the stale file has the same
// name and the same size — SD is repointed in place, so it never changes length.
// The card then keeps the old payload until the SD content itself changes.
func TestDiagnosticFetchDoesNotConsumeAPass(t *testing.T) {
	s := newTestServer(t)
	const host = "10.0.0.10"

	first := diagManifestFor(t, s, host)
	if len(managedDeletes(first)) != 1 {
		t.Error("the diagnostic must be told the truth about which pass is due")
	}
	if s.ConsoleMidSDRefresh(host) {
		t.Error("reading the manifest deleted nothing; the console still has its " +
			"files and must not be reported mid-update")
	}

	// Twice, because a tester runs the tool more than once. However many times
	// it runs, the sequence must not have moved.
	again := diagManifestFor(t, s, host)
	if managedDeletes(again)[0] != managedDeletes(first)[0] {
		t.Fatal("a second diagnostic fetch was answered with a different pass; " +
			"reading the manifest moved the state machine")
	}

	// And the patcher then walks the sequence from the START, having missed
	// nothing.
	for _, name := range s.cardSeq.order {
		x := manifestFor(t, s, host)
		if got := managedDeletes(x); len(got) != 1 || got[0] != name {
			t.Fatalf("the patcher was served %v where the sequence expects [%s]: "+
				"a Delete was spent on a diagnostic, so that file is never "+
				"removed and its download is skipped on size", got, name)
		}
	}
	if last := manifestFor(t, s, host); !downloadsManaged(last) {
		t.Fatal("the delivering pass did not carry the SD download")
	}
}

// TestVersionFloorSurvivesAFreshUnpack covers the other way a card stays stale
// with the right payload sitting in patches/.
//
// The Product Version lives in patches/.version, which the distribution does not
// carry, so it only counts up within one unpacked copy of the server. Unzip a
// new build beside the old one and the number restarts below what the card has
// already applied — and the patcher gates the WHOLE distribution on that number,
// so it fetches the manifest and downloads nothing at all.
func TestVersionFloorSurvivesAFreshUnpack(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"

	// What an older copy of the server left behind, and what the card applied.
	old := NewPatchServer(cfg)
	if err := os.WriteFile(filepath.Join(old.patchDir(), ".version"),
		[]byte("41 0badf00d\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := NewPatchServer(cfg)
	if fresh.productVersion < versionFloor {
		t.Fatalf("a fresh copy advertises Product Version %d, below the floor %d",
			fresh.productVersion, versionFloor)
	}
	if fresh.productVersion <= 41 {
		t.Fatalf("Product Version %d is not past the 41 the card already applied; "+
			"the patcher would download nothing", fresh.productVersion)
	}
}

// TestUpgradeCreditsConsolesFromBeforeDownloadTracking guards the direction this
// is allowed to be wrong in.
//
// A state file written before downloads were tracked carries `seen` and no
// `got`. Reading that absence literally says "no console has the payload", which
// would send every already-working player off to reboot for nothing — the exact
// error midRefresh exists to avoid. Absent is not empty.
func TestUpgradeCreditsConsolesFromBeforeDownloadTracking(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	s := NewPatchServer(cfg)
	managed := find(s, managedFile)
	if managed == nil {
		t.Skip("the managed file is not served")
	}

	// Exactly what the previous version wrote: a fingerprint and `seen`, no `got`.
	old := fmt.Sprintf(`{"fingerprint":%d,"seen":{"10.0.0.9":%d}}`, managed.CRC, managed.CRC)
	path := filepath.Join(s.patchDir(), ".sdstate")
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	upgraded := NewPatchServer(cfg)
	if upgraded.ConsoleMidSDRefresh("10.0.0.9") {
		t.Error("a console that finished the dance before download tracking existed " +
			"was reported mid-update; it would be told to reboot for nothing")
	}

	// IT IS NOW DANCED ONCE FOR THE CURRENT MANAGED FILE, and that is correct.
	//
	// This used to assert the opposite — that the history suppressed a second
	// dance — and it did so while SD was both the managed file and the one a v1
	// state file recorded. SD is not served at all now (EQOAHDD.ELF installs it),
	// so the migrated entry is inert and the console has genuinely never been
	// walked through a pass for SS. Dancing it once is new work, not repeated
	// work: the same distinction the old comment drew for eqahosts.txt.
	//
	// What must NOT happen is the migration being read as "this console has
	// nothing", which is the false-mid-refresh error checked above and the whole
	// reason the old file is read at all.
	danced := false
	for i := 0; i <= len(upgraded.cardSeq.order); i++ {
		if deletesManaged(manifestFor(t, upgraded, "10.0.0.9")) {
			danced = true
			break
		}
	}
	if !danced {
		t.Errorf("the console was never offered a Delete pass for %s, which it "+
			"has no recorded history for — a v1 state file must not credit a "+
			"console for a file that was not being managed when it was written",
			managedFile)
	}
}

// TestRearmOneHostLeavesTheOthersAlone is the operator's tool for the weakness
// that cannot be designed away: the sequencing key is a source IP, and an IP is
// not a console. When two consoles share an address the first spends the passes
// and the second keeps its stale file, so the operator needs a way to re-arm
// the one that missed — WITHOUT the blunt instrument of deleting .sdstate,
// which costs every other console in the field a boot.
func TestRearmOneHostLeavesTheOthersAlone(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	const stuck, fine = "203.0.113.9", "203.0.113.10"
	s1 := NewPatchServer(cfg)
	if find(s1, managedFile) == nil {
		t.Skip("the managed file is not served")
	}
	for _, h := range []string{stuck, fine} {
		walkToDelete(t, s1, h, managedFile)
		s1.cardSeq.noteDownloaded(h, managedFile) // the console fetched the replacement
		if deletesManaged(manifestFor(t, s1, h)) {
			t.Fatalf("%s was offered another Delete after converging", h)
		}
	}

	t.Setenv("EQOA_REFRESH_REARM", stuck)
	s2 := NewPatchServer(cfg)
	if !deletesManaged(manifestFor(t, s2, stuck)) {
		t.Error("the re-armed host was not offered a Delete; a stale card there " +
			"has no way back short of deleting the whole state file")
	}
	if deletesManaged(manifestFor(t, s2, fine)) {
		t.Error("re-arming one host cost another console a boot, which is the " +
			"whole reason this is not just `rm .sdstate`")
	}
}

// TestRearmAllIsTheDocumentedHammer. `rm patches/.sdstate` has always meant
// "everyone dances again"; it should not need a different mechanism from the
// targeted form, and it must still work when the state file is intact.
func TestRearmAllIsTheDocumentedHammer(t *testing.T) {
	inScratchPatches(t)
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Patch.WorldsUDPPort = 7003

	const a, b = "203.0.113.11", "203.0.113.12"
	s1 := NewPatchServer(cfg)
	if find(s1, managedFile) == nil {
		t.Skip("the managed file is not served")
	}
	for _, h := range []string{a, b} {
		walkToDelete(t, s1, h, managedFile)
		s1.cardSeq.noteDownloaded(h, managedFile)
	}

	t.Setenv("EQOA_REFRESH_REARM", "all")
	s2 := NewPatchServer(cfg)
	for _, h := range []string{a, b} {
		if !deletesManaged(manifestFor(t, s2, h)) {
			t.Errorf("%s was not re-armed by EQOA_REFRESH_REARM=all", h)
		}
	}
}

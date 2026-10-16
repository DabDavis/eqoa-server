package tcp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// The manifest is a DIRECTORY. These tests pin that contract, because the whole
// value of it is that an operator can add or remove a payload without touching
// Go — and "without touching Go" means nothing in Go asserts the file list, so
// the behaviour of the scan is the only thing left to check.

// A file dropped into serve/ is offered, under its own name, with its own bytes.
func TestDroppedFileIsServed(t *testing.T) {
	inScratchPatches(t)
	body := []byte("dropped in by hand\n")
	if err := os.WriteFile(filepath.Join("patches", "serve", "NEWTHING.BIN"),
		body, 0o644); err != nil {
		t.Fatal(err)
	}

	f := find(newServeTestServer(t), "NEWTHING.BIN")
	if f == nil {
		t.Fatal("a file in serve/ was not offered — dropping one in is the " +
			"whole interface")
	}
	if string(f.Content) != string(body) {
		t.Errorf("served %q, want %q", f.Content, body)
	}
	if f.RemoteName == "" || !strings.HasSuffix(f.RemoteName, ".gz") {
		t.Errorf("no gzipped remote name: %q", f.RemoteName)
	}
}

// And removing one stops it being offered. NOT the same as removing it from a
// card — see readDeleteList — but the manifest must at least stop naming it.
func TestRemovedFileIsNotServed(t *testing.T) {
	inScratchPatches(t)
	if err := os.Remove(filepath.Join("patches", "serve", "ss-art.csf")); err != nil {
		t.Fatal(err)
	}
	if find(newServeTestServer(t), "ss-art.csf") != nil {
		t.Fatal("a file removed from serve/ is still in the manifest")
	}
}

// eqahosts.txt is generated from server_ip. A copy in serve/ would be a second
// answer to the same question, and the two disagreeing is the exact silent
// failure EQOADIAG was built to find — so the folder's copy is refused, not
// merged and not preferred.
func TestServeDirCannotOverrideGeneratedEqahosts(t *testing.T) {
	inScratchPatches(t)
	if err := os.WriteFile(filepath.Join("patches", "serve", "eqahosts.txt"),
		[]byte("10.9.9.9\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newServeTestServer(t)
	f := find(s, "eqahosts.txt")
	if f == nil {
		t.Fatal("no eqahosts.txt at all")
	}
	if strings.Contains(string(f.Content), "10.9.9.9") {
		t.Fatal("the folder's eqahosts.txt won — a console would be pointed " +
			"somewhere other than server_ip, and the SD reads this file to " +
			"find the world server")
	}
	if !strings.HasPrefix(string(f.Content), "127.0.0.1") {
		t.Errorf("eqahosts.txt does not carry server_ip: %q", f.Content)
	}
	// Exactly one entry, or the client is offered the same LocalName twice.
	n := 0
	for _, e := range s.files {
		if e.LocalName == "eqahosts.txt" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d eqahosts.txt entries in the manifest, want 1", n)
	}
}

// A .AUT in serve/ must be refused by the SCAN, not merely absent from a list.
// The suffix is what makes the patcher DNAS-encrypt a module onto the card; it
// fails authentication and it redistributes Sony's code.
func TestServeDirRefusesAUT(t *testing.T) {
	inScratchPatches(t)
	if err := os.WriteFile(filepath.Join("patches", "serve", "SETUP.AUT"),
		[]byte("not really a module"), 0o644); err != nil {
		t.Fatal(err)
	}
	if find(newServeTestServer(t), "SETUP.AUT") != nil {
		t.Fatal("a .AUT in serve/ was served")
	}
}

// serve/.delete is how a file is taken OFF a card. Dropping it from serve/ only
// stops it being offered; the patcher never removes what the manifest does not
// name, so the file stays on every card that has it, forever.
func TestDeleteListIsHonoured(t *testing.T) {
	inScratchPatches(t)
	if err := os.WriteFile(filepath.Join("patches", "serve", ".delete"),
		[]byte("# a comment\nold-junk.bin  # trailing comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newServeTestServer(t)
	f := find(s, "old-junk.bin")
	if f == nil {
		t.Fatal("serve/.delete named a file and no Delete entry was produced")
	}
	if !f.Delete {
		t.Error("the entry is not a Delete")
	}
	if find(s, "# a comment") != nil || find(s, "") != nil {
		t.Error("comments or blank lines became LocalNames")
	}
}

// Dotfiles are server state (.version, .sdstate, .eqahosts) or instructions
// (.delete). None of them is a payload.
func TestDotfilesAreNotServed(t *testing.T) {
	inScratchPatches(t)
	if err := os.WriteFile(filepath.Join("patches", "serve", ".hidden"),
		[]byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range newServeTestServer(t).files {
		if strings.HasPrefix(f.LocalName, ".") {
			t.Errorf("dotfile %q is in the manifest", f.LocalName)
		}
	}
}

// The manifest order must not depend on directory order, because
// bumpProductVersion fingerprints the served set in order: a fingerprint that
// moved with the filesystem would bump the Product Version on a restart that
// changed nothing, and every console would re-fetch the manifest for no reason.
func TestManifestOrderIsStable(t *testing.T) {
	inScratchPatches(t)
	names := func() []string {
		var out []string
		for _, f := range newServeTestServer(t).files {
			out = append(out, f.LocalName)
		}
		return out
	}
	a, b := names(), names()
	if len(a) != len(b) {
		t.Fatalf("two scans produced %d and %d files", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("scan order differs at %d: %q vs %q", i, a[i], b[i])
		}
	}
	if a[0] != "eqahosts.txt" {
		t.Fatalf("first entry is %q, want the generated eqahosts.txt", a[0])
	}
	// Everything the SCAN produced is sorted. Deletes are appended after it,
	// so the scanned run ends at the first one.
	s := newServeTestServer(t)
	var scanned []string
	for _, f := range s.files[1:] {
		if f.Delete {
			break
		}
		scanned = append(scanned, f.LocalName)
	}
	if len(scanned) < 2 {
		t.Fatalf("only %d scanned files — the fixture is not exercising this", len(scanned))
	}
	if !slices.IsSorted(scanned) {
		t.Errorf("the scanned files are not sorted: %v", scanned)
	}
}

// An override serves a file from patches/ under a serve/ name. This is the
// rollback and the bisect control, and it must not rename anything: the
// LocalName is what the card keys on.
//
// Exercised on S2. It was SD until SD stopped being served — EQOAHDD.ELF
// installs it now — and the mechanism under test was never SD-specific.
func TestOverrideReplacesTheFolderCopy(t *testing.T) {
	inScratchPatches(t)
	folder, err := os.ReadFile(filepath.Join("patches", "serve", "S2"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EQOA_S2_FILE", "S2-stock")

	f := find(newServeTestServer(t), "S2")
	if f == nil {
		t.Fatal("the override produced no S2 entry at all")
	}
	if string(f.Content) == string(folder) {
		t.Fatal("the override served the folder's copy — it did nothing")
	}
	if f.LocalName != "S2" {
		t.Fatalf("the override renamed the card file to %q", f.LocalName)
	}
}

// An override naming a file that is not there falls back to the folder rather
// than serving nothing. A missing link of the loader chain is indistinguishable
// at the console from "the patch did nothing", and that has already cost a
// hardware run.
func TestUnreadableOverrideFallsBackToTheFolder(t *testing.T) {
	inScratchPatches(t)
	folder, err := os.ReadFile(filepath.Join("patches", "serve", "S2"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EQOA_S2_FILE", "S2-does-not-exist")

	f := find(newServeTestServer(t), "S2")
	if f == nil {
		t.Fatal("a bad override left the manifest with no S2")
	}
	if string(f.Content) != string(folder) {
		t.Error("a bad override did not fall back to the folder's copy")
	}
}

// Two payloads must never share a remote name: the client skips one it has
// already fetched. remoteNameFor has to be injective over the LocalNames, and
// the old derivation was not — it truncated at the first ".", so "ss-boot.gz"
// and "ss-art.csf.gz" both reduced to "ss".
func TestRemoteNameForIsDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, local := range []string{
		"SS", "S2", "SD", "ss-art.csf", "station.pak", "WORDS.TXT",
		"ATAD.IRX", "HDD.IRX", "PFS.IRX", "EQPREP.IRX", "EQSCENE.BIN",
	} {
		got := remoteNameFor(local)
		if got == "" {
			t.Errorf("%s produced an empty remote name", local)
		}
		if prev, ok := seen[got]; ok {
			t.Errorf("%q and %q both derive the remote name %q", prev, local, got)
		}
		seen[got] = local
	}
}

// newServeTestServer builds a server against the CURRENT working directory,
// which inScratchPatches has already pointed at a throwaway copy. Separate from
// newTestServer because these tests edit that copy first.
func newServeTestServer(t *testing.T) *PatchServer {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Server.PatchPort = 7000
	return NewPatchServer(cfg)
}

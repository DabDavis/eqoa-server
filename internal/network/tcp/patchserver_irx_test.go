package tcp

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// The three modules the SD's lazy init loads off mc0: before it can mount
// pfs0:. A missing one is not a visible failure at the console — the module
// load answers a negative id into the status block, the mount then fails, and
// world 9 simply never appears.
var hddModules = []string{"ATAD.IRX", "HDD.IRX", "PFS.IRX"}

// THEY ARE NOT SERVED ANY MORE. EQOAHDD.ELF installs them and is their only
// delivery — see elfDelivered in patchserver.go for why one filename may not
// have two writers. The checks below did not stop mattering when the delivery
// moved, so they moved with it: they read eqscene/card/, which is to that tool
// what patches/serve/ is to this server.
//
// Read before newTestServer, which chdirs into a scratch copy of patches/.
const cardDir = "../../../../eqscene/card"

func readCardModule(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cardDir, name))
	if err != nil {
		t.Skipf("%s is not in %s (%v) — run open-irx/build.sh", name, cardDir, err)
	}
	return b
}

// SCE's modules, extracted from SOCOM II (SCUS-97275 /RUN/IRX/HDD/). They work,
// they are what the port was verified against, and they must NEVER be shipped:
// handing them to players is the redistribution problem the open stack exists
// to remove. eqscene/card/ is a directory people copy files into, and both sets
// carry the same filenames, so this is pinned rather than trusted.
var sceModuleMD5 = map[string]string{
	"079648d35582e15f8a8761a7834edbdb": "SCE ATAD.IRX",
	"88559646b440efa98990a3b1ee6ed85f": "SCE HDD.IRX",
	"d9fa456ab679871b0e61c5e6eb628a41": "SCE PFS.IRX",
}

// TestHDDModulesAreNotServed is the inverse of the check that used to be here.
//
// While both ends delivered these files the rule was "the tool's copy must equal
// the served copy", and it was enforced at the tool's build. That cannot be made
// safe: the patcher decides whether to re-download by SIZE, so two copies that
// differ and happen to match in length leave the card holding whichever landed
// first — forever, with the server, the patcher and the tool all reporting
// success. Absence is checkable and agreement never really was.
func TestHDDModulesAreNotServed(t *testing.T) {
	s := newTestServer(t)
	for name := range elfDelivered {
		if f := find(s, name); f != nil {
			t.Errorf("%s is served (%d bytes). EQOAHDD.ELF installs it too, so a "+
				"card would end up with whichever arrived first and nothing "+
				"would report which. Remove it from patches/serve/.",
				name, len(f.Content))
		}
	}
}

// TestServeELFFilesOverrideStillWorks: the refusal is absolute by default and
// escapable for an afternoon, because bisecting a console against an older
// delivery is a real thing to need and editing the binary is not.
func TestServeELFFilesOverrideStillWorks(t *testing.T) {
	inScratchPatches(t)
	if err := os.WriteFile(filepath.Join("patches", "serve", "ATAD.IRX"),
		[]byte("\x7fELFnot a real module"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Server.ServerIP = "127.0.0.1"
	cfg.Server.PatchPort = 7000

	if s := NewPatchServer(cfg); find(s, "ATAD.IRX") != nil {
		t.Error("ATAD.IRX was served without EQOA_SERVE_ELF_FILES")
	}

	t.Setenv("EQOA_SERVE_ELF_FILES", "1")
	if s := NewPatchServer(cfg); find(s, "ATAD.IRX") == nil {
		t.Error("EQOA_SERVE_ELF_FILES=1 did not put the file back")
	}
}

func TestCardIRXAreNotSCEs(t *testing.T) {
	for _, name := range hddModules {
		sum := md5.Sum(readCardModule(t, name))
		if which, bad := sceModuleMD5[hex.EncodeToString(sum[:])]; bad {
			t.Fatalf("eqscene/card/%s is %s — that file cannot be redistributed, "+
				"and it is now baked into an ELF players download. Rebuild the "+
				"open stack with open-irx/build.sh, which installs the ps2sdk "+
				"builds over it.", name, which)
		}
	}
}

// ps2sdk's ps2hdd/ps2fs import ps2sdk's own file manager, `iomanx`, which is
// NOT resident under EQOA and whose presence would displace `ioman` and kill
// mc0: for the IOP module loader. open-irx/build.sh rewrites the import table's
// library name to `ioman`; skipping that step produces modules that load to
// -200 (unresolved imports) and a world that never streams. The name lives in
// the import table header (magic 0x41E00000, name at +0x0C), but a plain byte
// scan is enough to catch the unrelinked build, since nothing else in these
// modules contains the string.
func TestCardIRXDoNotNameIomanx(t *testing.T) {
	for _, name := range hddModules {
		content := readCardModule(t, name)
		for i := 0; i+6 <= len(content); i++ {
			if string(content[i:i+6]) == "iomanx" {
				t.Fatalf("eqscene/card/%s still names `iomanx` at offset %d — it "+
					"was not relinked, and will load to -200 against EQOA's "+
					"resident `ioman`. Run open-irx/build.sh.", name, i)
			}
		}
	}
}

func TestCardIRXAreELFs(t *testing.T) {
	for _, name := range hddModules {
		content := readCardModule(t, name)
		if len(content) == 0 {
			t.Errorf("eqscene/card/%s is empty", name)
			continue
		}
		if string(content[:4]) != "\x7fELF" {
			t.Errorf("eqscene/card/%s is not an ELF (%q) — an IRX that is not an "+
				"ELF is rejected by the IOP loader with no message the console "+
				"shows", name, content[:4])
		}
	}
}

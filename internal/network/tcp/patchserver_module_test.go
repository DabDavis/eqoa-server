package tcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No rebuilt retail module may be served. Two independent reasons, and either
// one alone is enough:
//
//  1. IT DOES NOT WORK. Measured 2026-08-11 with a rebuilt SETUP.AUT: the
//     client downloads it — the GET is in the server log — and then stops at
//     "One or more updates failed authentication" without installing anything.
//     No SETUP.ENC reached the memory card. That refutes the claim in
//     claude-notes/module-replacement.md that the auth trail is
//     content-independent and a modified module loads with no errors.
//
//  2. IT IS SONY'S CODE. A rebuilt .AUT ships their module whole. This is the
//     same problem open-irx/ exists to solve for ATAD/HDD/PFS, and the same
//     reasoning as TestServedIRXAreNotSCEs next door.
//
// patches/ is a directory people copy files into, and make_setup_menu.py will
// happily write a rebuilt module wherever it is pointed, so this is checked
// rather than trusted.
func TestNoRetailModulesServed(t *testing.T) {
	s := newTestServer(t)
	for _, f := range s.files {
		if strings.HasSuffix(strings.ToUpper(f.LocalName), ".AUT") {
			t.Errorf("%s is being served. A LocalName ending in .AUT is what "+
				"makes the patcher DNAS-encrypt a module onto the card — it "+
				"fails authentication, and it redistributes Sony's module. "+
				"Change module behaviour through the SS/S2/SD chain instead.",
				f.LocalName)
		}
	}
}

// And the artifact must not sit in either directory. serve/ is the sharper of
// the two now — its contents ARE the manifest, so a .AUT copied in is served,
// with no code change and nothing to uncomment. patches/ still matters because
// it is the override pool: a name there is one config line from going out.
func TestNoRetailModulesInPatchDir(t *testing.T) {
	root := patchDirForTest(t)
	for _, dir := range []string{root, filepath.Join(root, "serve")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Skipf("%s unreadable: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.Contains(strings.ToUpper(e.Name()), ".AUT") {
				t.Errorf("%s/%s. Rebuilt retail modules belong in "+
					"claude-eqoa-iso/decrypted_modules/"+
					"module-replacement-attempts/, which nothing serves.",
					dir, e.Name())
			}
		}
	}
}

func patchDirForTest(t *testing.T) string {
	t.Helper()
	// The tests run in the package directory; patches/ is at the module root.
	return filepath.Join("..", "..", "..", "patches")
}

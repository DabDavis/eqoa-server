package tcp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/eqoa/server/internal/config"
)

// ssFile selects which SS payload is served. SS is the BOOTSTRAP: it is the
// only thing that installs the loader stub which reads and applies S2. Serving
// an SS without that stub means S2 is downloaded to the memory card and then
// ignored entirely — every S2 record, Sandstorm's included, silently does
// nothing. There is no error and no log line; the file simply is not read.
//
//	ss-s2boot.dat  THE ONE TO SERVE. Sandstorm's bootstrap: their 1196-byte stub
//	               at 0x00157400 plus the CLIENT+0xBBEC hook, re-emitted as an
//	               aligned CLIENT-relative quadword (their own 4-byte write to
//	               0x00BC056C is misaligned by 12, and sceRead is DMA and needs
//	               16-byte alignment). The three neighbour words are byte-for-byte
//	               retail; only 0x00BC056C changes, from `jal 0x00BC0730` to
//	               `jal 0x00157400`. The stub recovers client_base as
//	               `$ra - 0xBBF4` = 0x00BB4980 and tail-jumps to CLIENT+0xBDB0,
//	               which IS 0x00BC0730, so the displaced call still happens.
//	ss-armor.dat   NOT a bootstrap. A self-contained armor patch with its own
//	               caves (0x0F9000-0x0FA800) and a CLIENT+0xBBE0 hook. No stub,
//	               no "S2" string — it never loads S2. Predates the move to
//	               extending S2 and is kept only for bisecting against it.
//	ss-remap9.dat  a diagnostic: no stub, no S2, no CHAR.ESF loading. It only
//	               raises the armor clamp and remaps set 9 onto retail set 4, so
//	               set 9 rendering as PLATE proves our SS-applied hooks reach
//	               SUPPORT and take effect.
//
//	ss-dual.dat    OURS, built from source by re-tools/build_stub.py. Same hook,
//	               same load address, same S2 semantics — stage 1 applies S2 in
//	               Sandstorm's arena with their malloc order and their rounding,
//	               so the cave pointer and every relocated address come out
//	               identical. Stage 2 then loads "SD" into a SEPARATE arena at
//	               0x0F000000. Verified by executing BOTH stubs under the R5900
//	               interpreter against a real EE image: 0 of 20,277 bytes differ
//	               and 0 extra, over 3 EE dumps x 3 S2 builds x 5 allocator
//	               bases. See claude-notes/own-loader-stub.md.
//
// The ss-step*/ss-3* payloads in patches/ were bisect aids and are unused.
//
// EMPTY MEANS patches/serve/SS. The served bytes live in the folder now, and
// this constant exists only so an override can name something else in patches/;
// see payloadOverrides. `ss-dual.dat` is what serve/SS was copied from.
const defaultSSFile = ""

// ssFile is the SS bootstrap to serve. EQOA_SS_FILE overrides it, mirroring
// EQOA_S2_FILE — SS is the single riskiest file in the set (it is what makes S2
// apply at all), and ours has never run on hardware, so rolling back to
// Sandstorm's must be a restart rather than a rebuild.
//
// Whichever is served, the switch needs the two-pass Delete dance below: SS is
// already on the memory card under that LocalName, and a CRC-stamped remote
// name is NOT enough on its own (re-tested 2026-08-04, see bumpProductVersion).
//
//	pass 1   EQOA_PATCH_DELETE=SS  <restart>   client patches, SS is removed
//	pass 2   <restart, no env>                 client patches, new SS + SD land
//
// Between the passes the client has no stub, so S2 is written to the card and
// ignored — the game boots unpatched. That is expected, not a failure.
//
// Before deploying, re-run the verifier against the exact files this will
// serve:
//
//	cd go-eqoa-esf-tools-v2 && go run ./cmd/stubverify \
//	  -ee ../memory-dumps/inworld-eqoa.live.eememory \
//	  -ref ../go-eqoa-server/patches/ss-s2boot.dat \
//	  -ours ../go-eqoa-server/patches/serve/SS \
//	  -s2 ../go-eqoa-server/patches/serve/S2 \
//	  -sd ../eqscene/card/SD
//
// go-eqoa-server/patches/build.sh runs exactly that, against exactly those
// files, on every invocation.
func (s *PatchServer) ssFile() string {
	return resolvePayload("EQOA_SS_FILE", s.cfg.Patch.SSFile, defaultSSFile)
}

// s2File selects which S2 payload is served.
//
//	S2-stock   Sandstorm's, byte-for-byte. Set 9 and robes only; the armor-set
//	           clamp is < 10, so set 10 is clamped away to "no armor".
//	S2-set10   the same file extended by re-tools/extend_s2.py — 3 of their 108
//	           records repointed, 5 added, clamp raised to < 11. (== patches/S2)
//	S2-race10  S2-set10 plus playable race 10, by re-tools/extend_s2_race.py:
//	           6 records added, 0 of theirs touched. gRaces[10] filled, the
//	           11-entry character-creation race menu relocated to cave 0x0F8000
//	           and grown to 12, its loop counter and list capacity bumped.
//	S2-race10-nohelm
//	           S2-race10 minus ONLY the four helm records and their two relocs,
//	           by re-tools/strip_s2_helm.py. Everything else is byte-identical.
//	S2-race10-noresload
//	           S2-race10 with ONE record reverted: the 0x004283CC repoint goes
//	           back to Sandstorm's `j 0xF9200`, so our set-10 loader never runs
//	           and its two dictionary registrations never happen. The clamp,
//	           the GetArmorSetTexture hook and every cave stay exactly as they
//	           are. TESTED ON THE CONSOLE: the symptom PERSISTED, so the
//	           set-10 loader is not the cause. One of a ladder of
//	           single-variable controls built by re-tools/s2_revert.py; the
//	           strip_s2_*.py ones each move three or four records, so a bug
//	           that clears with them names a group, not a cause.
//	S2-race10-noset10
//	           S2-race10 minus ONLY armor set 10, by re-tools/strip_s2_set10.py:
//	           the five set-10 caves dropped and the three records set 10 had
//	           REPOINTED (SetResources, the clamp, GetArmorSetTexture) reverted
//	           to Sandstorm's payloads, so set 9 is untouched. Diffed against
//	           S2-stock it adds exactly the 13 race+helm records and the SetSky
//	           repoint and nothing else. Froglok and helm still work.
//
// NAKED NPCs / INVISIBLE HANDS AND FEET — investigated 2026-08-05. The
// mechanism this comment used to assert is WRONG, and it was sending the
// bisect at the wrong records:
//
//	"$Armor stores baked VIRaster POOL INDICES, and set 10's loader creates two
//	 surfaces before the original body, shifting every index assigned after it."
//
// $Armor is not baked. SetResources makes 65 calls and every one of them is
// VIDictionary::FindTyped — it loads nothing. The instruction the hook
// displaces (`lui $v0, 0x0050` at +0x4) is the high half of $Armor itself, and
// the function's first act is to write -1 across all nine retail sets and then
// refill them from FindTyped results. So the table is computed at runtime and
// is self-consistent with however many resources exist. Sandstorm's own loader
// registers FIVE at the same site and retail models render fine, which the
// shift story never explained either.
//
// What the blast radius actually is (full applied-image diff, S2-stock vs
// S2-race10: five code hooks plus race data, nothing else):
//
//	GetArmorSetTexture  CLEARED — executed under the R5900 interpreter for every
//	                    (set, slot); retail sets 0-8 are byte-identical between
//	                    the two patches. cmd/armordiff.
//	ParseCSpriteObj     CLEARED — the gate `lw $t0, 36($sp)` is the record's own
//	                    DictID, not stack garbage. cmd/hookprobe.
//	VIHSprite::Raster   CLEARED — the gate is a bone's FixScale.y == 1.61f, and
//	                    0 of 22,199 bones across all 568 CHAR.ESF CSprites carry
//	                    that value. cmd/fixscalescan.
//	SetArmorSlot clamp  CLEARED — it only changes behaviour for sets >= 9, and
//	                    the 33 NPCs captured in packets.db use sets 0-8 only.
//	SetResources hook   NOT CLEARED. The one remaining path by which our patch
//	                    can change what sets 0-8 render: two extra dictionary
//	                    registrations performed before those 65 FindTyped calls.
//
// S2-race10-noresload tested that last one on the console: the symptom
// PERSISTED, so the set-10 loader is out too, and every hook now measures
// clean while one of them still causes it on hardware. cmd/dictcollide also
// rules out the obvious follow-up — stage 2 registers 10 DictIDs into the
// shared dictionary and none of them collides with the 65 SetResources asks
// for.
//
// What remains is a bisect where each step moves ONE record. re-tools/
// s2_revert.py builds them; each is verified to differ from S2-race10 in
// exactly one record, to carry no orphaned relocation, and to apply through
// the loader. Serve one with EQOA_S2_FILE and the first that clears the
// symptom names the cause:
//
//	S2-race10-nosetsky     CLIENT+0xC220 — stage 2 never runs. TEST FIRST: it
//	                       is the only remaining change that does real work at
//	                       runtime rather than redirecting a lookup, and its
//	                       froglok parse puts 5 surfaces, 2 RefMaps and a
//	                       SkinSubSprite into the same pool the armor system
//	                       draws from.
//	S2-race10-nocsprite    0x00437984 — the ParseCSpriteObj hook
//	S2-race10-noraster     0x004478B8 — the VIHSprite::Raster hook
//	S2-race10-noarmortex   0x004293C8 — our GetArmorSetTexture wrapper
//	S2-race10-noclamp      0x00422574 — clamp back to 10
//	S2-race10-noresload    0x004283CC — already tested, not it
//
// DROPPING a record must drop the relocations inside it. A leftover entry
// still fires, on whatever RETAIL left at that address — at
// ParseCSpriteObj+0x534 it turns retail's own `jal CreateParticleEmitters`
// into a jump into the cave arena, which is worse than not running the control
// and would read as a result. The strip_s2_*.py controls are clean (audited);
// s2_revert.py now refuses to emit an orphan.
//
// Serving the stock file is the control: if set 9 renders with it and not with
// S2-set10, our edit broke their machinery. If set 9 fails with BOTH, the S2 is
// not being applied at all and the fault is upstream of the armor entirely.
//
// PICK THE CONTROL THAT ISOLATES ONE VARIABLE. Reverting all the way to
// S2-stock removes the armor set, the race AND the helm at once, so a bug that
// clears tells you only that something of ours caused it. The helm is the part
// that reaches retail models — its two hooks are on paths every model runs
// (ParseCSpriteObj for all 568 CSprites, VIHSprite::Raster for every
// attachment) — so S2-race10-nohelm is the sharper control: same race, no helm.
// Bisect nohelm first and fall back to stock only if it does not clear.
//
// Changing this is NOT enough on its own. The patcher has already written S2 to
// the memory card, and a new payload under an existing LocalName needs the
// two-pass Delete dance described in bumpProductVersion — pass 1 with
// EQOA_PATCH_DELETE=S2, then pass 2 without it. Skipping pass 1 looks exactly
// like success: the manifest GET appears in the log and no file GET follows.
//
// EMPTY MEANS patches/serve/S2, which was copied from S2-race10. Every name
// above is still in patches/ and still reachable with EQOA_S2_FILE.
const defaultS2File = ""

// s2File is the S2 payload to serve. EQOA_S2_FILE overrides it so a bisect is a
// restart rather than a rebuild — the two-pass Delete dance already needs two
// restarts per swap, and needing a recompile between them is what makes people
// skip pass 1. The name is a patches/ filename; an unreadable one is logged and
// SKIPPED by buildPatchFiles, which serves a manifest with no S2 at all, so it
// is validated here instead of failing silently three steps later.
func (s *PatchServer) s2File() string {
	return resolvePayload("EQOA_S2_FILE", s.cfg.Patch.S2File, defaultS2File)
}

// defaultSDFile is the HDD build, the one on the console's card. It WAS "SD",
// the memory-card guk loader, and that was silently wrong from the moment
// worlds moved to the hard disk: patches/SD has a NINE-entry world table
// (indices 0..8) with its bound patched to 9, so world 9 is out of range
// entirely. Serving it over the card's copy takes the HDD world away and gives
// no error — the teleport falls out of bounds and the player lands in Tunaria,
// which for a world lifted OUT of Tunaria is nearly indistinguishable from
// success.
//
// It is now the CURRENT build rather than a bare fallback, because config.yaml
// is no longer the only thing standing between an operator and a wrong payload:
// DefaultConfig used to carry its own copy of this name, it had drifted to "SD",
// and a server that omitted sd_file got the memory-card build. That copy is
// gone (see config.PatchConfig) and this constant is the single answer.
//
// SD-hddnpc is the whole chain: the three VIFile hooks, the on-demand fetcher —
// whichever "hdd:" world the client opens, rather than the single slot an SD was
// built for — and the setup scene the Freeport technician launches. The older
// SD-hdd-mc0 has no fetcher at all, so it costs a console every world its disk
// does not already hold.
//
// THE PAYLOAD CARRIES NO SERVER ADDRESS. It reads one off the card at runtime,
// out of eqahosts.txt — the same file the game uses to find the server — so the
// same bytes are correct for any operator and the console never holds two
// copies of one address that can disagree. This server therefore serves the SD
// exactly as it was built and only CHECKS it; see sdverify.go.
//
// ---------------------------------------------------------------------------
// SD-c IS THE C PAYLOAD, AND IT IS WHAT IS SERVED (2026-08-14).
//
// re-tools/hdd_sd/ is re-tools/hdd_sd.S rewritten as C plus four assembly hook
// trampolines. Same status block, same wire, same behaviour under the R5900
// interpreter: pkg/mips TestHDDSD runs all 15 behavioural tests against BOTH
// payloads (70 subtests, 0 skips), and cmd/stubverify executes the stub
// applying this exact file against a relocated arena — 26 checks, "applies in
// full, 0/11370 wrong".
//
// WHAT IS NOT COVERED BY ANY OF THAT: the four hooks' own entry and exit.
// stubverify runs the STUB, not the payload's hooks, and the interpreter tests
// call the C bodies directly. The trampolines are checked against the game's
// displaced instructions by disassembly and no more. Hook 1 runs on every file
// open in the game, so if it is wrong it is wrong immediately and everywhere.
//
// THAT IS RECOVERABLE, and it is worth knowing why before serving this. The SS
// stub applies SD from a CLIENT-load hook, which happens AFTER the patcher has
// run — so a payload that crashes the game still lets the next boot patch. A
// bad SD costs the player a restart and the operator a two-pass deploy; it does
// not strand a console. Roll back with EQOA_SD_FILE=SD-hddnpc, which is the
// console-proved assembly build and is kept for exactly that.
//
// DEPLOYING IT NEEDS THE DELETE DANCE, and skipping pass 1 does NOT look like
// success here — for once. SD-c is 12,272 bytes against SD-hddnpc's 12,592, and
// the patcher client does not truncate, so an in-place overwrite leaves 320
// bytes of the old payload's tail. The stub then rejects the file outright:
// stub.S pass 2 requires the section table to consume the file EXACTLY and
// takes its bail path on trailing junk. So a skipped pass 1 gives a console
// with S2 applied, no SD at all, and no world streaming — silent, but a clean
// nothing rather than a corrupt something.
// ---------------------------------------------------------------------------
//
// EMPTY MEANS patches/serve/SD, which was copied from SD-c. The rollback is
// EQOA_SD_FILE=SD-hddnpc, still in patches/ and still executed by
// cmd/stubverify on every patches/build.sh run.
const defaultSDFile = ""

// sdFile is the SD payload to serve. EQOA_SD_FILE overrides it, for the same
// reason EQOA_S2_FILE exists: swapping a payload already needs the two-pass
// Delete dance (two restarts), and needing a recompile in between is what makes
// people skip pass 1.
//
// The names that matter today, all built by re-tools/make_hdd_sd.py:
//
//	SD-hdd-mc0 the one that runs. 10-entry table, entry 9 "t127" ->
//	           "hdd:T127.ESF", entry 8 a clone of entry 2 so an unclaimed index
//	           reads a real world. --modules mc0: it loads ATAD/HDD/PFS from
//	           mc0:/BASLUS-20744EQONLINE/, the redistributable open-irx builds
//	           this server also serves. Console-proved 2026-08-10.
//	SD-t127    the same table, --modules disc. It takes the IRX chain from
//	           cdrom0:\NETGUI\MODULES\{USBKB,USBD,PPP}.IRX, so it needs
//	           EQOA-hdd.iso, where those disc files were REPLACED. On a stock
//	           disc it loads the real USB modules, sceMount returns -19, and
//	           the world has no ground.
//	SD         the retired memory-card loader: 9 entries, "mc:guk.esf" at 8,
//	           no world 9, no HDD. Kept as history; serving it breaks world 9.
//
// Validated here rather than three steps later: buildPatchFiles SKIPS an
// unreadable name, and a manifest with no SD at all means the stub loads
// nothing and every symptom looks like "the patch did nothing".
func (s *PatchServer) sdFile() string {
	return resolvePayload("EQOA_SD_FILE", s.cfg.Patch.SDFile, defaultSDFile)
}

// resolvePayload picks a payload filename: environment override first, then
// config, then the built-in default. The environment deliberately outranks
// config — swapping a payload already costs a restart for the client's two-pass
// Delete dance, and a bisect should not also cost a config edit.
//
// A name containing a path separator is rejected rather than joined: these are
// bare filenames inside the patch directory, and a path here would escape it.
func resolvePayload(envVar, cfgVal, def string) string {
	if name := os.Getenv(envVar); name != "" {
		if strings.ContainsAny(name, `/\`) {
			log.Printf("Patch server: %s=%q must be a bare filename; ignoring", envVar, name)
		} else {
			log.Printf("Patch server: %s override — serving %s", envVar, name)
			return name
		}
	}
	if cfgVal != "" {
		if strings.ContainsAny(cfgVal, `/\`) {
			log.Printf("Patch server: patch config %q must be a bare filename; using %s", cfgVal, def)
			return def
		}
		return cfgVal
	}
	return def
}

// patchDir resolves the directory holding the payloads: the configured path if
// it exists, otherwise the same name alongside the executable. Running from a
// different working directory is the common case for a systemd unit.
func (s *PatchServer) patchDir() string {
	dir := s.cfg.Patch.Dir
	if dir == "" {
		dir = "patches"
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if exe, err := os.Executable(); err == nil {
			return filepath.Join(filepath.Dir(exe), dir)
		}
	}
	return dir
}

// patchFile represents a file entry in the VerantPatcher XML.
type patchFile struct {
	LocalName   string // filename on client (e.g. "eqahosts.txt")
	RemoteName  string // filename on server (e.g. "eqahosts.txt.gz")
	Content     []byte // uncompressed file content
	GzipContent []byte // gzipped content (computed at init)
	CRC         uint32 // CRC32 of uncompressed content
	Delete      bool   // if true, delete the file instead of downloading
}

// PatchServer handles patch server connections on port 7000.
// The EQOA client connects here via HTTP to check for game updates.
type PatchServer struct {
	server         *http.Server
	cfg            *config.Config
	files          []patchFile // patch files to serve
	productVersion int         // bumped whenever the served content changes
	changesCRC     uint32      // fingerprint of the served set; Sandstorm's is non-zero
	refresh        []string    // LocalNames to Delete before re-adding

	// cardSeq sequences the Delete dance per console, over every managed file.
	// productVersion is the FIRST pass's number and each later pass is one
	// more, which is why versions are
	// allocated two at a time below.
	cardSeq *cardRefresh
}

// versionFloor is the lowest <Product Version> this build will ever advertise.
//
// RAISE IT ON EVERY RELEASE, by more than any single copy of the server could
// plausibly have climbed since the last one (each content change costs 2). It
// is what makes the number monotonic ACROSS unpacked copies, which the .version
// file cannot do because the distribution does not carry it: it started at 37,
// the first stock version we could use, and a tester who unzipped a second copy
// silently handed his card a version it had already applied.
// 1000 shipped 2026-08-13; the build server itself reached 1002 the same day
// (EQSCENE.BIN redeploy), so a card patched here would outrank a fresh unpack
// of that release. 1100 for the Echocell release, with room for ~50 content
// changes before the next one has to clear it.
const versionFloor = 1100

// bumpProductVersion keeps <Product Version="N"> in step with the content.
//
// The Verant patcher gates the WHOLE distribution on this number: if the client
// has already applied version N it fetches the manifest and then downloads
// nothing, no matter how the per-file CRCs or remote names change. Editing a
// payload without bumping it means the change never reaches the memory card,
// and the only symptom is silence — the manifest GET appears in the log with no
// file GETs after it.
//
// The version must also be MONOTONIC, so it is persisted alongside a
// fingerprint of the served set rather than derived from a hash.
func (s *PatchServer) bumpProductVersion() {
	// Read the refresh set BEFORE fingerprinting. Pass 1 below serves the same
	// files plus a <File Delete="True"> for each refreshed LocalName, which is a
	// DIFFERENT distribution and must carry a DIFFERENT Product Version — the
	// patcher gates on that number alone, so if both passes shared one, pass 2
	// would download nothing and leave the file deleted and never restored, i.e.
	// strictly worse than not having run the dance at all. Folding the refresh
	// set into the fingerprint makes the two passes bump on their own instead of
	// depending on someone remembering to hand-edit .version between them.
	if v := os.Getenv("EQOA_PATCH_DELETE"); v != "" {
		s.refresh = strings.Split(v, ",")
	}

	fp := crc32.NewIEEE()
	for _, f := range s.files {
		fmt.Fprintf(fp, "%s:%d:%d;", f.LocalName, f.CRC, len(f.Content))
	}
	for _, name := range s.refresh {
		fmt.Fprintf(fp, "delete:%s;", name)
	}
	want := fmt.Sprintf("%08x", fp.Sum32())

	// Fingerprints are taken from the bytes that will actually be SERVED, and
	// only for files this server really serves: a Delete for a file we do not
	// serve would take it off the card and never put it back. eqahosts.txt is
	// generated here and always present; SD may legitimately be missing.
	fps := map[string]uint32{}
	for _, f := range s.files {
		if !f.Delete {
			fps[f.LocalName] = f.CRC
		}
	}
	// STILL .sdstate, though it now holds more than the SD. Renaming it would
	// orphan every state file in the field and cost each console one wasted
	// sequence, and "rm patches/.sdstate + restart" is the documented recovery
	// for a stuck card — a name is not worth breaking either of those.
	s.cardSeq = newCardRefresh(filepath.Join(s.patchDir(), ".sdstate"), fps,
		refreshFilesFromEnv(), len(s.refresh) > 0)

	path := filepath.Join(s.patchDir(), ".version")
	ver, prev := versionFloor, ""
	if b, err := os.ReadFile(path); err == nil {
		var v int
		var f string
		if n, _ := fmt.Sscanf(string(b), "%d %s", &v, &f); n == 2 {
			ver, prev = v, f
		}
	}
	// THE NUMBER LIVES IN A FILE THE DISTRIBUTION DOES NOT CARRY, so it is only
	// monotonic within one unpacked copy of the server. A tester who unzips a
	// new build beside the old one starts again from the floor while the card
	// still remembers the number the old copy reached — and the patcher gates
	// the whole distribution on that number, so it fetches the manifest and
	// downloads NOTHING. Every file on the card stays as it was, including SD,
	// and the log looks exactly like a console that was already up to date.
	//
	// The floor is therefore raised with each release rather than left at the
	// original 37, so a fresh copy always outranks any card in the field.
	if ver < versionFloor {
		log.Printf("Patch server: stored Product Version %d is below this "+
			"build's floor — using %d, so a card patched by an older copy of "+
			"the server is not mistaken for up to date", ver, versionFloor)
		ver, prev = versionFloor, ""
	}
	// A NEWLY ENGAGED DANCE IS ITSELF A CHANGE, even when every byte we serve is
	// identical to last time — what changed is that we no longer believe the
	// consoles are holding the right SD. Without this the dance would be
	// sequenced under a Product Version the field has already applied, so pass 1
	// is never fetched, the Delete never happens, and the mechanism is a no-op
	// that logs exactly like success. Version numbers are free; measure the cost
	// on the other side of that trade.
	if prev != want || s.cardSeq.newlyEngaged {
		if prev != "" {
			// A WHOLE SEQUENCE AT A TIME. The stored number is the first pass
			// and each later pass is one more, so an increment smaller than the
			// sequence would hand the next content change a first-pass number
			// that some console has already applied as a later pass — and the
			// patcher gates on that number alone, so it would fetch the
			// manifest and download nothing. Version numbers are free; a
			// silently skipped distribution is not.
			ver += s.cardSeq.passes()
		}
		if err := os.WriteFile(path, []byte(fmt.Sprintf("%d %s\n", ver, want)), 0o644); err != nil {
			log.Printf("Patch server: could not persist product version: %v", err)
		}
		log.Printf("Patch server: content changed, Product Version -> %d", ver)
	} else {
		log.Printf("Patch server: content unchanged, Product Version %d", ver)
	}
	s.productVersion = ver
	s.changesCRC = fp.Sum32()

	// Replacing a file the patcher has already written to the memory card takes
	// TWO passes, verified by watching the card:
	//
	//   pass 1  serve <File LocalName="X" Delete="True"/>. The patcher removes
	//           X and does NOT re-add it, even if the same manifest also
	//           carries a normal entry for X — delete and add cannot coexist
	//           for one LocalName.
	//   pass 2  serve X normally. It is now missing locally, so it downloads.
	//
	// Serving new bytes under the same LocalName without this does nothing at
	// all: the manifest GET appears in the log with no file GET after it. See
	// also claude-notes/ps2-live-connection-20260803.md on station.pak.
	//
	// RE-TESTED 2026-08-04 and it still holds. Product Version 72 served a new
	// S2 (new CRC, new remote name) with no Delete pass: the client fetched the
	// manifest and Changes.txt and then went straight to news.html without
	// requesting the payload. Do not be tempted by what pass 1 looks like —
	// the DELETE pass does fetch the file, so a log showing an s2-armor GET
	// during pass 1 is not evidence that one pass suffices.
	//
	// EQOA_PATCH_DELETE drives pass 1 (read above, before the fingerprint);
	// clear it for pass 2.
	if len(s.refresh) > 0 {
		log.Printf("Patch server: pass 1 — serving Delete for %v", s.refresh)
	}

	// An override WITHOUT a delete pass is the exact configuration that fails
	// silently, and it cost a hardware run on 2026-08-07: the server served
	// SD-disc8, the console kept the SD already on its memory card, and the
	// crash dump showed world-table entry 8 still reading "guk"/"mc:guk.esf".
	// Nothing in the log said otherwise — a manifest GET with no file GET after
	// it looks identical to success.
	//
	// The server cannot know what is on the card, so it cannot assert. What it
	// can do is name the remote filename to watch for, so "did it take?" is a
	// grep rather than an inference. The file GET is logged by serveFile.
	for _, o := range []struct{ env, def string }{
		{"EQOA_SS_FILE", resolvePayload("", s.cfg.Patch.SSFile, defaultSSFile)},
		{"EQOA_S2_FILE", resolvePayload("", s.cfg.Patch.S2File, defaultS2File)},
		{"EQOA_SD_FILE", resolvePayload("", s.cfg.Patch.SDFile, defaultSDFile)},
	} {
		name := os.Getenv(o.env)
		if name == "" || name == o.def {
			continue
		}
		local := strings.TrimSuffix(strings.TrimPrefix(o.env, "EQOA_"), "_FILE")
		if slices.Contains(s.refresh, local) {
			continue // this IS pass 1 for that file
		}
		var remote string
		for _, f := range s.files {
			if f.LocalName == local {
				remote = f.RemoteName
			}
		}
		log.Printf("Patch server: WARNING — %s=%s is an override but %q is not in "+
			"EQOA_PATCH_DELETE. If %q was already written to the memory card, the "+
			"client will KEEP the old payload and log nothing. Confirm with: "+
			"a GET for %s in this log. Otherwise run pass 1 (%s=%s EQOA_PATCH_DELETE=%s) first.",
			o.env, name, local, local, remote, o.env, name, local)
	}
}

// eqahostsPadMod bounds the padding. 64 keeps the file under 80 bytes — still
// something a human reads at a glance, and nowhere near whatever buffer the
// game reads it into — while giving 64 distinct sizes per address length. Two
// different addresses land on one size about once in 64; that residue is what
// eqahostsPad's caller has to care about, not the common case.
const eqahostsPadMod = 64

// eqahostsFile renders the file the console reads to find this server: the
// address, the blank line the format has always had, and then padding chosen
// from the content so the SIZE differs when the address does. See the call
// site for why the size has to do that work.
//
// `prev` is what this server served last, and it closes the one hole the
// derived pad leaves: two different addresses land on one size about once in
// eqahostsPadMod, and a change the client cannot see is a change that does not
// happen. When the file we are about to serve would be exactly as long as the
// one we served before while saying something else, it gets one more newline.
// A same-address restart takes neither branch, so a stable configuration
// produces a stable file and nobody re-downloads anything.
func eqahostsFile(ip string, prev hostsState) []byte {
	base := ip + "\n\n"
	pad := int(crc32.ChecksumIEEE([]byte(base)) % eqahostsPadMod)
	if prev.IP != "" && prev.IP != ip && len(base)+pad == prev.Size {
		pad++
	}
	return []byte(base + strings.Repeat("\n", pad))
}

// hostsState is the one fact the padding cannot derive from the config: the
// size this server last served for eqahosts.txt. It lives beside the patches in
// its own file rather than in .sdstate, which the refresh sequencer rewrites
// wholesale — and losing it costs one re-download, never a wrong address.
type hostsState struct {
	IP   string `json:"ip"`
	Size int    `json:"size"`
}

func (s *PatchServer) hostsStatePath() string {
	return filepath.Join(s.patchDir(), ".eqahosts")
}

func loadHostsSize(path string) hostsState {
	var st hostsState
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func saveHostsSize(path, ip string, size int) {
	b, err := json.Marshal(hostsState{IP: ip, Size: size})
	if err != nil {
		return
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Printf("Patch server: could not record the eqahosts.txt size (%v). "+
			"Harmless unless server_ip changes to an address whose padded file "+
			"is the same length as this one, which the client cannot see", err)
	}
}

// NewPatchServer creates a new patch server.
func NewPatchServer(cfg *config.Config) *PatchServer {
	s := &PatchServer{
		cfg: cfg,
	}
	s.buildPatchFiles()
	s.bumpProductVersion()
	return s
}

// buildPatchFiles generates the patch file entries from server config.
// Loads static patch files from the patches/ directory and generates
// eqahosts.txt dynamically from server config.
func (s *PatchServer) buildPatchFiles() {
	// eqahosts.txt — generated dynamically with this server's IP, and PADDED so
	// that its SIZE carries its content.
	//
	// THE PATCHER CANNOT SEE A SAME-SIZE CHANGE. Measured on the console
	// 2026-08-14: a card holding "5.161.217.188\n\n" was offered
	// "192.168.40.87\n\n" — same fifteen bytes, different address, different
	// CRC, both in the manifest — and the client did not fetch it, twice, with
	// the wrong address left in place and every log on both ends healthy. It
	// only ever fetched this file when it was ABSENT.
	//
	// That is why the Delete dance grew to cover this file, and the dance is
	// what hangs a console: the pass that removes eqahosts.txt leaves the game
	// stuck forever on "Checking memory card (PS2)… do not reset", which is the
	// one screen a player must not power-cycle from (sd-delivery-holes-20260814).
	//
	// Padding makes the deletion unnecessary. Trailing newlines change the byte
	// count without changing what any reader sees — the game's, and _dl_addr's,
	// which stops at the first character that is not a digit or a dot — so
	// "5.161.217.188" is 76 bytes here and "192.168.40.87" is 50, and the
	// client's own size rule replaces one with the other unaided.
	//
	// The pad is derived from the content, so it is STABLE: a restart, a
	// rebuild, and a config that says the same thing all produce the same file,
	// and a console that has it does not fetch it again.
	hostsContent := eqahostsFile(s.cfg.Server.ServerIP, loadHostsSize(s.hostsStatePath()))
	saveHostsSize(s.hostsStatePath(), s.cfg.Server.ServerIP, len(hostsContent))

	s.files = []patchFile{
		{
			LocalName:  "eqahosts.txt",
			RemoteName: "eqahosts.txt.gz",
			Content:    hostsContent,
		},
	}

	// Module replacement — serve .AUT modules via natural DNAS flow (NO PNACH needed).
	// LocalName MUST end in ".AUT" so the patcher DNAS-encrypts and renames to .ENC.
	// The PS2's own DNAS handles encryption. Launcher decrypts on next boot.
	// Content = raw decrypted .AUT data (same format as disc after DNAS decrypt).
	//
	// NO MODULE IS SERVED, AND NONE CAN BE. Two independent reasons, either one
	// of which is enough — see claude-notes/title-menu-item-20260811.md.
	//
	// 1. IT DOES NOT WORK. Measured 2026-08-11: a rebuilt SETUP.AUT downloads
	//    (the GET is in the log) and the client then stops at "One or more
	//    updates failed authentication" without installing anything — no
	//    SETUP.ENC reaches the card. This REFUTES the claim in
	//    claude-notes/module-replacement.md that the 8,663-byte auth trail is
	//    content-independent and that a modified module loads with no errors.
	//    Whatever that 2026-03-28 SUPPORT test established, it is not this.
	//
	// 2. WE MAY NOT REDISTRIBUTE IT. A rebuilt SETUP.AUT is a derivative of
	//    Sony's module, shipped whole. That is the same reason open-irx exists
	//    — the SOCOM II ATAD/HDD/PFS modules could not be redistributed either,
	//    and were replaced with ps2sdk builds that are ours to ship.
	//
	// The way to change module behaviour is the way the SS/S2/SD chain already
	// does it: OUR bytes, patched into memory at runtime, carrying none of
	// theirs. TestNoRetailModulesServed pins this so a stray .AUT in patches/
	// fails the build rather than shipping.
	for _, mod := range []struct{ localName, file string }{} {
		modPath := filepath.Join(s.patchDir(), mod.file)
		if content, err := os.ReadFile(modPath); err == nil {
			s.files = append(s.files, patchFile{
				LocalName:  mod.localName,
				RemoteName: mod.file + ".gz",
				Content:    content,
			})
			log.Printf("Patch file: %s module (%d bytes, natural DNAS flow)", mod.localName, len(content))
		}
	}

	// THE MANIFEST IS A DIRECTORY. Every readable file in patches/serve/ is
	// served, under its own filename as the LocalName — the name it will have on
	// the memory card. Dropping a file in adds it; removing one stops it being
	// offered. There is no table here to keep in step with the folder, which is
	// what this replaced: the old one repeated each name three times (local,
	// remote, on-disk) in 150 lines of Go, so adding a file meant editing code,
	// and four other places in the tree that also list these names had already
	// drifted out of agreement with it.
	//
	// What is deliberately NOT here:
	//
	//	eqahosts.txt  generated above from server_ip, so a copy in serve/ would
	//	              be a second answer to the same question. Refused.
	//	*.AUT         the suffix that makes the patcher DNAS-encrypt a module
	//	              onto the card. It fails authentication and it ships Sony's
	//	              code; see TestNoRetailModulesServed. Refused.
	//	worlds        no world ESF is served. They reach the console over the
	//	              HDD partition, which the patcher cannot write.
	//	dotfiles      .version / .sdstate / .eqahosts are server state, and
	//	              serve/.delete is read below rather than served.
	//
	// REMOVING A FILE FROM serve/ DOES NOT REMOVE IT FROM A CARD. The patcher
	// only acts on what the manifest names, so a file that merely stops being
	// offered stays on every card that has it. serve/.delete is how to reclaim
	// one; see readDeleteList.
	s.files = append(s.files, s.scanServeDir()...)

	// Compute CRC32 and gzip for dynamic files (eqahosts.txt)
	for i := range s.files {
		if s.files[i].Delete || s.files[i].GzipContent != nil {
			continue
		}
		s.files[i].CRC = crc32.ChecksumIEEE(s.files[i].Content)

		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write(s.files[i].Content)
		gz.Close()
		s.files[i].GzipContent = buf.Bytes()
	}

	// Stamp every remote name with its content CRC.
	//
	// The client skips a file whose remote name it already fetched, so editing a
	// payload without renaming it means the change never reaches the memory
	// card — and if the size is unchanged too, nothing about the manifest looks
	// different. That cost a debugging round when a rebuilt SS stayed 1228
	// bytes under the same name and was silently ignored. A CRC-derived name
	// changes exactly when the bytes change, and stays stable when they do not.
	for i := range s.files {
		if s.files[i].Delete || s.files[i].RemoteName == "" {
			continue
		}
		base := s.files[i].RemoteName
		if ext := strings.Index(base, "."); ext > 0 {
			base = base[:ext]
		}
		s.files[i].RemoteName = fmt.Sprintf("%s-%08x.gz", base, s.files[i].CRC)
	}

	for _, f := range s.files {
		if f.Delete {
			log.Printf("Patch file: DELETE %s", f.LocalName)
		} else {
			log.Printf("Patch file: %s -> %s (size=%d, gzip=%d, crc=%d)",
				f.LocalName, f.RemoteName, len(f.Content), len(f.GzipContent), f.CRC)
		}
	}
}

// serveDir is the folder whose contents ARE the manifest.
func (s *PatchServer) serveDir() string {
	return filepath.Join(s.patchDir(), "serve")
}

// payloadOverrides maps a LocalName to the patches/ filename that should be
// served under it INSTEAD of the serve/ copy.
//
// This is the rollback and bisect control, and it is why patches/ still holds a
// pool of variants next to the folder: swapping a payload already costs two
// restarts for the Delete dance, and it must not additionally cost a rebuild or
// a file shuffle. Empty means "serve the folder's copy", which is the default
// for all three.
//
//	EQOA_SS_FILE=ss-s2boot.dat   Sandstorm's bootstrap
//	EQOA_S2_FILE=S2-race10-nohelm  a single-variable control
//	EQOA_SD_FILE=SD-hddnpc       the last assembly payload
//
// SD IS NO LONGER OVERRIDABLE. It is not served at all — EQOAHDD.ELF installs it
// (elfDelivered) — so an override for it could only name a file this server
// would then refuse. Setting it is reported rather than ignored: an override
// that quietly does nothing is exactly the failure this mechanism exists to
// avoid, and `sd_file` in a config.yaml written before the move would otherwise
// look like it was still steering something. To bisect an SD, build the tool
// against a different eqscene/card/SD.
func (s *PatchServer) payloadOverrides() map[string]string {
	if name := s.sdFile(); name != "" {
		log.Printf("Patch server: IGNORING the SD override %q — SD is not served "+
			"any more. EQOAHDD.ELF installs it, so a payload is swapped by "+
			"putting it in eqscene/card/SD and rebuilding the tool, not here.",
			name)
	}
	over := map[string]string{}
	for local, name := range map[string]string{
		"SS": s.ssFile(), "S2": s.s2File(),
	} {
		if name != "" {
			over[local] = name
		}
	}
	return over
}

// loaderFiles is the part of the chain THIS server delivers. The chain is still
// three files — the SLUS hook loads SS, SS stage 1 applies S2, SS stage 2 loads
// SD — and a missing link still makes the others silently inert with no error at
// either end, which is why the two that are ours are named at startup rather
// than left to be inferred from a console that plays perfectly and patches
// nothing.
//
// SD IS NOT HERE. It is delivered by EQOAHDD.ELF and only by it — see
// elfDelivered — so its absence from serve/ is the correct state and warning
// about it every startup would train an operator to ignore the line that means
// something.
var loaderFiles = []string{"SS", "S2"}

// elfDelivered are the LocalNames EQOAHDD.ELF installs, and the ones this server
// must therefore never send.
//
// The tool used to stage its copies out of serve/, so the file it wrote and the
// file we sent were the same bytes by construction. That is a reasonable answer
// to drift and no answer at all to the real hazard: one filename with two
// writers. The patcher decides whether to re-download by SIZE, so two copies
// that differ and happen to match in length leave the card holding whichever
// landed first — forever, with the server, the patcher and the tool all
// reporting success, and nothing on the console able to say which arrived.
//
// So the name lives in exactly one manifest. Refused rather than warned about,
// for the same reason eqahosts.txt is: a warning in a startup log is not a
// property, and dropping a rebuilt IRX in here is a natural thing to do by hand.
// eqscene/tools/verify.py asserts the same split from the other side, because
// the two directories are edited by different people on different days.
//
// EQOA_SERVE_ELF_FILES=1 puts them back, for bisecting a console against an
// older delivery. It is deliberately not a config key: it is a thing to do for
// an afternoon, not a way to run a server.
var elfDelivered = map[string]string{
	"ATAD.IRX":    "the hard-disk driver stack",
	"HDD.IRX":     "the hard-disk driver stack",
	"PFS.IRX":     "the hard-disk driver stack",
	"EQPREP.IRX":  "makes the EQOAWORLDS partition",
	"EQSCENE.BIN": "the hard-disk setup screen",
	"SD":          "the hard-disk stage of the patch chain",
}

// serveELFFiles restores the old behaviour for one run.
func serveELFFiles() bool { return os.Getenv("EQOA_SERVE_ELF_FILES") == "1" }

// scanServeDir reads patches/serve/ into patchFile entries, sorted by name so
// the manifest — and therefore bumpProductVersion's fingerprint — does not
// depend on directory order.
func (s *PatchServer) scanServeDir() []patchFile {
	dir := s.serveDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Patch server: NO SERVED FILES — %s is unreadable (%v). The "+
			"manifest will carry eqahosts.txt and nothing else, so no console "+
			"will be patched. Create the directory and put the payloads in it.",
			dir, err)
		return nil
	}

	over := s.payloadOverrides()
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	slices.Sort(names)

	var out []patchFile
	present := map[string]bool{}
	for _, local := range names {
		if strings.EqualFold(local, "eqahosts.txt") {
			log.Printf("Patch server: IGNORING %s/eqahosts.txt — that file is "+
				"generated from server_ip, and serving a second copy is how a "+
				"console ends up holding two addresses that disagree. Delete it.",
				dir)
			continue
		}
		if why, ok := elfDelivered[local]; ok && !serveELFFiles() {
			log.Printf("Patch server: REFUSING %s/%s — %s, and EQOAHDD.ELF is "+
				"its only delivery now. Serving it too gives one filename two "+
				"writers: the patcher compares by SIZE, so if the copies ever "+
				"differ but match in length the card keeps whichever landed "+
				"first and nothing reports it. Put the file in eqscene/card/ "+
				"and rebuild the tool. (EQOA_SERVE_ELF_FILES=1 overrides.)",
				dir, local, why)
			continue
		}
		if strings.HasSuffix(strings.ToUpper(local), ".AUT") {
			log.Printf("Patch server: REFUSING %s/%s — a LocalName ending in "+
				".AUT makes the patcher DNAS-encrypt a module onto the card. It "+
				"fails authentication and it redistributes Sony's code; change "+
				"module behaviour through the SS/S2/SD chain instead.", dir, local)
			continue
		}

		// An override serves a file from patches/ under this LocalName. A name
		// that cannot be read falls back to the folder's copy rather than
		// serving nothing: a missing entry is indistinguishable at the console
		// from "the patch did nothing", and that has cost a hardware run.
		path := filepath.Join(dir, local)
		if name, ok := over[local]; ok {
			alt := filepath.Join(s.patchDir(), name)
			if _, err := os.Stat(alt); err == nil {
				path = alt
			} else {
				log.Printf("Patch server: override %s for %s is unreadable (%v) "+
					"— serving %s instead", name, local, err, path)
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Patch file: SKIP %s (%v)", local, err)
			continue
		}

		// station.pak carries its own server addresses, so it has to be
		// repointed at THIS server before it goes out.
		if local == "station.pak" {
			content = s.repointStationPak(content)
		}
		// The SD used to carry one too. It no longer does — the payload reads
		// the address off the card itself, out of eqahosts.txt — so this is a
		// READ, and the file goes out exactly as it was built. See sdverify.go.
		if local == "SD" {
			s.checkSDAddressing(content)
		}

		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write(content)
		gz.Close()

		present[local] = true
		out = append(out, patchFile{
			LocalName:   local,
			RemoteName:  remoteNameFor(local),
			Content:     content,
			GzipContent: buf.Bytes(),
			CRC:         crc32.ChecksumIEEE(content),
		})
	}

	for _, want := range loaderFiles {
		if !present[want] {
			log.Printf("Patch server: WARNING — no %s in %s. The loader chain is "+
				"SS -> S2 -> SD and a missing link makes the others inert with no "+
				"error at either end: the manifest is served, the files land on "+
				"the card, and nothing reads them.", want, dir)
		}
	}

	out = append(out, s.readDeleteList()...)
	log.Printf("Patch server: %d files from %s", len(out), dir)
	return out
}

// readDeleteList reads serve/.delete — LocalNames the patcher should REMOVE
// from every card, one per line, `#` comments.
//
// Separate from EQOA_PATCH_DELETE, which is the two-pass dance and puts the
// file back. These are permanent, and they are the only way to reclaim card
// space: a file dropped from serve/ simply stops being offered, which leaves it
// on every card that already has it.
func (s *PatchServer) readDeleteList() []patchFile {
	path := filepath.Join(s.serveDir(), ".delete")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []patchFile
	for _, line := range strings.Split(string(b), "\n") {
		name, _, _ := strings.Cut(line, "#")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, patchFile{LocalName: name, Delete: true})
	}
	return out
}

// remoteNameFor derives the HTTP filename from the LocalName. The CRC stamp is
// appended later, by buildPatchFiles, and is what actually distinguishes two
// payloads; this half only has to be stable and unique per LocalName.
//
// It is lowercased and punctuation-flattened because these names appear in
// URLs and in server logs, where "ATAD.IRX" and "atad-5bba3f43.gz" reading as
// obviously the same file is worth more than matching the card's spelling.
func remoteNameFor(local string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(local) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// Start begins the HTTP patch server.
func (s *PatchServer) Start() error {
	listenAddr := s.cfg.Server.BindAddress + ":" + itoa(s.cfg.Server.PatchPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.server = &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	log.Printf("Patch server (HTTP) started on %s", listenAddr)

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Patch server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the patch server.
func (s *PatchServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

// handleRequest handles all HTTP requests to the patch server.
func (s *PatchServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Patch server request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Handle the EQOA patch update XML request
	if r.URL.Path == "/patch/eqoa/eqoa-frontierslive-update.xml.gz" ||
		r.URL.Path == "/patch/eqoa/eqoa-frontierslive-update.xml" {
		s.serveUpdateXML(w, r)
		return
	}

	// Handle Changes.txt (client fetches this after reading the XML)
	if r.URL.Path == "/patch/eqoa/Changes.txt" {
		s.serveChanges(w, r)
		return
	}

	// Message of the day. The station UI requests this (the URL comes from
	// options.cfg inside station.pak) and we were answering 404 — five times
	// per visit to the server-select screen.
	if r.URL.Path == "/patch/web/eqoa/motd/news.html" {
		body := []byte("<html><body><p>Welcome to Spire.</p></body></html>")
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", itoa(len(body)))
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		log.Printf("Patch server: served news.html (%d bytes)", len(body))
		return
	}

	// World ESFs for the in-game HDD stager. Deliberately NOT part of the
	// manifest below: these go to pfs0: from inside the running game, and a
	// 6 MB world would not fit on the memory card the manifest writes to.
	if s.handleWorlds(w, r) {
		return
	}

	// Check if this is a patch file download
	for i := range s.files {
		if s.files[i].Delete {
			continue
		}
		if r.URL.Path == "/"+s.files[i].RemoteName {
			s.servePatchFile(w, r, &s.files[i])
			return
		}
	}

	// Handle other common endpoints
	switch r.URL.Path {
	case "/", "/patcher", "/patch", "/version":
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0"))

	default:
		log.Printf("Patch server 404: %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

// repointStationPak rewrites the two entries inside station.pak that name a
// server, so the STATION layer talks to THIS server.
//
// This is what caused "Server not ready, please try again later. (nodbs)" the
// moment a game server was picked: the pak we shipped had been taken from
// Sandstorm, and its stationLiveHosts.txt read "5.161.217.188:9735". That is a
// literal IP, so the PNACH's gethostbyname hook cannot redirect it the way it
// redirects the hostname in NewsURL — the client went straight to Sandstorm's
// auth host, got no database list back for the account, and errored out.
//
// Container format (big-endian, repeated until EOF):
//
//	uint32 nameLen | name[nameLen] | uint32 dataLen | data[dataLen]
//
// Entry sizes are rewritten, so replacements need not match the original
// length. Anything unparseable is returned untouched rather than half-rebuilt.
func (s *PatchServer) repointStationPak(data []byte) []byte {
	ip := s.cfg.Server.ServerIP
	replacements := map[string][]byte{
		"stationdata/servers/stationLiveHosts.txt": []byte(
			ip + ":" + itoa(s.cfg.Server.AuthPort) + "\r\n"),
		"stationdata/config/options.cfg": []byte(
			"NewsURL:http://" + ip + ":" + itoa(s.cfg.Server.PatchPort) +
				"/patch/web/eqoa/motd/news.html\r\n"),
	}

	var out bytes.Buffer
	var changed []string
	for off := 0; off+4 <= len(data); {
		nameLen := int(binary.BigEndian.Uint32(data[off:]))
		if nameLen <= 0 || nameLen > 512 || off+4+nameLen+4 > len(data) {
			log.Printf("station.pak: unparseable at offset %d, serving as-is", off)
			return data
		}
		name := string(bytes.TrimRight(data[off+4:off+4+nameLen], "\x00"))
		sizeOff := off + 4 + nameLen
		dataLen := int(binary.BigEndian.Uint32(data[sizeOff:]))
		bodyOff := sizeOff + 4
		if dataLen < 0 || bodyOff+dataLen > len(data) {
			log.Printf("station.pak: bad length for %q, serving as-is", name)
			return data
		}

		body := data[bodyOff : bodyOff+dataLen]
		if repl, ok := replacements[name]; ok {
			log.Printf("station.pak: %s: %q -> %q", name, body, repl)
			body = repl
			changed = append(changed, name)
		}

		// Name field is copied verbatim, padding included.
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(nameLen))
		out.Write(hdr[:])
		out.Write(data[off+4 : off+4+nameLen])
		binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
		out.Write(hdr[:])
		out.Write(body)

		off = bodyOff + dataLen
	}

	if len(changed) != len(replacements) {
		log.Printf("station.pak: WARNING only repointed %v — expected both entries; "+
			"the client may still contact another server", changed)
	}
	log.Printf("station.pak: repointed at %s (%d -> %d bytes)", ip, len(data), out.Len())
	return out.Bytes()
}

// deleteOrNone names what a pass carries, for the log. The last pass of a
// sequence deletes nothing and is the one that actually delivers.
func deleteOrNone(name string) string {
	if name == "" {
		return "no Delete, everything offered"
	}
	return "Delete " + name
}

// serveUpdateXML serves the EQOA update XML matching eqoa.live format.
func (s *PatchServer) serveUpdateXML(w http.ResponseWriter, r *http.Request) {
	serverIP := s.cfg.Server.ServerIP

	// WHICH PASS THIS CONSOLE IS ON. A console that has not yet been told to
	// drop a managed file gets one Delete for it — the whole set as usual, but
	// with that one file as a Delete and no download entry for it — and its
	// next fetch moves on to the next file. The last pass carries no Delete and
	// every download, so a console with nothing stale and a fresh card both
	// converge without the operator sequencing anything.
	//
	// ONE DELETE PER MANIFEST is the client's rule, and delete and add cannot
	// coexist for one LocalName (bumpProductVersion has both measurements), so
	// a pass must OMIT that file's entry rather than carry both, and two files
	// cannot be dropped in one pass however tempting.
	//
	// A DIAGNOSTIC FETCH MUST NOT COUNT AS A PASS. EQOADIAG asks for this exact
	// URL from this exact console to read the server's side of the card
	// comparison, and it cannot apply a Delete. Recording it as a pass left the
	// console holding its stale file for good — see cardRefresh.nextPass.
	host := requestHost(r)
	observer := isObserverFetch(r)
	pass, deleteName := s.cardSeq.nextPass(host, !observer)
	if observer {
		log.Printf("Patch server: %s fetched the manifest as a diagnostic — "+
			"reporting pass %d (%s) without sequencing it",
			host, pass+1, deleteOrNone(deleteName))
	}
	// Each pass carries its own Product Version: two passes sharing a number
	// means the second is gated out and the console is left with the file
	// deleted and never restored.
	version := s.productVersion + pass

	// Build File entries from patch files
	var fileEntries string
	for _, name := range s.refresh {
		fileEntries += `<File LocalName="` + name + `" Delete="True"></File>`
	}
	for _, f := range s.files {
		if deleteName != "" && f.LocalName == deleteName {
			fileEntries += `<File LocalName="` + deleteName + `" Delete="True"></File>`
			continue
		}
		if f.Delete {
			fileEntries += `<File LocalName="` + f.LocalName + `" Delete="True"></File>`
		} else {
			fileEntries += `<File LocalName="` + f.LocalName + `"` +
				` RemotePath=""` +
				` Server="` + serverIP + `"` +
				` Name="` + f.RemoteName + `"` +
				` CRC="` + uitoa(f.CRC) + `"` +
				` TotalSize="` + itoa(len(f.Content)) + `"` +
				` DownloadSize="` + itoa(len(f.GzipContent)) + `"` +
				`></File>`
		}
	}

	xml := `<VerantPatcher Version="1">` +
		`<Product Version="` + itoa(version) + `">` +
		`<Distribution Name="Spire" >` + fileEntries + `</Distribution>` +
		`<Changes Server="` + serverIP + `" Port="` + itoa(s.cfg.Server.PatchPort) +
		`" Version="` + itoa(version) + `" RemotePath="patch/eqoa/Changes.txt" CRC="` + uitoa(s.changesCRC) + `"></Changes>` +
		`</Product>` +
		`</VerantPatcher>`

	w.Header().Set("Connection", "close")

	if len(r.URL.Path) > 3 && r.URL.Path[len(r.URL.Path)-3:] == ".gz" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(xml))
		gz.Close()

		w.Header().Set("Content-Type", "application/x-gzip")
		w.Header().Set("Content-Length", itoa(buf.Len()))
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
		log.Printf("Patch server: served gzipped update XML (%d bytes)", buf.Len())
	} else {
		xmlBytes := []byte(xml)
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Content-Length", itoa(len(xmlBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(xmlBytes)
		log.Printf("Patch server: served update XML")
	}
}

// ConsoleMidSDRefresh reports whether the console at `host` is known to be
// between the two passes: told to delete one of the managed files and not yet
// seen downloading the replacement. The loader chain is inert while any link of
// it is missing, so anything that talks TO the payload — the Freeport
// technician's control strings — would be drawn on screen as unreadable glyphs
// instead of acted on.
//
// It asks about every MANAGED file rather than about "SD", which it named
// literally until SD stopped being served at all. The question was never really
// about that one file: it is "is the chain broken right now because we broke it
// on purpose", and a deleted SS breaks it exactly as thoroughly.
//
// False for a console we know nothing about. See cardRefresh.midRefresh for why
// the two ways of being wrong are not weighted equally.
func (s *PatchServer) ConsoleMidSDRefresh(host string) bool {
	if s.cardSeq == nil {
		return false
	}
	for _, name := range s.cardSeq.order {
		if s.cardSeq.midRefresh(host, name) {
			return true
		}
	}
	return false
}

// servePatchFile serves a gzipped patch file download.
func (s *PatchServer) servePatchFile(w http.ResponseWriter, r *http.Request, f *patchFile) {
	// A file GET is the only positive evidence the server ever gets that a
	// payload actually reached a card. The manifest only says what was offered.
	s.cardSeq.noteDownloaded(requestHost(r), f.LocalName)
	w.Header().Set("Content-Type", "application/x-gzip")
	w.Header().Set("Content-Length", itoa(len(f.GzipContent)))
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusOK)
	w.Write(f.GzipContent)
	log.Printf("Patch server: served %s (%d bytes gzipped)", f.RemoteName, len(f.GzipContent))
}

// serveChanges serves the Changes.txt changelog.
func (s *PatchServer) serveChanges(w http.ResponseWriter, r *http.Request) {
	changes := `Spire - EQOA Go Server
2026-02-14

Working:
- Auth, patch, server list, character select, world entry
- Character creation with SQLite persistence
- Character model, movement, power bar
- NPC loading (2400+), visibility system, state channels
- Chat (say + shout)
- Targeting (NPCs/players, con colors, faction)
- Basic combat (attack, damage, death/respawn)
- NPC AI (aggro, melee, leash, respawn)
- HP/Power regen
- Hotkey menu (L2 D-pad, 25 default hotkeys)
- Patch system with eqahosts.txt auto-discovery
`

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", itoa(len(changes)))
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(changes))
	log.Printf("Patch server: served Changes.txt")
}

// uitoa converts a uint32 to a string.
func uitoa(u uint32) string {
	if u == 0 {
		return "0"
	}
	var buf [10]byte
	pos := len(buf)
	for u > 0 {
		pos--
		buf[pos] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[pos:])
}

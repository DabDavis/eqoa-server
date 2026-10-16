package tcp

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// PER-CONSOLE SEQUENCING OF THE DELETE DANCE.
//
// Replacing a file the patcher has already written to the memory card takes two
// passes — see bumpProductVersion for the measurement. That was driven by hand
// with EQOA_PATCH_DELETE, and by hand it has a hole that no amount of care
// closes: a console whose FIRST contact happens after pass 2 was published
// never sees the Delete at all, and keeps its stale copy forever. The
// operator's log looks identical either way — a manifest GET with no file GET
// after it.
//
// CORRECTED 2026-10-16, from the decomp of SETUP's patcher: this was put down
// to the patcher gating the distribution on <Product Version>, which it never
// reads. The cause is the size rule below — without UseCRC the patcher judges a
// file by its length alone. The manifest now says UseCRC="True"
// (serveUpdateXML), so a same-size change is fetched without a Delete; the
// dance is still what replaces a file that SHRANK, whose old tail the card
// keeps. claude-notes/go-client-patcher-20261016.md.
//
// So the dance is sequenced here per requester instead of globally in time: a
// console that has not yet been served the Delete for a managed file's current
// content gets one, and its next manifest fetch moves it along. Whenever that
// fetch happens — a patcher-initiated restart, or simply the next boot — the
// console converges. Nothing depends on the operator having been running the
// right env var at the moment a given tester happened to connect.
//
// # WHICH FILES ARE MANAGED, AND WHY THESE
//
// A file belongs here when it carries a fact the patcher CANNOT SEE it holding.
// The patcher compares by SIZE, so any file whose content can change without
// changing its length is invisible to it, and the failure is silent at both
// ends.
//
//   - SD is our patch payload. A rebuild is very often the same length as the
//     one it replaces — the world table is a fixed-size record — so a new
//     payload reaches nobody without this.
//
//   - eqahosts.txt WAS managed here for the same reason, and is NOT any more.
//     It is the server's address and it was fifteen bytes for almost every
//     operator on earth — "5.161.217.188\n\n" and "192.168.40.87\n\n" are the
//     same length — so a card patched by a different server kept pointing there
//     forever while this operator's log showed a healthy fetch. Measured on
//     this project's own dev card 2026-08-13: it had held Sandstorm's address
//     for weeks.
//
//     But the cure was worse than the disease, which took a console to find
//     out. See the note on refreshOrder: the pass that deletes this file hangs
//     the game on the memory-card check until it is power-cycled. It is PADDED
//     now (eqahostsFile in patchserver.go) so its size carries its address and
//     the client's own rule replaces it. A file the client can see for itself
//     has no business in a mechanism whose every pass costs a boot.
//
// ONE DELETE PER MANIFEST. The client honours exactly one, so a console stale
// on both files is walked through them one pass at a time, in refreshOrder, and
// converges in len(managed)+1 fetches. That is the cost, and it is paid on
// first contact by every console including one with a blank card, because
// nothing here can see what a card holds. A Delete for a file the card does not
// have is a no-op that costs a pass.
//
// EVERY OTHER FILE IS SERVED NORMALLY IN EVERY PASS, so a patch that touches
// none of these costs nobody an extra boot.
//
// DELETING eqahosts.txt WAS BELIEVED SAFE, and the reasoning was sound as far
// as it went: it is the file that tells the GAME where to connect, so a console
// between passes cannot play, but it can still PATCH — the patcher reaches this
// server from the address baked into SLUS, not from this file, which is why a
// virgin card with no eqahosts.txt is provisioned in one pass
// (claude-notes/hdd-setup-npc.md, and the ten-file fresh-card run).
//
// What that argument does not cover is what the game DOES while the file is
// missing, and the answer is not "refuses to connect". It hangs, on the one
// screen that tells the player not to reset. "Cannot play for one pass" and
// "bricked until power-cycled" are different costs, and only one of them was
// ever measured. If a file has to be deleted again, measure that first.
//
// EQOA_REFRESH_FILES turns the whole mechanism off without a rebuild;
// EQOA_REFRESH_REARM puts one console back to the start of it.
type cardRefresh struct {
	mu    sync.Mutex
	path  string
	order []string // managed LocalNames, in the order a console is walked

	fp   map[string]uint32            // LocalName -> fingerprint being served
	seen map[string]map[string]uint32 // host -> LocalName -> fp it was told to delete
	got  map[string]map[string]uint32 // host -> LocalName -> fp it actually downloaded
	on   bool                         // false when nothing is managed, or the manual override is driving

	// newlyEngaged is set when this start is the first to sequence some file's
	// CURRENT content — a changed payload, or a server that has no record of
	// what the field is holding. bumpProductVersion treats it as a content
	// change, because a dance published under a version the consoles have
	// already applied is never fetched at all.
	newlyEngaged bool
}

// refreshOrder is the managed set, in the order a console is walked through it.
//
// eqahosts.txt IS NO LONGER IN IT, and the reason is a measurement rather than
// a preference. Deleting that file does not merely stop a console connecting
// for one pass, as the comment here used to assume: it leaves the game stuck
// FOREVER on "Checking memory card (PS2)… Do not remove memory card (PS2),
// reset, or switch off the console" — 37 pixel-identical screenshots, the
// payload never reached, and the only way out is the power cycle that screen
// explicitly forbids (sd-delivery-holes-20260814.md).
//
// It is out because it no longer needs to be in. The file is padded at build
// time so its SIZE carries its address (eqahostsFile in patchserver.go), and a
// size the client can see is a change the client makes for itself. That is a
// better answer than a Delete for any file it works for: no pass, no version
// stride, and nothing to sequence per console.
//
// SD IS NO LONGER IN IT EITHER, and for the opposite reason to eqahosts.txt: it
// is not delivered by this server at all any more. EQOAHDD.ELF installs it, and
// the tool has no need of a dance — it deletes and recreates unconditionally and
// then reads the file back, which is the thing the two passes were approximating.
//
// SS TOOK ITS PLACE. It inherits exactly the property that put SD here: it is
// fixed-shape MIPS at a fixed address, so a rebuild can change every byte and
// land on the same 1368, and padding a binary the loader walks is not the same
// safe trick as trailing newlines in a text file. Its failure between passes is
// survivable in the same way and better documented — a card with no SS boots
// unpatched, which is the intermediate state this dance was designed around.
//
// S2 is deliberately left unmanaged. It is a record container that grows with
// every feature added to it, so a content change that holds its length is
// possible rather than routine, and each managed file costs every console
// another pass. If one is ever needed: EQOA_REFRESH_FILES="SS,S2".
var refreshOrder = []string{"SS"}

// rearmFromEnv names hosts whose history is to be forgotten at startup, so the
// next manifest fetch from each walks the whole sequence again.
//
// THE SEQUENCING KEY IS A SOURCE IP, AND AN IP IS NOT A CONSOLE. Two consoles
// behind one address — a household with a PS2 and an emulator, a NAT, a
// development box running a diagnostic tool — share one record, so the first to
// arrive spends the passes and the second is told nothing and keeps its stale
// file. Measured here 2026-08-14: a card held another server's address for two
// days while this server's log showed a healthy, converged console
// (sd-delivery-holes-20260814.md).
//
// Nothing in a patcher request identifies a console: no cookie, no id, and the
// account is not known until long after patching is done. So this is not fixed,
// it is made RECOVERABLE — `EQOA_REFRESH_REARM=203.0.113.9` re-arms one tester
// without costing every other console a boot, which is what deleting
// patches/.sdstate does. `all` is that bigger hammer, kept because it is the
// documented recovery and should not need a different mechanism.
func rearmFromEnv() []string {
	v := strings.TrimSpace(os.Getenv("EQOA_REFRESH_REARM"))
	if v == "" {
		return nil
	}
	var out []string
	for _, h := range strings.Split(v, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// refreshFilesFromEnv is the operator's override, for turning this off or
// narrowing it without a rebuild: EQOA_REFRESH_FILES="SD" manages only the
// payload, and "none" (or "") manages nothing. Unknown names are kept and
// simply never match a served file, which is harmless — an unmanaged file is
// the old behaviour.
func refreshFilesFromEnv() []string {
	v, set := os.LookupEnv("EQOA_REFRESH_FILES")
	if !set {
		return refreshOrder
	}
	if v == "" || strings.EqualFold(v, "none") {
		log.Printf("Patch server: EQOA_REFRESH_FILES is empty — no file is " +
			"sequenced, so a card holding a same-size stale copy keeps it")
		return nil
	}
	var out []string
	for _, n := range strings.Split(v, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	log.Printf("Patch server: EQOA_REFRESH_FILES sequences %v", out)
	return out
}

// cardStateFile is the on-disk form. Keyed by host so that a console which
// reconnects from the same address is not made to dance twice.
type cardStateFile struct {
	Version int                       `json:"v"`
	Files   map[string]*cardFileState `json:"files"`
	// V1, when only the SD was managed. Read for the upgrade below and never
	// written: a state file that loses its history costs every console in the
	// field one wasted pass, silently.
	Fingerprint uint32            `json:"fingerprint,omitempty"`
	Seen        map[string]uint32 `json:"seen,omitempty"`
	Got         map[string]uint32 `json:"got,omitempty"`
}

type cardFileState struct {
	Fingerprint uint32            `json:"fp"`
	Seen        map[string]uint32 `json:"seen"`
	Got         map[string]uint32 `json:"got"`
}

// newCardRefresh loads the persisted state and decides what still needs
// sequencing.
//
// `fps` is the fingerprint of each managed file as it will actually be SERVED,
// and it must contain ONLY files this server really serves. A name that is not
// in it is not managed at all — deleting a file we do not serve would remove it
// from the card and never put it back.
//
// State is persisted so that restarting the server does not make every console
// re-dance. A missing or unreadable file is not an error: it means every
// console is treated as needing the whole sequence, which is the safe
// direction.
func newCardRefresh(path string, fps map[string]uint32, order []string, manualOverride bool) *cardRefresh {
	r := &cardRefresh{
		path: path,
		fp:   map[string]uint32{},
		seen: map[string]map[string]uint32{},
		got:  map[string]map[string]uint32{},
	}
	for _, name := range order {
		if fp, ok := fps[name]; ok && fp != 0 {
			r.order = append(r.order, name)
			r.fp[name] = fp
		}
	}

	var st cardStateFile
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Files == nil {
		st.Files = map[string]*cardFileState{}
	}
	// UPGRADE from the SD-only file. Its consoles have long since finished that
	// dance, and reading their absence as "nobody has been told" would send
	// every one of them round again for nothing.
	if st.Version < 2 && st.Fingerprint != 0 {
		up := &cardFileState{Fingerprint: st.Fingerprint, Seen: st.Seen, Got: st.Got}
		if up.Got == nil {
			// Older still: `got` did not exist. Absent is not empty — credit
			// the consoles we know about with what they were served, by the
			// rule in midRefresh.
			up.Got = map[string]uint32{}
			for host, fp := range up.Seen {
				up.Got[host] = fp
			}
		}
		// Named SD because that is what a v1 file's single history WAS. The
		// entry is inert now that SD is unmanaged — an unmanaged name never
		// matches a served file — but rewriting it to SS would be a lie about
		// which file those consoles were walked through.
		st.Files["SD"] = up
		log.Printf("Patch server: upgraded the refresh state file to v2, " +
			"carrying the SD history forward")
	}

	for name, fs := range st.Files {
		if fs == nil {
			continue
		}
		for host, fp := range fs.Seen {
			if r.seen[host] == nil {
				r.seen[host] = map[string]uint32{}
			}
			r.seen[host][name] = fp
		}
		for host, fp := range fs.Got {
			if r.got[host] == nil {
				r.got[host] = map[string]uint32{}
			}
			r.got[host][name] = fp
		}
	}

	switch {
	case len(r.order) == 0:
		// Nothing served that we manage; there is nothing to sequence.
		r.on = false
	case manualOverride:
		// EQOA_PATCH_DELETE is someone driving the dance by hand. Two drivers
		// on one wheel is worse than either alone, so this one lets go and says
		// so — the manual pass still works exactly as it always has.
		r.on = false
		log.Printf("Patch server: EQOA_PATCH_DELETE is set, so automatic " +
			"refresh sequencing is standing down for this run")
	default:
		r.on = true
		for _, host := range rearmFromEnv() {
			if strings.EqualFold(host, "all") {
				log.Printf("Patch server: EQOA_REFRESH_REARM=all — every console " +
					"will walk the whole sequence again, one boot each")
				r.seen = map[string]map[string]uint32{}
				r.got = map[string]map[string]uint32{}
				r.newlyEngaged = true
				break
			}
			if _, known := r.seen[host]; !known {
				if _, known = r.got[host]; !known {
					log.Printf("Patch server: EQOA_REFRESH_REARM names %s, which "+
						"has no history here — nothing to re-arm. Check the "+
						"address as this server SEES it, not the console's own",
						host)
					continue
				}
			}
			delete(r.seen, host)
			delete(r.got, host)
			r.newlyEngaged = true
			log.Printf("Patch server: re-armed %s — its next manifest fetch "+
				"walks the sequence from the start", host)
		}
		for _, name := range r.order {
			was := uint32(0)
			if fs := st.Files[name]; fs != nil {
				was = fs.Fingerprint
			}
			if was == r.fp[name] {
				continue
			}
			// The bytes we will serve differ from the bytes we last served.
			// Every console has to be told to drop what it has, including the
			// ones that were never here before — which is the whole point.
			r.newlyEngaged = true
			for host := range r.seen {
				delete(r.seen[host], name)
			}
			for host := range r.got {
				delete(r.got[host], name)
			}
			log.Printf("Patch server: %s changed (%08x -> %08x) — every console "+
				"will be served one Delete pass for it before the new one",
				name, was, r.fp[name])
		}
	}

	r.save()
	return r
}

// passes is how many manifest fetches one console needs to converge: one per
// managed file, plus the final one that carries every download. It is also the
// Product Version stride, because each pass must carry its own number.
func (r *cardRefresh) passes() int {
	if r == nil || !r.on {
		return 2 // the historical stride; a manual pass 1 and its pass 2
	}
	return len(r.order) + 1
}

// nextPass reports which pass this fetch is for `host` and which file, if any,
// it should carry a Delete for. An empty name means "no Delete: serve
// everything", which is the last pass of the sequence.
//
// `record` false answers the same question and moves nothing. That matters
// because the manifest is fetched by two different programs and only one of
// them can act on the answer: EQOADIAG fetches the identical URL from the
// identical console address to READ the server's side of the comparison.
// Served as a real pass, that diagnostic fetch consumed one: the server
// recorded the console as told, nothing on the card was deleted, and the
// console's next real fetch moved on to the following file. The card kept the
// old copy permanently, and the tool that caused it reported the result as the
// fault.
//
// Note the asymmetry that makes the observer path safe: a request wrongly
// treated as an observer costs one extra pass, while a real fetch wrongly
// recorded costs a console its file until that file's content next changes.
//
// Recording happens on the way OUT, because the server cannot observe the card:
// the next fetch from this console is taken as evidence the Delete was applied.
// A console that fetched and then died before applying it is therefore recorded
// as done and keeps its stale copy until the content changes again. That is the
// one case this gets wrong, and the alternative — never advancing without proof
// — would loop a healthy console forever.
func (r *cardRefresh) nextPass(host string, record bool) (int, string) {
	if r == nil || !r.on || host == "" {
		return 0, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	pass := 0
	for _, name := range r.order {
		if r.seen[host][name] == r.fp[name] {
			pass++
			continue
		}
		if record {
			if r.seen[host] == nil {
				r.seen[host] = map[string]uint32{}
			}
			r.seen[host][name] = r.fp[name]
			log.Printf("Patch server: %s has not been served the current %s "+
				"(%08x) — pass %d, Delete %s; its next manifest fetch moves on",
				host, name, r.fp[name], pass+1, name)
			r.saveLocked()
		}
		return pass, name
	}
	return pass, ""
}

// noteDownloaded records that this console actually fetched a managed file.
//
// This is EVIDENCE rather than inference: the manifest says what was offered,
// the file GET says what was taken. Only the second one distinguishes a console
// that has the payload from one that was merely told about it — which is the
// whole distinction the Freeport technician needs before it speaks to a payload
// that may not be there.
func (r *cardRefresh) noteDownloaded(host, name string) {
	if r == nil || host == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, managed := r.fp[name]; !managed {
		return
	}
	if r.got[host][name] == r.fp[name] {
		return
	}
	if r.got[host] == nil {
		r.got[host] = map[string]uint32{}
	}
	r.got[host][name] = r.fp[name]
	// Said out loud because it is the only moment an operator can point at and
	// say "that console now has it". A manifest fetch proves nothing — the
	// patcher makes one whether or not anything is stale — and the whole class
	// of bug here is invisible convergence. It also names the HOST, which is
	// the sequencing key and its weakness: see rearmFromEnv.
	log.Printf("Patch server: %s downloaded %s (%08x) — converged", host, name, r.fp[name])
	r.saveLocked()
}

// midRefresh reports whether this console is KNOWN to be without ONE NAMED
// file — told to delete it and not yet observed downloading the replacement.
//
// PER FILE, not "anywhere in the sequence". The caller asks about the SD
// because what it guards is a conversation with the SD payload, and a console
// mid-flight on eqahosts.txt may be holding a perfectly good payload. Answering
// the broader question here would tell that player to go and reboot for
// nothing, which is the error this whole function is shaped to avoid.
//
// Deliberately narrow in the other direction too. It answers "do we know this
// console is missing the file", not "do we believe it is fine", because the two
// ways of being wrong are not weighted equally: wrongly assuming it is fine
// costs only the silence that was already the old behaviour. A console that has
// never contacted the patch server is therefore NOT mid-refresh.
func (r *cardRefresh) midRefresh(host, name string) bool {
	if r == nil || !r.on || host == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[host][name] == r.fp[name] && r.got[host][name] != r.fp[name]
}

func (r *cardRefresh) save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveLocked()
}

// saveLocked persists the table. Caller holds r.mu.
//
// A write failure is logged and otherwise ignored: losing the file costs each
// console one extra sequence after a restart, which is a wasted boot rather
// than a wrong result, and refusing to serve patches over it would be worse.
func (r *cardRefresh) saveLocked() {
	if r.path == "" {
		return
	}
	st := cardStateFile{Version: 2, Files: map[string]*cardFileState{}}
	for _, name := range r.order {
		fs := &cardFileState{
			Fingerprint: r.fp[name],
			Seen:        map[string]uint32{},
			Got:         map[string]uint32{},
		}
		for host, per := range r.seen {
			if fp, ok := per[name]; ok {
				fs.Seen[host] = fp
			}
		}
		for host, per := range r.got {
			if fp, ok := per[name]; ok {
				fs.Got[host] = fp
			}
		}
		st.Files[name] = fs
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.WriteFile(r.path, b, 0o644); err != nil {
		log.Printf("Patch server: could not persist refresh state: %v", err)
	}
}

// isObserverFetch reports whether this manifest request came from something that
// only READS the manifest — EQOADIAG today — rather than from the patcher, which
// acts on it.
//
// The marker is a request header the patcher does not send. It has to be
// explicit: the diagnostic speaks the patcher's exact request, from the
// patcher's address, in the same boot, because that is the point of it — the
// answer it wants is the one the patcher would get. Nothing about the connection
// distinguishes them. A query parameter is accepted too, for anything that can
// only vary a URL.
//
// An old EQOADIAG in the field sends neither and is still served as a patcher.
// That is not fixable from this end; it is fixed by the operator handing out the
// rebuilt tool, and it is why deleting patches/.sdstate re-arms the sequence.
func isObserverFetch(r *http.Request) bool {
	if r.Header.Get("X-EQOA-Diag") != "" {
		return true
	}
	return r.URL.Query().Get("diag") != ""
}

// requestHost is the requester's address without its port. The port changes on
// every HTTP connection, so only the host can identify a console across passes.
func requestHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

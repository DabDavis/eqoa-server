// Package worlds resolves world ESF files for the in-game HDD stager.
//
// The stager reaches them two ways — HTTP (internal/network/tcp) and UDP
// (internal/network/udp) — and both take a filename straight off the wire. The
// name check therefore lives here rather than in either transport: two
// definitions of "safe filename" is one more than can be kept correct.
package worlds

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDir is used when the config leaves worlds_dir empty.
const DefaultDir = "worlds"

// Dir resolves the configured directory the way the patch dir resolves: the
// working directory first, then beside the executable, so running from either
// place finds the same files.
func Dir(configured string) string {
	dir := configured
	if dir == "" {
		dir = DefaultDir
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if exe, err := os.Executable(); err == nil {
			return filepath.Join(filepath.Dir(exe), dir)
		}
	}
	return dir
}

// Name is the ESF filename a world slot loads, and the ONLY spelling that
// works: the client's table entry for slot N is baked as "hdd:worldN.ESF" by
// make_hdd_sd.py, so an ESF has to be called this to be reachable at all.
func Name(slot int) string {
	return fmt.Sprintf("world%d.ESF", slot)
}

// Available reports which of slots [lo, count) this server actually holds an
// ESF for, as slot -> filename.
//
// A slot is not a world. Every slot from lo upward NAMES a file whether or not
// one exists, because the client's table is baked once and the whole point of
// the convention is that adding a world needs no rebuild. So the set of slots
// that can work is a directory listing, not a constant, and it is worth saying
// out loud at startup — a world that is simply not there otherwise presents as
// a client that fails to open it, three layers away from the missing file.
//
// This is necessary AND, for a console running the on-demand SD, close enough
// to sufficient: the payload fetches whatever "hdd:" world the client opens,
// once per world per boot, so a slot listed here is one a player can enter
// whether or not their hard disk has ever held it. It was not always so — the
// fetch used to be aimed at a single slot at SD build time, and a world listed
// here was still unreachable on a console that had never downloaded it.
func Available(configured string, lo, count int) map[int]string {
	dir := Dir(configured)
	out := make(map[int]string)
	for slot := lo; slot < count; slot++ {
		name := Name(slot)
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() {
			out[slot] = name
		}
	}
	return out
}

// SafeName reports whether a client-supplied name is a bare filename we are
// willing to open. This is the only place such a string reaches the filesystem,
// so it allows a fixed character set rather than trying to spot traversal.
func SafeName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	// ".." passes the character set above; the leading-dot rule kills it and
	// every other dotfile with it.
	return name[0] != '.'
}

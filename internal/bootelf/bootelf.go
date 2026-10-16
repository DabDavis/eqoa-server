// Package bootelf retargets EQOABOOT.ELF, the loader a tester runs to play the
// RETAIL disc against this server.
//
// WHY THE SERVER DOES THIS AND NOT THE BUILD
//
// Two of the three programs a tester is handed are the same bytes for every
// operator: EQOAHDD.ELF and EQOADIAG.ELF both read the server's address off the
// memory card at run time, out of eqahosts.txt, so one build is correct
// everywhere and the distribution can simply carry them.
//
// EQOABOOT cannot. It runs BEFORE the game does — that is its whole job, to
// read SLUS_207.44 off the untouched disc and patch it in RAM — and the address
// it patches in is the LOGIN address, which the retail disc hardcodes and no
// card file redirects. So it is correct for exactly one server, and a
// prebuilt copy in a distribution would aim every operator's testers at
// whichever machine happened to build the archive: right filename, right size,
// somebody else's network, and nothing on the console says so.
//
// That is not a hypothetical. It is the same failure the SD payload had until
// 2026-08-13 (claude-notes/sd-address-from-eqahosts-20260813.md) — a baked
// address that was invisible on the console and uncorrectable afterwards,
// because the patcher compares by SIZE and a repoint does not change the size.
// The fix there was to stop baking it. Here it cannot be un-baked, so the next
// best thing is to bake it from the one value that is already right: the
// server's own `server_ip`, the same field that renders eqahosts.txt, repoints
// station.pak and fills the patch manifest. An operator who runs this server
// cannot hand out a loader that disagrees with it.
//
// WHAT IS REWRITTEN
//
// Two words of the loader's SLUS patch table, which is an array of
// {addr, orig, val} little-endian u32 triples:
//
//	lui $v1, <ip hi>      0x3C03____
//	ori $v1, $v1, <ip lo> 0x3463____
//
// found by SHAPE rather than by address. The address is 0x00124B18 today and is
// carried in eqoa-boot's generated patches.h, which a distribution does not
// ship; a second copy of the constant here would be a second thing to keep in
// step, and the pair is already unmistakable — two consecutive triples building
// a 32-bit constant into $v1. Exactly one match is required, so a table this
// does not understand is refused rather than guessed at.
//
// AND THE CRC, WHICH IS THE PART THAT IS EASY TO MISS
//
// PCSX2 identifies a title by XOR-ing every 32-bit word of the boot ELF, and
// when it is given an ELF override it uses THAT file's CRC rather than the
// disc's. The loader is deliberately tuned so it hashes to the retail disc's
// EEEE1FCC — that is the entire reason it exists, so every EEEE1FCC.pnach keeps
// applying. Changing two words changes the XOR, so an untuned retarget would
// silently take every cheat and debug patch away from the operator who ran it.
// The trailing word crc_tune.py appended past the last program header is
// adjusted by the same delta, which restores the file's CRC exactly — no search
// and no target constant, because the CRC to keep is the one the file arrived
// with.
package bootelf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const (
	opLUIv1 = 0x3C030000 // lui $v1, imm
	opORIv1 = 0x34630000 // ori $v1, $v1, imm
	opMask  = 0xFFFF0000

	tripleLen = 12 // {addr, orig, val}
)

// ErrNoAddress is returned when the loader holds no login-address pair in the
// shape this understands, or holds more than one.
var ErrNoAddress = errors.New("no unique login-address pair in this loader")

// site is where the two `val` words live in the file.
type site struct {
	hiOff, loOff int // byte offsets of the two val words
	slusAddr     uint32
}

// find locates the one lui/ori pair that builds the login address.
//
// The scan is over 4-byte alignments rather than over parsed triples: the table
// is inside a stripped ELF with no symbol to anchor on, and the pattern is
// specific enough that finding it twice means something is wrong, which is
// reported rather than resolved by preferring one.
func find(elf []byte) (site, error) {
	var hits []site
	for i := 0; i+2*tripleLen <= len(elf); i += 4 {
		a1 := binary.LittleEndian.Uint32(elf[i:])
		v1 := binary.LittleEndian.Uint32(elf[i+8:])
		a2 := binary.LittleEndian.Uint32(elf[i+12:])
		v2 := binary.LittleEndian.Uint32(elf[i+20:])
		if a2 == a1+4 && v1&opMask == opLUIv1 && v2&opMask == opORIv1 {
			hits = append(hits, site{hiOff: i + 8, loOff: i + 20, slusAddr: a1})
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return site{}, fmt.Errorf("%w: no lui/ori $v1 pair at consecutive "+
			"patch addresses — this is not an EQOABOOT loader, or its patch "+
			"table has changed shape", ErrNoAddress)
	default:
		return site{}, fmt.Errorf("%w: %d candidate pairs (at SLUS %#08x and "+
			"others) — refusing to choose between them",
			ErrNoAddress, len(hits), hits[0].slusAddr)
	}
}

// crc is PCSX2's ElfObject::GetCRC: XOR of every whole 32-bit word.
func crc(b []byte) uint32 {
	var v uint32
	for i := 0; i+4 <= len(b); i += 4 {
		v ^= binary.LittleEndian.Uint32(b[i:])
	}
	return v
}

// Address reports the login address a built loader currently aims at.
func Address(elf []byte) (net.IP, error) {
	s, err := find(elf)
	if err != nil {
		return nil, err
	}
	hi := binary.LittleEndian.Uint32(elf[s.hiOff:]) & 0xFFFF
	lo := binary.LittleEndian.Uint32(elf[s.loOff:]) & 0xFFFF
	v := hi<<16 | lo
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)), nil
}

// Retarget returns a copy of a built EQOABOOT.ELF aimed at ip, with the file's
// CRC preserved. The input is not modified.
func Retarget(elf []byte, ip string) ([]byte, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("server_ip %q is not an IP address", ip)
	}
	v4 := parsed.To4()
	if v4 == nil {
		return nil, fmt.Errorf("server_ip %q is not IPv4; the login address the "+
			"retail disc hardcodes is a 32-bit constant and has nowhere to put "+
			"an IPv6 address", ip)
	}

	out := make([]byte, len(elf))
	copy(out, elf)
	s, err := find(out)
	if err != nil {
		return nil, err
	}
	// The CRC to keep is the one it came with. Reading it here rather than
	// naming EEEE1FCC means a loader tuned to something else stays tuned to
	// that, and there is no constant to fall out of step with eqoa-boot.
	want := crc(out)

	addr := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	binary.LittleEndian.PutUint32(out[s.hiOff:], opLUIv1|addr>>16)
	binary.LittleEndian.PutUint32(out[s.loOff:], opORIv1|addr&0xFFFF)

	// Restore the XOR with the trailing word crc_tune.py appended. It sits past
	// every program header, so the PS2's own loader never reads it — only
	// PCSX2's hash does.
	if len(out) < 4 {
		return nil, errors.New("loader is too short to carry a CRC word")
	}
	tail := len(out) - 4
	if tail%4 != 0 {
		return nil, fmt.Errorf("loader length %d is not a whole number of "+
			"words, so it carries no tunable CRC word", len(out))
	}
	binary.LittleEndian.PutUint32(out[tail:],
		binary.LittleEndian.Uint32(out[tail:])^crc(out)^want)

	if got := crc(out); got != want {
		return nil, fmt.Errorf("retarget left the CRC at %08X, wanted %08X", got, want)
	}
	return out, nil
}

// Stage writes dir/EQOABOOT.ELF from dir/EQOABOOT.template.ELF, aimed at ip.
//
// NO players/ AT ALL IS SILENT; players/ WITHOUT A TEMPLATE IS NOT.
//
// The first is a server built from source: there is no folder, nothing is
// expected, and saying so every boot would be noise. The second is an unpacked
// archive started from the wrong working directory — the same trap that makes
// the server miss eqoa.db and patches/ — and it fails far more quietly than
// those do, because the operator does not find out until a tester is holding
// either nothing or a loader left over from a previous server_ip. So it is
// reported, and it is reported as the working directory being wrong rather than
// as a missing file, because that is the thing to go and fix.
func Stage(dir, ip string) (string, error) {
	tmpl := filepath.Join(dir, "EQOABOOT.template.ELF")
	raw, err := os.ReadFile(tmpl)
	if os.IsNotExist(err) {
		if _, statErr := os.Stat(dir); statErr == nil {
			return "", fmt.Errorf("%s exists but holds no EQOABOOT.template.ELF"+
				" — if this is an unpacked archive, the server was started from"+
				" the wrong directory", dir)
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	out, err := Retarget(raw, ip)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tmpl, err)
	}
	dst := filepath.Join(dir, "EQOABOOT.ELF")
	// Written whole and replaced, never edited in place: a tester downloading
	// this while it is half-written gets a loader that fails its own CRC check
	// on somebody's console rather than here.
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

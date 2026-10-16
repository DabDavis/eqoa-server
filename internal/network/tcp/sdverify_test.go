package tcp

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// sdBlob builds an SDP1 container around one 20-byte world-server record, in
// the shape re-tools/sd_patch.py writes: header, records, a 0,0 terminator.
// Written from the FORMAT rather than by copying a payload, so the walk is
// tested against the description it claims to implement.
func sdBlob(srv []byte) []byte {
	out := make([]byte, 0, 64)
	hdr := make([]byte, 16)
	copy(hdr, "SDP1")
	binary.LittleEndian.PutUint32(hdr[4:], 1)
	binary.LittleEndian.PutUint32(hdr[8:], 0x3000)
	out = append(out, hdr...)
	if srv != nil {
		rec := make([]byte, 8)
		binary.LittleEndian.PutUint32(rec, 0x8F000000|0x1A40) // an arena address
		binary.LittleEndian.PutUint32(rec[4:], uint32(len(srv)))
		out = append(out, rec...)
		out = append(out, srv...)
	}
	return append(out, make([]byte, 8)...) // terminator
}

// srvRecord is a CURRENT record: word 0 zero because the IOP reads it, the
// address filled at runtime, the port baked, the magic past the block.
func srvRecord(ip, port uint32) []byte {
	b := make([]byte, sdSrvLen)
	binary.LittleEndian.PutUint32(b[4:], ip)
	binary.LittleEndian.PutUint32(b[16:], port)
	binary.LittleEndian.PutUint32(b[sdSrvMagicOff:], sdSrvMagic)
	return b
}

// srvRecordLegacy is the pre-eqahosts shape: no magic, an address baked in.
func srvRecordLegacy(ip, port uint32) []byte {
	b := make([]byte, sdSrvLegacy)
	binary.LittleEndian.PutUint32(b[4:], ip)
	binary.LittleEndian.PutUint32(b[16:], port)
	return b
}

// srvRecordWord0Magic is the BROKEN 2026-08-13 shape: the magic in word 0, the
// first word of the address struct sceInetSendTo ships to the IOP.
func srvRecordWord0Magic(port uint32) []byte {
	b := make([]byte, sdSrvLegacy)
	binary.LittleEndian.PutUint32(b, sdSrvMagic)
	binary.LittleEndian.PutUint32(b[16:], port)
	return b
}

// A CURRENT PAYLOAD carries the magic, no address, and a port. The zero IP is
// the point: the address is read off the card at runtime, so a matcher keyed on
// "a nonzero IP surrounded by zeros" — which is how the old repointer found
// this record — would now match nothing at all.
func TestSDVerifyFindsTheRecordByMagic(t *testing.T) {
	rec, legacy, ok := findSDWorldServer(sdBlob(srvRecord(0, 7003)))
	if !ok || rec == nil {
		t.Fatalf("the world-server record was not found (ok=%v rec=%v)", ok, rec)
	}
	if legacy != 0 {
		t.Errorf("legacyIP = %08x, want 0 — this payload bakes no address", legacy)
	}
	if got := binary.LittleEndian.Uint32(rec[16:]); got != 7003 {
		t.Errorf("port = %d, want 7003", got)
	}
	if got := binary.LittleEndian.Uint32(rec); got != 0 {
		t.Errorf("word 0 = %08x, want 0 — it is the IOP's, not ours", got)
	}
}

// WORD 0 BELONGS TO sceInetSendTo. The record is passed to it BY ADDRESS and
// the wrapper ships the first 16 bytes to the IOP as the destination address;
// the game's own send passes {0, ip, 0, 0}, read live off its call. A magic
// there — which is what shipped between 2026-08-13 and 08-14 — makes every send
// return short with nothing on the wire, so the world download and the Freeport
// technician's reply both go silent with no error recorded anywhere. Such a
// payload must be NAMED, not read as "no downloader" and not served quietly.
func TestSDVerifyRecognisesTheWord0MagicBuild(t *testing.T) {
	rec, _, ok := findSDWorldServer(sdBlob(srvRecordWord0Magic(7003)))
	if !ok || rec == nil {
		t.Fatalf("the broken 08-13 shape was not found (ok=%v rec=%v)", ok, rec)
	}
	if got := binary.LittleEndian.Uint32(rec); got != sdSrvMagic {
		t.Errorf("word 0 = %08x, want the magic — this fixture IS the broken build", got)
	}
}

// SERVING AN OLD PAYLOAD IS THE ONE REGRESSION THIS CHECK EXISTS FOR. Nothing
// rewrites the address any more, so a pre-eqahosts SD would send every console
// it reaches to whichever LAN it was built on — silently, because a console
// that downloads no world looks exactly like a console with none to download.
// It must not read as "this payload has no downloader".
func TestSDVerifyRecognisesAPreEqahostsPayload(t *testing.T) {
	const built = 0xC0A82857 // 192.168.40.87, the LAN every old SD was built on
	rec, legacy, ok := findSDWorldServer(sdBlob(srvRecordLegacy(built, 7003)))
	if !ok || rec == nil {
		t.Fatalf("the old record shape was not found (ok=%v rec=%v)", ok, rec)
	}
	if legacy != built {
		t.Errorf("legacyIP = %08x, want %08x", legacy, built)
	}
	if got := sdDottedQuad(legacy); got != "192.168.40.87" {
		t.Errorf("rendered %q, want 192.168.40.87", got)
	}
}

// A payload with no downloader is a legitimate build (SD-hdd-mc0), not an
// error: that console reaches only the worlds its disk already holds.
func TestSDVerifyAcceptsAPayloadWithNoDownloader(t *testing.T) {
	rec, legacy, ok := findSDWorldServer(sdBlob(nil))
	if !ok {
		t.Fatal("a payload with no downloader failed the walk")
	}
	if rec != nil || legacy != 0 {
		t.Errorf("found a world-server record in a payload that has none")
	}
}

// THE SERVED PAYLOAD IS THE CURRENT ONE. This is the test that fails if an
// older SD is dropped back into patches/, which is otherwise invisible: the
// manifest, the CRC and the size all look perfectly healthy.
func TestServedSDCarriesNoBakedAddress(t *testing.T) {
	s := newTestServer(t)
	sd := find(s, "SD")
	if sd == nil {
		t.Skip("no SD in the served set")
	}
	rec, legacy, ok := findSDWorldServer(sd.Content)
	if !ok {
		t.Fatal("the served SD does not walk as a payload")
	}
	if rec == nil {
		t.Fatal("the served SD has no world-download record, so no console " +
			"patched from here can fetch a world")
	}
	if legacy != 0 {
		t.Fatalf("the served SD bakes the address %s. Nothing repoints it any "+
			"more — rebuild it with make_hdd_sd.py --world-port",
			sdDottedQuad(legacy))
	}
	if got := binary.LittleEndian.Uint32(rec[4:]); got != 0 {
		t.Errorf("the served SD's address word is %08x, want 0", got)
	}
	// The gate that would have caught the 08-13 build before it reached a
	// console. Nothing about a payload with a nonzero word 0 looks wrong — it
	// walks, it carries a port, it is the right size — and every send it makes
	// fails silently.
	if got := binary.LittleEndian.Uint32(rec); got != 0 {
		t.Errorf("the served SD has %08x in word 0 of the world-download record. "+
			"sceInetSendTo ships those 16 bytes to the IOP as the destination "+
			"address, so this payload cannot send at all", got)
	}
	if len(rec) != sdSrvLen {
		t.Errorf("the served SD's record is %d bytes, want %d — an older shape",
			len(rec), sdSrvLen)
	}
}

// THE SERVED PAYLOAD OWNS EVERYTHING IT NAMES.
//
// The SD may address Sandstorm's S2 arena — the format has a whole relocation
// section for it, so a hook of ours can chain into one of their cave routines.
// But an S2 reference is a DEPENDENCY, and it fails by omission: with S2 absent
// the loader stub SKIPS such a record rather than misapplying it, so the
// payload comes up quietly missing a piece and nothing anywhere says which.
//
// The world table did exactly that until 2026-08-13. Seven of the eight disc
// worlds borrowed their name and path from strings in S2's cave, so without S2
// they came up nameless. The strings are 204 bytes; they are ours now.
//
// Checked here as well as in stubverify because this is the copy that SHIPS —
// patches/ is gitignored, and a payload restored from the attic would pass
// every other test in this file.
func TestServedSDDependsOnNoS2Arena(t *testing.T) {
	s := newTestServer(t)
	sd := find(s, "SD")
	if sd == nil {
		t.Skip("no SD in the served set")
	}
	relocs, records := sdS2References(t, sd.Content)
	if relocs != 0 || records != 0 {
		t.Errorf("the served SD names S2's arena in %d relocations and %d "+
			"records. Those pieces are silently dropped on any console whose "+
			"card has no S2 — copy what they point at into our own arena, as "+
			"the world table does with S2's strings", relocs, records)
	}
}

// sdS2References counts the payload's dependencies on Sandstorm's arena: RLC1
// relocation entries, and records addressed into the S2 window. Walked from the
// format (re-tools/sd_patch.py) rather than from a payload, so it can disagree
// with what the builder happens to emit.
func sdS2References(t *testing.T, data []byte) (relocs, records int) {
	t.Helper()
	const (
		s2WinLo = 0x000F8000
		s2WinSz = 0x00008000
		rlc1    = uint32(0x31434C52) // "RLC1"
		rlc2    = uint32(0x32434C52) // "RLC2"
	)
	if len(data) < sdHeaderLen || string(data[:4]) != sdMagic {
		t.Fatal("the served SD is not an SDP1 payload")
	}

	off := sdHeaderLen
	for off+8 <= len(data) {
		addr := binary.LittleEndian.Uint32(data[off:])
		size := int(binary.LittleEndian.Uint32(data[off+4:]))
		if addr == 0 && size == 0 {
			off += 8
			break
		}
		if off+8+size > len(data) {
			t.Fatalf("SD record at %d claims %d bytes, past the end", off, size)
		}
		if a := addr &^ 0x80000000; addr&0x80000000 != 0 &&
			a >= s2WinLo && a < s2WinLo+s2WinSz {
			records++
		}
		off += 8 + size + (4-size%4)%4
	}
	for off != len(data) {
		if off+8 > len(data) {
			t.Fatalf("%d bytes of junk after the SD's records", len(data)-off)
		}
		magic := binary.LittleEndian.Uint32(data[off:])
		count := int(binary.LittleEndian.Uint32(data[off+4:]))
		if magic != rlc1 && magic != rlc2 {
			t.Fatalf("unknown SD section %08x at %d", magic, off)
		}
		if magic == rlc1 {
			relocs += count
		}
		off += 8 + 4*count
		if off > len(data) {
			t.Fatal("an SD relocation section overruns the file")
		}
	}
	return relocs, records
}

// THE PAYLOAD THIS SERVER ACTUALLY SERVES, parsed the way the console's stub
// parses it.
//
// Added when defaultSDFile moved from the assembly build to the C one. Every
// other test in this file feeds sdverify a hand-built fixture, which checks the
// verifier and says nothing at all about the file going out of the door. The
// stub is strict in a way that matters here: its second pass requires the
// relocation sections to consume the file EXACTLY, and it bails on trailing
// junk -- so a payload with a malformed tail applies NOTHING, silently, and the
// symptom is a console with no world streaming and no error anywhere.
func TestServedSDParsesAsTheStubWouldParseIt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "patches", defaultSDFile))
	if err != nil {
		t.Skipf("patches/%s not present: %v", defaultSDFile, err)
	}
	const hdrSize = 16
	if len(data) < hdrSize+8 {
		t.Fatalf("%s is %d bytes, too short to be an SD", defaultSDFile, len(data))
	}
	magic := binary.LittleEndian.Uint32(data[0:])
	if magic != 0x31504453 { // 'SDP1'
		t.Fatalf("%s magic is %#08x, want SDP1", defaultSDFile, magic)
	}
	arena := binary.LittleEndian.Uint32(data[8:])
	if arena < 0x1000 || arena > 0x100000 || arena%4 != 0 {
		t.Fatalf("%s arena size %#x is outside what the stub reserves", defaultSDFile, arena)
	}

	// Records, to the 0,0 terminator.
	off, records := hdrSize, 0
	for {
		if off+8 > len(data) {
			t.Fatalf("records run off the end of %s", defaultSDFile)
		}
		addr := binary.LittleEndian.Uint32(data[off:])
		size := binary.LittleEndian.Uint32(data[off+4:])
		off += 8
		if addr == 0 && size == 0 {
			break
		}
		if int(size) > len(data)-off {
			t.Fatalf("a record body runs off the end of %s", defaultSDFile)
		}
		records++
		off += int(size)
		off = (off + 3) &^ 3
	}
	if records == 0 {
		t.Fatalf("%s has no records — it would apply nothing", defaultSDFile)
	}

	// The relocation sections, which MUST consume the file exactly. This is the
	// check stub.S makes and the one a shrinking in-place overwrite fails.
	for off != len(data) {
		if off+8 > len(data) {
			t.Fatalf("trailing junk after the records in %s — the stub would "+
				"reject the whole payload and apply nothing", defaultSDFile)
		}
		sm := binary.LittleEndian.Uint32(data[off:])
		count := binary.LittleEndian.Uint32(data[off+4:])
		if sm != 0x31434C52 && sm != 0x32434C52 { // RLC1 / RLC2
			t.Fatalf("unknown section %#08x in %s", sm, defaultSDFile)
		}
		end := off + 8 + 4*int(count)
		if end > len(data) {
			t.Fatalf("a relocation section overruns %s", defaultSDFile)
		}
		// RLC1 is a dependency on Sandstorm's arena, and it fails by OMISSION:
		// with no S2 the stub skips the record rather than misapplying it, so
		// the payload comes up quietly missing a piece.
		if sm == 0x31434C52 && count != 0 {
			t.Errorf("%s carries %d RLC1 relocations — the payload must name "+
				"S2's arena nowhere", defaultSDFile, count)
		}
		off = end
	}
	t.Logf("%s: %d bytes, %d records, arena %#x — consumes exactly",
		defaultSDFile, len(data), records, arena)
}

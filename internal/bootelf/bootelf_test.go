package bootelf

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// A minimal stand-in for the loader: some filler, one patch table holding the
// lui/ori pair, and a trailing CRC word. Built rather than read from disk so the
// tests run in a checkout that has no PS2 toolchain and no eqoa-boot/out.
func synth(ip [4]byte) []byte {
	b := make([]byte, 0, 128)
	put := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }

	for i := 0; i < 8; i++ { // filler that must not look like the pair
		put(0x11111111)
	}
	addr := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	put(0x001016B8)
	put(0x27BDFFF0)
	put(0x08055C18) // an ordinary triple ahead of it
	put(0x00124B18)
	put(0x0C049260)
	put(opLUIv1 | addr>>16)
	put(0x00124B1C)
	put(0x24060005)
	put(opORIv1 | addr&0xFFFF)
	put(0x00124B20)
	put(0x04400012)
	put(0xAE430004)
	for i := 0; i < 4; i++ {
		put(0x22222222)
	}
	put(0) // the trailing word crc_tune.py appends
	// Tune it, the way the build does.
	binary.LittleEndian.PutUint32(b[len(b)-4:], crc(b)^0xEEEE1FCC)
	return b
}

func TestRetargetChangesTheAddressAndKeepsTheCRC(t *testing.T) {
	orig := synth([4]byte{192, 168, 40, 87})
	if got := crc(orig); got != 0xEEEE1FCC {
		t.Fatalf("fixture CRC %08X, wanted EEEE1FCC", got)
	}
	ip, err := Address(orig)
	if err != nil || ip.String() != "192.168.40.87" {
		t.Fatalf("Address = %v, %v; wanted 192.168.40.87", ip, err)
	}

	out, err := Retarget(orig, "5.161.217.188")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Address(out)
	if err != nil || got.String() != "5.161.217.188" {
		t.Fatalf("after retarget Address = %v, %v", got, err)
	}
	// THE POINT OF THE WHOLE FILE: PCSX2 keys cheats off this number, so a
	// retarget that moved it would silently take every EEEE1FCC.pnach away.
	if c := crc(out); c != 0xEEEE1FCC {
		t.Errorf("retarget moved the CRC to %08X; every EEEE1FCC.pnach would "+
			"stop applying and nothing on the console would say why", c)
	}
	if a, _ := Address(orig); a.String() != "192.168.40.87" {
		t.Error("Retarget modified its input")
	}
}

func TestRetargetIsIdempotentAndReversible(t *testing.T) {
	a := synth([4]byte{192, 168, 40, 87})
	b, err := Retarget(a, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Retarget(b, "192.168.40.87")
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(c) {
		t.Error("there and back again is not the file we started with")
	}
}

func TestRefusesWhatItCannotUnderstand(t *testing.T) {
	good := synth([4]byte{1, 2, 3, 4})

	if _, err := Retarget(good, "not-an-ip"); err == nil {
		t.Error("accepted a non-address")
	}
	if _, err := Retarget(good, "2001:db8::1"); err == nil {
		t.Error("accepted IPv6, which has nowhere to go in a 32-bit constant")
	}
	if _, err := Retarget(make([]byte, 64), "10.0.0.1"); err == nil {
		t.Error("accepted a file with no address pair in it")
	}

	// Two pairs must be refused, not resolved by preferring the first: a file
	// this does not understand is one where the wrong half might be the half
	// that gets written, and that failure only shows up on a console.
	two := append(append([]byte{}, good...), good...)
	if _, err := Retarget(two, "10.0.0.1"); err == nil {
		t.Error("chose between two candidate pairs instead of refusing")
	}
}

func TestStageWritesOnlyWhenThereIsATemplate(t *testing.T) {
	dir := t.TempDir()

	// No players/ at all is not an error — a source build has none.
	if p, err := Stage(filepath.Join(dir, "absent"), "10.0.0.1"); err != nil || p != "" {
		t.Fatalf("Stage with no players/ = %q, %v; wanted no work and no error", p, err)
	}

	// But a players/ WITHOUT a template is the wrong-working-directory trap,
	// and it must not pass quietly: the operator finds out via a tester holding
	// a loader left over from a previous server_ip.
	if _, err := Stage(dir, "10.0.0.1"); err == nil {
		t.Error("an empty players/ was accepted silently")
	}

	if err := os.WriteFile(filepath.Join(dir, "EQOABOOT.template.ELF"),
		synth([4]byte{192, 168, 40, 87}), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Stage(dir, "203.0.113.9")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(dir, "EQOABOOT.ELF") {
		t.Fatalf("staged %q", p)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if ip, _ := Address(out); ip.String() != "203.0.113.9" {
		t.Errorf("staged loader aims at %v, not the server's own address", ip)
	}
	if _, err := os.Stat(filepath.Join(dir, "EQOABOOT.ELF.new")); !os.IsNotExist(err) {
		t.Error("left the temporary file behind")
	}
}

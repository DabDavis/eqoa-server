package bootelf

import (
	"os"
	"testing"
)

// The real loaders. Skipped where they are not built, so this stays a
// developer check rather than something a distribution has to satisfy.
func TestAgainstTheRealLoaders(t *testing.T) {
	for path, want := range map[string]string{
		"../../../eqoa-boot/out/EQOABOOT.ELF":  "192.168.40.87",
		"../../../eqoa-boot/dist/EQOAECHO.ELF": "192.168.1.191",
		"../../../eqoa-boot/dist/EQOASAND.ELF": "5.161.217.188",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Logf("skip %s: %v", path, err)
			continue
		}
		ip, err := Address(raw)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if ip.String() != want {
			t.Errorf("%s aims at %v, expected %s", path, ip, want)
		}
		before := crc(raw)
		out, err := Retarget(raw, "203.0.113.9")
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		got, _ := Address(out)
		if got.String() != "203.0.113.9" {
			t.Errorf("%s retargeted to %v", path, got)
		}
		if crc(out) != before {
			t.Errorf("%s: CRC moved %08X -> %08X", path, before, crc(out))
		}
		back, err := Retarget(out, want)
		if err != nil || string(back) != string(raw) {
			t.Errorf("%s: round trip did not reproduce the original", path)
		}
		t.Logf("%-24s %v  crc %08X  round trip exact", path, ip, before)
	}
}

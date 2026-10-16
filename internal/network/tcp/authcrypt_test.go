package tcp

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Two Login Request password fields (opcode 0x24, tcp/9735) for the same
// password, shaped like the pair a real PS2 sends on two runs: the first 16
// bytes identical, the last 16 different. CBC means the first two blocks cover
// the password and its NUL terminator, while the remaining blocks cover
// whatever was left in the client's stack buffer, and on a console that
// differs from login to login (two captures of one account showed exactly
// this). That difference is the point of the test -- a decoder that returned
// the whole 32 bytes, or that keyed off them, would "work" on one login and
// fail on the other. The vectors are synthetic (encryptBuffer with two stack
// tails), so no real account's field is published here.
var capturedLogins = []struct {
	name       string // which login
	ciphertext string
	want       string
}{
	{"first login", "a23e7824cef3902d80dcd7cb7afdae99055784b03b05e3dffdf3eb762e0e8ea0", "tester"},
	{"second login", "a23e7824cef3902d80dcd7cb7afdae99149b0fc8c878a23d3d6cf89f92948480", "tester"},
}

func TestDecryptPasswordAgainstRealCaptures(t *testing.T) {
	for _, tc := range capturedLogins {
		t.Run(tc.name, func(t *testing.T) {
			field, err := hex.DecodeString(tc.ciphertext)
			if err != nil {
				t.Fatalf("bad test vector: %v", err)
			}
			got, err := decryptPassword(field)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("recovered %q, want %q", got, tc.want)
			}
		})
	}
}

// The two captures must agree despite differing tails. If this ever fails while
// the test above passes, the trailing garbage has leaked into the credential.
func TestTrailingGarbageDoesNotAffectPassword(t *testing.T) {
	a, _ := hex.DecodeString(capturedLogins[0].ciphertext)
	b, _ := hex.DecodeString(capturedLogins[1].ciphertext)

	pwA, err := decryptPassword(a)
	if err != nil {
		t.Fatalf("capture A: %v", err)
	}
	pwB, err := decryptPassword(b)
	if err != nil {
		t.Fatalf("capture B: %v", err)
	}
	if pwA != pwB {
		t.Fatalf("same account, same password, different captures: %q vs %q", pwA, pwB)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	for _, pw := range []string{"a", "dab", "Pass:word9", "0123456789012345678901234567890"} {
		field, err := encryptPassword(pw)
		if err != nil {
			t.Fatalf("encrypt %q: %v", pw, err)
		}
		if len(field) != passwordFieldLen {
			t.Fatalf("encrypt %q produced %d bytes, want %d", pw, len(field), passwordFieldLen)
		}
		got, err := decryptPassword(field)
		if err != nil {
			t.Fatalf("decrypt %q: %v", pw, err)
		}
		if got != pw {
			t.Fatalf("round trip: got %q, want %q", got, pw)
		}
	}
}

// The PS2's garbage does not start at a block boundary -- it starts at the byte
// after the NUL. The real capture's first plaintext block is
// "dab\x00" + b0c81400, so a zero-filling client cannot produce byte-identical
// ciphertext and there is no point asserting that it does.
//
// What must hold is that the garbage is irrelevant to the recovered password,
// wherever it starts. This reproduces the PS2's buffer shape directly: same
// password, arbitrary trailing bytes, one answer.
func TestGarbageAfterTheNULIsIgnoredWhereverItStarts(t *testing.T) {
	const password = "dab"

	tails := [][]byte{
		{},                             // fully zero-filled
		{0xb0, 0xc8, 0x14, 0x00},       // the real capture's, mid-block
		{0xff},                         // one byte, immediately after the NUL
		bytes.Repeat([]byte{0xAB}, 27), // fills the rest of the buffer
		{0x00, 0x00, 0xde, 0xad, 0xbe, 0xef, 0x11}, // NULs then garbage
	}

	for i, tail := range tails {
		buf := make([]byte, passwordFieldLen)
		copy(buf, password)
		// buf[len(password)] stays NUL; the tail follows it.
		copy(buf[len(password)+1:], tail)

		field, err := encryptBuffer(buf)
		if err != nil {
			t.Fatalf("tail %d: encrypt: %v", i, err)
		}
		got, err := decryptPassword(field)
		if err != nil {
			t.Fatalf("tail %d: decrypt: %v", i, err)
		}
		if got != password {
			t.Fatalf("tail %d changed the password: got %q, want %q", i, got, password)
		}
	}
}

func TestDecryptPasswordRejectsGarbage(t *testing.T) {
	t.Run("wrong length", func(t *testing.T) {
		if _, err := decryptPassword(make([]byte, 16)); err == nil {
			t.Fatal("accepted a 16-byte field")
		}
	})

	// A field that was not encrypted with our key decrypts to binary noise.
	// Passing that to bcrypt would silently compare garbage, so it must fail.
	t.Run("not our ciphertext", func(t *testing.T) {
		junk := make([]byte, passwordFieldLen)
		for i := range junk {
			junk[i] = byte(i * 7)
		}
		if pw, err := decryptPassword(junk); err == nil {
			t.Fatalf("accepted non-ciphertext as password %q", pw)
		}
	})
}

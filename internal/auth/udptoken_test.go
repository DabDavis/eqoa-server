package auth

import (
	"encoding/hex"
	"testing"
)

// A token reads back as the password it was made from: the vector is a made-up
// password zero-filled to 32 bytes and encrypted by openssl (aes-256-ecb,
// -nopad) under the key, not by this package. The controls: a token one byte
// short, and one that decrypts to nothing (encrypted zeros), carry no password.
func TestPasswordFromUDPToken(t *testing.T) {
	tok, _ := hex.DecodeString("aff1cac9dfbc7b1799fd85409063a16cb71c12ae76c498fdf3ceeb444a0a49b5")
	if pw, ok := PasswordFromUDPToken(tok); !ok || pw != "made-up pass" {
		t.Fatalf("the token reads %q (%v), want %q", pw, ok, "made-up pass")
	}
	if _, ok := PasswordFromUDPToken(tok[:31]); ok {
		t.Error("a token of 31 bytes read as a password")
	}
	zeros, _ := hex.DecodeString("b71c12ae76c498fdf3ceeb444a0a49b5b71c12ae76c498fdf3ceeb444a0a49b5")
	if pw, ok := PasswordFromUDPToken(zeros); ok {
		t.Errorf("an empty password's token read as %q", pw)
	}
	tok[0] ^= 1
	if pw, _ := PasswordFromUDPToken(tok); pw == "made-up pass" {
		t.Error("a changed token still read as the password")
	}
}

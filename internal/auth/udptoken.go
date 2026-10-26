package auth

import (
	"bytes"
	"crypto/aes"
)

// udpTokenKey is the key STATION encrypts the UDP handshake's token under
// (EE 0x00353B10, claude-notes/auth-server-protocol.md): a protocol constant,
// as the TCP login's DES key is (tcp/authcrypt.go).
var udpTokenKey = [32]byte{
	0xec, 0xca, 0xbf, 0x71, 0xf4, 0xbf, 0x32, 0x6b, 0x57, 0x29, 0x72, 0x4d, 0xc1, 0xd7, 0xaa, 0xac,
	0x53, 0x84, 0x37, 0xda, 0x68, 0xe4, 0x3b, 0xb4, 0xa5, 0xe4, 0xa4, 0xe9, 0xd4, 0x89, 0x6d, 0x44,
}

// UDPTokenSize is the token's length: two AES blocks.
const UDPTokenSize = 32

// PasswordFromUDPToken reads the password the UDP handshake's token carries
// (Authenticate 0x0904 and Authenticate2 0x0001 end with it): the password
// zero-filled to 32 bytes and AES-256-ECB encrypted under udpTokenKey, as a
// PS2's token in a capture is. It is the password up to its first zero byte;
// false for a token of another length or one that carries none.
func PasswordFromUDPToken(tok []byte) (string, bool) {
	if len(tok) != UDPTokenSize {
		return "", false
	}
	c, err := aes.NewCipher(udpTokenKey[:])
	if err != nil {
		return "", false
	}
	plain := make([]byte, UDPTokenSize)
	for i := 0; i < UDPTokenSize; i += aes.BlockSize {
		c.Decrypt(plain[i:i+aes.BlockSize], tok[i:i+aes.BlockSize])
	}
	if n := bytes.IndexByte(plain, 0); n >= 0 {
		plain = plain[:n]
	}
	if len(plain) == 0 {
		return "", false
	}
	return string(plain), true
}

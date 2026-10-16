package tcp

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"encoding/hex"
	"errors"
)

// The password field of a Login Request is DES-CBC ciphertext, not a hash. The
// client encrypts the plaintext password into a 32-byte buffer, so the server
// can recover the password and verify it — which is what makes real password
// checking possible here at all.
//
// Key and IV are the same 8 bytes. They are a fixed constant of the protocol,
// derived once by the client from the string "LPSS-V1.0$@Z>.ADJ" via OpenSSL's
// des_string_to_key (XOR-fold to 8 bytes with DES odd parity, then a DES-CBC
// checksum of that same string under the folded key). The derivation is
// deterministic and takes no input from the session, so the result is hardcoded
// rather than recomputed on every login. See claude-notes/auth-server-protocol.md.
//
// Verified against real PS2 captures: decrypting the ciphertext in
// pcaps/breath_capture.pcap and pcaps/eqoa_live_capture.pcap with this key
// yields the account's password in both. See authcrypt_test.go.
const desKeyHex = "dfc2046b4983fd6b"

// passwordFieldLen is the size of the encrypted password field: 4 DES blocks.
const passwordFieldLen = 32

var errBadPasswordField = errors.New("malformed encrypted password field")

// decryptPassword recovers the plaintext password from the 32-byte DES-CBC
// field at Login Request payload[40:72].
//
// Only the bytes up to the first NUL are the password. Everything after it is
// whatever happened to be in the client's stack buffer — on real hardware that
// is live EE memory (pointers, prior packet contents), and it differs between
// two logins of the SAME account with the SAME password. It must never be
// treated as part of the credential.
func decryptPassword(field []byte) (string, error) {
	if len(field) != passwordFieldLen {
		return "", errBadPasswordField
	}

	key, err := hex.DecodeString(desKeyHex)
	if err != nil {
		return "", err
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return "", err
	}

	plain := make([]byte, len(field))
	cipher.NewCBCDecrypter(block, key).CryptBlocks(plain, field)

	if i := bytes.IndexByte(plain, 0); i >= 0 {
		plain = plain[:i]
	}

	// A password that fills the whole buffer with no terminator, or that
	// decrypts to binary, means the field was not encrypted with our key --
	// treat it as a failed login rather than passing garbage to bcrypt.
	for _, c := range plain {
		if c < 0x20 || c > 0x7E {
			return "", errBadPasswordField
		}
	}
	return string(plain), nil
}

// encryptPassword is the client side of the same transform. It exists so tests
// can build a Login Request the way the PS2 does, and so cmd/testlogin can act
// as a real client rather than sending a plaintext password the server would
// now reject.
//
// The buffer is zero-filled past the terminator. The PS2 instead leaves
// whatever was on its stack, so this does NOT reproduce a capture byte for byte
// — see TestGarbageAfterTheNULIsIgnoredWhereverItStarts.
func encryptPassword(password string) ([]byte, error) {
	if len(password) >= passwordFieldLen {
		return nil, errors.New("password too long for the 32-byte field")
	}
	buf := make([]byte, passwordFieldLen)
	copy(buf, password)
	return encryptBuffer(buf)
}

// encryptBuffer encrypts an already-prepared 32-byte password buffer, letting
// tests control the bytes after the terminator the way the console does.
func encryptBuffer(buf []byte) ([]byte, error) {
	if len(buf) != passwordFieldLen {
		return nil, errBadPasswordField
	}

	key, err := hex.DecodeString(desKeyHex)
	if err != nil {
		return nil, err
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}

	out := make([]byte, passwordFieldLen)
	cipher.NewCBCEncrypter(block, key).CryptBlocks(out, buf)
	return out, nil
}

// EncryptPasswordField exposes the client-side transform for tools that need to
// speak to the auth server, such as cmd/testlogin. Nothing in the server uses
// it: the server only ever decrypts.
func EncryptPasswordField(password string) ([]byte, error) {
	return encryptPassword(password)
}

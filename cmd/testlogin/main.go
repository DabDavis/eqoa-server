// Command testlogin performs a real Login Request against the auth server.
//
// It exists to answer "does this username and password actually get in?"
// without a PS2 in the loop. It builds the same 148-byte packet the console
// does, including the DES-CBC encrypted password field — sending the password
// in the clear, as this tool used to, is now correctly refused.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/eqoa/server/internal/network/tcp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9735", "auth server address")
	username := flag.String("user", "dab", "account name")
	password := flag.String("pass", "", "account password")
	plaintext := flag.Bool("plaintext", false, "send the password unencrypted (expected to FAIL; proves the server checks)")
	flag.Parse()

	if err := run(*addr, *username, *password, *plaintext); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(addr, username, password string, plaintext bool) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// The server opens with a 40-byte Server Hello. It must be read before the
	// login is sent, exactly as the console does.
	hello := make([]byte, 40)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := readFull(conn, hello); err != nil {
		return fmt.Errorf("read server hello: %w", err)
	}
	if op := binary.BigEndian.Uint32(hello[4:8]); op != 0x04 {
		return fmt.Errorf("expected Server Hello (0x04), got opcode 0x%02X", op)
	}

	packet, err := buildLoginRequest(username, password, plaintext)
	if err != nil {
		return err
	}

	fmt.Printf("login as %q (password %s)\n", username,
		map[bool]string{true: "PLAINTEXT — expected to fail", false: "DES-CBC encrypted"}[plaintext])

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(packet); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	response := make([]byte, 168)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := readFull(conn, response); err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	if op := binary.BigEndian.Uint32(response[4:8]); op != 0x25 {
		return fmt.Errorf("expected Login Response (0x25), got opcode 0x%02X", op)
	}

	errFlag := binary.BigEndian.Uint32(response[8:12])
	accountID := binary.BigEndian.Uint32(response[80:84])
	status := binary.BigEndian.Uint32(response[84:88])

	fmt.Printf("  error flag: %d\n  account ID: %d\n  status:     %d\n", errFlag, accountID, status)

	switch {
	case errFlag == 0 && status == 3:
		fmt.Println("RESULT: ACCEPTED")
		return nil
	case errFlag == 1 && status == 4:
		fmt.Println("RESULT: REJECTED (bad password or unknown account)")
		os.Exit(2)
	default:
		fmt.Printf("RESULT: UNRECOGNISED (error flag %d, status %d)\n", errFlag, status)
		os.Exit(3)
	}
	return nil
}

// buildLoginRequest assembles the 148-byte Login Request.
//
//	[0:4]    length, self-inclusive (148)
//	[4:8]    opcode 0x24
//	[8:12]   unknown (0)
//	[12:16]  version (2)
//	[16:48]  username, NUL-padded
//	[48:80]  password, DES-CBC encrypted
//	[80:84]  unknown (1)
//	[84:88]  "EQOA"
//	[88:148] padding
func buildLoginRequest(username, password string, plaintext bool) ([]byte, error) {
	packet := make([]byte, 148)

	// Self-inclusive. This previously said 144, which the server rejects as a
	// length mismatch before it looks at anything else.
	binary.BigEndian.PutUint32(packet[0:4], 148)
	binary.BigEndian.PutUint32(packet[4:8], 0x24)
	binary.BigEndian.PutUint32(packet[12:16], 2)
	copy(packet[16:48], username)

	if plaintext {
		copy(packet[48:80], password)
	} else {
		field, err := tcp.EncryptPasswordField(password)
		if err != nil {
			return nil, fmt.Errorf("encrypt password: %w", err)
		}
		copy(packet[48:80], field)
	}

	binary.BigEndian.PutUint32(packet[80:84], 1)
	copy(packet[84:88], "EQOA")
	return packet, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

package udp

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func worldUDPServer(t *testing.T, files map[string][]byte) *WorldServer {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return &WorldServer{dir: dir}
}

func request(name string, offset, length uint32) []byte {
	b := make([]byte, ReqSize)
	copy(b[0:4], reqMagic[:])
	binary.LittleEndian.PutUint32(b[4:8], offset)
	binary.LittleEndian.PutUint32(b[8:12], length)
	copy(b[12:], name)
	return b
}

func parseReply(t *testing.T, b []byte) (magic string, offset, length, total uint32, payload []byte) {
	t.Helper()
	if len(b) < ReplyHeader {
		t.Fatalf("reply is %d bytes, shorter than the %d-byte header", len(b), ReplyHeader)
	}
	return string(b[0:4]),
		binary.LittleEndian.Uint32(b[4:8]),
		binary.LittleEndian.Uint32(b[8:12]),
		binary.LittleEndian.Uint32(b[12:16]),
		b[ReplyHeader:]
}

// The reply header is a contract with hand-written MIPS: a fixed 16 bytes, then
// exactly `length` payload bytes, all little-endian because that is what the
// EE's `sw` writes.
func TestWorldUDPChunkShape(t *testing.T) {
	body := make([]byte, 5000)
	for i := range body {
		body[i] = byte(i)
	}
	s := worldUDPServer(t, map[string][]byte{"T127.ESF": body})

	magic, off, n, total, payload := parseReply(t, s.Handle(request("T127.ESF", 1024, 512)))
	if magic != "EQWD" {
		t.Errorf("magic = %q, want EQWD", magic)
	}
	if off != 1024 || n != 512 || total != 5000 {
		t.Errorf("header = off %d len %d total %d, want 1024/512/5000", off, n, total)
	}
	if len(payload) != 512 {
		t.Fatalf("payload is %d bytes, want 512", len(payload))
	}
	if string(payload) != string(body[1024:1536]) {
		t.Error("payload is not the file's bytes at that offset")
	}
}

// The stager sends length 0 before it decides to download anything: it is how
// it learns the finished size to compare against what is on pfs0:. Without it
// the size would have to be baked into the SD at build time, where it can drift
// from the file the server actually holds.
func TestWorldUDPSizeProbe(t *testing.T) {
	s := worldUDPServer(t, map[string][]byte{"W.ESF": make([]byte, 6192608)})

	magic, off, n, total, payload := parseReply(t, s.Handle(request("W.ESF", 0, 0)))
	if magic != "EQWD" || off != 0 || n != 0 || total != 6192608 || len(payload) != 0 {
		t.Errorf("probe = %q off %d len %d total %d payload %d", magic, off, n, total, len(payload))
	}
}

// A short reply is the ONLY end-of-file signal the stager has, so a read that
// runs off the end must return the tail, and a read at or past the end must
// return nothing while still saying EQWD.
func TestWorldUDPTailAndEOF(t *testing.T) {
	s := worldUDPServer(t, map[string][]byte{"W.ESF": []byte("0123456789")})

	_, _, n, _, payload := parseReply(t, s.Handle(request("W.ESF", 8, 512)))
	if n != 2 || string(payload) != "89" {
		t.Errorf("tail = len %d %q, want 2 \"89\"", n, payload)
	}

	magic, _, n, total, payload := parseReply(t, s.Handle(request("W.ESF", 10, 512)))
	if magic != "EQWD" || n != 0 || total != 10 || len(payload) != 0 {
		t.Errorf("EOF = %q len %d total %d payload %d", magic, n, total, len(payload))
	}
}

// Oversize is refused rather than clamped, and refusal has its own magic. A
// clamped reply would be read as EOF and finish the download early, leaving a
// truncated world; an EQWD with no payload and an EQWE mean opposite things.
func TestWorldUDPOversizeRefusedNotClamped(t *testing.T) {
	s := worldUDPServer(t, map[string][]byte{"W.ESF": make([]byte, 1<<20)})

	magic, off, _, _, payload := parseReply(t, s.Handle(request("W.ESF", 0, MaxChunk+1)))
	if magic != "EQWE" {
		t.Errorf("oversize = %q, want EQWE", magic)
	}
	if off != 0 || len(payload) != 0 {
		t.Errorf("error reply carried off %d and %d payload bytes", off, len(payload))
	}

	if magic, _, n, _, _ := parseReply(t, s.Handle(request("W.ESF", 0, MaxChunk))); magic != "EQWD" || n != MaxChunk {
		t.Errorf("MaxChunk itself = %q len %d, want EQWD %d", magic, n, MaxChunk)
	}
}

// The name is the only client-controlled string that reaches the filesystem.
func TestWorldUDPNameTraversalRefused(t *testing.T) {
	s := worldUDPServer(t, map[string][]byte{"W.ESF": []byte("x")})
	secret := filepath.Join(filepath.Dir(s.dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"../secret.txt", "..", ".hidden", "sub/W.ESF", "", "/etc/passwd"} {
		magic, _, _, _, _ := parseReply(t, s.Handle(request(bad, 0, 16)))
		if magic != "EQWE" {
			t.Errorf("name %q -> %q, want EQWE", bad, magic)
		}
	}
}

// This port is reachable from the internet, and a reply to a spoofed source
// address is a reflection amplifier. Only a well-formed, exactly-sized request
// gets any reply at all.
func TestWorldUDPGarbageGetsNoReply(t *testing.T) {
	s := worldUDPServer(t, map[string][]byte{"W.ESF": []byte("x")})

	short := request("W.ESF", 0, 16)[:ReqSize-1]
	long := append(request("W.ESF", 0, 16), 0)
	wrongMagic := request("W.ESF", 0, 16)
	wrongMagic[0] = 'X'

	for name, req := range map[string][]byte{
		"short": short, "long": long, "wrong magic": wrongMagic, "empty": {},
	} {
		if reply := s.Handle(req); reply != nil {
			t.Errorf("%s got a %d-byte reply, want silence", name, len(reply))
		}
	}
}

// End to end over a real socket, because Handle being right does not prove the
// datagram framing is: one request in, one reply out, no length prefix and no
// coalescing.
func TestWorldUDPOverASocket(t *testing.T) {
	dir := t.TempDir()
	body := make([]byte, 4096)
	for i := range body {
		body[i] = byte(i * 7)
	}
	if err := os.WriteFile(filepath.Join(dir, "W.ESF"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &WorldServer{dir: dir, port: 0}
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	c, err := net.DialUDP("udp4", nil, s.conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if _, err := c.Write(request("W.ESF", 2048, 1024)); err != nil {
		t.Fatalf("write: %v", err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	magic, off, length, total, payload := parseReply(t, buf[:n])
	if magic != "EQWD" || off != 2048 || length != 1024 || total != 4096 {
		t.Fatalf("header = %q %d %d %d", magic, off, length, total)
	}
	if string(payload) != string(body[2048:3072]) {
		t.Error("payload differs from the file")
	}
}

// Start on port 0 leaves the caller no way to find the socket, so the wiring in
// main.go must pass a real port; this just pins that Start reports a bind
// failure instead of logging and carrying on.
func TestWorldUDPStartReportsBindFailure(t *testing.T) {
	a := &WorldServer{dir: t.TempDir(), port: 0}
	if err := a.Start(); err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer a.Stop()

	port := a.conn.LocalAddr().(*net.UDPAddr).Port
	b := &WorldServer{dir: t.TempDir(), port: port}
	if err := b.Start(); err == nil {
		b.Stop()
		t.Fatalf("second bind on :%d succeeded, want an error", port)
	}
}

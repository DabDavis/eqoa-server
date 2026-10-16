package tcp

import (
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/config"
)

// worldTestServer builds a PatchServer whose worlds dir holds the given files.
func worldTestServer(t *testing.T, files map[string][]byte) *PatchServer {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	cfg := config.DefaultConfig()
	cfg.Patch.WorldsDir = dir
	return &PatchServer{cfg: cfg}
}

func worldGet(t *testing.T, s *PatchServer, path, rangeHdr string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if rangeHdr != "" {
		req.Header.Set("Range", rangeHdr)
	}
	rec := httptest.NewRecorder()
	if !s.handleWorlds(rec, req) {
		t.Fatalf("handleWorlds did not claim %s", path)
	}
	return rec.Result()
}

// The index is what the stager reads to decide whether to download at all, so
// its shape is a contract with hand-written MIPS: "NAME SIZE CRC32\n", the CRC
// as 8 lowercase hex digits.
func TestWorldIndexShape(t *testing.T) {
	body := []byte("not a real world, but a real length")
	s := worldTestServer(t, map[string][]byte{"T127.ESF": body})

	resp := worldGet(t, s, "/worlds/", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := recorderBody(t, resp)
	want := fmt.Sprintf("T127.ESF %d %08x\n", len(body), crc32.ChecksumIEEE(body))
	if got != want {
		t.Errorf("index = %q, want %q", got, want)
	}
}

// An absent worlds dir means "no downloadable worlds", which the stager must be
// able to tell apart from a broken server. A 404 or a connection error would
// present to it as the latter.
func TestWorldIndexMissingDirIsEmptyNot404(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Patch.WorldsDir = filepath.Join(t.TempDir(), "nope")
	s := &PatchServer{cfg: cfg}

	resp := worldGet(t, s, "/worlds/", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if b := recorderBody(t, resp); b != "" {
		t.Errorf("body = %q, want empty", b)
	}
}

// The EE side reads Content-Length and then that many bytes. Chunked encoding
// or a compressed body would both be undetectable to it until the file on the
// hard disk turned out to be wrong.
func TestWorldFileIsLengthedNeverChunkedNeverGzipped(t *testing.T) {
	body := make([]byte, 5000)
	for i := range body {
		body[i] = byte(i)
	}
	s := worldTestServer(t, map[string][]byte{"T127.ESF": body})

	req := httptest.NewRequest(http.MethodGet, "/worlds/T127.ESF", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	s.handleWorlds(rec, req)
	resp := rec.Result()

	if got := resp.Header.Get("Content-Length"); got != "5000" {
		t.Errorf("Content-Length = %q, want 5000", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
	if got := resp.Header.Get("Transfer-Encoding"); got != "" {
		t.Errorf("Transfer-Encoding = %q, want none", got)
	}
	if got := resp.Header.Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want close", got)
	}
	if got := recorderBody(t, resp); got != string(body) {
		t.Errorf("body differs (%d bytes vs %d)", len(got), len(body))
	}
}

// Resume is the reason this route exists rather than a plain file server: a
// console that drops at 5 MB of 6 must not start over.
func TestWorldRangeResume(t *testing.T) {
	body := []byte("0123456789")
	s := worldTestServer(t, map[string][]byte{"W.ESF": body})

	resp := worldGet(t, s, "/worlds/W.ESF", "bytes=4-")
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 4-9/10" {
		t.Errorf("Content-Range = %q", got)
	}
	if got := recorderBody(t, resp); got != "456789" {
		t.Errorf("body = %q, want 456789", got)
	}

	resp = worldGet(t, s, "/worlds/W.ESF", "bytes=2-4")
	if got := recorderBody(t, resp); got != "234" {
		t.Errorf("closed range body = %q, want 234", got)
	}
}

// A range the stager would never send is refused rather than approximated. If a
// malformed resume were answered with the whole file at status 200, the stager
// would write it at its resume offset and produce a corrupt world that still
// had the right length.
func TestWorldBadRangeIsRefusedNotApproximated(t *testing.T) {
	s := worldTestServer(t, map[string][]byte{"W.ESF": []byte("0123456789")})

	for _, bad := range []string{"bytes=0-1,4-5", "bytes=-500", "items=0-1", "bytes=99-", "bytes=5-2"} {
		resp := worldGet(t, s, "/worlds/W.ESF", bad)
		if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("Range %q -> %d, want 416", bad, resp.StatusCode)
		}
	}
}

// The name is the only client-controlled string that reaches the filesystem.
func TestWorldNameTraversalRefused(t *testing.T) {
	s := worldTestServer(t, map[string][]byte{"W.ESF": []byte("x")})
	secret := filepath.Join(filepath.Dir(s.worldsDir()), "secret.txt")
	if err := os.WriteFile(secret, []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"..%2fsecret.txt", "..", "../secret.txt", ".hidden", "sub/W.ESF"} {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+bad, nil)
		rec := httptest.NewRecorder()
		if !s.handleWorlds(rec, req) {
			continue // not claimed at all is also a refusal
		}
		if code := rec.Result().StatusCode; code == http.StatusOK || code == http.StatusPartialContent {
			t.Errorf("name %q was served (%d)", bad, code)
		}
	}
}

// Worlds must never be offered to the patcher: the memory card cannot hold one,
// and a manifest entry for a 6 MB file would wedge the card rather than fail.
func TestWorldsAreNotInTheManifest(t *testing.T) {
	s := worldTestServer(t, map[string][]byte{"T127.ESF": []byte("x")})
	s.cfg.Patch.Dir = t.TempDir()
	s.buildPatchFiles()

	for _, f := range s.files {
		if strings.Contains(strings.ToUpper(f.LocalName), ".ESF") {
			t.Errorf("manifest offers a world to the memory card: %s", f.LocalName)
		}
	}
}

func recorderBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

// The chunk route is what the EE actually uses: the game's own HTTP client
// builds its request from a fixed format string and cannot send a Range header,
// so the offset has to live in the URL.
func TestWorldChunk(t *testing.T) {
	s := worldTestServer(t, map[string][]byte{"W.ESF": []byte("0123456789")})

	resp := worldGet(t, s, "/worlds/W.ESF/4/3", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := recorderBody(t, resp); got != "456" {
		t.Errorf("body = %q, want 456", got)
	}

	// A tail read short of the asked length is honest and terminal.
	resp = worldGet(t, s, "/worlds/W.ESF/8/512", "")
	if got := recorderBody(t, resp); got != "89" {
		t.Errorf("tail = %q, want 89", got)
	}
	// Reading exactly at EOF is the loop terminator: 200, empty.
	resp = worldGet(t, s, "/worlds/W.ESF/10/512", "")
	if resp.StatusCode != http.StatusOK || recorderBody(t, resp) != "" {
		t.Errorf("EOF read = %d %q, want 200 and empty", resp.StatusCode, recorderBody(t, resp))
	}
}

// A chunk bigger than the buffer the EE allocated before asking would be an
// overwrite in the guest. Refuse it — clamping would return a short body, which
// the stager reads as EOF and writes a truncated world of plausible length.
func TestWorldChunkOversizeRefusedNotClamped(t *testing.T) {
	s := worldTestServer(t, map[string][]byte{"W.ESF": make([]byte, 2*1024*1024)})

	resp := worldGet(t, s, "/worlds/W.ESF/0/1048576", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversize chunk -> %d, want 400", resp.StatusCode)
	}
	for _, bad := range []string{"/worlds/W.ESF/x/10", "/worlds/W.ESF/0/0", "/worlds/W.ESF/-1/10",
		"/worlds/W.ESF/99999999/10", "/worlds/W.ESF/10"} {
		if code := worldGet(t, s, bad, "").StatusCode; code == http.StatusOK {
			t.Errorf("%s was served 200", bad)
		}
	}
}

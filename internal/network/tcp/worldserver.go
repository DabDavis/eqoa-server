package tcp

import (
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/worlds"
)

// World ESF delivery over HTTP, for the in-game HDD stager.
//
// WHY THIS IS NOT THE PATCHER
//
// The patcher cannot put a world on the hard disk, for two independent reasons:
//
//   - Its only writable target is the save directory. A manifest LocalName is a
//     FILENAME, not a path — the client builds mc0:/BASLUS-20744EQONLINE/<name>
//     from retail's own module-load format string. There is no device field.
//   - It runs before pfs0: exists. The SD's mount is lazy, on the first "hdd:"
//     open, because at the EULA sceSifSearchModuleByName("dev9") still answers
//     -202. The patcher runs before login; there is nothing to write to yet.
//
// So a world reaches the disk only from inside the running game, and the EE can
// do that itself: sceInet* is in SLUS at fixed addresses (sceInetCreate
// 0x00124CD0, sceInetOpen 0x00124D40, sceInetRecv 0x00124DD0, sceInetSend
// 0x00124FA8), so the stager opens a socket, sends a GET, and streams the body
// into pfs0: with sceWrite. No DRDP opcode, no protocol of our own — which also
// means a world can be re-fetched without touching the game connection's state.
//
// The client already knows where to connect: eqahosts.txt on the memory card is
// "<ip>\n\n", written by this same server from cfg.Server.ServerIP.
//
// THE RESPONSE SHAPE IS PART OF THE CONTRACT. The consumer is hand-written MIPS
// in a 8 KB arena, so this handler is deliberately narrower than net/http would
// allow: never chunked, always Content-Length, Connection: close, no compression
// regardless of Accept-Encoding, and at most one range. http.ServeContent was
// not used precisely because its generality (multipart ranges, conditional
// requests, content sniffing) is surface the EE side would have to parse.

// worldsDir resolves like patchDir: the working directory first, then beside the
// executable, so running from either place finds the same files.
func (s *PatchServer) worldsDir() string {
	return worlds.Dir(s.cfg.Patch.WorldsDir)
}

// worldCRCEntry caches a file's CRC32 against the identity it was computed from.
// A 6 MB CRC per request would be paid on every poll of the index, and the
// stager is expected to poll it on every world load.
type worldCRCEntry struct {
	size  int64
	mtime int64
	crc   uint32
}

var (
	worldCRCMu    sync.Mutex
	worldCRCCache = map[string]worldCRCEntry{}
)

// safeWorldName rejects anything that is not a bare filename. The UDP stager
// route applies the same rule, so the rule itself lives in internal/worlds.
func safeWorldName(name string) bool { return worlds.SafeName(name) }

func worldCRC(path string, fi os.FileInfo) (uint32, error) {
	key := path
	worldCRCMu.Lock()
	if e, ok := worldCRCCache[key]; ok && e.size == fi.Size() && e.mtime == fi.ModTime().UnixNano() {
		worldCRCMu.Unlock()
		return e.crc, nil
	}
	worldCRCMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	crc := h.Sum32()

	worldCRCMu.Lock()
	worldCRCCache[key] = worldCRCEntry{size: fi.Size(), mtime: fi.ModTime().UnixNano(), crc: crc}
	worldCRCMu.Unlock()
	return crc, nil
}

// handleWorlds serves /worlds/ (the index) and /worlds/<NAME> (the bytes).
// Reports whether the path was handled at all, so the caller keeps its 404.
func (s *PatchServer) handleWorlds(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/worlds/") {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, "/worlds/")

	if name == "" {
		s.serveWorldIndex(w)
		return true
	}

	// NAME/OFFSET/LENGTH — the same bytes a Range request would give, addressed
	// by path instead of by header.
	//
	// This exists because of what the EE side turned out to be. The stager does
	// not speak HTTP: it calls the game's OWN client (PatcherAPI, ctor at
	// 0x005DC568 taking a full URL, GET at 0x005DCBB8 filling a caller buffer
	// and returning the body length in v1), which builds its request from a
	// fixed format string with no Range header and no way to add one. Proved
	// from a cold guest call: three calls fetched /worlds/ into EE memory.
	//
	// So chunking has to be expressible in a URL. That is strictly better than
	// teaching the EE to parse headers — it keeps ALL the HTTP on this side,
	// and the client cannot ask for more than one buffer's worth by accident.
	if i := strings.IndexByte(name, '/'); i >= 0 {
		s.serveWorldChunk(w, r, name[:i], name[i+1:])
		return true
	}
	if !safeWorldName(name) {
		log.Printf("World server: rejected name %q", name)
		http.Error(w, "bad name", http.StatusBadRequest)
		return true
	}
	s.serveWorldFile(w, r, name)
	return true
}

// serveWorldIndex answers one line per world: NAME SIZE CRC32.
//
// Fixed-width lowercase hex and a decimal size, space separated, LF terminated —
// the stager parses this with a byte loop. Size is what it compares against the
// local sceLseek(END); the CRC is for verifying a completed download, since
// re-reading 6 MB off pfs0: to check a file that is probably fine is not
// something to do on every world load.
func (s *PatchServer) serveWorldIndex(w http.ResponseWriter) {
	dir := s.worldsDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		// An absent directory is not an error: it means this server offers no
		// downloadable worlds, and the stager should be told that cleanly
		// rather than left to interpret a 404 or a connection error.
		log.Printf("World server: no worlds dir (%v); serving empty index", err)
		writeWorldBody(w, http.StatusOK, "text/plain", nil)
		return
	}

	var names []string
	for _, e := range ents {
		if e.IsDir() || !safeWorldName(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		path := filepath.Join(dir, n)
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		crc, err := worldCRC(path, fi)
		if err != nil {
			log.Printf("World server: CRC %s: %v", n, err)
			continue
		}
		fmt.Fprintf(&b, "%s %d %08x\n", n, fi.Size(), crc)
	}
	writeWorldBody(w, http.StatusOK, "text/plain", []byte(b.String()))
	log.Printf("World server: served index (%d worlds)", len(names))
}

// serveWorldFile streams one world, honouring a single open- or closed-ended
// byte range so an interrupted 6 MB transfer resumes instead of restarting.
func (s *PatchServer) serveWorldFile(w http.ResponseWriter, r *http.Request, name string) {
	path := filepath.Join(s.worldsDir(), name)
	f, err := os.Open(path)
	if err != nil {
		log.Printf("World server: 404 %s (%v)", name, err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	total := fi.Size()

	start, end, status, ok := parseWorldRange(r.Header.Get("Range"), total)
	if !ok {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10))
		http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
		log.Printf("World server: 416 %s Range=%q size=%d", name, r.Header.Get("Range"), total)
		return
	}

	length := end - start + 1
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.FormatInt(length, 10))
	h.Set("Accept-Ranges", "bytes")
	h.Set("Connection", "close")
	if status == http.StatusPartialContent {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	}
	w.WriteHeader(status)

	if r.Method == http.MethodHead {
		log.Printf("World server: HEAD %s (size=%d)", name, total)
		return
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		log.Printf("World server: seek %s: %v", name, err)
		return
	}
	n, err := io.CopyN(w, f, length)
	if err != nil {
		// A console that drops mid-transfer is expected, and is exactly what
		// the range support above exists for. Log it as delivery progress, not
		// as a server fault.
		log.Printf("World server: %s sent %d/%d bytes from %d, interrupted: %v", name, n, length, start, err)
		return
	}
	log.Printf("World server: served %s bytes %d-%d/%d (%d)", name, start, end, total, status)
}

// maxWorldChunk bounds a single chunk. The EE reads into a fixed buffer it
// allocated before asking, so a chunk larger than that buffer is a memory
// overwrite in the guest, not a slow response. Refuse rather than clamp: a
// clamped reply is short, and the stager would take the short read as EOF and
// write a truncated world that still passed a length check on the next boot.
const maxWorldChunk = 512 * 1024

// serveWorldChunk answers /worlds/<NAME>/<OFFSET>/<LENGTH> with raw bytes.
//
// A read that starts at or past EOF is 200 with an empty body, which is the
// stager's loop terminator. A short read (fewer bytes than asked, because the
// file ended) is honest and also terminal. Anything malformed is an error, so a
// bad URL can never look like a finished download.
func (s *PatchServer) serveWorldChunk(w http.ResponseWriter, r *http.Request, name, rest string) {
	if !safeWorldName(name) {
		http.Error(w, "bad name", http.StatusBadRequest)
		return
	}
	offStr, lenStr, ok := strings.Cut(rest, "/")
	if !ok {
		http.Error(w, "want NAME/OFFSET/LENGTH", http.StatusBadRequest)
		return
	}
	off, err1 := strconv.ParseInt(offStr, 10, 64)
	n, err2 := strconv.ParseInt(lenStr, 10, 64)
	if err1 != nil || err2 != nil || off < 0 || n <= 0 {
		http.Error(w, "bad offset/length", http.StatusBadRequest)
		return
	}
	if n > maxWorldChunk {
		log.Printf("World server: chunk %s len=%d exceeds %d", name, n, maxWorldChunk)
		http.Error(w, "chunk too large", http.StatusBadRequest)
		return
	}

	path := filepath.Join(s.worldsDir(), name)
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if off > fi.Size() {
		http.Error(w, "offset past end", http.StatusBadRequest)
		return
	}
	if rem := fi.Size() - off; n > rem {
		n = rem
	}

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.FormatInt(n, 10))
	h.Set("Connection", "close")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead || n == 0 {
		return
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return
	}
	if _, err := io.CopyN(w, f, n); err != nil {
		log.Printf("World server: chunk %s@%d interrupted: %v", name, off, err)
		return
	}
	log.Printf("World server: chunk %s %d+%d of %d", name, off, n, fi.Size())
}

// parseWorldRange handles an absent header, "bytes=N-", and "bytes=N-M".
//
// Anything else — multiple ranges, suffix ranges, another unit — is refused
// rather than approximated, because the EE side sends exactly one shape and a
// request in any other shape did not come from the stager.
func parseWorldRange(hdr string, total int64) (start, end int64, status int, ok bool) {
	if hdr == "" {
		if total == 0 {
			return 0, -1, http.StatusOK, true // empty body, Content-Length 0
		}
		return 0, total - 1, http.StatusOK, true
	}
	const prefix = "bytes="
	if !strings.HasPrefix(hdr, prefix) {
		return 0, 0, 0, false
	}
	spec := strings.TrimPrefix(hdr, prefix)
	if strings.Contains(spec, ",") {
		return 0, 0, 0, false
	}
	lo, hi, found := strings.Cut(spec, "-")
	if !found || lo == "" {
		return 0, 0, 0, false // suffix range ("-500") unsupported
	}
	start, err := strconv.ParseInt(lo, 10, 64)
	if err != nil || start < 0 || start >= total {
		return 0, 0, 0, false
	}
	end = total - 1
	if hi != "" {
		end, err = strconv.ParseInt(hi, 10, 64)
		if err != nil || end < start {
			return 0, 0, 0, false
		}
		if end > total-1 {
			end = total - 1
		}
	}
	return start, end, http.StatusPartialContent, true
}

// writeWorldBody keeps every world response the same shape as the file one:
// explicit length, explicit close, never chunked and never compressed.
func writeWorldBody(w http.ResponseWriter, status int, ctype string, body []byte) {
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Connection", "close")
	w.WriteHeader(status)
	w.Write(body)
}

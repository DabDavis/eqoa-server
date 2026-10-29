package gmweb

import (
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
)

// A sanitised snapshot of the database for a GM's own machine, where the
// server's real-data tests (EQOA_REAL_DB) run on it: database.WriteSnapshot
// makes it (every credential and person's data out, the game whole, refused
// when a table holds something its rules do not know), gzipped here, and it
// is handed out as a link that works ONCE, for snapshotTTL, and only while the
// admin who asked is one and the credential they asked with still works. The
// file is deleted when the link is used, expires, is replaced or the server
// stops. Each step is in the audit log (action "snapshot"): made (who, the
// sizes, the sanitiser's version), fetched (from where), expired unused,
// refused.

const (
	// snapshotTTL is how long a link works: long enough for an agent's
	// person to allow its curl, short enough that a link in a log is spent.
	snapshotTTL = 10 * time.Minute
	// snapshotPerHour is how many an admin may make an hour (3 at once).
	snapshotPerHour = 6
	// snapshotMaxLinks bounds the links (and files) waiting at once; an
	// admin's new snapshot replaces their last.
	snapshotMaxLinks = 8
	// snapshotTicket starts every link's ticket.
	snapshotTicket = "snap_"
)

func init() {
	// in a snapshot only: a panel run on one (a GM's own machine) says what it is
	catalog[database.SnapshotInfoTable] = tableNote{
		Label: "Snapshot", Group: "Panel", Write: writeNone,
		Help: "This database is a sanitised snapshot of a live server's (download_snapshot): the sanitiser's version, " +
			"when it was made and by whom, and what it took out.",
	}
}

// snapshotLink is a snapshot waiting to be fetched.
type snapshotLink struct {
	seq       int64
	accountID int
	username  string
	tokenID   int64  // the token that asked, else 0
	session   string // the browser session that asked, else ""
	path      string // the gzipped file
	file      string // its name for the person
	bytes     int64  // gzipped
	dbBytes   int64
	sha256    string // of the gzipped file
	expires   time.Time
	timer     *time.Timer
}

type snapshotStore struct {
	mu    sync.Mutex
	build sync.Mutex               // one snapshot made at a time
	links map[string]*snapshotLink // by the ticket's SHA-256
	seq   int64
	rate  *bucketLimiter // per account
	ttl   time.Duration
}

func newSnapshotStore() *snapshotStore {
	return &snapshotStore{links: map[string]*snapshotLink{}, rate: newBucketLimiter(snapshotPerHour/60.0, 3), ttl: snapshotTTL}
}

// snapshotDir is where snapshots wait: beside the backups, only the server's
// own user reading it.
func (s *Server) snapshotDir() string {
	return filepath.Join(s.backupDir(), "snapshots")
}

// clearSnapshotDir removes what a server stopped without tidying left (its
// links died with it).
func (s *Server) clearSnapshotDir() {
	matches, _ := filepath.Glob(filepath.Join(s.snapshotDir(), "snap-*"))
	for _, m := range matches {
		os.Remove(m)
	}
}

func (s *Server) registerSnapshotOps() {
	s.addOp(&Op{
		Name: "download_snapshot", Title: "A sanitised copy of the database", Scope: scopeAdmin,
		Description: "A copy of the live database for your own machine, with every credential and person's data taken out " +
			"(password hashes, API/OAuth tokens, authenticator secrets, OAuth clients, the audit log, the work board, e-mail " +
			"and IP addresses, the address salts; a player's login name becomes account-<id>) and the game's content whole. " +
			"Answers a link to the gzipped SQLite file that works ONCE, for 10 minutes, while you are an admin: run the " +
			"answer's fetch command (curl), gunzip the file and run the server's real-data tests on it " +
			"(EQOA_REAL_DB=<file> go test ./internal/... -run Real). Never commit, attach or share the file: it is the " +
			"server's content. At most 6 an hour; refused, naming the table, when one holds something the sanitiser has no rule for.",
		Handler: func(c *opCtx) (any, error) { return c.s.makeSnapshot(c) },
	})
}

func (s *Server) makeSnapshot(c *opCtx) (any, error) {
	st := s.snap
	if !st.rate.allow(fmt.Sprint(c.a.AccountID)) {
		return nil, &opError{http.StatusTooManyRequests, fmt.Sprintf("at most %d snapshots an hour each: use the last one, or wait", snapshotPerHour)}
	}
	if !st.build.TryLock() {
		return nil, &opError{http.StatusTooManyRequests, "a snapshot is being made; try again in a moment"}
	}
	defer st.build.Unlock()
	dir := s.snapshotDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	os.Chmod(dir, 0o700)
	base := filepath.Join(dir, "snap-"+randomHex(8))
	start := time.Now()
	rep, err := s.d.DB.WriteSnapshot(base+".db", c.a.Username)
	if err != nil {
		var refused *database.SnapshotRefused
		if errors.As(err, &refused) {
			s.audit(c.a, "snapshot", "", nil, nil, nil, "a sanitised snapshot was refused: "+clip(err.Error(), 1500))
			return nil, &opError{http.StatusConflict, err.Error()}
		}
		return nil, fmt.Errorf("the snapshot failed: %w", err)
	}
	file := "eqoa-snapshot-" + rep.Made.Format("20060102-150405") + ".db.gz"
	gzBytes, sum, err := gzipFile(base+".db", base+".db.gz", strings.TrimSuffix(file, ".gz"))
	os.Remove(base + ".db")
	if err != nil {
		os.Remove(base + ".db.gz")
		return nil, fmt.Errorf("the snapshot was made, but not gzipped: %w", err)
	}

	ticket := snapshotTicket + randomTicket()
	link := &snapshotLink{
		accountID: c.a.AccountID, username: c.a.Username, tokenID: c.a.TokenID,
		path: base + ".db.gz", file: file, bytes: gzBytes, dbBytes: rep.Bytes, sha256: sum,
		expires: time.Now().Add(st.ttl),
	}
	if c.a.sess != nil {
		link.session = c.a.sess.Token
	}
	key := hashToken(ticket)
	st.mu.Lock()
	var replaced []*snapshotLink
	for h, l := range st.links {
		if l.accountID == c.a.AccountID { // one waiting each: the new one replaces it
			l.timer.Stop()
			delete(st.links, h)
			replaced = append(replaced, l)
		}
	}
	if len(st.links) >= snapshotMaxLinks {
		st.mu.Unlock()
		os.Remove(link.path)
		return nil, &opError{http.StatusTooManyRequests, "too many snapshot links are waiting; try again when one is used or expires"}
	}
	st.seq++
	link.seq = st.seq
	st.links[key] = link
	link.timer = time.AfterFunc(st.ttl, func() { s.expireSnapshot(key) })
	st.mu.Unlock()
	for _, l := range replaced {
		os.Remove(l.path)
		s.audit(c.a, "snapshot", "", nil, nil, nil, fmt.Sprintf("the link to the sanitised snapshot %s was replaced by a new one, unused; its file was deleted", l.file))
	}

	url := s.publicBase(c.r) + "/api/snapshot/" + ticket + "/" + file
	db := strings.TrimSuffix(file, ".gz")
	s.audit(c.a, "snapshot", "", nil, nil, nil, fmt.Sprintf(
		"made the sanitised snapshot %s (sanitiser %d, %d tables): %s gzipped (sha256 %s), %s as a database, in %v; its link works once until %s",
		file, rep.Version, rep.Tables, humanBytes(gzBytes), sum, humanBytes(rep.Bytes), time.Since(start).Round(time.Millisecond), stamp(link.expires)))
	return map[string]any{
		"url":              url,
		"file":             file,
		"expires":          stamp(link.expires),
		"expiresInSeconds": int(st.ttl / time.Second),
		"singleUse":        true,
		"bytes":            gzBytes,
		"databaseBytes":    rep.Bytes,
		"sha256":           sum,
		"sanitiser":        rep.Version,
		"tables":           rep.Tables,
		"removed":          rep.Steps,
		"fetch":            fmt.Sprintf("curl -fsS -o %s '%s'", file, url),
		"then": fmt.Sprintf("sha256sum %s (expect %s); gunzip %s; then in a clone of the server: "+
			"EQOA_REAL_DB=/full/path/to/%s go test ./internal/entity/ ./internal/handlers/ -run Real -count=1", file, sum, file, db),
		"note": "The link works once and only until expires; ask again for a new one. The file is the server's content with " +
			"no credential or person's data in it: keep it outside the repository, and never commit, attach or share it.",
	}, nil
}

// randomTicket is 256 bits, for a link.
func randomTicket() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// gzipFile writes src gzipped to dst (only its owner reading it) under the
// name given, and answers its size and SHA-256.
func gzipFile(src, dst, name string) (int64, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	zw := gzip.NewWriter(io.MultiWriter(out, h))
	zw.Name = name
	if _, err := io.Copy(zw, in); err != nil {
		out.Close()
		return 0, "", err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return 0, "", err
	}
	if err := out.Close(); err != nil {
		return 0, "", err
	}
	fi, err := os.Stat(dst)
	if err != nil {
		return 0, "", err
	}
	return fi.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

// humanBytes is a size as a person reads it.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// expireSnapshot drops a link its time ran out on, and its file.
func (s *Server) expireSnapshot(key string) {
	st := s.snap
	st.mu.Lock()
	link := st.links[key]
	delete(st.links, key)
	st.mu.Unlock()
	if link == nil {
		return
	}
	os.Remove(link.path)
	s.audit(&actor{AccountID: link.accountID, Username: link.username, Via: "snapshot link"}, "snapshot", "", nil, nil, nil,
		fmt.Sprintf("the link to the sanitised snapshot %s expired unused; its file was deleted", link.file))
}

// dropSnapshots drops every link and its file (the server stops).
func (s *Server) dropSnapshots() {
	st := s.snap
	st.mu.Lock()
	defer st.mu.Unlock()
	for key, l := range st.links {
		l.timer.Stop()
		os.Remove(l.path)
		delete(st.links, key)
	}
}

// snapshotHolderGone is why the admin a link was made for may not have it
// now, or "": no longer an admin with an authenticator, or the credential
// they asked with revoked, expired or signed out.
func (s *Server) snapshotHolderGone(l *snapshotLink) string {
	acc, err := s.accountByID(l.accountID)
	if err != nil {
		return "the account is gone"
	}
	if !s.isGM(acc) || acc.Level < s.cfg.AdminLevel {
		return "the account is no longer an admin"
	}
	if on, err := s.totpEnabled(acc.ID); err != nil || !on {
		return "the account has no authenticator now"
	}
	if l.tokenID != 0 {
		var revoked int
		var expires sql.NullString
		if err := s.d.DB.QueryRow(`SELECT revoked, expires FROM gm_tokens WHERE id = ?`, l.tokenID).Scan(&revoked, &expires); err != nil {
			return "the token that asked for it is gone"
		}
		if revoked != 0 {
			return "the token that asked for it was revoked"
		}
		if expires.Valid && expires.String != "" && time.Now().After(parseStamp(expires.String)) {
			return "the token that asked for it expired"
		}
	}
	if l.session != "" && s.sessions.get(l.session) == nil {
		return "the panel session that asked for it ended"
	}
	return ""
}

func (s *Server) snapshotHeaders(w http.ResponseWriter, l *snapshotLink) {
	h := w.Header()
	h.Set("Content-Type", "application/gzip")
	h.Set("Content-Disposition", `attachment; filename="`+l.file+`"`)
	h.Set("Content-Length", fmt.Sprint(l.bytes))
	h.Set("Cache-Control", "no-store")
	h.Set("X-Snapshot-SHA256", l.sha256)
}

// handleSnapshotDownload is a snapshot's link: api/snapshot/<ticket>[/<file>].
// GET hands the file over once; HEAD answers its headers and leaves the link.
func (s *Server) handleSnapshotDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "GET the link")
		return
	}
	ip := clientIP(r)
	limit := auth.LimitKey(ip)
	if s.badTokens.empty(limit) {
		writeError(w, http.StatusTooManyRequests, "too many unknown links from this address; wait a minute")
		return
	}
	ticket, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/snapshot/"), "/")
	key := hashToken(ticket)
	st := s.snap
	st.mu.Lock()
	link := st.links[key]
	if link != nil && r.Method == http.MethodGet {
		delete(st.links, key) // once
		link.timer.Stop()
	}
	st.mu.Unlock()
	if link == nil || !strings.HasPrefix(ticket, snapshotTicket) {
		s.badTokens.allow(limit)
		writeError(w, http.StatusNotFound, "no such snapshot link: a link works once, for ten minutes, and not after the "+
			"server restarts; ask for a new one (download_snapshot)")
		return
	}
	if r.Method == http.MethodHead {
		s.snapshotHeaders(w, link)
		w.WriteHeader(http.StatusOK)
		return
	}
	defer os.Remove(link.path)
	who := &actor{AccountID: link.accountID, Username: link.username, Via: "snapshot link", IP: ip}
	if time.Now().After(link.expires) {
		writeError(w, http.StatusGone, "this snapshot link expired; ask for a new one (download_snapshot)")
		return
	}
	if why := s.snapshotHolderGone(link); why != "" {
		s.audit(who, "snapshot", "", nil, nil, nil, fmt.Sprintf("refused the link to the sanitised snapshot %s: %s; its file was deleted", link.file, why))
		writeError(w, http.StatusForbidden, "this link was made for an admin who may not have it now: "+why)
		return
	}
	f, err := os.Open(link.path)
	if err != nil {
		log.Printf("GM panel: snapshot %s: %v", link.file, err)
		writeError(w, http.StatusGone, "the snapshot's file is gone; ask for a new one")
		return
	}
	defer f.Close()
	s.snapshotHeaders(w, link)
	// a slow line may take longer than the server's own write timeout
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Minute))
	n, err := io.Copy(w, f)
	detail := fmt.Sprintf("fetched the sanitised snapshot %s (%s)", link.file, humanBytes(link.bytes))
	if err != nil || n != link.bytes {
		detail += fmt.Sprintf(", but only %d of its %d bytes went out (%v); its file was deleted", n, link.bytes, err)
	}
	s.audit(who, "snapshot", "", nil, nil, nil, detail)
}

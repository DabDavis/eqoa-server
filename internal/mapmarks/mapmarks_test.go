package mapmarks

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func fixture(t *testing.T) (*Service, *database.SQLiteDB) {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(db)
	now := time.Date(2026, 10, 26, 19, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	return s, db
}

type listed struct {
	Version string
	Marks   []Mark
}

func get(t *testing.T, s *Service, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/mapmarks", nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeMarks(w, r)
	return w
}

func list(t *testing.T, s *Service) (listed, string) {
	t.Helper()
	w := get(t, s, nil)
	if w.Code != 200 {
		t.Fatalf("the list: %d %s", w.Code, w.Body)
	}
	var out listed
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out, w.Header().Get("ETag")
}

func suggest(t *testing.T, s *Service, ip string, body any) (int, map[string]any, http.Header) {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		raw, _ = json.Marshal(b)
	}
	w := httptest.NewRecorder()
	s.ServeSuggest(w, httptest.NewRequest("POST", "/api/mapmarks/suggest", bytes.NewReader(raw)), ip)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Header()
}

func spot(world int, x, z float64, name string) map[string]any {
	return map[string]any{"world": world, "x": x, "z": z, "name": name, "note": "", "client": "dev-abc1234"}
}

func addMark(t *testing.T, db *database.SQLiteDB, world int, x, z float64, name, kind string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO mapmarks (world, x, z, name, note, kind) VALUES (?, ?, ?, ?, 'by the well', ?)`, world, x, z, name, kind); err != nil {
		t.Fatal(err)
	}
	database.InvalidateMapMarks()
}

// The list: the marks with a version of their own as the ETag, cached for a
// minute, any site's; a matching If-None-Match is 304 (another tag is the
// control); a change to the marks is a new version at once; gzip is the same
// list; HEAD carries no body; only GET reads it.
func TestTheListAndItsVersion(t *testing.T) {
	s, db := fixture(t)
	empty, etag := list(t, s)
	if empty.Marks == nil || len(empty.Marks) != 0 || empty.Version == "" || etag != `"`+empty.Version+`"` {
		t.Fatalf("an empty list: %+v, ETag %s", empty, etag)
	}
	w := get(t, s, nil)
	if w.Header().Get("Cache-Control") != "public, max-age=60" || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("the list's headers: %v", w.Header())
	}
	if w := get(t, s, map[string]string{"If-None-Match": etag}); w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("ETag") != etag {
		t.Errorf("If-None-Match its own tag: %d %q", w.Code, w.Body)
	}
	if w := get(t, s, map[string]string{"If-None-Match": `"0000000000000000"`}); w.Code != 200 {
		t.Errorf("If-None-Match another tag (the control): %d", w.Code)
	}
	addMark(t, db, 0, 24611.04, -15502.36, "The Old Well", "landmark")
	one, etag2 := list(t, s)
	if etag2 == etag || len(one.Marks) != 1 {
		t.Fatalf("after a mark: %+v (ETag %s, was %s)", one, etag2, etag)
	}
	m := one.Marks[0]
	if m.World != 0 || m.X != 24611 || m.Z != -15502.4 || m.Name != "The Old Well" || m.Note != "by the well" || m.Kind != "landmark" || m.ID == 0 {
		t.Errorf("the mark: %+v", m)
	}
	if w := get(t, s, map[string]string{"If-None-Match": etag}); w.Code != 200 {
		t.Errorf("the old tag after a change: %d", w.Code)
	}
	// the same marks keep their version (a map's tag holds across restarts)
	if again, _ := list(t, New(db)); again.Version != one.Version {
		t.Errorf("another service, the same marks: %s, was %s", again.Version, one.Version)
	}
	r := httptest.NewRequest("GET", "/api/mapmarks", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	zw := httptest.NewRecorder()
	s.ServeMarks(zw, r)
	zr, err := gzip.NewReader(zw.Body)
	if err != nil {
		t.Fatalf("gzip: %v (%v)", err, zw.Header())
	}
	unz, _ := io.ReadAll(zr)
	if !bytes.Equal(unz, get(t, s, nil).Body.Bytes()) {
		t.Errorf("the gzipped list differs from the plain one")
	}
	head := httptest.NewRecorder()
	s.ServeMarks(head, httptest.NewRequest("HEAD", "/api/mapmarks", nil))
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Errorf("HEAD: %d with %d bytes", head.Code, head.Body.Len())
	}
	post := httptest.NewRecorder()
	s.ServeMarks(post, httptest.NewRequest("POST", "/api/mapmarks", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("POST to the list: %d %v", post.Code, post.Header())
	}
}

// A browser's preflight on both routes: 204 with the methods and the header a
// JSON POST asks for, any origin.
func TestPreflight(t *testing.T) {
	s, _ := fixture(t)
	for _, f := range []func(http.ResponseWriter, *http.Request){s.ServeMarks, func(w http.ResponseWriter, r *http.Request) { s.ServeSuggest(w, r, "198.51.100.7") }} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("OPTIONS", "/api/mapmarks/suggest", nil)
		r.Header.Set("Origin", "https://eqoa.live")
		r.Header.Set("Access-Control-Request-Method", "POST")
		f(w, r)
		h := w.Header()
		if w.Code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != "*" ||
			h.Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" || h.Get("Access-Control-Allow-Headers") != "Content-Type" {
			t.Errorf("preflight: %d %v", w.Code, h)
		}
	}
}

// A suggestion is kept as sent, but its sender only as a salted hash (no column
// holds the address); a note's line breaks become spaces and other control
// characters go; a body's kind is not read (a GM gives one).
func TestASuggestionIsKeptWithTheSenderHashed(t *testing.T) {
	s, db := fixture(t)
	body := spot(2, 1234.5, -987.25, "  Hidden Cave  ")
	body["note"] = "two lines\nof note\u0007 here"
	body["kind"] = "danger"
	code, out, hdr := suggest(t, s, "198.51.100.7", body)
	if code != 200 || out["ok"] != true || out["id"] == nil || hdr.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("suggest: %d %v %v", code, out, hdr)
	}
	rows, err := db.MapMarkSuggestions("all", -1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("kept: %v %v", rows, err)
	}
	r := rows[0]
	if r.ID != int64(out["id"].(float64)) || r.World != 2 || r.X != 1234.5 || r.Z != -987.25 || r.Name != "Hidden Cave" ||
		r.Note != "two lines of note here" || r.Client != "dev-abc1234" || r.Status != database.MapMarkWaiting || r.Created != "2026-10-26 19:00:00" {
		t.Errorf("the row: %+v", r)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(r.SenderHash) {
		t.Errorf("the sender as kept: %q", r.SenderHash)
	}
	// no column of any row holds the address
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mapmark_suggestions WHERE name || note || client || sender_hash || created LIKE '%198.51.100%'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the address is kept in clear (%d rows, %v)", n, err)
	}
	// the control: the same address again is the same sender
	suggest(t, s, "198.51.100.7", spot(2, 5000, 5000, "Second Cave"))
	again, _ := db.MapMarkSuggestions("all", -1)
	if len(again) != 2 || again[0].SenderHash != r.SenderHash {
		t.Errorf("the same address hashed again: %+v", again)
	}
}

// Every field's rule, each refused with a sentence and the CORS header; the
// controls: the same suggestion fixed passes.
func TestTheRouteRefuses(t *testing.T) {
	s, _ := fixture(t)
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name string
		body any
		code int
	}{
		{"not JSON", "{world: 0", 400},
		{"no world", map[string]any{"x": 1, "z": 1, "name": "Some Rock"}, 400},
		{"no x", map[string]any{"world": 0, "z": 1, "name": "Some Rock"}, 400},
		{"world -1", spot(-1, 1, 1, "Some Rock"), 400},
		{"world 30", spot(30, 1, 1, "Some Rock"), 400},
		{"a world that is not whole", `{"world": 1.5, "x": 1, "z": 1, "name": "Some Rock"}`, 400},
		{"x too far", spot(0, 200001, 1, "Some Rock"), 400},
		{"z too far", spot(0, 1, -200001, "Some Rock"), 400},
		{"one character", spot(0, 1, 1, " a "), 400},
		{"41 characters", spot(0, 1, 1, long(41)), 400},
		{"a control character", spot(0, 1, 1, "Some\u0007Rock"), 400},
		{"a profane name", spot(0, 1, 1, "the ass rock"), 403},
		{"a note of 201", map[string]any{"world": 0, "x": 1, "z": 1, "name": "Some Rock", "note": long(201)}, 400},
		{"a client of 33", map[string]any{"world": 0, "x": 1, "z": 1, "name": "Some Rock", "client": long(33)}, 400},
		{"a body past 4 KB", `{"world": 0, "x": 1, "z": 1, "name": "Some Rock", "note": "` + long(5000) + `"}`, 400},
	}
	for _, c := range cases {
		code, out, hdr := suggest(t, s, "198.51.100.7", c.body)
		if code != c.code || out["ok"] != false || out["error"] == "" || hdr.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: %d %v %v", c.name, code, out, hdr.Get("Access-Control-Allow-Origin"))
		}
	}
	// the controls: the limits themselves pass
	for i, b := range []any{
		spot(0, 1, 1, "Bass Rock"), spot(29, 200000, -200000, "ab"), spot(5, 9000, 9000, long(40)),
		map[string]any{"world": 1, "x": 3000, "z": 3000, "name": "Some Rock", "note": long(200), "client": long(32)},
	} {
		if code, out, _ := suggest(t, s, "198.51.100.7", b); code != 200 {
			t.Errorf("control %d: %d %v", i, code, out)
		}
	}
	w := httptest.NewRecorder()
	s.ServeSuggest(w, httptest.NewRequest("GET", "/api/mapmarks/suggest", nil), "198.51.100.7")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET a suggestion: %d", w.Code)
	}
}

// A sender's new suggestion within 5 of one of its own waiting in the same world
// takes its place (the same id); 6 off, another world or another sender is new.
// A sender holds PerSender waiting (an IPv6 one by its /64), a replacement still
// passing at the limit; the box holds MaxWaiting in all.
func TestASenderReplacesItsOwnNearbyAndIsLimited(t *testing.T) {
	s, db := fixture(t)
	id := func(code int, out map[string]any) int64 {
		t.Helper()
		if code != 200 {
			t.Fatalf("suggest: %d %v", code, out)
		}
		return int64(out["id"].(float64))
	}
	ip := "198.51.100.7"
	first := id(suggestAt(t, s, ip, 0, 100, 100, "Old Tree"))
	if again := id(suggestAt(t, s, ip, 0, 103, 104, "Old Oak")); again != first {
		t.Errorf("5 off, the same sender: id %d, want %d (replaced)", again, first)
	}
	if r, _ := db.MapMarkSuggestionByID(first); r.Name != "Old Oak" || r.X != 103 {
		t.Errorf("the replaced row: %+v", r)
	}
	if other := id(suggestAt(t, s, ip, 0, 109, 104, "Old Tree")); other == first {
		t.Errorf("6 off: replaced")
	}
	if other := id(suggestAt(t, s, ip, 1, 103, 104, "Old Tree")); other == first {
		t.Errorf("another world: replaced")
	}
	if other := id(suggestAt(t, s, "203.0.113.9", 0, 103, 104, "Old Tree")); other == first {
		t.Errorf("another sender: replaced")
	}

	s.PerSender = 3
	if code, out := suggestAt(t, s, ip, 0, 500, 500, "Far Rock"); code != http.StatusTooManyRequests || out["error"] != "You have 3 suggestions waiting for a GM already." {
		t.Errorf("a fourth: %d %v", code, out)
	}
	if code, out := suggestAt(t, s, ip, 0, 101, 101, "Old Oak again"); code != 200 {
		t.Errorf("a replacement at the limit: %d %v", code, out)
	}
	// an IPv6 sender by its /64: two addresses of one /64 are one sender
	v6a, v6b := "2001:db8:1:2::10", "2001:db8:1:2::99"
	for i := 0; i < 3; i++ {
		id(suggestAt(t, s, v6a, 0, float64(1000+100*i), 0, "Six Rock"))
	}
	if code, _ := suggestAt(t, s, v6b, 0, 5000, 0, "Six Rock"); code != http.StatusTooManyRequests {
		t.Errorf("the same /64's fourth: %d", code)
	}
	if code, _ := suggestAt(t, s, "2001:db8:1:3::10", 0, 5000, 0, "Six Rock"); code != 200 {
		t.Errorf("another /64 (the control): %d", code)
	}

	s.PerSender = 20
	n, _ := db.MapMarkSuggestionCount()
	s.MaxWaiting = n
	if code, out := suggestAt(t, s, "192.0.2.50", 3, 0, 0, "Box Rock"); code != http.StatusServiceUnavailable || out["error"] != "The suggestion box is full; try again later." {
		t.Errorf("a full box: %d %v", code, out)
	}
	if code, _ := suggestAt(t, s, ip, 0, 100, 100, "Old Oak at last"); code != 200 {
		t.Errorf("a replacement in a full box: %d", code)
	}
}

func suggestAt(t *testing.T, s *Service, ip string, world int, x, z float64, name string) (int, map[string]any) {
	t.Helper()
	code, out, _ := suggest(t, s, ip, spot(world, x, z, name))
	return code, out
}

// A fresh database has the three tables, the salt made once and kept.
func TestTheTablesAndTheSalt(t *testing.T) {
	_, db := fixture(t)
	for _, tbl := range []string{"mapmarks", "mapmark_suggestions", "mapmark_meta"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s: %d %v", tbl, n, err)
		}
	}
	a, err := db.MapMarkSalt()
	if err != nil || len(a) != 64 {
		t.Fatalf("salt: %q %v", a, err)
	}
	if b, _ := db.MapMarkSalt(); b != a {
		t.Errorf("a second read made another salt")
	}
}

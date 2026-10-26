// Package mapmarks serves the community's map marks (the user, 2026-10-26:
// "Players can submit waypoints to be added to the in-game map. Submissions
// would require official approval, similar to our spellfx"): ONE list, kept on
// this server, that eqoa-rust-client-vanilla's map draws for its player on any
// game server (Sandstorm too: the map is drawn off the player's own disc, so a
// place is the same on every server), and the suggestions players send from
// that map with no sign-in. They wait in mapmark_suggestions for a GM, who
// accepts (an audited insert into mapmarks, the name, note, kind and place the
// GM's to edit first) or dismisses each in the GM panel's Map suggestions
// (internal/gmweb).
//
// The account server (internal/network/tcp) carries the two routes, which nginx
// proxies at /eqoa/account/api/mapmarks and /eqoa/account/api/mapmarks/suggest:
//
//	GET  /api/mapmarks          {"version","marks":[{"id","world","x","z","name","note","kind"}]}
//	                            (ETag the version; If-None-Match: 304)
//	POST /api/mapmarks/suggest  {"world","x","z","name","note","client"} -> {"ok":true,"id"}
//	                            or {"ok":false,"error"}
//
// Every answer carries Access-Control-Allow-Origin * (other sites host the
// client's page; nothing here is a credential), and OPTIONS answers a browser's
// preflight. A sender is counted by its address (an IPv6 one by its /64), kept
// only as a salted hash: at most PerSender suggestions waiting, a new one within
// ReplaceWithin of one of its own waiting in the same world taking that one's
// place (a player moving the spot it meant), and MaxWaiting in the box in all.
package mapmarks

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/spellfx"
)

const (
	// DefaultPerSender is how many suggestions one sender may have waiting.
	DefaultPerSender = 20
	// DefaultMaxWaiting is how many may wait for a GM in all before the route
	// is refused: a full box, not a flood filling the disk.
	DefaultMaxWaiting = 5000
	// ReplaceWithin is how near (world units) a sender's new suggestion takes
	// the place of one of its own waiting in the same world.
	ReplaceWithin = 5.0
	// maxBody is the largest request the route reads.
	maxBody = 4096
	// MaxClient is the most characters the client's version may hold.
	MaxClient = 32
	// rebuildEvery is how old the list may grow before it is built again (an
	// accept, a GM's row edit or a revert rebuilds it at once:
	// database.InvalidateMapMarks).
	rebuildEvery = time.Minute
)

// Mark is one mark as the list gives it.
type Mark struct {
	ID    int64   `json:"id"`
	World int     `json:"world"`
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Name  string  `json:"name"`
	Note  string  `json:"note"`
	Kind  string  `json:"kind"`
}

// Service is the two routes over the database.
type Service struct {
	db *database.SQLiteDB
	// PerSender and MaxWaiting are the limits (tests lower them).
	PerSender  int
	MaxWaiting int
	// Now is the clock (tests set it).
	Now func() time.Time

	mu      sync.Mutex
	list    []byte // the list's JSON
	listGz  []byte // gzipped
	version string
	builtAt time.Time
	gen     int64
	salt    string
}

// New makes the service over db.
func New(db *database.SQLiteDB) *Service {
	return &Service{db: db, PerSender: DefaultPerSender, MaxWaiting: DefaultMaxWaiting, Now: time.Now}
}

// round1 is a coordinate as the list gives it: a tenth of a unit, which a map
// cannot show finer.
func round1(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v*10+0.5)) / 10
	}
	return float64(int64(v*10+0.5)) / 10
}

// fresh builds the list again when the marks changed or it is old. Must be
// called with mu held.
func (s *Service) fresh() error {
	now := s.Now()
	gen := database.MapMarksGeneration()
	if s.list != nil && gen == s.gen && now.Sub(s.builtAt) < rebuildEvery {
		return nil
	}
	rows, err := s.db.MapMarks(-1)
	if err != nil {
		return err
	}
	marks := make([]Mark, 0, len(rows))
	for _, m := range rows {
		marks = append(marks, Mark{ID: m.ID, World: m.World, X: round1(m.X), Z: round1(m.Z), Name: m.Name, Note: m.Note, Kind: m.Kind})
	}
	// the version is the marks' own: the same list answers the same one, so a
	// map's If-None-Match holds across restarts
	inner, err := json.Marshal(marks)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(inner)
	version := hex.EncodeToString(sum[:8])
	body, err := json.Marshal(map[string]any{"version": version, "marks": marks})
	if err != nil {
		return err
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(body)
	zw.Close()
	s.list, s.listGz, s.version, s.builtAt, s.gen = body, gz.Bytes(), version, now, gen
	return nil
}

// cors is every answer's: any site may read the list and send a suggestion.
func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
}

// preflight answers a browser's OPTIONS.
func preflight(w http.ResponseWriter) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

// etagMatches says whether an If-None-Match names etag (a list of them, or *).
func etagMatches(header, etag string) bool {
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "W/"))
		if t == etag || t == "*" {
			return true
		}
	}
	return false
}

// ServeMarks answers GET /api/mapmarks.
func (s *Service) ServeMarks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		preflight(w)
		return
	}
	cors(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "GET the list"})
		return
	}
	s.mu.Lock()
	err := s.fresh()
	body, gz, version := s.list, s.listGz, s.version
	s.mu.Unlock()
	if err != nil {
		log.Printf("MapMarks: the list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The map marks could not be read; try again in a moment."})
		return
	}
	h := w.Header()
	etag := `"` + version + `"`
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=60")
	h.Set("Vary", "Accept-Encoding")
	if inm := r.Header.Get("If-None-Match"); inm != "" && etagMatches(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "application/json; charset=utf-8")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		body = gz
	}
	h.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}

// suggestion is what the map sends. A kind is the GM's to give: a body's
// "kind" is not read.
type suggestion struct {
	World  *int     `json:"world"`
	X      *float64 `json:"x"`
	Z      *float64 `json:"z"`
	Name   string   `json:"name"`
	Note   string   `json:"note"`
	Client string   `json:"client"`
}

// plainName says whether s is a name a person typed: valid UTF-8 with no control
// or invisible separator characters.
func plainName(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return false
		}
	}
	return true
}

// cleanNote is a note as it is kept: line breaks and tabs as spaces, any other
// control character out, trimmed. ok is false for text that is not UTF-8.
func cleanNote(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ' ':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String()), true
}

// CheckName is a mark's name rule, the route's and the GM's: 2 to 40 characters
// once trimmed, plain text, nothing the profanity filter refuses (its rule for a
// voice room's name: a word the chat would mask, or a root a character's name may
// not carry). It answers the trimmed name, or the sentence a player reads.
func CheckName(name string) (string, int, string) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	switch {
	case !plainName(name):
		return "", http.StatusBadRequest, "A name is plain text."
	case n < database.MapMarkMinName || n > database.MapMarkMaxName:
		return "", http.StatusBadRequest, fmt.Sprintf("A name is %d to %d characters.", database.MapMarkMinName, database.MapMarkMaxName)
	case handlers.ProfaneRoomName(name):
		return "", http.StatusForbidden, "That name is not allowed; try another."
	}
	return name, 0, ""
}

// CheckNote is a mark's note rule: line breaks as spaces, control characters
// out, at most 200 characters.
func CheckNote(note string) (string, string) {
	note, ok := cleanNote(note)
	if !ok {
		return "", "A note is plain text."
	}
	if utf8.RuneCountInString(note) > database.MapMarkMaxNote {
		return "", fmt.Sprintf("A note is at most %d characters.", database.MapMarkMaxNote)
	}
	return note, ""
}

// addrHash is a sender's address as it is kept: salted and hashed, never the
// address.
func (s *Service) addrHash(ip string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.salt == "" {
		salt, err := s.db.MapMarkSalt()
		if err != nil {
			return "", err
		}
		s.salt = salt
	}
	sum := sha256.Sum256([]byte(s.salt + "|" + spellfx.AddrKey(ip)))
	return hex.EncodeToString(sum[:16]), nil
}

func refuse(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

// ServeSuggest answers POST /api/mapmarks/suggest from the client at ip (the
// account server's getClientIP: the proxy's word for it on loopback).
func (s *Service) ServeSuggest(w http.ResponseWriter, r *http.Request, ip string) {
	if r.Method == http.MethodOptions {
		preflight(w)
		return
	}
	cors(w)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		refuse(w, http.StatusMethodNotAllowed, "POST a suggestion.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var in suggestion
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		refuse(w, http.StatusBadRequest, `A suggestion is a small JSON object: {"world": 0, "x": 24611.5, "z": 15502.2, "name": "...", "note": "..."}.`)
		return
	}
	if in.World == nil || in.X == nil || in.Z == nil {
		refuse(w, http.StatusBadRequest, "Say where: the world, x and z.")
		return
	}
	if err := database.CheckMapMarkPlace(*in.World, *in.X, *in.Z); err != nil {
		refuse(w, http.StatusBadRequest, "That place is not on any map: "+err.Error()+".")
		return
	}
	name, code, msg := CheckName(in.Name)
	if msg != "" {
		refuse(w, code, msg)
		return
	}
	note, msg := CheckNote(in.Note)
	if msg != "" {
		refuse(w, http.StatusBadRequest, msg)
		return
	}
	client := strings.TrimSpace(in.Client)
	if utf8.RuneCountInString(client) > MaxClient || !plainName(client) {
		refuse(w, http.StatusBadRequest, fmt.Sprintf("The client's version is at most %d characters of plain text.", MaxClient))
		return
	}
	hash, err := s.addrHash(ip)
	if err != nil {
		log.Printf("MapMarks: the salt: %v", err)
		refuse(w, http.StatusInternalServerError, "The suggestion could not be kept; try again in a moment.")
		return
	}
	id, replaced, err := s.db.AddMapMarkSuggestion(database.MapMarkSuggestion{
		World: *in.World, X: *in.X, Z: *in.Z, Name: name, Note: note, Client: client, SenderHash: hash,
	}, s.Now(), ReplaceWithin, s.PerSender, s.MaxWaiting)
	switch {
	case errors.Is(err, database.ErrMapMarkSenderFull):
		refuse(w, http.StatusTooManyRequests, fmt.Sprintf("You have %d suggestions waiting for a GM already.", s.PerSender))
		return
	case errors.Is(err, database.ErrMapMarkBoxFull):
		refuse(w, http.StatusServiceUnavailable, "The suggestion box is full; try again later.")
		return
	case err != nil:
		log.Printf("MapMarks: keeping a suggestion: %v", err)
		refuse(w, http.StatusInternalServerError, "The suggestion could not be kept; try again in a moment.")
		return
	}
	verb := "suggested"
	if replaced {
		verb = "suggested again (in place of the sender's own nearby)"
	}
	log.Printf("MapMarks: %q %s in world %d at %.0f, %.0f (sender %s, %s): suggestion %d", name, verb, *in.World, *in.X, *in.Z, hash[:8], client, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

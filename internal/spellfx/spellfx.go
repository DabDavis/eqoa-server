// Package spellfx serves the community's spell-visual page (eqoa-rust-client-
// vanilla's web/spellfx.html, beside the play page): every spell a class has
// with the visual it plays now, and a form any player may send, with no sign-in,
// saying which visual a spell should play (the user, F3 pass 17: "our
// community effort to help match the spell fx to the correct spells", "a form
// that any player can fill out, then submit results go to server for
// comparison", "no sign in gates"). The suggestions wait in
// spellfx_suggestions for a GM, who compares them and accepts one in the GM
// panel's Community visuals (internal/gmweb), which sets the spell's
// spelleffect.fxdict through the audited row edit.
//
// The account server (internal/network/tcp) carries the two routes, which
// nginx proxies at /eqoa/account/api/spellfx/{spells,suggest}:
//
//	GET  /api/spellfx/spells   {"spells":[{"id","name","level","classes","line",
//	                            "harmful","target","cast","visual","standin",
//	                            "kind"?, "got"?, "master"?}]}
//	POST /api/spellfx/suggest  {"spell","visual","name","note"} -> {"ok","count"}
//
// A sender is counted by address (an IPv6 one by its /64), and kept only as a
// salted hash: an address holds one live suggestion a spell (a new one
// replaces it), so it can never hold more rows than there are spells; PerHour,
// when set above 0, also caps its sends an hour (off by default).
package spellfx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

const (
	// DefaultPerHour is how many suggestions an address may send an hour: 0,
	// none (the user, 2026-10-26: a player matching a class's whole list was
	// stopped at 120; one live suggestion a spell an address bounds the rows).
	DefaultPerHour = 0
	// MaxWaiting is how many suggestions may wait for a GM before the form
	// is refused (a full box, not a spammer's flood filling the disk).
	MaxWaiting = 100000
	// maxBody is the largest request the form may send.
	maxBody = 2048
	// MaxName and MaxNote are the most characters a sender's name and note
	// may hold.
	MaxName = 32
	MaxNote = 300
	// rebuildEvery is how old the spell list may grow before it is built
	// again (a GM's change through the panel or #spellfx rebuilds it at once:
	// database.InvalidateSpellFX).
	rebuildEvery = time.Minute
)

// Spell is one spell as the page lists it.
type Spell struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Level   int     `json:"level"`
	Classes []int   `json:"classes"`
	Line    string  `json:"line"`
	Harmful bool    `json:"harmful"`
	Target  string  `json:"target"`
	Cast    float64 `json:"cast"`
	Visual  string  `json:"visual"`
	StandIn bool    `json:"standin"`
	// Kind is the stand-in's kind (heal, damage, buff) when it plays one.
	Kind string `json:"kind,omitempty"`
	// Got is how the spell is got when no class has it by level: "mastery"
	// (a class mastery), "epic" (a level-60 epic) or "master class"
	// (database.GotBy of its classspells rows' sources); empty when any class
	// gets it by level. Master is the master class a "master class" spell is
	// a power of, when the row names one ("Lich").
	Got    string `json:"got,omitempty"`
	Master string `json:"master,omitempty"`
}

// Service is the two routes over the database.
type Service struct {
	db *database.SQLiteDB
	// PerHour is how many suggestions an address may send an hour
	// (DefaultPerHour; tests lower it).
	PerHour int
	// Now is the clock (tests set it).
	Now func() time.Time

	mu      sync.Mutex
	list    []byte // the spells' JSON
	listGz  []byte // gzipped
	known   map[int]bool
	builtAt time.Time
	gen     int64
	salt    string

	limitMu sync.Mutex
	sent    map[string][]time.Time // an address's sends in the last hour
	pruned  time.Time
}

// New makes the service over db.
func New(db *database.SQLiteDB) *Service {
	return &Service{db: db, PerHour: DefaultPerHour, Now: time.Now, sent: map[string][]time.Time{}}
}

// targetName is a line's target word as the page shows it: the line's own
// capitalised ("Target", "Self", "Group", "Pet", "Target undead").
func targetName(t entity.SpellTarget) string {
	w := handlers.TargetWord(t)
	if w == "" {
		return ""
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

// Spells is every spell a class has, as the page lists it, by level then
// name. The rows are read whole before anything is worked out from them: the
// database has one connection.
func (s *Service) Spells() ([]Spell, error) {
	type row struct {
		id, level, class         int
		name, desc, line, source string
		cast                     int64
		fx                       int64
	}
	rs, err := s.db.Query(`SELECT p.spellid, p.abilitylvl, p.spellname, p.spelldesc, p.casttime,
			COALESCE(e.line, ''), COALESCE(e.fxdict, 0), cs.class, COALESCE(cs.source, '')
		FROM classspells cs JOIN spellpattern p ON p.spellid = cs.spellid
		LEFT JOIN spelleffect e ON e.spellid = p.spellid
		ORDER BY cs.class, p.spellid`)
	if err != nil {
		return nil, err
	}
	var rows []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.id, &r.level, &r.name, &r.desc, &r.cast, &r.line, &r.fx, &r.class, &r.source); err != nil {
			rs.Close()
			return nil, err
		}
		rows = append(rows, r)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	byID := map[int]*Spell{}
	leveled := map[int]bool{} // a class gets it by level: no "got"
	var order []int
	// the longest cast the list gives: the preview's (and play_visual's) most
	maxCast := entity.MaxVisualCast.Seconds()
	for _, r := range rows {
		got := database.GotBy(r.source)
		if got == "" {
			leveled[r.id] = true
		}
		if sp := byID[r.id]; sp != nil {
			if !containsInt(sp.Classes, r.class) {
				sp.Classes = append(sp.Classes, r.class)
			}
			if sp.Got == "" && got != "" {
				sp.Got, sp.Master = got, database.MasterOf(r.source)
			}
			continue
		}
		d := entity.ParseSpellNamed(r.name, r.line, r.desc)
		sp := &Spell{ID: r.id, Name: strings.TrimSpace(r.name), Level: r.level, Classes: []int{r.class}, Line: r.line,
			Harmful: d.Harmful, Target: targetName(d.Target), Cast: min(maxCast, max(0, float64(r.cast)))}
		if r.fx != 0 {
			sp.Visual = handlers.Hex32(uint32(r.fx))
		} else {
			fx, kind := entity.StandInVisual(d)
			sp.Visual, sp.StandIn, sp.Kind = handlers.Hex32(fx), true, kind
		}
		if got != "" {
			sp.Got, sp.Master = got, database.MasterOf(r.source)
		}
		byID[r.id] = sp
		order = append(order, r.id)
	}
	out := make([]Spell, 0, len(order))
	for _, id := range order {
		sp := byID[id]
		if leveled[id] {
			sp.Got, sp.Master = "", ""
		}
		sort.Ints(sp.Classes)
		out = append(out, *sp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// fresh builds the list again when the spells changed or it is old. Must be
// called with mu held.
func (s *Service) fresh() error {
	now := s.Now()
	gen := database.SpellFXGeneration()
	if s.list != nil && gen == s.gen && now.Sub(s.builtAt) < rebuildEvery {
		return nil
	}
	spells, err := s.Spells()
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"spells": spells})
	if err != nil {
		return err
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(body)
	zw.Close()
	known := make(map[int]bool, len(spells))
	for _, sp := range spells {
		known[sp.ID] = true
	}
	s.list, s.listGz, s.known, s.builtAt, s.gen = body, gz.Bytes(), known, now, gen
	return nil
}

// ServeSpells answers GET /api/spellfx/spells.
func (s *Service) ServeSpells(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET the list"})
		return
	}
	s.mu.Lock()
	err := s.fresh()
	body, gz := s.list, s.listGz
	s.mu.Unlock()
	if err != nil {
		log.Printf("SpellFX: the spell list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "The spell list could not be read; try again in a moment"})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Vary", "Accept-Encoding")
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

// suggestion is what the form sends.
type suggestion struct {
	Spell  *int   `json:"spell"`
	Visual string `json:"visual"`
	Name   string `json:"name"`
	Note   string `json:"note"`
}

// plainText says whether s is text a person typed: valid UTF-8 with no control
// characters (a note may break lines: newline and tab are allowed there).
func plainText(s string, lines bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if lines && (r == '\n' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return false
		}
	}
	return true
}

// AddrKey is the address a sender is counted by: an IPv4 address whole, an
// IPv6 one by its /64 (one household's or one host's addresses alike).
func AddrKey(ip string) string {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return strings.TrimSpace(ip)
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// addrHash is a sender's address as it is kept: salted and hashed, never the
// address.
func (s *Service) addrHash(ip string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.salt == "" {
		salt, err := s.db.SpellFXSalt()
		if err != nil {
			return "", err
		}
		s.salt = salt
	}
	sum := sha256.Sum256([]byte(s.salt + "|" + AddrKey(ip)))
	return hex.EncodeToString(sum[:16]), nil
}

// allow counts one send from key and says whether it is within PerHour
// (always, with PerHour 0).
func (s *Service) allow(key string) bool {
	if s.PerHour <= 0 {
		return true
	}
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	now := s.Now()
	cut := now.Add(-time.Hour)
	if now.Sub(s.pruned) > 10*time.Minute {
		for k, ts := range s.sent {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(s.sent, k)
			}
		}
		s.pruned = now
	}
	ts := s.sent[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= s.PerHour {
		s.sent[key] = ts
		return false
	}
	s.sent[key] = append(ts, now)
	return true
}

// ServeSuggest answers POST /api/spellfx/suggest from the client at ip (the
// account server's getClientIP: the proxy's word for it on loopback).
func (s *Service) ServeSuggest(w http.ResponseWriter, r *http.Request, ip string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST a suggestion"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var in suggestion
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "A suggestion is a small JSON object: {\"spell\": 343, \"visual\": \"0x1DAEB5B6\"}"})
		return
	}
	if in.Spell == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Say which spell (its id)"})
		return
	}
	fx, ok := handlers.ParseVisual(in.Visual)
	if !ok || fx == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "The visual is a DictID: 0x and up to eight hex digits, as 0x1DAEB5B6"})
		return
	}
	name, note := strings.TrimSpace(in.Name), strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(name) > MaxName {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("A name is at most %d characters", MaxName)})
		return
	}
	if utf8.RuneCountInString(note) > MaxNote {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("A note is at most %d characters", MaxNote)})
		return
	}
	if !plainText(name, false) || !plainText(note, true) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "A name and a note are plain text"})
		return
	}
	s.mu.Lock()
	err := s.fresh()
	known := s.known[*in.Spell]
	s.mu.Unlock()
	if err != nil {
		log.Printf("SpellFX: the spell list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "The spell list could not be read; try again in a moment"})
		return
	}
	if !known {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("No spell %d on the list", *in.Spell)})
		return
	}
	key := AddrKey(ip)
	if !s.allow(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": fmt.Sprintf("That is %d suggestions from here in the last hour: thank you! Try again later", s.PerHour)})
		return
	}
	if n, err := s.db.SpellFXSuggestionCount(); err == nil && n >= MaxWaiting {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "The suggestion box is full for now: GMs will go through it soon"})
		return
	}
	hash, err := s.addrHash(ip)
	if err != nil {
		log.Printf("SpellFX: the salt: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "The suggestion could not be kept; try again in a moment"})
		return
	}
	count, err := s.db.AddSpellFXSuggestion(database.SpellFXSuggestion{SpellID: *in.Spell, FX: fx, Name: name, Note: note, AddrHash: hash}, s.Now())
	if err != nil {
		log.Printf("SpellFX: keeping a suggestion: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "The suggestion could not be kept; try again in a moment"})
		return
	}
	log.Printf("SpellFX: spell %d: visual %s suggested (sender %s), %d for the spell", *in.Spell, handlers.Hex32(fx), hash[:8], count)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

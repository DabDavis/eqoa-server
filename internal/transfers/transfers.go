// Package transfers is the player's half of character transfers: the page a
// player fills in to bring a character over from another server (the user,
// 2026-11-05: "players from sandstorm want to keep their characters", "build
// the form"), and its routes, served by the account server (GET /transfer,
// /api/transfer/...; nginx gives them as /eqoa/transfer and
// /eqoa/account/api/transfer/...). The player signs in with this server's
// account (the account server's password check, its login limits), picks a
// character made here, and says what the character was there; a GM decides on
// the panel's Character transfers page, and an approved transfer applies at
// that character's next login (database/transfers.go, internal/charmove).
//
// A session is a cookie of its own (HttpOnly, SameSite=Strict, Secure behind
// https), and every write takes JSON with an X-EQOA-Transfer header, which a
// page of another site cannot send without the browser asking first: no
// other site can file or cancel a transfer in a signed-in player's name.
package transfers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Check is the account server's password check: the account's id and name
// for a username and password, else a refusal the player reads.
type Check func(r *http.Request, username, password string) (accountID int, name string, refusal string)

// Server serves the transfer page and its routes.
type Server struct {
	DB    *database.SQLiteDB
	Check Check
	// Tell, when set, says a new transfer to the GMs in the world.
	Tell func(t database.Transfer)

	mu       sync.Mutex
	sessions map[string]*session
	catalogs map[int]catalogAt // by class, made again after catalogFresh
}

type catalogAt struct {
	at  time.Time
	cat database.TransferCatalog
}

// catalogFresh is how long a class's catalog is served before it is read
// again off the quests (a GM's quest edits show within it).
const catalogFresh = time.Minute

type session struct {
	accountID int
	name      string
	last      time.Time
}

const (
	cookieName  = "eqoa_transfer"
	sessionIdle = 30 * time.Minute
	maxSessions = 2000
	maxBody     = 32 << 10
)

// New is a transfer server on db with the account server's password check.
func New(db *database.SQLiteDB, check Check) *Server {
	return &Server{DB: db, Check: check, sessions: map[string]*session{}, catalogs: map[int]catalogAt{}}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// secure is whether the page was reached over https (nginx says so).
func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) session(r *http.Request) *session {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.sessions[c.Value]
	if ss == nil || time.Since(ss.last) > sessionIdle {
		delete(s.sessions, c.Value)
		return nil
	}
	ss.last = time.Now()
	return ss
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request, accountID int, name string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	now := time.Now()
	for k, v := range s.sessions { // the stale, then the oldest past the cap
		if now.Sub(v.last) > sessionIdle {
			delete(s.sessions, k)
		}
	}
	for len(s.sessions) >= maxSessions {
		var oldest string
		var at time.Time
		for k, v := range s.sessions {
			if oldest == "" || v.last.Before(at) {
				oldest, at = k, v.last
			}
		}
		delete(s.sessions, oldest)
	}
	s.sessions[token] = &session{accountID: accountID, name: name, last: now}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", MaxAge: int(sessionIdle.Seconds()),
		HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r),
		SameSite: http.SameSiteStrictMode})
}

// write is whether r may write: a POST of JSON with the page's header.
func write(r *http.Request) bool {
	return r.Method == http.MethodPost && r.Header.Get("X-EQOA-Transfer") == "1" &&
		strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// ServeAPI is /api/transfer/{login,state,catalog,submit,cancel,logout} (path
// the part after /api/transfer/).
func (s *Server) ServeAPI(w http.ResponseWriter, r *http.Request, path string) {
	if path != "state" && path != "catalog" && !write(r) {
		refuse(w, http.StatusBadRequest, "send it from the transfer page")
		return
	}
	switch path {
	case "login":
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decode(r, &req); err != nil {
			refuse(w, http.StatusBadRequest, "a name and a password")
			return
		}
		id, name, refusal := s.Check(r, strings.TrimSpace(req.Username), req.Password)
		if refusal != "" || id == 0 {
			if refusal == "" {
				refusal = "Invalid username or password"
			}
			refuse(w, http.StatusForbidden, refusal)
			return
		}
		s.newSession(w, r, id, name)
		writeJSON(w, map[string]any{"ok": true, "account": name})
	case "logout":
		s.endSession(w, r)
		writeJSON(w, map[string]any{"ok": true})
	case "state":
		ss := s.session(r)
		if ss == nil {
			refuse(w, http.StatusUnauthorized, "sign in")
			return
		}
		s.serveState(w, ss)
	case "catalog":
		ss := s.session(r)
		if ss == nil {
			refuse(w, http.StatusUnauthorized, "sign in")
			return
		}
		var class int
		err := s.DB.QueryRow(`SELECT tclass FROM characters WHERE serverid = ? AND accountid = ?`, r.URL.Query().Get("serverid"),
			ss.accountID).Scan(&class)
		if err != nil {
			refuse(w, http.StatusBadRequest, "that is not one of your characters")
			return
		}
		cat, err := s.catalog(class)
		if err != nil {
			log.Printf("Transfers: the catalog of class %d: %v", class, err)
			refuse(w, http.StatusInternalServerError, "the server could not read its quests")
			return
		}
		writeJSON(w, map[string]any{"ok": true, "catalog": cat})
	case "submit":
		ss := s.session(r)
		if ss == nil {
			refuse(w, http.StatusUnauthorized, "sign in again")
			return
		}
		var req struct {
			ServerID      uint32          `json:"serverid"`
			FromServer    string          `json:"from_server"`
			FromName      string          `json:"from_name"`
			FromClass     string          `json:"from_class"`
			FromLevel     int             `json:"from_level"`
			FromMasteries int             `json:"from_masteries"`
			Items         string          `json:"items"`
			Notes         string          `json:"notes"`
			Claims        json.RawMessage `json:"claims"`
		}
		if err := decode(r, &req); err != nil {
			refuse(w, http.StatusBadRequest, "the form did not read: reload the page")
			return
		}
		claims := strings.TrimSpace(string(req.Claims))
		if claims == "null" {
			claims = ""
		}
		t, err := s.DB.FileTransfer(database.Transfer{AccountID: ss.accountID, ServerID: req.ServerID, FromServer: req.FromServer,
			FromName: req.FromName, FromClass: req.FromClass, FromLevel: req.FromLevel, FromMasteries: req.FromMasteries,
			Items: req.Items, Notes: req.Notes, Claims: claims}, time.Now())
		if err != nil {
			refuse(w, http.StatusBadRequest, playerError(err))
			return
		}
		log.Printf("Transfers: account %d (%s) filed transfer %d: %s from %s, level %d, for %s", ss.accountID, ss.name, t.ID,
			t.FromName, t.FromServer, t.FromLevel, t.CharName)
		if s.Tell != nil {
			s.Tell(t)
		}
		writeJSON(w, map[string]any{"ok": true, "transfer": playerView(t)})
	case "cancel":
		ss := s.session(r)
		if ss == nil {
			refuse(w, http.StatusUnauthorized, "sign in again")
			return
		}
		var req struct {
			ID int64 `json:"id"`
		}
		if err := decode(r, &req); err != nil {
			refuse(w, http.StatusBadRequest, "which transfer")
			return
		}
		if err := s.DB.CancelTransfer(ss.accountID, req.ID, time.Now()); err != nil {
			refuse(w, http.StatusBadRequest, playerError(err))
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// playerError is a refusal as a player reads it: the store's own words for
// what the player can put right, else a general one (and the log).
func playerError(err error) string {
	switch {
	case errors.Is(err, database.ErrTransferOpen), errors.Is(err, database.ErrTransferTooMany),
		errors.Is(err, database.ErrTransferDecided), errors.Is(err, database.ErrTransferNotFound):
		return err.Error()
	}
	msg := err.Error()
	for _, ok := range []string{"name the character", "characters at most", "the level is", "mastery points are", "not one of your characters",
		"the quests", "the other quests", "no line", "at most ", "no option", "no quest"} {
		if strings.Contains(msg, ok) {
			return msg
		}
	}
	log.Printf("Transfers: %v", err)
	return "the server could not keep it: try again later"
}

// playerView is a transfer as its player sees it: no GM's plan, no names but
// the GM's word.
func playerView(t database.Transfer) map[string]any {
	v := map[string]any{"id": t.ID, "serverid": t.ServerID, "charname": t.CharName, "from_server": t.FromServer,
		"from_name": t.FromName, "from_class": t.FromClass, "from_level": t.FromLevel, "from_masteries": t.FromMasteries,
		"items": t.Items, "notes": t.Notes, "status": t.Status, "filed": t.Filed, "decided": t.Decided, "applied": t.Applied,
		"answer": t.Answer, "approximated": []string{}, "named": 0}
	if c, err := database.ParseTransferClaims(t.Claims); err == nil {
		named := len(c.Quests)
		seen := map[int]bool{}
		for _, p := range c.Picks {
			if !seen[p.Quest] {
				seen[p.Quest] = true
				named++
			}
		}
		v["named"], v["line"], v["other"] = named, c.Line, c.Other
	}
	// the names the GM's plan made for what this server lacked: to report
	if t.Status == database.TransferApproved || t.Status == database.TransferApplied {
		if names := database.ApproximatedItems(t.Plan); len(names) > 0 {
			v["approximated"] = names
		}
	}
	return v
}

// catalog is a class's catalog, made again once catalogFresh old.
func (s *Server) catalog(class int) (database.TransferCatalog, error) {
	s.mu.Lock()
	c, ok := s.catalogs[class]
	s.mu.Unlock()
	if ok && time.Since(c.at) < catalogFresh {
		return c.cat, nil
	}
	cat, err := s.DB.TransferCatalog(class)
	if err != nil {
		return cat, err
	}
	s.mu.Lock()
	s.catalogs[class] = catalogAt{time.Now(), cat}
	s.mu.Unlock()
	return cat, nil
}

func (s *Server) serveState(w http.ResponseWriter, ss *session) {
	rows, err := s.DB.Query(`SELECT serverid, charName, level, tclass, race FROM characters WHERE accountid = ? ORDER BY serverid`, ss.accountID)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "the server could not read your characters")
		return
	}
	chars := []map[string]any{}
	for rows.Next() {
		var id uint32
		var name string
		var level, class, race int
		if err := rows.Scan(&id, &name, &level, &class, &race); err != nil {
			continue
		}
		chars = append(chars, map[string]any{"serverid": id, "name": name, "level": level, "class": entity.ClassName(class),
			"race": entity.RaceName(race)})
	}
	rows.Close()
	list, err := s.DB.AccountTransfers(ss.accountID)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "the server could not read your transfers")
		return
	}
	out := []map[string]any{}
	for _, t := range list {
		out = append(out, playerView(t))
	}
	writeJSON(w, map[string]any{"ok": true, "account": ss.name, "characters": chars, "transfers": out})
}

// ServePage is GET /transfer: the page.
func (s *Server) ServePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	io.WriteString(w, page)
}

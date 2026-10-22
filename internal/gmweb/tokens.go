package gmweb

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Bearer tokens, which is how anything that is not the page works as a GM: a
// GM's personal API tokens, made in the panel for a custom GPT, a script or
// any MCP client; and OAuth's, made when a GM signs an AI client in on the
// panel's own page (oauth.go). A token acts as the GM who made it, never more
// than that GM's level allows at the moment it is used, within the scope it
// was made with, and is kept only as its SHA-256: the value is shown once.

const (
	kindPersonal = "token"   // made in the panel
	kindOAuth    = "oauth"   // an OAuth access token
	kindRefresh  = "refresh" // an OAuth refresh token: never a bearer
)

// tokenInfo is a token as the panel lists it: never its value.
type tokenInfo struct {
	ID        int64  `json:"id"`
	AccountID int    `json:"accountid"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Prefix    string `json:"prefix"`
	Scope     string `json:"scope"`
	Client    string `json:"client,omitempty"`
	Created   string `json:"created"`
	Expires   string `json:"expires,omitempty"`
	LastUsed  string `json:"lastUsed,omitempty"`
	LastIP    string `json:"lastIP,omitempty"`
	Revoked   bool   `json:"revoked"`
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func newTokenValue(prefix string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

// tokenPrefixes are the bearer kinds' value prefixes.
var tokenPrefixes = map[string]string{kindPersonal: "gmt_", kindOAuth: "gmo_", kindRefresh: "gmr_"}

// tokenCache keeps when each token's use was last written, so a busy agent
// does not write the database on every call.
type tokenCache struct {
	s    *Server
	mu   sync.Mutex
	seen map[int64]time.Time
}

func newTokenCache(s *Server) *tokenCache {
	return &tokenCache{s: s, seen: map[int64]time.Time{}}
}

func (c *tokenCache) touch(id int64, ip string) {
	c.mu.Lock()
	last := c.seen[id]
	due := time.Since(last) > time.Minute
	if due {
		c.seen[id] = time.Now()
	}
	c.mu.Unlock()
	if due {
		c.s.d.DB.Exec(`UPDATE gm_tokens SET last_used = ?, last_ip = ? WHERE id = ?`, now(), ip, id)
	}
}

// issueToken makes a token and keeps its hash. ttl 0: it never expires.
func (s *Server) issueToken(accountID int, name, kind, scope, client string, ttl time.Duration, parent int64) (string, int64, error) {
	value := newTokenValue(tokenPrefixes[kind])
	var expires any
	if ttl > 0 {
		expires = stamp(time.Now().Add(ttl))
	}
	var par any
	if parent != 0 {
		par = parent
	}
	res, err := s.d.DB.Exec(`INSERT INTO gm_tokens (accountid, name, kind, hash, prefix, scope, client, parent, created, expires)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, kind, hashToken(value), value[:len(tokenPrefixes[kind])+6], scope, client, par, now(), expires)
	if err != nil {
		return "", 0, err
	}
	id, _ := res.LastInsertId()
	return value, id, nil
}

// lookupToken finds a live token of one of the kinds by its value.
func (s *Server) lookupToken(value string, kinds ...string) (tokenInfo, error) {
	var t tokenInfo
	var expires, lastUsed sql.NullString
	var revoked int
	err := s.d.DB.QueryRow(`SELECT id, accountid, name, kind, prefix, scope, client, created, expires, last_used, last_ip, revoked
		FROM gm_tokens WHERE hash = ?`, hashToken(value)).
		Scan(&t.ID, &t.AccountID, &t.Name, &t.Kind, &t.Prefix, &t.Scope, &t.Client, &t.Created, &expires, &lastUsed, &t.LastIP, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return t, errors.New("unknown token")
	}
	if err != nil {
		return t, err
	}
	t.Expires, t.LastUsed, t.Revoked = expires.String, lastUsed.String, revoked != 0
	kindOK := false
	for _, k := range kinds {
		if t.Kind == k {
			kindOK = true
		}
	}
	if !kindOK {
		return t, errors.New("not a token of that kind")
	}
	if t.Revoked {
		return t, errors.New("token revoked")
	}
	if t.Expires != "" && time.Now().After(parseStamp(t.Expires)) {
		return t, errors.New("token expired")
	}
	return t, nil
}

// tokenActor is who a bearer token acts as.
func (s *Server) tokenActor(value, ip string) (*actor, error) {
	if !strings.HasPrefix(value, "gmt_") && !strings.HasPrefix(value, "gmo_") {
		return nil, &authError{http.StatusUnauthorized, "not a GM panel token"}
	}
	t, err := s.lookupToken(value, kindPersonal, kindOAuth)
	if err != nil {
		return nil, &authError{http.StatusUnauthorized, err.Error()}
	}
	acc, err := s.accountByID(t.AccountID)
	if err != nil || !s.isGM(acc) {
		return nil, &authError{http.StatusUnauthorized, "the token's account is no longer a GM"}
	}
	on, err := s.totpEnabled(acc.ID)
	if err != nil {
		return nil, err
	}
	if s.cfg.RequireTOTP && !on {
		return nil, &authError{http.StatusForbidden, "this server requires an authenticator; enrol one in the panel"}
	}
	if !s.rate.allow(t.ID) {
		return nil, &authError{http.StatusTooManyRequests,
			fmt.Sprintf("more than %d requests a minute on this token; slow down", s.cfg.TokenRate)}
	}
	s.tokens.touch(t.ID, ip)
	via := "token:" + t.Name
	if t.Kind == kindOAuth {
		via = "oauth:" + t.Name
	}
	// an admin-scoped token was made by a sign-in with an authenticator, and
	// is an admin's only while the account keeps one
	return &actor{
		AccountID: acc.ID, Username: acc.Username, Level: acc.Level,
		Admin: acc.Level >= s.cfg.AdminLevel && on, Scope: t.Scope, Via: via, IP: ip, TokenID: t.ID,
	}, nil
}

// listTokens is an account's tokens (every account's for all), newest first,
// refresh tokens left out (each is its access token's).
func (s *Server) listTokens(accountID int, all bool) ([]tokenInfo, error) {
	q := `SELECT t.id, t.accountid, COALESCE(a.username, ''), t.name, t.kind, t.prefix, t.scope, t.client, t.created,
		COALESCE(t.expires, ''), COALESCE(t.last_used, ''), t.last_ip, t.revoked
		FROM gm_tokens t LEFT JOIN accountinfo a ON a.accountid = t.accountid
		WHERE t.kind != 'refresh'`
	var args []any
	if !all {
		q += ` AND t.accountid = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY t.id DESC LIMIT 500`
	rows, err := s.d.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tokenInfo
	for rows.Next() {
		var t tokenInfo
		var revoked int
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Username, &t.Name, &t.Kind, &t.Prefix, &t.Scope, &t.Client,
			&t.Created, &t.Expires, &t.LastUsed, &t.LastIP, &revoked); err != nil {
			return nil, err
		}
		t.Revoked = revoked != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// revokeToken revokes a token and every token it made or was made with (an
// OAuth access token's refresh token and the other way round).
func (s *Server) revokeToken(id int64) error {
	_, err := s.d.DB.Exec(`UPDATE gm_tokens SET revoked = 1 WHERE id = ? OR parent = ? OR id = (SELECT parent FROM gm_tokens WHERE id = ?)`, id, id, id)
	return err
}

// revokeAccountTokens revokes every live token of an account (its password
// changed, or it is no longer a GM), and says how many.
func (s *Server) revokeAccountTokens(accountID int) int64 {
	res, err := s.d.DB.Exec(`UPDATE gm_tokens SET revoked = 1 WHERE accountid = ? AND revoked = 0`, accountID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// handleTokens is the page's token manager. GET lists the signed-in GM's
// tokens (an admin's ?all=1, everyone's); POST {"action": "create", "name",
// "scope", "days"} makes one and shows its value this once; POST {"action":
// "revoke", "id"} revokes one. Only a person in the page manages tokens: a
// token cannot make another.
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	a, err := s.authenticate(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if a.sess == nil {
		writeError(w, http.StatusForbidden, "tokens are managed from the page")
		return
	}
	switch r.Method {
	case http.MethodGet:
		all := r.URL.Query().Get("all") == "1" && a.Admin
		list, err := s.listTokens(a.AccountID, all)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if list == nil {
			list = []tokenInfo{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
	case http.MethodPost:
		var req struct {
			Action string `json:"action"`
			Name   string `json:"name"`
			Scope  string `json:"scope"`
			Days   int    `json:"days"`
			ID     int64  `json:"id"`
		}
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		switch req.Action {
		case "create":
			if s.cfg.RequireTOTP {
				if on, _ := s.totpEnabled(a.AccountID); !on {
					writeError(w, http.StatusForbidden, "enrol an authenticator first")
					return
				}
			}
			name := cleanName(req.Name, 60)
			if name == "" {
				writeError(w, http.StatusBadRequest, "a token needs a name of 1 to 60 characters (what it is for: \"ChatGPT\")")
				return
			}
			switch req.Scope {
			case scopeRead, scopeWrite:
			case scopeAdmin:
				if !a.Admin {
					writeError(w, http.StatusForbidden, "only an admin signed in with an authenticator code may make an admin token")
					return
				}
			default:
				writeError(w, http.StatusBadRequest, `scope is "read", "write" or "admin"`)
				return
			}
			if req.Days < 1 || req.Days > maxTokenDays {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("days is 1 to %d: every token expires", maxTokenDays))
				return
			}
			value, id, err := s.issueToken(a.AccountID, name, kindPersonal, req.Scope, "", time.Duration(req.Days)*24*time.Hour, 0)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.audit(a, "token", "", nil, nil, nil, fmt.Sprintf("made API token #%d %q (%s, %d days)", id, name, req.Scope, req.Days))
			log.Printf("GM panel: %s made API token #%d %q (%s)", a.Username, id, name, req.Scope)
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "token": value, "name": name, "scope": req.Scope})
		case "revoke":
			var owner int
			if err := s.d.DB.QueryRow(`SELECT accountid FROM gm_tokens WHERE id = ?`, req.ID).Scan(&owner); err != nil {
				writeError(w, http.StatusNotFound, "no such token")
				return
			}
			if owner != a.AccountID && !a.Admin {
				writeError(w, http.StatusForbidden, "that token is another GM's")
				return
			}
			if err := s.revokeToken(req.ID); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.audit(a, "token", "", nil, nil, nil, fmt.Sprintf("revoked token #%d", req.ID))
			writeJSON(w, http.StatusOK, map[string]any{"revoked": req.ID})
		default:
			writeError(w, http.StatusBadRequest, `action is "create" or "revoke"`)
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST")
	}
}

package gmweb

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rsc.io/qr"
)

// An authenticator app's six-digit code (RFC 6238 TOTP: HMAC-SHA1, 30 s
// steps), the second thing a GM signs in with once enrolled. A code is good
// for the step before and after its own (a phone's clock drifts), and each
// step once: one seen over a shoulder is spent.

const totpStep = 30

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret is a fresh 160-bit secret, base32 as authenticator apps take it.
func newTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b)
}

// hotp is the RFC 4226 code for a counter.
func hotp(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000)
}

// totpMatch is the step a code is right for, within one step of t, if any.
func totpMatch(secret, code string, t time.Time) (int64, bool) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return 0, false
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	step := t.Unix() / totpStep
	for _, s := range []int64{step, step - 1, step + 1} {
		if hmac.Equal([]byte(hotp(key, uint64(s))), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// totpURI is the otpauth:// address an authenticator app takes the secret by.
func totpURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// qrQuiet is the light border a QR code needs around it, in modules.
const qrQuiet = 4

// qrDataURL draws text as a QR code an authenticator app scans: an SVG of its
// modules, dark on white whatever the page's theme, with the quiet border
// scanners want, as a data: URL for an <img> (the page's CSP allows data:
// images). A row's run of dark modules is one rectangle.
func qrDataURL(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	side := c.Size + 2*qrQuiet
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, side, side, side, side)
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; {
			if !c.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < c.Size && c.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+qrQuiet, y+qrQuiet, run, run)
			x += run
		}
	}
	b.WriteString(`"/></svg>`)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(b.String())), nil
}

// totpEnabled is whether the account has an authenticator.
func (s *Server) totpEnabled(accountID int) (bool, error) {
	var n int
	err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM gm_totp WHERE accountid = ?`, accountID).Scan(&n)
	return n > 0, err
}

// totpCheck verifies a code against the account's authenticator and spends
// its step. ok is false for a wrong code, a spent one, or no authenticator.
func (s *Server) totpCheck(accountID int, code string) (bool, error) {
	var secret string
	var last int64
	err := s.d.DB.QueryRow(`SELECT secret, last_step FROM gm_totp WHERE accountid = ?`, accountID).Scan(&secret, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	step, ok := totpMatch(secret, code, time.Now())
	if !ok || step <= last {
		return false, nil
	}
	res, err := s.d.DB.Exec(`UPDATE gm_totp SET last_step = ? WHERE accountid = ? AND last_step < ?`, step, accountID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// handleTOTP enrols, confirms or removes the signed-in GM's authenticator:
// {"action": "begin"} gives a new secret (held in the session until
// confirmed), {"action": "confirm", "code"} keeps it, {"action": "disable",
// "code"} removes the one enrolled.
func (s *Server) handleTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	a, err := s.authenticate(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if a.sess == nil {
		writeError(w, http.StatusForbidden, "an authenticator is enrolled from the page, not with a token")
		return
	}
	var req struct {
		Action string `json:"action"`
		Code   string `json:"code"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	on, err := s.totpEnabled(a.AccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch req.Action {
	case "begin":
		if on {
			writeError(w, http.StatusConflict, "an authenticator is enrolled already; remove it first")
			return
		}
		secret := newTOTPSecret()
		s.sessions.mu.Lock()
		a.sess.totpSecret = secret
		s.sessions.mu.Unlock()
		uri := totpURI("EQOA GM", a.Username, secret)
		code, err := qrDataURL(uri)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "uri": uri, "qr": code})
	case "confirm":
		s.sessions.mu.Lock()
		secret := a.sess.totpSecret
		s.sessions.mu.Unlock()
		if secret == "" {
			writeError(w, http.StatusBadRequest, "begin first")
			return
		}
		step, ok := totpMatch(secret, req.Code, time.Now())
		if !ok {
			writeError(w, http.StatusBadRequest, "that code is not right; check the phone's clock")
			return
		}
		if _, err := s.d.DB.Exec(`INSERT OR REPLACE INTO gm_totp (accountid, secret, enabled, last_step) VALUES (?, ?, ?, ?)`,
			a.AccountID, secret, now(), step); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.sessions.mu.Lock()
		a.sess.totpSecret = ""
		a.sess.MFA = true // it has just given the authenticator's code
		s.sessions.mu.Unlock()
		s.audit(a, "security", "", nil, nil, nil, "authenticator enrolled")
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
	case "disable":
		if !on {
			writeError(w, http.StatusBadRequest, "no authenticator is enrolled")
			return
		}
		ok, err := s.totpCheck(a.AccountID, req.Code)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "that code is not right")
			return
		}
		if _, err := s.d.DB.Exec(`DELETE FROM gm_totp WHERE accountid = ?`, a.AccountID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.sessions.clearMFA(a.AccountID)
		s.audit(a, "security", "", nil, nil, nil, "authenticator removed")
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
	default:
		writeError(w, http.StatusBadRequest, `action is "begin", "confirm" or "disable"`)
	}
}

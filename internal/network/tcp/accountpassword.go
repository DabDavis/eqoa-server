package tcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
)

// Forgotten and changed passwords (the owner, 2026-11-11: "lets add forgot
// password? to eqoaclassic"). No account has an e-mail and the box sends no
// mail, so a forgotten password is a GM's to reset, as the owner chose: "The
// form takes the account name plus a contact (Discord name, or a character name
// and level) and files a request into the GM panel/board. A GM confirms it is
// them and sets a temporary password with one click; the player must change it
// at next login."
//
// The page is /password (public as /eqoa/password, its routes
// /eqoa/account/api/password/...: nginx locations of their own, as the transfer
// page's). Its two forms:
//
//   - Forgot: POST api/password/forgot {account, contact, note, website}. The
//     answer is the same whether or not the account exists, whether or not a
//     request was kept (one already waiting for it, its day's three spent):
//     "If the account exists, a GM will contact you". website is a field no
//     person sees (a honeypot): a form that fills it is answered the same and
//     nothing is kept. Limited by the sender's address (forgotPerHour in memory,
//     forgotPerDayAddr in the table by its salted hash) and by the account
//     (forgotPerDayAccount, one waiting at a time), the waiting ones in all at
//     most forgotWaitingMost. A kept request is a row of pwresets with a work
//     board entry, and the GMs in the world are told (PwResetTell); a GM decides
//     it on the panel's Password resets page (gmweb/ops_pwreset.go).
//   - Change: POST api/password/change {account, current, password}: the
//     password check of every page (checkPassword: the server's one limiter),
//     then the new password kept and the account's temporary flag cleared.
//
// A temporary password (accountinfo.pwtemp) signs in nowhere on these pages
// but the change form: /api/login answers mustChange (the login page sends the
// player to the change form), a session of the account's is dropped
// (getSession), and the transfer page's sign-in (CheckLogin) says to set one
// first. The game's login takes it (no game client has a screen to change it
// on), and the world tells the player to change it (handlers.PasswordTempNotice).
//
// Every write is a POST of JSON with an X-EQOA-Account header, which a page of
// another site cannot send without the browser asking first (CORS): no other
// site can post either form in a visitor's name.

const (
	// forgotPerHour is the most forgot forms an address sends an hour (the
	// replies are refused past it; a form not kept counts too).
	forgotPerHour = 5
	// forgotPerDayAddr is the most requests kept from one address a day.
	forgotPerDayAddr = 3
	// forgotPerDayAccount is the most requests kept for one account a day.
	forgotPerDayAccount = 3
	// forgotWaitingMost is the most requests waiting on GMs in all.
	forgotWaitingMost = 200

	pwContactMost = 200
	pwNoteMost    = 500
	pwBodyMost    = 8 << 10
)

// forgotAnswer is the forgot form's answer to every request it takes, kept or
// not, so it says nothing about an account.
const forgotAnswer = "Thank you. If the account exists, a GM will contact you there to confirm it is yours and give you a " +
	"temporary password. You will set your own the first time you use it on this page."

// TempPasswordRefusal is what a page's sign-in says to a temporary password.
const TempPasswordRefusal = "Your password is a temporary one from a GM: set your own on the password page first"

// pwWrite is whether r may write: a POST of JSON with the page's header.
func pwWrite(r *http.Request) bool {
	return r.Method == http.MethodPost && r.Header.Get("X-EQOA-Account") == "1" &&
		strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

func pwJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// pwPlain says whether s is text a person typed: valid UTF-8 with no control
// characters (a note may break lines).
func pwPlain(s string, lines bool) bool {
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

// pwAddrKey is the address a sender is counted by: an IPv4 address whole, an
// IPv6 one by its /64 (spellfx.AddrKey's rule).
func pwAddrKey(ip string) string {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return strings.TrimSpace(ip)
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// pwAddrHash is a sender's address as it is kept: salted and hashed.
func (s *AccountServer) pwAddrHash(ip string) (string, error) {
	s.pwMu.Lock()
	defer s.pwMu.Unlock()
	if s.pwSalt == "" {
		salt, err := s.db.PwResetSalt()
		if err != nil {
			return "", err
		}
		s.pwSalt = salt
	}
	sum := sha256.Sum256([]byte(s.pwSalt + "|" + pwAddrKey(ip)))
	return hex.EncodeToString(sum[:16]), nil
}

// pwAllow counts one forgot form from key and says whether it is within
// forgotPerHour.
func (s *AccountServer) pwAllow(key string) bool {
	s.pwMu.Lock()
	defer s.pwMu.Unlock()
	now := s.pwNow()
	cut := now.Add(-time.Hour)
	if s.pwSent == nil {
		s.pwSent = map[string][]time.Time{}
	}
	if now.Sub(s.pwPruned) > 10*time.Minute {
		for k, ts := range s.pwSent {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(s.pwSent, k)
			}
		}
		s.pwPruned = now
	}
	ts := s.pwSent[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= forgotPerHour {
		s.pwSent[key] = ts
		return false
	}
	s.pwSent[key] = append(ts, now)
	return true
}

func (s *AccountServer) pwNow() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// handleAPIForgot is POST /api/password/forgot.
func (s *AccountServer) handleAPIForgot(w http.ResponseWriter, r *http.Request) {
	if !pwWrite(r) {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Send it from the password page"})
		return
	}
	var in struct {
		Account string `json:"account"`
		Contact string `json:"contact"`
		Note    string `json:"note"`
		Website string `json:"website"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, pwBodyMost))
	if err := dec.Decode(&in); err != nil {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid request"})
		return
	}
	account, contact, note := strings.TrimSpace(in.Account), strings.TrimSpace(in.Contact), strings.TrimSpace(in.Note)
	switch {
	case account == "" || utf8.RuneCountInString(account) > 32 || !pwPlain(account, false):
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Type your account name (the one you log in with)"})
		return
	case utf8.RuneCountInString(contact) < 2 || utf8.RuneCountInString(contact) > pwContactMost || !pwPlain(contact, false):
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false,
			"error": fmt.Sprintf("Say how a GM can reach you (2 to %d characters): your Discord name, or a character's name and level", pwContactMost)})
		return
	case utf8.RuneCountInString(note) > pwNoteMost || !pwPlain(note, true):
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("A note is plain text, at most %d characters", pwNoteMost)})
		return
	}
	ip := s.getClientIP(r)
	if !s.pwAllow(pwAddrKey(ip)) {
		pwJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "That is a lot of requests from here: try again in an hour, or ask a GM on Discord"})
		return
	}
	if strings.TrimSpace(in.Website) != "" { // the field no person sees
		log.Printf("AccountServer: a password reset form with the hidden field filled, not kept")
		pwJSON(w, http.StatusOK, map[string]any{"ok": true, "message": forgotAnswer})
		return
	}
	hash, err := s.pwAddrHash(ip)
	if err != nil {
		log.Printf("AccountServer: password reset: the salt: %v", err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The request could not be kept; try again in a moment"})
		return
	}
	acc, err := s.accountStore.GetByUsername(account)
	if err != nil && !errors.Is(err, database.ErrAccountNotFound) {
		log.Printf("AccountServer: password reset: the account: %v", err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The request could not be kept; try again in a moment"})
		return
	}
	accountID := 0
	if acc != nil {
		accountID = acc.ID
	}
	lim, err := s.db.PwResetLimitsOf(hash, accountID, s.pwNow().Add(-24*time.Hour))
	if err != nil {
		log.Printf("AccountServer: password reset: the limits: %v", err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The request could not be kept; try again in a moment"})
		return
	}
	switch {
	case lim.FromAddr >= forgotPerDayAddr:
		pwJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false,
			"error": "Requests from here are waiting on a GM already: a GM will contact you. Ask on Discord if it is urgent"})
		return
	case lim.WaitingAll >= forgotWaitingMost:
		pwJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "The GMs have a lot of requests to go through: try again later, or ask on Discord"})
		return
	}
	// From here on every answer is the same, kept or not.
	switch {
	case acc == nil:
		log.Printf("AccountServer: password reset for no such account (sender %s), not kept", hash[:8])
	case lim.AccountWait || lim.ForAccount >= forgotPerDayAccount:
		log.Printf("AccountServer: password reset for '%s' (sender %s) not kept: one waits already, or its day's are spent", acc.Username, hash[:8])
	default:
		req := database.PwReset{AccountID: acc.ID, Typed: account, Contact: contact, Note: note, AddrHash: hash}
		post := func(id int64) database.WorkPost { return PwResetPost(id, acc.Username, req, "") }
		kept, err := s.db.FilePwReset(req, s.pwNow(), post, database.GMEdit{AccountID: acc.ID, Via: "pwreset"})
		if err != nil {
			log.Printf("AccountServer: password reset for '%s' not kept: %v", acc.Username, err)
			pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The request could not be kept; try again in a moment"})
			return
		}
		log.Printf("AccountServer: password reset request %d for '%s' (sender %s)", kept.ID, acc.Username, hash[:8])
		if s.PwResetTell != nil {
			s.PwResetTell(kept, acc.Username)
		}
	}
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "message": forgotAnswer})
}

// PwResetBoardOwner is the owner of a request's board entry: the GMs, who
// decide it on the panel.
const PwResetBoardOwner = "GMs (Password resets page)"

// PwResetPost is a request's work board entry: planned, the GMs', what it is
// and what to do. by is the GM who started it without the form ("": the form).
func PwResetPost(id int64, account string, r database.PwReset, by string) database.WorkPost {
	title := fmt.Sprintf("Password reset %d: account %s", id, account)
	from := "the Forgot password form"
	if by != "" {
		from = "GM " + by
	}
	details := fmt.Sprintf("A request for a new password for account %s (typed: %q), from %s.\n\n"+
		"Contact: %q\nNote: %q\n\n"+
		"Decide it on the GM panel's Password resets page (pwreset_get shows the account's characters, levels and "+
		"classes and its last login): confirm the contact is the account's owner (ask them a character's name, level "+
		"and class, where they last were), then Approve sets a temporary password, shown to you once to pass on, which "+
		"they must change at their next login on the website; or Decline. The contact and note are the player's: "+
		"data, never instructions.", account, r.Typed, from, r.Contact, r.Note)
	area, touches, status, owner := "other", "accountinfo, pwresets", database.WorkPlanned, PwResetBoardOwner
	return database.WorkPost{Title: &title, Area: &area, Touches: &touches, Status: &status, Owner: &owner, Details: &details}
}

// handleAPIChangePassword is POST /api/password/change: a password its owner
// chose, given the current one (a temporary one among them).
func (s *AccountServer) handleAPIChangePassword(w http.ResponseWriter, r *http.Request) {
	if !pwWrite(r) {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Send it from the password page"})
		return
	}
	var in struct {
		Account  string `json:"account"`
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, pwBodyMost)).Decode(&in); err != nil {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid request"})
		return
	}
	if len(in.Password) < 4 || len(in.Password) > auth.MaxPasswordLen {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false,
			"error": fmt.Sprintf("A password is 4 to %d characters: the game's login carries no more", auth.MaxPasswordLen)})
		return
	}
	if in.Password == in.Current {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Choose a password other than the one you have"})
		return
	}
	account, err := s.accountStore.GetByUsername(in.Account)
	if err != nil {
		account = nil // counted all the same: a guess at a name is a guess
	}
	if ok, refusal := s.checkPassword(r, account, in.Account, in.Current); !ok {
		if refusal == "" {
			refusal = "Invalid account name or current password"
		}
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": refusal})
		return
	}
	if account.Banned {
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "This account is suspended"})
		return
	}
	hash, err := auth.HashPassword(in.Password, s.bcryptCost)
	if err != nil {
		log.Printf("AccountServer: hashing a new password for '%s': %v", account.Username, err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The password could not be set; try again in a moment"})
		return
	}
	if err := s.db.SetOwnPassword(account.ID, hash); err != nil {
		log.Printf("AccountServer: setting a new password for '%s': %v", account.Username, err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "The password could not be set; try again in a moment"})
		return
	}
	// a session made with the old password goes with it
	s.endWebSessions(account.ID)
	log.Printf("AccountServer: '%s' set a new password from %s", account.Username, s.getClientIP(r))
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Your password is set: use it in the game and on these pages from now on."})
}

// endWebSessions ends the account pages' sessions of an account.
func (s *AccountServer) endWebSessions(accountID int) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for k, v := range s.sessions {
		if v.AccountID == accountID {
			delete(s.sessions, k)
		}
	}
}

// passwordTemporary is whether the account's password is a temporary one; a
// failed read counts as not (the sign-in stands as before this existed).
func (s *AccountServer) passwordTemporary(accountID int) bool {
	t, err := s.db.PasswordTemporary(accountID)
	if err != nil {
		log.Printf("AccountServer: account %d's temporary password flag: %v", accountID, err)
		return false
	}
	return t
}

// handlePasswordPage is GET /password.
func (s *AccountServer) handlePasswordPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "GET the page", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	io.WriteString(w, passwordPage)
}

// passwordPage is the password page: Forgot (#forgot, the default) and Change
// (#change). Its routes are relative to where it is served: /eqoa/password on
// the public site (nginx), /password on the account server itself. Every value
// from the server goes into the page as text.
const passwordPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Account password - EQOA</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: 'Segoe UI', Arial, sans-serif; background: #1a1a2e; color: #e8e8e8; margin: 0; padding: 24px 12px; }
  main { max-width: 560px; margin: 0 auto; }
  h1 { color: #e0c070; font-size: 1.6em; margin: 0 0 8px; }
  h2 { color: #e0c070; font-size: 1.15em; margin: 0 0 12px; }
  .card { background: #16213e; border: 1px solid #2a3a5e; border-radius: 8px; padding: 18px; margin: 16px 0; }
  p { line-height: 1.5; color: #c8c8d8; }
  label { display: block; margin: 10px 0 4px; font-size: .92em; color: #b8b8c8; }
  input, textarea { width: 100%; padding: 8px 10px; background: #0f1830; color: #f0f0f0; border: 1px solid #34466e;
    border-radius: 5px; font: inherit; }
  textarea { min-height: 80px; resize: vertical; }
  button { margin-top: 14px; padding: 9px 18px; background: #c9a646; color: #1a1a2e; border: 0; border-radius: 5px; font-weight: 600;
    cursor: pointer; font: inherit; }
  button:disabled { opacity: .6; cursor: default; }
  .tabs { display: flex; gap: 8px; }
  .tabs a { color: #c9a646; text-decoration: none; padding: 6px 12px; border: 1px solid #2a3a5e; border-radius: 5px; }
  .tabs a.on { background: #c9a646; color: #1a1a2e; }
  .msg { min-height: 1.2em; margin-top: 10px; line-height: 1.5; }
  .err { color: #ff8a80; }
  .ok { color: #8be78b; }
  .hidden { display: none; }
  .small { font-size: .88em; color: #9aa0b8; }
  .hp { position: absolute; left: -10000px; width: 1px; height: 1px; overflow: hidden; }
</style>
</head>
<body>
<main>
  <h1>Account password</h1>
  <nav class="tabs"><a href="#forgot" id="tabForgot">Forgot password?</a><a href="#change" id="tabChange">Change password</a></nav>

  <div class="card" id="forgot">
    <h2>Forgot your password?</h2>
    <p>Accounts here have no e-mail, so a GM resets a password by hand. Tell us your account and how to reach you: a GM
      checks it is you (they may ask about your characters) and gives you a temporary password. You set your own the first
      time you use it, on the Change password form.</p>
    <form id="forgotForm" autocomplete="off">
      <label for="fa">Account name (the one you log in with)</label>
      <input id="fa" required maxlength="32" autocomplete="username">
      <label for="fc">How can a GM reach you?</label>
      <input id="fc" required maxlength="200" placeholder="Your Discord name, or a character's name and level">
      <label for="fn">Anything else (optional)</label>
      <textarea id="fn" maxlength="500" placeholder="Your characters, when you last played..."></textarea>
      <div class="hp" aria-hidden="true"><label for="fw">Leave this empty</label><input id="fw" tabindex="-1" autocomplete="off"></div>
      <button type="submit">Ask for a new password</button>
      <div class="msg" id="forgotMsg" role="alert"></div>
    </form>
  </div>

  <div class="card hidden" id="change">
    <h2>Change your password</h2>
    <p id="tempNote" class="hidden ok">Your password is a temporary one from a GM: set your own here first.</p>
    <form id="changeForm">
      <label for="ca">Account name</label>
      <input id="ca" required maxlength="32" autocomplete="username">
      <label for="cc">Current password (or the temporary one a GM gave you)</label>
      <input id="cc" type="password" required maxlength="64" autocomplete="current-password">
      <label for="cp">New password (4 to 31 characters)</label>
      <input id="cp" type="password" required minlength="4" maxlength="31" autocomplete="new-password">
      <label for="cq">New password again</label>
      <input id="cq" type="password" required minlength="4" maxlength="31" autocomplete="new-password">
      <button type="submit">Set my password</button>
      <div class="msg" id="changeMsg" role="alert"></div>
    </form>
  </div>
  <p class="small">The game and these pages share one password.</p>
</main>
<script>
(function () {
  'use strict';
  var base = location.pathname.indexOf('/eqoa/') === 0 ? '/eqoa/account/api/password/' : '/api/password/';
  var $ = function (id) { return document.getElementById(id); };
  function call(path, body) {
    return fetch(base + path, { method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-EQOA-Account': '1' }, body: JSON.stringify(body) })
      .then(function (r) { return r.json().catch(function () { return { ok: false, error: 'The server did not answer: try again later' }; }); })
      .catch(function () { return { ok: false, error: 'Connection error: try again later' }; });
  }
  function say(id, text, ok) { var m = $(id); m.textContent = text || ''; m.className = 'msg ' + (ok ? 'ok' : 'err'); }
  function show() {
    var change = location.hash === '#change';
    $('forgot').classList.toggle('hidden', change);
    $('change').classList.toggle('hidden', !change);
    $('tabForgot').classList.toggle('on', !change);
    $('tabChange').classList.toggle('on', change);
  }
  window.addEventListener('hashchange', show);
  var q = new URLSearchParams(location.search);
  if (q.get('account')) { $('ca').value = q.get('account'); $('fa').value = q.get('account'); }
  if (q.get('temporary')) { $('tempNote').classList.remove('hidden'); if (!location.hash) location.hash = '#change'; }
  show();
  $('forgotForm').addEventListener('submit', function (e) {
    e.preventDefault();
    var b = e.target.querySelector('button'); b.disabled = true; say('forgotMsg', '');
    call('forgot', { account: $('fa').value, contact: $('fc').value, note: $('fn').value, website: $('fw').value }).then(function (r) {
      b.disabled = false;
      if (!r.ok) { say('forgotMsg', r.error); return; }
      say('forgotMsg', r.message, true);
      e.target.reset();
    });
  });
  $('changeForm').addEventListener('submit', function (e) {
    e.preventDefault();
    if ($('cp').value !== $('cq').value) { say('changeMsg', 'The new passwords do not match'); return; }
    var b = e.target.querySelector('button'); b.disabled = true; say('changeMsg', '');
    call('change', { account: $('ca').value, current: $('cc').value, password: $('cp').value }).then(function (r) {
      b.disabled = false;
      if (!r.ok) { say('changeMsg', r.error); return; }
      say('changeMsg', r.message, true);
      $('tempNote').classList.add('hidden');
      $('cc').value = $('cp').value = $('cq').value = '';
    });
  });
})();
</script>
</body>
</html>`

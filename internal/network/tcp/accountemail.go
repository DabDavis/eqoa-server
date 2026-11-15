package tcp

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

// The account page's e-mail (the owner, 2026-11-15: "lets add email
// verification to accounts ... and we can setup email password reset";
// internal/accountmail). The routes sit under /api/password/ beside the
// password forms, so the public site's one location for them
// (/eqoa/account/api/password/) passes them on; the page is the password page,
// its E-mail tab (#email), and the two links' landings (#verify=<token>,
// #reset=<token>: a fragment, which no server or log sees).
//
//   - POST api/password/email {account, password, code, email}: the account's
//     password (the server's one limiter) and, when the account has an
//     authenticator on the GM panel, its code; then the address kept,
//     unverified, and a verify link mailed to it.
//   - POST api/password/email/status {account, password, code}: the address
//     whole and whether it is verified (its owner's alone to see).
//   - POST api/password/email/resend {account, password, code}: the link again,
//     within accountmail's limits.
//   - POST api/password/email/verify {token}: the link spent.
//   - POST api/password/reset {token, password}: a reset link spent, the
//     password set (the temporary flag cleared, an authenticator kept), the
//     account's page sessions ended as the Change form ends them.
//
// The Forgot form (accountpassword.go) mails a reset link to an account with a
// verified address and files a GM's request for one without, answering the
// same either way. Every route answers 404 while mail.accounts is off.

const emailBodyMost = 4 << 10

// emailWho is the account a form names and proves: the password, and the
// authenticator's code when the account has one. ok false has been answered.
func (s *AccountServer) emailWho(w http.ResponseWriter, r *http.Request, in *emailForm) (*database.AccountRecord, bool) {
	account, err := s.accountStore.GetByUsername(strings.TrimSpace(in.Account))
	if err != nil {
		account = nil // counted all the same: a guess at a name is a guess
	}
	if ok, refusal := s.checkPassword(r, account, in.Account, in.Password); !ok {
		if refusal == "" {
			refusal = "Invalid account name or password"
		}
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": refusal})
		return nil, false
	}
	if account.Banned {
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "This account is suspended"})
		return nil, false
	}
	if s.TOTP != nil {
		if enrolled, good := s.TOTP(account.ID, strings.TrimSpace(in.Code)); enrolled && !good {
			msg := "This account has an authenticator: type its current code"
			if strings.TrimSpace(in.Code) != "" {
				msg = "That authenticator code is wrong or used: type the current one"
			}
			pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": msg, "needCode": true})
			return nil, false
		}
	}
	return account, true
}

type emailForm struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Code     string `json:"code"`
	Email    string `json:"email"`
	Token    string `json:"token"`
}

// emailRead is the form of a write, or false (answered).
func (s *AccountServer) emailRead(w http.ResponseWriter, r *http.Request) (emailForm, bool) {
	var in emailForm
	if s.Mail == nil {
		http.NotFound(w, r)
		return in, false
	}
	if !pwWrite(r) {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Send it from the account page"})
		return in, false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, emailBodyMost)).Decode(&in); err != nil {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid request"})
		return in, false
	}
	return in, true
}

// emailFailed answers a refusal of accountmail's: a limit (429) or the
// player's mistake (400), else a failure of ours, logged.
func emailFailed(w http.ResponseWriter, what string, err error) {
	switch {
	case accountmail.IsLimit(err):
		pwJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": err.Error()})
	case errors.Is(err, database.ErrEmailToken):
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
	case errors.Is(err, database.ErrAccountNotFound):
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "Invalid account name or password"})
	case errors.Is(err, accountmail.ErrNoEmail):
		pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "This account has no e-mail address yet: set one below"})
	default:
		var bad *accountmail.BadAddress
		if errors.As(err, &bad) {
			pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": bad.Error()})
			return
		}
		log.Printf("AccountServer: %s: %v", what, err)
		pwJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "That did not work; try again in a moment"})
	}
}

// handleAPIEmailSet is POST /api/password/email.
func (s *AccountServer) handleAPIEmailSet(w http.ResponseWriter, r *http.Request) {
	in, ok := s.emailRead(w, r)
	if !ok {
		return
	}
	if _, err := mailer.Normalize(in.Email); err != nil {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	acc, ok := s.emailWho(w, r, &in)
	if !ok {
		return
	}
	masked, err := s.Mail.SetEmail(acc.ID, in.Email, s.getClientIP(r))
	if err != nil {
		emailFailed(w, "setting an e-mail", err)
		return
	}
	st, _ := s.db.AccountEmail(acc.ID)
	if st.Verified() && st.VerifiedBy == "link" {
		pwJSON(w, http.StatusOK, map[string]any{"ok": true, "email": masked, "verified": true,
			"message": "That is your address already, and it is verified."})
		return
	}
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "email": masked, "verified": false,
		"message": "Saved. We sent a link to " + masked + ": open it within 24 hours to verify the address (check spam too)."})
}

// handleAPIEmailStatus is POST /api/password/email/status: the address whole,
// to the account's owner.
func (s *AccountServer) handleAPIEmailStatus(w http.ResponseWriter, r *http.Request) {
	in, ok := s.emailRead(w, r)
	if !ok {
		return
	}
	acc, ok := s.emailWho(w, r, &in)
	if !ok {
		return
	}
	st, err := s.db.AccountEmail(acc.ID)
	if err != nil {
		emailFailed(w, "reading an e-mail", err)
		return
	}
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "email": st.Email, "verified": st.Verified(), "verifiedAt": st.VerifiedAt})
}

// handleAPIEmailResend is POST /api/password/email/resend.
func (s *AccountServer) handleAPIEmailResend(w http.ResponseWriter, r *http.Request) {
	in, ok := s.emailRead(w, r)
	if !ok {
		return
	}
	acc, ok := s.emailWho(w, r, &in)
	if !ok {
		return
	}
	masked, err := s.Mail.SendVerify(acc.ID, s.getClientIP(r), false)
	if err != nil {
		emailFailed(w, "resending a link", err)
		return
	}
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "email": masked,
		"message": "We sent the link again to " + masked + ": it works for 24 hours (check spam too)."})
}

// handleAPIEmailVerify is POST /api/password/email/verify.
func (s *AccountServer) handleAPIEmailVerify(w http.ResponseWriter, r *http.Request) {
	in, ok := s.emailRead(w, r)
	if !ok {
		return
	}
	st, err := s.Mail.Verify(in.Token)
	if err != nil {
		emailFailed(w, "verifying an e-mail", err)
		return
	}
	pwJSON(w, http.StatusOK, map[string]any{"ok": true, "email": mailer.Mask(st.Email),
		"message": "Thank you: " + mailer.Mask(st.Email) + " is verified for account " + st.Username + "."})
}

// handleAPIPasswordReset is POST /api/password/reset: a reset link's new
// password.
func (s *AccountServer) handleAPIPasswordReset(w http.ResponseWriter, r *http.Request) {
	in, ok := s.emailRead(w, r)
	if !ok {
		return
	}
	if len(in.Password) < 4 || len(in.Password) > auth.MaxPasswordLen {
		pwJSON(w, http.StatusBadRequest, map[string]any{"ok": false,
			"error": "A password is 4 to 31 characters: the game's login carries no more"})
		return
	}
	hash, err := auth.HashPassword(in.Password, s.bcryptCost)
	if err != nil {
		emailFailed(w, "hashing a password", err)
		return
	}
	st, err := s.Mail.Reset(in.Token, hash)
	if err != nil {
		if !errors.Is(err, database.ErrEmailToken) && strings.Contains(err.Error(), "suspended") {
			pwJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "This account is suspended"})
			return
		}
		emailFailed(w, "resetting a password", err)
		return
	}
	// as the Change form: a session made with the old password goes with it
	s.endWebSessions(st.AccountID)
	log.Printf("AccountServer: '%s' set a new password by an e-mailed link from %s", st.Username, s.getClientIP(r))
	pwJSON(w, http.StatusOK, map[string]any{"ok": true,
		"message": "Your password is set for account " + st.Username + ": use it in the game and on these pages from now on."})
}

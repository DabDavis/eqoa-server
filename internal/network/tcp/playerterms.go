package tcp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const PlayerTermsVersion = "1.4"
const playerTermsAddress = "eqoaclassic.com/terms"
const playerTermsURL = "https://" + playerTermsAddress
const playerTermsInstruction = "Use " + playerTermsAddress + " to create or accept for your account."
const eligibilityCookie = "eqoa_terms_eligibility"

// The versioned files preserve the wording associated with a receipt.
//
//go:embed playerpolicy/*.html
var playerPolicy embed.FS

var policyPage = template.Must(template.ParseFS(playerPolicy, "playerpolicy/*.html"))

type playerTermsPortal struct {
	authority bool
	key       [32]byte
}

// EnablePlayerTerms installs the public documents and permits account writes
// only on the shard whose accountinfo is authoritative.
func (s *AccountServer) EnablePlayerTerms(authority bool) {
	p := &playerTermsPortal{authority: authority}
	rand.Read(p.key[:])
	s.playerTerms = p
}

type policyView struct {
	Stage, Message, Proof, Version, Username, Email string
	Signup, Mail                                    bool
}

func (s *AccountServer) policyLinks() string {
	if s.playerTerms == nil {
		return ""
	}
	return `<a href="https://eqoaclassic.com/terms">Read and accept Player Terms</a> <a href="https://eqoaclassic.com/privacy">Privacy Notice</a>`
}

func writePolicy(w http.ResponseWriter, status int, v policyView) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	v.Version = PlayerTermsVersion
	if err := policyPage.ExecuteTemplate(w, "page.html", v); err != nil {
		log.Printf("Player Terms: rendering page: %v", err)
	}
}

func (s *AccountServer) handlePlayerPrivacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writePolicy(w, http.StatusOK, policyView{Stage: "privacy"})
}

func (p *playerTermsPortal) signature(value string) string {
	mac := hmac.New(sha256.New, p.key[:])
	mac.Write([]byte(PlayerTermsVersion + "|" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

// The signed, short-lived cookie contains the outcome and a random nonce. Its
// copy in the form binds a decision to this browser without retaining exact ages.
func (p *playerTermsPortal) setEligibility(w http.ResponseWriter, r *http.Request, outcome string, ttl time.Duration) {
	var nonce [16]byte
	rand.Read(nonce[:])
	value := outcome + "." + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10) + "." + hex.EncodeToString(nonce[:])
	http.SetCookie(w, &http.Cookie{Name: eligibilityCookie, Value: value + "." + p.signature(value),
		Path: "/terms", MaxAge: int(ttl.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")})
}

func finishTermsForm(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: eligibilityCookie, Path: "/terms", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")})
}

func (p *playerTermsPortal) eligibility(r *http.Request) (outcome, proof string) {
	cookie, err := r.Cookie(eligibilityCookie)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 4 || !hmac.Equal([]byte(parts[3]), []byte(p.signature(strings.Join(parts[:3], ".")))) {
		return "", ""
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expires {
		return "", ""
	}
	return parts[0], cookie.Value
}

func (s *AccountServer) handlePlayerTerms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !s.playerTerms.authority {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			http.Redirect(w, r, playerTermsURL, http.StatusSeeOther)
		} else {
			http.Error(w, playerTermsInstruction, http.StatusForbidden)
		}
		return
	}
	outcome, proof := s.playerTerms.eligibility(r)
	v := policyView{Stage: "age", Proof: proof, Mail: s.Mail != nil, Signup: r.URL.Query().Get("mode") == "create"}
	switch outcome {
	case "eligible":
		v.Stage = "account"
	case "ineligible":
		v.Stage = "ineligible"
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writePolicy(w, http.StatusOK, v)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && u.Scheme != "http") {
			http.Error(w, "Open the account page to continue.", http.StatusForbidden)
			return
		}
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "Open the account page to continue.", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the form.", http.StatusBadRequest)
		return
	}
	if outcome == "ineligible" {
		writePolicy(w, http.StatusForbidden, v)
		return
	}
	form := r.PostForm // credentials, age and assent are never taken from the URL
	if form.Get("step") == "age" {
		age, err := strconv.Atoi(form.Get("age"))
		if err != nil || age < 0 || age > 130 {
			v.Message = "Enter your age in years."
			writePolicy(w, http.StatusBadRequest, v)
			return
		}
		if age < 13 {
			s.playerTerms.setEligibility(w, r, "ineligible", 24*time.Hour)
			writePolicy(w, http.StatusForbidden, policyView{Stage: "ineligible"})
			return
		}
		s.playerTerms.setEligibility(w, r, "eligible", 30*time.Minute)
		destination := "/terms"
		if form.Get("mode") == "create" {
			destination += "?mode=create"
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	if outcome != "eligible" || !hmac.Equal([]byte(form.Get("proof")), []byte(proof)) {
		v.Message = "Reopen the account page and complete the age check."
		writePolicy(w, http.StatusForbidden, v)
		return
	}
	if form.Get("step") == "decline" {
		finishTermsForm(w, r)
		writePolicy(w, http.StatusOK, policyView{Stage: "declined"})
		return
	}
	v.Signup = form.Get("step") == "create"
	v.Username, v.Email = form.Get("username"), form.Get("email")
	if (!v.Signup && form.Get("step") != "accept") || form.Get("agree") != PlayerTermsVersion {
		v.Message = "Read the current Player Terms and select the agreement box to continue."
		writePolicy(w, http.StatusBadRequest, v)
		return
	}
	if v.Signup {
		reply, err := s.registerAccount(s.getClientIP(r), v.Username, form.Get("password"), v.Email, PlayerTermsVersion)
		if err != nil {
			v.Message = err.Error()
			writePolicy(w, http.StatusBadRequest, v)
			return
		}
		v.Message = reply.Message
	} else {
		account, refusal := s.CheckLogin(r, v.Username, form.Get("password"))
		if account == nil {
			v.Message = refusal
			writePolicy(w, http.StatusForbidden, v)
			return
		}
		if err := s.db.AcceptPlayerTerms(account, PlayerTermsVersion); err != nil {
			log.Printf("Player Terms: recording acceptance: %v", err)
			v.Message = "We could not record your agreement. Please try signing in again."
			writePolicy(w, http.StatusServiceUnavailable, v)
			return
		}
	}
	v.Stage = "accepted"
	finishTermsForm(w, r)
	writePolicy(w, http.StatusOK, v)
}

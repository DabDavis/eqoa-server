// Package accountmail is the accounts' e-mail (the owner, 2026-11-15: "lets
// add email verification to accounts, so we can make sure players dont lose
// their account. and we can setup email password reset"): an address set on
// an account and confirmed by a link mailed to it, a forgotten password reset
// by a link mailed to a confirmed address, and the requirement that every
// account has one (Gate).
//
// The links' tokens are random, kept only hashed (database/emailverify.go),
// good once: a verify link for 24 hours, a reset link for one. Mail goes out
// through internal/mailer's queue, in the background: no page or login waits
// on the relay, and a relay that is down costs a log line.
//
// An address is its owner's alone to see: the pages show it whole only to who
// gave the account's password, the game and the GM panel show mailer.Mask of
// it, and the log never has more than the mask.
package accountmail

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mailer"
)

// The links' lives and the sends' limits.
const (
	VerifyTTL = 24 * time.Hour
	ResetTTL  = time.Hour

	// SendGap is the least time between two links of a kind to one account
	// (a GM's resend aside).
	SendGap = 2 * time.Minute
	// VerifyPerDay is the most verify links a day an account gets, and an
	// address (over every account naming it).
	VerifyPerDay = 5
	// ResetPerDay is the most reset links a day an account gets.
	ResetPerDay = 3
	// PerHourFromAddr is the most mails a sender's network address asks for
	// an hour (kept in memory).
	PerHourFromAddr = 10
)

// Defaults of the mail block.
const (
	DefaultSMTP     = "127.0.0.1:25"
	DefaultFrom     = "no-reply@eqoaclassic.com"
	DefaultBaseURL  = "https://eqoaclassic.com"
	DefaultPagePath = "/eqoa/password"
	DefaultRemind   = 30 * time.Minute
)

// LimitError is a send refused by a limit: the player is told to wait.
type LimitError struct{ Msg string }

func (e *LimitError) Error() string { return e.Msg }

// BadAddress is an address mailer.Normalize refused: the player's to fix.
type BadAddress struct{ Err error }

func (b *BadAddress) Error() string { return b.Err.Error() }

// ErrNoEmail is a link asked for an account with no address.
var ErrNoEmail = errors.New("the account has no e-mail address: add one first")

// IsLimit says whether err is a LimitError.
func IsLimit(err error) bool {
	var l *LimitError
	return errors.As(err, &l)
}

// Filled is cfg with its defaults in.
func Filled(cfg config.MailConfig) config.MailConfig {
	if cfg.SMTP == "" {
		cfg.SMTP = DefaultSMTP
	}
	if cfg.From == "" {
		cfg.From = DefaultFrom
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.PagePath == "" {
		cfg.PagePath = DefaultPagePath
	}
	if !strings.HasPrefix(cfg.PagePath, "/") {
		cfg.PagePath = "/" + cfg.PagePath
	}
	if cfg.RemindEvery <= 0 {
		cfg.RemindEvery = DefaultRemind
	}
	return cfg
}

// Short is the page as a player types it: the site's host and the page's
// path, no scheme ("eqoaclassic.com/eqoa/password").
func Short(cfg config.MailConfig) string {
	cfg = Filled(cfg)
	host := cfg.BaseURL
	if u, err := url.Parse(cfg.BaseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return host + cfg.PagePath
}

// Service mails the links and spends them, for the process that owns the
// accounts (the account pages and the Station login).
type Service struct {
	db    *database.SQLiteDB
	cfg   config.MailConfig
	queue *mailer.Queue
	// Now is the clock (tests set it); nil: time.Now.
	Now func() time.Time

	mu     sync.Mutex
	byAddr map[string][]time.Time
}

// New is the service over db with cfg's settings, sending through sender
// (nil: SMTP to cfg.smtp when cfg.enabled, else the log).
func New(db *database.SQLiteDB, cfg config.MailConfig, sender mailer.Sender) *Service {
	cfg = Filled(cfg)
	if sender == nil {
		if cfg.Enabled {
			sender = mailer.SMTP{Addr: cfg.SMTP, From: cfg.From}
		} else {
			sender = mailer.Log{Bodies: cfg.LogBodies}
		}
	}
	return &Service{db: db, cfg: cfg, queue: mailer.NewQueue(sender, 0, nil), byAddr: map[string][]time.Time{}}
}

// NewWithQueue is New with a queue of the caller's (tests: no backoff).
func NewWithQueue(db *database.SQLiteDB, cfg config.MailConfig, q *mailer.Queue) *Service {
	return &Service{db: db, cfg: Filled(cfg), queue: q, byAddr: map[string][]time.Time{}}
}

// Close stops the queue.
func (s *Service) Close() {
	if s != nil && s.queue != nil {
		s.queue.Stop()
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// PageURL is the account page's address, whole.
func (s *Service) PageURL() string { return s.cfg.BaseURL + s.cfg.PagePath }

// PageShort is Short of the service's settings.
func (s *Service) PageShort() string { return Short(s.cfg) }

// addrKey is the network address a sender is counted by: an IPv4 address
// whole, an IPv6 one by its /64.
func addrKey(ip string) string {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return strings.TrimSpace(ip)
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// allowFrom counts one mail asked for from ip and says whether it is within
// PerHourFromAddr. An empty ip is not counted.
func (s *Service) allowFrom(ip string) bool {
	if ip == "" {
		return true
	}
	key := addrKey(ip)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	cut := now.Add(-time.Hour)
	if len(s.byAddr) > 4096 {
		for k, ts := range s.byAddr {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(s.byAddr, k)
			}
		}
	}
	ts := s.byAddr[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= PerHourFromAddr {
		s.byAddr[key] = ts
		return false
	}
	s.byAddr[key] = append(ts, now)
	return true
}

// SetEmail keeps raw (normalised) as the account's address, unconfirmed
// unless it is the one it has, and mails it a verify link. It answers the
// address masked; a refusal's error is the player's to read.
func (s *Service) SetEmail(accountID int, raw, ip string) (string, error) {
	email, err := mailer.Normalize(raw)
	if err != nil {
		return "", &BadAddress{err}
	}
	cur, err := s.db.AccountEmail(accountID)
	if err != nil {
		return "", err
	}
	if cur.Email == email && cur.Verified() && cur.VerifiedBy == "link" {
		return mailer.Mask(email), nil
	}
	// the new address's own limit before anything changes
	n, err := s.db.EmailSendsSince(accountID, email, database.EmailTokenVerify, s.now().Add(-24*time.Hour))
	if err != nil {
		return "", err
	}
	if n.ToAddress >= VerifyPerDay {
		return "", &LimitError{"That address has had a lot of mail from us today: try again tomorrow"}
	}
	if err := s.db.SetAccountEmail(accountID, email, s.now()); err != nil {
		return "", err
	}
	log.Printf("Accounts: account %d's e-mail set to %s (unverified)", accountID, mailer.Mask(email))
	return s.SendVerify(accountID, ip, false)
}

// SendVerify mails the account's address a verify link, within the limits
// (byGM: a GM's resend, which no per-account limit holds back). It answers
// the address masked.
func (s *Service) SendVerify(accountID int, ip string, byGM bool) (string, error) {
	e, err := s.db.AccountEmail(accountID)
	if err != nil {
		return "", err
	}
	if e.Email == "" {
		return "", ErrNoEmail
	}
	if e.Verified() && e.VerifiedBy == "link" {
		return mailer.Mask(e.Email), &LimitError{"That address is verified already"}
	}
	now := s.now()
	n, err := s.db.EmailSendsSince(accountID, e.Email, database.EmailTokenVerify, now.Add(-24*time.Hour))
	if err != nil {
		return "", err
	}
	switch {
	case !byGM && !n.Last.IsZero() && now.Sub(n.Last) < SendGap:
		return "", &LimitError{fmt.Sprintf("A link went out %s ago: give it a minute or two to arrive (check spam too)",
			now.Sub(n.Last).Round(time.Second))}
	case !byGM && n.ForAccount >= VerifyPerDay:
		return "", &LimitError{"That is a lot of links for one day: try again tomorrow, or ask a GM"}
	case n.ToAddress >= VerifyPerDay:
		return "", &LimitError{"That address has had a lot of mail from us today: try again tomorrow"}
	case !byGM && !s.allowFrom(ip):
		return "", &LimitError{"That is a lot of mail asked for from here: try again in an hour"}
	}
	token, err := s.db.NewEmailToken(accountID, database.EmailTokenVerify, e.Email, VerifyTTL, now)
	if err != nil {
		return "", err
	}
	link := s.PageURL() + "#verify=" + token
	s.queue.Post(mailer.Message{To: e.Email, Subject: "Verify your EQOA Classic e-mail address", Text: fmt.Sprintf(
		"Hello %s,\n\n"+
			"This address was set as the e-mail of the EQOA Classic account \"%s\". Open this link within 24 hours to verify it:\n\n"+
			"%s\n\n"+
			"A verified address keeps your account yours: with it you can reset a forgotten password by e-mail.\n\n"+
			"If this was not you, ignore this mail: nothing changes until the link is opened.\n",
		e.Username, e.Username, link)})
	log.Printf("Accounts: a verify link for account %d sent to %s", accountID, mailer.Mask(e.Email))
	return mailer.Mask(e.Email), nil
}

// Verify spends a verify link.
func (s *Service) Verify(token string) (database.EmailState, error) {
	e, err := s.db.VerifyEmail(token, s.now())
	if err == nil {
		log.Printf("Accounts: account %d (%s) verified its e-mail %s", e.AccountID, e.Username, mailer.Mask(e.Email))
	}
	return e, err
}

// HasVerifiedEmail is whether e can be sent a reset link: an address
// confirmed by its own link (a GM's override with no address cannot).
func HasVerifiedEmail(e database.EmailState) bool {
	return e.Email != "" && e.Verified()
}

// SendReset mails the account's verified address a reset link, within the
// limits, and says whether it did; why not is logged, never told (the forgot
// forms answer the same whatever happens). typed, when not empty, is an
// address the player gave, which must be the account's.
func (s *Service) SendReset(e database.EmailState, typed, ip string) bool {
	if !HasVerifiedEmail(e) || e.Banned {
		return false
	}
	if typed != "" {
		if t, err := mailer.Normalize(typed); err != nil || t != e.Email {
			log.Printf("Accounts: a reset for account %d named another address: not sent", e.AccountID)
			return false
		}
	}
	now := s.now()
	n, err := s.db.EmailSendsSince(e.AccountID, "", database.EmailTokenReset, now.Add(-24*time.Hour))
	if err != nil {
		log.Printf("Accounts: a reset for account %d: %v", e.AccountID, err)
		return false
	}
	if (!n.Last.IsZero() && now.Sub(n.Last) < SendGap) || n.ForAccount >= ResetPerDay || !s.allowFrom(ip) {
		log.Printf("Accounts: a reset for account %d held back by its limits (%d today)", e.AccountID, n.ForAccount)
		return false
	}
	token, err := s.db.NewEmailToken(e.AccountID, database.EmailTokenReset, e.Email, ResetTTL, now)
	if err != nil {
		log.Printf("Accounts: a reset for account %d: %v", e.AccountID, err)
		return false
	}
	link := s.PageURL() + "#reset=" + token
	s.queue.Post(mailer.Message{To: e.Email, Subject: "Reset your EQOA Classic password", Text: fmt.Sprintf(
		"Hello %s,\n\n"+
			"Someone asked to reset the password of the EQOA Classic account \"%s\". Open this link within an hour to choose a new one:\n\n"+
			"%s\n\n"+
			"If you did not ask, ignore this mail: your password stays as it is.\n",
		e.Username, e.Username, link)})
	log.Printf("Accounts: a reset link for account %d sent to %s", e.AccountID, mailer.Mask(e.Email))
	return true
}

// Reset spends a reset link: hash is the account's password from now.
func (s *Service) Reset(token, hash string) (database.EmailState, error) {
	e, err := s.db.ResetPasswordByEmail(token, hash, s.now())
	if err == nil {
		log.Printf("Accounts: account %d (%s) set a new password by an e-mailed link", e.AccountID, e.Username)
	}
	return e, err
}

// Gate is the requirement that every account has a verified address: a
// reminder in the world before Cutoff, a refused login from it on. It reads
// the process's own accounts, so a shard's copy serves it.
type Gate struct {
	DB interface {
		EmailVerified(accountID int) (bool, error)
	}
	// Cutoff is when an unverified account stops entering the world; zero:
	// never (reminders only).
	Cutoff time.Time
	// Every is the time between reminders to a player in the world.
	Every time.Duration
	// Page is the account page as a player types it.
	Page string
}

// ParseCutoff reads mail.require_after: a date (UTC midnight) or an RFC 3339
// time; empty is zero.
func ParseCutoff(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("mail.require_after %q is no date (2006-01-02)", s)
	}
	return t.UTC(), nil
}

// NewGate is the requirement as cfg sets it, or nil when it sets none
// (mail.accounts or mail.require_verified off).
func NewGate(db interface {
	EmailVerified(accountID int) (bool, error)
}, cfg config.MailConfig) (*Gate, error) {
	if !cfg.Accounts || !cfg.RequireVerified {
		return nil, nil
	}
	cut, err := ParseCutoff(cfg.RequireAfter)
	if err != nil {
		return nil, err
	}
	cfg = Filled(cfg)
	return &Gate{DB: db, Cutoff: cut, Every: cfg.RemindEvery, Page: Short(cfg)}, nil
}

// unverified is whether the account has no verified address; a read that
// fails counts as verified (nobody is locked out by a database hiccup).
func (g *Gate) unverified(accountID int) bool {
	if g == nil || accountID <= 0 {
		return false
	}
	ok, err := g.DB.EmailVerified(accountID)
	if err != nil {
		log.Printf("Accounts: account %d's e-mail state: %v (let in)", accountID, err)
		return false
	}
	return !ok
}

// Enforced is whether now is past the cutoff.
func (g *Gate) Enforced(now time.Time) bool {
	return g != nil && !g.Cutoff.IsZero() && !now.Before(g.Cutoff)
}

// MaxRefusal is the longest text STATION's login reply holds (63 and a NUL).
const MaxRefusal = 63

// Refusal is what a login of the account is told when it may not enter the
// world now, or "". It fits STATION's reply (MaxRefusal).
func (g *Gate) Refusal(accountID int, now time.Time) string {
	if !g.Enforced(now) || !g.unverified(accountID) {
		return ""
	}
	return RefusalText(g.Page)
}

// RefusalText is the refusal naming page, cut to MaxRefusal.
func RefusalText(page string) string {
	t := "Verify your e-mail first at " + page
	if len(t) > MaxRefusal {
		t = "Verify your e-mail first: " + page
	}
	if len(t) > MaxRefusal {
		t = t[:MaxRefusal]
	}
	return t
}

// Reminder is the line the world says to an unverified account before the
// cutoff, or "".
func (g *Gate) Reminder(accountID int, now time.Time) string {
	if g == nil || g.Enforced(now) || !g.unverified(accountID) {
		return ""
	}
	t := "Your account has no verified e-mail address. Add one at " + g.Page +
		" (the E-mail tab) so you can never lose your account"
	if !g.Cutoff.IsZero() {
		t += ": from " + g.Cutoff.Format("January 2, 2006") + " an account without one cannot log in"
	}
	return t + "."
}
